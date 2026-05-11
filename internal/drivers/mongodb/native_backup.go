package mongodb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kronos/kronos/internal/drivers"
)

type mongoBackupCollection struct {
	Database string
	Name     string
	Type     string
}

func mongoNativeBackupFull(ctx context.Context, target drivers.Target, w drivers.RecordWriter, queryer mongoQueryer) (drivers.ResumePoint, error) {
	_, err := queryer.SimpleQuery(ctx, target, "ping")
	if err != nil {
		return drivers.ResumePoint{}, err
	}

	database := databaseName(target)
	collections, err := mongoListBackupCollections(ctx, target, queryer, database)
	if err != nil {
		return drivers.ResumePoint{}, err
	}

	var payload bytes.Buffer
	var totalDocs int64

	payload.WriteString("use " + database + "\n\n")

	for _, coll := range collections {
		if coll.Type != "collection" {
			continue
		}

		collInfo, err := mongoBackupCollectionInfoData(ctx, target, queryer, coll.Database, coll.Name)
		if err != nil {
			return drivers.ResumePoint{}, err
		}

		payload.WriteString("// " + strings.Repeat("-", 60) + "\n")
		payload.WriteString("// Collection: " + coll.Database + "." + coll.Name + "\n")
		payload.WriteString("// " + strings.Repeat("-", 60) + "\n\n")

		payload.WriteString(collInfo.CreateStatement)
		payload.WriteString("\n\n")

		count, docs, err := mongoBackupCollectionData(ctx, target, queryer, &payload, coll.Database, coll.Name)
		if err != nil {
			return drivers.ResumePoint{}, err
		}
		totalDocs += count

		if count > 0 {
			payload.WriteString("// " + fmt.Sprintf("%d", count) + " documents in " + coll.Name + "\n")
			for _, doc := range docs {
				payload.WriteString("db." + coll.Name + ".insertOne(" + doc + ");\n")
			}
			payload.WriteString("\n")
		}
	}

	obj := drivers.ObjectRef{Name: database, Kind: databaseObjectKind}
	if err := w.WriteRecord(obj, payload.Bytes()); err != nil {
		return drivers.ResumePoint{}, err
	}
	if err := w.FinishObject(obj, totalDocs); err != nil {
		return drivers.ResumePoint{}, err
	}

	return drivers.ResumePoint{Driver: "mongodb", Position: "mongodb:native"}, nil
}

type mongoBackupCollectionInfo struct {
	CreateStatement string
	Indexes         []string
}

func mongoListBackupCollections(ctx context.Context, target drivers.Target, queryer mongoQueryer, database string) ([]mongoBackupCollection, error) {
	result, err := queryer.SimpleQuery(ctx, target, "listCollections")
	if err != nil {
		return nil, err
	}

	var collections []mongoBackupCollection

	if len(result.Rows) == 0 {
		return collections, nil
	}

	if len(result.Rows) > 0 {
		row := result.Rows[0]
		if cursor, ok := row["cursor"]; ok {
			if cursorDoc, ok := cursor.Data.(map[string]interface{}); ok {
				if firstBatch, ok := cursorDoc["firstBatch"].([]interface{}); ok {
					for _, c := range firstBatch {
						if doc, ok := c.(map[string]interface{}); ok {
							coll := mongoBackupCollection{Database: database}

							if name, ok := doc["name"].(string); ok {
								coll.Name = name
							}
							if tp, ok := doc["type"].(string); ok {
								coll.Type = tp
							} else {
								coll.Type = "collection"
							}

							collections = append(collections, coll)
						}
					}
				}
			}
		}
	}

	return collections, nil
}

func mongoBackupCollectionInfoData(ctx context.Context, target drivers.Target, queryer mongoQueryer, db, coll string) (mongoBackupCollectionInfo, error) {
	info := mongoBackupCollectionInfo{Indexes: []string{}}

	createResult, err := queryer.SimpleQuery(ctx, target, fmt.Sprintf("listIndexes.%s.%s", db, coll))
	if err != nil {
		return info, err
	}

	for _, idxDoc := range createResult.Rows {
		if idxDoc == nil {
			continue
		}
		if nameVal, ok := idxDoc["name"]; ok {
			if name, ok := nameVal.Data.(string); ok && name != "_id_" {
				if _, ok := idxDoc["v"]; ok {
					if spec, ok := idxDoc["spec"]; ok {
						info.Indexes = append(info.Indexes, fmt.Sprintf("db.%s.createIndex(%v, {v: %v})", coll, name, spec))
					}
				}
			}
		}
	}

	return info, nil
}

func mongoBackupCollectionData(ctx context.Context, target drivers.Target, queryer mongoQueryer, w *bytes.Buffer, db, coll string) (int64, []string, error) {
	var count int64
	var docs []string

	cursor, err := queryer.Find(ctx, target, db, coll, nil, nil, 100)
	if err != nil {
		return 0, nil, err
	}
	defer cursor.Close()

	for {
		doc, err := cursor.Next()
		if err != nil {
			if err.Error() == "EOF" || strings.Contains(err.Error(), "EOF") {
				break
			}
			return count, docs, err
		}

		if docJSON, err := mongoDocToJSON(doc); err == nil {
			docs = append(docs, docJSON)
			count++
		}

		if count >= 1000 {
			break
		}
	}

	return count, docs, nil
}

func mongoDocToJSON(doc map[string]bsonValue) (string, error) {
	out := make(map[string]interface{})
	for k, v := range doc {
		out[k] = mongoValueToInterface(v)
	}

	data, err := json.Marshal(out)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func mongoValueToInterface(val bsonValue) interface{} {
	switch v := val.Data.(type) {
	case nil:
		return nil
	case float64:
		return v
	case string:
		return v
	case bool:
		return v
	case int32:
		return v
	case int64:
		return v
	case []byte:
		return v
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = mongoValueToInterface(bsonValue{Type: 0, Data: item})
		}
		return result
	case map[string]interface{}:
		return v
	default:
		if val.Type == 3 || val.Type == 4 {
			if m, ok := v.(map[string]interface{}); ok {
				out := make(map[string]interface{})
				for k, iv := range m {
					out[k] = mongoValueToInterface(bsonValue{Type: 0, Data: iv})
				}
				return out
			}
		}
		return v
	}
}

func mongoBSONToJSON(data []byte) (string, error) {
	doc, _ := parseBSONDocument(data)
	out := make(map[string]interface{})
	for k, v := range doc {
		out[k] = mongoValueToInterface(v)
	}
	data, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

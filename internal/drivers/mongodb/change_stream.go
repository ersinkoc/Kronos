package mongodb

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kronos/kronos/internal/drivers"
	"github.com/kronos/kronos/internal/manifest"
)

func mongoNativeBackupIncremental(ctx context.Context, target drivers.Target, parent manifest.Manifest, w drivers.RecordWriter, queryer mongoQueryer) (drivers.ResumePoint, error) {
	if !mongoOplogEnabled(target) {
		return drivers.ResumePoint{}, drivers.ErrIncrementalUnsupported
	}

	if queryer == nil {
		queryer = mongoRunner{}
	}

	database := databaseName(target)

	var resumeToken map[string]interface{}
	if parent.Streams != nil {
		if tokenStr, ok := parent.Streams["mongodb_resume_token"]; ok {
			resumeToken = parseResumeToken(tokenStr)
		}
	}

	if resumeToken != nil {
		return mongoCaptureChangeStreamResume(ctx, target, w, queryer, resumeToken)
	}

	return mongoCaptureOplogStart(ctx, target, queryer, database)
}

func mongoCaptureChangeStreamResume(ctx context.Context, target drivers.Target, w drivers.RecordWriter, queryer mongoQueryer, resumeToken map[string]interface{}) (drivers.ResumePoint, error) {
	position := formatResumeToken(resumeToken)
	return drivers.ResumePoint{
		Driver:   "mongodb",
		Position: position,
		Metadata: map[string]string{
			"resumeToken": position,
			"kind":        "change_stream",
		},
	}, nil
}

func mongoCaptureOplogStart(ctx context.Context, target drivers.Target, queryer mongoQueryer, database string) (drivers.ResumePoint, error) {
	result, err := queryer.GetServerStatus(ctx, target)
	if err != nil {
		return drivers.ResumePoint{}, err
	}

	var optime string

	if status, ok := result["repl"]; ok {
		if repl, ok := status.Data.(map[string]interface{}); ok {
			if opt, ok := repl["optime"]; ok {
				if optStr, ok := opt.(string); ok {
					optime = optStr
				}
			}
		}
	}

	position := fmt.Sprintf("mongodb:oplog:%s", optime)
	return drivers.ResumePoint{
		Driver:   "mongodb",
		Position: position,
		Metadata: map[string]string{
			"oplog_optime": optime,
			"kind":         "oplog",
		},
	}, nil
}

func mongoStreamChangeEvents(ctx context.Context, target drivers.Target, w drivers.StreamWriter, queryer mongoQueryer, resumeToken map[string]interface{}) (drivers.ResumePoint, error) {
	cursor, err := queryer.Watch(ctx, target, resumeToken)
	if err != nil {
		return drivers.ResumePoint{}, err
	}
	defer cursor.Close()

	var lastToken map[string]interface{}
	eventCount := 0

	for {
		event, err := cursor.Next()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return drivers.ResumePoint{}, err
		}

		if event == nil {
			continue
		}

		lastToken = event

		streamRecord := drivers.StreamRecord{
			ResumePoint: drivers.ResumePoint{
				Driver:   "mongodb",
				Position: formatResumeToken(event),
				Metadata: map[string]string{
					"kind": "change_stream",
				},
			},
			Payload: mustMarshalJSON(event),
		}

		if err := w.WriteStream(streamRecord); err != nil {
			return drivers.ResumePoint{}, err
		}

		eventCount++
	}

	if eventCount > 0 && lastToken != nil {
		return drivers.ResumePoint{
			Driver:   "mongodb",
			Position: formatResumeToken(lastToken),
			Metadata: map[string]string{
				"kind": "change_stream",
			},
		}, nil
	}

	return drivers.ResumePoint{}, nil
}

func formatResumeToken(token map[string]interface{}) string {
	if token == nil {
		return ""
	}
	if ts, ok := token["_id"].(map[string]interface{}); ok {
		if tsVal, ok := ts["timestamp"].(time.Time); ok {
			if clusterTime, ok := ts["clusterTime"].(time.Time); ok {
				return fmt.Sprintf("ts:%s,ct:%s", tsVal.Format(time.RFC3339), clusterTime.Format(time.RFC3339))
			}
			return tsVal.Format(time.RFC3339)
		}
	}
	return fmt.Sprintf("%v", token)
}

func mustMarshalJSON(v interface{}) []byte {
	if m, ok := v.(map[string]interface{}); ok {
		out := make(map[string]interface{})
		for k, val := range m {
			out[k] = mongoValueToInterface(bsonValue{Type: 0, Data: val})
		}
		if data, err := json.Marshal(out); err == nil {
			return data
		}
	}
	return nil
}

func mongoNativeStream(ctx context.Context, target drivers.Target, rp drivers.ResumePoint, w drivers.StreamWriter, queryer mongoQueryer) error {
	if rp.Metadata != nil {
		if resumeTokenStr := rp.Metadata["resumeToken"]; resumeTokenStr != "" {
			resumeToken := parseResumeToken(resumeTokenStr)
			if resumeToken != nil {
				_, err := mongoStreamChangeEvents(ctx, target, w, queryer, resumeToken)
				return err
			}
		}
	}

	if rp.Metadata != nil {
		if optime := rp.Metadata["oplog_optime"]; optime != "" {
			return mongoStreamOplog(ctx, target, w, queryer, optime)
		}
	}

	<-ctx.Done()
	return ctx.Err()
}

func mongoStreamOplog(ctx context.Context, target drivers.Target, w drivers.StreamWriter, queryer mongoQueryer, since string) error {
	return nil
}

func parseResumeToken(tokenStr string) map[string]interface{} {
	if tokenStr == "" {
		return nil
	}
	var token map[string]interface{}
	if err := json.Unmarshal([]byte(tokenStr), &token); err != nil {
		return nil
	}
	return token
}

func mongoNativeReplayStream(ctx context.Context, target drivers.Target, r drivers.StreamReader, targetPoint drivers.ReplayTarget, queryer mongoQueryer) error {
	for {
		record, err := r.NextStream()
		if err != nil {
			if err.Error() == "EOF" {
				return nil
			}
			return err
		}

		if targetPoint.Position != "" {
			if record.ResumePoint.Position >= targetPoint.Position {
				return nil
			}
		}
		if !targetPoint.Time.IsZero() && !record.ResumePoint.Time.IsZero() {
			if record.ResumePoint.Time.After(targetPoint.Time) {
				return nil
			}
		}

		if len(record.Payload) > 0 {
			if err := mongoExecuteChangeEvent(ctx, target, queryer, record.Payload); err != nil {
				return err
			}
		}
	}
}

func mongoExecuteChangeEvent(ctx context.Context, target drivers.Target, queryer mongoQueryer, payload []byte) error {
	var event map[string]interface{}
	if err := json.Unmarshal(payload, &event); err != nil {
		return err
	}

	operationType, ok := event["operationType"].(string)
	if !ok {
		return nil
	}

	ns, ok := event["ns"].(map[string]interface{})
	if !ok {
		return nil
	}
	db, _ := ns["db"].(string)
	coll, _ := ns["coll"].(string)

	if db == "" || coll == "" {
		return nil
	}

	switch operationType {
	case "insert":
		if doc, ok := event["fullDocument"].(map[string]interface{}); ok {
			return queryer.InsertOne(ctx, target, db, coll, doc)
		}
	case "update", "replace":
		if doc, ok := event["fullDocument"].(map[string]interface{}); ok {
			return queryer.InsertOne(ctx, target, db, coll, doc)
		}
	case "delete":
		return nil
	}

	return nil
}

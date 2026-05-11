package mongodb

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/kronos/kronos/internal/drivers"
)

type mongoConfig struct {
	Address   string
	Host      string
	Port      string
	Database  string
	Username  string
	Password  string
	AuthSource string
	TLSConfig string
	ReplicaSet string
}

type mongoDialer func(ctx context.Context, network string, address string) (net.Conn, error)

type mongoQueryer interface {
	SimpleQuery(ctx context.Context, target drivers.Target, query string) (mongoQueryResult, error)
	GetServerStatus(ctx context.Context, target drivers.Target) (map[string]bsonValue, error)
	ListDatabases(ctx context.Context, target drivers.Target) ([]string, error)
	ListCollections(ctx context.Context, target drivers.Target, db string) ([]string, error)
	GetCollectionStats(ctx context.Context, target drivers.Target, db, coll string) (map[string]bsonValue, error)
	Find(ctx context.Context, target drivers.Target, db, coll string, filter map[string]interface{}, projection map[string]interface{}, batchSize int) (*mongoCursor, error)
	InsertOne(ctx context.Context, target drivers.Target, db, coll string, doc map[string]interface{}) error
	Watch(ctx context.Context, target drivers.Target, resumeToken map[string]interface{}) (*mongoChangeStreamCursor, error)
}

type mongoRunner struct{}

func (mongoRunner) SimpleQuery(ctx context.Context, target drivers.Target, query string) (mongoQueryResult, error) {
	cfg, err := mongoConfigFromTarget(target)
	if err != nil {
		return mongoQueryResult{}, err
	}
	return mongoSimpleQueryWithDialer(ctx, cfg, query, defaultMongoDialer)
}

func (mongoRunner) GetServerStatus(ctx context.Context, target drivers.Target) (map[string]bsonValue, error) {
	cfg, err := mongoConfigFromTarget(target)
	if err != nil {
		return nil, err
	}
	return mongoGetServerStatusWithDialer(ctx, cfg, defaultMongoDialer)
}

func (mongoRunner) ListDatabases(ctx context.Context, target drivers.Target) ([]string, error) {
	cfg, err := mongoConfigFromTarget(target)
	if err != nil {
		return nil, err
	}
	return mongoListDatabasesWithDialer(ctx, cfg, defaultMongoDialer)
}

func (mongoRunner) ListCollections(ctx context.Context, target drivers.Target, db string) ([]string, error) {
	cfg, err := mongoConfigFromTarget(target)
	if err != nil {
		return nil, err
	}
	return mongoListCollectionsWithDialer(ctx, cfg, db, defaultMongoDialer)
}

func (mongoRunner) GetCollectionStats(ctx context.Context, target drivers.Target, db, coll string) (map[string]bsonValue, error) {
	cfg, err := mongoConfigFromTarget(target)
	if err != nil {
		return nil, err
	}
	return mongoGetCollectionStatsWithDialer(ctx, cfg, db, coll, defaultMongoDialer)
}

func (mongoRunner) Find(ctx context.Context, target drivers.Target, db, coll string, filter map[string]interface{}, projection map[string]interface{}, batchSize int) (*mongoCursor, error) {
	cfg, err := mongoConfigFromTarget(target)
	if err != nil {
		return nil, err
	}
	return mongoFindWithDialer(ctx, cfg, db, coll, filter, projection, batchSize, defaultMongoDialer)
}

func (mongoRunner) InsertOne(ctx context.Context, target drivers.Target, db, coll string, doc map[string]interface{}) error {
	cfg, err := mongoConfigFromTarget(target)
	if err != nil {
		return err
	}
	return mongoInsertOneWithDialer(ctx, cfg, db, coll, doc, defaultMongoDialer)
}

func (mongoRunner) Watch(ctx context.Context, target drivers.Target, resumeToken map[string]interface{}) (*mongoChangeStreamCursor, error) {
	cfg, err := mongoConfigFromTarget(target)
	if err != nil {
		return nil, err
	}
	return mongoWatchWithDialer(ctx, cfg, resumeToken, defaultMongoDialer)
}

type mongoCursor struct {
	id        int64
	ns        string
	batch     []map[string]bsonValue
	batchPos  int
	conn      net.Conn
	dialer    mongoDialer
	closed    bool
}

type mongoChangeStreamCursor struct {
	session   *mongoWireSession
	batch     []map[string]interface{}
	batchPos  int
	closed    bool
}

func (c *mongoChangeStreamCursor) Next() (map[string]interface{}, error) {
	if c.closed {
		return nil, fmt.Errorf("cursor closed")
	}

	for {
		if c.batchPos < len(c.batch) {
			doc := c.batch[c.batchPos]
			c.batchPos++
			return doc, nil
		}

		if c.session == nil {
			return nil, fmt.Errorf("EOF")
		}
	}
	return nil, fmt.Errorf("EOF")
}

func (c *mongoChangeStreamCursor) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	if c.session != nil {
		c.session.Close()
	}
	return nil
}

func (c *mongoCursor) Next() (map[string]bsonValue, error) {
	if c.closed {
		return nil, fmt.Errorf("cursor closed")
	}

	for {
		if c.batchPos < len(c.batch) {
			doc := c.batch[c.batchPos]
			c.batchPos++
			return doc, nil
		}

		if c.id == 0 {
			return nil, fmt.Errorf("EOF")
		}

		if err := c.getMore(); err != nil {
			return nil, err
		}
	}
}

func (c *mongoCursor) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	if c.id != 0 {
		c.killCursors()
	}
	if c.conn != nil {
		c.conn.Close()
	}
	return nil
}

func (c *mongoCursor) getMore() error {
	return nil
}

func (c *mongoCursor) killCursors() error {
	return nil
}

func mongoConfigFromTarget(target drivers.Target) (mongoConfig, error) {
	cfg := mongoConfig{
		Host:      "127.0.0.1",
		Port:      "27017",
		Database:  "admin",
		AuthSource: "admin",
	}

	if dsn := strings.TrimSpace(target.Connection["dsn"]); dsn != "" {
		parsed, err := url.Parse(dsn)
		if err != nil {
			return mongoConfig{}, fmt.Errorf("parse mongodb dsn: %w", err)
		}
		if parsed.Scheme != "mongodb" && parsed.Scheme != "mongodb+srv" {
			return mongoConfig{}, fmt.Errorf("unsupported mongodb dsn scheme %q", parsed.Scheme)
		}
		if host := parsed.Hostname(); host != "" {
			cfg.Host = host
		}
		if port := parsed.Port(); port != "" {
			cfg.Port = port
		}
		if database := strings.TrimPrefix(parsed.EscapedPath(), "/"); database != "" {
			cfg.Database = database
		}
		if parsed.User != nil {
			cfg.Username = parsed.User.Username()
			if password, ok := parsed.User.Password(); ok {
				cfg.Password = password
			}
		}
		if authSource := parsed.Query().Get("authSource"); authSource != "" {
			cfg.AuthSource = authSource
		}
		if replicaSet := parsed.Query().Get("replicaSet"); replicaSet != "" {
			cfg.ReplicaSet = replicaSet
		}
		if tls := parsed.Query().Get("tls"); tls != "" {
			cfg.TLSConfig = tls
		}
	}

	if addr := strings.TrimSpace(target.Connection["addr"]); addr != "" {
		host, port := splitAddress(addr)
		if host != "" {
			cfg.Host = host
		}
		if port != "" {
			cfg.Port = port
		}
	}
	if value := strings.TrimSpace(target.Connection["host"]); value != "" {
		cfg.Host = value
	}
	if value := strings.TrimSpace(target.Connection["port"]); value != "" {
		cfg.Port = value
	}
	if value := strings.TrimSpace(firstNonEmpty(target.Connection["database"], target.Options["database"])); value != "" {
		cfg.Database = value
	}
	if value := strings.TrimSpace(firstNonEmpty(target.Connection["username"], target.Connection["user"], target.Options["username"], target.Options["user"])); value != "" {
		cfg.Username = value
	}
	if value := firstNonEmpty(target.Connection["password"], target.Options["password"]); strings.TrimSpace(value) != "" {
		cfg.Password = value
	}
	if value := strings.TrimSpace(firstNonEmpty(target.Connection["authSource"], target.Connection["auth_source"], target.Options["authSource"], target.Options["auth_source"])); value != "" {
		cfg.AuthSource = value
	}
	cfg.Address = net.JoinHostPort(cfg.Host, cfg.Port)
	return cfg, nil
}

func mongoConfigAuthSource(cfg mongoConfig, target drivers.Target) string {
	if cfg.AuthSource != "" {
		return cfg.AuthSource
	}
	return strings.TrimSpace(firstNonEmpty(
		target.Connection["authSource"],
		target.Connection["auth_source"],
		target.Options["authSource"],
		target.Options["auth_source"],
		"admin",
	))
}

func mongoSimpleQueryWithDialer(ctx context.Context, cfg mongoConfig, query string, dial mongoDialer) (mongoQueryResult, error) {
	session, err := mongoDialWithAuth(ctx, cfg, dial)
	if err != nil {
		return mongoQueryResult{}, err
	}
	defer session.Close()

	result, err := session.query(ctx, query)
	if err != nil {
		return mongoQueryResult{}, err
	}

	return result, nil
}

func mongoGetServerStatusWithDialer(ctx context.Context, cfg mongoConfig, dial mongoDialer) (map[string]bsonValue, error) {
	session, err := mongoDialWithAuth(ctx, cfg, dial)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	result, err := session.query(ctx, "serverStatus")
	if err != nil {
		return nil, err
	}

	if len(result.Rows) == 0 {
		return nil, fmt.Errorf("serverStatus returned no documents")
	}

	return result.Rows[0], nil
}

func mongoListDatabasesWithDialer(ctx context.Context, cfg mongoConfig, dial mongoDialer) ([]string, error) {
	session, err := mongoDialWithAuth(ctx, cfg, dial)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	result, err := session.query(ctx, "listDatabases")
	if err != nil {
		return nil, err
	}

	if len(result.Rows) == 0 {
		return nil, fmt.Errorf("listDatabases returned no documents")
	}

	var dbs []string
	if len(result.Rows) > 0 {
		if datalist, ok := result.Rows[0]["databases"]; ok {
			if docs, ok := datalist.Data.([]interface{}); ok {
				for _, d := range docs {
					if doc, ok := d.(map[string]interface{}); ok {
						if name, ok := doc["name"].(string); ok {
							dbs = append(dbs, name)
						}
					}
				}
			}
		}
	}

	return dbs, nil
}

func mongoListCollectionsWithDialer(ctx context.Context, cfg mongoConfig, db string, dial mongoDialer) ([]string, error) {
	session, err := mongoDialWithAuth(ctx, cfg, dial)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	query := fmt.Sprintf("listCollections.%s", db)
	result, err := session.query(ctx, query)
	if err != nil {
		return nil, err
	}

	if len(result.Rows) == 0 {
		return nil, fmt.Errorf("listCollections returned no documents")
	}

	var colls []string
	if len(result.Rows) > 0 {
		if collList, ok := result.Rows[0]["cursor"]; ok {
			if cursorDoc, ok := collList.Data.(map[string]interface{}); ok {
				if firstBatch, ok := cursorDoc["firstBatch"].([]interface{}); ok {
					for _, c := range firstBatch {
						if doc, ok := c.(map[string]interface{}); ok {
							if name, ok := doc["name"].(string); ok {
								colls = append(colls, name)
							}
						}
					}
				}
			}
		}
	}

	return colls, nil
}

func mongoGetCollectionStatsWithDialer(ctx context.Context, cfg mongoConfig, db, coll string, dial mongoDialer) (map[string]bsonValue, error) {
	session, err := mongoDialWithAuth(ctx, cfg, dial)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	query := fmt.Sprintf("collStats.%s.%s", db, coll)
	result, err := session.query(ctx, query)
	if err != nil {
		return nil, err
	}

	if len(result.Rows) == 0 {
		return nil, fmt.Errorf("collStats returned no documents")
	}

	return result.Rows[0], nil
}

func mongoFindWithDialer(ctx context.Context, cfg mongoConfig, db, coll string, filter map[string]interface{}, projection map[string]interface{}, batchSize int, dial mongoDialer) (*mongoCursor, error) {
	session, err := mongoDialWithAuth(ctx, cfg, dial)
	if err != nil {
		return nil, err
	}

	cursor := &mongoCursor{
		conn:   session.conn,
		ns:     cfg.Database + "." + coll,
		dialer: dial,
	}

	session.conn = nil
	session.Close()

	return cursor, nil
}

func mongoInsertOneWithDialer(ctx context.Context, cfg mongoConfig, db, coll string, doc map[string]interface{}, dial mongoDialer) error {
	session, err := mongoDialWithAuth(ctx, cfg, dial)
	if err != nil {
		return err
	}
	defer session.Close()

	query := fmt.Sprintf("insert.%s.%s", db, coll)
	result, err := session.query(ctx, query)
	if err != nil {
		return err
	}

	if len(result.Rows) == 0 {
		return fmt.Errorf("insert returned no result")
	}

	if len(result.Rows) > 0 {
		row := result.Rows[0]
		if okVal, ok := row["ok"]; ok {
			if fl, ok := okVal.Data.(float64); ok && fl == 0 {
				if errmsg, ok := row["errmsg"]; ok {
					return fmt.Errorf("insert failed: %v", errmsg)
				}
				return fmt.Errorf("insert failed")
			}
		}
	}

	return nil
}

func mongoDialWithAuth(ctx context.Context, cfg mongoConfig, dial mongoDialer) (*mongoWireSession, error) {
	conn, err := dial(ctx, "tcp", cfg.Address)
	if err != nil {
		return nil, err
	}

	session, err := mongoWireHandshake(ctx, conn, cfg.Username, cfg.Password, cfg.Database, "")
	if err != nil {
		conn.Close()
		return nil, err
	}

	return session, nil
}

func mongoWatchWithDialer(ctx context.Context, cfg mongoConfig, resumeToken map[string]interface{}, dial mongoDialer) (*mongoChangeStreamCursor, error) {
	session, err := mongoDialWithAuth(ctx, cfg, dial)
	if err != nil {
		return nil, err
	}

	return &mongoChangeStreamCursor{
		session: session,
		batch:   []map[string]interface{}{},
	}, nil
}

func defaultMongoDialer(ctx context.Context, network string, address string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, address)
}

func useMongoNativeProtocol(target drivers.Target) bool {
	value := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		target.Connection["protocol"],
		target.Connection["native_protocol"],
		target.Connection["native"],
		target.Options["protocol"],
		target.Options["native_protocol"],
		target.Options["native"],
	)))
	switch value {
	case "mongodump", "shell", "external":
		return false
	default:
		return true
	}
}

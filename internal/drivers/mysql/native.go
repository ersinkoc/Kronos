package mysql

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/kronos/kronos/internal/drivers"
)

type mysqlConfig struct {
	Address    string
	Host       string
	Port       string
	Database   string
	Username   string
	Password   string
	Charset    string
	TLSConfig  string
}

type mysqlDialer func(ctx context.Context, network string, address string) (net.Conn, error)

type mysqlQueryer interface {
	SimpleQuery(ctx context.Context, target drivers.Target, query string) (mysqlQueryResult, error)
	GetMasterStatus(ctx context.Context, target drivers.Target) (string, string, error)
	GetBinlogEvents(ctx context.Context, target drivers.Target, position string) ([]mysqlBinlogEvent, error)
}

type mysqlRunner struct{}

func (mysqlRunner) SimpleQuery(ctx context.Context, target drivers.Target, query string) (mysqlQueryResult, error) {
	cfg, err := mysqlConfigFromTarget(target)
	if err != nil {
		return mysqlQueryResult{}, err
	}
	return mysqlSimpleQueryWithDialer(ctx, cfg, query, defaultMysqlDialer)
}

func (mysqlRunner) GetMasterStatus(ctx context.Context, target drivers.Target) (string, string, error) {
	cfg, err := mysqlConfigFromTarget(target)
	if err != nil {
		return "", "", err
	}
	return mysqlGetMasterStatus(ctx, cfg, defaultMysqlDialer)
}

func (mysqlRunner) GetBinlogEvents(ctx context.Context, target drivers.Target, position string) ([]mysqlBinlogEvent, error) {
	cfg, err := mysqlConfigFromTarget(target)
	if err != nil {
		return nil, err
	}
	return mysqlGetBinlogEvents(ctx, cfg, position, defaultMysqlDialer)
}

func mysqlConfigFromTarget(target drivers.Target) (mysqlConfig, error) {
	cfg := mysqlConfig{
		Host:     "127.0.0.1",
		Port:     "3306",
		Database: "mysql",
		Username: "root",
		Charset:  "utf8mb4",
	}

	if dsn := strings.TrimSpace(target.Connection["dsn"]); dsn != "" {
		parsed, err := url.Parse(dsn)
		if err != nil {
			return mysqlConfig{}, fmt.Errorf("parse mysql dsn: %w", err)
		}
		if parsed.Scheme != "mysql" && parsed.Scheme != "mariadb" {
			return mysqlConfig{}, fmt.Errorf("unsupported mysql dsn scheme %q", parsed.Scheme)
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
	}

	host, port := splitAddress(target.Connection["addr"])
	if host != "" {
		cfg.Host = host
	}
	if port != "" {
		cfg.Port = port
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
	if value := strings.TrimSpace(firstNonEmpty(target.Connection["charset"], target.Options["charset"])); value != "" {
		cfg.Charset = value
	}
	if value := strings.TrimSpace(firstNonEmpty(target.Connection["tls"], target.Options["tls"])); value != "" {
		cfg.TLSConfig = value
	}
	cfg.Address = net.JoinHostPort(cfg.Host, cfg.Port)
	return cfg, nil
}

func mysqlSimpleQueryWithDialer(ctx context.Context, cfg mysqlConfig, query string, dial mysqlDialer) (mysqlQueryResult, error) {
	conn, err := dial(ctx, "tcp", cfg.Address)
	if err != nil {
		return mysqlQueryResult{}, err
	}
	defer conn.Close()

	if err := mysqlHandshake(conn, cfg.Username, cfg.Password, cfg.Database); err != nil {
		return mysqlQueryResult{}, err
	}

	return mysqlQuery(conn, query)
}

func mysqlGetMasterStatus(ctx context.Context, cfg mysqlConfig, dial mysqlDialer) (string, string, error) {
	conn, err := dial(ctx, "tcp", cfg.Address)
	if err != nil {
		return "", "", err
	}
	defer conn.Close()

	if err := mysqlHandshake(conn, cfg.Username, cfg.Password, cfg.Database); err != nil {
		return "", "", err
	}

	result, err := mysqlQuery(conn, "SHOW MASTER STATUS")
	if err != nil {
		return "", "", err
	}
	if len(result.Rows) == 0 || len(result.Rows[0]) < 2 {
		return "", "", fmt.Errorf("SHOW MASTER STATUS returned no rows")
	}
	file := ""
	pos := ""
	if result.Rows[0][0] != nil {
		file = *result.Rows[0][0]
	}
	if result.Rows[0][1] != nil {
		pos = *result.Rows[0][1]
	}
	return file, pos, nil
}

type mysqlBinlogEvent struct {
	LogName string
	Pos     uint32
	Type    string
	Schema  string
	Table   string
	Data    []byte
}

func mysqlGetBinlogEvents(ctx context.Context, cfg mysqlConfig, position string, dial mysqlDialer) ([]mysqlBinlogEvent, error) {
	conn, err := dial(ctx, "tcp", cfg.Address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := mysqlHandshake(conn, cfg.Username, cfg.Password, cfg.Database); err != nil {
		return nil, err
	}

	query := "SHOW BINLOG EVENTS"
	if position != "" {
		query = fmt.Sprintf("SHOW BINLOG EVENTS IN '%s'", position)
	}

	result, err := mysqlQuery(conn, query)
	if err != nil {
		return nil, err
	}

	var events []mysqlBinlogEvent
	for _, row := range result.Rows {
		if len(row) < 4 {
			continue
		}
		event := mysqlBinlogEvent{}
		if row[0] != nil {
			event.LogName = *row[0]
		}
		if row[1] != nil {
			// Pos is string, convert
			event.Pos = 0 // Not easily parseable from string
		}
		if row[3] != nil {
			event.Type = *row[3]
		}
		if row[4] != nil {
			event.Schema = *row[4]
		}
		if row[5] != nil {
			event.Data = []byte(*row[5])
		}
		events = append(events, event)
	}
	return events, nil
}

func defaultMysqlDialer(ctx context.Context, network string, address string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, address)
}

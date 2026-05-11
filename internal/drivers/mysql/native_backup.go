package mysql

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/kronos/kronos/internal/drivers"
)

func mysqlNativeBackupFull(ctx context.Context, target drivers.Target, w drivers.RecordWriter, queryer mysqlQueryer) (drivers.ResumePoint, error) {
	// List all tables
	tables, err := mysqlListTables(ctx, target, queryer)
	if err != nil {
		return drivers.ResumePoint{}, err
	}

	var payload bytes.Buffer
	payload.WriteString("SET NAMES utf8mb4;\n\n")

	var totalRows int64
	for _, table := range tables {
		// Get CREATE TABLE
		createSQL, err := mysqlGetCreateTable(ctx, target, queryer, table)
		if err != nil {
			return drivers.ResumePoint{}, err
		}
		payload.WriteString(createSQL)
		payload.WriteString(";\n\n")

		// Get row count
		count, err := mysqlGetRowCount(ctx, target, queryer, table)
		if err != nil {
			return drivers.ResumePoint{}, err
		}
		totalRows += count

		// Get column names for SELECT
		columns, err := mysqlListColumns(ctx, target, queryer, table)
		if err != nil {
			return drivers.ResumePoint{}, err
		}

		// Get all rows
		rows, err := mysqlReadTable(ctx, target, queryer, table, columns)
		if err != nil {
			return drivers.ResumePoint{}, err
		}

		// Write INSERT statements
		if len(rows) > 0 {
			writeMySQLTableData(&payload, table, columns, rows)
		}
	}

	obj := drivers.ObjectRef{Name: databaseName(target), Kind: databaseObjectKind}
	if err := w.WriteRecord(obj, payload.Bytes()); err != nil {
		return drivers.ResumePoint{}, err
	}
	if err := w.FinishObject(obj, totalRows); err != nil {
		return drivers.ResumePoint{}, err
	}
	return drivers.ResumePoint{Driver: "mysql", Position: "mysql:native"}, nil
}

type mysqlTable struct {
	Schema string
	Name   string
}

type mysqlColumn struct {
	Name     string
	Type     string
	NotNull  bool
	Key      string
	Default *string
	Extra    string
}

func mysqlListTables(ctx context.Context, target drivers.Target, queryer mysqlQueryer) ([]mysqlTable, error) {
	query := fmt.Sprintf("SELECT TABLE_SCHEMA, TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = %s AND TABLE_TYPE = 'BASE TABLE' ORDER BY TABLE_NAME",
		quoteMysqlValue(databaseName(target)))
	result, err := queryer.SimpleQuery(ctx, target, query)
	if err != nil {
		return nil, err
	}
	tables := make([]mysqlTable, 0, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) >= 2 && row[0] != nil && row[1] != nil {
			tables = append(tables, mysqlTable{Schema: *row[0], Name: *row[1]})
		}
	}
	return tables, nil
}

func mysqlGetCreateTable(ctx context.Context, target drivers.Target, queryer mysqlQueryer, table mysqlTable) (string, error) {
	query := fmt.Sprintf("SHOW CREATE TABLE %s.%s", quoteMysqlIdentifier(table.Schema), quoteMysqlIdentifier(table.Name))
	result, err := queryer.SimpleQuery(ctx, target, query)
	if err != nil {
		return "", err
	}
	if len(result.Rows) == 0 || len(result.Rows[0]) < 2 || result.Rows[0][1] == nil {
		return "", fmt.Errorf("SHOW CREATE TABLE returned no result for %s.%s", table.Schema, table.Name)
	}
	return *result.Rows[0][1], nil
}

func mysqlListColumns(ctx context.Context, target drivers.Target, queryer mysqlQueryer, table mysqlTable) ([]mysqlColumn, error) {
	query := fmt.Sprintf("SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE, COLUMN_KEY, COLUMN_DEFAULT, EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = %s AND TABLE_NAME = %s ORDER BY ORDINAL_POSITION",
		quoteMysqlValue(table.Schema), quoteMysqlValue(table.Name))
	result, err := queryer.SimpleQuery(ctx, target, query)
	if err != nil {
		return nil, err
	}
	columns := make([]mysqlColumn, 0, len(result.Rows))
	for _, row := range result.Rows {
		if len(row) < 4 {
			continue
		}
		col := mysqlColumn{}
		if row[0] != nil {
			col.Name = *row[0]
		}
		if row[1] != nil {
			col.Type = *row[1]
		}
		if row[2] != nil {
			col.NotNull = strings.ToUpper(*row[2]) == "NO"
		}
		if row[3] != nil {
			col.Key = *row[3]
		}
		if row[4] != nil {
			col.Default = row[4]
		}
		if row[5] != nil {
			col.Extra = *row[5]
		}
		columns = append(columns, col)
	}
	return columns, nil
}

func mysqlGetRowCount(ctx context.Context, target drivers.Target, queryer mysqlQueryer, table mysqlTable) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", quoteMysqlIdentifier(table.Schema), quoteMysqlIdentifier(table.Name))
	result, err := queryer.SimpleQuery(ctx, target, query)
	if err != nil {
		return 0, err
	}
	if len(result.Rows) == 0 || result.Rows[0][0] == nil {
		return 0, nil
	}
	var count int64
	fmt.Sscanf(*result.Rows[0][0], "%d", &count)
	return count, nil
}

func mysqlReadTable(ctx context.Context, target drivers.Target, queryer mysqlQueryer, table mysqlTable, columns []mysqlColumn) ([][]*string, error) {
	if len(columns) == 0 {
		return nil, nil
	}
	colNames := make([]string, 0, len(columns))
	for _, col := range columns {
		colNames = append(colNames, quoteMysqlIdentifier(col.Name))
	}
	query := fmt.Sprintf("SELECT %s FROM %s.%s", strings.Join(colNames, ", "), quoteMysqlIdentifier(table.Schema), quoteMysqlIdentifier(table.Name))
	result, err := queryer.SimpleQuery(ctx, target, query)
	if err != nil {
		return nil, err
	}
	return result.Rows, nil
}

func writeMySQLTableData(w *bytes.Buffer, table mysqlTable, columns []mysqlColumn, rows [][]*string) {
	if len(rows) == 0 {
		return
	}
	colNames := make([]string, 0, len(columns))
	for _, col := range columns {
		colNames = append(colNames, quoteMysqlIdentifier(col.Name))
	}
	for _, row := range rows {
		values := make([]string, 0, len(columns))
		for i := range columns {
			if i >= len(row) || row[i] == nil {
				values = append(values, "NULL")
				continue
			}
			values = append(values, mysqlQuoteValue(*row[i]))
		}
		fmt.Fprintf(w, "INSERT INTO %s.%s (%s) VALUES (%s);\n",
			quoteMysqlIdentifier(table.Schema),
			quoteMysqlIdentifier(table.Name),
			strings.Join(colNames, ", "),
			strings.Join(values, ", "))
	}
}

func quoteMysqlIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func quoteMysqlValue(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func mysqlQuoteValue(value string) string {
	// Handle special SQL values
	upper := strings.ToUpper(value)
	if upper == "NULL" || upper == "CURRENT_TIMESTAMP" || strings.HasPrefix(upper, "0X") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

package mysql

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/kronos/kronos/internal/drivers"
)

func mysqlNativeRestore(ctx context.Context, target drivers.Target, r drivers.RecordReader, opts drivers.RestoreOptions, queryer mysqlQueryer) error {
	for {
		record, err := r.NextRecord()
		if err != nil {
			if err.Error() == "EOF" {
				return nil
			}
			return err
		}
		if record.Done || record.Object.Kind != databaseObjectKind {
			continue
		}
		if opts.DryRun {
			continue
		}
		if !opts.ReplaceExisting {
			return fmt.Errorf("mysql restore requires replace_existing=true because SQL restore can overwrite existing objects")
		}

		// Split SQL into statements and execute each
		statements := splitMySQLStatements(string(record.Payload))
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			_, err := queryer.SimpleQuery(ctx, target, stmt)
			if err != nil {
				return fmt.Errorf("executing SQL: %w", err)
			}
		}
	}
}

func splitMySQLStatements(sql string) []string {
	var statements []string
	var current bytes.Buffer
	inString := false
	stringChar := byte('\'')

	for i := 0; i < len(sql); i++ {
		c := sql[i]

		// Handle string literals
		if !inString && c == stringChar {
			inString = true
			current.WriteByte(c)
			continue
		}
		if inString {
			current.WriteByte(c)
			if c == stringChar {
				// Check for escaped quote
				if i+1 < len(sql) && sql[i+1] == stringChar {
					i++
					current.WriteByte(sql[i])
				} else {
					inString = false
				}
			}
			continue
		}

		// Handle DELIMITER commands (not commonly used in dumps)
		if strings.HasPrefix(sql[i:], "DELIMITER ") {
			if current.Len() > 0 {
				stmt := current.String()
				if s := strings.TrimSpace(stmt); s != "" {
					statements = append(statements, s)
				}
				current.Reset()
			}
			// Find end of delimiter command
			end := i + 10
			for end < len(sql) && sql[end] != '\n' && sql[end] != ';' {
				end++
			}
			i = end - 1
			continue
		}

		// Handle semicolon as statement separator
		if c == ';' {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		// Handle comments
		if c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			// Skip to end of line
			for i+1 < len(sql) && sql[i+1] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			// Skip comment block
			i += 2
			for i+1 < len(sql) {
				if sql[i] == '*' && sql[i+1] == '/' {
					i++
					break
				}
				i++
			}
			continue
		}

		current.WriteByte(c)
	}

	// Don't forget the last statement
	if s := strings.TrimSpace(current.String()); s != "" {
		statements = append(statements, s)
	}

	return statements
}

package mongodb

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/kronos/kronos/internal/drivers"
)

func mongoNativeRestore(ctx context.Context, target drivers.Target, r drivers.RecordReader, opts drivers.RestoreOptions, queryer mongoQueryer) error {
	for {
		record, err := r.NextRecord()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if record.Done || record.Object.Kind != databaseObjectKind {
			continue
		}
		if opts.DryRun {
			continue
		}
		if !opts.ReplaceExisting {
			return fmt.Errorf("mongodb restore requires replace_existing=true because archive restore can overwrite existing collections")
		}

		statements := splitMongoStatements(string(record.Payload))
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if strings.HasPrefix(stmt, "use ") {
				continue
			}
			if strings.HasPrefix(stmt, "//") {
				continue
			}

			if err := mongoExecuteStatement(ctx, target, queryer, stmt); err != nil {
				return fmt.Errorf("executing statement: %w", err)
			}
		}
	}
}

func splitMongoStatements(sql string) []string {
	var statements []string
	var current strings.Builder
	inString := false
	stringChar := byte('\'')
	inComment := false
	inBlockComment := false

	for i := 0; i < len(sql); i++ {
		c := sql[i]

		if inComment {
			if c == '\n' {
				inComment = false
			}
			continue
		}

		if inBlockComment {
			if c == '*' && i+1 < len(sql) && sql[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if !inString && c == stringChar {
			inString = true
			current.WriteByte(c)
			continue
		}

		if inString {
			current.WriteByte(c)
			if c == stringChar {
				if i+1 < len(sql) && sql[i+1] == stringChar {
					i++
					current.WriteByte(sql[i])
				} else {
					inString = false
				}
			}
			continue
		}

		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			inBlockComment = true
			i++
			continue
		}

		if c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			inComment = true
			i++
			continue
		}

		if c == ';' {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteByte(c)
	}

	last := strings.TrimSpace(current.String())
	if last != "" {
		statements = append(statements, last)
	}

	return statements
}

func mongoExecuteStatement(ctx context.Context, target drivers.Target, queryer mongoQueryer, stmt string) error {
	stmt = strings.TrimSpace(stmt)
	if stmt == "" {
		return nil
	}

	if strings.HasPrefix(stmt, "db.") {
		parts := strings.SplitN(stmt, "(", 2)
		if len(parts) != 2 {
			return nil
		}

		collDotIdx := strings.Index(parts[0], ".")
		if collDotIdx == -1 {
			return nil
		}
		collPart := parts[0][collDotIdx+1:]

		methodEnd := strings.LastIndex(parts[1], ")")
		if methodEnd == -1 {
			return nil
		}
		methodBody := strings.TrimSpace(parts[1][:methodEnd])

		if strings.HasPrefix(parts[0], "db.") {
			methodName := collPart
			for i := 0; i < len(methodName); i++ {
				if methodName[i] == '.' {
					methodName = methodName[i+1:]
					break
				}
			}

			if strings.HasPrefix(methodName, "insertOne") {
				return mongoExecInsertOne(ctx, target, queryer, methodBody)
			}
			if strings.HasPrefix(methodName, "insertMany") {
				return mongoExecInsertMany(ctx, target, queryer, methodBody)
			}
			if strings.HasPrefix(methodName, "createIndex") {
				return mongoExecCreateIndex(ctx, target, queryer, methodBody)
			}
		}
	}

	return nil
}

func mongoExecInsertOne(ctx context.Context, target drivers.Target, queryer mongoQueryer, body string) error {
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(body, "{") || !strings.HasSuffix(body, "}") {
		return fmt.Errorf("invalid insertOne body: %s", body)
	}

	return nil
}

func mongoExecInsertMany(ctx context.Context, target drivers.Target, queryer mongoQueryer, body string) error {
	return nil
}

func mongoExecCreateIndex(ctx context.Context, target drivers.Target, queryer mongoQueryer, body string) error {
	return nil
}

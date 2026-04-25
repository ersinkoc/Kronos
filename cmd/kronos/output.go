package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func writeCommandJSON(ctx context.Context, out io.Writer, value any) error {
	data, err := formatStructuredValue(ctx, value)
	if err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}

func formatStructuredValue(ctx context.Context, value any) ([]byte, error) {
	switch controlOutput(ctx) {
	case "pretty":
		var body bytes.Buffer
		encoder := json.NewEncoder(&body)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(value); err != nil {
			return nil, err
		}
		return body.Bytes(), nil
	case "yaml":
		return yamlFromJSONValue(value)
	case "table":
		return tableFromJSONValue(value)
	default:
		var body bytes.Buffer
		if err := json.NewEncoder(&body).Encode(value); err != nil {
			return nil, err
		}
		return body.Bytes(), nil
	}
}

func formatStructuredJSONBytes(ctx context.Context, data []byte) ([]byte, error) {
	if !json.Valid(data) {
		return data, nil
	}
	switch controlOutput(ctx) {
	case "pretty":
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, data, "", "  "); err != nil {
			return nil, err
		}
		return append(pretty.Bytes(), '\n'), nil
	case "yaml", "table":
		var decoded any
		if err := json.Unmarshal(data, &decoded); err != nil {
			return nil, err
		}
		return formatStructuredValue(ctx, decoded)
	default:
		return data, nil
	}
}

func yamlFromJSONValue(value any) ([]byte, error) {
	decoded, err := normalizeJSONValue(value)
	if err != nil {
		return nil, err
	}
	return yaml.Marshal(decoded)
}

func tableFromJSONValue(value any) ([]byte, error) {
	decoded, err := normalizeJSONValue(value)
	if err != nil {
		return nil, err
	}
	switch typed := decoded.(type) {
	case map[string]any:
		if len(typed) == 1 {
			for label, nested := range typed {
				if rows, ok := nested.([]any); ok {
					return tableFromRows(label, rows), nil
				}
			}
		}
		return keyValueTable(typed), nil
	case []any:
		return tableFromRows("items", typed), nil
	default:
		return []byte(formatTableValue(typed) + "\n"), nil
	}
}

func normalizeJSONValue(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func tableFromRows(label string, rows []any) []byte {
	if len(rows) == 0 {
		return []byte(strings.ToUpper(label) + "\n(empty)\n")
	}
	maps := make([]map[string]any, 0, len(rows))
	columnsSeen := map[string]struct{}{}
	scalarRows := false
	for _, row := range rows {
		item, ok := row.(map[string]any)
		if !ok {
			scalarRows = true
			continue
		}
		maps = append(maps, item)
		for key := range item {
			columnsSeen[key] = struct{}{}
		}
	}
	if scalarRows || len(maps) != len(rows) {
		tableRows := make([][]string, 0, len(rows))
		for _, row := range rows {
			tableRows = append(tableRows, []string{formatTableValue(row)})
		}
		return renderTable([]string{"value"}, tableRows)
	}
	columns := sortedKeys(columnsSeen)
	tableRows := make([][]string, 0, len(maps))
	for _, row := range maps {
		values := make([]string, 0, len(columns))
		for _, column := range columns {
			values = append(values, formatTableValue(row[column]))
		}
		tableRows = append(tableRows, values)
	}
	return renderTable(columns, tableRows)
}

func keyValueTable(values map[string]any) []byte {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := make([][]string, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, []string{key, formatTableValue(values[key])})
	}
	return renderTable([]string{"key", "value"}, rows)
}

func renderTable(columns []string, rows [][]string) []byte {
	if len(columns) == 0 {
		return []byte("(empty)\n")
	}
	headers := make([]string, len(columns))
	widths := make([]int, len(columns))
	for i, column := range columns {
		headers[i] = strings.ToUpper(column)
		widths[i] = len(headers[i])
	}
	for _, row := range rows {
		for i, value := range row {
			if i < len(widths) && len(value) > widths[i] {
				widths[i] = len(value)
			}
		}
	}
	var out strings.Builder
	writeTableRow(&out, headers, widths)
	for _, row := range rows {
		writeTableRow(&out, row, widths)
	}
	return []byte(out.String())
}

func writeTableRow(out *strings.Builder, row []string, widths []int) {
	for i := range widths {
		value := ""
		if i < len(row) {
			value = row[i]
		}
		if i > 0 {
			out.WriteString("  ")
		}
		out.WriteString(value)
		if i < len(widths)-1 {
			out.WriteString(strings.Repeat(" ", widths[i]-len(value)))
		}
	}
	out.WriteByte('\n')
}

func formatTableValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%g", typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

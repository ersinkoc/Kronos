package openapi_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRenderedAPIDocsMatchOpenAPISpec(t *testing.T) {
	data, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("ReadFile(openapi.yaml) error = %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("Unmarshal(openapi.yaml) error = %v", err)
	}

	rendered := renderOpenAPIMarkdown(t, doc)
	path := filepath.Join("..", "..", "docs", "api.md")
	if os.Getenv("KRONOS_UPDATE_API_DOCS") == "1" {
		if err := os.WriteFile(path, []byte(rendered), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if string(current) != rendered {
		t.Fatalf("%s is out of date; rerun with KRONOS_UPDATE_API_DOCS=1", path)
	}
}

func renderOpenAPIMarkdown(t *testing.T, doc map[string]any) string {
	t.Helper()

	var out bytes.Buffer
	info := getMap(t, doc, "info")
	fmt.Fprintf(&out, "# %s\n\n", getString(t, info, "title"))
	fmt.Fprintf(&out, "Generated from [`api/openapi/openapi.yaml`](../api/openapi/openapi.yaml). Do not edit endpoint tables by hand; update the OpenAPI spec and rerun `KRONOS_UPDATE_API_DOCS=1 go test ./api/openapi`.\n\n")
	fmt.Fprintf(&out, "Version: `%s`\n\n", getString(t, info, "version"))
	description := strings.TrimSpace(getString(t, info, "description"))
	if description != "" {
		fmt.Fprintf(&out, "%s\n\n", normalizeWhitespace(description))
	}

	servers, _ := doc["servers"].([]any)
	if len(servers) > 0 {
		first, ok := servers[0].(map[string]any)
		if ok {
			fmt.Fprintf(&out, "## Server\n\n- `%s`\n\n", getString(t, first, "url"))
		}
	}

	fmt.Fprintf(&out, "## Endpoints\n\n")
	fmt.Fprintf(&out, "| Method | Path | Summary | Success responses |\n")
	fmt.Fprintf(&out, "| --- | --- | --- | --- |\n")
	for _, endpoint := range renderedEndpoints(t, getMap(t, doc, "paths")) {
		fmt.Fprintf(&out, "| `%s` | `%s` | %s | %s |\n", endpoint.method, endpoint.path, markdownCell(endpoint.summary), markdownCell(strings.Join(endpoint.successResponses, ", ")))
	}
	fmt.Fprintf(&out, "\n")

	components := getMap(t, doc, "components")
	schemas := getMap(t, components, "schemas")
	names := sortedKeys(schemas)
	fmt.Fprintf(&out, "## Schemas\n\n")
	for _, name := range names {
		schema, ok := schemas[name].(map[string]any)
		if !ok {
			continue
		}
		fmt.Fprintf(&out, "### %s\n\n", name)
		if description, ok := schema["description"].(string); ok && strings.TrimSpace(description) != "" {
			fmt.Fprintf(&out, "%s\n\n", normalizeWhitespace(description))
		}
		if typ, ok := schema["type"].(string); ok && typ != "" {
			fmt.Fprintf(&out, "- Type: `%s`\n", typ)
		}
		if required, ok := stringList(schema["required"]); ok && len(required) > 0 {
			fmt.Fprintf(&out, "- Required: %s\n", inlineCodeList(required))
		}
		if properties, ok := schema["properties"].(map[string]any); ok && len(properties) > 0 {
			fmt.Fprintf(&out, "- Properties: %s\n", inlineCodeList(sortedKeys(properties)))
		}
		if allOf, ok := schema["allOf"].([]any); ok && len(allOf) > 0 {
			refs, properties := allOfSummary(allOf)
			if len(refs) > 0 {
				fmt.Fprintf(&out, "- Composes: %s\n", inlineCodeList(refs))
			}
			if len(properties) > 0 {
				fmt.Fprintf(&out, "- Additional properties: %s\n", inlineCodeList(properties))
			}
		}
		fmt.Fprintf(&out, "\n")
	}

	return out.String()
}

type renderedEndpoint struct {
	method           string
	path             string
	summary          string
	successResponses []string
}

func renderedEndpoints(t *testing.T, paths map[string]any) []renderedEndpoint {
	t.Helper()

	methodOrder := map[string]int{
		"get": 0, "head": 1, "post": 2, "put": 3, "patch": 4, "delete": 5,
	}
	var endpoints []renderedEndpoint
	for _, path := range sortedKeys(paths) {
		pathItem, ok := paths[path].(map[string]any)
		if !ok {
			continue
		}
		for _, method := range sortedKeys(pathItem) {
			operation, ok := pathItem[method].(map[string]any)
			if !ok {
				continue
			}
			if _, ok := methodOrder[method]; !ok {
				continue
			}
			endpoints = append(endpoints, renderedEndpoint{
				method:           strings.ToUpper(method),
				path:             path,
				summary:          stringValue(operation["summary"]),
				successResponses: successResponses(operation),
			})
		}
	}
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].path != endpoints[j].path {
			return endpoints[i].path < endpoints[j].path
		}
		return methodOrder[strings.ToLower(endpoints[i].method)] < methodOrder[strings.ToLower(endpoints[j].method)]
	})
	return endpoints
}

func successResponses(operation map[string]any) []string {
	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		return nil
	}
	var statuses []string
	for status := range responses {
		if strings.HasPrefix(status, "2") {
			statuses = append(statuses, status)
		}
	}
	sort.Strings(statuses)
	return statuses
}

func getMap(t *testing.T, value map[string]any, key string) map[string]any {
	t.Helper()

	child, ok := value[key].(map[string]any)
	if !ok {
		t.Fatalf("%s missing or not a map", key)
	}
	return child
}

func getString(t *testing.T, value map[string]any, key string) string {
	t.Helper()

	text, ok := value[key].(string)
	if !ok {
		t.Fatalf("%s missing or not a string", key)
	}
	return text
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func stringList(value any) ([]string, bool) {
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if ok {
			out = append(out, text)
		}
	}
	sort.Strings(out)
	return out, true
}

func allOfSummary(items []any) ([]string, []string) {
	var refs []string
	propertySet := map[string]any{}
	for _, item := range items {
		schema, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if ref, ok := schema["$ref"].(string); ok {
			refs = append(refs, strings.TrimPrefix(ref, "#/components/schemas/"))
		}
		if properties, ok := schema["properties"].(map[string]any); ok {
			for key := range properties {
				propertySet[key] = struct{}{}
			}
		}
	}
	sort.Strings(refs)
	return refs, sortedKeys(propertySet)
}

func inlineCodeList(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, "`"+item+"`")
	}
	return strings.Join(quoted, ", ")
}

func markdownCell(value string) string {
	value = normalizeWhitespace(value)
	value = strings.ReplaceAll(value, "|", `\|`)
	if value == "" {
		return "-"
	}
	return value
}

func normalizeWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

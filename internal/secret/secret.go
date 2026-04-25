package secret

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kronos/kronos/internal/core"
	"gopkg.in/yaml.v3"
)

// Reference identifies a secret value in a resolver-specific namespace.
type Reference struct {
	Scheme string
	Path   string
	Field  string
}

// Resolver resolves secret references at use time.
type Resolver interface {
	Resolve(context.Context, Reference) (string, error)
}

// ResolverFunc adapts a function to Resolver.
type ResolverFunc func(context.Context, Reference) (string, error)

// Resolve calls fn(ctx, ref).
func (fn ResolverFunc) Resolve(ctx context.Context, ref Reference) (string, error) {
	return fn(ctx, ref)
}

// Registry dispatches references to resolvers by scheme.
type Registry struct {
	resolvers map[string]Resolver
}

// NewRegistry returns a resolver registry with built-in env and file providers.
func NewRegistry() *Registry {
	registry := &Registry{resolvers: make(map[string]Resolver)}
	registry.Register("env", EnvResolver{})
	registry.Register("file", FileResolver{})
	return registry
}

// Register associates scheme with resolver.
func (r *Registry) Register(scheme string, resolver Resolver) {
	if r.resolvers == nil {
		r.resolvers = make(map[string]Resolver)
	}
	r.resolvers[scheme] = resolver
}

// Resolve dispatches ref to a resolver registered for ref.Scheme.
func (r *Registry) Resolve(ctx context.Context, ref Reference) (string, error) {
	resolver, ok := r.resolvers[ref.Scheme]
	if !ok {
		return "", core.WrapKind(core.ErrorKindNotFound, "resolve secret", fmt.Errorf("unknown secret scheme %q", ref.Scheme))
	}
	value, err := resolver.Resolve(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("resolve %s:%s: %w", ref.Scheme, ref.Path, err)
	}
	return value, nil
}

// EnvResolver resolves references from process environment variables.
type EnvResolver struct{}

// Resolve returns the environment variable named by ref.Path.
func (EnvResolver) Resolve(ctx context.Context, ref Reference) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if ref.Field != "" {
		return "", fmt.Errorf("env resolver does not support field selectors")
	}
	value, ok := os.LookupEnv(ref.Path)
	if !ok {
		return "", core.WrapKind(core.ErrorKindNotFound, "lookup env", fmt.Errorf("environment variable %q is not set", ref.Path))
	}
	return value, nil
}

// FileResolver resolves references from local files.
type FileResolver struct{}

// Resolve reads the file named by ref.Path and trims trailing line breaks.
func (FileResolver) Resolve(ctx context.Context, ref Reference) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(ref.Path)
	if err != nil {
		return "", err
	}
	if ref.Field != "" {
		return selectStructuredSecret(data, ref.Field)
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}

func selectStructuredSecret(data []byte, field string) (string, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		if yamlErr := yaml.Unmarshal(data, &value); yamlErr != nil {
			return "", fmt.Errorf("parse structured secret: json: %v; yaml: %w", err, yamlErr)
		}
	}
	selected, err := selectField(value, strings.Split(field, "."))
	if err != nil {
		return "", err
	}
	switch typed := selected.(type) {
	case string:
		return typed, nil
	case bool:
		return fmt.Sprint(typed), nil
	case int, int64, float64:
		return fmt.Sprint(typed), nil
	default:
		return "", fmt.Errorf("secret field %q is not a scalar string/number/bool", field)
	}
}

func selectField(value any, parts []string) (any, error) {
	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("secret field selector is empty")
	}
	current := value
	for _, part := range parts {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("secret field %q is not an object", part)
		}
		next, ok := mapping[part]
		if !ok {
			return nil, core.WrapKind(core.ErrorKindNotFound, "select secret field", fmt.Errorf("field %q not found", part))
		}
		current = next
	}
	return current, nil
}

// ParseReference parses scheme:path#field syntax.
func ParseReference(expr string) (Reference, error) {
	scheme, rest, ok := strings.Cut(expr, ":")
	if !ok || scheme == "" || rest == "" {
		return Reference{}, fmt.Errorf("secret reference must be scheme:path")
	}

	path, field, _ := strings.Cut(rest, "#")
	if path == "" {
		return Reference{}, fmt.Errorf("secret reference path is empty")
	}
	return Reference{Scheme: scheme, Path: path, Field: field}, nil
}

// ParsePlaceholder parses ${scheme:path#field} syntax.
func ParsePlaceholder(value string) (Reference, bool, error) {
	if !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return Reference{}, false, nil
	}
	ref, err := ParseReference(strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}"))
	if err != nil {
		return Reference{}, true, err
	}
	return ref, true, nil
}

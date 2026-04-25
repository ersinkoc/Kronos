package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/kronos/kronos/internal/secret"
	"gopkg.in/yaml.v3"
)

func expandYAML(ctx context.Context, data []byte, resolver secret.Resolver) ([]byte, error) {
	var value any
	if err := yaml.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("parse yaml before expansion: %w", err)
	}

	expanded, err := expandValue(ctx, value, resolver)
	if err != nil {
		return nil, err
	}

	out, err := yaml.Marshal(expanded)
	if err != nil {
		return nil, fmt.Errorf("marshal expanded yaml: %w", err)
	}
	return out, nil
}

func expandValue(ctx context.Context, value any, resolver secret.Resolver) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			expanded, err := expandValue(ctx, child, resolver)
			if err != nil {
				return nil, err
			}
			out[key] = expanded
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			textKey, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("yaml map key %v is not a string", key)
			}
			expanded, err := expandValue(ctx, child, resolver)
			if err != nil {
				return nil, err
			}
			out[textKey] = expanded
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(typed))
		for _, child := range typed {
			expanded, err := expandValue(ctx, child, resolver)
			if err != nil {
				return nil, err
			}
			out = append(out, expanded)
		}
		return out, nil
	case string:
		return expandString(ctx, typed, resolver)
	default:
		return value, nil
	}
}

func expandString(ctx context.Context, value string, resolver secret.Resolver) (string, error) {
	var out strings.Builder
	remaining := value
	for {
		start := strings.Index(remaining, "${")
		if start < 0 {
			out.WriteString(remaining)
			return out.String(), nil
		}
		out.WriteString(remaining[:start])

		afterStart := remaining[start+2:]
		end := strings.IndexByte(afterStart, '}')
		if end < 0 {
			return "", fmt.Errorf("unterminated secret placeholder in %q", value)
		}

		ref, err := secret.ParseReference(afterStart[:end])
		if err != nil {
			return "", err
		}
		resolved, err := resolver.Resolve(ctx, ref)
		if err != nil {
			return "", err
		}
		out.WriteString(resolved)
		remaining = afterStart[end+1:]
	}
}

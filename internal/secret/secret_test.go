package secret

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kronos/kronos/internal/core"
)

func TestEnvResolver(t *testing.T) {
	t.Setenv("KRONOS_TEST_SECRET", "from-env")

	value, err := EnvResolver{}.Resolve(context.Background(), Reference{Scheme: "env", Path: "KRONOS_TEST_SECRET"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if value != "from-env" {
		t.Fatalf("Resolve() = %q, want from-env", value)
	}
	if _, err := (EnvResolver{}).Resolve(context.Background(), Reference{Scheme: "env", Path: "KRONOS_TEST_SECRET", Field: "value"}); err == nil {
		t.Fatal("Resolve(env with field) error = nil, want error")
	}
}

func TestFileResolver(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("from-file\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	value, err := FileResolver{}.Resolve(context.Background(), Reference{Scheme: "file", Path: path})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if value != "from-file" {
		t.Fatalf("Resolve() = %q, want from-file", value)
	}
}

func TestFileResolverSelectsJSONField(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "secret.json")
	if err := os.WriteFile(path, []byte(`{"database":{"password":"pg-secret","port":5432}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	value, err := FileResolver{}.Resolve(context.Background(), Reference{Scheme: "file", Path: path, Field: "database.password"})
	if err != nil {
		t.Fatalf("Resolve(password) error = %v", err)
	}
	if value != "pg-secret" {
		t.Fatalf("Resolve(password) = %q, want pg-secret", value)
	}
	value, err = FileResolver{}.Resolve(context.Background(), Reference{Scheme: "file", Path: path, Field: "database.port"})
	if err != nil {
		t.Fatalf("Resolve(port) error = %v", err)
	}
	if value != "5432" {
		t.Fatalf("Resolve(port) = %q, want 5432", value)
	}
	path = filepath.Join(t.TempDir(), "secret-bool.json")
	if err := os.WriteFile(path, []byte(`{"database":{"enabled":true}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(bool) error = %v", err)
	}
	value, err = FileResolver{}.Resolve(context.Background(), Reference{Scheme: "file", Path: path, Field: "database.enabled"})
	if err != nil {
		t.Fatalf("Resolve(bool) error = %v", err)
	}
	if value != "true" {
		t.Fatalf("Resolve(bool) = %q, want true", value)
	}
}

func TestFileResolverSelectsYAMLField(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "secret.yaml")
	if err := os.WriteFile(path, []byte("database:\n  password: pg-secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	value, err := FileResolver{}.Resolve(context.Background(), Reference{Scheme: "file", Path: path, Field: "database.password"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if value != "pg-secret" {
		t.Fatalf("Resolve() = %q, want pg-secret", value)
	}
}

func TestFileResolverMissingField(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "secret.json")
	if err := os.WriteFile(path, []byte(`{"database":{}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := FileResolver{}.Resolve(context.Background(), Reference{Scheme: "file", Path: path, Field: "database.password"})
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Resolve() error = %v, want ErrNotFound", err)
	}
}

func TestResolverFuncAndRegistryDispatch(t *testing.T) {
	t.Parallel()

	registry := &Registry{}
	registry.Register("test", ResolverFunc(func(ctx context.Context, ref Reference) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return ref.Path + "#" + ref.Field, nil
	}))
	value, err := registry.Resolve(context.Background(), Reference{Scheme: "test", Path: "path", Field: "field"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if value != "path#field" {
		t.Fatalf("Resolve() = %q", value)
	}
}

func TestResolversRespectCanceledContextAndRejectBadStructuredFields(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (EnvResolver{}).Resolve(ctx, Reference{Path: "MISSING"}); err == nil {
		t.Fatal("EnvResolver canceled error = nil, want error")
	}
	if _, err := (FileResolver{}).Resolve(ctx, Reference{Path: "missing"}); err == nil {
		t.Fatal("FileResolver canceled error = nil, want error")
	}

	path := filepath.Join(t.TempDir(), "secret.json")
	if err := os.WriteFile(path, []byte(`{"database":{"nested":{"value":"x"},"list":["x"]}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := (FileResolver{}).Resolve(context.Background(), Reference{Path: path, Field: ""}); err != nil {
		t.Fatalf("Resolve(raw) error = %v", err)
	}
	if _, err := (FileResolver{}).Resolve(context.Background(), Reference{Path: path, Field: "database.nested"}); err == nil {
		t.Fatal("Resolve(non-scalar field) error = nil, want error")
	}
	if _, err := (FileResolver{}).Resolve(context.Background(), Reference{Path: path, Field: "database.list.0"}); err == nil {
		t.Fatal("Resolve(non-object path) error = nil, want error")
	}
	if _, err := selectStructuredSecret([]byte("{"), "database.password"); err == nil {
		t.Fatal("selectStructuredSecret(invalid) error = nil, want error")
	}
	if _, err := selectStructuredSecret([]byte(`{"database":"x"}`), "database.password"); err == nil {
		t.Fatal("selectStructuredSecret(non-object) error = nil, want error")
	}
	if _, err := selectStructuredSecret([]byte(`{"database":{"password":"x"}}`), ""); err == nil {
		t.Fatal("selectStructuredSecret(empty field) error = nil, want error")
	}
}

func TestRegistryUnknownScheme(t *testing.T) {
	t.Parallel()

	_, err := NewRegistry().Resolve(context.Background(), Reference{Scheme: "vault", Path: "secret/app"})
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Resolve() error = %v, want ErrNotFound", err)
	}
}

func TestParsePlaceholder(t *testing.T) {
	t.Parallel()

	ref, ok, err := ParsePlaceholder("${vault:secret/prod/pg#password}")
	if err != nil {
		t.Fatalf("ParsePlaceholder() error = %v", err)
	}
	if !ok {
		t.Fatal("ParsePlaceholder() ok = false, want true")
	}
	if ref.Scheme != "vault" || ref.Path != "secret/prod/pg" || ref.Field != "password" {
		t.Fatalf("ParsePlaceholder() = %#v", ref)
	}
	if _, ok, err := ParsePlaceholder("plain"); ok || err != nil {
		t.Fatalf("ParsePlaceholder(plain) ok=%v err=%v, want false nil", ok, err)
	}
	if _, _, err := ParsePlaceholder("${missing}"); err == nil {
		t.Fatal("ParsePlaceholder(invalid) error = nil, want error")
	}
	if _, err := ParseReference(":path"); err == nil {
		t.Fatal("ParseReference(no scheme) error = nil, want error")
	}
	if _, err := ParseReference("env:"); err == nil {
		t.Fatal("ParseReference(no path) error = nil, want error")
	}
}

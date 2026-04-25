package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRepairDB(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	path := filepath.Join(t.TempDir(), "kronos.db")
	err := run(context.Background(), &out, []string{"repair-db", "--db", path})
	if err != nil {
		t.Fatalf("repair-db error = %v", err)
	}
	if !strings.Contains(out.String(), `"ok":true`) || !strings.Contains(out.String(), `"path":`) {
		t.Fatalf("repair-db output = %q", out.String())
	}
}

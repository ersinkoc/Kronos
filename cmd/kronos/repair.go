package main

import (
	"context"
	"fmt"
	"io"

	"github.com/kronos/kronos/internal/kvstore"
)

func runRepairDB(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("repair-db", out)
	dbPath := fs.String("db", "", "embedded database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		return fmt.Errorf("--db is required")
	}
	if err := kvstore.Repair(*dbPath); err != nil {
		return err
	}
	return writeCommandJSON(ctx, out, map[string]any{
		"ok":   true,
		"path": *dbPath,
	})
}

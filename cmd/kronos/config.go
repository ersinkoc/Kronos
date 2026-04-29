package main

import (
	"context"
	"fmt"
	"io"

	"github.com/kronos/kronos/internal/config"
	"github.com/kronos/kronos/internal/secret"
)

func runConfig(ctx context.Context, out io.Writer, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(out, "Usage:")
		fmt.Fprintln(out, "  kronos config validate --config <path>")
		fmt.Fprintln(out, "  kronos config inspect --config <path> [--include-secrets]")
		return nil
	}

	switch args[0] {
	case "inspect":
		return runConfigInspect(ctx, out, args[1:])
	case "validate":
		return runConfigValidate(ctx, out, args[1:])
	default:
		return fmt.Errorf("unknown config command %q", args[0])
	}
}

func runConfigValidate(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("config validate", out)
	configPath := fs.String("config", "", "path to kronos YAML config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return fmt.Errorf("--config is required")
	}

	cfg, err := config.LoadFile(ctx, *configPath, secret.NewRegistry())
	if err != nil {
		return err
	}
	return writeCommandJSON(ctx, out, map[string]any{
		"ok":       true,
		"projects": len(cfg.Projects),
	})
}

func runConfigInspect(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("config inspect", out)
	configPath := fs.String("config", "", "path to kronos YAML config")
	includeSecrets := fs.Bool("include-secrets", false, "include secret values in output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return fmt.Errorf("--config is required")
	}

	cfg, err := config.LoadFile(ctx, *configPath, secret.NewRegistry())
	if err != nil {
		return err
	}
	view := *cfg
	if !*includeSecrets {
		view = redactConfig(view)
	}
	return writeCommandJSON(ctx, out, view)
}

func redactConfig(cfg config.Config) config.Config {
	cfg.Server.MasterPassphrase = redactString(cfg.Server.MasterPassphrase)
	cfg.Server.Auth.BootstrapToken = redactString(cfg.Server.Auth.BootstrapToken)
	cfg.Server.Auth.OIDC.ClientSecret = redactString(cfg.Server.Auth.OIDC.ClientSecret)
	for i := range cfg.Projects {
		project := &cfg.Projects[i]
		for j := range project.Storages {
			storage := &project.Storages[j]
			storage.Credentials = redactString(storage.Credentials)
			storage.EncryptionKey = redactString(storage.EncryptionKey)
			storage.Options = redactOptions(storage.Options)
		}
		for j := range project.Targets {
			target := &project.Targets[j]
			target.Connection.Password = redactString(target.Connection.Password)
			target.Options = redactOptions(target.Options)
		}
	}
	return cfg
}

func redactString(value string) string {
	if value == "" {
		return ""
	}
	return "***REDACTED***"
}

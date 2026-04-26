package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	agentpkg "github.com/kronos/kronos/internal/agent"
	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/storage"
	"github.com/kronos/kronos/internal/storage/local"
)

func runStorage(ctx context.Context, out io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("storage subcommand is required")
	}
	switch args[0] {
	case "add":
		return runStorageAdd(ctx, out, args[1:])
	case "inspect":
		return runStorageInspect(ctx, out, args[1:])
	case "list":
		return runStorageList(ctx, out, args[1:])
	case "remove":
		return runStorageRemove(ctx, out, args[1:])
	case "test":
		return runStorageTest(ctx, out, args[1:])
	case "du":
		return runStorageDU(ctx, out, args[1:])
	case "update":
		return runStorageUpdate(ctx, out, args[1:])
	default:
		return fmt.Errorf("unknown storage subcommand %q", args[0])
	}
}

func runStorageList(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("storage list", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return getControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/storages", out)
}

func runStorageInspect(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("storage inspect", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	id := fs.String("id", "", "storage id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	return getControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/storages/"+*id, out)
}

func runStorageAdd(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("storage add", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	id := fs.String("id", "", "storage id")
	name := fs.String("name", "", "storage name")
	kind := fs.String("kind", "", "storage kind")
	uri := fs.String("uri", "", "storage uri")
	region := fs.String("region", "", "storage region")
	endpoint := fs.String("endpoint", "", "storage API endpoint")
	credentials := fs.String("credentials", "", "storage credentials mode or reference")
	accessKey := fs.String("access-key", "", "static S3 access key")
	secretKey := fs.String("secret-key", "", "static S3 secret key")
	sessionToken := fs.String("session-token", "", "static S3 session token")
	forcePathStyle := fs.Bool("force-path-style", false, "use path-style S3 requests")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *kind == "" {
		return fmt.Errorf("--kind is required")
	}
	if *uri == "" {
		return fmt.Errorf("--uri is required")
	}
	payload := core.Storage{
		ID:   core.ID(*id),
		Name: *name,
		Kind: core.StorageKind(*kind),
		URI:  *uri,
	}
	options := map[string]any{}
	if *region != "" {
		options["region"] = *region
	}
	if *endpoint != "" {
		options["endpoint"] = *endpoint
	}
	if *credentials != "" {
		options["credentials"] = *credentials
	}
	if *accessKey != "" {
		options["access_key"] = *accessKey
	}
	if *secretKey != "" {
		options["secret_key"] = *secretKey
	}
	if *sessionToken != "" {
		options["session_token"] = *sessionToken
	}
	if *forcePathStyle {
		options["force_path_style"] = true
	}
	if len(options) > 0 {
		payload.Options = options
	}
	return postControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/storages", payload, out)
}

func runStorageUpdate(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("storage update", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	id := fs.String("id", "", "storage id")
	name := fs.String("name", "", "storage name")
	kind := fs.String("kind", "", "storage kind")
	uri := fs.String("uri", "", "storage uri")
	region := fs.String("region", "", "storage region")
	endpoint := fs.String("endpoint", "", "storage API endpoint")
	credentials := fs.String("credentials", "", "storage credentials mode or reference")
	accessKey := fs.String("access-key", "", "static S3 access key")
	secretKey := fs.String("secret-key", "", "static S3 secret key")
	sessionToken := fs.String("session-token", "", "static S3 session token")
	forcePathStyle := fs.Bool("force-path-style", false, "use path-style S3 requests")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *kind == "" {
		return fmt.Errorf("--kind is required")
	}
	if *uri == "" {
		return fmt.Errorf("--uri is required")
	}
	payload := core.Storage{
		ID:   core.ID(*id),
		Name: *name,
		Kind: core.StorageKind(*kind),
		URI:  *uri,
	}
	options := map[string]any{}
	if *region != "" {
		options["region"] = *region
	}
	if *endpoint != "" {
		options["endpoint"] = *endpoint
	}
	if *credentials != "" {
		options["credentials"] = *credentials
	}
	if *accessKey != "" {
		options["access_key"] = *accessKey
	}
	if *secretKey != "" {
		options["secret_key"] = *secretKey
	}
	if *sessionToken != "" {
		options["session_token"] = *sessionToken
	}
	if *forcePathStyle {
		options["force_path_style"] = true
	}
	if len(options) > 0 {
		payload.Options = options
	}
	return putControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/storages/"+*id, payload, out)
}

func runStorageRemove(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("storage remove", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	id := fs.String("id", "", "storage id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	return deleteControl(ctx, http.DefaultClient, *serverAddr, "/api/v1/storages/"+*id, out)
}

func runStorageTest(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("storage test", out)
	uri := fs.String("uri", "", "storage uri")
	kind := fs.String("kind", "", "storage kind; inferred from uri when omitted")
	region := fs.String("region", "", "storage region")
	endpoint := fs.String("endpoint", "", "storage API endpoint")
	credentials := fs.String("credentials", "", "storage credentials mode or JSON object")
	accessKey := fs.String("access-key", "", "static S3 access key")
	secretKey := fs.String("secret-key", "", "static S3 secret key")
	sessionToken := fs.String("session-token", "", "static S3 session token")
	forcePathStyle := fs.Bool("force-path-style", false, "use path-style S3 requests")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *uri == "" {
		return fmt.Errorf("--uri is required")
	}
	backend, err := openStorageURI(*uri, *kind, map[string]any{
		"region":           *region,
		"endpoint":         *endpoint,
		"credentials":      *credentials,
		"access_key":       *accessKey,
		"secret_key":       *secretKey,
		"session_token":    *sessionToken,
		"force_path_style": *forcePathStyle,
	})
	if err != nil {
		return err
	}
	payload := []byte("kronos-storage-probe\n")
	key := fmt.Sprintf(".kronos/probes/%d", time.Now().UnixNano())
	info, err := backend.Put(ctx, key, bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return err
	}
	stream, _, err := backend.Get(ctx, key)
	if err != nil {
		return err
	}
	var got bytes.Buffer
	_, copyErr := got.ReadFrom(stream)
	closeErr := stream.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if !bytes.Equal(got.Bytes(), payload) {
		return fmt.Errorf("storage probe readback mismatch")
	}
	if _, err := backend.Head(ctx, key); err != nil {
		return err
	}
	if err := backend.Delete(ctx, key); err != nil {
		return err
	}
	exists, err := backend.Exists(ctx, key)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("storage probe object still exists after delete")
	}
	return writeCommandJSON(ctx, out, map[string]any{
		"ok":      true,
		"backend": backend.Name(),
		"bytes":   info.Size,
		"etag":    info.ETag,
	})
}

func openStorageURI(rawURI string, kind string, options map[string]any) (storage.Backend, error) {
	if kind == "" {
		parsed, err := url.Parse(rawURI)
		if err != nil {
			return nil, err
		}
		kind = inferStorageKind(parsed.Scheme)
		if kind == "" {
			return nil, fmt.Errorf("--kind is required for storage uri %q", rawURI)
		}
	}
	if !storageKindImplemented(core.StorageKind(kind)) {
		return nil, unsupportedStorageKindError(core.StorageKind(kind))
	}
	cleanOptions := make(map[string]any)
	for key, value := range options {
		switch v := value.(type) {
		case string:
			if v != "" {
				cleanOptions[key] = v
			}
		case bool:
			if v {
				cleanOptions[key] = v
			}
		default:
			if v != nil {
				cleanOptions[key] = v
			}
		}
	}
	return agentpkg.OpenStorageBackend(core.Storage{
		Name:    kind,
		Kind:    core.StorageKind(kind),
		URI:     rawURI,
		Options: cleanOptions,
	})
}

func inferStorageKind(scheme string) string {
	switch scheme {
	case "file":
		return string(core.StorageKindLocal)
	case "s3":
		return string(core.StorageKindS3)
	case "sftp", "ssh":
		return string(core.StorageKindSFTP)
	case "azure", "azblob":
		return string(core.StorageKindAzure)
	case "gs", "gcs":
		return string(core.StorageKindGCS)
	default:
		return ""
	}
}

func storageKindImplemented(kind core.StorageKind) bool {
	switch kind {
	case core.StorageKindLocal, core.StorageKindS3:
		return true
	default:
		return false
	}
}

func unsupportedStorageKindError(kind core.StorageKind) error {
	return fmt.Errorf("storage kind %q is not implemented in this build; supported storage kinds: %s", kind, strings.Join(supportedStorageKinds(), ", "))
}

func supportedStorageKinds() []string {
	return []string{string(core.StorageKindLocal), string(core.StorageKindS3)}
}

func runStorageDU(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("storage du", out)
	uri := fs.String("uri", "", "storage uri")
	kind := fs.String("kind", "", "storage kind; inferred from uri when omitted")
	prefix := fs.String("prefix", "", "object key prefix")
	region := fs.String("region", "", "storage region")
	endpoint := fs.String("endpoint", "", "storage API endpoint")
	credentials := fs.String("credentials", "", "storage credentials mode or JSON object")
	accessKey := fs.String("access-key", "", "static S3 access key")
	secretKey := fs.String("secret-key", "", "static S3 secret key")
	sessionToken := fs.String("session-token", "", "static S3 session token")
	forcePathStyle := fs.Bool("force-path-style", false, "use path-style S3 requests")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *uri == "" {
		return fmt.Errorf("--uri is required")
	}
	backend, err := openStorageURI(*uri, *kind, map[string]any{
		"region":           *region,
		"endpoint":         *endpoint,
		"credentials":      *credentials,
		"access_key":       *accessKey,
		"secret_key":       *secretKey,
		"session_token":    *sessionToken,
		"force_path_style": *forcePathStyle,
	})
	if err != nil {
		return err
	}
	var objects int
	var bytesTotal int64
	token := ""
	for {
		page, err := backend.List(ctx, *prefix, token)
		if err != nil {
			return err
		}
		for _, object := range page.Objects {
			objects++
			bytesTotal += object.Size
		}
		if page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	return writeCommandJSON(ctx, out, map[string]any{"objects": objects, "bytes": bytesTotal})
}

func openLocalStorageURI(uri string) (*local.Backend, error) {
	root, err := localRootFromURI(uri)
	if err != nil {
		return nil, err
	}
	return local.New("local", root)
}

func localRootFromURI(uri string) (string, error) {
	if !strings.Contains(uri, "://") {
		return uri, nil
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "file" {
		return "", fmt.Errorf("only file:// storage URIs are supported by this command")
	}
	if parsed.Host != "" {
		return "", fmt.Errorf("file:// storage URI must use an absolute local path")
	}
	if parsed.Path == "" {
		return "", fmt.Errorf("file:// storage URI path is required")
	}
	return parsed.Path, nil
}

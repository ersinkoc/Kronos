package repository

import (
	"context"
	"crypto/ed25519"
	"fmt"

	"github.com/kronos/kronos/internal/storage"
)

// GCOptions controls repository garbage collection.
type GCOptions struct {
	DryRun bool
}

// GCReport summarizes a mark-and-sweep pass over repository chunks.
type GCReport struct {
	Manifests        int
	ReferencedChunks int
	ScannedChunks    int
	DeletedChunks    int
	DeletedBytes     int64
	OrphanChunks     []storage.ObjectInfo
}

// GarbageCollect deletes chunks under data/ that are not referenced by any manifest.
func GarbageCollect(ctx context.Context, backend storage.Backend, publicKey ed25519.PublicKey, opts GCOptions) (GCReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if backend == nil {
		return GCReport{}, fmt.Errorf("storage backend is required")
	}

	report := GCReport{}
	marked := make(map[string]struct{})
	if err := listAll(ctx, backend, "manifests/", func(info storage.ObjectInfo) error {
		m, _, err := LoadManifest(ctx, backend, info.Key, publicKey)
		if err != nil {
			return fmt.Errorf("load manifest %q: %w", info.Key, err)
		}
		report.Manifests++
		for _, object := range m.Objects {
			for _, ref := range object.Chunks {
				if ref.Key == "" {
					continue
				}
				if _, exists := marked[ref.Key]; !exists {
					marked[ref.Key] = struct{}{}
					report.ReferencedChunks++
				}
			}
		}
		return nil
	}); err != nil {
		return report, err
	}

	if err := listAll(ctx, backend, "data/", func(info storage.ObjectInfo) error {
		report.ScannedChunks++
		if _, keep := marked[info.Key]; keep {
			return nil
		}
		report.OrphanChunks = append(report.OrphanChunks, info)
		if opts.DryRun {
			return nil
		}
		if err := backend.Delete(ctx, info.Key); err != nil {
			return err
		}
		report.DeletedChunks++
		report.DeletedBytes += info.Size
		return nil
	}); err != nil {
		return report, err
	}
	return report, nil
}

func listAll(ctx context.Context, backend storage.Backend, prefix string, fn func(storage.ObjectInfo) error) error {
	token := ""
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		page, err := backend.List(ctx, prefix, token)
		if err != nil {
			return err
		}
		for _, info := range page.Objects {
			if err := fn(info); err != nil {
				return err
			}
		}
		if page.NextToken != "" {
			token = page.NextToken
			continue
		}
		break
	}
	return nil
}

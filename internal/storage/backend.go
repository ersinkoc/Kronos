package storage

import (
	"context"
	"io"
	"time"
)

// Backend stores and retrieves immutable object payloads by key.
type Backend interface {
	Name() string
	Put(ctx context.Context, key string, r io.Reader, size int64) (ObjectInfo, error)
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)
	GetRange(ctx context.Context, key string, off, length int64) (io.ReadCloser, error)
	Head(ctx context.Context, key string) (ObjectInfo, error)
	Exists(ctx context.Context, key string) (bool, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string, token string) (ListPage, error)
}

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Key       string
	Size      int64
	ETag      string
	UpdatedAt time.Time
}

// ListPage is one page of object listings.
type ListPage struct {
	Objects   []ObjectInfo
	NextToken string
}

package local

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/storage"
)

const defaultPageSize = 1000

// Backend stores objects under a root directory.
type Backend struct {
	name string
	root string
}

var _ storage.Backend = (*Backend)(nil)

// New returns a local filesystem backend rooted at root.
func New(name string, root string) (*Backend, error) {
	if name == "" {
		name = "local"
	}
	if root == "" {
		return nil, fmt.Errorf("local storage root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local storage root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create local storage root: %w", err)
	}
	return &Backend{name: name, root: abs}, nil
}

// Name returns the configured backend name.
func (b *Backend) Name() string {
	return b.name
}

// Put streams r into key using a temp file, fsync, and atomic rename.
func (b *Backend) Put(ctx context.Context, key string, r io.Reader, size int64) (storage.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return storage.ObjectInfo{}, err
	}
	finalPath, err := b.pathForKey(key)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	parent := filepath.Dir(finalPath)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return storage.ObjectInfo{}, fmt.Errorf("create object parent: %w", err)
	}
	if _, err := os.Stat(finalPath); err == nil {
		return storage.ObjectInfo{}, core.WrapKind(core.ErrorKindConflict, "put object", fmt.Errorf("object %q already exists", key))
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return storage.ObjectInfo{}, err
	}

	lockPath := finalPath + ".lock"
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return storage.ObjectInfo{}, core.WrapKind(core.ErrorKindConflict, "put object", fmt.Errorf("object %q is already being written", key))
		}
		return storage.ObjectInfo{}, fmt.Errorf("create object lock: %w", err)
	}
	lock.Close()
	defer os.Remove(lockPath)
	if _, err := os.Stat(finalPath); err == nil {
		return storage.ObjectInfo{}, core.WrapKind(core.ErrorKindConflict, "put object", fmt.Errorf("object %q already exists", key))
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return storage.ObjectInfo{}, err
	}

	tmpPath := filepath.Join(parent, "."+filepath.Base(finalPath)+"."+randomSuffix()+".tmp")
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return storage.ObjectInfo{}, fmt.Errorf("create temp object: %w", err)
	}

	hash := sha256.New()
	written, copyErr := copyWithContext(ctx, io.MultiWriter(file, hash), r)
	syncErr := file.Sync()
	closeErr := file.Close()
	if copyErr != nil {
		os.Remove(tmpPath)
		return storage.ObjectInfo{}, copyErr
	}
	if syncErr != nil {
		os.Remove(tmpPath)
		return storage.ObjectInfo{}, fmt.Errorf("sync temp object: %w", syncErr)
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		return storage.ObjectInfo{}, fmt.Errorf("close temp object: %w", closeErr)
	}
	if size >= 0 && written != size {
		os.Remove(tmpPath)
		return storage.ObjectInfo{}, fmt.Errorf("size mismatch for %q: wrote %d bytes, expected %d", key, written, size)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		if errors.Is(err, os.ErrExist) {
			return storage.ObjectInfo{}, core.WrapKind(core.ErrorKindConflict, "put object", fmt.Errorf("object %q already exists", key))
		}
		return storage.ObjectInfo{}, fmt.Errorf("commit object: %w", err)
	}
	if err := syncDir(parent); err != nil {
		return storage.ObjectInfo{}, fmt.Errorf("sync object parent: %w", err)
	}

	stat, err := os.Stat(finalPath)
	if err != nil {
		return storage.ObjectInfo{}, fmt.Errorf("stat committed object: %w", err)
	}
	info := objectInfo(key, stat)
	info.ETag = hex.EncodeToString(hash.Sum(nil))
	return info, nil
}

// Get returns a full object stream.
func (b *Backend) Get(ctx context.Context, key string) (io.ReadCloser, storage.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, storage.ObjectInfo{}, err
	}
	objectPath, err := b.pathForKey(key)
	if err != nil {
		return nil, storage.ObjectInfo{}, err
	}
	file, err := os.Open(objectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, storage.ObjectInfo{}, core.WrapKind(core.ErrorKindNotFound, "get object", fmt.Errorf("object %q not found", key))
		}
		return nil, storage.ObjectInfo{}, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, storage.ObjectInfo{}, err
	}
	object := objectInfo(key, info)
	object.ETag = hashFile(objectPath)
	return file, object, nil
}

// GetRange returns a range stream from key.
func (b *Backend) GetRange(ctx context.Context, key string, off, length int64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if off < 0 || length < 0 {
		return nil, fmt.Errorf("invalid range off=%d length=%d", off, length)
	}
	objectPath, err := b.pathForKey(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(objectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, core.WrapKind(core.ErrorKindNotFound, "get object range", fmt.Errorf("object %q not found", key))
		}
		return nil, err
	}
	if _, err := file.Seek(off, io.SeekStart); err != nil {
		file.Close()
		return nil, err
	}
	return rangedReadCloser{reader: io.LimitReader(file, length), closer: file}, nil
}

// Head returns object metadata.
func (b *Backend) Head(ctx context.Context, key string) (storage.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return storage.ObjectInfo{}, err
	}
	objectPath, err := b.pathForKey(key)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	info, err := os.Stat(objectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return storage.ObjectInfo{}, core.WrapKind(core.ErrorKindNotFound, "head object", fmt.Errorf("object %q not found", key))
		}
		return storage.ObjectInfo{}, err
	}
	object := objectInfo(key, info)
	object.ETag = hashFile(objectPath)
	return object, nil
}

// Exists reports whether key exists.
func (b *Backend) Exists(ctx context.Context, key string) (bool, error) {
	_, err := b.Head(ctx, key)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, core.ErrNotFound) {
		return false, nil
	}
	return false, err
}

// Delete removes key. Missing keys are ignored.
func (b *Backend) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	objectPath, err := b.pathForKey(key)
	if err != nil {
		return err
	}
	if err := os.Remove(objectPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDir(filepath.Dir(objectPath))
}

// List returns objects matching prefix after token, ordered lexicographically.
func (b *Backend) List(ctx context.Context, prefix string, token string) (storage.ListPage, error) {
	if err := ctx.Err(); err != nil {
		return storage.ListPage{}, err
	}
	var objects []storage.ObjectInfo
	err := filepath.WalkDir(b.root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".lock") || strings.HasSuffix(name, ".tmp") {
			return nil
		}
		key, err := filepath.Rel(b.root, filePath)
		if err != nil {
			return err
		}
		key = filepath.ToSlash(key)
		if !strings.HasPrefix(key, prefix) || (token != "" && key <= token) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		object := objectInfo(key, info)
		object.ETag = hashFile(filePath)
		objects = append(objects, object)
		return nil
	})
	if err != nil {
		return storage.ListPage{}, err
	}

	sort.Slice(objects, func(i, j int) bool {
		return objects[i].Key < objects[j].Key
	})
	if len(objects) <= defaultPageSize {
		return storage.ListPage{Objects: objects}, nil
	}
	next := objects[defaultPageSize-1].Key
	return storage.ListPage{Objects: objects[:defaultPageSize], NextToken: next}, nil
}

func (b *Backend) pathForKey(key string) (string, error) {
	cleaned := path.Clean(key)
	if key == "" || cleaned == "." || path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", storage.InvalidKeyError{Key: key}
	}
	if strings.Contains(key, "\\") {
		return "", storage.InvalidKeyError{Key: key}
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == ".." {
			return "", storage.InvalidKeyError{Key: key}
		}
	}
	return filepath.Join(b.root, filepath.FromSlash(cleaned)), nil
}

func objectInfo(key string, info os.FileInfo) storage.ObjectInfo {
	return storage.ObjectInfo{
		Key:       key,
		Size:      info.Size(),
		UpdatedAt: info.ModTime().UTC(),
	}
}

func hashFile(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ""
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 128*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				return written, nil
			}
			return written, er
		}
	}
}

func randomSuffix() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func syncDir(dir string) error {
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

type rangedReadCloser struct {
	reader io.Reader
	closer io.Closer
}

func (r rangedReadCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r rangedReadCloser) Close() error {
	return r.closer.Close()
}

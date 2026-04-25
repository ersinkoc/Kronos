package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/storage"
)

const (
	defaultMultipartThreshold = 64 * 1024 * 1024
	defaultMultipartPartSize  = 16 * 1024 * 1024
)

// Config configures an S3-compatible backend.
type Config struct {
	Name                string
	Endpoint            string
	Region              string
	Bucket              string
	Credentials         Credentials
	CredentialsProvider CredentialsProvider
	ForcePathStyle      bool
	HTTPClient          *http.Client
	MultipartThreshold  int64
	MultipartPartSize   int64
	Retry               RetryConfig
	Sleep               func(time.Duration)
}

// RetryConfig controls retry behaviour for transient S3 failures.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// Backend stores objects in S3 or an S3-compatible service.
type Backend struct {
	name               string
	endpoint           *url.URL
	region             string
	bucket             string
	creds              Credentials
	credsProvider      CredentialsProvider
	credsMu            sync.Mutex
	forcePathStyle     bool
	client             *http.Client
	signer             Signer
	multipartThreshold int64
	multipartPartSize  int64
	retry              RetryConfig
	sleep              func(time.Duration)
}

var _ storage.Backend = (*Backend)(nil)

// New returns an S3-compatible storage backend.
func New(cfg Config) (*Backend, error) {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://s3.amazonaws.com"
	}
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse s3 endpoint: %w", err)
	}
	if endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, fmt.Errorf("s3 endpoint must include scheme and host")
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("s3 region is required")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("s3 bucket is required")
	}
	creds := cfg.Credentials
	if cfg.CredentialsProvider != nil {
		var err error
		creds, err = cfg.CredentialsProvider.Resolve(context.Background())
		if err != nil {
			return nil, fmt.Errorf("resolve s3 credentials: %w", err)
		}
	}
	if _, err := validateCredentials(creds); err != nil {
		return nil, fmt.Errorf("s3 credentials are required: %w", err)
	}
	name := cfg.Name
	if name == "" {
		name = "s3"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	threshold := cfg.MultipartThreshold
	if threshold <= 0 {
		threshold = defaultMultipartThreshold
	}
	partSize := cfg.MultipartPartSize
	if partSize <= 0 {
		partSize = defaultMultipartPartSize
	}
	if partSize <= 0 {
		return nil, fmt.Errorf("s3 multipart part size must be greater than zero")
	}
	retry := cfg.Retry
	if retry.MaxAttempts <= 0 {
		retry.MaxAttempts = 5
	}
	if retry.BaseDelay <= 0 {
		retry.BaseDelay = 100 * time.Millisecond
	}
	if retry.MaxDelay <= 0 {
		retry.MaxDelay = 5 * time.Second
	}
	sleep := cfg.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	return &Backend{
		name:               name,
		endpoint:           endpoint,
		region:             cfg.Region,
		bucket:             cfg.Bucket,
		creds:              creds,
		credsProvider:      cfg.CredentialsProvider,
		forcePathStyle:     cfg.ForcePathStyle,
		client:             client,
		signer:             Signer{Region: cfg.Region, Service: "s3"},
		multipartThreshold: threshold,
		multipartPartSize:  partSize,
		retry:              retry,
		sleep:              sleep,
	}, nil
}

// Name returns the configured backend name.
func (b *Backend) Name() string {
	return b.name
}

// Put stores key with a single PUT Object request.
func (b *Backend) Put(ctx context.Context, key string, r io.Reader, size int64) (storage.ObjectInfo, error) {
	if err := validateKey(key); err != nil {
		return storage.ObjectInfo{}, err
	}
	spooled, payloadHash, written, err := spoolAndHash(r)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	defer os.Remove(spooled.Name())
	defer spooled.Close()
	if size >= 0 && written != size {
		return storage.ObjectInfo{}, fmt.Errorf("size mismatch for %q: wrote %d bytes, expected %d", key, written, size)
	}
	if written >= b.multipartThreshold {
		return b.putMultipart(ctx, key, spooled, written)
	}
	return b.putSingle(ctx, key, spooled, written, payloadHash)
}

func (b *Backend) putSingle(ctx context.Context, key string, spooled *os.File, written int64, payloadHash string) (storage.ObjectInfo, error) {
	if _, err := spooled.Seek(0, io.SeekStart); err != nil {
		return storage.ObjectInfo{}, err
	}

	req, err := b.newRequest(ctx, http.MethodPut, key, nil, spooled)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	req.Body = io.NopCloser(spooled)
	req.ContentLength = written
	req.Header.Set("If-None-Match", "*")
	req.Header.Set("Content-Length", strconv.FormatInt(written, 10))
	req.GetBody = func() (io.ReadCloser, error) {
		if _, err := spooled.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		return io.NopCloser(spooled), nil
	}
	if err := b.sign(ctx, req, payloadHash); err != nil {
		return storage.ObjectInfo{}, err
	}

	resp, err := b.do(req)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusPreconditionFailed || resp.StatusCode == http.StatusConflict {
		return storage.ObjectInfo{}, core.WrapKind(core.ErrorKindConflict, "put s3 object", fmt.Errorf("object %q already exists", key))
	}
	if err := expectStatus(resp, http.StatusOK, http.StatusCreated, http.StatusNoContent); err != nil {
		return storage.ObjectInfo{}, err
	}
	return objectInfoFromHeaders(key, written, resp.Header), nil
}

func (b *Backend) putMultipart(ctx context.Context, key string, file *os.File, size int64) (storage.ObjectInfo, error) {
	exists, err := b.Exists(ctx, key)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	if exists {
		return storage.ObjectInfo{}, core.WrapKind(core.ErrorKindConflict, "put s3 object", fmt.Errorf("object %q already exists", key))
	}

	uploadID, err := b.createMultipartUpload(ctx, key)
	if err != nil {
		return storage.ObjectInfo{}, err
	}

	parts := make([]completedPart, 0, (size+b.multipartPartSize-1)/b.multipartPartSize)
	for offset, partNumber := int64(0), 1; offset < size; offset, partNumber = offset+b.multipartPartSize, partNumber+1 {
		partSize := b.multipartPartSize
		if remaining := size - offset; remaining < partSize {
			partSize = remaining
		}
		etag, err := b.uploadPart(ctx, key, uploadID, partNumber, file, offset, partSize)
		if err != nil {
			abortErr := b.abortMultipartUpload(ctx, key, uploadID)
			if abortErr != nil {
				return storage.ObjectInfo{}, fmt.Errorf("upload part %d: %w; abort multipart upload: %v", partNumber, err, abortErr)
			}
			return storage.ObjectInfo{}, fmt.Errorf("upload part %d: %w", partNumber, err)
		}
		parts = append(parts, completedPart{PartNumber: partNumber, ETag: etag})
	}

	info, err := b.completeMultipartUpload(ctx, key, uploadID, parts)
	if err != nil {
		abortErr := b.abortMultipartUpload(ctx, key, uploadID)
		if abortErr != nil {
			return storage.ObjectInfo{}, fmt.Errorf("complete multipart upload: %w; abort multipart upload: %v", err, abortErr)
		}
		return storage.ObjectInfo{}, err
	}
	if info.Size < 0 {
		info.Size = size
	}
	return info, nil
}

// Get returns the full object stream.
func (b *Backend) Get(ctx context.Context, key string) (io.ReadCloser, storage.ObjectInfo, error) {
	req, err := b.newRequest(ctx, http.MethodGet, key, nil, nil)
	if err != nil {
		return nil, storage.ObjectInfo{}, err
	}
	if err := b.sign(ctx, req, emptyPayloadSHA256); err != nil {
		return nil, storage.ObjectInfo{}, err
	}
	resp, err := b.do(req)
	if err != nil {
		return nil, storage.ObjectInfo{}, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, storage.ObjectInfo{}, core.WrapKind(core.ErrorKindNotFound, "get s3 object", fmt.Errorf("object %q not found", key))
	}
	if err := expectStatus(resp, http.StatusOK); err != nil {
		resp.Body.Close()
		return nil, storage.ObjectInfo{}, err
	}
	return resp.Body, objectInfoFromHeaders(key, resp.ContentLength, resp.Header), nil
}

// GetRange returns a byte range stream from key.
func (b *Backend) GetRange(ctx context.Context, key string, off, length int64) (io.ReadCloser, error) {
	if off < 0 || length < 0 {
		return nil, fmt.Errorf("invalid range off=%d length=%d", off, length)
	}
	if length == 0 {
		return io.NopCloser(strings.NewReader("")), nil
	}
	req, err := b.newRequest(ctx, http.MethodGet, key, nil, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", off, off+length-1))
	if err := b.sign(ctx, req, emptyPayloadSHA256); err != nil {
		return nil, err
	}
	resp, err := b.do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, core.WrapKind(core.ErrorKindNotFound, "get s3 object range", fmt.Errorf("object %q not found", key))
	}
	if err := expectStatus(resp, http.StatusPartialContent, http.StatusOK); err != nil {
		resp.Body.Close()
		return nil, err
	}
	if resp.StatusCode == http.StatusPartialContent {
		return &limitedReadCloser{Reader: io.LimitReader(resp.Body, length), closer: resp.Body}, nil
	}
	return resp.Body, nil
}

// Head returns object metadata.
func (b *Backend) Head(ctx context.Context, key string) (storage.ObjectInfo, error) {
	req, err := b.newRequest(ctx, http.MethodHead, key, nil, nil)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	if err := b.sign(ctx, req, emptyPayloadSHA256); err != nil {
		return storage.ObjectInfo{}, err
	}
	resp, err := b.do(req)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return storage.ObjectInfo{}, core.WrapKind(core.ErrorKindNotFound, "head s3 object", fmt.Errorf("object %q not found", key))
	}
	if err := expectStatus(resp, http.StatusOK); err != nil {
		return storage.ObjectInfo{}, err
	}
	return objectInfoFromHeaders(key, resp.ContentLength, resp.Header), nil
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
	req, err := b.newRequest(ctx, http.MethodDelete, key, nil, nil)
	if err != nil {
		return err
	}
	if err := b.sign(ctx, req, emptyPayloadSHA256); err != nil {
		return err
	}
	resp, err := b.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return expectStatus(resp, http.StatusOK, http.StatusNoContent)
}

// List returns one ListObjectsV2 page.
func (b *Backend) List(ctx context.Context, prefix string, token string) (storage.ListPage, error) {
	values := url.Values{}
	values.Set("list-type", "2")
	if prefix != "" {
		values.Set("prefix", prefix)
	}
	if token != "" {
		values.Set("continuation-token", token)
	}
	req, err := b.newRequest(ctx, http.MethodGet, "", values, nil)
	if err != nil {
		return storage.ListPage{}, err
	}
	if err := b.sign(ctx, req, emptyPayloadSHA256); err != nil {
		return storage.ListPage{}, err
	}
	resp, err := b.do(req)
	if err != nil {
		return storage.ListPage{}, err
	}
	defer resp.Body.Close()
	if err := expectStatus(resp, http.StatusOK); err != nil {
		return storage.ListPage{}, err
	}

	var listing listBucketResult
	if err := xml.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return storage.ListPage{}, fmt.Errorf("decode s3 list response: %w", err)
	}
	objects := make([]storage.ObjectInfo, 0, len(listing.Contents))
	for _, item := range listing.Contents {
		objects = append(objects, storage.ObjectInfo{
			Key:       item.Key,
			Size:      item.Size,
			ETag:      trimETag(item.ETag),
			UpdatedAt: item.LastModified.Time,
		})
	}
	return storage.ListPage{Objects: objects, NextToken: listing.NextContinuationToken}, nil
}

func (b *Backend) createMultipartUpload(ctx context.Context, key string) (string, error) {
	values := url.Values{}
	values.Set("uploads", "")
	req, err := b.newRequest(ctx, http.MethodPost, key, values, nil)
	if err != nil {
		return "", err
	}
	if err := b.sign(ctx, req, emptyPayloadSHA256); err != nil {
		return "", err
	}
	resp, err := b.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := expectStatus(resp, http.StatusOK, http.StatusCreated); err != nil {
		return "", err
	}
	var created createMultipartUploadResult
	if err := xml.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", fmt.Errorf("decode create multipart response: %w", err)
	}
	if created.UploadID == "" {
		return "", fmt.Errorf("create multipart response missing upload id")
	}
	return created.UploadID, nil
}

func (b *Backend) uploadPart(ctx context.Context, key string, uploadID string, partNumber int, file *os.File, offset int64, size int64) (string, error) {
	payloadHash, err := hashSection(file, offset, size)
	if err != nil {
		return "", err
	}
	body := io.NewSectionReader(file, offset, size)
	values := url.Values{}
	values.Set("partNumber", strconv.Itoa(partNumber))
	values.Set("uploadId", uploadID)
	req, err := b.newRequest(ctx, http.MethodPut, key, values, body)
	if err != nil {
		return "", err
	}
	req.ContentLength = size
	req.Header.Set("Content-Length", strconv.FormatInt(size, 10))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(io.NewSectionReader(file, offset, size)), nil
	}
	if err := b.sign(ctx, req, payloadHash); err != nil {
		return "", err
	}
	resp, err := b.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := expectStatus(resp, http.StatusOK); err != nil {
		return "", err
	}
	etag := resp.Header.Get("ETag")
	if trimETag(etag) == "" {
		return "", fmt.Errorf("upload part response missing etag")
	}
	return etag, nil
}

func (b *Backend) completeMultipartUpload(ctx context.Context, key string, uploadID string, parts []completedPart) (storage.ObjectInfo, error) {
	body, err := xml.Marshal(completeMultipartUpload{Parts: parts})
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	body = append([]byte(xml.Header), body...)
	payloadHash := hexSHA256(body)
	values := url.Values{}
	values.Set("uploadId", uploadID)
	req, err := b.newRequest(ctx, http.MethodPost, key, values, bytesReader(body))
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	if err := b.sign(ctx, req, payloadHash); err != nil {
		return storage.ObjectInfo{}, err
	}
	resp, err := b.do(req)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	defer resp.Body.Close()
	if err := expectStatus(resp, http.StatusOK, http.StatusCreated); err != nil {
		return storage.ObjectInfo{}, err
	}
	return objectInfoFromHeaders(key, -1, resp.Header), nil
}

func (b *Backend) abortMultipartUpload(ctx context.Context, key string, uploadID string) error {
	values := url.Values{}
	values.Set("uploadId", uploadID)
	req, err := b.newRequest(ctx, http.MethodDelete, key, values, nil)
	if err != nil {
		return err
	}
	if err := b.sign(ctx, req, emptyPayloadSHA256); err != nil {
		return err
	}
	resp, err := b.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return expectStatus(resp, http.StatusOK, http.StatusNoContent)
}

func (b *Backend) sign(ctx context.Context, req *http.Request, payloadHash string) error {
	creds, err := b.credentials(ctx)
	if err != nil {
		return err
	}
	_, err = b.signer.SignRequest(req, creds, time.Now(), payloadHash)
	return err
}

func (b *Backend) credentials(ctx context.Context) (Credentials, error) {
	if b.credsProvider == nil {
		return b.creds, nil
	}
	b.credsMu.Lock()
	defer b.credsMu.Unlock()
	if b.creds.AccessKey != "" && !credentialsNeedRefresh(b.creds, time.Now()) {
		return b.creds, nil
	}
	creds, err := b.credsProvider.Resolve(ctx)
	if err != nil {
		return Credentials{}, fmt.Errorf("refresh s3 credentials: %w", err)
	}
	creds, err = validateCredentials(creds)
	if err != nil {
		return Credentials{}, fmt.Errorf("refresh s3 credentials: %w", err)
	}
	b.creds = creds
	return creds, nil
}

func credentialsNeedRefresh(creds Credentials, now time.Time) bool {
	if creds.ExpiresAt.IsZero() {
		return false
	}
	return !creds.ExpiresAt.After(now.Add(5 * time.Minute))
}

func (b *Backend) do(req *http.Request) (*http.Response, error) {
	var lastErr error
	attempts := b.retry.MaxAttempts
	if attempts <= 0 {
		attempts = 1
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		attemptReq := req.Clone(req.Context())
		if req.Body != nil {
			if attempt == 1 {
				attemptReq.Body = req.Body
			} else {
				if req.GetBody == nil {
					return nil, lastErr
				}
				body, err := req.GetBody()
				if err != nil {
					return nil, err
				}
				attemptReq.Body = body
			}
		}
		attemptReq.GetBody = req.GetBody
		attemptReq.ContentLength = req.ContentLength

		resp, err := b.client.Do(attemptReq)
		if err != nil {
			lastErr = err
			if attempt == attempts {
				return nil, err
			}
			b.sleep(b.retryDelay(attempt, nil))
			continue
		}
		if !isRetryableStatus(resp.StatusCode) || attempt == attempts {
			return resp, nil
		}

		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		lastErr = fmt.Errorf("s3 request failed: %s", resp.Status)
		b.sleep(b.retryDelay(attempt, resp.Header))
	}
	return nil, lastErr
}

func (b *Backend) retryDelay(attempt int, header http.Header) time.Duration {
	if header != nil {
		if value := header.Get("Retry-After"); value != "" {
			if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
				return time.Duration(seconds) * time.Second
			}
			if when, err := http.ParseTime(value); err == nil {
				delay := time.Until(when)
				if delay > 0 {
					return delay
				}
				return 0
			}
		}
	}

	delay := b.retry.BaseDelay
	if delay <= 0 {
		delay = 100 * time.Millisecond
	}
	if attempt > 1 {
		multiplier := 1 << min(attempt-1, 10)
		delay *= time.Duration(multiplier)
	}
	if b.retry.MaxDelay > 0 && delay > b.retry.MaxDelay {
		delay = b.retry.MaxDelay
	}
	if delay <= 0 {
		return 0
	}
	jitter := time.Duration(time.Now().UnixNano() % int64(delay/2+1))
	return delay/2 + jitter
}

func isRetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests ||
		status == http.StatusRequestTimeout ||
		status == http.StatusInternalServerError ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout
}

func (b *Backend) newRequest(ctx context.Context, method string, key string, query url.Values, body io.Reader) (*http.Request, error) {
	if key != "" {
		if err := validateKey(key); err != nil {
			return nil, err
		}
	}
	requestURL := *b.endpoint
	requestURL.RawQuery = query.Encode()
	if b.forcePathStyle {
		if key == "" {
			requestURL.Path = "/" + b.bucket + "/"
		} else {
			requestURL.Path = "/" + b.bucket + "/" + key
		}
	} else {
		requestURL.Host = b.bucket + "." + b.endpoint.Host
		if key == "" {
			requestURL.Path = "/"
		} else {
			requestURL.Path = "/" + key
		}
	}
	return http.NewRequestWithContext(ctx, method, requestURL.String(), body)
}

func validateKey(key string) error {
	cleaned := path.Clean(key)
	if key == "" || cleaned == "." || path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return storage.InvalidKeyError{Key: key}
	}
	if strings.Contains(key, "\\") {
		return storage.InvalidKeyError{Key: key}
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == ".." {
			return storage.InvalidKeyError{Key: key}
		}
	}
	return nil
}

func spoolAndHash(r io.Reader) (*os.File, string, int64, error) {
	if r == nil {
		return nil, "", 0, fmt.Errorf("reader is required")
	}
	file, err := os.CreateTemp("", "kronos-s3-put-*")
	if err != nil {
		return nil, "", 0, err
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), r)
	if err != nil {
		file.Close()
		os.Remove(file.Name())
		return nil, "", 0, err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(file.Name())
		return nil, "", 0, err
	}
	return file, hex.EncodeToString(hash.Sum(nil)), written, nil
}

func hashSection(file *os.File, offset int64, size int64) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, io.NewSectionReader(file, offset, size)); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func bytesReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}

func objectInfoFromHeaders(key string, fallbackSize int64, header http.Header) storage.ObjectInfo {
	size := fallbackSize
	if value := header.Get("Content-Length"); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			if parsed > 0 || fallbackSize < 0 {
				size = parsed
			}
		}
	}
	updatedAt := time.Time{}
	if value := header.Get("Last-Modified"); value != "" {
		if parsed, err := http.ParseTime(value); err == nil {
			updatedAt = parsed.UTC()
		}
	}
	return storage.ObjectInfo{
		Key:       key,
		Size:      size,
		ETag:      trimETag(header.Get("ETag")),
		UpdatedAt: updatedAt,
	}
}

func trimETag(etag string) string {
	return strings.Trim(etag, `"`)
}

type limitedReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r *limitedReadCloser) Close() error {
	return r.closer.Close()
}

func expectStatus(resp *http.Response, allowed ...int) error {
	for _, status := range allowed {
		if resp.StatusCode == status {
			return nil
		}
	}
	return fmt.Errorf("s3 request failed: %s", resp.Status)
}

type listBucketResult struct {
	Contents              []listObject `xml:"Contents"`
	NextContinuationToken string       `xml:"NextContinuationToken"`
}

type createMultipartUploadResult struct {
	UploadID string `xml:"UploadId"`
}

type completeMultipartUpload struct {
	XMLName xml.Name        `xml:"CompleteMultipartUpload"`
	Parts   []completedPart `xml:"Part"`
}

type completedPart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

type listObject struct {
	Key          string `xml:"Key"`
	LastModified s3Time `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
}

type s3Time struct {
	time.Time
}

func (t *s3Time) UnmarshalText(data []byte) error {
	if len(data) == 0 {
		t.Time = time.Time{}
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, string(data))
	if err != nil {
		return err
	}
	t.Time = parsed.UTC()
	return nil
}

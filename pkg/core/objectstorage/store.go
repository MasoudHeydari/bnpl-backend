package objectstorage

import (
	"context"
	"io"
	"time"
)

// PutOptions contains non-provider-specific object metadata.
type PutOptions struct {
	ContentType string
	Metadata    map[string]string
}

// ObjectInfo describes an object stored by a provider.
type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
	Metadata     map[string]string
}

// Store persists immutable-style application artifacts.
//
// PutIfAbsent must not overwrite an existing key. Callers that need idempotent
// archival should use deterministic keys and verify Head/Get when ErrExists is
// returned.
type Store interface {
	PutIfAbsent(ctx context.Context, key string, body io.Reader, size int64, opts PutOptions) (ObjectInfo, error)
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)
	Head(ctx context.Context, key string) (ObjectInfo, error)
	Close() error
}

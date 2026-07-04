package objectstorage

import "errors"

var (
	// ErrDisabled indicates object persistence is intentionally not configured.
	ErrDisabled = errors.New("object storage disabled")

	// ErrExists indicates PutIfAbsent targeted an already-existing object key.
	ErrExists = errors.New("object already exists")

	// ErrNotFound indicates the requested object key does not exist.
	ErrNotFound = errors.New("object not found")
)

package noop

import (
	"context"
	"io"

	coreobjectstorage "github.com/parstis/bnpl-backend/pkg/core/objectstorage"
)

var _ coreobjectstorage.Store = (*Store)(nil)

// Store is a disabled object-storage provider.
type Store struct{}

// New creates a no-op object store.
func New() *Store {
	return &Store{}
}

// PutIfAbsent fails closed. A disabled archival backend must never pretend that
// evidence was persisted successfully.
func (s *Store) PutIfAbsent(
	_ context.Context,
	_ string,
	_ io.Reader,
	_ int64,
	_ coreobjectstorage.PutOptions,
) (coreobjectstorage.ObjectInfo, error) {
	return coreobjectstorage.ObjectInfo{}, coreobjectstorage.ErrDisabled
}

// Get reports missing because the no-op provider never persists bytes.
func (s *Store) Get(context.Context, string) (io.ReadCloser, coreobjectstorage.ObjectInfo, error) {
	return nil, coreobjectstorage.ObjectInfo{}, coreobjectstorage.ErrNotFound
}

// Head reports missing because the no-op provider never persists bytes.
func (s *Store) Head(context.Context, string) (coreobjectstorage.ObjectInfo, error) {
	return coreobjectstorage.ObjectInfo{}, coreobjectstorage.ErrNotFound
}

// Close releases no resources.
func (s *Store) Close() error {
	return nil
}

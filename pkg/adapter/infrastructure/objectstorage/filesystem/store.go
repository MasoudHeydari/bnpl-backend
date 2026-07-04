package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	coreobjectstorage "github.com/parstis/bnpl-backend/pkg/core/objectstorage"
)

var _ coreobjectstorage.Store = (*Store)(nil)

const metadataSuffix = ".metadata.json"

// Store persists immutable objects under one filesystem root.
//
// Object keys are slash-separated logical paths. Absolute paths and path
// traversal are rejected so callers cannot escape the configured root.
type Store struct {
	root string
}

// New validates and creates the filesystem object-storage root.
func New(root string) (*Store, error) {
	const op = "filesystem.New"

	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("%s: root directory is required", op)
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve root: %w", op, err)
	}

	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("%s: create root: %w", op, err)
	}

	return &Store{root: abs}, nil
}

// PutIfAbsent atomically creates an object without overwriting an existing key.
func (s *Store) PutIfAbsent(
	ctx context.Context,
	key string,
	body io.Reader,
	size int64,
	opts coreobjectstorage.PutOptions,
) (coreobjectstorage.ObjectInfo, error) {
	const op = "filesystem.Store.PutIfAbsent"

	if err := ctx.Err(); err != nil {
		return coreobjectstorage.ObjectInfo{}, err
	}

	path, err := s.objectPath(key)
	if err != nil {
		return coreobjectstorage.ObjectInfo{}, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return coreobjectstorage.ObjectInfo{}, fmt.Errorf("%s: create parent directory: %w", op, err)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if errors.Is(err, os.ErrExist) {
		return coreobjectstorage.ObjectInfo{}, coreobjectstorage.ErrExists
	}

	if err != nil {
		return coreobjectstorage.ObjectInfo{}, fmt.Errorf("%s: create object: %w", op, err)
	}

	removeOnFailure := true
	defer func() {
		_ = file.Close()
		if removeOnFailure {
			_ = os.Remove(path)
			_ = os.Remove(path + metadataSuffix)
		}
	}()

	hash := sha256.New()
	written, err := copyWithContext(ctx, io.MultiWriter(file, hash), body)
	if err != nil {
		return coreobjectstorage.ObjectInfo{}, fmt.Errorf("%s: write object: %w", op, err)
	}

	if size >= 0 && written != size {
		return coreobjectstorage.ObjectInfo{}, fmt.Errorf("%s: object size mismatch: expected=%d written=%d", op, size, written)
	}

	if err := file.Sync(); err != nil {
		return coreobjectstorage.ObjectInfo{}, fmt.Errorf("%s: sync object: %w", op, err)
	}

	if err := file.Close(); err != nil {
		return coreobjectstorage.ObjectInfo{}, fmt.Errorf("%s: close object: %w", op, err)
	}

	info := coreobjectstorage.ObjectInfo{
		Key:          key,
		Size:         written,
		ContentType:  opts.ContentType,
		ETag:         "sha256:" + hex.EncodeToString(hash.Sum(nil)),
		LastModified: time.Now().UTC(),
		Metadata:     cloneMetadata(opts.Metadata),
	}
	if err := writeMetadata(path+metadataSuffix, info); err != nil {
		return coreobjectstorage.ObjectInfo{}, fmt.Errorf("%s: write metadata: %w", op, err)
	}

	removeOnFailure = false
	return info, nil
}

// Get opens an object for reading and returns its stored metadata.
func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, coreobjectstorage.ObjectInfo, error) {
	const op = "filesystem.Store.Get"

	if err := ctx.Err(); err != nil {
		return nil, coreobjectstorage.ObjectInfo{}, err
	}

	path, err := s.objectPath(key)
	if err != nil {
		return nil, coreobjectstorage.ObjectInfo{}, err
	}

	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, coreobjectstorage.ObjectInfo{}, coreobjectstorage.ErrNotFound
	}

	if err != nil {
		return nil, coreobjectstorage.ObjectInfo{}, fmt.Errorf("%s: open object: %w", op, err)
	}

	info, err := s.headPath(key, path)
	if err != nil {
		_ = file.Close()
		return nil, coreobjectstorage.ObjectInfo{}, err
	}

	return file, info, nil
}

// Head returns object metadata without opening its body.
func (s *Store) Head(ctx context.Context, key string) (coreobjectstorage.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return coreobjectstorage.ObjectInfo{}, err
	}

	path, err := s.objectPath(key)
	if err != nil {
		return coreobjectstorage.ObjectInfo{}, err
	}

	return s.headPath(key, path)
}

// Close releases no long-lived resources for the filesystem provider.
func (s *Store) Close() error {
	return nil
}

func (s *Store) headPath(key, path string) (coreobjectstorage.ObjectInfo, error) {
	const op = "filesystem.Store.Head"

	stat, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return coreobjectstorage.ObjectInfo{}, coreobjectstorage.ErrNotFound
	}

	if err != nil {
		return coreobjectstorage.ObjectInfo{}, fmt.Errorf("%s: stat object: %w", op, err)
	}

	if !stat.Mode().IsRegular() {
		return coreobjectstorage.ObjectInfo{}, fmt.Errorf("%s: object %q is not a regular file", op, key)
	}

	var stored coreobjectstorage.ObjectInfo
	metadataBytes, err := os.ReadFile(path + metadataSuffix)
	if err == nil {
		if unmarshalErr := json.Unmarshal(metadataBytes, &stored); unmarshalErr != nil {
			return coreobjectstorage.ObjectInfo{}, fmt.Errorf("%s: decode metadata: %w", op, unmarshalErr)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return coreobjectstorage.ObjectInfo{}, fmt.Errorf("%s: read metadata: %w", op, err)
	}

	stored.Key = key
	stored.Size = stat.Size()
	if stored.LastModified.IsZero() {
		stored.LastModified = stat.ModTime().UTC()
	}

	return stored, nil
}

func (s *Store) objectPath(key string) (string, error) {
	const op = "filesystem.Store.objectPath"

	key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	if key == "" || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("%s: invalid object key %q", op, key)
	}

	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return "", fmt.Errorf("%s: object key escapes storage root: %q", op, key)
	}

	path := filepath.Join(s.root, clean)
	rel, err := filepath.Rel(s.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: object key escapes storage root: %q", op, key)
	}

	return path, nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64

	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			m, writeErr := dst.Write(buf[:n])
			written += int64(m)
			if writeErr != nil {
				return written, writeErr
			}

			if m != n {
				return written, io.ErrShortWrite
			}
		}

		if errors.Is(readErr, io.EOF) {
			return written, nil
		}

		if readErr != nil {
			return written, readErr
		}
	}
}

func writeMetadata(path string, info coreobjectstorage.ObjectInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o640)
}

func cloneMetadata(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}

	out := make(map[string]string, len(src))
	for key, value := range src {
		out[key] = value
	}

	return out
}

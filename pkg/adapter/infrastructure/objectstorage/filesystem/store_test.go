package filesystem

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	coreobjectstorage "github.com/parstis/bnpl-backend/pkg/core/objectstorage"
)

func TestStorePutIfAbsentAndGet(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	payload := []byte("immutable audit payload")
	info, err := store.PutIfAbsent(
		context.Background(),
		"audit/2026/09/event.json",
		bytes.NewReader(payload),
		int64(len(payload)),
		coreobjectstorage.PutOptions{
			ContentType: "application/json",
			Metadata:    map[string]string{"schema": "v1"},
		},
	)
	if err != nil {
		t.Fatalf("PutIfAbsent() error = %v", err)
	}

	if info.Size != int64(len(payload)) {
		t.Fatalf("PutIfAbsent() size = %d, want %d", info.Size, len(payload))
	}

	if info.ETag == "" {
		t.Fatal("PutIfAbsent() ETag is empty")
	}

	body, gotInfo, err := store.Get(context.Background(), "audit/2026/09/event.json")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer body.Close()

	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Fatalf("Get() payload = %q, want %q", got, payload)
	}

	if gotInfo.ETag != info.ETag {
		t.Fatalf("Get() ETag = %q, want %q", gotInfo.ETag, info.ETag)
	}

	if gotInfo.Metadata["schema"] != "v1" {
		t.Fatalf("Get() metadata schema = %q, want v1", gotInfo.Metadata["schema"])
	}
}

func TestStorePutIfAbsentDoesNotOverwrite(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := store.PutIfAbsent(context.Background(), "same-key", bytes.NewBufferString("first"), 5, coreobjectstorage.PutOptions{}); err != nil {
		t.Fatalf("first PutIfAbsent() error = %v", err)
	}

	if _, err := store.PutIfAbsent(context.Background(), "same-key", bytes.NewBufferString("second"), 6, coreobjectstorage.PutOptions{}); !errors.Is(err, coreobjectstorage.ErrExists) {
		t.Fatalf("second PutIfAbsent() error = %v, want ErrExists", err)
	}
}

func TestStoreRejectsPathTraversal(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := store.PutIfAbsent(context.Background(), "../escape", bytes.NewBufferString("x"), 1, coreobjectstorage.PutOptions{}); err == nil {
		t.Fatal("PutIfAbsent() error = nil, want path traversal rejection")
	}
}

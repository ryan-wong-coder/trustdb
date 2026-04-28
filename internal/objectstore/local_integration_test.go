//go:build integration

package objectstore

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStorePutFileAndOpen(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	src := filepath.Join(tmp, "payload.bin")
	raw := []byte("object store payload")
	if err := os.WriteFile(src, raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store := LocalStore{Root: filepath.Join(tmp, "objects")}
	result, err := store.PutFile(context.Background(), src)
	if err != nil {
		t.Fatalf("PutFile() error = %v", err)
	}
	if result.URI == "" || result.ContentLength != int64(len(raw)) {
		t.Fatalf("PutFile() result = %+v", result)
	}
	f, err := store.Open(result.ContentHash)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("stored payload = %q, want %q", got, raw)
	}
}

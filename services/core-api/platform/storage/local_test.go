package storage

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

func TestLocalStorePutGetDelete(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	object, err := store.Put(context.Background(), "sources/demo/file.txt", strings.NewReader("نص عربي"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if object.Size != int64(len("نص عربي")) {
		t.Fatalf("size = %d", object.Size)
	}
	reader, metadata, err := store.Get(context.Background(), "sources/demo/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || string(content) != "نص عربي" || metadata.Size != object.Size {
		t.Fatalf("content = %q, metadata = %+v, error = %v", content, metadata, err)
	}
	if err := store.Delete(context.Background(), "sources/demo/file.txt"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(context.Background(), "sources/demo/file.txt"); err != ErrNotFound {
		t.Fatalf("missing object error = %v", err)
	}
}

func TestLocalStoreRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	store, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, root+"/linked"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), "linked/file.txt", strings.NewReader("x"), "text/plain"); err != ErrInvalidKey {
		t.Fatalf("symlink error = %v", err)
	}
}

func TestLocalStoreRejectsTraversal(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), "../escape.txt", strings.NewReader("x"), "text/plain"); err != ErrInvalidKey {
		t.Fatalf("traversal error = %v", err)
	}
}

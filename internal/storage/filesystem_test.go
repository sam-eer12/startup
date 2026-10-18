package storage

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestContentAddressedConcurrentWritesAndIntegrity(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFilesystem(dir)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := store.Put([]byte(`{"redacted":true}`)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one immutable object: %v", err)
	}
	ref, err := store.Put([]byte(`{"redacted":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Name() != strings.TrimPrefix(ref, "sha256:")+".json" {
		t.Fatal("object key is not its digest")
	}
	if err := os.WriteFile(filepath.Join(dir, entries[0].Name()), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put([]byte(`{"redacted":true}`)); err == nil {
		t.Fatal("corrupt existing object accepted")
	}
}

func TestQuotaAndRetention(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewFilesystem(dir)
	store.maxBytes = 3
	ref, err := store.Put([]byte("one"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put([]byte("one")); err != nil {
		t.Fatal("duplicate consumed quota")
	}
	if _, err := store.Put([]byte("two")); err == nil {
		t.Fatal("quota not enforced")
	}
	path := filepath.Join(dir, strings.TrimPrefix(ref, "sha256:")+".json")
	old := time.Now().Add(-Retention - time.Minute)
	if err := os.WriteFile(filepath.Join(dir, ".object-interrupted"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if err := store.Prune(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".object-interrupted")); !os.IsNotExist(err) {
		t.Fatal("interrupted write retained")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expired sample retained")
	}
	if _, err := store.Put([]byte("two")); err != nil {
		t.Fatal("expired data consumed quota")
	}
}

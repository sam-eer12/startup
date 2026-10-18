package durable

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptedOperationalObjectsAndIntegrity(t *testing.T) {
	root := t.TempDir()
	key := bytes.Repeat([]byte{3}, 32)
	s, err := NewObjects(root, key)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"synthetic_secret":"seed_DO_NOT_EXPORT_9842"}`)
	ref, err := s.Put(raw)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := s.Put(raw); err != nil || second != ref {
		t.Fatal("retry lost object identity", err)
	}
	path, _ := s.path(ref)
	encrypted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, raw) || bytes.Contains(encrypted, []byte("seed_DO_NOT_EXPORT")) {
		t.Fatal("plaintext persisted")
	}
	reopened, err := NewObjects(root, key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get(ref)
	if err != nil || !bytes.Equal(raw, got) {
		t.Fatal("restart did not recover bytes", err)
	}
	encrypted[len(encrypted)-1] ^= 1
	if err = os.WriteFile(path, encrypted, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(ref); err == nil {
		t.Fatal("tampering accepted")
	}
	if _, err = s.Put(raw); err == nil {
		t.Fatal("corrupt existing object overwritten")
	}
	if _, err = s.Get("../../elsewhere"); err == nil {
		t.Fatal("unsafe object ref")
	}
	if entries, _ := filepath.Glob(filepath.Join(root, ".pending-*")); len(entries) != 0 {
		t.Fatal("temporary files left")
	}
}

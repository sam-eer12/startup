// Package durable is a synthetic-only object-first/ledger-commit feasibility
// spike. It has no provider-facing HTTP endpoint or production delivery worker.
package durable

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sam-eer12/startup/internal/capture"
	"github.com/sam-eer12/startup/internal/trust"
)

type Objects struct {
	root string
	aead cipher.AEAD
	mu   sync.Mutex
}

// Each Objects directory has one writer process and one trusted route scope.
// Key lifecycle/KMS, replication and distributed quota are outside this spike.
func NewObjects(root string, key []byte) (*Objects, error) {
	if len(key) != 32 {
		return nil, errors.New("AES-256 key required")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Objects{root: root, aead: aead}, nil
}

func (s *Objects) path(ref string) (string, error) {
	if len(ref) != 71 || !strings.HasPrefix(ref, "sha256:") {
		return "", errors.New("invalid object ref")
	}
	for _, c := range ref[7:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", errors.New("invalid object ref")
		}
	}
	return filepath.Join(s.root, ref[7:]+".aesgcm"), nil
}

func (s *Objects) Get(ref string) ([]byte, error) {
	path, err := s.path(ref)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	encrypted, err := io.ReadAll(io.LimitReader(f, capture.MaxBodyBytes+128))
	if err != nil {
		return nil, err
	}
	if len(encrypted) < s.aead.NonceSize() || len(encrypted) > capture.MaxBodyBytes+s.aead.NonceSize()+s.aead.Overhead() {
		return nil, errors.New("invalid encrypted object")
	}
	raw, err := s.aead.Open(nil, encrypted[:s.aead.NonceSize()], encrypted[s.aead.NonceSize():], []byte(ref))
	if err != nil {
		return nil, errors.New("object integrity failure")
	}
	if trust.Digest(raw) != ref {
		return nil, errors.New("object digest mismatch")
	}
	return raw, nil
}

func (s *Objects) Put(raw []byte) (string, error) {
	if len(raw) > capture.MaxBodyBytes {
		return "", errors.New("object too large")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ref := trust.Digest(raw)
	target, _ := s.path(ref)
	if existing, err := s.Get(ref); err == nil {
		if !bytes.Equal(existing, raw) {
			return "", errors.New("object collision")
		}
		return ref, s.syncDir()
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	// Admission bounds the development spool; referenced objects are never aged
	// out to make room. Orphans remain for explicit reconciliation.
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return "", err
	}
	var used int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return "", err
		}
		used += info.Size()
	}
	if len(entries) >= 10000 || used+int64(len(raw)+s.aead.NonceSize()+s.aead.Overhead()) > 64*1024*1024 {
		return "", errors.New("operational storage full")
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	encrypted := s.aead.Seal(nonce, nonce, raw, []byte(ref))
	tmp, err := os.CreateTemp(s.root, ".pending-")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err = tmp.Write(encrypted); err != nil {
		return "", err
	}
	if err = tmp.Sync(); err != nil {
		return "", err
	}
	if err = tmp.Close(); err != nil {
		return "", err
	}
	if err = os.Link(tmp.Name(), target); err != nil {
		return "", err
	}
	if err = s.syncDir(); err != nil {
		return "", err
	}
	return ref, nil
}

func (s *Objects) syncDir() error {
	dir, err := os.Open(s.root)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

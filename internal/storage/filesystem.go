// Package storage contains the development-only content-addressed object store.
// Production storage must provide authenticated access and encryption at rest.
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Filesystem struct {
	root     string
	mu       sync.Mutex
	maxBytes int64
}

const Retention = 24 * time.Hour

func NewFilesystem(root string) (*Filesystem, error) {
	if root == "" {
		return nil, errors.New("object directory is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &Filesystem{root: root, maxBytes: 64 * 1024 * 1024}, nil
}

// Prune bounds development telemetry retention. A single collector owns a volume.
// This store does not implement the durable webhook acceptance protocol.
func (s *Filesystem) Prune() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _, err := s.pruneLocked(time.Now())
	return err
}

func (s *Filesystem) pruneLocked(now time.Time) (int64, int, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return 0, 0, err
	}
	var total int64
	count := 0
	for _, entry := range entries {
		// No writer is active while this process-local mutex is held. A leftover
		// temporary object is from an interrupted previous write.
		if strings.HasPrefix(entry.Name(), ".object-") {
			if !entry.Type().IsRegular() {
				return 0, 0, errors.New("unexpected temporary object")
			}
			if err := os.Remove(filepath.Join(s.root, entry.Name())); err != nil {
				return 0, 0, err
			}
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, 0, err
		}
		if !info.Mode().IsRegular() {
			return 0, 0, errors.New("unexpected nonregular object")
		}
		if now.Sub(info.ModTime()) >= Retention {
			if err := os.Remove(filepath.Join(s.root, entry.Name())); err != nil {
				return 0, 0, err
			}
		} else {
			total += info.Size()
			count++
		}
	}
	return total, count, nil
}

// Put stores only already-redacted bytes. Link provides atomic create-if-absent;
// a concurrent writer cannot replace an existing content-addressed object.
func (s *Filesystem) Put(body []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	total, count, err := s.pruneLocked(time.Now())
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	name := hex.EncodeToString(sum[:])
	target := filepath.Join(s.root, name+".json")
	if existing, err := os.ReadFile(target); err == nil {
		if sha256.Sum256(existing) != sum {
			return "", errors.New("existing object integrity check failed")
		}
		// Identical redacted structures share an object. Retain it for the most
		// recent observation's TTL rather than returning a nearly expired ref.
		now := time.Now()
		if err := os.Chtimes(target, now, now); err != nil {
			return "", err
		}
		return "sha256:" + name, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if total+int64(len(body)) > s.maxBytes || count >= 10000 {
		return "", errors.New("local sample quota exhausted")
	}
	tmp, err := os.CreateTemp(s.root, ".object-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := tmp.Write(body); err != nil {
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Link(tmp.Name(), target); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
		existing, readErr := os.ReadFile(target)
		if readErr != nil || sha256.Sum256(existing) != sum {
			return "", fmt.Errorf("existing object integrity check failed")
		}
	}
	return "sha256:" + name, nil
}

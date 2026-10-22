// Package storage keeps uploaded files in one directory on the VPS disk.
// Keys are random names generated here, so callers never build a path from user input.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Store keeps whole objects by key. Datasets live in one (ADR 0008); Disk is the one for tests and development.
type Store interface {
	Put(key string, data []byte) error
	Get(key string) ([]byte, error)
	Delete(key string) error
}

// ponytail: one local directory; swap for object storage only when the disk fills or load demands it.
type Disk struct {
	dir string
}

// Open makes sure dir exists and returns a store that writes into it.
func Open(dir string) (*Disk, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create storage directory: %w", err)
	}
	return &Disk{dir: dir}, nil
}

// Save streams r to a new file and returns its key and size. A failed write leaves no file behind.
func (d *Disk) Save(r io.Reader) (string, int64, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", 0, err
	}
	key := hex.EncodeToString(b[:])
	f, err := os.OpenFile(d.path(key), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", 0, err
	}
	n, err := io.Copy(f, r)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(d.path(key))
		return "", 0, err
	}
	return key, n, nil
}

// Put writes data under key, replacing any file there. Readers never see a half-written file.
func (d *Disk) Put(key string, data []byte) error {
	f, err := os.CreateTemp(d.dir, ".tmp-*")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), d.path(key))
	}
	if err != nil {
		_ = os.Remove(f.Name())
	}
	return err
}

// Open returns the stored file for key. The caller closes it.
func (d *Disk) Open(key string) (*os.File, error) {
	return os.Open(d.path(key))
}

// Get returns the data stored under key.
func (d *Disk) Get(key string) ([]byte, error) {
	return os.ReadFile(d.path(key))
}

// Delete removes the data under key; a key with nothing stored is already deleted.
func (d *Disk) Delete(key string) error {
	if err := os.Remove(d.path(key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Remove deletes the stored file for key.
func (d *Disk) Remove(key string) error {
	return os.Remove(d.path(key))
}

func (d *Disk) path(key string) string {
	return filepath.Join(d.dir, key)
}

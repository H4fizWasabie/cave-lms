// Package storage keeps uploaded files: on the VPS disk (Disk), or for Datasets in production in R2 (ADR 0008).
// Keys are names the app generates, so callers never build a path from user input.
package storage

import (
	"context"
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
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}

// Disk keeps files in one local directory: the Gallery's and certificates' files, and Datasets in tests and
// development. ctx is unused: local file calls do not block on the network.
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
func (d *Disk) Save(r io.Reader) (string, int64, error) { return d.SaveAs("", r) }

// SaveAs is Save with a key starting with prefix, a constant with no path separator.
func (d *Disk) SaveAs(prefix string, r io.Reader) (string, int64, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", 0, err
	}
	key := prefix + hex.EncodeToString(b[:])
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
func (d *Disk) Put(_ context.Context, key string, data []byte) error {
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
func (d *Disk) Get(_ context.Context, key string) ([]byte, error) {
	return os.ReadFile(d.path(key))
}

// Delete removes the data under key; a key with nothing stored is already deleted.
func (d *Disk) Delete(_ context.Context, key string) error {
	if err := os.Remove(d.path(key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (d *Disk) path(key string) string {
	return filepath.Join(d.dir, key)
}

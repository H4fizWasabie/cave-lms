// Package storage keeps uploaded files in one directory on the VPS disk.
// Keys are random names generated here, so callers never build a path from user input.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

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

// Remove deletes the stored file for key.
func (d *Disk) Remove(key string) error {
	return os.Remove(d.path(key))
}

func (d *Disk) path(key string) string {
	return filepath.Join(d.dir, key)
}

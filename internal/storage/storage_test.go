package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

type failingReader struct{ data string }

var errRead = errors.New("read failed")

func (r *failingReader) Read(p []byte) (int, error) {
	if r.data == "" {
		return 0, errRead
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

func TestSaveFromFailingReaderLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.Save(&failingReader{data: "partial"}); !errors.Is(err, errRead) {
		t.Fatalf("Save err = %v, want %v", err, errRead)
	}
	if got := entries(t, dir); len(got) != 0 {
		t.Fatalf("storage directory holds %v after a failed Save", got)
	}
}

func TestSaveAsFailsWhenFileCannotBeCreated(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if key, n, err := d.SaveAs("missing-dir/", strings.NewReader("x")); err == nil || key != "" || n != 0 {
		t.Fatalf("SaveAs = %q, %d, %v; want an error and nothing else", key, n, err)
	}
}

func TestSaveAsKeyStartsWithPrefixAndGetReturnsData(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key, n, err := d.SaveAs("cert-", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "cert-") || n != 5 {
		t.Fatalf("SaveAs = %q, %d; want a cert- key and 5 bytes", key, n)
	}
	got, err := d.Get(context.Background(), key)
	if err != nil || string(got) != "hello" {
		t.Fatalf("Get = %q, %v; want hello", got, err)
	}
	f, err := d.Open(key)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if b, _ := io.ReadAll(f); !bytes.Equal(b, []byte("hello")) {
		t.Fatalf("Open read %q", b)
	}
}

func TestFailedPutLeavesNoTempFileAndKeepsEarlierData(t *testing.T) {
	ctx := context.Background()
	for name, fail := range map[string]func(t *testing.T, dir string) (key string){
		// The key names a directory, so the final rename fails after the temp file is written.
		"rename": func(t *testing.T, dir string) string {
			if err := os.Mkdir(dir+"/blocked", 0o700); err != nil {
				t.Fatal(err)
			}
			return "blocked"
		},
		// A read-only directory refuses the temp file in the first place.
		"create temp": func(t *testing.T, dir string) string {
			if err := os.Chmod(dir, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			if f, err := os.CreateTemp(dir, ".probe-*"); err == nil {
				f.Close()
				os.Remove(f.Name())
				t.Skip("directory permissions do not stop this user (root?)")
			}
			return "kept"
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			d, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := d.Put(ctx, "kept", []byte("old")); err != nil {
				t.Fatal(err)
			}
			key := fail(t, dir)
			if err := d.Put(ctx, key, []byte("new")); err == nil {
				t.Fatal("Put succeeded, want an error")
			}
			for _, n := range entries(t, dir) {
				if strings.HasPrefix(n, ".tmp-") {
					t.Fatalf("temp file %s left behind", n)
				}
			}
			if got, err := d.Get(ctx, "kept"); err != nil || string(got) != "old" {
				t.Fatalf("Get kept = %q, %v; want old", got, err)
			}
		})
	}
}

func TestPutReplacesAndDeleteIsIdempotent(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"one", "two"} {
		if err := d.Put(ctx, "k", []byte(v)); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := d.Get(ctx, "k"); string(got) != "two" {
		t.Fatalf("Get = %q, want two", got)
	}
	if err := d.Delete(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete of an absent key = %v, want nil", err)
	}
}

func TestDeleteReportsOtherErrors(t *testing.T) {
	dir := t.TempDir()
	d, _ := Open(dir)
	if err := os.MkdirAll(dir+"/full/inner", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(context.Background(), "full"); err == nil {
		t.Fatal("Delete of a non-empty directory = nil, want an error")
	}
}

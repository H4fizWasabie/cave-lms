//go:build unix

package storage

import (
	"context"
	"os/signal"
	"strings"
	"syscall"
	"testing"
)

// TestPutWhoseWriteFailsLeavesNoTempFileAndKeepsEarlierData makes the temp file's write fail by capping file size.
func TestPutWhoseWriteFailsLeavesNoTempFileAndKeepsEarlierData(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Put(ctx, "k", []byte("old")); err != nil {
		t.Fatal(err)
	}

	var old syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &old); err != nil {
		t.Skip(err)
	}
	signal.Ignore(syscall.SIGXFSZ) // otherwise the oversize write kills the process instead of failing
	defer signal.Reset(syscall.SIGXFSZ)
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: 4, Max: old.Max}); err != nil {
		t.Skip(err)
	}
	err = d.Put(ctx, "k", []byte(strings.Repeat("x", 1<<16)))
	_ = syscall.Setrlimit(syscall.RLIMIT_FSIZE, &old)

	if err == nil {
		t.Fatal("Put succeeded, want a write error")
	}
	for _, n := range entries(t, dir) {
		if strings.HasPrefix(n, ".tmp-") {
			t.Fatalf("temp file %s left behind", n)
		}
	}
	if got, err := d.Get(ctx, "k"); err != nil || string(got) != "old" {
		t.Fatalf("Get = %q, %v; want old", got, err)
	}
}

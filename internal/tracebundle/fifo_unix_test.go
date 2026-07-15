//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package tracebundle

import (
	"context"
	"errors"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOpenRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), path); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("FIFO error = %v", err)
	}
}

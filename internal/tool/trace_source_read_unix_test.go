//go:build !windows

package tool

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestB1697HeldSourceReadRejectsFIFOSwapWithoutBlocking(t *testing.T) {
	ctx, path, _, result := b1697ToolNativeSource(t)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := readTraceQuerySourceFile(ctx, result.TraceQuerySourceRead)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("held original receipt read a FIFO replacement")
		}
	case <-time.After(time.Second):
		t.Fatal("raw capture read blocked opening a replacement FIFO")
	}
}

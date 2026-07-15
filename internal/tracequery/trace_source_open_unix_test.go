//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package tracequery

import (
	"context"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestTraceSourceEntrypointsRejectFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.systrace")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func() error{
		"build_index": func() error {
			_, err := BuildIndex(context.Background(), path)
			return err
		},
		"stream_scan": func() error {
			_, err := StreamScan(context.Background(), path, TraceFlavorAuto, func(Event) bool { return true })
			return err
		},
	}
	for name, run := range tests {
		t.Run(name, func(t *testing.T) {
			result := make(chan error, 1)
			go func() { result <- run() }()
			select {
			case err := <-result:
				if err == nil || !strings.Contains(err.Error(), "not a regular file") {
					t.Fatalf("FIFO error = %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("trace source FIFO open blocked")
			}
		})
	}
}

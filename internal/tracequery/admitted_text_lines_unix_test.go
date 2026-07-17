//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestStreamAdmittedTraceTextLinesRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	callbacks := 0
	_, err := StreamAdmittedTraceTextLines(context.Background(), path, func(AdmittedTraceTextLine) bool {
		callbacks++
		return true
	})
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("FIFO was not rejected by the nonblocking regular-file gate: %v", err)
	}
	if callbacks != 0 {
		t.Fatalf("FIFO reached raw line consumer: callbacks=%d", callbacks)
	}
	if info, statErr := os.Stat(path); statErr != nil || info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("test fixture is not a FIFO: info=%v err=%v", info, statErr)
	}
}

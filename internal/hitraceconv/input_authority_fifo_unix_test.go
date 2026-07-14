//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package hitraceconv

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestConversionInputAuthorityFIFOPathsFailWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "input.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	type openResult struct {
		authority *conversionInputAuthority
		err       error
	}
	opened := make(chan openResult, 1)
	go func() {
		authority, err := openConversionInputAuthority(fifo)
		opened <- openResult{authority: authority, err: err}
	}()
	select {
	case result := <-opened:
		if result.authority != nil || conversionInputErrorCode(result.err) != ConversionInputCodeNotRegular {
			t.Fatalf("FIFO open authority=%v err=%v", result.authority, result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("FIFO input open blocked")
	}

	path := filepath.Join(dir, "stable.sys")
	if err := os.WriteFile(path, []byte("stable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(path)
	if unavailableConversionInputAuthority(t, err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	validated := make(chan error, 1)
	go func() { validated <- authority.Validate(conversionInputStagePreCommit) }()
	select {
	case err := <-validated:
		if conversionInputErrorCode(err) != ConversionInputCodeGenerationChanged {
			t.Fatalf("FIFO replacement error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("FIFO path validation blocked")
	}
}

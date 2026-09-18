package hitraceconv

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestManagedOutputDirectoryReleaseFailureCannotRegainCleanupAuthority(t *testing.T) {
	managed, err := NewManagedOutputDirectory(t.TempDir(), "", "managed-release-*")
	if err != nil {
		t.Fatal(err)
	}
	defer managed.Cleanup()
	sentinel := filepath.Join(managed.Path(), "successful-output")
	if err := os.WriteFile(sentinel, []byte("retain after release failure"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Release all real handles, then inject the OS-close failure surface at
	// this exact boundary. os.Root.Close is idempotent, so double-closing it
	// would not actually exercise release-error handling.
	failure := errors.New("injected held-handle close failure")
	releaseErr := managed.closeWithRelease(func(dir *privateConversionDir) error {
		return errors.Join(dir.closeHandlesLocked(), failure)
	})
	if !errors.Is(releaseErr, failure) {
		t.Fatal("injected release failure was ignored")
	}
	if !managed.dir.terminal || managed.dir.root != nil {
		t.Fatal("failed release left live/retryable directory authority")
	}
	if err := managed.Cleanup(); err == nil || err.Error() != releaseErr.Error() {
		t.Fatalf("cleanup after failed release did not preserve terminal error: close=%v cleanup=%v", releaseErr, err)
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "retain after release failure" {
		t.Fatalf("failed release triggered a pathname rollback: %q %v", data, err)
	}
}

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

func TestManagedOutputDirectoryCreateFileUsesExclusiveHeldChild(t *testing.T) {
	managed, err := NewManagedOutputDirectory(t.TempDir(), "", "stream-*")
	if err != nil {
		t.Fatal(err)
	}
	defer managed.Cleanup()
	for _, name := range []string{"", "..", "../escape", "nested/child", "/tmp/escape"} {
		if file, err := managed.CreateFile(name); err == nil {
			file.Close()
			t.Fatalf("unsafe child accepted: %s", name)
		}
	}
	file, err := managed.CreateFile("input.trace")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if file, err := managed.CreateFile("input.trace"); err == nil {
		file.Close()
		t.Fatal("existing child overwritten")
	}
	moved := managed.Path() + "-moved"
	if err := os.Rename(managed.Path(), moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(managed.Path(), 0700); err != nil {
		t.Fatal(err)
	}
	if file, err := managed.CreateFile("unowned"); err == nil {
		file.Close()
		t.Fatal("directory replacement acquired write authority")
	}
	if _, err := os.Stat(filepath.Join(managed.Path(), "unowned")); !os.IsNotExist(err) {
		t.Fatal("replacement directory was written")
	}
}

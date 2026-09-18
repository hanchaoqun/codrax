package traceinput

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestManagedDirectoryCleanupUsesHeldAuthority(t *testing.T) {
	dir, err := newManagedDirectory(filepath.Join(t.TempDir(), "runtime"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer dir.close()
	child := filepath.Join(dir.path, "nested", "partial")
	if err := os.MkdirAll(filepath.Dir(child), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child, []byte("owned partial output"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dir.cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dir.path); !os.IsNotExist(err) {
		t.Fatalf("owned failed output remains: %v", err)
	}
	if err := dir.cleanup(); err != nil {
		t.Fatalf("cleanup is not idempotent: %v", err)
	}
}

func TestManagedDirectorySuccessfulCloseRetainsOutputAndRevokesCleanup(t *testing.T) {
	dir, err := newManagedDirectory(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(dir.path, "successful.systrace")
	if err := os.WriteFile(child, []byte("successful output"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dir.close(); err != nil {
		t.Fatal(err)
	}
	if err := dir.validate(); err == nil {
		t.Fatal("closed directory retained publication authority")
	}
	if err := dir.cleanup(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(child); err != nil || string(data) != "successful output" {
		t.Fatalf("cleanup after release removed successful output: %q %v", data, err)
	}
}

func TestManagedDirectoryRenameSwapCannotDeleteReplacementOrRearm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows held DELETE guard prevents this rename; converter has dedicated guard regressions")
	}
	dir, err := newManagedDirectory(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer dir.close()
	if err := dir.validate(); err != nil {
		t.Fatal(err)
	}
	// Deterministically replace the public directory after the caller's last
	// valid observation. Cleanup must use held authority, not that pathname.
	moved := dir.path + ".owned-moved"
	if err := os.Rename(dir.path, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir.path, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(dir.path, "replacement-sentinel")
	if err := os.WriteFile(sentinel, []byte("not-owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstErr := dir.cleanup()
	if firstErr == nil {
		t.Fatal("directory replacement was accepted for cleanup")
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "not-owned" {
		t.Fatalf("replacement directory was deleted: %q %v", data, err)
	}
	saved := dir.path + ".replacement-saved"
	if err := os.Rename(dir.path, saved); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(moved, dir.path); err != nil {
		t.Fatal(err)
	}
	if retryErr := dir.cleanup(); retryErr == nil || retryErr.Error() != firstErr.Error() {
		t.Fatalf("terminal failed cleanup rearmed after ABA: first=%v retry=%v", firstErr, retryErr)
	}
	if _, err := os.Stat(dir.path); err != nil {
		t.Fatalf("revoked cleanup deleted restored original: %v", err)
	}
}

func TestManagedDirectoryAncestorSwapCleansHeldParentNotReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows held parent/DELETE semantics have dedicated converter regressions")
	}
	root := t.TempDir()
	parent := filepath.Join(root, "runtime")
	dir, err := newManagedDirectory(parent, "")
	if err != nil {
		t.Fatal(err)
	}
	defer dir.close()
	// Use the canonical path returned by the authority (including /private
	// on Darwin), not an unresolved display alias of the parent.
	parent = filepath.Dir(dir.path)
	if err := os.WriteFile(filepath.Join(dir.path, "owned-partial"), []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	movedParent := parent + ".moved"
	if err := os.Rename(parent, movedParent); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(parent, filepath.Base(dir.path))
	if err := os.MkdirAll(replacement, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(replacement, "sentinel")
	if err := os.WriteFile(sentinel, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dir.close(); err == nil {
		t.Fatal("Close accepted a retargeted public path")
	}
	// Failed pre-release validation must keep the authority usable: the old
	// parent handle can still clean only our real directory under its new name.
	if err := dir.cleanup(); err != nil {
		t.Fatalf("held cleanup after Close validation failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(movedParent, filepath.Base(dir.path))); !os.IsNotExist(err) {
		t.Fatalf("held original was not cleaned: %v", err)
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "replacement" {
		t.Fatalf("ancestor replacement was touched: %q %v", data, err)
	}
}

func TestManagedDirectoryDelegatesDeletionToSharedAuthority(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve source path")
	}
	body, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "managed_directory.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, forbidden := range []string{"os.RemoveAll(", "os.Remove(", "os.MkdirTemp("} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("managed cleanup reintroduced pathname authority: %s", forbidden)
		}
	}
	for _, required := range []string{"hitraceconv.NewManagedOutputDirectory(", "d.authority.Cleanup()", "d.authority.Close()"} {
		if !strings.Contains(text, required) {
			t.Fatalf("shared held-directory lifecycle missing %q", required)
		}
	}
}

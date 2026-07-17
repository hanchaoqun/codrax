//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package repl

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func TestRuntimeArtifactStoreLoadLatestRejectsFIFOWithoutBlocking(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runtime_artifacts")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "latest.json"), 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	store := NewRuntimeArtifactStore(root)
	done := make(chan error, 1)
	go func() {
		_, err := store.LoadLatest()
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("LoadLatest error=%v, want FIFO rejection", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("LoadLatest blocked while opening FIFO")
	}
}

func TestRuntimeArtifactStoreLoadLatestRejectsDevice(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runtime_artifacts")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/null", filepath.Join(root, "latest.json")); err != nil {
		t.Fatalf("symlink device: %v", err)
	}
	store := NewRuntimeArtifactStore(root)
	if _, err := store.LoadLatest(); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("LoadLatest error=%v, want device rejection", err)
	}
}

func TestReadRuntimeArtifactSnapshotRejectsPathReplacement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runtime_artifacts")
	store := NewRuntimeArtifactStore(root)
	ref, err := store.Put("log", "stable log\n", "app.log")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.SaveLatest(RuntimeArtifactSnapshot{Log: ref}); err != nil {
		t.Fatalf("SaveLatest: %v", err)
	}
	path := filepath.Join(root, "latest.json")
	file, opened, err := filegeneration.OpenRegularReadOnly(path)
	if err != nil {
		t.Fatalf("open latest: %v", err)
	}
	defer file.Close()

	replacement := filepath.Join(root, "replacement.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacement, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if _, err := readRuntimeArtifactSnapshot(path, file, opened); err == nil || !strings.Contains(err.Error(), "path generation changed") {
		t.Fatalf("readRuntimeArtifactSnapshot error=%v, want replacement rejection", err)
	}
}

func TestRuntimeArtifactStoreLoadRejectsFIFOWithoutBlocking(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runtime_artifacts")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "trace-fifo.txt")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	store := NewRuntimeArtifactStore(root)
	ref := runtimeArtifactRefForNonRegularTest("trace-fifo", path)
	done := make(chan error, 1)
	go func() {
		_, err := store.Load(ref, 1)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("Load error=%v, want FIFO rejection", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Load blocked while opening FIFO")
	}
}

func TestRuntimeArtifactStoreLoadRejectsDevice(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runtime_artifacts")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "trace-device.txt")
	if err := os.Symlink("/dev/null", path); err != nil {
		t.Fatalf("symlink device: %v", err)
	}
	store := NewRuntimeArtifactStore(root)
	ref := runtimeArtifactRefForNonRegularTest("trace-device", path)
	if _, err := store.Load(ref, 1); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("Load error=%v, want device rejection", err)
	}
}

func runtimeArtifactRefForNonRegularTest(id, path string) RuntimeArtifactRef {
	sum := sha256.Sum256([]byte("x"))
	return RuntimeArtifactRef{
		SchemaVersion: runtimeArtifactStoreSchemaVersion,
		ID:            id,
		Kind:          "trace",
		Path:          path,
		Bytes:         1,
		SHA256:        hex.EncodeToString(sum[:]),
	}
}

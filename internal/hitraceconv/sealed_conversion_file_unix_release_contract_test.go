//go:build unix

package hitraceconv

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestReleaseSealedConversionFileUnixPathReplacementKeepsHeldGeneration(t *testing.T) {
	dir, err := newPrivateConversionDir(t.TempDir(), "codrax-sealed-replace-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cleanupErr := dir.FinalizeCleanup(); cleanupErr != nil {
			t.Errorf("cleanup replacement fixture: %v", cleanupErr)
		}
	}()
	path, err := dir.ChildPath("report")
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("generation-A")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	sealed, err := dir.AdoptRegularChild("report", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(dir.Path(), "report.old")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("generation-B"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, readErr := io.ReadAll(sealed.Reader())
	if readErr != nil || string(got) != string(want) {
		t.Fatalf("held reader followed replacement: got=%q err=%v", got, readErr)
	}
	if err := sealed.Validate(); !errors.Is(err, errSealedConversionFileIdentityChanged) {
		t.Fatalf("replacement validation error=%v, want identity changed", err)
	}
	if err := sealed.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseSealedConversionFileUnixDetectsHeldInodeMutation(t *testing.T) {
	dir, err := newPrivateConversionDir(t.TempDir(), "codrax-sealed-mutate-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cleanupErr := dir.FinalizeCleanup(); cleanupErr != nil {
			t.Errorf("cleanup mutation fixture: %v", cleanupErr)
		}
	}()
	path, err := dir.ChildPath("report")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("generation-A"), 0o600); err != nil {
		t.Fatal(err)
	}
	sealed, err := dir.AdoptRegularChild("report", true)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	writer, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("-mutated")); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sealed.Validate(); !errors.Is(err, errSealedConversionFileIdentityChanged) {
		t.Fatalf("held mutation validation error=%v, want identity changed", err)
	}
	if err := sealed.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseSealedConversionFileUnixFinishPreservesOperationAndGenerationErrors(t *testing.T) {
	dir, err := newPrivateConversionDir(t.TempDir(), "codrax-sealed-errors-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cleanupErr := dir.FinalizeCleanup(); cleanupErr != nil {
			t.Errorf("cleanup combined-error fixture: %v", cleanupErr)
		}
	}()
	path, err := dir.ChildPath("report")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("generation-A"), 0o600); err != nil {
		t.Fatal(err)
	}
	sealed, err := dir.AdoptRegularChild("report", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("generation-A-mutated"), 0o600); err != nil {
		sealed.Close()
		t.Fatal(err)
	}
	operationErr := errors.New("parser rejected record")
	err = finishSealedConversionFile(sealed, operationErr)
	for _, want := range []error{operationErr, errSealedConversionFileIdentityChanged} {
		if !errors.Is(err, want) {
			t.Fatalf("combined finish error %v lost %v", err, want)
		}
	}
}

func TestReleaseSealedConversionFileUnixDetectsSameSizeRewriteWithRestoredMtime(t *testing.T) {
	dir, err := newPrivateConversionDir(t.TempDir(), "codrax-sealed-aba-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cleanupErr := dir.FinalizeCleanup(); cleanupErr != nil {
			t.Errorf("cleanup same-size rewrite fixture: %v", cleanupErr)
		}
	}()
	path, err := dir.ChildPath("report")
	if err != nil {
		t.Fatal(err)
	}
	baseline := []byte("generation-A")
	if err := os.WriteFile(path, baseline, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := dir.AdoptRegularChild("report", true)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := os.WriteFile(path, []byte("generation-B"), 0o600); err != nil {
		sealed.Close()
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		sealed.Close()
		t.Fatal(err)
	}
	if err := sealed.Validate(); !errors.Is(err, errSealedConversionFileIdentityChanged) {
		sealed.Close()
		t.Fatalf("same-size restored-mtime validation error=%v, want identity changed", err)
	}
	if err := sealed.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseSealedConversionFileUnixRejectsNonRegularChildren(t *testing.T) {
	dir, err := newPrivateConversionDir(t.TempDir(), "codrax-sealed-types-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cleanupErr := dir.FinalizeCleanup(); cleanupErr != nil {
			t.Errorf("cleanup type fixture: %v", cleanupErr)
		}
	}()

	directory, err := dir.ChildPath("directory")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	symlink, err := dir.ChildPath("symlink")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(directory, symlink); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}
	fifo, err := dir.ChildPath("fifo")
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("FIFO fixture unavailable: %v", err)
	}

	for _, name := range []string{"directory", "symlink", "fifo"} {
		sealed, found, err := dir.TryAdoptRegularChild(name, false)
		if !found || sealed != nil || err == nil {
			t.Fatalf("non-regular %s result: sealed=%v found=%t err=%v", name, sealed, found, err)
		}
	}
}

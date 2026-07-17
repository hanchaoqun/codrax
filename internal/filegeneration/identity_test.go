package filegeneration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIdentityPortableTupleAndStrongComparisonAreClosed(t *testing.T) {
	left := NewPortable(7, 11, 0o644)
	right := NewPortable(7, 11, 0o644)
	if !left.Initialized() || left.Strong() || !left.SameVersion(right) {
		t.Fatalf("portable identity mismatch: left=%q right=%q", left.CacheToken(), right.CacheToken())
	}
	if left.SameVersion(NewPortable(8, 11, 0o644)) || left.SameVersion(Identity{}) {
		t.Fatal("portable identity admitted a different or uninitialized tuple")
	}
}

func TestIdentityFromFileRejectsSameSizeRestoredMtimeRewriteWhenStrong(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generation.trace")
	original := "first-version\n"
	replacement := "other-version\n"
	if len(original) != len(replacement) {
		t.Fatal("test fixture lengths differ")
	}
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	opened, err := FromFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !opened.Strong() {
		t.Skip("host does not expose a strong file generation identity")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := os.WriteFile(path, []byte(replacement), info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	current, err := FromFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if opened.SameVersion(current) {
		t.Fatalf("strong identity missed same-size/restored-mtime rewrite: before=%q after=%q", opened.CacheToken(), current.CacheToken())
	}
	if !strings.HasPrefix(opened.CacheToken(), "strong:") {
		t.Fatalf("strong token lost its closed profile: %q", opened.CacheToken())
	}
}

func TestIdentityFromPathMatchesOpenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "same.trace")
	if err := os.WriteFile(path, []byte("stable\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	opened, err := FromFile(file)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := FromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if !opened.SameVersion(resolved) {
		t.Fatalf("same file identities differ: open=%q path=%q", opened.CacheToken(), resolved.CacheToken())
	}
}

func TestOpenRegularReadOnlyReturnsHeldGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact.txt")
	if err := os.WriteFile(path, []byte("durable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, opened, err := OpenRegularReadOnly(path)
	if err != nil {
		t.Fatalf("OpenRegularReadOnly: %v", err)
	}
	defer file.Close()
	if !opened.Initialized() || !opened.Mode().IsRegular() || opened.Size() != int64(len("durable\n")) {
		t.Fatalf("unexpected held generation: initialized=%t mode=%s size=%d", opened.Initialized(), opened.Mode(), opened.Size())
	}
	if final, err := FromFile(file); err != nil || !opened.SameVersion(final) {
		t.Fatalf("held generation drift: final=%v err=%v", final, err)
	}
	if bound, err := FromPath(path); err != nil || !opened.SameVersion(bound) {
		t.Fatalf("path generation mismatch: bound=%v err=%v", bound, err)
	}
}

func TestOpenRegularReadOnlyRejectsDirectory(t *testing.T) {
	if file, _, err := OpenRegularReadOnly(t.TempDir()); err == nil {
		_ = file.Close()
		t.Fatal("OpenRegularReadOnly should reject a directory")
	}
}

func TestWindowsStrongIdentitySentinelsFailClosed(t *testing.T) {
	for _, test := range []struct {
		name       string
		volume     uint64
		fileIndex  uint64
		changeTime int64
		want       bool
	}{
		{name: "complete", volume: 1, fileIndex: 2, changeTime: 3, want: true},
		{name: "missing_volume", fileIndex: 2, changeTime: 3},
		{name: "missing_file_id", volume: 1, changeTime: 3},
		{name: "missing_change_clock", volume: 1, fileIndex: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := validWindowsStrongIdentity(test.volume, test.fileIndex, test.changeTime); got != test.want {
				t.Fatalf("validWindowsStrongIdentity()=%t want=%t", got, test.want)
			}
		})
	}
}

package logging

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// SEC #29 pin: .codrax log artifacts are owner-only. Default-level (debug)
// logs carry full prompts and repository excerpts; on shared machines other
// local accounts must not be able to read them.
func TestLogDirAndFilesAreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission-bit pin")
	}
	dir := filepath.Join(t.TempDir(), "logs")
	lg, err := NewFromFlags(dir, "debug", false)
	if err != nil {
		t.Fatal(err)
	}
	lg.Info("perm pin line")
	if err := lg.Close(); err != nil {
		t.Fatal(err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("log dir mode = %o, want no group/other bits (0700)", perm)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no log file created")
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Fatalf("log file %s mode = %o, want no group/other bits (0600)", e.Name(), perm)
		}
	}
}

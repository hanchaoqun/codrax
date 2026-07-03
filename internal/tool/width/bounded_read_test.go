package width

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestReadFileBounded pins the stat-first bound: oversized files return the
// typed error WITHOUT reading, within-bound files read normally, and a
// non-positive cap falls back to SourceReadMaxBytes.
func TestReadFileBounded(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "small.txt")
	if err := os.WriteFile(small, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	big := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(big, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := ReadFileBounded(small, 1024)
	if err != nil || string(data) != "line1\nline2\n" {
		t.Fatalf("small read failed: %v %q", err, data)
	}

	_, err = ReadFileBounded(big, 1024)
	var oversized *ErrSourceReadOversized
	if !errors.As(err, &oversized) {
		t.Fatalf("big read must return typed oversized error, got %v", err)
	}
	if oversized.Size != 4096 || oversized.Cap != 1024 {
		t.Fatalf("oversized error must carry stat size and cap: %+v", oversized)
	}

	if _, err := ReadFileBounded(big, 0); err != nil {
		t.Fatalf("non-positive cap must fall back to SourceReadMaxBytes (4KiB well within): %v", err)
	}

	if _, err := ReadFileBounded(filepath.Join(dir, "missing.txt"), 1024); err == nil || errors.As(err, &oversized) {
		t.Fatalf("missing file must return the stat error, not oversized: %v", err)
	}
}

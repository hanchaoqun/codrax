package repomap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestCacheVersionRoundTrip writes a graph to a temp cache dir and
// reads it back through the versioned payload wrapper, verifying
// that a fresh save-then-load reproduces the FileInfo slice.
func TestCacheVersionRoundTrip(t *testing.T) {
	dir := t.TempDir()

	files := []*FileInfo{
		{
			RelPath:  "a.go",
			Language: LangGo,
			Package:  "main",
			Hash:     "deadbeef",
			Size:     42,
			Symbols: []Symbol{
				{Name: "Foo", Kind: "type", File: "a.go", Line: 1, EndLine: 1},
			},
		},
	}
	if err := saveFileInfos(dir, "", files); err != nil {
		t.Fatalf("saveFileInfos: %v", err)
	}
	got := LoadFileInfos(dir)
	if len(got) != 1 {
		t.Fatalf("LoadFileInfos: got %d files, want 1", len(got))
	}
	if got[0].RelPath != "a.go" || got[0].Symbols[0].Name != "Foo" {
		t.Errorf("round-trip corrupted: %+v", got[0])
	}
}

// TestCacheVersionMismatchInvalidates writes a cache with the
// current schema then tampers with the SchemaVersion field and
// verifies LoadFileInfos returns nil (forcing a rescan).
func TestCacheVersionMismatchInvalidates(t *testing.T) {
	dir := t.TempDir()
	files := []*FileInfo{{RelPath: "a.go", Language: LangGo}}
	if err := saveFileInfos(dir, "", files); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, cacheFileInfosFile))
	if err != nil {
		t.Fatal(err)
	}
	var payload cachePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	payload.SchemaVersion = cacheSchemaVersion + 99
	out, _ := json.Marshal(&payload)
	if err := os.WriteFile(filepath.Join(dir, cacheFileInfosFile), out, 0o644); err != nil {
		t.Fatal(err)
	}

	if got := LoadFileInfos(dir); got != nil {
		t.Errorf("expected nil on schema mismatch, got %d files", len(got))
	}
}

// TestCacheExtractorVersionMismatchInvalidates verifies that a
// per-language extractor version bump invalidates caches
// previously written against the older version, but caches that
// match on every key are still accepted.
func TestCacheExtractorVersionMismatchInvalidates(t *testing.T) {
	dir := t.TempDir()
	files := []*FileInfo{{RelPath: "a.go", Language: LangGo}}
	if err := saveFileInfos(dir, "", files); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, cacheFileInfosFile))
	if err != nil {
		t.Fatal(err)
	}
	var payload cachePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	payload.ExtractorVersions[LangGo] = 99
	out, _ := json.Marshal(&payload)
	if err := os.WriteFile(filepath.Join(dir, cacheFileInfosFile), out, 0o644); err != nil {
		t.Fatal(err)
	}

	if got := LoadFileInfos(dir); got != nil {
		t.Errorf("expected nil on extractor version mismatch, got %d files", len(got))
	}
}

// TestCacheChecksumMismatchInvalidates tampers with the Files
// slice after the checksum has been computed and verifies the
// loader rejects the corrupted cache.
func TestCacheChecksumMismatchInvalidates(t *testing.T) {
	dir := t.TempDir()
	files := []*FileInfo{{RelPath: "a.go", Language: LangGo}}
	if err := saveFileInfos(dir, "", files); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, cacheFileInfosFile))
	if err != nil {
		t.Fatal(err)
	}
	var payload cachePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	// Flip the path on the persisted copy to corrupt the payload
	// without touching the checksum field.
	payload.Files = []*FileInfo{{RelPath: "TAMPERED.go", Language: LangGo}}
	out, _ := json.Marshal(&payload)
	if err := os.WriteFile(filepath.Join(dir, cacheFileInfosFile), out, 0o644); err != nil {
		t.Fatal(err)
	}

	if got := LoadFileInfos(dir); got != nil {
		t.Errorf("expected nil on checksum mismatch, got %+v", got)
	}
}

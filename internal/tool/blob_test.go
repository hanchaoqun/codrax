package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSetBlobLimits is the contract test for the runtime override
// hook used by main.go after loading codrax.yaml. The function must:
//
//   - Apply positive overrides verbatim.
//   - Treat zero or negative values as "leave the current value
//     alone", so callers can override one knob without touching the
//     others (and so the cumulative pattern of "merge nil → 0 →
//     SetBlobLimits" produces the right thing).
//   - Compose: a second SetBlobLimits call with mixed positive /
//     zero values updates only the positive ones.
//
// The test restores the historical defaults at the end so it does
// not pollute other tests in this package that may rely on them.
func TestSetBlobLimits(t *testing.T) {
	origMax := MaxInlineBytes
	origHead := previewHeadBytes
	origTail := previewTailBytes
	t.Cleanup(func() {
		MaxInlineBytes = origMax
		previewHeadBytes = origHead
		previewTailBytes = origTail
	})

	// All three positive → all three updated.
	SetBlobLimits(64*1024, 48*1024, 16*1024)
	if MaxInlineBytes != 64*1024 {
		t.Errorf("MaxInlineBytes = %d, want %d", MaxInlineBytes, 64*1024)
	}
	if previewHeadBytes != 48*1024 {
		t.Errorf("previewHeadBytes = %d, want %d", previewHeadBytes, 48*1024)
	}
	if previewTailBytes != 16*1024 {
		t.Errorf("previewTailBytes = %d, want %d", previewTailBytes, 16*1024)
	}

	// Two zeros → only the non-zero one moves.
	SetBlobLimits(0, 32*1024, 0)
	if MaxInlineBytes != 64*1024 {
		t.Errorf("MaxInlineBytes after partial update = %d, want %d", MaxInlineBytes, 64*1024)
	}
	if previewHeadBytes != 32*1024 {
		t.Errorf("previewHeadBytes after partial update = %d, want %d", previewHeadBytes, 32*1024)
	}
	if previewTailBytes != 16*1024 {
		t.Errorf("previewTailBytes after partial update = %d, want %d", previewTailBytes, 16*1024)
	}

	// Negatives are also treated as "leave alone" — defensive,
	// because main.go funnels in YAML int values without bounds.
	SetBlobLimits(-1, -1, -1)
	if MaxInlineBytes != 64*1024 || previewHeadBytes != 32*1024 || previewTailBytes != 16*1024 {
		t.Errorf("negative values should be no-op; got max=%d head=%d tail=%d",
			MaxInlineBytes, previewHeadBytes, previewTailBytes)
	}
}

func TestStoreBlobArtifactAlwaysWritesStructuredRef(t *testing.T) {
	dir := t.TempDir()
	ref := StoreBlobArtifact(dir, "answer_observation_row_set", "aggregate-000-current_source-row-set.jsonl", "one\n")
	if ref == "" {
		t.Fatal("expected artifact ref")
	}
	if filepath.Dir(ref) != dir {
		t.Fatalf("artifact should be stored in work dir, got %s", ref)
	}
	if !strings.HasSuffix(ref, ".jsonl") {
		t.Fatalf("artifact should preserve structured extension, got %s", ref)
	}
	body, err := os.ReadFile(ref)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if string(body) != "one\n" {
		t.Fatalf("artifact body = %q", string(body))
	}
}

func TestStoreBlobArtifactFromFileStreamsWithPrefix(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "raw.tmp")
	if err := os.WriteFile(src, []byte("late raw line\n"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	ref := StoreBlobArtifactFromFile(dir, "grep", "grep-full.txt", src, "header\n")
	if ref == "" {
		t.Fatal("expected artifact ref")
	}
	if filepath.Dir(ref) != dir {
		t.Fatalf("artifact should be stored in work dir, got %s", ref)
	}
	if !strings.HasSuffix(ref, ".txt") {
		t.Fatalf("artifact should preserve txt extension, got %s", ref)
	}
	body, err := os.ReadFile(ref)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if string(body) != "header\nlate raw line\n" {
		t.Fatalf("artifact body = %q", string(body))
	}
}

// Package types — evidence_closure_path_canon_test.go (2026-05-10).
//
// Audit-driven tests pinning the path canonicalization invariants
// across EvidenceClosure surfaces. Without these, three classes of
// silent bugs slip through:
//
//   1. SetScannedSet stored raw paths but IsScanned looked up raw —
//      consistent with itself but inconsistent with ReadSet's
//      canonical-only contract; cross-surface comparisons in
//      runForcedReads silently misclassify.
//   2. AddPendingRead's dedup compared raw existing.File against
//      raw p.File; two callers raising "./foo.go" and "foo.go"
//      bypass dedup and produce duplicate forced-read demands.
//   3. ClearPendingReadFor compared raw input against stored raw
//      p.File; runForcedReads's post-success clear with a
//      different prefix form leaves the entry in pendingReads,
//      causing the same forced-read directive to re-fire.
package types

import (
	"testing"
)

// === IsScanned canonicalization ===

func TestIsScanned_CanonicalisesLookup(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetScannedSet(map[string]bool{
		"internal/foo.go": true,
		"internal/bar.go": true,
	})
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"canonical", "internal/foo.go", true},
		{"with-dot-slash", "./internal/foo.go", true},
		{"backslash", `internal\foo.go`, true},
		{"unrelated", "internal/baz.go", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.IsScanned(tc.input)
			if got != tc.want {
				t.Errorf("IsScanned(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestSetScannedSet_CanonicalisesAtInsert(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetScannedSet(map[string]bool{
		"./internal/foo.go": true, // ./ prefix
		`internal\bar.go`:   true, // Windows-style
	})
	// Both should be lookupable via canonical form.
	if !c.IsScanned("internal/foo.go") {
		t.Error("./prefix path stored raw should canonicalise on read")
	}
	if !c.IsScanned("internal/bar.go") {
		t.Error("Windows-style path stored raw should canonicalise on read")
	}
}

func TestIsScanned_EmptySet_FallsThroughToTrue(t *testing.T) {
	// Legacy semantic: empty ScannedSet means "pre-scan never ran"
	// → every query returns true so historical behaviour is
	// preserved. Canonicalization must not break this.
	c := NewEvidenceClosure("")
	if !c.IsScanned("any/path.go") {
		t.Error("empty scannedSet should fall through to true (historical contract)")
	}
}

// === AddPendingRead dedup canonicalization ===

func TestAddPendingRead_DedupAcrossPathForms(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{File: "./internal/foo.go", Origin: "test"})
	c.AddPendingRead(PendingRead{File: "internal/foo.go", Origin: "test"})
	c.AddPendingRead(PendingRead{File: `internal\foo.go`, Origin: "test"})
	pending := c.PendingReads()
	if len(pending) != 1 {
		t.Fatalf("3 path-form variants of same file + same Origin should dedup to 1; got %d (%+v)", len(pending), pending)
	}
}

func TestAddPendingRead_StoresCanonicalForm(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{File: "./internal/foo.go", Origin: "test"})
	pending := c.PendingReads()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending read; got %d", len(pending))
	}
	if pending[0].File != "internal/foo.go" {
		t.Errorf("stored File should be canonical; got %q", pending[0].File)
	}
}

func TestAddPendingRead_SeparateOriginsDoNotDedup(t *testing.T) {
	// Even when path is canonical, different Origin is a different
	// dedup key.
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{File: "internal/foo.go", Origin: "chain"})
	c.AddPendingRead(PendingRead{File: "internal/foo.go", Origin: "phase1"})
	pending := c.PendingReads()
	if len(pending) != 2 {
		t.Fatalf("different Origin should not dedup; got %d", len(pending))
	}
}

// === ClearPendingReadFor canonicalization ===

func TestClearPendingReadFor_CanonicalisesInput(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{File: "internal/foo.go", Origin: "test"})
	c.AddPendingRead(PendingRead{File: "internal/bar.go", Origin: "test"})
	// Clear with a non-canonical form — must still match the
	// canonical entry.
	c.ClearPendingReadFor("./internal/foo.go")
	pending := c.PendingReads()
	if len(pending) != 1 {
		t.Fatalf("./prefix Clear should match canonical entry; got %d remaining", len(pending))
	}
	if pending[0].File != "internal/bar.go" {
		t.Errorf("wrong entry remained; got %q", pending[0].File)
	}
}

func TestClearPendingReadFor_WindowsStyle(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{File: "internal/foo.go", Origin: "test"})
	c.ClearPendingReadFor(`internal\foo.go`)
	pending := c.PendingReads()
	if len(pending) != 0 {
		t.Errorf("Windows-style Clear should match canonical entry; got %d remaining", len(pending))
	}
}

func TestClearPendingReadFor_EmptyInput_NoOp(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{File: "foo.go", Origin: "test"})
	c.ClearPendingReadFor("")
	if len(c.PendingReads()) != 1 {
		t.Error("empty input should be a no-op")
	}
}

// === Cross-surface integration: ReadSet ∪ PendingReads ∪ ScannedSet ===

func TestCrossSurface_ReadSetAndPendingReadsAlignCanonical(t *testing.T) {
	// Mimic the runForcedReads bug: ReadSet stored canonical,
	// PendingRead stored canonical, lookup must align.
	c := NewEvidenceClosure("")
	c.SetReadSet(map[string]bool{"internal/foo.go": true})
	c.AddPendingRead(PendingRead{File: "./internal/foo.go", Origin: "test"})
	// Pending entry's stored File should be canonical so it
	// matches the readSet.
	pending := c.PendingReads()
	if len(pending) != 1 {
		t.Fatalf("got %d pending; want 1", len(pending))
	}
	if !c.HasRead(pending[0].File) {
		t.Error("pending[0].File should be readable via HasRead — both surfaces canonical")
	}
}

func TestCrossSurface_ScannedSetAndPendingReadsAlignCanonical(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetScannedSet(map[string]bool{"internal/foo.go": true})
	c.AddPendingRead(PendingRead{File: `internal\foo.go`, Origin: "test"})
	pending := c.PendingReads()
	if len(pending) != 1 {
		t.Fatalf("got %d pending; want 1", len(pending))
	}
	// IsScanned with the canonical pending File should hit.
	if !c.IsScanned(pending[0].File) {
		t.Error("pending[0].File should be IsScanned — both surfaces canonical")
	}
}

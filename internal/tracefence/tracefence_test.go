package tracefence

import (
	"strings"
	"testing"
)

// The opener composition is load-bearing on three faces: goldmark/glamour
// read the FIRST word (must stay "text" — terminal/markdown byte parity),
// the preview hard gate reads the SECOND (exact match). Cross-package
// behavior pins live in internal/tool (census) and internal/render (glamour
// parity corpus); this file pins the composition itself.
func TestOpenerComposition(t *testing.T) {
	if Opener != "```text trace-causal-projection" {
		t.Fatalf("Opener drifted: %q", Opener)
	}
	if !strings.HasPrefix(Info, "text ") {
		t.Fatalf("first info token must stay \"text\" (terminal/markdown parity): %q", Info)
	}
	if strings.ContainsAny(InfoToken, " \t\n") || InfoToken == "" {
		t.Fatalf("InfoToken must be a single field: %q", InfoToken)
	}
}

func TestClosedSetsShape(t *testing.T) {
	heads := FlatFallbackHeads()
	if len(heads) != 6 {
		t.Fatalf("⊘ banner closed set must stay 2 typed causes + 1 opaque fallback x zh/en = 6, got %d", len(heads))
	}
	for _, head := range heads {
		if !strings.HasPrefix(head, "⊘ ") {
			t.Fatalf("banner head lost its ⊘ mark: %q", head)
		}
	}
	if len(TargetProvenanceChips()) != 4 {
		t.Fatalf("⊚ provenance chip closed set must stay 4")
	}
	// DISPHYG-3 件3 (2026-07-20). EVOLUTION RECORD: the honest no-ruler head
	// (a windowless board without a wall-clock scale anchor) joins the closed
	// set as its own zh/en marker pair — 2 → 4; the v5 P0 备-2 derivation
	// invariant (generator head set ≡ preview archive fallback) is exactly
	// why the new head registers here instead of bypassing the set.
	if len(ScaleNoteMarkers()) != 4 {
		t.Fatalf("scale marker closed set must stay 4 (满格=/bar full = + no-ruler, zh/en)")
	}
}

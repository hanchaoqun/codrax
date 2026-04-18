package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestFormatUnverifiedFindings_Empty verifies the renderer returns
// an empty string when there are no findings, so the caller can
// suppress the entire prompt section (no empty "## Unverified
// Analyzer Findings\n\n" noise on clean runs).
func TestFormatUnverifiedFindings_Empty(t *testing.T) {
	if got := formatUnverifiedFindings(nil); got != "" {
		t.Errorf("nil input must return empty string, got %q", got)
	}
	if got := formatUnverifiedFindings([]types.UnverifiedFinding{}); got != "" {
		t.Errorf("empty slice must return empty string, got %q", got)
	}
}

// TestFormatUnverifiedFindings_RendersBothKinds covers the C1 rendering
// path for the two UnverifiedFinding.Kind values the analyzer can
// produce ("path" and "symbol"). Both should surface with their Reason.
func TestFormatUnverifiedFindings_RendersBothKinds(t *testing.T) {
	out := formatUnverifiedFindings([]types.UnverifiedFinding{
		{Token: "internal/agent/ghost.go", Kind: "path", Reason: "file does not exist in repo"},
		{Token: "NonexistentHandler", Kind: "symbol", Reason: "symbol not found in graph"},
	})
	if out == "" {
		t.Fatal("expected non-empty render")
	}
	for _, want := range []string{
		"UNRELIABLE",
		"internal/agent/ghost.go",
		"file does not exist in repo",
		"NonexistentHandler",
		"symbol not found in graph",
		"`path`",
		"`symbol`",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered block missing %q, got:\n%s", want, out)
		}
	}
}

// TestFormatUnverifiedFindings_DedupesByTokenAndKind asserts C4: the
// render helper drops duplicate Token+Kind entries so the prompt
// block stays clean even if the analyzer path flags the same item
// twice.
func TestFormatUnverifiedFindings_DedupesByTokenAndKind(t *testing.T) {
	out := formatUnverifiedFindings([]types.UnverifiedFinding{
		{Token: "Foo", Kind: "symbol", Reason: "missing"},
		{Token: "Foo", Kind: "symbol", Reason: "missing (duplicate)"},
		{Token: "Foo", Kind: "path", Reason: "different kind kept"},
	})
	// "Foo" should appear twice at most (once for symbol, once for path).
	// Using a rough count by scanning for "`symbol` Foo" etc.
	if strings.Count(out, "`symbol` Foo") != 1 {
		t.Errorf("expected exactly one `symbol` Foo line, got:\n%s", out)
	}
	if strings.Count(out, "`path` Foo") != 1 {
		t.Errorf("expected exactly one `path` Foo line, got:\n%s", out)
	}
}

// TestFormatUnverifiedFindings_CapRenders_TrailingCount covers C4's
// render cap. When the slice exceeds unverifiedFindingsRenderCap,
// the block stops at the cap and prints "... and N more" so the
// operator knows truncation happened without the prompt ballooning.
func TestFormatUnverifiedFindings_CapRenders_TrailingCount(t *testing.T) {
	// Build 20 distinct findings, exceeding the cap (12).
	var finds []types.UnverifiedFinding
	for i := 0; i < 20; i++ {
		finds = append(finds, types.UnverifiedFinding{
			Token:  "Sym" + string(rune('A'+i)),
			Kind:   "symbol",
			Reason: "not found",
		})
	}
	out := formatUnverifiedFindings(finds)
	if !strings.Contains(out, "... and 8 more") {
		t.Errorf("expected '... and 8 more' trailing count, got:\n%s", out)
	}
	// Count rendered bullet lines — must be exactly the cap (12).
	bullets := strings.Count(out, "  - `symbol`")
	if bullets != unverifiedFindingsRenderCap {
		t.Errorf("bullet count=%d, want %d", bullets, unverifiedFindingsRenderCap)
	}
}

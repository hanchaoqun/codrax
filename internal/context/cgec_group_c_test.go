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

// TestFormatSubjectMatchSummary_EmptyMap_NoRender asserts E4+E5
// returns "" when there is no chain match data so the caller
// suppresses the entire Subject Match Summary section on
// analyzer-only dispatches / SubjectUnknown questions.
func TestFormatSubjectMatchSummary_EmptyMap_NoRender(t *testing.T) {
	expected := types.AnswerSubject{Kind: types.SubjectFunctionName, Confidence: 0.8}
	if got := formatSubjectMatchSummary(nil, expected); got != "" {
		t.Errorf("nil matches must return empty, got %q", got)
	}
	if got := formatSubjectMatchSummary(map[string]float64{}, expected); got != "" {
		t.Errorf("empty map must return empty, got %q", got)
	}
}

// TestFormatSubjectMatchSummary_UnknownSubject_NoRender asserts the
// helper skips rendering when AnswerSubject.Kind is SubjectUnknown
// (ranker's match=0 on all chains is noise, not signal).
func TestFormatSubjectMatchSummary_UnknownSubject_NoRender(t *testing.T) {
	matches := map[string]float64{"chain A": 0.9}
	got := formatSubjectMatchSummary(matches, types.AnswerSubject{Kind: types.SubjectUnknown})
	if got != "" {
		t.Errorf("SubjectUnknown must return empty, got %q", got)
	}
}

// TestFormatSubjectMatchSummary_TopKSorted covers the primary
// E4+E5 render path: chains sorted by score descending, capped
// at subjectMatchRenderCap (5), preceded by the expected subject
// label, followed by the "prefer top-scored" directive.
func TestFormatSubjectMatchSummary_TopKSorted(t *testing.T) {
	matches := map[string]float64{
		"chain A": 0.9,
		"chain B": 0.3,
		"chain C": 0.7,
		"chain D": 0.5,
		"chain E": 0.25,
		"chain F": 0.8,
		"chain G": 0.1, // below floor — must NOT render
	}
	expected := types.AnswerSubject{Kind: types.SubjectFunctionName, Confidence: 0.8}
	out := formatSubjectMatchSummary(matches, expected)
	if out == "" {
		t.Fatal("expected non-empty render")
	}
	if !strings.Contains(out, "Expected answer subject: `function_name`") {
		t.Errorf("missing expected-subject header, got:\n%s", out)
	}
	if !strings.Contains(out, "Chain candidates") {
		t.Errorf("missing Chain candidates section, got:\n%s", out)
	}
	// Order: A (0.9) > F (0.8) > C (0.7) > D (0.5) > B (0.3).
	// Chain E (0.25) and G (0.1) must not appear.
	aIdx := strings.Index(out, "chain A")
	fIdx := strings.Index(out, "chain F")
	cIdx := strings.Index(out, "chain C")
	if !(aIdx < fIdx && fIdx < cIdx) {
		t.Errorf("chains not sorted by descending score, got:\n%s", out)
	}
	if strings.Contains(out, "chain G") {
		t.Error("chain G (below floor) must NOT appear")
	}
	if !strings.Contains(out, "chain B") {
		t.Error("chain B (above floor, within cap) must appear")
	}
	if !strings.Contains(out, "Prefer the top-scored") {
		t.Error("missing closing directive")
	}
}

// TestFormatSubjectMatchSummary_AllBelowFloor_Warns asserts that
// when every chain scored below the floor, the helper swaps the
// candidate list for a SKEPTICISM warning — telling the LLM the
// chain producer is likely off-target.
func TestFormatSubjectMatchSummary_AllBelowFloor_Warns(t *testing.T) {
	matches := map[string]float64{
		"chain A": 0.1,
		"chain B": 0.15,
	}
	expected := types.AnswerSubject{Kind: types.SubjectFunctionName, Confidence: 0.8}
	out := formatSubjectMatchSummary(matches, expected)
	if !strings.Contains(out, "SKEPTICISM") {
		t.Errorf("expected SKEPTICISM warning, got:\n%s", out)
	}
	if strings.Contains(out, "Prefer the top-scored") {
		t.Error("closing 'Prefer top-scored' directive must not appear when nothing cleared floor")
	}
}

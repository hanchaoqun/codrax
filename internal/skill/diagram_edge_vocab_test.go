package skill

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// G3 step 3 (post_v2_runtime_gap_remediation, 2026-05-04). SST locks
// for the LLM-facing diagram-edge prompt helpers.

// BuildDiagramRelationKindList renders the typed enum verbatim.
// Lock the contract: every value of types.AllDiagramRelationKinds()
// MUST appear (backtick-quoted) in the rendered string. Adding a new
// relation kind to the enum surfaces here automatically; renaming
// breaks this test deliberately so the prompt cannot drift.
func TestBuildDiagramRelationKindList_SST(t *testing.T) {
	got := BuildDiagramRelationKindList()
	for _, k := range types.AllDiagramRelationKinds() {
		quoted := "`" + string(k) + "`"
		if !strings.Contains(got, quoted) {
			t.Errorf("BuildDiagramRelationKindList missing relation kind %q (output=%q)", quoted, got)
		}
	}
	// No DiagramRelUnknown sentinel — it's intentionally NOT a member
	// of the typed set (see types.allDiagramRelationKinds doc).
	if strings.Contains(got, "``") {
		t.Errorf("rendered list contains empty-string entry: %q", got)
	}
	// Determinism: alphabetical sort. The rendered list MUST be sorted
	// so prompt cache stays warm across builds.
	parts := strings.Split(got, " / ")
	for i := 1; i < len(parts); i++ {
		if parts[i-1] > parts[i] {
			t.Errorf("rendered list not sorted: pos %d (%q) > pos %d (%q)", i-1, parts[i-1], i, parts[i])
		}
	}
}

// G3 step 3: Vocabulary doc renders entries from
// types.DiagramRelationKeywords; lock that adding a keyword to the
// dictionary surfaces in the rendered prompt.
func TestBuildDiagramEdgeLabelVocabularyDoc_SST(t *testing.T) {
	got := BuildDiagramEdgeLabelVocabularyDoc()
	for _, e := range types.DiagramRelationKeywords() {
		// Each keyword appears backtick-quoted in the vocabulary line.
		if !strings.Contains(got, "`"+e.Keyword+"`") {
			t.Errorf("vocabulary doc missing keyword %q (kind=%s)", e.Keyword, e.Kind)
		}
	}
}

// R4 lock — the new SST helper output MUST NOT contain stage codenames
// or Go type names. The rendered string is embedded verbatim in
// skill/defaults.go, which is grepped by TestNoInternalTermsInPrompts;
// this lock is the focused first-line defence.
func TestBuildDiagramRelationKindList_NoInternalJargon(t *testing.T) {
	got := BuildDiagramRelationKindList()
	bannedTokens := []string{
		"DiagramRelationKind", "ClaimForm", "Phase ", "Layer ",
		"finalizer", "extractor", "explorer", "analyzer",
	}
	for _, banned := range bannedTokens {
		if strings.Contains(got, banned) {
			t.Errorf("BuildDiagramRelationKindList contains banned token %q (full=%q)", banned, got)
		}
	}
}

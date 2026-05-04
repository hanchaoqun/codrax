package orchestrator

import (
	"strings"
	"testing"
)

// llm_facing_jargon_audit_test.go — Phase 1-D (V2 runtime eval
// followup, 2026-05-04). Extends the R4 "no internal jargon in
// LLM prompts" red-line check beyond skill/.
//
// Existing coverage: internal/skill/glossary_test.go::
// TestNoInternalTermsInPrompts walks every Config in the skill
// registry. That covers the static prompt corpora.
//
// This test extends to orchestrator-side LLM-facing strings that
// the skill scanner cannot reach:
//   - SelfConsistencyReviewer system prompt (long const)
//   - SelfConsistencyReviewer tool Description (separate string)
//
// Add new entries here as new LLM-facing string sources are
// introduced in this package.

// localInternalTerms mirrors a subset of skill.InternalTermsBlocklist
// — duplicated so the orchestrator package does NOT acquire a
// dependency on internal/skill (would create import cycles for
// some test-only callers).
//
// Mirror discipline: if skill.InternalTermsBlocklist gains an
// entry that materially changes the audit shape, add the same
// entry here. The audit covers the categories most likely to
// leak into orchestrator-owned LLM strings.
var localInternalTerms = []string{
	"finalizer",
	"BusContext",
	"MutableState",
	"FacetCoverageContract",
	"AnswerSemanticView",
	"TaskGraph",
	"EvidenceClosure",
	"Tier 1",
	"Tier 2",
	"Layer 1",
	"Layer 2",
	"Phase 1",
	"Phase 2",
	"Phase 3",
	"Phase 4",
	"Phase 5",
	"Tier-1",
	"Tier-2",
}

// TestSelfConsistencyReviewer_LLMFacingNoInternalJargon enforces
// R4 for the reviewer's two LLM-facing string surfaces.
func TestSelfConsistencyReviewer_LLMFacingNoInternalJargon(t *testing.T) {
	surfaces := map[string]string{
		"selfConsistencyReviewerSystemPrompt": selfConsistencyReviewerSystemPrompt,
		"selfConsistencyTool.Description":     selfConsistencyTool.Description,
	}
	for label, text := range surfaces {
		for _, term := range localInternalTerms {
			if strings.Contains(text, term) {
				preview := text
				if i := strings.Index(text, term); i >= 0 {
					start := i - 30
					if start < 0 {
						start = 0
					}
					end := i + len(term) + 30
					if end > len(text) {
						end = len(text)
					}
					preview = "…" + text[start:end] + "…"
				}
				t.Errorf("%s leaks internal term %q: %s", label, term, preview)
			}
		}
	}
}

package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestStripSystemHedgeArtifactsFromDoc_StripsCaveatsToo: round-6
// extension — LLM may copy `[hedged]` etc. tokens from prior
// conversation into doc.Caveats[]. Without stripping, those
// LLM-written caveats render as user-facing caveats containing
// what looks like a system marker, polluting the user view and
// confusing dedup. The strip preserves LLM caveat prose but removes
// any inline marker tokens.
func TestStripSystemHedgeArtifactsFromDoc_StripsCaveatsToo(t *testing.T) {
	doc := &types.AnswerDocument{
		Shape: types.ShapeExplanation,
		Caveats: []string{
			render.HedgeMarkerHistorical + " evidence is partial.",
			"emit_answer_document was retried twice.",
		},
	}
	stripSystemHedgeArtifactsFromDoc(doc)
	for _, c := range doc.Caveats {
		if strings.Contains(c, render.HedgeMarkerHistorical) ||
			strings.Contains(c, render.HedgeMarkerConditional) ||
			strings.Contains(c, render.HedgeMarkerIllustrative) {
			t.Errorf("Caveat retained marker after strip: %q", c)
		}
	}
	// Both LLM caveats must still be present (just without markers).
	if len(doc.Caveats) != 2 {
		t.Errorf("Caveats count changed: %d (want 2)", len(doc.Caveats))
	}
	gotPartial := false
	gotRetry := false
	for _, c := range doc.Caveats {
		if strings.Contains(c, "evidence is partial") {
			gotPartial = true
		}
		if strings.Contains(c, "retried twice") {
			gotRetry = true
		}
	}
	if !gotPartial || !gotRetry {
		t.Errorf("LLM caveat content lost; doc.Caveats=%v", doc.Caveats)
	}
}

// TestStripSystemHedgeArtifactsFromDoc_DropsEmptyCaveats: when a
// caveat IS the system caveat (whole line is the tagged caveat
// rendered as italic), StripAuthorityArtifacts drops it whole and
// the trim leaves an empty string. Such empty caveats must be
// removed from the slice (they would otherwise render as `**` —
// empty italic span).
func TestStripSystemHedgeArtifactsFromDoc_DropsEmptyCaveats(t *testing.T) {
	systemCaveat := render.AuthorityCaveatPrefix + render.AuthorityCaveatTag() +
		"1 historical observation."
	doc := &types.AnswerDocument{
		Shape: types.ShapeExplanation,
		Caveats: []string{
			systemCaveat,
			"a real LLM caveat",
		},
	}
	stripSystemHedgeArtifactsFromDoc(doc)
	if len(doc.Caveats) != 1 {
		t.Errorf("expected 1 caveat after dropping system caveat; got %d: %v",
			len(doc.Caveats), doc.Caveats)
	}
	if len(doc.Caveats) == 1 && doc.Caveats[0] != "a real LLM caveat" {
		t.Errorf("survived caveat = %q; want LLM caveat", doc.Caveats[0])
	}
}

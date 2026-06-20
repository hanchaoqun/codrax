package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestExtractorValueEvidenceScore_SalienceLockedSurvivesZeroNeedle(t *testing.T) {
	item := types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Subject:      "HiddenFact",
		Source:       "internal/hidden.go",
		LineStart:    12,
		AnchorKind:   types.AnchorAssignment,
		AnchorSymbol: "HiddenFact",
		Salience:     types.SalienceLoadBearing,
	}
	needles := map[string]bool{"totally_unrelated": true}
	if got := extractorValueEvidenceScore(item, "HiddenFact", "HiddenFact assigns value", needles, extractorValueRankGeneric); got < 6 {
		t.Fatalf("locked salience should create a score floor, got %d", got)
	}
	item.Salience = types.SalienceUnset
	if got := extractorValueEvidenceScore(item, "HiddenFact", "HiddenFact assigns value", needles, extractorValueRankGeneric); got != 0 {
		t.Fatalf("unset salience must preserve zero-needle drop, got %d", got)
	}
}

func TestRenderExtractorValueEvidenceFacts_LoadBearingNotDroppedWhenOthersScore(t *testing.T) {
	ctx := &types.AgentContext{Objective: "needle"}
	ta := &types.TurnAArtifacts{
		UserQuestion: "needle",
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:         types.EvidenceDirect,
				Subject:      "Relevant",
				Object:       "needle",
				Source:       "internal/relevant.go",
				LineStart:    10,
				AnchorKind:   types.AnchorAssignment,
				AnchorSymbol: "Relevant",
				Snippet:      "Relevant = \"needle\"",
			},
			{
				Kind:         types.EvidenceDirect,
				Subject:      "HiddenFact",
				Source:       "internal/hidden.go",
				LineStart:    12,
				AnchorKind:   types.AnchorAssignment,
				AnchorSymbol: "HiddenFact",
				Snippet:      "HiddenFact = \"opaque\"",
				Salience:     types.SalienceLoadBearing,
			},
		},
	}
	rendered := renderExtractorValueEvidenceFacts(ctx, ta)
	if !strings.Contains(rendered, "Relevant") {
		t.Fatalf("expected relevant row, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "HiddenFact") {
		t.Fatalf("locked load-bearing row should survive score filter, got:\n%s", rendered)
	}
}

func TestExtractorValueEvidenceDisplayLimit_WidensForLockedSalience(t *testing.T) {
	items := make([]types.EvidenceItem, 0, 30)
	for i := 0; i < 30; i++ {
		items = append(items, types.EvidenceItem{
			Kind:         types.EvidenceDirect,
			Subject:      "Item",
			Source:       "internal/items.go",
			LineStart:    10 + i,
			AnchorKind:   types.AnchorAssignment,
			AnchorSymbol: "Item",
			Salience:     types.SalienceExhaustListed,
		})
	}
	limit := extractorValueEvidenceDisplayLimit(nil, &types.TurnAArtifacts{EvidenceItems: items}, extractorValueRankGeneric)
	if limit != extractorMaxValueFacts {
		t.Fatalf("limit = %d, want max cap %d for 30 locked rows + reserve", limit, extractorMaxValueFacts)
	}

	for i := range items {
		items[i].Salience = types.SalienceUnset
	}
	limit = extractorValueEvidenceDisplayLimit(nil, &types.TurnAArtifacts{EvidenceItems: items}, extractorValueRankGeneric)
	if limit != extractorDefaultValueFacts {
		t.Fatalf("unset salience must keep default limit, got %d", limit)
	}
}

// Gap 3 handoff fidelity: principal (typed load-bearing / exhaustively-listed)
// evidence must stably precede incidental rows so the Turn-A→Turn-B top-N cap
// cannot crowd answer-bearing evidence out. Order within each band is preserved.
func TestExtractorPrioritizePrincipalEvidence_StableBandedOrder(t *testing.T) {
	items := []types.EvidenceItem{
		{ID: "ctx1", Salience: types.SalienceContext},
		{ID: "lb1", Salience: types.SalienceLoadBearing},
		{ID: "sup1", Salience: types.SalienceSupporting},
		{ID: "ex1", Salience: types.SalienceExhaustListed},
		{ID: "lb2", Salience: types.SalienceLoadBearing},
	}
	got := extractorPrioritizePrincipalEvidence(items)
	var ids []string
	for _, it := range got {
		ids = append(ids, it.ID)
	}
	// Principal rows first in original relative order (lb1, ex1, lb2), then the
	// rest in original relative order (ctx1, sup1).
	want := []string{"lb1", "ex1", "lb2", "ctx1", "sup1"}
	if len(ids) != len(want) {
		t.Fatalf("got %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("order = %v, want %v (principal stably first)", ids, want)
		}
	}
	// Must not mutate the caller's slice.
	if items[0].ID != "ctx1" {
		t.Fatalf("input slice was mutated: %s", items[0].ID)
	}
}

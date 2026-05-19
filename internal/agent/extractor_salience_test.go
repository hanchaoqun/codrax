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

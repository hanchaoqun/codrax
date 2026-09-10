package orchestrator

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1642bPostEndpointCandidateRequiresOneExactRelation(t *testing.T) {
	left := types.EvidenceItem{ID: "left", Kind: types.EvidenceRelationship, Source: "flow.go", LineStart: 3, LineEnd: 3,
		AnchorKind: types.AnchorCall, AnchorSymbol: "Left", Subject: "Dispatch", Object: "Left",
		GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo}
	right := left
	right.ID, right.AnchorSymbol, right.Object = "right", "Right", "Right"
	for _, tc := range []struct {
		name  string
		pool  []types.EvidenceItem
		label string
		refs  []int
		want  int
	}{
		{"same_line_left_first", []types.EvidenceItem{left, right}, "Dispatch", []int{0}, 0},
		{"same_line_right_first", []types.EvidenceItem{right, left}, "Dispatch", []int{0}, 0},
		{"unique_wrong_reference", []types.EvidenceItem{left}, "Dispatch", []int{0}, 1},
		{"unique_selected_reference", []types.EvidenceItem{left}, "Dispatch", []int{1}, 0},
		{"selected_additional_reference", []types.EvidenceItem{left}, "Dispatch", []int{0, 1}, 0},
		{"explicit_edge_still_checked", []types.EvidenceItem{left, right}, "Dispatch -> Left", []int{0}, 1},
		{"explicit_edge_selected_reference", []types.EvidenceItem{left, right}, "Dispatch -> Left", []int{1}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mut := types.NewMutableState("endpoint role")
			mut.AppendEvidence(tc.pool)
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "roles", Kind: types.BlockTable,
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
				Items:     []types.AnswerBlockItem{{ID: "owner", Label: tc.label, Text: "Model-owned description.", CitationRefs: tc.refs}}}},
				Citations: []types.Citation{{File: "flow.go", Line: 2}, {File: "flow.go", Line: 3}}}
			before, _ := json.Marshal(doc)
			got := validateCallChainItemCitationRoleAlignment(doc, nil, mut)
			if len(got) != tc.want {
				t.Errorf("role checks=%+v want %d", got, tc.want)
			}
			after, _ := json.Marshal(doc)
			if string(before) != string(after) {
				t.Fatal("post check changed model text, citation selection or role metadata")
			}
		})
	}
}

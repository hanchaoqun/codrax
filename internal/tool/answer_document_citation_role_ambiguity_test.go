package tool

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1642PreEmitCitationRoleCandidateMatrix(t *testing.T) {
	ev := func(id, source string, line int, anchor types.AnchorKind, label string) types.EvidenceItem {
		return types.EvidenceItem{ID: id, Source: source, LineStart: line, Kind: types.EvidenceDirect, AnchorKind: anchor, AnchorSymbol: label, Subject: label,
			GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo}
	}
	lookup := ev("lookup", "Config.java", 21, types.AnchorStringLiteral, "config.key")
	value := ev("value", "application.properties", 1, types.AnchorPrecedence, "config.key")
	wrong := ev("definition", "Config.java", 13, types.AnchorDefinition, "Container")
	call := ev("call", "flow.go", 7, types.AnchorCall, "A")
	call.Object = "B"
	reverseCall := ev("reverse", "flow.go", 8, types.AnchorCall, "B")
	reverseCall.Object = "A"
	for _, tc := range []struct {
		name        string
		pool        []types.EvidenceItem
		forms       []types.ClaimForm
		label, text string
		citations   []types.Citation
		refs        []int
		legacy      bool
		want        int
	}{
		{"same_label_lookup_first", []types.EvidenceItem{lookup, value}, []types.ClaimForm{types.ClaimLiteralValueFact, types.ClaimPrecedenceRole}, "config.key", "50", []types.Citation{{File: value.Source, Line: 1}}, nil, false, 0},
		{"same_label_value_first", []types.EvidenceItem{value, lookup}, []types.ClaimForm{types.ClaimLiteralValueFact, types.ClaimPrecedenceRole}, "config.key", "50", []types.Citation{{File: value.Source, Line: 1}}, nil, false, 0},
		{"ambiguous_wrong_not_first_winner", []types.EvidenceItem{lookup, value, wrong}, []types.ClaimForm{types.ClaimLiteralValueFact, types.ClaimPrecedenceRole}, "config.key", "", []types.Citation{{File: wrong.Source, Line: 13}}, nil, false, 0},
		{"unique_wrong_keeps_warning", []types.EvidenceItem{lookup, wrong}, []types.ClaimForm{types.ClaimLiteralValueFact}, "config.key", "", []types.Citation{{File: wrong.Source, Line: 13}}, nil, false, 1},
		{"second_explicit_ref_is_legal", []types.EvidenceItem{lookup, wrong}, []types.ClaimForm{types.ClaimLiteralValueFact}, "config.key", "", []types.Citation{{File: wrong.Source, Line: 13}, {File: lookup.Source, Line: 21}}, []int{0, 1}, false, 0},
		{"multiple_wrong_refs_keep_warning", []types.EvidenceItem{lookup, wrong}, []types.ClaimForm{types.ClaimLiteralValueFact}, "config.key", "", []types.Citation{{File: wrong.Source, Line: 13}, {File: "unrelated.go", Line: 20}}, []int{0, 1}, false, 1},
		{"edge_exact_current", []types.EvidenceItem{call, wrong}, []types.ClaimForm{types.ClaimCallEdge}, "A -> B", "", []types.Citation{{File: call.Source, Line: 7}}, nil, false, 0},
		{"edge_reverse_citation_not_enough", []types.EvidenceItem{call, reverseCall}, []types.ClaimForm{types.ClaimCallEdge}, "A -> B", "", []types.Citation{{File: reverseCall.Source, Line: 8}}, nil, false, 1},
		{"legacy_exact_endpoint_wrong", []types.EvidenceItem{call, wrong}, []types.ClaimForm{types.ClaimCallEdge}, "A", "", []types.Citation{{File: wrong.Source, Line: 13}}, nil, true, 1},
		{"legacy_exact_endpoint_supported", []types.EvidenceItem{call, wrong}, []types.ClaimForm{types.ClaimCallEdge}, "A", "", []types.Citation{{File: call.Source, Line: 7}}, nil, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mut := types.NewMutableState("citation roles")
			mut.AppendEvidence(tc.pool)
			block := types.AnswerBlock{ID: "facts", Kind: types.BlockTable, Items: []types.AnswerBlockItem{{ID: "row", Label: tc.label, Text: tc.text, CitationRef: 0, CitationRefs: tc.refs}}}
			if !tc.legacy {
				for _, form := range tc.forms {
					block.ClaimUses = append(block.ClaimUses, types.RenderedClaimUse{ClaimForm: form})
				}
			}
			view := &types.AnswerSemanticView{OptionalBlocks: []types.BlockRequirement{{Kind: types.BlockTable, AcceptableClaimForms: tc.forms}}}
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{block}, Citations: tc.citations}
			before, _ := json.Marshal(doc)
			got := preCheckCallChainItemCitationRoleAlignment(doc, view, &types.BusContext{Mutable: mut})
			if len(got) != tc.want {
				t.Errorf("role hints=%+v, want %d", got, tc.want)
			}
			after, _ := json.Marshal(doc)
			if string(after) != string(before) {
				t.Fatal("validator rewrote model-selected source or text")
			}
		})
	}
}

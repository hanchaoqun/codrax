package tool

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This entry was already correct before B1606. Keep its exact selected-form
// behavior while moving the decision into the shared pre/post helper.
func TestB1606PreEmitCitationRoleSelectionContractUnchanged(t *testing.T) {
	for _, family := range []types.QuestionFamily{types.QFEnumeration, types.QFCallChain, types.QFRootCauseTrace} {
		for _, kind := range []types.AnswerBlockKind{types.BlockOrderedList, types.BlockBulletList, types.BlockTable} {
			for _, tc := range []struct {
				name   string
				claims []types.RenderedClaimUse
				want   int
			}{
				{"explicit_definition", []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact}}, 0},
				{"explicit_import", []types.RenderedClaimUse{{ClaimForm: types.ClaimImportEdge}}, 1},
				{"mixed_forms", []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact}, {ClaimForm: types.ClaimImportEdge}}, 1},
				{"legacy_available_import", nil, 1},
			} {
				t.Run(string(family)+"/"+string(kind)+"/"+tc.name, func(t *testing.T) {
					mut := types.NewMutableState("selected role")
					mut.AppendEvidence([]types.EvidenceItem{
						{ID: "def", Kind: types.EvidenceDirect, Source: "widget.h", LineStart: 10, AnchorKind: types.AnchorDefinition, Subject: "Widget", GroundingStatus: types.GroundingGrounded},
						{ID: "import", Kind: types.EvidenceRelationship, Source: "widget.h", LineStart: 3, AnchorKind: types.AnchorImport, Subject: "Widget", Object: "Base", GroundingStatus: types.GroundingGrounded},
					})
					ctx := &types.BusContext{Mutable: mut}
					block := types.AnswerBlock{ID: "members", Kind: kind, FacetIDs: []string{string(types.FacetEnumerationItem)},
						ClaimUses: tc.claims, Items: []types.AnswerBlockItem{{ID: "widget", Label: "Widget", CitationRef: 0}}}
					view := &types.AnswerSemanticView{Family: family, OptionalBlocks: []types.BlockRequirement{{
						Kind: kind, FacetIDs: []string{string(types.FacetEnumerationItem)}, AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact, types.ClaimImportEdge},
					}}}
					doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{block}, Citations: []types.Citation{{File: "widget.h", Line: 10}}}
					if hints := preCheckCallChainItemCitationRoleAlignment(doc, view, ctx); len(hints) != tc.want {
						t.Fatalf("pre-emit selected-form semantics changed: hints=%+v want=%d", hints, tc.want)
					}
					if !reflect.DeepEqual(block, doc.Blocks[0]) {
						t.Fatal("role validation rewrote model block")
					}
					// A matching citation still satisfies explicit and legacy roles.
					doc.Citations[0].Line = 3
					if hints := preCheckCallChainItemCitationRoleAlignment(doc, view, ctx); len(hints) != 0 {
						t.Fatalf("matching relation citation rejected: %+v", hints)
					}
				})
			}
		}
	}
}

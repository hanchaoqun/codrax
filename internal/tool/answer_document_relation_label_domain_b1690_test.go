package tool

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// White-box classification/ownership negatives supplement the public agent
// transaction tests. Display classification must never serve as admission.
func TestB1690ReaderDomainDoesNotGrantAdmission(t *testing.T) {
	for _, kind := range []types.AnswerBlockKind{types.BlockOrderedList, types.BlockBulletList, types.BlockTable} {
		for _, shape := range []string{"selected", "no_claim", "no_item", "existing_anchor", "duplicate_id", "unknown_id", "has_diagram", "unsupported_kind"} {
			t.Run(string(kind)+"/"+shape, func(t *testing.T) {
				block := types.AnswerBlock{ID: "path", Kind: kind,
					ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, EvidenceID: "selected"}},
					Items:     []types.AnswerBlockItem{{ID: "step", EvidenceIDs: []string{"selected"}}}}
				candidate := types.AnswerDiagramRelationRepairCandidate{BlockID: "path", EvidenceID: "selected", RelationKind: types.DiagramRelCall}
				wantDisplay, wantAdmission := true, true
				switch shape {
				case "no_claim":
					block.ClaimUses = nil
					wantAdmission = false
				case "no_item":
					block.Items = nil
					wantAdmission = false
				case "existing_anchor":
					block.EdgeAnchors = []types.DiagramEdgeAnchor{{RelationKind: types.DiagramRelCall}}
					wantAdmission = false
				case "duplicate_id", "unknown_id":
					wantDisplay, wantAdmission = false, false
				case "has_diagram":
					block.Diagram = &types.AnswerDiagramBlock{}
					wantDisplay = false
				case "unsupported_kind":
					block.Kind = types.BlockSummary
					wantDisplay, wantAdmission = false, false
				}
				doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{block}}
				if shape == "duplicate_id" {
					doc.Blocks = append(doc.Blocks, block)
				}
				if shape == "unknown_id" {
					candidate.BlockID = "other"
				}
				if got := answerDocumentRelationEndpointsAreReaderLabels(doc, candidate.BlockID); got != wantDisplay {
					t.Errorf("display classification=%v want=%v", got, wantDisplay)
				}
				if got := types.AnswerDocumentStandaloneRelationAdditionCandidateSelected(doc, candidate); got != wantAdmission {
					t.Errorf("existing admission changed=%v want=%v", got, wantAdmission)
				}
			})
		}
	}
	if answerDocumentRelationEndpointsAreReaderLabels(nil, "path") || answerDocumentRelationEndpointsAreReaderLabels(&types.AnswerDocumentV2{}, "") {
		t.Fatal("unknown display domain was guessed")
	}
}

func TestB1690CandidateDisplayProjectionPreservesOtherDomainsAndIdentity(t *testing.T) {
	for _, identity := range []string{"module.Service.run", "Factory::create", "legitimate_0123456789abcdef"} {
		t.Run(identity, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
				{ID: "list", Kind: types.BlockOrderedList}, {ID: "diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{}},
				{ID: "ambiguous", Kind: types.BlockTable}, {ID: "ambiguous", Kind: types.BlockTable},
			}}
			var candidates []types.AnswerDiagramRelationRepairCandidate
			for _, id := range []string{"list", "diagram", "unknown", "ambiguous"} {
				candidates = append(candidates, types.AnswerDiagramRelationRepairCandidate{BlockID: id, FromIdentity: identity,
					ToIdentity: "model.Selected", RelationKind: types.DiagramRelCall, EvidenceID: "ev-chosen", Source: "src/file.go:17",
					FromNodeIDs: []string{"carrier_A"}, ToNodeIDs: []string{"carrier_B"}})
			}
			before := append([]types.AnswerDiagramRelationRepairCandidate(nil), candidates...)
			got := standaloneRelationRepairReaderCandidates(doc, candidates)
			want := append([]types.AnswerDiagramRelationRepairCandidate(nil), before...)
			want[0].FromNodeIDs, want[0].ToNodeIDs = nil, nil
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(candidates, before) {
				t.Fatalf("projection changed authority/input or another carrier: got=%+v want=%+v original=%+v", got, want, candidates)
			}
		})
	}
}

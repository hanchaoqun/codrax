package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBindConceptualTerminalResolutionReceiptsUsesExactDispatchContract(t *testing.T) {
	view := &types.AnswerSemanticView{
		ConceptualTerminalResolutionContract: &types.ConceptualTerminalResolutionContract{
			Rows: []types.ConceptualTerminalResolutionRow{{
				EvidenceID:       "ev-terminal",
				TerminalCallable: "AuditLog.record",
				ExactOperation:   "System.out.println",
				Source:           "src/AuditLog.java:6",
				AllowedConclusions: []types.ConceptualTerminalResolutionConclusion{
					types.ConceptualTerminalResolutionCurrentTerminalDiffers,
					types.ConceptualTerminalResolutionDestinationUnproven,
				},
			}},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "summary", Kind: types.BlockSummary, Text: "model conclusion",
		ConceptualTerminalResolution: &types.AnswerConceptualTerminalResolutionReceipt{
			EvidenceID: "ev-terminal",
			Conclusion: types.ConceptualTerminalResolutionCurrentTerminalDiffers,
		},
	}}}
	if err := bindConceptualTerminalResolutionReceipts(doc, view); err != nil {
		t.Fatalf("bind exact conceptual-terminal receipt: %v", err)
	}
	if got := doc.Blocks[0].ConceptualTerminalResolution; !got.IsBound() || got.BoundRow.ExactOperation != "System.out.println" {
		t.Fatalf("receipt did not bind exact terminal operation: %+v", got)
	}

	doc.Blocks[0].ConceptualTerminalResolution = &types.AnswerConceptualTerminalResolutionReceipt{
		EvidenceID: "ev-invented",
		Conclusion: types.ConceptualTerminalResolutionCurrentTerminalDiffers,
	}
	if err := bindConceptualTerminalResolutionReceipts(doc, view); err == nil {
		t.Fatal("invented terminal evidence id must be rejected")
	}
}

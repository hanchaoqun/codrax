package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answerDocumentRelationEndpointsAreReaderLabels classifies only the display
// domain of one exact existing carrier. It is not an admission predicate: the
// lease, claim/evidence selection and ordinary relation gates still decide
// whether an operation exists. In particular, existing anchors do not turn a
// list/table into a diagram during a remove+add transaction.
func answerDocumentRelationEndpointsAreReaderLabels(doc *types.AnswerDocumentV2, blockID string) bool {
	blockID = strings.TrimSpace(blockID)
	if doc == nil || blockID == "" {
		return false
	}
	matches, readerLabels := 0, false
	for _, block := range doc.Blocks {
		if strings.TrimSpace(block.ID) != blockID {
			continue
		}
		matches++
		switch block.Kind {
		case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
			readerLabels = block.Diagram == nil
		default:
			readerLabels = false
		}
	}
	return matches == 1 && readerLabels
}

// standaloneRelationRepairReaderCandidates projects the display contract at
// publication, after all candidate/receipt aliases have been collected. The
// exact relation, evidence source and identities remain intact. Syntax-only
// Mermaid aliases are not suggestions for a list/table's reader-facing fields;
// no replacement wording is generated and the caller's candidate pool is not
// mutated. Real diagram candidates retain their complete alias authority.
func standaloneRelationRepairReaderCandidates(doc *types.AnswerDocumentV2, candidates []types.AnswerDiagramRelationRepairCandidate) []types.AnswerDiagramRelationRepairCandidate {
	out := append([]types.AnswerDiagramRelationRepairCandidate(nil), candidates...)
	for i := range out {
		if answerDocumentRelationEndpointsAreReaderLabels(doc, out[i].BlockID) {
			out[i].FromNodeIDs, out[i].ToNodeIDs = nil, nil
		}
	}
	return out
}

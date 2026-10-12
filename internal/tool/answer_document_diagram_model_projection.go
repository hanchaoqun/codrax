package tool

import "github.com/hanchaoqun/codrax/internal/types"

// diagramModelAuthoredRelationDocument is a read-only claim projection, not a
// display filter. Private deterministic blocks keep their producer authority
// and remain in the original answer; they cannot lend labels, edges or owners
// to a model-authored sibling. The marker cannot be supplied by model JSON.
func diagramModelAuthoredRelationDocument(doc *types.AnswerDocumentV2) *types.AnswerDocumentV2 {
	if doc == nil {
		return nil
	}
	for _, block := range doc.Blocks {
		if block.SystemGeneratedKind == types.AnswerSystemGeneratedBlockUnknown {
			continue
		}
		out := *doc
		out.Blocks = make([]types.AnswerBlock, 0, len(doc.Blocks))
		for _, candidate := range doc.Blocks {
			if candidate.SystemGeneratedKind == types.AnswerSystemGeneratedBlockUnknown {
				out.Blocks = append(out.Blocks, candidate)
			}
		}
		return &out
	}
	return doc
}

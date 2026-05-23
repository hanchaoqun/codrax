package types

// AnswerPresentationContract captures model-output display affordances that
// are orthogonal to the question family. A family decides the principal answer
// obligations; the presentation contract says which extra visible carriers the
// schema should tolerate so a model-authored table, scalar, decision, or
// explicitly requested diagram is not squeezed back into prose.
type AnswerPresentationContract struct {
	AllowedBlocks       []AnswerBlockKind
	DiagramRequired     bool
	DiagramKind         DiagramKind
	RequestedDimensions []RequestedAnswerDimension
}

func CompileAnswerPresentationContract(ir *AnalysisIR, plan *AnswerSurfacePlan) AnswerPresentationContract {
	contract := AnswerPresentationContract{}
	// These carriers are display shapes, not alternate answer intent. Keeping
	// them schema-allowed avoids finalizer retries when the model chooses a
	// clearer table/value/verdict surface for a family whose main scaffold is
	// prose or sections.
	contract.AllowBlocks(BlockTable, BlockScalar, BlockDecision)
	if plan != nil && plan.Diagram != nil && plan.Diagram.Required {
		contract.DiagramRequired = true
		contract.DiagramKind = resolvedDiagramKindForPlan(plan, DiagramNone)
		contract.AllowBlocks(BlockDiagram)
	}
	if plan != nil && len(plan.RequestedAnswerDimensions) > 0 {
		contract.RequestedDimensions = append([]RequestedAnswerDimension(nil), plan.RequestedAnswerDimensions...)
	} else if ir != nil && ir.RequestModel.RequestedAnswerDimensions != nil && ir.RequestModel.RequestedAnswerDimensions.Active() {
		contract.RequestedDimensions = append([]RequestedAnswerDimension(nil), ir.RequestModel.RequestedAnswerDimensions.Dimensions...)
	}
	return contract
}

func (c *AnswerPresentationContract) AllowBlocks(kinds ...AnswerBlockKind) {
	if c == nil {
		return
	}
	for _, kind := range kinds {
		if kind == "" || c.AllowsBlock(kind) {
			continue
		}
		c.AllowedBlocks = append(c.AllowedBlocks, kind)
	}
}

func (c AnswerPresentationContract) AllowsBlock(kind AnswerBlockKind) bool {
	if kind == "" {
		return false
	}
	for _, allowed := range c.AllowedBlocks {
		if allowed == kind {
			return true
		}
	}
	return false
}

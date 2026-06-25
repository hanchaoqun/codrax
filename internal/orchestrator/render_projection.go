package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func renderAnswerDimensionsForPresentation(contract types.AnswerPresentationContract) []render.AnswerDimensionInfo {
	out := make([]render.AnswerDimensionInfo, 0, len(contract.RequestedDimensions))
	for _, dim := range contract.RequestedDimensions {
		label := strings.TrimSpace(dim.Label)
		if label == "" {
			continue
		}
		out = append(out, render.AnswerDimensionInfo{
			Index:    dim.Index,
			Label:    label,
			Required: dim.Required,
		})
	}
	return out
}

func renderVisibleEvidenceNodeCount(nodes []types.TaskNode) int {
	count := 0
	for _, n := range nodes {
		if n.IsCounterfactual || n.Type != types.NodeEvidence {
			continue
		}
		count++
	}
	return count
}

func renderInvestigationUnitForEvidenceNode(plan types.InvestigationPlan, nodeID string, ordinal, evidenceNodeCount int) (types.InvestigationUnit, bool) {
	if unit, ok := plan.InvestigationUnitForEvidenceNode(nodeID); ok {
		return unit, true
	}
	if len(plan.Units) < 2 || evidenceNodeCount != len(plan.Units) {
		return types.InvestigationUnit{}, false
	}
	if ordinal < 0 || ordinal >= len(plan.Units) {
		return types.InvestigationUnit{}, false
	}
	return plan.Units[ordinal], true
}

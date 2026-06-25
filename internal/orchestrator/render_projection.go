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

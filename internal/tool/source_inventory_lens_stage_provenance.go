package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// SourceInventoryLensStageProvenance records which pipeline stage produced a
// repo_map source-inventory observation.
func SourceInventoryLensStageProvenance(ctx *types.BusContext) string {
	if ctx == nil {
		return ""
	}
	stage := strings.TrimSpace(string(ctx.PipelineStage))
	if stage == "" {
		return ""
	}
	return "repo_lens:stage:" + stage
}

func sourceInventoryLensQueryProvenance(ctx *types.BusContext, query types.SourceInventoryLensQuery) []string {
	provenance := []string{types.SourceInventoryProvenanceRepoLensToolQuery}
	if stage := SourceInventoryLensStageProvenance(ctx); stage != "" {
		provenance = append(provenance, stage)
	}
	return append(provenance, query.Provenance...)
}

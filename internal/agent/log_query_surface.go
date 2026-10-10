package agent

import "github.com/hanchaoqun/codrax/internal/types"

// The live catalog, never a path token or a persisted preview, owns access.
// Pre-triage keeps its existing preview extraction coordinates; full-source
// collection and physical evidence are published by the exploration tool.
func logQueryToolVisible(ctx *types.AgentContext) bool {
	return ctx != nil && ctx.Stage == types.StageExplore && ctx.AttachedLogCatalog != nil
}

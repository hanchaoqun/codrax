package context

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// builder_worktree_base_test.go — V5-1 (§40.35 复核二): the analysis base
// must survive BusContext → AgentContext so the tool dispatch can rebuild it.
func TestBuildAgentContextMirrorsWorktreeBase(t *testing.T) {
	bus := &types.BusContext{
		PipelineStage: types.StageVerify, ActiveAgent: types.AgentVerifier,
		RepoRoot: "/wt", WorktreePath: "/wt", WorktreeBaseSHA: "abc123", WorktreeBaseDirtyPaths: []string{"cfg.py"},
		Mutable: types.NewMutableState("x"),
	}
	ctx := BuildAgentContext(bus, types.AgentVerifier, types.StageVerify)
	if ctx == nil || ctx.WorktreeBaseSHA != "abc123" || len(ctx.WorktreeBaseDirtyPaths) != 1 || ctx.WorktreeBaseDirtyPaths[0] != "cfg.py" {
		t.Fatalf("AgentContext must mirror the analysis base: %+v", ctx)
	}
	tool := types.ToolBusContext(ctx, types.AgentVerifier)
	if tool == nil || tool.WorktreeBaseSHA != "abc123" || len(tool.WorktreeBaseDirtyPaths) != 1 {
		t.Fatalf("the tool BusContext rebuilt from the AgentContext must carry the base end to end: %+v", tool)
	}
}

package types

import "testing"

// bus_context_projection_worktree_base_test.go — V5-1 (§40.35 复核二 ★inert
// lane): the run_tests dispatch rebuilds its BusContext from the
// AgentContext, so the analysis base must survive that projection or the
// whole source-line binding lane is silently disabled in production.
func TestToolBusContextCarriesWorktreeBase(t *testing.T) {
	ctx := &AgentContext{WorktreePath: "/wt", WorktreeBaseSHA: "abc123", WorktreeBaseDirtyPaths: []string{"cfg.py"}, Mutable: NewMutableState("x")}
	bus := ToolBusContext(ctx, "verifier")
	if bus == nil || bus.WorktreeBaseSHA != "abc123" || len(bus.WorktreeBaseDirtyPaths) != 1 || bus.WorktreeBaseDirtyPaths[0] != "cfg.py" {
		t.Fatalf("tool BusContext must carry the analysis base: %+v", bus)
	}
	ctx.WorktreeBaseDirtyPaths[0] = "mutated"
	if bus.WorktreeBaseDirtyPaths[0] != "cfg.py" {
		t.Fatal("dirty roster must be copied, not aliased")
	}
}

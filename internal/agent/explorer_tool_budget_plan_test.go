package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The analyzer-compiled per-tool plan must reach the explorer prompt
// as soft guidance, with the heaviest tool leading.
func TestExplorerToolBudgetPlanRendered(t *testing.T) {
	mu := types.NewMutableState("q")
	mu.SetExploreBudget(&types.ExploreBudget{
		PerToolCap: map[string]int{"repo_map": 8, "grep": 6, "read_file": 6},
		OverallCap: 20,
	})
	ctx := &types.AgentContext{Objective: "q", Mutable: mu}
	got := renderExplorerToolBudgetPlan(ctx)
	if !strings.Contains(got, "Tool Budget Plan") {
		t.Fatalf("plan section missing: %q", got)
	}
	if !strings.Contains(got, "`repo_map` ×8") {
		t.Fatalf("plan must show per-tool allowances: %q", got)
	}
	if idx := strings.Index(got, "repo_map"); idx < 0 || idx > strings.Index(got, "grep") {
		t.Fatalf("heaviest tool must lead the plan: %q", got)
	}
}

// Without an installed budget the section must not render — non-DAG
// paths (tests, one-shot dispatches) stay byte-identical.
func TestExplorerToolBudgetPlanAbsent(t *testing.T) {
	mu := types.NewMutableState("q")
	ctx := &types.AgentContext{Objective: "q", Mutable: mu}
	if got := renderExplorerToolBudgetPlan(ctx); got != "" {
		t.Fatalf("no budget installed must render nothing, got %q", got)
	}
	if got := renderExplorerToolBudgetPlan(nil); got != "" {
		t.Fatalf("nil ctx must render nothing")
	}
}

package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The analyzer-compiled per-tool plan must reach the explorer prompt
// as low-noise soft guidance without leaking raw allowance numbers.
func TestExplorerToolBudgetPlanRendered(t *testing.T) {
	mu := types.NewMutableState("q")
	mu.SetExploreBudget(&types.ExploreBudget{
		PerToolCap: map[string]int{"repo_map": 8, "grep": 66, "read_file": 57},
		OverallCap: 20,
	})
	ctx := &types.AgentContext{Objective: "q", Mutable: mu}
	got := renderExplorerToolBudgetPlan(ctx)
	if !strings.Contains(got, "Tool Budget Plan") {
		t.Fatalf("plan section missing: %q", got)
	}
	for _, want := range []string{"`repo_map`", "`grep`", "`read_file`", "not a work quota", "typed navigation/runtime policy"} {
		if !strings.Contains(got, want) {
			t.Fatalf("plan missing %q: %q", want, got)
		}
	}
	for _, notWant := range []string{"×66", "×57", "×8", "largest allowance", "expected to carry discovery"} {
		if strings.Contains(got, notWant) {
			t.Fatalf("plan leaked raw budget/ranking phrase %q: %q", notWant, got)
		}
	}
	if idx := strings.Index(got, "repo_map"); idx < 0 || idx > strings.Index(got, "grep") {
		t.Fatalf("repo_map should remain first in source-navigation display order despite lower cap: %q", got)
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

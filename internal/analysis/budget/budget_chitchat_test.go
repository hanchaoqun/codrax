package budget

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// CHATFIX-1: a chitchat classification (zero keywords/entities) must
// mint the gate-floor budget, not scale below budget_sanity (≥5 files /
// ≥4 iters) — the sub-floor budget hard-rejected the IR and pushed the
// greeting into the degraded full-explore path (live e2e witness).
func TestComputeChitchatMintsGateFloorBudget(t *testing.T) {
	rm := types.RequestModel{Scenario: types.ScenarioChitchat, ChitchatReply: "你好！"}
	got := Compute(rm, BudgetSignals{Complexity: types.ComplexitySimple})
	if got.MaxFiles < 5 || got.MaxReactIters < 4 {
		t.Fatalf("chitchat budget must satisfy the budget_sanity floor, got files=%d iters=%d", got.MaxFiles, got.MaxReactIters)
	}
	// Reply-less degenerate form keeps the normal scaled path.
	bare := Compute(types.RequestModel{Scenario: types.ScenarioChitchat}, BudgetSignals{Complexity: types.ComplexitySimple})
	if bare.MaxFiles == got.MaxFiles && bare.MaxReactIters == got.MaxReactIters && bare.MaxToolCalls == got.MaxToolCalls {
		t.Fatalf("reply-less chitchat must not take the carve-out (got same budget %+v)", bare)
	}
}

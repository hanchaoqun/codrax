package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC292BudgetPromptUsesSharedConsumptionAndDisclosesHistory(t *testing.T) {
	for _, historical := range []int{0, 4} {
		mu := types.NewMutableState("continue")
		mu.SetWriteWorkflowRun(&types.WriteWorkflowRun{RunID: "restored", Budget: types.WriteWorkflowBudget{MaxBatches: 5, BatchesUsed: historical}, Batches: []types.WriteWorkflowBatch{{ID: "source"}}})
		text := renderWriteControllerRunSection(&types.AgentContext{Mutable: mu})
		if historical == 0 && !strings.Contains(text, "budget: batches 1/5") {
			t.Fatalf("roster consumption absent: %s", text)
		}
		if historical == 4 && (!strings.Contains(text, "budget: batches 4/5") || !strings.Contains(text, "retained batch roster: 1; recorded consumption lower bound: 4")) {
			t.Fatalf("historical use masquerades as roster: %s", text)
		}
	}
}

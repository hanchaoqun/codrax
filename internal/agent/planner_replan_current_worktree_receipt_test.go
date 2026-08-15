package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBuildReplanCurrentWorktreeReceiptSectionLeadsWithCurrentGeneration(t *testing.T) {
	mu := types.NewMutableState("current generation prompt")
	mu.SetWriteWorkflowRun(&types.WriteWorkflowRun{ActiveBatchID: "batch-1"})
	mu.SetReplanCurrentWorktreeReceipt(&types.ReplanCurrentWorktreeReceipt{
		BatchID:           "batch-1",
		SourcePlanID:      "plan-applied",
		ApplyGeneration:   2,
		DiffFingerprint:   "diff-sha",
		TriggerReasonCode: "truth_ledger_failed_requires_repair",
		Paths: []types.ReplanCurrentPathState{{
			Path:                "relativedelta.py",
			State:               types.ReplanWorktreePathPresent,
			CurrentSHA256:       strings.Repeat("a", 64),
			CurrentBytes:        123,
			AppliedEditTotal:    1,
			AppliedEditComplete: true,
			AppliedEdits: []types.ReplanAppliedEditReceipt{{
				Kind: "added", Line: 16, Text: "    def _normalize(self, value, name):",
			}},
			CurrentSourceSnapshots: []types.RepairSourceSnapshot{{
				Path: "relativedelta.py", LineStart: 10, LineEnd: 22,
				ReasonCode: "current_applied_generation",
				Snippet:    "class relativedelta(object):\n    def _normalize(self, value, name):\n        return value\n",
			}},
		}},
	})

	section := (&plannerEvaluator{}).buildReplanCurrentWorktreeReceiptSection(&types.AgentContext{Mutable: mu})
	for _, want := range []string{
		"Current applied worktree generation",
		"source_plan: plan-applied apply_generation: 2",
		"current_path: relativedelta.py state=present",
		"already_applied_edit: kind=added line=16",
		"def _normalize",
		"Do not re-add an already-applied edit",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("current-state section missing %q:\n%s", want, section)
		}
	}
}

func TestBuildReplanCurrentWorktreeReceiptSectionRejectsOtherBatch(t *testing.T) {
	mu := types.NewMutableState("wrong batch receipt")
	mu.SetWriteWorkflowRun(&types.WriteWorkflowRun{ActiveBatchID: "batch-2"})
	mu.SetReplanCurrentWorktreeReceipt(&types.ReplanCurrentWorktreeReceipt{
		BatchID: "batch-1", SourcePlanID: "plan-old",
		Paths: []types.ReplanCurrentPathState{{Path: "x.go"}},
	})
	if got := (&plannerEvaluator{}).buildReplanCurrentWorktreeReceiptSection(&types.AgentContext{Mutable: mu}); got != "" {
		t.Fatalf("other-batch receipt must not enter planner prompt: %s", got)
	}
}

package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func hmc367Actions(t *testing.T, run types.WriteWorkflowRun, want []string) {
	t.Helper()
	mu := types.NewMutableState("resume the existing run without changes")
	mu.SetWriteWorkflowRun(&run)
	ctx := &types.AgentContext{Mode: types.ModeApply, Mutable: mu}
	var schema struct {
		Properties map[string]struct{ Enum []string }
	}
	if err := json.Unmarshal((&tool.EmitWriteWorkflowDecision{}).ParametersFor(ctx), &schema); err != nil {
		t.Fatal(err)
	}
	got := schema.Properties["action"].Enum
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("actual tool schema advertises impossible or hides legal actions: got %v want %v", got, want)
	}
	if prompt := hmc366ControllerPrompt(t, &run); !strings.Contains(prompt, "- action enum: "+strings.Join(want, ", ")+"\n") {
		t.Fatal("controller teaching disagrees with the actual tool schema")
	}
}

func TestHMC367ControllerCompletedActiveActionsAgreeWithExecution(t *testing.T) {
	run := types.WriteWorkflowRun{RunID: "rolling", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "active", Batches: []types.WriteWorkflowBatch{{
		ID: "active", PlanID: "proof", Status: types.WriteWorkflowBatchComplete,
		Completion: &types.WriteWorkflowCompletion{Verdict: types.WriteWorkflowCompletionVerified, ReasonCode: "tests_passed"},
	}}}
	hmc367Actions(t, run, []string{"explore_code", "append_batch", "split_batch", "finish", "block"})
	view := writeflow.DeriveWorkflowExecutionView(types.ModeApply, run, nil)
	for _, id := range []string{"active", "next"} {
		d := writeflow.WriteWorkflowDecision{Action: writeflow.ActionExploreCode, Batch: &writeflow.WriteBatchPlan{ID: id, Goal: "Read the next bounded slice"},
			ExplorationRequest: &types.WriteExplorationRequest{BatchID: id, Goal: "Read the next bounded slice"}}
		if got := writeflow.ValidateWorkflowTransition(view, d); got.Allowed != (id == "next") {
			t.Fatalf("completion permits distinct exploration only: %s = %+v", id, got)
		}
	}
	run.Status = types.WriteWorkflowRunComplete
	hmc367Actions(t, run, []string{"finish", "block"})
	mu := types.NewMutableState("restored")
	mu.SetWriteWorkflowRun(&run)
	result, err := (&tool.EmitWriteWorkflowDecision{}).Execute(&types.BusContext{Mode: types.ModeApply, Mutable: mu}, json.RawMessage(`{"action":"verify_batch"}`))
	if err != nil || result.Success || len(mu.WriteWorkflowDecisionJSON()) > 0 {
		t.Fatal("executor admitted an inapplicable completed-batch action")
	}
}

func TestHMC367ControllerReplaysAdvertisedActionAgainstRestoredProof(t *testing.T) {
	path := os.Getenv("CODRAX_HMC367_RESTORE_ARCHIVE")
	if path == "" {
		t.Skip("requires the sealed §366 restored workflow")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const digest = "b5b7377c0a909e3a0a116ec6401e1cba2d0875d364f0e0ee5e9a539ef46e001e"
	if fmt.Sprintf("%x", sha256.Sum256(body)) != digest {
		t.Fatal("not the exact §366 original workflow")
	}
	var run types.WriteWorkflowRun
	if err := json.Unmarshal(body, &run); err != nil {
		t.Fatal(err)
	}
	run.Status, run.Completion = types.WriteWorkflowRunInProgress, nil // at the final controller dispatch
	hmc367Actions(t, run, []string{"explore_code", "append_batch", "split_batch", "finish", "block"})
	if writeflow.WorkflowActionAllowedInRun(writeflow.ActionVerifyBatch, types.ModeApply, &run) {
		t.Fatal("the original model's repeated verify is still advertised")
	}
	for i := range run.Batches {
		if run.Batches[i].ID == run.ActiveBatchID {
			run.Batches[i].Status = types.WriteWorkflowBatchVerifying
			run.Batches[i].Completion = nil
		}
	}
	if !writeflow.WorkflowActionAllowedInRun(writeflow.ActionVerifyBatch, types.ModeApply, &run) {
		t.Fatal("masked the legitimate first verification on the same restored plan")
	}
}

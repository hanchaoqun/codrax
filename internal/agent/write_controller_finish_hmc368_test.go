package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func hmc368FinishWithMissingHistory(t *testing.T, run types.WriteWorkflowRun) {
	t.Helper()
	mu := types.NewMutableState("Complete the verification for the current scope")
	mu.SetWriteWorkflowRun(&run)
	parameters := (&tool.EmitWriteWorkflowDecision{}).ParametersFor(&types.AgentContext{Mode: types.ModeApply, Mutable: mu})
	var schema struct {
		Properties map[string]struct{ Description string }
	}
	if err := json.Unmarshal(parameters, &schema); err != nil {
		t.Fatal(err)
	}
	if want := "Omit finish_disposition for normal completion"; !strings.Contains(schema.Properties["finish_disposition"].Description, want) {
		t.Errorf("actual tool schema omitted the normal completion choice: %s", schema.Properties["finish_disposition"].Description)
	}
	if got := hmc366ControllerPrompt(t, &run); !strings.Contains(got, "finish the current scope without finish_disposition") {
		t.Error("controller prompt explains why all_verified is wrong but not how to deliver a finished current scope")
	}
	if !writeflow.WorkflowActionAllowedInRun(writeflow.ActionAppendBatch, types.ModeApply, &run) {
		t.Fatal("a legitimate different next batch must remain available")
	}
	bus := &types.BusContext{Mode: types.ModeApply, Mutable: mu}
	result, err := (&tool.EmitWriteWorkflowDecision{}).Execute(bus, json.RawMessage(`{"action":"finish"}`))
	if err != nil || !result.Success {
		t.Fatalf("plain finish did not reach the real emit boundary: %+v %v", result, err)
	}
	var decision writeflow.WriteWorkflowDecision
	if err := json.Unmarshal(mu.WriteWorkflowDecisionJSON(), &decision); err != nil {
		t.Fatal(err)
	}
	out, err := writeflow.ApplyWorkflowDecisionToRun(run, decision)
	if err != nil || out.Status != types.WriteWorkflowRunComplete || out.ActiveBatchID != run.ActiveBatchID ||
		len(out.Batches) != len(run.Batches) || out.Completion == nil ||
		out.Completion.Verdict != types.WriteWorkflowCompletionUnverified || out.Completion.ReasonCode != writeflow.MissingTerminalVerificationReason {
		t.Fatalf("plain finish failed or invented proof/changed ownership: %+v %v", out, err)
	}
	if proof, ok := writeflow.CompletedBatchVerification(out.Batches[len(out.Batches)-1]); !ok || proof == nil || proof.Verdict != types.WriteWorkflowCompletionVerified {
		t.Fatalf("plain finish lost the current proof: %+v", proof)
	}
	if prior, complete := writeflow.CompletedBatchVerification(out.Batches[0]); !complete || prior != nil {
		t.Fatalf("plain finish back-signed history: %+v", prior)
	}
}

func TestHMC368ControllerExplainsFinishWithoutHistoricalVerification(t *testing.T) {
	hmc368FinishWithMissingHistory(t, types.WriteWorkflowRun{
		RunID: "restored", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "current",
		Batches: []types.WriteWorkflowBatch{
			{ID: "source", Status: types.WriteWorkflowBatchComplete, PlanID: "applied"},
			{ID: "current", Status: types.WriteWorkflowBatchComplete, PlanID: "verified",
				Completion: &types.WriteWorkflowCompletion{Verdict: types.WriteWorkflowCompletionVerified, ReasonCode: "tests_passed"}},
		},
	})
}

func TestHMC368ControllerReplaysPlainFinishAfterOriginalFailedDispatch(t *testing.T) {
	path, digest := os.Getenv("CODRAX_HMC368_RESTORE_ARCHIVE"), os.Getenv("CODRAX_HMC368_RESTORE_ARCHIVE_SHA256")
	if path == "" || digest == "" {
		t.Skip("requires the sealed §367 workflow")
	}
	body, err := os.ReadFile(path)
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(body)) != digest {
		t.Fatalf("cannot use the exact original workflow: %v", err)
	}
	var run types.WriteWorkflowRun
	if err := json.Unmarshal(body, &run); err != nil {
		t.Fatal(err)
	}
	if len(run.Batches) != 3 || run.Batches[2].Status != types.WriteWorkflowBatchReadyToPlan ||
		run.Batches[1].ID != "proof" || run.Batches[2].ID != run.ActiveBatchID {
		t.Fatalf("not the original erroneous continuation: %+v", run.Batches)
	}
	// Replay the dispatch boundary before the erroneous append, using the
	// exact saved source/proof batches and their native attempts, not edits.
	run.ActiveBatchID = run.Batches[1].ID
	run.Batches = run.Batches[:2]
	hmc368FinishWithMissingHistory(t, run)
}

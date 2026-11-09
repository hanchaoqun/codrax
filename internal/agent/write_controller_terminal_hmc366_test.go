package agent

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func hmc366ControllerPrompt(t *testing.T, run *types.WriteWorkflowRun) string {
	t.Helper()
	dir := t.TempDir()
	mu := types.NewMutableState("Run the retained tests without modifying any files.")
	mu.SetWriteWorkflowRun(run)
	before, _ := json.Marshal(mu.WriteWorkflowRun())
	reg := tool.NewRegistry()
	reg.Register(&tool.EmitWriteWorkflowDecision{})
	capture := &traceTeachingCaptureLLM{stop: errors.New("capture actual controller adapter request")}
	controller := NewWriteControllerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1})
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Mode: types.ModeApply, AgentName: types.AgentWriteController, Stage: types.StageWriteController, Mutable: mu}
	if _, err := controller.Execute(ctx, traceTeachingSkill(t, "write-controller-skill")); !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("did not reach the actual controller adapter once: calls=%d err=%v", capture.calls, err)
	}
	after, _ := json.Marshal(mu.WriteWorkflowRun())
	if string(before) != string(after) {
		t.Fatal("a model-facing projection modified durable completion")
	}
	var all strings.Builder
	for _, msg := range capture.messages {
		all.WriteString(msg.Content)
		all.WriteByte('\n')
	}
	return all.String()
}

func TestHMC366ControllerKeepsBatchPhaseAndVerificationSeparate(t *testing.T) {
	// The active local verification can pass while the earlier apply-only
	// batch has no source proof. The capped roster cannot hide that fact.
	run := &types.WriteWorkflowRun{RunID: "restore", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "proof"}
	for i := range 8 {
		run.Batches = append(run.Batches, types.WriteWorkflowBatch{ID: fmt.Sprintf("peer-%d", i), Status: types.WriteWorkflowBatchComplete,
			Completion: &types.WriteWorkflowCompletion{Verdict: types.WriteWorkflowCompletionVerified, ReasonCode: "tests_passed"}})
	}
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{ID: "source", PlanID: "apply-plan", Status: types.WriteWorkflowBatchComplete, ApplyRef: "applied-commit",
		Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied"}}},
		types.WriteWorkflowBatch{ID: "proof", PlanID: "proof-plan", Status: types.WriteWorkflowBatchComplete,
			Completion: &types.WriteWorkflowCompletion{Verdict: types.WriteWorkflowCompletionVerified, ReasonCode: "tests_passed"}})
	prompt := hmc366ControllerPrompt(t, run)
	if !strings.Contains(prompt, "terminal verification among complete batches: verified=9 unverified=0 accepted_failed=0 missing=1") ||
		!strings.Contains(prompt, "complete is a lifecycle state, not verification") {
		t.Fatal("omitted apply-only batch was not accounted for as missing proof in the actual request")
	}
	run.Batches = append([]types.WriteWorkflowBatch(nil), run.Batches[8:]...)
	prompt = hmc366ControllerPrompt(t, run)
	for _, want := range []string{"source state=complete verification_terminal=missing verification_reason=missing_terminal_verify_verdict",
		"proof state=complete verification_terminal=verified verification_reason=tests_passed"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("actual controller request omitted typed fact %q", want)
		}
	}
	run.Status = types.WriteWorkflowRunComplete
	writeflow.MarkWorkflowRunCompletionFromBatches(run)
	if run.Completion == nil || run.Completion.Verdict != types.WriteWorkflowCompletionUnverified || run.Completion.ReasonCode != "missing_terminal_verify_verdict" {
		t.Fatal("the existing durable aggregation boundary changed")
	}
}

func TestHMC366ControllerReplaysArchivedNegativeRestore(t *testing.T) {
	path := os.Getenv("CODRAX_HMC366_RESTORE_ARCHIVE")
	if path == "" {
		t.Skip("requires the sealed §365 actual restored workflow")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(body)) != "84a8f5aea66a4135113258da5da1b239a0c6ca20a4c745555a29c8b374ef6b2a" {
		t.Fatal("the exact previous workflow original is required")
	}
	var run types.WriteWorkflowRun
	if err := json.Unmarshal(body, &run); err != nil {
		t.Fatal(err)
	}
	// Present the same batch facts at decision time; the old terminal
	// adjudication is never supplied as the answer to the new dispatch.
	run.Status, run.Completion = types.WriteWorkflowRunInProgress, nil
	prompt := hmc366ControllerPrompt(t, &run)
	for _, want := range []string{"source state=complete verification_terminal=missing verification_reason=missing_terminal_verify_verdict",
		"proof state=complete verification_terminal=verified verification_reason=tests_passed",
		"terminal verification among complete batches: verified=1 unverified=0 accepted_failed=0 missing=1"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("archived proof/source split absent: %s", want)
		}
	}
}

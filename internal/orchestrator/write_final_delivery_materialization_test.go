package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Decode the public durable wire, rather than reaching into the new producer,
// so the RED also proves that the terminal report actually publishes it.
type b1693MaterializationWire struct {
	SchemaVersion   int      `json:"schema_version"`
	Status          string   `json:"status"`
	ReasonCode      string   `json:"reason_code"`
	RunID           string   `json:"run_id"`
	FinalPlanID     string   `json:"final_plan_id"`
	RetainedPlanIDs []string `json:"retained_plan_ids"`
	Owners          []struct {
		PlanID    string   `json:"plan_id"`
		CommitSHA string   `json:"commit_sha"`
		Paths     []string `json:"paths"`
	} `json:"owners"`
}

func b1693AppliedPlan(id, sha string, paths ...string) *types.ChangePlan {
	plan := &types.ChangePlan{ID: id, Status: types.PlanStatusVerifyFailed,
		AppliedCommitSHA: sha, AppliedPaths: append([]string(nil), paths...),
		ApplyCheckpoint: &types.ApplyCheckpointRecord{CommitSHA: sha,
			RecoveryRef: "refs/codrax/applied/" + id, CommittedPaths: append([]string(nil), paths...)}}
	for _, path := range paths {
		plan.Changes = append(plan.Changes, types.FileChange{Path: path, Kind: "patch"})
		plan.TargetPaths = append(plan.TargetPaths, path)
	}
	return plan
}

func b1693ProofPlan(id string) *types.ChangePlan {
	return &types.ChangePlan{ID: id, Status: types.PlanStatusUnverified,
		PersistenceKind: types.PlanPersistenceProofProbeOnly, TargetPaths: []string{"src/app.py"},
		VerificationProbes: []types.VerificationProbe{{ID: "probe", Language: "python", Code: "assert True"}}}
}

func b1693Run(planIDs ...string) *types.WriteWorkflowRun {
	run := &types.WriteWorkflowRun{RunID: "run-materialization", Status: types.WriteWorkflowRunComplete,
		ActiveBatchID: "batch", Completion: &types.WriteWorkflowCompletion{Verdict: types.WriteWorkflowCompletionUnverified},
		Batches: []types.WriteWorkflowBatch{{ID: "batch", Status: types.WriteWorkflowBatchComplete}}}
	for i, id := range planIDs {
		run.Batches[0].Attempts = append(run.Batches[0].Attempts, types.WriteWorkflowAttempt{
			Kind: "apply", Status: "applied", PlanID: id, FinishedAt: time.Unix(100+int64(i), 0)})
	}
	return run
}

func b1693PersistReceipt(t *testing.T, run *types.WriteWorkflowRun, finalPlan *types.ChangePlan, history ...*types.ChangePlan) (*b1693MaterializationWire, *types.WriteFinalReport) {
	t.Helper()
	dir := t.TempDir()
	for _, plan := range history {
		if err := types.WritePlanToFile(plan, filepath.Join(dir, "plans", plan.ID+".json")); err != nil {
			t.Fatal(err)
		}
	}
	mu := types.NewMutableState("materialize all retained applied changes")
	mu.SetChangePlan(finalPlan)
	mu.SetChangeReport(&types.ChangeReport{PlanID: finalPlan.ID, Passed: false,
		VerificationStatus: types.VerificationStatusUnavailable, FailureKind: types.FailureKindRunnerMissing})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: dir, Mode: types.ModeApply}}
	before, err := json.Marshal(finalPlan)
	if err != nil {
		t.Fatal(err)
	}
	o.persistWriteFinalReportIfAuditable(run, "")
	path := filepath.Join(dir, "plans", finalPlan.ID+".final.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Delivery struct {
			Materialization *b1693MaterializationWire `json:"materialization"`
		} `json:"delivery"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Delivery.Materialization == nil {
		t.Fatalf("public final report for %q omitted delivery.materialization", finalPlan.ID)
	}
	receipt := wire.Delivery.Materialization
	if receipt.SchemaVersion != 1 || receipt.RunID != run.RunID || receipt.FinalPlanID != finalPlan.ID {
		t.Fatalf("materialization identity mismatch: %+v", receipt)
	}
	final, err := types.LoadWriteFinalReportFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if final.Plan.ID != finalPlan.ID || final.Plan.Status != finalPlan.Status || final.Verification.Passed ||
		final.Verification.Status != types.VerificationStatusUnavailable || final.Completion == nil ||
		final.Completion.Verdict != types.WriteWorkflowCompletionUnverified {
		t.Fatalf("materialization must not upgrade independent plan/verification/completion: %+v", final)
	}
	after, _ := json.Marshal(finalPlan)
	if string(before) != string(after) {
		t.Fatal("materialization mutated the original final plan")
	}
	return receipt, final
}

func b1693OwnerIDs(receipt *b1693MaterializationWire) []string {
	var ids []string
	for _, owner := range receipt.Owners {
		ids = append(ids, owner.PlanID)
	}
	return ids
}

func TestB1693PublicMaterializationIncludesHistoricalTestOwnersAndProofOnlyFinal(t *testing.T) {
	source := b1693AppliedPlan("source", strings.Repeat("a", 40), "src/app.py")
	tests := b1693AppliedPlan("tests", strings.Repeat("b", 40), "tests/test_app.py")
	// Checkpoint paths are the staged-path eligibility set, not role-filtered
	// planned paths or a claim that every path has a nonempty git delta.
	tests.ApplyCheckpoint.CommittedPaths = []string{"tests/test_app.py", "fixtures/input.bin"}
	proof := b1693ProofPlan("proof")
	run := b1693Run("source", "tests", "proof", "source")
	receipt, final := b1693PersistReceipt(t, run, proof, source, tests)
	if receipt.Status != "available" || !reflect.DeepEqual(b1693OwnerIDs(receipt), []string{"source", "tests"}) ||
		!reflect.DeepEqual(receipt.RetainedPlanIDs, []string{"source", "tests"}) {
		t.Fatalf("historical source/test owners and only real roots must survive proof-only final: %+v", receipt)
	}
	if !reflect.DeepEqual(receipt.Owners[1].Paths, []string{"fixtures/input.bin", "tests/test_app.py"}) || receipt.Owners[1].CommitSHA != tests.AppliedCommitSHA {
		t.Fatalf("receipt lost checkpoint paths or SHA: %+v", receipt)
	}
	// Preserve the pre-existing role projection, including its treatment of
	// the proof plan's target scope; this receipt does not repair that surface.
	if final.Delivery.PrimarySourcePlanID != "source" || !reflect.DeepEqual(final.Delivery.SourceOwnerPlanIDs, []string{"source", "proof"}) {
		t.Fatalf("new all-owner receipt changed legacy source ownership: %+v", final.Delivery)
	}
}

func TestB1693PublicMaterializationSeparatesHistoricalCandidatesFromRestoreRoots(t *testing.T) {
	a := b1693AppliedPlan("old-test", strings.Repeat("a", 40), "tests/test_app.py")
	b := b1693AppliedPlan("source", strings.Repeat("b", 40), "src/app.py")
	c := b1693AppliedPlan("rolled-back", strings.Repeat("c", 40), "src/later.py")
	run := b1693Run(a.ID, b.ID, c.ID)
	run.ProgressLedger = []types.WriteWorkflowProgress{{BatchID: "batch", ReasonCode: "checkpoint_restored_before_replan", At: time.Unix(110, 0)}}
	run.Batches[0].SliceEvents = []types.WriteWorkflowSliceEvent{{Event: types.WriteWorkflowSliceEventRestored, PlanID: b.ID, At: time.Unix(110, 0)}}
	receipt, _ := b1693PersistReceipt(t, run, b, a, c)
	if receipt.Status != "available" || !reflect.DeepEqual(b1693OwnerIDs(receipt), []string{a.ID, b.ID, c.ID}) ||
		!reflect.DeepEqual(receipt.RetainedPlanIDs, []string{b.ID}) {
		t.Fatalf("A→B→C→restore B needs candidate ABC plus root B; consumer, not producer, selects Git ancestors: %+v", receipt)
	}
}

func TestB1693PublicMaterializationRejectsIncompleteOwnerWithoutSubset(t *testing.T) {
	for _, mode := range []string{"missing_plan", "missing_checkpoint", "partial_checkpoint", "commit_error", "missing_commit_sha", "missing_applied_sha", "mismatched_sha", "wrong_recovery_ref", "blank_path", "escape_path", "unrecorded_final_apply", "empty_mutation_not_proof", "missing_attempt_plan_id", "rolled_back_missing_checkpoint"} {
		t.Run(mode, func(t *testing.T) {
			source := b1693AppliedPlan("source", strings.Repeat("a", 40), "src/app.py")
			finalPlan := b1693AppliedPlan("tests", strings.Repeat("b", 40), "tests/test_app.py")
			run := b1693Run(source.ID, finalPlan.ID)
			history := []*types.ChangePlan{source}
			switch mode {
			case "missing_plan":
				history = nil
			case "missing_checkpoint":
				source.ApplyCheckpoint = nil
			case "partial_checkpoint":
				source.ApplyCheckpoint.Partial = true
			case "commit_error":
				source.ApplyCheckpoint.CommitError = "checkpoint failed"
			case "missing_commit_sha":
				source.ApplyCheckpoint.CommitSHA = ""
			case "missing_applied_sha":
				source.AppliedCommitSHA = ""
			case "mismatched_sha":
				source.AppliedCommitSHA = strings.Repeat("d", 40)
			case "wrong_recovery_ref":
				source.ApplyCheckpoint.RecoveryRef = "refs/codrax/applied/other"
			case "blank_path":
				source.ApplyCheckpoint.CommittedPaths = []string{"src/app.py", " "}
			case "escape_path":
				source.ApplyCheckpoint.CommittedPaths = []string{"../outside"}
			case "unrecorded_final_apply":
				run = b1693Run(source.ID)
			case "empty_mutation_not_proof":
				finalPlan = &types.ChangePlan{ID: "tests", Status: types.PlanStatusUnverified}
			case "missing_attempt_plan_id":
				run.Batches[0].Attempts[0].PlanID = ""
			case "rolled_back_missing_checkpoint":
				source.ApplyCheckpoint = nil
				run.ProgressLedger = []types.WriteWorkflowProgress{{BatchID: "batch", ReasonCode: "checkpoint_restored_before_replan", At: time.Unix(110, 0)}}
				run.Batches[0].SliceEvents = []types.WriteWorkflowSliceEvent{{Event: types.WriteWorkflowSliceEventRestored, PlanID: finalPlan.ID, At: time.Unix(110, 0)}}
			}
			receipt, _ := b1693PersistReceipt(t, run, finalPlan, history...)
			if receipt.Status != "unavailable" || receipt.ReasonCode == "" || len(receipt.Owners) != 0 || len(receipt.RetainedPlanIDs) != 0 {
				t.Fatalf("incomplete %s must not publish a recoverable subset or empty success: %+v", mode, receipt)
			}
		})
	}
}

func TestB1693PublicMaterializationPreservesRealAllowEmptyCheckpoint(t *testing.T) {
	plan := b1693AppliedPlan("allow-empty", strings.Repeat("e", 40), "src/app.py")
	plan.ApplyCheckpoint.CommittedPaths = nil // durable omitempty represents the empty set.
	receipt, _ := b1693PersistReceipt(t, b1693Run(plan.ID), plan)
	if receipt.Status != "available" || len(receipt.Owners) != 1 || receipt.Owners[0].Paths == nil ||
		len(receipt.Owners[0].Paths) != 0 || !reflect.DeepEqual(receipt.RetainedPlanIDs, []string{plan.ID}) {
		t.Fatalf("complete real checkpoint with an empty path upper bound must remain a candidate/root: %+v", receipt)
	}
}

func TestB1693PublicMaterializationDoesNotPromoteVerificationScopeToApplyHistory(t *testing.T) {
	plan := b1693ProofPlan("proof")
	plan.CumulativeVerificationScope = &types.CumulativeVerificationScope{
		SourcePlanIDs: []string{"not-an-apply-owner"}, TargetPaths: []string{"src/old.py"}}
	run := b1693Run(plan.ID)
	run.Batches[0].Attempts = append(run.Batches[0].Attempts,
		types.WriteWorkflowAttempt{Kind: "verify", Status: "passed", PlanID: "verify-only"},
		types.WriteWorkflowAttempt{Kind: "apply", Status: "failed", PlanID: "failed-apply"})
	receipt, _ := b1693PersistReceipt(t, run, plan)
	if receipt.Status != "available" || len(receipt.Owners) != 0 || len(receipt.RetainedPlanIDs) != 0 {
		t.Fatalf("verify-only scope or failed apply minted materialization ownership: %+v", receipt)
	}
}

func TestB1693PublicMaterializationAllowsCheckpointDespiteVerificationOrRefFailure(t *testing.T) {
	for _, status := range []string{types.PlanStatusVerifyFailed, types.PlanStatusUnverified} {
		t.Run(status, func(t *testing.T) {
			plan := b1693AppliedPlan("source", strings.Repeat("a", 40), "src/app.py")
			plan.Status = status
			plan.ApplyCheckpoint.TagError = "ref pin failed"
			plan.ApplyCheckpoint.RecoveryRef = ""
			receipt, final := b1693PersistReceipt(t, b1693Run(plan.ID), plan)
			if receipt.Status != "available" || len(receipt.Owners) != 1 || final.Plan.Status != status || !plan.ApplyCheckpoint.DeliveryBroken() {
				t.Fatalf("checkpoint SHA availability must stay separate from verify/ref failure disclosure: receipt=%+v final=%+v", receipt, final)
			}
		})
	}
}

func TestB1693PublicMaterializationEmptyAndStrictProofOnlyRemainDistinct(t *testing.T) {
	for _, mode := range []string{"empty_run", "proof_only", "proof_with_applied_paths", "proof_with_checkpoint"} {
		t.Run(mode, func(t *testing.T) {
			plan := b1693ProofPlan("proof")
			run := b1693Run(plan.ID)
			wantStatus := "available"
			wantReason := "proof_only_no_applied_mutation"
			switch mode {
			case "empty_run":
				run = b1693Run()
				plan = &types.ChangePlan{ID: "empty", Status: types.PlanStatusNoChangeRequired}
				wantReason = "no_applied_mutations"
			case "proof_with_applied_paths":
				plan.AppliedPaths = []string{"src/app.py"}
				wantStatus, wantReason = "unavailable", "apply_checkpoint_missing"
			case "proof_with_checkpoint":
				plan.ApplyCheckpoint = &types.ApplyCheckpointRecord{}
				wantStatus, wantReason = "unavailable", "checkpoint_commit_sha_missing"
			}
			receipt, _ := b1693PersistReceipt(t, run, plan)
			if receipt.Status != wantStatus || receipt.ReasonCode != wantReason || len(receipt.Owners) != 0 || len(receipt.RetainedPlanIDs) != 0 {
				t.Fatalf("empty/proof-only distinction changed for %s: %+v", mode, receipt)
			}
		})
	}
}

package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/reportartifact"
	"github.com/hanchaoqun/codrax/internal/types"
)

func reportHistoryFixtureHMC260(t *testing.T) (*Orchestrator, *types.ChangePlan, *types.ChangeReport, *types.ChangeReport) {
	t.Helper()
	plan := &types.ChangePlan{ID: "plan-history", TargetPaths: []string{"app.py"}}
	old := &types.ChangeReport{PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify,
		GeneratedAt: time.Unix(100, 0), FailureSummary: "first execution failed",
		TestResults: []types.TestResult{{AssertionID: "first", Passed: false}}}
	newer := &types.ChangeReport{PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify,
		GeneratedAt: time.Unix(200, 0), Passed: true,
		TestResults: []types.TestResult{{AssertionID: "second", Passed: true}}}
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.busCtx = &types.BusContext{Mutable: types.NewMutableState("history")}
	o.reportDir = t.TempDir()
	o.busCtx.Mutable.SetChangePlan(plan)
	if err := types.WritePlanToFile(plan, filepath.Join(o.reportDir, plan.ID+".json")); err != nil {
		t.Fatal(err)
	}
	return o, plan, old, newer
}

func TestHMC260VerificationSnapshotsBindHistoryCurrentCountsAndSlices(t *testing.T) {
	o, plan, old, newer := reportHistoryFixtureHMC260(t)
	run := &types.WriteWorkflowRun{RunID: "wf-history", ActiveBatchID: "b", Batches: []types.WriteWorkflowBatch{{
		ID: "b", PlanID: plan.ID, ActiveSliceID: "s", Slices: []types.WriteWorkflowSlice{{ID: "s"}}}}}
	o.recordWorkflowVerification(run, old, "failed", "tests_failed", true)
	first := run.Batches[0].Attempts[0]
	if !reportartifact.IsExactID(first.ReportID) || first.VerificationResults == nil {
		t.Fatalf("unpublished report bound: %+v", first)
	}
	firstBytes, err := os.ReadFile(filepath.Join(o.reportDir, first.ReportID))
	if err != nil {
		t.Fatal(err)
	}
	o.saveChangeReport(old) // duplicate publication is not a new execution
	o.recordWorkflowVerification(run, newer, "passed", "tests_passed", true)
	batch := run.Batches[0]
	second := batch.Attempts[1]
	if first.ReportID == second.ReportID || len(batch.Attempts) != 2 || batch.VerifyRef != second.ReportID ||
		batch.Slices[0].VerifyRef != second.ReportID || batch.Slices[0].Attempts[0].ReportID != first.ReportID {
		t.Fatalf("version/current/slice identities disagree: %+v", batch)
	}
	retained, _ := os.ReadFile(filepath.Join(o.reportDir, first.ReportID))
	if string(retained) != string(firstBytes) {
		t.Fatal("later report or duplicate save changed original")
	}
	if got, reason := o.loadWorkflowAttemptReport(first); got == nil || got.Passed || reason != "" {
		t.Fatalf("old failure could not be restored: %+v %s", got, reason)
	}
	counts := types.CurrentWorkflowVerificationResults(batch)
	if counts == nil || counts.Results.Total != 1 || counts.Results.Passed != 1 || counts.ReportID != second.ReportID {
		t.Fatalf("history executions were summed as current results: %+v", counts)
	}
	newer.TestResults = append(newer.TestResults, types.TestResult{AssertionID: "late mutation", Passed: true})
	if got, _ := o.loadWorkflowAttemptReport(second); got == nil || len(got.TestResults) != 1 {
		t.Fatal("mutation after publication changed frozen source")
	}
}

func TestHMC260ReportHistorySurvivesCanonicalRecoveryAndFinalProjection(t *testing.T) {
	o, plan, old, newer := reportHistoryFixtureHMC260(t)
	canonical := t.TempDir()
	o.writeWorkflowRunStore = repl.NewWriteWorkflowRunStore(canonical)
	run := &types.WriteWorkflowRun{RunID: "wf-final", Status: types.WriteWorkflowRunComplete, ActiveBatchID: "b",
		Batches: []types.WriteWorkflowBatch{{ID: "b", PlanID: plan.ID, Status: types.WriteWorkflowBatchComplete}}}
	o.recordWorkflowVerification(run, old, "failed", "tests_failed", false)
	o.recordWorkflowVerification(run, newer, "passed", "tests_passed", false)
	selected := run.Batches[0].VerifyRef
	o.busCtx.Mutable.SetChangeReport(old) // same-plan stale Mutable must not win
	o.persistWriteWorkflowRun(run)
	final, err := types.LoadWriteFinalReportFromFile(filepath.Join(canonical, plan.ID+".final.json"))
	if err != nil {
		t.Fatal(err)
	}
	if final.Verification.ReportID != selected || final.Artifacts.ReportPath != filepath.Join(canonical, selected) || !final.Verification.Passed {
		t.Fatalf("final explanation points to wrong generation: %+v %+v", final.Artifacts, final.Verification)
	}
	// New process sees only the durable root, with the turn-local mirror absent.
	restored := New(types.PipelineSettings{}, nil, nil, nil)
	restored.busCtx = &types.BusContext{Mutable: types.NewMutableState("resume")}
	restored.reportDir = canonical
	got, reason := restored.loadWorkflowAttemptReport(run.Batches[0].Attempts[0])
	if got == nil || got.Passed || reason != "" {
		t.Fatalf("canonical original lost across recovery: %+v %s", got, reason)
	}
}

func TestHMC260UnavailablePublicationDoesNotBorrowPreviousReference(t *testing.T) {
	o, plan, _, newer := reportHistoryFixtureHMC260(t)
	blocked := filepath.Join(o.reportDir, "not-a-directory")
	if err := os.WriteFile(blocked, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	o.reportDir = blocked
	run := &types.WriteWorkflowRun{ActiveBatchID: "b", Batches: []types.WriteWorkflowBatch{{ID: "b", PlanID: plan.ID, VerifyRef: "old.report.json"}}}
	o.recordWorkflowVerification(run, newer, "passed", "tests_passed", false)
	a := run.Batches[0].Attempts[0]
	if !run.PersistenceDegraded || run.Batches[0].VerifyRef != "" || a.ReportID != "" || a.Status != "passed" || a.VerificationResults != nil {
		t.Fatalf("publication failure fabricated reference or changed execution: %+v %+v", run, a)
	}
}

func TestHMC260LegacyHistoryRequiresItsBoundCanonicalSnapshot(t *testing.T) {
	o, plan, old, newer := reportHistoryFixtureHMC260(t)
	ref := plan.ID + ".report.json"
	if err := types.WriteChangeReportToFile(old, filepath.Join(o.reportDir, ref)); err != nil {
		t.Fatal(err)
	}
	a := types.WriteWorkflowAttempt{ID: "old", Kind: "verify", PlanID: plan.ID, ReportID: ref, ArtifactRef: ref}
	if got, reason := o.loadWorkflowAttemptReport(a); got != nil || reason != "durable_legacy_history_unverified" {
		t.Fatal("legacy current view silently became historical evidence")
	}
	if got := o.loadLegacyCurrentReport(plan.ID); got == nil {
		t.Fatal("explicit legacy current view became unreadable")
	}
	a.VerificationResults = types.BindWriteWorkflowVerificationResults(plan.ID, a, old)
	if got, _ := o.loadWorkflowAttemptReport(a); got == nil || got.Passed {
		t.Fatal("matching legacy report was not recovered")
	}
	if err := types.WriteChangeReportToFile(newer, filepath.Join(o.reportDir, ref)); err != nil {
		t.Fatal(err)
	}
	if got, _ := o.loadWorkflowAttemptReport(a); got != nil {
		t.Fatal("legacy digest mismatch borrowed replacement success")
	}
	for _, ref := range []string{"../" + ref, filepath.Join(o.reportDir, ref)} {
		a.ReportID = ref
		if got, _ := o.loadWorkflowAttemptReport(a); got != nil {
			t.Fatal("report traversal accepted")
		}
	}
}

func TestHMC260CurrentReportUsesOwningBatchSelectionNotLatestListPosition(t *testing.T) {
	o, plan, old, newer := reportHistoryFixtureHMC260(t)
	run := &types.WriteWorkflowRun{RunID: "wf-selected", ActiveBatchID: "active", Batches: []types.WriteWorkflowBatch{{ID: "active", PlanID: plan.ID}}}
	o.recordWorkflowVerification(run, old, "failed", "tests_failed", false)
	selected := run.Batches[0].VerifyRef
	o.recordWorkflowVerification(run, newer, "passed", "tests_passed", false)
	latest := run.Batches[0].VerifyRef
	// An explicit current selection may point to retained earlier material.
	// This is not a new execution and does not grant its completion status.
	run.Batches[0].VerifyRef = selected
	run.Batches = append(run.Batches, types.WriteWorkflowBatch{ID: "later", PlanID: plan.ID, VerifyRef: latest})
	run.Batches[0].Status = types.WriteWorkflowBatchComplete
	proof := o.writeFinalReportProofArtifacts(run, plan, newer)
	if len(proof) != 1 || proof[0].Report == nil || proof[0].Report.Passed {
		t.Fatal("cumulative proof replaced explicitly selected failure with newer success")
	}
	got, ref := o.workflowReportSource(run, plan.ID, newer)
	if ref != selected || got == nil || got.Passed || len(run.Batches[0].Attempts) != 2 {
		t.Fatalf("current selected material replaced by later list entry: %s %+v", ref, got)
	}
	if types.CurrentWorkflowVerificationResults(run.Batches[0]) != nil {
		t.Fatal("display selection manufactured current execution counts")
	}
	if err := os.Remove(filepath.Join(o.reportDir, selected)); err != nil {
		t.Fatal(err)
	}
	got, ref = o.workflowReportSource(run, plan.ID, newer)
	if got != nil || ref != selected {
		t.Fatal("missing selected version fell back to newer same-plan report")
	}
}

func TestHMC260RecoveryUsesFailedAttemptEvenWhenCurrentViewChanged(t *testing.T) {
	o, plan, old, newer := reportHistoryFixtureHMC260(t)
	run := &types.WriteWorkflowRun{RunID: "wf-resume", ActiveBatchID: "b", Status: types.WriteWorkflowRunInProgress,
		ProgressLedger: []types.WriteWorkflowProgress{{ReasonCode: "workflow_resumed"}},
		Batches:        []types.WriteWorkflowBatch{{ID: "b", PlanID: plan.ID, Status: types.WriteWorkflowBatchReadyToPlan}}}
	o.recordWorkflowVerification(run, old, "failed", "tests_failed", false)
	o.saveChangeReport(newer)
	if err := o.hydrateResumedWorkflowState(run, map[string]int{}); err != nil {
		t.Fatal(err)
	}
	handoff := o.busCtx.Mutable.VerifyFailureHandoff()
	if handoff == nil || handoff.FailureSummary != old.FailureSummary || len(handoff.FailingTests) != 1 {
		t.Fatalf("recovery lost old failure to latest success: %+v", handoff)
	}
	// Exact reference belongs to another plan: same payload never crosses scope.
	a := run.Batches[0].Attempts[0]
	a.PlanID = "different-plan"
	if got, _ := o.loadWorkflowAttemptReport(a); got != nil {
		t.Fatal("exact artifact bypassed expected plan identity")
	}
}

func TestHMC260RejectedLegacyGenerationNeverBecomesFinalDeliveryLink(t *testing.T) {
	o, plan, old, newer := reportHistoryFixtureHMC260(t)
	ref := plan.ID + ".report.json"
	if err := types.WriteChangeReportToFile(old, filepath.Join(o.reportDir, ref)); err != nil {
		t.Fatal(err)
	}
	a := types.WriteWorkflowAttempt{ID: "old", Kind: "verify", Status: "failed", PlanID: plan.ID, ReportID: ref, ArtifactRef: ref}
	a.VerificationResults = types.BindWriteWorkflowVerificationResults(plan.ID, a, old)
	run := &types.WriteWorkflowRun{RunID: "wf-legacy", Status: types.WriteWorkflowRunComplete, ActiveBatchID: "b",
		Batches: []types.WriteWorkflowBatch{{ID: "b", Status: types.WriteWorkflowBatchComplete, PlanID: plan.ID,
			VerifyRef: ref, Attempts: []types.WriteWorkflowAttempt{a}}}}
	if err := types.WriteChangeReportToFile(newer, filepath.Join(o.reportDir, ref)); err != nil {
		t.Fatal(err)
	}
	o.busCtx.Mutable.SetChangeReport(newer)
	o.persistWriteWorkflowRun(run)
	final, err := types.LoadWriteFinalReportFromFile(filepath.Join(o.reportDir, plan.ID+".final.json"))
	if err != nil {
		t.Fatal(err)
	}
	if final.Artifacts.ReportPath != "" || final.Verification.Passed || final.Verification.ReportID != ref {
		t.Fatalf("rejected report came back through final delivery path: %+v %+v", final.Artifacts, final.Verification)
	}
	if !writeControllerFinalReportHasRisk(final, "verification_report_original_unavailable") {
		t.Fatal("missing original was not disclosed")
	}
}

func TestHMC260EquivalentCurrentReportRetainsPrivateRestrictions(t *testing.T) {
	o, plan, _, current := reportHistoryFixtureHMC260(t)
	got, ref := o.workflowReportSource(nil, plan.ID, current)
	if got != current || !reportartifact.IsExactID(ref) {
		t.Fatal("no-attempt current report lost non-persistent restrictions")
	}
	run := &types.WriteWorkflowRun{ActiveBatchID: "b", Batches: []types.WriteWorkflowBatch{{ID: "b", PlanID: plan.ID}}}
	o.recordWorkflowVerification(run, current, "passed", "tests_passed", false)
	got, _ = o.workflowReportSource(run, plan.ID, current)
	if got != current {
		t.Fatal("canonical-equal current report lost non-persistent restrictions")
	}
	current.TestResults[0].AssertionID = "mutated"
	got, _ = o.workflowReportSource(run, plan.ID, current)
	if got == current || got == nil || got.TestResults[0].AssertionID == "mutated" {
		t.Fatal("same-plan mutation was reused without full snapshot match")
	}
}

// Publicly persisted attempt identity must win over an unrelated in-memory
// report with the same PlanID. The old file can be absent: no latest fallback.
func TestHMC260ProofSelectionDoesNotReplaceSelectedReportWithSamePlan(t *testing.T) {
	o, plan, old, newer := reportHistoryFixtureHMC260(t)
	const oldID = "first.report.json"
	if err := types.WriteChangeReportToFile(old, filepath.Join(o.reportDir, oldID)); err != nil {
		t.Fatal(err)
	}
	attempt := types.WriteWorkflowAttempt{ID: "a1", Kind: "verify", Status: "failed", PlanID: plan.ID, ReportID: oldID, ArtifactRef: oldID}
	attempt.VerificationResults = types.BindWriteWorkflowVerificationResults(plan.ID, attempt, old)
	run := &types.WriteWorkflowRun{Batches: []types.WriteWorkflowBatch{{ID: "b", PlanID: plan.ID,
		Status: types.WriteWorkflowBatchComplete, VerifyRef: oldID, Attempts: []types.WriteWorkflowAttempt{attempt}}}}
	got := o.writeFinalReportProofArtifacts(run, plan, newer)
	if len(got) != 1 || got[0].Report == nil || got[0].Report.Passed || got[0].Report.TestResults[0].AssertionID != "first" {
		t.Fatalf("selected historical report replaced by same-plan current report: %+v", got)
	}
	if err := os.Remove(filepath.Join(o.reportDir, oldID)); err != nil {
		t.Fatal(err)
	}
	o.saveChangeReport(newer)
	got = o.writeFinalReportProofArtifacts(run, plan, newer)
	if len(got) != 1 || got[0].Report != nil {
		t.Fatalf("missing selected payload borrowed current success: %+v", got)
	}
}

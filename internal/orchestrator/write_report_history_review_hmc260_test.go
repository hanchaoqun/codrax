package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/repl"
	"github.com/hanchaoqun/codrax/internal/reportartifact"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC260ReviewOwningBatchSelectionPrecedesOlderCrossBatchHistory(t *testing.T) {
	o, plan, old, newer := reportHistoryFixtureHMC260(t)
	oldReceipt := o.publishChangeReportSnapshot(old)
	newReceipt := o.publishChangeReportSnapshot(newer)
	if oldReceipt == nil || newReceipt == nil {
		t.Fatal("publish premise failed")
	}
	run := &types.WriteWorkflowRun{ActiveBatchID: "owner", Batches: []types.WriteWorkflowBatch{
		{ID: "earlier", PlanID: "another-plan", Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{{Kind: "verify", PlanID: plan.ID, ReportID: oldReceipt.ReportID}}},
		{ID: "owner", PlanID: plan.ID, Status: types.WriteWorkflowBatchComplete, VerifyRef: newReceipt.ReportID,
			Attempts: []types.WriteWorkflowAttempt{{Kind: "verify", PlanID: plan.ID, ReportID: newReceipt.ReportID}}},
	}}
	assertSelected := func(wantAvailable bool) {
		t.Helper()
		proof := o.writeFinalReportProofArtifacts(run, plan, newer)
		if len(proof) != 1 || proof[0].Plan != plan {
			t.Fatalf("owning plan disappeared: %+v", proof)
		}
		if wantAvailable {
			if proof[0].Report != newer || !proof[0].Report.Passed {
				t.Fatal("earlier batch history replaced the current owning selection")
			}
		} else if proof[0].Report != nil {
			t.Fatal("missing owning selection fell back to earlier batch history")
		}
	}
	assertSelected(true)
	if err := os.Remove(filepath.Join(o.reportDir, newReceipt.ReportID)); err != nil {
		t.Fatal(err)
	}
	assertSelected(false)
}

func TestHMC260ReviewSkippedVerificationClearsCurrentRefsWithoutErasingHistory(t *testing.T) {
	o, plan, _, current := reportHistoryFixtureHMC260(t)
	run := &types.WriteWorkflowRun{ActiveBatchID: "b", Batches: []types.WriteWorkflowBatch{{
		ID: "b", PlanID: plan.ID, ActiveSliceID: "s", Slices: []types.WriteWorkflowSlice{{ID: "s"}},
	}}}
	o.recordWorkflowVerification(run, current, "passed", "tests_passed", true)
	oldRef := run.Batches[0].VerifyRef
	if !reportartifact.IsExactID(oldRef) || run.Batches[0].Slices[0].ObserveRef == "" {
		t.Fatal("previous verification premise not installed")
	}
	updateWorkflowRunBatchVerifySkipped(run, "b", plan.ID)
	updateWorkflowRunActiveSliceObserveSkipped(run, "b", plan.ID)
	batch, slice := run.Batches[0], run.Batches[0].Slices[0]
	if batch.VerifyRef != "" || slice.VerifyRef != "" || slice.ObserveRef != "" {
		t.Fatal("a skip retained prior verification as its current report")
	}
	if len(batch.Attempts) != 2 || len(slice.Attempts) != 2 || batch.Attempts[0].ReportID != oldRef || slice.Attempts[0].ReportID != oldRef {
		t.Fatal("clearing current state erased immutable execution history")
	}
	selected, ok := selectedWorkflowReportAttempt(run, plan.ID)
	if !ok || selected.Status != "skipped" || selected.ReportID != "" || types.CurrentWorkflowVerificationResults(batch) != nil {
		t.Fatalf("current skip was replaced by previous counts/status: %+v", selected)
	}
	if report, ref := o.workflowReportSource(run, plan.ID, current); report != nil || ref != "" {
		t.Fatal("final explanation borrowed previous same-plan success after skip")
	}
	if report, _ := o.loadWorkflowAttemptReport(batch.Attempts[0]); report == nil || !report.Passed {
		t.Fatal("historical execution could no longer be read independently")
	}
}

func TestHMC260ReviewOwnerLifecycleNeverBorrowsAnotherBatchCompletion(t *testing.T) {
	o, plan, old, newer := reportHistoryFixtureHMC260(t)
	oldReceipt := o.publishChangeReportSnapshot(old)
	newReceipt := o.publishChangeReportSnapshot(newer)
	if oldReceipt == nil || newReceipt == nil {
		t.Fatal("publish premise failed")
	}
	missing := *newer
	missing.FailureSummary = "missing selected original"
	missingReceipt := o.publishChangeReportSnapshot(&missing)
	if missingReceipt == nil {
		t.Fatal("publish missing-original premise failed")
	}
	if err := os.Remove(filepath.Join(o.reportDir, missingReceipt.ReportID)); err != nil {
		t.Fatal(err)
	}
	for _, complete := range []bool{false, true} {
		for _, selection := range []string{"available", "missing_original", "unselected"} {
			for _, primary := range []string{"present", "absent"} {
				state := "incomplete"
				if complete {
					state = "complete"
				}
				t.Run(state+"/"+selection+"/primary_"+primary, func(t *testing.T) {
					owner := types.WriteWorkflowBatch{ID: "active", PlanID: plan.ID, Status: types.WriteWorkflowBatchApplying}
					if complete {
						owner.Status = types.WriteWorkflowBatchComplete
					}
					if selection != "unselected" {
						owner.VerifyRef = newReceipt.ReportID
						if selection == "missing_original" {
							owner.VerifyRef = missingReceipt.ReportID
						}
						owner.Attempts = []types.WriteWorkflowAttempt{{Kind: "verify", PlanID: plan.ID, ReportID: owner.VerifyRef}}
					}
					run := &types.WriteWorkflowRun{ActiveBatchID: "active", Batches: []types.WriteWorkflowBatch{
						{ID: "completed", PlanID: plan.ID, Status: types.WriteWorkflowBatchComplete, VerifyRef: oldReceipt.ReportID,
							Attempts: []types.WriteWorkflowAttempt{{Kind: "verify", PlanID: plan.ID, ReportID: oldReceipt.ReportID}}}, owner,
					}}
					var current *types.ChangeReport
					if primary == "present" {
						current = newer
					}
					proof := o.writeFinalReportProofArtifacts(run, plan, current)
					if !complete || selection == "unselected" && current != nil {
						if len(proof) != 0 {
							t.Fatalf("owner borrowed completion or duplicated current primary: %+v", proof)
						}
						return
					}
					if len(proof) != 1 || proof[0].Plan != plan {
						t.Fatalf("owning plan's proof/unknown obligation disappeared: %+v", proof)
					}
					if selection == "available" {
						if proof[0].Report == nil || !proof[0].Report.Passed || proof[0].Report.TestResults[0].AssertionID != "second" {
							t.Fatal("owning selection was replaced by another batch's report")
						}
					} else if proof[0].Report != nil {
						t.Fatal("unknown selected material borrowed a different generation")
					}
				})
			}
		}
	}
}

func TestHMC260ReviewLegacyDeliveryPathMatchesExpectedGenerationAcrossRoots(t *testing.T) {
	o, plan, old, newer := reportHistoryFixtureHMC260(t)
	canonical := t.TempDir()
	o.writeWorkflowRunStore = repl.NewWriteWorkflowRunStore(canonical)
	ref := plan.ID + ".report.json"
	if err := types.WriteChangeReportToFile(newer, filepath.Join(canonical, ref)); err != nil {
		t.Fatal(err)
	}
	if err := types.WriteChangeReportToFile(old, filepath.Join(o.reportDir, ref)); err != nil {
		t.Fatal(err)
	}
	if got := o.reportArtifactPath(ref, plan.ID, old); got != filepath.Join(o.reportDir, ref) {
		t.Fatalf("same filename in preferred root replaced selected generation: %q", got)
	}
	if got := o.reportArtifactPath(ref, plan.ID, newer); got != filepath.Join(canonical, ref) {
		t.Fatalf("matching canonical original became unavailable: %q", got)
	}
	if err := os.Remove(filepath.Join(o.reportDir, ref)); err != nil {
		t.Fatal(err)
	}
	if got := o.reportArtifactPath(ref, plan.ID, old); got != "" {
		t.Fatalf("missing selected generation linked a different legacy generation: %q", got)
	}
	if got := o.reportArtifactPath(ref, plan.ID, nil); got != "" {
		t.Fatal("unverified generation minted a delivery link")
	}
}

func TestHMC260ReviewPublisherNormalizesPlanIdentityBeforeFreezing(t *testing.T) {
	o, plan, _, current := reportHistoryFixtureHMC260(t)
	current.PlanID = " \t" + plan.ID + "\n"
	run := &types.WriteWorkflowRun{ActiveBatchID: "b", Batches: []types.WriteWorkflowBatch{{ID: "b", PlanID: plan.ID}}}
	o.recordWorkflowVerification(run, current, "passed", "tests_passed", false)
	attempt := run.Batches[0].Attempts[0]
	if current.PlanID != plan.ID || attempt.PlanID != plan.ID || !reportartifact.IsExactID(attempt.ReportID) {
		t.Fatalf("producer identity disagrees with frozen artifact: %+v", attempt)
	}
	loaded, reason := o.loadWorkflowAttemptReport(attempt)
	if loaded == nil || loaded.PlanID != plan.ID || reason != "" {
		t.Fatalf("producer published a self-unreadable original: %+v %s", loaded, reason)
	}
	firstRef := attempt.ReportID
	o.saveChangeReport(current)
	if !reportartifact.IsExactID(firstRef) {
		t.Fatal("repeat publication changed artifact identity")
	}
	if second := o.publishChangeReportSnapshot(current); second == nil || second.ReportID != firstRef {
		t.Fatal("normalization was repeated inconsistently")
	}
}

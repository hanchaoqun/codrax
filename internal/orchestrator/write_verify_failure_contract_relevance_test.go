package orchestrator

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// write_verify_failure_contract_relevance_test.go — V5-3 pin (colleague_merge_audit
// §40.23): the handoff built at every verify-failure site is finalized while
// the failed plan is still live on Mutable, so ContractRelevance is the typed
// join of the report's failed rows with THAT plan's declared refs; a
// report-less carrier stays unavailable.

func TestFinalizeVerifyFailureHandoffStampsContractRelevanceFromLivePlan(t *testing.T) {
	mu := types.NewMutableState("repair")
	mu.SetChangePlan(&types.ChangePlan{
		ID:                 "plan-1",
		VerificationProbes: []types.VerificationProbe{{ID: "shape_probe", ContractRefs: []string{"stale-soft"}}},
		ProjectTestObservations: []types.ProjectTestObservation{
			{ID: "o1", TestPath: "tests/widget_test.py", AssertionSuite: "widget_test", AssertionID: "test_value", ContractRefs: []string{"exact-value"}},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply}}
	report := &types.ChangeReport{PlanID: "plan-1", Passed: false, FailureKind: types.FailureKindTestsFailed, TestResults: []types.TestResult{
		{Suite: "verification_probe/python", AssertionID: "shape_probe", Passed: false},
		{Suite: "tests.widget_test", AssertionID: "test_value", ObservationScope: types.TestObservationScopeAssertion, Passed: false},
		{Suite: "tests.widget_test", AssertionID: "test_other", ObservationScope: types.TestObservationScopeAssertion, Passed: false},
	}}
	// EVOLUTION RECORD: names alone were formerly sufficient to retire a
	// project contract. Include the exact failed execution and path roster so
	// this pin exercises the production resolver rather than a suffix guess.
	report.TestSurface = &types.TestSurface{Candidates: []types.TestSurfaceCandidate{{
		Runner: "python", Framework: "unittest", WorkingDir: ".", HasTestSignal: true,
	}}}
	report.ExecutedCommands = []types.ExecutedCommand{{
		Runner: "python", Framework: "unittest", WorkingDir: ".", Suite: "tests/widget_test.py",
		Outcome: types.ExecutedCommandOutcomeExecuted, ExitCode: 1,
	}}

	handoff := o.finalizeVerifyFailureHandoff(types.BuildVerifyFailureHandoff(report, "batch-1", 1, "", ""), report)
	if handoff == nil || handoff.ContractRelevance == nil || handoff.ContractRelevance.Status != types.VerifyFailureContractRelevanceAvailable {
		t.Fatalf("relevance not stamped from the live plan: %+v", handoff)
	}
	want := []types.VerifyFailureContractHit{
		{ContractID: "exact-value", Reason: types.WriteBehaviorContractRetiredFailedProjectTestAssertion, EvidenceRefs: []string{"assertion:tests/widget_test.py::widget_test::test_value"}},
		{ContractID: "stale-soft", Reason: types.WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:shape_probe"}},
	}
	if !reflect.DeepEqual(handoff.ContractRelevance.Hits, want) {
		t.Fatalf("hits = %+v, want %+v", handoff.ContractRelevance.Hits, want)
	}
	// Removing runner authority keeps direct probes but cannot retire a
	// project expectation. This also pins the scheduler's pure-helper wiring.
	report.TestSurface = nil
	unbound := o.finalizeVerifyFailureHandoff(types.BuildVerifyFailureHandoff(report, "batch-1", 1, "", ""), report)
	if !reflect.DeepEqual(unbound.ContractRelevance.Hits, want[1:]) {
		t.Fatalf("unbound project row escaped the scheduler resolver: %+v", unbound.ContractRelevance.Hits)
	}

	// Report-less resume carrier: unavailable, authorizes nothing.
	bare := o.finalizeVerifyFailureHandoff(types.BuildVerifyFailureHandoffWithoutReport("plan-1", "batch-1", 1, "tests_failed", "", "", "durable_report_unavailable"), nil)
	if bare.ContractRelevance == nil || bare.ContractRelevance.Available() {
		t.Fatalf("report-less carrier must be unavailable: %+v", bare.ContractRelevance)
	}
	// Plan/report mismatch (a stale Mutable plan) is unavailable too.
	mu.SetChangePlan(&types.ChangePlan{ID: "plan-other"})
	mismatch := o.finalizeVerifyFailureHandoff(types.BuildVerifyFailureHandoff(report, "batch-1", 1, "", ""), report)
	if mismatch.ContractRelevance.Available() || mismatch.ContractRelevance.ReasonCode != "plan_report_mismatch" {
		t.Fatalf("mismatched plan must be unavailable: %+v", mismatch.ContractRelevance)
	}
}

func TestStampCumulativeVerificationScopeHonoursTypedTombstoneCarrier(t *testing.T) {
	retained := &types.ChangePlan{
		ID:                "plan-old",
		TargetPaths:       []string{"old.c"},
		Changes:           []types.FileChange{{Path: "old.c", Kind: "patch"}},
		BehaviorContracts: []types.WriteBehaviorContract{{ID: "retired-soft", Expected: "the failed shape"}, {ID: "retained-only", Expected: "still valid"}},
	}
	active := &types.ChangePlan{
		ID:                         "plan-new",
		TargetPaths:                []string{"new.c"},
		Changes:                    []types.FileChange{{Path: "new.c", Kind: "patch"}},
		BehaviorContractGeneration: types.WriteBehaviorContractGenerationPlanAcceptanceRebase,
		// Typed carrier only — no legacy id list.
		SupersededBehaviorContracts: []types.WriteBehaviorContractTombstone{{ID: "retired-soft", Reason: types.WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:p1"}}},
	}
	run := &types.WriteWorkflowRun{Batches: []types.WriteWorkflowBatch{{ID: "batch-1", Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: retained.ID}}}}}
	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.stampCumulativeVerificationScope(active, run, retained)
	if active.CumulativeVerificationScope == nil {
		t.Fatal("expected retained verification scope")
	}
	if ids := behaviorContractIDs(active.CumulativeVerificationScope.BehaviorContracts); !reflect.DeepEqual(ids, []string{"retained-only"}) {
		t.Fatalf("typed tombstone must shadow the retired id without a legacy id list: %v", ids)
	}
}

// ---- §40.46 fold-in: the scheduler's handoff replacement merges the failed
// plan's tombstones into the run ledger, the persisted run envelope carries
// the ledger, and cumulative-scope stamping shadows ledger ids.

func TestFinalizeVerifyFailureHandoffMergesLivePlanTombstonesIntoLedger(t *testing.T) {
	mu := types.NewMutableState("repair")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply}}
	plan2 := &types.ChangePlan{ID: "plan-2", BehaviorContractGeneration: types.WriteBehaviorContractGenerationPlanAcceptanceRebase,
		SupersededBehaviorContracts: []types.WriteBehaviorContractTombstone{{ID: "stale-soft", Reason: types.WriteBehaviorContractRetiredFailedVerificationProbe, EvidenceRefs: []string{"probe:shape_probe"}, PlanID: "plan-1", Attempt: 1}}}
	mu.SetChangePlan(plan2)
	mu.ResetBehaviorContractTombstoneLedger() // isolate the finalize lane from the SetChangePlan lane
	report := &types.ChangeReport{PlanID: "plan-2", Passed: false, FailureKind: types.FailureKindBuildFailure}
	handoff := o.finalizeVerifyFailureHandoff(types.BuildVerifyFailureHandoff(report, "batch-1", 2, "", ""), report)
	mu.SetVerifyFailureHandoff(handoff)
	rows := mu.BehaviorContractTombstoneLedger()
	if len(rows) != 1 || rows[0].ID != "stale-soft" || rows[0].PlanID != "plan-1" {
		t.Fatalf("finalize did not merge the failed plan's tombstones: %+v", rows)
	}
	// The persisted envelope carries the ledger (no store: degraded, but the
	// run pointer is stamped), and installing that run seeds a fresh state.
	run := &types.WriteWorkflowRun{RunID: "run-1"}
	o.persistWriteWorkflowRun(run)
	if len(run.BehaviorContractTombstones) != 1 || run.BehaviorContractTombstones[0].ID != "stale-soft" {
		t.Fatalf("persisted run lacks the ledger: %+v", run.BehaviorContractTombstones)
	}
	fresh := types.NewMutableState("resume")
	fresh.SetWriteWorkflowRun(run)
	if rows := fresh.BehaviorContractTombstoneLedger(); len(rows) != 1 || rows[0].ID != "stale-soft" {
		t.Fatalf("resumed state did not seed from the envelope: %+v", rows)
	}
}

func TestStampCumulativeVerificationScopeShadowsLedgerIDs(t *testing.T) {
	retained := &types.ChangePlan{ID: "plan-old", TargetPaths: []string{"old.c"},
		BehaviorContracts: []types.WriteBehaviorContract{{ID: "stale-soft", Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpSatisfies, Expected: "old shape", Required: true, Source: "write_analyzer"}}}
	active := &types.ChangePlan{ID: "plan-new", TargetPaths: []string{"new.c"}}
	mu := types.NewMutableState("repair")
	mu.MergeBehaviorContractTombstones(types.WriteBehaviorContractTombstone{ID: "stale-soft", Reason: types.WriteBehaviorContractRetiredFailedVerificationProbe})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply}}
	run := &types.WriteWorkflowRun{RunID: "run-1", ActiveBatchID: "batch-1", Batches: []types.WriteWorkflowBatch{{ID: "batch-1", Status: types.WriteWorkflowBatchApplying}}}
	o.stampCumulativeVerificationScope(active, run, retained)
	for _, contract := range types.ChangePlanVerificationBehaviorContracts(active) {
		if contract.ID == "stale-soft" {
			t.Fatalf("ledger-retired id re-entered the cumulative scope from a retained plan: %+v", active.CumulativeVerificationScope)
		}
	}
}

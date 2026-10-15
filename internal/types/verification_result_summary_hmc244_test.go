package types

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func verificationSnapshotBatchHMC244(t *testing.T) WriteWorkflowBatch {
	t.Helper()
	attempt := WriteWorkflowAttempt{ID: "attempt-2", Kind: "verify", PlanID: "plan", ReportID: "plan.report.json", ArtifactRef: "plan.report.json", Status: "passed"}
	report := &ChangeReport{PlanID: "plan", Channel: ChangeReportChannelPostApplyVerify, Passed: true,
		TestResults: []TestResult{{Passed: true}, {Passed: true}}}
	attempt.VerificationResults = BindWriteWorkflowVerificationResults("plan", attempt, report)
	if attempt.VerificationResults == nil {
		t.Fatal("snapshot missing")
	}
	return WriteWorkflowBatch{ID: "batch", PlanID: "plan", VerifyRef: "plan.report.json", Attempts: []WriteWorkflowAttempt{attempt}}
}

func TestHMC244VerificationResultSummaryUsesTypedRows(t *testing.T) {
	if SummarizeVerificationResults(nil) != nil {
		t.Fatal("absent report invented zero")
	}
	if got := SummarizeVerificationResults(&ChangeReport{}); got == nil || got.Total != 0 {
		t.Fatal("empty report lost known record count")
	}
	report := &ChangeReport{TestResults: []TestResult{
		{Passed: true, ObservationScope: TestObservationScopeAggregate},
		{Passed: true, ObservationScope: TestObservationScopeAssertion},
		{Passed: true, ObservationScope: "future_scope"},
		{Passed: true, ObservationScope: TestObservationScopeNonAsserting},
		{Passed: false, ObservationScope: TestObservationScopeNonAsserting},
		{Passed: false, Kind: TestResultKindBuildError},
		{Passed: false, ObservationScope: TestObservationScopeAssertion},
	}}
	want := &VerificationResultSummary{Total: 7, Passed: 3, Failed: 2, NonAsserting: 2}
	if got := SummarizeVerificationResults(report); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
	for _, c := range []VerificationResultSummary{{Total: -1}, {Total: 1, Passed: 2}, {Total: math.MaxInt, Passed: math.MaxInt, Failed: math.MaxInt}} {
		if c.valid() {
			t.Fatalf("invalid/overflow counts accepted: %+v", c)
		}
	}
}

func TestHMC244VerificationSnapshotBoundToCurrentAttempt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*WriteWorkflowBatch)
	}{
		{"plan", func(b *WriteWorkflowBatch) { b.PlanID = "new-plan" }},
		{"report", func(b *WriteWorkflowBatch) { b.VerifyRef = "other.report.json" }},
		{"attempt", func(b *WriteWorkflowBatch) { b.Attempts[0].ID = "attempt-3" }},
		{"attempt_plan", func(b *WriteWorkflowBatch) { b.Attempts[0].PlanID = "new-plan" }},
		{"attempt_report", func(b *WriteWorkflowBatch) { b.Attempts[0].ReportID = "other.report.json" }},
		{"artifact", func(b *WriteWorkflowBatch) { b.Attempts[0].ArtifactRef = "other.report.json" }},
		{"invalid_digest", func(b *WriteWorkflowBatch) { b.Attempts[0].VerificationResults.ReportSHA256 = "unknown" }},
		{"invalid_count", func(b *WriteWorkflowBatch) { b.Attempts[0].VerificationResults.Results.Passed = 3 }},
		{"latest_legacy", func(b *WriteWorkflowBatch) {
			b.Attempts = append(b.Attempts, WriteWorkflowAttempt{ID: "attempt-3", Kind: "verify", PlanID: "plan", Status: "passed"})
		}},
		{"later_apply", func(b *WriteWorkflowBatch) {
			b.Attempts = append(b.Attempts, WriteWorkflowAttempt{Kind: "apply", PlanID: "plan", Status: "applied"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := verificationSnapshotBatchHMC244(t)
			tc.change(&b)
			if got := CurrentWorkflowVerificationResults(b); got != nil {
				t.Fatalf("foreign/stale snapshot accepted: %+v", got)
			}
		})
	}
	batch := verificationSnapshotBatchHMC244(t)
	before := batch.Attempts[0].VerificationResults.Results
	run := NormalizeWriteWorkflowRun(WriteWorkflowRun{RunID: "run", Batches: []WriteWorkflowBatch{batch}})
	run.Batches[0].Attempts[0].VerificationResults.Results.Passed = 0
	if batch.Attempts[0].VerificationResults.Results != before {
		t.Fatal("normalization shared mutable snapshot")
	}
	wire, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	var restored WriteWorkflowBatch
	if err := json.Unmarshal(wire, &restored); err != nil {
		t.Fatal(err)
	}
	if got := CurrentWorkflowVerificationResults(restored); got == nil || got.Results != before {
		t.Fatal("restart lost exact snapshot")
	}
}

func TestHMC244VerificationSnapshotRejectsUnboundReports(t *testing.T) {
	b := verificationSnapshotBatchHMC244(t)
	a := b.Attempts[0]
	for _, report := range []*ChangeReport{nil, {PlanID: "other", Channel: ChangeReportChannelPostApplyVerify}, {PlanID: "plan"}, {PlanID: "plan", Channel: ChangeReportChannel("reference")}} {
		if got := BindWriteWorkflowVerificationResults("plan", a, report); got != nil {
			t.Fatalf("unbound report accepted: %+v", report)
		}
	}
	if got := BindWriteWorkflowVerificationResults("", a, &ChangeReport{PlanID: "plan", Channel: ChangeReportChannelPostApplyVerify}); got != nil {
		t.Fatal("missing current plan accepted")
	}
}

func TestHMC244VerificationSnapshotOptionalLegacyJSON(t *testing.T) {
	for _, value := range []any{WriteWorkflowAttempt{ID: "a", Kind: "verify"}, WriteWorkflowProgress{Message: "legacy message"}} {
		wire, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(wire), "verification_results") || strings.Contains(string(wire), "advisory_message") {
			t.Fatalf("legacy wire changed: %s", wire)
		}
	}
}

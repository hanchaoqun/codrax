package types

import (
	"strings"
	"testing"
)

func TestBuildVerifyFailureHandoff_ProjectsTypedRows(t *testing.T) {
	report := &ChangeReport{
		PlanID:                "plan-1",
		Passed:                false,
		FailureKind:           FailureKindTestsFailed,
		FailureSummary:        "1 test failed",
		FailureSummaryBlobRef: "/tmp/blob/run.txt",
		TestResults: []TestResult{
			{AssertionID: "TestA", Suite: "pkg", Passed: false, FailureDetail: "want 2 got 3"},
			{AssertionID: "TestB", Suite: "pkg", Passed: true},
			{Kind: TestResultKindBuildError, AssertionID: "build", Passed: false,
				BuildErrors: []BuildError{{File: "src/x.c", Line: 42, Message: "expected ;"}}},
		},
		ExecutedCommands: []ExecutedCommand{
			{Runner: "make", WorkingDir: ".", Command: "make check", ExitCode: 2, Outcome: "executed"},
		},
		TestSurface: &TestSurface{Candidates: []TestSurfaceCandidate{
			{ID: "make@.", Runner: "make", WorkingDir: ".", HasTestSignal: true},
			{ID: "go@sub", Runner: "go", WorkingDir: "sub", HasTestSignal: true},
		}},
	}
	h := BuildVerifyFailureHandoff(report, "batch-1", 2, "plan-1.attempt-2.diff", "plan-1.attempt-2.surface.json")
	if h == nil {
		t.Fatal("failed report must build a handoff")
	}
	if h.PlanID != "plan-1" || h.BatchID != "batch-1" || h.Attempt != 2 {
		t.Fatalf("identity fields wrong: %+v", h)
	}
	if len(h.FailingTests) != 1 || h.FailingTests[0].AssertionID != "TestA" {
		t.Fatalf("only failed unit rows belong in FailingTests: %+v", h.FailingTests)
	}
	if len(h.BuildErrors) != 1 || h.BuildErrors[0].File != "src/x.c" {
		t.Fatalf("build errors must flatten: %+v", h.BuildErrors)
	}
	if len(h.Executed) != 1 || h.Executed[0].Command != "make check" {
		t.Fatalf("executed commands must carry over: %+v", h.Executed)
	}
	if h.DiffArtifactRef != "plan-1.attempt-2.diff" ||
		h.SurfaceArtifactRef != "plan-1.attempt-2.surface.json" ||
		h.BlobRef != "/tmp/blob/run.txt" {
		t.Fatalf("artifact refs missing: %+v", h)
	}
	// make@. was executed; go@sub is the next unexecuted candidate with work.
	if h.NextSurfaceCandidateID != "go@sub" {
		t.Fatalf("next surface candidate = %q, want go@sub", h.NextSurfaceCandidateID)
	}
	if h.GeneratedAt.IsZero() {
		t.Fatal("handoff must be timestamped")
	}
}

func TestBuildVerifyFailureHandoff_NilForPassedOrNil(t *testing.T) {
	if h := BuildVerifyFailureHandoff(nil, "b", 1, "", ""); h != nil {
		t.Fatal("nil report must not build a handoff")
	}
	if h := BuildVerifyFailureHandoff(&ChangeReport{Passed: true}, "b", 1, "", ""); h != nil {
		t.Fatal("passed report must not build a handoff")
	}
}

func TestBuildVerifyFailureHandoff_Bounds(t *testing.T) {
	report := &ChangeReport{PlanID: "p", Passed: false}
	for i := 0; i < 30; i++ {
		report.TestResults = append(report.TestResults, TestResult{
			AssertionID: strings.Repeat("t", 3), Passed: false,
		})
	}
	h := BuildVerifyFailureHandoff(report, "b", 1, "", "")
	if len(h.FailingTests) > maxHandoffFailingTests {
		t.Fatalf("failing tests must be bounded, got %d", len(h.FailingTests))
	}
}

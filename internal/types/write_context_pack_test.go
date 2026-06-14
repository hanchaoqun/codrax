package types

import (
	"fmt"
	"strings"
	"testing"
)

func TestWriteContextPackFromWriteAnalysisIRPreservesP0Constraints(t *testing.T) {
	ir := &WriteAnalysisIR{
		Request: WriteRequestModel{
			Task: WriteTask{Summary: "add approval gate"},
			Risk: WriteRiskProfile{
				AffectsPublicAPI:   true,
				ChangesBuildSystem: true,
				Overall:            RiskBandHigh,
			},
			ScopeAnchors: []string{"internal/orchestrator/stage_hooks.go"},
			Constraints: []WriteConstraint{{
				Kind:   "preserve_read_mode",
				Target: "scheduler",
				Note:   "do not change read scheduler byte identity",
			}},
			ExpectedOutcomes: []string{"plan-file apply passes approval gate"},
		},
		RepoFacts: WriteRepoFacts{TestRunner: "go"},
	}
	pack := WriteContextPackFromWriteAnalysisIR(ir)
	view := pack.View(WriteConsumerPlanner, 10)
	if len(view.Items) == 0 {
		t.Fatal("expected planner items")
	}
	if view.Items[0].Priority != WriteContextP0 {
		t.Fatalf("first item priority = %s, want p0; items=%+v", view.Items[0].Priority, view.Items)
	}
	if !writeContextViewContains(view, "constraint", "do not change read scheduler") {
		t.Fatalf("constraint missing from view: %+v", view.Items)
	}
	if !writeContextViewContains(view, "risk_profile", "changes_build_system=true") {
		t.Fatalf("risk profile missing from view: %+v", view.Items)
	}
}

func TestWriteContextPackFromExplorationHandoffPrioritizesEvidence(t *testing.T) {
	handoff := WriteExplorationHandoff{
		BatchID:          "batch-1",
		Goal:             "thread handoff into planner",
		TargetFiles:      []string{"internal/agent/planner.go"},
		RelevantSymbols:  []string{"BuildInitialInstruction"},
		ExistingPatterns: []string{"planner sections are pure data"},
		Invariants:       []string{"do not bypass ChangePlan validation"},
		TestSurface:      []string{"go test ./internal/agent"},
		RiskNotes:        []string{"prompt ordering can hide approval constraints"},
		Unknowns:         []string{"which planner view should render first"},
		EvidenceRefs: []WriteExplorationEvidenceRef{{
			ID:        "ev1",
			Kind:      "mechanism",
			Source:    "internal/agent/planner.go",
			LineStart: 105,
			Subject:   "BuildInitialInstruction",
			Summary:   "composes planner sections",
		}},
	}
	pack := WriteContextPackFromExplorationHandoff(handoff)
	planner := pack.View(WriteConsumerPlanner, 20)
	verifier := pack.View(WriteConsumerVerifier, 20)
	if !writeContextViewContains(planner, "risk_note", "prompt ordering") {
		t.Fatalf("risk note missing from planner view: %+v", planner.Items)
	}
	if !writeContextViewContains(planner, "evidence_ref", "BuildInitialInstruction @ internal/agent/planner.go:105") {
		t.Fatalf("evidence ref missing from planner view: %+v", planner.Items)
	}
	if !writeContextViewContains(planner, "pattern_hint", "planner sections") {
		t.Fatalf("pattern hint missing from planner view: %+v", planner.Items)
	}
	if writeContextViewContains(verifier, "pattern_hint", "planner sections") {
		t.Fatalf("verifier view should not receive planner-only pattern hints: %+v", verifier.Items)
	}
	if !writeContextViewContains(verifier, "test_surface", "go test ./internal/agent") {
		t.Fatalf("verifier test surface missing: %+v", verifier.Items)
	}
}

func TestWriteContextPackFromChangeReportCarriesVerifyFailure(t *testing.T) {
	report := &ChangeReport{
		PlanID:                "plan-1",
		Passed:                false,
		BuildFailed:           true,
		FailureSummary:        "compile failed",
		FailureSummaryBlobRef: "/tmp/codrax/blob/run-tests-plan-1.txt",
		TestResults: []TestResult{{
			Kind:          TestResultKindBuildError,
			AssertionID:   "internal/foo.go:10",
			Suite:         "build",
			FailureDetail: "undefined: Foo",
			BuildErrors: []BuildError{{
				File:    "internal/foo.go",
				Line:    10,
				Symbol:  "Foo",
				Message: "undefined: Foo",
			}},
		}},
		RegressionAssertions: []string{"TestExisting"},
	}
	pack := WriteContextPackFromChangeReport(report)
	view := pack.View(WriteConsumerPlanner, 10)
	for _, want := range []struct {
		kind string
		text string
	}{
		{"build_failure", "compile failed"},
		{"failure_summary_blob_ref", "/tmp/codrax/blob/run-tests-plan-1.txt"},
		{"failed_test", "undefined: Foo"},
		{"build_error", "internal/foo.go:10 Foo undefined: Foo"},
		{"regression_assertion", "TestExisting"},
	} {
		if !writeContextViewContains(view, want.kind, want.text) {
			t.Fatalf("missing %s/%q from verify context: %+v", want.kind, want.text, view.Items)
		}
	}
}

func TestWriteContextPackFromChangeReportCarriesNoTestsToController(t *testing.T) {
	report := &ChangeReport{PlanID: "plan-no-tests", Passed: true, NoTestsRunners: []string{"python"}}
	pack := WriteContextPackFromChangeReport(report)
	view := pack.View(WriteConsumerController, 10)
	if !writeContextViewContains(view, "no_tests_runner", "python") {
		t.Fatalf("controller must receive no-tests context: %+v", view.Items)
	}
}

func TestWriteContextPackFromChangeReportDedupesVerifyFailureIdentity(t *testing.T) {
	first := WriteContextPackFromChangeReport(&ChangeReport{
		PlanID:         "plan-1",
		Passed:         false,
		FailureKind:    FailureKindTestsFailed,
		FailureSummary: "first run failed",
		TestResults: []TestResult{{
			AssertionID:   "TestA",
			Suite:         "pkg",
			Passed:        false,
			FailureDetail: "want 1 got 2",
			BuildErrors: []BuildError{{
				File:    "src/x.go",
				Line:    7,
				Symbol:  "Foo",
				Message: "undefined: Foo",
			}},
		}},
		ExecutedCommands: []ExecutedCommand{{
			Runner:     "go",
			WorkingDir: ".",
			Command:    "go test ./...",
			ExitCode:   1,
			Outcome:    "executed",
		}},
	})
	second := WriteContextPackFromChangeReport(&ChangeReport{
		PlanID:         "plan-1",
		Passed:         false,
		FailureKind:    FailureKindTestsFailed,
		FailureSummary: "second run failed with the same shape",
		TestResults: []TestResult{{
			AssertionID:   "TestA",
			Suite:         "pkg",
			Passed:        false,
			FailureDetail: "want 1 got 3",
			BuildErrors: []BuildError{{
				File:    "src/x.go",
				Line:    7,
				Symbol:  "Foo",
				Message: "still undefined: Foo",
			}},
		}},
		ExecutedCommands: []ExecutedCommand{{
			Runner:     "go",
			WorkingDir: ".",
			Command:    "go test ./...",
			ExitCode:   1,
			Outcome:    "executed",
		}},
	})
	merged := MergeWriteContextPacks("batch-1", "repair", first, second)
	view := merged.View(WriteConsumerPlanner, 20)
	for _, kind := range []string{"failed_test", "build_error", "executed_command"} {
		if got := writeContextViewKindCount(view, kind); got != 1 {
			t.Fatalf("%s should dedupe by typed identity, got %d: %+v", kind, got, view.Items)
		}
	}
}

func TestWriteContextPackPlannerViewPrioritizesVerifyFailureBeforeStaleP1Facts(t *testing.T) {
	oldFacts := WriteContextPack{
		PackID:      "old-exploration",
		BatchID:     "batch-1",
		SourceStage: "explore",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "internal/old.go",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}
	failure := WriteContextPackFromChangeReport(&ChangeReport{
		PlanID:         "plan-1",
		Passed:         false,
		FailureKind:    FailureKindTestsFailed,
		FailureSummary: "red test",
		TestResults: []TestResult{{
			AssertionID: "TestA",
			Suite:       "pkg",
			Passed:      false,
		}},
	})
	view := MergeWriteContextPacks("batch-1", "repair", oldFacts, failure).View(WriteConsumerPlanner, 2)
	if len(view.Items) == 0 || view.Items[0].Kind != "verify_failure" {
		t.Fatalf("planner replan view should lead with verify failure before stale P1 facts: %+v", view.Items)
	}
}

func TestNormalizeWriteContextPackMergesDuplicateConsumerViews(t *testing.T) {
	pack := NormalizeWriteContextPack(WriteContextPack{
		PackID: "pack-1",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "internal/write.go",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "internal/write.go",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerVerifier},
		}},
	})
	if len(pack.Items) != 1 {
		t.Fatalf("duplicate typed fact should merge, got %+v", pack.Items)
	}
	if !writeContextConsumersContain(pack.Items[0].Consumers, WriteConsumerPlanner) ||
		!writeContextConsumersContain(pack.Items[0].Consumers, WriteConsumerVerifier) {
		t.Fatalf("duplicate merge should union consumers, got %+v", pack.Items[0].Consumers)
	}
	if got := len(pack.View(WriteConsumerPlanner, 10).Items); got != 1 {
		t.Fatalf("planner view should see merged fact once, got %d", got)
	}
	if got := len(pack.View(WriteConsumerVerifier, 10).Items); got != 1 {
		t.Fatalf("verifier view should see merged fact once, got %d", got)
	}
}

func TestWriteContextPackPlannerLimitedViewRetainsVerifyFailureLane(t *testing.T) {
	manyP0 := WriteContextPack{
		PackID:      "risk",
		BatchID:     "batch-1",
		SourceStage: "risk",
	}
	for i := 0; i < 8; i++ {
		manyP0.Items = append(manyP0.Items, WriteContextItem{
			Priority:    WriteContextP0,
			Kind:        "constraint",
			Text:        fmt.Sprintf("hard constraint %d", i),
			SourceStage: "risk",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		})
	}
	failure := WriteContextPackFromChangeReport(&ChangeReport{
		PlanID:         "plan-1",
		Passed:         false,
		FailureKind:    FailureKindTestsFailed,
		FailureSummary: "red test",
		TestResults: []TestResult{{
			AssertionID: "TestRepair",
			Suite:       "pkg",
			Passed:      false,
		}},
	})
	view := MergeWriteContextPacks("batch-1", "repair", manyP0, failure).View(WriteConsumerPlanner, 5)
	if len(view.Items) != 5 {
		t.Fatalf("limited view should remain bounded, got %d: %+v", len(view.Items), view.Items)
	}
	if !writeContextViewContains(view, "verify_failure", "red test") {
		t.Fatalf("planner limited view should retain typed verify failure lane: %+v", view.Items)
	}
	if got := writeContextViewPriorityCount(view, WriteContextP0); got < 3 {
		t.Fatalf("planner limited view should retain core p0 context, got %d: %+v", got, view.Items)
	}
}

func TestWriteContextPackViewBoundsAndDefensiveCopy(t *testing.T) {
	pack := WriteContextPack{PackID: "x"}
	for i := 0; i < 20; i++ {
		priority := WriteContextP3
		if i == 10 {
			priority = WriteContextP0
		}
		pack.Items = append(pack.Items, WriteContextItem{
			Priority: priority,
			Kind:     "k",
			Text:     strings.Repeat("x", writeContextPackTextLen+20),
			Consumers: []WriteContextConsumer{
				WriteConsumerPlanner,
				WriteConsumerPlanner,
				"bogus",
			},
		})
	}
	view := pack.View(WriteConsumerPlanner, 3)
	if len(view.Items) != 2 {
		t.Fatalf("dedupe should leave one p0 and one p3 item before limit, got %d: %+v", len(view.Items), view.Items)
	}
	if view.Items[0].Priority != WriteContextP0 {
		t.Fatalf("p0 item should sort first, got %+v", view.Items[0])
	}
	if len(view.Items[0].Text) > writeContextPackTextLen+3 || !strings.HasSuffix(view.Items[0].Text, "...") {
		t.Fatalf("text not trimmed: %d", len(view.Items[0].Text))
	}
	view.Items[0].Text = "mutated"
	again := pack.View(WriteConsumerPlanner, 3)
	if again.Items[0].Text == "mutated" {
		t.Fatalf("view mutation leaked back into pack")
	}
}

func writeContextConsumersContain(consumers []WriteContextConsumer, want WriteContextConsumer) bool {
	for _, c := range consumers {
		if c == want {
			return true
		}
	}
	return false
}

func writeContextViewPriorityCount(view WriteContextView, priority WriteContextPriority) int {
	count := 0
	for _, item := range view.Items {
		if item.Priority == priority {
			count++
		}
	}
	return count
}

func writeContextViewKindCount(view WriteContextView, kind string) int {
	count := 0
	for _, item := range view.Items {
		if item.Kind == kind {
			count++
		}
	}
	return count
}

func writeContextViewContains(view WriteContextView, kind, substring string) bool {
	for _, item := range view.Items {
		if item.Kind == kind && strings.Contains(item.Text, substring) {
			return true
		}
	}
	return false
}

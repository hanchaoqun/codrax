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
			BehaviorContracts: []WriteBehaviorContract{{
				ID:       "approval-gate",
				Kind:     WriteBehaviorInvariant,
				Polarity: WriteBehaviorPolarityExpected,
				Operator: WriteBehaviorOpSatisfies,
				Expected: "all writes pass apply-pre approval gate",
				Required: true,
				Source:   "write_analyzer",
			}},
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
	if !writeContextViewContains(view, "behavior_contract", "id=approval-gate") {
		t.Fatalf("behavior contract missing from view: %+v", view.Items)
	}
	if !writeContextViewContains(view, "behavior_contract", "polarity=expected") {
		t.Fatalf("behavior contract polarity missing from view: %+v", view.Items)
	}
}

func TestWriteContextPackFromWriteAnalysisIR_ObservedContractNotP0(t *testing.T) {
	ir := &WriteAnalysisIR{
		Request: WriteRequestModel{
			Task: WriteTask{Summary: "fix observed crash"},
			BehaviorContracts: []WriteBehaviorContract{{
				ID:       "observed-crash",
				Kind:     WriteBehaviorException,
				Polarity: WriteBehaviorPolarityObserved,
				Operator: WriteBehaviorOpRaises,
				Subject:  "update_normal",
				Expected: "ZeroDivisionError",
				Required: true,
				Source:   "write_analyzer",
			}},
		},
	}
	pack := WriteContextPackFromWriteAnalysisIR(ir)
	view := pack.View(WriteConsumerPlanner, 10)
	var found *WriteContextItem
	for i := range view.Items {
		if strings.Contains(view.Items[i].Text, "id=observed-crash") {
			found = &view.Items[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("observed contract should remain visible in handoff: %+v", view.Items)
	}
	if found.Priority == WriteContextP0 {
		t.Fatalf("observed pre-fix fact must not become a P0 completion target: %+v", *found)
	}
	if !strings.Contains(found.Text, "polarity=observed") {
		t.Fatalf("observed polarity missing from handoff text: %+v", *found)
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

func TestWriteContextPackFromPlanContextCoverageExcludesPlanSelfContext(t *testing.T) {
	prior := []WriteContextPack{{
		PackID:      "exploration-handoff",
		BatchID:     "batch-1",
		SourceStage: "explore",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "bug.py",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "helper.py",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "tests/test_bug.py",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}, {
		PackID:      "change-plan",
		BatchID:     "batch-1",
		SourceStage: "plan",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "self.py",
			SourceStage: "plan",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}}
	plan := &ChangePlan{
		ID:          "plan-1",
		Summary:     "repair bug",
		TargetPaths: []string{"bug.py"},
		Changes:     []FileChange{{Path: "bug.py", Kind: "modify"}},
	}

	pack := WriteContextPackFromPlanContextCoverage("batch-1", "repair", prior, plan)
	view := pack.View(WriteConsumerPlanner, 10)
	if !writeContextViewContains(view, "plan_context_coverage", "covered=1/2") ||
		!writeContextViewContains(view, "plan_context_coverage", "ratio=0.5000") ||
		!writeContextViewContains(view, "plan_context_coverage", "uncovered_paths=[helper.py]") {
		t.Fatalf("coverage summary should be based on prior non-test context only: %+v", view.Items)
	}
	if !writeContextViewContains(view, "plan_context_uncovered_path", "helper.py") {
		t.Fatalf("missing uncovered production path: %+v", view.Items)
	}
	if writeContextViewContains(view, "plan_context_coverage", "tests/test_bug.py") ||
		writeContextViewContains(view, "plan_context_coverage", "self.py") {
		t.Fatalf("coverage must exclude test paths and plan-authored self context: %+v", view.Items)
	}
}

func TestWriteContextPackFromPlanContextCoverageReportsMissingPriorContext(t *testing.T) {
	plan := &ChangePlan{
		ID:          "plan-1",
		Summary:     "repair bug",
		TargetPaths: []string{"pkg/a.py", "pkg/tests/test_a.py"},
		Changes: []FileChange{
			{Path: "pkg/a.py", Kind: "modify"},
			{Path: "pkg/b.py", Kind: "modify"},
			{Path: "pkg/tests/test_a.py", Kind: "modify"},
		},
	}

	pack := WriteContextPackFromPlanContextCoverage("batch-1", "repair", nil, plan)
	view := pack.View(WriteConsumerPlanner, 10)
	if !writeContextViewContains(view, "plan_context_coverage", "covered=0/0") ||
		!writeContextViewContains(view, "plan_context_coverage", "missing_source_paths=[pkg/a.py,pkg/b.py]") {
		t.Fatalf("coverage summary should report source paths when prior context is absent: %+v", view.Items)
	}
	if !writeContextViewContains(view, "plan_context_missing_source_path", "pkg/a.py") ||
		!writeContextViewContains(view, "plan_context_missing_source_path", "pkg/b.py") {
		t.Fatalf("missing source paths should be typed context items: %+v", view.Items)
	}
	if writeContextViewContains(view, "plan_context_missing_source_path", "pkg/tests/test_a.py") {
		t.Fatalf("test paths should not be reported as missing source context: %+v", view.Items)
	}
}

func TestWriteContextPackFromPlanContextCoverageNormalizesSymbolAnchorsToFiles(t *testing.T) {
	prior := []WriteContextPack{{
		PackID:      "exploration-handoff",
		BatchID:     "batch-1",
		SourceStage: "explore",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "src/_pytest/python_api.py:RaisesContext",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}}
	plan := &ChangePlan{
		ID:          "plan-1",
		Summary:     "repair bug",
		TargetPaths: []string{"src/_pytest/python_api.py"},
		Changes:     []FileChange{{Path: "src/_pytest/python_api.py", Kind: "modify"}},
	}

	pack := WriteContextPackFromPlanContextCoverage("batch-1", "repair", prior, plan)
	view := pack.View(WriteConsumerPlanner, 10)
	if !writeContextViewContains(view, "plan_context_coverage", "covered=1/1") ||
		writeContextViewContains(view, "plan_context_uncovered_path", "src/_pytest/python_api.py") {
		t.Fatalf("symbol-qualified context anchors should cover the changed source file: %+v", view.Items)
	}
}

func TestWriteContextPackFromChangeReportCarriesVerifyFailure(t *testing.T) {
	report := &ChangeReport{
		PlanID:                "plan-1",
		Passed:                false,
		BuildFailed:           true,
		FailureSummary:        "compile failed",
		FailureSummaryBlobRef: "/tmp/codrax/blob/run-tests-plan-1.txt",
		FailureReasonCode:     "pytest_import_startup_error",
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
		ExecutedCommands: []ExecutedCommand{{
			Runner:     "python",
			Framework:  "pytest",
			WorkingDir: ".",
			Command:    "pytest",
			ExitCode:   1,
			Outcome:    "parser_error",
			ReasonCode: "pytest_import_startup_error",
		}},
		VerificationDiagnostics: []VerificationDiagnostic{{
			Source:     "pre_suite_verification_probe",
			Category:   "probe_authoring",
			Severity:   "warning",
			ReasonCode: "verification_probe_name_error",
			Runner:     "verification_probe",
			Outcome:    "parser_error",
		}},
		VerificationConfidence: []VerificationConfidenceRecord{{
			Source:     "verification_probe",
			Category:   "probe_contract_refs",
			Status:     "missing",
			Severity:   "warning",
			ReasonCode: "verification_probe_missing_required_contract_ref",
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
		{"failure_reason_code", "pytest_import_startup_error"},
		{"verification_diagnostic", "category=probe_authoring"},
		{"verification_diagnostic", "reason_code=verification_probe_name_error"},
		{"verification_confidence", "category=probe_contract_refs"},
		{"verification_confidence", "reason_code=verification_probe_missing_required_contract_ref"},
		{"failed_test", "undefined: Foo"},
		{"build_error", "internal/foo.go:10 Foo undefined: Foo"},
		{"executed_command", "reason_code=pytest_import_startup_error"},
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

func TestWriteContextPackScopedViewCarriesP0ButFiltersStaleBatchEvidence(t *testing.T) {
	oldFailure := WriteContextPack{
		PackID:      "old-failure",
		BatchID:     "batch-1",
		SourceStage: "verify",
		Items: []WriteContextItem{{
			Priority:    WriteContextP0,
			Kind:        "constraint",
			Text:        "do not touch persistence",
			SourceStage: "write_analysis",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}, {
			Priority:    WriteContextP2,
			Kind:        "verify_failure",
			Text:        "batch-1 red test",
			SourceStage: "verify",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}
	newTarget := WriteContextPack{
		PackID:      "new-target",
		BatchID:     "batch-2",
		SourceStage: "explore",
		Items: []WriteContextItem{{
			Priority:    WriteContextP1,
			Kind:        "target_file",
			Text:        "pkg/new.py",
			SourceStage: "explore",
			Consumers:   []WriteContextConsumer{WriteConsumerPlanner},
		}},
	}
	view := MergeWriteContextPacks("batch-1", "repair", oldFailure, newTarget).
		ViewForScope(WriteConsumerPlanner, 10, "batch-2", "")
	if !writeContextViewContains(view, "constraint", "do not touch persistence") {
		t.Fatalf("P0 global safety context should carry across batch scope: %+v", view.Items)
	}
	if !writeContextViewContains(view, "target_file", "pkg/new.py") {
		t.Fatalf("active batch context should be visible: %+v", view.Items)
	}
	if writeContextViewContains(view, "verify_failure", "batch-1 red test") {
		t.Fatalf("stale batch verify failure leaked into scoped view: %+v", view.Items)
	}
}

func TestWriteContextPackScopedViewFiltersStaleItems(t *testing.T) {
	pack := NormalizeWriteContextPack(WriteContextPack{
		PackID:  "mixed",
		BatchID: "batch-1",
		Items: []WriteContextItem{{
			Priority:  WriteContextP2,
			Kind:      "verify_failure",
			Text:      "old failure",
			Consumers: []WriteContextConsumer{WriteConsumerPlanner},
			Stale:     true,
		}, {
			Priority:  WriteContextP0,
			Kind:      "constraint",
			Text:      "hard boundary",
			Consumers: []WriteContextConsumer{WriteConsumerPlanner},
			Stale:     true,
		}},
	})
	view := pack.ViewForScope(WriteConsumerPlanner, 10, "batch-1", "")
	if writeContextViewContains(view, "verify_failure", "old failure") {
		t.Fatalf("stale non-P0 item should not render: %+v", view.Items)
	}
	if !writeContextViewContains(view, "constraint", "hard boundary") {
		t.Fatalf("stale P0 safety boundary should still render: %+v", view.Items)
	}
}

func TestWriteContextPackWithScopeRebasesArtifactPackToWorkflowBatch(t *testing.T) {
	reportPack := WriteContextPackFromChangeReport(&ChangeReport{
		PlanID:         "plan-1",
		Passed:         false,
		FailureKind:    FailureKindTestsFailed,
		FailureSummary: "red test",
	})
	scoped := reportPack.WithScope("batch-1", "slice-2")
	view := scoped.ViewForScope(WriteConsumerPlanner, 10, "batch-1", "slice-2")
	if !writeContextViewContains(view, "verify_failure", "red test") {
		t.Fatalf("scoped report pack should remain visible to active batch: %+v", view.Items)
	}
	if got := scoped.BatchID; got != "batch-1" {
		t.Fatalf("scoped pack batch id = %q, want batch-1", got)
	}
	for _, item := range scoped.Items {
		if item.BatchID != "batch-1" || item.SliceID != "slice-2" {
			t.Fatalf("item scope not rebound: %+v", item)
		}
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

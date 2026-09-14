package orchestrator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1673VerifySuccessCountsRecordedResultsNotIndependentTests(t *testing.T) {
	// r1067: Make records the entire suite once, while the native unittest
	// parser records its two assertions. Three result rows are not three
	// independent tests. The rendering must stay honest for legacy scopes too.
	for _, tc := range []struct {
		name string
		rows []types.TestResult
	}{
		{"make_and_two_assertions", []types.TestResult{
			{Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAggregate, AssertionID: "make-test", Suite: "check", Passed: true},
			{Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion, AssertionID: "test_consecutive_newline_run_uses_pair_merge_token", Suite: "tests.test_tokenizer.TokenizerTest", Passed: true},
			{Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion, AssertionID: "test_merge_order", Suite: "tests.test_tokenizer.TokenizerTest", Passed: true},
		}},
		{"assertions_only", []types.TestResult{
			{Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion, AssertionID: "TestA", Passed: true},
			{Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAssertion, AssertionID: "TestB", Passed: true},
		}},
		{"aggregate_only", []types.TestResult{{Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAggregate, AssertionID: "make-test", Passed: true}}},
		{"legacy_scope_absent", []types.TestResult{{AssertionID: "legacy-suite", Passed: true}}},
		{"unknown_scope", []types.TestResult{{ObservationScope: "future_scope", AssertionID: "opaque-result", Passed: true}}},
		{"empty_results", nil},
	} {
		for _, lang := range []string{"zh", "en"} {
			t.Run(tc.name+"/"+lang, func(t *testing.T) {
				report := &types.ChangeReport{PlanID: "b1673-plan", Channel: types.ChangeReportChannelPostApplyVerify, Passed: true, TestResults: tc.rows}
				before := b1673JSON(t, report)
				want := fmt.Sprintf("\n## 测试通过\n\n%d 条验证结果已记录为通过。报告已存到 .codrax/plans/b1673-plan.report.json。\n", len(tc.rows))
				if lang == "en" {
					want = fmt.Sprintf("\n## Tests verified\n\n%d verification result(s) recorded as passed. Report saved to .codrax/plans/b1673-plan.report.json.\n", len(tc.rows))
				}
				if got := renderVerifySuccess(report, lang); got != want {
					t.Errorf("result rows were presented as independent tests:\ngot: %q\nwant: %q", got, want)
				}
				if !bytes.Equal(before, b1673JSON(t, report)) {
					t.Fatal("display changed the report or legacy observation scope")
				}
			})
		}
	}
	for lang, want := range map[string]string{
		"zh": "\n## 测试通过\n\n本次未产出测试报告,验证阶段已完成但没有可显示的细节。\n",
		"en": "\n## Tests verified\n\nNo report produced; the verify step completed but emitted no details.\n",
	} {
		t.Run("nil_report/"+lang, func(t *testing.T) {
			if got := renderVerifySuccess(nil, lang); got != want {
				t.Fatalf("nil report invented a result count or changed its disclosure: %q", got)
			}
		})
	}
}

func TestB1673VerifyResultCountPreservesAdjacentDisclosuresAndProof(t *testing.T) {
	plan := &types.ChangePlan{ID: "b1673-plan", Status: types.PlanStatusApplied, TargetPaths: []string{"module.py"}, AppliedPaths: []string{"module.py"}}
	report := &types.ChangeReport{
		PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify, Passed: true,
		TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, ObservationScope: types.TestObservationScopeAggregate, AssertionID: "make-test", Suite: "check", Passed: true}},
		WorktreeAudit: &types.VerificationWorktreeAudit{
			Status: types.VerificationWorktreeAuditUntrackedSideEffects, UntrackedEffectCount: 1,
			Effects: []types.VerificationWorktreeEffect{{Path: "generated.bin", Kind: types.VerificationWorktreeEffectUntrackedCreated}},
		},
		VerificationDiagnostics: []types.VerificationDiagnostic{{
			Category: "probe_comparator_authority", ReasonCode: "model_authored_probe_comparator_unverified", Runner: "verification_probe", Outcome: "observed_failure",
			FailureObservations: []types.VerificationFailureObservation{{AssertionID: "diagnostic-only", Suite: "probe", FailureDetail: "observed=41 expected=42", OutputRef: "/tmp/diagnostic-only.txt"}},
		}},
	}
	snapshot := func() []byte {
		return b1673JSON(t, []any{plan, report, types.BuildVerificationProofLedger(plan, report, nil), types.EffectiveVerificationConfidence(plan, report)})
	}
	before := snapshot()
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			audit := renderVerificationWorktreeAuditNote(report.WorktreeAudit, lang == "zh")
			observation := renderVerifyFailureObservationNote(report, lang)
			if audit == "" || observation == "" {
				t.Fatal("fixture failed to exercise both independent neighboring disclosures")
			}
			want := "\n## 测试通过\n\n1 条验证结果已记录为通过。报告已存到 .codrax/plans/b1673-plan.report.json。" + audit + "\n" + observation
			if lang == "en" {
				want = "\n## Tests verified\n\n1 verification result(s) recorded as passed. Report saved to .codrax/plans/b1673-plan.report.json." + audit + "\n" + observation
			}
			got := renderVerifySuccess(report, lang)
			if got != want || strings.Count(got, audit) != 1 || strings.Count(got, observation) != 1 {
				t.Fatalf("count wording changed or duplicated adjacent disclosures:\ngot: %q\nwant: %q", got, want)
			}
			if !bytes.Equal(before, snapshot()) {
				t.Fatal("display changed verdict, evidence, plan, proof ledger or confidence")
			}
		})
	}
}

func b1673JSON(t *testing.T, value any) []byte {
	t.Helper()
	wire, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

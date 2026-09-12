package orchestrator

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// This exercises the public runner and durable final-report boundary, not a
// hand-written positive confidence record. The private calls between them are
// the same post-verify synchronization/persistence seams used by the controller.
// A failed model comparator remains a non-authoritative observation: this test
// does not require it to turn a successful native suite into a product failure.
func TestR1061ContractAuthoritySurvivesRunnerAndFinalPersistence(t *testing.T) {
	for _, binary := range []string{"python3", "make"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skipf("native protocol fixture requires %s: %v", binary, err)
		}
	}
	for _, tc := range []struct {
		name         string
		failedProbe  bool
		planningOnly bool
		exactPTO     bool
		historical   bool
		externalErr  bool
	}{
		{name: "native_path_only_is_not_exact_contract"},
		{name: "failed_probe_three_refs_not_laundered_by_make", failedProbe: true},
		{name: "new_planning_only_refs_do_not_mint_obligations", failedProbe: true, planningOnly: true},
		{name: "historical_planning_only_refs_remain_advisory", failedProbe: true, planningOnly: true, historical: true},
		{name: "exact_native_pto_positive", exactPTO: true},
		{name: "failed_probe_does_not_hide_exact_native_pto", failedProbe: true, exactPTO: true},
		{name: "external_verify_error_cannot_be_overridden_by_native_receipt", exactPTO: true, externalErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{
				"widget.py":           "VALUE = 41\n",
				"tests/__init__.py":   "",
				"tests/test_value.py": "import unittest\nimport widget\n\nclass ValueTest(unittest.TestCase):\n    def test_value(self):\n        self.assertEqual(widget.VALUE, 41)\n",
				"Makefile":            ".PHONY: check\ncheck: widget.py tests/test_value.py\n\tPYTHONPATH=. python3 -B -m unittest tests.test_value -v\n",
			}
			for path, content := range files {
				full := filepath.Join(root, path)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			refs := []string{"value-a", "value-b", "value-c"}
			plan := &types.ChangePlan{
				ID: "plan-contract-authority", Status: types.PlanStatusApplied,
				TargetPaths: []string{"widget.py"}, AppliedPaths: []string{"widget.py"},
				Changes: []types.FileChange{{Path: "widget.py", Kind: "modify", NewContent: files["widget.py"], Rationale: "fixture value change"}},
			}
			for _, ref := range refs {
				contract := types.WriteBehaviorContract{
					ID: ref, Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected,
					Subject: "widget.VALUE", Operator: types.WriteBehaviorOpEquals, Expected: "41", Required: !tc.planningOnly,
					EvidenceRef: "tests/test_value.py:6", Source: "fixture_declared_contract",
				}
				if tc.planningOnly && !tc.historical {
					contract.Source = "write_analyzer;" + types.WriteBehaviorContractSourcePlanningOnlyUngrounded
				}
				plan.BehaviorContracts = append(plan.BehaviorContracts, contract)
			}
			if tc.failedProbe {
				plan.VerificationProbes = []types.VerificationProbe{{
					ID: "observed-counterexample", Language: "python", WorkingDir: ".", TimeoutSeconds: 5,
					Code:         "import widget\nassert widget.VALUE == 42, 'model comparator: expected 42, got %r' % widget.VALUE\n",
					ContractRefs: refs, ChangedSymbolRefs: []string{"path:widget.py"},
				}}
			} else {
				// Use the ordinary slice-declared relation to create exactly the
				// same contract impact domain without running a synthetic probe.
				plan.Slices = []types.ChangePlanSlice{{ID: "slice-1", Paths: []string{"widget.py"}, ContractRefs: refs}}
			}
			if tc.exactPTO {
				plan.ProjectTestObservations = []types.ProjectTestObservation{{
					ID: "native-value", TestPath: "tests/test_value.py", AssertionSuite: "ValueTest",
					AssertionID: "test_value", ContractRefs: refs,
				}}
			}
			stampChangePlanImpactObligations(plan, nil)
			review := writeflow.ReviewAppliedPatchSemantic(writeflow.SemanticPatchReviewInput{
				Plan: plan, ImpactAnalysis: plan.ImpactAnalysis, ImpactObligations: plan.ImpactObligations,
			})
			plan.PatchReview = &review
			if tc.historical {
				// Simulate the durable legacy shape from before planning-only
				// obligation filtering. These are deliberately old status claims,
				// not new runtime witnesses; the public read projection must
				// preserve the original snapshot while declining its authority.
				for i := range plan.BehaviorContracts {
					plan.BehaviorContracts[i].Source = "write_analyzer;" + types.WriteBehaviorContractSourcePlanningOnlyUngrounded
				}
				for i := range plan.ImpactAnalysis.VerificationTargets {
					plan.ImpactAnalysis.VerificationTargets[i].CoverageStatus = "verified"
				}
				for i := range plan.PatchReview.Findings {
					if plan.PatchReview.Findings[i].Category == types.PatchReviewCategorySemanticCoverage {
						plan.PatchReview.Findings[i].CoverageStatus = types.PatchReviewCoverageVerified
					}
				}
				var loaded types.ChangePlan
				if err := json.Unmarshal(mustR1061JSON(t, plan), &loaded); err != nil {
					t.Fatal(err)
				}
				plan = &loaded
			}
			inputContracts := mustR1061JSON(t, plan.BehaviorContracts)
			inputProbes := mustR1061JSON(t, plan.VerificationProbes)
			mu := types.NewMutableState("contract authority public runner")
			mu.SetChangePlan(plan)
			ctx := &types.BusContext{
				Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify,
				RepoRoot: root, MainRepoRoot: root, WorkDir: t.TempDir(),
			}
			params := json.RawMessage(`{"runner":"make","suite":"check"}`)
			if tc.exactPTO {
				params = json.RawMessage(`{"runner":"python","framework":"unittest","suite":"tests/test_value.py"}`)
			}
			result, err := (&tool.RunTests{}).Execute(ctx, params)
			if err != nil || !result.Success {
				t.Fatalf("public native runner premise failed: err=%v result=%+v report=%+v", err, result, mu.ChangeReport())
			}
			report := mu.ChangeReport()
			if report == nil || !report.Passed {
				t.Fatalf("native success was not retained: %+v", report)
			}
			// Round-trip the actual system result, as the final artifact consumer
			// does. Neither confidence nor assertion outcomes are manufactured.
			reportJSON, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var persistedReport types.ChangeReport
			if err := json.Unmarshal(reportJSON, &persistedReport); err != nil {
				t.Fatal(err)
			}
			report = &persistedReport
			mu.SetChangeReport(report)
			failedProbeSeen, behaviorPathSeen, nativeAssertionSeen := false, false, false
			for _, command := range report.ExecutedCommands {
				if command.Runner == "verification_probe" && command.Outcome == types.ExecutedCommandOutcomeExecuted && command.ExitCode != 0 {
					failedProbeSeen = true
				}
			}
			if tc.historical {
				legacyJSON := mustR1061JSON(t, plan)
				ledger := types.BuildVerificationProofLedger(plan, report, nil)
				matched := 0
				for _, item := range ledger.Obligations {
					if item.Kind != "behavior_contract" {
						continue
					}
					matched++
					if item.Status != types.VerificationProofLedgerItemAdvisory {
						t.Errorf("public historical ledger trusted saved planning-only proof: %+v", item)
					}
				}
				if matched != 2*len(refs) {
					t.Errorf("historical advisory rows disappeared: got=%d want=%d", matched, 2*len(refs))
				}
				if !bytes.Equal(legacyJSON, mustR1061JSON(t, plan)) {
					t.Error("public historical ledger rewrote saved plan input")
				}
			}
			for _, coverage := range report.ChangedPathCoverage {
				if coverage.Path == "widget.py" && coverage.Status == types.ChangedPathVerificationCovered && coverage.Capability == types.VerificationCapabilityTargetBehavior && coverage.Caliber == types.ChangedPathVerificationProjectRunner {
					behaviorPathSeen = true
				}
			}
			for _, row := range report.TestResults {
				if row.Passed && row.AssertionID == "test_value" && row.ObservationScope == types.TestObservationScopeAssertion {
					nativeAssertionSeen = true
				}
			}
			if tc.failedProbe != failedProbeSeen || !behaviorPathSeen || (tc.exactPTO && !nativeAssertionSeen) {
				t.Fatalf("real execution premise: failedProbe=%v pathBehavior=%v nativeAssertion=%v report=%s", failedProbeSeen, behaviorPathSeen, nativeAssertionSeen, reportJSON)
			}
			if tc.exactPTO {
				covered := types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, report.VerificationConfidence)
				if len(covered) != len(refs) {
					t.Fatalf("exact native assertion did not mint its existing PTO receipt: covered=%v confidence=%+v", covered, report.VerificationConfidence)
				}
			}
			o := &Orchestrator{busCtx: ctx}
			var verifyErr error
			if tc.externalErr {
				verifyErr = errors.New("fixture external verification failure after native result")
			}
			o.syncMutablePlanStatusAfterVerify(report, verifyErr)
			if tc.externalErr {
				for _, target := range mu.ChangePlan().ImpactAnalysis.VerificationTargets {
					if target.Kind == "behavior_contract" && target.CoverageStatus == "verified" {
						t.Errorf("sync erased external verify failure: %+v", target)
					}
				}
				for _, finding := range mu.ChangePlan().PatchReview.Findings {
					if finding.Code == "behavior_contract_without_verify_coverage" && finding.CoverageStatus == types.PatchReviewCoverageVerified {
						t.Errorf("sync erased external verify failure in patch review: %+v", finding)
					}
				}
				if mu.ChangePlan().Status != types.PlanStatusVerifyFailed {
					t.Errorf("external verification error did not preserve failed plan status: %s", mu.ChangePlan().Status)
				}
				if !bytes.Equal(reportJSON, mustR1061JSON(t, report)) || !report.Passed {
					t.Error("external verification error rewrote the separately passed native report")
				}
				// Do not forge a successful workflow completion after this
				// intentional failing scheduler seam. The other arms exercise
				// terminal persistence after genuinely successful verification.
				return
			}
			if tc.exactPTO && !tc.failedProbe && !tc.externalErr {
				// Cumulative input may legitimately retain a different plan's
				// report, but only as that exact plan/report pair. Reusing its
				// confidence against a mismatched report ID is not a receipt.
				current := &types.ChangePlan{ID: "next-plan"}
				currentReport := &types.ChangeReport{PlanID: current.ID, Passed: true, VerificationStatus: types.VerificationStatusPassed}
				for _, wrongPlanID := range []bool{false, true} {
					var historicalReport types.ChangeReport
					if err := json.Unmarshal(reportJSON, &historicalReport); err != nil {
						t.Fatal(err)
					}
					if wrongPlanID {
						historicalReport.PlanID = "different-plan"
					}
					artifactPlan := mu.ChangePlan()
					planBefore := mustR1061JSON(t, artifactPlan)
					reportBefore := mustR1061JSON(t, &historicalReport)
					ledger := types.BuildVerificationProofLedger(current, currentReport, []types.VerificationProofArtifact{{Plan: artifactPlan, Report: &historicalReport}})
					matched := 0
					for _, item := range ledger.Obligations {
						if item.Kind != "behavior_contract" || (item.Source != "change_plan_slice" && item.Source != "patch_review") {
							continue
						}
						matched++
						if (item.Status == types.VerificationProofLedgerItemCovered) == wrongPlanID {
							t.Errorf("cumulative exact-plan witness mismatch wrongPlanID=%v: %+v", wrongPlanID, item)
						}
					}
					if matched != 2*len(refs) {
						t.Errorf("cumulative source rows disappeared: wrongPlanID=%v got=%d", wrongPlanID, matched)
					}
					if !bytes.Equal(planBefore, mustR1061JSON(t, artifactPlan)) || !bytes.Equal(reportBefore, mustR1061JSON(t, &historicalReport)) {
						t.Error("cumulative proof projection rewrote source artifacts")
					}
				}
			}
			completion := &types.WriteWorkflowCompletion{Verdict: types.WriteWorkflowCompletionVerified, ReasonCode: "native_fixture_passed", Source: "verify_attempt"}
			o.persistWriteWorkflowRun(&types.WriteWorkflowRun{
				RunID: "wf-contract-authority", Status: types.WriteWorkflowRunComplete, ActiveBatchID: "batch-1", Completion: completion,
				Batches: []types.WriteWorkflowBatch{{ID: "batch-1", Status: types.WriteWorkflowBatchComplete, PlanID: plan.ID, Completion: completion}},
			})
			finalPath := filepath.Join(ctx.WorkDir, "plans", plan.ID+".final.json")
			final, err := types.LoadWriteFinalReportFromFile(finalPath)
			if err != nil {
				t.Fatal(err)
			}
			matchedImpact, matchedReview := 0, 0
			for _, item := range final.ProofLedger.Obligations {
				impact := item.Kind == "behavior_contract" && (item.Source == "verification_probe" || item.Source == "change_plan_slice")
				patchReview := item.Source == "patch_review" && item.Kind == "behavior_contract" && item.Category == string(types.PatchReviewCategorySemanticCoverage)
				if !impact && !patchReview {
					continue
				}
				if impact {
					matchedImpact++
				} else {
					matchedReview++
				}
				if tc.exactPTO && !tc.externalErr {
					if item.Status != types.VerificationProofLedgerItemCovered {
						t.Errorf("exact native contract receipt was lost: %+v", item)
					}
				} else if tc.planningOnly {
					if item.Status != types.VerificationProofLedgerItemAdvisory {
						t.Errorf("planning-only declaration acquired proof/mandatory debt: %+v", item)
					}
				} else if item.Status == types.VerificationProofLedgerItemCovered {
					t.Errorf("path-only success authorized an exact contract without a positive receipt: %+v", item)
				}
			}
			wantRows := len(refs)
			if tc.planningOnly && !tc.historical {
				wantRows = 0
			}
			if matchedImpact != wantRows || matchedReview != wantRows {
				t.Errorf("final contract lanes must be nonempty: impact=%d review=%d ledger=%+v", matchedImpact, matchedReview, final.ProofLedger)
			}
			if tc.planningOnly && len(types.HardRequiredWriteBehaviorContractIDs(plan.BehaviorContracts)) != 0 {
				t.Error("planning-only fixture unexpectedly became hard-required")
			}
			afterReportJSON, _ := json.Marshal(report)
			if !bytes.Equal(reportJSON, afterReportJSON) {
				t.Error("controller mutated the original execution report")
			}
			if !bytes.Equal(inputContracts, mustR1061JSON(t, mu.ChangePlan().BehaviorContracts)) || !bytes.Equal(inputProbes, mustR1061JSON(t, mu.ChangePlan().VerificationProbes)) {
				t.Error("verification removed or rewrote declared contract/probe inventory")
			}
			for path, content := range files {
				got, err := os.ReadFile(filepath.Join(root, path))
				if err != nil || string(got) != content {
					t.Errorf("verification changed fixture source %s: %v", path, err)
				}
			}
			t.Logf("native passed=%v failed_probe=%v exact_pto=%v impact_verified=%d final=%s confidence=%s", report.Passed, failedProbeSeen, tc.exactPTO, final.Proof.ImpactVerifiedCount, finalPath, strings.TrimSpace(string(mustR1061JSON(t, report.VerificationConfidence))))
		})
	}
}

func mustR1061JSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

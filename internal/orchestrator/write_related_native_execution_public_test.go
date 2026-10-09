package orchestrator

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

const relatedNativeTestPath = "packages/widget/tests/test_widget.py"

// This is an ordinary edit with a graph-selected related test, not a user
// execution requirement or a manufactured project-test/behavior declaration.
// Real emit/apply/native execution produce the evidence consumed below.
func relatedNativeExecutionPublicFixture(t *testing.T, testBody string) (*types.BusContext, *types.ChangePlan) {
	t.Helper()
	for _, name := range []string{"git", "python3"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skip(name + " unavailable")
		}
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		".gitignore":                        "__pycache__/\n*.pyc\n.codrax/\n",
		"packages/widget/setup.py":          "from setuptools import setup\nsetup(name='widget', version='0.1', py_modules=['widget'])\n",
		"packages/widget/widget.py":         "def increment(value):\n    return value\n",
		"packages/widget/tests/__init__.py": "",
		"packages/widget/external_tests.py": "import unittest\nclass ExternalTest(unittest.TestCase):\n    def test_external(self): self.assertEqual(1, 1)\n",
		relatedNativeTestPath:               testBody,
	} {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string { return runGitForWorkflowRestoreTest(t, root, args...) }
	git("init", "-q")
	git("add", ".")
	git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "before")
	mu := types.NewMutableState("correct increment and check the related tests")
	ctx := &types.BusContext{Mutable: mu, RepoRoot: root, MainRepoRoot: root, WorkDir: t.TempDir(), Mode: types.ModeApply, PipelineStage: types.StagePlan}
	controllerRegistrationTool(t, ctx, (&tool.EmitChangePlan{}).Execute, map[string]any{"request": "correct increment", "summary": "Return the next integer.", "changes": []map[string]any{{"path": "packages/widget/widget.py", "kind": "modify", "new_content": "def increment(value):\n    return value + 1\n", "rationale": "fix the result"}}})
	plan := mu.ChangePlan()
	ctx.PipelineStage = types.StageApply
	controllerRegistrationTool(t, ctx, (&tool.ApplyPatch{}).Execute, map[string]any{"path": "packages/widget/widget.py", "kind": "modify"})
	git("add", "packages/widget/widget.py")
	git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-qm", "applied")
	head := strings.TrimSpace(git("rev-parse", "HEAD"))
	diff, err := worktree.CaptureCommitPatch(root, head)
	if err != nil {
		t.Fatal(err)
	}
	effect := writeflow.PatchEffectRecordFromUnifiedDiff(plan.ID, "", "applied_commit", head+"^", head, diff)
	plan.PatchEffect, plan.AppliedCommitSHA, plan.WorktreePath, plan.Status = &effect, head, root, types.PlanStatusApplied
	// Graph discovery is a selection seam, never pre-populated execution proof.
	plan.ImpactAnalysis = &types.ImpactAnalysisResult{VerificationTargets: []types.ImpactVerificationTarget{{Kind: "test_surface", Path: "packages/widget/widget.py", RelatedPath: relatedNativeTestPath, Source: "impact_engine", Strength: "inferred", CoverageStatus: "unverified"}}}
	plan.PatchReview = &types.PatchReviewRecord{Findings: []types.PatchReviewFinding{{Code: "related_test_surface_unverified", Category: types.PatchReviewCategorySemanticCoverage, Path: "packages/widget/widget.py", RelatedPath: relatedNativeTestPath, CoverageStatus: types.PatchReviewCoverageUnverified}}}
	mu.SetChangePlan(plan)
	ctx.PipelineStage = types.StageVerify
	return ctx, plan
}

func TestRelatedNativeExecutionPublicCoverage(t *testing.T) {
	ctx, plan := relatedNativeExecutionPublicFixture(t, "import unittest\nfrom widget import increment\nclass IncrementTest(unittest.TestCase):\n    def test_value(self): self.assertEqual(increment(2), 3)\n")
	_, err := (&tool.RunTests{}).Execute(ctx, json.RawMessage(`{}`))
	report := ctx.Mutable.ChangeReport()
	if err != nil || report == nil || !report.Passed {
		t.Fatalf("real related native execution failed: %v %+v", err, report)
	}
	if len(report.ExistingTestExecutions) != 1 || report.ExistingTestExecutions[0].TestPath != relatedNativeTestPath {
		t.Errorf("ordinary related test lacks exact current file receipt: %+v", report.ExistingTestExecutions)
	}
	rootZero := false
	for _, command := range report.ExecutedCommands {
		rootZero = rootZero || command.WorkingDir == "." && command.Outcome == types.ExecutedCommandOutcomeZeroTests
	}
	if !rootZero {
		t.Error("root zero-tests observation was lost")
	}
	applyVerifyCoverageToChangePlan(plan, report, nil)
	if len(plan.ImpactAnalysis.VerificationTargets) != 1 || plan.ImpactAnalysis.VerificationTargets[0].CoverageStatus != "verified" {
		t.Errorf("real file execution not consumed by impact: %+v", plan.ImpactAnalysis.VerificationTargets)
	}
	if len(plan.PatchReview.Findings) != 1 || plan.PatchReview.Findings[0].CoverageStatus != types.PatchReviewCoverageVerified {
		t.Errorf("real file execution not consumed by review: %+v", plan.PatchReview.Findings)
	}
	profile := types.BuildVerificationProofProfile(plan, report)
	if profile.ImpactUnverifiedCount != 0 || profile.PatchReviewVerdict == types.PatchReviewCoverageVerdictUnverified {
		t.Fatalf("final proof profile retained already-closed file debt: %+v", profile)
	}
	if len(types.RequiredExistingTestPaths(plan)) != 0 || len(plan.ProjectTestObservations) != 0 || len(types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, report.VerificationConfidence)) != 0 {
		t.Fatal("related execution manufactured user intent or behavior proof")
	}
}

func TestRelatedNativeExecutionPublicNoFalseFileProof(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"skipped", "import unittest\nclass IncrementTest(unittest.TestCase):\n    @unittest.skip('not executed')\n    def test_value(self): self.assertEqual(1, 1)\n"},
		{"imported", "from external_tests import ExternalTest\n"},
		{"load_tests_redirect", "import unittest\nfrom external_tests import ExternalTest\ndef load_tests(loader, tests, pattern):\n    return loader.loadTestsFromTestCase(ExternalTest)\n"},
		{"failed", "import unittest\nclass IncrementTest(unittest.TestCase):\n    def test_value(self): self.assertEqual(1, 2)\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, plan := relatedNativeExecutionPublicFixture(t, tc.body)
			_, err := (&tool.RunTests{}).Execute(ctx, json.RawMessage(`{}`))
			report := ctx.Mutable.ChangeReport()
			if err != nil || report == nil {
				t.Fatalf("native run unavailable: %v", err)
			}
			if got := types.VerifiedNativeTestExecutionPaths(plan, report); len(got) != 0 {
				t.Fatalf("non-file or failed execution lent proof: %v", got)
			}
			applyVerifyCoverageToChangePlan(plan, report, nil)
			if plan.ImpactAnalysis.VerificationTargets[0].CoverageStatus == "verified" || plan.PatchReview.Findings[0].CoverageStatus == types.PatchReviewCoverageVerified {
				t.Fatal("unobserved file became verified")
			}
		})
	}
}

func TestRelatedNativeExecutionPublicReceiptAndPathBoundaries(t *testing.T) {
	ctx, plan := relatedNativeExecutionPublicFixture(t, "import unittest\nclass IncrementTest(unittest.TestCase):\n    def test_value(self): self.assertEqual(1, 1)\n")
	_, err := (&tool.RunTests{}).Execute(ctx, json.RawMessage(`{}`))
	if err != nil || ctx.Mutable.ChangeReport() == nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal([]any{plan, ctx.Mutable.ChangeReport()})
	for _, name := range []string{"no_receipt", "stale_commit", "duplicate_invocation", "sibling", "parent", "source", "restored_no_receipt", "restored_stale_commit", "restored_duplicate_invocation"} {
		t.Run(name, func(t *testing.T) {
			var p types.ChangePlan
			var r types.ChangeReport
			if err := json.Unmarshal(data, &[]any{&p, &r}); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(name, "restored_") {
				applyVerifyCoverageToChangePlan(&p, &r, nil)
				if p.ImpactAnalysis.VerificationTargets[0].CoverageStatus != "verified" {
					t.Fatal("restoration fixture never acquired proof")
				}
				persisted, _ := json.Marshal(&p)
				if err := json.Unmarshal(persisted, &p); err != nil {
					t.Fatal(err)
				}
			}
			switch strings.TrimPrefix(name, "restored_") {
			case "no_receipt":
				r.ExistingTestExecutions = nil
			case "stale_commit":
				p.AppliedCommitSHA = strings.Repeat("f", 40)
			case "duplicate_invocation":
				r.ExecutedCommands = append(r.ExecutedCommands, r.ExecutedCommands[r.ExistingTestExecutions[0].CommandIndex])
			default:
				target := map[string]string{"sibling": "packages/other/tests/test_widget.py", "parent": "packages/widget/tests", "source": "packages/widget/widget.py"}[name]
				p.ImpactAnalysis.VerificationTargets[0].RelatedPath = target
				p.PatchReview.Findings[0].RelatedPath = target
			}
			applyVerifyCoverageToChangePlan(&p, &r, nil)
			if p.ImpactAnalysis.VerificationTargets[0].CoverageStatus == "verified" || p.PatchReview.Findings[0].CoverageStatus == types.PatchReviewCoverageVerified {
				t.Fatal("unbound receipt or unrelated file borrowed proof")
			}
		})
	}
}

func TestRelatedNativeExecutionPublicCumulativeAndContinuation(t *testing.T) {
	ctx, source := relatedNativeExecutionPublicFixture(t, "import unittest\nclass IncrementTest(unittest.TestCase):\n    def test_value(self): self.assertEqual(1, 1)\n")
	_, err := (&tool.RunTests{}).Execute(ctx, json.RawMessage(`{}`))
	report := ctx.Mutable.ChangeReport()
	if err != nil || report == nil {
		t.Fatal(err)
	}
	beforeSource, _ := json.Marshal(source)
	beforeReport, _ := json.Marshal(report)
	for _, mode := range []string{"current", "foreign_source", "stale_delivery", "failed_call"} {
		t.Run(mode, func(t *testing.T) {
			var authority, view types.ChangePlan
			_ = json.Unmarshal(beforeSource, &authority)
			_ = json.Unmarshal(beforeSource, &view)
			view.ID = "synthetic-cumulative-view"
			view.ImpactAnalysis.VerificationTargets = append(view.ImpactAnalysis.VerificationTargets, types.ImpactVerificationTarget{Kind: "behavior_contract", Path: "packages/widget/widget.py", RelatedPath: relatedNativeTestPath, CoverageStatus: "unverified", ContractRef: "not-declared"})
			var callErr error
			switch mode {
			case "foreign_source":
				authority.ID = "foreign"
			case "stale_delivery":
				authority.AppliedCommitSHA = strings.Repeat("b", 40)
			case "failed_call":
				callErr = os.ErrInvalid
			}
			applyNativeTestFileCoverage(&view, &authority, report, callErr)
			verified := false
			for _, target := range view.ImpactAnalysis.VerificationTargets {
				if target.Kind == "test_surface" {
					verified = target.CoverageStatus == "verified"
				}
				if target.Kind == "behavior_contract" && target.CoverageStatus == "verified" {
					t.Fatal("file execution signed behavior")
				}
			}
			if verified != (mode == "current") {
				t.Fatalf("cumulative scope=%s verified=%v", mode, verified)
			}
		})
	}
	afterSource, _ := json.Marshal(source)
	afterReport, _ := json.Marshal(report)
	if string(beforeSource) != string(afterSource) || string(beforeReport) != string(afterReport) {
		t.Fatal("cumulative projection rewrote delivery/history")
	}

	run := &types.WriteWorkflowRun{RunID: "related-continuation", ActiveBatchID: "source", Batches: []types.WriteWorkflowBatch{{ID: "source", PlanID: source.ID, Status: types.WriteWorkflowBatchComplete, Attempts: []types.WriteWorkflowAttempt{{Kind: "verify", Status: "passed", PlanID: source.ID}}}}}
	items := []impactRepairQueueItem{
		{ID: "test", Kind: "test_surface", Path: "packages/widget/widget.py", RelatedPath: relatedNativeTestPath, CoverageStatus: "unverified", Source: "impact_engine"},
		{ID: "other", Kind: "test_surface", RelatedPath: "packages/other/tests/test_widget.py", CoverageStatus: "unverified"},
		{ID: "dependency", Kind: "dependent", Path: "packages/widget/widget.py", CoverageStatus: "unverified"},
	}
	candidates := nativeTestFileRetryCandidates(source, report)
	kept := filterPassedVerifyGraphTelemetryItems(run, "source", items, candidates)
	if len(kept) != 1 || kept[0].ID != "test" {
		t.Fatalf("exact retry scope lost or broadened: candidates=%v kept=%+v", candidates, kept)
	}
	batch := newImpactRepairFollowupBatch(run, "source", "impact-repair", kept)
	if batch.ExecutionMode != types.WriteWorkflowBatchExecutionVerifyOnly || batch.Purpose != "impact_obligation_followup" {
		t.Fatalf("file execution retry invented code/proof plan: %+v", batch)
	}
	var missingReceipt types.ChangeReport
	_ = json.Unmarshal(beforeReport, &missingReceipt)
	missingReceipt.ExistingTestExecutions = nil
	followup, suppressed := impactObligationRepairFollowupDecision(run, "source", source, &missingReceipt)
	if suppressed || followup == nil || followup.ExecutionMode != types.WriteWorkflowBatchExecutionVerifyOnly || followup.Purpose != "impact_obligation_followup" {
		t.Fatalf("actual continuation dropped exact missing execution after another suite passed: %+v suppressed=%v", followup, suppressed)
	}
	for _, criterion := range followup.SuccessCriteria {
		if strings.Contains(criterion, "verification_probe_required=true") {
			t.Fatalf("ordinary file-execution continuation invented a probe obligation: %s", criterion)
		}
	}
	for _, mode := range []string{"foreign_report", "no_surface", "ambiguous_candidate", "missing_file"} {
		t.Run(mode, func(t *testing.T) {
			var r types.ChangeReport
			var p types.ChangePlan
			_ = json.Unmarshal(beforeReport, &r)
			_ = json.Unmarshal(beforeSource, &p)
			switch mode {
			case "foreign_report":
				r.PlanID = "other"
			case "no_surface":
				r.TestSurface = nil
			case "ambiguous_candidate":
				for _, candidate := range r.TestSurface.Candidates {
					if candidate.Runner == "python" && candidate.Framework == "unittest" && candidate.WorkingDir == "packages/widget" {
						r.TestSurface.Candidates = append(r.TestSurface.Candidates, candidate)
						break
					}
				}
			case "missing_file":
				p.WorktreePath = t.TempDir()
			}
			if got := nativeTestFileRetryCandidates(&p, &r); len(got) != 0 {
				t.Fatalf("unbound retry candidate: %v", got)
			}
		})
	}
}

func TestNativeFileCoverageLedgerKeepsObligationSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, kind, source, symbol, contract string
		proof                                bool
	}{
		{name: "ordinary", kind: "test_surface", source: "impact_engine"},
		{name: "missing_source", kind: "test_surface"},
		{name: "probe_source", kind: "test_surface", source: "verification_probe", proof: true},
		{name: "confidence_source", kind: "test_surface", source: "verification_confidence", proof: true},
		{name: "symbol_ref", kind: "test_surface", source: "impact_engine", symbol: "increment", proof: true},
		{name: "contract_ref", kind: "test_surface", source: "impact_engine", contract: "returns-next", proof: true},
		{name: "behavior", kind: "behavior_contract", source: "impact_engine", contract: "returns-next", proof: true},
		{name: "changed_symbol", kind: "changed_symbol", source: "impact_engine", symbol: "increment", proof: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := &types.ChangePlan{ID: "file-obligation", ImpactAnalysis: &types.ImpactAnalysisResult{VerificationTargets: []types.ImpactVerificationTarget{{
				Kind: tc.kind, Source: tc.source, Symbol: tc.symbol, ContractRef: tc.contract,
				Path: "packages/widget/widget.py", RelatedPath: relatedNativeTestPath, CoverageStatus: "unverified",
			}}}}
			items := verificationProofLedgerRepairQueueItems(plan, &types.ChangeReport{PlanID: plan.ID, Passed: true})
			found := false
			for _, item := range items {
				if item.Kind != tc.kind || item.RelatedPath != relatedNativeTestPath {
					continue
				}
				found = true
				if got := impactRepairQueueItemFromVerificationProof(item); got != tc.proof {
					t.Fatalf("ledger changed obligation semantics: %+v proof=%v want=%v", item, got, tc.proof)
				}
			}
			if !found {
				t.Fatalf("ledger lost the original obligation: %+v", items)
			}
		})
	}
}

func TestNativeFileCoverageLegacyAndOtherFrameworkCompatibility(t *testing.T) {
	for _, tc := range []struct {
		runner, framework, invocation string
		want                          bool
	}{
		{"go", "", "identified", true}, {"python", "pytest", "identified", true}, {"node", "jest", "identified", true},
		{"python", "unittest", "", true}, {"python", "unittest", "identified", false},
	} {
		t.Run(tc.runner+"/"+tc.framework+"/"+tc.invocation, func(t *testing.T) {
			r := &types.ChangeReport{Passed: true, ExecutedCommands: []types.ExecutedCommand{{InvocationID: tc.invocation, Runner: tc.runner, Framework: tc.framework, WorkingDir: ".", Suite: "tests/test_widget.py", Outcome: types.ExecutedCommandOutcomeExecuted}}, TestResults: []types.TestResult{{InvocationID: tc.invocation, Suite: "tests/test_widget.py", Passed: true}}}
			got := verifyCoverageConfidenceFromEffectiveReport(r).CoversPath("tests/test_widget.py")
			if got != tc.want {
				t.Fatalf("legacy/unrelated native protocol changed: covered=%v", got)
			}
		})
	}
}

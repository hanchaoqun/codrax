package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// These are actual RunTests executions over fresh applied git trees, not
// reports asserting that a process/target happened to execute. The source
// marker is test-only independent ground truth; production must not inspect
// checker text to decide whether its aggregate success proves behavior.
func TestB1678OpaqueMakeSuccessDoesNotMintTargetBehavior(t *testing.T) {
	for _, tc := range []struct {
		name, check, probe string
		called, receipt    bool
	}{
		{"read and AST only", b1678StaticChecker, "", false, false},
		{"real call without target receipt", b1678CallingChecker, "", true, false},
		{"import-only probe", b1678StaticChecker, "import widget\nassert callable(widget.increment)\n", false, false},
		{"actual changed-target probe", b1678StaticChecker, "import widget\nassert widget.increment(2) == 3\n", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, plan := b1678MakeContext(t, tc.check, tc.probe)
			result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{}))
			if err != nil {
				t.Fatalf("public local execution: %v", err)
			}
			report := ctx.Mutable.ChangeReport()
			if report == nil {
				t.Fatalf("report missing: %+v", result)
			}
			called := false
			if data, err := os.ReadFile(filepath.Join(ctx.RepoRoot, ".target-called")); err == nil {
				called = string(data) == "executed"
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if called != tc.called {
				t.Fatalf("fixture target called=%v want=%v", called, tc.called)
			}
			b1678AssertMakeProcessPreserved(t, report)
			if !report.Passed {
				t.Errorf("an unobserved capability must not rewrite successful process/path verification: %+v", report)
			}
			encoded, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var restored types.ChangeReport
			if err := json.Unmarshal(encoded, &restored); err != nil {
				t.Fatal(err)
			}
			if len(restored.ChangedPathCoverage) != 1 {
				t.Fatalf("expected exact target path only: %+v", restored.ChangedPathCoverage)
			}
			row := restored.ChangedPathCoverage[0]
			if row.Path != "widget.py" || row.Status != types.ChangedPathVerificationCovered {
				t.Errorf("repository-declared exact path check was lost: %+v", row)
			}
			if tc.receipt {
				resolved := types.ResolveVerificationProbeTargetExecution(plan, plan.VerificationProbes[0], &restored)
				if len(resolved.Paths) != 1 || resolved.Paths[0] != "widget.py" {
					t.Fatalf("positive control needs actual executor-owned receipt: %+v", resolved)
				}
				if row.Capability != types.VerificationCapabilityTargetExecution || row.Caliber != types.ChangedPathVerificationProbe || row.Source != "target-probe" {
					t.Errorf("opaque Make must not outrank an actual target-only receipt: %+v", row)
				}
				// Earlier releases selected Make's over-strong label instead of
				// the receipt already present in this report. Restore authority
				// from that real receipt, without rewriting persisted history.
				legacy := restored
				legacy.ChangedPathCoverage = append([]types.ChangedPathVerificationCoverage(nil), restored.ChangedPathCoverage...)
				legacy.ChangedPathCoverage[0].Runner = "make"
				legacy.ChangedPathCoverage[0].Source = verificationProbeContinuationSourceDeclaredCoverage
				legacy.ChangedPathCoverage[0].Caliber = types.ChangedPathVerificationProjectRunner
				legacy.ChangedPathCoverage[0].Capability = types.VerificationCapabilityTargetBehavior
				before, _ := json.Marshal(&legacy)
				effective := types.EffectiveChangedPathVerificationCoverage(plan, &legacy)
				if len(effective) != 1 || effective[0].Capability != types.VerificationCapabilityTargetExecution || effective[0].Source != "target-probe" {
					t.Errorf("historical Make label hid genuine target receipt: %+v", effective)
				}
				after, _ := json.Marshal(&legacy)
				if !bytes.Equal(before, after) {
					t.Error("historical report was migrated in place")
				}
			} else if row.Capability != types.VerificationCapabilityUnknown {
				t.Errorf("same driver language + declared path is not target-execution proof: %+v", row)
			}
			if restored.HasTargetExecutionCoverage() != tc.receipt {
				t.Errorf("execution authority=%v want receipt=%v", restored.HasTargetExecutionCoverage(), tc.receipt)
			}
			if changeReportHasExecutionCapabilityDebt(&restored) != !tc.receipt {
				t.Errorf("unknown successful scope must remain an execution-capability debt: %+v", restored.ChangedPathCoverage)
			}
			profile := types.BuildVerificationProofProfile(plan, &restored)
			ledger := types.BuildVerificationProofLedger(plan, &restored, nil)
			if profile.TargetBehaviorPaths != 0 {
				t.Errorf("aggregate/target-execution-only evidence minted behavior: %+v", profile)
			}
			if !tc.receipt && (profile.Status == types.VerificationProofStrong || ledger.State == types.VerificationProofLedgerVerified) {
				t.Errorf("unobserved target execution became verified/strong: profile=%+v ledger=%+v", profile, ledger)
			}
			after, _ := json.Marshal(&restored)
			if !bytes.Equal(encoded, after) {
				t.Error("read-only proof projections modified original process/assertion evidence")
			}
		})
	}
}

func TestB1678UnknownCapabilityRemainsBoundedExecutionDebt(t *testing.T) {
	for _, tc := range []struct {
		name       string
		passed     bool
		capability types.VerificationCapability
		want       bool
	}{
		{"unknown passing check", true, types.VerificationCapabilityUnknown, true},
		{"legacy absent capability", true, "", true},
		{"actual target execution", true, types.VerificationCapabilityTargetExecution, false},
		{"native target behavior", true, types.VerificationCapabilityTargetBehavior, false},
		{"failed command", false, types.VerificationCapabilityUnknown, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := &types.ChangeReport{Passed: tc.passed, ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{Path: "widget.py", Status: types.ChangedPathVerificationCovered, Capability: tc.capability}}}
			if got := changeReportHasExecutionCapabilityDebt(report); got != tc.want {
				t.Errorf("execution debt=%v want=%v", got, tc.want)
			}
		})
	}
}

// Native assertion receipts retain their established authority. This test
// genuinely calls the target and observes its result; it is not an aggregate
// Make success repackaged as a made-up named assertion.
func TestB1678NativeProjectAssertionRemainsAvailable(t *testing.T) {
	ctx, plan := b1678MakeContext(t, b1678StaticChecker, "")
	b1678InstallNativeAssertion(t, ctx, plan)
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": "python", "framework": "unittest"}))
	if err != nil {
		t.Fatal(err)
	}
	b1678AssertNativeReceipt(t, plan, ctx.Mutable.ChangeReport(), result.Success)
}

func TestB1678HistoricalMakeWinnerPreservesActualNativeAssertion(t *testing.T) {
	ctx, plan := b1678MakeContext(t, b1678StaticChecker, "")
	b1678InstallNativeAssertion(t, ctx, plan)
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	report := ctx.Mutable.ChangeReport()
	b1678AssertNativeReceipt(t, plan, report, result.Success)
	b1678AssertMakeProcessPreserved(t, report)
	native := false
	for _, command := range report.ExecutedCommands {
		native = native || (command.Runner == "python" && command.Framework == pythonFrameworkUnittest && command.Outcome == types.ExecutedCommandOutcomeExecuted && command.ExitCode == 0 && command.Suite == "tests/test_widget.py" && command.Source == "impact_test_surface")
	}
	if !native {
		t.Fatalf("fixture must have both already-planned real project executions: %+v", report.ExecutedCommands)
	}
	// The old strict-greater comparison retained the first Make behavior row
	// even after this equally ranked native command and assertion executed.
	legacy := *report
	legacy.ChangedPathCoverage = append([]types.ChangedPathVerificationCoverage(nil), report.ChangedPathCoverage...)
	legacy.ChangedPathCoverage[0].Runner = "make"
	legacy.ChangedPathCoverage[0].Source = verificationProbeContinuationSourceDeclaredCoverage
	legacy.ChangedPathCoverage[0].Capability = types.VerificationCapabilityTargetBehavior
	before, _ := json.Marshal(&legacy)
	rows := types.EffectiveChangedPathVerificationCoverage(plan, &legacy)
	if len(rows) != 1 || rows[0].Runner != "python" || rows[0].Capability != types.VerificationCapabilityTargetBehavior || !legacy.HasTargetExecutionCoverage() {
		t.Errorf("historical aggregate winner hid real native target evidence: %+v", rows)
	}
	after, _ := json.Marshal(&legacy)
	if !bytes.Equal(before, after) {
		t.Error("historical native report was rewritten")
	}
}

func TestB1678OpaqueMakeContinuesNativeSurfaceWithoutFalseStaticDisclosure(t *testing.T) {
	checker := b1678StaticChecker + "print('" + strings.Repeat("bounded fixture output ", MaxInlineBytes/10+1) + "')\n"
	ctx, plan := b1678MakeContext(t, checker, "")
	ctx.WorkDir = t.TempDir()
	b1678InstallNativeAssertion(t, ctx, plan)
	// This branch tests existing broad native discovery after a Make-only
	// selection. The separate positive above pins an exact declared assertion.
	plan.BehaviorContracts, plan.ProjectTestObservations = nil, nil
	ctx.Mutable.SetChangePlan(plan)
	result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": "make", "suite": "check"}))
	if err != nil {
		t.Fatal(err)
	}
	report := ctx.Mutable.ChangeReport()
	b1678AssertNativeReceipt(t, plan, report, result.Success)
	b1678AssertMakeProcessPreserved(t, report)
	found := false
	for _, command := range report.ExecutedCommands {
		found = found || (command.Runner == "python" && command.Outcome == types.ExecutedCommandOutcomeExecuted && command.Source == "execution_capability_escalation")
	}
	if !found {
		t.Fatalf("unknown Make capability must continue the discovered native suite: %+v", report.ExecutedCommands)
	}
	if result.RawRef == "" {
		t.Fatal("fixture must preserve the full executor disclosure artifact")
	}
	output, err := os.ReadFile(result.RawRef)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "target execution has not been established") || strings.Contains(string(output), "coverage is static/syntax-only") {
		t.Errorf("unknown aggregate capability was described as proven static-only: %s", output[len(output)-min(len(output), 1800):])
	}
}

func b1678InstallNativeAssertion(t *testing.T, ctx *types.BusContext, plan *types.ChangePlan) {
	t.Helper()
	const testPath = "tests/test_widget.py"
	path := filepath.Join(ctx.RepoRoot, filepath.FromSlash(testPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "tests", "__init__.py"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("import unittest\nimport widget\n\nclass WidgetTest(unittest.TestCase):\n    def test_increment(self):\n        self.assertEqual(widget.increment(2), 3)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan.BehaviorContracts = []types.WriteBehaviorContract{{ID: "increment-result", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpEquals, Expected: "3", Required: true, Source: "write_analyzer"}}
	plan.ProjectTestObservations = []types.ProjectTestObservation{{ID: "native-increment", TestPath: testPath, AssertionSuite: "WidgetTest", AssertionID: "test_increment", ContractRefs: []string{"increment-result"}}}
	ctx.Mutable.SetChangePlan(plan)
}

func b1678AssertNativeReceipt(t *testing.T, plan *types.ChangePlan, report *types.ChangeReport, success bool) {
	t.Helper()
	if report == nil || !report.Passed || !success {
		t.Fatalf("native test failed: success=%v report=%+v", success, report)
	}
	assertion := false
	for _, row := range report.TestResults {
		assertion = assertion || (row.Passed && row.ObservationScope == types.TestObservationScopeAssertion && row.AssertionID == "test_increment")
	}
	if !assertion || !report.HasTargetExecutionCoverage() {
		t.Fatalf("actual native assertion receipt lost: %+v", report)
	}
	covered := types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, report.VerificationConfidence)
	if _, ok := covered["increment-result"]; len(plan.BehaviorContracts) > 0 && !ok {
		t.Errorf("exact project assertion did not preserve compatible contract witness: %+v", report.VerificationConfidence)
	}
}

const b1678StaticChecker = "import ast\nfrom pathlib import Path\nsource = Path('widget.py').read_text()\ntree = ast.parse(source)\nassert isinstance(tree, ast.Module)\nassert any(isinstance(node, ast.FunctionDef) and node.name == 'increment' for node in tree.body)\nprint('declared source check passed')\n"

const b1678CallingChecker = "import sys\nsys.path.insert(0, '.')\nimport widget\nassert widget.increment(2) == 3\nprint('declared project check passed')\n"

func b1678MakeContext(t *testing.T, checker, probe string) (*types.BusContext, *types.ChangePlan) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make unavailable")
	}
	root := t.TempDir()
	const source = "def increment(value):\n    from pathlib import Path\n    Path('.target-called').write_text('executed')\n    return value + 1\n"
	for path, contents := range map[string]string{
		"widget.py":              source,
		"checks/check_widget.py": checker,
		"Makefile":               "check: widget.py checks/check_widget.py\n\t@python3 checks/check_widget.py\n",
	} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	plan := &types.ChangePlan{
		ID: "plan-b1678", Status: types.PlanStatusApplied,
		TargetPaths: []string{"widget.py"},
		Changes:     []types.FileChange{{Path: "widget.py", Kind: "modify", NewContent: source}},
	}
	if probe != "" {
		// Preserve the independent project-suite obligation, so the real
		// aggregate and the probe coexist in this public execution report.
		plan.BehaviorContracts = []types.WriteBehaviorContract{{ID: "increment-result", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpEquals, Expected: "3", Required: true, Source: "write_analyzer"}}
		plan.VerificationProbes = []types.VerificationProbe{{ID: "target-probe", Language: "python", Code: probe, ChangedSymbolRefs: []string{"path:widget.py"}}}
	}
	ctx := &types.BusContext{Mutable: types.NewMutableState("opaque aggregate proof boundary"), Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root}
	b1575BindAppliedPythonLines(t, ctx, plan, "widget.py", []int{4})
	return ctx, plan
}

func b1678AssertMakeProcessPreserved(t *testing.T, report *types.ChangeReport) {
	t.Helper()
	command, aggregate := false, false
	for _, row := range report.ExecutedCommands {
		if row.Runner == "make" && row.Suite == "check" && row.ExitCode == 0 && row.Outcome == types.ExecutedCommandOutcomeExecuted {
			command = len(row.CoveredPaths) == 1 && row.CoveredPaths[0] == "widget.py"
		}
	}
	for _, row := range report.TestResults {
		aggregate = aggregate || (row.AssertionID == "make-test" && row.Suite == "check" && row.Passed && row.ObservationScope == types.TestObservationScopeAggregate)
	}
	if !command || !aggregate {
		t.Errorf("original successful Make command/aggregate/path scope changed: command=%v aggregate=%v report=%+v", command, aggregate, report)
	}
}

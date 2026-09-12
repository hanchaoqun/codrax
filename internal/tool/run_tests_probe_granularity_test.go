package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Process success and declared refs cannot establish execution or assertion
// ownership. These legacy (no applied-effect) probes retain their real process
// outcome without acquiring contract proof, even through report projections.
func TestRunTestsProbeGranularityActualExecutionAndProjection(t *testing.T) {
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable python on PATH")
	}
	for _, tc := range []struct {
		name, code, changed string
		want                bool
		dryRun              bool
	}{
		{"dynamic", "import widget\nassert widget.value() == 42\n", "path:widget.py", true, false},
		{"AST only", "import ast\nfrom pathlib import Path\ntree = ast.parse(Path('widget.py').read_text())\nassert isinstance(tree.body[0], ast.FunctionDef)\n", "path:widget.py", true, false},
		{"failed", "import widget\nassert widget.value() == 0\n", "path:widget.py", false, false},
		{"unavailable", "raise RuntimeError('probe setup unavailable')\n", "path:widget.py", false, false},
		{"uncoupled", "assert True\n", "path:unrelated.py", false, false},
		{"planner dynamic", "import widget\nassert widget.value() == 42\n", "path:widget.py", true, true},
		{"planner failed", "import widget\nassert widget.value() == 0\n", "path:widget.py", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for path, body := range map[string]string{
				"widget.py":            "def value():\n    return 42\n",
				"tests/__init__.py":    "",
				"tests/test_widget.py": "import unittest\nclass WidgetTest(unittest.TestCase):\n    def test_available(self):\n        self.assertTrue(True)\n",
			} {
				full := filepath.Join(root, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			plan := &types.ChangePlan{
				ID: "plan-granularity", Status: types.PlanStatusApplied, TargetPaths: []string{"widget.py"},
				BehaviorContracts: []types.WriteBehaviorContract{
					{ID: "value", Kind: types.WriteBehaviorObservable, Operator: types.WriteBehaviorOpEquals,
						Polarity: types.WriteBehaviorPolarityExpected, Expected: "42", Required: true},
					{ID: "shape", Kind: types.WriteBehaviorObservable, Operator: types.WriteBehaviorOpSatisfies,
						Polarity: types.WriteBehaviorPolarityExpected, Expected: "returns a number", Required: true},
				},
				VerificationProbes: []types.VerificationProbe{{
					ID: "value-probe", Language: "python", Code: tc.code,
					ContractRefs: []string{"value", "shape"}, ChangedSymbolRefs: []string{tc.changed},
				}},
			}
			mu := types.NewMutableState("probe execution granularity")
			mu.SetChangePlan(plan)
			ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root}
			params := map[string]any{"runner": "python", "framework": "unittest"}
			if tc.dryRun {
				ctx.PipelineStage = types.StagePlan
				params = map[string]any{"dry_run": true, "verification_probe": plan.VerificationProbes[0]}
			}
			result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, params))
			if err != nil {
				t.Fatal(err)
			}
			report := mu.ChangeReport()
			if tc.dryRun {
				if report != nil || len(mu.PlanStageProbeReports()) != 1 {
					t.Fatalf("planner report changed installation lane: %+v", report)
				}
				report = mu.PlanStageProbeReports()[0]
			}
			if report == nil {
				t.Fatalf("no installed report: %+v", result)
			}
			const phrase = "not per-contract/method execution receipts or runtime coverage"
			count := strings.Count(result.Summary, phrase)
			if count != 0 {
				t.Errorf("unadmitted Python refs must not retain a satisfied-ref disclosure: %s", result.Summary)
			}
			if tc.want {
				passed := false
				for _, row := range report.TestResults {
					passed = passed || (row.AssertionID == "value-probe" && row.Suite == "verification_probe/python" && row.Passed)
				}
				if !passed {
					t.Fatalf("authority limitation rewrote original probe success: %+v", report)
				}
			}
			covered := types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, report.VerificationConfidence)
			if len(covered) != 0 {
				t.Fatalf("declared refs minted assertion proof: covered=%v records=%+v", covered, report.VerificationConfidence)
			}
			before, _ := json.Marshal(report)
			for _, record := range report.VerificationConfidence {
				if strings.Contains(record.Detail, phrase) {
					t.Fatal("display explanation was persisted into original report")
				}
			}
			pack := types.WriteContextPackFromChangeReport(report)
			ledger := types.BuildVerificationProofLedger(plan, report, nil)
			packHas, ledgerHas := false, false
			for _, item := range pack.Items {
				packHas = packHas || strings.Contains(item.Text, phrase)
			}
			for _, item := range ledger.Obligations {
				ledgerHas = ledgerHas || strings.Contains(item.Detail, phrase)
			}
			if packHas || ledgerHas {
				t.Errorf("projections revived withdrawn satisfied refs: context=%v ledger=%v", packHas, ledgerHas)
			}
			if ledger.State == types.VerificationProofLedgerVerified || ledger.UncoveredCount == 0 {
				t.Errorf("plain probe closed required contract obligations: %+v", ledger)
			}
			after, _ := json.Marshal(report)
			if !bytes.Equal(before, after) {
				t.Fatal("projections changed original report")
			}
		})
	}
}

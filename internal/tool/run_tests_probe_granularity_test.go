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

// No source-inspection rule can establish per-method execution. These two
// successful probes therefore get the same honest display boundary despite
// one exercising a method and the other only inspecting its definition.
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
			if (tc.want && count != 1) || (!tc.want && count != 0) {
				t.Errorf("actual run_tests exit disclosure count=%d want=%v: %s", count, tc.want, result.Summary)
			}
			if tc.want && (!result.Success || !report.Passed) {
				t.Fatalf("display limitation must not reject a passed probe: %+v", report)
			}
			covered := types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, report.VerificationConfidence)
			if (tc.want && len(covered) != 2) || (!tc.want && len(covered) != 0) {
				t.Fatalf("proof granularity changed existing admission: covered=%v records=%+v", covered, report.VerificationConfidence)
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
			if packHas != tc.want || ledgerHas != tc.want {
				t.Errorf("actual report dropped granularity on projection: context=%v ledger=%v want=%v", packHas, ledgerHas, tc.want)
			}
			after, _ := json.Marshal(report)
			if !bytes.Equal(before, after) {
				t.Fatal("projections changed original report")
			}
		})
	}
}

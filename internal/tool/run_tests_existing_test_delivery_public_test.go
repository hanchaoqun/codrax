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

// These tests execute RunTests and real Python/git in a temporary repository.
// A delivery snapshot is controller input, never a substitute for a fresh run.
func TestExistingTestDeliveryPublicFollowup(t *testing.T) {
	ctx, source, plan := existingTestDeliveryPublicFixture(t)
	beforeSource, _ := json.Marshal(source)
	beforePlan, _ := json.Marshal(plan)
	beforeTest, _ := os.ReadFile(filepath.Join(ctx.RepoRoot, "test_widget.py"))
	var prior *types.ChangeReport
	for attempt := 0; attempt < 2; attempt++ {
		report := existingTestDeliveryPublicRun(t, ctx)
		if report.PlanID != plan.ID || report.Channel != types.ChangeReportChannelPostApplyVerify {
			t.Fatalf("current execution scope lost: %+v", report)
		}
		confidence := types.ExistingTestExecutionConfidence(plan, report)
		if len(confidence) != 1 || confidence[0].Status != "satisfied" {
			t.Errorf("fresh native execution not bound to retained delivery: %+v; receipts=%+v", confidence, report.ExistingTestExecutions)
		}
		if len(report.ExistingTestExecutions) != 1 {
			t.Errorf("expected one new native receipt, got %+v", report.ExistingTestExecutions)
		} else {
			r := report.ExistingTestExecutions[0]
			if r.PlanID != plan.ID || r.SourcePlanID != source.ID || r.AppliedCommitSHA != source.AppliedCommitSHA || r.PatchEffectID != source.PatchEffect.RecordID || r.AssertionCount != 1 {
				t.Errorf("execution/source identities conflated: %+v", r)
			}
			if prior != nil {
				old := prior.ExistingTestExecutions[0]
				if report.ExecutedCommands[r.CommandIndex].InvocationID == prior.ExecutedCommands[old.CommandIndex].InvocationID {
					t.Error("second verification reused an old native invocation")
				}
			}
		}
		resolved := types.ResolveVerificationProbeTargetExecution(plan, plan.VerificationProbes[0], report)
		if len(resolved.Paths) != 1 || resolved.Paths[0] != "widget.py" {
			t.Errorf("fresh target execution unavailable: %+v; receipts=%s", resolved, b1575TargetReceiptsJSON(report))
		}
		for _, command := range report.ExecutedCommands {
			if command.ProbeExecution == nil || command.ProbeExecution.TargetExecution == nil {
				continue
			}
			r := command.ProbeExecution.TargetExecution
			if r.PlanID != plan.ID || r.SourcePlanID != source.ID || r.SourceCommitSHA != source.AppliedCommitSHA {
				t.Errorf("target receipt must retain both owners: %+v", r)
			}
		}
		if refs := types.CoveredWriteBehaviorContractIDs(plan.BehaviorContracts, report.VerificationConfidence); len(refs) != 0 {
			t.Errorf("execution-only receipt signed behavior: %v", refs)
		}
		for _, row := range report.ChangedPathCoverage {
			if row.Caliber == types.ChangedPathVerificationProbe && row.Capability == types.VerificationCapabilityTargetBehavior {
				t.Error("target execution promoted to behavior proof")
			}
		}
		prior = report
	}
	afterPlan, _ := json.Marshal(plan)
	afterSource, _ := json.Marshal(source)
	afterTest, _ := os.ReadFile(filepath.Join(ctx.RepoRoot, "test_widget.py"))
	if !bytes.Equal(beforePlan, afterPlan) || !bytes.Equal(beforeSource, afterSource) || !bytes.Equal(beforeTest, afterTest) || plan.PatchEffect != nil || plan.AppliedCommitSHA != "" {
		t.Error("verification rewrote source plan, proof-only plan, or existing test")
	}
}

func TestExistingTestDeliveryPublicPhysicalGuards(t *testing.T) {
	for _, name := range []string{"wrong_commit", "wrong_fingerprint", "wrong_head_ref", "different_head", "dirty_source", "dirty_test", "missing_snapshot", "two_sources"} {
		t.Run(name, func(t *testing.T) {
			ctx, _, plan := existingTestDeliveryPublicFixture(t)
			source := &plan.CumulativeVerificationScope.AppliedSources[0]
			switch name {
			case "wrong_commit":
				source.AppliedCommitSHA = strings.Repeat("1", 40)
			case "wrong_fingerprint":
				source.PatchEffect.DiffFingerprint = strings.Repeat("1", 64)
			case "wrong_head_ref":
				source.PatchEffect.HeadRef = "HEAD^"
			case "different_head":
				b1575FixtureGit(t, ctx.RepoRoot, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "--allow-empty", "-qm", "later head")
			case "dirty_source", "dirty_test":
				path := "widget.py"
				if name == "dirty_test" {
					path = "test_widget.py"
				}
				data, _ := os.ReadFile(filepath.Join(ctx.RepoRoot, path))
				if err := os.WriteFile(filepath.Join(ctx.RepoRoot, path), append(data, []byte("# uncommitted\n")...), 0644); err != nil {
					t.Fatal(err)
				}
			case "missing_snapshot":
				plan.CumulativeVerificationScope.AppliedSources = nil
			case "two_sources":
				plan.CumulativeVerificationScope.AppliedSources = append(plan.CumulativeVerificationScope.AppliedSources, types.CloneVerificationDeliverySnapshot(*source))
			}
			ctx.Mutable.SetChangePlan(plan)
			report := existingTestDeliveryPublicRun(t, ctx)
			if len(report.ExistingTestExecutions) != 0 {
				t.Errorf("unbound delivery minted native receipt: %+v", report.ExistingTestExecutions)
			}
			for _, c := range types.ExistingTestExecutionConfidence(plan, report) {
				if c.Status == "satisfied" {
					t.Errorf("identity failure hidden by native PASS: %+v", c)
				}
			}
			if got := types.ResolveVerificationProbeTargetExecution(plan, plan.VerificationProbes[0], report); len(got.Paths) != 0 {
				t.Errorf("identity failure hidden by probe PASS: %+v", got)
			}
		})
	}
}

func existingTestDeliveryPublicFixture(t *testing.T) (*types.BusContext, *types.ChangePlan, *types.ChangePlan) {
	t.Helper()
	return existingTestDeliveryPublicFixtureWithTest(t, "import unittest\nfrom widget import increment\nclass Tests(unittest.TestCase):\n    def test_value(self): self.assertEqual(increment(2), 3)\n")
}

func existingTestDeliveryPublicFixtureWithTest(t *testing.T, testSource string) (*types.BusContext, *types.ChangePlan, *types.ChangePlan) {
	t.Helper()
	if _, ok := resolvePythonDryBuildRunner(); !ok {
		t.Skip("no usable Python")
	}
	root := t.TempDir()
	for path, body := range map[string]string{
		"widget.py":      "def increment(value):\n    return value + 1\n",
		"test_widget.py": testSource,
	} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	ctx := &types.BusContext{RepoRoot: root, MainRepoRoot: root, Mode: types.ModeApply, PipelineStage: types.StageVerify, Mutable: types.NewMutableState("fresh verification")}
	source := &types.ChangePlan{ID: "source-delivery", Status: types.PlanStatusApplied, TargetPaths: []string{"widget.py"}, Changes: []types.FileChange{{Path: "widget.py", Kind: "modify"}}}
	b1575BindAppliedPythonLines(t, ctx, source, "widget.py", []int{2})
	snapshot, ok := types.VerificationDeliverySnapshotFromAppliedPlan(source)
	if !ok {
		t.Fatal("real git delivery failed snapshot validation")
	}
	plan := &types.ChangePlan{
		ID: "proof-followup", Status: types.PlanStatusNoChangeRequired, PersistenceKind: types.PlanPersistenceProofProbeOnly,
		TargetPaths: []string{"widget.py"}, WorktreePath: root,
		WriteAnalysisIR:             &types.WriteAnalysisIR{Request: types.WriteRequestModel{Constraints: []types.WriteConstraint{{Kind: types.WriteConstraintRunExistingTest, Target: "test_widget.py"}}}},
		BehaviorContracts:           []types.WriteBehaviorContract{{ID: "value-result", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpEquals, Expected: "3", Required: true, Source: "write_analyzer"}},
		VerificationProbes:          []types.VerificationProbe{{ID: "target-probe", Language: "python", Code: "import widget\nassert widget.increment(2) == 3\n", ChangedSymbolRefs: []string{"path:widget.py"}, ContractRefs: []string{"value-result"}}},
		CumulativeVerificationScope: &types.CumulativeVerificationScope{SourcePlanIDs: []string{source.ID}, TargetPaths: []string{"widget.py"}, AppliedSources: []types.VerificationDeliverySnapshot{snapshot}},
	}
	if _, ok := types.ResolveVerificationDelivery(plan); !ok {
		t.Fatal("invalid proof-only fixture")
	}
	ctx.Mutable.SetChangePlan(plan)
	return ctx, source, plan
}

func existingTestDeliveryPublicRun(t *testing.T, ctx *types.BusContext) *types.ChangeReport {
	t.Helper()
	result, err := (&RunTests{}).Execute(ctx, json.RawMessage(`{}`))
	if err != nil || ctx.Mutable.ChangeReport() == nil {
		t.Fatalf("public RunTests failed: %+v %v", result, err)
	}
	encoded, err := json.Marshal(ctx.Mutable.ChangeReport())
	if err != nil {
		t.Fatal(err)
	}
	var report types.ChangeReport
	if err := json.Unmarshal(encoded, &report); err != nil {
		t.Fatal(err)
	}
	return &report
}

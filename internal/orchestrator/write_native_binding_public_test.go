package orchestrator

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

const nativeBindingPublicSource = "packages/value/value.py"
const nativeBindingPublicTest = "packages/value/tests/test_value.py"
const nativeBindingPublicContract = "reject-fraction"

// This is an honest-unverified baseline for the still-unimplemented read-only
// native-assertion binding lane, not a successful follow-up implementation.
// Only model choices are scripted: Run, public tools, git and Python create the
// source plan, applied worktree, native assertions and changed-target receipt.
// The controller requests honest unverified completion; this does not claim to
// prove automatic discovery/rebinding of the missing native declaration.
func TestWriteNativeBindingPublicNoDeclarationRemainsUnverified(t *testing.T) {
	for _, name := range []string{"git", "python3"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skip(name + " unavailable")
		}
	}
	root := t.TempDir()
	files := map[string]string{
		".gitignore":                       "__pycache__/\n*.pyc\n.codrax/\n",
		nativeBindingPublicSource:          "def coerce_integer(value):\n    if isinstance(value, float) and not value.is_integer():\n        raise TypeError(\"fractional value\")\n    return int(value)\n",
		"packages/value/setup.py":          "from setuptools import setup\nsetup(name='value', version='0.1.0', py_modules=['value'])\n",
		"packages/value/tests/__init__.py": "",
		nativeBindingPublicTest: `import unittest
from value import coerce_integer

class CoerceTest(unittest.TestCase):
    def test_reject_fraction(self):
        with self.assertRaises(ValueError):
            coerce_integer(1.5)
    def test_integral_float(self):
        self.assertEqual(coerce_integer(2.0), 2)
        self.assertIsInstance(coerce_integer(2.0), int)
    def test_integer_domain(self):
        for value in (-2**64, -1, 0, 1, 2**64):
            with self.subTest(value=value):
                self.assertEqual(coerce_integer(value), value)
`,
	}
	for path, body := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "user.name=Codrax Test", "-c", "user.email=codrax-test@example.invalid"}, args...)...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "main")
	git("add", ".")
	git("commit", "-qm", "baseline")
	seed := git("rev-parse", "HEAD")
	request := "Correct coerce_integer so coerce_integer(1.5) raises ValueError; preserve integer and integer-valued-float behavior. Only change implementation, keep existing tests unchanged, and run " + nativeBindingPublicTest + "."
	var o *Orchestrator
	var sourcePlan *types.ChangePlan
	var firstReport *types.ChangeReport
	postHookChecked := false
	plannerCalls, coderCalls, verifyCalls := 0, 0, 0
	execute := func(call func(*types.BusContext, json.RawMessage) (types.ToolResult, error), params any) types.ToolResult {
		t.Helper()
		raw, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		result, err := call(o.busCtx, raw)
		if err != nil || (!result.Success && result.ToolName != "run_tests") {
			t.Fatalf("HARNESS: real public tool rejected: err=%v result=%+v", err, result)
		}
		o.busCtx.Mutable.AppendDispatchToolResult(result)
		return result
	}
	read := func(path string) types.ToolResult {
		return execute((&tool.ReadFile{}).Execute, map[string]any{"path": path})
	}
	assertWorktreeBytes := func() {
		t.Helper()
		for path, body := range files {
			want := body
			if path == nativeBindingPublicSource {
				want = strings.Replace(body, "raise TypeError", "raise ValueError", 1)
			}
			actual, err := os.ReadFile(filepath.Join(o.busCtx.RepoRoot, path))
			if err != nil || string(actual) != want {
				t.Fatalf("applied implementation or protected bytes changed: %s err=%v got=%q", path, err, actual)
			}
		}
	}
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentWriteAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			results := []types.ToolResult{read(nativeBindingPublicSource), read(nativeBindingPublicTest)}
			results = append(results, execute((&tool.EmitWriteAnalysis{}).Execute, map[string]any{
				"task":          map[string]any{"kind": "bugfix", "scope": "micro", "summary": "Correct the existing exception type."},
				"risk":          map[string]any{"overall": "low"},
				"scope_anchors": []string{nativeBindingPublicSource, nativeBindingPublicTest},
				"constraints": []map[string]any{
					{"kind": "run_existing_test", "target": nativeBindingPublicTest},
					{"kind": "preserve_regression_test", "target": nativeBindingPublicTest},
				},
				"behavior_contracts": []types.WriteBehaviorContract{{ID: nativeBindingPublicContract, Kind: types.WriteBehaviorException,
					Polarity: types.WriteBehaviorPolarityExpected, Subject: "coerce_integer(1.5)", Operator: types.WriteBehaviorOpRaises, Expected: "ValueError", Required: true}},
				"phase_proposal": map[string]any{"split": "single"},
			}))
			return &agent.StageOutput{ToolResults: results}, nil
		},
		types.AgentPlanner: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			plannerCalls++
			if plannerCalls != 1 {
				t.Fatalf("execution-only probe must not force an impossible repeated plan: calls=%d", plannerCalls)
			}
			results := []types.ToolResult{read(nativeBindingPublicSource), read(nativeBindingPublicTest)}
			results = append(results, execute((&tool.EmitChangePlan{}).Execute, map[string]any{
				"request": request, "summary": "Correct only the exception type; keep every existing test unchanged.",
				"changes": []map[string]any{{"path": nativeBindingPublicSource, "kind": "patch", "rationale": "Reject a fractional value with the required exception.",
					"edits": []map[string]any{{"kind": "replace", "start_line": 3, "old_text": "        raise TypeError(\"fractional value\")", "content": "        raise ValueError(\"fractional value\")"}}}},
				"verification_probes": []types.VerificationProbe{{ID: "exercise-rejection", Language: "python", WorkingDir: "packages/value",
					Code:         "from value import coerce_integer\ntry:\n    coerce_integer(1.5)\nexcept ValueError:\n    pass\nelse:\n    raise AssertionError('fraction was accepted')\nassert coerce_integer(2.0) == 2\n",
					ContractRefs: []string{nativeBindingPublicContract}, ChangedSymbolRefs: []string{"path:" + nativeBindingPublicSource}}},
				"project_test_observations": []any{},
			}))
			return &agent.StageOutput{ToolResults: results}, nil
		},
		types.AgentCoder: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			coderCalls++
			result := execute((&tool.ApplyPatch{}).Execute, map[string]any{"path": nativeBindingPublicSource, "kind": "patch"})
			assertWorktreeBytes()
			return &agent.StageOutput{ToolResults: []types.ToolResult{result}}, nil
		},
		types.AgentVerifier: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			verifyCalls++
			result := execute((&tool.RunTests{}).Execute, map[string]any{})
			assertWorktreeBytes()
			if firstReport == nil {
				raw, _ := json.Marshal(ctx.Mutable.ChangePlan())
				if err := json.Unmarshal(raw, &sourcePlan); err != nil {
					t.Fatal(err)
				}
				raw, _ = json.Marshal(ctx.Mutable.ChangeReport())
				if err := json.Unmarshal(raw, &firstReport); err != nil {
					t.Fatal(err)
				}
				// The current public source-free shape still rejects native
				// declarations. Preserve this negative control until a separate
				// controller-authorized shape is implemented; never forge a grant.
				before, _ := json.Marshal([]any{ctx.Mutable.ChangePlan(), ctx.Mutable.ChangeReport()})
				p := map[string]any{"summary": "Bind the unchanged native assertion without edits.", "changes": []any{},
					"project_test_observations": []types.ProjectTestObservation{{ID: "existing-rejection", TestPath: nativeBindingPublicTest,
						AssertionSuite: "python/unittest@packages/value::tests.test_value.CoerceTest", AssertionID: "python/unittest@packages/value::test_reject_fraction", ContractRefs: []string{nativeBindingPublicContract}}}}
				raw, _ = json.Marshal(p)
				planCtx := o.busCtx.ShallowClone()
				planCtx.PipelineStage = types.StagePlan
				for _, emit := range []func(*types.BusContext, json.RawMessage) (types.ToolResult, error){(&tool.EmitChangePlan{}).Execute, (&tool.EmitPlanSkeleton{}).Execute} {
					rejected, err := emit(planCtx, raw)
					if err != nil || rejected.Success || !strings.Contains(rejected.Summary, "project_test_observations cannot be carried by a source-free sentinel plan") {
						t.Fatalf("ungranted read-only binding boundary changed: %v %+v", err, rejected)
					}
					t.Logf("UNIMPLEMENTED_NATIVE_BINDING_CONTROL tool=%s summary=%s", rejected.ToolName, rejected.Summary)
				}
				after, _ := json.Marshal([]any{ctx.Mutable.ChangePlan(), ctx.Mutable.ChangeReport()})
				if !bytes.Equal(before, after) {
					t.Fatal("rejected declaration altered the current source plan or real report")
				}
			}
			return &agent.StageOutput{ToolResults: []types.ToolResult{result}}, nil
		},
		types.AgentWriteController: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			if firstReport != nil && !postHookChecked {
				// The public verify post-hook refreshes impact/review projections.
				// Observe those real persisted projections, not the intermediate
				// pre-hook plan held while the verifier was still executing.
				raw, _ := json.Marshal(ctx.Mutable.ChangePlan())
				if err := json.Unmarshal(raw, &sourcePlan); err != nil {
					t.Fatal(err)
				}
				raw, _ = json.Marshal(ctx.Mutable.ChangeReport())
				if err := json.Unmarshal(raw, &firstReport); err != nil {
					t.Fatal(err)
				}
				nativeBindingPublicAssertDebt(t, sourcePlan, firstReport)
				postHookChecked = true
			}
			out, err := defaultTestWriteController(ctx, sk)
			if err != nil {
				return nil, err
			}
			var decision writeflow.WriteWorkflowDecision
			if err := json.Unmarshal(out.Data, &decision); err != nil {
				t.Fatal(err)
			}
			if ctx.Mutable.ChangeReport() != nil {
				decision = writeflow.WriteWorkflowDecision{Action: writeflow.ActionFinish, FinishDisposition: writeflow.FinishDispositionAcceptUnverified, ReasonCode: "native_declaration_not_supplied"}
			}
			result := execute((&tool.EmitWriteWorkflowDecision{}).Execute, decision)
			return &agent.StageOutput{Data: ctx.Mutable.WriteWorkflowDecisionJSON(), ToolResults: []types.ToolResult{result}}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o = New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.SetMode(types.ModeApply)
	o.SetMaxSteps(30)
	o.SetWorktreeBase(filepath.Join(t.TempDir(), "worktrees"))
	bus, err := o.Run(request, root, "main")
	if err != nil || bus == nil || firstReport == nil || !postHookChecked || plannerCalls != 1 || coderCalls != 1 || verifyCalls < 1 {
		t.Fatalf("HARNESS: complete real source/apply/verify path unavailable: err=%v planner=%d coder=%d verifier=%d", err, plannerCalls, coderCalls, verifyCalls)
	}
	run := bus.Mutable.WriteWorkflowRun()
	if run == nil || run.Status != types.WriteWorkflowRunComplete || run.Completion == nil || run.Completion.Verdict != types.WriteWorkflowCompletionUnverified {
		t.Fatalf("missing native binding must remain explicitly unverified: %+v", run)
	}
	nativeBindingPublicAssertDebt(t, sourcePlan, firstReport)
	if current := bus.Mutable.ChangePlan(); current != nil && types.BuildVerificationProofLedger(current, bus.Mutable.ChangeReport(), nil).State == types.VerificationProofLedgerVerified {
		t.Fatal("final cumulative projection silently discharged missing native assertion binding")
	}
	if git("rev-parse", "HEAD") != seed {
		t.Fatal("Run changed the seed checkout HEAD")
	}
	for path, body := range files {
		got, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || string(got) != body {
			t.Fatalf("Run changed seed bytes %s: %v", path, err)
		}
	}
	t.Logf("REAL_RUN_NATIVE_BINDING_BASELINE source_plan=%s verify_calls=%d original_assertions=3 native_pass=true changed_target_executed=true required_contract=%s unique_uncovered_contracts=1 completion=%s", sourcePlan.ID, verifyCalls, nativeBindingPublicContract, run.Completion.Verdict)
}

func nativeBindingPublicAssertDebt(t *testing.T, plan *types.ChangePlan, report *types.ChangeReport) {
	t.Helper()
	if plan == nil || report == nil || plan.ID != report.PlanID || report.Channel != types.ChangeReportChannelPostApplyVerify || !report.Passed || len(plan.ProjectTestObservations) != 0 {
		t.Fatalf("HARNESS: initial current native pass/no-declaration premise failed: plan=%+v report=%+v", plan, report)
	}
	if ids := types.RequiredWriteBehaviorContractIDs(plan.BehaviorContracts, true); len(ids) != 1 {
		t.Fatalf("HARNESS: request-grounded required contract was lost: %+v", plan.BehaviorContracts)
	}
	passed := map[string]bool{}
	for _, row := range report.TestResults {
		if row.ObservationScope == types.TestObservationScopeAssertion && row.Suite == "python/unittest@packages/value::tests.test_value.CoerceTest" && row.Passed {
			passed[row.AssertionID] = true
		}
	}
	for _, name := range []string{"test_reject_fraction", "test_integral_float", "test_integer_domain"} {
		if !passed["python/unittest@packages/value::"+name] {
			t.Fatalf("HARNESS: real original assertion %s did not pass: %+v", name, report.TestResults)
		}
	}
	if len(report.ExistingTestExecutions) != 1 {
		t.Fatalf("HARNESS: exact existing-file native execution receipt missing: %+v", report.ExistingTestExecutions)
	}
	receipt := report.ExistingTestExecutions[0]
	if receipt.PlanID != plan.ID || receipt.AppliedCommitSHA != plan.AppliedCommitSHA || receipt.TestPath != nativeBindingPublicTest ||
		receipt.AssertionCount != 3 || receipt.FailedAssertionCount != 0 || len(receipt.AssertionDigests) != 3 {
		t.Fatalf("HARNESS: native receipt does not describe this applied file and three original tests: %+v", receipt)
	}
	confidence := types.ExistingTestExecutionConfidence(plan, report)
	if len(confidence) != 1 || confidence[0].Status != "satisfied" {
		t.Fatalf("HARNESS: actual native receipt did not survive independent current-delivery validation: %+v", confidence)
	}
	if len(plan.VerificationProbes) != 1 {
		t.Fatal("HARNESS: changed-target probe missing")
	}
	resolution := types.ResolveVerificationProbeTargetExecution(plan, plan.VerificationProbes[0], report)
	if len(resolution.Paths) != 1 || resolution.Paths[0] != nativeBindingPublicSource {
		t.Fatalf("HARNESS: actual changed owner was not executed: %+v", resolution)
	}
	ledger := types.BuildVerificationProofLedger(plan, report, nil)
	if ledger.State != types.VerificationProofLedgerLowConfidence || ledger.UncoveredCount == 0 || ledger.FailedCount != 0 || ledger.CapabilityFailedCount != 0 {
		t.Fatalf("HARNESS: must isolate one native-binding debt, not execution/infra debt: %+v", ledger)
	}
	if types.BehaviorContractRefHasVerificationWitness(plan, report, nativeBindingPublicContract) {
		t.Fatal("native PASS or plain probe retroactively fabricated the absent contract declaration")
	}
	found := false
	for _, row := range ledger.Obligations {
		if !verificationProofLedgerObligationNeedsFollowup(row.Status) {
			continue
		}
		// One required contract can have several typed projections. Do not
		// confuse the number of those records with independent obligations.
		if row.Kind != "behavior_contract" || row.ContractRef != nativeBindingPublicContract {
			t.Fatalf("HARNESS: non-binding debt remains after the real verify post-hook: %+v", row)
		}
		found = true
	}
	if !found {
		t.Fatalf("HARNESS: the remaining debt is not the requested exception contract: %+v", ledger)
	}
}

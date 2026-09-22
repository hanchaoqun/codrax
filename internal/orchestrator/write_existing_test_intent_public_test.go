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

// Only model choices are scripted. Run, the public emit/apply/verify tools,
// worktree commits, and Python produce every accepted plan and execution receipt.
// No completion flag, report, applied-line mapping, or test outcome is fabricated.
func TestWriteExistingTestIntentPublicRun(t *testing.T) {
	for _, tc := range []struct {
		name, intent                 string
		nativeObservation, smallOnly bool
	}{
		{name: "required_native_pass", intent: "run_existing_test"},
		{name: "required_native_failure_survives_probe_pass", intent: "run_existing_test", smallOnly: true},
		{name: "legacy_probe_only"},
		{name: "preservation_is_not_execution", intent: "preserve_regression_test"},
		{name: "existing_observation_route", nativeObservation: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runExistingTestIntentPublic(t, tc.intent, tc.nativeObservation, tc.smallOnly)
			if got.plan == nil || got.report == nil || got.verifyCalls != 1 {
				t.Fatalf("HARNESS: real apply/verify did not finish once: %+v", got)
			}
			if got.report.PlanID != got.plan.ID || got.report.Channel != types.ChangeReportChannelPostApplyVerify {
				t.Fatalf("HARNESS: report identity/channel mismatch: %+v", got.report)
			}
			if len(got.plan.ProjectTestObservations) != 0 && !tc.nativeObservation {
				t.Fatal("HARNESS: planner did not drop observation declarations")
			}
			if len(got.plan.BehaviorContracts) == 0 || len(types.RequiredWriteBehaviorContractIDs(got.plan.BehaviorContracts, true)) != 0 {
				t.Fatalf("HARNESS: grounded behavior debt would mask the execution-intent gap: %+v", got.plan.BehaviorContracts)
			}
			if got.plan.WriteAnalysisIR == nil || len(got.plan.WriteAnalysisIR.Request.Constraints) != boolIntExistingTestIntentPublic(tc.intent != "") {
				t.Fatal("HARNESS: request intent was not pinned through the real plan post-hook")
			}
			if tc.intent != "" && (got.plan.WriteAnalysisIR.Request.Constraints[0].Kind != tc.intent || got.plan.WriteAnalysisIR.Request.Constraints[0].Target != existingTestIntentPublicTestPath) {
				t.Fatal("HARNESS: exact typed constraint changed during persistence")
			}
			probePassed, nativeCount, nativeFailed, skipped := false, 0, false, false
			for _, row := range got.report.TestResults {
				probePassed = probePassed || (row.Suite == "verification_probe/python" && row.AssertionID == "small-values" && row.Passed)
				if row.ObservationScope == types.TestObservationScopeAssertion && strings.Contains(row.Suite, "IncrementTest") {
					nativeCount++
					nativeFailed = nativeFailed || !row.Passed
				}
			}
			for _, command := range got.report.ExecutedCommands {
				skipped = skipped || command.Outcome == types.ExecutedCommandOutcomeSuiteSkipped
			}
			resolution := types.ResolveVerificationProbeTargetExecution(got.plan, got.plan.VerificationProbes[0], got.report)
			if !probePassed || len(resolution.Paths) != 1 || resolution.Paths[0] != existingTestIntentPublicSourcePath {
				t.Fatalf("HARNESS: bounded probe did not execute the actual changed owner: passed=%t resolution=%+v", probePassed, resolution)
			}
			t.Logf("ACTUAL_RUN intent=%q observation=%t small_only=%t plan=%s probe_passed=%t native_assertions=%d native_failed=%t suite_skipped=%t report_status=%s workflow_status=%s run_error=%v", tc.intent, tc.nativeObservation, tc.smallOnly, got.plan.ID, probePassed, nativeCount, nativeFailed, skipped, got.report.NormalizeVerificationStatus(), got.workflowStatus, got.err)
			wantNative := tc.intent == "run_existing_test" || tc.nativeObservation
			if wantNative && (nativeCount < 3 || skipped) {
				t.Errorf("EXISTING_TEST_EXECUTION: exact requested native tests were replaced by a bounded probe: assertions=%d skipped=%t", nativeCount, skipped)
			}
			if !wantNative && (nativeCount != 0 || !skipped) {
				t.Errorf("legacy or preservation-only request gained an execution obligation: assertions=%d skipped=%t", nativeCount, skipped)
			}
			if tc.smallOnly && (!nativeFailed || got.report.Passed || got.workflowStatus == types.WriteWorkflowRunComplete) {
				t.Logf("NATIVE_FAILURE_ROWS: %+v", got.report.TestResults)
				t.Errorf("NATIVE_FAILURE_AUTHORITY: passing small-value probe hid the required native large-integer failure: report_passed=%t native_failed=%t workflow=%s", got.report.Passed, nativeFailed, got.workflowStatus)
			}
			if !tc.smallOnly && (!got.report.Passed || got.err != nil || got.workflowStatus != types.WriteWorkflowRunComplete) {
				t.Errorf("successful native/legacy verification did not complete: report=%t workflow=%s err=%v", got.report.Passed, got.workflowStatus, got.err)
			}
		})
	}
}

const existingTestIntentPublicSourcePath = "packages/widget/widget.py"
const existingTestIntentPublicTestPath = "packages/widget/tests/test_widget.py"
const existingTestIntentPublicTests = `import unittest
from widget import increment

class IncrementTest(unittest.TestCase):
    def test_negative_integers(self):
        for value in (-5, -1, -(2**64)):
            with self.subTest(value=value):
                self.assertEqual(increment(value), value + 1)
    def test_zero(self):
        self.assertEqual(increment(0), 1)
    def test_positive_integers(self):
        for value in (1, 7, 2**64):
            with self.subTest(value=value):
                self.assertEqual(increment(value), value + 1)
`

type existingTestIntentPublicResult struct {
	plan              *types.ChangePlan
	report            *types.ChangeReport
	verifyCalls       int
	workflowStatus    types.WriteWorkflowRunStatus
	completionVerdict types.WriteWorkflowCompletionVerdict
	err               error
}

func runExistingTestIntentPublic(t *testing.T, intent string, observations, smallOnly bool) existingTestIntentPublicResult {
	return runExistingTestIntentPublicFixture(t, intent, observations, smallOnly, existingTestIntentPublicTestPath, existingTestIntentPublicTests, nil)
}

func runExistingTestIntentPublicFixture(t *testing.T, intent string, observations, smallOnly bool, testPath, testBody string, extras map[string]string, verifyParams ...map[string]any) existingTestIntentPublicResult {
	t.Helper()
	for _, executable := range []string{"git", "python3"} {
		if _, err := exec.LookPath(executable); err != nil {
			t.Skip(executable + " unavailable")
		}
	}
	root := t.TempDir()
	files := map[string]string{
		".gitignore":                        "__pycache__/\n*.pyc\n.codrax/\n",
		existingTestIntentPublicSourcePath:  "def increment(value):\n    return value\n",
		testPath:                            testBody,
		"packages/widget/tests/__init__.py": "",
		"packages/widget/setup.py":          "from setuptools import setup\nsetup(name='widget', version='0.1.0', py_modules=['widget'])\n",
	}
	for path, body := range extras {
		files[path] = body
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
	var got existingTestIntentPublicResult
	var o *Orchestrator
	execute := func(call func(*types.BusContext, json.RawMessage) (types.ToolResult, error), params any) types.ToolResult {
		t.Helper()
		raw, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		result, err := call(o.busCtx, raw)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Success && result.ToolName != "run_tests" {
			t.Fatalf("HARNESS: public %s rejected: %s", result.ToolName, result.Summary)
		}
		o.busCtx.Mutable.AppendDispatchToolResult(result)
		return result
	}
	read := func(path string) types.ToolResult {
		return execute((&tool.ReadFile{}).Execute, map[string]any{"path": path})
	}
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentWriteAnalyzer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			results := []types.ToolResult{read(existingTestIntentPublicSourcePath), read(testPath)}
			constraints := []map[string]any{}
			if intent != "" {
				constraints = append(constraints, map[string]any{"kind": intent, "target": testPath, "note": "Keep the existing test file as requested."})
			}
			results = append(results, execute((&tool.EmitWriteAnalysis{}).Execute, map[string]any{
				"task":          map[string]any{"kind": "bugfix", "scope": "micro", "summary": "Correct the existing increment implementation."},
				"risk":          map[string]any{"overall": "low", "affects_public_api": false, "changes_persistence": false, "changes_build_system": false},
				"scope_anchors": []string{existingTestIntentPublicSourcePath, testPath}, "constraints": constraints,
				"behavior_contracts": []map[string]any{{"id": "increment-domain", "kind": "observable", "operator": "satisfies", "expected": "All inspected integer examples should retain increment semantics."}},
				"phase_proposal":     map[string]any{"split": "single"},
			}))
			return &agent.StageOutput{ToolResults: results}, nil
		},
		types.AgentPlanner: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			results := []types.ToolResult{read(existingTestIntentPublicSourcePath), read(testPath)}
			after := "    return value + 1"
			if smallOnly {
				after = "    return value + 1 if value.bit_length() <= 32 else value"
			}
			params := map[string]any{
				"request": "Correct increment in its existing project.", "summary": "Modify the implementation only.",
				"changes":             []map[string]any{{"path": existingTestIntentPublicSourcePath, "kind": "patch", "rationale": "Return the incremented input.", "edits": []map[string]any{{"kind": "replace", "start_line": 2, "old_text": "    return value", "content": after}}}},
				"acceptance_tests":    []string{"Run the unchanged existing negative, zero, and positive integer tests."},
				"verification_probes": []types.VerificationProbe{{ID: "small-values", Language: "python", WorkingDir: "packages/widget", Code: "from widget import increment\nfor value in (-5, -1, 0, 1, 7):\n    assert increment(value) == value + 1\n", ContractRefs: []string{"increment-domain"}, ChangedSymbolRefs: []string{"path:" + existingTestIntentPublicSourcePath}}},
			}
			if observations {
				params["project_test_observations"] = []types.ProjectTestObservation{{ID: "native-zero", TestPath: testPath, AssertionSuite: "python/unittest@packages/widget::tests.test_widget.IncrementTest", AssertionID: "python/unittest@packages/widget::test_zero", ContractRefs: []string{"increment-domain"}}}
			}
			results = append(results, execute((&tool.EmitChangePlan{}).Execute, params))
			return &agent.StageOutput{ToolResults: results}, nil
		},
		types.AgentCoder: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			result := execute((&tool.ApplyPatch{}).Execute, map[string]any{"path": existingTestIntentPublicSourcePath, "kind": "patch"})
			return &agent.StageOutput{ToolResults: []types.ToolResult{result}}, nil
		},
		types.AgentVerifier: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			got.verifyCalls++
			params := map[string]any{}
			if len(verifyParams) > 0 {
				params = verifyParams[0]
			}
			result := execute((&tool.RunTests{}).Execute, params)
			// JSON round trips inspect the actual persisted shapes, not a second
			// fixture projection or hand-built authority surface.
			raw, _ := json.Marshal(ctx.Mutable.ChangePlan())
			if err := json.Unmarshal(raw, &got.plan); err != nil {
				t.Fatal(err)
			}
			raw, _ = json.Marshal(ctx.Mutable.ChangeReport())
			if err := json.Unmarshal(raw, &got.report); err != nil {
				t.Fatal(err)
			}
			for path, body := range files {
				if path == existingTestIntentPublicSourcePath {
					continue
				}
				actual, err := os.ReadFile(filepath.Join(o.busCtx.RepoRoot, path))
				if err != nil || !bytes.Equal(actual, []byte(body)) {
					t.Fatalf("protected fixture bytes changed: %s err=%v", path, err)
				}
			}
			return &agent.StageOutput{ToolResults: []types.ToolResult{result}}, nil
		},
		types.AgentWriteController: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			out, err := defaultTestWriteController(ctx, sk)
			if err != nil {
				return nil, err
			}
			var decision writeflow.WriteWorkflowDecision
			if err := json.Unmarshal(out.Data, &decision); err != nil {
				t.Fatal(err)
			}
			if ctx.Mutable.ChangeReport() != nil && !ctx.Mutable.ChangeReport().Passed {
				decision = writeflow.WriteWorkflowDecision{Action: writeflow.ActionBlock, ReasonCode: "observed_native_failure"}
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
	bus, err := o.Run("Correct increment using only implementation edits; run the existing tests and report what actually ran.", root, "main")
	got.err = err
	if bus != nil && bus.Mutable.WriteWorkflowRun() != nil {
		got.workflowStatus = bus.Mutable.WriteWorkflowRun().Status
		if completion := bus.Mutable.WriteWorkflowRun().Completion; completion != nil {
			got.completionVerdict = completion.Verdict
		}
	}
	if git("rev-parse", "HEAD") != seed {
		t.Fatal("Run modified the seed checkout HEAD")
	}
	for path, body := range files {
		actual, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || !bytes.Equal(actual, []byte(body)) {
			t.Fatalf("Run modified seed file %s: %v", path, err)
		}
	}
	return got
}

func boolIntExistingTestIntentPublic(value bool) int {
	if value {
		return 1
	}
	return 0
}

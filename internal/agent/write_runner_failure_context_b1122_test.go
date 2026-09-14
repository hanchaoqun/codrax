package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Real native runners feed the public tool result, durable report and agent
// instructions. No model, compiler-specific error recognizer or live eval is
// needed to demonstrate that a runner's explanatory output was dropped.
func TestB1122PublicRunnerFailureContextPreservesObservedCounterexample(t *testing.T) {
	for _, binary := range []string{"make", "python3"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skipf("native fixture requires %s: %v", binary, err)
		}
	}
	const counterexample = "slot=leaf; observed=41; expected=42"
	for _, tc := range []struct {
		name    string
		runner  string
		padding int
	}{
		{"make_short_output", "make", 0},
		{"make_counterexample_after_progress", "make", 30},
		{"python_assertion_output", "python", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, mu, root := b1122RunNative(t, tc.runner, tc.padding, counterexample, false)
			report := mu.ChangeReport()
			if result.Success || report.Passed || report.BuildFailed || report.NormalizeVerificationStatus() != types.VerificationStatusFailed {
				t.Fatalf("native check failure must retain its real verdict, not become a compile failure: success=%t report=%+v", result.Success, report)
			}
			if report.Channel != types.ChangeReportChannelPostApplyVerify || report.PlanID != "b1122-plan" {
				t.Fatalf("public runner lost post-apply report identity: %+v", report)
			}
			if len(report.TestResults) != 1 || report.TestResults[0].Passed {
				t.Fatalf("failure observation invented/dropped test results: %+v", report.TestResults)
			}
			if tc.runner == "make" && report.TestResults[0].ObservationScope != types.TestObservationScopeAggregate {
				t.Fatal("a Make output excerpt was promoted to an assertion witness")
			}
			commands := 0
			for _, command := range report.ExecutedCommands {
				if command.Runner == tc.runner && command.Outcome == types.ExecutedCommandOutcomeExecuted {
					commands++
					if command.ExitCode == 0 {
						t.Fatal("failed native command was converted to success")
					}
				}
			}
			if commands != 1 {
				t.Fatalf("wanted exactly one real native command; got %d", commands)
			}

			// This Summary is the verifier's actual post-tool model context;
			// BuildInitialInstruction runs before this command and cannot repair it.
			if !strings.Contains(result.Summary, counterexample) {
				t.Errorf("verifier tool message discarded the executed counterexample: %q", result.Summary)
			}
			wire := b1122JSON(t, report)
			if !bytes.Contains(wire, []byte(counterexample)) {
				t.Error("durable report discarded the raw counterexample before any prompt cap")
			}
			var restored types.ChangeReport
			if err := json.Unmarshal(wire, &restored); err != nil {
				t.Fatal(err)
			}
			mu.SetChangeReport(&restored)
			for _, crowded := range []bool{false, true} {
				t.Run(fmt.Sprintf("crowded_%t", crowded), func(t *testing.T) {
					pack := types.WriteContextPackFromChangeReport(&restored).WithScope("batch", "")
					if crowded {
						for i := 0; i < 100; i++ {
							pack.Items = append(pack.Items, types.WriteContextItem{
								ID: fmt.Sprintf("constraint-%03d", i), Kind: "constraint", Priority: types.WriteContextP0,
								Text: "preserve the declared interface",
							})
						}
					}
					mu.SetWriteContextPack(&pack)
					mu.SetWriteWorkflowRun(&types.WriteWorkflowRun{ActiveBatchID: "batch", Batches: []types.WriteWorkflowBatch{{ID: "batch", PlanID: restored.PlanID}}})
					ctx := &types.AgentContext{Mutable: mu, Mode: types.ModeApply, RepoRoot: root}
					before := b1122JSON(t, []any{mu.ChangePlan(), mu.ChangeReport(), mu.WriteContextPack()})
					proofBefore := b1122JSON(t, types.BuildVerificationProofLedger(mu.ChangePlan(), mu.ChangeReport(), nil))
					for label, prompt := range map[string]string{
						"controller": (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil),
						"planner":    (&plannerEvaluator{}).BuildInitialInstruction(ctx, nil),
					} {
						if !strings.Contains(prompt, counterexample) {
							t.Errorf("%s context lost native counterexample (crowded=%t)", label, crowded)
						}
					}
					if !bytes.Equal(before, b1122JSON(t, []any{mu.ChangePlan(), mu.ChangeReport(), mu.WriteContextPack()})) ||
						!bytes.Equal(proofBefore, b1122JSON(t, types.BuildVerificationProofLedger(mu.ChangePlan(), mu.ChangeReport(), nil))) {
						t.Fatal("failure disclosure changed original evidence, plan or proof authority")
					}
				})
			}
		})
	}
}

func TestB1122PublicRunnerOutputTextCannotChangeVerdict(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skipf("native fixture requires make: %v", err)
	}
	for _, tc := range []struct {
		name   string
		output string
		pass   bool
	}{
		{"success_with_failure_vocabulary", "FAILURE: error: expected=42 observed=41", true},
		{"failure_without_counterexample", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, mu, _ := b1122RunNative(t, "make", 0, tc.output, tc.pass)
			if result.Success != tc.pass || mu.ChangeReport().Passed != tc.pass || mu.ChangeReport().BuildFailed {
				t.Fatalf("prose changed native verdict or invented build phase: %+v", mu.ChangeReport())
			}
			if len(mu.ChangeReport().TestResults) != 1 || mu.ChangeReport().TestResults[0].ObservationScope != types.TestObservationScopeAggregate {
				t.Fatal("display vocabulary changed assertion granularity")
			}
		})
	}
}

func TestB1122PublicFailureOutputReferenceIsReadableOrHonestlyUnavailable(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skipf("native fixture requires make: %v", err)
	}
	const detail = "slot=leaf; observed=41; expected=42"
	for _, denied := range []bool{false, true} {
		t.Run(fmt.Sprintf("artifact_denied_%t", denied), func(t *testing.T) {
			result, mu, root := b1122RunNative(t, "make", 0, detail, false, denied)
			report := mu.ChangeReport()
			if result.Success || report.Passed || !strings.Contains(result.Summary, detail) {
				t.Fatal("artifact persistence changed verdict or discarded inline failure")
			}
			if denied {
				if report.FailureSummaryBlobRef != "" || !strings.Contains(result.Summary, "unavailable (no persisted output reference)") {
					t.Fatal("failed persistence published a fake readable path")
				}
				return
			}
			ref := report.FailureSummaryBlobRef
			if ref == "" || result.RawRef != ref || !strings.Contains(result.Summary, ref) {
				t.Fatalf("short failure has no complete published output reference: report=%q raw=%q", ref, result.RawRef)
			}
			withoutRef := *report
			withoutRef.FailureSummaryBlobRef = ""
			if !bytes.Equal(b1122JSON(t, types.BuildVerificationProofLedger(mu.ChangePlan(), report, nil)), b1122JSON(t, types.BuildVerificationProofLedger(mu.ChangePlan(), &withoutRef, nil))) ||
				!bytes.Equal(b1122JSON(t, types.EffectiveVerificationConfidence(mu.ChangePlan(), report)), b1122JSON(t, types.EffectiveVerificationConfidence(mu.ChangePlan(), &withoutRef))) {
				t.Fatal("an output reference acquired proof or confidence authority")
			}
			raw, err := os.ReadFile(ref)
			if err != nil || !bytes.Contains(raw, []byte(detail)) || len(raw) >= tool.MaxInlineBytes {
				t.Fatalf("short output artifact is not complete/readable: bytes=%d err=%v", len(raw), err)
			}
			ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, RepoRoot: root, WorkDir: filepath.Dir(ref)}
			read, err := (&tool.ReadFile{}).Execute(ctx, b1122JSON(t, map[string]string{"path": ref}))
			if err != nil || !read.Success || !strings.Contains(read.Summary, detail) {
				t.Fatalf("published reference is not usable by public read_file: err=%v summary=%s", err, read.Summary)
			}
		})
	}
}

func TestB1122CurrentFailureSectionRejectsReadAndForeignReports(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    types.PipelineMode
		planID  string
		channel types.ChangeReportChannel
	}{
		{"read", types.ModeRead, "current", types.ChangeReportChannelPostApplyVerify},
		{"foreign", types.ModeApply, "other", types.ChangeReportChannelPostApplyVerify},
		{"planner_probe", types.ModeApply, "current", types.ChangeReportChannelPlannerProbe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mu := types.NewMutableState("current write task")
			mu.SetChangePlan(&types.ChangePlan{ID: "current"})
			mu.SetChangeReport(&types.ChangeReport{PlanID: tc.planID, Channel: tc.channel,
				TestResults: []types.TestResult{{FailureDetail: "unrelated report text"}}})
			ctx := &types.AgentContext{Mutable: mu, Mode: tc.mode}
			if got := buildWriteFailureObservationSection(ctx, types.WriteConsumerController); got != "" {
				t.Fatalf("%s report became current failure guidance: %s", tc.name, got)
			}
		})
	}
}

func b1122RunNative(t *testing.T, runner string, padding int, message string, pass bool, artifactDenied ...bool) (types.ToolResult, *types.MutableState, string) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{"widget.py": "VALUE = 41\n"}
	params := json.RawMessage(`{"runner":"make","suite":"check"}`)
	target := "check.sh"
	if runner == "python" {
		target = "widget.py"
		params = json.RawMessage(`{"runner":"python","framework":"unittest"}`)
		files["tests/__init__.py"] = ""
		files["tests/test_value.py"] = fmt.Sprintf("import unittest\nimport widget\nclass ValueTest(unittest.TestCase):\n    def test_value(self):\n        self.assertEqual(widget.VALUE, 42, %q)\n", message)
	} else {
		var script strings.Builder
		script.WriteString("printf '%s\\n' 'preparing local check'\n")
		for i := 0; i < padding; i++ {
			fmt.Fprintf(&script, "printf 'progress %03d\\n'\n", i)
		}
		if message != "" {
			fmt.Fprintf(&script, "printf '%%s\\n' '%s'\n", message)
		}
		if pass {
			script.WriteString("exit 0\n")
		} else {
			script.WriteString("exit 9\n")
		}
		files["check.sh"] = script.String()
		files["Makefile"] = ".PHONY: check\ncheck:\n\t@sh ./check.sh\n"
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mu := types.NewMutableState("preserve the requested behavior")
	mu.SetChangePlan(&types.ChangePlan{ID: "b1122-plan", Status: types.PlanStatusApplied,
		TargetPaths: []string{target}, AppliedPaths: []string{target}, Changes: []types.FileChange{{Path: target, Kind: "modify", NewContent: files[target]}}})
	workDir := t.TempDir()
	if len(artifactDenied) != 0 && artifactDenied[0] {
		workDir = filepath.Join(workDir, "not-a-directory")
		if err := os.WriteFile(workDir, []byte("preserve this sentinel"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify,
		RepoRoot: root, MainRepoRoot: root, WorkDir: workDir}
	result, err := (&tool.RunTests{}).Execute(ctx, params)
	if err != nil || mu.ChangeReport() == nil {
		t.Fatalf("public native Execute failed to create a report: error=%v result=%+v", err, result)
	}
	return result, mu, root
}

func b1122JSON(t *testing.T, value any) []byte {
	t.Helper()
	wire, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

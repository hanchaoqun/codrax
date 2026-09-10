package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSuiteSkippedControllerInstructionPreservesVerificationScope(t *testing.T) {
	mut := types.NewMutableState("preserve the existing regression tests")
	plan := &types.ChangePlan{ID: "skip-display", Status: types.PlanStatusApplied,
		Summary: "model-owned explanation remains unchanged", AcceptanceTests: []string{"run the original regression tests"}}
	report := &types.ChangeReport{PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify,
		Passed: true, VerificationStatus: types.VerificationStatusPassed,
		TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, Suite: "verification_probe/python", AssertionID: "bounded", Passed: true}},
		ExecutedCommands: []types.ExecutedCommand{
			{Runner: "verification_probe", WorkingDir: ".", Command: "python -c <bounded>", Outcome: types.ExecutedCommandOutcomeExecuted, Source: "pre_suite_verification_probe"},
			{Runner: "python", Framework: "unittest", WorkingDir: ".", Command: "python3 -m unittest discover -v", Outcome: types.ExecutedCommandOutcomeSuiteSkipped, Source: "probe_primary_suite_skipped"},
		}}
	mut.SetChangePlan(plan)
	mut.SetChangeReport(report)
	pack := types.WriteContextPackFromChangeReport(report)
	mut.SetWriteContextPack(&pack)
	before, _ := json.Marshal([]any{plan, report, types.BuildVerificationProofLedger(plan, report, nil), pack})
	ctx := &types.AgentContext{Mutable: mut, Mode: types.ModeApply}
	got := (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil)
	var skipped string
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "- verification_command: runner=python ") {
			skipped = line
		}
	}
	for _, want := range []string{"execution=not_run", "verification policy", "bounded probes passed", "not evidence of a missing environment", "not customer-source search targets", `source=probe_primary_suite_skipped`, `command="python3 -m unittest discover -v"`} {
		if !strings.Contains(skipped, want) {
			t.Errorf("controller omitted policy/non-execution meaning %q: %s", want, skipped)
		}
	}
	if strings.Contains(skipped, "exit_code=") {
		t.Errorf("skipped command presented an unmeasured exit code: %s", skipped)
	}
	for _, want := range []string{
		`verification_command: runner=verification_probe cwd=. suite= outcome=executed exit_code=0 source=pre_suite_verification_probe command="python -c <bounded>"`,
		"required_typed_contracts=0 covered_required_typed_contracts=0",
		"axis=capability kind=executed_command status=unavailable category=unavailable reason_code=suite_skipped",
		"all_verified requires every applied batch to pass its latest verification",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("display change altered existing proof accounting %q", want)
		}
	}
	after, _ := json.Marshal([]any{plan, report, types.BuildVerificationProofLedger(plan, report, nil), pack})
	if string(before) != string(after) || got != (&writeControllerEvaluator{}).BuildInitialInstruction(ctx, nil) {
		t.Fatal("display changed authoritative input, proof, or repeat rendering")
	}
}

func TestSuiteSkippedControllerPreservesOtherOutcomeBytes(t *testing.T) {
	for _, outcome := range append(types.AllExecutedCommandOutcomes(), "", "future_outcome") {
		if outcome == types.ExecutedCommandOutcomeSuiteSkipped {
			continue
		}
		for _, code := range []int{0, 7} {
			mut := types.NewMutableState("unchanged command display")
			mut.SetChangeReport(&types.ChangeReport{Passed: true, ExecutedCommands: []types.ExecutedCommand{{
				Runner: "python", WorkingDir: ".", Command: "python3 -m unittest", Outcome: outcome, Source: "probe_primary_suite_skipped", ExitCode: code,
			}}})
			got := (&writeControllerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
			want := fmt.Sprintf("- verification_command: runner=python cwd=. suite= outcome=%s exit_code=%d source=probe_primary_suite_skipped command=\"python3 -m unittest\"\n", outcome, code)
			if !strings.Contains(got, want) || strings.Contains(got, "execution=not_run") {
				t.Errorf("non-skip command %q/%d changed: expected %s", outcome, code, want)
			}
		}
	}
}

func TestSuiteSkippedControllerUnknownSourceDoesNotInventPolicy(t *testing.T) {
	for _, source := range []string{"", "other_policy", "probe_primary_suite_skipped_extra"} {
		t.Run(source, func(t *testing.T) {
			mut := types.NewMutableState("unknown skip provenance")
			mut.SetChangeReport(&types.ChangeReport{Passed: true, ExecutedCommands: []types.ExecutedCommand{{
				Runner: "python", Command: "python3 -m unittest", Outcome: types.ExecutedCommandOutcomeSuiteSkipped, Source: source, ExitCode: 9,
			}}})
			got := (&writeControllerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
			if !strings.Contains(got, "execution=not_run") || !strings.Contains(got, "source does not establish why") {
				t.Fatalf("unknown skip needs a conservative non-execution display: %s", got)
			}
			if strings.Contains(got, "exit_code=9") || strings.Contains(got, "bounded probes passed") {
				t.Fatalf("unknown skip invented measured exit/policy: %s", got)
			}
		})
	}
}

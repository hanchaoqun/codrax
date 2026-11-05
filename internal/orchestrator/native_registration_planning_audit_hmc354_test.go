package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// A dispatch is an attempt, not an installed plan or a fresh test invocation.
// Keep each attempt independently auditable instead of increasing steps==1.
type registrationPlanningAttemptHMC354 struct {
	Dispatch          int    `json:"dispatch"`
	AuthorizedAtEntry bool   `json:"authorized_at_entry"`
	ReportAbsent      bool   `json:"report_absent"`
	Failed            bool   `json:"failed"`
	PlanID            string `json:"plan_id,omitempty"`
	NativeReadOnly    bool   `json:"native_read_only"`
}

func observeRegistrationPlanningAttemptHMC354(dispatch int, authorized bool, mu *types.MutableState, err error) registrationPlanningAttemptHMC354 {
	a := registrationPlanningAttemptHMC354{Dispatch: dispatch, AuthorizedAtEntry: authorized, ReportAbsent: mu.ChangeReport() == nil, Failed: err != nil}
	if p := mu.ChangePlan(); p != nil {
		a.PlanID = p.ID
		a.NativeReadOnly = types.IsPersistedNativeTestRegistrationPlan(p) && len(p.Changes) == 0 && p.PatchEffect == nil && p.AppliedCommitSHA == ""
	}
	return a
}

func validateRegistrationPlanningAttemptsHMC354(attempts []registrationPlanningAttemptHMC354, steps int, finalPlan *types.ChangePlan) error {
	if len(attempts) == 0 || len(attempts) != steps || !types.IsPersistedNativeTestRegistrationPlan(finalPlan) {
		return fmt.Errorf("planning dispatch/attempt/final native-plan identity mismatch")
	}
	installed := 0
	for i, a := range attempts {
		if a.Dispatch != i+1 || !a.AuthorizedAtEntry || !a.ReportAbsent {
			return fmt.Errorf("dispatch %d lacks isolated read-only authority or acquired execution results", i+1)
		}
		if a.PlanID == "" {
			continue // A rejected/no-plan attempt does not count as registration.
		}
		if a.Failed || !a.NativeReadOnly || a.PlanID != finalPlan.ID || i != len(attempts)-1 {
			return fmt.Errorf("dispatch %d installed a failed, foreign, mutating or superseded plan", i+1)
		}
		installed++
	}
	if installed != 1 {
		return fmt.Errorf("expected exactly one installed current registration, got %d", installed)
	}
	return nil
}

// A distinct opt-in preserves the original historical single-dispatch harness
// and its failures. This lane audits successful registration independently of
// permitted controller retries; all original restore/execution gates remain.
func TestNativeRegistrationLiveRetryAuditedMissingTerminal(t *testing.T) {
	runNativeRegistrationLiveEvaluationWithScope(t, os.Getenv("CODRAX_LIVE_REGISTRATION_RETRY_AUDITED_EVAL_DIR"), false, false, true, true)
}

func TestHMC354RegistrationPlanningSeparatesRejectedAttemptAndInstalledPlan(t *testing.T) {
	f := newControllerRegistrationFixture(t)
	o, mu := f.o, f.o.busCtx.Mutable
	if err := types.WriteChangeReportToFile(mu.ChangeReport(), filepath.Join(o.ensureChangeReportDir(), f.source.ID+".report.json")); err != nil {
		t.Fatal(err)
	}
	mu.ResetChangeReport()
	registry := tool.NewRegistry()
	registry.Register(&tool.ReadFile{})
	registry.Register(&tool.EmitChangePlan{})
	planner := agent.NewPlannerAgent(&agent.Dependencies{Tools: registry, LLM: &controllerRegistrationLLM{t: t, entry: "emit_change_plan"}, MaxIterations: 2, AgentSettings: types.DefaultAgentSettings(), Emit: func(render.Event) {}})
	var attempts []registrationPlanningAttemptHMC354
	o.controllerWriteStageFn = func(stage types.PipelineStage, steps *int) (*agent.StageOutput, error) {
		if stage != types.StagePlan {
			t.Fatal("unexpected mutation or execution stage")
		}
		*steps++
		authorized := mu.NativeTestRegistrationAuthorization() != nil
		ctx := &types.AgentContext{RepoRoot: o.busCtx.RepoRoot, WorkDir: o.busCtx.WorkDir, MainRepoRoot: o.busCtx.MainRepoRoot, Mode: types.ModeApply, Stage: stage, AgentName: types.AgentPlanner, Mutable: mu}
		var out *agent.StageOutput
		var err error
		if len(attempts) == 0 {
			bad, badErr := (&tool.EmitChangePlan{}).Execute(types.ToolBusContext(ctx, types.AgentPlanner), json.RawMessage(`{"request":"verify retained tests","summary":"invalid empty change","changes":[{}]}`))
			if badErr != nil || bad.Success || mu.ChangePlan() != nil || mu.ChangeReport() != nil {
				t.Fatal("rejected public emit installed a plan or executed tests")
			}
			out = &agent.StageOutput{ToolResults: []types.ToolResult{bad}}
		} else {
			out, err = planner.Execute(ctx, &skill.Config{ToolSuggestions: []string{"read_file", "emit_change_plan"}})
			if err == nil {
				err = planPostHook(o, out)
			}
		}
		attempts = append(attempts, observeRegistrationPlanningAttemptHMC354(len(attempts)+1, authorized, mu, err))
		return out, err
	}
	steps := 0
	if err := o.runControllerPlanBatch(&writeflow.WriteBatchPlan{ID: "proof", Purpose: "verification_proof_followup", ExpectedPaths: []string{"value.py"}}, &steps); err != nil {
		t.Fatal(err)
	}
	if steps != 2 || len(attempts) != 2 || attempts[0].PlanID != "" || !attempts[1].NativeReadOnly || mu.NativeTestRegistrationAuthorization() != nil || mu.ChangeReport() != nil {
		t.Fatal("controller did not preserve the exact rejected→registered read-only sequence")
	}
	if err := validateRegistrationPlanningAttemptsHMC354(attempts, steps, mu.ChangePlan()); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"missing_attempt", "wrong_dispatch", "missing_grant", "execution_during_plan", "failed_plan", "foreign_plan", "mutating_plan", "duplicate_registration"} {
		t.Run(mode, func(t *testing.T) {
			bad := append([]registrationPlanningAttemptHMC354(nil), attempts...)
			switch mode {
			case "missing_attempt":
				bad = bad[1:]
			case "wrong_dispatch":
				bad[1].Dispatch++
			case "missing_grant":
				bad[0].AuthorizedAtEntry = false
			case "execution_during_plan":
				bad[0].ReportAbsent = false
			case "failed_plan":
				bad[1].Failed = true
			case "foreign_plan":
				bad[1].PlanID += "-other"
			case "mutating_plan":
				bad[1].NativeReadOnly = false
			case "duplicate_registration":
				bad[0].PlanID, bad[0].NativeReadOnly = bad[1].PlanID, true
			}
			if err := validateRegistrationPlanningAttemptsHMC354(bad, steps, mu.ChangePlan()); err == nil {
				t.Fatal("invalid planning audit was accepted")
			}
		})
	}
}

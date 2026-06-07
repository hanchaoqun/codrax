package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestAdmitActionDAGPlanSplitsThenAcceptsProtectedPrefix(t *testing.T) {
	plan := dataquery.TaskPlan{
		Actions: []dataquery.DataAction{
			{ID: "derive", Kind: dataquery.DataActionDeriveFields, OutputArtifact: "derived.json"},
			{ID: "compute", Kind: dataquery.DataActionComputeContribs, InputPaths: []string{"derived.json"}},
		},
	}
	decision := AdmitActionDAGPlan(ActionDAGAdmissionInput{
		Plan: plan,
		Protect: func(p dataquery.TaskPlan) dataquery.TaskPlan {
			p.Status = "ready"
			return p
		},
		PrefixFallback: func(p dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, string, bool) {
			if len(p.Actions) != 2 {
				return dataquery.TaskPlan{}, dataquery.TaskPlan{}, "", false
			}
			prefix := p
			prefix.Actions = p.Actions[:1]
			remainder := p
			remainder.Actions = p.Actions[1:]
			return prefix, remainder, "split dependency rank", true
		},
		Guard: func(p dataquery.TaskPlan) GuardResult {
			if p.Status != "ready" {
				return NewGuardResult("not_protected", "error", RepairNeedsTypedAction, "plan was not protected")
			}
			return GuardResult{}
		},
	})
	if !decision.Rewritten {
		t.Fatalf("decision=%+v, want rewritten split", decision)
	}
	if decision.Plan.Status != "ready" || len(decision.Plan.Actions) != 1 || decision.Plan.Actions[0].ID != "derive" {
		t.Fatalf("Plan=%+v, want protected derive prefix", decision.Plan)
	}
	if len(decision.Remainder.Actions) != 1 || decision.Remainder.Actions[0].ID != "compute" {
		t.Fatalf("Remainder=%+v, want compute suffix", decision.Remainder.Actions)
	}
	if !strings.Contains(decision.Reason, "split dependency rank") {
		t.Fatalf("Reason=%q, want split reason", decision.Reason)
	}
}

func TestAdmitActionDAGPlanUsesDeterministicFallbackBeforeBlocking(t *testing.T) {
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "bad", Kind: dataquery.DataActionFilterRecords}}}
	decision := AdmitActionDAGPlan(ActionDAGAdmissionInput{
		Plan: plan,
		Guard: func(p dataquery.TaskPlan) GuardResult {
			if len(p.Actions) > 0 && p.Actions[0].ID == "fixed" {
				return GuardResult{}
			}
			return NewGuardResult("missing_action_inputs", "error", RepairNeedsTypedAction, "filter needs input_paths")
		},
		DeterministicFallback: func(p dataquery.TaskPlan, guard GuardResult) (dataquery.TaskPlan, dataquery.TaskPlan, string, bool) {
			if guard.Code != "missing_action_inputs" {
				return dataquery.TaskPlan{}, dataquery.TaskPlan{}, "", false
			}
			p.Actions[0].ID = "fixed"
			p.Actions[0].InputPaths = []string{"records.json"}
			return p, dataquery.TaskPlan{}, "materialized action input", true
		},
	})
	if !decision.Rewritten || decision.FinalGuardErr != "" {
		t.Fatalf("decision=%+v, want repaired admission", decision)
	}
	if decision.Plan.Actions[0].ID != "fixed" || len(decision.Plan.Actions[0].InputPaths) != 1 {
		t.Fatalf("Plan=%+v, want fixed action", decision.Plan.Actions)
	}
}

func TestAdmitActionDAGPlanReturnsTypedFinalGuardWhenUnrepairable(t *testing.T) {
	decision := AdmitActionDAGPlan(ActionDAGAdmissionInput{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "bad", Kind: dataquery.DataActionCustomTransform}}},
		Guard: func(dataquery.TaskPlan) GuardResult {
			return NewGuardResult("admission_rejected", "error", RepairNeedsTypedAction, "custom transform disabled")
		},
		DeterministicFallback: func(dataquery.TaskPlan, GuardResult) (dataquery.TaskPlan, dataquery.TaskPlan, string, bool) {
			return dataquery.TaskPlan{}, dataquery.TaskPlan{}, "", false
		},
	})
	if decision.FinalGuard.Code != "admission_rejected" || decision.FinalGuardErr != "custom transform disabled" {
		t.Fatalf("decision=%+v, want typed final guard", decision)
	}
}

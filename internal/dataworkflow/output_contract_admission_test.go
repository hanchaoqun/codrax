package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// output_contract_admission_test.go — V9-4 (colleague_merge_audit §40.56)
// pin ④: system-built plans pass the drift guard unchanged, a
// contract-dependent rejection becomes the typed drift guard with the
// executor's own locus, and contract-independent parameter defects keep
// their existing lanes.

func TestActionOutputContractGuardSystemBuiltPlansPassUnchanged(t *testing.T) {
	strict := dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false, Delimiter: ","}
	completion := requiredLedgerCompletionBasePlan(RequiredLedgerCompletionPlanInput{
		Current: dataquery.TaskPlan{Goal: "sum per target", OutputContract: strict},
		Output:  strict,
	})
	completion.Actions = []dataquery.DataAction{RuleCoverageCompletionAction(dataquery.CoverageContract{ValidationRules: []string{"rule"}})}
	continuation := baseContinuationPlan(dataquery.TaskPlan{Goal: "continue"}, dataquery.CoverageContract{}, strict)
	continuation.Actions = []dataquery.DataAction{{
		ID:     "complete_output_contract_answer",
		Kind:   dataquery.DataActionAssembleAnswer,
		Params: map[string]string{"order_by": "group_key", "value_field": "actual", "delimiter": ","},
	}}
	freeform := dataquery.TaskPlan{
		Status:         "ready",
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true},
		Actions: []dataquery.DataAction{{
			ID:     "inventory",
			Kind:   dataquery.DataActionMaterialInventory,
			Params: map[string]string{"limit": "3"},
		}},
	}
	for name, plan := range map[string]dataquery.TaskPlan{"ledger_completion": completion, "continuation_projection": continuation, "freeform_reset": freeform, "no_actions": {OutputContract: strict}} {
		before := cloneTaskPlanValue(plan)
		if guard := ActionOutputContractGuardResult(plan); !guard.Empty() {
			t.Fatalf("%s: guard=%+v, want system-built plan admitted", name, guard)
		}
		for i := range plan.Actions {
			if len(plan.Actions[i].Params) != len(before.Actions[i].Params) {
				t.Fatalf("%s: guard rewrote action params %v -> %v", name, before.Actions[i].Params, plan.Actions[i].Params)
			}
		}
	}
}

func TestActionOutputContractGuardTypesContractDependentDriftOnly(t *testing.T) {
	strict := dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false}
	drift := dataquery.TaskPlan{
		OutputContract: strict,
		Actions: []dataquery.DataAction{
			{ID: "reconcile", Kind: dataquery.DataActionReconcile},
			{ID: "project", Kind: dataquery.DataActionAssembleAnswer, Params: map[string]string{"projection": "json_object"}},
		},
	}
	guard := ActionOutputContractGuardResult(drift)
	if guard.Code != GuardCodeActionOutputContractDrift || guard.Repairability != RepairNeedsTypedAction || guard.Severity != "error" {
		t.Fatalf("guard=%+v, want typed drift guard on the repair lane", guard)
	}
	if !strings.Contains(guard.Message, "action 2 (project)") || !strings.Contains(guard.Message, "output_contract.format=plain_single_line") {
		t.Fatalf("message=%q, want the offending action and the effective format", guard.Message)
	}
	if len(guard.Violations) != 1 {
		t.Fatalf("violations=%+v, want exactly the offending action", guard.Violations)
	}
	violation := guard.Violations[0]
	if violation.Code != "action_param_violation" || violation.Param != "projection/output_contract.format" || violation.ActionID != "project" || violation.ActionKind != string(dataquery.DataActionAssembleAnswer) {
		t.Fatalf("violation=%+v, want the executor's typed locus (same code the runtime would raise)", violation)
	}
	projected := DataTaskViolationFromGuard(guard)
	if projected.Code != "action_param_violation" || projected.Param != "projection/output_contract.format" || projected.ActionID != "project" {
		t.Fatalf("repair carrier=%+v, want the same typed locus the executor raises", projected)
	}
	// A contract-independent defect (unknown parameter) is not drift: it
	// keeps its existing lane (execution admission), so this guard stays
	// silent and does not widen the staging surface.
	independent := dataquery.TaskPlan{
		OutputContract: strict,
		Actions:        []dataquery.DataAction{{ID: "project", Kind: dataquery.DataActionAssembleAnswer, Params: map[string]string{"projection": "values", "bogus": "1"}}},
	}
	if guard := ActionOutputContractGuardResult(independent); !guard.Empty() {
		t.Fatalf("contract-independent defect surfaced as drift: %+v", guard)
	}
}

func TestOutputContractDeclared(t *testing.T) {
	if OutputContractDeclared(dataquery.OutputContract{}) {
		t.Fatal("zero contract reported as declared")
	}
	for _, contract := range []dataquery.OutputContract{
		{Format: dataquery.OutputFreeform},
		{Delimiter: ";"},
		{CompleteReference: true},
		{ReferencePath: "targets.csv"},
		{ReferenceKeyField: "canonical_label"},
	} {
		if !OutputContractDeclared(contract) {
			t.Fatalf("%+v reported as undeclared", contract)
		}
	}
}

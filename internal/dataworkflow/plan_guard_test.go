package dataworkflow

import (
	"strings"
	"testing"
)

func TestPlanShapeGuardRejectsMissingExecutableBody(t *testing.T) {
	guard := PlanShapeGuardResult(PlanShapeGuardInput{
		Status:    "ready",
		HasScript: false,
	})
	if guard.Code != "missing_executable_body" || guard.Empty() {
		t.Fatalf("guard=%+v, want missing_executable_body", guard)
	}
	if len(guard.Violations) != 1 || guard.Violations[0].Repairability != RepairNeedsTypedAction {
		t.Fatalf("violations=%+v, want typed action repair violation", guard.Violations)
	}
}

func TestPlanShapeGuardRejectsMissingEmitter(t *testing.T) {
	guard := PlanShapeGuardResult(PlanShapeGuardInput{
		Status:                 "ready",
		HasScript:              true,
		ScriptLines:            2,
		ScriptHasResultEmitter: false,
	})
	if guard.Code != "missing_result_emitter" || guard.Empty() {
		t.Fatalf("guard=%+v, want missing_result_emitter", guard)
	}
	if len(guard.Violations) != 1 || guard.Violations[0].Code != "missing_result_emitter" {
		t.Fatalf("violations=%+v, want typed missing_result_emitter violation", guard.Violations)
	}
}

func TestPlanShapeGuardRejectsComplexTopLevelScript(t *testing.T) {
	guard := PlanShapeGuardResult(PlanShapeGuardInput{
		Status:                 "ready",
		HasScript:              true,
		ScriptLines:            8,
		ScriptHasResultEmitter: true,
		InputCount:             4,
		RequiredMaterialCount:  4,
		ValidationLedgerCount:  2,
	})
	if guard.Code != "complex_top_level_script" {
		t.Fatalf("guard=%+v, want complex_top_level_script", guard)
	}
}

func TestPlanShapeGuardRejectsOversizedBatch(t *testing.T) {
	guard := PlanShapeGuardResult(PlanShapeGuardInput{
		Status:                 "ready",
		HasScript:              true,
		ScriptLines:            421,
		ScriptHasResultEmitter: true,
		InputCount:             1,
		ContinueAfter:          true,
		HardScriptLineLimit:    420,
	})
	if guard.Code != "oversized_data_batch" {
		t.Fatalf("guard=%+v, want oversized_data_batch", guard)
	}
	if got := guard.ErrorText(); got == "" || !containsAll(got, []string{"script_lines=421", "continue_after=true"}) {
		t.Fatalf("error=%q, want script lines and true continue_after", got)
	}
}

func TestPlanShapeGuardAllowsActionBatch(t *testing.T) {
	guard := PlanShapeGuardResult(PlanShapeGuardInput{
		Status:        "ready",
		HasActions:    true,
		ScriptLines:   999,
		InputCount:    99,
		ContinueAfter: true,
	})
	if !guard.Empty() {
		t.Fatalf("guard=%+v, want empty for action batch", guard)
	}
}

func containsAll(s string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}

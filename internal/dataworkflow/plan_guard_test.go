package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
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

func TestActionBatchShapeGuardResultUsesTypedViolation(t *testing.T) {
	action := dataquery.DataAction{
		ID:         "wide",
		Kind:       dataquery.DataActionCustomTransform,
		InputPaths: []string{"a.csv", "b.csv", "c.csv", "d.csv"},
		Script:     "emit_result('ok')\n",
	}
	guard := ActionBatchShapeGuardResult(ActionBatchShapeGuardInput{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{action}},
		Checks: ActionBatchShapeChecks{
			ActionScripts: true,
		},
		RequiredMaterialCount:        4,
		ValidationLedgerCount:        2,
		ComplexCustomScriptLineLimit: 1,
		ActionFacts: []ActionShapeFact{{
			Action:                 action,
			ActionIndex:            0,
			ScriptLines:            1,
			HasResultEmitter:       true,
			LooksLikeWholeWorkflow: true,
		}},
	})
	if guard.Code != "broad_custom_transform" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed broad_custom_transform", guard)
	}
	v := guard.Violations[0]
	if v.ActionID != "wide" || v.ActionKind != string(dataquery.DataActionCustomTransform) || v.IdempotencyKey == "" {
		t.Fatalf("violation=%+v, want action metadata and idempotency key", v)
	}
}

func TestAllowedNextActionGuardResultUsesTypedViolation(t *testing.T) {
	action := dataquery.DataAction{
		ID:             "join_now",
		Kind:           dataquery.DataActionJoinRecords,
		InputPaths:     []string{"left", "right"},
		OutputArtifact: "joined",
	}
	guard := AllowedNextActionGuardResult(AllowedNextActionGuardInput{
		State: WorkflowStateView{
			MaterialCoverageSufficient: true,
			NextStage:                  StagePrepareContributionInputs,
			AllowedNextActions:         []string{string(dataquery.DataActionFilterRecords)},
		},
		Action:      action,
		ActionIndex: 2,
	})
	if guard.Code != "action_outside_allowed_next_stage" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed action_outside_allowed_next_stage", guard)
	}
	v := guard.Violations[0]
	if v.ActionID != "join_now" || v.ActionKind != string(dataquery.DataActionJoinRecords) || len(v.RepairActionHints) != 1 || v.RepairActionHints[0] != string(dataquery.DataActionFilterRecords) {
		t.Fatalf("violation=%+v, want action metadata and allowed-action hint", v)
	}
}

func TestStageProgressGuardResultUsesTypedViolationForRankCrossing(t *testing.T) {
	state := WorkflowStateView{
		MaterialCoverageSufficient: true,
		NextStage:                  StagePrepareContributionInputs,
		WorkflowContract: CoverageContractView{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
		AllowedNextActions: []string{string(dataquery.DataActionFilterRecords)},
	}
	guard := StageProgressGuardResult(StageProgressGuardInput{
		State:                   state,
		CrossesTypedActionRanks: true,
	})
	if guard.Code != "action_batch_crosses_dependent_ranks" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed rank-crossing violation", guard)
	}
	if got := guard.Violations[0].RepairActionHints; len(got) != 1 || got[0] != string(dataquery.DataActionFilterRecords) {
		t.Fatalf("repair hints=%v, want allowed next actions", got)
	}
}

func TestStageProgressGuardResultUsesTypedViolationForBroadCustomTransform(t *testing.T) {
	state := WorkflowStateView{
		MaterialCoverageSufficient: true,
		NextStage:                  StageComputeContributions,
		WorkflowContract: CoverageContractView{
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
	}
	guard := StageProgressGuardResult(StageProgressGuardInput{
		State:                      state,
		HasScriptedCustomTransform: true,
	})
	if guard.Code != "custom_transform_crosses_unfinished_stages" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed custom-transform stage violation", guard)
	}
	if guard.Violations[0].Repairability != RepairNeedsTypedAction {
		t.Fatalf("repairability=%s, want needs typed action", guard.Violations[0].Repairability)
	}
}

func TestCoverageLoopGuardResultUsesTypedViolation(t *testing.T) {
	state := WorkflowStateView{
		MaterialCoverageSufficient:   true,
		NextStage:                    StagePrepareContributionInputs,
		RequiredMaterialCount:        3,
		MissingRequiredMaterialCount: 0,
		AllowedNextActions:           []string{string(dataquery.DataActionDeriveFields)},
	}
	guard := CoverageLoopGuardResult(CoverageLoopGuardInput{
		State:        state,
		CoverageOnly: true,
	})
	if guard.Code != "coverage_loop_after_sufficient_materials" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed coverage loop violation", guard)
	}
	if got := guard.Violations[0].RepairActionHints; len(got) != 1 || got[0] != string(dataquery.DataActionDeriveFields) {
		t.Fatalf("repair hints=%v, want allowed next actions", got)
	}
}

func TestCoverageLoopGuardResultAllowsGeneratedArtifactDiagnostics(t *testing.T) {
	guard := CoverageLoopGuardResult(CoverageLoopGuardInput{
		State: WorkflowStateView{
			MaterialCoverageSufficient: true,
			NextStage:                  StagePrepareContributionInputs,
		},
		CoverageOnly:                true,
		GeneratedArtifactDiagnostic: true,
	})
	if !guard.Empty() {
		t.Fatalf("guard=%+v, want generated artifact diagnostics allowed", guard)
	}
}

func TestNumericConstantReuseGuardResultUsesTypedViolation(t *testing.T) {
	guard := NumericConstantReuseGuardResult(NumericConstantReuseGuardInput{
		Constants: []DerivedConstantFieldFact{{
			ActionIndex:   0,
			ActionID:      "derive_label",
			Action:        dataquery.DataAction{ID: "derive_label", Kind: dataquery.DataActionDeriveFields, OutputArtifact: "derived"},
			Field:         "score",
			Value:         "approved",
			OutputAliases: []string{"derived"},
		}},
		Uses: []NumericFieldUseFact{{
			ActionIndex:  1,
			Action:       dataquery.DataAction{ID: "compute", Kind: dataquery.DataActionComputeContribs, InputPaths: []string{"derived"}},
			Field:        "score",
			InputAliases: []string{"derived"},
		}},
	})
	if guard.Code != "numeric_constant_reused_as_numeric_field" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want typed numeric constant violation", guard)
	}
	v := guard.Violations[0]
	if v.ActionID != "compute" || len(v.CandidateArtifacts) != 1 || v.CandidateArtifacts[0] != "derived" || len(v.MissingFields) != 1 || v.MissingFields[0] != "score" {
		t.Fatalf("violation=%+v, want consumer action metadata and constant field", v)
	}
}

func TestNumericConstantReuseGuardResultAllowsUnconsumedAliases(t *testing.T) {
	guard := NumericConstantReuseGuardResult(NumericConstantReuseGuardInput{
		Constants: []DerivedConstantFieldFact{{
			ActionIndex:   0,
			Field:         "score",
			Value:         "approved",
			OutputAliases: []string{"other"},
		}},
		Uses: []NumericFieldUseFact{{
			ActionIndex:  1,
			Action:       dataquery.DataAction{ID: "compute", Kind: dataquery.DataActionComputeContribs, InputPaths: []string{"derived"}},
			Field:        "score",
			InputAliases: []string{"derived"},
		}},
	})
	if !guard.Empty() {
		t.Fatalf("guard=%+v, want unrelated aliases allowed", guard)
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

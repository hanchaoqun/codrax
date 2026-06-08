package dataworkflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type PlanShapeGuardInput struct {
	Status                 string
	HasActions             bool
	HasScript              bool
	ScriptLines            int
	ScriptHasResultEmitter bool
	InputCount             int
	RequiredMaterialCount  int
	ValidationLedgerCount  int
	ContinueAfter          bool
	SoftScriptLineLimit    int
	HardScriptLineLimit    int
	RequiredMaterialLimit  int
	ValidationLedgerLimit  int
}

type ActionBatchShapeChecks struct {
	TopLevelScript       bool
	ActionCount          bool
	MultipleCustomScript bool
	ActionScripts        bool
}

type ActionShapeFact struct {
	Action                 dataquery.DataAction
	ActionIndex            int
	ScriptLines            int
	HasResultEmitter       bool
	LooksLikeWholeWorkflow bool
	BroadPrereqSurface     bool
	RawInputCount          int
}

type ActionBatchShapeGuardInput struct {
	Plan                         dataquery.TaskPlan
	Checks                       ActionBatchShapeChecks
	TopLevelScriptLines          int
	MaxActionsPerBatch           int
	CustomScriptActionCount      int
	RequiredMaterialCount        int
	ValidationLedgerCount        int
	ComplexCustomScriptLineLimit int
	OneShotScriptLineSoftLimit   int
	ActionFacts                  []ActionShapeFact
	WorkflowAware                bool
}

type AllowedNextActionGuardInput struct {
	State                       WorkflowStateView
	Action                      dataquery.DataAction
	ActionIndex                 int
	GeneratedArtifactDiagnostic bool
	RecordMaterialization       bool
}

type StageProgressGuardInput struct {
	State                             WorkflowStateView
	HasScriptedCustomTransform        bool
	CrossesTypedActionRanks           bool
	NarrowSingleIntermediateTransform bool
}

type CoverageLoopGuardInput struct {
	State                            WorkflowStateView
	CoverageOnly                     bool
	GeneratedArtifactDiagnostic      bool
	ActionsAllAllowedForCurrentStage bool
}

func PlanShapeGuardResult(input PlanShapeGuardInput) GuardResult {
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status != "" && status != "ready" {
		return GuardResult{}
	}
	if input.HasActions {
		return GuardResult{}
	}
	if !input.HasScript {
		return planShapeGuard("missing_executable_body", "data planning incomplete: ready data plan has no executable body. Emit a bounded actions[] batch or a script with emit_result/emit; do not mark a data plan ready when it only declares inputs, contracts, or prose.")
	}
	if input.ScriptLines > 0 && !input.ScriptHasResultEmitter {
		return planShapeGuard("missing_result_emitter", fmt.Sprintf("data planning incomplete: script has no result emitter (script_lines=%d). A bounded data script must call emit(...), emit_result(...), or assign result before it can complete; otherwise split the workflow into typed actions that produce reusable artifacts.",
			input.ScriptLines))
	}
	complexBatch := input.RequiredMaterialCount >= 4 || input.ValidationLedgerCount >= 2 || input.InputCount >= 4
	if input.ScriptLines > 0 && complexBatch {
		return planShapeGuard("complex_top_level_script", fmt.Sprintf("data planning incomplete: complex data task should not start as one top-level script (script_lines=%d input_paths=%d required_materials=%d validation_ledgers=%d). Emit an atomic actions[] batch such as inspect_material, extract_records, derive_rules, derive_fields, extract_fields, group_records, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, or a bounded custom_transform, and set continue_after=true when more graph work remains.",
			input.ScriptLines, input.InputCount, input.RequiredMaterialCount, input.ValidationLedgerCount))
	}
	soft := input.SoftScriptLineLimit
	if soft <= 0 {
		soft = 260
	}
	hard := input.HardScriptLineLimit
	if hard <= 0 {
		hard = 420
	}
	requiredLimit := input.RequiredMaterialLimit
	if requiredLimit <= 0 {
		requiredLimit = 8
	}
	ledgerLimit := input.ValidationLedgerLimit
	if ledgerLimit <= 0 {
		ledgerLimit = 3
	}
	oversized := input.ScriptLines >= hard ||
		(input.ScriptLines >= soft && (input.RequiredMaterialCount >= requiredLimit || input.ValidationLedgerCount >= ledgerLimit)) ||
		(input.ScriptLines >= 180 && complexBatch) ||
		(input.RequiredMaterialCount >= requiredLimit+4 && input.ValidationLedgerCount >= ledgerLimit)
	if !oversized {
		return GuardResult{}
	}
	return planShapeGuard("oversized_data_batch", fmt.Sprintf("data planning incomplete: plan is too large for one bounded data batch (script_lines=%d input_paths=%d required_materials=%d validation_ledgers=%d continue_after=%t). Emit a smaller atomic actions[] batch such as material_inventory, inspect_material, extract_records, derive_rules, derive_fields, extract_fields, group_records, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, or a bounded custom_transform; set continue_after=true when further work remains, and let the workflow feed real results into later batches.",
		input.ScriptLines, input.InputCount, input.RequiredMaterialCount, input.ValidationLedgerCount, input.ContinueAfter))
}

func planShapeGuard(code, message string) GuardResult {
	violation := NewGenericViolation(GenericViolationInput{
		Code:          code,
		Severity:      "error",
		Repairability: RepairNeedsTypedAction,
		Reason:        message,
	})
	return NewGuardResult(code, "error", RepairNeedsTypedAction, message, violation)
}

func ActionBatchShapeGuardResult(input ActionBatchShapeGuardInput) GuardResult {
	if input.Checks.TopLevelScript && input.TopLevelScriptLines > 0 {
		message := fmt.Sprintf("data planning incomplete: actions[] plans must not carry a top-level script (script_lines=%d). Put each bounded transform script on its custom_transform action, or split the workflow into typed atomic actions; top-level script is only for simple non-actions plans.",
			input.TopLevelScriptLines)
		return actionBatchShapeGuard("action_batch_top_level_script", message, dataquery.DataAction{})
	}
	if input.Checks.ActionCount && input.MaxActionsPerBatch > 0 && len(input.Plan.Actions) > input.MaxActionsPerBatch {
		message := fmt.Sprintf("data planning incomplete: actions[] batch contains %d action(s), above the atomic batch limit %d. Emit only the next small DAG batch and set continue_after=true when more data workflow work remains.",
			len(input.Plan.Actions), input.MaxActionsPerBatch)
		return actionBatchShapeGuard("action_batch_too_large", message, dataquery.DataAction{})
	}
	if input.Checks.MultipleCustomScript && input.CustomScriptActionCount > 1 {
		message := fmt.Sprintf("data planning incomplete: actions[] batch contains %d custom_transform scripts. A batch may have at most one bounded custom_transform; split independent transforms into separate batches or use typed actions that produce reusable artifacts.",
			input.CustomScriptActionCount)
		return actionBatchShapeGuard("multiple_custom_transform_scripts", message, dataquery.DataAction{})
	}
	if !input.Checks.ActionScripts {
		return GuardResult{}
	}
	complexLimit := input.ComplexCustomScriptLineLimit
	if complexLimit <= 0 {
		complexLimit = 160
	}
	softLimit := input.OneShotScriptLineSoftLimit
	if softLimit <= 0 {
		softLimit = 260
	}
	for _, fact := range input.ActionFacts {
		action := fact.Action
		kind := NormalizeActionKind(action.Kind)
		lines := fact.ScriptLines
		if kind == dataquery.DataActionCustomTransform && lines == 0 {
			message := fmt.Sprintf("data planning incomplete: action %d (%s) is custom_transform but has no script. Emit a typed action such as inspect_material, extract_records, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, or provide one bounded custom_transform script that calls emit(...), emit_result(...), or assigns result.",
				guardActionNumber(fact.ActionIndex), guardActionLabel(action))
			if input.WorkflowAware {
				message = fmt.Sprintf("data planning incomplete: action %d (%s) is custom_transform but has no script. Emit a typed action from workflow_state_json.allowed_next_actions, or provide one bounded custom_transform script that calls emit(...), emit_result(...), or assigns result.",
					guardActionNumber(fact.ActionIndex), guardActionLabel(action))
			}
			return actionBatchShapeGuard("missing_custom_transform_script", message, action)
		}
		if lines == 0 {
			continue
		}
		if kind != dataquery.DataActionCustomTransform {
			message := fmt.Sprintf("data planning incomplete: action %d (%s) is a typed data action but carries a script (script_lines=%d). Only custom_transform may carry a script. For %s, remove script and express the operation with input_paths, output_artifact, and params; if a script is truly required, use custom_transform only when the workflow allows it.",
				guardActionNumber(fact.ActionIndex), guardActionLabel(action), lines, kind)
			if input.WorkflowAware {
				message = fmt.Sprintf("data planning incomplete: action %d (%s) is a typed data action but carries a script (script_lines=%d). Only custom_transform may carry a script. For %s, remove script and express the operation with input_paths, output_artifact, and params; if a script is truly required, use custom_transform only when workflow_state_json allows it.",
					guardActionNumber(fact.ActionIndex), guardActionLabel(action), lines, kind)
			}
			return actionBatchShapeGuard("typed_action_carries_script", message, action)
		}
		if !fact.HasResultEmitter {
			message := fmt.Sprintf("data planning incomplete: action %d (%s) script has no result emitter (script_lines=%d). A custom_transform must call emit(...), emit_result(...), or assign result.",
				guardActionNumber(fact.ActionIndex), strings.TrimSpace(string(kind)), lines)
			return actionBatchShapeGuard("custom_transform_missing_result_emitter", message, action)
		}
		if input.WorkflowAware && fact.BroadPrereqSurface {
			if lines >= complexLimit && fact.LooksLikeWholeWorkflow {
				message := fmt.Sprintf("data planning incomplete: action %d (%s) is too broad for one bounded custom_transform (script_lines=%d input_paths=%d required_materials=%d validation_ledgers=%d). Prior coverage of materials does not make one script a valid substitute for the remaining data DAG. Continue with typed actions such as derive_fields, expand_records, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, and reserve custom_transform for one narrow transform over known artifacts.",
					guardActionNumber(fact.ActionIndex), strings.TrimSpace(string(kind)), lines, len(action.InputPaths), input.RequiredMaterialCount, input.ValidationLedgerCount)
				return actionBatchShapeGuard("broad_custom_transform", message, action)
			}
			if lines >= softLimit {
				message := fmt.Sprintf("data planning incomplete: action %d (%s) is too large for one atomic data action (script_lines=%d). Split the workflow into smaller typed actions such as material_inventory, inspect_material, and bounded custom_transform nodes.",
					guardActionNumber(fact.ActionIndex), strings.TrimSpace(string(kind)), lines)
				return actionBatchShapeGuard("oversized_atomic_action", message, action)
			}
			if fact.RawInputCount >= 3 && input.ValidationLedgerCount >= 3 {
				message := fmt.Sprintf("data planning incomplete: action %d (%s) is too broad for one custom_transform data DAG node (raw_inputs=%d input_paths=%d required_materials=%d validation_ledgers=%d). Split the work into typed atomic actions such as inspect_material, derive_rules, derive_fields, filter_records, qualify_records, normalize_entities, enrich_records, join_records, compute_contributions, and reconcile_artifacts. Reserve custom_transform for one narrow transform over known generated artifacts.",
					guardActionNumber(fact.ActionIndex), strings.TrimSpace(string(kind)), fact.RawInputCount, len(action.InputPaths), input.RequiredMaterialCount, input.ValidationLedgerCount)
				return actionBatchShapeGuard("broad_custom_transform_raw_inputs", message, action)
			}
			continue
		}
		if lines >= complexLimit && fact.LooksLikeWholeWorkflow {
			message := fmt.Sprintf("data planning incomplete: action %d (%s) is too broad for one bounded custom_transform (script_lines=%d input_paths=%d required_materials=%d validation_ledgers=%d). Split it into smaller typed actions such as inspect_material, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, and reserve custom_transform for one narrow transform.",
				guardActionNumber(fact.ActionIndex), strings.TrimSpace(string(kind)), lines, len(action.InputPaths), input.RequiredMaterialCount, input.ValidationLedgerCount)
			return actionBatchShapeGuard("broad_custom_transform", message, action)
		}
		if lines >= softLimit {
			message := fmt.Sprintf("data planning incomplete: action %d (%s) is too large for one atomic data action (script_lines=%d). Split the workflow into smaller typed actions such as material_inventory, inspect_material, and bounded custom_transform nodes.",
				guardActionNumber(fact.ActionIndex), strings.TrimSpace(string(kind)), lines)
			return actionBatchShapeGuard("oversized_atomic_action", message, action)
		}
		if fact.BroadPrereqSurface && input.ValidationLedgerCount >= 3 {
			message := fmt.Sprintf("data planning incomplete: action %d (%s) is too broad for one custom_transform data DAG node (input_paths=%d required_materials=%d validation_ledgers=%d). Split the work into typed atomic actions such as inspect_material, derive_rules, derive_fields, filter_records, qualify_records, normalize_entities, enrich_records, join_records, compute_contributions, and reconcile_artifacts. Reserve custom_transform for one narrow transform over known generated artifacts.",
				guardActionNumber(fact.ActionIndex), strings.TrimSpace(string(kind)), len(action.InputPaths), input.RequiredMaterialCount, input.ValidationLedgerCount)
			return actionBatchShapeGuard("broad_custom_transform_prerequisites", message, action)
		}
	}
	return GuardResult{}
}

func actionBatchShapeGuard(code, message string, action dataquery.DataAction) GuardResult {
	violation := NewGenericViolation(GenericViolationInput{
		Code:          code,
		Severity:      "error",
		Repairability: RepairNeedsTypedAction,
		Action:        action,
		Reason:        message,
	})
	return NewGuardResult(code, "error", RepairNeedsTypedAction, message, violation)
}

func AllowedNextActionGuardResult(input AllowedNextActionGuardInput) GuardResult {
	state := input.State
	if len(state.AllowedNextActions) == 0 || input.GeneratedArtifactDiagnostic || input.RecordMaterialization {
		return GuardResult{}
	}
	action := input.Action
	kind := NormalizeActionKind(action.Kind)
	if kind == dataquery.DataActionExtractRecords && state.MaterialCoverageSufficient {
		message := fmt.Sprintf("data planning incomplete: workflow next_stage=%s can use extract_records only as record materialization after material coverage is sufficient. Action %d (%s) must declare output_artifact and consume explicit current-batch input paths or generated artifact aliases; do not re-run coverage-only extraction.",
			state.NextStage, guardActionNumber(input.ActionIndex), guardActionLabel(action))
		return allowedNextActionGuard("extract_records_after_coverage_requires_output", message, action, state.AllowedNextActions)
	}
	for _, allowed := range state.AllowedNextActions {
		if kind == dataquery.DataActionKind(allowed) {
			return GuardResult{}
		}
	}
	message := fmt.Sprintf("data planning incomplete: workflow next_stage=%s allows only next actions [%s], but action %d (%s) uses %s. Emit the next bounded DAG batch using workflow_state_json.allowed_next_actions, set continue_after=true when later stages remain, and let the evaluator advance the workflow after this batch.",
		state.NextStage,
		strings.Join(cleanStrings(state.AllowedNextActions), ", "),
		guardActionNumber(input.ActionIndex),
		guardActionLabel(action),
		kind)
	return allowedNextActionGuard("action_outside_allowed_next_stage", message, action, state.AllowedNextActions)
}

func allowedNextActionGuard(code, message string, action dataquery.DataAction, allowed []string) GuardResult {
	violation := NewGenericViolation(GenericViolationInput{
		Code:              code,
		Severity:          "error",
		Repairability:     RepairNeedsTypedAction,
		Action:            action,
		RepairActionHints: cleanStrings(allowed),
		Reason:            message,
	})
	return NewGuardResult(code, "error", RepairNeedsTypedAction, message, violation)
}

func StageProgressGuardResult(input StageProgressGuardInput) GuardResult {
	state := input.State
	if !state.MaterialCoverageSufficient || !stageProgressGuardApplies(state.NextStage) {
		return GuardResult{}
	}
	missingStages := state.MissingValidationStages()
	if len(missingStages) <= 1 || !input.HasScriptedCustomTransform {
		if len(missingStages) > 1 && input.CrossesTypedActionRanks {
			message := fmt.Sprintf("data planning incomplete: workflow next_stage=%s action batch crosses multiple dependent DAG ranks while %d validation stage(s) remain: %s. Execute only the first typed action rank now, let it materialize artifacts and field contracts, then plan the next rank from real results.",
				state.NextStage, len(missingStages), strings.Join(missingStages, ", "))
			return stageProgressGuard("action_batch_crosses_dependent_ranks", message, state)
		}
		return GuardResult{}
	}
	if input.NarrowSingleIntermediateTransform {
		return GuardResult{}
	}
	message := fmt.Sprintf("data planning incomplete: workflow next_stage=%s still has %d unfinished validation stage(s): %s. Do not cross multiple unfinished data DAG stages with one custom_transform. Emit the next atomic stage first (for example derive_fields/normalize_entities/enrich_records/join_records before compute_contributions, compute_contributions before reconcile_artifacts, reconcile_artifacts before the final answer), set continue_after=true, and let the workflow evaluate the reusable artifact before the next batch.",
		state.NextStage, len(missingStages), strings.Join(missingStages, ", "))
	return stageProgressGuard("custom_transform_crosses_unfinished_stages", message, state)
}

func stageProgressGuardApplies(stage string) bool {
	switch strings.TrimSpace(stage) {
	case StageNormalizeOrEnrichEntities,
		StagePrepareContributionInputs,
		StageComputeContributions,
		StageReconcileArtifacts,
		StageEmitOutputContractAnswer:
		return true
	default:
		return false
	}
}

func stageProgressGuard(code, message string, state WorkflowStateView) GuardResult {
	violation := NewGenericViolation(GenericViolationInput{
		Code:              code,
		Severity:          "error",
		Repairability:     RepairNeedsTypedAction,
		RepairActionHints: cleanStrings(state.AllowedNextActions),
		Reason:            message,
	})
	return NewGuardResult(code, "error", RepairNeedsTypedAction, message, violation)
}

func CoverageLoopGuardResult(input CoverageLoopGuardInput) GuardResult {
	state := input.State
	if !state.MaterialCoverageSufficient ||
		!input.CoverageOnly ||
		input.GeneratedArtifactDiagnostic ||
		input.ActionsAllAllowedForCurrentStage {
		return GuardResult{}
	}
	message := fmt.Sprintf("data planning incomplete: material coverage is already sufficient for required runner materials (%d covered, missing=%d). Do not emit another coverage-only batch (material_inventory/inspect_material/derive_rules or extract_records without output_artifact) unless a specific new missing material is listed. extract_records with output_artifact may materialize already-covered sources into reusable record artifacts. Continue toward the user's data goal with compute-stage atomic actions such as derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, or assemble_answer. If you need schema diagnostics, inspect a generated artifact alias from artifact_access instead of re-covering original materials. workflow_next_stage=%s",
		state.RequiredMaterialCount-state.MissingRequiredMaterialCount,
		state.MissingRequiredMaterialCount,
		state.NextStage)
	violation := NewGenericViolation(GenericViolationInput{
		Code:              "coverage_loop_after_sufficient_materials",
		Severity:          "error",
		Repairability:     RepairNeedsTypedAction,
		RepairActionHints: cleanStrings(state.AllowedNextActions),
		Reason:            message,
	})
	return NewGuardResult("coverage_loop_after_sufficient_materials", "error", RepairNeedsTypedAction, message, violation)
}

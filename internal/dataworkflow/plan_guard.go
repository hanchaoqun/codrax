package dataworkflow

import (
	"fmt"
	"strings"
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

package repl

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/dataworkflow"
)

const (
	DefaultDataTaskMaxRepairRounds                 = 6
	DefaultDataTaskMaxDataRounds                   = 18
	DefaultDataTaskMaxNodeFailures                 = 2
	DefaultDataTaskMaxCustomTransformClassFailures = 3
	dataTaskMaxRepairRoundsCeiling                 = 12
	dataTaskMaxDataRoundsCeiling                   = 36

	dataTaskOneShotScriptLineSoftLimit   = 260
	dataTaskOneShotScriptLineHardLimit   = 420
	dataTaskComplexCustomScriptLineLimit = 180
	dataTaskOneShotRequiredMaterialLimit = 8
	dataTaskOneShotValidationLedgerLimit = 3
	dataTaskBroadMaterialDiscoveryLimit  = 12
	dataTaskMaxActionsPerBatch           = 4
	dataTaskExactExtractRecordLimit      = 100000
	dataTaskUserExplicitMaterialPurpose  = "user explicitly referenced this candidate material; keep coverage verifiable for the data goal"
)

func normalizeDataTaskMaxRepairRounds(value int) int {
	if value <= 0 {
		return DefaultDataTaskMaxRepairRounds
	}
	if value > dataTaskMaxRepairRoundsCeiling {
		return dataTaskMaxRepairRoundsCeiling
	}
	return value
}

func normalizeDataTaskMaxDataRounds(value int) int {
	if value <= 0 {
		return DefaultDataTaskMaxDataRounds
	}
	if value > dataTaskMaxDataRoundsCeiling {
		return dataTaskMaxDataRoundsCeiling
	}
	return value
}

func dataTaskRepeatedNodeFailure(records []dataTaskWorkflowRecord, currentErr string, limit int) (key string, count int, repeated bool) {
	if limit <= 0 {
		limit = DefaultDataTaskMaxNodeFailures
	}
	current := dataquery.ClassifyExecutionError(currentErr)
	key = dataTaskViolationNodeKey(current)
	if key == "" {
		return "", 0, false
	}
	count = 1
	for _, rec := range records {
		if strings.TrimSpace(rec.Err) == "" {
			continue
		}
		if dataTaskViolationNodeKey(dataquery.ClassifyExecutionError(rec.Err)) == key {
			count++
		}
	}
	return key, count, count >= limit
}

func dataTaskViolationNodeKey(v dataquery.DataTaskViolation) string {
	id := strings.TrimSpace(v.ActionID)
	if id == "" {
		return ""
	}
	kind := strings.TrimSpace(v.ActionKind)
	return id + "|" + kind
}

func dataTaskPlanStagingGuardError(plan dataquery.TaskPlan) string {
	return dataTaskPlanStagingGuardResult(plan).ErrorText()
}

func dataTaskPlanStagingGuardResult(plan dataquery.TaskPlan) dataworkflow.GuardResult {
	status := strings.ToLower(strings.TrimSpace(plan.Status))
	if status != "" && status != "ready" {
		return dataworkflow.GuardResult{}
	}
	if len(plan.Actions) > 0 {
		return dataTaskActionStagingGuardResult(plan)
	}
	if errText := dataTaskTextConstraintCoverageGuardError(plan); errText != "" {
		return dataTaskGuardResultFromMessage("text_constraint_coverage_guard", errText)
	}
	if strings.TrimSpace(plan.Script) == "" {
		return dataTaskGuardResultFromMessage("missing_executable_body", "data planning incomplete: ready data plan has no executable body. Emit a bounded actions[] batch or a script with emit_result/emit; do not mark a data plan ready when it only declares inputs, contracts, or prose.")
	}
	lines := dataTaskScriptLineCount(plan.Script)
	if lines > 0 && !dataTaskScriptHasResultEmitter(plan.Script) {
		return dataTaskGuardResultFromMessage("missing_result_emitter", fmt.Sprintf("data planning incomplete: script has no result emitter (script_lines=%d). A bounded data script must call emit(...), emit_result(...), or assign result before it can complete; otherwise split the workflow into typed actions that produce reusable artifacts.",
			lines))
	}
	requiredMaterials := len(plan.CoverageContract.RequiredMaterials)
	validationLedgers := dataTaskValidationLedgerCount(plan.CoverageContract)
	inputs := len(plan.InputPaths)
	complexBatch := requiredMaterials >= 4 || validationLedgers >= 2 || inputs >= 4
	if lines > 0 && complexBatch {
		return dataTaskGuardResultFromMessage("complex_top_level_script", fmt.Sprintf("data planning incomplete: complex data task should not start as one top-level script (script_lines=%d input_paths=%d required_materials=%d validation_ledgers=%d). Emit an atomic actions[] batch such as inspect_material, extract_records, derive_rules, derive_fields, extract_fields, group_records, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, or a bounded custom_transform, and set continue_after=true when more graph work remains.",
			lines, inputs, requiredMaterials, validationLedgers))
	}
	oversized := lines >= dataTaskOneShotScriptLineHardLimit ||
		(lines >= dataTaskOneShotScriptLineSoftLimit && (requiredMaterials >= dataTaskOneShotRequiredMaterialLimit || validationLedgers >= dataTaskOneShotValidationLedgerLimit)) ||
		(lines >= 180 && complexBatch) ||
		(requiredMaterials >= dataTaskOneShotRequiredMaterialLimit+4 && validationLedgers >= dataTaskOneShotValidationLedgerLimit)
	if !oversized {
		if errText := dataTaskTerminalRequiredMaterialSchedulingError(nil, plan); errText != "" {
			return dataTaskGuardResultFromMessage("required_material_scheduling", errText)
		}
		return dataworkflow.GuardResult{}
	}
	return dataTaskGuardResultFromMessage("oversized_data_batch", fmt.Sprintf("data planning incomplete: plan is too large for one bounded data batch (script_lines=%d input_paths=%d required_materials=%d validation_ledgers=%d continue_after=false). Emit a smaller atomic actions[] batch such as material_inventory, inspect_material, extract_records, derive_rules, derive_fields, extract_fields, group_records, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, or a bounded custom_transform; set continue_after=true when further work remains, and let the workflow feed real results into later batches.",
		lines, inputs, requiredMaterials, validationLedgers))
}

func dataTaskWorkflowStagingGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	return dataTaskWorkflowStagingGuardResult(records, plan).ErrorText()
}

func dataTaskWorkflowStagingGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	status := strings.ToLower(strings.TrimSpace(plan.Status))
	if status != "" && status != "ready" {
		return dataworkflow.GuardResult{}
	}
	if len(plan.Actions) > 0 {
		return dataTaskWorkflowActionStagingGuardResult(records, plan)
	}
	if errText := dataTaskTextConstraintCoverageGuardError(plan); errText != "" {
		return dataTaskGuardResultFromMessage("text_constraint_coverage_guard", errText)
	}
	if errText := dataTaskTerminalRequiredMaterialSchedulingError(records, plan); errText != "" {
		return dataTaskGuardResultFromMessage("required_material_scheduling", errText)
	}
	return dataTaskPlanStagingGuardResult(plan)
}

func dataTaskGuardResultFromMessage(code, message string) dataworkflow.GuardResult {
	message = strings.TrimSpace(message)
	if message == "" {
		return dataworkflow.GuardResult{}
	}
	violation := dataworkflow.NewGenericViolation(dataworkflow.GenericViolationInput{
		Code:          code,
		Severity:      "error",
		Repairability: dataworkflow.RepairNeedsTypedAction,
		Reason:        message,
	})
	return dataworkflow.NewGuardResult(code, "error", dataworkflow.RepairNeedsTypedAction, message, violation)
}

func dataTaskActionGuardResultFromMessage(code, message string, action dataquery.DataAction) dataworkflow.GuardResult {
	message = strings.TrimSpace(message)
	if message == "" {
		return dataworkflow.GuardResult{}
	}
	violation := dataworkflow.NewGenericViolation(dataworkflow.GenericViolationInput{
		Code:          code,
		Severity:      "error",
		Repairability: dataworkflow.RepairNeedsTypedAction,
		Action:        action,
		InputAliases:  action.InputPaths,
		OutputAlias:   action.OutputArtifact,
		Reason:        message,
	})
	return dataworkflow.NewGuardResult(code, "error", dataworkflow.RepairNeedsTypedAction, message, violation)
}

func dataTaskActionStagingGuardResult(plan dataquery.TaskPlan) dataworkflow.GuardResult {
	msg := dataTaskActionStagingGuardError(plan)
	if strings.TrimSpace(msg) == "" {
		return dataworkflow.GuardResult{}
	}
	if guard := dataTaskMatchingActionDependencyGuardResult(nil, plan, msg); !guard.Empty() {
		return guard
	}
	return dataTaskGuardResultFromMessage("action_staging_guard", msg)
}

func dataTaskWorkflowActionStagingGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	msg := dataTaskWorkflowActionStagingGuardError(records, plan)
	if strings.TrimSpace(msg) == "" {
		return dataworkflow.GuardResult{}
	}
	if guard := dataTaskMatchingActionDependencyGuardResult(records, plan, msg); !guard.Empty() {
		return guard
	}
	return dataTaskGuardResultFromMessage("workflow_action_staging_guard", msg)
}

func dataTaskMatchingActionDependencyGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, renderedMessage string) dataworkflow.GuardResult {
	renderedMessage = strings.TrimSpace(renderedMessage)
	if renderedMessage == "" {
		return dataworkflow.GuardResult{}
	}
	for i, action := range plan.Actions {
		guard := dataTaskActionDependencyGuardResult(records, plan, action, i)
		if guard.Empty() {
			continue
		}
		if strings.TrimSpace(guard.ErrorText()) == renderedMessage {
			return guard
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskWorkflowDeterministicFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (fallback dataquery.TaskPlan, remainder dataquery.TaskPlan, reason string, ok bool) {
	if strings.TrimSpace(dataquery.ClassifyExecutionError(errText).Code) == "text_constraint_rule_coverage_required" {
		contract := dataTaskWorkflowCoverageContract(records, plan)
		action := dataTaskRuleCoverageCompletionAction(contract)
		if strings.TrimSpace(action.ID) != "" {
			out := plan
			out.Status = "ready"
			out.Script = ""
			out.Actions = []dataquery.DataAction{action}
			out.InputPaths = mergeDataTaskInputPaths(out.InputPaths, action.InputPaths)
			out.ContinueAfter = true
			out.NextBatch = "continue with typed data processing after rule coverage materializes"
			out.WhyThisBatch = "complete source-backed rule coverage requested by the structural data validator"
			return out, dataquery.TaskPlan{}, "converted missing rule coverage to a typed derive_rules batch", true
		}
	}
	if fb, rem, hit := dataTaskInitialRankPrefixFallback(records, plan); hit {
		return fb, rem, "split initial data plan at typed dependency rank", true
	}
	if fb, rem, hit := dataTaskIntraBatchDependencyPrefixFallback(plan); hit {
		return fb, rem, "split data plan at intra-batch artifact dependency", true
	}
	if fb, hit := dataTaskNoEmitterScriptObservationFallback(records, plan, errText); hit {
		return fb, dataquery.TaskPlan{}, "converted exploratory script to atomic material observation batch", true
	}
	if fb, hit := dataTaskCoverageExpansionFallback(records, plan, errText); hit {
		return fb, dataquery.TaskPlan{}, "missing material coverage converted to atomic coverage batch", true
	}
	if fb, hit := dataTaskMaterialDiscoveryFallback(records, plan, errText); hit {
		return fb, dataquery.TaskPlan{}, "broad material plan converted to material discovery", true
	}
	if fb, hit := dataTaskCustomTransformDisabledFallback(records, plan, errText); hit {
		return fb, dataquery.TaskPlan{}, "custom_transform disabled plan converted to typed workflow fallback", true
	}
	if fb, rem, hit := dataTaskWorkflowStagePrefixFallbackWithRemainder(records, plan, errText); hit {
		return fb, rem, "trimmed multi-stage data plan to current DAG stage", true
	}
	if fb, hit := dataTaskExecutablePrefixFallback(records, plan, errText); hit {
		return fb, dataquery.TaskPlan{}, "executed valid typed prefix and discarded invalid suffix for replanning", true
	}
	if fb, hit := dataTaskInvalidRecordActionFallback(records, plan, errText); hit {
		return fb, dataquery.TaskPlan{}, "converted invalid record action to bounded record extraction", true
	}
	if fb, hit := dataTaskHistoricalMissingJoinFieldFallback(records, plan); hit {
		return fb, dataquery.TaskPlan{}, "materialized historical missing join field from existing artifacts", true
	}
	return dataquery.TaskPlan{}, dataquery.TaskPlan{}, "", false
}

func dataTaskInitialRankPrefixFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, bool) {
	if len(plan.Actions) < 2 || dataTaskWorkflowHasSuccessfulResult(records) {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	prefix, rest, split := dataworkflow.SplitInitialDiscoveryDependencyRank(plan.Actions)
	if !split || len(prefix) == 0 || len(rest) == 0 {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	out := plan
	out.Actions = prefix
	out.Script = ""
	out.ContinueAfter = true
	if strings.TrimSpace(out.WhyThisBatch) == "" {
		out.WhyThisBatch = "execute the first typed data action rank before downstream graph stages"
	}
	if strings.TrimSpace(out.NextBatch) == "" {
		out.NextBatch = "continue with the deferred typed data action ranks after this batch materializes artifacts"
	}
	remainder := plan
	remainder.Actions = rest
	remainder.Script = ""
	remainder.ContinueAfter = true
	return out, remainder, true
}

func dataTaskIntraBatchDependencyPrefixFallback(plan dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, bool) {
	dependencyIndex, _, _, ok := dataTaskFirstIntraBatchDependency(plan.Actions)
	if !ok || dependencyIndex <= 0 || dependencyIndex >= len(plan.Actions) {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	out := plan
	out.Actions = append([]dataquery.DataAction(nil), plan.Actions[:dependencyIndex]...)
	out.Script = ""
	out.ContinueAfter = true
	if strings.TrimSpace(out.WhyThisBatch) == "" {
		out.WhyThisBatch = "execute the producer action rank before consuming its generated artifact"
	}
	if strings.TrimSpace(out.NextBatch) == "" {
		out.NextBatch = "continue with the deferred dependent action after real artifact fields are available"
	}
	remainder := plan
	remainder.Actions = append([]dataquery.DataAction(nil), plan.Actions[dependencyIndex:]...)
	remainder.Script = ""
	remainder.ContinueAfter = true
	return out, remainder, true
}

func dataTaskFirstIntraBatchDependency(actions []dataquery.DataAction) (int, string, dataquery.DataAction, bool) {
	produced := map[string]dataquery.DataAction{}
	for i, action := range actions {
		for _, input := range cleanDataTaskStrings(action.InputPaths) {
			key := normalizeDataTaskCoveragePath(input)
			if key == "" {
				continue
			}
			if producer, ok := produced[key]; ok {
				return i, input, producer, true
			}
		}
		for _, alias := range dataTaskActionOutputAliases(action) {
			key := normalizeDataTaskCoveragePath(alias)
			if key != "" {
				produced[key] = action
			}
		}
	}
	return 0, "", dataquery.DataAction{}, false
}

func dataTaskNoEmitterScriptObservationFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	if len(plan.Actions) > 0 || strings.TrimSpace(plan.Script) == "" || dataTaskScriptHasResultEmitter(plan.Script) {
		return dataquery.TaskPlan{}, false
	}
	lines := dataTaskScriptLineCount(plan.Script)
	if lines == 0 {
		return dataquery.TaskPlan{}, false
	}
	paths := dataTaskDiscoveryPaths(plan)
	requiredMaterials := len(plan.CoverageContract.RequiredMaterials)
	validationLedgers := dataTaskValidationLedgerCount(plan.CoverageContract)
	complex := len(paths) >= 3 || requiredMaterials >= 3 || validationLedgers >= 2
	if !complex {
		return dataquery.TaskPlan{}, false
	}
	state := dataTaskWorkflowState(records, plan)
	if state.MaterialCoverageSufficient && len(records) > 0 {
		return dataquery.TaskPlan{}, false
	}
	var ruleInputs, structuredInputs, inspectInputs []string
	for _, p := range paths {
		switch {
		case dataTaskPathLooksLikeTextConstraintMaterial(p) && dataTaskPlanShouldDeriveRulesForTextCoverage(plan):
			ruleInputs = append(ruleInputs, p)
		case dataTaskPathLooksLikeStructuredMaterial(p):
			structuredInputs = append(structuredInputs, p)
		default:
			inspectInputs = append(inspectInputs, p)
		}
	}
	var actions []dataquery.DataAction
	if len(ruleInputs) > 0 {
		actions = append(actions, dataquery.DataAction{
			ID:             "observe_rules",
			Kind:           dataquery.DataActionDeriveRules,
			Purpose:        "derive generic rule records from rule or constraint materials before calculation",
			InputPaths:     cleanDataTaskStrings(ruleInputs),
			OutputArtifact: "observed_rules.json",
		})
	}
	if len(structuredInputs) > 0 {
		actions = append(actions, dataquery.DataAction{
			ID:             "observe_records",
			Kind:           dataquery.DataActionExtractRecords,
			Purpose:        "extract bounded generic record samples before choosing the next typed data action",
			InputPaths:     cleanDataTaskStrings(structuredInputs),
			OutputArtifact: "observed_records.json",
			Params:         map[string]string{"limit": "120"},
		})
	}
	if len(inspectInputs) > 0 {
		actions = append(actions, dataquery.DataAction{
			ID:             "observe_materials",
			Kind:           dataquery.DataActionInspectMaterial,
			Purpose:        "inspect non-structured materials before choosing the next typed data action",
			InputPaths:     cleanDataTaskStrings(inspectInputs),
			OutputArtifact: "observed_materials.json",
		})
	}
	if len(actions) == 0 {
		return dataquery.TaskPlan{}, false
	}
	if len(actions) > dataTaskMaxActionsPerBatch {
		actions = actions[:dataTaskMaxActionsPerBatch]
	}
	contract := dataTaskWorkflowCoverageContract(records, plan)
	if strings.TrimSpace(errText) != "" {
		contract.ValidationRules = mergeDataTaskValidationRules(
			contract.ValidationRules,
			[]string{"previous exploratory script had no result emitter and was converted into typed observation actions: " + oneLineClamp(errText, 240)},
		)
	}
	out := plan
	out.Status = "ready"
	out.Script = ""
	out.Actions = actions
	out.InputPaths = paths
	out.OutputContract = dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true}
	out.CoverageContract = contract
	out.ContinueAfter = true
	out.WhyThisBatch = "observe objective material and rule structure before later bounded calculation actions"
	out.NextBatch = "continue with typed data workflow actions over the observed artifacts"
	if strings.TrimSpace(out.Goal) == "" {
		out.Goal = "observe data task materials"
	}
	return out, true
}

func dataTaskRepairPlannerErrorAllowsContinuation(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "data task planner returned no tool_call") ||
		strings.Contains(text, "compact tool-param repair returned no tool_call")
}

func normalizeDataTaskPlanShape(plan dataquery.TaskPlan) (dataquery.TaskPlan, []string) {
	var reasons []string
	if normalizeDataTaskCompleteStatusWithExecutablePayload(&plan) {
		reasons = append(reasons, "changed status=complete to ready because the plan contains executable script/actions")
	}
	if normalizeDataTaskPlanPathLists(&plan) {
		reasons = append(reasons, "normalized comma-separated path lists in data plan fields")
	}
	if dataworkflow.NormalizeRolePathActionInputs(&plan) {
		reasons = append(reasons, "filled typed action input_paths from role path params")
	}
	if normalizeDataTaskMaterializationActionInputs(&plan) {
		reasons = append(reasons, "copied batch input_paths into materialization actions with empty input_paths")
	}
	if normalizeDataTaskCustomActionScriptInputs(&plan) {
		reasons = append(reasons, "added top-level declared inputs referenced by custom_transform scripts")
	}
	if normalizeDataTaskDeriveRulesInputs(&plan) {
		reasons = append(reasons, "filled derive_rules input_paths from required rule/constraint materials")
	}
	if normalizeDataTaskPlanContractFromActions(&plan) {
		reasons = append(reasons, "enabled validation contracts implied by typed data actions such as derive_rules/compute_contributions/reconcile_artifacts")
	}
	if len(plan.Actions) > 0 && strings.TrimSpace(plan.Script) != "" && !dataTaskScriptHasResultEmitter(plan.Script) {
		plan.Script = ""
		reasons = append(reasons, "removed non-emitting top-level script from an actions[] plan")
	}
	if len(plan.Actions) == 0 && strings.TrimSpace(plan.Script) != "" {
		if dataTaskPlanCanWrapTopLevelScriptAsCustomAction(plan) {
			actionID := "custom_transform_1"
			if strings.TrimSpace(plan.Goal) != "" {
				actionID = "bounded_transform"
			}
			plan.Actions = []dataquery.DataAction{{
				ID:         actionID,
				Kind:       dataquery.DataActionCustomTransform,
				Purpose:    firstNonEmptyString(strings.TrimSpace(plan.WhyThisBatch), strings.TrimSpace(plan.Goal), "bounded deterministic data transform"),
				InputPaths: append([]string(nil), plan.InputPaths...),
				Script:     plan.Script,
			}}
			plan.Script = ""
			reasons = append(reasons, "wrapped bounded top-level script into a custom_transform action")
			if normalized, ok := normalizeDataTaskOversizedActionBatch(plan); ok {
				plan = normalized
				reasons = append(reasons, "trimmed oversized actions[] plan to the next executable batch")
			}
			return plan, reasons
		}
	}
	if len(plan.Actions) == 0 || strings.TrimSpace(plan.Script) == "" {
		if normalized, ok := normalizeDataTaskOversizedActionBatch(plan); ok {
			plan = normalized
			reasons = append(reasons, "trimmed oversized actions[] plan to the next executable batch")
		}
		return plan, reasons
	}
	if dataTaskTopLevelScriptDuplicatesActionScript(plan.Script, plan.Actions) {
		plan.Script = ""
		reasons = append(reasons, "removed duplicate top-level script from actions[] plan")
		if normalized, ok := normalizeDataTaskOversizedActionBatch(plan); ok {
			plan = normalized
			reasons = append(reasons, "trimmed oversized actions[] plan to the next executable batch")
		}
		return plan, reasons
	}
	if len(plan.Actions) > 0 && strings.TrimSpace(plan.Script) != "" {
		plan.Script = ""
		reasons = append(reasons, "removed stray top-level script from an actions[] plan; actions[] is the executable DAG contract")
		if normalized, ok := normalizeDataTaskOversizedActionBatch(plan); ok {
			plan = normalized
			reasons = append(reasons, "trimmed oversized actions[] plan to the next executable batch")
		}
		return plan, reasons
	}
	if normalized, ok := normalizeDataTaskOversizedActionBatch(plan); ok {
		plan = normalized
		reasons = append(reasons, "trimmed oversized actions[] plan to the next executable batch")
	}
	return plan, reasons
}

func normalizeDataTaskCompleteStatusWithExecutablePayload(plan *dataquery.TaskPlan) bool {
	if plan == nil || !strings.EqualFold(strings.TrimSpace(plan.Status), "complete") {
		return false
	}
	if strings.TrimSpace(plan.Script) == "" && len(plan.Actions) == 0 {
		return false
	}
	plan.Status = "ready"
	return true
}

func normalizeDataTaskCustomActionScriptInputs(plan *dataquery.TaskPlan) bool {
	if plan == nil || len(plan.Actions) == 0 {
		return false
	}
	topInputs := cleanDataTaskStrings(plan.InputPaths)
	if len(topInputs) == 0 {
		return false
	}
	changed := false
	for i := range plan.Actions {
		if normalizeDataActionKindForWorkflow(plan.Actions[i].Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		script := strings.TrimSpace(plan.Actions[i].Script)
		if script == "" {
			continue
		}
		inputSet := map[string]bool{}
		for _, p := range cleanDataTaskStrings(plan.Actions[i].InputPaths) {
			inputSet[normalizeDataTaskCoveragePath(p)] = true
		}
		actionChanged := false
		for _, p := range topInputs {
			normalized := normalizeDataTaskCoveragePath(p)
			if normalized == "" || inputSet[normalized] || !dataTaskScriptReferencesPathLiteral(script, normalized) {
				continue
			}
			plan.Actions[i].InputPaths = append(plan.Actions[i].InputPaths, p)
			inputSet[normalized] = true
			actionChanged = true
			changed = true
		}
		if actionChanged {
			plan.Actions[i].InputPaths = cleanDataTaskStrings(plan.Actions[i].InputPaths)
		}
	}
	return changed
}

func normalizeDataTaskMaterializationActionInputs(plan *dataquery.TaskPlan) bool {
	if plan == nil || len(plan.Actions) == 0 {
		return false
	}
	topInputs := cleanDataTaskStrings(plan.InputPaths)
	if len(topInputs) == 0 {
		return false
	}
	changed := false
	for i := range plan.Actions {
		if len(cleanDataTaskStrings(plan.Actions[i].InputPaths)) > 0 {
			continue
		}
		if !dataTaskActionCanInheritBatchInputs(plan.Actions[i]) {
			continue
		}
		plan.Actions[i].InputPaths = append([]string(nil), topInputs...)
		changed = true
	}
	return changed
}

func dataTaskActionCanInheritBatchInputs(action dataquery.DataAction) bool {
	if strings.TrimSpace(action.Script) != "" {
		return false
	}
	switch normalizeDataActionKindForWorkflow(action.Kind) {
	case dataquery.DataActionMaterialInventory, dataquery.DataActionInspectMaterial, dataquery.DataActionExtractRecords, dataquery.DataActionExtractFields, dataquery.DataActionGroupRecords:
		return true
	default:
		return false
	}
}

func dataTaskScriptReferencesPathLiteral(script, p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	return strings.Contains(script, `"`+p+`"`) || strings.Contains(script, `'`+p+`'`)
}

func normalizeDataTaskPlanShapeForPolicy(plan dataquery.TaskPlan, policy TurnPolicy) (dataquery.TaskPlan, []string) {
	var reasons []string
	if normalizeDataTaskAggregationContractFromPolicy(&plan, policy) {
		reasons = append(reasons, "enabled contribution/reconcile contract for a complex typed data aggregation plan")
	}
	if normalizeDataTaskStrictOutputContractFromShape(&plan) {
		reasons = append(reasons, "enabled contribution/reconcile contract for a complex strict-output data plan")
	}
	normalized, shapeReasons := normalizeDataTaskPlanShape(plan)
	reasons = append(reasons, shapeReasons...)
	if normalizeDataTaskAggregationContractFromPolicy(&normalized, policy) {
		reasons = append(reasons, "preserved contribution/reconcile contract after data plan shape normalization")
	}
	if normalizeDataTaskStrictOutputContractFromShape(&normalized) {
		reasons = append(reasons, "preserved contribution/reconcile contract after strict-output shape normalization")
	}
	return normalized, reasons
}

func normalizeDataTaskAggregationContractFromPolicy(plan *dataquery.TaskPlan, policy TurnPolicy) bool {
	if plan == nil || !dataTaskPolicyRequiresAggregationLedger(policy) || !dataTaskPlanIsComplexAggregationCandidate(*plan) {
		return false
	}
	changed := false
	if !plan.CoverageContract.ContributionLedgerRequired {
		plan.CoverageContract.ContributionLedgerRequired = true
		changed = true
	}
	if !plan.CoverageContract.ReconcileRequired {
		plan.CoverageContract.ReconcileRequired = true
		changed = true
	}
	return changed
}

func dataTaskPolicyRequiresAggregationLedger(policy TurnPolicy) bool {
	switch strings.ToLower(strings.TrimSpace(firstNonEmptyString(policy.DataTaskKind, policy.Operation))) {
	case "data_aggregation":
		return true
	default:
		return false
	}
}

func dataTaskPlanIsComplexAggregationCandidate(plan dataquery.TaskPlan) bool {
	return len(plan.CoverageContract.RequiredMaterials) >= 2 ||
		len(plan.InputPaths) >= 2 ||
		len(plan.Actions) > 0
}

func normalizeDataTaskStrictOutputContractFromShape(plan *dataquery.TaskPlan) bool {
	if plan == nil || !dataTaskPlanHasStrictOutputContract(*plan) || !dataTaskPlanIsComplexStrictOutputCandidate(*plan) {
		return false
	}
	changed := false
	if !plan.CoverageContract.DecisionRecordsRequired {
		plan.CoverageContract.DecisionRecordsRequired = true
		changed = true
	}
	if !plan.CoverageContract.ContributionLedgerRequired {
		plan.CoverageContract.ContributionLedgerRequired = true
		changed = true
	}
	if !plan.CoverageContract.ReconcileRequired {
		plan.CoverageContract.ReconcileRequired = true
		changed = true
	}
	return changed
}

func dataTaskPlanHasStrictOutputContract(plan dataquery.TaskPlan) bool {
	contract := plan.OutputContract.Normalize()
	if contract.ExplanationAllowed {
		return false
	}
	switch contract.Format {
	case dataquery.OutputPlainSingleLine, dataquery.OutputCSVLine, dataquery.OutputJSONOnly, dataquery.OutputMarkdownTable:
		return true
	default:
		return false
	}
}

func dataTaskPlanIsComplexStrictOutputCandidate(plan dataquery.TaskPlan) bool {
	return len(plan.CoverageContract.RequiredMaterials) >= 4 ||
		len(plan.InputPaths) >= 4 ||
		len(plan.Actions) >= 2 ||
		dataTaskScriptLineCount(plan.Script) >= 40
}

func normalizeDataTaskDeriveRulesInputs(plan *dataquery.TaskPlan) bool {
	if plan == nil || len(plan.Actions) == 0 {
		return false
	}
	inputs := dataTaskRuleMaterialInputs(plan.CoverageContract)
	changed := false
	for i := range plan.Actions {
		if normalizeDataActionKindForWorkflow(plan.Actions[i].Kind) != dataquery.DataActionDeriveRules {
			continue
		}
		current := cleanDataTaskStrings(plan.Actions[i].InputPaths)
		if len(current) == 0 {
			if len(inputs) == 0 || dataTaskDeriveRulesActionHasExplicitRules(plan.Actions[i]) {
				continue
			}
			plan.Actions[i].InputPaths = append([]string(nil), inputs...)
			changed = true
			continue
		}
		filtered := filterDataTaskRuleMaterialInputs(current, inputs)
		if len(filtered) == len(current) && strings.Join(filtered, "\x00") == strings.Join(current, "\x00") {
			continue
		}
		if len(filtered) > 0 {
			plan.Actions[i].InputPaths = filtered
		} else if len(plan.CoverageContract.ValidationRules) > 0 || dataTaskDeriveRulesActionHasExplicitRules(plan.Actions[i]) {
			plan.Actions[i].InputPaths = nil
		} else if len(inputs) > 0 {
			plan.Actions[i].InputPaths = append([]string(nil), inputs...)
		} else {
			continue
		}
		changed = true
	}
	return changed
}

func dataTaskRuleMaterialInputs(contract dataquery.CoverageContract) []string {
	var out []string
	collect := func(materials []dataquery.CoverageMaterial) {
		for _, material := range materials {
			mode := normalizeCoverageMaterialUseModeForWorkflow(material.UsageMode)
			var p string
			switch mode {
			case dataquery.MaterialUseScriptConsumed, dataquery.MaterialUsePlannerDistilled:
				p = normalizeDataTaskCoveragePath(material.Path)
			case dataquery.MaterialUseTextEvidenceConsumed:
				p = normalizeDataTaskCoveragePath(material.TextEvidencePath)
			default:
				continue
			}
			if p == "" || !dataTaskPathLooksLikeTextConstraintMaterial(p) {
				continue
			}
			out = append(out, p)
		}
	}
	collect(contract.RequiredMaterials)
	collect(contract.OptionalMaterials)
	return cleanDataTaskStrings(out)
}

func filterDataTaskRuleMaterialInputs(current, allowed []string) []string {
	allowedSet := map[string]bool{}
	for _, p := range cleanDataTaskStrings(allowed) {
		allowedSet[normalizeDataTaskCoveragePath(p)] = true
	}
	allowTextShapeFallback := len(allowedSet) == 0
	var out []string
	for _, p := range cleanDataTaskStrings(current) {
		key := normalizeDataTaskCoveragePath(p)
		if key == "" {
			continue
		}
		if allowedSet[key] || (allowTextShapeFallback && dataTaskPathLooksLikeTextConstraintMaterial(key)) {
			out = append(out, p)
		}
	}
	return cleanDataTaskStrings(out)
}

func dataTaskDeriveRulesActionHasExplicitRules(action dataquery.DataAction) bool {
	if len(action.Params) == 0 {
		return false
	}
	for _, key := range []string{"rules_json", "rules", "validation_rules_json", "rule_text", "rule"} {
		if strings.TrimSpace(action.Params[key]) != "" {
			return true
		}
	}
	return false
}

func dataTaskRequiredRuleMaterialInputs(contract dataquery.CoverageContract) []string {
	var out []string
	for _, material := range contract.RequiredMaterials {
		mode := normalizeCoverageMaterialUseModeForWorkflow(material.UsageMode)
		var p string
		switch mode {
		case dataquery.MaterialUseScriptConsumed:
			p = normalizeDataTaskCoveragePath(material.Path)
		case dataquery.MaterialUseTextEvidenceConsumed:
			p = normalizeDataTaskCoveragePath(material.TextEvidencePath)
		default:
			continue
		}
		if p == "" || !dataTaskPathLooksLikeTextConstraintMaterial(p) {
			continue
		}
		out = append(out, p)
	}
	return cleanDataTaskStrings(out)
}

func dataTaskTopLevelScriptDuplicatesActionScript(script string, actions []dataquery.DataAction) bool {
	normalizedTop := normalizeDataTaskScriptForDuplicateCompare(script)
	if normalizedTop == "" {
		return false
	}
	for _, action := range actions {
		if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		if normalizedTop == normalizeDataTaskScriptForDuplicateCompare(action.Script) {
			return true
		}
	}
	return false
}

func normalizeDataTaskScriptForDuplicateCompare(script string) string {
	var lines []string
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func normalizeDataTaskPlanPathLists(plan *dataquery.TaskPlan) bool {
	if plan == nil {
		return false
	}
	changed := false
	normalize := func(values []string) []string {
		next := expandDataTaskPathListStrings(values)
		if strings.Join(next, "\x00") != strings.Join(cleanDataTaskStrings(values), "\x00") {
			changed = true
		}
		return next
	}
	plan.InputPaths = normalize(plan.InputPaths)
	for i := range plan.Actions {
		plan.Actions[i].InputPaths = normalize(plan.Actions[i].InputPaths)
	}
	normalizeMaterials := func(materials []dataquery.CoverageMaterial) []dataquery.CoverageMaterial {
		var out []dataquery.CoverageMaterial
		for _, material := range materials {
			expanded := expandDataTaskCoverageMaterialPaths(material)
			if len(expanded) != 1 || expanded[0].Path != material.Path || expanded[0].TextEvidencePath != material.TextEvidencePath {
				changed = true
			}
			out = append(out, expanded...)
		}
		return out
	}
	plan.CoverageContract.RequiredMaterials = normalizeMaterials(plan.CoverageContract.RequiredMaterials)
	plan.CoverageContract.OptionalMaterials = normalizeMaterials(plan.CoverageContract.OptionalMaterials)
	return changed
}

func expandDataTaskCoverageMaterialPaths(material dataquery.CoverageMaterial) []dataquery.CoverageMaterial {
	paths := expandDataTaskPathListStrings([]string{material.Path})
	textPaths := expandDataTaskPathListStrings([]string{material.TextEvidencePath})
	if len(paths) == 0 {
		paths = []string{strings.TrimSpace(material.Path)}
	}
	if len(paths) <= 1 && len(textPaths) <= 1 {
		material.Path = strings.TrimSpace(material.Path)
		material.TextEvidencePath = strings.TrimSpace(material.TextEvidencePath)
		return []dataquery.CoverageMaterial{material}
	}
	out := make([]dataquery.CoverageMaterial, 0, maxInt(len(paths), len(textPaths)))
	n := maxInt(len(paths), len(textPaths))
	for i := 0; i < n; i++ {
		next := material
		if i < len(paths) {
			next.Path = paths[i]
		} else {
			next.Path = ""
		}
		if i < len(textPaths) {
			next.TextEvidencePath = textPaths[i]
		} else if len(textPaths) == 1 {
			next.TextEvidencePath = textPaths[0]
		} else {
			next.TextEvidencePath = ""
		}
		if len(paths) > 1 || len(textPaths) > 1 {
			baseID := strings.TrimSpace(material.ID)
			if baseID != "" {
				next.ID = fmt.Sprintf("%s_%d", baseID, i+1)
			}
		}
		if strings.TrimSpace(next.Path) == "" && strings.TrimSpace(next.TextEvidencePath) == "" {
			continue
		}
		out = append(out, next)
	}
	return out
}

func expandDataTaskPathListStrings(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parts := splitDataTaskPathListString(value)
		if len(parts) == 0 {
			out = append(out, value)
			continue
		}
		out = append(out, parts...)
	}
	return cleanDataTaskStrings(out)
}

func splitDataTaskPathListString(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.Contains(value, ",") {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !dataTaskStringLooksLikePath(part) {
			return nil
		}
		out = append(out, part)
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

func dataTaskStringLooksLikePath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\n\r") {
		return false
	}
	if strings.Contains(value, "/") || strings.Contains(value, "\\") {
		return true
	}
	ext := path.Ext(value)
	return len(ext) > 1 && len(ext) <= 12
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func normalizeDataTaskOversizedActionBatch(plan dataquery.TaskPlan) (dataquery.TaskPlan, bool) {
	if len(plan.Actions) <= dataTaskMaxActionsPerBatch {
		return plan, false
	}
	keep := make([]dataquery.DataAction, 0, dataTaskMaxActionsPerBatch)
	for _, action := range plan.Actions {
		if len(keep) >= dataTaskMaxActionsPerBatch {
			break
		}
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		if kind == dataquery.DataActionCustomTransform && strings.TrimSpace(action.Script) != "" && len(keep) > 0 {
			break
		}
		keep = append(keep, action)
	}
	if len(keep) == 0 {
		keep = append(keep, plan.Actions[:dataTaskMaxActionsPerBatch]...)
	}
	plan.Actions = keep
	plan.Script = ""
	plan.ContinueAfter = true
	if strings.TrimSpace(plan.NextBatch) == "" {
		plan.NextBatch = "continue with the remaining data workflow actions after this bounded batch produces artifacts"
	}
	if strings.TrimSpace(plan.WhyThisBatch) == "" {
		plan.WhyThisBatch = "execute the next atomic data workflow batch within the system action budget"
	}
	return plan, true
}

func normalizeDataTaskPlanContractFromActions(plan *dataquery.TaskPlan) bool {
	if plan == nil {
		return false
	}
	changed := false
	for _, action := range plan.Actions {
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		if dataworkflow.IsLeafFallback(kind) {
			continue
		}
		if dataworkflow.ProducesLedger(kind, dataworkflow.LedgerRuleCoverage) && !plan.CoverageContract.RuleCoverageRequired {
			plan.CoverageContract.RuleCoverageRequired = true
			changed = true
		}
		if dataworkflow.ProducesLedger(kind, dataworkflow.LedgerDecisions) && !plan.CoverageContract.DecisionRecordsRequired {
			plan.CoverageContract.DecisionRecordsRequired = true
			changed = true
		}
		if dataworkflow.ProducesLedger(kind, dataworkflow.LedgerContributions) && !plan.CoverageContract.ContributionLedgerRequired {
			plan.CoverageContract.ContributionLedgerRequired = true
			changed = true
		}
		if dataworkflow.ProducesLedger(kind, dataworkflow.LedgerReconcile) && !plan.CoverageContract.ReconcileRequired {
			plan.CoverageContract.ReconcileRequired = true
			changed = true
		}
		if dataworkflow.ProducesLedger(kind, dataworkflow.LedgerFinalProjection) && !plan.CoverageContract.ReconcileRequired {
			plan.CoverageContract.ReconcileRequired = true
			changed = true
		}
	}
	return changed
}

func dataTaskPlanCanWrapTopLevelScriptAsCustomAction(plan dataquery.TaskPlan) bool {
	lines := dataTaskScriptLineCount(plan.Script)
	if lines == 0 || lines >= dataTaskComplexCustomScriptLineLimit {
		return false
	}
	if !dataTaskScriptHasResultEmitter(plan.Script) {
		return false
	}
	requiredMaterials := len(plan.CoverageContract.RequiredMaterials)
	validationLedgers := dataTaskValidationLedgerCount(plan.CoverageContract)
	inputs := len(plan.InputPaths)
	return requiredMaterials >= 4 || validationLedgers >= 2 || inputs >= 4
}

func dataTaskMaterialDiscoveryFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	state := dataTaskWorkflowState(records, plan)
	if state.MaterialCoverageSufficient {
		return dataquery.TaskPlan{}, false
	}
	if dataTaskWorkflowHasActionKind(records, dataquery.DataActionMaterialInventory) {
		return dataquery.TaskPlan{}, false
	}
	if len(plan.Actions) == 1 && normalizeDataActionKindForWorkflow(plan.Actions[0].Kind) == dataquery.DataActionMaterialInventory {
		return dataquery.TaskPlan{}, false
	}
	if dataTaskPlanNonCustomActionCount(plan.Actions, len(plan.Actions)) > 0 {
		return dataquery.TaskPlan{}, false
	}
	paths := dataTaskDiscoveryPaths(plan)
	if len(paths) < dataTaskBroadMaterialDiscoveryLimit {
		return dataquery.TaskPlan{}, false
	}
	if strings.TrimSpace(plan.Script) == "" && !dataTaskPlanHasCustomTransform(plan) {
		return dataquery.TaskPlan{}, false
	}
	contract := dataTaskWorkflowCoverageContract(records, plan)
	contract.RequiredMaterials = nil
	contract.OptionalMaterials = nil
	out := dataquery.TaskPlan{
		Status:           "ready",
		InputPaths:       paths,
		OutputContract:   dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true},
		CoverageContract: contract,
		Goal:             strings.TrimSpace(plan.Goal),
		SuccessCriteria:  append([]string(nil), plan.SuccessCriteria...),
		ContinueAfter:    true,
		WhyThisBatch:     "discover objective material inventory before choosing the next bounded data action batch",
		Actions: []dataquery.DataAction{{
			ID:         "material_inventory",
			Kind:       dataquery.DataActionMaterialInventory,
			Purpose:    "discover material types, paths, and objective metadata for the next data workflow batch",
			InputPaths: paths,
		}},
	}
	if strings.TrimSpace(out.Goal) == "" {
		out.Goal = "discover data task materials"
	}
	if strings.TrimSpace(errText) != "" {
		out.CoverageContract.ValidationRules = mergeDataTaskValidationRules(
			out.CoverageContract.ValidationRules,
			[]string{"previous broad plan was converted into material discovery: " + oneLineClamp(errText, 240)},
		)
	}
	return out, true
}

func dataTaskCoverageExpansionFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	state := dataTaskWorkflowState(records, plan)
	if state.MaterialCoverageSufficient && dataTaskPlanIsCoverageOnly(plan) {
		return dataquery.TaskPlan{}, false
	}
	if !dataTaskCoverageExpansionFallbackNeeded(records, plan) {
		return dataquery.TaskPlan{}, false
	}
	return dataTaskBuildCoverageExpansionFallback(records, plan, errText)
}

func dataTaskCoverageExpansionFallbackAfterResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	missing := dataTaskCoverageExpansionMissingPaths(records, plan)
	if len(missing) == 0 {
		return dataquery.TaskPlan{}, false
	}
	return dataTaskBuildCoverageExpansionFallback(records, plan, errText)
}

func dataTaskBuildCoverageExpansionFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	missing := dataTaskCoverageExpansionMissingPaths(records, plan)
	if len(missing) == 0 {
		return dataquery.TaskPlan{}, false
	}
	var ruleInputs, structuredInputs, inspectInputs []string
	for _, p := range missing {
		switch {
		case dataTaskPathLooksLikeTextConstraintMaterial(p) && dataTaskPlanShouldDeriveRulesForTextCoverage(plan):
			ruleInputs = append(ruleInputs, p)
		case dataTaskPathLooksLikeStructuredMaterial(p):
			structuredInputs = append(structuredInputs, p)
		default:
			inspectInputs = append(inspectInputs, p)
		}
	}
	var actions []dataquery.DataAction
	if len(ruleInputs) > 0 {
		actions = append(actions, dataquery.DataAction{
			ID:             "cover_required_rules",
			Kind:           dataquery.DataActionDeriveRules,
			Purpose:        "derive generic rules from required text or constraint materials before later data transforms",
			InputPaths:     cleanDataTaskStrings(ruleInputs),
			OutputArtifact: "coverage_rules.json",
		})
	}
	if len(structuredInputs) > 0 {
		actions = append(actions, dataquery.DataAction{
			ID:             "cover_required_records",
			Kind:           dataquery.DataActionExtractRecords,
			Purpose:        "extract record samples from required structured materials before later data transforms",
			InputPaths:     cleanDataTaskStrings(structuredInputs),
			OutputArtifact: "coverage_records.json",
			Params:         map[string]string{"limit": "120"},
		})
	}
	if len(inspectInputs) > 0 {
		actions = append(actions, dataquery.DataAction{
			ID:             "cover_required_materials",
			Kind:           dataquery.DataActionInspectMaterial,
			Purpose:        "inspect required materials before later data transforms",
			InputPaths:     cleanDataTaskStrings(inspectInputs),
			OutputArtifact: "coverage_inspection.json",
		})
	}
	if len(actions) == 0 {
		return dataquery.TaskPlan{}, false
	}
	if len(actions) > dataTaskMaxActionsPerBatch {
		actions = actions[:dataTaskMaxActionsPerBatch]
	}
	contract := dataTaskWorkflowCoverageContract(records, plan)
	if len(contract.ValidationRules) == 0 && strings.TrimSpace(errText) != "" {
		contract.ValidationRules = []string{"previous structural coverage guard requested an atomic material-coverage batch: " + oneLineClamp(errText, 240)}
	}
	return dataquery.TaskPlan{
		Status:           "ready",
		InputPaths:       missing,
		OutputContract:   dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true},
		CoverageContract: contract,
		Goal:             strings.TrimSpace(plan.Goal),
		KnownConstraints: append([]string(nil), plan.KnownConstraints...),
		SuccessCriteria:  append([]string(nil), plan.SuccessCriteria...),
		ContinueAfter:    true,
		WhyThisBatch:     "cover missing required or prerequisite materials with atomic data actions before later transforms",
		NextBatch:        "continue the data workflow using these material artifacts instead of re-planning the same broad transform",
		Actions:          actions,
	}, true
}

func dataTaskPlanShouldDeriveRulesForTextCoverage(plan dataquery.TaskPlan) bool {
	return plan.CoverageContract.RuleCoverageRequired || dataTaskValidationLedgerCount(plan.CoverageContract) >= 2
}

func dataTaskCoverageExpansionFallbackNeeded(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) bool {
	if len(plan.Actions) == 0 && strings.TrimSpace(plan.Script) == "" && len(dataTaskCoverageExpansionMissingPaths(records, plan)) > 0 {
		return true
	}
	if strings.TrimSpace(dataTaskTerminalRequiredMaterialSchedulingError(records, plan)) != "" {
		return true
	}
	for i, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		if !dataTaskActionHasBroadPrerequisiteSurface(plan, action) {
			continue
		}
		if len(dataTaskMissingCustomTransformPrerequisites(records, plan, action, i)) > 0 {
			return true
		}
	}
	return false
}

func applyDataTaskUserMaterialFloor(userLine string, candidates []dataquery.CandidateFile, plan dataquery.TaskPlan) dataquery.TaskPlan {
	materials := dataTaskUserMentionedCandidateMaterials(userLine, candidates)
	if len(materials) == 0 {
		return plan
	}
	plan.CoverageContract.RequiredMaterials = mergeDataTaskCoverageMaterials(materials, plan.CoverageContract.RequiredMaterials, true)
	requiredKeys := dataTaskCoverageMaterialKeySet(plan.CoverageContract.RequiredMaterials)
	plan.CoverageContract.OptionalMaterials = dataTaskFilterCoverageMaterials(plan.CoverageContract.OptionalMaterials, func(m dataquery.CoverageMaterial) bool {
		key := dataTaskCoverageMaterialKey(m)
		return key == "" || !requiredKeys[key]
	})
	plan.CoverageContract.ValidationRules = mergeDataTaskValidationRules(
		[]string{"user-explicit candidate materials must remain in the coverage contract with a verifiable usage_mode"},
		plan.CoverageContract.ValidationRules,
	)
	plan.InputPaths = mergeDataTaskInputPaths(plan.InputPaths, plan.CoverageContract.RequiredRunnerInputPaths())
	return plan
}

func prepareDataTaskWorkflowPlanForExecution(userLine string, candidates []dataquery.CandidateFile, records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataquery.TaskPlan {
	plan = applyDataTaskUserMaterialFloor(userLine, candidates, plan)
	normalizeDataTaskPlanPathLists(&plan)
	dataworkflow.NormalizeRolePathActionInputs(&plan)
	if bootstrap, ok := dataTaskCandidateInventoryBootstrapPlan(candidates, records, plan); ok {
		return bootstrap
	}
	normalizeDataTaskMaterializationActionInputs(&plan)
	normalizeDataTaskCustomActionScriptInputs(&plan)
	normalizeDataTaskExactExtractLimitsForWorkflow(&plan)
	plan = dataTaskNarrowSingleRecordSetActionInputsForExecution(records, plan)
	return dataTaskScopePlanToCurrentBatch(records, plan)
}

func dataTaskCandidateInventoryBootstrapPlan(candidates []dataquery.CandidateFile, records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) (dataquery.TaskPlan, bool) {
	if len(records) > 0 || len(candidates) == 0 {
		return dataquery.TaskPlan{}, false
	}
	if dataTaskValidationLedgerCount(plan.CoverageContract) < 2 && !dataTaskPlanRequiresCompleteRecordSets(plan) {
		return dataquery.TaskPlan{}, false
	}
	if dataTaskPlanHasActionKind(plan, dataquery.DataActionMaterialInventory) {
		return dataquery.TaskPlan{}, false
	}
	candidatePaths := dataTaskCandidatePaths(candidates)
	if len(candidatePaths) == 0 {
		return dataquery.TaskPlan{}, false
	}
	planned := map[string]bool{}
	for _, p := range dataTaskDiscoveryPaths(plan) {
		planned[normalizeDataTaskCoveragePath(p)] = true
	}
	var omitted []string
	for _, p := range candidatePaths {
		key := normalizeDataTaskCoveragePath(p)
		if key == "" || planned[key] {
			continue
		}
		omitted = append(omitted, p)
	}
	if len(omitted) == 0 {
		return dataquery.TaskPlan{}, false
	}
	out := plan
	out.Status = "ready"
	out.Script = ""
	out.InputPaths = candidatePaths
	out.Actions = []dataquery.DataAction{{
		ID:             "material_inventory",
		Kind:           dataquery.DataActionMaterialInventory,
		Purpose:        "discover objective candidate material inventory before classifying required, reference, or irrelevant materials",
		InputPaths:     candidatePaths,
		OutputArtifact: "candidate_material_inventory.json",
		Params:         map[string]string{"limit": fmt.Sprintf("%d", len(candidatePaths))},
	}}
	out.ContinueAfter = true
	out.OutputContract = dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true}
	if strings.TrimSpace(out.Goal) == "" {
		out.Goal = "discover candidate data materials"
	}
	out.WhyThisBatch = fmt.Sprintf("complex data workflow has %d unclassified candidate material(s); first expose objective inventory before typed calculation", len(omitted))
	out.NextBatch = "classify candidate materials for the user goal, then continue with typed data workflow actions"
	out.CoverageContract.OptionalMaterials = mergeDataTaskCoverageMaterials(
		out.CoverageContract.OptionalMaterials,
		dataTaskCandidateOptionalMaterials(candidates),
		false,
	)
	return out, true
}

func dataTaskCandidatePaths(candidates []dataquery.CandidateFile) []string {
	var paths []string
	for _, candidate := range candidates {
		p := normalizeDataTaskCoveragePath(candidate.Path)
		if p != "" {
			paths = append(paths, p)
		}
	}
	return cleanDataTaskStrings(paths)
}

func dataTaskCandidateOptionalMaterials(candidates []dataquery.CandidateFile) []dataquery.CoverageMaterial {
	out := make([]dataquery.CoverageMaterial, 0, len(candidates))
	for _, candidate := range candidates {
		p := normalizeDataTaskCoveragePath(candidate.Path)
		if p == "" {
			continue
		}
		material := dataquery.CoverageMaterial{
			ID:        p,
			Path:      p,
			Purpose:   "candidate material awaiting model classification for the current data goal",
			UsageMode: dataquery.MaterialUseReferenceOnly,
		}
		if len(candidate.TextEvidencePaths) > 0 {
			material.TextEvidencePath = normalizeDataTaskCoveragePath(candidate.TextEvidencePaths[0])
		}
		out = append(out, material)
	}
	return out
}

func normalizeDataTaskExactExtractLimitsForWorkflow(plan *dataquery.TaskPlan) bool {
	if plan == nil || !dataTaskPlanRequiresCompleteRecordSets(*plan) {
		return false
	}
	changed := false
	for i := range plan.Actions {
		action := plan.Actions[i]
		if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionExtractRecords {
			continue
		}
		if dataTaskExtractActionSampleOnly(action) {
			continue
		}
		if action.Params == nil {
			action.Params = map[string]string{}
		}
		current := dataTaskExtractActionLimit(action)
		if current >= dataTaskExactExtractRecordLimit {
			continue
		}
		action.Params["limit"] = fmt.Sprintf("%d", dataTaskExactExtractRecordLimit)
		action.Params["record_completeness_required"] = "true"
		if strings.TrimSpace(action.Purpose) != "" && !strings.Contains(action.Purpose, "full record-set") {
			action.Purpose = strings.TrimSpace(action.Purpose) + " (full record-set materialization required by workflow contract)"
		}
		plan.Actions[i] = action
		changed = true
	}
	return changed
}

func dataTaskPlanRequiresCompleteRecordSets(plan dataquery.TaskPlan) bool {
	contract := plan.OutputContract.Normalize()
	if plan.CoverageContract.ContributionLedgerRequired || plan.CoverageContract.ReconcileRequired {
		return true
	}
	if plan.CoverageContract.DecisionRecordsRequired && dataTaskOutputContractNeedsFinalProjection(contract) {
		return true
	}
	return false
}

func dataTaskExtractActionSampleOnly(action dataquery.DataAction) bool {
	for _, key := range []string{"sample_only", "schema_only", "preview_only"} {
		if dataTaskStringParamBool(action.Params, key) {
			return true
		}
	}
	return false
}

func dataTaskExtractActionLimit(action dataquery.DataAction) int {
	for _, key := range []string{"limit", "max_records"} {
		raw := strings.TrimSpace(action.Params[key])
		if raw == "" {
			continue
		}
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func dataTaskStringParamBool(params map[string]string, key string) bool {
	if params == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(params[key])) {
	case "true", "1", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func dataTaskNarrowSingleRecordSetActionInputsForExecution(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataquery.TaskPlan {
	if len(records) == 0 || len(plan.Actions) == 0 {
		return plan
	}
	access, _, _ := dataTaskWorkflowArtifactAvailability(records, 512)
	if len(access) == 0 {
		return plan
	}
	out := plan
	for i, action := range out.Actions {
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		switch kind {
		case dataquery.DataActionDeriveFields, dataquery.DataActionExtractFields, dataquery.DataActionGroupRecords, dataquery.DataActionExpandRecords, dataquery.DataActionFilterRecords, dataquery.DataActionQualifyRecords:
		default:
			continue
		}
		inputs := cleanDataTaskStrings(action.InputPaths)
		if len(inputs) <= 1 {
			continue
		}
		fieldRefs := dataTaskSingleRecordSetActionFieldRefs(kind, action)
		if len(fieldRefs) == 0 {
			continue
		}
		var matches []string
		for _, input := range inputs {
			if _, ok := dataTaskArtifactAccessByAlias(access, input); !ok {
				continue
			}
			if len(dataTaskMissingFieldsOnArtifact(access, input, fieldRefs)) == 0 {
				matches = append(matches, input)
			}
		}
		if len(matches) != 1 {
			continue
		}
		action.InputPaths = []string{matches[0]}
		if strings.TrimSpace(action.Purpose) != "" {
			action.Purpose = strings.TrimSpace(action.Purpose) + " (input narrowed by field contract)"
		}
		out.Actions[i] = action
	}
	return out
}

func dataTaskSingleRecordSetActionFieldRefs(kind dataquery.DataActionKind, action dataquery.DataAction) []string {
	switch kind {
	case dataquery.DataActionDeriveFields, dataquery.DataActionExtractFields:
		return dataTaskDeriveActionSourceFieldRefs(action)
	case dataquery.DataActionGroupRecords:
		return dataTaskGroupRecordsActionFieldRefs(action)
	case dataquery.DataActionExpandRecords:
		return parseDataTaskActionStringListParam(firstNonEmptyString(
			action.Params["source_field"],
			action.Params["input_field"],
			action.Params["field"],
			action.Params["value_field"],
		))
	case dataquery.DataActionFilterRecords:
		return dataTaskFilterActionFieldRefs(action)
	case dataquery.DataActionQualifyRecords:
		return dataTaskQualifyActionFieldRefs(action)
	default:
		return nil
	}
}

func dataTaskDeriveActionSourceFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	for _, spec := range dataTaskActionObjectListParam(action.Params, "field_specs_json", "extract_specs_json", "derive_specs_json", "transforms_json", "field_specs", "extract_specs", "derive_specs", "transforms") {
		op := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
			dataTaskMapStringValue(spec, "operation"),
			dataTaskMapStringValue(spec, "op"),
			dataTaskMapStringValue(spec, "transform"),
		)))
		sources := dataTaskFieldRefsFromSpec(spec, "source_field", "input_field", "field", "value_field")
		sources = append(sources, dataTaskFieldRefsFromSpec(spec, "source_fields", "input_fields", "fields")...)
		if len(sources) == 0 && op != "constant" {
			sources = append(sources, dataTaskFieldRefsFromSpec(spec, "left_field", "right_field")...)
		}
		fields = append(fields, sources...)
	}
	fields = append(fields, parseDataTaskActionStringListParam(firstNonEmptyString(
		action.Params["source_field"],
		action.Params["input_field"],
		action.Params["field"],
		action.Params["value_field"],
	))...)
	return cleanDataTaskStrings(fields)
}

func dataTaskGroupRecordsActionFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	fields = append(fields, parseDataTaskActionStringListParam(firstNonEmptyString(
		action.Params["group_fields"],
		action.Params["group_by_fields"],
		action.Params["key_fields"],
		action.Params["group_field"],
		action.Params["group_by"],
		action.Params["key_field"],
	))...)
	fields = append(fields, parseDataTaskActionStringListParam(firstNonEmptyString(
		action.Params["text_fields"],
		action.Params["concat_fields"],
		action.Params["aggregate_fields"],
		action.Params["source_fields"],
		action.Params["text_field"],
		action.Params["source_field"],
		action.Params["field"],
	))...)
	fields = append(fields, parseDataTaskActionStringListParam(firstNonEmptyString(
		action.Params["first_fields"],
		action.Params["copy_fields"],
		action.Params["preserve_fields"],
		action.Params["keep_fields"],
	))...)
	return cleanDataTaskStrings(fields)
}

func dataTaskScopePlanToCurrentBatch(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataquery.TaskPlan {
	out := plan
	workflowContract := dataTaskWorkflowCoverageContract(records, plan)
	out.CoverageContract = dataTaskExecutionCoverageContract(records, plan, workflowContract)
	if len(plan.Actions) == 0 {
		return out
	}
	inputs := dataTaskCurrentBatchInputPaths(plan)
	if len(inputs) > 0 {
		out.InputPaths = inputs
	}
	return out
}

func dataTaskUserMentionedCandidateMaterials(userLine string, candidates []dataquery.CandidateFile) []dataquery.CoverageMaterial {
	request := normalizeDataTaskUserMaterialMatchText(userLine)
	if request == "" || len(candidates) == 0 {
		return nil
	}
	var out []dataquery.CoverageMaterial
	seen := map[string]bool{}
	for _, candidate := range candidates {
		path := normalizeDataTaskCoveragePath(candidate.Path)
		if path == "" || !dataTaskCandidatePathMentioned(request, path) {
			continue
		}
		material := dataquery.CoverageMaterial{
			ID:        dataTaskCoverageIDFromPath(path),
			Path:      path,
			Purpose:   dataTaskUserExplicitMaterialPurpose,
			Required:  true,
			UsageMode: dataTaskUsageModeForCandidate(candidate),
		}
		if material.UsageMode == dataquery.MaterialUseTextEvidenceConsumed && len(candidate.TextEvidencePaths) > 0 {
			material.TextEvidencePath = normalizeDataTaskCoveragePath(candidate.TextEvidencePaths[0])
		}
		key := dataTaskCoverageMaterialKey(material)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, material)
	}
	return out
}

func normalizeDataTaskUserMaterialMatchText(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.ToLower(value)
	return value
}

func dataTaskCandidatePathMentioned(request, candidatePath string) bool {
	candidatePath = normalizeDataTaskUserMaterialMatchText(candidatePath)
	if candidatePath == "" {
		return false
	}
	if strings.Contains(request, candidatePath) {
		return true
	}
	base := path.Base(candidatePath)
	return strings.Contains(base, ".") && strings.Contains(request, base)
}

func dataTaskCoverageIDFromPath(value string) string {
	value = normalizeDataTaskCoveragePath(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "_") {
				b.WriteByte('_')
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func dataTaskUsageModeForCandidate(candidate dataquery.CandidateFile) dataquery.CoverageMaterialUseMode {
	switch strings.TrimSpace(candidate.Kind) {
	case "csv", "tsv", "json", "jsonl", "text", "generated_json":
		return dataquery.MaterialUseScriptConsumed
	default:
		if len(candidate.TextEvidencePaths) > 0 {
			return dataquery.MaterialUseTextEvidenceConsumed
		}
		return dataquery.MaterialUseScriptConsumed
	}
}

func dataTaskFilterCoverageMaterials(materials []dataquery.CoverageMaterial, keep func(dataquery.CoverageMaterial) bool) []dataquery.CoverageMaterial {
	if len(materials) == 0 {
		return nil
	}
	out := make([]dataquery.CoverageMaterial, 0, len(materials))
	for _, material := range materials {
		if keep == nil || keep(material) {
			out = append(out, material)
		}
	}
	return out
}

func mergeDataTaskValidationRules(previous, next []string) []string {
	return cleanDataTaskStrings(append(append([]string(nil), previous...), next...))
}

func dataTaskCoverageExpansionMissingPaths(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) []string {
	seen := map[string]bool{}
	var missing []string
	add := func(values []string) {
		for _, p := range cleanDataTaskStrings(values) {
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			missing = append(missing, p)
		}
	}
	requiredContract := dataTaskWorkflowCoverageContract(records, plan)
	if strings.TrimSpace(dataTaskTerminalRequiredMaterialSchedulingError(records, plan)) != "" {
		requiredContract = plan.CoverageContract
	}
	required := cleanDataTaskStrings(requiredContract.RequiredRunnerInputPaths())
	if len(required) > 0 {
		covered := dataTaskWorkflowCoveredMaterialPaths(records, dataquery.TaskPlan{}, 0)
		for _, p := range required {
			if !dataTaskCoveragePathCovered(covered, p) {
				add([]string{p})
			}
		}
	}
	for i, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		if !dataTaskActionHasBroadPrerequisiteSurface(plan, action) {
			continue
		}
		add(dataTaskMissingCustomTransformPrerequisites(records, plan, action, i))
	}
	sort.Strings(missing)
	return missing
}

func dataTaskWorkflowHasActionKind(records []dataTaskWorkflowRecord, kind dataquery.DataActionKind) bool {
	for _, rec := range records {
		for _, action := range rec.Plan.Actions {
			if normalizeDataActionKindForWorkflow(action.Kind) == kind {
				return true
			}
		}
	}
	return false
}

func dataTaskPlanHasActionKind(plan dataquery.TaskPlan, kind dataquery.DataActionKind) bool {
	for _, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) == kind {
			return true
		}
	}
	return false
}

func dataTaskDiscoveryPaths(plan dataquery.TaskPlan) []string {
	var paths []string
	paths = append(paths, plan.InputPaths...)
	paths = append(paths, plan.CoverageContract.RequiredRunnerInputPaths()...)
	paths = append(paths, plan.CoverageContract.RequiredPaths()...)
	for _, action := range plan.Actions {
		paths = append(paths, action.InputPaths...)
	}
	return cleanDataTaskStrings(paths)
}

func dataTaskPlanHasCustomTransform(plan dataquery.TaskPlan) bool {
	for _, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionCustomTransform {
			return true
		}
	}
	return false
}

func dataTaskActionStagingGuardError(plan dataquery.TaskPlan) string {
	topLevelLines := dataTaskScriptLineCount(plan.Script)
	if topLevelLines > 0 {
		return fmt.Sprintf("data planning incomplete: actions[] plans must not carry a top-level script (script_lines=%d). Put each bounded transform script on its custom_transform action, or split the workflow into typed atomic actions; top-level script is only for simple non-actions plans.",
			topLevelLines)
	}
	if len(plan.Actions) > dataTaskMaxActionsPerBatch {
		return fmt.Sprintf("data planning incomplete: actions[] batch contains %d action(s), above the atomic batch limit %d. Emit only the next small DAG batch and set continue_after=true when more data workflow work remains.",
			len(plan.Actions), dataTaskMaxActionsPerBatch)
	}
	if errText := dataTaskTextConstraintCoverageGuardError(plan); errText != "" {
		return errText
	}
	if errText := dataTaskTerminalRequiredMaterialSchedulingError(nil, plan); errText != "" {
		return errText
	}
	if count := dataTaskCustomScriptActionCount(plan.Actions); count > 1 {
		return fmt.Sprintf("data planning incomplete: actions[] batch contains %d custom_transform scripts. A batch may have at most one bounded custom_transform; split independent transforms into separate batches or use typed actions that produce reusable artifacts.",
			count)
	}
	for i, action := range plan.Actions {
		if errText := dataTaskActionDependencyGuardError(nil, plan, action, i); errText != "" {
			return errText
		}
		lines := dataTaskScriptLineCount(action.Script)
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		if kind == dataquery.DataActionCustomTransform && lines == 0 {
			return fmt.Sprintf("data planning incomplete: action %d (%s) is custom_transform but has no script. Emit a typed action such as inspect_material, extract_records, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, or provide one bounded custom_transform script that calls emit(...), emit_result(...), or assigns result.",
				i+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))))
		}
		if lines == 0 {
			continue
		}
		if kind != dataquery.DataActionCustomTransform {
			return fmt.Sprintf("data planning incomplete: action %d (%s) is a typed data action but carries a script (script_lines=%d). Only custom_transform may carry a script. For %s, remove script and express the operation with input_paths, output_artifact, and params; if a script is truly required, use custom_transform only when the workflow allows it.",
				i+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))), lines, kind)
		}
		if kind == dataquery.DataActionCustomTransform && !dataTaskScriptHasResultEmitter(action.Script) {
			return fmt.Sprintf("data planning incomplete: action %d (%s) script has no result emitter (script_lines=%d). A custom_transform must call emit(...), emit_result(...), or assign result.",
				i+1, strings.TrimSpace(string(action.Kind)), lines)
		}
		if kind == dataquery.DataActionCustomTransform &&
			lines >= dataTaskComplexCustomScriptLineLimit &&
			dataTaskActionLooksLikeWholeWorkflow(plan, action, i) {
			return fmt.Sprintf("data planning incomplete: action %d (%s) is too broad for one bounded custom_transform (script_lines=%d input_paths=%d required_materials=%d validation_ledgers=%d). Split it into smaller typed actions such as inspect_material, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, and reserve custom_transform for one narrow transform.",
				i+1, strings.TrimSpace(string(action.Kind)), lines, len(action.InputPaths), len(plan.CoverageContract.RequiredMaterials), dataTaskValidationLedgerCount(plan.CoverageContract))
		}
		if lines >= dataTaskOneShotScriptLineSoftLimit {
			return fmt.Sprintf("data planning incomplete: action %d (%s) is too large for one atomic data action (script_lines=%d). Split the workflow into smaller typed actions such as material_inventory, inspect_material, and bounded custom_transform nodes.",
				i+1, strings.TrimSpace(string(action.Kind)), lines)
		}
		if kind == dataquery.DataActionCustomTransform &&
			dataTaskActionHasBroadPrerequisiteSurface(plan, action) &&
			dataTaskValidationLedgerCount(plan.CoverageContract) >= 3 {
			return fmt.Sprintf("data planning incomplete: action %d (%s) is too broad for one custom_transform data DAG node (input_paths=%d required_materials=%d validation_ledgers=%d). Split the work into typed atomic actions such as inspect_material, derive_rules, derive_fields, filter_records, qualify_records, normalize_entities, enrich_records, join_records, compute_contributions, and reconcile_artifacts. Reserve custom_transform for one narrow transform over known generated artifacts.",
				i+1, strings.TrimSpace(string(action.Kind)), len(action.InputPaths), len(plan.CoverageContract.RequiredMaterials), dataTaskValidationLedgerCount(plan.CoverageContract))
		}
	}
	return ""
}

func dataTaskWorkflowActionStagingGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	topLevelLines := dataTaskScriptLineCount(plan.Script)
	if topLevelLines > 0 {
		return fmt.Sprintf("data planning incomplete: actions[] plans must not carry a top-level script (script_lines=%d). Put each bounded transform script on its custom_transform action, or split the workflow into typed atomic actions; top-level script is only for simple non-actions plans.",
			topLevelLines)
	}
	if len(plan.Actions) > dataTaskMaxActionsPerBatch {
		return fmt.Sprintf("data planning incomplete: actions[] batch contains %d action(s), above the atomic batch limit %d. Emit only the next small DAG batch and set continue_after=true when more data workflow work remains.",
			len(plan.Actions), dataTaskMaxActionsPerBatch)
	}
	if errText := dataTaskTextConstraintCoverageGuardError(plan); errText != "" {
		return errText
	}
	if errText := dataTaskTerminalRequiredMaterialSchedulingError(records, plan); errText != "" {
		return errText
	}
	if errText := dataTaskWorkflowCustomTransformDisabledGuardError(records, plan); errText != "" {
		return errText
	}
	if count := dataTaskCustomScriptActionCount(plan.Actions); count > 1 {
		return fmt.Sprintf("data planning incomplete: actions[] batch contains %d custom_transform scripts. A batch may have at most one bounded custom_transform; split independent transforms into separate batches or use typed actions that produce reusable artifacts.",
			count)
	}
	if errText := dataTaskCoverageLoopGuardError(records, plan); errText != "" {
		return errText
	}
	if errText := dataTaskWorkflowAllowedNextActionGuardError(records, plan); errText != "" {
		return errText
	}
	if errText := dataTaskWorkflowStageProgressGuardError(records, plan); errText != "" {
		return errText
	}
	if errText := dataTaskWorkflowNumericConstantReuseGuardError(plan); errText != "" {
		return errText
	}
	for i, action := range plan.Actions {
		if errText := dataTaskActionDependencyGuardError(records, plan, action, i); errText != "" {
			return errText
		}
		if errText := dataTaskRepeatedCustomTransformGuardError(records, action); errText != "" {
			return errText
		}
		if errText := dataTaskRepeatedCustomTransformClassGuardError(records, plan, action, i); errText != "" {
			return errText
		}
		lines := dataTaskScriptLineCount(action.Script)
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		if kind == dataquery.DataActionCustomTransform && lines == 0 {
			return fmt.Sprintf("data planning incomplete: action %d (%s) is custom_transform but has no script. Emit a typed action from workflow_state_json.allowed_next_actions, or provide one bounded custom_transform script that calls emit(...), emit_result(...), or assigns result.",
				i+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))))
		}
		if lines == 0 {
			continue
		}
		if kind != dataquery.DataActionCustomTransform && lines > 0 {
			return fmt.Sprintf("data planning incomplete: action %d (%s) is a typed data action but carries a script (script_lines=%d). Only custom_transform may carry a script. For %s, remove script and express the operation with input_paths, output_artifact, and params; if a script is truly required, use custom_transform only when workflow_state_json allows it.",
				i+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))), lines, kind)
		}
		if kind == dataquery.DataActionCustomTransform && !dataTaskScriptHasResultEmitter(action.Script) {
			return fmt.Sprintf("data planning incomplete: action %d (%s) script has no result emitter (script_lines=%d). A custom_transform must call emit(...), emit_result(...), or assign result.",
				i+1, strings.TrimSpace(string(action.Kind)), lines)
		}
		if errText := dataTaskTerminalRawMaterialCustomTransformGuardError(records, plan, action, i, lines); errText != "" {
			return errText
		}
		if kind == dataquery.DataActionCustomTransform &&
			dataTaskActionHasBroadPrerequisiteSurface(plan, action) {
			if missing := dataTaskMissingCustomTransformPrerequisites(records, plan, action, i); len(missing) > 0 {
				return fmt.Sprintf("data planning incomplete: broad custom_transform action %d (%s) reads or depends on %d material(s) that were not covered by prior typed actions/results: %s. First add smaller atomic actions such as inspect_material, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, or extract_records for the missing inputs, then use custom_transform only as a bounded transform over known materials.",
					i+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))), len(missing), strings.Join(missing, ", "))
			}
			if lines >= dataTaskComplexCustomScriptLineLimit && dataTaskActionLooksLikeWholeWorkflow(plan, action, i) {
				return fmt.Sprintf("data planning incomplete: action %d (%s) is too broad for one bounded custom_transform (script_lines=%d input_paths=%d required_materials=%d validation_ledgers=%d). Prior coverage of materials does not make one script a valid substitute for the remaining data DAG. Continue with typed actions such as derive_fields, expand_records, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, and reserve custom_transform for one narrow transform over known artifacts.",
					i+1, strings.TrimSpace(string(action.Kind)), lines, len(action.InputPaths), len(plan.CoverageContract.RequiredMaterials), dataTaskValidationLedgerCount(plan.CoverageContract))
			}
			if lines >= dataTaskOneShotScriptLineSoftLimit {
				return fmt.Sprintf("data planning incomplete: action %d (%s) is too large for one atomic data action (script_lines=%d). Split the workflow into smaller typed actions such as material_inventory, inspect_material, and bounded custom_transform nodes.",
					i+1, strings.TrimSpace(string(action.Kind)), lines)
			}
			rawInputs := dataTaskCustomTransformRawMaterialInputs(records, action)
			if len(rawInputs) >= 3 && dataTaskValidationLedgerCount(plan.CoverageContract) >= 3 {
				return fmt.Sprintf("data planning incomplete: action %d (%s) is too broad for one custom_transform data DAG node (raw_inputs=%d input_paths=%d required_materials=%d validation_ledgers=%d). Split the work into typed atomic actions such as inspect_material, derive_rules, derive_fields, filter_records, qualify_records, normalize_entities, enrich_records, join_records, compute_contributions, and reconcile_artifacts. Reserve custom_transform for one narrow transform over known generated artifacts.",
					i+1, strings.TrimSpace(string(action.Kind)), len(rawInputs), len(action.InputPaths), len(plan.CoverageContract.RequiredMaterials), dataTaskValidationLedgerCount(plan.CoverageContract))
			}
			continue
		}
		if kind == dataquery.DataActionCustomTransform && lines >= dataTaskComplexCustomScriptLineLimit &&
			dataTaskActionLooksLikeWholeWorkflow(plan, action, i) {
			return fmt.Sprintf("data planning incomplete: action %d (%s) is too broad for one bounded custom_transform (script_lines=%d input_paths=%d required_materials=%d validation_ledgers=%d). Split it into smaller typed actions such as inspect_material, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, and reserve custom_transform for one narrow transform.",
				i+1, strings.TrimSpace(string(action.Kind)), lines, len(action.InputPaths), len(plan.CoverageContract.RequiredMaterials), dataTaskValidationLedgerCount(plan.CoverageContract))
		}
		if lines >= dataTaskOneShotScriptLineSoftLimit {
			return fmt.Sprintf("data planning incomplete: action %d (%s) is too large for one atomic data action (script_lines=%d). Split the workflow into smaller typed actions such as material_inventory, inspect_material, and bounded custom_transform nodes.",
				i+1, strings.TrimSpace(string(action.Kind)), lines)
		}
	}
	return ""
}

type dataTaskDerivedConstantField struct {
	ActionIndex int
	ActionID    string
	Field       string
	Value       string
	Aliases     []string
}

func dataTaskWorkflowNumericConstantReuseGuardError(plan dataquery.TaskPlan) string {
	if len(plan.Actions) < 2 {
		return ""
	}
	var constants []dataTaskDerivedConstantField
	for i, action := range plan.Actions {
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		if kind != dataquery.DataActionDeriveFields && kind != dataquery.DataActionExtractFields {
			continue
		}
		for _, spec := range dataTaskActionObjectListParam(action.Params, "field_specs_json", "derive_specs_json", "extract_specs_json", "transforms_json", "field_specs", "derive_specs", "extract_specs", "transforms") {
			op := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
				dataTaskMapStringValue(spec, "operation"),
				dataTaskMapStringValue(spec, "op"),
				dataTaskMapStringValue(spec, "transform"),
			)))
			if op != "constant" {
				continue
			}
			field := strings.TrimSpace(firstNonEmptyString(
				dataTaskMapStringValue(spec, "target_field"),
				dataTaskMapStringValue(spec, "output_field"),
				dataTaskMapStringValue(spec, "derived_field"),
				dataTaskMapStringValue(spec, "name"),
			))
			value := strings.TrimSpace(dataTaskMapStringValue(spec, "value"))
			if field == "" || dataTaskStringLooksNumeric(value) {
				continue
			}
			constants = append(constants, dataTaskDerivedConstantField{
				ActionIndex: i,
				ActionID:    firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(kind))),
				Field:       field,
				Value:       value,
				Aliases:     dataTaskActionOutputAliases(action),
			})
		}
	}
	if len(constants) == 0 {
		return ""
	}
	for j, action := range plan.Actions {
		uses := dataTaskNumericFieldUsesFromAction(action)
		if len(uses) == 0 {
			continue
		}
		inputs := cleanDataTaskStrings(action.InputPaths)
		for _, constant := range constants {
			if constant.ActionIndex >= j || !dataTaskActionInputsMayConsumeAliases(inputs, constant.Aliases) {
				continue
			}
			if !dataTaskStringSliceContainsFold(uses, constant.Field) {
				continue
			}
			return fmt.Sprintf("data planning incomplete: action %d (%s) uses field %q as numeric filter/value, but action %d (%s) materializes it as non-numeric constant %q. Keep constant fields for fixed labels/flags only; first materialize a numeric field with derive_fields operation=parse_number from an existing numeric source field, then use that numeric field for gt/gte/lt/lte or compute_contributions.value_field.",
				j+1,
				firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(normalizeDataActionKindForWorkflow(action.Kind)))),
				constant.Field,
				constant.ActionIndex+1,
				constant.ActionID,
				constant.Value)
		}
	}
	return ""
}

func dataTaskNumericFieldUsesFromAction(action dataquery.DataAction) []string {
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	var fields []string
	switch kind {
	case dataquery.DataActionFilterRecords, dataquery.DataActionQualifyRecords:
		fields = append(fields, dataTaskNumericFilterFieldRefs(action)...)
	case dataquery.DataActionComputeContribs:
		op := strings.ToLower(strings.TrimSpace(firstNonEmptyString(action.Params["operation"], action.Params["op"])))
		valueField := strings.TrimSpace(action.Params["value_field"])
		if valueField != "" && op != "count" {
			fields = append(fields, valueField)
		}
		fields = append(fields, dataTaskNumericFilterFieldRefs(action)...)
	}
	return cleanDataTaskStrings(fields)
}

func dataTaskNumericFilterFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	for _, spec := range dataTaskActionObjectListParam(action.Params, "filters_json", "filters") {
		op := strings.ToLower(strings.TrimSpace(dataTaskMapStringValue(spec, "op")))
		if op == "" {
			op = strings.ToLower(strings.TrimSpace(dataTaskMapStringValue(spec, "operation")))
		}
		if !dataTaskFilterOpRequiresNumeric(op) {
			continue
		}
		fields = append(fields, dataTaskFieldRefsFromSpec(spec, "field", "source_field", "input_field")...)
	}
	op := strings.ToLower(strings.TrimSpace(firstNonEmptyString(action.Params["filter_op"], action.Params["op"])))
	if dataTaskFilterOpRequiresNumeric(op) {
		fields = append(fields, parseDataTaskActionStringListParam(action.Params["filter_field"])...)
	}
	return cleanDataTaskStrings(fields)
}

func dataTaskFilterOpRequiresNumeric(op string) bool {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "gt", "gte", "lt", "lte":
		return true
	default:
		return false
	}
}

func dataTaskActionInputsMayConsumeAliases(inputs, aliases []string) bool {
	inputs = cleanDataTaskStrings(inputs)
	aliases = cleanDataTaskStrings(aliases)
	if len(inputs) == 0 || len(aliases) == 0 {
		return false
	}
	aliasSet := map[string]bool{}
	for _, alias := range aliases {
		key := normalizeDataTaskCoveragePath(alias)
		if key != "" {
			aliasSet[key] = true
		}
	}
	for _, input := range inputs {
		if aliasSet[normalizeDataTaskCoveragePath(input)] {
			return true
		}
	}
	return false
}

func dataTaskStringSliceContainsFold(values []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), needle) {
			return true
		}
	}
	return false
}

func dataTaskStringLooksNumeric(value string) bool {
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", ""))
	if value == "" {
		return false
	}
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

func dataTaskWorkflowCustomTransformDisabledGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	if len(records) == 0 || len(plan.Actions) == 0 || !dataTaskWorkflowHasSuccessfulResult(records) {
		return ""
	}
	state := dataTaskWorkflowState(records, plan)
	if !state.CustomTransformDisabled {
		return ""
	}
	for i, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		return fmt.Sprintf("data planning incomplete: workflow custom_transform_disabled=true at next_stage=%s; action %d (%s) uses custom_transform. Use workflow_state_json.allowed_next_actions [%s] and emit typed actions that consume existing source materials or generated artifact aliases; free-form scripts are disabled for this workflow stage.",
			state.NextStage,
			i+1,
			firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
			strings.Join(state.AllowedNextActions, ", "))
	}
	return ""
}

func dataTaskCustomTransformDisabledFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	if len(records) == 0 || strings.TrimSpace(errText) == "" {
		return dataquery.TaskPlan{}, false
	}
	state := dataTaskWorkflowState(records, plan)
	if !state.CustomTransformDisabled {
		return dataquery.TaskPlan{}, false
	}
	hasCustom := false
	for _, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionCustomTransform {
			hasCustom = true
			break
		}
	}
	if !hasCustom {
		return dataquery.TaskPlan{}, false
	}
	if fallback, _, ok := dataTaskWorkflowNextStageFallback(records, plan, "custom_transform disabled repair rejected"); ok {
		return fallback, true
	}
	if fallback, _, ok := dataTaskConcreteScaffoldFallback(records, plan, "custom_transform disabled repair rejected"); ok {
		return fallback, true
	}
	inputs := dataTaskGeneratedDiagnosticInputsForPlan(records, plan, 3)
	if len(inputs) == 0 {
		return dataquery.TaskPlan{}, false
	}
	out := plan
	out.Status = "ready"
	out.Script = ""
	out.Actions = []dataquery.DataAction{{
		ID:             "inspect_generated_artifact_schema",
		Kind:           dataquery.DataActionInspectMaterial,
		Purpose:        "inspect generated artifact schema after a script-disabled repair attempted to use custom_transform",
		InputPaths:     inputs,
		OutputArtifact: "generated_artifact_schema_diagnostics.json",
	}}
	out.InputPaths = inputs
	out.OutputContract = dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true}
	out.CoverageContract = dataTaskWorkflowCoverageContract(records, plan)
	out.ContinueAfter = true
	out.WhyThisBatch = "custom_transform is disabled for the current workflow stage; inspect generated artifact schemas before the next typed repair"
	out.NextBatch = "continue with allowed typed data actions using the inspected artifact fields"
	if strings.TrimSpace(out.Goal) == "" {
		out.Goal = "inspect generated data artifacts for the next typed workflow action"
	}
	if dataTaskFallbackPlanAlreadySeen(records, out) {
		return dataquery.TaskPlan{}, false
	}
	return out, true
}

func dataTaskConcreteScaffoldFallback(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, reasonPrefix string) (dataquery.TaskPlan, string, bool) {
	state := dataTaskWorkflowState(records, current)
	if len(state.ActionScaffold) == 0 || len(state.AllowedNextActions) == 0 {
		return dataquery.TaskPlan{}, "", false
	}
	contract := dataTaskWorkflowCoverageContract(records, current)
	base := dataquery.TaskPlan{
		Status:           "ready",
		OutputContract:   dataTaskWorkflowOutputContract(records, current),
		CoverageContract: contract,
		Goal:             strings.TrimSpace(current.Goal),
		SuccessCriteria:  append([]string(nil), current.SuccessCriteria...),
		ContinueAfter:    true,
	}
	if strings.TrimSpace(base.Goal) == "" {
		base.Goal = "continue the typed data workflow toward the user's requested output"
	}
	for _, scaffold := range dataTaskConcreteScaffoldCandidatesForState(state) {
		action, ok := dataTaskConcreteActionFromScaffold(scaffold)
		if !ok {
			continue
		}
		base.Actions = []dataquery.DataAction{action}
		base.InputPaths = mergeDataTaskInputPaths(base.InputPaths, action.InputPaths)
		base.WhyThisBatch = reasonPrefix + "; execute the next concrete typed action scaffold instead of another script or schema-only inspection"
		base.NextBatch = "evaluate this materialized typed artifact, then continue with the next legal DAG stage"
		if strings.TrimSpace(action.Purpose) != "" {
			base.WhyThisBatch = reasonPrefix + "; " + strings.TrimSpace(action.Purpose)
		}
		if dataTaskFallbackPlanAlreadySeen(records, base) {
			continue
		}
		return base, reasonPrefix + "; converted to concrete typed action scaffold", true
	}
	return dataquery.TaskPlan{}, "", false
}

func dataTaskConcreteScaffoldCandidatesForState(state dataTaskWorkflowStateView) []dataTaskActionScaffold {
	if len(state.ActionScaffold) == 0 {
		return nil
	}
	if !state.EntityStageMaterialized || (state.NextStage != "prepare_contribution_inputs" && state.NextStage != "compute_contributions") {
		return append([]dataTaskActionScaffold(nil), state.ActionScaffold...)
	}
	priority := map[string]int{
		string(dataquery.DataActionApplyResolutions): 1,
		string(dataquery.DataActionEnrichRecords):    2,
		string(dataquery.DataActionJoinRecords):      3,
		string(dataquery.DataActionFilterRecords):    4,
		string(dataquery.DataActionQualifyRecords):   5,
		string(dataquery.DataActionComputeContribs):  6,
	}
	out := append([]dataTaskActionScaffold(nil), state.ActionScaffold...)
	sort.SliceStable(out, func(i, j int) bool {
		left := priority[normalizeDataTaskActionKindString(out[i].Kind)]
		right := priority[normalizeDataTaskActionKindString(out[j].Kind)]
		if left == 0 {
			left = 100
		}
		if right == 0 {
			right = 100
		}
		return left < right
	})
	return out
}

func normalizeDataTaskActionKindString(kind string) string {
	return string(normalizeDataActionKindForWorkflow(dataquery.DataActionKind(kind)))
}

func dataTaskConcreteActionFromScaffold(scaffold dataTaskActionScaffold) (dataquery.DataAction, bool) {
	kind := normalizeDataActionKindForWorkflow(dataquery.DataActionKind(scaffold.Kind))
	params := concreteScaffoldParams(scaffold.ParamsTemplate)
	switch kind {
	case dataquery.DataActionNormalizeEntities:
		if len(scaffold.InputPaths) < 2 || !dataTaskScaffoldParamsConcrete(params, "source_field", "reference_name_fields", "canonical_id_field") {
			return dataquery.DataAction{}, false
		}
		if strings.TrimSpace(params["match_mode"]) == "" || strings.Contains(params["match_mode"], "|") {
			params["match_mode"] = "exact"
		}
		return dataquery.DataAction{
			ID:             dataTaskConcreteScaffoldActionID("continue_normalize_entities", scaffold.InputPaths),
			Kind:           dataquery.DataActionNormalizeEntities,
			Purpose:        "materialize source-to-reference mappings from concrete artifact fields",
			InputPaths:     append([]string(nil), scaffold.InputPaths[:2]...),
			OutputArtifact: dataTaskConcreteScaffoldArtifactID("entity_mappings", scaffold.InputPaths),
			Params:         params,
		}, true
	case dataquery.DataActionJoinRecords:
		if len(scaffold.InputPaths) < 2 || len(scaffold.CommonFields) == 0 {
			return dataquery.DataAction{}, false
		}
		field := strings.TrimSpace(scaffold.CommonFields[0])
		if field == "" || strings.Contains(field, "<") {
			return dataquery.DataAction{}, false
		}
		if strings.TrimSpace(params["join_type"]) == "" || strings.Contains(params["join_type"], "|") {
			params["join_type"] = "inner"
		}
		params["left_fields"] = mustCompactJSONString([]string{field})
		params["right_fields"] = mustCompactJSONString([]string{field})
		return dataquery.DataAction{
			ID:             dataTaskConcreteScaffoldActionID("continue_join_records", scaffold.InputPaths),
			Kind:           dataquery.DataActionJoinRecords,
			Purpose:        "join two concrete record artifacts on an existing common field",
			InputPaths:     append([]string(nil), scaffold.InputPaths[:2]...),
			OutputArtifact: dataTaskConcreteScaffoldArtifactID("joined_records", scaffold.InputPaths),
			Params:         params,
		}, true
	case dataquery.DataActionApplyResolutions:
		if len(scaffold.InputPaths) < 2 || !dataTaskScaffoldParamsConcrete(params, "base_path", "resolution_specs") {
			return dataquery.DataAction{}, false
		}
		return dataquery.DataAction{
			ID:             dataTaskConcreteScaffoldActionID("continue_apply_resolutions", scaffold.InputPaths),
			Kind:           dataquery.DataActionApplyResolutions,
			Purpose:        "apply concrete entity-resolution ledger fields onto base records",
			InputPaths:     append([]string(nil), scaffold.InputPaths[:2]...),
			OutputArtifact: dataTaskConcreteScaffoldArtifactID("resolved_records", scaffold.InputPaths),
			Params:         params,
		}, true
	default:
		return dataquery.DataAction{}, false
	}
}

func concreteScaffoldParams(template map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range template {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func dataTaskScaffoldParamsConcrete(params map[string]string, required ...string) bool {
	for _, key := range required {
		value := strings.TrimSpace(params[key])
		if value == "" || strings.Contains(value, "<") || strings.Contains(value, "|") {
			return false
		}
	}
	for _, value := range params {
		if strings.Contains(value, "<") {
			return false
		}
	}
	return true
}

func dataTaskConcreteScaffoldActionID(prefix string, inputs []string) string {
	return cleanDataTaskIdentifier(prefix + "_" + strings.Join(clampDataTaskStringSlice(inputs, 2), "_"))
}

func dataTaskConcreteScaffoldArtifactID(prefix string, inputs []string) string {
	id := cleanDataTaskIdentifier(prefix + "_" + strings.Join(clampDataTaskStringSlice(inputs, 2), "_"))
	if id == "" {
		return prefix + ".json"
	}
	return id + ".json"
}

func cleanDataTaskIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if len(out) > 96 {
		out = strings.TrimRight(out[:96], "_")
	}
	return out
}

func dataTaskGeneratedDiagnosticInputsForPlan(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, limit int) []string {
	if limit <= 0 {
		limit = 1
	}
	generated := dataTaskWorkflowGeneratedArtifactPathSet(records)
	if len(generated) == 0 {
		return nil
	}
	access := dataTaskWorkflowArtifactAccess(records, 64)
	accessByAlias := map[string]dataTaskArtifactAccessPrompt{}
	accessOrder := map[string]int{}
	for i, artifact := range access {
		for _, alias := range artifact.Aliases {
			key := normalizeDataTaskCoveragePath(alias)
			if key == "" {
				continue
			}
			if _, ok := accessByAlias[key]; !ok {
				accessByAlias[key] = artifact
				accessOrder[key] = i
			}
		}
		if key := normalizeDataTaskCoveragePath(artifact.ID); key != "" {
			if _, ok := accessByAlias[key]; !ok {
				accessByAlias[key] = artifact
				accessOrder[key] = i
			}
		}
	}
	type diagnosticCandidate struct {
		alias string
		key   string
		score int
		order int
	}
	candidates := map[string]diagnosticCandidate{}
	add := func(value string, fromPlan bool, order int) {
		value = strings.TrimSpace(value)
		key := normalizeDataTaskCoveragePath(value)
		if key == "" || !generated[key] {
			return
		}
		artifact, ok := accessByAlias[key]
		if !ok || !dataTaskArtifactUsableForGeneratedSchemaDiagnostic(artifact) {
			return
		}
		score := 10
		if fromPlan {
			score += 5
		}
		if artifactOrder, ok := accessOrder[key]; ok {
			order = dataTaskMinInt(order, artifactOrder)
		}
		score += dataTaskMinInt(len(artifact.Fields), 24)
		score += 45
		shape := strings.ToLower(strings.TrimSpace(artifact.JSONShape))
		if strings.Contains(shape, "array") || strings.Contains(shape, "records") || strings.Contains(shape, "object") {
			score += 8
		}
		if dataTaskArtifactKindHasPrefix(artifact.Kind,
			dataquery.DataActionApplyResolutions,
			dataquery.DataActionEnrichRecords,
			dataquery.DataActionJoinRecords,
			dataquery.DataActionDeriveFields,
			dataquery.DataActionExtractFields,
			dataquery.DataActionGroupRecords,
			dataquery.DataActionFilterRecords,
			dataquery.DataActionQualifyRecords,
			dataquery.DataActionExtractRecords,
			dataquery.DataActionExpandRecords,
			dataquery.DataActionComputeContribs,
		) {
			score += 16
		}
		next := diagnosticCandidate{alias: value, key: key, score: score, order: order}
		if prev, ok := candidates[key]; ok {
			if prev.score > next.score || (prev.score == next.score && prev.order <= next.order) {
				return
			}
		}
		candidates[key] = next
	}
	order := 0
	collect := func(values []string) {
		for _, value := range cleanDataTaskStrings(values) {
			add(value, true, order)
			order++
		}
	}
	collect(plan.InputPaths)
	for _, action := range plan.Actions {
		collect(action.InputPaths)
	}
	for i, artifact := range access {
		alias := firstDataTaskArtifactAlias(artifact)
		if alias == "" {
			continue
		}
		add(alias, false, 1000+i)
	}
	ranked := make([]diagnosticCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		ranked = append(ranked, candidate)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].order < ranked[j].order
	})
	out := make([]string, 0, dataTaskMinInt(limit, len(ranked)))
	for _, candidate := range ranked {
		out = append(out, candidate.alias)
		if len(out) >= limit {
			break
		}
	}
	return cleanDataTaskStrings(out)
}

func dataTaskArtifactUsableForGeneratedSchemaDiagnostic(artifact dataTaskArtifactAccessPrompt) bool {
	if dataTaskArtifactKindHasPrefix(artifact.Kind,
		dataquery.DataActionMaterialInventory,
		dataquery.DataActionInspectMaterial,
		dataquery.DataActionDeriveRules,
		dataquery.DataActionNormalizeEntities,
		dataquery.DataActionReconcile,
		dataquery.DataActionAssembleAnswer,
	) {
		return false
	}
	if dataTaskArtifactKindHasPrefix(artifact.Kind,
		dataquery.DataActionExtractRecords,
		dataquery.DataActionDeriveFields,
		dataquery.DataActionExtractFields,
		dataquery.DataActionGroupRecords,
		dataquery.DataActionExpandRecords,
		dataquery.DataActionFilterRecords,
		dataquery.DataActionQualifyRecords,
		dataquery.DataActionApplyResolutions,
		dataquery.DataActionEnrichRecords,
		dataquery.DataActionJoinRecords,
		dataquery.DataActionComputeContribs,
		dataquery.DataActionCustomTransform,
	) {
		return len(artifact.Fields) > 0 || dataTaskArtifactShapeLooksRecordLike(artifact)
	}
	if len(artifact.Fields) == 0 || dataTaskArtifactLooksLikeMapping(artifact) {
		return false
	}
	return dataTaskArtifactShapeLooksRecordLike(artifact)
}

func dataTaskArtifactShapeLooksRecordLike(artifact dataTaskArtifactAccessPrompt) bool {
	shape := strings.ToLower(strings.TrimSpace(artifact.JSONShape))
	return strings.Contains(shape, "array") || strings.Contains(shape, "records")
}

func dataTaskArtifactKindHasPrefix(kind string, wants ...dataquery.DataActionKind) bool {
	got := strings.ToLower(strings.TrimSpace(kind))
	for _, want := range wants {
		prefix := strings.ToLower(strings.TrimSpace(string(want)))
		if prefix == "" {
			continue
		}
		if got == prefix || strings.HasPrefix(got, prefix+"/") {
			return true
		}
	}
	return false
}

func dataTaskMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func dataTaskWorkflowAllowedNextActionGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	if len(records) == 0 || len(plan.Actions) == 0 {
		return ""
	}
	if !dataTaskWorkflowHasSuccessfulResult(records) {
		return ""
	}
	state := dataTaskWorkflowState(records, plan)
	if len(state.AllowedNextActions) == 0 {
		return ""
	}
	for i, action := range plan.Actions {
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		if dataTaskActionIsGeneratedArtifactDiagnostic(records, action) {
			continue
		}
		if dataTaskActionIsRecordMaterialization(records, action) {
			continue
		}
		if kind == dataquery.DataActionExtractRecords && state.MaterialCoverageSufficient {
			return fmt.Sprintf("data planning incomplete: workflow next_stage=%s can use extract_records only as record materialization after material coverage is sufficient. Action %d (%s) must declare output_artifact and consume explicit current-batch input paths or generated artifact aliases; do not re-run coverage-only extraction.",
				state.NextStage, i+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))))
		}
		if dataTaskWorkflowActionKindAllowed(kind, state) {
			continue
		}
		return fmt.Sprintf("data planning incomplete: workflow next_stage=%s allows only next actions [%s], but action %d (%s) uses %s. Emit the next bounded DAG batch using workflow_state_json.allowed_next_actions, set continue_after=true when later stages remain, and let the evaluator advance the workflow after this batch.",
			state.NextStage, strings.Join(state.AllowedNextActions, ", "), i+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))), kind)
	}
	return ""
}

func dataTaskActionIsRecordMaterialization(records []dataTaskWorkflowRecord, action dataquery.DataAction) bool {
	if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionExtractRecords {
		return false
	}
	if strings.TrimSpace(action.Script) != "" || strings.TrimSpace(action.OutputArtifact) == "" {
		return false
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	return len(inputs) > 0
}

func dataTaskWorkflowHasSuccessfulResult(records []dataTaskWorkflowRecord) bool {
	for _, rec := range records {
		if rec.Result != nil && strings.TrimSpace(rec.Err) == "" {
			return true
		}
	}
	return false
}

func dataTaskPlanActionsAllAllowedForWorkflowStage(plan dataquery.TaskPlan, state dataTaskWorkflowStateView) bool {
	if len(plan.Actions) == 0 || len(state.AllowedNextActions) == 0 {
		return false
	}
	for _, action := range plan.Actions {
		if !dataTaskWorkflowActionKindAllowed(normalizeDataActionKindForWorkflow(action.Kind), state) {
			return false
		}
	}
	return true
}

func dataTaskActionIsGeneratedArtifactDiagnostic(records []dataTaskWorkflowRecord, action dataquery.DataAction) bool {
	if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionInspectMaterial {
		return false
	}
	if strings.TrimSpace(action.Script) != "" {
		return false
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if len(inputs) == 0 {
		return false
	}
	generated := dataTaskWorkflowGeneratedArtifactPathSet(records)
	if len(generated) == 0 {
		return false
	}
	for _, input := range inputs {
		if !generated[normalizeDataTaskCoveragePath(input)] {
			return false
		}
	}
	return true
}

func dataTaskWorkflowActionKindAllowed(kind dataquery.DataActionKind, state dataTaskWorkflowStateView) bool {
	for _, action := range state.AllowedNextActions {
		if kind == dataquery.DataActionKind(action) {
			return true
		}
	}
	return false
}

func dataTaskWorkflowStageProgressGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if !state.MaterialCoverageSufficient {
		return ""
	}
	switch state.NextStage {
	case "normalize_or_enrich_entities", "prepare_contribution_inputs", "compute_contributions", "reconcile_artifacts", "emit_output_contract_answer":
	default:
		return ""
	}
	missingStages := dataTaskWorkflowMissingValidationStages(state)
	if len(missingStages) <= 1 || !dataTaskPlanHasScriptedCustomTransform(plan) {
		if len(missingStages) > 1 && dataTaskPlanCrossesTypedActionRanks(plan) {
			return fmt.Sprintf("data planning incomplete: workflow next_stage=%s action batch crosses multiple dependent DAG ranks while %d validation stage(s) remain: %s. Execute only the first typed action rank now, let it materialize artifacts and field contracts, then plan the next rank from real results.",
				state.NextStage, len(missingStages), strings.Join(missingStages, ", "))
		}
		return ""
	}
	if dataTaskPlanIsNarrowSingleIntermediateCustomTransform(plan, state) {
		return ""
	}
	return fmt.Sprintf("data planning incomplete: workflow next_stage=%s still has %d unfinished validation stage(s): %s. Do not cross multiple unfinished data DAG stages with one custom_transform. Emit the next atomic stage first (for example derive_fields/normalize_entities/enrich_records/join_records before compute_contributions, compute_contributions before reconcile_artifacts, reconcile_artifacts before the final answer), set continue_after=true, and let the workflow evaluate the reusable artifact before the next batch.",
		state.NextStage, len(missingStages), strings.Join(missingStages, ", "))
}

func dataTaskWorkflowStagePrefixFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	prefix, _, ok := dataTaskWorkflowStagePrefixFallbackWithRemainder(records, plan, errText)
	return prefix, ok
}

func dataTaskWorkflowStagePrefixFallbackWithRemainder(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, dataquery.TaskPlan, bool) {
	if len(records) == 0 || len(plan.Actions) < 2 || strings.TrimSpace(errText) == "" {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	if !dataTaskWorkflowHasSuccessfulResult(records) {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	state := dataTaskWorkflowState(records, plan)
	if len(state.AllowedNextActions) == 0 || len(dataTaskWorkflowMissingValidationStages(state)) <= 1 {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	keep, rest := dataTaskSplitActionRankForState(plan.Actions, state)
	if len(keep) == 0 || len(keep) == len(plan.Actions) {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	out := plan
	out.Actions = keep
	out.Script = ""
	out.ContinueAfter = true
	if strings.TrimSpace(out.WhyThisBatch) == "" {
		out.WhyThisBatch = "execute the typed action prefix for the current data workflow stage"
	}
	if strings.TrimSpace(out.NextBatch) == "" {
		out.NextBatch = "continue with the remaining data workflow stages after this prefix produces reusable artifacts"
	}
	remainder := plan
	remainder.Actions = rest
	remainder.Script = ""
	remainder.ContinueAfter = true
	return out, remainder, true
}

func dataTaskExecutablePrefixFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	if len(plan.Actions) < 2 || strings.TrimSpace(errText) == "" {
		return dataquery.TaskPlan{}, false
	}
	if strings.TrimSpace(dataTaskWorkflowStagingGuardError(records, plan)) == "" {
		return dataquery.TaskPlan{}, false
	}
	for prefixLen := len(plan.Actions) - 1; prefixLen >= 1; prefixLen-- {
		out := plan
		out.Actions = append([]dataquery.DataAction(nil), plan.Actions[:prefixLen]...)
		out.Script = ""
		out.ContinueAfter = true
		if strings.TrimSpace(out.WhyThisBatch) == "" {
			out.WhyThisBatch = "execute the valid typed data action prefix before repairing the rejected suffix"
		}
		if strings.TrimSpace(out.NextBatch) == "" {
			out.NextBatch = "continue from the prefix result and replan the invalid suffix against real artifact fields"
		}
		if strings.TrimSpace(dataTaskWorkflowStagingGuardError(records, out)) == "" {
			return out, true
		}
	}
	return dataquery.TaskPlan{}, false
}

func dataTaskPopDeferredActionBatch(records []dataTaskWorkflowRecord, deferred dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, bool) {
	if len(deferred.Actions) == 0 {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if len(state.AllowedNextActions) == 0 {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	keep, rest := dataTaskSelectDeferredReadyRankForState(records, deferred.Actions, state)
	if len(keep) == 0 {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	if errText := dataTaskDeferredDispatchGuardError(records, deferred, keep); errText != "" {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	out := deferred
	out.Actions = keep
	out.Script = ""
	out.ContinueAfter = len(rest) > 0 || deferred.ContinueAfter
	if strings.TrimSpace(out.WhyThisBatch) == "" {
		out.WhyThisBatch = "continue the deferred typed data action rank from the accepted plan graph"
	}
	if strings.TrimSpace(out.NextBatch) == "" && len(rest) > 0 {
		out.NextBatch = "continue with the remaining deferred typed data action ranks after this batch materializes artifacts"
	}
	remainder := deferred
	remainder.Actions = rest
	remainder.Script = ""
	remainder.ContinueAfter = len(rest) > 0 || deferred.ContinueAfter
	return out, remainder, true
}

func dataTaskSelectDeferredReadyRankForState(records []dataTaskWorkflowRecord, actions []dataquery.DataAction, state dataTaskWorkflowStateView) ([]dataquery.DataAction, []dataquery.DataAction) {
	if len(actions) == 0 {
		return nil, nil
	}
	for i := range actions {
		first, ok := dataTaskDeferredReadyAction(records, actions[i])
		if !ok || !dataTaskDeferredActionAllowedInState(first, state) {
			continue
		}
		firstRank := dataTaskActionDependencyRank(normalizeDataActionKindForWorkflow(first.Kind))
		selected := map[int]dataquery.DataAction{i: first}
		keep := []dataquery.DataAction{first}
		for j := i + 1; j < len(actions); j++ {
			next, ok := dataTaskDeferredReadyAction(records, actions[j])
			if !ok || !dataTaskDeferredActionAllowedInState(next, state) {
				break
			}
			nextRank := dataTaskActionDependencyRank(normalizeDataActionKindForWorkflow(next.Kind))
			if firstRank > 0 && nextRank > 0 && nextRank != firstRank {
				break
			}
			selected[j] = next
			keep = append(keep, next)
		}
		rest := make([]dataquery.DataAction, 0, len(actions)-len(keep))
		for j, action := range actions {
			if _, ok := selected[j]; ok {
				continue
			}
			rest = append(rest, action)
		}
		return keep, rest
	}
	return nil, append([]dataquery.DataAction(nil), actions...)
}

func dataTaskDeferredReadyAction(records []dataTaskWorkflowRecord, action dataquery.DataAction) (dataquery.DataAction, bool) {
	if dataTaskDeferredActionInputsReady(records, action) {
		return action, true
	}
	if rewritten, ok := dataTaskRewriteDeferredActionFirstInputToCompatibleArtifact(records, action); ok && dataTaskDeferredActionInputsReady(records, rewritten) {
		return rewritten, true
	}
	return dataquery.DataAction{}, false
}

func dataTaskDeferredActionAllowedInState(action dataquery.DataAction, state dataTaskWorkflowStateView) bool {
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	if !dataTaskWorkflowActionKindAllowed(kind, state) {
		return false
	}
	if kind == dataquery.DataActionCustomTransform && strings.TrimSpace(action.Script) != "" {
		return false
	}
	return true
}

func dataTaskDeferredDispatchGuardError(records []dataTaskWorkflowRecord, deferred dataquery.TaskPlan, actions []dataquery.DataAction) string {
	if len(actions) == 0 {
		return ""
	}
	candidate := deferred
	candidate.Status = "ready"
	candidate.Actions = append([]dataquery.DataAction(nil), actions...)
	candidate.Script = ""
	candidate.ContinueAfter = true
	return dataTaskWorkflowStagingGuardError(records, candidate)
}

type dataTaskDeferredQueueState struct {
	Actions         int
	ReadyActions    int
	BlockedActions  int
	FirstActionID   string
	FirstActionKind string
	Ready           bool
	Reason          string
}

func dataTaskDeferredQueueStatus(records []dataTaskWorkflowRecord, deferred dataquery.TaskPlan) dataTaskDeferredQueueState {
	state := dataTaskDeferredQueueState{Actions: len(deferred.Actions)}
	if len(deferred.Actions) == 0 {
		state.Reason = "deferred queue is empty"
		return state
	}
	first := deferred.Actions[0]
	state.FirstActionID = strings.TrimSpace(first.ID)
	state.FirstActionKind = strings.TrimSpace(string(normalizeDataActionKindForWorkflow(first.Kind)))
	workflow := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if len(workflow.AllowedNextActions) == 0 {
		state.Reason = "workflow has no allowed next typed actions"
		return state
	}
	keep, rest := dataTaskSelectDeferredReadyRankForState(records, deferred.Actions, workflow)
	state.ReadyActions = len(keep)
	state.BlockedActions = len(rest)
	if len(keep) == 0 {
		state.Reason = dataTaskDeferredActionBlockedReason(records, first)
		if strings.TrimSpace(state.Reason) == "" {
			state.Reason = "no deferred action is executable from current artifact aliases and field contracts"
		}
		return state
	}
	if errText := dataTaskDeferredDispatchGuardError(records, deferred, keep); errText != "" {
		state.Reason = errText
		return state
	}
	state.Ready = true
	return state
}

func dataTaskDeferredActionBlockedReason(records []dataTaskWorkflowRecord, action dataquery.DataAction) string {
	inputs := cleanDataTaskStrings(action.InputPaths)
	if len(inputs) > 0 {
		covered := dataTaskWorkflowCoveredMaterialPaths(records, dataquery.TaskPlan{}, 0)
		artifacts, _, _ := dataTaskWorkflowArtifactAvailability(records, 256)
		var missing []string
		for _, input := range inputs {
			if dataTaskCoveragePathCovered(covered, input) {
				continue
			}
			if _, ok := dataTaskArtifactAccessByAlias(artifacts, input); ok {
				continue
			}
			missing = append(missing, input)
		}
		if len(missing) > 0 {
			return fmt.Sprintf("deferred action %s:%s waits for unavailable input alias/material [%s]",
				firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
				normalizeDataActionKindForWorkflow(action.Kind),
				strings.Join(missing, ", "))
		}
	}
	if errText := dataTaskActionFieldContractGuardError(records, dataquery.TaskPlan{}, action, 0); errText != "" {
		return errText
	}
	return ""
}

func dataTaskDeferredQueueSavedSegment(lang string, deferred dataquery.TaskPlan, reason string) string {
	reason = oneLineClamp(reason, 120)
	first := ""
	if len(deferred.Actions) > 0 {
		action := deferred.Actions[0]
		first = firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))
	}
	if isZh(lang) {
		parts := []string{fmt.Sprintf("已保存延后 typed 队列 %d 个动作", len(deferred.Actions))}
		if first != "" {
			parts = append(parts, "首个 "+first)
		}
		if reason != "" {
			parts = append(parts, reason)
		}
		return strings.Join(parts, "；")
	}
	parts := []string{fmt.Sprintf("saved deferred typed queue with %d action(s)", len(deferred.Actions))}
	if first != "" {
		parts = append(parts, "first "+first)
	}
	if reason != "" {
		parts = append(parts, reason)
	}
	return strings.Join(parts, "; ")
}

func dataTaskDeferredQueueDiscardedSegment(lang string, deferred dataquery.TaskPlan, reason string) string {
	reason = oneLineClamp(reason, 220)
	first := ""
	if len(deferred.Actions) > 0 {
		action := deferred.Actions[0]
		first = firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))
	}
	if isZh(lang) {
		parts := []string{fmt.Sprintf("延后 typed 队列已丢弃 %d 个动作", len(deferred.Actions))}
		if first != "" {
			parts = append(parts, "首个 "+first)
		}
		if reason != "" {
			parts = append(parts, reason)
		}
		return strings.Join(parts, "；")
	}
	parts := []string{fmt.Sprintf("discarded deferred typed queue with %d action(s)", len(deferred.Actions))}
	if first != "" {
		parts = append(parts, "first "+first)
	}
	if reason != "" {
		parts = append(parts, reason)
	}
	return strings.Join(parts, "; ")
}

func dataTaskDeferredQueueBlockedSegment(lang string, state dataTaskDeferredQueueState) string {
	reason := oneLineClamp(state.Reason, 160)
	action := firstNonEmptyString(state.FirstActionID, state.FirstActionKind, "deferred action")
	if isZh(lang) {
		return fmt.Sprintf("延后队列未执行：首个 %s 尚未就绪；ready=%d blocked=%d；%s",
			action, state.ReadyActions, state.BlockedActions, reason)
	}
	return fmt.Sprintf("deferred queue not dispatched: first %s is not ready; ready=%d blocked=%d; %s",
		action, state.ReadyActions, state.BlockedActions, reason)
}

func dataTaskSplitDeferredActionsByReadyInputs(records []dataTaskWorkflowRecord, actions []dataquery.DataAction) ([]dataquery.DataAction, []dataquery.DataAction) {
	var ready []dataquery.DataAction
	for i, action := range actions {
		if !dataTaskDeferredActionInputsReady(records, action) {
			return ready, append([]dataquery.DataAction(nil), actions[i:]...)
		}
		ready = append(ready, action)
	}
	return ready, nil
}

func dataTaskRewriteDeferredActionFirstInputToCompatibleArtifact(records []dataTaskWorkflowRecord, action dataquery.DataAction) (dataquery.DataAction, bool) {
	input, fields, explicitSource := dataTaskActionFirstInputFieldContract(action)
	if input == "" || len(fields) == 0 {
		return dataquery.DataAction{}, false
	}
	access := dataTaskWorkflowArtifactContractAccess(records)
	if len(access) == 0 {
		return dataquery.DataAction{}, false
	}
	missing := dataTaskMissingFieldsOnArtifact(access, input, fields)
	if len(missing) == 0 {
		return dataquery.DataAction{}, false
	}
	candidate, ok := dataTaskBestArtifactAliasWithFields(access, input, missing)
	if !ok {
		return dataquery.DataAction{}, false
	}
	rewritten := action
	if explicitSource {
		if rewritten.Params == nil {
			rewritten.Params = map[string]string{}
		} else {
			next := make(map[string]string, len(rewritten.Params)+1)
			for k, v := range rewritten.Params {
				next[k] = v
			}
			rewritten.Params = next
		}
		rewritten.Params["source_path"] = candidate
		return rewritten, true
	}
	inputs := append([]string(nil), rewritten.InputPaths...)
	if len(inputs) == 0 {
		return dataquery.DataAction{}, false
	}
	inputs[0] = candidate
	rewritten.InputPaths = inputs
	return rewritten, true
}

func dataTaskActionFirstInputFieldContract(action dataquery.DataAction) (input string, fields []string, explicitSource bool) {
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	switch kind {
	case dataquery.DataActionNormalizeEntities:
		input = firstNonEmptyString(
			strings.TrimSpace(action.Params["source_path"]),
			strings.TrimSpace(action.Params["base_path"]),
			strings.TrimSpace(action.Params["record_path"]),
		)
		explicitSource = input != ""
		if input == "" {
			input = firstDataTaskActionInput(action, 0)
		}
		fields = cleanDataTaskStrings(append(
			parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["source_fields"], action.Params["source_fields_json"], action.Params["source_names"], action.Params["source_names_json"])),
			parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["source_field"], action.Params["source_name"]))...,
		))
		fields = cleanDataTaskStrings(append(fields, dataTaskFilterActionFieldRefs(action)...))
	default:
		return "", nil, false
	}
	return strings.TrimSpace(input), cleanDataTaskStrings(fields), explicitSource
}

func dataTaskBestArtifactAliasWithFields(access []dataTaskArtifactAccessPrompt, current string, fields []string) (string, bool) {
	currentKey := strings.ToLower(strings.TrimSpace(current))
	for _, artifact := range access {
		alias := firstDataTaskArtifactAlias(artifact)
		if alias == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(alias), currentKey) || strings.EqualFold(strings.TrimSpace(artifact.ID), currentKey) {
			continue
		}
		if len(dataTaskMissingFieldsOnArtifact(access, alias, fields)) == 0 {
			return alias, true
		}
	}
	return "", false
}

func dataTaskDeferredActionInputsReady(records []dataTaskWorkflowRecord, action dataquery.DataAction) bool {
	inputs := cleanDataTaskStrings(action.InputPaths)
	if len(inputs) == 0 {
		return true
	}
	covered := dataTaskWorkflowCoveredMaterialPaths(records, dataquery.TaskPlan{}, 0)
	artifacts, _, _ := dataTaskWorkflowArtifactAvailability(records, 256)
	for _, input := range inputs {
		if dataTaskCoveragePathCovered(covered, input) {
			continue
		}
		if _, ok := dataTaskArtifactAccessByAlias(artifacts, input); ok {
			continue
		}
		return false
	}
	if dataTaskActionFieldContractGuardError(records, dataquery.TaskPlan{}, action, 0) != "" {
		return false
	}
	return true
}

func dataTaskSplitActionRankForState(actions []dataquery.DataAction, state dataTaskWorkflowStateView) ([]dataquery.DataAction, []dataquery.DataAction) {
	keep := make([]dataquery.DataAction, 0, len(actions))
	firstRank := 0
	for _, action := range actions {
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		if !dataTaskWorkflowActionKindAllowed(kind, state) {
			break
		}
		if kind == dataquery.DataActionCustomTransform && strings.TrimSpace(action.Script) != "" {
			break
		}
		rank := dataTaskActionDependencyRank(kind)
		if rank > 0 {
			if firstRank == 0 {
				firstRank = rank
			} else if rank != firstRank {
				break
			}
		}
		keep = append(keep, action)
	}
	if len(keep) == 0 {
		return nil, append([]dataquery.DataAction(nil), actions...)
	}
	rest := append([]dataquery.DataAction(nil), actions[len(keep):]...)
	return keep, rest
}

func dataTaskInvalidRecordActionFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	if strings.TrimSpace(errText) == "" || len(plan.Actions) == 0 {
		return dataquery.TaskPlan{}, false
	}
	state := dataTaskWorkflowState(records, plan)
	for _, action := range plan.Actions {
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		if kind != dataquery.DataActionDeriveFields && kind != dataquery.DataActionExtractFields {
			continue
		}
		inputs := cleanDataTaskStrings(action.InputPaths)
		if len(inputs) == 0 {
			continue
		}
		if len(inputs) <= 1 && dataTaskDeriveFieldsActionHasSpec(action) {
			continue
		}
		if state.MaterialCoverageSufficient && state.NextStage != "cover_required_materials" && state.NextStage != "derive_rules" {
			if fallback, ok := dataTaskInvalidRecordActionEnrichFallback(records, plan, action, inputs); ok {
				return fallback, true
			}
			continue
		}
		out := plan
		out.Actions = []dataquery.DataAction{{
			ID:             firstNonEmptyString(strings.TrimSpace(action.ID), "extract_records") + "_records",
			Kind:           dataquery.DataActionExtractRecords,
			Purpose:        "materialize record samples for later single-record-set derivation, enrichment, join, or contribution steps",
			InputPaths:     inputs,
			OutputArtifact: firstNonEmptyString(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "records"),
			Params: map[string]string{
				"limit":       "100000",
				"max_records": "100000",
			},
		}}
		out.Script = ""
		out.Status = "ready"
		out.ContinueAfter = true
		if strings.TrimSpace(out.WhyThisBatch) == "" {
			out.WhyThisBatch = fmt.Sprintf("%s was not executable as one single-record-set action; first materialize the declared records as a bounded atomic batch", kind)
		}
		if strings.TrimSpace(out.NextBatch) == "" {
			out.NextBatch = "continue with derive_fields, extract_fields, group_records, enrich_records, join_records, or compute_contributions after record artifacts materialize"
		}
		return out, true
	}
	return dataquery.TaskPlan{}, false
}

func dataTaskInvalidRecordActionEnrichFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, inputs []string) (dataquery.TaskPlan, bool) {
	if len(inputs) < 2 {
		return dataquery.TaskPlan{}, false
	}
	targetField := firstNonEmptyString(
		dataTaskHistoricalMissingJoinFieldName(records),
		strings.TrimSpace(action.Params["target_field"]),
		strings.TrimSpace(action.Params["output_field"]),
		strings.TrimSpace(action.Params["derived_field"]),
	)
	if strings.TrimSpace(targetField) == "" {
		return dataquery.TaskPlan{}, false
	}
	basePath := firstNonEmptyString(
		strings.TrimSpace(action.Params["base_path"]),
		strings.TrimSpace(action.Params["record_path"]),
		strings.TrimSpace(action.Params["input_path"]),
		inputs[0],
	)
	sourceFields := cleanDataTaskStrings(append(
		parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["source_fields"], action.Params["base_fields"], action.Params["left_fields"])),
		parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["source_field"], action.Params["base_field"], action.Params["left_field"], action.Params["field"]))...,
	))
	access := dataTaskWorkflowArtifactAccess(records, 24)
	if len(sourceFields) == 0 {
		if base, ok := dataTaskArtifactAccessByAlias(access, basePath); ok {
			sourceFields = dataTaskSourceFieldsForMissingTarget(base.Fields, targetField, 6)
		}
	}
	if len(sourceFields) == 0 {
		return dataquery.TaskPlan{}, false
	}
	mappingPath := firstNonEmptyString(
		strings.TrimSpace(action.Params["mapping_path"]),
		strings.TrimSpace(action.Params["reference_path"]),
		strings.TrimSpace(action.Params["lookup_path"]),
	)
	if mappingPath == "" {
		for _, input := range inputs {
			if normalizeDataTaskCoveragePath(input) == normalizeDataTaskCoveragePath(basePath) {
				continue
			}
			mappingPath = input
			break
		}
	}
	if mappingPath == "" {
		return dataquery.TaskPlan{}, false
	}
	valueField := firstNonEmptyString(
		strings.TrimSpace(action.Params["mapping_value_field"]),
		strings.TrimSpace(action.Params["reference_value_field"]),
		strings.TrimSpace(action.Params["canonical_id_field"]),
		targetField,
	)
	mappingFields := cleanDataTaskStrings(append(
		parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["mapping_source_fields"], action.Params["reference_fields"], action.Params["lookup_fields"])),
		parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["mapping_source_field"], action.Params["reference_field"], action.Params["lookup_field"]))...,
	))
	if mapping, ok := dataTaskArtifactAccessByAlias(access, mappingPath); ok {
		if exact, _ := dataTaskMappingValueFieldForMissingTarget(mapping.Fields, targetField); exact != "" {
			valueField = exact
		}
		if len(mappingFields) == 0 {
			mappingFields = dataTaskReferenceSourceFieldsForMissingTarget(mapping.Fields, valueField, targetField, 8)
		}
	}
	spec := map[string]any{
		"lookup_path":        mappingPath,
		"base_fields":        sourceFields,
		"lookup_value_field": valueField,
		"target_field":       targetField,
		"match_mode":         "contains",
	}
	if len(mappingFields) > 0 {
		spec["lookup_fields"] = mappingFields
	}
	out := plan
	out.Status = "ready"
	out.Script = ""
	out.ContinueAfter = true
	out.Actions = []dataquery.DataAction{{
		ID:             "materialize_" + dataTaskSafeActionName(targetField),
		Kind:           dataquery.DataActionEnrichRecords,
		Purpose:        fmt.Sprintf("materialize field %s on %s using a mapping/reference input", targetField, basePath),
		InputPaths:     []string{basePath, mappingPath},
		OutputArtifact: dataTaskMissingFieldOutputArtifact(basePath, targetField),
		Params: map[string]string{
			"base_path":         basePath,
			"lookup_specs_json": mustCompactJSONString([]map[string]any{spec}),
		},
	}}
	if strings.TrimSpace(out.WhyThisBatch) == "" {
		out.WhyThisBatch = fmt.Sprintf("derive_fields cannot consume multiple record sets; convert the lookup/reference mapping into enrich_records to materialize %s first", targetField)
	}
	if strings.TrimSpace(out.NextBatch) == "" {
		out.NextBatch = "continue with join_records or compute_contributions after the enriched artifact materializes"
	}
	return out, true
}

func dataTaskHistoricalMissingJoinFieldName(records []dataTaskWorkflowRecord) string {
	for i := len(records) - 1; i >= 0; i-- {
		matches := dataTaskJoinMissingFieldPattern.FindStringSubmatch(strings.TrimSpace(records[i].Err))
		if len(matches) >= 3 && strings.TrimSpace(matches[2]) != "" {
			return strings.TrimSpace(matches[2])
		}
	}
	return ""
}

var dataTaskJoinMissingFieldPattern = regexp.MustCompile(`join_records\s+(left|right)\s+field\s+"([^"]+)"\s+was\s+not\s+found\s+in\s+(?:left|right)\s+input\s+(\S+)`)

func dataTaskMissingJoinFieldFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	matches := dataTaskJoinMissingFieldPattern.FindStringSubmatch(errText)
	if len(matches) < 4 {
		return dataquery.TaskPlan{}, false
	}
	missingField := strings.TrimSpace(matches[2])
	baseAlias := strings.Trim(strings.TrimSpace(matches[3]), `"'`)
	if missingField == "" || baseAlias == "" {
		return dataquery.TaskPlan{}, false
	}
	access := dataTaskWorkflowArtifactAccess(records, 24)
	base, ok := dataTaskArtifactAccessByAlias(access, baseAlias)
	if !ok || len(base.Fields) == 0 {
		return dataquery.TaskPlan{}, false
	}
	if firstDataTaskExistingField(base.Fields, missingField) != "" {
		return dataquery.TaskPlan{}, false
	}
	sourceFields := dataTaskSourceFieldsForMissingTarget(base.Fields, missingField, 6)
	if len(sourceFields) == 0 {
		return dataquery.TaskPlan{}, false
	}
	mapping, valueField, mappingFields, matchMode, ok := dataTaskMappingForMissingTarget(access, base, missingField, 8)
	if !ok {
		return dataquery.TaskPlan{}, false
	}
	mappingAlias := firstDataTaskArtifactAlias(mapping)
	if mappingAlias == "" {
		return dataquery.TaskPlan{}, false
	}
	outputArtifact := dataTaskMissingFieldOutputArtifact(baseAlias, missingField)
	if dataTaskWorkflowHasArtifactAlias(records, outputArtifact) {
		return dataquery.TaskPlan{}, false
	}
	if normalizeDataTaskCoveragePath(outputArtifact) == normalizeDataTaskCoveragePath(mappingAlias) ||
		normalizeDataTaskCoveragePath(outputArtifact) == normalizeDataTaskCoveragePath(baseAlias) {
		return dataquery.TaskPlan{}, false
	}
	out := plan
	out.Status = "ready"
	out.Script = ""
	out.ContinueAfter = true
	out.Actions = []dataquery.DataAction{{
		ID:             "materialize_" + dataTaskSafeActionName(missingField),
		Kind:           dataquery.DataActionEnrichRecords,
		Purpose:        fmt.Sprintf("materialize missing field %s on %s using an existing mapping/reference artifact", missingField, baseAlias),
		InputPaths:     []string{baseAlias, mappingAlias},
		OutputArtifact: outputArtifact,
		Params: map[string]string{
			"base_path": baseAlias,
			"lookup_specs_json": mustCompactJSONString([]map[string]any{{
				"lookup_path":        mappingAlias,
				"base_fields":        sourceFields,
				"lookup_fields":      mappingFields,
				"lookup_value_field": valueField,
				"target_field":       missingField,
				"match_mode":         matchMode,
			}}),
		},
	}}
	if strings.TrimSpace(out.WhyThisBatch) == "" {
		out.WhyThisBatch = fmt.Sprintf("join_records failed because %s did not contain field %s; materialize that field first from existing mapping/reference rows", baseAlias, missingField)
	}
	if strings.TrimSpace(out.NextBatch) == "" {
		out.NextBatch = "retry the join_records stage using the newly enriched artifact, then continue to compute_contributions"
	}
	if dataTaskFallbackPlanAlreadySeen(records, out) {
		return dataquery.TaskPlan{}, false
	}
	return out, true
}

func dataTaskHistoricalMissingJoinFieldFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) (dataquery.TaskPlan, bool) {
	if len(records) == 0 {
		return dataquery.TaskPlan{}, false
	}
	for i := len(records) - 1; i >= 0; i-- {
		errText := strings.TrimSpace(records[i].Err)
		if errText == "" || !dataTaskJoinMissingFieldPattern.MatchString(errText) {
			continue
		}
		fallback, ok := dataTaskMissingJoinFieldFallback(records, plan, errText)
		if !ok {
			continue
		}
		if dataTaskFallbackPlanAlreadySeen(records, fallback) {
			continue
		}
		if strings.TrimSpace(fallback.WhyThisBatch) == "" {
			fallback.WhyThisBatch = "a previous join_records action found that a base artifact was missing a join field; use the now-available mapping/reference artifact to materialize that field before retrying the join"
		}
		if strings.TrimSpace(fallback.NextBatch) == "" {
			fallback.NextBatch = "retry the join_records stage using the newly enriched artifact, then continue to compute_contributions"
		}
		return fallback, true
	}
	return dataquery.TaskPlan{}, false
}

func dataTaskFallbackPlanAlreadySeen(records []dataTaskWorkflowRecord, candidate dataquery.TaskPlan) bool {
	signature := dataTaskFallbackPlanSignature(candidate)
	if signature == "" {
		return false
	}
	for _, rec := range records {
		if dataTaskFallbackPlanSignature(rec.Plan) == signature {
			return true
		}
	}
	return false
}

func dataTaskFallbackPlanSignature(plan dataquery.TaskPlan) string {
	if len(plan.Actions) == 0 {
		return ""
	}
	type actionSig struct {
		Kind           string            `json:"kind,omitempty"`
		InputPaths     []string          `json:"input_paths,omitempty"`
		OutputArtifact string            `json:"output_artifact,omitempty"`
		Params         map[string]string `json:"params,omitempty"`
	}
	sig := struct {
		Actions []actionSig `json:"actions,omitempty"`
	}{Actions: make([]actionSig, 0, len(plan.Actions))}
	for _, action := range plan.Actions {
		sig.Actions = append(sig.Actions, actionSig{
			Kind:           string(normalizeDataActionKindForWorkflow(action.Kind)),
			InputPaths:     cleanDataTaskStrings(action.InputPaths),
			OutputArtifact: normalizeDataTaskCoveragePath(action.OutputArtifact),
			Params:         action.Params,
		})
	}
	raw, err := json.Marshal(sig)
	if err != nil {
		return ""
	}
	return string(raw)
}

func dataTaskArtifactAccessByAlias(access []dataTaskArtifactAccessPrompt, alias string) (dataTaskArtifactAccessPrompt, bool) {
	want := normalizeDataTaskCoveragePath(alias)
	for _, artifact := range access {
		if normalizeDataTaskCoveragePath(artifact.ID) == want && want != "" {
			return artifact, true
		}
		for _, candidate := range artifact.Aliases {
			if normalizeDataTaskCoveragePath(candidate) == want && want != "" {
				return artifact, true
			}
		}
	}
	return dataTaskArtifactAccessPrompt{}, false
}

func dataTaskMappingForMissingTarget(access []dataTaskArtifactAccessPrompt, base dataTaskArtifactAccessPrompt, missingField string, limit int) (dataTaskArtifactAccessPrompt, string, []string, string, bool) {
	baseAlias := firstDataTaskArtifactAlias(base)
	bestScore := -1
	var best dataTaskArtifactAccessPrompt
	bestValue := ""
	var bestSources []string
	bestMode := "exact"
	for _, artifact := range access {
		alias := firstDataTaskArtifactAlias(artifact)
		if alias == "" || alias == baseAlias || len(artifact.Fields) == 0 {
			continue
		}
		valueField, exactValue := dataTaskMappingValueFieldForMissingTarget(artifact.Fields, missingField)
		if valueField == "" {
			continue
		}
		sourceFields := dataTaskReferenceSourceFieldsForMissingTarget(artifact.Fields, valueField, missingField, limit)
		if len(sourceFields) == 0 {
			continue
		}
		score := 0
		if exactValue {
			score += 8
		}
		if dataTaskArtifactLooksLikeMapping(artifact) {
			score += 4
		}
		score += dataTaskFieldTokenOverlapScore(artifact.Fields, missingField)
		score += len(sourceFields)
		mode := "exact"
		kind := strings.ToLower(strings.TrimSpace(artifact.Kind))
		if !strings.Contains(kind, "normalize") && !strings.Contains(kind, "entity_resolution") {
			mode = "contains"
		}
		if score > bestScore {
			bestScore = score
			best = artifact
			bestValue = valueField
			bestSources = sourceFields
			bestMode = mode
		}
	}
	if bestScore < 0 {
		return dataTaskArtifactAccessPrompt{}, "", nil, "", false
	}
	return best, bestValue, bestSources, bestMode, true
}

func dataTaskMappingValueFieldForMissingTarget(fields []string, missingField string) (string, bool) {
	if exact := firstDataTaskExistingField(fields, missingField); exact != "" {
		return exact, true
	}
	value := dataTaskReferenceValueField(fields)
	if value == "" {
		return "", false
	}
	return value, false
}

func dataTaskSourceFieldsForMissingTarget(fields []string, missingField string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	tokens := dataTaskFieldTokens(missingField)
	var out []string
	seen := map[string]bool{}
	add := func(field string) {
		field = strings.TrimSpace(field)
		key := strings.ToLower(field)
		if field == "" || seen[key] || strings.HasPrefix(key, "_") || key == strings.ToLower(strings.TrimSpace(missingField)) {
			return
		}
		seen[key] = true
		out = append(out, field)
	}
	for _, field := range fields {
		if dataTaskFieldTokenOverlap(dataTaskFieldTokens(field), tokens) > 0 {
			add(field)
			if len(out) >= limit {
				return out
			}
		}
	}
	for _, field := range dataTaskCandidateSourceFields(fields, limit-len(out)) {
		add(field)
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func dataTaskReferenceSourceFieldsForMissingTarget(fields []string, valueField, missingField string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(field string) {
		field = strings.TrimSpace(field)
		key := strings.ToLower(field)
		if field == "" || seen[key] || strings.HasPrefix(key, "_") || key == strings.ToLower(strings.TrimSpace(valueField)) {
			return
		}
		seen[key] = true
		out = append(out, field)
	}
	tokens := dataTaskFieldTokens(missingField)
	for _, field := range fields {
		if dataTaskFieldTokenOverlap(dataTaskFieldTokens(field), tokens) > 0 {
			add(field)
			if len(out) >= limit {
				return out
			}
		}
	}
	for _, field := range dataTaskCandidateReferenceSourceFields(fields, valueField, limit-len(out)) {
		add(field)
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func dataTaskFieldTokenOverlapScore(fields []string, target string) int {
	targetTokens := dataTaskFieldTokens(target)
	score := 0
	for _, field := range fields {
		score += dataTaskFieldTokenOverlap(dataTaskFieldTokens(field), targetTokens)
	}
	return score
}

func dataTaskFieldTokens(field string) map[string]bool {
	out := map[string]bool{}
	var b strings.Builder
	flush := func() {
		token := strings.ToLower(strings.TrimSpace(b.String()))
		b.Reset()
		if len(token) >= 3 {
			out[token] = true
		}
	}
	for _, r := range field {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func dataTaskFieldTokenOverlap(left, right map[string]bool) int {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	score := 0
	for token := range left {
		if right[token] {
			score++
		}
	}
	return score
}

func dataTaskMissingFieldOutputArtifact(baseAlias, missingField string) string {
	base := strings.TrimSpace(path.Base(baseAlias))
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem = "enriched_records"
	}
	return stem + "_with_" + dataTaskSafeActionName(missingField) + ".json"
}

func dataTaskSafeActionName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "field"
	}
	return out
}

func dataTaskPlanCrossesTypedActionRanks(plan dataquery.TaskPlan) bool {
	firstRank := 0
	for _, action := range plan.Actions {
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		if kind == dataquery.DataActionCustomTransform && strings.TrimSpace(action.Script) != "" {
			continue
		}
		rank := dataTaskActionDependencyRank(kind)
		if rank == 0 {
			continue
		}
		if firstRank == 0 {
			firstRank = rank
			continue
		}
		if rank != firstRank {
			return true
		}
	}
	return false
}

func dataTaskActionDependencyRank(kind dataquery.DataActionKind) int {
	return dataworkflow.DependencyRank(normalizeDataActionKindForWorkflow(kind))
}

func dataTaskWorkflowMissingValidationStages(state dataTaskWorkflowStateView) []string {
	var out []string
	if state.RuleCoverageRequired && state.RuleCoverageRecords == 0 {
		out = append(out, "rule_coverage")
	}
	if state.EntityResolutionRequired && state.EntityResolutionRecords == 0 && !state.EntityStageMaterialized {
		out = append(out, "entity_resolution")
	}
	if state.DecisionRecordsRequired && state.DecisionRecords == 0 {
		out = append(out, "decision_records")
	}
	if state.ContributionLedgerRequired && state.ContributionRecords == 0 {
		out = append(out, "contribution_ledger")
	}
	if state.ReconcileRequired && !state.HasReconcile {
		out = append(out, "reconcile")
	}
	if !state.HasAnswer {
		out = append(out, "final_answer")
	}
	return out
}

func dataTaskPlanHasScriptedCustomTransform(plan dataquery.TaskPlan) bool {
	for _, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionCustomTransform && strings.TrimSpace(action.Script) != "" {
			return true
		}
	}
	return false
}

func dataTaskPlanIsSingleIntermediateCustomTransform(plan dataquery.TaskPlan) bool {
	if !plan.ContinueAfter || len(plan.Actions) != 1 {
		return false
	}
	action := plan.Actions[0]
	return normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionCustomTransform && strings.TrimSpace(action.Script) != ""
}

func dataTaskPlanIsNarrowSingleIntermediateCustomTransform(plan dataquery.TaskPlan, state dataTaskWorkflowStateView) bool {
	if !dataTaskPlanIsSingleIntermediateCustomTransform(plan) {
		return false
	}
	action := plan.Actions[0]
	lines := dataTaskScriptLineCount(action.Script)
	if lines == 0 || lines >= dataTaskComplexCustomScriptLineLimit {
		return false
	}
	if dataTaskActionLooksLikeWholeWorkflow(plan, action, 0) {
		return false
	}
	if dataTaskValidationLedgerCount(plan.CoverageContract) >= 2 {
		return false
	}
	if (state.NextStage == "prepare_contribution_inputs" || state.NextStage == "compute_contributions") && len(dataTaskWorkflowMissingValidationStages(state)) > 1 {
		return false
	}
	return true
}

func dataTaskTextConstraintCoverageGuardError(plan dataquery.TaskPlan) string {
	if plan.ContinueAfter || plan.CoverageContract.RuleCoverageRequired {
		return ""
	}
	if dataTaskValidationLedgerCount(plan.CoverageContract) < 2 {
		return ""
	}
	var customInputs []string
	if len(plan.Actions) == 0 {
		if strings.TrimSpace(plan.Script) == "" {
			return ""
		}
		customInputs = append(customInputs, plan.InputPaths...)
	} else {
		for _, action := range plan.Actions {
			if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform || strings.TrimSpace(action.Script) == "" {
				continue
			}
			customInputs = append(customInputs, action.InputPaths...)
		}
	}
	if len(customInputs) == 0 {
		return ""
	}
	inputs := map[string]bool{}
	for _, p := range cleanDataTaskStrings(customInputs) {
		inputs[normalizeDataTaskCoveragePath(p)] = true
	}
	var materials []string
	for _, material := range plan.CoverageContract.RequiredMaterials {
		if normalizeCoverageMaterialUseModeForWorkflow(material.UsageMode) != dataquery.MaterialUseScriptConsumed {
			continue
		}
		p := normalizeDataTaskCoveragePath(material.Path)
		if p == "" || !inputs[p] || !dataTaskPathLooksLikeTextConstraintMaterial(p) {
			continue
		}
		materials = append(materials, p)
	}
	if len(materials) == 0 {
		return ""
	}
	sort.Strings(materials)
	return fmt.Sprintf("data planning incomplete: terminal data calculation consumes text/rule material(s) %s with decision/contribution/reconcile ledgers but coverage_contract.rule_coverage_required=false. Add a derive_rules action or emit result.rule_coverage records linked from decisions/contributions/entity_resolutions before completing.",
		strings.Join(materials, ", "))
}

func dataTaskPathLooksLikeTextConstraintMaterial(p string) bool {
	ext := strings.ToLower(path.Ext(strings.TrimSpace(p)))
	switch ext {
	case ".md", ".markdown", ".txt", ".text", ".rst", ".adoc", ".asciidoc":
		return true
	default:
		return false
	}
}

func normalizeCoverageMaterialUseModeForWorkflow(mode dataquery.CoverageMaterialUseMode) dataquery.CoverageMaterialUseMode {
	switch dataquery.CoverageMaterialUseMode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case "", dataquery.MaterialUseScriptConsumed:
		return dataquery.MaterialUseScriptConsumed
	case dataquery.MaterialUseTextEvidenceConsumed:
		return dataquery.MaterialUseTextEvidenceConsumed
	case dataquery.MaterialUsePlannerDistilled:
		return dataquery.MaterialUsePlannerDistilled
	case dataquery.MaterialUseReferenceOnly:
		return dataquery.MaterialUseReferenceOnly
	default:
		return dataquery.MaterialUseScriptConsumed
	}
}

func dataTaskTerminalRequiredMaterialSchedulingError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	if plan.ContinueAfter {
		return ""
	}
	required := cleanDataTaskStrings(plan.CoverageContract.RequiredRunnerInputPaths())
	if len(required) == 0 {
		return ""
	}
	scheduled := dataTaskScheduledMaterialConsumption(records, plan)
	var missing []string
	for _, p := range required {
		if !scheduled[p] {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("data planning incomplete: terminal batch declares %d required material(s) that are not scheduled for script/typed-action consumption: %s. Add focused actions that read these materials, change their usage_mode to planner_distilled/text_evidence_consumed when appropriate, or keep continue_after=true until the required materials are covered.",
		len(missing), strings.Join(missing, ", "))
}

func dataTaskScheduledMaterialConsumption(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) map[string]bool {
	out := map[string]bool{}
	mark := func(values []string) {
		for _, value := range cleanDataTaskStrings(values) {
			out[value] = true
		}
	}
	for _, rec := range records {
		if rec.Result != nil {
			mark(rec.Result.ConsumedPaths)
			for _, artifact := range rec.Result.Artifacts {
				mark(artifact.SourcePaths)
				mark(dataTaskArtifactAliasPaths(artifact))
			}
		}
	}
	if len(plan.Actions) == 0 {
		mark(plan.InputPaths)
	}
	for _, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionMaterialInventory {
			continue
		}
		mark(action.InputPaths)
	}
	return out
}

func dataTaskActionDependencyGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	return dataTaskActionDependencyGuardResult(records, plan, action, actionIndex).ErrorText()
}

func dataTaskActionDependencyGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	if errText := dataTaskActionIntraBatchDependencyGuardError(plan, actionIndex); errText != "" {
		return dataTaskActionGuardResultFromMessage("intra_batch_dependency", errText, action)
	}
	if errText := dataTaskActionInputAvailabilityGuardError(records, plan, action, actionIndex); errText != "" {
		return dataTaskActionGuardResultFromMessage("unavailable_action_input", errText, action)
	}
	if guard := dataTaskActionFieldContractGuardResult(records, plan, action, actionIndex); !guard.Empty() {
		return guard
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if contract, ok := dataworkflow.InputPathContract(kind); ok {
		if len(inputs) < contract.Min {
			if kind == dataquery.DataActionComputeContribs {
				return dataTaskActionGuardResultFromMessage("missing_action_inputs", fmt.Sprintf("data planning incomplete: action %d (%s) requires input_paths containing existing records or generated artifact aliases before contribution computation.",
					actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))), action)
			}
			return dataTaskActionGuardResultFromMessage("missing_action_inputs", fmt.Sprintf("data planning incomplete: action %d (%s) requires input_paths. Choose concrete candidate material paths or prior artifact aliases; do not emit an empty %s action.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))), strings.TrimSpace(string(kind))), action)
		}
		if contract.Max > 0 && len(inputs) > contract.Max {
			if contract.SingleRecordSet {
				return dataTaskActionGuardResultFromMessage("too_many_action_inputs", fmt.Sprintf("data planning incomplete: action %d (%s) is %s with %d input_paths. %s is a single-record-set action. Do not use it for lookup/reference-table mapping. Split different schemas into separate actions, or first use normalize_entities, enrich_records, or join_records to create one joined/generated artifact.",
					actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))), kind, len(inputs), kind), action)
			}
			return dataTaskActionGuardResultFromMessage("too_many_action_inputs", fmt.Sprintf("data planning incomplete: action %d (%s) is %s with %d input_paths, but this action accepts at most %d. Split the graph into atomic DAG ranks; for joins, combine exactly two record sets first, let the output artifact materialize, then join that artifact with the next record set.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))), kind, len(inputs), contract.Max), action)
		}
	}
	switch kind {
	case dataquery.DataActionInspectMaterial, dataquery.DataActionExtractRecords, dataquery.DataActionDeriveFields, dataquery.DataActionExtractFields, dataquery.DataActionGroupRecords, dataquery.DataActionExpandRecords, dataquery.DataActionFilterRecords, dataquery.DataActionQualifyRecords:
		if kind == dataquery.DataActionDeriveFields && !dataTaskDeriveFieldsActionHasSpec(action) {
			return dataTaskActionGuardResultFromMessage("missing_action_spec", fmt.Sprintf("data planning incomplete: action %d (%s) is derive_fields but has no field specification. Add params.field_specs_json (array of source_field/target_field/operation specs) or a single source_field+target_field+operation spec; if this batch only needs to materialize rows without deriving fields, use extract_records instead.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))), action)
		}
		if kind == dataquery.DataActionExtractFields && !dataTaskDeriveFieldsActionHasSpec(action) {
			return dataTaskActionGuardResultFromMessage("missing_action_spec", fmt.Sprintf("data planning incomplete: action %d (%s) is extract_fields but has no field specification. Add params.field_specs or params.extract_specs as an array of source_field/target_field/operation/pattern specs; use it to materialize structured fields from one or more same-schema record/text artifacts.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))), action)
		}
		if kind == dataquery.DataActionGroupRecords && !dataTaskGroupRecordsActionHasSpec(action) {
			return dataTaskActionGuardResultFromMessage("missing_action_spec", fmt.Sprintf("data planning incomplete: action %d (%s) is group_records but has no grouping/text specification. Add params.group_field or params.group_fields, params.text_fields/source_fields, and params.target_field; use group_records to combine multiple rows/spans from one artifact into one grouped record before extract_fields, join_records, filtering, or contribution calculation.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))), action)
		}
		if kind == dataquery.DataActionExpandRecords && !dataTaskExpandRecordsActionHasSpec(action) {
			return dataTaskActionGuardResultFromMessage("missing_action_spec", fmt.Sprintf("data planning incomplete: action %d (%s) is expand_records but has no source_field. Add params.source_field and optionally params.target_field plus delimiter/separator or split_pattern; use expand_records only to turn one multi-value field into multiple records.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))), action)
		}
		if kind == dataquery.DataActionFilterRecords && !dataTaskFilterRecordsActionHasSpec(action) {
			return dataTaskActionGuardResultFromMessage("missing_action_spec", fmt.Sprintf("data planning incomplete: action %d (%s) is filter_records but has no filters. Add params.filters_json (array of field/op/value filters) or filter_field/filter_op/filter_value; use filter_records only when the filter fields already exist on the input artifact.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))), action)
		}
	case dataquery.DataActionApplyResolutions:
		if len(inputs) < 2 && strings.TrimSpace(action.Params["resolution_path"]) == "" && strings.TrimSpace(action.Params["resolution_specs_json"]) == "" {
			return dataTaskActionGuardResultFromMessage("missing_action_inputs", fmt.Sprintf("data planning incomplete: action %d (%s) is apply_entity_resolutions but has no resolution input. Provide input_paths=[base_record_artifact, entity_resolution_artifact...] or params.resolution_specs with resolution_path.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))), action)
		}
	case dataquery.DataActionReconcile:
		if !dataTaskWorkflowHasContributionProducer(records, plan, actionIndex) {
			return dataTaskActionGuardResultFromMessage("missing_upstream_ledger", fmt.Sprintf("data planning incomplete: action %d (%s) requires contribution records, but no prior compute_contributions result or earlier compute_contributions action is available. Add a bounded compute_contributions batch first, let it execute, then reconcile in a later batch or after a previous contribution-producing action.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))), action)
		}
	case dataquery.DataActionAssembleAnswer:
		if !dataTaskWorkflowHasReconcileProducer(records, plan, actionIndex) {
			return dataTaskActionGuardResultFromMessage("missing_upstream_ledger", fmt.Sprintf("data planning incomplete: action %d (%s) requires a prior reconcile report. Add reconcile_artifacts first, let it execute, then assemble the final output in a later batch.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))), action)
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskActionIntraBatchDependencyGuardError(plan dataquery.TaskPlan, actionIndex int) string {
	dependencyIndex, input, producer, ok := dataTaskFirstIntraBatchDependency(plan.Actions)
	if !ok || dependencyIndex != actionIndex {
		return ""
	}
	return fmt.Sprintf("data planning incomplete: action %d (%s) consumes prior action output %s from action %s in the same batch. Execute the producer action rank first, let it materialize a real artifact and field contract, then continue with dependent actions from the deferred queue.",
		actionIndex+1,
		firstNonEmptyString(strings.TrimSpace(plan.Actions[actionIndex].ID), strings.TrimSpace(string(plan.Actions[actionIndex].Kind))),
		input,
		firstNonEmptyString(strings.TrimSpace(producer.ID), strings.TrimSpace(producer.OutputArtifact), strings.TrimSpace(string(producer.Kind))))
}

func dataTaskActionInputAvailabilityGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	if len(records) == 0 || !dataTaskWorkflowHasSuccessfulResult(records) {
		return ""
	}
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	switch kind {
	case dataquery.DataActionMaterialInventory,
		dataquery.DataActionInspectMaterial,
		dataquery.DataActionExtractRecords,
		dataquery.DataActionDeriveRules:
		return ""
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if len(inputs) == 0 {
		return ""
	}
	available := dataTaskWorkflowAvailableActionInputPaths(records, plan, actionIndex)
	var missing []string
	for _, input := range inputs {
		if dataTaskCoveragePathCovered(available, input) {
			continue
		}
		missing = append(missing, input)
	}
	if len(missing) == 0 {
		return ""
	}
	sort.Strings(missing)
	return fmt.Sprintf("data planning incomplete: action %d (%s) references unavailable input_path(s) [%s]. Use covered source materials, prior generated artifact aliases from workflow_state_json.artifact_availability, or add earlier typed actions in this batch that output these aliases before consuming them; do not assume deferred or future action outputs already exist.",
		actionIndex+1,
		firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
		strings.Join(missing, ", "))
}

func dataTaskActionFieldContractGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	return dataTaskActionFieldContractGuardResult(records, plan, action, actionIndex).ErrorText()
}

func dataTaskActionFieldContractGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	if len(records) == 0 || !dataTaskWorkflowHasSuccessfulResult(records) {
		return dataworkflow.GuardResult{}
	}
	access := dataTaskWorkflowArtifactContractAccess(records)
	if len(access) == 0 {
		return dataworkflow.GuardResult{}
	}
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	if guard := dataTaskActionZeroMatchFilterInputGuardResult(records, plan, action, actionIndex); !guard.Empty() {
		return guard
	}
	if guard := dataTaskActionUnmatchedResolutionInputGuardResult(records, plan, action, actionIndex); !guard.Empty() {
		return guard
	}
	if guard := dataTaskActionZeroEligibleInputGuardResult(records, plan, action, actionIndex); !guard.Empty() {
		return guard
	}
	switch kind {
	case dataquery.DataActionDeriveFields, dataquery.DataActionExtractFields:
		inputs := cleanDataTaskStrings(action.InputPaths)
		if len(inputs) != 1 {
			return dataworkflow.GuardResult{}
		}
		missing := dataTaskMissingDeriveFieldInputs(access, inputs[0], action)
		return dataTaskActionMissingFieldContractGuardResult(action, actionIndex, inputs[0], missing, access)
	case dataquery.DataActionGroupRecords:
		inputs := cleanDataTaskStrings(action.InputPaths)
		if len(inputs) != 1 {
			return dataworkflow.GuardResult{}
		}
		missing := dataTaskMissingFieldRefsOnArtifact(access, inputs[0], dataTaskGroupRecordsActionFieldRefs(action))
		return dataTaskActionMissingFieldContractGuardResult(action, actionIndex, inputs[0], missing, access)
	case dataquery.DataActionExpandRecords:
		inputs := cleanDataTaskStrings(action.InputPaths)
		if len(inputs) != 1 {
			return dataworkflow.GuardResult{}
		}
		fields := parseDataTaskActionStringListParam(firstNonEmptyString(
			action.Params["source_field"],
			action.Params["input_field"],
			action.Params["field"],
			action.Params["value_field"],
		))
		if len(fields) == 0 {
			return dataworkflow.GuardResult{}
		}
		missing := dataTaskMissingFieldsOnArtifact(access, inputs[0], fields)
		return dataTaskActionMissingFieldContractGuardResult(action, actionIndex, inputs[0], missing, access)
	case dataquery.DataActionFilterRecords:
		inputs := cleanDataTaskStrings(action.InputPaths)
		if len(inputs) != 1 {
			return dataworkflow.GuardResult{}
		}
		fields := dataTaskFilterActionFieldRefs(action)
		if len(fields) == 0 {
			return dataworkflow.GuardResult{}
		}
		missing := dataTaskMissingFieldsOnArtifact(access, inputs[0], fields)
		return dataTaskActionMissingFieldContractGuardResult(action, actionIndex, inputs[0], missing, access)
	case dataquery.DataActionQualifyRecords:
		inputs := cleanDataTaskStrings(action.InputPaths)
		if len(inputs) != 1 {
			return dataworkflow.GuardResult{}
		}
		fields := dataTaskQualifyActionFieldRefs(action)
		if len(fields) == 0 {
			return dataworkflow.GuardResult{}
		}
		missing := dataTaskMissingFieldsOnArtifact(access, inputs[0], fields)
		return dataTaskActionMissingFieldContractGuardResult(action, actionIndex, inputs[0], missing, access)
	case dataquery.DataActionComputeContribs:
		inputs := cleanDataTaskStrings(action.InputPaths)
		if len(inputs) != 1 {
			return dataworkflow.GuardResult{}
		}
		fields := dataTaskComputeActionFieldRefs(action)
		if len(fields) == 0 {
			return dataworkflow.GuardResult{}
		}
		missing := dataTaskMissingFieldsOnArtifact(access, inputs[0], fields)
		return dataTaskActionMissingFieldContractGuardResult(action, actionIndex, inputs[0], missing, access)
	case dataquery.DataActionJoinRecords:
		return dataTaskJoinActionFieldContractGuardResult(access, action, actionIndex)
	case dataquery.DataActionNormalizeEntities:
		return dataTaskNormalizeActionFieldContractGuardResult(access, action, actionIndex)
	case dataquery.DataActionEnrichRecords:
		return dataTaskEnrichActionFieldContractGuardResult(access, action, actionIndex)
	case dataquery.DataActionApplyResolutions:
		return dataTaskApplyResolutionActionFieldContractGuardResult(access, action, actionIndex)
	}
	return dataworkflow.GuardResult{}
}

func dataTaskActionZeroMatchFilterInputGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	return dataTaskActionZeroMatchFilterInputGuardResult(records, plan, action, actionIndex).ErrorText()
}

func dataTaskActionZeroMatchFilterInputGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	switch kind {
	case dataquery.DataActionJoinRecords, dataquery.DataActionComputeContribs, dataquery.DataActionReconcile, dataquery.DataActionAssembleAnswer:
	default:
		return dataworkflow.GuardResult{}
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if len(inputs) == 0 {
		return dataworkflow.GuardResult{}
	}
	contract := dataTaskWorkflowCoverageContract(records, plan)
	if !contract.ContributionLedgerRequired && !contract.ReconcileRequired {
		return dataworkflow.GuardResult{}
	}
	state := dataTaskWorkflowStateView{
		ContributionLedgerRequired: contract.ContributionLedgerRequired,
		ReconcileRequired:          contract.ReconcileRequired,
	}
	for _, rec := range records {
		if rec.Result != nil {
			state.ContributionRecords += len(rec.Result.Contributions)
		}
	}
	issues := dataTaskWorkflowZeroMatchFilterIssues(records, state, 16)
	if len(issues) == 0 {
		return dataworkflow.GuardResult{}
	}
	for _, input := range inputs {
		inputKey := normalizeDataTaskCoveragePath(input)
		for _, issue := range issues {
			for _, alias := range dataTaskZeroMatchFilterIssueAliases(issue) {
				if inputKey != "" && inputKey == normalizeDataTaskCoveragePath(alias) {
					message := fmt.Sprintf("data planning incomplete: action %d (%s) consumes zero-match filter artifact %s (%d/%d rows) while contribution/reconcile is still required. Inspect actual filter field values or rerun filter_records against %s with corrected filters before join_records or compute_contributions; do not continue from an empty candidate set.",
						actionIndex+1,
						firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
						firstNonEmptyString(issue.ArtifactID, input),
						issue.OutputRows,
						issue.InputRows,
						firstNonEmptyString(issue.InputPath, "the non-empty source artifact"))
					reason := firstNonEmptyString(
						strings.TrimSpace(issue.Reason),
						fmt.Sprintf("zero-match filter artifact %s produced %d rows from %d input rows", firstNonEmptyString(issue.ArtifactID, input), issue.OutputRows, issue.InputRows),
					)
					violation := dataworkflow.NewActionInputViolation(
						"zero_match_filter",
						"error",
						dataworkflow.RepairNeedsTypedAction,
						action,
						input,
						issue.FilterFields,
						reason,
						issue.RepairActionHints,
					)
					return dataworkflow.NewGuardResult("zero_match_filter", "error", dataworkflow.RepairNeedsTypedAction, message, violation)
				}
			}
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskZeroMatchFilterIssueAliases(issue dataTaskZeroMatchFilterIssue) []string {
	aliases := append([]string(nil), issue.Aliases...)
	if strings.TrimSpace(issue.ArtifactID) != "" {
		aliases = append(aliases, issue.ArtifactID)
	}
	return cleanDataTaskStrings(aliases)
}

func dataTaskActionUnmatchedResolutionInputGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	return dataTaskActionUnmatchedResolutionInputGuardResult(records, plan, action, actionIndex).ErrorText()
}

func dataTaskActionUnmatchedResolutionInputGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	switch kind {
	case dataquery.DataActionDeriveFields,
		dataquery.DataActionExtractFields,
		dataquery.DataActionGroupRecords,
		dataquery.DataActionExpandRecords,
		dataquery.DataActionFilterRecords,
		dataquery.DataActionQualifyRecords,
		dataquery.DataActionEnrichRecords,
		dataquery.DataActionJoinRecords,
		dataquery.DataActionComputeContribs,
		dataquery.DataActionReconcile,
		dataquery.DataActionAssembleAnswer:
	default:
		return dataworkflow.GuardResult{}
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if len(inputs) == 0 {
		return dataworkflow.GuardResult{}
	}
	state := dataTaskWorkflowState(records, plan)
	issues := dataTaskWorkflowUnmatchedResolutionIssues(records, state, 16)
	if len(issues) == 0 {
		return dataworkflow.GuardResult{}
	}
	for _, input := range inputs {
		inputKey := normalizeDataTaskCoveragePath(input)
		for _, issue := range issues {
			for _, alias := range dataTaskUnmatchedResolutionIssueAliases(issue) {
				if inputKey != "" && inputKey == normalizeDataTaskCoveragePath(alias) {
					message := fmt.Sprintf("data planning incomplete: action %d (%s) consumes all-unmatched resolution artifact %s (base_rows=%d target_fields=[%s]) while contribution/reconcile is still required. Repair apply_entity_resolutions key/role matching against %s before filtering, qualification, join, or contribution calculation.",
						actionIndex+1,
						firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
						firstNonEmptyString(issue.ArtifactID, input),
						issue.BaseRows,
						strings.Join(issue.TargetFields, ", "),
						firstNonEmptyString(issue.BasePath, "the base record artifact"))
					reason := firstNonEmptyString(
						strings.TrimSpace(issue.Reason),
						fmt.Sprintf("all-unmatched resolution artifact %s has %d base rows and target fields [%s]", firstNonEmptyString(issue.ArtifactID, input), issue.BaseRows, strings.Join(issue.TargetFields, ", ")),
					)
					violation := dataworkflow.NewActionInputViolation(
						"unmatched_resolution",
						"error",
						dataworkflow.RepairNeedsTypedAction,
						action,
						input,
						issue.TargetFields,
						reason,
						issue.RepairActionHints,
					)
					return dataworkflow.NewGuardResult("unmatched_resolution", "error", dataworkflow.RepairNeedsTypedAction, message, violation)
				}
			}
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskUnmatchedResolutionIssueAliases(issue dataTaskUnmatchedResolutionIssue) []string {
	aliases := append([]string(nil), issue.Aliases...)
	if strings.TrimSpace(issue.ArtifactID) != "" {
		aliases = append(aliases, issue.ArtifactID)
	}
	return cleanDataTaskStrings(aliases)
}

func dataTaskActionZeroEligibleInputGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	return dataTaskActionZeroEligibleInputGuardResult(records, plan, action, actionIndex).ErrorText()
}

func dataTaskActionZeroEligibleInputGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	switch kind {
	case dataquery.DataActionJoinRecords, dataquery.DataActionComputeContribs, dataquery.DataActionReconcile, dataquery.DataActionAssembleAnswer:
	default:
		return dataworkflow.GuardResult{}
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if len(inputs) == 0 {
		return dataworkflow.GuardResult{}
	}
	state := dataTaskWorkflowState(records, plan)
	issues := dataTaskWorkflowZeroEligibleIssues(records, state, 16)
	if len(issues) == 0 {
		return dataworkflow.GuardResult{}
	}
	for _, input := range inputs {
		inputKey := normalizeDataTaskCoveragePath(input)
		for _, issue := range issues {
			for _, alias := range dataTaskZeroEligibleIssueAliases(issue) {
				if inputKey != "" && inputKey == normalizeDataTaskCoveragePath(alias) {
					message := fmt.Sprintf("data planning incomplete: action %d (%s) consumes zero-eligible qualification artifact %s (%d/%d eligible rows) while contribution/reconcile is still required. Repair the upstream field materialization, resolution application, or qualify_records filters before join_records or compute_contributions.",
						actionIndex+1,
						firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
						firstNonEmptyString(issue.ArtifactID, input),
						issue.EligibleRows,
						issue.InputRows)
					reason := firstNonEmptyString(
						strings.TrimSpace(issue.Reason),
						fmt.Sprintf("zero-eligible qualification artifact %s has %d eligible rows from %d input rows", firstNonEmptyString(issue.ArtifactID, input), issue.EligibleRows, issue.InputRows),
					)
					violation := dataworkflow.NewActionInputViolation(
						"zero_eligible_records",
						"error",
						dataworkflow.RepairNeedsTypedAction,
						action,
						input,
						nil,
						reason,
						issue.RepairActionHints,
					)
					return dataworkflow.NewGuardResult("zero_eligible_records", "error", dataworkflow.RepairNeedsTypedAction, message, violation)
				}
			}
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskZeroEligibleIssueAliases(issue dataTaskZeroEligibleIssue) []string {
	aliases := append([]string(nil), issue.Aliases...)
	if strings.TrimSpace(issue.ArtifactID) != "" {
		aliases = append(aliases, issue.ArtifactID)
	}
	return cleanDataTaskStrings(aliases)
}

func dataTaskActionMissingFieldContractMessage(action dataquery.DataAction, actionIndex int, input string, missing []string, access []dataTaskArtifactAccessPrompt) string {
	return dataTaskActionMissingFieldContractGuardResult(action, actionIndex, input, missing, access).ErrorText()
}

func dataTaskActionMissingFieldContractGuardResult(action dataquery.DataAction, actionIndex int, input string, missing []string, access []dataTaskArtifactAccessPrompt) dataworkflow.GuardResult {
	violation, ok := dataTaskActionMissingFieldContractViolation(action, input, missing, access)
	if !ok {
		return dataworkflow.GuardResult{}
	}
	return dataworkflow.NewGuardResult(
		"field_contract_violation",
		"error",
		dataworkflow.RepairNeedsTypedAction,
		dataTaskActionMissingFieldContractViolationMessage(action, actionIndex, violation),
		violation,
	)
}

func dataTaskActionInputContractGuardResult(code string, action dataquery.DataAction, inputAlias string, missingFields []string, message string, hints []string) dataworkflow.GuardResult {
	message = strings.TrimSpace(message)
	if message == "" {
		return dataworkflow.GuardResult{}
	}
	violation := dataworkflow.NewActionInputViolation(
		code,
		"error",
		dataworkflow.RepairNeedsTypedAction,
		action,
		inputAlias,
		missingFields,
		message,
		hints,
	)
	return dataworkflow.NewGuardResult(code, "error", dataworkflow.RepairNeedsTypedAction, message, violation)
}

func dataTaskActionMissingFieldContractViolation(action dataquery.DataAction, input string, missing []string, access []dataTaskArtifactAccessPrompt) (dataworkflow.WorkflowViolation, bool) {
	if len(missing) == 0 {
		return dataworkflow.WorkflowViolation{}, false
	}
	artifact, ok := dataTaskArtifactAccessByAlias(access, input)
	if !ok || len(artifact.Fields) == 0 {
		return dataworkflow.WorkflowViolation{}, false
	}
	return dataworkflow.NewFieldContractViolation(dataworkflow.FieldContractViolationInput{
		Action:            action,
		InputAlias:        input,
		MissingFields:     missing,
		AvailableFields:   artifact.Fields,
		SchemaProjections: dataTaskArtifactAccessSchemaProjection(access),
	}), true
}

func dataTaskActionMissingFieldContractViolationMessage(action dataquery.DataAction, actionIndex int, violation dataworkflow.WorkflowViolation) string {
	candidates := violation.CandidateArtifacts
	candidateHint := ""
	if len(candidates) > 0 {
		candidateHint = " Candidate artifact(s) with relevant field(s): " + strings.Join(candidates, "; ") + "."
	}
	return fmt.Sprintf("data planning incomplete: action %d (%s) references field(s) [%s] that are not present on input %s fields [%s].%s Use an existing artifact from workflow_state_json.artifact_availability, or first materialize the missing field(s) with derive_fields, extract_fields, group_records, enrich_records, join_records, or a valid prior typed action before consuming them.",
		actionIndex+1,
		firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
		strings.Join(violation.MissingFields, ", "),
		violation.InputAlias,
		strings.Join(clampDataTaskStringSlice(violation.AvailableFieldSample, 32), ", "),
		candidateHint)
}

func dataTaskFieldContractCandidateArtifacts(access []dataTaskArtifactAccessPrompt, input string, missing []string) []string {
	candidates := dataworkflow.FieldContractCandidateArtifacts(dataTaskArtifactAccessSchemaProjection(access), input, missing, 4)
	return dataworkflow.FieldContractCandidateLabels(candidates)
}

func dataTaskMissingFieldsOnArtifact(access []dataTaskArtifactAccessPrompt, alias string, fields []string) []string {
	missing := dataworkflow.MissingFieldsOnArtifactSchema(dataTaskArtifactAccessSchemaProjection(access), alias, fields)
	sort.Strings(missing)
	return missing
}

func dataTaskArtifactAccessSchemaProjection(access []dataTaskArtifactAccessPrompt) []dataworkflow.ArtifactSchemaProjection {
	out := make([]dataworkflow.ArtifactSchemaProjection, 0, len(access))
	for _, artifact := range access {
		out = append(out, dataTaskArtifactAccessToSchemaProjection(artifact))
	}
	return out
}

func dataTaskMissingDeriveFieldInputs(access []dataTaskArtifactAccessPrompt, alias string, action dataquery.DataAction) []string {
	artifact, ok := dataTaskArtifactAccessByAlias(access, alias)
	if !ok || len(artifact.Fields) == 0 {
		return nil
	}
	known := dataTaskFieldSet(artifact.Fields)
	var missing []string
	seenMissing := map[string]bool{}
	specs := dataTaskActionObjectListParam(action.Params, "field_specs_json", "derive_specs_json", "transforms_json", "field_specs", "derive_specs", "transforms")
	for _, spec := range specs {
		op := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
			dataTaskMapStringValue(spec, "operation"),
			dataTaskMapStringValue(spec, "op"),
			dataTaskMapStringValue(spec, "transform"),
		)))
		sources := dataTaskFieldRefsFromSpec(spec, "source_field", "input_field", "field", "value_field")
		sources = append(sources, dataTaskFieldRefsFromSpec(spec, "source_fields", "input_fields", "fields")...)
		if len(sources) == 0 && op != "constant" {
			sources = append(sources, dataTaskFieldRefsFromSpec(spec, "left_field", "right_field")...)
		}
		cleanSources := cleanDataTaskStrings(sources)
		if dataTaskDeriveOperationAllowsPartialSources(op) && len(cleanSources) > 0 {
			knownCount := 0
			for _, source := range cleanSources {
				if known[strings.ToLower(strings.TrimSpace(source))] != "" {
					knownCount++
				}
			}
			if knownCount > 0 {
				for _, target := range dataTaskFieldRefsFromSpec(spec, "target_field", "output_field", "derived_field", "name") {
					target = strings.TrimSpace(target)
					if target != "" {
						known[strings.ToLower(target)] = target
					}
				}
				continue
			}
		}
		for _, source := range cleanSources {
			key := strings.ToLower(strings.TrimSpace(source))
			if key == "" || known[key] != "" {
				continue
			}
			if !seenMissing[key] {
				missing = append(missing, source)
				seenMissing[key] = true
			}
		}
		for _, target := range dataTaskFieldRefsFromSpec(spec, "target_field", "output_field", "derived_field", "name") {
			target = strings.TrimSpace(target)
			if target != "" {
				known[strings.ToLower(target)] = target
			}
		}
	}
	sort.Strings(missing)
	return missing
}

func dataTaskMissingFieldRefsOnArtifact(access []dataTaskArtifactAccessPrompt, alias string, refs []string) []string {
	artifact, ok := dataTaskArtifactAccessByAlias(access, alias)
	if !ok || len(artifact.Fields) == 0 {
		return nil
	}
	known := dataTaskFieldSet(artifact.Fields)
	var missing []string
	seen := map[string]bool{}
	for _, ref := range cleanDataTaskStrings(refs) {
		key := strings.ToLower(strings.TrimSpace(ref))
		if key == "" || known[key] != "" || seen[key] {
			continue
		}
		missing = append(missing, ref)
		seen[key] = true
	}
	sort.Strings(missing)
	return missing
}

func dataTaskDeriveOperationAllowsPartialSources(op string) bool {
	normalized := strings.ToLower(strings.TrimSpace(op))
	normalized = strings.ReplaceAll(normalized, " ", "")
	switch normalized {
	case "concat", "join", "join_fields", "coalesce", "first_non_empty",
		"concat/join_fields", "join_fields/concat", "concat/join", "join/concat",
		"coalesce/first_non_empty", "first_non_empty/coalesce":
		return true
	default:
		return false
	}
}

func dataTaskFilterActionFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	for _, spec := range dataTaskActionObjectListParam(action.Params, "filters_json", "filters") {
		fields = append(fields, dataTaskFieldRefsFromSpec(spec, "field", "source_field", "input_field")...)
	}
	fields = append(fields, parseDataTaskActionStringListParam(action.Params["filter_field"])...)
	return cleanDataTaskStrings(fields)
}

func dataTaskQualifyActionFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	fields = append(fields, dataTaskFilterActionFieldRefs(action)...)
	for _, spec := range dataTaskActionObjectListParam(action.Params, "reject_filters_json", "exclude_filters_json", "block_filters_json", "reject_filters", "exclude_filters", "block_filters") {
		fields = append(fields, dataTaskFieldRefsFromSpec(spec, "field", "source_field", "input_field")...)
	}
	fields = append(fields, parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["required_fields"], action.Params["non_empty_fields"]))...)
	fields = append(fields, parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["evidence_fields"], action.Params["required_evidence_fields"]))...)
	fields = append(fields, parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["status_fields"], action.Params["generated_status_fields"]))...)
	return cleanDataTaskStrings(fields)
}

func dataTaskComputeActionFieldRefs(action dataquery.DataAction) []string {
	var fields []string
	fields = append(fields, parseDataTaskActionStringListParam(action.Params["value_field"])...)
	fields = append(fields, parseDataTaskActionStringListParam(action.Params["group_key_field"])...)
	fields = append(fields, dataTaskFilterActionFieldRefs(action)...)
	return cleanDataTaskStrings(fields)
}

func dataTaskJoinActionFieldContractGuardError(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) string {
	return dataTaskJoinActionFieldContractGuardResult(access, action, actionIndex).ErrorText()
}

func dataTaskJoinActionFieldContractGuardResult(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	inputs := cleanDataTaskStrings(action.InputPaths)
	leftPath := firstNonEmptyString(strings.TrimSpace(action.Params["left_path"]), strings.TrimSpace(action.Params["base_path"]))
	rightPath := firstNonEmptyString(strings.TrimSpace(action.Params["right_path"]), strings.TrimSpace(action.Params["lookup_path"]), strings.TrimSpace(action.Params["reference_path"]))
	if leftPath == "" && len(inputs) >= 1 {
		leftPath = inputs[0]
	}
	if rightPath == "" && len(inputs) >= 2 {
		rightPath = inputs[1]
	}
	leftFields := cleanDataTaskStrings(append(
		parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["left_fields"], action.Params["left_fields_json"], action.Params["base_fields"], action.Params["base_fields_json"])),
		parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["left_field"], action.Params["base_field"]))...,
	))
	rightFields := cleanDataTaskStrings(append(
		parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["right_fields"], action.Params["right_fields_json"], action.Params["lookup_fields"], action.Params["lookup_fields_json"], action.Params["reference_fields"], action.Params["reference_fields_json"])),
		parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["right_field"], action.Params["lookup_field"], action.Params["reference_field"]))...,
	))
	if leftPath != "" {
		if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, leftPath, dataTaskMissingFieldsOnArtifact(access, leftPath, leftFields), access); !guard.Empty() {
			return guard
		}
	}
	if rightPath != "" {
		if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, rightPath, dataTaskMissingFieldsOnArtifact(access, rightPath, rightFields), access); !guard.Empty() {
			return guard
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskNormalizeActionFieldContractGuardError(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) string {
	return dataTaskNormalizeActionFieldContractGuardResult(access, action, actionIndex).ErrorText()
}

func dataTaskNormalizeActionFieldContractGuardResult(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	if len(dataTaskActionObjectListParam(action.Params, "resolutions_json", "mappings_json", "entity_resolutions_json", "resolutions", "mappings", "entity_resolutions")) > 0 {
		return dataworkflow.GuardResult{}
	}
	sourceFields := cleanDataTaskStrings(append(
		parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["source_fields"], action.Params["source_fields_json"], action.Params["name_fields"], action.Params["name_fields_json"], action.Params["value_fields"], action.Params["value_fields_json"])),
		parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["source_field"], action.Params["name_field"], action.Params["value_field"], action.Params["field"]))...,
	))
	referenceFields := cleanDataTaskStrings(append(
		parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["reference_fields"], action.Params["reference_fields_json"], action.Params["reference_name_fields"], action.Params["reference_name_fields_json"], action.Params["mapping_source_fields"], action.Params["mapping_source_fields_json"], action.Params["lookup_fields"], action.Params["lookup_fields_json"])),
		parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["reference_field"], action.Params["reference_name_field"], action.Params["mapping_source_field"], action.Params["lookup_field"], action.Params["lookup_source_field"]))...,
	))
	filterFields := dataTaskFilterActionFieldRefs(action)
	canonicalIDField := firstNonEmptyString(
		strings.TrimSpace(action.Params["canonical_id_field"]),
		strings.TrimSpace(action.Params["id_field"]),
		strings.TrimSpace(action.Params["reference_value_field"]),
		strings.TrimSpace(action.Params["lookup_value_field"]),
	)
	canonicalLabelField := firstNonEmptyString(
		strings.TrimSpace(action.Params["canonical_label_field"]),
		strings.TrimSpace(action.Params["label_field"]),
	)
	explicitSourcePath := firstNonEmptyString(
		strings.TrimSpace(action.Params["source_path"]),
		strings.TrimSpace(action.Params["base_path"]),
		strings.TrimSpace(action.Params["record_path"]),
	)
	explicitReferencePath := firstNonEmptyString(
		strings.TrimSpace(action.Params["reference_path"]),
		strings.TrimSpace(action.Params["mapping_path"]),
		strings.TrimSpace(action.Params["lookup_path"]),
	)
	sourcePath := explicitSourcePath
	if sourcePath == "" {
		sourcePath = firstDataTaskActionInput(action, 0)
	}
	referencePath := explicitReferencePath
	if referencePath == "" {
		referencePath = firstDataTaskActionInput(action, 1)
	}
	if sourcePath == "" {
		return dataworkflow.GuardResult{}
	}
	if referencePath != "" && explicitSourcePath == "" && explicitReferencePath == "" && len(sourceFields) > 0 {
		sourceMissing := dataTaskMissingFieldsOnArtifact(access, sourcePath, sourceFields)
		referenceCanBeSource := len(dataTaskMissingFieldsOnArtifact(access, referencePath, sourceFields)) == 0
		if len(sourceMissing) > 0 && referenceCanBeSource {
			sourcePath, referencePath = referencePath, sourcePath
		}
	}
	sourceRequired := cleanDataTaskStrings(append(sourceFields, filterFields...))
	if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, sourcePath, dataTaskMissingFieldsOnArtifact(access, sourcePath, sourceRequired), access); !guard.Empty() {
		return guard
	}
	if referencePath == "" {
		return dataworkflow.GuardResult{}
	}
	referenceRequired := append([]string{}, referenceFields...)
	if strings.TrimSpace(canonicalIDField) != "" {
		referenceRequired = append(referenceRequired, canonicalIDField)
	}
	if strings.TrimSpace(canonicalLabelField) != "" {
		referenceRequired = append(referenceRequired, canonicalLabelField)
	}
	if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, referencePath, dataTaskMissingFieldsOnArtifact(access, referencePath, referenceRequired), access); !guard.Empty() {
		return guard
	}
	return dataworkflow.GuardResult{}
}

func dataTaskEnrichActionFieldContractGuardError(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) string {
	return dataTaskEnrichActionFieldContractGuardResult(access, action, actionIndex).ErrorText()
}

func dataTaskEnrichActionFieldContractGuardResult(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	basePath := firstNonEmptyString(strings.TrimSpace(action.Params["base_path"]), firstDataTaskActionInput(action, 0))
	if basePath == "" {
		return dataworkflow.GuardResult{}
	}
	for _, spec := range dataTaskActionObjectListParam(action.Params, "lookup_specs_json", "enrich_specs_json", "mapping_specs_json", "map_specs_json", "lookup_specs", "enrich_specs", "mapping_specs", "map_specs") {
		specBase := firstNonEmptyString(dataTaskMapStringValue(spec, "base_path"), basePath)
		baseFields := cleanDataTaskStrings(append(
			dataTaskFieldRefsFromSpec(spec, "base_fields", "source_fields", "left_fields"),
			dataTaskFieldRefsFromSpec(spec, "base_field", "source_field", "left_field")...,
		))
		if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, specBase, dataTaskMissingFieldsOnArtifact(access, specBase, baseFields), access); !guard.Empty() {
			return guard
		}
		lookupPath := firstNonEmptyString(
			dataTaskMapStringValue(spec, "lookup_path"),
			dataTaskMapStringValue(spec, "mapping_path"),
			dataTaskMapStringValue(spec, "reference_path"),
			firstDataTaskActionInput(action, 1),
		)
		lookupFields := cleanDataTaskStrings(append(
			dataTaskFieldRefsFromSpec(spec, "lookup_fields", "mapping_fields", "reference_fields"),
			dataTaskFieldRefsFromSpec(spec, "lookup_field", "mapping_field", "reference_field", "lookup_value_field", "mapping_value_field", "reference_value_field")...,
		))
		if lookupPath != "" {
			if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, lookupPath, dataTaskMissingFieldsOnArtifact(access, lookupPath, lookupFields), access); !guard.Empty() {
				return guard
			}
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskApplyResolutionActionFieldContractGuardError(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) string {
	return dataTaskApplyResolutionActionFieldContractGuardResult(access, action, actionIndex).ErrorText()
}

func dataTaskApplyResolutionActionFieldContractGuardResult(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	inputs := cleanDataTaskStrings(action.InputPaths)
	topBasePath := firstNonEmptyString(strings.TrimSpace(action.Params["base_path"]), strings.TrimSpace(action.Params["record_path"]), strings.TrimSpace(action.Params["input_path"]), strings.TrimSpace(action.Params["source_path"]))
	basePath := topBasePath
	specs := dataTaskActionObjectListParam(action.Params, "resolution_specs_json", "apply_specs_json", "resolution_specs", "apply_specs")
	if len(specs) == 0 {
		specs = []map[string]any{{}}
	}
	for i, spec := range specs {
		specBasePath := firstNonEmptyString(
			dataTaskMapStringValue(spec, "base_path"),
			dataTaskMapStringValue(spec, "record_path"),
			dataTaskMapStringValue(spec, "input_path"),
			dataTaskMapStringValue(spec, "source_path"),
		)
		if basePath == "" && specBasePath != "" {
			basePath = specBasePath
		}
		if basePath != "" && specBasePath != "" && normalizeDataTaskCoveragePath(basePath) != normalizeDataTaskCoveragePath(specBasePath) {
			message := fmt.Sprintf("data planning incomplete: action %d (%s) has conflicting apply_entity_resolutions base paths [%s] and [%s]. Apply resolutions to one base record artifact per action, or split separate base artifacts into separate actions.",
				actionIndex+1,
				firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
				basePath,
				specBasePath)
			return dataTaskActionInputContractGuardResult("apply_resolution_contract", action, basePath, []string{"base_path"}, message, nil)
		}
		if i == len(specs)-1 && basePath == "" {
			basePath = firstDataTaskActionInput(action, 0)
		}
	}
	if basePath == "" {
		return dataworkflow.GuardResult{}
	}
	inferredBasePath, inferredResolutionPath, roleErr := dataTaskInferApplyResolutionRolePaths(access, action, actionIndex, inputs, basePath)
	if roleErr != "" {
		return dataTaskActionInputContractGuardResult("apply_resolution_role_contract", action, basePath, nil, roleErr, nil)
	}
	if inferredBasePath != "" {
		basePath = inferredBasePath
	}
	for i, spec := range specs {
		specBasePath := firstNonEmptyString(
			dataTaskMapStringValue(spec, "base_path"),
			dataTaskMapStringValue(spec, "record_path"),
			dataTaskMapStringValue(spec, "input_path"),
			dataTaskMapStringValue(spec, "source_path"),
			basePath,
		)
		if inferredBasePath != "" && dataTaskApplyResolutionPathLooksLikeResolutionLedger(access, specBasePath) {
			specBasePath = inferredBasePath
		}
		baseFields := cleanDataTaskStrings(append(
			dataTaskFieldRefsFromSpec(spec, "base_key_fields", "source_key_fields"),
			dataTaskFieldRefsFromSpec(spec, "base_key_field", "source_key_field")...,
		))
		baseFields = cleanDataTaskStrings(append(baseFields,
			dataTaskFieldRefsFromSpec(spec, "source_fields", "source_field", "base_fields", "base_field", "record_fields", "record_field")...,
		))
		if len(baseFields) == 0 {
			baseFields = parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["base_key_fields"], action.Params["base_key_field"], action.Params["source_key_fields"], action.Params["source_key_field"]))
		}
		if missing := dataTaskMissingFieldsOnArtifactWithVirtual(access, specBasePath, baseFields, "_source_index", "source_index", "_source_line", "source_line", "line", "row_index", "row"); len(missing) > 0 {
			if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, specBasePath, missing, access); !guard.Empty() {
				return guard
			}
		}
		existingIDField := firstNonEmptyString(
			dataTaskMapStringValue(spec, "existing_id_field"),
			dataTaskMapStringValue(spec, "base_id_field"),
			dataTaskMapStringValue(spec, "current_id_field"),
			dataTaskMapStringValue(spec, "source_id_field"),
			dataTaskMapStringValue(spec, "existing_canonical_id_field"),
			strings.TrimSpace(action.Params["existing_id_field"]),
			strings.TrimSpace(action.Params["base_id_field"]),
			strings.TrimSpace(action.Params["current_id_field"]),
			strings.TrimSpace(action.Params["source_id_field"]),
			strings.TrimSpace(action.Params["existing_canonical_id_field"]),
		)
		if existingIDField != "" {
			if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, specBasePath, dataTaskMissingFieldsOnArtifact(access, specBasePath, []string{existingIDField}), access); !guard.Empty() {
				return guard
			}
			referencePath := firstNonEmptyString(
				dataTaskMapStringValue(spec, "reference_path"),
				dataTaskMapStringValue(spec, "verification_path"),
				dataTaskMapStringValue(spec, "canonical_reference_path"),
				strings.TrimSpace(action.Params["reference_path"]),
				strings.TrimSpace(action.Params["verification_path"]),
				strings.TrimSpace(action.Params["canonical_reference_path"]),
			)
			if referencePath == "" {
				message := fmt.Sprintf("data planning incomplete: action %d (%s) apply_entity_resolutions spec %d declares existing_id_field %q but no reference_path. Provide a reference/lookup artifact plus reference_id_field so the existing canonical value can be verified structurally, or omit existing_id_field.",
					actionIndex+1,
					firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
					i+1,
					existingIDField)
				return dataTaskActionInputContractGuardResult("apply_resolution_contract", action, specBasePath, []string{"reference_path"}, message, nil)
			}
			referenceIDField := firstNonEmptyString(dataTaskMapStringValue(spec, "reference_id_field"), dataTaskMapStringValue(spec, "reference_key_field"), dataTaskMapStringValue(spec, "lookup_id_field"), dataTaskMapStringValue(spec, "canonical_reference_id_field"), strings.TrimSpace(action.Params["reference_id_field"]), strings.TrimSpace(action.Params["reference_key_field"]), strings.TrimSpace(action.Params["lookup_id_field"]), strings.TrimSpace(action.Params["canonical_reference_id_field"]))
			referenceFields := cleanDataTaskStrings([]string{
				referenceIDField,
				firstNonEmptyString(dataTaskMapStringValue(spec, "reference_label_field"), dataTaskMapStringValue(spec, "reference_name_field"), dataTaskMapStringValue(spec, "lookup_label_field"), dataTaskMapStringValue(spec, "canonical_reference_label_field"), strings.TrimSpace(action.Params["reference_label_field"]), strings.TrimSpace(action.Params["reference_name_field"]), strings.TrimSpace(action.Params["lookup_label_field"]), strings.TrimSpace(action.Params["canonical_reference_label_field"])),
				firstNonEmptyString(dataTaskMapStringValue(spec, "reference_status_field"), dataTaskMapStringValue(spec, "lookup_status_field"), dataTaskMapStringValue(spec, "canonical_reference_status_field"), strings.TrimSpace(action.Params["reference_status_field"]), strings.TrimSpace(action.Params["lookup_status_field"]), strings.TrimSpace(action.Params["canonical_reference_status_field"])),
			})
			if referenceIDField == "" {
				message := fmt.Sprintf("data planning incomplete: action %d (%s) apply_entity_resolutions spec %d declares existing_id_field %q but no reference_id_field. Provide the reference key/code/id field used to verify existing values.",
					actionIndex+1,
					firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
					i+1,
					existingIDField)
				return dataTaskActionInputContractGuardResult("apply_resolution_contract", action, referencePath, []string{"reference_id_field"}, message, nil)
			}
			if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, referencePath, dataTaskMissingFieldsOnArtifact(access, referencePath, referenceFields), access); !guard.Empty() {
				return guard
			}
		}
		resolutionPath := firstNonEmptyString(
			dataTaskMapStringValue(spec, "resolution_path"),
			dataTaskMapStringValue(spec, "mapping_path"),
			dataTaskMapStringValue(spec, "lookup_path"),
			dataTaskMapStringValue(spec, "path"),
			strings.TrimSpace(action.Params["resolution_path"]),
			strings.TrimSpace(action.Params["mapping_path"]),
			strings.TrimSpace(action.Params["lookup_path"]),
			firstNonBaseDataTaskActionInput(inputs, specBasePath),
		)
		if inferredResolutionPath != "" && (resolutionPath == "" || normalizeDataTaskCoveragePath(resolutionPath) == normalizeDataTaskCoveragePath(inferredBasePath)) {
			resolutionPath = inferredResolutionPath
		}
		if resolutionPath == "" {
			continue
		}
		if resolutionArtifact, ok := dataTaskArtifactAccessByAlias(access, resolutionPath); ok && dataTaskArtifactIsDiagnosticChild(resolutionArtifact) {
			message := fmt.Sprintf("data planning incomplete: action %d (%s) apply_entity_resolutions uses diagnostic artifact %s as a resolution input. Diagnostic/source child artifacts can be inspected for schema evidence, but executable resolution application must use a mapping/entity-resolution ledger or an already materialized canonical field on the base record artifact.",
				actionIndex+1,
				firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
				resolutionPath)
			return dataTaskActionInputContractGuardResult("apply_resolution_diagnostic_input", action, resolutionPath, nil, message, []string{
				string(dataquery.DataActionInspectMaterial),
				string(dataquery.DataActionDeriveFields),
				string(dataquery.DataActionFilterRecords),
				string(dataquery.DataActionQualifyRecords),
				string(dataquery.DataActionComputeContribs),
			})
		}
		baseArtifact, baseOK := dataTaskArtifactAccessByAlias(access, basePath)
		if mapping, ok := dataTaskArtifactAccessByAlias(access, resolutionPath); ok && baseOK && !dataTaskApplyResolutionBaseCompatibleWithMapping(baseArtifact, mapping) {
			message := fmt.Sprintf("data planning incomplete: action %d (%s) applies resolution input %s to base artifact %s, but the resolution source lineage [%s] is not compatible with the base lineage [%s]. Apply entity resolutions only to the source record lineage they were produced from, or first materialize a compatible joined/enriched artifact.",
				actionIndex+1,
				firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
				resolutionPath,
				basePath,
				strings.Join(mapping.SourcePaths, ", "),
				dataTaskArtifactLineageSummary(basePath, baseArtifact))
			return dataTaskActionInputContractGuardResult("apply_resolution_lineage_contract", action, resolutionPath, nil, message, nil)
		}
		resolutionFields := cleanDataTaskStrings(append(
			dataTaskFieldRefsFromSpec(spec, "resolution_key_fields", "mapping_key_fields"),
			dataTaskFieldRefsFromSpec(spec, "resolution_key_field", "mapping_key_field")...,
		))
		if len(resolutionFields) == 0 {
			resolutionFields = parseDataTaskActionStringListParam(firstNonEmptyString(action.Params["resolution_key_fields"], action.Params["resolution_key_field"], action.Params["mapping_key_fields"], action.Params["mapping_key_field"]))
		}
		if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, resolutionPath, dataTaskMissingApplyResolutionFieldsOnArtifact(access, resolutionPath, resolutionFields), access); !guard.Empty() {
			return guard
		}
		canonicalIDField := firstNonEmptyString(dataTaskMapStringValue(spec, "canonical_id_field"), strings.TrimSpace(action.Params["canonical_id_field"]), "canonical_id")
		canonicalLabelField := firstNonEmptyString(dataTaskMapStringValue(spec, "canonical_label_field"), strings.TrimSpace(action.Params["canonical_label_field"]), "canonical_label")
		if !dataTaskArtifactHasAnyField(access, resolutionPath, canonicalIDField, canonicalLabelField) {
			artifact, ok := dataTaskArtifactAccessByAlias(access, resolutionPath)
			if !ok || len(artifact.Fields) == 0 {
				continue
			}
			if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, resolutionPath, cleanDataTaskStrings([]string{canonicalIDField, canonicalLabelField}), access); !guard.Empty() {
				return guard
			}
			return dataTaskActionGuardResultFromMessage("apply_resolution_canonical_field_contract", fmt.Sprintf("data planning incomplete: action %d (%s) apply_entity_resolutions spec %d needs a canonical value field on resolution input %s, but neither [%s] nor [%s] is present. Use fields from %s, or first materialize the needed canonical field with a typed action.",
				actionIndex+1,
				firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
				i+1,
				resolutionPath,
				canonicalIDField,
				canonicalLabelField,
				strings.Join(clampDataTaskStringSlice(artifact.Fields, 32), ", ")), action)
		}
		if msg := dataTaskApplyResolutionNoProgressGuardError(access, action, actionIndex, specBasePath, resolutionPath, spec, len(specs)); msg != "" {
			return dataTaskActionInputContractGuardResult("apply_resolution_no_progress", action, resolutionPath, nil, msg, nil)
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskApplyResolutionNoProgressGuardError(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int, basePath, resolutionPath string, spec map[string]any, specCount int) string {
	base, ok := dataTaskArtifactAccessByAlias(access, basePath)
	if !ok || len(base.Fields) == 0 || strings.TrimSpace(resolutionPath) == "" {
		return ""
	}
	targetFields := dataTaskApplyResolutionTargetFields(action, spec, specCount)
	if len(targetFields) == 0 {
		return ""
	}
	if !dataTaskArtifactHasAnyNamedField(base, targetFields...) {
		return ""
	}
	if !dataTaskArtifactLineageContains(base, resolutionPath) {
		return ""
	}
	return fmt.Sprintf("data planning incomplete: action %d (%s) repeats apply_entity_resolutions for resolution input %s on base artifact %s; base already carries target field(s) [%s] from that resolution lineage. This graph edge is idempotent and would not create contribution/reconcile progress. Use the existing resolved artifact for derive_fields, filter_records, qualify_records, join_records, compute_contributions, or inspect a concrete field gap instead.",
		actionIndex+1,
		firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
		resolutionPath,
		basePath,
		strings.Join(targetFields, ", "))
}

func dataTaskApplyResolutionTargetFields(action dataquery.DataAction, spec map[string]any, specCount int) []string {
	targetID := firstNonEmptyString(
		dataTaskMapStringValue(spec, "target_id_field"),
		dataTaskMapStringValue(spec, "target_field"),
		dataTaskMapStringValue(spec, "canonical_target_field"),
		strings.TrimSpace(action.Params["target_id_field"]),
		strings.TrimSpace(action.Params["target_field"]),
		strings.TrimSpace(action.Params["canonical_target_field"]),
	)
	if targetID == "" && specCount <= 1 {
		targetID = "canonical_id"
	}
	targetLabel := firstNonEmptyString(
		dataTaskMapStringValue(spec, "target_label_field"),
		dataTaskMapStringValue(spec, "label_target_field"),
		strings.TrimSpace(action.Params["target_label_field"]),
		strings.TrimSpace(action.Params["label_target_field"]),
	)
	targetStatus := firstNonEmptyString(
		dataTaskMapStringValue(spec, "target_status_field"),
		dataTaskMapStringValue(spec, "status_target_field"),
		strings.TrimSpace(action.Params["target_status_field"]),
		strings.TrimSpace(action.Params["status_target_field"]),
	)
	if targetStatus == "" && targetID != "" {
		targetStatus = targetID + "_status"
	}
	return cleanDataTaskStrings([]string{targetID, targetLabel, targetStatus})
}

func dataTaskArtifactHasAnyNamedField(artifact dataTaskArtifactAccessPrompt, fields ...string) bool {
	set := dataTaskFieldSet(artifact.Fields)
	for _, field := range fields {
		if set[strings.ToLower(strings.TrimSpace(field))] != "" {
			return true
		}
	}
	return false
}

func dataTaskArtifactLineageContains(artifact dataTaskArtifactAccessPrompt, alias string) bool {
	want := normalizeDataTaskCoveragePath(alias)
	if want == "" {
		return false
	}
	if normalizeDataTaskCoveragePath(artifact.ID) == want {
		return true
	}
	for _, source := range artifact.SourcePaths {
		if normalizeDataTaskCoveragePath(source) == want {
			return true
		}
	}
	for _, candidate := range artifact.Aliases {
		if normalizeDataTaskCoveragePath(candidate) == want {
			return true
		}
	}
	return false
}

func dataTaskApplyResolutionBaseCompatibleWithMapping(base dataTaskArtifactAccessPrompt, mapping dataTaskArtifactAccessPrompt) bool {
	sourcePaths := cleanDataTaskStrings(mapping.SourcePaths)
	if len(sourcePaths) == 0 {
		return true
	}
	for _, source := range sourcePaths {
		if dataTaskArtifactLineageContains(base, source) {
			return true
		}
	}
	return false
}

func dataTaskArtifactLineageSummary(alias string, artifact dataTaskArtifactAccessPrompt) string {
	values := cleanDataTaskStrings(append(append([]string{alias, artifact.ID}, artifact.Aliases...), artifact.SourcePaths...))
	return strings.Join(clampDataTaskStringSlice(values, 8), ", ")
}

func dataTaskInferApplyResolutionRolePaths(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int, inputs []string, basePath string) (string, string, string) {
	if !dataTaskApplyResolutionPathLooksLikeMapping(access, basePath) {
		return "", "", ""
	}
	var candidates []string
	for _, input := range cleanDataTaskStrings(inputs) {
		if normalizeDataTaskCoveragePath(input) == "" || normalizeDataTaskCoveragePath(input) == normalizeDataTaskCoveragePath(basePath) {
			continue
		}
		artifact, ok := dataTaskArtifactAccessByAlias(access, input)
		if !ok || len(artifact.Fields) == 0 || dataTaskArtifactLooksLikeEntityResolutionLedger(artifact) {
			continue
		}
		candidates = append(candidates, input)
	}
	actionLabel := firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind)))
	switch len(candidates) {
	case 0:
		return "", "", fmt.Sprintf("data planning incomplete: action %d (%s) apply_entity_resolutions selected base input %s, but that artifact looks like an entity_resolution ledger. Provide input_paths=[base_record_artifact, entity_resolution_artifact...] or set base_path to the base records.",
			actionIndex+1, actionLabel, basePath)
	case 1:
		return candidates[0], basePath, ""
	default:
		return "", "", fmt.Sprintf("data planning incomplete: action %d (%s) apply_entity_resolutions selected base input %s, but that artifact looks like an entity_resolution ledger and multiple possible base record artifacts were found [%s]. Set base_path explicitly or split the action.",
			actionIndex+1, actionLabel, basePath, strings.Join(candidates, ", "))
	}
}

func dataTaskApplyResolutionPathLooksLikeMapping(access []dataTaskArtifactAccessPrompt, path string) bool {
	return dataTaskApplyResolutionPathLooksLikeResolutionLedger(access, path)
}

func dataTaskApplyResolutionPathLooksLikeResolutionLedger(access []dataTaskArtifactAccessPrompt, path string) bool {
	artifact, ok := dataTaskArtifactAccessByAlias(access, path)
	return ok && dataTaskArtifactLooksLikeEntityResolutionLedger(artifact)
}

func firstNonBaseDataTaskActionInput(inputs []string, basePath string) string {
	base := normalizeDataTaskCoveragePath(basePath)
	for _, input := range cleanDataTaskStrings(inputs) {
		if normalizeDataTaskCoveragePath(input) == "" || normalizeDataTaskCoveragePath(input) == base {
			continue
		}
		return input
	}
	return ""
}

func dataTaskMissingFieldsOnArtifactWithVirtual(access []dataTaskArtifactAccessPrompt, alias string, fields []string, virtualFields ...string) []string {
	missing := dataTaskMissingFieldsOnArtifact(access, alias, fields)
	if len(missing) == 0 {
		return nil
	}
	virtual := map[string]bool{}
	for _, field := range virtualFields {
		field = strings.ToLower(strings.TrimSpace(field))
		if field != "" {
			virtual[field] = true
		}
	}
	var out []string
	for _, field := range missing {
		if virtual[strings.ToLower(strings.TrimSpace(field))] {
			continue
		}
		out = append(out, field)
	}
	return out
}

func dataTaskMissingApplyResolutionFieldsOnArtifact(access []dataTaskArtifactAccessPrompt, alias string, fields []string) []string {
	missing := dataTaskMissingFieldsOnArtifact(access, alias, fields)
	if len(missing) == 0 {
		return nil
	}
	artifact, ok := dataTaskArtifactAccessByAlias(access, alias)
	if !ok || len(artifact.Fields) == 0 {
		return missing
	}
	set := dataTaskFieldSet(artifact.Fields)
	var out []string
	for _, field := range missing {
		if dataTaskResolutionLocatorEquivalentField(set, field) != "" {
			continue
		}
		out = append(out, field)
	}
	return out
}

func dataTaskResolutionLocatorEquivalentField(set map[string]string, requested string) string {
	switch strings.ToLower(strings.TrimSpace(requested)) {
	case "item_id", "source_locator", "_source_locator", "record_id", "row_id", "id",
		"_source_index", "source_index", "_source_line", "source_line", "line", "row_index", "row":
	default:
		return ""
	}
	for _, candidate := range []string{"item_id", "source_locator", "_source_locator", "record_id", "row_id", "id"} {
		if actual := set[strings.ToLower(candidate)]; actual != "" {
			return actual
		}
	}
	return ""
}

func dataTaskArtifactHasAnyField(access []dataTaskArtifactAccessPrompt, alias string, fields ...string) bool {
	artifact, ok := dataTaskArtifactAccessByAlias(access, alias)
	if !ok || len(artifact.Fields) == 0 {
		return true
	}
	set := dataTaskFieldSet(artifact.Fields)
	for _, field := range fields {
		field = strings.ToLower(strings.TrimSpace(field))
		if field != "" && set[field] != "" {
			return true
		}
	}
	return false
}

func firstDataTaskActionInput(action dataquery.DataAction, index int) string {
	inputs := cleanDataTaskStrings(action.InputPaths)
	if index < 0 || index >= len(inputs) {
		return ""
	}
	return inputs[index]
}

func dataTaskActionObjectListParam(params map[string]string, keys ...string) []map[string]any {
	for _, key := range keys {
		raw := strings.TrimSpace(params[key])
		if raw == "" {
			continue
		}
		if objects := dataTaskParseObjectList(raw); len(objects) > 0 {
			return objects
		}
	}
	return nil
}

func dataTaskParseObjectList(raw string) []map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(raw), &list); err == nil {
		return list
	}
	var one map[string]any
	if err := json.Unmarshal([]byte(raw), &one); err == nil {
		return []map[string]any{one}
	}
	return nil
}

func dataTaskFieldRefsFromSpec(spec map[string]any, keys ...string) []string {
	var out []string
	for _, key := range keys {
		value, ok := spec[key]
		if !ok {
			continue
		}
		out = append(out, dataTaskAnyStringList(value)...)
	}
	return cleanDataTaskStrings(out)
}

func dataTaskMapStringValue(spec map[string]any, key string) string {
	value, ok := spec[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.12f", typed), "0"), ".")
	case bool:
		return fmt.Sprintf("%t", typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func dataTaskAnyStringList(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return parseDataTaskActionStringListParam(typed)
	case []string:
		return cleanDataTaskStrings(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, dataTaskAnyStringList(item)...)
		}
		return cleanDataTaskStrings(out)
	default:
		return cleanDataTaskStrings([]string{fmt.Sprint(typed)})
	}
}

func dataTaskDeriveFieldsActionHasSpec(action dataquery.DataAction) bool {
	if len(action.Params) == 0 {
		return false
	}
	for _, key := range []string{
		"field_specs_json", "extract_specs_json", "derive_specs_json", "transforms_json",
		"field_specs", "extract_specs", "derive_specs", "transforms",
		"source_field", "input_field", "field",
		"target_field", "output_field", "derived_field",
		"operation", "op", "transform",
	} {
		if strings.TrimSpace(action.Params[key]) != "" {
			return true
		}
	}
	return false
}

func dataTaskExpandRecordsActionHasSpec(action dataquery.DataAction) bool {
	if len(action.Params) == 0 {
		return false
	}
	for _, key := range []string{"source_field", "input_field", "field", "value_field"} {
		if strings.TrimSpace(action.Params[key]) != "" {
			return true
		}
	}
	return false
}

func dataTaskGroupRecordsActionHasSpec(action dataquery.DataAction) bool {
	if len(action.Params) == 0 {
		return false
	}
	hasGroup := parseBoolDataTaskActionParam(action.Params["group_all"])
	if !hasGroup {
		for _, key := range []string{"group_fields", "group_by_fields", "key_fields", "group_field", "group_by", "key_field"} {
			if strings.TrimSpace(action.Params[key]) != "" {
				hasGroup = true
				break
			}
		}
	}
	if !hasGroup {
		return false
	}
	for _, key := range []string{"text_fields", "concat_fields", "aggregate_fields", "source_fields", "text_field", "source_field", "field"} {
		if strings.TrimSpace(action.Params[key]) != "" {
			return true
		}
	}
	return true
}

func parseBoolDataTaskActionParam(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func dataTaskFilterRecordsActionHasSpec(action dataquery.DataAction) bool {
	if len(action.Params) == 0 {
		return false
	}
	for _, key := range []string{"filters_json", "filters", "filter_field"} {
		if strings.TrimSpace(action.Params[key]) != "" {
			return true
		}
	}
	return false
}

func dataTaskCoverageLoopGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	state := dataTaskWorkflowState(records, plan)
	if !state.MaterialCoverageSufficient || !dataTaskPlanIsCoverageOnly(plan) {
		return ""
	}
	if dataTaskPlanIsGeneratedArtifactDiagnostic(records, plan) {
		return ""
	}
	if dataTaskPlanActionsAllAllowedForWorkflowStage(plan, state) {
		return ""
	}
	return fmt.Sprintf("data planning incomplete: material coverage is already sufficient for required runner materials (%d covered, missing=%d). Do not emit another coverage-only batch (material_inventory/inspect_material/derive_rules or extract_records without output_artifact) unless a specific new missing material is listed. extract_records with output_artifact may materialize already-covered sources into reusable record artifacts. Continue toward the user's data goal with compute-stage atomic actions such as derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, or assemble_answer. If you need schema diagnostics, inspect a generated artifact alias from artifact_access instead of re-covering original materials. workflow_next_stage=%s",
		state.RequiredMaterialCount-state.MissingRequiredMaterialCount, state.MissingRequiredMaterialCount, state.NextStage)
}

func dataTaskPlanIsGeneratedArtifactDiagnostic(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) bool {
	if len(plan.Actions) == 0 || strings.TrimSpace(plan.Script) != "" {
		return false
	}
	for _, action := range plan.Actions {
		if !dataTaskActionIsGeneratedArtifactDiagnostic(records, action) {
			return false
		}
	}
	return true
}

func dataTaskRepeatedCustomTransformGuardError(records []dataTaskWorkflowRecord, action dataquery.DataAction) string {
	if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform {
		return ""
	}
	actionID := strings.TrimSpace(action.ID)
	if actionID == "" {
		return ""
	}
	key := actionID + "|" + string(dataquery.DataActionCustomTransform)
	count := 0
	for _, rec := range records {
		if strings.TrimSpace(rec.Err) == "" {
			continue
		}
		if dataTaskViolationNodeKey(dataquery.ClassifyExecutionError(rec.Err)) == key {
			count++
		}
	}
	if count < DefaultDataTaskMaxNodeFailures {
		return ""
	}
	return fmt.Sprintf("data planning incomplete: custom_transform node %q already failed %d time(s). Do not retry the same free-form script node; replace it with smaller typed atomic actions such as extract_records, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, and reconcile_artifacts, or emit a new narrow custom_transform over one known artifact.",
		actionID, count)
}

func dataTaskRepeatedCustomTransformClassGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform || strings.TrimSpace(action.Script) == "" {
		return ""
	}
	if !dataTaskActionHasBroadPrerequisiteSurface(plan, action) && !dataTaskActionLooksLikeWholeWorkflow(plan, action, actionIndex) {
		return ""
	}
	total, topCode, topCodeCount := dataTaskCustomTransformFailureClassStats(records)
	if total < DefaultDataTaskMaxCustomTransformClassFailures && topCodeCount < DefaultDataTaskMaxNodeFailures {
		return ""
	}
	codeHint := ""
	if topCode != "" {
		codeHint = fmt.Sprintf(" most_common_failure=%s count=%d.", topCode, topCodeCount)
	}
	return fmt.Sprintf("data planning incomplete: workflow already has %d custom_transform failure(s).%s Do not bypass this by changing action_id; avoid another broad free-form script. Continue with typed atomic actions that produce reusable artifacts and consume fields from artifact_access.",
		total, codeHint)
}

func dataTaskCustomTransformFailureClassStats(records []dataTaskWorkflowRecord) (total int, topCode string, topCodeCount int) {
	counts := map[string]int{}
	for _, rec := range records {
		if strings.TrimSpace(rec.Err) == "" || !dataTaskRecordHasCustomTransformScript(rec) {
			continue
		}
		total++
		v := dataquery.ClassifyExecutionError(rec.Err)
		code := strings.TrimSpace(v.Code)
		if code == "" {
			code = "unknown_custom_transform_failure"
		}
		counts[code]++
		if counts[code] > topCodeCount {
			topCode = code
			topCodeCount = counts[code]
		}
	}
	return total, topCode, topCodeCount
}

func dataTaskRecordHasCustomTransformScript(rec dataTaskWorkflowRecord) bool {
	if strings.TrimSpace(rec.Plan.Script) != "" {
		return true
	}
	return dataTaskPlanHasCustomScript(rec.Plan.Actions, len(rec.Plan.Actions))
}

func dataTaskWorkflowHasContributionProducer(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, beforeActionIndex int) bool {
	for _, rec := range records {
		if rec.Result != nil && len(rec.Result.Contributions) > 0 {
			return true
		}
	}
	if beforeActionIndex > len(plan.Actions) {
		beforeActionIndex = len(plan.Actions)
	}
	for i := 0; i < beforeActionIndex; i++ {
		if normalizeDataActionKindForWorkflow(plan.Actions[i].Kind) == dataquery.DataActionComputeContribs {
			return true
		}
	}
	return false
}

func dataTaskWorkflowHasReconcileProducer(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, beforeActionIndex int) bool {
	for _, rec := range records {
		if rec.Result != nil && rec.Result.Reconcile != nil {
			return true
		}
	}
	if beforeActionIndex > len(plan.Actions) {
		beforeActionIndex = len(plan.Actions)
	}
	for i := 0; i < beforeActionIndex; i++ {
		if normalizeDataActionKindForWorkflow(plan.Actions[i].Kind) == dataquery.DataActionReconcile {
			return true
		}
	}
	return false
}

func dataTaskCustomScriptActionCount(actions []dataquery.DataAction) int {
	count := 0
	for _, action := range actions {
		if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		if strings.TrimSpace(action.Script) == "" {
			continue
		}
		count++
	}
	return count
}

func normalizeDataActionKindForWorkflow(kind dataquery.DataActionKind) dataquery.DataActionKind {
	return dataworkflow.NormalizeActionKind(kind)
}

func dataTaskActionLooksLikeWholeWorkflow(plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) bool {
	if dataTaskPlanHasTypedActionContext(plan.Actions, actionIndex) &&
		dataTaskScriptLineCount(action.Script) < dataTaskOneShotScriptLineSoftLimit &&
		len(cleanDataTaskStrings(action.InputPaths)) <= 1 &&
		len(plan.CoverageContract.RequiredMaterials) <= 1 &&
		dataTaskValidationLedgerCount(plan.CoverageContract) <= 1 {
		return false
	}
	return len(action.InputPaths) >= 4 ||
		len(plan.CoverageContract.RequiredMaterials) >= 4 ||
		dataTaskValidationLedgerCount(plan.CoverageContract) >= 2
}

func dataTaskActionHasBroadPrerequisiteSurface(plan dataquery.TaskPlan, action dataquery.DataAction) bool {
	inputs := cleanDataTaskStrings(action.InputPaths)
	lines := dataTaskScriptLineCount(action.Script)
	return len(inputs) >= 4 ||
		(lines >= dataTaskComplexCustomScriptLineLimit && dataTaskValidationLedgerCount(plan.CoverageContract) >= 2)
}

func dataTaskTerminalRawMaterialCustomTransformGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex, lines int) string {
	if len(records) == 0 || plan.ContinueAfter {
		return ""
	}
	if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform || strings.TrimSpace(action.Script) == "" {
		return ""
	}
	state := dataTaskWorkflowState(records, plan)
	if state.NextStage != "emit_output_contract_answer" {
		return ""
	}
	rawInputs := dataTaskCustomTransformRawMaterialInputs(records, action)
	if len(rawInputs) == 0 {
		return ""
	}
	if lines < dataTaskComplexCustomScriptLineLimit && len(rawInputs) <= 1 {
		return ""
	}
	return fmt.Sprintf("data planning incomplete: terminal custom_transform action %d (%s) reads %d original material(s) after prior workflow progress: %s. At the final answer stage, custom_transform may only project or lightly format generated artifacts such as contribution, reconcile, or assembled-answer aliases. Do not recompute material cleaning/joining/aggregation in one script; continue with typed actions (derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, assemble_answer) or a narrow custom_transform over generated artifact aliases only.",
		actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))), len(rawInputs), strings.Join(rawInputs, ", "))
}

func dataTaskCustomTransformRawMaterialInputs(records []dataTaskWorkflowRecord, action dataquery.DataAction) []string {
	generated := dataTaskWorkflowGeneratedArtifactPathSet(records)
	var raw []string
	for _, input := range cleanDataTaskStrings(action.InputPaths) {
		normalized := normalizeDataTaskCoveragePath(input)
		if normalized == "" || generated[normalized] {
			continue
		}
		raw = append(raw, input)
	}
	sort.Strings(raw)
	return raw
}

func dataTaskBroadCustomPrerequisiteGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	if len(plan.Actions) == 0 {
		return ""
	}
	for i, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		if strings.TrimSpace(action.Script) == "" {
			continue
		}
		if !dataTaskActionHasBroadPrerequisiteSurface(plan, action) {
			continue
		}
		missing := dataTaskMissingCustomTransformPrerequisites(records, plan, action, i)
		if len(missing) == 0 {
			continue
		}
		return fmt.Sprintf("data planning incomplete: broad custom_transform action %d (%s) reads or depends on %d material(s) that were not covered by prior typed actions/results: %s. First add smaller atomic actions such as inspect_material, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, or extract_records for the missing inputs, then use custom_transform only as a bounded transform over known materials.",
			i+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))), len(missing), strings.Join(missing, ", "))
	}
	return ""
}

func dataTaskMissingCustomTransformPrerequisites(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) []string {
	required := dataTaskCustomTransformPrerequisitePaths(plan, action)
	if len(required) == 0 {
		return nil
	}
	covered := dataTaskWorkflowCoveredMaterialPaths(records, plan, actionIndex)
	var missing []string
	for _, p := range required {
		if p == "" || dataTaskCoveragePathCovered(covered, p) {
			continue
		}
		missing = append(missing, p)
	}
	sort.Strings(missing)
	return missing
}

func dataTaskCustomTransformPrerequisitePaths(plan dataquery.TaskPlan, action dataquery.DataAction) []string {
	paths := append([]string(nil), action.InputPaths...)
	return cleanDataTaskStrings(paths)
}

func dataTaskWorkflowCoveredMaterialPaths(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, beforeActionIndex int) map[string]bool {
	covered := map[string]bool{}
	mark := func(values []string) {
		for _, value := range cleanDataTaskStrings(values) {
			normalized := normalizeDataTaskCoveragePath(value)
			if normalized != "" {
				covered[normalized] = true
			}
		}
	}
	var markArtifact func(dataquery.DataArtifact)
	markArtifact = func(artifact dataquery.DataArtifact) {
		mark(artifact.SourcePaths)
		mark(dataTaskArtifactAliasPaths(artifact))
		for _, child := range artifact.Children {
			markArtifact(child)
		}
	}
	for _, rec := range records {
		if rec.Result != nil {
			mark(rec.Result.ConsumedPaths)
			for _, artifact := range rec.Result.Artifacts {
				markArtifact(artifact)
			}
		}
	}
	mark(plan.InputPaths)
	if beforeActionIndex > len(plan.Actions) {
		beforeActionIndex = len(plan.Actions)
	}
	for i := 0; i < beforeActionIndex; i++ {
		action := plan.Actions[i]
		switch normalizeDataActionKindForWorkflow(action.Kind) {
		case dataquery.DataActionCustomTransform, dataquery.DataActionMaterialInventory:
			continue
		default:
			mark(action.InputPaths)
			mark(dataTaskActionOutputAliases(action))
		}
	}
	return covered
}

func dataTaskWorkflowAvailableActionInputPaths(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, beforeActionIndex int) map[string]bool {
	available := map[string]bool{}
	mark := func(values []string) {
		for _, value := range cleanDataTaskStrings(values) {
			normalized := normalizeDataTaskCoveragePath(value)
			if normalized != "" {
				available[normalized] = true
			}
		}
	}
	var markArtifact func(dataquery.DataArtifact)
	markArtifact = func(artifact dataquery.DataArtifact) {
		mark(artifact.SourcePaths)
		mark(dataTaskArtifactAliasPaths(artifact))
		for _, child := range artifact.Children {
			markArtifact(child)
		}
	}
	for _, rec := range records {
		if rec.Result == nil {
			continue
		}
		mark(rec.Result.ConsumedPaths)
		for _, artifact := range rec.Result.Artifacts {
			markArtifact(artifact)
		}
	}
	mark(dataTaskTopLevelExternalMaterialInputs(plan.InputPaths))
	if beforeActionIndex > len(plan.Actions) {
		beforeActionIndex = len(plan.Actions)
	}
	for i := 0; i < beforeActionIndex; i++ {
		action := plan.Actions[i]
		switch normalizeDataActionKindForWorkflow(action.Kind) {
		case dataquery.DataActionCustomTransform, dataquery.DataActionMaterialInventory:
			continue
		default:
			mark(action.InputPaths)
			mark(dataTaskActionOutputAliases(action))
		}
	}
	return available
}

func dataTaskTopLevelExternalMaterialInputs(values []string) []string {
	var out []string
	for _, value := range cleanDataTaskStrings(values) {
		if dataTaskPathLooksLikeExternalMaterial(value) {
			out = append(out, value)
		}
	}
	return out
}

func dataTaskPathLooksLikeExternalMaterial(value string) bool {
	value = strings.TrimSpace(filepath.ToSlash(value))
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") {
		return true
	}
	if strings.Contains(value, "/") {
		return true
	}
	ext := strings.TrimSpace(path.Ext(value))
	return ext != ""
}

func dataTaskCoveragePathCovered(covered map[string]bool, required string) bool {
	required = normalizeDataTaskCoveragePath(required)
	if required == "" {
		return true
	}
	if covered[required] {
		return true
	}
	if path.Ext(required) != "" {
		return false
	}
	prefix := strings.TrimSuffix(required, "/") + "/"
	for value := range covered {
		value = normalizeDataTaskCoveragePath(value)
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func dataTaskActionOutputAliases(action dataquery.DataAction) []string {
	var out []string
	if id := strings.TrimSpace(action.ID); id != "" {
		out = append(out, id)
	}
	if artifact := strings.TrimSpace(action.OutputArtifact); artifact != "" {
		out = append(out, artifact, path.Base(artifact))
	}
	return cleanDataTaskStrings(out)
}

func dataTaskPlanHasTypedActionContext(actions []dataquery.DataAction, beforeIndex int) bool {
	return dataTaskPlanNonCustomActionCount(actions, beforeIndex) >= 2
}

func dataTaskPlanNonCustomActionCount(actions []dataquery.DataAction, beforeIndex int) int {
	if beforeIndex > len(actions) {
		beforeIndex = len(actions)
	}
	count := 0
	for i := 0; i < beforeIndex; i++ {
		if normalizeDataActionKindForWorkflow(actions[i].Kind) == dataquery.DataActionCustomTransform {
			continue
		}
		count++
	}
	return count
}

func dataTaskPlanHasCustomScript(actions []dataquery.DataAction, beforeIndex int) bool {
	if beforeIndex > len(actions) {
		beforeIndex = len(actions)
	}
	for i := 0; i < beforeIndex; i++ {
		if normalizeDataActionKindForWorkflow(actions[i].Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		if strings.TrimSpace(actions[i].Script) != "" {
			return true
		}
	}
	return false
}

func dataTaskScriptHasResultEmitter(script string) bool {
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(trimmed, "emit_result(") || strings.Contains(trimmed, "emit(") {
			return true
		}
		if strings.HasPrefix(trimmed, "result =") || strings.HasPrefix(trimmed, "result=") {
			return true
		}
	}
	return false
}

func dataTaskValidationLedgerCount(contract dataquery.CoverageContract) int {
	n := 0
	if contract.DecisionRecordsRequired {
		n++
	}
	if contract.RuleCoverageRequired {
		n++
	}
	if contract.ContributionLedgerRequired {
		n++
	}
	if contract.EntityResolutionRequired {
		n++
	}
	if contract.ReconcileRequired {
		n++
	}
	return n
}

type dataTaskWorkflowRecord struct {
	Plan       dataquery.TaskPlan
	Result     *dataquery.Result
	Err        string
	Evaluation *dataquery.Evaluation
}

type dataTaskWorkflowPlanPreflight struct {
	Plan          dataquery.TaskPlan
	Original      dataquery.TaskPlan
	Remainder     dataquery.TaskPlan
	GuardErr      string
	FinalGuardErr string
	Reason        string
	Rewritten     bool
}

const dataTaskPreflightMaxRewrites = 3

func dataTaskPreflightWorkflowPlan(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, protect func(dataquery.TaskPlan) dataquery.TaskPlan) dataTaskWorkflowPlanPreflight {
	if protect == nil {
		protect = func(p dataquery.TaskPlan) dataquery.TaskPlan { return p }
	}
	protected := protect(plan)
	out := dataTaskWorkflowPlanPreflight{Plan: protected, Original: protected}
	current := protected
	for i := 0; i < dataTaskPreflightMaxRewrites; i++ {
		if fallback, remainder, ok := dataTaskInitialRankPrefixFallback(records, current); ok {
			out.Remainder = dataTaskPreflightMergeRemainder(remainder, out.Remainder)
			out.Reason = appendDataTaskPreflightReason(out.Reason, "split initial data plan at typed dependency rank")
			out.Rewritten = true
			current = protect(fallback)
			out.Plan = current
			continue
		}
		errText := dataTaskWorkflowStagingGuardError(records, current)
		if strings.TrimSpace(errText) == "" {
			out.Plan = current
			return out
		}
		fallback, remainder, reason, ok := dataTaskWorkflowDeterministicFallback(records, current, errText)
		if !ok {
			out.Plan = current
			out.GuardErr = errText
			out.FinalGuardErr = errText
			return out
		}
		out.Remainder = dataTaskPreflightMergeRemainder(remainder, out.Remainder)
		out.GuardErr = errText
		out.Reason = appendDataTaskPreflightReason(out.Reason, reason)
		out.Rewritten = true
		current = protect(fallback)
		out.Plan = current
	}
	if errText := dataTaskWorkflowStagingGuardError(records, current); strings.TrimSpace(errText) != "" {
		out.Plan = current
		out.GuardErr = errText
		out.FinalGuardErr = errText
		return out
	}
	out.Plan = current
	return out
}

func appendDataTaskPreflightReason(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" {
		return next
	}
	if next == "" || strings.Contains(current, next) {
		return current
	}
	return current + "; " + next
}

func dataTaskPreflightMergeRemainder(first, second dataquery.TaskPlan) dataquery.TaskPlan {
	if len(first.Actions) == 0 {
		return second
	}
	if len(second.Actions) == 0 {
		return first
	}
	out := first
	out.Actions = append(append([]dataquery.DataAction(nil), first.Actions...), second.Actions...)
	out.Script = ""
	out.ContinueAfter = true
	out.InputPaths = mergeDataTaskInputPaths(first.InputPaths, second.InputPaths)
	if strings.TrimSpace(out.NextBatch) == "" {
		out.NextBatch = strings.TrimSpace(second.NextBatch)
	}
	if strings.TrimSpace(out.WhyThisBatch) == "" {
		out.WhyThisBatch = strings.TrimSpace(second.WhyThisBatch)
	}
	return out
}

type dataTaskWorkflowPromptRecord struct {
	Round               int                       `json:"round"`
	PlanStatus          string                    `json:"plan_status,omitempty"`
	Goal                string                    `json:"goal,omitempty"`
	Actions             []dataquery.DataAction    `json:"actions,omitempty"`
	InputPaths          []string                  `json:"input_paths,omitempty"`
	OutputContract      dataquery.OutputContract  `json:"output_contract,omitempty"`
	SuccessCriteria     []string                  `json:"success_criteria,omitempty"`
	MissingObservations []string                  `json:"missing_observations,omitempty"`
	NextBatch           string                    `json:"next_batch,omitempty"`
	WhyThisBatch        string                    `json:"why_this_batch,omitempty"`
	ContinueAfter       bool                      `json:"continue_after,omitempty"`
	ScriptPreview       string                    `json:"script_preview,omitempty"`
	Result              *dataTaskResultPromptView `json:"result,omitempty"`
	Error               string                    `json:"error,omitempty"`
	Evaluation          *dataquery.Evaluation     `json:"evaluation,omitempty"`
}

type dataTaskWorkflowStateView struct {
	MaterialCoverageSufficient    bool                               `json:"material_coverage_sufficient"`
	MaterialCoverageAuthoritative bool                               `json:"material_coverage_authoritative"`
	WorkflowContract              dataworkflow.CoverageContractView  `json:"workflow_contract,omitempty"`
	CurrentBatchContract          dataworkflow.CoverageContractView  `json:"current_batch_contract,omitempty"`
	ActionGraph                   dataworkflow.ActionGraph           `json:"action_graph,omitempty"`
	RequiredMaterialCount         int                                `json:"required_material_count"`
	RequiredMaterials             []string                           `json:"required_materials,omitempty"`
	CoveredRequiredMaterials      []string                           `json:"covered_required_materials,omitempty"`
	MissingRequiredMaterialCount  int                                `json:"missing_required_material_count"`
	MissingRequiredMaterials      []string                           `json:"missing_required_materials,omitempty"`
	CoverageNote                  string                             `json:"coverage_note,omitempty"`
	OutputContract                dataquery.OutputContract           `json:"output_contract,omitempty"`
	RuleCoverageRequired          bool                               `json:"rule_coverage_required,omitempty"`
	RuleCoverageRecords           int                                `json:"rule_coverage_records,omitempty"`
	DecisionRecordsRequired       bool                               `json:"decision_records_required,omitempty"`
	DecisionRecords               int                                `json:"decision_records,omitempty"`
	EntityResolutionRequired      bool                               `json:"entity_resolution_required,omitempty"`
	EntityResolutionRecords       int                                `json:"entity_resolution_records,omitempty"`
	EntityStageMaterialized       bool                               `json:"entity_stage_materialized,omitempty"`
	ContributionLedgerRequired    bool                               `json:"contribution_ledger_required,omitempty"`
	ContributionRecords           int                                `json:"contribution_records,omitempty"`
	ReconcileRequired             bool                               `json:"reconcile_required,omitempty"`
	HasReconcile                  bool                               `json:"has_reconcile,omitempty"`
	HasAnswer                     bool                               `json:"has_answer,omitempty"`
	NextStage                     string                             `json:"next_stage,omitempty"`
	CustomTransformFailures       int                                `json:"custom_transform_failures,omitempty"`
	CustomTransformDisabled       bool                               `json:"custom_transform_disabled,omitempty"`
	CustomTransformDisabledNote   string                             `json:"custom_transform_disabled_note,omitempty"`
	ArtifactAvailabilityCount     int                                `json:"artifact_availability_count,omitempty"`
	ArtifactAvailabilityTruncated bool                               `json:"artifact_availability_truncated,omitempty"`
	ArtifactAvailability          []dataTaskArtifactAccessPrompt     `json:"artifact_availability,omitempty"`
	AllowedNextActions            []string                           `json:"allowed_next_actions,omitempty"`
	AllowedNextActionContracts    []dataTaskActionContract           `json:"allowed_next_action_contracts,omitempty"`
	ActionScaffold                []dataTaskActionScaffold           `json:"action_scaffold,omitempty"`
	FieldContractViolations       []dataTaskFieldContractViolation   `json:"field_contract_violations,omitempty"`
	ZeroMatchFilterViolations     []dataTaskZeroMatchFilterIssue     `json:"zero_match_filter_violations,omitempty"`
	UnmatchedResolutionViolations []dataTaskUnmatchedResolutionIssue `json:"unmatched_resolution_violations,omitempty"`
	ZeroEligibleViolations        []dataTaskZeroEligibleIssue        `json:"zero_eligible_qualification_violations,omitempty"`
	WorkflowViolations            []dataworkflow.WorkflowViolation   `json:"workflow_violations,omitempty"`
}

type dataTaskActionContract struct {
	Kind          string `json:"kind"`
	InputBoundary string `json:"input_boundary,omitempty"`
	UseWhen       string `json:"use_when,omitempty"`
	Output        string `json:"output,omitempty"`
}

type dataTaskActionScaffold struct {
	Kind           string            `json:"kind"`
	UseWhen        string            `json:"use_when,omitempty"`
	InputPath      string            `json:"input_path,omitempty"`
	InputPaths     []string          `json:"input_paths,omitempty"`
	Fields         []string          `json:"fields,omitempty"`
	CommonFields   []string          `json:"common_fields,omitempty"`
	ParamsTemplate map[string]string `json:"params_template,omitempty"`
	Note           string            `json:"note,omitempty"`
}

type dataTaskFieldContractViolation struct {
	Round                int      `json:"round,omitempty"`
	ActionIndex          int      `json:"action_index,omitempty"`
	ActionID             string   `json:"action_id,omitempty"`
	ArtifactID           string   `json:"artifact_id,omitempty"`
	ActionKind           string   `json:"action_kind,omitempty"`
	InputAlias           string   `json:"input_alias,omitempty"`
	InputAliases         []string `json:"input_aliases,omitempty"`
	OutputAlias          string   `json:"output_alias,omitempty"`
	IdempotencyKey       string   `json:"idempotency_key,omitempty"`
	DependencyRank       int      `json:"dependency_rank,omitempty"`
	MissingFields        []string `json:"missing_fields,omitempty"`
	AvailableFieldSample []string `json:"available_field_sample,omitempty"`
	CandidateArtifacts   []string `json:"candidate_artifacts,omitempty"`
	RepairActionHints    []string `json:"repair_action_hints,omitempty"`
	Reason               string   `json:"reason,omitempty"`
}

type dataTaskZeroMatchFilterIssue struct {
	Round             int      `json:"round,omitempty"`
	ArtifactID        string   `json:"artifact_id,omitempty"`
	Aliases           []string `json:"aliases,omitempty"`
	InputPath         string   `json:"input_path,omitempty"`
	InputRows         int      `json:"input_rows,omitempty"`
	OutputRows        int      `json:"output_rows,omitempty"`
	FilterFields      []string `json:"filter_fields,omitempty"`
	FilterDiagnostics string   `json:"filter_diagnostics,omitempty"`
	RepairActionHints []string `json:"repair_action_hints,omitempty"`
	Reason            string   `json:"reason,omitempty"`
}

type dataTaskUnmatchedResolutionIssue struct {
	Round             int      `json:"round,omitempty"`
	ArtifactID        string   `json:"artifact_id,omitempty"`
	Aliases           []string `json:"aliases,omitempty"`
	BasePath          string   `json:"base_path,omitempty"`
	BaseRows          int      `json:"base_rows,omitempty"`
	OutputRows        int      `json:"output_rows,omitempty"`
	TargetFields      []string `json:"target_fields,omitempty"`
	MatchedCounts     []string `json:"matched_counts,omitempty"`
	UnmatchedCounts   []string `json:"unmatched_counts,omitempty"`
	RepairActionHints []string `json:"repair_action_hints,omitempty"`
	Reason            string   `json:"reason,omitempty"`
}

type dataTaskZeroEligibleIssue struct {
	Round                    int      `json:"round,omitempty"`
	ArtifactID               string   `json:"artifact_id,omitempty"`
	Aliases                  []string `json:"aliases,omitempty"`
	InputPath                string   `json:"input_path,omitempty"`
	InputRows                int      `json:"input_rows,omitempty"`
	EligibleRows             int      `json:"eligible_rows,omitempty"`
	IncludeFilterDiagnostics string   `json:"include_filter_diagnostics,omitempty"`
	QualificationReasons     string   `json:"qualification_reasons,omitempty"`
	RepairActionHints        []string `json:"repair_action_hints,omitempty"`
	Reason                   string   `json:"reason,omitempty"`
}

type dataTaskResultPromptView struct {
	Answer                   string                             `json:"answer,omitempty"`
	AnswerItemCount          int                                `json:"answer_item_count,omitempty"`
	OutputContract           dataquery.OutputContract           `json:"output_contract,omitempty"`
	AuditSummary             string                             `json:"audit_summary,omitempty"`
	LedgerProjection         []dataTaskLedgerProjection         `json:"ledger_projection,omitempty"`
	DecisionRecords          int                                `json:"decision_records,omitempty"`
	DecisionSamples          []dataquery.RowDecision            `json:"decision_samples,omitempty"`
	RuleCoverageRecords      int                                `json:"rule_coverage_records,omitempty"`
	RuleCoverageSamples      []dataquery.RuleCoverageRecord     `json:"rule_coverage_samples,omitempty"`
	ContributionRecords      int                                `json:"contribution_records,omitempty"`
	ContributionSamples      []dataquery.ContributionRecord     `json:"contribution_samples,omitempty"`
	EntityResolutionRecords  int                                `json:"entity_resolution_records,omitempty"`
	EntityResolutionSamples  []dataquery.EntityResolutionRecord `json:"entity_resolution_samples,omitempty"`
	Reconcile                *dataquery.ReconcileReport         `json:"reconcile,omitempty"`
	ReconcileGroupCount      int                                `json:"reconcile_group_count,omitempty"`
	ReconcileGroupKeySample  []string                           `json:"reconcile_group_key_sample,omitempty"`
	ReconcileGroupsTruncated bool                               `json:"reconcile_groups_truncated,omitempty"`
	Metrics                  []dataquery.Metric                 `json:"metrics,omitempty"`
	Artifacts                []dataquery.DataArtifact           `json:"artifacts,omitempty"`
	ArtifactAccess           []dataTaskArtifactAccessPrompt     `json:"artifact_access,omitempty"`
	MaterialSetHandles       []dataTaskMaterialSetHandlePrompt  `json:"material_set_handles,omitempty"`
	ContractWarnings         []string                           `json:"contract_warnings,omitempty"`
}

type dataTaskLedgerProjection struct {
	Kind          string         `json:"kind"`
	Count         int            `json:"count"`
	StatusCounts  map[string]int `json:"status_counts,omitempty"`
	DecisionCount map[string]int `json:"decision_counts,omitempty"`
	GroupKeys     []string       `json:"group_key_sample,omitempty"`
	Metrics       []string       `json:"metric_sample,omitempty"`
	Roles         []string       `json:"role_sample,omitempty"`
	Truncated     bool           `json:"truncated,omitempty"`
}

type dataTaskArtifactAccessPrompt struct {
	ID           string              `json:"id,omitempty"`
	Kind         string              `json:"kind,omitempty"`
	NodeClass    string              `json:"node_class,omitempty"`
	Aliases      []string            `json:"aliases,omitempty"`
	JSONShape    string              `json:"json_shape,omitempty"`
	Fields       []string            `json:"fields,omitempty"`
	FieldSamples map[string][]string `json:"field_samples,omitempty"`
	AccessHint   string              `json:"access_hint,omitempty"`
	SourcePaths  []string            `json:"source_paths,omitempty"`
}

type dataTaskMaterialSetHandlePrompt struct {
	ID                string   `json:"id,omitempty"`
	Kind              string   `json:"kind,omitempty"`
	Scope             string   `json:"scope,omitempty"`
	MemberPaths       []string `json:"member_paths,omitempty"`
	TextEvidencePaths []string `json:"text_evidence_paths,omitempty"`
	AccessHint        string   `json:"access_hint,omitempty"`
}

func renderDataTaskRecordsForPrompt(records []dataTaskWorkflowRecord) string {
	return renderDataTaskRecordsForPromptWithBudget(records, dataTaskPromptRecordBudget{
		MaxRecords:          6,
		MaxActions:          8,
		ActionScriptLimit:   1200,
		ActionParamLimit:    1200,
		ScriptPreviewLimit:  1800,
		ErrorLimit:          1600,
		EvalReasonLimit:     1000,
		ResultAnswerLimit:   2400,
		ResultAuditLimit:    1000,
		ResultDecisionLimit: 6,
		ResultRuleLimit:     4,
		ResultContribLimit:  6,
		ResultArtifactLimit: 6,
		ResultAccessLimit:   10,
		ResultMaterialLimit: 8,
	})
}

func renderCompactDataTaskRecordsForPrompt(records []dataTaskWorkflowRecord) string {
	return renderDataTaskRecordsForPromptWithBudget(records, dataTaskPromptRecordBudget{
		MaxRecords:          3,
		MaxActions:          4,
		ActionScriptLimit:   320,
		ActionParamLimit:    480,
		ScriptPreviewLimit:  480,
		ErrorLimit:          700,
		EvalReasonLimit:     600,
		ResultAnswerLimit:   700,
		ResultAuditLimit:    500,
		ResultDecisionLimit: 2,
		ResultRuleLimit:     2,
		ResultContribLimit:  3,
		ResultArtifactLimit: 4,
		ResultAccessLimit:   6,
		ResultMaterialLimit: 4,
	})
}

type dataTaskPromptRecordBudget struct {
	MaxRecords          int
	MaxActions          int
	ActionScriptLimit   int
	ActionParamLimit    int
	ScriptPreviewLimit  int
	ErrorLimit          int
	EvalReasonLimit     int
	ResultAnswerLimit   int
	ResultAuditLimit    int
	ResultDecisionLimit int
	ResultRuleLimit     int
	ResultContribLimit  int
	ResultArtifactLimit int
	ResultAccessLimit   int
	ResultMaterialLimit int
}

func renderDataTaskRecordsForPromptWithBudget(records []dataTaskWorkflowRecord, budget dataTaskPromptRecordBudget) string {
	if len(records) == 0 {
		return "(none)\n"
	}
	if budget.MaxRecords <= 0 {
		budget.MaxRecords = 1
	}
	start := len(records) - budget.MaxRecords
	if start < 0 {
		start = 0
	}
	views := make([]dataTaskWorkflowPromptRecord, 0, len(records)-start)
	for i := start; i < len(records); i++ {
		rec := records[i]
		view := dataTaskWorkflowPromptRecord{
			Round:               i + 1,
			PlanStatus:          strings.TrimSpace(rec.Plan.Status),
			Goal:                strings.TrimSpace(rec.Plan.Goal),
			Actions:             compactDataTaskActionsForPromptWithParamLimit(rec.Plan.Actions, budget.MaxActions, budget.ActionScriptLimit, budget.ActionParamLimit),
			InputPaths:          append([]string(nil), rec.Plan.InputPaths...),
			OutputContract:      rec.Plan.OutputContract.Normalize(),
			SuccessCriteria:     append([]string(nil), rec.Plan.SuccessCriteria...),
			MissingObservations: append([]string(nil), rec.Plan.MissingObservations...),
			NextBatch:           strings.TrimSpace(rec.Plan.NextBatch),
			WhyThisBatch:        strings.TrimSpace(rec.Plan.WhyThisBatch),
			ContinueAfter:       rec.Plan.ContinueAfter,
			ScriptPreview:       clampDataTaskWorkflowText(rec.Plan.Script, budget.ScriptPreviewLimit),
			Error:               clampDataTaskWorkflowText(rec.Err, budget.ErrorLimit),
		}
		if rec.Result != nil {
			view.Result = compactDataTaskResultPromptViewWithArtifactLimits(*rec.Result,
				budget.ResultAnswerLimit,
				budget.ResultAuditLimit,
				budget.ResultDecisionLimit,
				budget.ResultRuleLimit,
				budget.ResultContribLimit,
				budget.ResultArtifactLimit,
				budget.ResultAccessLimit,
				budget.ResultMaterialLimit,
			)
		}
		if rec.Evaluation != nil {
			eval := *rec.Evaluation
			eval.Reason = clampDataTaskWorkflowText(eval.Reason, budget.EvalReasonLimit)
			view.Evaluation = &eval
		}
		views = append(views, view)
	}
	raw, err := json.MarshalIndent(views, "", "  ")
	if err != nil {
		return fmt.Sprintf("render data workflow records failed: %v\n", err)
	}
	if start > 0 {
		return fmt.Sprintf("(omitted %d older data rounds; workflow_state_json is authoritative for cumulative coverage, counts, and next-stage state)\n%s", start, string(raw))
	}
	return string(raw)
}

func dataTaskWorkflowState(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataTaskWorkflowStateView {
	return dataTaskWorkflowStateWithDeferred(records, current, dataquery.TaskPlan{})
}

func dataTaskWorkflowStateWithDeferred(records []dataTaskWorkflowRecord, current, deferred dataquery.TaskPlan) dataTaskWorkflowStateView {
	contract := dataTaskWorkflowCoverageContract(records, current)
	currentBatchContract, currentBatchLayer := dataTaskWorkflowCurrentBatchContract(records, current, contract)
	outputContract := dataTaskWorkflowOutputContract(records, current)
	required := cleanDataTaskStrings(contract.RequiredRunnerInputPaths())
	covered := dataTaskWorkflowCoveredMaterialPaths(records, dataquery.TaskPlan{}, 0)
	var missing []string
	var coveredRequired []string
	for _, p := range required {
		if p == "" {
			continue
		}
		if dataTaskCoveragePathCovered(covered, p) {
			coveredRequired = append(coveredRequired, p)
		} else {
			missing = append(missing, p)
		}
	}
	sort.Strings(required)
	sort.Strings(coveredRequired)
	sort.Strings(missing)
	materialCoverageSufficient := len(required) > 0 && len(missing) == 0
	if len(required) == 0 && dataTaskWorkflowHasMaterialProgress(records) {
		materialCoverageSufficient = true
	}
	state := dataTaskWorkflowStateView{
		MaterialCoverageAuthoritative: true,
		WorkflowContract:              dataworkflow.CoverageContractViewFor(dataworkflow.CoverageLayerWorkflow, contract),
		CurrentBatchContract:          dataworkflow.CoverageContractViewFor(currentBatchLayer, currentBatchContract),
		ActionGraph:                   dataTaskWorkflowActionGraphWithDeferred(records, current, deferred, 48),
		RequiredMaterialCount:         len(required),
		RequiredMaterials:             required,
		CoveredRequiredMaterials:      coveredRequired,
		MissingRequiredMaterialCount:  len(missing),
		MissingRequiredMaterials:      missing,
		CoverageNote:                  "required/covered/missing material fields are deterministic workflow truth; compact result.artifact_access and previous_data_rounds are samples, not coverage authority",
		OutputContract:                outputContract,
		MaterialCoverageSufficient:    materialCoverageSufficient,
		RuleCoverageRequired:          contract.RuleCoverageRequired,
		DecisionRecordsRequired:       contract.DecisionRecordsRequired,
		EntityResolutionRequired:      contract.EntityResolutionRequired,
		ContributionLedgerRequired:    contract.ContributionLedgerRequired,
		ReconcileRequired:             contract.ReconcileRequired,
	}
	var decisionRecords []dataquery.RowDecision
	var ruleCoverageRecords []dataquery.RuleCoverageRecord
	var contributionRecords []dataquery.ContributionRecord
	var entityResolutionRecords []dataquery.EntityResolutionRecord
	for _, rec := range records {
		if rec.Result == nil {
			continue
		}
		ruleCoverageRecords = append(ruleCoverageRecords, rec.Result.RuleCoverage...)
		decisionRecords = append(decisionRecords, rec.Result.Rows...)
		entityResolutionRecords = append(entityResolutionRecords, rec.Result.EntityResolutions...)
		contributionRecords = append(contributionRecords, rec.Result.Contributions...)
		if rec.Result.Reconcile != nil {
			state.HasReconcile = true
		}
		if dataTaskResultIsFinalAnswerCandidate(rec.Plan, *rec.Result, contract, outputContract) {
			state.HasAnswer = true
		}
	}
	state.RuleCoverageRecords = len(dataquery.DedupeRuleCoverageRecords(ruleCoverageRecords))
	state.DecisionRecords = len(dataquery.DedupeRowDecisionRecords(decisionRecords))
	state.EntityResolutionRecords = len(dataquery.DedupeEntityResolutionRecords(entityResolutionRecords))
	state.ContributionRecords = len(dataquery.DedupeContributionRecords(contributionRecords))
	state.EntityStageMaterialized = state.EntityResolutionRecords > 0 || dataTaskWorkflowEntityStageMaterialized(records)
	state.CustomTransformFailures, _, _ = dataTaskCustomTransformFailureClassStats(records)
	state.NextStage = dataTaskWorkflowNextStage(state)
	state.CustomTransformDisabled = dataTaskCustomTransformCooldownForState(records, state)
	state.AllowedNextActionContracts = dataTaskWorkflowAllowedNextActionContracts(state)
	if state.CustomTransformDisabled {
		state.CustomTransformDisabledNote = "free-form custom_transform scripts are disabled after workflow/script risk; typed actions remain executable and are the preferred path"
		state.AllowedNextActionContracts = dataTaskFilterCustomTransformContracts(state.AllowedNextActionContracts)
	}
	state.ArtifactAvailability, state.ArtifactAvailabilityCount, state.ArtifactAvailabilityTruncated = dataTaskWorkflowArtifactAvailability(records, 48)
	state.AllowedNextActions = dataTaskActionKindsFromContracts(state.AllowedNextActionContracts)
	state.ActionScaffold = dataTaskWorkflowActionScaffold(records, state)
	state.FieldContractViolations = dataTaskWorkflowFieldContractViolations(records, state, 4)
	state.ZeroMatchFilterViolations = dataTaskWorkflowZeroMatchFilterIssues(records, state, 4)
	state.UnmatchedResolutionViolations = dataTaskWorkflowUnmatchedResolutionIssues(records, state, 4)
	state.ZeroEligibleViolations = dataTaskWorkflowZeroEligibleIssues(records, state, 4)
	state.WorkflowViolations = dataTaskWorkflowTypedViolations(records, state)
	state.ActionGraph = dataTaskWorkflowActionGraphWithDeferredAndViolations(records, current, deferred, state.WorkflowViolations, 48)
	return state
}

func dataTaskWorkflowTypedViolations(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView) []dataworkflow.WorkflowViolation {
	var out []dataworkflow.WorkflowViolation
	out = append(out, dataTaskWorkflowStageNoProgressViolations(records, state)...)
	for _, issue := range state.FieldContractViolations {
		out = append(out, dataworkflow.WorkflowViolation{
			Code:                 "field_contract_violation",
			Severity:             "error",
			Repairability:        dataworkflow.RepairNeedsTypedAction,
			ActionID:             strings.TrimSpace(issue.ActionID),
			ActionKind:           strings.TrimSpace(issue.ActionKind),
			InputAlias:           strings.TrimSpace(issue.InputAlias),
			InputAliases:         cleanDataTaskStrings(issue.InputAliases),
			OutputAlias:          strings.TrimSpace(issue.OutputAlias),
			IdempotencyKey:       strings.TrimSpace(issue.IdempotencyKey),
			DependencyRank:       issue.DependencyRank,
			MissingFields:        append([]string(nil), issue.MissingFields...),
			AvailableFieldSample: append([]string(nil), issue.AvailableFieldSample...),
			CandidateArtifacts:   append([]string(nil), issue.CandidateArtifacts...),
			RepairActionHints:    append([]string(nil), issue.RepairActionHints...),
			Reason:               strings.TrimSpace(issue.Reason),
		})
	}
	for _, issue := range state.ZeroMatchFilterViolations {
		aliases := dataTaskZeroMatchFilterIssueAliases(issue)
		input := strings.TrimSpace(issue.InputPath)
		if input == "" && len(aliases) > 0 {
			input = aliases[0]
		}
		action := dataquery.DataAction{
			Kind:           dataquery.DataActionFilterRecords,
			InputPaths:     cleanDataTaskStrings([]string{input}),
			OutputArtifact: firstNonEmptyString(issue.ArtifactID, strings.Join(aliases, ",")),
		}
		out = append(out, dataworkflow.NewActionInputViolation(
			"zero_match_filter",
			"warning",
			dataworkflow.RepairNeedsTypedAction,
			action,
			input,
			issue.FilterFields,
			issue.Reason,
			issue.RepairActionHints,
		))
	}
	for _, issue := range state.UnmatchedResolutionViolations {
		aliases := dataTaskUnmatchedResolutionIssueAliases(issue)
		input := strings.TrimSpace(issue.BasePath)
		if input == "" && len(aliases) > 0 {
			input = aliases[0]
		}
		action := dataquery.DataAction{
			Kind:           dataquery.DataActionApplyResolutions,
			InputPaths:     cleanDataTaskStrings([]string{input}),
			OutputArtifact: firstNonEmptyString(issue.ArtifactID, strings.Join(aliases, ",")),
		}
		out = append(out, dataworkflow.NewActionInputViolation(
			"unmatched_resolution",
			"warning",
			dataworkflow.RepairNeedsTypedAction,
			action,
			input,
			issue.TargetFields,
			issue.Reason,
			issue.RepairActionHints,
		))
	}
	for _, issue := range state.ZeroEligibleViolations {
		aliases := dataTaskZeroEligibleIssueAliases(issue)
		input := strings.TrimSpace(issue.InputPath)
		if input == "" && len(aliases) > 0 {
			input = aliases[0]
		}
		action := dataquery.DataAction{
			Kind:           dataquery.DataActionQualifyRecords,
			InputPaths:     cleanDataTaskStrings([]string{input}),
			OutputArtifact: firstNonEmptyString(issue.ArtifactID, strings.Join(aliases, ",")),
		}
		out = append(out, dataworkflow.NewActionInputViolation(
			"zero_eligible_records",
			"warning",
			dataworkflow.RepairNeedsTypedAction,
			action,
			input,
			nil,
			issue.Reason,
			issue.RepairActionHints,
		))
	}
	return out
}

func dataTaskWorkflowStageNoProgressViolations(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView) []dataworkflow.WorkflowViolation {
	if state.NextStage != "prepare_contribution_inputs" && state.NextStage != "compute_contributions" {
		return nil
	}
	if (!state.ContributionLedgerRequired && !state.ReconcileRequired) || state.ContributionRecords > 0 || state.HasReconcile {
		return nil
	}
	count := dataTaskRecentJoinNoProgressCount(records)
	if count < DefaultDataTaskMaxNodeFailures {
		return nil
	}
	return []dataworkflow.WorkflowViolation{{
		Code:          "stage_no_progress",
		Severity:      "warning",
		Repairability: dataworkflow.RepairNeedsTypedAction,
		ActionKind:    string(dataquery.DataActionJoinRecords),
		RepairActionHints: cleanDataTaskStrings([]string{
			string(dataquery.DataActionDeriveFields),
			string(dataquery.DataActionExtractFields),
			string(dataquery.DataActionFilterRecords),
			string(dataquery.DataActionQualifyRecords),
			string(dataquery.DataActionComputeContribs),
			string(dataquery.DataActionReconcile),
		}),
		Reason: fmt.Sprintf("the workflow is still at %s after %d recent join_records result(s) without contribution or reconcile progress; stop automatic relation joins and repair from artifact fields, eligibility filters, contribution calculation, or reconciliation", state.NextStage, count),
	}}
}

func dataTaskRecentJoinNoProgressCount(records []dataTaskWorkflowRecord) int {
	count := 0
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if rec.Result == nil || strings.TrimSpace(rec.Err) != "" {
			break
		}
		if len(rec.Result.Contributions) > 0 || rec.Result.Reconcile != nil {
			break
		}
		if !dataTaskPlanHasOnlyActionKind(rec.Plan, dataquery.DataActionJoinRecords) {
			break
		}
		count++
	}
	return count
}

func dataTaskPlanHasOnlyActionKind(plan dataquery.TaskPlan, kind dataquery.DataActionKind) bool {
	if len(plan.Actions) == 0 {
		return false
	}
	for _, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) != kind {
			return false
		}
	}
	return true
}

func dataTaskWorkflowActionGraph(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, limit int) dataworkflow.ActionGraph {
	return dataTaskWorkflowActionGraphWithDeferredAndViolations(records, current, dataquery.TaskPlan{}, nil, limit)
}

func dataTaskWorkflowActionGraphWithViolations(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, violations []dataworkflow.WorkflowViolation, limit int) dataworkflow.ActionGraph {
	return dataTaskWorkflowActionGraphWithDeferredAndViolations(records, current, dataquery.TaskPlan{}, violations, limit)
}

func dataTaskWorkflowActionGraphWithDeferred(records []dataTaskWorkflowRecord, current, deferred dataquery.TaskPlan, limit int) dataworkflow.ActionGraph {
	return dataTaskWorkflowActionGraphWithDeferredAndViolations(records, current, deferred, nil, limit)
}

func dataTaskWorkflowActionGraphWithDeferredAndViolations(records []dataTaskWorkflowRecord, current, deferred dataquery.TaskPlan, violations []dataworkflow.WorkflowViolation, limit int) dataworkflow.ActionGraph {
	var ready []dataquery.DataAction
	if dataTaskPlanHasExecutableBatch(current) {
		ready = current.Actions
	}
	var deferredActions []dataquery.DataAction
	if dataTaskPlanHasExecutableBatch(deferred) {
		deferredActions = deferred.Actions
	}
	return dataworkflow.ReduceActionGraphState(dataworkflow.ActionGraphInput{
		Events:     dataTaskWorkflowActionEvents(records),
		Ready:      ready,
		Deferred:   deferredActions,
		Blocked:    violations,
		EventLimit: limit,
	})
}

func dataTaskWorkflowActionEvents(records []dataTaskWorkflowRecord) []dataworkflow.ActionEvent {
	events := make([]dataworkflow.ActionEvent, 0, len(records))
	for _, rec := range records {
		status := dataworkflow.ActionStatusExecuted
		if strings.TrimSpace(rec.Err) != "" {
			status = dataworkflow.ActionStatusFailed
		}
		events = append(events, dataworkflow.ActionEvent{
			Actions: rec.Plan.Actions,
			Status:  status,
		})
	}
	return events
}

func dataTaskWorkflowCurrentBatchContract(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, workflow dataquery.CoverageContract) (dataquery.CoverageContract, dataworkflow.CoverageLayer) {
	if dataTaskPlanHasExecutableBatch(current) {
		return dataTaskExecutionCoverageContract(records, current, workflow), dataworkflow.CoverageLayerCurrentBatch
	}
	if latest, ok := dataTaskLatestWorkflowPlan(records); ok {
		return latest.CoverageContract, dataworkflow.CoverageLayerLastBatch
	}
	return dataquery.CoverageContract{}, dataworkflow.CoverageLayerCurrentBatch
}

func dataTaskExecutionCoverageContract(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, workflow dataquery.CoverageContract) dataquery.CoverageContract {
	currentContract := dataTaskConstrainCurrentRequiredMaterials(dataquery.CoverageContract{}, plan)
	currentContract.DecisionRecordsRequired = workflow.DecisionRecordsRequired
	currentContract.RuleCoverageRequired = workflow.RuleCoverageRequired
	currentContract.ContributionLedgerRequired = workflow.ContributionLedgerRequired
	currentContract.EntityResolutionRequired = workflow.EntityResolutionRequired
	currentContract.ReconcileRequired = workflow.ReconcileRequired
	if len(currentContract.ValidationRules) == 0 {
		currentContract.ValidationRules = append([]string(nil), workflow.ValidationRules...)
	}
	return currentContract
}

func dataTaskPlanHasExecutableBatch(plan dataquery.TaskPlan) bool {
	return len(plan.Actions) > 0 || strings.TrimSpace(plan.Script) != "" || len(cleanDataTaskStrings(plan.InputPaths)) > 0
}

func dataTaskLatestWorkflowPlan(records []dataTaskWorkflowRecord) (dataquery.TaskPlan, bool) {
	for i := len(records) - 1; i >= 0; i-- {
		if dataTaskPlanHasExecutableBatch(records[i].Plan) || len(records[i].Plan.CoverageContract.RequiredMaterials) > 0 || len(records[i].Plan.CoverageContract.OptionalMaterials) > 0 {
			return records[i].Plan, true
		}
	}
	return dataquery.TaskPlan{}, false
}

func dataTaskWorkflowHasMaterialProgress(records []dataTaskWorkflowRecord) bool {
	for _, rec := range records {
		if rec.Result == nil || strings.TrimSpace(rec.Err) != "" {
			continue
		}
		if len(cleanDataTaskStrings(rec.Result.ConsumedPaths)) > 0 {
			return true
		}
		for _, artifact := range rec.Result.Artifacts {
			if strings.TrimSpace(artifact.ID) != "" || len(cleanDataTaskStrings(artifact.SourcePaths)) > 0 {
				return true
			}
			if artifact.Fields != nil && strings.TrimSpace(artifact.Fields["artifact_path"]) != "" {
				return true
			}
		}
	}
	return false
}

func dataTaskWorkflowFieldContractViolations(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView, limit int) []dataTaskFieldContractViolation {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	access := dataTaskWorkflowArtifactContractAccess(records)
	if len(access) == 0 {
		return nil
	}
	allowed := map[string]bool{}
	for _, action := range state.AllowedNextActions {
		allowed[action] = true
	}
	var out []dataTaskFieldContractViolation
	for i := len(records) - 1; i >= 0 && len(out) < limit; i-- {
		rec := records[i]
		for _, violation := range dataTaskFieldContractViolationsFromRecord(i+1, rec, access, allowed) {
			out = append(out, violation)
			if len(out) >= limit {
				break
			}
		}
		if rec.Result != nil {
			for j := len(rec.Result.Artifacts) - 1; j >= 0 && len(out) < limit; j-- {
				for _, violation := range dataTaskExtractFieldContractViolationsFromArtifact(i+1, rec.Result.Artifacts[j], access) {
					out = append(out, violation)
					if len(out) >= limit {
						break
					}
				}
			}
		}
	}
	return out
}

func dataTaskWorkflowZeroMatchFilterIssues(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView, limit int) []dataTaskZeroMatchFilterIssue {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if (!state.ContributionLedgerRequired && !state.ReconcileRequired) || state.ContributionRecords > 0 {
		return nil
	}
	var out []dataTaskZeroMatchFilterIssue
	clearedAliases := map[string]bool{}
	for i := len(records) - 1; i >= 0 && len(out) < limit; i-- {
		rec := records[i]
		if rec.Result == nil {
			continue
		}
		for j := len(rec.Result.Artifacts) - 1; j >= 0 && len(out) < limit; j-- {
			dataTaskMarkPositiveFilterArtifactAliases(rec.Result.Artifacts[j], clearedAliases)
			for _, issue := range dataTaskZeroMatchFilterIssuesFromArtifact(i+1, rec.Result.Artifacts[j]) {
				if dataTaskZeroMatchFilterIssueCleared(issue, clearedAliases) {
					continue
				}
				out = append(out, issue)
				if len(out) >= limit {
					break
				}
			}
		}
	}
	return out
}

func dataTaskMarkPositiveFilterArtifactAliases(artifact dataquery.DataArtifact, cleared map[string]bool) {
	if cleared == nil {
		return
	}
	var walk func(dataquery.DataArtifact)
	walk = func(current dataquery.DataArtifact) {
		if dataTaskArtifactKindHasPrefix(current.Kind, dataquery.DataActionFilterRecords) {
			outputRows := dataTaskArtifactIntField(current, "output_rows", "filter_matched")
			if outputRows == 0 {
				outputRows = current.RowCount
			}
			if outputRows > 0 {
				for _, alias := range dataTaskArtifactAliasPaths(current) {
					if key := normalizeDataTaskCoveragePath(alias); key != "" {
						cleared[key] = true
					}
				}
				if key := normalizeDataTaskCoveragePath(current.ID); key != "" {
					cleared[key] = true
				}
			}
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(artifact)
}

func dataTaskZeroMatchFilterIssueCleared(issue dataTaskZeroMatchFilterIssue, cleared map[string]bool) bool {
	if len(cleared) == 0 {
		return false
	}
	for _, alias := range dataTaskZeroMatchFilterIssueAliases(issue) {
		if cleared[normalizeDataTaskCoveragePath(alias)] {
			return true
		}
	}
	return false
}

func dataTaskZeroMatchFilterIssuesFromArtifact(round int, artifact dataquery.DataArtifact) []dataTaskZeroMatchFilterIssue {
	var out []dataTaskZeroMatchFilterIssue
	var walk func(dataquery.DataArtifact)
	walk = func(current dataquery.DataArtifact) {
		if dataTaskArtifactKindHasPrefix(current.Kind, dataquery.DataActionFilterRecords) {
			inputRows, _ := strconv.Atoi(strings.TrimSpace(current.Fields["input_rows"]))
			outputRows, _ := strconv.Atoi(strings.TrimSpace(firstNonEmptyString(current.Fields["output_rows"], current.Fields["filter_matched"])))
			if inputRows > 0 && outputRows == 0 {
				aliases := dataTaskArtifactAliasPaths(current)
				out = append(out, dataTaskZeroMatchFilterIssue{
					Round:             round,
					ArtifactID:        strings.TrimSpace(current.ID),
					Aliases:           clampDataTaskStringSlice(aliases, 8),
					InputPath:         strings.TrimSpace(current.Fields["input_path"]),
					InputRows:         inputRows,
					OutputRows:        outputRows,
					FilterFields:      cleanDataTaskStrings(strings.Split(current.Fields["filter_fields"], ",")),
					FilterDiagnostics: clampDataTaskWorkflowText(current.Fields["filter_diagnostics"], 1200),
					RepairActionHints: []string{
						"inspect the original input artifact or current filter artifact to compare actual field values against every filter",
						"rerun filter_records against the non-empty input artifact with corrected filters before join_records or compute_contributions",
						"do not join or compute contributions from this zero-row artifact while contribution/reconcile is still required",
					},
					Reason: "filter_records produced zero rows from a non-empty input while contribution/reconcile remains required",
				})
			}
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(artifact)
	return out
}

func dataTaskWorkflowUnmatchedResolutionIssues(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView, limit int) []dataTaskUnmatchedResolutionIssue {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if !state.EntityResolutionRequired || (!state.ContributionLedgerRequired && !state.ReconcileRequired) || state.ContributionRecords > 0 {
		return nil
	}
	var out []dataTaskUnmatchedResolutionIssue
	for i := len(records) - 1; i >= 0 && len(out) < limit; i-- {
		rec := records[i]
		if rec.Result == nil {
			continue
		}
		for j := len(rec.Result.Artifacts) - 1; j >= 0 && len(out) < limit; j-- {
			for _, issue := range dataTaskUnmatchedResolutionIssuesFromArtifact(i+1, rec.Result.Artifacts[j]) {
				out = append(out, issue)
				if len(out) >= limit {
					break
				}
			}
		}
	}
	return out
}

func dataTaskUnmatchedResolutionIssuesFromArtifact(round int, artifact dataquery.DataArtifact) []dataTaskUnmatchedResolutionIssue {
	var out []dataTaskUnmatchedResolutionIssue
	var walk func(dataquery.DataArtifact)
	walk = func(current dataquery.DataArtifact) {
		if dataTaskArtifactKindHasPrefix(current.Kind, dataquery.DataActionApplyResolutions) {
			baseRows := dataTaskArtifactIntField(current, "base_rows", "input_rows")
			outputRows := dataTaskArtifactIntField(current, "output_rows", "row_count")
			if outputRows == 0 {
				outputRows = current.RowCount
			}
			targets := cleanDataTaskStrings(strings.Split(current.Fields["target_fields"], ","))
			if len(targets) > 0 && baseRows > 0 {
				var unmatchedTargets []string
				var matchedCounts []string
				var unmatchedCounts []string
				for _, target := range targets {
					matched := dataTaskArtifactIntField(current, "matched_"+target)
					unmatched := dataTaskArtifactIntField(current, "unmatched_"+target)
					if matched == 0 && unmatched > 0 {
						unmatchedTargets = append(unmatchedTargets, target)
						matchedCounts = append(matchedCounts, target+"=0")
						unmatchedCounts = append(unmatchedCounts, fmt.Sprintf("%s=%d", target, unmatched))
					}
				}
				if len(unmatchedTargets) == len(targets) {
					aliases := dataTaskArtifactAliasPaths(current)
					out = append(out, dataTaskUnmatchedResolutionIssue{
						Round:           round,
						ArtifactID:      strings.TrimSpace(current.ID),
						Aliases:         clampDataTaskStringSlice(aliases, 8),
						BasePath:        strings.TrimSpace(current.Fields["base_path"]),
						BaseRows:        baseRows,
						OutputRows:      outputRows,
						TargetFields:    unmatchedTargets,
						MatchedCounts:   matchedCounts,
						UnmatchedCounts: unmatchedCounts,
						RepairActionHints: []string{
							"repair apply_entity_resolutions before filtering, qualification, join, or contribution calculation",
							"use locator-compatible key fields: when one side uses item_id/source_locator and the other side uses _source_locator/_source_index, match them through the row locator/index contract",
							"do not consume this all-unmatched resolution artifact for downstream contribution work",
						},
						Reason: "apply_entity_resolutions produced no matched canonical target values for a non-empty base record set",
					})
				}
			}
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(artifact)
	return out
}

func dataTaskWorkflowZeroEligibleIssues(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView, limit int) []dataTaskZeroEligibleIssue {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if (!state.ContributionLedgerRequired && !state.ReconcileRequired) || state.ContributionRecords > 0 {
		return nil
	}
	var out []dataTaskZeroEligibleIssue
	for i := len(records) - 1; i >= 0 && len(out) < limit; i-- {
		rec := records[i]
		if rec.Result == nil {
			continue
		}
		for j := len(rec.Result.Artifacts) - 1; j >= 0 && len(out) < limit; j-- {
			for _, issue := range dataTaskZeroEligibleIssuesFromArtifact(i+1, rec.Result.Artifacts[j]) {
				out = append(out, issue)
				if len(out) >= limit {
					break
				}
			}
		}
	}
	return out
}

func dataTaskZeroEligibleIssuesFromArtifact(round int, artifact dataquery.DataArtifact) []dataTaskZeroEligibleIssue {
	var out []dataTaskZeroEligibleIssue
	var walk func(dataquery.DataArtifact)
	walk = func(current dataquery.DataArtifact) {
		if dataTaskArtifactKindHasPrefix(current.Kind, dataquery.DataActionQualifyRecords) {
			inputRows := dataTaskArtifactIntField(current, "input_rows")
			eligibleRows := dataTaskArtifactIntField(current, "eligible_rows", "output_rows")
			if inputRows > 0 && eligibleRows == 0 {
				aliases := dataTaskArtifactAliasPaths(current)
				out = append(out, dataTaskZeroEligibleIssue{
					Round:                    round,
					ArtifactID:               strings.TrimSpace(current.ID),
					Aliases:                  clampDataTaskStringSlice(aliases, 8),
					InputPath:                strings.TrimSpace(current.Fields["input_path"]),
					InputRows:                inputRows,
					EligibleRows:             eligibleRows,
					IncludeFilterDiagnostics: clampDataTaskWorkflowText(current.Fields["include_filters"], 1200),
					QualificationReasons:     clampDataTaskWorkflowText(current.Fields["qualification_reasons"], 800),
					RepairActionHints: []string{
						"repair the upstream field materialization, resolution application, or qualification filters before joining or computing contributions",
						"inspect actual field values when every row failed an include/status/evidence condition",
						"do not join or compute contributions from this zero-eligible artifact while contribution/reconcile remains required",
					},
					Reason: "qualify_records produced zero eligible rows from a non-empty input while contribution/reconcile remains required",
				})
			}
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(artifact)
	return out
}

func dataTaskArtifactIntField(artifact dataquery.DataArtifact, keys ...string) int {
	for _, key := range keys {
		raw := strings.TrimSpace(artifact.Fields[key])
		if raw == "" {
			continue
		}
		value, err := strconv.Atoi(raw)
		if err == nil {
			return value
		}
	}
	return 0
}

var dataTaskPlanningMissingFieldRe = regexp.MustCompile(`data planning incomplete: action (\d+) \(([^)]*)\) references field\(s\) \[([^\]]*)\] that are not present on input ([^ ]+) fields \[([^\]]*)\]`)
var dataTaskNumericFieldContractRe = regexp.MustCompile(`numeric field contract failed: (filter_records|compute_contributions) input ([^ ]+) (?:field|value_field) "([^"]+)"`)

func dataTaskFieldContractViolationsFromRecord(round int, rec dataTaskWorkflowRecord, access []dataTaskArtifactAccessPrompt, allowed map[string]bool) []dataTaskFieldContractViolation {
	errText := strings.TrimSpace(rec.Err)
	if errText == "" {
		return nil
	}
	matches := dataTaskPlanningMissingFieldRe.FindAllStringSubmatch(errText, -1)
	var out []dataTaskFieldContractViolation
	for _, match := range matches {
		actionIndex := 0
		fmt.Sscanf(match[1], "%d", &actionIndex)
		actionID := strings.TrimSpace(match[2])
		actionKind := ""
		var action dataquery.DataAction
		hasAction := false
		if actionIndex > 0 && actionIndex <= len(rec.Plan.Actions) {
			action = rec.Plan.Actions[actionIndex-1]
			hasAction = true
			actionID = firstNonEmptyString(strings.TrimSpace(action.ID), actionID)
			actionKind = strings.TrimSpace(string(normalizeDataActionKindForWorkflow(action.Kind)))
		}
		missing := cleanDataTaskStrings(parseDataTaskActionStringListParam(match[3]))
		input := strings.TrimSpace(match[4])
		available := clampDataTaskStringSlice(cleanDataTaskStrings(parseDataTaskActionStringListParam(match[5])), 24)
		if hasAction {
			violation := dataworkflow.NewFieldContractViolation(dataworkflow.FieldContractViolationInput{
				Action:             action,
				InputAlias:         input,
				MissingFields:      missing,
				AvailableFields:    available,
				SchemaProjections:  dataTaskArtifactAccessSchemaProjection(access),
				AllowedNextActions: dataTaskAllowedActionNamesFromMap(allowed),
			})
			view := dataTaskFieldContractViolationFromWorkflowViolation(round, actionIndex, violation)
			view.ActionID = firstNonEmptyString(view.ActionID, actionID)
			view.Reason = clampDataTaskWorkflowText(match[0], 700)
			out = append(out, view)
			continue
		}
		violation := dataTaskFieldContractViolation{
			Round:                round,
			ActionIndex:          actionIndex,
			ActionID:             actionID,
			ActionKind:           actionKind,
			InputAlias:           input,
			MissingFields:        missing,
			AvailableFieldSample: available,
			CandidateArtifacts:   dataTaskFieldContractCandidateArtifacts(access, input, missing),
			RepairActionHints:    dataTaskFieldContractRepairHints(allowed),
			Reason:               clampDataTaskWorkflowText(match[0], 700),
		}
		out = append(out, violation)
	}
	for _, match := range dataTaskNumericFieldContractRe.FindAllStringSubmatch(errText, -1) {
		input := strings.TrimSpace(match[2])
		field := strings.TrimSpace(match[3])
		available := []string{}
		if artifact, ok := dataTaskArtifactAccessByAlias(access, input); ok {
			available = clampDataTaskStringSlice(artifact.Fields, 24)
		}
		out = append(out, dataTaskFieldContractViolation{
			Round:                round,
			ActionKind:           strings.TrimSpace(match[1]),
			InputAlias:           input,
			MissingFields:        []string{field},
			AvailableFieldSample: available,
			CandidateArtifacts:   dataTaskNumericFieldCandidateArtifacts(access, input),
			RepairActionHints: []string{
				"use derive_fields operation=parse_number to materialize a numeric field from an existing numeric-looking source field",
				"do not repeat numeric filter_records or compute_contributions with a flag/status/text field as the numeric field",
				"use flag/status fields only for eq/in/contains/decision filters, then use a separate numeric value_field for contribution/reconcile",
			},
			Reason: clampDataTaskWorkflowText(match[0], 700),
		})
	}
	return out
}

func dataTaskFieldContractViolationFromWorkflowViolation(round, actionIndex int, violation dataworkflow.WorkflowViolation) dataTaskFieldContractViolation {
	return dataTaskFieldContractViolation{
		Round:                round,
		ActionIndex:          actionIndex,
		ActionID:             strings.TrimSpace(violation.ActionID),
		ActionKind:           strings.TrimSpace(violation.ActionKind),
		InputAlias:           strings.TrimSpace(violation.InputAlias),
		InputAliases:         cleanDataTaskStrings(violation.InputAliases),
		OutputAlias:          strings.TrimSpace(violation.OutputAlias),
		IdempotencyKey:       strings.TrimSpace(violation.IdempotencyKey),
		DependencyRank:       violation.DependencyRank,
		MissingFields:        append([]string(nil), violation.MissingFields...),
		AvailableFieldSample: append([]string(nil), violation.AvailableFieldSample...),
		CandidateArtifacts:   append([]string(nil), violation.CandidateArtifacts...),
		RepairActionHints:    append([]string(nil), violation.RepairActionHints...),
		Reason:               strings.TrimSpace(violation.Reason),
	}
}

func dataTaskExtractFieldContractViolationsFromArtifact(round int, artifact dataquery.DataArtifact, access []dataTaskArtifactAccessPrompt) []dataTaskFieldContractViolation {
	var out []dataTaskFieldContractViolation
	var walk func(dataquery.DataArtifact)
	walk = func(current dataquery.DataArtifact) {
		if dataTaskArtifactKindHasPrefix(current.Kind, dataquery.DataActionExtractFields) {
			inputRows := dataTaskArtifactIntField(current, "input_rows")
			outputRows := dataTaskArtifactIntField(current, "output_rows")
			required := cleanDataTaskStrings(strings.Split(current.Fields["required_fields"], ","))
			extracted := cleanDataTaskStrings(strings.Split(current.Fields["extracted_fields"], ","))
			if inputRows > 0 && outputRows == 0 && (len(required) > 0 || len(extracted) > 0) {
				missing := required
				if len(missing) == 0 {
					missing = extracted
				}
				inputAlias := firstNonEmptyString(strings.TrimSpace(current.Fields["input_path"]), strings.TrimSpace(current.Fields["input_paths"]))
				available := cleanDataTaskStrings(strings.Split(firstNonEmptyString(current.Fields["source_fields"], strings.Join(current.Headers, ",")), ","))
				candidateFields := cleanDataTaskStrings(strings.Split(firstNonEmptyString(current.Fields["source_fields"], strings.Join(current.Headers, ",")), ","))
				candidates := dataTaskFieldContractCandidateArtifacts(access, inputAlias, candidateFields)
				if len(candidates) == 0 {
					candidates = dataTaskFieldContractCandidateArtifacts(access, inputAlias, missing)
				}
				if len(candidates) == 0 {
					candidates = clampDataTaskStringSlice(dataTaskArtifactAliasPaths(current), 8)
				}
				reasonParts := []string{"extract_fields produced zero matched records from a non-empty input"}
				if diag := strings.TrimSpace(current.Fields["zero_match_diagnostics"]); diag != "" {
					reasonParts = append(reasonParts, "diagnostics="+diag)
				}
				if samples := strings.TrimSpace(current.Fields["source_field_samples"]); samples != "" {
					reasonParts = append(reasonParts, "source_field_samples="+samples)
				}
				out = append(out, dataTaskFieldContractViolation{
					Round:                round,
					ArtifactID:           strings.TrimSpace(current.ID),
					ActionKind:           string(dataquery.DataActionExtractFields),
					InputAlias:           inputAlias,
					MissingFields:        clampDataTaskStringSlice(missing, 16),
					AvailableFieldSample: clampDataTaskStringSlice(available, 24),
					CandidateArtifacts:   clampDataTaskStringSlice(candidates, 8),
					RepairActionHints: []string{
						"inspect source_field_samples and current artifact fields before retrying extraction",
						"repair extract_fields by adjusting source_field/source_fields, regex/pattern, document scope, grouping, or required_fields",
						"do not compute contributions or assemble a final answer from this zero-match extraction while contribution/reconcile/projection remains required",
					},
					Reason: clampDataTaskWorkflowText(strings.Join(reasonParts, "; "), 1200),
				})
			}
		}
		for _, child := range current.Children {
			walk(child)
		}
	}
	walk(artifact)
	return out
}

func dataTaskNumericFieldCandidateArtifacts(access []dataTaskArtifactAccessPrompt, input string) []string {
	inputKey := normalizeDataTaskCoveragePath(input)
	type candidate struct {
		label string
		score int
	}
	var candidates []candidate
	for _, artifact := range access {
		alias := firstDataTaskArtifactAlias(artifact)
		if alias == "" {
			alias = artifact.ID
		}
		if strings.TrimSpace(alias) == "" {
			continue
		}
		fields := dataTaskNumericLookingSampleFields(artifact.FieldSamples, 6)
		if len(fields) == 0 {
			continue
		}
		score := len(fields)
		if normalizeDataTaskCoveragePath(alias) == inputKey || normalizeDataTaskCoveragePath(artifact.ID) == inputKey {
			score += 10
		}
		candidates = append(candidates, candidate{
			label: fmt.Sprintf("%s numeric-like fields [%s]", alias, strings.Join(fields, ", ")),
			score: score,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].label < candidates[j].label
	})
	var out []string
	for _, candidate := range candidates {
		out = append(out, candidate.label)
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func dataTaskNumericLookingSampleFields(samples map[string][]string, limit int) []string {
	if limit <= 0 {
		limit = 1
	}
	var out []string
	for field, values := range samples {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		for _, value := range values {
			if dataTaskStringLooksNumeric(value) {
				out = append(out, field)
				break
			}
		}
	}
	sort.Strings(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func dataTaskFieldContractRepairHints(allowed map[string]bool) []string {
	return dataworkflow.FieldContractRepairHints(dataTaskAllowedActionNamesFromMap(allowed))
}

func dataTaskAllowedActionNamesFromMap(allowed map[string]bool) []string {
	allowedActions := make([]string, 0, len(allowed))
	for action, ok := range allowed {
		if ok {
			allowedActions = append(allowedActions, action)
		}
	}
	sort.Strings(allowedActions)
	return allowedActions
}

func dataTaskWorkflowEntityStageMaterialized(records []dataTaskWorkflowRecord) bool {
	for _, rec := range records {
		if rec.Result == nil || strings.TrimSpace(rec.Err) != "" {
			continue
		}
		for _, artifact := range rec.Result.Artifacts {
			if dataTaskArtifactMaterializesEntityStage(artifact) {
				return true
			}
		}
	}
	return false
}

func dataTaskArtifactMaterializesEntityStage(artifact dataquery.DataArtifact) bool {
	kind := strings.ToLower(strings.TrimSpace(artifact.Kind))
	if strings.Contains(kind, string(dataquery.DataActionNormalizeEntities)) ||
		strings.Contains(kind, string(dataquery.DataActionEnrichRecords)) ||
		strings.Contains(kind, string(dataquery.DataActionJoinRecords)) {
		return true
	}
	for _, child := range artifact.Children {
		if dataTaskArtifactMaterializesEntityStage(child) {
			return true
		}
	}
	return false
}

func dataTaskWorkflowActionScaffold(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView) []dataTaskActionScaffold {
	if state.NextStage != "prepare_contribution_inputs" && state.NextStage != "compute_contributions" && state.NextStage != "normalize_or_enrich_entities" {
		return nil
	}
	access := dataTaskWorkflowArtifactContractAccess(records)
	if len(access) == 0 {
		access = dataTaskWorkflowArtifactAccess(records, 10)
	}
	if len(access) == 0 {
		return nil
	}
	recordAccess := dataTaskWorkflowRecordActionArtifacts(access)
	if len(recordAccess) == 0 {
		return nil
	}
	allowed := map[string]bool{}
	for _, action := range state.AllowedNextActions {
		allowed[action] = true
	}
	var out []dataTaskActionScaffold
	if allowed[string(dataquery.DataActionDeriveFields)] {
		for _, artifact := range recordAccess {
			if len(out) >= 6 {
				return out
			}
			alias := firstDataTaskArtifactAlias(artifact)
			if alias == "" || len(artifact.Fields) == 0 {
				continue
			}
			if !dataTaskArtifactHasTextExtractionField(artifact.Fields) {
				continue
			}
			out = append(out, dataTaskActionScaffold{
				Kind:      string(dataquery.DataActionDeriveFields),
				UseWhen:   "materialize a derived field on one existing record artifact before filtering, joining, or aggregating",
				InputPath: alias,
				Fields:    clampDataTaskStringSlice(artifact.Fields, 16),
				ParamsTemplate: map[string]string{
					"field_specs_json": `[{"source_field":"<existing field from fields>","source_fields":["<existing field A>","<existing field B>"],"target_field":"<new field>","operation":"parse_number|extract_year|lower|upper|trim|map|regex_extract|concat|coalesce|constant|case_when","separator":" ","cases":[{"filters":[{"field":"<existing field>","op":"eq|in|gt|gte|lt|lte|not_empty","value":"<expected value>"}],"value_field":"<existing field to copy>"}],"default_field":"<optional existing fallback field>","default":"<optional fallback value>"}]`,
				},
				Note: "Use only field names present in fields. concat/coalesce use source_fields; case_when uses existing-field filters plus value/value_field/default. The system executes conditions but does not choose business semantics.",
			})
		}
	}
	if allowed[string(dataquery.DataActionGroupRecords)] {
		for _, artifact := range recordAccess {
			if len(out) >= 8 {
				return out
			}
			alias := firstDataTaskArtifactAlias(artifact)
			if alias == "" || len(artifact.Fields) == 0 || !dataTaskArtifactHasTextExtractionField(artifact.Fields) {
				continue
			}
			out = append(out, dataTaskActionScaffold{
				Kind:      string(dataquery.DataActionGroupRecords),
				UseWhen:   "one logical record is split across multiple rows/spans and later extraction needs neighboring text from the same group",
				InputPath: alias,
				Fields:    clampDataTaskStringSlice(artifact.Fields, 16),
				ParamsTemplate: map[string]string{
					"group_field":  "<existing key field from fields, such as a document/page/message/block id>",
					"text_fields":  `["<existing text-like field from fields>"]`,
					"target_field": "<new grouped text field>",
					"separator":    `\n`,
				},
				Note: "Use only existing group/text fields. The system groups rows and concatenates text; it does not decide business semantics.",
			})
		}
	}
	if allowed[string(dataquery.DataActionExtractFields)] {
		for _, artifact := range recordAccess {
			if len(out) >= 8 {
				return out
			}
			alias := firstDataTaskArtifactAlias(artifact)
			if alias == "" || len(artifact.Fields) == 0 {
				continue
			}
			out = append(out, dataTaskActionScaffold{
				Kind:      string(dataquery.DataActionExtractFields),
				UseWhen:   "materialize structured fields from text or mixed record fields before filtering, joining, or aggregating",
				InputPath: alias,
				Fields:    clampDataTaskStringSlice(artifact.Fields, 16),
				ParamsTemplate: map[string]string{
					"field_specs":     `[{"source_field":"<existing text/source field from fields>","target_field":"<new field>","operation":"regex_extract|parse_number|copy|trim","pattern":"<regex when operation is regex_extract>","group":1}]`,
					"required_fields": `["<new field that must be non-empty for output rows>"]`,
				},
				Note: "Use only existing source_field names from fields. If the source text has multiple numeric tokens, use regex_extract with a context pattern instead of unanchored parse_number. The model supplies the regex or parse specs from observed material shape; the system only executes extraction and preserves source locators.",
			})
		}
	}
	if allowed[string(dataquery.DataActionExpandRecords)] {
		for _, artifact := range recordAccess {
			if len(out) >= 8 {
				return out
			}
			alias := firstDataTaskArtifactAlias(artifact)
			if alias == "" || len(artifact.Fields) == 0 {
				continue
			}
			out = append(out, dataTaskActionScaffold{
				Kind:      string(dataquery.DataActionExpandRecords),
				UseWhen:   "turn one multi-value field on one record artifact into multiple records before enrichment, join, or contribution calculation",
				InputPath: alias,
				Fields:    clampDataTaskStringSlice(artifact.Fields, 16),
				ParamsTemplate: map[string]string{
					"source_field":   "<existing multi-value field from fields>",
					"target_field":   "<expanded value field>",
					"delimiter":      "auto",
					"original_field": "<optional field to preserve the unsplit source value>",
				},
				Note: "Use this for delimited values such as aliases, tags, labels, roles, categories, ids, terms, or other lists. It changes row cardinality; derive_fields does not.",
			})
		}
	}
	if allowed[string(dataquery.DataActionEnrichRecords)] {
		for _, scaffold := range dataTaskWorkflowActionScaffoldViews(dataworkflow.EnrichRecordScaffolds(dataTaskArtifactAccessSchemaProjection(access), 4)) {
			if len(out) >= 8 {
				return out
			}
			out = append(out, scaffold)
		}
	}
	if allowed[string(dataquery.DataActionJoinRecords)] {
		for _, scaffold := range dataTaskWorkflowActionScaffoldViews(dataworkflow.JoinRecordScaffolds(dataTaskArtifactAccessSchemaProjection(recordAccess), 4)) {
			if len(out) >= 8 {
				return out
			}
			out = append(out, scaffold)
		}
	}
	if allowed[string(dataquery.DataActionNormalizeEntities)] {
		for _, scaffold := range dataTaskWorkflowActionScaffoldViews(dataworkflow.NormalizeEntityScaffolds(dataTaskArtifactAccessSchemaProjection(recordAccess), 4)) {
			if len(out) >= 8 {
				return out
			}
			out = append(out, scaffold)
		}
	}
	if allowed[string(dataquery.DataActionApplyResolutions)] {
		for _, scaffold := range dataTaskWorkflowActionScaffoldViews(dataworkflow.ApplyResolutionScaffolds(dataTaskArtifactAccessSchemaProjection(access), 4)) {
			if len(out) >= 8 {
				return out
			}
			out = append(out, scaffold)
		}
	}
	if allowed[string(dataquery.DataActionFilterRecords)] {
		for _, artifact := range recordAccess {
			if len(out) >= 10 {
				return out
			}
			alias := firstDataTaskArtifactAlias(artifact)
			if alias == "" || len(artifact.Fields) == 0 {
				continue
			}
			out = append(out, dataTaskActionScaffold{
				Kind:      string(dataquery.DataActionFilterRecords),
				UseWhen:   "select a smaller reusable record set using fields that already exist on one artifact",
				InputPath: alias,
				Fields:    clampDataTaskStringSlice(artifact.Fields, 18),
				ParamsTemplate: map[string]string{
					"filters_json": `[{"field":"<existing field from fields>","op":"eq|ne|in|not_in|contains|not_contains|empty|not_empty|gt|gte|lt|lte","value":"<value>"}]`,
				},
				Note: "Use only filter field names present in fields. If the desired filter field is missing, first use derive_fields, enrich_records, or join_records to materialize it.",
			})
		}
	}
	if allowed[string(dataquery.DataActionQualifyRecords)] {
		for _, artifact := range recordAccess {
			if len(out) >= 10 {
				return out
			}
			alias := firstDataTaskArtifactAlias(artifact)
			if alias == "" || len(artifact.Fields) == 0 {
				continue
			}
			out = append(out, dataTaskActionScaffold{
				Kind:      string(dataquery.DataActionQualifyRecords),
				UseWhen:   "turn rule/evidence eligibility into auditable include/exclude rows before contribution calculation",
				InputPath: alias,
				Fields:    clampDataTaskStringSlice(artifact.Fields, 20),
				ParamsTemplate: map[string]string{
					"filters":         `[{"field":"<existing field that must pass>","op":"eq|in|not_empty|exists","value":"<value>"}]`,
					"reject_filters":  `[{"field":"<existing field that excludes a row>","op":"eq|in|contains","value":"<value>"}]`,
					"required_fields": `["<existing field that must be non-empty>"]`,
					"evidence_fields": `["<existing evidence/provenance field if the current rules require it>"]`,
					"status_fields":   `["<generated *_status field if relevant>"]`,
					"output_mode":     "filter",
					"item_id_field":   "<existing stable row id field if available>",
				},
				Note: "Use this when records need explicit eligibility decisions before compute_contributions. The model chooses conditions from the current rules; the system only executes typed field/filter/status/evidence checks and emits decision rows.",
			})
		}
	}
	if allowed[string(dataquery.DataActionComputeContribs)] {
		for _, artifact := range recordAccess {
			if len(out) >= 10 {
				return out
			}
			alias := firstDataTaskArtifactAlias(artifact)
			if alias == "" || len(artifact.Fields) == 0 {
				continue
			}
			out = append(out, dataTaskActionScaffold{
				Kind:      string(dataquery.DataActionComputeContribs),
				UseWhen:   "compute a generic contribution ledger from one eligible artifact that already contains the value/group/filter fields",
				InputPath: alias,
				Fields:    clampDataTaskStringSlice(artifact.Fields, 20),
				ParamsTemplate: map[string]string{
					"value_field":     "<existing numeric value field, or omit for count>",
					"group_key_field": "<existing grouping field, or use group_key for a constant group>",
					"filters_json":    `[{"field":"<existing field from fields>","op":"eq|in|not_in|contains|exists","value":"<value>"}]`,
					"operation":       "add|count",
					"metric":          "<metric name>",
					"item_id_field":   "<existing stable row id field if available>",
				},
				Note: "Do not invent value/group/filter fields; materialize missing fields with derive_fields, enrich_records, or join_records first. If rule/evidence eligibility is not already decided, run qualify_records before this action.",
			})
		}
	}
	return out
}

func dataTaskWorkflowActionScaffoldViews(scaffolds []dataworkflow.ActionScaffold) []dataTaskActionScaffold {
	out := make([]dataTaskActionScaffold, 0, len(scaffolds))
	for _, scaffold := range scaffolds {
		out = append(out, dataTaskActionScaffold{
			Kind:           strings.TrimSpace(scaffold.Kind),
			UseWhen:        strings.TrimSpace(scaffold.UseWhen),
			InputPath:      strings.TrimSpace(scaffold.InputPath),
			InputPaths:     cleanDataTaskStrings(scaffold.InputPaths),
			Fields:         cleanDataTaskStrings(scaffold.Fields),
			CommonFields:   cleanDataTaskStrings(scaffold.CommonFields),
			ParamsTemplate: scaffold.ParamsTemplate,
			Note:           strings.TrimSpace(scaffold.Note),
		})
	}
	return out
}

func dataTaskWorkflowRecordActionArtifacts(access []dataTaskArtifactAccessPrompt) []dataTaskArtifactAccessPrompt {
	var out []dataTaskArtifactAccessPrompt
	for _, artifact := range access {
		if dataTaskArtifactUsableForRecordAction(artifact) {
			out = append(out, artifact)
		}
	}
	return out
}

func dataTaskArtifactUsableForRecordAction(artifact dataTaskArtifactAccessPrompt) bool {
	return dataworkflow.ArtifactUsableForRecordAction(dataTaskArtifactAccessToSchemaProjection(artifact))
}

func dataTaskArtifactAccessToSchemaProjection(artifact dataTaskArtifactAccessPrompt) dataworkflow.ArtifactSchemaProjection {
	return dataworkflow.ArtifactSchemaProjection{
		ID:          strings.TrimSpace(artifact.ID),
		Kind:        strings.TrimSpace(artifact.Kind),
		NodeClass:   strings.TrimSpace(artifact.NodeClass),
		Aliases:     cleanDataTaskStrings(artifact.Aliases),
		JSONShape:   strings.TrimSpace(artifact.JSONShape),
		Fields:      cleanDataTaskStrings(artifact.Fields),
		AccessHint:  strings.TrimSpace(artifact.AccessHint),
		SourcePaths: cleanDataTaskStrings(artifact.SourcePaths),
	}
}

func dataTaskArtifactHasTextExtractionField(fields []string) bool {
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || strings.HasPrefix(lower, "_") {
			continue
		}
		switch {
		case lower == "text" || lower == "content" || lower == "body" || lower == "message" || lower == "line":
			return true
		case strings.Contains(lower, "text") || strings.Contains(lower, "content") || strings.Contains(lower, "message") || strings.Contains(lower, "description"):
			return true
		case strings.HasSuffix(lower, "_raw") || lower == "raw":
			return true
		}
	}
	return false
}

func dataTaskArtifactIsDiagnosticChild(artifact dataTaskArtifactAccessPrompt) bool {
	return strings.TrimSpace(artifact.NodeClass) == dataworkflow.ArtifactNodeClassDiagnosticChild ||
		dataworkflow.IsDiagnosticChildArtifact(artifact.ID, artifact.Kind)
}

func dataTaskArtifactIsWorkflowLedgerHandle(artifact dataTaskArtifactAccessPrompt) bool {
	return strings.TrimSpace(artifact.NodeClass) == dataworkflow.ArtifactNodeClassWorkflowLedger ||
		dataworkflow.IsWorkflowLedgerArtifact(artifact.ID, artifact.Kind, nil)
}

func dataTaskApplyResolutionActionScaffolds(access []dataTaskArtifactAccessPrompt, limit int) []dataTaskActionScaffold {
	if limit <= 0 {
		return nil
	}
	var mappings []dataTaskArtifactAccessPrompt
	for _, artifact := range access {
		if dataTaskArtifactIsWorkflowLedgerHandle(artifact) || dataTaskArtifactIsDiagnosticChild(artifact) {
			continue
		}
		if dataTaskArtifactLooksLikeApplyResolutionLedger(artifact) && firstDataTaskExistingField(artifact.Fields, "item_id", "source_line", "source_index", "canonical_id", "canonical_label") != "" {
			mappings = append(mappings, artifact)
		}
	}
	if len(mappings) == 0 {
		return nil
	}
	var out []dataTaskActionScaffold
	for _, base := range access {
		baseAlias := firstDataTaskArtifactAlias(base)
		if baseAlias == "" || len(base.Fields) == 0 || dataTaskArtifactIsDiagnosticChild(base) || dataTaskArtifactLooksLikeApplyResolutionLedger(base) {
			continue
		}
		for _, mapping := range mappings {
			mappingAlias := firstDataTaskArtifactAlias(mapping)
			if mappingAlias == "" || mappingAlias == baseAlias {
				continue
			}
			if !dataTaskApplyResolutionBaseCompatibleWithMapping(base, mapping) {
				continue
			}
			targetPrefix := dataTaskResolutionTargetFieldPrefix(mappingAlias)
			targetFields := []string{
				targetPrefix + "_canonical_id",
				targetPrefix + "_canonical_label",
				targetPrefix + "_resolution_status",
			}
			if dataTaskArtifactHasAnyNamedField(base, targetFields...) && dataTaskArtifactLineageContains(base, mappingAlias) {
				continue
			}
			resolutionSpec := []map[string]any{{
				"resolution_path":       mappingAlias,
				"target_id_field":       targetPrefix + "_canonical_id",
				"target_label_field":    targetPrefix + "_canonical_label",
				"target_status_field":   targetPrefix + "_resolution_status",
				"unmatched_status":      "unmatched",
				"resolution_key_fields": []string{"item_id"},
			}}
			if existingIDField := dataTaskApplyResolutionExistingIDField(base.Fields, targetPrefix); existingIDField != "" {
				if reference, ok := dataTaskApplyResolutionReferenceArtifact(access, baseAlias, mappingAlias, existingIDField, mapping); ok {
					spec := resolutionSpec[0]
					spec["existing_id_field"] = existingIDField
					spec["reference_path"] = firstDataTaskArtifactAlias(reference)
					spec["reference_id_field"] = existingIDField
					if labelField := dataTaskApplyResolutionReferenceLabelField(reference.Fields, existingIDField); labelField != "" {
						spec["reference_label_field"] = labelField
					}
				}
			}
			out = append(out, dataTaskActionScaffold{
				Kind:       string(dataquery.DataActionApplyResolutions),
				UseWhen:    "apply an existing entity_resolution ledger back onto base rows before filtering, joining, or contribution calculation",
				InputPaths: []string{baseAlias, mappingAlias},
				Fields:     clampDataTaskStringSlice(base.Fields, 20),
				ParamsTemplate: map[string]string{
					"base_path":          baseAlias,
					"base_filter_mode":   "preserve",
					"preserve_base_rows": "true",
					"resolution_specs":   mustCompactJSONString(resolutionSpec),
				},
				Note: "Use this after normalize_entities when base rows need canonical fields from entity_resolution records. It preserves all base rows by default; base/source filters only choose rows eligible for applying mappings. Use filter_records or base_filter_mode=filter_output only when shrinking rows is intentional. If a workflow ledger contains multiple dimensions or source fields, filter the resolution side for the intended target field. If key fields are omitted, the runner uses base _source_index and parses #N from resolution item_id/source_locator. If the template includes existing_id_field/reference_path/reference_id_field, that value is used only after structural verification against the reference artifact. This action applies existing mappings; it does not decide new mappings.",
			})
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func dataTaskArtifactLooksLikeApplyResolutionLedger(artifact dataTaskArtifactAccessPrompt) bool {
	if dataTaskArtifactLooksLikeEntityResolutionLedger(artifact) {
		return true
	}
	return dataTaskArtifactLooksLikeGeneratedEntityMapping(artifact) &&
		firstDataTaskExistingField(artifact.Fields, "item_id", "source_locator", "_source_locator", "source_line", "source_index") != ""
}

func dataTaskResolutionTargetFieldPrefix(alias string) string {
	value := strings.TrimSpace(alias)
	for _, suffix := range []string{".json", ".jsonl", ".csv", ".tsv", ".txt"} {
		if strings.HasSuffix(strings.ToLower(value), suffix) {
			value = strings.TrimSuffix(value, value[len(value)-len(suffix):])
			break
		}
	}
	value = dataTaskSafeActionName(value)
	for {
		next := value
		for _, suffix := range []string{
			"_entity_resolutions",
			"_entity_resolution",
			"_resolutions",
			"_resolution",
			"_entity_mappings",
			"_entity_mapping",
			"_normalizations",
			"_normalization",
			"_normalized",
			"_mappings",
			"_mapping",
			"_records",
			"_record",
		} {
			if strings.HasSuffix(next, suffix) && len(next) > len(suffix) {
				next = strings.TrimSuffix(next, suffix)
				break
			}
		}
		if next == value {
			break
		}
		value = next
	}
	value = strings.Trim(value, "_")
	if value == "" {
		return "resolved_entity"
	}
	return value
}

func dataTaskApplyResolutionExistingIDField(fields []string, targetPrefix string) string {
	prefix := strings.Trim(dataTaskSafeActionName(targetPrefix), "_")
	if prefix == "" {
		return ""
	}
	var candidates []string
	add := func(value string) {
		value = strings.Trim(value, "_")
		if value != "" {
			candidates = append(candidates, value)
		}
	}
	add(prefix + "_id")
	add(prefix + "_code")
	for _, suffix := range []string{"_canonical", "_entity", "_dimension"} {
		if strings.HasSuffix(prefix, suffix) && len(prefix) > len(suffix) {
			base := strings.TrimSuffix(prefix, suffix)
			add(base + "_id")
			add(base + "_code")
		}
	}
	return firstDataTaskExistingField(fields, candidates...)
}

func dataTaskApplyResolutionReferenceArtifact(access []dataTaskArtifactAccessPrompt, baseAlias, mappingAlias, existingIDField string, mapping dataTaskArtifactAccessPrompt) (dataTaskArtifactAccessPrompt, bool) {
	sourceHints := map[string]int{}
	for i, source := range mapping.SourcePaths {
		source = normalizeDataTaskCoveragePath(source)
		if source != "" {
			sourceHints[source] = 100 - i
		}
	}
	bestScore := -1
	var best dataTaskArtifactAccessPrompt
	for _, artifact := range access {
		alias := firstDataTaskArtifactAlias(artifact)
		if alias == "" || normalizeDataTaskCoveragePath(alias) == normalizeDataTaskCoveragePath(baseAlias) || normalizeDataTaskCoveragePath(alias) == normalizeDataTaskCoveragePath(mappingAlias) {
			continue
		}
		if len(artifact.Fields) == 0 || dataTaskArtifactLooksLikeApplyResolutionLedger(artifact) {
			continue
		}
		if firstDataTaskExistingField(artifact.Fields, existingIDField) == "" {
			continue
		}
		score := 1
		if sourceHints[normalizeDataTaskCoveragePath(alias)] > score {
			score = sourceHints[normalizeDataTaskCoveragePath(alias)]
		}
		for _, candidate := range artifact.Aliases {
			if sourceHints[normalizeDataTaskCoveragePath(candidate)] > score {
				score = sourceHints[normalizeDataTaskCoveragePath(candidate)]
			}
		}
		if score > bestScore {
			bestScore = score
			best = artifact
		}
	}
	return best, bestScore >= 0
}

func dataTaskApplyResolutionReferenceLabelField(fields []string, existingIDField string) string {
	base := strings.TrimSpace(existingIDField)
	lower := strings.ToLower(base)
	var candidates []string
	if strings.HasSuffix(lower, "_id") && len(base) > 3 {
		candidates = append(candidates, base[:len(base)-3]+"_label", base[:len(base)-3]+"_name")
	}
	if strings.HasSuffix(lower, "_code") && len(base) > 5 {
		candidates = append(candidates, base[:len(base)-5]+"_label", base[:len(base)-5]+"_name")
	}
	candidates = append(candidates, "canonical_label", "label", "name", "display_name", "title")
	return firstDataTaskExistingField(fields, candidates...)
}

func dataTaskEnrichActionScaffolds(access []dataTaskArtifactAccessPrompt, limit int) []dataTaskActionScaffold {
	if limit <= 0 {
		return nil
	}
	var mappings []dataTaskArtifactAccessPrompt
	for _, artifact := range access {
		if dataTaskArtifactLooksLikeMapping(artifact) {
			mappings = append(mappings, artifact)
		}
	}
	if len(mappings) == 0 {
		return nil
	}
	var out []dataTaskActionScaffold
	for _, base := range access {
		baseAlias := firstDataTaskArtifactAlias(base)
		if baseAlias == "" || len(base.Fields) == 0 || dataTaskArtifactLooksLikeMapping(base) {
			continue
		}
		for _, mapping := range mappings {
			mappingAlias := firstDataTaskArtifactAlias(mapping)
			if mappingAlias == "" || mappingAlias == baseAlias {
				continue
			}
			valueField := dataTaskReferenceValueField(mapping.Fields)
			if valueField == "" {
				continue
			}
			sourceFields := dataTaskCandidateSourceFields(base.Fields, 4)
			if len(sourceFields) == 0 {
				sourceFields = []string{"<field from the base artifact fields>"}
			}
			mappingSourceFields := dataTaskCandidateReferenceSourceFields(mapping.Fields, valueField, 6)
			if len(mappingSourceFields) == 0 {
				mappingSourceFields = []string{"source_value"}
			}
			out = append(out, dataTaskActionScaffold{
				Kind:       string(dataquery.DataActionEnrichRecords),
				UseWhen:    "apply mapping/reference rows back onto base records while preserving the base record fields",
				InputPaths: []string{baseAlias, mappingAlias},
				Fields:     clampDataTaskStringSlice(base.Fields, 20),
				ParamsTemplate: map[string]string{
					"base_path":    baseAlias,
					"lookup_specs": fmt.Sprintf(`[{"lookup_path":%q,"base_fields":%s,"lookup_fields":%s,"lookup_value_field":%q,"target_field":"<new canonical/reference field>","match_mode":"exact|contains|mapping_contains_source|source_contains_mapping"}]`, mappingAlias, mustCompactJSONString(sourceFields), mustCompactJSONString(mappingSourceFields), valueField),
				},
				Note: "Use this after normalize_entities or reference-table extraction when later joins/filters/aggregation need canonical/reference fields on original base rows. base_fields must exist on the base artifact; lookup_fields and lookup_value_field must exist on the lookup/reference artifact. Do not feed lookup rows directly into compute_contributions unless they are the task items.",
			})
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func dataTaskNormalizeEntityActionScaffolds(access []dataTaskArtifactAccessPrompt, limit int) []dataTaskActionScaffold {
	if limit <= 0 {
		return nil
	}
	type candidate struct {
		scaffold dataTaskActionScaffold
		score    int
		order    int
	}
	var candidates []candidate
	order := 0
	for _, source := range access {
		sourceAlias := firstDataTaskArtifactAlias(source)
		if sourceAlias == "" || len(source.Fields) == 0 || dataTaskArtifactLooksLikeGeneratedEntityMapping(source) {
			continue
		}
		sourceFields := dataTaskCandidateSourceFields(source.Fields, 4)
		if len(sourceFields) == 0 {
			continue
		}
		for _, reference := range access {
			refAlias := firstDataTaskArtifactAlias(reference)
			if refAlias == "" || refAlias == sourceAlias || len(reference.Fields) == 0 {
				continue
			}
			canonicalIDField := dataTaskReferenceValueField(reference.Fields)
			if canonicalIDField == "" {
				continue
			}
			canonicalLabelField := dataTaskReferenceLabelField(reference.Fields, canonicalIDField)
			referenceFields := dataTaskCandidateReferenceSourceFields(reference.Fields, canonicalIDField, 8)
			if len(referenceFields) == 0 {
				continue
			}
			scaffold := dataTaskActionScaffold{
				Kind:       string(dataquery.DataActionNormalizeEntities),
				UseWhen:    "build a source-to-reference mapping ledger before enriching, joining, or aggregating records",
				InputPaths: []string{sourceAlias, refAlias},
				Fields:     clampDataTaskStringSlice(source.Fields, 18),
				ParamsTemplate: map[string]string{
					"source_field":          sourceFields[0],
					"reference_name_fields": mustCompactJSONString(referenceFields),
					"canonical_id_field":    canonicalIDField,
					"canonical_label_field": canonicalLabelField,
					"match_mode":            "exact|mapping_contains_source|source_contains_mapping|contains",
				},
				Note: "Use this when one record set has raw source values and another record set has canonical/reference values. Choose source/reference fields from real fields, then feed this mapping artifact into enrich_records.",
			}
			candidates = append(candidates, candidate{
				scaffold: scaffold,
				score:    dataTaskNormalizeEntityScaffoldScore(source.Fields, reference.Fields, sourceFields, referenceFields, canonicalIDField, canonicalLabelField),
				order:    order,
			})
			order++
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].order < candidates[j].order
	})
	out := make([]dataTaskActionScaffold, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.scaffold)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func dataTaskNormalizeEntityScaffoldScore(sourceFields, referenceFields, chosenSourceFields, chosenReferenceFields []string, canonicalIDField, canonicalLabelField string) int {
	score := 0
	baseHints := dataTaskCanonicalFieldBaseHints(canonicalIDField)
	if dataTaskFieldsHaveRawCue(chosenSourceFields) {
		score += 30
	}
	if dataTaskFieldsHaveRawCue(sourceFields) {
		score += 10
	}
	if dataTaskFieldsHaveBaseRawCue(sourceFields, baseHints) {
		score += 80
		if firstDataTaskExistingField(sourceFields, canonicalIDField) != "" {
			score += 40
		}
	}
	if firstDataTaskExistingField(referenceFields, canonicalIDField) != "" {
		score += 20
	}
	if strings.TrimSpace(canonicalLabelField) != "" {
		score += 20
	}
	if len(chosenReferenceFields) > 1 {
		score += 5
	}
	if dataTaskFieldsHaveBaseRawCue(referenceFields, baseHints) {
		score -= 60
	}
	if dataTaskFieldsHaveRawCue(referenceFields) {
		score -= 10
	}
	return score
}

func dataTaskCanonicalFieldBaseHints(field string) []string {
	lower := strings.ToLower(strings.TrimSpace(field))
	if lower == "" {
		return nil
	}
	candidates := []string{lower}
	for _, suffix := range []string{"_id", "_code", "_key", "_value", "_canonical_id", "_canonical_code"} {
		if strings.HasSuffix(lower, suffix) && len(lower) > len(suffix) {
			candidates = append(candidates, strings.TrimSuffix(lower, suffix))
		}
	}
	out := cleanDataTaskStrings(candidates)
	sort.Strings(out)
	return out
}

func dataTaskFieldsHaveRawCue(fields []string) bool {
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" {
			continue
		}
		if lower == "raw" || strings.HasSuffix(lower, "_raw") || strings.Contains(lower, "raw_") || strings.Contains(lower, "free_text") {
			return true
		}
	}
	return false
}

func dataTaskFieldsHaveBaseRawCue(fields []string, bases []string) bool {
	if len(bases) == 0 {
		return false
	}
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" {
			continue
		}
		for _, base := range bases {
			base = strings.TrimSpace(base)
			if base == "" {
				continue
			}
			if strings.Contains(lower, base) && (strings.Contains(lower, "raw") || strings.Contains(lower, "source") || strings.Contains(lower, "input")) {
				return true
			}
		}
	}
	return false
}

func dataTaskArtifactLooksLikeGeneratedEntityMapping(artifact dataTaskArtifactAccessPrompt) bool {
	kind := strings.ToLower(strings.TrimSpace(artifact.Kind))
	if strings.Contains(kind, "normalize_entities") || strings.Contains(kind, "entity_resolution") {
		return true
	}
	fields := dataTaskFieldSet(artifact.Fields)
	hasSource := fields["source_value"] != "" || fields["source_field"] != ""
	hasCanonical := fields["canonical_id"] != "" || fields["canonical_label"] != "" || fields["canonical_value"] != ""
	return hasSource && hasCanonical
}

func dataTaskArtifactLooksLikeMapping(artifact dataTaskArtifactAccessPrompt) bool {
	kind := strings.ToLower(strings.TrimSpace(artifact.Kind))
	if strings.Contains(kind, "normalize_entities") || strings.Contains(kind, "entity_resolution") {
		return true
	}
	fields := dataTaskFieldSet(artifact.Fields)
	if fields["source_value"] != "" && (fields["canonical_id"] != "" || fields["canonical_label"] != "") {
		return true
	}
	if dataTaskReferenceValueField(artifact.Fields) != "" && len(dataTaskCandidateReferenceSourceFields(artifact.Fields, dataTaskReferenceValueField(artifact.Fields), 3)) > 0 {
		return true
	}
	return false
}

func dataTaskArtifactLooksLikeEntityResolutionLedger(artifact dataTaskArtifactAccessPrompt) bool {
	fields := dataTaskFieldSet(artifact.Fields)
	hasLocator := fields["item_id"] != "" || fields["source_locator"] != "" || fields["_source_locator"] != "" || fields["record_id"] != "" || fields["row_id"] != ""
	if !hasLocator {
		return false
	}
	hasSource := fields["source_value"] != "" || fields["source_field"] != "" || fields["evidence_refs"] != ""
	hasCanonical := fields["canonical_id"] != "" || fields["canonical_label"] != "" || fields["canonical_value"] != ""
	return hasSource && hasCanonical
}

func dataTaskReferenceLabelField(fields []string, idField string) string {
	idLower := strings.ToLower(strings.TrimSpace(idField))
	best := ""
	bestScore := -1
	for i, field := range fields {
		trimmed := strings.TrimSpace(field)
		lower := strings.ToLower(trimmed)
		if trimmed == "" || strings.HasPrefix(lower, "_") || lower == idLower {
			continue
		}
		score := 0
		switch {
		case lower == "label" || lower == "name" || lower == "title":
			score = 100
		case strings.Contains(lower, "label") || strings.Contains(lower, "name") || strings.Contains(lower, "title"):
			score = 80
		case strings.Contains(lower, "description") || strings.Contains(lower, "text"):
			score = 40
		default:
			score = 1
		}
		if score > bestScore || (score == bestScore && best == "" && i == 0) {
			best = trimmed
			bestScore = score
		}
	}
	return best
}

func dataTaskReferenceValueField(fields []string) string {
	if field := firstDataTaskExistingField(fields, "canonical_id", "canonical_label", "value", "target_value", "code"); field != "" {
		return field
	}
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || strings.HasPrefix(lower, "_") {
			continue
		}
		if strings.Contains(lower, "canonical") && (strings.Contains(lower, "id") || strings.Contains(lower, "code") || strings.Contains(lower, "value")) {
			return strings.TrimSpace(field)
		}
	}
	for _, field := range fields {
		lower := strings.ToLower(strings.TrimSpace(field))
		if lower == "" || strings.HasPrefix(lower, "_") {
			continue
		}
		if strings.HasSuffix(lower, "_id") || strings.HasSuffix(lower, "_code") {
			return strings.TrimSpace(field)
		}
	}
	return ""
}

func dataTaskCandidateSourceFields(fields []string, limit int) []string {
	var out []string
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		lower := strings.ToLower(trimmed)
		if trimmed == "" || strings.HasPrefix(lower, "_") || lower == "amount" || lower == "year" {
			continue
		}
		if strings.Contains(lower, "name") || strings.Contains(lower, "raw") || strings.Contains(lower, "description") || strings.Contains(lower, "purpose") || strings.Contains(lower, "label") || strings.Contains(lower, "text") || strings.Contains(lower, "title") {
			out = append(out, trimmed)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func dataTaskCandidateReferenceSourceFields(fields []string, valueField string, limit int) []string {
	var out []string
	valueLower := strings.ToLower(strings.TrimSpace(valueField))
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		lower := strings.ToLower(trimmed)
		if trimmed == "" || strings.HasPrefix(lower, "_") || lower == valueLower {
			continue
		}
		if strings.Contains(lower, "negative") || strings.Contains(lower, "exclude") || strings.Contains(lower, "except") {
			continue
		}
		if strings.Contains(lower, "source") || strings.Contains(lower, "term") || strings.Contains(lower, "name") || strings.Contains(lower, "label") || strings.Contains(lower, "keyword") || strings.Contains(lower, "example") || strings.Contains(lower, "description") || strings.Contains(lower, "text") {
			out = append(out, trimmed)
			if len(out) >= limit {
				return out
			}
		}
	}
	if len(out) == 0 {
		for _, field := range fields {
			trimmed := strings.TrimSpace(field)
			lower := strings.ToLower(trimmed)
			if trimmed == "" || strings.HasPrefix(lower, "_") || lower == valueLower {
				continue
			}
			if strings.Contains(lower, "negative") || strings.Contains(lower, "exclude") || strings.Contains(lower, "except") {
				continue
			}
			out = append(out, trimmed)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func firstDataTaskExistingField(fields []string, candidates ...string) string {
	set := dataTaskFieldSet(fields)
	for _, candidate := range candidates {
		if field := set[strings.ToLower(strings.TrimSpace(candidate))]; field != "" {
			return field
		}
	}
	return ""
}

func mustCompactJSONString(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func dataTaskWorkflowArtifactAccess(records []dataTaskWorkflowRecord, limit int) []dataTaskArtifactAccessPrompt {
	artifacts := dataTaskWorkflowArtifactsNewestFirst(records)
	return sampleDataTaskArtifactAccess(artifacts, limit)
}

func dataTaskWorkflowArtifactContractAccess(records []dataTaskWorkflowRecord) []dataTaskArtifactAccessPrompt {
	artifacts := dataTaskWorkflowArtifactsNewestFirst(records)
	if len(artifacts) == 0 {
		return nil
	}
	projections := dataworkflow.ProjectArtifactSchemasNewestFirst(artifacts)
	out := make([]dataTaskArtifactAccessPrompt, 0, len(projections))
	for _, projection := range projections {
		out = append(out, dataTaskArtifactAccessPrompt{
			ID:          projection.ID,
			Kind:        projection.Kind,
			NodeClass:   projection.NodeClass,
			Aliases:     projection.Aliases,
			JSONShape:   projection.JSONShape,
			Fields:      projection.Fields,
			AccessHint:  projection.AccessHint,
			SourcePaths: projection.SourcePaths,
		})
	}
	return out
}

func dataTaskWorkflowArtifactAvailability(records []dataTaskWorkflowRecord, limit int) ([]dataTaskArtifactAccessPrompt, int, bool) {
	artifacts := dataTaskWorkflowArtifactsNewestFirst(records)
	total := dataTaskFlattenedArtifactCount(artifacts)
	access := sampleDataTaskArtifactAccess(artifacts, limit)
	for i := range access {
		access[i].FieldSamples = nil
	}
	return access, total, total > len(access)
}

func dataTaskWorkflowArtifactsNewestFirst(records []dataTaskWorkflowRecord) []dataquery.DataArtifact {
	var artifacts []dataquery.DataArtifact
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if rec.Result == nil {
			continue
		}
		// ActionRunner appends seed artifacts first and the freshly
		// materialized action artifact last. Present newest artifacts first
		// so compact prompts expose the just-created dependency instead of
		// burying it behind old seed rows.
		for j := len(rec.Result.Artifacts) - 1; j >= 0; j-- {
			artifacts = append(artifacts, rec.Result.Artifacts[j])
		}
	}
	return artifacts
}

func dataTaskFlattenedArtifactCount(artifacts []dataquery.DataArtifact) int {
	return dataworkflow.FlattenedArtifactCount(artifacts)
}

func firstDataTaskArtifactAlias(artifact dataTaskArtifactAccessPrompt) string {
	for _, alias := range artifact.Aliases {
		alias = strings.TrimSpace(alias)
		if alias != "" {
			return alias
		}
	}
	return strings.TrimSpace(artifact.ID)
}

func dataTaskJoinActionScaffolds(access []dataTaskArtifactAccessPrompt, limit int) []dataTaskActionScaffold {
	if limit <= 0 {
		return nil
	}
	var out []dataTaskActionScaffold
	for i := 0; i < len(access); i++ {
		leftAlias := firstDataTaskArtifactAlias(access[i])
		if leftAlias == "" || len(access[i].Fields) == 0 {
			continue
		}
		leftFields := dataTaskFieldSet(access[i].Fields)
		for j := i + 1; j < len(access); j++ {
			rightAlias := firstDataTaskArtifactAlias(access[j])
			if rightAlias == "" || len(access[j].Fields) == 0 {
				continue
			}
			common := dataTaskCommonFields(leftFields, access[j].Fields, 8)
			if len(common) == 0 {
				continue
			}
			out = append(out, dataTaskActionScaffold{
				Kind:         string(dataquery.DataActionJoinRecords),
				UseWhen:      "join two generated record artifacts before later derivation or contribution calculation",
				InputPaths:   []string{leftAlias, rightAlias},
				CommonFields: common,
				ParamsTemplate: map[string]string{
					"left_fields":  `["<field from common_fields or another field present on the left artifact>"]`,
					"right_fields": `["<matching field present on the right artifact>"]`,
					"join_type":    "inner|left",
					"collision":    "prefix",
				},
				Note: "Join fields are side-specific. A left field must exist on the left artifact, and a right field must exist on the right artifact. join_type=inner keeps only matched rows; join_type=left preserves all left rows for later diagnostics or filtering. Prefer enrich_records when applying lookup/reference values onto base rows.",
			})
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func dataTaskFieldSet(fields []string) map[string]string {
	out := map[string]string{}
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		key := strings.ToLower(trimmed)
		if key != "" && out[key] == "" {
			out[key] = trimmed
		}
	}
	return out
}

func dataTaskCommonFields(left map[string]string, right []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, field := range right {
		key := strings.ToLower(strings.TrimSpace(field))
		if key == "" || seen[key] || left[key] == "" || !dataworkflow.FieldUsableForRecordJoin(field) || !dataworkflow.FieldUsableForRecordJoin(left[key]) {
			continue
		}
		seen[key] = true
		out = append(out, left[key])
		if len(out) >= limit {
			break
		}
	}
	sort.Strings(out)
	return out
}

func dataTaskActionKindsFromContracts(contracts []dataTaskActionContract) []string {
	out := make([]string, 0, len(contracts))
	for _, contract := range contracts {
		if contract.Kind != "" {
			out = append(out, contract.Kind)
		}
	}
	return out
}

func dataTaskFilterCustomTransformContracts(contracts []dataTaskActionContract) []dataTaskActionContract {
	out := make([]dataTaskActionContract, 0, len(contracts))
	for _, contract := range contracts {
		if contract.Kind == string(dataquery.DataActionCustomTransform) {
			continue
		}
		out = append(out, contract)
	}
	return out
}

func dataTaskCustomTransformCooldown(records []dataTaskWorkflowRecord) bool {
	total, _, topCodeCount := dataTaskCustomTransformFailureClassStats(records)
	if total >= DefaultDataTaskMaxCustomTransformClassFailures || topCodeCount >= DefaultDataTaskMaxNodeFailures {
		return true
	}
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if strings.TrimSpace(rec.Err) != "" && dataTaskRecordHasCustomTransformScript(rec) {
			return true
		}
		if strings.TrimSpace(rec.Err) == "" && dataTaskRecordHasPostScriptTypedProgress(rec) {
			return false
		}
	}
	return false
}

func dataTaskCustomTransformCooldownForState(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView) bool {
	if dataTaskCustomTransformCooldown(records) {
		return true
	}
	if dataTaskWorkflowShouldPreferTypedActions(state) {
		return true
	}
	total, _, _ := dataTaskCustomTransformFailureClassStats(records)
	if total == 0 {
		return false
	}
	// Once a broad custom transform has failed in a ledgered workflow, keep
	// the remaining validation DAG on typed actions until only final answer
	// projection remains. A later successful typed node is progress, but it
	// should not reopen the same broad-script escape hatch while contribution
	// or reconcile ledgers are still structurally missing.
	for _, missing := range dataTaskWorkflowMissingValidationStages(state) {
		if missing != "final_answer" {
			return true
		}
	}
	return false
}

func dataTaskWorkflowShouldPreferTypedActions(state dataTaskWorkflowStateView) bool {
	if state.NextStage == "" || state.NextStage == "complete" || state.NextStage == "emit_output_contract_answer" {
		return false
	}
	if !(state.DecisionRecordsRequired || state.EntityResolutionRequired || state.ContributionLedgerRequired || state.ReconcileRequired) {
		return false
	}
	for _, missing := range dataTaskWorkflowMissingValidationStages(state) {
		if missing != "final_answer" {
			return true
		}
	}
	return false
}

func dataTaskRecordHasPostScriptTypedProgress(rec dataTaskWorkflowRecord) bool {
	if rec.Result == nil || dataTaskRecordHasCustomTransformScript(rec) {
		return false
	}
	if len(rec.Result.Contributions) > 0 || len(rec.Result.EntityResolutions) > 0 || rec.Result.Reconcile != nil {
		return true
	}
	for _, action := range rec.Plan.Actions {
		switch normalizeDataActionKindForWorkflow(action.Kind) {
		case dataquery.DataActionDeriveFields,
			dataquery.DataActionExtractFields,
			dataquery.DataActionGroupRecords,
			dataquery.DataActionExpandRecords,
			dataquery.DataActionFilterRecords,
			dataquery.DataActionQualifyRecords,
			dataquery.DataActionNormalizeEntities,
			dataquery.DataActionApplyResolutions,
			dataquery.DataActionEnrichRecords,
			dataquery.DataActionJoinRecords,
			dataquery.DataActionComputeContribs,
			dataquery.DataActionReconcile,
			dataquery.DataActionAssembleAnswer:
			return true
		}
	}
	return false
}

func dataTaskResultIsFinalAnswerCandidate(plan dataquery.TaskPlan, result dataquery.Result, contract dataquery.CoverageContract, expected dataquery.OutputContract) bool {
	if strings.TrimSpace(result.Answer) == "" {
		return false
	}
	if dataquery.AnswerLooksLikeArtifactSummary(result.Answer) {
		return false
	}
	if plan.ContinueAfter {
		return false
	}
	if !dataTaskPlanMayProduceFinalAnswer(plan, result) {
		return false
	}
	if expected.Format != "" {
		actual := result.OutputContract.Normalize()
		want := expected.Normalize()
		if actual.Format == dataquery.OutputFreeform && want.Format != dataquery.OutputFreeform {
			return false
		}
	}
	if contract.RuleCoverageRequired && len(result.RuleCoverage) == 0 {
		return false
	}
	if contract.DecisionRecordsRequired && len(result.Rows) == 0 {
		return false
	}
	if contract.EntityResolutionRequired && len(result.EntityResolutions) == 0 {
		return false
	}
	if contract.ContributionLedgerRequired && len(result.Contributions) == 0 {
		return false
	}
	if contract.ReconcileRequired && result.Reconcile == nil {
		return false
	}
	return true
}

func dataTaskPlanMayProduceFinalAnswer(plan dataquery.TaskPlan, result dataquery.Result) bool {
	if len(plan.Actions) == 0 {
		return true
	}
	if result.Reconcile != nil && (strings.TrimSpace(result.Reconcile.ActualAnswer.String()) != "" || strings.TrimSpace(result.Reconcile.ExpectedAnswer.String()) != "") {
		return true
	}
	for _, action := range plan.Actions {
		switch normalizeDataActionKindForWorkflow(action.Kind) {
		case dataquery.DataActionCustomTransform:
			if strings.TrimSpace(action.Script) != "" {
				return true
			}
		case dataquery.DataActionReconcile:
			return true
		}
	}
	return false
}

func dataTaskWorkflowOutputContract(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataquery.OutputContract {
	var values []dataquery.OutputContract
	for _, rec := range records {
		values = append(values, rec.Plan.OutputContract)
		if rec.Result != nil {
			values = append(values, rec.Result.OutputContract)
		}
	}
	values = append(values, current.OutputContract)
	return firstNonEmptyOutputContract(values...)
}

func dataTaskWorkflowNextStage(state dataTaskWorkflowStateView) string {
	if !state.MaterialCoverageSufficient {
		return "cover_required_materials"
	}
	if state.RuleCoverageRequired && state.RuleCoverageRecords == 0 {
		return "derive_rules"
	}
	if state.EntityResolutionRequired && state.EntityResolutionRecords == 0 && !state.EntityStageMaterialized {
		return "normalize_or_enrich_entities"
	}
	if state.ContributionLedgerRequired && state.ContributionRecords == 0 {
		return "prepare_contribution_inputs"
	}
	if state.ReconcileRequired && !state.HasReconcile {
		return "reconcile_artifacts"
	}
	if !state.HasAnswer {
		if !state.RuleCoverageRequired && !state.EntityResolutionRequired && !state.ContributionLedgerRequired && !state.ReconcileRequired {
			return "compute_contributions"
		}
		return "emit_output_contract_answer"
	}
	return "complete"
}

func dataTaskWorkflowAllowedNextActions(state dataTaskWorkflowStateView) []string {
	contracts := dataTaskWorkflowAllowedNextActionContracts(state)
	out := make([]string, 0, len(contracts))
	for _, contract := range contracts {
		if contract.Kind != "" {
			out = append(out, contract.Kind)
		}
	}
	return out
}

func dataTaskWorkflowAllowedNextActionContracts(state dataTaskWorkflowStateView) []dataTaskActionContract {
	contract := func(kind dataquery.DataActionKind, boundary, useWhen, output string) dataTaskActionContract {
		return dataTaskActionContract{
			Kind:          string(kind),
			InputBoundary: boundary,
			UseWhen:       useWhen,
			Output:        output,
		}
	}
	switch state.NextStage {
	case "cover_required_materials":
		return []dataTaskActionContract{
			contract(dataquery.DataActionMaterialInventory, "many local material paths are OK", "discover objective file metadata before choosing specific inputs", "material inventory artifact"),
			contract(dataquery.DataActionInspectMaterial, "one or more concrete material paths are OK", "profile file shape, headers, samples, or text preview", "inspection artifact"),
			contract(dataquery.DataActionExtractRecords, "one or more structured/text materials are OK", "convert selected materials into bounded generic record samples", "record sample artifact"),
			contract(dataquery.DataActionDeriveRules, "rule/constraint/instruction materials or explicit rules_json", "turn task rules or constraints into rule_coverage records", "rule coverage artifact and records"),
		}
	case "derive_rules":
		return []dataTaskActionContract{
			contract(dataquery.DataActionDeriveRules, "rule/constraint/instruction materials or explicit rules_json only", "emit rule_coverage records before later decisions, contributions, or reconcile", "rule coverage artifact and records"),
		}
	case "normalize_or_enrich_entities":
		return []dataTaskActionContract{
			contract(dataquery.DataActionExtractRecords, "covered source material or generated artifact with output_artifact", "materialize a record artifact needed by normalization, enrichment, join, filtering, or contribution actions; this is record materialization, not repeated material coverage", "one reusable record artifact"),
			contract(dataquery.DataActionDeriveFields, "exactly one existing record artifact/path", "derive same-record fields such as parsed numbers, extracted year, trimmed/lowercase values, constants, regex fields, or maps that do not require a second table", "one enriched record artifact"),
			contract(dataquery.DataActionExtractFields, "one or more same-schema record/text artifacts/paths", "extract structured fields from text/record artifacts using one declared regex/parse spec set before filtering, joining, or contribution calculation", "one merged structured record artifact"),
			contract(dataquery.DataActionGroupRecords, "exactly one existing record artifact/path", "group rows/spans by declared key fields and concatenate text/value fields before extraction, joining, filtering, or contribution calculation", "one grouped record artifact"),
			contract(dataquery.DataActionExpandRecords, "exactly one existing record artifact/path", "expand one multi-value field into multiple records before enrichment or join, for aliases, tags, categories, roles, labels, terms, or other delimited values", "one expanded record artifact"),
			contract(dataquery.DataActionFilterRecords, "exactly one existing record artifact/path", "keep/drop rows using existing fields before enrichment, join, or contribution calculation", "one filtered record artifact"),
			contract(dataquery.DataActionNormalizeEntities, "one or more inputs are OK when producing source-to-canonical mappings", "produce canonical mappings for identifiers, names, categories, accounts, people, devices, labels, or other entities", "entity_resolution records and mapping artifact"),
			contract(dataquery.DataActionApplyResolutions, "base record artifact plus entity_resolution artifact(s)", "apply existing source-to-canonical mappings back onto base rows before enrichment, join, filtering, or contribution calculation; preserves base rows by default", "one record artifact with canonical id/label/status fields"),
			contract(dataquery.DataActionEnrichRecords, "base record artifact plus mapping/reference artifact(s)", "apply mapping/reference records to base rows and materialize added canonical or derived fields", "one enriched record artifact"),
			contract(dataquery.DataActionJoinRecords, "two structured record artifacts/paths", "join two record sets using explicit key fields before later derivation or contribution calculation; join_type=inner drops unmatched left rows, join_type=left preserves them", "one joined record artifact"),
			contract(dataquery.DataActionCustomTransform, "small bounded fallback over known artifacts only", "use only when typed actions cannot express this single normalization/enrichment step", "one reusable artifact or ledger slice"),
		}
	case "prepare_contribution_inputs":
		return []dataTaskActionContract{
			contract(dataquery.DataActionExtractRecords, "covered source material or generated artifact with output_artifact", "materialize a record artifact needed before field derivation, filtering, joining, contribution calculation, or final projection", "one reusable record artifact"),
			contract(dataquery.DataActionDeriveFields, "exactly one existing record artifact/path", "derive numeric, grouping, filtering, or reference fields that are missing before contribution calculation", "one enriched record artifact"),
			contract(dataquery.DataActionExtractFields, "one or more same-schema record/text artifacts/paths", "extract structured fields from text or mixed record fields before filtering, joining, or contribution calculation", "one merged structured record artifact"),
			contract(dataquery.DataActionGroupRecords, "exactly one existing record artifact/path", "group rows/spans by declared keys when one logical record spans multiple source rows before extraction, joining, filtering, or contribution calculation", "one grouped record artifact"),
			contract(dataquery.DataActionExpandRecords, "exactly one existing record artifact/path", "expand one multi-value field into multiple records before enrichment, join, or contribution calculation", "one expanded record artifact"),
			contract(dataquery.DataActionFilterRecords, "exactly one existing record artifact/path", "keep/drop rows with existing filter fields before contribution calculation", "one filtered record artifact"),
			contract(dataquery.DataActionQualifyRecords, "exactly one existing record artifact/path", "execute model-declared record eligibility conditions and emit include/exclude decision records before contribution calculation", "one eligible/annotated record artifact plus decision rows"),
			contract(dataquery.DataActionNormalizeEntities, "one or more inputs are OK when producing mappings", "materialize canonical mappings needed by later enrichment, joins, or contribution calculation", "entity_resolution records and mapping artifact"),
			contract(dataquery.DataActionApplyResolutions, "base record artifact plus entity_resolution artifact(s)", "materialize existing canonical mappings as fields on contribution candidate rows; preserves base rows by default", "one record artifact with canonical id/label/status fields"),
			contract(dataquery.DataActionEnrichRecords, "base record artifact plus mapping/reference artifact(s)", "apply mappings or reference fields needed by contribution calculation", "one enriched record artifact"),
			contract(dataquery.DataActionJoinRecords, "two structured record artifacts/paths", "join base records with target/query/reference rows before contribution calculation; use join_type=left if unmatched base rows must remain observable", "one joined record artifact"),
			contract(dataquery.DataActionComputeContribs, "one structured record artifact/path with existing value/group/filter fields", "use only when the input artifact already contains every value, group, and filter field named in params", "contribution records and contribution artifact"),
		}
	case "compute_contributions":
		return []dataTaskActionContract{
			contract(dataquery.DataActionExtractRecords, "covered source material or generated artifact with output_artifact", "materialize a missing record artifact when contribution inputs still exist only as covered material or generated non-record payload", "one reusable record artifact"),
			contract(dataquery.DataActionDeriveFields, "exactly one existing record artifact/path", "derive final numeric/group/filter fields before contribution calculation", "one enriched record artifact"),
			contract(dataquery.DataActionExtractFields, "one or more same-schema record/text artifacts/paths", "extract structured fields from text or mixed record fields before contribution calculation", "one merged structured record artifact"),
			contract(dataquery.DataActionGroupRecords, "exactly one existing record artifact/path", "group rows/spans by declared keys when contribution fields require neighboring source rows or text spans", "one grouped record artifact"),
			contract(dataquery.DataActionExpandRecords, "exactly one existing record artifact/path", "expand one multi-value field into multiple records before enrichment, join, or contribution calculation", "one expanded record artifact"),
			contract(dataquery.DataActionFilterRecords, "exactly one existing record artifact/path", "keep/drop rows with existing filter fields before contribution calculation", "one filtered record artifact"),
			contract(dataquery.DataActionQualifyRecords, "exactly one existing record artifact/path", "execute model-declared eligibility conditions when rule/evidence-qualified item decisions are needed before contribution calculation", "one eligible/annotated record artifact plus decision rows"),
			contract(dataquery.DataActionNormalizeEntities, "one or more inputs are OK when producing mappings", "repair missing canonical mappings needed by contribution calculation", "entity_resolution records and mapping artifact"),
			contract(dataquery.DataActionApplyResolutions, "base record artifact plus entity_resolution artifact(s)", "apply existing canonical mappings onto rows before contribution calculation; preserves base rows by default", "one record artifact with canonical id/label/status fields"),
			contract(dataquery.DataActionEnrichRecords, "base record artifact plus mapping/reference artifact(s)", "apply mappings or reference fields needed by contribution calculation", "one enriched record artifact"),
			contract(dataquery.DataActionJoinRecords, "two structured record artifacts/paths", "join base records with target/query/reference rows before contribution calculation; use join_type=left if unmatched base rows must remain observable", "one joined record artifact"),
			contract(dataquery.DataActionComputeContribs, "one structured record artifact/path with existing value/group/filter fields", "compute generic sums, counts, grouped totals, or contribution ledgers", "contribution records and contribution artifact"),
		}
	case "reconcile_artifacts":
		return []dataTaskActionContract{
			contract(dataquery.DataActionReconcile, "prior contribution records are required", "recompute group totals from contribution records and verify expected/actual values", "reconcile report"),
		}
	case "emit_output_contract_answer":
		return []dataTaskActionContract{
			contract(dataquery.DataActionReconcile, "prior contribution records are required", "refresh deterministic reconcile before answer assembly when needed", "reconcile report"),
			contract(dataquery.DataActionAssembleAnswer, "prior reconcile report is required", "project reconcile groups into the strict user-facing output contract without changing business decisions or numeric values", "final answer matching output_contract"),
			contract(dataquery.DataActionCustomTransform, "small projection over reconcile/contribution artifacts only", "assemble the strict user-facing output format without changing business decisions or numeric values", "final answer matching output_contract"),
		}
	default:
		return nil
	}
}

func dataTaskPlanIsCoverageOnly(plan dataquery.TaskPlan) bool {
	if strings.TrimSpace(plan.Script) != "" || len(plan.Actions) == 0 {
		return false
	}
	for _, action := range plan.Actions {
		if strings.TrimSpace(action.Script) != "" {
			return false
		}
		switch normalizeDataActionKindForWorkflow(action.Kind) {
		case dataquery.DataActionExtractRecords:
			if strings.TrimSpace(action.OutputArtifact) != "" {
				return false
			}
			continue
		case dataquery.DataActionMaterialInventory, dataquery.DataActionInspectMaterial, dataquery.DataActionDeriveRules:
			continue
		default:
			return false
		}
	}
	return true
}

func compactDataTaskResultPromptView(result dataquery.Result, answerLimit, auditLimit, decisionLimit, ruleLimit, contributionLimit int) *dataTaskResultPromptView {
	return compactDataTaskResultPromptViewWithArtifactLimits(result, answerLimit, auditLimit, decisionLimit, ruleLimit, contributionLimit, 6, 10, 8)
}

func compactDataTaskResultPromptViewWithArtifactLimits(result dataquery.Result, answerLimit, auditLimit, decisionLimit, ruleLimit, contributionLimit, artifactLimit, artifactAccessLimit, materialSetLimit int) *dataTaskResultPromptView {
	contract := result.OutputContract.Normalize()
	clampedReconcile := clampPromptReconcileReport(result.Reconcile)
	groupCount, groupKeys, _ := promptReconcileGroupSummary(result.Reconcile, 20)
	groupsTruncated := false
	if result.Reconcile != nil && clampedReconcile != nil {
		groupsTruncated = len(result.Reconcile.Groups) > len(clampedReconcile.Groups)
	}
	return &dataTaskResultPromptView{
		Answer:                   clampDataTaskWorkflowText(result.Answer, answerLimit),
		AnswerItemCount:          inferDataTaskAnswerItemCount(result.Answer, contract),
		OutputContract:           contract,
		AuditSummary:             clampDataTaskWorkflowText(result.AuditSummary, auditLimit),
		LedgerProjection:         dataTaskLedgerProjections(result, 8),
		DecisionRecords:          len(result.Rows),
		DecisionSamples:          sampleDataTaskRowDecisions(result.Rows, decisionLimit),
		RuleCoverageRecords:      len(result.RuleCoverage),
		RuleCoverageSamples:      sampleDataTaskRuleCoverage(result.RuleCoverage, ruleLimit),
		ContributionRecords:      len(result.Contributions),
		ContributionSamples:      sampleDataTaskContributions(result.Contributions, contributionLimit),
		EntityResolutionRecords:  len(result.EntityResolutions),
		EntityResolutionSamples:  sampleDataTaskEntityResolutions(result.EntityResolutions, 2),
		Reconcile:                clampedReconcile,
		ReconcileGroupCount:      groupCount,
		ReconcileGroupKeySample:  groupKeys,
		ReconcileGroupsTruncated: groupsTruncated,
		Metrics:                  result.Metrics,
		Artifacts:                sampleDataTaskArtifacts(result.Artifacts, artifactLimit),
		ArtifactAccess:           sampleDataTaskArtifactAccess(result.Artifacts, artifactAccessLimit),
		MaterialSetHandles:       sampleDataTaskMaterialSetHandles(result.Artifacts, materialSetLimit),
		ContractWarnings:         append([]string(nil), result.ContractWarnings...),
	}
}

func sampleDataTaskMaterialSetHandles(artifacts []dataquery.DataArtifact, limit int) []dataTaskMaterialSetHandlePrompt {
	if limit <= 0 || len(artifacts) == 0 {
		return nil
	}
	type group struct {
		kind    string
		members []string
	}
	groups := map[string]*group{}
	var related []dataTaskMaterialSetHandlePrompt
	var walk func(dataquery.DataArtifact)
	walk = func(artifact dataquery.DataArtifact) {
		for _, p := range cleanDataTaskStrings(artifact.SourcePaths) {
			dir := path.Dir(p)
			if dir != "." && dir != "" {
				g := groups[dir]
				if g == nil {
					g = &group{kind: strings.TrimSpace(artifact.Kind)}
					groups[dir] = g
				}
				g.members = append(g.members, p)
			}
		}
		if artifact.Fields != nil {
			textPaths := cleanDataTaskStrings(strings.Split(strings.TrimSpace(artifact.Fields["text_evidence_paths"]), ","))
			if len(textPaths) > 0 {
				related = append(related, dataTaskMaterialSetHandlePrompt{
					ID:                "related_text:" + strings.TrimSpace(artifact.ID),
					Kind:              "related_text_evidence",
					Scope:             strings.TrimSpace(artifact.ID),
					MemberPaths:       clampDataTaskStringSlice(cleanDataTaskStrings(artifact.SourcePaths), 4),
					TextEvidencePaths: clampDataTaskStringSlice(textPaths, 8),
					AccessHint:        "if this source material is relevant to the data goal, add the concrete text_evidence_paths to a bounded coverage/action batch before compute",
				})
			}
		}
		for _, child := range artifact.Children {
			walk(child)
		}
	}
	for _, artifact := range artifacts {
		walk(artifact)
	}
	var out []dataTaskMaterialSetHandlePrompt
	sort.Slice(related, func(i, j int) bool { return related[i].ID < related[j].ID })
	for _, h := range related {
		if len(out) >= limit {
			return out
		}
		out = append(out, h)
	}
	keys := make([]string, 0, len(groups))
	for key, g := range groups {
		g.members = uniqueSortedDataTaskStrings(g.members)
		if len(g.members) >= 2 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(out) >= limit {
			break
		}
		g := groups[key]
		out = append(out, dataTaskMaterialSetHandlePrompt{
			ID:          "dir:" + key,
			Kind:        firstNonEmptyString(g.kind, "material_group"),
			Scope:       key,
			MemberPaths: clampDataTaskStringSlice(g.members, 12),
			AccessHint:  "candidate file group from inventory/inspection; expand only the concrete members required by the current data goal",
		})
	}
	return out
}

func uniqueSortedDataTaskStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range cleanDataTaskStrings(in) {
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func sampleDataTaskArtifactAccess(artifacts []dataquery.DataArtifact, limit int) []dataTaskArtifactAccessPrompt {
	return sampleDataTaskArtifactAccessWithFieldSamples(artifacts, limit, 6, 2)
}

func sampleDataTaskArtifactAccessWithFieldSamples(artifacts []dataquery.DataArtifact, limit, maxSampleFields, maxSampleValues int) []dataTaskArtifactAccessPrompt {
	if limit <= 0 || len(artifacts) == 0 {
		return nil
	}
	var out []dataTaskArtifactAccessPrompt
	seen := map[string]bool{}
	var walk func(dataquery.DataArtifact)
	walk = func(artifact dataquery.DataArtifact) {
		if len(out) >= limit {
			return
		}
		aliases := dataTaskArtifactAliasPaths(artifact)
		shape := ""
		if artifact.Fields != nil {
			shape = strings.TrimSpace(artifact.Fields["json_shape"])
		}
		if len(aliases) > 0 || shape != "" {
			key := dataTaskArtifactAccessKey(artifact)
			if key != "" && seen[key] {
				for _, child := range artifact.Children {
					walk(child)
				}
				return
			}
			if key != "" {
				seen[key] = true
			}
			fields := clampDataTaskStringSlice(artifactAccessFields(artifact), 32)
			out = append(out, dataTaskArtifactAccessPrompt{
				ID:           strings.TrimSpace(artifact.ID),
				Kind:         strings.TrimSpace(artifact.Kind),
				NodeClass:    dataworkflow.ArtifactNodeClass(artifact),
				Aliases:      clampDataTaskStringSlice(aliases, 8),
				JSONShape:    clampDataTaskWorkflowText(shape, 240),
				Fields:       fields,
				FieldSamples: artifactAccessFieldSamples(artifact, fields, maxSampleFields, maxSampleValues),
				AccessHint:   dataTaskArtifactAccessHint(shape),
				SourcePaths:  clampDataTaskStringSlice(artifact.SourcePaths, 6),
			})
		}
		for _, child := range artifact.Children {
			walk(child)
		}
	}
	for _, artifact := range artifacts {
		walk(artifact)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func dataTaskArtifactAccessKey(artifact dataquery.DataArtifact) string {
	for _, alias := range dataTaskArtifactAliasPaths(artifact) {
		if key := normalizeDataTaskCoveragePath(alias); key != "" {
			return key
		}
	}
	if artifact.Fields != nil {
		if path := normalizeDataTaskCoveragePath(artifact.Fields["artifact_path"]); path != "" {
			return path
		}
	}
	id := strings.TrimSpace(artifact.ID)
	if id != "" {
		return "id:" + id
	}
	kind := strings.TrimSpace(artifact.Kind)
	if kind != "" || len(artifact.SourcePaths) > 0 {
		return "kind:" + kind + "\x00" + strings.Join(cleanDataTaskStrings(artifact.SourcePaths), "\x00")
	}
	return ""
}

func artifactAccessFieldSamples(artifact dataquery.DataArtifact, fields []string, maxFields, maxValues int) map[string][]string {
	if maxFields <= 0 || maxValues <= 0 || len(fields) == 0 || len(artifact.Sample) == 0 {
		return nil
	}
	allowed := map[string]string{}
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			continue
		}
		allowed[strings.ToLower(trimmed)] = trimmed
	}
	if len(allowed) == 0 {
		return nil
	}
	out := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, sample := range artifact.Sample {
		if len(out) >= maxFields {
			break
		}
		sample = strings.TrimSpace(sample)
		if sample == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(sample), &obj); err != nil {
			continue
		}
		keys := make([]string, 0, len(obj))
		for key := range obj {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			canonical := allowed[strings.ToLower(strings.TrimSpace(key))]
			if canonical == "" {
				continue
			}
			values := out[canonical]
			if len(values) >= maxValues {
				continue
			}
			value := dataTaskSampleScalar(obj[key])
			if value == "" {
				continue
			}
			if seen[canonical] == nil {
				seen[canonical] = map[string]bool{}
			}
			if seen[canonical][value] {
				continue
			}
			seen[canonical][value] = true
			out[canonical] = append(out[canonical], value)
			if len(out) >= maxFields {
				break
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func dataTaskSampleScalar(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return clampDataTaskWorkflowText(typed, 120)
	case bool:
		return fmt.Sprintf("%t", typed)
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.12f", typed), "0"), ".")
	case int:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func artifactAccessFields(artifact dataquery.DataArtifact) []string {
	seen := map[string]bool{}
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	for _, header := range artifact.Headers {
		add(header)
	}
	if artifact.Fields != nil {
		for _, key := range []string{"output_headers", "headers", "fields"} {
			for _, field := range strings.Split(artifact.Fields[key], ",") {
				add(field)
			}
		}
	}
	return out
}

func dataTaskArtifactAccessHint(shape string) string {
	shape = strings.TrimSpace(strings.ToLower(shape))
	switch {
	case strings.HasPrefix(shape, "array"):
		return "read with json_records(alias) or iterate json_load(alias) as a list; do not call .get() on the top-level value"
	case strings.HasPrefix(shape, "object") && strings.Contains(shape, "records"):
		return "read with json_records(alias); this wrapper object contains a records array plus metadata"
	case strings.HasPrefix(shape, "object"):
		return "read with json_load(alias) for object fields, or json_records(alias) when treating object values/wrappers as records"
	case strings.HasPrefix(shape, "scalar"):
		return "read with json_load(alias) as a scalar value"
	default:
		return "use json_records(alias) for record-oriented access; inspect artifact json_shape before assuming dict/list shape"
	}
}

func compactDataTaskActionsForPrompt(actions []dataquery.DataAction, limit, scriptLimit int) []dataquery.DataAction {
	return compactDataTaskActionsForPromptWithParamLimit(actions, limit, scriptLimit, scriptLimit)
}

func compactDataTaskActionsForPromptWithParamLimit(actions []dataquery.DataAction, limit, scriptLimit, paramLimit int) []dataquery.DataAction {
	if limit <= 0 || len(actions) == 0 {
		return nil
	}
	if len(actions) > limit {
		actions = actions[:limit]
	}
	out := make([]dataquery.DataAction, 0, len(actions))
	for _, action := range actions {
		action.Purpose = clampDataTaskWorkflowText(action.Purpose, 240)
		action.InputPaths = clampDataTaskStringSlice(action.InputPaths, 12)
		action.Script = clampDataTaskWorkflowText(action.Script, scriptLimit)
		action.Params = compactDataTaskActionParamsForPrompt(action.Params, 16, paramLimit)
		action.SuccessCriteria = clampDataTaskStringSlice(action.SuccessCriteria, 8)
		out = append(out, action)
	}
	return out
}

func compactDataTaskActionParamsForPrompt(params map[string]string, maxKeys, valueLimit int) map[string]string {
	if len(params) == 0 || maxKeys <= 0 {
		return nil
	}
	if valueLimit <= 0 {
		valueLimit = 240
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(keys))
	for i, key := range keys {
		if i >= maxKeys {
			out["_truncated_params"] = fmt.Sprintf("%d additional param(s) omitted from prompt preview", len(keys)-maxKeys)
			break
		}
		out[key] = clampDataTaskWorkflowText(params[key], valueLimit)
	}
	return out
}

func sampleDataTaskArtifacts(artifacts []dataquery.DataArtifact, limit int) []dataquery.DataArtifact {
	if limit <= 0 || len(artifacts) == 0 {
		return nil
	}
	if len(artifacts) > limit {
		artifacts = artifacts[:limit]
	}
	out := make([]dataquery.DataArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, compactDataTaskArtifactForPrompt(artifact, 0))
	}
	return out
}

func compactDataTaskArtifactForPrompt(artifact dataquery.DataArtifact, depth int) dataquery.DataArtifact {
	artifact.Summary = clampDataTaskWorkflowText(artifact.Summary, 300)
	artifact.SourcePaths = clampDataTaskStringSlice(artifact.SourcePaths, 8)
	artifact.Headers = clampDataTaskStringSlice(artifact.Headers, 12)
	artifact.Sample = clampDataTaskStringSlice(artifact.Sample, dataTaskArtifactPromptSampleLimit(depth))
	artifact.Fields = compactDataTaskArtifactFieldsForPrompt(artifact.Fields)
	childLimit := dataTaskArtifactPromptChildLimit(depth)
	if childLimit <= 0 {
		artifact.Children = nil
		return artifact
	}
	if len(artifact.Children) > childLimit {
		artifact.Children = append([]dataquery.DataArtifact(nil), artifact.Children[:childLimit]...)
	}
	for i := range artifact.Children {
		artifact.Children[i] = compactDataTaskArtifactForPrompt(artifact.Children[i], depth+1)
	}
	return artifact
}

func dataTaskArtifactPromptSampleLimit(depth int) int {
	if depth <= 0 {
		return 4
	}
	return 2
}

func dataTaskArtifactPromptChildLimit(depth int) int {
	switch {
	case depth <= 0:
		return 4
	case depth == 1:
		return 2
	default:
		return 0
	}
}

func compactDataTaskArtifactFieldsForPrompt(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = clampDataTaskWorkflowText(fields[key], 240)
	}
	return out
}

func inferDataTaskAnswerItemCount(answer string, contract dataquery.OutputContract) int {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return 0
	}
	var arr []any
	if err := json.Unmarshal([]byte(answer), &arr); err == nil {
		return len(arr)
	}
	if contract.Normalize().Format == dataquery.OutputCSVLine || strings.Contains(answer, ",") {
		parts := strings.Split(answer, ",")
		count := 0
		for _, part := range parts {
			if strings.TrimSpace(part) != "" {
				count++
			}
		}
		if count > 1 {
			return count
		}
	}
	if contract.Normalize().Format == dataquery.OutputMarkdownTable {
		lines := strings.Split(answer, "\n")
		count := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "|") && strings.Contains(line, "|") && !strings.Contains(line, "---") {
				count++
			}
		}
		if count > 1 {
			return count - 1
		}
	}
	return 0
}

func promptReconcileGroupSummary(report *dataquery.ReconcileReport, limit int) (int, []string, bool) {
	if report == nil || len(report.Groups) == 0 || limit <= 0 {
		if report == nil {
			return 0, nil, false
		}
		return len(report.Groups), nil, len(report.Groups) > 0
	}
	total := len(report.Groups)
	if total < limit {
		limit = total
	}
	keys := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		keys = append(keys, promptReconcileGroupKey(report.Groups[i]))
	}
	return total, keys, total > limit
}

func promptReconcileGroupKey(group dataquery.ReconcileGroup) string {
	groupKey := strings.TrimSpace(group.GroupKey.String())
	metric := strings.TrimSpace(group.Metric.String())
	switch {
	case groupKey != "" && metric != "":
		return groupKey + "/" + metric
	case groupKey != "":
		return groupKey
	case metric != "":
		return metric
	default:
		return "(default)"
	}
}

func dataTaskLedgerProjections(result dataquery.Result, sampleLimit int) []dataTaskLedgerProjection {
	if sampleLimit <= 0 {
		sampleLimit = 4
	}
	var out []dataTaskLedgerProjection
	if len(result.Rows) > 0 {
		proj := dataTaskLedgerProjection{
			Kind:          "decision_records",
			Count:         len(result.Rows),
			DecisionCount: map[string]int{},
		}
		for _, row := range result.Rows {
			key := strings.TrimSpace(row.Decision)
			if key == "" {
				key = "(blank)"
			}
			proj.DecisionCount[key]++
		}
		out = append(out, compactDataTaskLedgerProjection(proj, sampleLimit))
	}
	if len(result.RuleCoverage) > 0 {
		proj := dataTaskLedgerProjection{
			Kind:         "rule_coverage",
			Count:        len(result.RuleCoverage),
			StatusCounts: map[string]int{},
		}
		for _, rec := range result.RuleCoverage {
			key := strings.TrimSpace(rec.Status.String())
			if key == "" {
				key = "(blank)"
			}
			proj.StatusCounts[key]++
		}
		out = append(out, compactDataTaskLedgerProjection(proj, sampleLimit))
	}
	if len(result.EntityResolutions) > 0 {
		proj := dataTaskLedgerProjection{
			Kind:         "entity_resolutions",
			Count:        len(result.EntityResolutions),
			StatusCounts: map[string]int{},
		}
		for _, rec := range result.EntityResolutions {
			key := strings.TrimSpace(rec.Status.String())
			if key == "" {
				key = "(blank)"
			}
			proj.StatusCounts[key]++
		}
		out = append(out, compactDataTaskLedgerProjection(proj, sampleLimit))
	}
	if len(result.Contributions) > 0 {
		proj := dataTaskLedgerProjection{
			Kind:  "contributions",
			Count: len(result.Contributions),
		}
		seenGroup := map[string]bool{}
		seenMetric := map[string]bool{}
		seenRole := map[string]bool{}
		for _, rec := range result.Contributions {
			addLedgerProjectionSample(&proj.GroupKeys, seenGroup, promptContributionGroupKey(rec), sampleLimit)
			addLedgerProjectionSample(&proj.Metrics, seenMetric, rec.Metric.String(), sampleLimit)
			addLedgerProjectionSample(&proj.Roles, seenRole, rec.Role.String(), sampleLimit)
		}
		proj.Truncated = len(result.Contributions) > sampleLimit
		out = append(out, proj)
	}
	if result.Reconcile != nil {
		proj := dataTaskLedgerProjection{
			Kind:  "reconcile",
			Count: len(result.Reconcile.Groups),
		}
		if status := strings.TrimSpace(result.Reconcile.Status.String()); status != "" {
			proj.StatusCounts = map[string]int{status: 1}
		}
		seenGroup := map[string]bool{}
		seenMetric := map[string]bool{}
		for _, group := range result.Reconcile.Groups {
			addLedgerProjectionSample(&proj.GroupKeys, seenGroup, promptReconcileGroupKey(group), sampleLimit)
			addLedgerProjectionSample(&proj.Metrics, seenMetric, group.Metric.String(), sampleLimit)
		}
		proj.Truncated = len(result.Reconcile.Groups) > sampleLimit
		out = append(out, proj)
	}
	return out
}

func compactDataTaskLedgerProjection(proj dataTaskLedgerProjection, sampleLimit int) dataTaskLedgerProjection {
	proj.StatusCounts, proj.Truncated = compactDataTaskLedgerCounts(proj.StatusCounts, sampleLimit, proj.Truncated)
	proj.DecisionCount, proj.Truncated = compactDataTaskLedgerCounts(proj.DecisionCount, sampleLimit, proj.Truncated)
	return proj
}

func compactDataTaskLedgerCounts(values map[string]int, limit int, truncated bool) (map[string]int, bool) {
	if len(values) == 0 || limit <= 0 || len(values) <= limit {
		return values, truncated
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]int, limit)
	for _, key := range keys[:limit] {
		out[clampDataTaskWorkflowText(key, 120)] = values[key]
	}
	return out, true
}

func addLedgerProjectionSample(out *[]string, seen map[string]bool, value string, limit int) {
	value = clampDataTaskWorkflowText(value, 160)
	if strings.TrimSpace(value) == "" || seen[value] || len(*out) >= limit {
		return
	}
	seen[value] = true
	*out = append(*out, value)
}

func promptContributionGroupKey(rec dataquery.ContributionRecord) string {
	groupKey := strings.TrimSpace(rec.GroupKey.String())
	metric := strings.TrimSpace(rec.Metric.String())
	switch {
	case groupKey != "" && metric != "":
		return groupKey + "/" + metric
	case groupKey != "":
		return groupKey
	case metric != "":
		return metric
	default:
		return "(default)"
	}
}

func sampleDataTaskRuleCoverage(records []dataquery.RuleCoverageRecord, limit int) []dataquery.RuleCoverageRecord {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if len(records) > limit {
		records = records[:limit]
	}
	out := make([]dataquery.RuleCoverageRecord, 0, len(records))
	for _, rec := range records {
		rec.RuleID = dataquery.LooseText(clampDataTaskWorkflowText(rec.RuleID.String(), 120))
		rec.RuleText = dataquery.LooseText(clampDataTaskWorkflowText(rec.RuleText.String(), 300))
		rec.Status = dataquery.LooseText(clampDataTaskWorkflowText(rec.Status.String(), 80))
		rec.Notes = dataquery.LooseText(clampDataTaskWorkflowText(rec.Notes.String(), 300))
		if len(rec.EvidenceRefs) > 6 {
			rec.EvidenceRefs = append([]string(nil), rec.EvidenceRefs[:6]...)
		}
		out = append(out, rec)
	}
	return out
}

func sampleDataTaskContributions(records []dataquery.ContributionRecord, limit int) []dataquery.ContributionRecord {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if len(records) > limit {
		records = records[:limit]
	}
	out := make([]dataquery.ContributionRecord, 0, len(records))
	for _, rec := range records {
		rec.ItemID = dataquery.LooseText(clampDataTaskWorkflowText(rec.ItemID.String(), 140))
		rec.Source = dataquery.LooseText(clampDataTaskWorkflowText(rec.Source.String(), 200))
		rec.SourceLocator = dataquery.LooseText(clampDataTaskWorkflowText(rec.SourceLocator.String(), 200))
		rec.GroupKey = dataquery.LooseText(clampDataTaskWorkflowText(rec.GroupKey.String(), 160))
		rec.Metric = dataquery.LooseText(clampDataTaskWorkflowText(rec.Metric.String(), 120))
		rec.Value = dataquery.LooseText(clampDataTaskWorkflowText(rec.Value.String(), 120))
		rec.Operation = dataquery.LooseText(clampDataTaskWorkflowText(rec.Operation.String(), 80))
		rec.Reason = dataquery.LooseText(clampDataTaskWorkflowText(rec.Reason.String(), 300))
		if len(rec.EvidenceRefs) > 6 {
			rec.EvidenceRefs = append([]string(nil), rec.EvidenceRefs[:6]...)
		}
		out = append(out, rec)
	}
	return out
}

func sampleDataTaskEntityResolutions(records []dataquery.EntityResolutionRecord, limit int) []dataquery.EntityResolutionRecord {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if len(records) > limit {
		records = records[:limit]
	}
	out := make([]dataquery.EntityResolutionRecord, 0, len(records))
	for _, rec := range records {
		rec.ItemID = dataquery.LooseText(clampDataTaskWorkflowText(rec.ItemID.String(), 140))
		rec.SourceValue = dataquery.LooseText(clampDataTaskWorkflowText(rec.SourceValue.String(), 200))
		rec.CanonicalID = dataquery.LooseText(clampDataTaskWorkflowText(rec.CanonicalID.String(), 160))
		rec.CanonicalLabel = dataquery.LooseText(clampDataTaskWorkflowText(rec.CanonicalLabel.String(), 200))
		rec.Status = dataquery.LooseText(clampDataTaskWorkflowText(rec.Status.String(), 80))
		rec.Reason = dataquery.LooseText(clampDataTaskWorkflowText(rec.Reason.String(), 300))
		if len(rec.Candidates) > 4 {
			rec.Candidates = append([]dataquery.EntityCandidate(nil), rec.Candidates[:4]...)
		}
		if len(rec.EvidenceRefs) > 6 {
			rec.EvidenceRefs = append([]string(nil), rec.EvidenceRefs[:6]...)
		}
		out = append(out, rec)
	}
	return out
}

func clampPromptReconcileReport(report *dataquery.ReconcileReport) *dataquery.ReconcileReport {
	if report == nil {
		return nil
	}
	out := *report
	out.Status = dataquery.LooseText(clampDataTaskWorkflowText(out.Status.String(), 80))
	out.ExpectedAnswer = dataquery.LooseText(clampDataTaskWorkflowText(out.ExpectedAnswer.String(), 400))
	out.ActualAnswer = dataquery.LooseText(clampDataTaskWorkflowText(out.ActualAnswer.String(), 400))
	if len(out.Differences) > 6 {
		out.Differences = append([]string(nil), out.Differences[:6]...)
	}
	if len(out.Groups) > 8 {
		out.Groups = append([]dataquery.ReconcileGroup(nil), out.Groups[:8]...)
	}
	for i := range out.Groups {
		out.Groups[i].GroupKey = dataquery.LooseText(clampDataTaskWorkflowText(out.Groups[i].GroupKey.String(), 160))
		out.Groups[i].Metric = dataquery.LooseText(clampDataTaskWorkflowText(out.Groups[i].Metric.String(), 120))
		out.Groups[i].Expected = dataquery.LooseText(clampDataTaskWorkflowText(out.Groups[i].Expected.String(), 120))
		out.Groups[i].Actual = dataquery.LooseText(clampDataTaskWorkflowText(out.Groups[i].Actual.String(), 120))
		out.Groups[i].Difference = dataquery.LooseText(clampDataTaskWorkflowText(out.Groups[i].Difference.String(), 120))
	}
	return &out
}

func sampleDataTaskRowDecisions(rows []dataquery.RowDecision, limit int) []dataquery.RowDecision {
	if limit <= 0 || len(rows) == 0 {
		return nil
	}
	out := make([]dataquery.RowDecision, 0, limit)
	used := map[int]bool{}
	for i, row := range rows {
		if len(out) >= limit {
			break
		}
		if !rowDecisionHasPromptSignal(row) {
			continue
		}
		out = append(out, clampPromptRowDecision(row))
		used[i] = true
	}
	for i, row := range rows {
		if len(out) >= limit {
			break
		}
		if used[i] {
			continue
		}
		out = append(out, clampPromptRowDecision(row))
	}
	return out
}

func rowDecisionHasPromptSignal(row dataquery.RowDecision) bool {
	return strings.TrimSpace(row.RowID) != "" ||
		strings.TrimSpace(row.Source) != "" ||
		strings.TrimSpace(row.SourceLocator) != "" ||
		strings.TrimSpace(row.Decision) != "" ||
		strings.TrimSpace(row.Reason) != "" ||
		strings.TrimSpace(row.Value) != "" ||
		strings.TrimSpace(row.Contribution) != "" ||
		len(row.NormalizedFields) > 0 ||
		len(row.EvidenceRef) > 0
}

func clampPromptRowDecision(row dataquery.RowDecision) dataquery.RowDecision {
	row.RowID = clampDataTaskWorkflowText(row.RowID, 160)
	row.Source = clampDataTaskWorkflowText(row.Source, 240)
	row.SourceLocator = clampDataTaskWorkflowText(row.SourceLocator, 240)
	row.Decision = clampDataTaskWorkflowText(row.Decision, 160)
	row.Reason = clampDataTaskWorkflowText(row.Reason, 400)
	row.Value = clampDataTaskWorkflowText(row.Value, 200)
	row.Contribution = clampDataTaskWorkflowText(row.Contribution, 200)
	if len(row.NormalizedFields) > 0 {
		next := make(map[string]string, len(row.NormalizedFields))
		keys := make([]string, 0, len(row.NormalizedFields))
		for key := range row.NormalizedFields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for i, key := range keys {
			if i >= 24 {
				next["..."] = fmt.Sprintf("%d more field(s)", len(keys)-i)
				break
			}
			next[key] = clampDataTaskWorkflowText(row.NormalizedFields[key], 240)
		}
		row.NormalizedFields = next
	}
	if len(row.EvidenceRef) > 8 {
		row.EvidenceRef = append([]string(nil), row.EvidenceRef[:8]...)
	}
	return row
}

func clampDataTaskWorkflowText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "\n...[truncated]"
}

func clampDataTaskStringSlice(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return append([]string(nil), values...)
	}
	out := append([]string(nil), values[:limit]...)
	out = append(out, fmt.Sprintf("...%d more", len(values)-limit))
	return out
}

func latestDataTaskResult(records []dataTaskWorkflowRecord) (dataquery.Result, bool) {
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Result != nil {
			return *records[i].Result, true
		}
	}
	return dataquery.Result{}, false
}

func dataTaskActionRunnerSeed(records []dataTaskWorkflowRecord) dataquery.Result {
	var out dataquery.Result
	seenArtifacts := map[string]bool{}
	seenConsumed := map[string]bool{}
	addArtifact := func(artifact dataquery.DataArtifact) {
		key := strings.TrimSpace(artifact.ID)
		if key == "" && len(artifact.SourcePaths) > 0 {
			key = strings.Join(cleanDataTaskStrings(artifact.SourcePaths), "\x00")
		}
		if key != "" {
			if seenArtifacts[key] {
				return
			}
			seenArtifacts[key] = true
		}
		out.Artifacts = append(out.Artifacts, artifact)
	}
	addConsumed := func(values []string) {
		for _, p := range cleanDataTaskStrings(values) {
			if seenConsumed[p] {
				continue
			}
			seenConsumed[p] = true
			out.ConsumedPaths = append(out.ConsumedPaths, p)
		}
	}
	for _, rec := range records {
		if rec.Result == nil {
			continue
		}
		result := *rec.Result
		addConsumed(result.ConsumedPaths)
		for _, artifact := range result.Artifacts {
			addArtifact(artifact)
		}
		out.Rows = dataquery.DedupeRowDecisionRecords(append(out.Rows, result.Rows...))
		out.RuleCoverage = append(out.RuleCoverage, result.RuleCoverage...)
		out.Contributions = append(out.Contributions, result.Contributions...)
		out.EntityResolutions = append(out.EntityResolutions, result.EntityResolutions...)
		out.Metrics = append(out.Metrics, result.Metrics...)
		out.ContractWarnings = append(out.ContractWarnings, result.ContractWarnings...)
		if strings.TrimSpace(result.AuditSummary) != "" {
			if strings.TrimSpace(out.AuditSummary) == "" {
				out.AuditSummary = result.AuditSummary
			} else {
				out.AuditSummary += "; " + result.AuditSummary
			}
		}
		if result.Reconcile != nil {
			reconcile := *result.Reconcile
			out.Reconcile = &reconcile
		}
		if strings.TrimSpace(result.Answer) != "" && (!dataquery.AnswerLooksLikeArtifactSummary(result.Answer) || strings.TrimSpace(out.Answer) == "" || dataquery.AnswerLooksLikeArtifactSummary(out.Answer)) {
			out.Answer = result.Answer
		}
		if result.OutputContract.Format != "" {
			out.OutputContract = result.OutputContract
		}
	}
	return out
}

func dataTaskResultHasHandoffSignal(result dataquery.Result) bool {
	return strings.TrimSpace(result.Answer) != "" ||
		strings.TrimSpace(result.AuditSummary) != "" ||
		len(result.Artifacts) > 0 ||
		len(result.Rows) > 0 ||
		len(result.RuleCoverage) > 0 ||
		len(result.Contributions) > 0 ||
		len(result.EntityResolutions) > 0 ||
		result.Reconcile != nil ||
		len(result.ConsumedPaths) > 0 ||
		len(result.Metrics) > 0 ||
		len(result.ContractWarnings) > 0
}

func dataTaskWorkflowRecordWithOptionalResult(plan dataquery.TaskPlan, result dataquery.Result, errText string) dataTaskWorkflowRecord {
	rec := dataTaskWorkflowRecord{Plan: plan, Err: errText}
	if dataTaskResultHasHandoffSignal(result) {
		rec.Result = &result
	}
	return rec
}

func dataTaskArtifactAliasPaths(artifact dataquery.DataArtifact) []string {
	var out []string
	if strings.TrimSpace(artifact.ID) != "" {
		out = append(out, artifact.ID)
		if !strings.HasSuffix(strings.TrimSpace(artifact.ID), ".json") {
			out = append(out, artifact.ID+".json")
		}
	}
	if artifact.Fields != nil {
		for _, alias := range strings.Split(artifact.Fields["artifact_aliases"], ",") {
			alias = strings.TrimSpace(alias)
			if alias != "" {
				out = append(out, alias)
			}
		}
	}
	return cleanDataTaskStrings(out)
}

func dataTaskWorkflowGeneratedArtifactPathSet(records []dataTaskWorkflowRecord) map[string]bool {
	out := map[string]bool{}
	var mark func([]string)
	mark = func(values []string) {
		for _, value := range cleanDataTaskStrings(values) {
			normalized := normalizeDataTaskCoveragePath(value)
			if normalized != "" {
				out[normalized] = true
			}
		}
	}
	var walk func(dataquery.DataArtifact)
	walk = func(artifact dataquery.DataArtifact) {
		mark(dataTaskArtifactAliasPaths(artifact))
		if artifact.Fields != nil {
			mark([]string{artifact.Fields["artifact_path"]})
		}
		for _, child := range artifact.Children {
			walk(child)
		}
	}
	for _, rec := range records {
		if rec.Result == nil {
			continue
		}
		for _, artifact := range rec.Result.Artifacts {
			walk(artifact)
		}
	}
	return out
}

func dataTaskWorkflowHasArtifactAlias(records []dataTaskWorkflowRecord, alias string) bool {
	want := normalizeDataTaskCoveragePath(alias)
	if want == "" {
		return false
	}
	return dataTaskWorkflowGeneratedArtifactPathSet(records)[want]
}

func dataTaskCoverageContractWithoutGeneratedScriptMaterials(contract dataquery.CoverageContract, generated map[string]bool) dataquery.CoverageContract {
	if len(generated) == 0 {
		return contract
	}
	filter := func(materials []dataquery.CoverageMaterial) []dataquery.CoverageMaterial {
		out := make([]dataquery.CoverageMaterial, 0, len(materials))
		for _, material := range materials {
			mode := normalizeCoverageMaterialUseModeForWorkflow(material.UsageMode)
			if mode == dataquery.MaterialUseScriptConsumed && dataTaskCoverageMaterialIsGeneratedArtifact(material, generated) {
				continue
			}
			out = append(out, material)
		}
		return out
	}
	contract.RequiredMaterials = filter(contract.RequiredMaterials)
	contract.OptionalMaterials = filter(contract.OptionalMaterials)
	return contract
}

func dataTaskCoverageMaterialIsGeneratedArtifact(material dataquery.CoverageMaterial, generated map[string]bool) bool {
	for _, raw := range []string{material.Path, material.ID} {
		normalized := normalizeDataTaskCoveragePath(raw)
		if normalized != "" && generated[normalized] {
			return true
		}
	}
	return false
}

func dataTaskCandidatesWithWorkflowArtifacts(base []dataquery.CandidateFile, records []dataTaskWorkflowRecord) []dataquery.CandidateFile {
	out := append([]dataquery.CandidateFile(nil), base...)
	seen := map[string]bool{}
	for _, candidate := range out {
		if strings.TrimSpace(candidate.Path) != "" {
			seen[strings.TrimSpace(candidate.Path)] = true
		}
	}
	var addArtifact func(dataquery.DataArtifact)
	addArtifact = func(artifact dataquery.DataArtifact) {
		aliases := dataTaskArtifactAliasPaths(artifact)
		if len(aliases) == 0 {
			for _, child := range artifact.Children {
				addArtifact(child)
			}
			return
		}
		alias := aliases[0]
		if seen[alias] {
			return
		}
		seen[alias] = true
		out = append(out, dataquery.CandidateFile{
			Path:    alias,
			Kind:    "generated_json",
			Headers: artifact.Headers,
			Sample:  dataTaskArtifactCandidateSample(artifact),
		})
		for _, child := range artifact.Children {
			addArtifact(child)
		}
	}
	for _, rec := range records {
		if rec.Result == nil {
			continue
		}
		for _, artifact := range rec.Result.Artifacts {
			addArtifact(artifact)
		}
	}
	return out
}

func dataTaskArtifactCandidateSample(artifact dataquery.DataArtifact) []string {
	var sample []string
	if strings.TrimSpace(artifact.Summary) != "" {
		sample = append(sample, "summary: "+strings.TrimSpace(artifact.Summary))
	}
	if artifact.Fields != nil {
		if aliases := strings.TrimSpace(artifact.Fields["artifact_aliases"]); aliases != "" {
			sample = append(sample, "aliases: "+aliases)
		}
		if path := strings.TrimSpace(artifact.Fields["artifact_path"]); path != "" {
			sample = append(sample, "materialized: "+path)
		}
		if shape := strings.TrimSpace(artifact.Fields["json_shape"]); shape != "" {
			sample = append(sample, "json_shape: "+shape)
		}
	}
	sample = append(sample, artifact.Sample...)
	return clampDataTaskStringSlice(sample, 4)
}

func dataTaskWorkflowCoverageContract(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataquery.CoverageContract {
	contract := dataquery.CoverageContract{}
	for _, rec := range records {
		contract = mergeDataTaskCoverageContracts(contract, dataTaskWorkflowRecordCoverageContract(rec))
	}
	currentContract := current.CoverageContract
	currentContract = dataTaskConstrainWorkflowRequiredMaterials(contract, current)
	contract = mergeDataTaskCoverageContracts(contract, currentContract)
	return dataTaskCoverageContractWithoutGeneratedScriptMaterials(contract, dataTaskWorkflowGeneratedArtifactPathSet(records))
}

func dataTaskWorkflowRecordCoverageContract(rec dataTaskWorkflowRecord) dataquery.CoverageContract {
	contract := rec.Plan.CoverageContract
	if strings.TrimSpace(rec.Err) == "" {
		return contract
	}
	out := contract
	out.RequiredMaterials = nil
	for _, material := range contract.RequiredMaterials {
		if dataTaskCoverageMaterialIsUserExplicitFloor(material) {
			out.RequiredMaterials = append(out.RequiredMaterials, material)
			continue
		}
		if strings.TrimSpace(string(material.UsageMode)) == "" {
			material.UsageMode = dataquery.MaterialUseReferenceOnly
		}
		material.Required = false
		out.OptionalMaterials = append(out.OptionalMaterials, material)
	}
	return out
}

func dataTaskConstrainCurrentRequiredMaterials(previous dataquery.CoverageContract, current dataquery.TaskPlan) dataquery.CoverageContract {
	if len(current.CoverageContract.RequiredMaterials) == 0 {
		return current.CoverageContract
	}
	previousRequired := dataTaskCoverageMaterialKeySet(previous.RequiredMaterials)
	currentInputs := dataTaskCurrentBatchInputPathSet(current)
	if len(currentInputs) == 0 {
		return current.CoverageContract
	}
	out := current.CoverageContract
	out.RequiredMaterials = nil
	for _, material := range current.CoverageContract.RequiredMaterials {
		if dataTaskCoverageMaterialShouldRemainRequiredInCurrentBatch(material, previousRequired, currentInputs) {
			out.RequiredMaterials = append(out.RequiredMaterials, material)
			continue
		}
		if strings.TrimSpace(string(material.UsageMode)) == "" {
			material.UsageMode = dataquery.MaterialUseReferenceOnly
		}
		// Keep workflow-level requirements only for deterministic user-pinned
		// material floors. Model-authored future materials outside this batch
		// remain optional so they do not create spurious hard coverage gates.
		material.Required = dataTaskCoverageMaterialIsUserExplicitFloor(material)
		out.OptionalMaterials = append(out.OptionalMaterials, material)
	}
	return out
}

func dataTaskConstrainWorkflowRequiredMaterials(previous dataquery.CoverageContract, current dataquery.TaskPlan) dataquery.CoverageContract {
	if len(current.CoverageContract.RequiredMaterials) == 0 {
		return current.CoverageContract
	}
	currentInputs := dataTaskCurrentBatchInputPathSet(current)
	if len(currentInputs) == 0 {
		return current.CoverageContract
	}
	previousRequired := dataTaskCoverageMaterialMap(previous.RequiredMaterials)
	userFloorMode := dataTaskCoverageHasUserExplicitFloor(previous.RequiredMaterials) ||
		dataTaskCoverageHasUserExplicitFloor(previous.OptionalMaterials) ||
		dataTaskCoverageHasUserExplicitFloor(current.CoverageContract.RequiredMaterials) ||
		dataTaskCoverageHasUserExplicitFloor(current.CoverageContract.OptionalMaterials)
	out := current.CoverageContract
	out.RequiredMaterials = nil
	for _, material := range current.CoverageContract.RequiredMaterials {
		if dataTaskCoverageMaterialShouldRemainRequiredInWorkflow(material, previousRequired, currentInputs, userFloorMode) {
			out.RequiredMaterials = append(out.RequiredMaterials, material)
			continue
		}
		if strings.TrimSpace(string(material.UsageMode)) == "" {
			material.UsageMode = dataquery.MaterialUseReferenceOnly
		}
		material.Required = dataTaskCoverageMaterialIsUserExplicitFloor(material)
		out.OptionalMaterials = append(out.OptionalMaterials, material)
	}
	return out
}

func dataTaskCoverageMaterialIsUserExplicitFloor(material dataquery.CoverageMaterial) bool {
	return strings.TrimSpace(material.Purpose) == dataTaskUserExplicitMaterialPurpose
}

func dataTaskCoverageHasUserExplicitFloor(materials []dataquery.CoverageMaterial) bool {
	for _, material := range materials {
		if dataTaskCoverageMaterialIsUserExplicitFloor(material) {
			return true
		}
	}
	return false
}

func dataTaskCoverageMaterialMap(materials []dataquery.CoverageMaterial) map[string]dataquery.CoverageMaterial {
	out := map[string]dataquery.CoverageMaterial{}
	for _, material := range materials {
		if key := dataTaskCoverageMaterialKey(material); key != "" {
			out[key] = material
		}
	}
	return out
}

func dataTaskCoverageMaterialShouldRemainRequiredInCurrentBatch(material dataquery.CoverageMaterial, previousRequired, currentInputs map[string]bool) bool {
	key := dataTaskCoverageMaterialKey(material)
	if key != "" && previousRequired[key] {
		return true
	}
	for _, raw := range []string{material.Path, material.TextEvidencePath} {
		p := normalizeDataTaskCoveragePath(raw)
		if p != "" && currentInputs[p] {
			return true
		}
	}
	mode := normalizeCoverageMaterialUseModeForWorkflow(material.UsageMode)
	return mode == dataquery.MaterialUsePlannerDistilled && len(material.DistilledNotes) > 0
}

func dataTaskCoverageMaterialShouldRemainRequiredInWorkflow(material dataquery.CoverageMaterial, previousRequired map[string]dataquery.CoverageMaterial, currentInputs map[string]bool, userFloorMode bool) bool {
	if dataTaskCoverageMaterialIsUserExplicitFloor(material) {
		return true
	}
	key := dataTaskCoverageMaterialKey(material)
	if key != "" {
		if previous, ok := previousRequired[key]; ok {
			return !userFloorMode || dataTaskCoverageMaterialIsUserExplicitFloor(previous)
		}
	}
	if userFloorMode {
		return false
	}
	for _, raw := range []string{material.Path, material.TextEvidencePath} {
		p := normalizeDataTaskCoveragePath(raw)
		if p != "" && currentInputs[p] {
			return true
		}
	}
	mode := normalizeCoverageMaterialUseModeForWorkflow(material.UsageMode)
	return mode == dataquery.MaterialUsePlannerDistilled && len(material.DistilledNotes) > 0
}

func dataTaskCurrentBatchInputPathSet(plan dataquery.TaskPlan) map[string]bool {
	out := map[string]bool{}
	for _, p := range dataTaskCurrentBatchInputPaths(plan) {
		normalized := normalizeDataTaskCoveragePath(p)
		if normalized != "" {
			out[normalized] = true
		}
	}
	return out
}

func dataTaskCurrentBatchInputPaths(plan dataquery.TaskPlan) []string {
	var inputs []string
	for _, action := range plan.Actions {
		inputs = append(inputs, action.InputPaths...)
	}
	if len(cleanDataTaskStrings(inputs)) == 0 && len(plan.Actions) == 0 {
		inputs = append(inputs, plan.InputPaths...)
	}
	return cleanDataTaskStrings(inputs)
}

func preserveDataTaskWorkflowMaterialCoverage(records []dataTaskWorkflowRecord, current, next dataquery.TaskPlan) dataquery.TaskPlan {
	workflow := current
	workflow.CoverageContract = dataTaskWorkflowCoverageContract(records, current)
	preserved := preserveDataTaskMaterialRepairCoverage(workflow, next)
	preserved.CoverageContract = dataTaskCoverageContractWithoutGeneratedScriptMaterials(preserved.CoverageContract, dataTaskWorkflowGeneratedArtifactPathSet(records))
	return preserved
}

func preserveDataTaskWorkflowMaterialCoverageForError(records []dataTaskWorkflowRecord, current, next dataquery.TaskPlan, errText string) dataquery.TaskPlan {
	workflow := current
	workflow.CoverageContract = dataTaskWorkflowCoverageContract(records, current)
	preserved := preserveDataTaskMaterialRepairCoverageForError(workflow, next, errText)
	preserved.CoverageContract = dataTaskCoverageContractWithoutGeneratedScriptMaterials(preserved.CoverageContract, dataTaskWorkflowGeneratedArtifactPathSet(records))
	return preserved
}

func validateDataTaskWorkflowResult(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) error {
	contract := dataTaskWorkflowCoverageContract(records, current)
	return dataquery.ValidateResultAgainstContract(contract, result)
}

func dataTaskWorkflowCompletionGateError(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) string {
	return dataTaskWorkflowCompletionGateErrorWithRepo("", records, current, result)
}

func dataTaskWorkflowCompletionGateErrorWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) string {
	if err := validateDataTaskWorkflowResult(records, current, result); err != nil {
		return fmt.Sprintf("validate data workflow completion: %v", err)
	}
	if candidate, answerItems, ok := dataTaskOutputReferenceProjectionGap(repoRoot, records, current, result); ok {
		return fmt.Sprintf("validate data workflow completion: data output incomplete: final answer has %d item(s), but reference field %q in %q defines %d output key(s); run assemble_answer with complete_reference=true, reference_path, and reference_key_field to project missing zero/empty values without changing contribution records",
			answerItems, candidate.Field, candidate.Path, candidate.KeyCount)
	}
	if dataTaskResultNeedsOutputProjection(records, current, result) {
		return "validate data workflow completion: data output incomplete: output_contract requires final answer projection but result.answer is empty while reconcile groups are available"
	}
	return ""
}

func dataTaskResultNeedsOutputProjection(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) bool {
	contract := firstNonEmptyOutputContract(result.OutputContract, dataTaskWorkflowOutputContract(records, current), current.OutputContract)
	if contract.Format == "" {
		return false
	}
	contract = contract.Normalize()
	if result.Reconcile != nil && len(result.Reconcile.Groups) > 0 &&
		dataTaskOutputContractNeedsFinalProjection(contract) &&
		!dataTaskResultHasAssembleAnswerArtifact(result) &&
		!dataTaskPlanHasCustomTransform(current) {
		return true
	}
	if strings.TrimSpace(result.Answer) != "" && !dataquery.AnswerLooksLikeArtifactSummary(result.Answer) {
		return false
	}
	if result.Reconcile == nil || len(result.Reconcile.Groups) == 0 {
		return false
	}
	if contract.Format != dataquery.OutputFreeform {
		return true
	}
	if !contract.ExplanationAllowed {
		return true
	}
	coverage := dataTaskWorkflowCoverageContract(records, current)
	return coverage.ReconcileRequired
}

func dataTaskOutputContractNeedsFinalProjection(contract dataquery.OutputContract) bool {
	contract = contract.Normalize()
	if contract.Format == "" {
		return false
	}
	return contract.Format != dataquery.OutputFreeform || !contract.ExplanationAllowed
}

func dataTaskResultHasAssembleAnswerArtifact(result dataquery.Result) bool {
	for _, artifact := range result.Artifacts {
		if strings.EqualFold(strings.TrimSpace(artifact.Kind), string(dataquery.DataActionAssembleAnswer)) {
			return true
		}
	}
	return false
}

func dataTaskOutputReferenceProjectionGap(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) (dataquery.ReferenceKeyCandidate, int, bool) {
	if result.Reconcile == nil || len(result.Reconcile.Groups) == 0 {
		return dataquery.ReferenceKeyCandidate{}, 0, false
	}
	contract := firstNonEmptyOutputContract(result.OutputContract, dataTaskWorkflowOutputContract(records, current), current.OutputContract).Normalize()
	if contract.Format == "" || (contract.Format == dataquery.OutputFreeform && contract.ExplanationAllowed) {
		return dataquery.ReferenceKeyCandidate{}, 0, false
	}
	groupKeys := dataTaskReconcileGroupKeys(result.Reconcile)
	if len(groupKeys) == 0 {
		return dataquery.ReferenceKeyCandidate{}, 0, false
	}
	artifacts := dataTaskWorkflowArtifacts(records, result)
	candidatePaths := dataTaskReferenceCandidatePaths(records, current, result, contract)
	candidateFields := cleanDataTaskStrings([]string{contract.ReferenceKeyField})
	runner := dataquery.ActionRunner{
		RepoRoot: strings.TrimSpace(repoRoot),
		Seed:     result,
	}
	candidate, ok := runner.InferReferenceKeyCandidate(artifacts, groupKeys, candidatePaths, candidateFields, 100000)
	if !ok || candidate.KeyCount <= len(groupKeys) {
		return dataquery.ReferenceKeyCandidate{}, 0, false
	}
	answerItems := inferDataTaskAnswerItemCount(result.Answer, contract)
	if strings.TrimSpace(result.Answer) == "" || dataquery.AnswerLooksLikeArtifactSummary(result.Answer) {
		return candidate, answerItems, true
	}
	if answerItems > 0 && answerItems < candidate.KeyCount {
		return candidate, answerItems, true
	}
	return dataquery.ReferenceKeyCandidate{}, answerItems, false
}

func dataTaskReconcileGroupKeys(report *dataquery.ReconcileReport) []string {
	if report == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, group := range report.Groups {
		key := strings.TrimSpace(group.GroupKey.String())
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return cleanDataTaskStrings(out)
}

func dataTaskWorkflowArtifacts(records []dataTaskWorkflowRecord, result dataquery.Result) []dataquery.DataArtifact {
	var out []dataquery.DataArtifact
	for _, rec := range records {
		if rec.Result != nil {
			out = append(out, rec.Result.Artifacts...)
		}
	}
	out = append(out, result.Artifacts...)
	return out
}

func dataTaskReferenceCandidatePaths(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, contract dataquery.OutputContract) []string {
	var paths []string
	paths = append(paths, contract.ReferencePath)
	paths = append(paths, current.InputPaths...)
	for _, action := range current.Actions {
		paths = append(paths, action.InputPaths...)
		paths = append(paths, action.Params["reference_path"], action.Params["reference_paths"])
	}
	paths = append(paths, result.ConsumedPaths...)
	for _, rec := range records {
		paths = append(paths, rec.Plan.InputPaths...)
		for _, action := range rec.Plan.Actions {
			paths = append(paths, action.InputPaths...)
			paths = append(paths, action.Params["reference_path"], action.Params["reference_paths"])
		}
		if rec.Result != nil {
			paths = append(paths, rec.Result.ConsumedPaths...)
		}
	}
	return cleanDataTaskStrings(paths)
}

func dataTaskRequiredOutputProjectionPlan(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) (dataquery.TaskPlan, bool) {
	return dataTaskRequiredOutputProjectionPlanWithRepo("", records, current, result)
}

func dataTaskRequiredOutputProjectionPlanWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) (dataquery.TaskPlan, bool) {
	needsProjection := dataTaskResultNeedsOutputProjection(records, current, result)
	candidate, _, hasReferenceGap := dataTaskOutputReferenceProjectionGap(repoRoot, records, current, result)
	if !needsProjection && !hasReferenceGap {
		return dataquery.TaskPlan{}, false
	}
	contract := firstNonEmptyOutputContract(result.OutputContract, dataTaskWorkflowOutputContract(records, current), current.OutputContract)
	coverage := dataTaskWorkflowCoverageContract(records, current)
	coverage.ReconcileRequired = true
	params := map[string]string{
		"order_by":    "group_key",
		"value_field": "actual",
	}
	if delimiter := strings.TrimSpace(contract.Delimiter); delimiter != "" {
		params["delimiter"] = delimiter
	}
	if hasReferenceGap {
		contract.CompleteReference = true
		contract.ReferencePath = candidate.Path
		contract.ReferenceKeyField = candidate.Field
		params["complete_reference"] = "true"
		params["reference_path"] = candidate.Path
		params["reference_key_field"] = candidate.Field
	}
	base := dataquery.TaskPlan{
		Status:           "ready",
		OutputContract:   contract,
		CoverageContract: coverage,
		Goal:             strings.TrimSpace(current.Goal),
		SuccessCriteria:  append([]string(nil), current.SuccessCriteria...),
		ContinueAfter:    false,
		Actions: []dataquery.DataAction{{
			ID:             "complete_output_contract_answer",
			Kind:           dataquery.DataActionAssembleAnswer,
			Purpose:        "project existing reconcile groups into the requested output contract without changing business decisions or numeric values",
			OutputArtifact: "final_answer.json",
			Params:         params,
		}},
		WhyThisBatch: "project existing reconcile groups into the requested output contract",
	}
	if hasReferenceGap {
		base.WhyThisBatch = "project existing reconcile groups across the complete structural reference key universe"
		base.NextBatch = fmt.Sprintf("assemble_answer will preserve %d reference key(s) from %s.%s and fill missing groups with zero/empty values", candidate.KeyCount, candidate.Path, candidate.Field)
	}
	if strings.TrimSpace(base.Goal) == "" {
		base.Goal = "complete the final answer projection from already reconciled data"
	}
	return base, true
}

func dataTaskRequiredLedgerCompletionPlan(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, errText string) (dataquery.TaskPlan, bool) {
	return dataTaskRequiredLedgerCompletionPlanWithRepo("", records, current, result, errText)
}

func dataTaskRequiredLedgerCompletionPlanWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, errText string) (dataquery.TaskPlan, bool) {
	if plan, ok := dataTaskRequiredOutputProjectionPlanWithRepo(repoRoot, records, current, result); ok {
		return plan, true
	}
	violation := dataquery.ClassifyExecutionError(errText)
	if strings.TrimSpace(violation.Code) != "missing_required_ledger" {
		return dataquery.TaskPlan{}, false
	}
	contract := dataTaskWorkflowCoverageContract(records, current)
	base := dataquery.TaskPlan{
		Status:           "ready",
		OutputContract:   firstNonEmptyOutputContract(result.OutputContract, current.OutputContract),
		CoverageContract: contract,
		Goal:             strings.TrimSpace(current.Goal),
		SuccessCriteria:  append([]string(nil), current.SuccessCriteria...),
		ContinueAfter:    false,
	}
	if strings.TrimSpace(base.Goal) == "" {
		base.Goal = "complete required data validation ledgers without changing the computed answer"
	}
	jsonPath := strings.TrimSpace(violation.JSONPath)
	switch jsonPath {
	case "/rule_coverage":
		action := dataTaskRuleCoverageCompletionAction(contract)
		if strings.TrimSpace(action.ID) == "" {
			return dataquery.TaskPlan{}, false
		}
		base.Actions = []dataquery.DataAction{action}
		base.InputPaths = mergeDataTaskInputPaths(base.InputPaths, action.InputPaths)
		base.WhyThisBatch = "complete missing source-backed rule coverage using a typed derive_rules node"
		return base, true
	case "/entity_resolutions":
		return dataquery.TaskPlan{}, false
	case "/reconcile":
		if len(result.Contributions) == 0 {
			return dataquery.TaskPlan{}, false
		}
		base.Actions = []dataquery.DataAction{{
			ID:             "complete_reconcile",
			Kind:           dataquery.DataActionReconcile,
			Purpose:        "complete missing reconcile ledger from existing contribution records",
			OutputArtifact: "reconcile_result.json",
		}}
		base.WhyThisBatch = "complete missing reconcile ledger from existing contribution records"
		return base, true
	default:
		return dataquery.TaskPlan{}, false
	}
}

func dataTaskTerminalWorkflowFallback(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) (dataquery.TaskPlan, string, bool) {
	if !dataTaskPlanStatusLooksTerminal(current.Status) {
		return dataquery.TaskPlan{}, "", false
	}
	fallback, reason, ok := dataTaskWorkflowNextStageFallback(records, current, "terminal plan ended")
	if !ok {
		return dataquery.TaskPlan{}, "", false
	}
	if len(fallback.Actions) == 1 && normalizeDataActionKindForWorkflow(fallback.Actions[0].Kind) == dataquery.DataActionExtractRecords {
		return dataquery.TaskPlan{}, "", false
	}
	return fallback, reason, true
}

func dataTaskDeterministicContinuationFallback(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, err error) (dataquery.TaskPlan, string, bool) {
	if err == nil {
		return dataquery.TaskPlan{}, "", false
	}
	return dataTaskWorkflowNextStageFallback(records, current, "continuation planner failed")
}

func dataTaskWorkflowNextStageFallback(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, reasonPrefix string) (dataquery.TaskPlan, string, bool) {
	return dataTaskWorkflowNextStageFallbackWithRepo("", records, current, reasonPrefix)
}

func dataTaskWorkflowNextStageFallbackWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, reasonPrefix string) (dataquery.TaskPlan, string, bool) {
	state := dataTaskWorkflowState(records, current)
	if len(dataTaskWorkflowMissingValidationStages(state)) == 0 {
		return dataquery.TaskPlan{}, "", false
	}
	contract := dataTaskWorkflowCoverageContract(records, current)
	base := dataquery.TaskPlan{
		Status:           "ready",
		OutputContract:   dataTaskWorkflowOutputContract(records, current),
		CoverageContract: contract,
		Goal:             strings.TrimSpace(current.Goal),
		SuccessCriteria:  append([]string(nil), current.SuccessCriteria...),
		ContinueAfter:    true,
	}
	if strings.TrimSpace(base.Goal) == "" {
		base.Goal = "continue the typed data workflow toward the user's requested output"
	}
	switch state.NextStage {
	case "derive_rules":
		action := dataTaskRuleCoverageCompletionAction(contract)
		if strings.TrimSpace(action.ID) == "" {
			return dataquery.TaskPlan{}, "", false
		}
		base.Actions = []dataquery.DataAction{action}
		base.InputPaths = mergeDataTaskInputPaths(base.InputPaths, action.InputPaths)
		base.NextBatch = "continue with later typed data stages after deriving rule coverage"
		base.WhyThisBatch = reasonPrefix + " before required rule coverage was materialized"
		return base, reasonPrefix + "; converted to required derive_rules stage", true
	case "normalize_or_enrich_entities":
		if plan, reason, ok := dataTaskWorkflowRecordMaterializationFallback(records, base, state, reasonPrefix); ok {
			return plan, reason, true
		}
		if plan, reason, ok := dataTaskWorkflowConcreteNextActionFallback(records, base, state, reasonPrefix); ok {
			return plan, reason, true
		}
		return dataquery.TaskPlan{}, "", false
	case "prepare_contribution_inputs", "compute_contributions":
		if plan, reason, ok := dataTaskWorkflowRecordMaterializationFallback(records, base, state, reasonPrefix); ok {
			return plan, reason, true
		}
		if plan, reason, ok := dataTaskWorkflowConcreteNextActionFallback(records, base, state, reasonPrefix); ok {
			return plan, reason, true
		}
		return dataquery.TaskPlan{}, "", false
	case "reconcile_artifacts":
		latest, ok := latestDataTaskResult(records)
		if !ok || len(latest.Contributions) == 0 {
			return dataquery.TaskPlan{}, "", false
		}
		base.Actions = []dataquery.DataAction{{
			ID:             "continue_reconcile",
			Kind:           dataquery.DataActionReconcile,
			Purpose:        "reconcile existing contribution records before final answer projection",
			OutputArtifact: "reconcile_result.json",
		}}
		base.NextBatch = "continue with assemble_answer after reconcile materializes"
		base.WhyThisBatch = reasonPrefix + " before required reconcile stage"
		return base, reasonPrefix + "; converted to required reconcile stage", true
	case "emit_output_contract_answer":
		latest, ok := latestDataTaskResult(records)
		if !ok {
			return dataquery.TaskPlan{}, "", false
		}
		if plan, ok := dataTaskRequiredOutputProjectionPlanWithRepo(repoRoot, records, current, latest); ok {
			plan.ContinueAfter = false
			return plan, reasonPrefix + "; converted to required answer projection stage", true
		}
	}
	return dataquery.TaskPlan{}, "", false
}

func dataTaskWorkflowRecordMaterializationFallback(records []dataTaskWorkflowRecord, base dataquery.TaskPlan, state dataTaskWorkflowStateView, reasonPrefix string) (dataquery.TaskPlan, string, bool) {
	if !dataTaskStringSliceContainsFold(state.AllowedNextActions, string(dataquery.DataActionExtractRecords)) {
		return dataquery.TaskPlan{}, "", false
	}
	access := dataTaskWorkflowArtifactContractAccess(records)
	if len(dataTaskWorkflowRecordActionArtifacts(access)) > 0 {
		return dataquery.TaskPlan{}, "", false
	}
	paths := dataTaskRecordMaterializationPaths(base.CoverageContract)
	if len(paths) == 0 {
		return dataquery.TaskPlan{}, "", false
	}
	out := base
	out.Status = "ready"
	out.Script = ""
	out.Actions = []dataquery.DataAction{{
		ID:             "continue_extract_records",
		Kind:           dataquery.DataActionExtractRecords,
		Purpose:        "materialize required local materials into reusable record artifacts before typed graph computation",
		InputPaths:     paths,
		OutputArtifact: "source_records.json",
		Params: map[string]string{
			"limit": fmt.Sprintf("%d", dataTaskExactExtractRecordLimit),
		},
	}}
	out.InputPaths = mergeDataTaskInputPaths(out.InputPaths, paths)
	out.ContinueAfter = true
	out.WhyThisBatch = reasonPrefix + "; materialize required local materials as reusable record artifacts before the next typed DAG stage"
	out.NextBatch = "continue with normalization, enrichment, filtering, contribution, and reconcile actions after record artifacts exist"
	if dataTaskFallbackPlanAlreadySeen(records, out) {
		return dataquery.TaskPlan{}, "", false
	}
	return out, reasonPrefix + "; converted to required record materialization stage", true
}

func dataTaskRecordMaterializationPaths(contract dataquery.CoverageContract) []string {
	var out []string
	for _, material := range contract.RequiredMaterials {
		if !material.Required {
			continue
		}
		mode := dataquery.CoverageMaterialUseMode(strings.ToLower(strings.TrimSpace(string(material.UsageMode))))
		switch mode {
		case dataquery.MaterialUsePlannerDistilled, dataquery.MaterialUseReferenceOnly:
			continue
		case dataquery.MaterialUseTextEvidenceConsumed:
			if textPath := strings.TrimSpace(material.TextEvidencePath); textPath != "" {
				out = append(out, textPath)
				continue
			}
		}
		if path := strings.TrimSpace(material.Path); path != "" {
			out = append(out, path)
		}
	}
	return cleanDataTaskStrings(out)
}

func dataTaskWorkflowConcreteNextActionFallback(records []dataTaskWorkflowRecord, base dataquery.TaskPlan, state dataTaskWorkflowStateView, reasonPrefix string) (dataquery.TaskPlan, string, bool) {
	allowed := map[string]bool{}
	for _, action := range state.AllowedNextActions {
		if action = strings.TrimSpace(action); action != "" {
			allowed[action] = true
		}
	}
	if len(allowed) == 0 {
		return dataquery.TaskPlan{}, "", false
	}
	access := dataTaskWorkflowArtifactContractAccess(records)
	if len(access) == 0 {
		access = dataTaskWorkflowArtifactAccess(records, 20)
	}
	recordAccess := dataTaskWorkflowRecordActionArtifacts(access)
	var scaffolds []dataTaskActionScaffold
	appendAllowed := func(kind dataquery.DataActionKind, items []dataTaskActionScaffold) {
		if !allowed[string(kind)] || len(items) == 0 {
			return
		}
		scaffolds = append(scaffolds, items...)
	}
	switch state.NextStage {
	case "normalize_or_enrich_entities":
		appendAllowed(dataquery.DataActionNormalizeEntities, dataTaskNormalizeEntityActionScaffolds(recordAccess, 8))
		appendAllowed(dataquery.DataActionApplyResolutions, dataTaskApplyResolutionActionScaffolds(access, 4))
		appendAllowed(dataquery.DataActionEnrichRecords, dataTaskEnrichActionScaffolds(access, 4))
		appendAllowed(dataquery.DataActionJoinRecords, dataTaskJoinActionScaffolds(recordAccess, 4))
	case "prepare_contribution_inputs", "compute_contributions":
		if !state.EntityStageMaterialized {
			appendAllowed(dataquery.DataActionNormalizeEntities, dataTaskNormalizeEntityActionScaffolds(recordAccess, 8))
		}
		appendAllowed(dataquery.DataActionApplyResolutions, dataTaskApplyResolutionActionScaffolds(access, 6))
		appendAllowed(dataquery.DataActionEnrichRecords, dataTaskEnrichActionScaffolds(access, 6))
		appendAllowed(dataquery.DataActionJoinRecords, dataTaskJoinActionScaffolds(recordAccess, 6))
		scaffolds = append(scaffolds, dataTaskConcreteScaffoldCandidatesForState(state)...)
	default:
		scaffolds = append(scaffolds, dataTaskConcreteScaffoldCandidatesForState(state)...)
	}
	for _, scaffold := range scaffolds {
		if !allowed[normalizeDataTaskActionKindString(scaffold.Kind)] {
			continue
		}
		action, ok := dataTaskConcreteActionFromScaffold(scaffold)
		if !ok {
			continue
		}
		if dataTaskConcreteActionWouldRepeatNoProgress(records, state, action) {
			continue
		}
		out := base
		out.Actions = []dataquery.DataAction{action}
		out.InputPaths = mergeDataTaskInputPaths(out.InputPaths, action.InputPaths)
		out.Script = ""
		out.ContinueAfter = true
		if strings.TrimSpace(action.Purpose) != "" {
			out.WhyThisBatch = reasonPrefix + "; " + strings.TrimSpace(action.Purpose)
		} else {
			out.WhyThisBatch = reasonPrefix + "; execute the next concrete typed action scaffold from current artifact fields"
		}
		out.NextBatch = "evaluate this typed artifact, then continue with the next legal DAG stage"
		if dataTaskFallbackPlanAlreadySeen(records, out) {
			continue
		}
		return out, reasonPrefix + "; advanced " + state.NextStage + " with a concrete typed action scaffold", true
	}
	return dataquery.TaskPlan{}, "", false
}

func dataTaskConcreteActionWouldRepeatNoProgress(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView, action dataquery.DataAction) bool {
	if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionJoinRecords {
		return false
	}
	if state.NextStage != "prepare_contribution_inputs" && state.NextStage != "compute_contributions" {
		return false
	}
	if (!state.ContributionLedgerRequired && !state.ReconcileRequired) || state.ContributionRecords > 0 || state.HasReconcile {
		return false
	}
	return dataTaskRecentJoinNoProgressCount(records) >= DefaultDataTaskMaxNodeFailures
}

func dataTaskTerminalWorkflowGuardResult(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataworkflow.GuardResult {
	if !dataTaskPlanStatusLooksTerminal(current.Status) {
		return dataworkflow.GuardResult{}
	}
	state := dataTaskWorkflowState(records, current)
	missing := dataTaskWorkflowMissingValidationStages(state)
	if len(missing) == 0 || len(state.AllowedNextActions) == 0 {
		return dataworkflow.GuardResult{}
	}
	msg := fmt.Sprintf("data planning incomplete: terminal status=%q is invalid because workflow next_stage=%s still has unfinished validation stage(s): %s and legal next actions [%s]. Emit status=ready with one bounded typed action batch from workflow_state_json.allowed_next_actions; use continue_after=true when later stages remain. blocked is only for true capability/policy barriers, not for ordinary unfinished typed data workflow stages.",
		strings.TrimSpace(current.Status),
		state.NextStage,
		strings.Join(missing, ", "),
		strings.Join(state.AllowedNextActions, ", "))
	violation := dataworkflow.WorkflowViolation{
		Code:              "unfinished_validation_stage",
		Severity:          "error",
		Repairability:     dataworkflow.RepairNeedsTypedAction,
		ActionKind:        firstNonEmptyString(state.NextStage, "data_workflow"),
		RepairActionHints: append([]string(nil), state.AllowedNextActions...),
		Reason:            msg,
	}
	return dataworkflow.NewGuardResult("unfinished_validation_stage", "error", dataworkflow.RepairNeedsTypedAction, msg, violation)
}

func dataTaskTerminalWorkflowGuardError(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) string {
	return dataTaskTerminalWorkflowGuardResult(records, current).ErrorText()
}

func dataTaskPlanStatusLooksTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "needs_clarification", "blocked", "complete", "completed", "budget_exhausted", "partial_answer_possible":
		return true
	default:
		return false
	}
}

func firstNonEmptyOutputContract(values ...dataquery.OutputContract) dataquery.OutputContract {
	var best dataquery.OutputContract
	bestScore := -1
	for _, value := range values {
		score := dataTaskOutputContractSpecificity(value)
		if score > bestScore {
			best = value.Normalize()
			bestScore = score
		}
	}
	if bestScore >= 0 {
		return best
	}
	return dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true}
}

func dataTaskOutputContractSpecificity(value dataquery.OutputContract) int {
	rawFormat := strings.TrimSpace(string(value.Format))
	rawDelimiter := strings.TrimSpace(value.Delimiter)
	if rawFormat == "" && rawDelimiter == "" {
		return -1
	}
	value = value.Normalize()
	score := 0
	if rawFormat != "" {
		score++
	}
	if value.Format != dataquery.OutputFreeform {
		score += 10
	}
	if !value.ExplanationAllowed {
		score += 2
	}
	if rawDelimiter != "" {
		score++
	}
	return score
}

func dataTaskRuleCoverageCompletionAction(contract dataquery.CoverageContract) dataquery.DataAction {
	var inputs []string
	for _, material := range contract.RequiredMaterials {
		mode := normalizeCoverageMaterialUseModeForWorkflow(material.UsageMode)
		if mode != dataquery.MaterialUseScriptConsumed && mode != dataquery.MaterialUsePlannerDistilled {
			continue
		}
		p := normalizeDataTaskCoveragePath(material.Path)
		if p == "" || !dataTaskPathLooksLikeTextConstraintMaterial(p) {
			continue
		}
		inputs = append(inputs, p)
	}
	params := map[string]string{}
	if len(inputs) == 0 && len(contract.ValidationRules) > 0 {
		params["rules"] = strings.Join(cleanDataTaskStrings(contract.ValidationRules), "\n")
	}
	if len(inputs) == 0 && strings.TrimSpace(params["rules"]) == "" {
		return dataquery.DataAction{}
	}
	return dataquery.DataAction{
		ID:             "complete_rule_coverage",
		Kind:           dataquery.DataActionDeriveRules,
		Purpose:        "complete missing generic rule coverage ledger from required rule/constraint materials",
		InputPaths:     cleanDataTaskStrings(inputs),
		OutputArtifact: "rules_artifacts.json",
		Params:         params,
	}
}

func dataTaskEntityResolutionCompletionInputs(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) []string {
	var candidates []string
	contract := dataTaskWorkflowCoverageContract(records, current)
	for _, p := range contract.RequiredRunnerInputPaths() {
		if dataTaskPathLooksLikeStructuredMaterial(p) {
			candidates = append(candidates, p)
		}
	}
	for _, p := range result.ConsumedPaths {
		if dataTaskPathLooksLikeStructuredMaterial(p) {
			candidates = append(candidates, p)
		}
	}
	for _, artifact := range result.Artifacts {
		for _, p := range artifact.SourcePaths {
			if dataTaskPathLooksLikeStructuredMaterial(p) {
				candidates = append(candidates, p)
			}
		}
	}
	return cleanDataTaskStrings(candidates)
}

func dataTaskPathLooksLikeStructuredMaterial(p string) bool {
	ext := strings.ToLower(path.Ext(strings.TrimSpace(p)))
	switch ext {
	case ".csv", ".tsv", ".json", ".jsonl":
		return true
	default:
		return false
	}
}

func dataTaskTerminalPlanCompletionGateError(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) string {
	return dataTaskTerminalPlanCompletionGateErrorWithRepo("", records, current)
}

func dataTaskTerminalPlanCompletionGateErrorWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan) string {
	status := strings.ToLower(strings.TrimSpace(current.Status))
	if status != "complete" && status != "completed" {
		return ""
	}
	result, ok := latestDataTaskResult(records)
	if !ok {
		return ""
	}
	return dataTaskWorkflowCompletionGateErrorWithRepo(repoRoot, records, current, result)
}

func shouldValidateDataTaskWorkflowResult(current dataquery.TaskPlan) bool {
	return !current.ContinueAfter
}

func preserveDataTaskRepairCoverage(previous, repaired dataquery.TaskPlan) dataquery.TaskPlan {
	repaired.CoverageContract = mergeDataTaskCoverageContracts(previous.CoverageContract, repaired.CoverageContract)
	repaired.InputPaths = mergeDataTaskInputPaths(repaired.InputPaths, repaired.CoverageContract.RequiredRunnerInputPaths())
	return repaired
}

func preserveDataTaskMaterialRepairCoverage(previous, repaired dataquery.TaskPlan) dataquery.TaskPlan {
	repaired.CoverageContract = mergeDataTaskCoverageContracts(previous.CoverageContract, repaired.CoverageContract)
	repaired.InputPaths = mergeDataTaskInputPaths(repaired.InputPaths, repaired.CoverageContract.RequiredRunnerInputPaths())
	return repaired
}

func preserveDataTaskMaterialRepairCoverageForError(previous, repaired dataquery.TaskPlan, errText string) dataquery.TaskPlan {
	violation := dataquery.ClassifyExecutionError(errText)
	if shouldKeepDataTaskRepairCoverageScoped(previous, repaired, violation) {
		repaired.InputPaths = mergeDataTaskInputPaths(repaired.InputPaths, repaired.CoverageContract.RequiredRunnerInputPaths())
		repaired.ContinueAfter = true
		return repaired
	}
	if violation.Code == "oversized_data_plan" && repaired.ContinueAfter && (len(repaired.CoverageContract.RequiredMaterials) > 0 || len(repaired.CoverageContract.OptionalMaterials) > 0) {
		repaired.InputPaths = mergeDataTaskInputPaths(repaired.InputPaths, repaired.CoverageContract.RequiredRunnerInputPaths())
		return repaired
	}
	return preserveDataTaskMaterialRepairCoverage(previous, repaired)
}

func shouldKeepDataTaskRepairCoverageScoped(previous, repaired dataquery.TaskPlan, violation dataquery.DataTaskViolation) bool {
	if len(repaired.Actions) == 0 {
		return false
	}
	if len(repaired.CoverageContract.RequiredMaterials) == 0 && len(repaired.CoverageContract.OptionalMaterials) == 0 {
		return false
	}
	switch violation.Code {
	case "oversized_data_plan", "oversized_action_plan", "action_top_level_script", "data_action_failed":
		return dataTaskRepairCoverageIsScoped(previous, repaired)
	case "terminal_required_material_not_scheduled", "required_material_not_consumed", "required_material_not_declared", "text_evidence_not_consumed":
		return false
	default:
		return false
	}
}

func dataTaskRepairCoverageIsScoped(previous, repaired dataquery.TaskPlan) bool {
	prevRequired := len(previous.CoverageContract.RequiredMaterials)
	nextRequired := len(repaired.CoverageContract.RequiredMaterials)
	if prevRequired > 0 && nextRequired > 0 && nextRequired < prevRequired {
		return true
	}
	prevLedgers := dataTaskValidationLedgerCount(previous.CoverageContract)
	nextLedgers := dataTaskValidationLedgerCount(repaired.CoverageContract)
	if prevLedgers > 0 && nextLedgers < prevLedgers {
		return true
	}
	prevInputs := len(cleanDataTaskStrings(previous.InputPaths))
	nextInputs := len(cleanDataTaskStrings(repaired.InputPaths))
	if prevInputs > 0 && nextInputs > 0 && nextInputs < prevInputs {
		return true
	}
	for _, action := range repaired.Actions {
		if len(cleanDataTaskStrings(action.InputPaths)) > 0 && prevInputs > 0 && len(cleanDataTaskStrings(action.InputPaths)) < prevInputs {
			return true
		}
	}
	return false
}

func cleanDataTaskStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func parseDataTaskActionStringListParam(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var stringsValue []string
		if err := json.Unmarshal([]byte(raw), &stringsValue); err == nil {
			return cleanDataTaskStrings(stringsValue)
		}
		var anyValue []any
		if err := json.Unmarshal([]byte(raw), &anyValue); err == nil {
			out := make([]string, 0, len(anyValue))
			for _, item := range anyValue {
				if item == nil {
					continue
				}
				out = append(out, strings.TrimSpace(fmt.Sprint(item)))
			}
			return cleanDataTaskStrings(out)
		}
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', '，', ';', '；', '|', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
	return cleanDataTaskStrings(fields)
}

func mergeDataTaskCoverageContracts(previous, next dataquery.CoverageContract) dataquery.CoverageContract {
	out := next
	out.RequiredMaterials = mergeDataTaskCoverageMaterialsExcept(previous.RequiredMaterials, next.RequiredMaterials, true, dataTaskCoverageOptionalOverrideKeySet(next.OptionalMaterials))
	out.RequiredMaterials = mergeDataTaskCoverageMaterials(out.RequiredMaterials, dataTaskRequiredOptionalCoverageMaterials(next.OptionalMaterials), true)
	out.OptionalMaterials = mergeDataTaskCoverageMaterials(previous.OptionalMaterials, next.OptionalMaterials, false)
	requiredKeys := dataTaskCoverageMaterialKeySet(out.RequiredMaterials)
	out.OptionalMaterials = dataTaskFilterCoverageMaterials(out.OptionalMaterials, func(m dataquery.CoverageMaterial) bool {
		key := dataTaskCoverageMaterialKey(m)
		return key == "" || !requiredKeys[key]
	})
	if len(out.ValidationRules) == 0 && len(previous.ValidationRules) > 0 {
		out.ValidationRules = append([]string(nil), previous.ValidationRules...)
	}
	out.DecisionRecordsRequired = previous.DecisionRecordsRequired || next.DecisionRecordsRequired
	out.RuleCoverageRequired = previous.RuleCoverageRequired || next.RuleCoverageRequired
	out.ContributionLedgerRequired = previous.ContributionLedgerRequired || next.ContributionLedgerRequired
	out.EntityResolutionRequired = previous.EntityResolutionRequired || next.EntityResolutionRequired
	out.ReconcileRequired = previous.ReconcileRequired || next.ReconcileRequired
	return out
}

func dataTaskRequiredOptionalCoverageMaterials(materials []dataquery.CoverageMaterial) []dataquery.CoverageMaterial {
	var out []dataquery.CoverageMaterial
	for _, material := range materials {
		if material.Required && dataTaskCoverageMaterialIsUserExplicitFloor(material) {
			out = append(out, material)
		}
	}
	return out
}

func dataTaskCoverageOptionalOverrideKeySet(materials []dataquery.CoverageMaterial) map[string]bool {
	out := map[string]bool{}
	for _, material := range materials {
		if material.Required {
			continue
		}
		if key := dataTaskCoverageMaterialKey(material); key != "" {
			out[key] = true
		}
	}
	return out
}

func mergeDataTaskCoverageMaterials(previous, next []dataquery.CoverageMaterial, forceRequired bool) []dataquery.CoverageMaterial {
	return mergeDataTaskCoverageMaterialsExcept(previous, next, forceRequired, nil)
}

func mergeDataTaskCoverageMaterialsExcept(previous, next []dataquery.CoverageMaterial, forceRequired bool, skipPreviousKeys map[string]bool) []dataquery.CoverageMaterial {
	out := make([]dataquery.CoverageMaterial, 0, len(previous)+len(next))
	seen := map[string]int{}
	appendOne := func(m dataquery.CoverageMaterial) {
		path := strings.TrimSpace(m.Path)
		id := strings.TrimSpace(m.ID)
		purpose := strings.TrimSpace(m.Purpose)
		if path == "" && id == "" && purpose == "" {
			return
		}
		key := dataTaskCoverageMaterialKey(m)
		if key == "" {
			return
		}
		if idx, ok := seen[key]; ok {
			if forceRequired {
				out[idx].Required = true
			}
			if out[idx].ID == "" && id != "" {
				out[idx].ID = id
			}
			if out[idx].Purpose == "" && purpose != "" {
				out[idx].Purpose = purpose
			}
			if strings.TrimSpace(string(out[idx].UsageMode)) == "" && strings.TrimSpace(string(m.UsageMode)) != "" {
				out[idx].UsageMode = m.UsageMode
			}
			if out[idx].TextEvidencePath == "" && strings.TrimSpace(m.TextEvidencePath) != "" {
				out[idx].TextEvidencePath = strings.TrimSpace(m.TextEvidencePath)
			}
			if len(out[idx].DistilledNotes) == 0 && len(m.DistilledNotes) > 0 {
				out[idx].DistilledNotes = append([]string(nil), m.DistilledNotes...)
			}
			return
		}
		seen[key] = len(out)
		if forceRequired {
			m.Required = true
		}
		out = append(out, m)
	}
	for _, m := range previous {
		if key := dataTaskCoverageMaterialKey(m); key != "" && skipPreviousKeys[key] {
			continue
		}
		appendOne(m)
	}
	for _, m := range next {
		appendOne(m)
	}
	return out
}

func dataTaskCoverageMaterialKeySet(materials []dataquery.CoverageMaterial) map[string]bool {
	out := map[string]bool{}
	for _, material := range materials {
		key := dataTaskCoverageMaterialKey(material)
		if key != "" {
			out[key] = true
		}
	}
	return out
}

func dataTaskCoverageMaterialKey(m dataquery.CoverageMaterial) string {
	if normalized := normalizeDataTaskCoveragePath(m.Path); normalized != "" {
		return "path:" + normalized
	}
	if normalized := normalizeDataTaskCoveragePath(m.TextEvidencePath); normalized != "" {
		return "evidence:" + normalized
	}
	id := strings.TrimSpace(m.ID)
	purpose := strings.TrimSpace(m.Purpose)
	if id == "" && purpose == "" {
		return ""
	}
	return "meta:" + id + "\x00" + purpose
}

func normalizeDataTaskCoveragePath(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	raw = strings.TrimPrefix(raw, "./")
	if raw == "" {
		return ""
	}
	cleaned := path.Clean(raw)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func mergeDataTaskInputPaths(paths, required []string) []string {
	out := make([]string, 0, len(paths)+len(required))
	seen := map[string]bool{}
	for _, list := range [][]string{paths, required} {
		for _, path := range list {
			path = strings.TrimSpace(path)
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, path)
		}
	}
	return out
}

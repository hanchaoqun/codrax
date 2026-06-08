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
	return dataworkflow.RepeatedNodeFailureFromErrors(dataTaskWorkflowErrorTexts(records), currentErr, limit)
}

func dataTaskRepeatedFailureReplacementFallback(stateRecords []dataTaskWorkflowRecord, previousErrors []string, current dataquery.TaskPlan, errText string) (dataquery.TaskPlan, string, bool) {
	state := dataTaskWorkflowState(stateRecords, current)
	if len(state.ActionScaffold) == 0 || len(state.AllowedNextActions) == 0 {
		return dataquery.TaskPlan{}, "", false
	}
	return dataworkflow.BuildRepeatedFailureReplacementPlan(dataworkflow.RepeatedFailureReplacementPlanInput{
		Current:        current,
		Coverage:       dataTaskWorkflowCoverageContract(stateRecords, current),
		Output:         dataTaskWorkflowOutputContract(stateRecords, current),
		Scaffolds:      state.ActionScaffold,
		Facts:          state.Facts(),
		PreviousErrors: previousErrors,
		CurrentError:   errText,
		FailureLimit:   DefaultDataTaskMaxNodeFailures,
		SeenActionKeys: dataTaskWorkflowSeenActionKeys(stateRecords),
		ProgressEvents: dataTaskWorkflowProgressEvents(stateRecords),
		NoProgressStop: DefaultDataTaskMaxNodeFailures,
	})
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
	if guard := dataTaskTextConstraintCoverageGuardResult(plan); !guard.Empty() {
		return guard
	}
	lines := dataTaskScriptLineCount(plan.Script)
	requiredMaterials := len(plan.CoverageContract.RequiredMaterials)
	validationLedgers := dataTaskValidationLedgerCount(plan.CoverageContract)
	inputs := len(plan.InputPaths)
	if guard := dataworkflow.PlanShapeGuardResult(dataworkflow.PlanShapeGuardInput{
		Status:                 plan.Status,
		HasActions:             len(plan.Actions) > 0,
		HasScript:              strings.TrimSpace(plan.Script) != "",
		ScriptLines:            lines,
		ScriptHasResultEmitter: dataTaskScriptHasResultEmitter(plan.Script),
		InputCount:             inputs,
		RequiredMaterialCount:  requiredMaterials,
		ValidationLedgerCount:  validationLedgers,
		ContinueAfter:          plan.ContinueAfter,
		SoftScriptLineLimit:    dataTaskOneShotScriptLineSoftLimit,
		HardScriptLineLimit:    dataTaskOneShotScriptLineHardLimit,
		RequiredMaterialLimit:  dataTaskOneShotRequiredMaterialLimit,
		ValidationLedgerLimit:  dataTaskOneShotValidationLedgerLimit,
	}); !guard.Empty() {
		return guard
	}
	if guard := dataTaskTerminalRequiredMaterialSchedulingGuardResult(nil, plan); !guard.Empty() {
		return guard
	}
	return dataworkflow.GuardResult{}
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
	if guard := dataTaskTextConstraintCoverageGuardResult(plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskTerminalRequiredMaterialSchedulingGuardResult(records, plan); !guard.Empty() {
		return guard
	}
	return dataTaskPlanStagingGuardResult(plan)
}

func dataTaskActionStagingGuardResult(plan dataquery.TaskPlan) dataworkflow.GuardResult {
	if guard := dataTaskActionBatchShapeGuardResult(nil, plan, dataworkflow.ActionBatchShapeChecks{
		TopLevelScript: true,
		ActionCount:    true,
	}, false); !guard.Empty() {
		return guard
	}
	if guard := dataTaskTextConstraintCoverageGuardResult(plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskTerminalRequiredMaterialSchedulingGuardResult(nil, plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskActionBatchShapeGuardResult(nil, plan, dataworkflow.ActionBatchShapeChecks{
		MultipleCustomScript: true,
	}, false); !guard.Empty() {
		return guard
	}
	for i, action := range plan.Actions {
		if guard := dataTaskActionDependencyGuardResult(nil, plan, action, i); !guard.Empty() {
			return guard
		}
		if guard := dataTaskSingleActionShapeGuardResult(nil, plan, action, i, false); !guard.Empty() {
			return guard
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskWorkflowActionStagingGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	if guard := dataTaskActionBatchShapeGuardResult(records, plan, dataworkflow.ActionBatchShapeChecks{
		TopLevelScript: true,
		ActionCount:    true,
	}, true); !guard.Empty() {
		return guard
	}
	if guard := dataTaskTextConstraintCoverageGuardResult(plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskTerminalRequiredMaterialSchedulingGuardResult(records, plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskWorkflowCustomTransformDisabledGuardResult(records, plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskActionBatchShapeGuardResult(records, plan, dataworkflow.ActionBatchShapeChecks{
		MultipleCustomScript: true,
	}, true); !guard.Empty() {
		return guard
	}
	if guard := dataTaskCoverageLoopGuardResult(records, plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskWorkflowAllowedNextActionGuardResult(records, plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskWorkflowStageProgressGuardResult(records, plan); !guard.Empty() {
		return guard
	}
	if guard := dataTaskWorkflowNumericConstantReuseGuardResult(plan); !guard.Empty() {
		return guard
	}
	for i, action := range plan.Actions {
		if guard := dataTaskActionDependencyGuardResult(records, plan, action, i); !guard.Empty() {
			return guard
		}
		if guard := dataworkflow.RepeatedCustomTransformGuardResult(action, dataTaskWorkflowErrorTexts(records), DefaultDataTaskMaxNodeFailures); !guard.Empty() {
			return guard
		}
		if guard := dataworkflow.RepeatedCustomTransformClassGuardResult(
			action,
			dataTaskActionHasBroadPrerequisiteSurface(plan, action) || dataTaskActionLooksLikeWholeWorkflow(plan, action, i),
			dataTaskCustomTransformFailureErrors(records),
			DefaultDataTaskMaxCustomTransformClassFailures,
			DefaultDataTaskMaxNodeFailures,
		); !guard.Empty() {
			return guard
		}
		if guard := dataTaskTerminalRawMaterialCustomTransformGuardResult(records, plan, action, i, dataTaskScriptLineCount(action.Script)); !guard.Empty() {
			return guard
		}
		if normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionCustomTransform && dataTaskActionHasBroadPrerequisiteSurface(plan, action) {
			if guard := dataTaskBroadCustomPrerequisiteGuardResult(records, plan, action, i); !guard.Empty() {
				return guard
			}
		}
		if guard := dataTaskSingleActionShapeGuardResult(records, plan, action, i, true); !guard.Empty() {
			return guard
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskActionBatchShapeGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, checks dataworkflow.ActionBatchShapeChecks, workflowAware bool) dataworkflow.GuardResult {
	return dataworkflow.ActionBatchShapeGuardResult(dataworkflow.ActionBatchShapeGuardInput{
		Plan:                         plan,
		Checks:                       checks,
		TopLevelScriptLines:          dataTaskScriptLineCount(plan.Script),
		MaxActionsPerBatch:           dataTaskMaxActionsPerBatch,
		CustomScriptActionCount:      dataTaskCustomScriptActionCount(plan.Actions),
		RequiredMaterialCount:        len(plan.CoverageContract.RequiredMaterials),
		ValidationLedgerCount:        dataTaskValidationLedgerCount(plan.CoverageContract),
		ComplexCustomScriptLineLimit: dataTaskComplexCustomScriptLineLimit,
		OneShotScriptLineSoftLimit:   dataTaskOneShotScriptLineSoftLimit,
		ActionFacts:                  dataTaskActionShapeFacts(records, plan, plan.Actions),
		WorkflowAware:                workflowAware,
	})
}

func dataTaskSingleActionShapeGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int, workflowAware bool) dataworkflow.GuardResult {
	facts := dataTaskActionShapeFacts(records, plan, []dataquery.DataAction{action})
	if len(facts) > 0 {
		facts[0].ActionIndex = actionIndex
	}
	return dataworkflow.ActionBatchShapeGuardResult(dataworkflow.ActionBatchShapeGuardInput{
		Plan:                         plan,
		Checks:                       dataworkflow.ActionBatchShapeChecks{ActionScripts: true},
		RequiredMaterialCount:        len(plan.CoverageContract.RequiredMaterials),
		ValidationLedgerCount:        dataTaskValidationLedgerCount(plan.CoverageContract),
		ComplexCustomScriptLineLimit: dataTaskComplexCustomScriptLineLimit,
		OneShotScriptLineSoftLimit:   dataTaskOneShotScriptLineSoftLimit,
		ActionFacts:                  facts,
		WorkflowAware:                workflowAware,
	})
}

func dataTaskActionShapeFacts(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, actions []dataquery.DataAction) []dataworkflow.ActionShapeFact {
	out := make([]dataworkflow.ActionShapeFact, 0, len(actions))
	for offset, action := range actions {
		out = append(out, dataworkflow.ActionShapeFact{
			Action:                 action,
			ActionIndex:            offset,
			ScriptLines:            dataTaskScriptLineCount(action.Script),
			HasResultEmitter:       dataTaskScriptHasResultEmitter(action.Script),
			LooksLikeWholeWorkflow: dataTaskActionLooksLikeWholeWorkflow(plan, action, offset),
			BroadPrereqSurface:     dataTaskActionHasBroadPrerequisiteSurface(plan, action),
			RawInputCount:          len(dataTaskCustomTransformRawMaterialInputs(records, action)),
		})
	}
	return out
}

func dataTaskWorkflowDeterministicFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, guard dataworkflow.GuardResult) (fallback dataquery.TaskPlan, remainder dataquery.TaskPlan, reason string, ok bool) {
	errText := guard.ErrorText()
	if dataTaskGuardHasCode(guard, "text_constraint_coverage_required", "text_constraint_rule_coverage_required") {
		contract := dataTaskWorkflowCoverageContract(records, plan)
		action := dataworkflow.RuleCoverageCompletionAction(contract)
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
	if fb, hit := dataTaskCustomTransformDisabledFallback(records, plan, guard); hit {
		return fb, dataquery.TaskPlan{}, "custom_transform disabled plan converted to typed workflow fallback", true
	}
	if fb, rem, hit := dataTaskWorkflowStagePrefixFallbackWithRemainder(records, plan, guard); hit {
		return fb, rem, "trimmed multi-stage data plan to current DAG stage", true
	}
	if fb, hit := dataTaskExecutablePrefixFallback(records, plan, guard); hit {
		return fb, dataquery.TaskPlan{}, "executed valid typed prefix and discarded invalid suffix for replanning", true
	}
	if fb, hit := dataTaskInvalidRecordActionFallback(records, plan, guard); hit {
		return fb, dataquery.TaskPlan{}, "converted invalid record action to bounded record extraction", true
	}
	if fb, hit := dataTaskHistoricalMissingJoinFieldFallback(records, plan); hit {
		return fb, dataquery.TaskPlan{}, "materialized historical missing join field from existing artifacts", true
	}
	return dataquery.TaskPlan{}, dataquery.TaskPlan{}, "", false
}

func dataTaskGuardHasCode(guard dataworkflow.GuardResult, codes ...string) bool {
	allowed := map[string]bool{}
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code != "" {
			allowed[code] = true
		}
	}
	if len(allowed) == 0 {
		return false
	}
	if allowed[strings.TrimSpace(guard.Code)] {
		return true
	}
	for _, violation := range guard.Violations {
		if allowed[strings.TrimSpace(violation.Code)] {
			return true
		}
	}
	return false
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
	validationRule := ""
	if strings.TrimSpace(errText) != "" {
		validationRule = "previous broad plan was converted into material discovery: " + oneLineClamp(errText, 240)
	}
	currentInventory := len(plan.Actions) == 1 && normalizeDataActionKindForWorkflow(plan.Actions[0].Kind) == dataquery.DataActionMaterialInventory
	return dataworkflow.BuildMaterialDiscoveryTransition(dataworkflow.MaterialDiscoveryTransitionInput{
		Current:                    plan,
		Coverage:                   dataTaskWorkflowCoverageContract(records, plan),
		Paths:                      dataTaskDiscoveryPaths(plan),
		ValidationRule:             validationRule,
		MaterialCoverageSufficient: state.MaterialCoverageSufficient,
		PriorMaterialInventorySeen: dataTaskWorkflowHasActionKind(records, dataquery.DataActionMaterialInventory),
		CurrentMaterialInventory:   currentInventory,
		NonCustomActionCount:       dataTaskPlanNonCustomActionCount(plan.Actions, len(plan.Actions)),
		HasExecutableSurface:       strings.TrimSpace(plan.Script) != "" || dataTaskPlanHasCustomTransform(plan),
		BroadMaterialLimit:         dataTaskBroadMaterialDiscoveryLimit,
	})
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
	contract := dataTaskWorkflowCoverageContract(records, plan)
	validationRule := ""
	if len(contract.ValidationRules) == 0 && strings.TrimSpace(errText) != "" {
		validationRule = "previous structural coverage guard requested an atomic material-coverage batch: " + oneLineClamp(errText, 240)
	}
	return dataworkflow.BuildCoverageExpansionPlan(dataworkflow.CoverageExpansionPlanInput{
		Current:            plan,
		Coverage:           contract,
		MissingPaths:       missing,
		DeriveRulesForText: dataTaskPlanShouldDeriveRulesForTextCoverage(plan),
		MaxActions:         dataTaskMaxActionsPerBatch,
		ValidationRule:     validationRule,
	})
}

func dataTaskPlanShouldDeriveRulesForTextCoverage(plan dataquery.TaskPlan) bool {
	return plan.CoverageContract.RuleCoverageRequired || dataTaskValidationLedgerCount(plan.CoverageContract) >= 2
}

func dataTaskCoverageExpansionFallbackNeeded(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) bool {
	if len(plan.Actions) == 0 && strings.TrimSpace(plan.Script) == "" && len(dataTaskCoverageExpansionMissingPaths(records, plan)) > 0 {
		return true
	}
	if !dataTaskTerminalRequiredMaterialSchedulingGuardResult(records, plan).Empty() {
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
	out := plan
	for i, action := range out.Actions {
		if narrowed, ok := dataTaskNarrowSingleRecordSetActionForExecution(records, action); ok {
			out.Actions[i] = narrowed
		}
	}
	return out
}

func dataTaskNarrowSingleRecordSetActionForExecution(records []dataTaskWorkflowRecord, action dataquery.DataAction) (dataquery.DataAction, bool) {
	if len(records) == 0 {
		return dataquery.DataAction{}, false
	}
	access, _, _ := dataTaskWorkflowArtifactAvailability(records, 512)
	if len(access) == 0 {
		return dataquery.DataAction{}, false
	}
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	switch kind {
	case dataquery.DataActionDeriveFields, dataquery.DataActionExtractFields, dataquery.DataActionGroupRecords, dataquery.DataActionExpandRecords, dataquery.DataActionFilterRecords, dataquery.DataActionQualifyRecords:
	default:
		return dataquery.DataAction{}, false
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if len(inputs) <= 1 {
		return dataquery.DataAction{}, false
	}
	fieldRefs := dataTaskSingleRecordSetActionFieldRefs(kind, action)
	if len(fieldRefs) == 0 {
		return dataquery.DataAction{}, false
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
		return dataquery.DataAction{}, false
	}
	action.InputPaths = []string{matches[0]}
	if strings.TrimSpace(action.Purpose) != "" && !strings.Contains(action.Purpose, "input narrowed by field contract") {
		action.Purpose = strings.TrimSpace(action.Purpose) + " (input narrowed by field contract)"
	}
	return action, true
}

func dataTaskSingleRecordSetActionFieldRefs(kind dataquery.DataActionKind, action dataquery.DataAction) []string {
	return dataworkflow.SingleRecordSetActionFieldRefs(kind, action)
}

func dataTaskDeriveActionSourceFieldRefs(action dataquery.DataAction) []string {
	return dataworkflow.DeriveActionSourceFieldRefs(action)
}

func dataTaskGroupRecordsActionFieldRefs(action dataquery.DataAction) []string {
	return dataworkflow.GroupRecordsActionFieldRefs(action)
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
	if !dataTaskTerminalRequiredMaterialSchedulingGuardResult(records, plan).Empty() {
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
	return dataTaskActionStagingGuardResult(plan).ErrorText()
}

func dataTaskWorkflowActionStagingGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	return dataTaskWorkflowActionStagingGuardResult(records, plan).ErrorText()
}

func dataTaskWorkflowNumericConstantReuseGuardError(plan dataquery.TaskPlan) string {
	return dataTaskWorkflowNumericConstantReuseGuardResult(plan).ErrorText()
}

func dataTaskWorkflowNumericConstantReuseGuardResult(plan dataquery.TaskPlan) dataworkflow.GuardResult {
	if len(plan.Actions) < 2 {
		return dataworkflow.GuardResult{}
	}
	var constants []dataworkflow.DerivedConstantFieldFact
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
			constants = append(constants, dataworkflow.DerivedConstantFieldFact{
				ActionIndex:   i,
				ActionID:      firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(kind))),
				Action:        action,
				Field:         field,
				Value:         value,
				OutputAliases: dataTaskActionOutputAliases(action),
			})
		}
	}
	if len(constants) == 0 {
		return dataworkflow.GuardResult{}
	}
	var uses []dataworkflow.NumericFieldUseFact
	for j, action := range plan.Actions {
		for _, field := range dataTaskNumericFieldUsesFromAction(action) {
			uses = append(uses, dataworkflow.NumericFieldUseFact{
				ActionIndex:  j,
				Action:       action,
				Field:        field,
				InputAliases: cleanDataTaskStrings(action.InputPaths),
			})
		}
	}
	return dataworkflow.NumericConstantReuseGuardResult(dataworkflow.NumericConstantReuseGuardInput{
		Constants: constants,
		Uses:      uses,
	})
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
	return dataTaskWorkflowCustomTransformDisabledGuardResult(records, plan).ErrorText()
}

func dataTaskWorkflowCustomTransformDisabledGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	state := dataTaskWorkflowState(records, plan)
	return dataworkflow.CustomTransformDisabledGuardResult(dataworkflow.CustomTransformDisabledGuardInput{
		RecordsPresent:      len(records) > 0,
		HasActions:          len(plan.Actions) > 0,
		HasSuccessfulResult: dataTaskWorkflowHasSuccessfulResult(records),
		State:               state,
		Actions:             plan.Actions,
	})
}

func dataTaskCustomTransformDisabledFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, guard dataworkflow.GuardResult) (dataquery.TaskPlan, bool) {
	if len(records) == 0 || guard.Empty() || !dataTaskGuardHasCode(guard, "custom_transform_disabled") {
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
	return dataworkflow.BuildConcreteFallbackPlan(dataworkflow.ConcreteFallbackPlanInput{
		Current:        current,
		Coverage:       dataTaskWorkflowCoverageContract(records, current),
		Output:         dataTaskWorkflowOutputContract(records, current),
		Scaffolds:      state.ActionScaffold,
		Facts:          state.Facts(),
		ReasonPrefix:   reasonPrefix,
		SeenActionKeys: dataTaskWorkflowSeenActionKeys(records),
	})
}

func dataTaskWorkflowSeenActionKeys(records []dataTaskWorkflowRecord) map[string]bool {
	var actions []dataquery.DataAction
	for _, rec := range records {
		actions = append(actions, rec.Plan.Actions...)
	}
	return dataworkflow.ActionIdempotencyKeys(actions)
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
		dataquery.DataActionMappingCandidate,
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
	return dataTaskWorkflowAllowedNextActionGuardResult(records, plan).ErrorText()
}

func dataTaskWorkflowAllowedNextActionGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	if len(records) == 0 || len(plan.Actions) == 0 {
		return dataworkflow.GuardResult{}
	}
	if !dataTaskWorkflowHasSuccessfulResult(records) {
		return dataworkflow.GuardResult{}
	}
	state := dataTaskWorkflowState(records, plan)
	if len(state.AllowedNextActions) == 0 {
		return dataworkflow.GuardResult{}
	}
	for i, action := range plan.Actions {
		if guard := dataworkflow.AllowedNextActionGuardResult(dataworkflow.AllowedNextActionGuardInput{
			State:                       state,
			Action:                      action,
			ActionIndex:                 i,
			GeneratedArtifactDiagnostic: dataTaskActionIsGeneratedArtifactDiagnostic(records, action),
			RecordMaterialization:       dataTaskActionIsRecordMaterialization(records, action),
		}); !guard.Empty() {
			return guard
		}
	}
	return dataworkflow.GuardResult{}
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
	return dataTaskWorkflowStageProgressGuardResult(records, plan).ErrorText()
}

func dataTaskWorkflowStageProgressGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	return dataworkflow.StageProgressGuardResult(dataworkflow.StageProgressGuardInput{
		State:                             state,
		HasScriptedCustomTransform:        dataTaskPlanHasScriptedCustomTransform(plan),
		CrossesTypedActionRanks:           dataTaskPlanCrossesTypedActionRanks(plan),
		NarrowSingleIntermediateTransform: dataTaskPlanIsNarrowSingleIntermediateCustomTransform(plan, state),
	})
}

func dataTaskWorkflowStagePrefixFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, guard dataworkflow.GuardResult) (dataquery.TaskPlan, bool) {
	prefix, _, ok := dataTaskWorkflowStagePrefixFallbackWithRemainder(records, plan, guard)
	return prefix, ok
}

func dataTaskWorkflowStagePrefixFallbackWithRemainder(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, guard dataworkflow.GuardResult) (dataquery.TaskPlan, dataquery.TaskPlan, bool) {
	if len(records) == 0 || len(plan.Actions) < 2 || guard.Empty() {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	if !dataTaskWorkflowHasSuccessfulResult(records) {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	state := dataTaskWorkflowState(records, plan)
	if len(state.AllowedNextActions) == 0 || len(state.MissingValidationStages()) <= 1 {
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

func dataTaskExecutablePrefixFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, guard dataworkflow.GuardResult) (dataquery.TaskPlan, bool) {
	if len(plan.Actions) < 2 || guard.Empty() {
		return dataquery.TaskPlan{}, false
	}
	if dataTaskWorkflowStagingGuardResult(records, plan).Empty() {
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
		if dataTaskWorkflowStagingGuardResult(records, out).Empty() {
			return out, true
		}
	}
	return dataquery.TaskPlan{}, false
}

func dataTaskPopDeferredActionBatch(records []dataTaskWorkflowRecord, deferred dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, bool) {
	next, remainder, _, ok := dataTaskPopDeferredActionBatchWithStatus(records, deferred)
	return next, remainder, ok
}

func dataTaskPopDeferredActionBatchWithStatus(records []dataTaskWorkflowRecord, deferred dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, dataworkflow.DeferredDispatchStatus, bool) {
	if len(deferred.Actions) == 0 {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, dataworkflow.DeferredDispatchStatus{ReasonCode: dataworkflow.DeferredBlockEmpty, Reason: "deferred queue is empty"}, false
	}
	state := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	if len(state.AllowedNextActions) == 0 {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, dataworkflow.DeferredDispatchStatus{Actions: len(deferred.Actions), ReasonCode: dataworkflow.DeferredBlockNoAllowedActions, Reason: "workflow has no allowed next typed actions"}, false
	}
	out, remainder, status, ok := dataworkflow.BuildDeferredDispatchPlan(dataworkflow.DeferredDispatchInput{
		Plan:               deferred,
		Candidates:         dataTaskDeferredActionCandidates(records, deferred.Actions),
		AllowedNextActions: state.AllowedNextActions,
	})
	if !ok {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, status, false
	}
	if guard := dataTaskDeferredDispatchGuardResult(records, deferred, out.Actions); !guard.Empty() {
		status.Ready = false
		status.ReasonCode = dataworkflow.DeferredBlockAdmissionRejected
		status.Reason = guard.ErrorText()
		status.GuardCode = strings.TrimSpace(guard.Code)
		status.Guard = &guard
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, status, false
	}
	status.Ready = true
	return out, remainder, status, true
}

func dataTaskDeferredReadyAction(records []dataTaskWorkflowRecord, action dataquery.DataAction) (dataquery.DataAction, bool) {
	if narrowed, ok := dataTaskNarrowSingleRecordSetActionForExecution(records, action); ok {
		action = narrowed
	}
	if dataTaskDeferredActionInputsReady(records, action) {
		return action, true
	}
	if rewritten, ok := dataTaskRewriteDeferredActionFirstInputToCompatibleArtifact(records, action); ok && dataTaskDeferredActionInputsReady(records, rewritten) {
		return rewritten, true
	}
	return dataquery.DataAction{}, false
}

func dataTaskDeferredDispatchGuardResult(records []dataTaskWorkflowRecord, deferred dataquery.TaskPlan, actions []dataquery.DataAction) dataworkflow.GuardResult {
	if len(actions) == 0 {
		return dataworkflow.GuardResult{}
	}
	candidate := deferred
	candidate.Status = "ready"
	candidate.Actions = append([]dataquery.DataAction(nil), actions...)
	candidate.Script = ""
	candidate.ContinueAfter = true
	return dataTaskWorkflowStagingGuardResult(records, candidate)
}

type dataTaskDeferredQueueState = dataworkflow.DeferredDispatchStatus

func dataTaskDeferredQueueStatus(records []dataTaskWorkflowRecord, deferred dataquery.TaskPlan) dataTaskDeferredQueueState {
	workflow := dataTaskWorkflowState(records, dataquery.TaskPlan{})
	next, _, state, ok := dataworkflow.BuildDeferredDispatchPlan(dataworkflow.DeferredDispatchInput{
		Plan:               deferred,
		Candidates:         dataTaskDeferredActionCandidates(records, deferred.Actions),
		AllowedNextActions: workflow.AllowedNextActions,
	})
	if !ok {
		return state
	}
	if guard := dataTaskDeferredDispatchGuardResult(records, deferred, next.Actions); !guard.Empty() {
		state.Reason = guard.ErrorText()
		state.ReasonCode = dataworkflow.DeferredBlockAdmissionRejected
		state.GuardCode = strings.TrimSpace(guard.Code)
		state.Guard = &guard
		state.Ready = false
		return state
	}
	state.Ready = true
	return state
}

func dataTaskDeferredActionCandidates(records []dataTaskWorkflowRecord, actions []dataquery.DataAction) []dataworkflow.DeferredActionCandidate {
	out := make([]dataworkflow.DeferredActionCandidate, 0, len(actions))
	for i, action := range actions {
		readyAction, ok := dataTaskDeferredReadyAction(records, action)
		candidate := dataworkflow.DeferredActionCandidate{
			Index:  i,
			Action: action,
			Ready:  ok,
		}
		if ok {
			candidate.Action = readyAction
		} else {
			candidate.BlockedCode, candidate.BlockedReason = dataTaskDeferredActionBlockedStatus(records, action)
		}
		out = append(out, candidate)
	}
	return out
}

func dataTaskDeferredActionBlockedReason(records []dataTaskWorkflowRecord, action dataquery.DataAction) string {
	_, reason := dataTaskDeferredActionBlockedStatus(records, action)
	return reason
}

func dataTaskDeferredActionBlockedStatus(records []dataTaskWorkflowRecord, action dataquery.DataAction) (string, string) {
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
			return dataworkflow.DeferredBlockInputUnavailable, fmt.Sprintf("deferred action %s:%s waits for unavailable input alias/material [%s]",
				firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))),
				normalizeDataActionKindForWorkflow(action.Kind),
				strings.Join(missing, ", "))
		}
	}
	if guard := dataTaskActionFieldContractGuardResult(records, dataquery.TaskPlan{}, action, 0); !guard.Empty() {
		return firstNonEmptyString(strings.TrimSpace(guard.Code), dataworkflow.DeferredBlockFieldContract), guard.ErrorText()
	}
	return dataworkflow.DeferredBlockNotReady, ""
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

func dataTaskDeferredQueueRetainedSegment(lang string, state dataTaskDeferredQueueState) string {
	reason := oneLineClamp(state.Reason, 180)
	action := firstNonEmptyString(state.FirstActionID, state.FirstActionKind, "deferred action")
	if isZh(lang) {
		parts := []string{fmt.Sprintf("延后队列已保留：首个 %s 尚未就绪", action)}
		if state.ReadyActions > 0 || state.BlockedActions > 0 {
			parts = append(parts, fmt.Sprintf("ready=%d blocked=%d", state.ReadyActions, state.BlockedActions))
		}
		if strings.TrimSpace(state.ReasonCode) != "" {
			parts = append(parts, "原因码 "+state.ReasonCode)
		}
		if reason != "" {
			parts = append(parts, reason)
		}
		return strings.Join(parts, "；")
	}
	parts := []string{fmt.Sprintf("retained deferred queue: first %s is not ready", action)}
	if state.ReadyActions > 0 || state.BlockedActions > 0 {
		parts = append(parts, fmt.Sprintf("ready=%d blocked=%d", state.ReadyActions, state.BlockedActions))
	}
	if strings.TrimSpace(state.ReasonCode) != "" {
		parts = append(parts, "reason_code "+state.ReasonCode)
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
	if !dataTaskActionFieldContractGuardResult(records, dataquery.TaskPlan{}, action, 0).Empty() {
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

func dataTaskInvalidRecordActionFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, guard dataworkflow.GuardResult) (dataquery.TaskPlan, bool) {
	if guard.Empty() || len(plan.Actions) == 0 {
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
	if (state.NextStage == "prepare_contribution_inputs" || state.NextStage == "compute_contributions") && len(state.MissingValidationStages()) > 1 {
		return false
	}
	return true
}

func dataTaskTextConstraintCoverageGuardError(plan dataquery.TaskPlan) string {
	return dataTaskTextConstraintCoverageGuardResult(plan).ErrorText()
}

func dataTaskTextConstraintCoverageGuardResult(plan dataquery.TaskPlan) dataworkflow.GuardResult {
	var customInputs []string
	if len(plan.Actions) == 0 {
		if strings.TrimSpace(plan.Script) != "" {
			customInputs = append(customInputs, plan.InputPaths...)
		}
	} else {
		for _, action := range plan.Actions {
			if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform || strings.TrimSpace(action.Script) == "" {
				continue
			}
			customInputs = append(customInputs, action.InputPaths...)
		}
	}
	var materials []string
	for _, material := range plan.CoverageContract.RequiredMaterials {
		if normalizeCoverageMaterialUseModeForWorkflow(material.UsageMode) != dataquery.MaterialUseScriptConsumed {
			continue
		}
		materials = append(materials, material.Path)
	}
	return dataworkflow.TextConstraintCoverageGuardResult(dataworkflow.TextConstraintCoverageGuardInput{
		ContinueAfter:               plan.ContinueAfter,
		RuleCoverageRequired:        plan.CoverageContract.RuleCoverageRequired,
		ValidationLedgerCount:       dataTaskValidationLedgerCount(plan.CoverageContract),
		CustomInputPaths:            customInputs,
		ScriptConsumedMaterialPaths: materials,
	})
}

func dataTaskPathLooksLikeTextConstraintMaterial(p string) bool {
	return dataworkflow.PathLooksLikeTextConstraintMaterial(p)
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
	return dataTaskTerminalRequiredMaterialSchedulingGuardResult(records, plan).ErrorText()
}

func dataTaskTerminalRequiredMaterialSchedulingGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	scheduled := dataTaskScheduledMaterialConsumption(records, plan)
	scheduledPaths := make([]string, 0, len(scheduled))
	for p := range scheduled {
		scheduledPaths = append(scheduledPaths, p)
	}
	return dataworkflow.RequiredMaterialSchedulingGuardResult(dataworkflow.RequiredMaterialSchedulingGuardInput{
		ContinueAfter:  plan.ContinueAfter,
		RequiredPaths:  plan.CoverageContract.RequiredRunnerInputPaths(),
		ScheduledPaths: scheduledPaths,
	})
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
	if guard := dataTaskActionIntraBatchDependencyGuardResult(plan, actionIndex); !guard.Empty() {
		return guard
	}
	if guard := dataTaskActionInputAvailabilityGuardResult(records, plan, action, actionIndex); !guard.Empty() {
		return guard
	}
	if guard := dataTaskActionSchemaShapeGuardResult(records, action, actionIndex); !guard.Empty() {
		return guard
	}
	if guard := dataTaskActionFieldContractGuardResult(records, plan, action, actionIndex); !guard.Empty() {
		return guard
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if contract, ok := dataworkflow.InputPathContract(kind); ok {
		if guard := dataworkflow.InputPathContractGuardResult(dataworkflow.InputPathContractGuardInput{
			Action:      action,
			ActionIndex: actionIndex,
			Contract:    contract,
		}); !guard.Empty() {
			return guard
		}
	}
	switch kind {
	case dataquery.DataActionInspectMaterial, dataquery.DataActionExtractRecords, dataquery.DataActionDeriveFields, dataquery.DataActionExtractFields, dataquery.DataActionGroupRecords, dataquery.DataActionExpandRecords, dataquery.DataActionFilterRecords, dataquery.DataActionQualifyRecords:
		if kind == dataquery.DataActionDeriveFields && !dataTaskDeriveFieldsActionHasSpec(action) {
			return dataworkflow.MissingActionSpecGuardResult(dataworkflow.MissingActionSpecGuardInput{Action: action, ActionIndex: actionIndex})
		}
		if kind == dataquery.DataActionExtractFields && !dataTaskDeriveFieldsActionHasSpec(action) {
			return dataworkflow.MissingActionSpecGuardResult(dataworkflow.MissingActionSpecGuardInput{Action: action, ActionIndex: actionIndex})
		}
		if kind == dataquery.DataActionGroupRecords && !dataTaskGroupRecordsActionHasSpec(action) {
			return dataworkflow.MissingActionSpecGuardResult(dataworkflow.MissingActionSpecGuardInput{Action: action, ActionIndex: actionIndex})
		}
		if kind == dataquery.DataActionExpandRecords && !dataTaskExpandRecordsActionHasSpec(action) {
			return dataworkflow.MissingActionSpecGuardResult(dataworkflow.MissingActionSpecGuardInput{Action: action, ActionIndex: actionIndex})
		}
		if kind == dataquery.DataActionFilterRecords && !dataTaskFilterRecordsActionHasSpec(action) {
			return dataworkflow.MissingActionSpecGuardResult(dataworkflow.MissingActionSpecGuardInput{Action: action, ActionIndex: actionIndex})
		}
	case dataquery.DataActionApplyResolutions:
		if len(inputs) < 2 && strings.TrimSpace(action.Params["resolution_path"]) == "" && strings.TrimSpace(action.Params["resolution_specs_json"]) == "" {
			return dataworkflow.MissingApplyResolutionInputGuardResult(action, actionIndex)
		}
	case dataquery.DataActionReconcile:
		if !dataTaskWorkflowHasContributionProducer(records, plan, actionIndex) {
			return dataworkflow.MissingUpstreamLedgerGuardResult(dataworkflow.MissingUpstreamLedgerGuardInput{Action: action, ActionIndex: actionIndex, Ledger: dataworkflow.LedgerContributions})
		}
	case dataquery.DataActionAssembleAnswer:
		if !dataTaskWorkflowHasReconcileProducer(records, plan, actionIndex) {
			return dataworkflow.MissingUpstreamLedgerGuardResult(dataworkflow.MissingUpstreamLedgerGuardInput{Action: action, ActionIndex: actionIndex, Ledger: dataworkflow.LedgerReconcile})
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskActionIntraBatchDependencyGuardError(plan dataquery.TaskPlan, actionIndex int) string {
	return dataTaskActionIntraBatchDependencyGuardResult(plan, actionIndex).ErrorText()
}

func dataTaskActionIntraBatchDependencyGuardResult(plan dataquery.TaskPlan, actionIndex int) dataworkflow.GuardResult {
	dependencyIndex, input, producer, ok := dataTaskFirstIntraBatchDependency(plan.Actions)
	if !ok || dependencyIndex != actionIndex {
		return dataworkflow.GuardResult{}
	}
	return dataworkflow.IntraBatchDependencyGuardResult(dataworkflow.IntraBatchDependencyGuardInput{
		Action:        plan.Actions[actionIndex],
		ActionIndex:   actionIndex,
		ConsumedInput: input,
		Producer:      producer,
	})
}

func dataTaskActionInputAvailabilityGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	return dataTaskActionInputAvailabilityGuardResult(records, plan, action, actionIndex).ErrorText()
}

func dataTaskActionInputAvailabilityGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	if len(records) == 0 || !dataTaskWorkflowHasSuccessfulResult(records) {
		return dataworkflow.GuardResult{}
	}
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	switch kind {
	case dataquery.DataActionMaterialInventory,
		dataquery.DataActionInspectMaterial,
		dataquery.DataActionExtractRecords,
		dataquery.DataActionDeriveRules:
		return dataworkflow.GuardResult{}
	}
	inputs := cleanDataTaskStrings(action.InputPaths)
	if len(inputs) == 0 {
		return dataworkflow.GuardResult{}
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
		return dataworkflow.GuardResult{}
	}
	sort.Strings(missing)
	return dataworkflow.UnavailableActionInputGuardResult(dataworkflow.UnavailableActionInputGuardInput{
		Action:      action,
		ActionIndex: actionIndex,
		Missing:     missing,
	})
}

func dataTaskActionFieldContractGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	return dataTaskActionFieldContractGuardResult(records, plan, action, actionIndex).ErrorText()
}

func dataTaskActionSchemaShapeGuardResult(records []dataTaskWorkflowRecord, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	if len(records) == 0 || !dataTaskWorkflowHasSuccessfulResult(records) {
		return dataworkflow.GuardResult{}
	}
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	if !dataTaskActionRequiresRecordInputs(kind) {
		return dataworkflow.GuardResult{}
	}
	projections := dataTaskWorkflowArtifactSchemaProjections(records)
	if len(projections) == 0 {
		return dataworkflow.GuardResult{}
	}
	for _, input := range cleanDataTaskStrings(action.InputPaths) {
		projection, ok := dataworkflow.ArtifactSchemaByAlias(projections, input)
		if !ok {
			continue
		}
		if dataTaskArtifactSchemaCompatibleRecordInput(projection) {
			continue
		}
		reason := fmt.Sprintf("data planning incomplete: action %d (%s) consumes input %s, but artifact schema projection classifies it as node_class=%s json_shape=%s kind=%s, not a record-shaped artifact for this typed action. Use a record-producing typed action first, choose a record-shaped alias from artifact_graph, or inspect the artifact schema before consuming it.",
			actionIndex+1,
			firstNonEmptyString(strings.TrimSpace(action.ID), string(kind)),
			input,
			firstNonEmptyString(strings.TrimSpace(projection.NodeClass), "unknown"),
			firstNonEmptyString(strings.TrimSpace(projection.JSONShape), "unknown"),
			firstNonEmptyString(strings.TrimSpace(projection.Kind), "unknown"),
		)
		violation := dataworkflow.NewActionInputViolation(
			"artifact_schema_incompatible",
			"error",
			dataworkflow.RepairNeedsTypedAction,
			action,
			input,
			nil,
			reason,
			[]string{"choose a record-shaped artifact alias or materialize one with a typed record-producing action"},
		)
		violation.AvailableFieldSample = append([]string(nil), projection.Fields...)
		return dataworkflow.NewGuardResult("artifact_schema_incompatible", "error", dataworkflow.RepairNeedsTypedAction, reason, violation)
	}
	return dataworkflow.GuardResult{}
}

func dataTaskArtifactSchemaCompatibleRecordInput(projection dataworkflow.ArtifactSchemaProjection) bool {
	if len(projection.Fields) == 0 {
		return false
	}
	if projection.NodeClass == dataworkflow.ArtifactNodeClassDiagnosticChild || projection.NodeClass == dataworkflow.ArtifactNodeClassWorkflowLedger {
		return false
	}
	if projection.NodeClass == dataworkflow.ArtifactNodeClassRecord {
		return true
	}
	shape := strings.ToLower(strings.TrimSpace(projection.JSONShape))
	return strings.Contains(shape, "array") || strings.Contains(shape, "records")
}

func dataTaskActionRequiresRecordInputs(kind dataquery.DataActionKind) bool {
	switch kind {
	case dataquery.DataActionDeriveFields,
		dataquery.DataActionGroupRecords,
		dataquery.DataActionExpandRecords,
		dataquery.DataActionFilterRecords,
		dataquery.DataActionValueDistribution,
		dataquery.DataActionQualifyRecords,
		dataquery.DataActionMappingCandidate,
		dataquery.DataActionNormalizeEntities,
		dataquery.DataActionApplyResolutions,
		dataquery.DataActionEnrichRecords,
		dataquery.DataActionJoinRecords,
		dataquery.DataActionComputeContribs:
		return true
	default:
		return false
	}
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
	case dataquery.DataActionMappingCandidate:
		return dataTaskMappingCandidateActionFieldContractGuardResult(access, action, actionIndex)
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
					return dataworkflow.ZeroMatchFilterGuardResult(dataworkflow.ZeroMatchFilterGuardInput{
						Action:            action,
						ActionIndex:       actionIndex,
						InputAlias:        input,
						ArtifactID:        issue.ArtifactID,
						InputRows:         issue.InputRows,
						OutputRows:        issue.OutputRows,
						SourcePath:        issue.InputPath,
						FilterFields:      issue.FilterFields,
						Reason:            issue.Reason,
						RepairActionHints: issue.RepairActionHints,
					})
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
					return dataworkflow.UnmatchedResolutionGuardResult(dataworkflow.UnmatchedResolutionGuardInput{
						Action:            action,
						ActionIndex:       actionIndex,
						InputAlias:        input,
						ArtifactID:        issue.ArtifactID,
						BasePath:          issue.BasePath,
						BaseRows:          issue.BaseRows,
						TargetFields:      issue.TargetFields,
						Reason:            issue.Reason,
						RepairActionHints: issue.RepairActionHints,
					})
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
					return dataworkflow.ZeroEligibleGuardResult(dataworkflow.ZeroEligibleGuardInput{
						Action:            action,
						ActionIndex:       actionIndex,
						InputAlias:        input,
						ArtifactID:        issue.ArtifactID,
						InputRows:         issue.InputRows,
						EligibleRows:      issue.EligibleRows,
						Reason:            issue.Reason,
						RepairActionHints: issue.RepairActionHints,
					})
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
	return dataworkflow.FieldContractGuardResult(dataworkflow.FieldContractGuardInput{
		Action:      action,
		ActionIndex: actionIndex,
		Violation:   violation,
	})
}

func dataTaskActionInputContractGuardResult(code string, action dataquery.DataAction, inputAlias string, missingFields []string, message string, hints []string) dataworkflow.GuardResult {
	return dataworkflow.ActionInputContractGuardResult(dataworkflow.ActionInputContractGuardInput{
		Code:              code,
		Action:            action,
		InputAlias:        inputAlias,
		MissingFields:     missingFields,
		Message:           message,
		RepairActionHints: hints,
	})
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
	return dataworkflow.ArtifactAccessSchemaProjections(access)
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
	return dataworkflow.FilterActionFieldRefs(action)
}

func dataTaskQualifyActionFieldRefs(action dataquery.DataAction) []string {
	return dataworkflow.QualifyActionFieldRefs(action)
}

func dataTaskComputeActionFieldRefs(action dataquery.DataAction) []string {
	return dataworkflow.ComputeContributionActionFieldRefs(action)
}

func dataTaskJoinActionFieldContractGuardError(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) string {
	return dataTaskJoinActionFieldContractGuardResult(access, action, actionIndex).ErrorText()
}

func dataTaskJoinActionFieldContractGuardResult(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	for _, req := range dataworkflow.JoinActionFieldRequirements(action) {
		if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, req.Path, dataTaskMissingFieldsOnArtifact(access, req.Path, req.Fields), access); !guard.Empty() {
			return guard
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskNormalizeActionFieldContractGuardError(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) string {
	return dataTaskNormalizeActionFieldContractGuardResult(access, action, actionIndex).ErrorText()
}

func dataTaskMappingCandidateActionFieldContractGuardResult(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	return dataTaskNormalizeActionFieldContractGuardResult(access, action, actionIndex)
}

func dataTaskNormalizeActionFieldContractGuardResult(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	requirements, skip := dataworkflow.NormalizeEntityActionFieldRequirements(action)
	if skip {
		return dataworkflow.GuardResult{}
	}
	sourceReq, sourceOK := dataTaskActionFieldRequirementByRole(requirements, "source")
	referenceReq, referenceOK := dataTaskActionFieldRequirementByRole(requirements, "reference")
	if !sourceOK {
		return dataworkflow.GuardResult{}
	}
	if referenceOK && !sourceReq.Explicit && !referenceReq.Explicit && len(sourceReq.Fields) > 0 {
		sourceMissing := dataTaskMissingFieldsOnArtifact(access, sourceReq.Path, sourceReq.Fields)
		referenceCanBeSource := len(dataTaskMissingFieldsOnArtifact(access, referenceReq.Path, sourceReq.Fields)) == 0
		if len(sourceMissing) > 0 && referenceCanBeSource {
			sourceReq.Path, referenceReq.Path = referenceReq.Path, sourceReq.Path
		}
	}
	if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, sourceReq.Path, dataTaskMissingFieldsOnArtifact(access, sourceReq.Path, sourceReq.Fields), access); !guard.Empty() {
		return guard
	}
	if referenceOK {
		if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, referenceReq.Path, dataTaskMissingFieldsOnArtifact(access, referenceReq.Path, referenceReq.Fields), access); !guard.Empty() {
			return guard
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskActionFieldRequirementByRole(requirements []dataworkflow.ActionFieldRequirement, role string) (dataworkflow.ActionFieldRequirement, bool) {
	role = strings.TrimSpace(role)
	for _, req := range requirements {
		if strings.TrimSpace(req.Role) == role {
			return req, true
		}
	}
	return dataworkflow.ActionFieldRequirement{}, false
}

func dataTaskEnrichActionFieldContractGuardError(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) string {
	return dataTaskEnrichActionFieldContractGuardResult(access, action, actionIndex).ErrorText()
}

func dataTaskEnrichActionFieldContractGuardResult(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int) dataworkflow.GuardResult {
	for _, req := range dataworkflow.EnrichActionFieldRequirements(action) {
		if guard := dataTaskActionMissingFieldContractGuardResult(action, actionIndex, req.Path, dataTaskMissingFieldsOnArtifact(access, req.Path, req.Fields), access); !guard.Empty() {
			return guard
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
			return dataworkflow.ApplyResolutionDiagnosticInputGuardResult(dataworkflow.ApplyResolutionDiagnosticInputGuardInput{
				Action:         action,
				ActionIndex:    actionIndex,
				ResolutionPath: resolutionPath,
			})
		}
		baseArtifact, baseOK := dataTaskArtifactAccessByAlias(access, basePath)
		if mapping, ok := dataTaskArtifactAccessByAlias(access, resolutionPath); ok && baseOK && !dataTaskApplyResolutionBaseCompatibleWithMapping(baseArtifact, mapping) {
			return dataworkflow.ApplyResolutionLineageGuardResult(dataworkflow.ApplyResolutionLineageGuardInput{
				Action:                  action,
				ActionIndex:             actionIndex,
				ResolutionPath:          resolutionPath,
				BasePath:                basePath,
				ResolutionSourceLineage: dataTaskArtifactSourceLineageSummary(mapping),
				BaseLineage:             dataTaskArtifactLineageSummary(basePath, baseArtifact),
			})
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
			return dataworkflow.ApplyResolutionCanonicalFieldGuardResult(dataworkflow.ApplyResolutionCanonicalFieldGuardInput{
				Action:          action,
				ActionIndex:     actionIndex,
				SpecIndex:       i,
				ResolutionPath:  resolutionPath,
				CanonicalFields: cleanDataTaskStrings([]string{canonicalIDField, canonicalLabelField}),
				AvailableFields: artifact.Fields,
			})
		}
		if guard := dataTaskApplyResolutionNoProgressGuardResult(access, action, actionIndex, specBasePath, resolutionPath, spec, len(specs)); !guard.Empty() {
			return guard
		}
	}
	return dataworkflow.GuardResult{}
}

func dataTaskApplyResolutionNoProgressGuardResult(access []dataTaskArtifactAccessPrompt, action dataquery.DataAction, actionIndex int, basePath, resolutionPath string, spec map[string]any, specCount int) dataworkflow.GuardResult {
	base, ok := dataTaskArtifactAccessByAlias(access, basePath)
	if !ok || len(base.Fields) == 0 || strings.TrimSpace(resolutionPath) == "" {
		return dataworkflow.GuardResult{}
	}
	targetFields := dataTaskApplyResolutionTargetFields(action, spec, specCount)
	if len(targetFields) == 0 {
		return dataworkflow.GuardResult{}
	}
	if !dataTaskArtifactHasAnyNamedField(base, targetFields...) {
		return dataworkflow.GuardResult{}
	}
	if !dataTaskArtifactLineageContains(base, resolutionPath) {
		return dataworkflow.GuardResult{}
	}
	return dataworkflow.ApplyResolutionNoProgressGuardResult(dataworkflow.ApplyResolutionNoProgressGuardInput{
		Action:         action,
		ActionIndex:    actionIndex,
		ResolutionPath: resolutionPath,
		BasePath:       basePath,
		TargetFields:   targetFields,
	})
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
	return dataworkflow.ArtifactLineageContains(dataTaskArtifactAccessToSchemaProjection(artifact), alias)
}

func dataTaskApplyResolutionBaseCompatibleWithMapping(base dataTaskArtifactAccessPrompt, mapping dataTaskArtifactAccessPrompt) bool {
	return dataworkflow.ArtifactResolutionLineageCompatible(
		dataTaskArtifactAccessToSchemaProjection(base),
		dataTaskArtifactAccessToSchemaProjection(mapping),
	)
}

func dataTaskArtifactLineageSummary(alias string, artifact dataTaskArtifactAccessPrompt) string {
	return dataworkflow.ArtifactLineageSummary(alias, dataTaskArtifactAccessToSchemaProjection(artifact))
}

func dataTaskArtifactSourceLineageSummary(artifact dataTaskArtifactAccessPrompt) string {
	return dataworkflow.ArtifactSourceLineageSummary(dataTaskArtifactAccessToSchemaProjection(artifact))
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
	return dataTaskCoverageLoopGuardResult(records, plan).ErrorText()
}

func dataTaskCoverageLoopGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) dataworkflow.GuardResult {
	state := dataTaskWorkflowState(records, plan)
	return dataworkflow.CoverageLoopGuardResult(dataworkflow.CoverageLoopGuardInput{
		State:                            state,
		CoverageOnly:                     dataTaskPlanIsCoverageOnly(plan),
		GeneratedArtifactDiagnostic:      dataTaskPlanIsGeneratedArtifactDiagnostic(records, plan),
		ActionsAllAllowedForCurrentStage: dataTaskPlanActionsAllAllowedForWorkflowStage(plan, state),
	})
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
	return dataworkflow.RepeatedCustomTransformGuardResult(action, dataTaskWorkflowErrorTexts(records), DefaultDataTaskMaxNodeFailures).ErrorText()
}

func dataTaskRepeatedCustomTransformClassGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) string {
	broad := dataTaskActionHasBroadPrerequisiteSurface(plan, action) || dataTaskActionLooksLikeWholeWorkflow(plan, action, actionIndex)
	return dataworkflow.RepeatedCustomTransformClassGuardResult(
		action,
		broad,
		dataTaskCustomTransformFailureErrors(records),
		DefaultDataTaskMaxCustomTransformClassFailures,
		DefaultDataTaskMaxNodeFailures,
	).ErrorText()
}

func dataTaskCustomTransformFailureClassStats(records []dataTaskWorkflowRecord) (total int, topCode string, topCodeCount int) {
	return dataworkflow.CustomTransformFailureClassStats(dataTaskCustomTransformFailureErrors(records))
}

func dataTaskWorkflowErrorTexts(records []dataTaskWorkflowRecord) []string {
	var out []string
	for _, rec := range records {
		if strings.TrimSpace(rec.Err) == "" {
			continue
		}
		out = append(out, rec.Err)
	}
	return out
}

func dataTaskCustomTransformFailureErrors(records []dataTaskWorkflowRecord) []string {
	var out []string
	for _, rec := range records {
		if strings.TrimSpace(rec.Err) == "" || !dataTaskRecordHasCustomTransformScript(rec) {
			continue
		}
		out = append(out, rec.Err)
	}
	return out
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
	return dataTaskTerminalRawMaterialCustomTransformGuardResult(records, plan, action, actionIndex, lines).ErrorText()
}

func dataTaskTerminalRawMaterialCustomTransformGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex, lines int) dataworkflow.GuardResult {
	state := dataTaskWorkflowState(records, plan)
	return dataworkflow.TerminalRawMaterialCustomTransformGuardResult(dataworkflow.TerminalRawMaterialCustomTransformGuardInput{
		RecordsPresent:         len(records) > 0,
		ContinueAfter:          plan.ContinueAfter,
		State:                  state,
		Action:                 action,
		ActionIndex:            actionIndex,
		ScriptLines:            lines,
		RawInputAliases:        dataTaskCustomTransformRawMaterialInputs(records, action),
		ComplexScriptLineLimit: dataTaskComplexCustomScriptLineLimit,
	})
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
	return dataTaskBroadCustomPrerequisiteGuardResult(records, plan, dataquery.DataAction{}, -1).ErrorText()
}

func dataTaskBroadCustomPrerequisiteGuardResult(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, target dataquery.DataAction, targetIndex int) dataworkflow.GuardResult {
	if len(plan.Actions) == 0 {
		return dataworkflow.GuardResult{}
	}
	for i, action := range plan.Actions {
		if targetIndex >= 0 && i != targetIndex {
			continue
		}
		if targetIndex < 0 && strings.TrimSpace(target.ID) != "" && strings.TrimSpace(action.ID) != strings.TrimSpace(target.ID) {
			continue
		}
		if normalizeDataActionKindForWorkflow(action.Kind) != dataquery.DataActionCustomTransform {
			continue
		}
		missing := dataTaskMissingCustomTransformPrerequisites(records, plan, action, i)
		guard := dataworkflow.BroadCustomPrerequisiteGuardResult(dataworkflow.BroadCustomPrerequisiteGuardInput{
			Action:      action,
			ActionIndex: i,
			IsBroad:     dataTaskActionHasBroadPrerequisiteSurface(plan, action),
			Missing:     missing,
		})
		if !guard.Empty() {
			return guard
		}
	}
	return dataworkflow.GuardResult{}
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

type dataTaskWorkflowRecord = dataworkflow.WorkflowRecord

type dataTaskWorkflowPlanPreflight = dataworkflow.ActionDAGAdmissionDecision

const dataTaskPreflightMaxRewrites = 3

func dataTaskPreflightWorkflowPlan(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, protect func(dataquery.TaskPlan) dataquery.TaskPlan) dataTaskWorkflowPlanPreflight {
	return dataworkflow.AdmitActionDAGPlan(dataworkflow.ActionDAGAdmissionInput{
		Plan:        plan,
		Protect:     protect,
		MaxRewrites: dataTaskPreflightMaxRewrites,
		PrefixFallback: func(current dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, string, bool) {
			fallback, remainder, ok := dataTaskInitialRankPrefixFallback(records, current)
			if !ok {
				return dataquery.TaskPlan{}, dataquery.TaskPlan{}, "", false
			}
			return fallback, remainder, "split initial data plan at typed dependency rank", true
		},
		Guard: func(current dataquery.TaskPlan) dataworkflow.GuardResult {
			return dataTaskWorkflowStagingGuardResult(records, current)
		},
		DeterministicFallback: func(current dataquery.TaskPlan, guard dataworkflow.GuardResult) (dataquery.TaskPlan, dataquery.TaskPlan, string, bool) {
			return dataTaskWorkflowDeterministicFallback(records, current, guard)
		},
	})
}

func dataTaskAdmissionDecisionForPlan(decision dataworkflow.ActionDAGAdmissionDecision, plan dataquery.TaskPlan) *dataworkflow.ActionDAGAdmissionDecision {
	if !dataTaskAdmissionPlanMatches(decision.Plan, plan) {
		return nil
	}
	copied := decision
	return &copied
}

func dataTaskWorkflowRecordForGuard(plan dataquery.TaskPlan, guard dataworkflow.GuardResult) dataTaskWorkflowRecord {
	errText := guard.ErrorText()
	return dataTaskWorkflowRecord{
		Plan:      plan,
		Err:       errText,
		Admission: dataTaskAdmissionDecisionForGuard(plan, guard),
	}
}

func dataTaskAdmissionDecisionForGuard(plan dataquery.TaskPlan, guard dataworkflow.GuardResult) *dataworkflow.ActionDAGAdmissionDecision {
	if guard.Empty() {
		return nil
	}
	copiedPlan := plan
	decision := dataworkflow.ActionDAGAdmissionDecision{
		Plan:          copiedPlan,
		Original:      copiedPlan,
		Guard:         guard,
		FinalGuard:    guard,
		GuardErr:      guard.ErrorText(),
		FinalGuardErr: guard.ErrorText(),
	}
	return &decision
}

func dataTaskAdmissionPlanMatches(a, b dataquery.TaskPlan) bool {
	if len(a.Actions) != len(b.Actions) {
		return false
	}
	if strings.TrimSpace(a.Status) != strings.TrimSpace(b.Status) {
		return false
	}
	if strings.TrimSpace(a.Script) != strings.TrimSpace(b.Script) {
		return false
	}
	for i := range a.Actions {
		if dataworkflow.ActionIdempotencyKey(a.Actions[i]) != dataworkflow.ActionIdempotencyKey(b.Actions[i]) {
			return false
		}
	}
	return true
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

type dataTaskWorkflowStateView = dataworkflow.WorkflowStateView

type dataTaskActionContract = dataworkflow.ActionContract

type dataTaskFieldContractViolation = dataworkflow.FieldContractIssue

type dataTaskZeroMatchFilterIssue = dataworkflow.ZeroMatchFilterIssue

type dataTaskUnmatchedResolutionIssue = dataworkflow.UnmatchedResolutionIssue

type dataTaskZeroEligibleIssue = dataworkflow.ZeroEligibleIssue

type dataTaskResultPromptView = dataworkflow.ResultPromptView

type dataTaskLedgerProjection = dataworkflow.LedgerProjection

type dataTaskArtifactAccessPrompt = dataworkflow.ArtifactAccessView

type dataTaskMaterialSetHandlePrompt = dataworkflow.MaterialCollectionView

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
	return dataworkflow.RenderWorkflowRecordViewsForPrompt(records, dataworkflow.WorkflowRecordViewBudget{
		MaxRecords:          budget.MaxRecords,
		MaxActions:          budget.MaxActions,
		ActionScriptLimit:   budget.ActionScriptLimit,
		ActionParamLimit:    budget.ActionParamLimit,
		ScriptPreviewLimit:  budget.ScriptPreviewLimit,
		ErrorLimit:          budget.ErrorLimit,
		EvalReasonLimit:     budget.EvalReasonLimit,
		ResultAnswerLimit:   budget.ResultAnswerLimit,
		ResultAuditLimit:    budget.ResultAuditLimit,
		ResultDecisionLimit: budget.ResultDecisionLimit,
		ResultRuleLimit:     budget.ResultRuleLimit,
		ResultContribLimit:  budget.ResultContribLimit,
		ResultArtifactLimit: budget.ResultArtifactLimit,
		ResultAccessLimit:   budget.ResultAccessLimit,
		ResultMaterialLimit: budget.ResultMaterialLimit,
	}, func(result dataquery.Result, budget dataworkflow.WorkflowRecordViewBudget) any {
		return compactDataTaskResultPromptViewWithArtifactLimits(result,
			budget.ResultAnswerLimit,
			budget.ResultAuditLimit,
			budget.ResultDecisionLimit,
			budget.ResultRuleLimit,
			budget.ResultContribLimit,
			budget.ResultArtifactLimit,
			budget.ResultAccessLimit,
			budget.ResultMaterialLimit,
		)
	})
}

func dataTaskWorkflowState(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataTaskWorkflowStateView {
	return dataTaskWorkflowStateWithDeferred(records, current, dataquery.TaskPlan{})
}

func dataTaskWorkflowStateWithDeferred(records []dataTaskWorkflowRecord, current, deferred dataquery.TaskPlan) dataTaskWorkflowStateView {
	return dataTaskWorkflowStateWithDeferredQueue(records, current, dataworkflow.NewDeferredQueue(deferred))
}

func dataTaskWorkflowStateWithDeferredQueue(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, deferredQueue dataworkflow.DeferredQueueState) dataTaskWorkflowStateView {
	contract := dataTaskWorkflowCoverageContract(records, current)
	currentBatchContract, currentBatchLayer := dataTaskWorkflowCurrentBatchContract(records, current, contract)
	outputContract := dataTaskWorkflowOutputContract(records, current)
	covered := dataTaskWorkflowCoveredMaterialPaths(records, dataquery.TaskPlan{}, 0)
	var decisionRecords []dataquery.RowDecision
	var ruleCoverageRecords []dataquery.RuleCoverageRecord
	var contributionRecords []dataquery.ContributionRecord
	var entityResolutionRecords []dataquery.EntityResolutionRecord
	hasReconcile := false
	hasAnswer := false
	for _, rec := range records {
		if rec.Result == nil {
			continue
		}
		ruleCoverageRecords = append(ruleCoverageRecords, rec.Result.RuleCoverage...)
		decisionRecords = append(decisionRecords, rec.Result.Rows...)
		entityResolutionRecords = append(entityResolutionRecords, rec.Result.EntityResolutions...)
		contributionRecords = append(contributionRecords, rec.Result.Contributions...)
		if rec.Result.Reconcile != nil {
			hasReconcile = true
		}
		if dataworkflow.ResultIsFinalAnswerCandidate(rec.Plan, *rec.Result, contract, outputContract) {
			hasAnswer = true
		}
	}
	customTransformFailures, _, _ := dataTaskCustomTransformFailureClassStats(records)
	artifactAvailability, artifactAvailabilityCount, artifactAvailabilityTruncated := dataTaskWorkflowArtifactAvailability(records, 48)
	dedupedEntityResolutions := dataquery.DedupeEntityResolutionRecords(entityResolutionRecords)
	var latestEvaluation *dataquery.Evaluation
	if eval, ok := latestDataTaskEvaluation(records); ok {
		copied := eval
		latestEvaluation = &copied
	}
	guardViolations := dataTaskWorkflowGuardViolations(records)
	return dataworkflow.BuildWorkflowStateView(dataworkflow.WorkflowStateViewBuildInput{
		Records:              records,
		Current:              current,
		DeferredQueue:        deferredQueue,
		WorkflowContract:     contract,
		CurrentBatchContract: currentBatchContract,
		CurrentBatchLayer:    currentBatchLayer,
		OutputContract:       outputContract,
		CoveredMaterialPaths: covered,
		HasMaterialProgress:  dataTaskWorkflowHasMaterialProgress(records),
		LedgerCounts: dataworkflow.WorkflowStateLedgerCounts{
			RuleCoverageRecords:     len(dataquery.DedupeRuleCoverageRecords(ruleCoverageRecords)),
			DecisionRecords:         len(dataquery.DedupeRowDecisionRecords(decisionRecords)),
			EntityResolutionRecords: len(dedupedEntityResolutions),
			EntityStageMaterialized: len(dedupedEntityResolutions) > 0 || dataTaskWorkflowEntityStageMaterialized(records),
			ContributionRecords:     len(dataquery.DedupeContributionRecords(contributionRecords)),
			HasReconcile:            hasReconcile,
			HasAnswer:               hasAnswer,
		},
		CustomTransformFailures: customTransformFailures,
		CustomTransformDisabled: dataTaskCustomTransformCooldown(records),
		CustomTransformDisabledFunc: func(state dataworkflow.WorkflowStateView) bool {
			return dataTaskCustomTransformCooldownForState(records, state)
		},
		ArtifactAvailability:          artifactAvailability,
		ArtifactAvailabilityCount:     artifactAvailabilityCount,
		ArtifactAvailabilityTruncated: artifactAvailabilityTruncated,
		ActionScaffoldArtifacts:       dataTaskWorkflowArtifactSchemaProjections(records),
		ActionScaffoldLimit:           10,
		DiagnosticArtifactAccess:      dataTaskWorkflowArtifactContractAccess(records),
		DiagnosticBatches:             dataTaskArtifactDiagnosticBatches(records),
		DiagnosticLimit:               4,
		GuardViolations:               guardViolations,
		NoProgressThreshold:           DefaultDataTaskMaxNodeFailures,
		LatestEvaluation:              latestEvaluation,
		ArtifactLimit:                 48,
		ProgressLimit:                 6,
		ActionEventLimit:              48,
	})
}

func dataTaskWorkflowGuardViolations(records []dataTaskWorkflowRecord) []dataworkflow.WorkflowViolation {
	var guardViolations []dataworkflow.WorkflowViolation
	for _, rec := range records {
		if rec.Admission == nil || rec.Admission.FinalGuard.Empty() {
			continue
		}
		guardViolations = append(guardViolations, rec.Admission.FinalGuard.Violations...)
	}
	return guardViolations
}

func dataTaskRecentJoinNoProgressCount(records []dataTaskWorkflowRecord) int {
	count, kinds := dataTaskRecentRelationNoProgressCount(records)
	if len(kinds) == 1 && kinds[0] == string(dataquery.DataActionJoinRecords) {
		return count
	}
	return 0
}

func dataTaskRecentRelationNoProgressCount(records []dataTaskWorkflowRecord) (int, []string) {
	return dataworkflow.RecentRelationNoProgressCount(dataTaskWorkflowProgressEvents(records))
}

func dataTaskWorkflowProgressEvents(records []dataTaskWorkflowRecord) []dataworkflow.ProgressEvent {
	return dataworkflow.ProgressEventsFromRecords(records)
}

func dataTaskPlanSingleRelationMaterializationKind(plan dataquery.TaskPlan) (string, bool) {
	return dataworkflow.SingleRelationMaterializationKind(plan.Actions)
}

func dataTaskActionKindIsRelationMaterialization(kind dataquery.DataActionKind) bool {
	return dataworkflow.IsRelationMaterialization(kind)
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
	return dataTaskWorkflowActionGraphWithDeferredQueueAndViolations(records, current, dataworkflow.NewDeferredQueue(deferred), violations, limit)
}

func dataTaskWorkflowActionGraphWithDeferredQueue(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, deferredQueue dataworkflow.DeferredQueueState, limit int) dataworkflow.ActionGraph {
	return dataTaskWorkflowActionGraphWithDeferredQueueAndViolations(records, current, deferredQueue, nil, limit)
}

func dataTaskWorkflowActionGraphWithDeferredQueueAndViolations(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, deferredQueue dataworkflow.DeferredQueueState, violations []dataworkflow.WorkflowViolation, limit int) dataworkflow.ActionGraph {
	return dataworkflow.ReduceActionGraphState(dataTaskWorkflowActionGraphReducerInput(records, current, deferredQueue, violations, limit))
}

func dataTaskWorkflowActionGraphReducerInput(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, deferredQueue dataworkflow.DeferredQueueState, violations []dataworkflow.WorkflowViolation, limit int) dataworkflow.ActionGraphInput {
	var ready []dataquery.DataAction
	if dataTaskPlanHasExecutableBatch(current) {
		ready = current.Actions
	}
	deferred := dataworkflow.DeferredQueuePlan(deferredQueue)
	var deferredActions []dataquery.DataAction
	if dataTaskPlanHasExecutableBatch(deferred) {
		deferredActions = deferred.Actions
	}
	return dataworkflow.ActionGraphInput{
		Events:        dataTaskWorkflowActionEvents(records),
		Ready:         ready,
		Deferred:      deferredActions,
		DeferredPlan:  deferred,
		DeferredQueue: deferredQueue,
		Blocked:       violations,
		EventLimit:    limit,
	}
}

func dataTaskWorkflowActionEvents(records []dataTaskWorkflowRecord) []dataworkflow.ActionEvent {
	return dataworkflow.BuildWorkflowActionEvents(records)
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

func dataTaskFieldContractViolationsFromRecord(round int, rec dataTaskWorkflowRecord, access []dataTaskArtifactAccessPrompt, allowed map[string]bool) []dataTaskFieldContractViolation {
	allowedActions := make([]string, 0, len(allowed))
	for action, ok := range allowed {
		if ok {
			allowedActions = append(allowedActions, action)
		}
	}
	sort.Strings(allowedActions)
	issues := dataworkflow.DiscoverFieldContractIssues(dataworkflow.FieldContractIssueInput{
		Records:            []dataworkflow.WorkflowRecord{rec},
		ArtifactAccess:     access,
		AllowedNextActions: allowedActions,
		Limit:              16,
	})
	for i := range issues {
		issues[i].Round = round
	}
	return issues
}

func dataTaskWorkflowZeroMatchFilterIssues(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView, limit int) []dataTaskZeroMatchFilterIssue {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if (!state.ContributionLedgerRequired && !state.ReconcileRequired) || state.ContributionRecords > 0 {
		return nil
	}
	return dataworkflow.ZeroMatchFilterIssuesFromBatches(dataTaskArtifactDiagnosticBatches(records), limit)
}

func dataTaskWorkflowUnmatchedResolutionIssues(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView, limit int) []dataTaskUnmatchedResolutionIssue {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if !state.EntityResolutionRequired || (!state.ContributionLedgerRequired && !state.ReconcileRequired) || state.ContributionRecords > 0 {
		return nil
	}
	return dataworkflow.UnmatchedResolutionIssuesFromBatches(dataTaskArtifactDiagnosticBatches(records), limit)
}

func dataTaskWorkflowZeroEligibleIssues(records []dataTaskWorkflowRecord, state dataTaskWorkflowStateView, limit int) []dataTaskZeroEligibleIssue {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	if (!state.ContributionLedgerRequired && !state.ReconcileRequired) || state.ContributionRecords > 0 {
		return nil
	}
	return dataworkflow.ZeroEligibleIssuesFromBatches(dataTaskArtifactDiagnosticBatches(records), limit)
}

func dataTaskArtifactDiagnosticBatches(records []dataTaskWorkflowRecord) []dataworkflow.ArtifactDiagnosticBatch {
	var out []dataworkflow.ArtifactDiagnosticBatch
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if rec.Result == nil || len(rec.Result.Artifacts) == 0 {
			continue
		}
		out = append(out, dataworkflow.ArtifactDiagnosticBatch{
			Round:     i + 1,
			Artifacts: rec.Result.Artifacts,
		})
	}
	return out
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

func dataTaskArtifactAccessToSchemaProjection(artifact dataTaskArtifactAccessPrompt) dataworkflow.ArtifactSchemaProjection {
	return dataworkflow.ArtifactSchemaProjection{
		ID:                strings.TrimSpace(artifact.ID),
		Kind:              strings.TrimSpace(artifact.Kind),
		NodeClass:         strings.TrimSpace(artifact.NodeClass),
		Aliases:           cleanDataTaskStrings(artifact.Aliases),
		JSONShape:         strings.TrimSpace(artifact.JSONShape),
		Fields:            cleanDataTaskStrings(artifact.Fields),
		AccessHint:        strings.TrimSpace(artifact.AccessHint),
		SourcePaths:       cleanDataTaskStrings(artifact.SourcePaths),
		SourceRecordPaths: cleanDataTaskStrings(artifact.SourceRecordPaths),
		ReferencePaths:    cleanDataTaskStrings(artifact.ReferencePaths),
		EvidencePaths:     cleanDataTaskStrings(artifact.EvidencePaths),
	}
}

func dataTaskArtifactIsDiagnosticChild(artifact dataTaskArtifactAccessPrompt) bool {
	return strings.TrimSpace(artifact.NodeClass) == dataworkflow.ArtifactNodeClassDiagnosticChild ||
		dataworkflow.IsDiagnosticChildArtifact(artifact.ID, artifact.Kind)
}

func dataTaskArtifactLooksLikeApplyResolutionLedger(artifact dataTaskArtifactAccessPrompt) bool {
	if dataTaskArtifactLooksLikeEntityResolutionLedger(artifact) {
		return true
	}
	return dataTaskArtifactLooksLikeGeneratedEntityMapping(artifact) &&
		firstDataTaskExistingField(artifact.Fields, "item_id", "source_locator", "_source_locator", "source_line", "source_index") != ""
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
	return dataworkflow.BuildArtifactAccessViews(artifacts, limit)
}

func dataTaskWorkflowArtifactContractAccess(records []dataTaskWorkflowRecord) []dataTaskArtifactAccessPrompt {
	projections := dataTaskWorkflowArtifactSchemaProjections(records)
	return dataworkflow.ArtifactAccessViewsFromProjections(projections, len(projections))
}

func dataTaskWorkflowArtifactSchemaProjections(records []dataTaskWorkflowRecord) []dataworkflow.ArtifactSchemaProjection {
	artifacts := dataTaskWorkflowArtifactsNewestFirst(records)
	if len(artifacts) == 0 {
		return nil
	}
	return dataworkflow.ProjectArtifactSchemasNewestFirst(artifacts)
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
	return dataworkflow.ArtifactsNewestFirst(records)
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
	for _, missing := range state.MissingValidationStages() {
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
	for _, missing := range state.MissingValidationStages() {
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
			dataquery.DataActionMappingCandidate,
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
	return dataworkflow.BuildCompactResultPromptView(result, answerLimit, auditLimit, decisionLimit, ruleLimit, contributionLimit)
}

func compactDataTaskResultPromptViewWithArtifactLimits(result dataquery.Result, answerLimit, auditLimit, decisionLimit, ruleLimit, contributionLimit, artifactLimit, artifactAccessLimit, materialSetLimit int) *dataTaskResultPromptView {
	return dataworkflow.BuildResultPromptView(result, dataworkflow.ResultPromptViewBudget{
		AnswerLimit:       answerLimit,
		AuditLimit:        auditLimit,
		DecisionLimit:     decisionLimit,
		RuleLimit:         ruleLimit,
		ContributionLimit: contributionLimit,
		ArtifactLimit:     artifactLimit,
		ArtifactAccess:    artifactAccessLimit,
		MaterialSetLimit:  materialSetLimit,
	})
}

func sampleDataTaskMaterialSetHandles(artifacts []dataquery.DataArtifact, limit int) []dataTaskMaterialSetHandlePrompt {
	return dataworkflow.BuildMaterialCollectionViews(artifacts, limit)
}

func sampleDataTaskArtifactAccess(artifacts []dataquery.DataArtifact, limit int) []dataTaskArtifactAccessPrompt {
	return dataworkflow.BuildArtifactAccessViews(artifacts, limit)
}

func sampleDataTaskArtifactAccessWithFieldSamples(artifacts []dataquery.DataArtifact, limit, maxSampleFields, maxSampleValues int) []dataTaskArtifactAccessPrompt {
	return dataworkflow.BuildArtifactAccessViewsWithFieldSamples(artifacts, limit, maxSampleFields, maxSampleValues)
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
	return dataworkflow.SampleArtifactsForPrompt(artifacts, limit)
}

func inferDataTaskAnswerItemCount(answer string, contract dataquery.OutputContract) int {
	return dataworkflow.InferAnswerItemCount(answer, contract)
}

func promptReconcileGroupSummary(report *dataquery.ReconcileReport, limit int) (int, []string, bool) {
	return dataworkflow.ReconcileGroupSummary(report, limit)
}

func dataTaskLedgerProjections(result dataquery.Result, sampleLimit int) []dataTaskLedgerProjection {
	return dataworkflow.BuildLedgerProjections(result, sampleLimit)
}

func sampleDataTaskRuleCoverage(records []dataquery.RuleCoverageRecord, limit int) []dataquery.RuleCoverageRecord {
	return dataworkflow.SampleRuleCoverage(records, limit)
}

func sampleDataTaskContributions(records []dataquery.ContributionRecord, limit int) []dataquery.ContributionRecord {
	return dataworkflow.SampleContributions(records, limit)
}

func sampleDataTaskEntityResolutions(records []dataquery.EntityResolutionRecord, limit int) []dataquery.EntityResolutionRecord {
	return dataworkflow.SampleEntityResolutions(records, limit)
}

func clampPromptReconcileReport(report *dataquery.ReconcileReport) *dataquery.ReconcileReport {
	return dataworkflow.ClampPromptReconcileReport(report)
}

func sampleDataTaskRowDecisions(rows []dataquery.RowDecision, limit int) []dataquery.RowDecision {
	return dataworkflow.SampleRowDecisions(rows, limit)
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

func latestDataTaskEvaluation(records []dataTaskWorkflowRecord) (dataquery.Evaluation, bool) {
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Evaluation != nil {
			return *records[i].Evaluation, true
		}
	}
	return dataquery.Evaluation{}, false
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
		if strings.TrimSpace(result.Answer) != "" && (dataworkflow.ResultAnswerPresent(result) || strings.TrimSpace(out.Answer) == "" || !dataworkflow.ResultAnswerPresent(out)) {
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
	return dataTaskWorkflowCompletionGateGuardResultWithRepo("", records, current, result).ErrorText()
}

func dataTaskWorkflowCompletionGateErrorWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) string {
	return dataTaskWorkflowCompletionGateGuardResultWithRepo(repoRoot, records, current, result).ErrorText()
}

func dataTaskWorkflowCompletionGateGuardResult(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) dataworkflow.GuardResult {
	return dataTaskWorkflowCompletionGateGuardResultWithRepo("", records, current, result)
}

func dataTaskWorkflowCompletionGateGuardResultWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) dataworkflow.GuardResult {
	var validationErr error
	if err := validateDataTaskWorkflowResult(records, current, result); err != nil {
		validationErr = err
	}
	var gap dataworkflow.ReferenceProjectionGap
	candidate, _, hasReferenceGap := dataTaskOutputReferenceProjectionGap(repoRoot, records, current, result)
	if hasReferenceGap {
		gap = dataworkflow.ReferenceProjectionGap{Candidate: candidate, Present: true}
	}
	return dataworkflow.CompletionGateGuardResult(dataworkflow.CompletionGateGuardInput{
		ValidationErr: validationErr,
		LedgerGraph:   dataTaskWorkflowCompletionLedgerGraph(records, current, result),
		OutputGraph:   dataTaskWorkflowCompletionOutputProjectionGraph(repoRoot, records, current, result),
		ReferenceGap:  gap,
	})
}

func dataTaskWorkflowCompletionLedgerGuardResult(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) dataworkflow.GuardResult {
	return dataworkflow.LedgerGraphCompletionGuardResult(dataTaskWorkflowCompletionLedgerGraph(records, current, result))
}

func dataTaskWorkflowCompletionLedgerGraph(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) dataworkflow.LedgerGraph {
	completionRecords := make([]dataTaskWorkflowRecord, 0, len(records)+1)
	completionRecords = append(completionRecords, records...)
	completionRecords = append(completionRecords, dataTaskWorkflowRecord{Plan: current, Result: &result})
	state := dataTaskWorkflowState(completionRecords, current)
	return state.LedgerGraph
}

func dataTaskResultNeedsOutputProjection(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) bool {
	return dataworkflow.ResultNeedsOutputProjection(dataworkflow.ResultProjectionNeedInput{
		Current:                current,
		Coverage:               dataTaskWorkflowCoverageContract(records, current),
		Output:                 dataTaskWorkflowOutputContract(records, current),
		Result:                 result,
		PlanHasCustomTransform: dataTaskPlanHasCustomTransform(current),
	})
}

func dataTaskOutputContractNeedsFinalProjection(contract dataquery.OutputContract) bool {
	return dataworkflow.OutputContractNeedsFinalProjection(contract)
}

func dataTaskWorkflowCompletionOutputProjectionGraph(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result) dataworkflow.OutputProjectionGraph {
	var referenceGap dataworkflow.ReferenceProjectionGap
	var answerItems int
	candidate, count, hasReferenceGap := dataTaskOutputReferenceProjectionGap(repoRoot, records, current, result)
	if hasReferenceGap {
		referenceGap = dataworkflow.ReferenceProjectionGap{Candidate: candidate, Present: true}
		answerItems = count
	}
	reconcileGroups := 0
	if result.Reconcile != nil {
		reconcileGroups = len(result.Reconcile.Groups)
	}
	return dataworkflow.BuildOutputProjectionGraph(dataworkflow.OutputProjectionGraphInput{
		Output:                    firstNonEmptyOutputContract(result.OutputContract, dataTaskWorkflowOutputContract(records, current), current.OutputContract),
		Coverage:                  dataTaskWorkflowCoverageContract(records, current),
		AnswerPresent:             dataworkflow.ResultAnswerPresent(result),
		ProjectionArtifactPresent: dataworkflow.ResultHasAssembleAnswerArtifact(result),
		ReconcilePresent:          result.Reconcile != nil,
		ReconcileGroups:           reconcileGroups,
		PlanHasCustomTransform:    dataTaskPlanHasCustomTransform(current),
		ReferenceGapPresent:       referenceGap.Present,
		ReferenceKeyCount:         referenceGap.Candidate.KeyCount,
		AnswerItemCount:           answerItems,
	})
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
	if !dataworkflow.ResultAnswerPresent(result) {
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
	var gap dataworkflow.ReferenceProjectionGap
	candidate, _, hasReferenceGap := dataTaskOutputReferenceProjectionGap(repoRoot, records, current, result)
	if hasReferenceGap {
		gap = dataworkflow.ReferenceProjectionGap{Candidate: candidate, Present: true}
	}
	return dataworkflow.BuildRequiredOutputProjectionPlan(dataworkflow.OutputProjectionPlanInput{
		Current:                current,
		Coverage:               dataTaskWorkflowCoverageContract(records, current),
		Output:                 dataTaskWorkflowOutputContract(records, current),
		Result:                 result,
		OutputGraph:            dataTaskWorkflowCompletionOutputProjectionGraph(repoRoot, records, current, result),
		UseOutputGraph:         true,
		ReferenceGap:           gap,
		PlanHasCustomTransform: dataTaskPlanHasCustomTransform(current),
	})
}

func dataTaskRequiredLedgerCompletionPlan(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, errText string) (dataquery.TaskPlan, bool) {
	return dataTaskRequiredLedgerCompletionPlanWithRepo("", records, current, result, errText)
}

func dataTaskRequiredLedgerCompletionPlanWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, errText string) (dataquery.TaskPlan, bool) {
	transition := dataTaskCompletionRepairTransitionWithRepo(repoRoot, records, current, result, errText)
	if transition.HasPlan() {
		return transition.Plan, true
	}
	return dataquery.TaskPlan{}, false
}

func dataTaskCompletionRepairTransitionWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, errText string) dataworkflow.CompletionRepairTransition {
	var gap dataworkflow.ReferenceProjectionGap
	candidate, _, hasReferenceGap := dataTaskOutputReferenceProjectionGap(repoRoot, records, current, result)
	if hasReferenceGap {
		gap = dataworkflow.ReferenceProjectionGap{Candidate: candidate, Present: true}
	}
	return dataworkflow.BuildCompletionRepairTransition(dataworkflow.CompletionRepairTransitionInput{
		Current:                current,
		Coverage:               dataTaskWorkflowCoverageContract(records, current),
		Output:                 dataTaskWorkflowOutputContract(records, current),
		Result:                 result,
		LedgerGraph:            dataTaskWorkflowCompletionLedgerGraph(records, current, result),
		UseLedgerGraph:         true,
		OutputGraph:            dataTaskWorkflowCompletionOutputProjectionGraph(repoRoot, records, current, result),
		UseOutputGraph:         true,
		ErrorText:              errText,
		ReferenceGap:           gap,
		PlanHasCustomTransform: dataTaskPlanHasCustomTransform(current),
	})
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
	contract := dataTaskWorkflowCoverageContract(records, current)
	output := dataTaskWorkflowOutputContract(records, current)
	access := dataTaskWorkflowArtifactContractAccess(records)
	if len(access) == 0 {
		access = dataTaskWorkflowArtifactAccess(records, 20)
	}
	var latest *dataquery.Result
	if result, ok := latestDataTaskResult(records); ok {
		copyResult := result
		latest = &copyResult
	}
	var gap dataworkflow.ReferenceProjectionGap
	if latest != nil {
		candidate, _, hasReferenceGap := dataTaskOutputReferenceProjectionGap(repoRoot, records, current, *latest)
		if hasReferenceGap {
			gap = dataworkflow.ReferenceProjectionGap{Candidate: candidate, Present: true}
		}
	}
	return dataworkflow.BuildWorkflowNextStageFallbackPlan(dataworkflow.WorkflowNextStageFallbackPlanInput{
		Current:                current,
		Coverage:               contract,
		Output:                 output,
		Facts:                  state.Facts(),
		AllowedNextActions:     state.AllowedNextActions,
		Artifacts:              dataTaskArtifactAccessSchemaProjection(access),
		ExtraScaffolds:         state.ActionScaffold,
		Result:                 latest,
		ReferenceGap:           gap,
		PlanHasCustomTransform: dataTaskPlanHasCustomTransform(current),
		ReasonPrefix:           reasonPrefix,
		SeenActionKeys:         dataTaskWorkflowSeenActionKeys(records),
		ProgressEvents:         dataTaskWorkflowProgressEvents(records),
		NoProgressStop:         DefaultDataTaskMaxNodeFailures,
		ExtractLimit:           dataTaskExactExtractRecordLimit,
	})
}

func dataTaskTerminalWorkflowGuardResult(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataworkflow.GuardResult {
	state := dataTaskWorkflowState(records, current)
	return dataworkflow.TerminalWorkflowGuardResult(current.Status, state.Facts(), state.AllowedNextActions)
}

func dataTaskTerminalWorkflowGuardError(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) string {
	return dataTaskTerminalWorkflowGuardResult(records, current).ErrorText()
}

func dataTaskPlanStatusLooksTerminal(status string) bool {
	return dataworkflow.PlanStatusLooksTerminal(status)
}

func firstNonEmptyOutputContract(values ...dataquery.OutputContract) dataquery.OutputContract {
	return dataworkflow.BestOutputContract(values...)
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
	return dataworkflow.PathLooksLikeStructuredMaterial(p)
}

func dataTaskTerminalPlanCompletionGateError(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) string {
	return dataTaskTerminalPlanCompletionGateErrorWithRepo("", records, current)
}

func dataTaskTerminalPlanCompletionGateErrorWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan) string {
	return dataTaskTerminalPlanCompletionGateGuardResultWithRepo(repoRoot, records, current).ErrorText()
}

func dataTaskTerminalPlanCompletionGateGuardResult(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataworkflow.GuardResult {
	return dataTaskTerminalPlanCompletionGateGuardResultWithRepo("", records, current)
}

func dataTaskTerminalPlanCompletionGateGuardResultWithRepo(repoRoot string, records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataworkflow.GuardResult {
	status := strings.ToLower(strings.TrimSpace(current.Status))
	if status != "complete" && status != "completed" {
		return dataworkflow.GuardResult{}
	}
	result, ok := latestDataTaskResult(records)
	if !ok {
		return dataworkflow.GuardResult{}
	}
	return dataTaskWorkflowCompletionGateGuardResultWithRepo(repoRoot, records, current, result)
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

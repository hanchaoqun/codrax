package repl

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

const (
	DefaultDataTaskMaxRepairRounds                 = 6
	DefaultDataTaskMaxDataRounds                   = 12
	DefaultDataTaskMaxNodeFailures                 = 2
	DefaultDataTaskMaxCustomTransformClassFailures = 3
	dataTaskMaxRepairRoundsCeiling                 = 12
	dataTaskMaxDataRoundsCeiling                   = 24

	dataTaskOneShotScriptLineSoftLimit   = 260
	dataTaskOneShotScriptLineHardLimit   = 420
	dataTaskComplexCustomScriptLineLimit = 180
	dataTaskOneShotRequiredMaterialLimit = 8
	dataTaskOneShotValidationLedgerLimit = 3
	dataTaskBroadMaterialDiscoveryLimit  = 12
	dataTaskMaxActionsPerBatch           = 4
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
	status := strings.ToLower(strings.TrimSpace(plan.Status))
	if status != "" && status != "ready" {
		return ""
	}
	if len(plan.Actions) > 0 {
		return dataTaskActionStagingGuardError(plan)
	}
	if errText := dataTaskTextConstraintCoverageGuardError(plan); errText != "" {
		return errText
	}
	lines := dataTaskScriptLineCount(plan.Script)
	if lines > 0 && !dataTaskScriptHasResultEmitter(plan.Script) {
		return fmt.Sprintf("data planning incomplete: script has no result emitter (script_lines=%d). A bounded data script must call emit(...), emit_result(...), or assign result before it can complete; otherwise split the workflow into typed actions that produce reusable artifacts.",
			lines)
	}
	requiredMaterials := len(plan.CoverageContract.RequiredMaterials)
	validationLedgers := dataTaskValidationLedgerCount(plan.CoverageContract)
	inputs := len(plan.InputPaths)
	complexBatch := requiredMaterials >= 4 || validationLedgers >= 2 || inputs >= 4
	if lines > 0 && complexBatch {
		return fmt.Sprintf("data planning incomplete: complex data task should not start as one top-level script (script_lines=%d input_paths=%d required_materials=%d validation_ledgers=%d). Emit an atomic actions[] batch such as inspect_material, extract_records, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, or a bounded custom_transform, and set continue_after=true when more graph work remains.",
			lines, inputs, requiredMaterials, validationLedgers)
	}
	oversized := lines >= dataTaskOneShotScriptLineHardLimit ||
		(lines >= dataTaskOneShotScriptLineSoftLimit && (requiredMaterials >= dataTaskOneShotRequiredMaterialLimit || validationLedgers >= dataTaskOneShotValidationLedgerLimit)) ||
		(lines >= 180 && complexBatch) ||
		(requiredMaterials >= dataTaskOneShotRequiredMaterialLimit+4 && validationLedgers >= dataTaskOneShotValidationLedgerLimit)
	if !oversized {
		if errText := dataTaskTerminalRequiredMaterialSchedulingError(nil, plan); errText != "" {
			return errText
		}
		return ""
	}
	return fmt.Sprintf("data planning incomplete: plan is too large for one bounded data batch (script_lines=%d input_paths=%d required_materials=%d validation_ledgers=%d continue_after=false). Emit a smaller atomic actions[] batch such as material_inventory, inspect_material, extract_records, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, or a bounded custom_transform; set continue_after=true when further work remains, and let the workflow feed real results into later batches.",
		lines, inputs, requiredMaterials, validationLedgers)
}

func dataTaskWorkflowStagingGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	status := strings.ToLower(strings.TrimSpace(plan.Status))
	if status != "" && status != "ready" {
		return ""
	}
	if len(plan.Actions) > 0 {
		return dataTaskWorkflowActionStagingGuardError(records, plan)
	}
	if errText := dataTaskTextConstraintCoverageGuardError(plan); errText != "" {
		return errText
	}
	if errText := dataTaskTerminalRequiredMaterialSchedulingError(records, plan); errText != "" {
		return errText
	}
	return dataTaskPlanStagingGuardError(plan)
}

func normalizeDataTaskPlanShape(plan dataquery.TaskPlan) (dataquery.TaskPlan, []string) {
	var reasons []string
	if normalizeDataTaskPlanPathLists(&plan) {
		reasons = append(reasons, "normalized comma-separated path lists in data plan fields")
	}
	if normalizeDataTaskPlanContractFromActions(&plan) {
		reasons = append(reasons, "enabled rule_coverage_required because the plan contains derive_rules actions")
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
	customIndex := -1
	nonEmptyCustomScript := false
	for i, action := range plan.Actions {
		if !strings.EqualFold(strings.TrimSpace(string(action.Kind)), string(dataquery.DataActionCustomTransform)) {
			continue
		}
		if strings.TrimSpace(action.Script) != "" {
			nonEmptyCustomScript = true
			continue
		}
		if customIndex >= 0 {
			customIndex = -2
			break
		}
		customIndex = i
	}
	if customIndex >= 0 && !nonEmptyCustomScript {
		plan.Actions[customIndex].Script = plan.Script
		plan.Script = ""
		if len(plan.InputPaths) > 0 {
			plan.Actions[customIndex].InputPaths = mergeDataTaskInputPaths(plan.Actions[customIndex].InputPaths, plan.InputPaths)
		}
		reasons = append(reasons, "moved top-level script into the single custom_transform action")
		if normalized, ok := normalizeDataTaskOversizedActionBatch(plan); ok {
			plan = normalized
			reasons = append(reasons, "trimmed oversized actions[] plan to the next executable batch")
		}
		return plan, reasons
	}
	if dataTaskPlanCanAppendTopLevelScriptAsCustomAction(plan) {
		actionID := fmt.Sprintf("custom_transform_%d", len(plan.Actions)+1)
		plan.Actions = append(plan.Actions, dataquery.DataAction{
			ID:         actionID,
			Kind:       dataquery.DataActionCustomTransform,
			Purpose:    firstNonEmptyString(strings.TrimSpace(plan.WhyThisBatch), strings.TrimSpace(plan.Goal), "bounded deterministic data transform"),
			InputPaths: append([]string(nil), plan.InputPaths...),
			Script:     plan.Script,
		})
		plan.Script = ""
		reasons = append(reasons, "appended bounded top-level script as a final custom_transform action")
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
	if plan == nil || plan.CoverageContract.RuleCoverageRequired {
		return false
	}
	for _, action := range plan.Actions {
		if normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionDeriveRules {
			plan.CoverageContract.RuleCoverageRequired = true
			return true
		}
	}
	return false
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

func dataTaskPlanCanAppendTopLevelScriptAsCustomAction(plan dataquery.TaskPlan) bool {
	lines := dataTaskScriptLineCount(plan.Script)
	if lines == 0 || lines >= dataTaskOneShotScriptLineSoftLimit {
		return false
	}
	if !dataTaskScriptHasResultEmitter(plan.Script) {
		return false
	}
	if dataTaskPlanHasTypedActionContext(plan.Actions, len(plan.Actions)) {
		return true
	}
	return dataTaskPlanNonCustomActionCount(plan.Actions, len(plan.Actions)) >= 1 && dataTaskPlanHasCustomScript(plan.Actions, len(plan.Actions))
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
	out := dataquery.TaskPlan{
		Status:          "ready",
		InputPaths:      paths,
		OutputContract:  dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true},
		Goal:            strings.TrimSpace(plan.Goal),
		SuccessCriteria: append([]string(nil), plan.SuccessCriteria...),
		ContinueAfter:   true,
		WhyThisBatch:    "discover objective material inventory before choosing the next bounded data action batch",
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
		out.CoverageContract.ValidationRules = []string{"previous broad plan was converted into material discovery: " + oneLineClamp(errText, 240)}
	}
	return out, true
}

func dataTaskCoverageExpansionFallback(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan, errText string) (dataquery.TaskPlan, bool) {
	state := dataTaskWorkflowState(records, plan)
	if state.MaterialCoverageSufficient && dataTaskPlanIsCoverageOnly(plan) {
		return dataquery.TaskPlan{}, false
	}
	missing := dataTaskCoverageExpansionMissingPaths(records, plan)
	if len(missing) == 0 {
		return dataquery.TaskPlan{}, false
	}
	var ruleInputs, structuredInputs, inspectInputs []string
	for _, p := range missing {
		switch {
		case dataTaskPathLooksLikeTextConstraintMaterial(p) && plan.CoverageContract.RuleCoverageRequired:
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
			Purpose:   "user explicitly referenced this candidate material; keep coverage verifiable for the data goal",
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
	if !plan.ContinueAfter {
		required := cleanDataTaskStrings(plan.CoverageContract.RequiredRunnerInputPaths())
		if len(required) > 0 {
			scheduled := dataTaskScheduledMaterialConsumption(records, plan)
			for _, p := range required {
				if !scheduled[p] {
					add([]string{p})
				}
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
		if lines == 0 {
			continue
		}
		if normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionCustomTransform && !dataTaskScriptHasResultEmitter(action.Script) {
			return fmt.Sprintf("data planning incomplete: action %d (%s) script has no result emitter (script_lines=%d). A custom_transform must call emit(...), emit_result(...), or assign result.",
				i+1, strings.TrimSpace(string(action.Kind)), lines)
		}
		if normalizeDataActionKindForWorkflow(action.Kind) == dataquery.DataActionCustomTransform &&
			lines >= dataTaskComplexCustomScriptLineLimit &&
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
		if lines == 0 {
			continue
		}
		kind := normalizeDataActionKindForWorkflow(action.Kind)
		if kind == dataquery.DataActionCustomTransform && !dataTaskScriptHasResultEmitter(action.Script) {
			return fmt.Sprintf("data planning incomplete: action %d (%s) script has no result emitter (script_lines=%d). A custom_transform must call emit(...), emit_result(...), or assign result.",
				i+1, strings.TrimSpace(string(action.Kind)), lines)
		}
		if kind == dataquery.DataActionCustomTransform &&
			dataTaskActionHasBroadPrerequisiteSurface(plan, action) {
			if missing := dataTaskMissingCustomTransformPrerequisites(records, plan, action, i); len(missing) > 0 {
				return fmt.Sprintf("data planning incomplete: broad custom_transform action %d (%s) reads or depends on %d material(s) that were not covered by prior typed actions/results: %s. First add smaller atomic actions such as inspect_material, derive_rules, derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, or extract_records for the missing inputs, then use custom_transform only as a bounded transform over known materials.",
					i+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))), len(missing), strings.Join(missing, ", "))
			}
			if lines >= dataTaskOneShotScriptLineSoftLimit {
				return fmt.Sprintf("data planning incomplete: action %d (%s) is too large for one atomic data action (script_lines=%d). Split the workflow into smaller typed actions such as material_inventory, inspect_material, and bounded custom_transform nodes.",
					i+1, strings.TrimSpace(string(action.Kind)), lines)
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
		if dataTaskWorkflowActionKindAllowed(kind, state) {
			continue
		}
		return fmt.Sprintf("data planning incomplete: workflow next_stage=%s allows only next actions [%s], but action %d (%s) uses %s. Emit the next bounded DAG batch using workflow_state_json.allowed_next_actions, set continue_after=true when later stages remain, and let the evaluator advance the workflow after this batch.",
			state.NextStage, strings.Join(state.AllowedNextActions, ", "), i+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))), kind)
	}
	return ""
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

func dataTaskWorkflowActionKindAllowed(kind dataquery.DataActionKind, state dataTaskWorkflowStateView) bool {
	for _, action := range state.AllowedNextActions {
		if kind == dataquery.DataActionKind(action) {
			return true
		}
	}
	return false
}

func dataTaskWorkflowStageProgressGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	state := dataTaskWorkflowState(records, plan)
	if !state.MaterialCoverageSufficient {
		return ""
	}
	switch state.NextStage {
	case "normalize_or_enrich_entities", "compute_contributions", "reconcile_artifacts", "emit_output_contract_answer":
	default:
		return ""
	}
	missingStages := dataTaskWorkflowMissingValidationStages(state)
	if len(missingStages) <= 1 || !dataTaskPlanHasScriptedCustomTransform(plan) {
		return ""
	}
	if dataTaskPlanIsSingleIntermediateCustomTransform(plan) {
		return ""
	}
	return fmt.Sprintf("data planning incomplete: workflow next_stage=%s still has %d unfinished validation stage(s): %s. Do not cross multiple unfinished data DAG stages with one custom_transform. Emit the next atomic stage first (for example derive_fields/normalize_entities/enrich_records/join_records before compute_contributions, compute_contributions before reconcile_artifacts, reconcile_artifacts before the final answer), set continue_after=true, and let the workflow evaluate the reusable artifact before the next batch.",
		state.NextStage, len(missingStages), strings.Join(missingStages, ", "))
}

func dataTaskWorkflowMissingValidationStages(state dataTaskWorkflowStateView) []string {
	var out []string
	if state.RuleCoverageRequired && state.RuleCoverageRecords == 0 {
		out = append(out, "rule_coverage")
	}
	if state.EntityResolutionRequired && state.EntityResolutionRecords == 0 {
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
	kind := normalizeDataActionKindForWorkflow(action.Kind)
	switch kind {
	case dataquery.DataActionInspectMaterial, dataquery.DataActionExtractRecords, dataquery.DataActionDeriveFields:
		inputs := cleanDataTaskStrings(action.InputPaths)
		if len(inputs) == 0 {
			return fmt.Sprintf("data planning incomplete: action %d (%s) requires input_paths. Choose concrete candidate material paths or prior artifact aliases; do not emit an empty %s action.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))), strings.TrimSpace(string(kind)))
		}
		if kind == dataquery.DataActionDeriveFields && len(inputs) > 1 {
			return fmt.Sprintf("data planning incomplete: action %d (%s) is derive_fields with %d input_paths. derive_fields is a single-record-set field derivation action. Do not use derive_fields for lookup/reference-table mapping. Split different schemas into separate derive_fields actions, or first use normalize_entities, enrich_records, or join_records to create one joined/generated artifact, then derive fields from that one artifact.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))), len(inputs))
		}
		if kind == dataquery.DataActionDeriveFields && !dataTaskDeriveFieldsActionHasSpec(action) {
			return fmt.Sprintf("data planning incomplete: action %d (%s) is derive_fields but has no field specification. Add params.field_specs_json (array of source_field/target_field/operation specs) or a single source_field+target_field+operation spec; if this batch only needs to materialize rows without deriving fields, use extract_records instead.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))))
		}
	case dataquery.DataActionComputeContribs:
		if len(cleanDataTaskStrings(action.InputPaths)) == 0 {
			return fmt.Sprintf("data planning incomplete: action %d (%s) requires input_paths containing existing records or generated artifact aliases before contribution computation.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))))
		}
	case dataquery.DataActionReconcile:
		if !dataTaskWorkflowHasContributionProducer(records, plan, actionIndex) {
			return fmt.Sprintf("data planning incomplete: action %d (%s) requires contribution records, but no prior compute_contributions result or earlier compute_contributions action is available. Add a bounded compute_contributions batch first, let it execute, then reconcile in a later batch or after a previous contribution-producing action.",
				actionIndex+1, firstNonEmptyString(strings.TrimSpace(action.ID), strings.TrimSpace(string(action.Kind))))
		}
	}
	return ""
}

func dataTaskDeriveFieldsActionHasSpec(action dataquery.DataAction) bool {
	if len(action.Params) == 0 {
		return false
	}
	for _, key := range []string{
		"field_specs_json", "derive_specs_json", "transforms_json",
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

func dataTaskCoverageLoopGuardError(records []dataTaskWorkflowRecord, plan dataquery.TaskPlan) string {
	state := dataTaskWorkflowState(records, plan)
	if !state.MaterialCoverageSufficient || !dataTaskPlanIsCoverageOnly(plan) {
		return ""
	}
	if dataTaskPlanActionsAllAllowedForWorkflowStage(plan, state) {
		return ""
	}
	return fmt.Sprintf("data planning incomplete: material coverage is already sufficient for required runner materials (%d covered, missing=%d). Do not emit another coverage-only batch (material_inventory/inspect_material/extract_records/derive_rules) unless a specific new missing material is listed. Continue toward the user's data goal with compute-stage atomic actions such as derive_fields, normalize_entities, enrich_records, join_records, compute_contributions, reconcile_artifacts, or one narrow custom_transform over generated artifacts. workflow_next_stage=%s",
		state.RequiredMaterialCount-state.MissingRequiredMaterialCount, state.MissingRequiredMaterialCount, state.NextStage)
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
	return fmt.Sprintf("data planning incomplete: workflow already has %d custom_transform failure(s).%s Do not bypass this by changing action_id; avoid another broad free-form script. Continue with typed atomic actions that produce reusable artifacts, or use one narrow custom_transform over one known generated artifact with its json_shape/access catalog.",
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
	if strings.EqualFold(strings.TrimSpace(string(kind)), string(dataquery.DataActionCustomTransform)) || strings.TrimSpace(string(kind)) == "" {
		return dataquery.DataActionCustomTransform
	}
	return kind
}

func dataTaskActionLooksLikeWholeWorkflow(plan dataquery.TaskPlan, action dataquery.DataAction, actionIndex int) bool {
	if dataTaskPlanHasTypedActionContext(plan.Actions, actionIndex) && dataTaskScriptLineCount(action.Script) < dataTaskOneShotScriptLineSoftLimit {
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
		if p == "" || covered[p] {
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
			covered[value] = true
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
	MaterialCoverageSufficient   bool                     `json:"material_coverage_sufficient"`
	RequiredMaterialCount        int                      `json:"required_material_count"`
	MissingRequiredMaterialCount int                      `json:"missing_required_material_count"`
	MissingRequiredMaterials     []string                 `json:"missing_required_materials,omitempty"`
	OutputContract               dataquery.OutputContract `json:"output_contract,omitempty"`
	RuleCoverageRequired         bool                     `json:"rule_coverage_required,omitempty"`
	RuleCoverageRecords          int                      `json:"rule_coverage_records,omitempty"`
	DecisionRecordsRequired      bool                     `json:"decision_records_required,omitempty"`
	DecisionRecords              int                      `json:"decision_records,omitempty"`
	EntityResolutionRequired     bool                     `json:"entity_resolution_required,omitempty"`
	EntityResolutionRecords      int                      `json:"entity_resolution_records,omitempty"`
	ContributionLedgerRequired   bool                     `json:"contribution_ledger_required,omitempty"`
	ContributionRecords          int                      `json:"contribution_records,omitempty"`
	ReconcileRequired            bool                     `json:"reconcile_required,omitempty"`
	HasReconcile                 bool                     `json:"has_reconcile,omitempty"`
	HasAnswer                    bool                     `json:"has_answer,omitempty"`
	NextStage                    string                   `json:"next_stage,omitempty"`
	AllowedNextActions           []string                 `json:"allowed_next_actions,omitempty"`
	AllowedNextActionContracts   []dataTaskActionContract `json:"allowed_next_action_contracts,omitempty"`
}

type dataTaskActionContract struct {
	Kind          string `json:"kind"`
	InputBoundary string `json:"input_boundary,omitempty"`
	UseWhen       string `json:"use_when,omitempty"`
	Output        string `json:"output,omitempty"`
}

type dataTaskResultPromptView struct {
	Answer                   string                             `json:"answer,omitempty"`
	AnswerItemCount          int                                `json:"answer_item_count,omitempty"`
	OutputContract           dataquery.OutputContract           `json:"output_contract,omitempty"`
	AuditSummary             string                             `json:"audit_summary,omitempty"`
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

type dataTaskArtifactAccessPrompt struct {
	ID          string   `json:"id,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	JSONShape   string   `json:"json_shape,omitempty"`
	AccessHint  string   `json:"access_hint,omitempty"`
	SourcePaths []string `json:"source_paths,omitempty"`
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
	if len(records) == 0 {
		return "(none)\n"
	}
	start := len(records) - 6
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
			Actions:             compactDataTaskActionsForPrompt(rec.Plan.Actions, 8, 1200),
			InputPaths:          append([]string(nil), rec.Plan.InputPaths...),
			OutputContract:      rec.Plan.OutputContract.Normalize(),
			SuccessCriteria:     append([]string(nil), rec.Plan.SuccessCriteria...),
			MissingObservations: append([]string(nil), rec.Plan.MissingObservations...),
			NextBatch:           strings.TrimSpace(rec.Plan.NextBatch),
			WhyThisBatch:        strings.TrimSpace(rec.Plan.WhyThisBatch),
			ContinueAfter:       rec.Plan.ContinueAfter,
			ScriptPreview:       clampDataTaskWorkflowText(rec.Plan.Script, 1800),
			Error:               clampDataTaskWorkflowText(rec.Err, 1600),
		}
		if rec.Result != nil {
			view.Result = compactDataTaskResultPromptView(*rec.Result, 2400, 1000, 6, 4, 6)
		}
		if rec.Evaluation != nil {
			eval := *rec.Evaluation
			eval.Reason = clampDataTaskWorkflowText(eval.Reason, 1000)
			view.Evaluation = &eval
		}
		views = append(views, view)
	}
	raw, err := json.MarshalIndent(views, "", "  ")
	if err != nil {
		return fmt.Sprintf("render data workflow records failed: %v\n", err)
	}
	return string(raw)
}

func dataTaskWorkflowState(records []dataTaskWorkflowRecord, current dataquery.TaskPlan) dataTaskWorkflowStateView {
	contract := dataTaskWorkflowCoverageContract(records, current)
	outputContract := dataTaskWorkflowOutputContract(records, current)
	required := cleanDataTaskStrings(contract.RequiredRunnerInputPaths())
	covered := dataTaskWorkflowCoveredMaterialPaths(records, dataquery.TaskPlan{}, 0)
	var missing []string
	for _, p := range required {
		if p == "" || covered[p] {
			continue
		}
		missing = append(missing, p)
	}
	sort.Strings(missing)
	state := dataTaskWorkflowStateView{
		RequiredMaterialCount:        len(required),
		MissingRequiredMaterialCount: len(missing),
		MissingRequiredMaterials:     missing,
		OutputContract:               outputContract,
		MaterialCoverageSufficient:   len(required) > 0 && len(missing) == 0,
		RuleCoverageRequired:         contract.RuleCoverageRequired,
		DecisionRecordsRequired:      contract.DecisionRecordsRequired,
		EntityResolutionRequired:     contract.EntityResolutionRequired,
		ContributionLedgerRequired:   contract.ContributionLedgerRequired,
		ReconcileRequired:            contract.ReconcileRequired,
	}
	for _, rec := range records {
		if rec.Result == nil {
			continue
		}
		state.RuleCoverageRecords += len(rec.Result.RuleCoverage)
		state.DecisionRecords += len(rec.Result.Rows)
		state.EntityResolutionRecords += len(rec.Result.EntityResolutions)
		state.ContributionRecords += len(rec.Result.Contributions)
		if rec.Result.Reconcile != nil {
			state.HasReconcile = true
		}
		if dataTaskResultIsFinalAnswerCandidate(rec.Plan, *rec.Result, contract, outputContract) {
			state.HasAnswer = true
		}
	}
	state.NextStage = dataTaskWorkflowNextStage(state)
	state.AllowedNextActions = dataTaskWorkflowAllowedNextActions(state)
	state.AllowedNextActionContracts = dataTaskWorkflowAllowedNextActionContracts(state)
	return state
}

func dataTaskResultIsFinalAnswerCandidate(plan dataquery.TaskPlan, result dataquery.Result, contract dataquery.CoverageContract, expected dataquery.OutputContract) bool {
	if strings.TrimSpace(result.Answer) == "" {
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
	if state.EntityResolutionRequired && state.EntityResolutionRecords == 0 {
		return "normalize_or_enrich_entities"
	}
	if state.ContributionLedgerRequired && state.ContributionRecords == 0 {
		return "compute_contributions"
	}
	if state.ReconcileRequired && !state.HasReconcile {
		return "reconcile_artifacts"
	}
	if !state.HasAnswer {
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
			contract(dataquery.DataActionDeriveFields, "exactly one existing record artifact/path", "derive same-record fields such as parsed numbers, extracted year, trimmed/lowercase values, constants, regex fields, or maps that do not require a second table", "one enriched record artifact"),
			contract(dataquery.DataActionNormalizeEntities, "one or more inputs are OK when producing source-to-canonical mappings", "produce canonical mappings for identifiers, names, categories, accounts, people, devices, labels, or other entities", "entity_resolution records and mapping artifact"),
			contract(dataquery.DataActionEnrichRecords, "base record artifact plus mapping/reference artifact(s)", "apply mapping/reference records to base rows and materialize added canonical or derived fields", "one enriched record artifact"),
			contract(dataquery.DataActionJoinRecords, "two structured record artifacts/paths", "join two record sets using explicit key fields before later derivation or contribution calculation", "one joined record artifact"),
			contract(dataquery.DataActionCustomTransform, "small bounded fallback over known artifacts only", "use only when typed actions cannot express this single normalization/enrichment step", "one reusable artifact or ledger slice"),
		}
	case "compute_contributions":
		return []dataTaskActionContract{
			contract(dataquery.DataActionDeriveFields, "exactly one existing record artifact/path", "derive final numeric/group/filter fields before contribution calculation", "one enriched record artifact"),
			contract(dataquery.DataActionNormalizeEntities, "one or more inputs are OK when producing mappings", "repair missing canonical mappings needed by contribution calculation", "entity_resolution records and mapping artifact"),
			contract(dataquery.DataActionEnrichRecords, "base record artifact plus mapping/reference artifact(s)", "apply mappings or reference fields needed by contribution calculation", "one enriched record artifact"),
			contract(dataquery.DataActionJoinRecords, "two structured record artifacts/paths", "join base records with target/query/reference rows before contribution calculation", "one joined record artifact"),
			contract(dataquery.DataActionComputeContribs, "one structured record artifact/path with existing value/group/filter fields", "compute generic sums, counts, grouped totals, or contribution ledgers", "contribution records and contribution artifact"),
			contract(dataquery.DataActionCustomTransform, "small bounded fallback over generated/covered artifacts; do not cross compute, reconcile, and final answer in one script", "use only when generic contribution params cannot express this single compute step", "decision/contribution records or one reusable compute artifact"),
		}
	case "reconcile_artifacts":
		return []dataTaskActionContract{
			contract(dataquery.DataActionReconcile, "prior contribution records are required", "recompute group totals from contribution records and verify expected/actual values", "reconcile report"),
		}
	case "emit_output_contract_answer":
		return []dataTaskActionContract{
			contract(dataquery.DataActionReconcile, "prior contribution records are required", "refresh deterministic reconcile before answer assembly when needed", "reconcile report"),
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
		case dataquery.DataActionMaterialInventory, dataquery.DataActionInspectMaterial, dataquery.DataActionExtractRecords, dataquery.DataActionDeriveRules:
			continue
		default:
			return false
		}
	}
	return true
}

func compactDataTaskResultPromptView(result dataquery.Result, answerLimit, auditLimit, decisionLimit, ruleLimit, contributionLimit int) *dataTaskResultPromptView {
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
		DecisionRecords:          len(result.Rows),
		DecisionSamples:          sampleDataTaskRowDecisions(result.Rows, decisionLimit),
		RuleCoverageRecords:      len(result.RuleCoverage),
		RuleCoverageSamples:      sampleDataTaskRuleCoverage(result.RuleCoverage, ruleLimit),
		ContributionRecords:      len(result.Contributions),
		ContributionSamples:      sampleDataTaskContributions(result.Contributions, contributionLimit),
		EntityResolutionRecords:  len(result.EntityResolutions),
		EntityResolutionSamples:  sampleDataTaskEntityResolutions(result.EntityResolutions, 4),
		Reconcile:                clampedReconcile,
		ReconcileGroupCount:      groupCount,
		ReconcileGroupKeySample:  groupKeys,
		ReconcileGroupsTruncated: groupsTruncated,
		Metrics:                  result.Metrics,
		Artifacts:                sampleDataTaskArtifacts(result.Artifacts, 6),
		ArtifactAccess:           sampleDataTaskArtifactAccess(result.Artifacts, 10),
		MaterialSetHandles:       sampleDataTaskMaterialSetHandles(result.Artifacts, 8),
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
	if limit <= 0 || len(artifacts) == 0 {
		return nil
	}
	var out []dataTaskArtifactAccessPrompt
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
			out = append(out, dataTaskArtifactAccessPrompt{
				ID:          strings.TrimSpace(artifact.ID),
				Aliases:     clampDataTaskStringSlice(aliases, 8),
				JSONShape:   clampDataTaskWorkflowText(shape, 240),
				AccessHint:  dataTaskArtifactAccessHint(shape),
				SourcePaths: clampDataTaskStringSlice(artifact.SourcePaths, 6),
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

func dataTaskArtifactAccessHint(shape string) string {
	shape = strings.TrimSpace(strings.ToLower(shape))
	switch {
	case strings.HasPrefix(shape, "array"):
		return "read with json_records(alias) or iterate json_load(alias) as a list; do not call .get() on the top-level value"
	case strings.HasPrefix(shape, "object"):
		return "read with json_load(alias) for object fields, or json_records(alias) when treating object values/wrappers as records"
	case strings.HasPrefix(shape, "scalar"):
		return "read with json_load(alias) as a scalar value"
	default:
		return "use json_records(alias) for record-oriented access; inspect artifact json_shape before assuming dict/list shape"
	}
}

func compactDataTaskActionsForPrompt(actions []dataquery.DataAction, limit, scriptLimit int) []dataquery.DataAction {
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
		action.SuccessCriteria = clampDataTaskStringSlice(action.SuccessCriteria, 8)
		out = append(out, action)
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
		artifact.Summary = clampDataTaskWorkflowText(artifact.Summary, 300)
		artifact.SourcePaths = clampDataTaskStringSlice(artifact.SourcePaths, 8)
		artifact.Headers = clampDataTaskStringSlice(artifact.Headers, 12)
		artifact.Sample = clampDataTaskStringSlice(artifact.Sample, 4)
		if len(artifact.Children) > 4 {
			artifact.Children = append([]dataquery.DataArtifact(nil), artifact.Children[:4]...)
		}
		out = append(out, artifact)
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
	result, ok := latestDataTaskResult(records)
	if !ok {
		return dataquery.Result{}
	}
	return result
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
	for _, child := range artifact.Children {
		out = append(out, dataTaskArtifactAliasPaths(child)...)
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
		contract = mergeDataTaskCoverageContracts(contract, rec.Plan.CoverageContract)
	}
	contract = mergeDataTaskCoverageContracts(contract, current.CoverageContract)
	return dataTaskCoverageContractWithoutGeneratedScriptMaterials(contract, dataTaskWorkflowGeneratedArtifactPathSet(records))
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
	if err := validateDataTaskWorkflowResult(records, current, result); err != nil {
		return fmt.Sprintf("validate data workflow completion: %v", err)
	}
	return ""
}

func dataTaskRequiredLedgerCompletionPlan(records []dataTaskWorkflowRecord, current dataquery.TaskPlan, result dataquery.Result, errText string) (dataquery.TaskPlan, bool) {
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
		inputs := dataTaskEntityResolutionCompletionInputs(records, current, result)
		if len(inputs) == 0 {
			return dataquery.TaskPlan{}, false
		}
		base.Actions = []dataquery.DataAction{{
			ID:             "complete_entity_resolutions",
			Kind:           dataquery.DataActionNormalizeEntities,
			Purpose:        "complete missing generic source-to-canonical entity resolution ledger from already covered structured materials",
			InputPaths:     inputs,
			OutputArtifact: "entity_resolutions.json",
			Params: map[string]string{
				"reason":          "completed required entity resolution ledger from structured materials",
				"max_resolutions": "200000",
			},
		}}
		base.InputPaths = mergeDataTaskInputPaths(base.InputPaths, inputs)
		base.WhyThisBatch = "complete missing entity resolution ledger with a focused normalize_entities node"
		return base, true
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
	status := strings.ToLower(strings.TrimSpace(current.Status))
	if status != "complete" && status != "completed" {
		return ""
	}
	result, ok := latestDataTaskResult(records)
	if !ok {
		return ""
	}
	return dataTaskWorkflowCompletionGateError(records, current, result)
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

func mergeDataTaskCoverageContracts(previous, next dataquery.CoverageContract) dataquery.CoverageContract {
	out := next
	out.RequiredMaterials = mergeDataTaskCoverageMaterialsExcept(previous.RequiredMaterials, next.RequiredMaterials, true, dataTaskCoverageMaterialKeySet(next.OptionalMaterials))
	out.OptionalMaterials = mergeDataTaskCoverageMaterials(previous.OptionalMaterials, next.OptionalMaterials, false)
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

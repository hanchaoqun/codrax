package dataworkflow

import (
	"fmt"
	"path"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

const DefaultRecordMaterializationLimit = 100000

type RecordMaterializationFallbackInput struct {
	Current            dataquery.TaskPlan
	Coverage           dataquery.CoverageContract
	Output             dataquery.OutputContract
	Facts              StageFacts
	AllowedNextActions []string
	Artifacts          []ArtifactSchemaProjection
	ReasonPrefix       string
	SeenActionKeys     map[string]bool
	ExtractLimit       int
}

type CoverageExpansionPlanInput struct {
	Current            dataquery.TaskPlan
	Coverage           dataquery.CoverageContract
	MissingPaths       []string
	DeriveRulesForText bool
	MaxActions         int
	ValidationRule     string
}

type WorkflowNextStageFallbackPlanInput struct {
	Current                dataquery.TaskPlan
	Coverage               dataquery.CoverageContract
	Output                 dataquery.OutputContract
	Facts                  StageFacts
	AllowedNextActions     []string
	Artifacts              []ArtifactSchemaProjection
	ExtraScaffolds         []ActionScaffold
	Result                 *dataquery.Result
	ReferenceGap           ReferenceProjectionGap
	PlanHasCustomTransform bool
	ReasonPrefix           string
	SeenActionKeys         map[string]bool
	ProgressEvents         []ProgressEvent
	NoProgressStop         int
	ExtractLimit           int
}

type CompletionRepairTransitionInput struct {
	Current                dataquery.TaskPlan
	Coverage               dataquery.CoverageContract
	Output                 dataquery.OutputContract
	Result                 dataquery.Result
	ErrorText              string
	ReferenceGap           ReferenceProjectionGap
	PlanHasCustomTransform bool
}

type CompletionRepairTransition struct {
	Plan               dataquery.TaskPlan `json:"plan,omitempty"`
	Reason             string             `json:"reason,omitempty"`
	ErrorText          string             `json:"error_text,omitempty"`
	Deterministic      bool               `json:"deterministic,omitempty"`
	NeedsPlannerRepair bool               `json:"needs_planner_repair,omitempty"`
}

func (t CompletionRepairTransition) HasPlan() bool {
	return len(t.Plan.Actions) > 0 || strings.TrimSpace(t.Plan.Script) != "" || len(cleanStrings(t.Plan.InputPaths)) > 0
}

func BuildCompletionRepairTransition(input CompletionRepairTransitionInput) CompletionRepairTransition {
	errText := strings.TrimSpace(input.ErrorText)
	if errText == "" {
		return CompletionRepairTransition{}
	}
	if plan, ok := BuildRequiredLedgerCompletionPlan(RequiredLedgerCompletionPlanInput{
		Current:                input.Current,
		Coverage:               input.Coverage,
		Output:                 input.Output,
		Result:                 input.Result,
		ErrorText:              errText,
		ReferenceGap:           input.ReferenceGap,
		PlanHasCustomTransform: input.PlanHasCustomTransform,
	}); ok {
		return CompletionRepairTransition{
			Plan:          plan,
			Reason:        errText,
			ErrorText:     errText,
			Deterministic: true,
		}
	}
	return CompletionRepairTransition{
		Reason:             errText,
		ErrorText:          errText,
		NeedsPlannerRepair: true,
	}
}

func BuildWorkflowNextStageFallbackPlan(input WorkflowNextStageFallbackPlanInput) (dataquery.TaskPlan, string, bool) {
	if len(input.Current.Actions) == 0 && input.Current.ContinueAfter {
		return dataquery.TaskPlan{}, "", false
	}
	if len(MissingValidationStages(input.Facts)) == 0 {
		return dataquery.TaskPlan{}, "", false
	}
	reasonPrefix := strings.TrimSpace(input.ReasonPrefix)
	if reasonPrefix == "" {
		reasonPrefix = "workflow state requires the next typed data stage"
	}
	base := dataquery.TaskPlan{
		Status:           "ready",
		OutputContract:   input.Output,
		CoverageContract: input.Coverage,
		Goal:             strings.TrimSpace(input.Current.Goal),
		SuccessCriteria:  append([]string(nil), input.Current.SuccessCriteria...),
		ContinueAfter:    true,
	}
	if strings.TrimSpace(base.Goal) == "" {
		base.Goal = "continue the typed data workflow toward the user's requested output"
	}
	switch input.Facts.NextStage() {
	case StageDeriveRules:
		action := RuleCoverageCompletionAction(input.Coverage)
		if strings.TrimSpace(action.ID) == "" {
			return dataquery.TaskPlan{}, "", false
		}
		base.Actions = []dataquery.DataAction{action}
		base.InputPaths = mergeActionInputPaths(base.InputPaths, action.InputPaths)
		base.NextBatch = "continue with later typed data stages after deriving rule coverage"
		base.WhyThisBatch = reasonPrefix + " before required rule coverage was materialized"
		return base, reasonPrefix + "; converted to required derive_rules stage", true
	case StageNormalizeOrEnrichEntities, StagePrepareContributionInputs, StageComputeContributions:
		if plan, reason, ok := BuildRecordMaterializationFallbackPlan(RecordMaterializationFallbackInput{
			Current:            base,
			Coverage:           input.Coverage,
			Output:             input.Output,
			Facts:              input.Facts,
			AllowedNextActions: input.AllowedNextActions,
			Artifacts:          input.Artifacts,
			ReasonPrefix:       reasonPrefix,
			SeenActionKeys:     input.SeenActionKeys,
			ExtractLimit:       input.ExtractLimit,
		}); ok {
			return plan, reason, true
		}
		if plan, reason, ok := BuildNextStageConcreteFallbackPlan(NextStageFallbackPlanInput{
			Current:            base,
			Coverage:           input.Coverage,
			Output:             input.Output,
			Facts:              input.Facts,
			AllowedNextActions: input.AllowedNextActions,
			Artifacts:          input.Artifacts,
			ExtraScaffolds:     input.ExtraScaffolds,
			ReasonPrefix:       reasonPrefix + "; advanced " + input.Facts.NextStage(),
			SeenActionKeys:     input.SeenActionKeys,
			ProgressEvents:     input.ProgressEvents,
			NoProgressStop:     input.NoProgressStop,
		}); ok {
			return plan, reason, true
		}
		return dataquery.TaskPlan{}, "", false
	case StageReconcileArtifacts:
		if input.Result == nil || len(input.Result.Contributions) == 0 {
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
	case StageEmitOutputContractAnswer:
		if input.Result == nil {
			return dataquery.TaskPlan{}, "", false
		}
		if plan, ok := BuildRequiredOutputProjectionPlan(OutputProjectionPlanInput{
			Current:                input.Current,
			Coverage:               input.Coverage,
			Output:                 input.Output,
			Result:                 *input.Result,
			ReferenceGap:           input.ReferenceGap,
			PlanHasCustomTransform: input.PlanHasCustomTransform,
		}); ok {
			plan.ContinueAfter = false
			return plan, reasonPrefix + "; converted to required answer projection stage", true
		}
	}
	return dataquery.TaskPlan{}, "", false
}

func BuildCoverageExpansionPlan(input CoverageExpansionPlanInput) (dataquery.TaskPlan, bool) {
	missing := cleanStrings(input.MissingPaths)
	if len(missing) == 0 {
		return dataquery.TaskPlan{}, false
	}
	var ruleInputs, structuredInputs, inspectInputs []string
	for _, p := range missing {
		switch {
		case input.DeriveRulesForText && PathLooksLikeTextConstraintMaterial(p):
			ruleInputs = append(ruleInputs, p)
		case PathLooksLikeStructuredMaterial(p):
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
			InputPaths:     cleanStrings(ruleInputs),
			OutputArtifact: "coverage_rules.json",
		})
	}
	if len(structuredInputs) > 0 {
		actions = append(actions, dataquery.DataAction{
			ID:             "cover_required_records",
			Kind:           dataquery.DataActionExtractRecords,
			Purpose:        "extract record samples from required structured materials before later data transforms",
			InputPaths:     cleanStrings(structuredInputs),
			OutputArtifact: "coverage_records.json",
			Params:         map[string]string{"limit": "120"},
		})
	}
	if len(inspectInputs) > 0 {
		actions = append(actions, dataquery.DataAction{
			ID:             "cover_required_materials",
			Kind:           dataquery.DataActionInspectMaterial,
			Purpose:        "inspect required materials before later data transforms",
			InputPaths:     cleanStrings(inspectInputs),
			OutputArtifact: "coverage_inspection.json",
		})
	}
	if len(actions) == 0 {
		return dataquery.TaskPlan{}, false
	}
	maxActions := input.MaxActions
	if maxActions > 0 && len(actions) > maxActions {
		actions = actions[:maxActions]
	}
	coverage := input.Coverage
	if len(coverage.ValidationRules) == 0 {
		coverage.ValidationRules = appendValidationRule(coverage.ValidationRules, input.ValidationRule)
	}
	return dataquery.TaskPlan{
		Status:           "ready",
		InputPaths:       missing,
		OutputContract:   dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true},
		CoverageContract: coverage,
		Goal:             strings.TrimSpace(input.Current.Goal),
		KnownConstraints: append([]string(nil), input.Current.KnownConstraints...),
		SuccessCriteria:  append([]string(nil), input.Current.SuccessCriteria...),
		ContinueAfter:    true,
		WhyThisBatch:     "cover missing required or prerequisite materials with atomic data actions before later transforms",
		NextBatch:        "continue the data workflow using these material artifacts instead of re-planning the same broad transform",
		Actions:          actions,
	}, true
}

type MaterialDiscoveryPlanInput struct {
	Current        dataquery.TaskPlan
	Coverage       dataquery.CoverageContract
	Paths          []string
	ValidationRule string
}

func BuildMaterialDiscoveryPlan(input MaterialDiscoveryPlanInput) (dataquery.TaskPlan, bool) {
	paths := cleanStrings(input.Paths)
	if len(paths) == 0 {
		return dataquery.TaskPlan{}, false
	}
	coverage := input.Coverage
	coverage.RequiredMaterials = nil
	coverage.OptionalMaterials = nil
	coverage.ValidationRules = appendValidationRule(coverage.ValidationRules, input.ValidationRule)
	plan := dataquery.TaskPlan{
		Status:           "ready",
		InputPaths:       paths,
		OutputContract:   dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true},
		CoverageContract: coverage,
		Goal:             strings.TrimSpace(input.Current.Goal),
		SuccessCriteria:  append([]string(nil), input.Current.SuccessCriteria...),
		ContinueAfter:    true,
		WhyThisBatch:     "discover objective material inventory before choosing the next bounded data action batch",
		Actions: []dataquery.DataAction{{
			ID:         "material_inventory",
			Kind:       dataquery.DataActionMaterialInventory,
			Purpose:    "discover material types, paths, and objective metadata for the next data workflow batch",
			InputPaths: paths,
		}},
	}
	if strings.TrimSpace(plan.Goal) == "" {
		plan.Goal = "discover data task materials"
	}
	return plan, true
}

func BuildRecordMaterializationFallbackPlan(input RecordMaterializationFallbackInput) (dataquery.TaskPlan, string, bool) {
	if !actionAllowed(input.AllowedNextActions, dataquery.DataActionExtractRecords) {
		return dataquery.TaskPlan{}, "", false
	}
	if HasRecordActionArtifact(input.Artifacts) {
		return dataquery.TaskPlan{}, "", false
	}
	paths := RecordMaterializationPaths(input.Coverage)
	if len(paths) == 0 {
		return dataquery.TaskPlan{}, "", false
	}
	limit := input.ExtractLimit
	if limit <= 0 {
		limit = DefaultRecordMaterializationLimit
	}
	reasonPrefix := strings.TrimSpace(input.ReasonPrefix)
	if reasonPrefix == "" {
		reasonPrefix = "workflow state requested record materialization"
	}
	plan := baseContinuationPlan(input.Current, input.Coverage, input.Output)
	action := dataquery.DataAction{
		ID:             "continue_extract_records",
		Kind:           dataquery.DataActionExtractRecords,
		Purpose:        "materialize required local materials into reusable record artifacts before typed graph computation",
		InputPaths:     paths,
		OutputArtifact: "source_records.json",
		Params: map[string]string{
			"limit": fmt.Sprintf("%d", limit),
		},
	}
	if input.SeenActionKeys[ActionIdempotencyKey(action)] {
		return dataquery.TaskPlan{}, "", false
	}
	plan.Actions = []dataquery.DataAction{action}
	plan.InputPaths = mergeActionInputPaths(plan.InputPaths, paths)
	plan.WhyThisBatch = reasonPrefix + "; materialize required local materials as reusable record artifacts before the next typed DAG stage"
	plan.NextBatch = "continue with normalization, enrichment, filtering, contribution, and reconcile actions after record artifacts exist"
	return plan, reasonPrefix + "; converted to required record materialization stage", true
}

func HasRecordActionArtifact(artifacts []ArtifactSchemaProjection) bool {
	for _, artifact := range artifacts {
		if ArtifactUsableForRecordAction(artifact) {
			return true
		}
	}
	return false
}

func RecordMaterializationPaths(contract dataquery.CoverageContract) []string {
	var out []string
	for _, material := range contract.RequiredMaterials {
		if !material.Required {
			continue
		}
		mode := NormalizeCoverageMaterialUseMode(material.UsageMode)
		switch mode {
		case dataquery.MaterialUsePlannerDistilled, dataquery.MaterialUseReferenceOnly:
			continue
		case dataquery.MaterialUseTextEvidenceConsumed:
			if textPath := strings.TrimSpace(material.TextEvidencePath); textPath != "" {
				out = append(out, textPath)
				continue
			}
		}
		if p := strings.TrimSpace(material.Path); p != "" {
			out = append(out, p)
		}
	}
	return cleanStrings(out)
}

type ReferenceProjectionGap struct {
	Candidate dataquery.ReferenceKeyCandidate
	Present   bool
}

type OutputProjectionPlanInput struct {
	Current                dataquery.TaskPlan
	Coverage               dataquery.CoverageContract
	Output                 dataquery.OutputContract
	Result                 dataquery.Result
	ReferenceGap           ReferenceProjectionGap
	PlanHasCustomTransform bool
}

func BuildRequiredOutputProjectionPlan(input OutputProjectionPlanInput) (dataquery.TaskPlan, bool) {
	needsProjection := ResultNeedsOutputProjection(ResultProjectionNeedInput{
		Current:                input.Current,
		Coverage:               input.Coverage,
		Output:                 input.Output,
		Result:                 input.Result,
		PlanHasCustomTransform: input.PlanHasCustomTransform,
	})
	if !needsProjection && !input.ReferenceGap.Present {
		return dataquery.TaskPlan{}, false
	}
	contract := BestOutputContract(input.Result.OutputContract, input.Output, input.Current.OutputContract)
	coverage := input.Coverage
	coverage.ReconcileRequired = true
	params := map[string]string{
		"order_by":    "group_key",
		"value_field": "actual",
	}
	if delimiter := strings.TrimSpace(contract.Delimiter); delimiter != "" {
		params["delimiter"] = delimiter
	}
	if input.ReferenceGap.Present {
		candidate := input.ReferenceGap.Candidate
		contract.CompleteReference = true
		contract.ReferencePath = candidate.Path
		contract.ReferenceKeyField = candidate.Field
		params["complete_reference"] = "true"
		params["reference_path"] = candidate.Path
		params["reference_key_field"] = candidate.Field
	}
	plan := dataquery.TaskPlan{
		Status:           "ready",
		OutputContract:   contract,
		CoverageContract: coverage,
		Goal:             strings.TrimSpace(input.Current.Goal),
		SuccessCriteria:  append([]string(nil), input.Current.SuccessCriteria...),
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
	if input.ReferenceGap.Present {
		candidate := input.ReferenceGap.Candidate
		plan.WhyThisBatch = "project existing reconcile groups across the complete structural reference key universe"
		plan.NextBatch = fmt.Sprintf("assemble_answer will preserve %d reference key(s) from %s.%s and fill missing groups with zero/empty values", candidate.KeyCount, candidate.Path, candidate.Field)
	}
	if strings.TrimSpace(plan.Goal) == "" {
		plan.Goal = "complete the final answer projection from already reconciled data"
	}
	return plan, true
}

type ResultProjectionNeedInput struct {
	Current                dataquery.TaskPlan
	Coverage               dataquery.CoverageContract
	Output                 dataquery.OutputContract
	Result                 dataquery.Result
	PlanHasCustomTransform bool
}

func ResultNeedsOutputProjection(input ResultProjectionNeedInput) bool {
	contract := BestOutputContract(input.Result.OutputContract, input.Output, input.Current.OutputContract)
	if contract.Format == "" {
		return false
	}
	contract = contract.Normalize()
	if input.Result.Reconcile != nil && len(input.Result.Reconcile.Groups) > 0 &&
		OutputContractNeedsFinalProjection(contract) &&
		!ResultHasAssembleAnswerArtifact(input.Result) &&
		!input.PlanHasCustomTransform {
		return true
	}
	if strings.TrimSpace(input.Result.Answer) != "" && !dataquery.AnswerLooksLikeArtifactSummary(input.Result.Answer) {
		return false
	}
	if input.Result.Reconcile == nil || len(input.Result.Reconcile.Groups) == 0 {
		return false
	}
	if contract.Format != dataquery.OutputFreeform {
		return true
	}
	if !contract.ExplanationAllowed {
		return true
	}
	return input.Coverage.ReconcileRequired
}

func OutputContractNeedsFinalProjection(contract dataquery.OutputContract) bool {
	contract = contract.Normalize()
	if contract.Format == "" {
		return false
	}
	return contract.Format != dataquery.OutputFreeform || !contract.ExplanationAllowed
}

func ResultHasAssembleAnswerArtifact(result dataquery.Result) bool {
	for _, artifact := range result.Artifacts {
		if strings.EqualFold(strings.TrimSpace(artifact.Kind), string(dataquery.DataActionAssembleAnswer)) {
			return true
		}
	}
	return false
}

type RequiredLedgerCompletionPlanInput struct {
	Current                dataquery.TaskPlan
	Coverage               dataquery.CoverageContract
	Output                 dataquery.OutputContract
	Result                 dataquery.Result
	ErrorText              string
	ReferenceGap           ReferenceProjectionGap
	PlanHasCustomTransform bool
}

func BuildRequiredLedgerCompletionPlan(input RequiredLedgerCompletionPlanInput) (dataquery.TaskPlan, bool) {
	if plan, ok := BuildRequiredOutputProjectionPlan(OutputProjectionPlanInput{
		Current:                input.Current,
		Coverage:               input.Coverage,
		Output:                 input.Output,
		Result:                 input.Result,
		ReferenceGap:           input.ReferenceGap,
		PlanHasCustomTransform: input.PlanHasCustomTransform,
	}); ok {
		return plan, true
	}
	violation := dataquery.ClassifyExecutionError(input.ErrorText)
	if strings.TrimSpace(violation.Code) != "missing_required_ledger" {
		return dataquery.TaskPlan{}, false
	}
	plan := dataquery.TaskPlan{
		Status:           "ready",
		OutputContract:   BestOutputContract(input.Result.OutputContract, input.Output, input.Current.OutputContract),
		CoverageContract: input.Coverage,
		Goal:             strings.TrimSpace(input.Current.Goal),
		SuccessCriteria:  append([]string(nil), input.Current.SuccessCriteria...),
		ContinueAfter:    false,
	}
	if strings.TrimSpace(plan.Goal) == "" {
		plan.Goal = "complete required data validation ledgers without changing the computed answer"
	}
	switch strings.TrimSpace(violation.JSONPath) {
	case "/rule_coverage":
		action := RuleCoverageCompletionAction(input.Coverage)
		if strings.TrimSpace(action.ID) == "" {
			return dataquery.TaskPlan{}, false
		}
		plan.Actions = []dataquery.DataAction{action}
		plan.InputPaths = mergeActionInputPaths(plan.InputPaths, action.InputPaths)
		plan.WhyThisBatch = "complete missing source-backed rule coverage using a typed derive_rules node"
		return plan, true
	case "/entity_resolutions":
		return dataquery.TaskPlan{}, false
	case "/reconcile":
		if len(input.Result.Contributions) == 0 {
			return dataquery.TaskPlan{}, false
		}
		plan.Actions = []dataquery.DataAction{{
			ID:             "complete_reconcile",
			Kind:           dataquery.DataActionReconcile,
			Purpose:        "complete missing reconcile ledger from existing contribution records",
			OutputArtifact: "reconcile_result.json",
		}}
		plan.WhyThisBatch = "complete missing reconcile ledger from existing contribution records"
		return plan, true
	default:
		return dataquery.TaskPlan{}, false
	}
}

func RuleCoverageCompletionAction(contract dataquery.CoverageContract) dataquery.DataAction {
	var inputs []string
	for _, material := range contract.RequiredMaterials {
		mode := NormalizeCoverageMaterialUseMode(material.UsageMode)
		if mode != dataquery.MaterialUseScriptConsumed && mode != dataquery.MaterialUsePlannerDistilled {
			continue
		}
		p := strings.TrimSpace(material.Path)
		if p == "" || !PathLooksLikeTextConstraintMaterial(p) {
			continue
		}
		inputs = append(inputs, p)
	}
	params := map[string]string{}
	if len(inputs) == 0 && len(contract.ValidationRules) > 0 {
		params["rules"] = strings.Join(cleanStrings(contract.ValidationRules), "\n")
	}
	if len(inputs) == 0 && strings.TrimSpace(params["rules"]) == "" {
		return dataquery.DataAction{}
	}
	return dataquery.DataAction{
		ID:             "complete_rule_coverage",
		Kind:           dataquery.DataActionDeriveRules,
		Purpose:        "complete missing generic rule coverage ledger from required rule/constraint materials",
		InputPaths:     cleanStrings(inputs),
		OutputArtifact: "rules_artifacts.json",
		Params:         params,
	}
}

func BestOutputContract(values ...dataquery.OutputContract) dataquery.OutputContract {
	var best dataquery.OutputContract
	bestScore := -1
	for _, value := range values {
		score := outputContractSpecificity(value)
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

func outputContractSpecificity(value dataquery.OutputContract) int {
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

func NormalizeCoverageMaterialUseMode(mode dataquery.CoverageMaterialUseMode) dataquery.CoverageMaterialUseMode {
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

func PathLooksLikeTextConstraintMaterial(p string) bool {
	ext := strings.ToLower(path.Ext(strings.TrimSpace(p)))
	switch ext {
	case ".md", ".markdown", ".txt", ".text", ".rst", ".adoc", ".asciidoc":
		return true
	default:
		return false
	}
}

func PathLooksLikeStructuredMaterial(p string) bool {
	ext := strings.ToLower(path.Ext(strings.TrimSpace(p)))
	switch ext {
	case ".csv", ".tsv", ".json", ".jsonl":
		return true
	default:
		return false
	}
}

func baseContinuationPlan(current dataquery.TaskPlan, coverage dataquery.CoverageContract, output dataquery.OutputContract) dataquery.TaskPlan {
	plan := dataquery.TaskPlan{
		Status:           "ready",
		OutputContract:   output,
		CoverageContract: coverage,
		Goal:             strings.TrimSpace(current.Goal),
		SuccessCriteria:  append([]string(nil), current.SuccessCriteria...),
		ContinueAfter:    true,
	}
	if strings.TrimSpace(plan.Goal) == "" {
		plan.Goal = "continue the typed data workflow toward the user's requested output"
	}
	return plan
}

func actionAllowed(values []string, kind dataquery.DataActionKind) bool {
	allowed := allowedActionSet(values)
	return allowed[string(NormalizeActionKind(kind))]
}

func appendValidationRule(values []string, rule string) []string {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return cleanStrings(values)
	}
	return cleanStrings(append(append([]string(nil), values...), rule))
}

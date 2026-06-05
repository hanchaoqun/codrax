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
	DefaultDataTaskMaxRepairRounds = 6
	DefaultDataTaskMaxDataRounds   = 12
	DefaultDataTaskMaxNodeFailures = 2
	dataTaskMaxRepairRoundsCeiling = 12
	dataTaskMaxDataRoundsCeiling   = 24

	dataTaskOneShotScriptLineSoftLimit   = 260
	dataTaskOneShotScriptLineHardLimit   = 420
	dataTaskOneShotRequiredMaterialLimit = 8
	dataTaskOneShotValidationLedgerLimit = 3
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
	if plan.ContinueAfter {
		return ""
	}
	lines := dataTaskScriptLineCount(plan.Script)
	requiredMaterials := len(plan.CoverageContract.RequiredMaterials)
	validationLedgers := dataTaskValidationLedgerCount(plan.CoverageContract)
	inputs := len(plan.InputPaths)
	complexBatch := requiredMaterials >= 4 || validationLedgers >= 2 || inputs >= 4
	oversized := lines >= dataTaskOneShotScriptLineHardLimit ||
		(lines >= dataTaskOneShotScriptLineSoftLimit && (requiredMaterials >= dataTaskOneShotRequiredMaterialLimit || validationLedgers >= dataTaskOneShotValidationLedgerLimit)) ||
		(lines >= 180 && complexBatch) ||
		(requiredMaterials >= dataTaskOneShotRequiredMaterialLimit+4 && validationLedgers >= dataTaskOneShotValidationLedgerLimit)
	if !oversized {
		return ""
	}
	return fmt.Sprintf("data planning incomplete: plan is too large for one bounded data batch (script_lines=%d input_paths=%d required_materials=%d validation_ledgers=%d continue_after=false). Emit a smaller atomic actions[] batch such as material_inventory, inspect_material, extract_records, derive_rules, normalize_entities, compute_contributions, reconcile_artifacts, or a bounded custom_transform; set continue_after=true when further work remains, and let the workflow feed real results into later batches.",
		lines, inputs, requiredMaterials, validationLedgers)
}

func dataTaskActionStagingGuardError(plan dataquery.TaskPlan) string {
	topLevelLines := dataTaskScriptLineCount(plan.Script)
	if topLevelLines > 0 {
		return fmt.Sprintf("data planning incomplete: actions[] plans must not carry a top-level script (script_lines=%d). Put each bounded transform script on its custom_transform action, or split the workflow into typed atomic actions; top-level script is only for simple non-actions plans.",
			topLevelLines)
	}
	for i, action := range plan.Actions {
		lines := dataTaskScriptLineCount(action.Script)
		if lines == 0 {
			continue
		}
		if lines >= dataTaskOneShotScriptLineSoftLimit {
			return fmt.Sprintf("data planning incomplete: action %d (%s) is too large for one atomic data action (script_lines=%d). Split the workflow into smaller typed actions such as material_inventory, inspect_material, and bounded custom_transform nodes.",
				i+1, strings.TrimSpace(string(action.Kind)), lines)
		}
	}
	return ""
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
	ContractWarnings         []string                           `json:"contract_warnings,omitempty"`
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
		ContractWarnings:         append([]string(nil), result.ContractWarnings...),
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

func preserveDataTaskRepairCoverage(previous, repaired dataquery.TaskPlan) dataquery.TaskPlan {
	repaired.CoverageContract = mergeDataTaskCoverageContracts(previous.CoverageContract, repaired.CoverageContract)
	repaired.InputPaths = mergeDataTaskInputPaths(repaired.InputPaths, repaired.CoverageContract.RequiredRunnerInputPaths())
	return repaired
}

func preserveDataTaskMaterialRepairCoverage(previous, repaired dataquery.TaskPlan) dataquery.TaskPlan {
	if len(repaired.CoverageContract.RequiredMaterials) == 0 && len(repaired.CoverageContract.OptionalMaterials) == 0 {
		return preserveDataTaskRepairCoverage(previous, repaired)
	}
	repaired.CoverageContract.DecisionRecordsRequired = previous.CoverageContract.DecisionRecordsRequired || repaired.CoverageContract.DecisionRecordsRequired
	repaired.CoverageContract.RuleCoverageRequired = previous.CoverageContract.RuleCoverageRequired || repaired.CoverageContract.RuleCoverageRequired
	repaired.CoverageContract.ContributionLedgerRequired = previous.CoverageContract.ContributionLedgerRequired || repaired.CoverageContract.ContributionLedgerRequired
	repaired.CoverageContract.EntityResolutionRequired = previous.CoverageContract.EntityResolutionRequired || repaired.CoverageContract.EntityResolutionRequired
	repaired.CoverageContract.ReconcileRequired = previous.CoverageContract.ReconcileRequired || repaired.CoverageContract.ReconcileRequired
	repaired.InputPaths = mergeDataTaskInputPaths(repaired.InputPaths, repaired.CoverageContract.RequiredRunnerInputPaths())
	return repaired
}

func preserveDataTaskMaterialRepairCoverageForError(previous, repaired dataquery.TaskPlan, errText string) dataquery.TaskPlan {
	violation := dataquery.ClassifyExecutionError(errText)
	if violation.Code == "oversized_data_plan" && repaired.ContinueAfter && (len(repaired.CoverageContract.RequiredMaterials) > 0 || len(repaired.CoverageContract.OptionalMaterials) > 0) {
		repaired.InputPaths = mergeDataTaskInputPaths(repaired.InputPaths, repaired.CoverageContract.RequiredRunnerInputPaths())
		return repaired
	}
	return preserveDataTaskMaterialRepairCoverage(previous, repaired)
}

func mergeDataTaskCoverageContracts(previous, next dataquery.CoverageContract) dataquery.CoverageContract {
	out := next
	out.RequiredMaterials = mergeDataTaskCoverageMaterials(previous.RequiredMaterials, next.RequiredMaterials, true)
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
		appendOne(m)
	}
	for _, m := range next {
		appendOne(m)
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

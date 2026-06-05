package repl

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

const (
	DefaultDataTaskMaxRepairRounds = 6
	DefaultDataTaskMaxDataRounds   = 12
	dataTaskMaxRepairRoundsCeiling = 12
	dataTaskMaxDataRoundsCeiling   = 24
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
	Answer                  string                             `json:"answer,omitempty"`
	OutputContract          dataquery.OutputContract           `json:"output_contract,omitempty"`
	AuditSummary            string                             `json:"audit_summary,omitempty"`
	DecisionRecords         int                                `json:"decision_records,omitempty"`
	DecisionSamples         []dataquery.RowDecision            `json:"decision_samples,omitempty"`
	RuleCoverageRecords     int                                `json:"rule_coverage_records,omitempty"`
	RuleCoverageSamples     []dataquery.RuleCoverageRecord     `json:"rule_coverage_samples,omitempty"`
	ContributionRecords     int                                `json:"contribution_records,omitempty"`
	ContributionSamples     []dataquery.ContributionRecord     `json:"contribution_samples,omitempty"`
	EntityResolutionRecords int                                `json:"entity_resolution_records,omitempty"`
	EntityResolutionSamples []dataquery.EntityResolutionRecord `json:"entity_resolution_samples,omitempty"`
	Reconcile               *dataquery.ReconcileReport         `json:"reconcile,omitempty"`
	Metrics                 []dataquery.Metric                 `json:"metrics,omitempty"`
	ContractWarnings        []string                           `json:"contract_warnings,omitempty"`
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
			view.Result = &dataTaskResultPromptView{
				Answer:                  clampDataTaskWorkflowText(rec.Result.Answer, 2400),
				OutputContract:          rec.Result.OutputContract.Normalize(),
				AuditSummary:            clampDataTaskWorkflowText(rec.Result.AuditSummary, 1000),
				DecisionRecords:         len(rec.Result.Rows),
				DecisionSamples:         sampleDataTaskRowDecisions(rec.Result.Rows, 6),
				RuleCoverageRecords:     len(rec.Result.RuleCoverage),
				RuleCoverageSamples:     sampleDataTaskRuleCoverage(rec.Result.RuleCoverage, 4),
				ContributionRecords:     len(rec.Result.Contributions),
				ContributionSamples:     sampleDataTaskContributions(rec.Result.Contributions, 6),
				EntityResolutionRecords: len(rec.Result.EntityResolutions),
				EntityResolutionSamples: sampleDataTaskEntityResolutions(rec.Result.EntityResolutions, 4),
				Reconcile:               clampPromptReconcileReport(rec.Result.Reconcile),
				Metrics:                 rec.Result.Metrics,
				ContractWarnings:        append([]string(nil), rec.Result.ContractWarnings...),
			}
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
	repaired.InputPaths = mergeDataTaskInputPaths(repaired.InputPaths, repaired.CoverageContract.RequiredPaths())
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
	repaired.InputPaths = mergeDataTaskInputPaths(repaired.InputPaths, repaired.CoverageContract.RequiredPaths())
	return repaired
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
	seen := map[string]bool{}
	appendOne := func(m dataquery.CoverageMaterial) {
		path := strings.TrimSpace(m.Path)
		id := strings.TrimSpace(m.ID)
		purpose := strings.TrimSpace(m.Purpose)
		if path == "" && id == "" && purpose == "" {
			return
		}
		key := path + "\x00" + id + "\x00" + purpose
		if seen[key] {
			return
		}
		seen[key] = true
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

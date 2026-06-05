package repl

import (
	"encoding/json"
	"fmt"
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
	Answer           string                   `json:"answer,omitempty"`
	OutputContract   dataquery.OutputContract `json:"output_contract,omitempty"`
	AuditSummary     string                   `json:"audit_summary,omitempty"`
	DecisionRecords  int                      `json:"decision_records,omitempty"`
	Metrics          []dataquery.Metric       `json:"metrics,omitempty"`
	ContractWarnings []string                 `json:"contract_warnings,omitempty"`
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
				Answer:           clampDataTaskWorkflowText(rec.Result.Answer, 2400),
				OutputContract:   rec.Result.OutputContract.Normalize(),
				AuditSummary:     clampDataTaskWorkflowText(rec.Result.AuditSummary, 1000),
				DecisionRecords:  len(rec.Result.Rows),
				Metrics:          rec.Result.Metrics,
				ContractWarnings: append([]string(nil), rec.Result.ContractWarnings...),
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

func mergeDataTaskCoverageContracts(previous, next dataquery.CoverageContract) dataquery.CoverageContract {
	out := next
	out.RequiredMaterials = mergeDataTaskCoverageMaterials(previous.RequiredMaterials, next.RequiredMaterials, true)
	out.OptionalMaterials = mergeDataTaskCoverageMaterials(previous.OptionalMaterials, next.OptionalMaterials, false)
	if len(out.ValidationRules) == 0 && len(previous.ValidationRules) > 0 {
		out.ValidationRules = append([]string(nil), previous.ValidationRules...)
	}
	out.DecisionRecordsRequired = previous.DecisionRecordsRequired || next.DecisionRecordsRequired
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

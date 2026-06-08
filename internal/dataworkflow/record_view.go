package dataworkflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type WorkflowRecordView struct {
	Round               int                      `json:"round"`
	PlanStatus          string                   `json:"plan_status,omitempty"`
	Goal                string                   `json:"goal,omitempty"`
	Actions             []dataquery.DataAction   `json:"actions,omitempty"`
	InputPaths          []string                 `json:"input_paths,omitempty"`
	OutputContract      dataquery.OutputContract `json:"output_contract,omitempty"`
	SuccessCriteria     []string                 `json:"success_criteria,omitempty"`
	MissingObservations []string                 `json:"missing_observations,omitempty"`
	NextBatch           string                   `json:"next_batch,omitempty"`
	WhyThisBatch        string                   `json:"why_this_batch,omitempty"`
	ContinueAfter       bool                     `json:"continue_after,omitempty"`
	ScriptPreview       string                   `json:"script_preview,omitempty"`
	Result              any                      `json:"result,omitempty"`
	Error               string                   `json:"error,omitempty"`
	Evaluation          *dataquery.Evaluation    `json:"evaluation,omitempty"`
}

type WorkflowRecordViewBudget struct {
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

type WorkflowRecordResultViewFunc func(result dataquery.Result, budget WorkflowRecordViewBudget) any

func RenderWorkflowRecordViewsForPrompt(records []WorkflowRecord, budget WorkflowRecordViewBudget, resultView WorkflowRecordResultViewFunc) string {
	views, omitted := BuildWorkflowRecordViews(records, budget, resultView)
	if len(views) == 0 {
		return "(none)\n"
	}
	raw, err := json.MarshalIndent(views, "", "  ")
	if err != nil {
		return fmt.Sprintf("render data workflow records failed: %v\n", err)
	}
	if omitted > 0 {
		return fmt.Sprintf("(omitted %d older data rounds; workflow_state_json is authoritative for cumulative coverage, counts, and next-stage state)\n%s", omitted, string(raw))
	}
	return string(raw)
}

func BuildWorkflowRecordViews(records []WorkflowRecord, budget WorkflowRecordViewBudget, resultView WorkflowRecordResultViewFunc) ([]WorkflowRecordView, int) {
	if len(records) == 0 {
		return nil, 0
	}
	if budget.MaxRecords <= 0 {
		budget.MaxRecords = 1
	}
	start := len(records) - budget.MaxRecords
	if start < 0 {
		start = 0
	}
	views := make([]WorkflowRecordView, 0, len(records)-start)
	for i := start; i < len(records); i++ {
		rec := records[i]
		view := WorkflowRecordView{
			Round:               i + 1,
			PlanStatus:          strings.TrimSpace(rec.Plan.Status),
			Goal:                strings.TrimSpace(rec.Plan.Goal),
			Actions:             CompactActionsForRecordView(rec.Plan.Actions, budget.MaxActions, budget.ActionScriptLimit, budget.ActionParamLimit),
			InputPaths:          append([]string(nil), rec.Plan.InputPaths...),
			OutputContract:      rec.Plan.OutputContract.Normalize(),
			SuccessCriteria:     append([]string(nil), rec.Plan.SuccessCriteria...),
			MissingObservations: append([]string(nil), rec.Plan.MissingObservations...),
			NextBatch:           strings.TrimSpace(rec.Plan.NextBatch),
			WhyThisBatch:        strings.TrimSpace(rec.Plan.WhyThisBatch),
			ContinueAfter:       rec.Plan.ContinueAfter,
			ScriptPreview:       ClampRecordViewText(rec.Plan.Script, budget.ScriptPreviewLimit),
			Error:               ClampRecordViewText(rec.Err, budget.ErrorLimit),
		}
		if rec.Result != nil && resultView != nil {
			view.Result = resultView(*rec.Result, budget)
		}
		if rec.Evaluation != nil {
			eval := *rec.Evaluation
			eval.MissingInputs = append([]string(nil), rec.Evaluation.MissingInputs...)
			eval.Reason = ClampRecordViewText(eval.Reason, budget.EvalReasonLimit)
			view.Evaluation = &eval
		}
		views = append(views, view)
	}
	return views, start
}

func CompactActionsForRecordView(actions []dataquery.DataAction, limit, scriptLimit, paramLimit int) []dataquery.DataAction {
	if limit <= 0 || len(actions) == 0 {
		return nil
	}
	if len(actions) > limit {
		actions = actions[:limit]
	}
	out := make([]dataquery.DataAction, 0, len(actions))
	for _, action := range actions {
		action.Purpose = ClampRecordViewText(action.Purpose, 240)
		action.InputPaths = ClampRecordViewStringSlice(action.InputPaths, 12)
		action.Script = ClampRecordViewText(action.Script, scriptLimit)
		action.Params = compactActionParamsForRecordView(action.Params, 16, paramLimit)
		action.SuccessCriteria = ClampRecordViewStringSlice(action.SuccessCriteria, 8)
		out = append(out, action)
	}
	return out
}

func compactActionParamsForRecordView(params map[string]string, maxKeys, valueLimit int) map[string]string {
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
		out[key] = ClampRecordViewText(params[key], valueLimit)
	}
	return out
}

func ClampRecordViewText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "\n...[truncated]"
}

func ClampRecordViewStringSlice(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return append([]string(nil), values...)
	}
	out := append([]string(nil), values[:limit]...)
	out = append(out, fmt.Sprintf("...%d more", len(values)-limit))
	return out
}

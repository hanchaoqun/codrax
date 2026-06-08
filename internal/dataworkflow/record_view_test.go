package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestRenderWorkflowRecordViewsForPromptBoundsRecordsAndText(t *testing.T) {
	records := []WorkflowRecord{
		{
			Plan: dataquery.TaskPlan{Status: "ready", Goal: "older"},
		},
		{
			Plan: dataquery.TaskPlan{
				Status: "ready",
				Goal:   "newer",
				Actions: []dataquery.DataAction{{
					ID:         "a1",
					Kind:       dataquery.DataActionFilterRecords,
					Purpose:    strings.Repeat("p", 300),
					InputPaths: []string{"records.json"},
					Script:     strings.Repeat("s", 50),
					Params: map[string]string{
						"z": strings.Repeat("z", 20),
					},
				}},
				ContinueAfter: true,
			},
			Err: strings.Repeat("e", 50),
			Result: &dataquery.Result{
				Answer: "intermediate",
			},
			Evaluation: &dataquery.Evaluation{
				Status: dataquery.EvalContinueData,
				Reason: strings.Repeat("r", 50),
			},
		},
	}
	rendered := RenderWorkflowRecordViewsForPrompt(records, WorkflowRecordViewBudget{
		MaxRecords:         1,
		MaxActions:         1,
		ActionScriptLimit:  8,
		ActionParamLimit:   8,
		ErrorLimit:         8,
		EvalReasonLimit:    8,
		ResultAnswerLimit:  8,
		ScriptPreviewLimit: 8,
	}, func(result dataquery.Result, budget WorkflowRecordViewBudget) any {
		return map[string]string{"answer": ClampRecordViewText(result.Answer, budget.ResultAnswerLimit)}
	})
	for _, want := range []string{
		"(omitted 1 older data rounds",
		`"round": 2`,
		`"goal": "newer"`,
		"...[truncated]",
		`"answer": "intermed`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered prompt missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, `"goal": "older"`) {
		t.Fatalf("rendered prompt should omit older record:\n%s", rendered)
	}
}

func TestCompactActionsForRecordViewSortsAndTruncatesParams(t *testing.T) {
	params := map[string]string{}
	for _, key := range []string{"z", "a", "m", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "n", "o", "p"} {
		params[key] = strings.Repeat(key, 20)
	}
	actions := CompactActionsForRecordView([]dataquery.DataAction{{
		Kind:   dataquery.DataActionDeriveFields,
		Params: params,
	}}, 1, 10, 5)
	if len(actions) != 1 {
		t.Fatalf("actions=%+v, want one compact action", actions)
	}
	if _, ok := actions[0].Params["_truncated_params"]; !ok {
		t.Fatalf("params=%+v, want truncation marker", actions[0].Params)
	}
	if actions[0].Params["a"] != "aaaaa\n...[truncated]" {
		t.Fatalf("param a=%q, want bounded value", actions[0].Params["a"])
	}
}

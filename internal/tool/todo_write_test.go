package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func newMutableBus() *types.BusContext {
	return &types.BusContext{
		Mutable: types.NewMutableState(types.TaskList{Objective: "test objective"}),
	}
}

func TestTodoWrite_HappyPath(t *testing.T) {
	tw := &TodoWrite{}
	bus := newMutableBus()

	params := []byte(`{
		"tasks": [
			{"id": "t1", "title": "Read README", "type": "analysis", "status": "done"},
			{"id": "t2", "title": "Map agent layer", "type": "analysis", "status": "in_progress"},
			{"id": "t3", "title": "Summarize", "type": "analysis", "status": "pending"}
		]
	}`)

	res, err := tw.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success; got summary: %s", res.Summary)
	}

	tl := bus.Mutable.TaskList()
	if got, want := len(tl.Tasks), 3; got != want {
		t.Fatalf("len(Tasks) = %d, want %d", got, want)
	}
	if tl.CurrentTaskID != "t2" {
		t.Errorf("CurrentTaskID = %q, want t2 (the in_progress one)", tl.CurrentTaskID)
	}
	if tl.Objective != "test objective" {
		t.Errorf("Objective lost: got %q", tl.Objective)
	}
}

func TestTodoWrite_PreservesObjective(t *testing.T) {
	tw := &TodoWrite{}
	bus := newMutableBus()

	params := []byte(`{"tasks": [{"id": "t1", "title": "do thing", "type": "analysis"}]}`)
	if _, err := tw.Execute(bus, params); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := bus.Mutable.TaskList().Objective; got != "test objective" {
		t.Errorf("Objective = %q, want preserved", got)
	}
}

func TestTodoWrite_NilMutableRejected(t *testing.T) {
	tw := &TodoWrite{}
	bus := &types.BusContext{} // no Mutable
	res, err := tw.Execute(bus, []byte(`{"tasks":[{"title":"x"}]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Error("expected failure when Mutable is nil")
	}
	if !strings.Contains(res.Summary, "Mutable") {
		t.Errorf("summary should mention Mutable, got %q", res.Summary)
	}
}

func TestTodoWrite_EmptyTasksRejected(t *testing.T) {
	tw := &TodoWrite{}
	bus := newMutableBus()
	res, _ := tw.Execute(bus, []byte(`{"tasks":[]}`))
	if res.Success {
		t.Error("empty task list should be rejected")
	}
}

func TestTodoWrite_DuplicateIDRejected(t *testing.T) {
	tw := &TodoWrite{}
	bus := newMutableBus()
	res, _ := tw.Execute(bus, []byte(`{
		"tasks": [
			{"id": "x", "title": "a"},
			{"id": "x", "title": "b"}
		]
	}`))
	if res.Success {
		t.Error("duplicate id should be rejected")
	}
	if !strings.Contains(res.Summary, "duplicate") {
		t.Errorf("summary should mention duplicate, got %q", res.Summary)
	}
}

func TestTodoWrite_MissingTitleRejected(t *testing.T) {
	tw := &TodoWrite{}
	bus := newMutableBus()
	res, _ := tw.Execute(bus, []byte(`{"tasks":[{"id":"t1"}]}`))
	if res.Success {
		t.Error("missing title should be rejected")
	}
}

func TestTodoWrite_AutoAssignedID(t *testing.T) {
	tw := &TodoWrite{}
	bus := newMutableBus()
	res, _ := tw.Execute(bus, []byte(`{"tasks":[{"title":"first"},{"title":"second"}]}`))
	if !res.Success {
		t.Fatalf("expected success: %s", res.Summary)
	}
	tl := bus.Mutable.TaskList()
	if tl.Tasks[0].ID == "" || tl.Tasks[1].ID == "" {
		t.Error("missing IDs should be auto-assigned")
	}
	if tl.Tasks[0].ID == tl.Tasks[1].ID {
		t.Error("auto-assigned IDs must be unique")
	}
}

func TestNormalizeTodoStatus(t *testing.T) {
	cases := []struct {
		in   string
		want types.TaskStatus
	}{
		{"in_progress", types.TaskInProgress},
		{"in-progress", types.TaskInProgress},
		{"working", types.TaskInProgress},
		{"done", types.TaskDone},
		{"completed", types.TaskDone},
		{"blocked", types.TaskBlocked},
		{"failed", types.TaskFailed},
		{"error", types.TaskFailed},
		{"", types.TaskPending},
		{"wat", types.TaskPending},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := normalizeTodoStatus(c.in); got != c.want {
				t.Errorf("normalizeTodoStatus(%q) = %s, want %s", c.in, got, c.want)
			}
		})
	}
}

// TestTodoWrite_BoolFieldsPropagate verifies that the writing and
// high_risk flags from the tool params land verbatim on TaskItem,
// replacing the old TaskType enum-based classification.
func TestTodoWrite_BoolFieldsPropagate(t *testing.T) {
	tw := &TodoWrite{}
	bus := newMutableBus()

	params := []byte(`{
		"tasks": [
			{"id": "t1", "title": "read code", "writing": false, "high_risk": false},
			{"id": "t2", "title": "edit file", "writing": true,  "high_risk": false},
			{"id": "t3", "title": "rm -rf schema", "writing": true, "high_risk": true}
		]
	}`)
	res, err := tw.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success: %s", res.Summary)
	}

	tl := bus.Mutable.TaskList()
	cases := []struct {
		id       string
		writing  bool
		highRisk bool
	}{
		{"t1", false, false},
		{"t2", true, false},
		{"t3", true, true},
	}
	for i, c := range cases {
		if tl.Tasks[i].ID != c.id {
			t.Errorf("tasks[%d].ID = %q, want %q", i, tl.Tasks[i].ID, c.id)
		}
		if tl.Tasks[i].Writing != c.writing {
			t.Errorf("tasks[%d].Writing = %v, want %v", i, tl.Tasks[i].Writing, c.writing)
		}
		if tl.Tasks[i].HighRisk != c.highRisk {
			t.Errorf("tasks[%d].HighRisk = %v, want %v", i, tl.Tasks[i].HighRisk, c.highRisk)
		}
	}
}

func TestPickCurrentTaskID_PreferInProgress(t *testing.T) {
	items := []types.TaskItem{
		{ID: "a", Status: types.TaskDone},
		{ID: "b", Status: types.TaskInProgress},
		{ID: "c", Status: types.TaskPending},
	}
	if got := pickCurrentTaskID(items); got != "b" {
		t.Errorf("pickCurrentTaskID = %q, want b (in_progress)", got)
	}
}

func TestPickCurrentTaskID_FallbackToPending(t *testing.T) {
	items := []types.TaskItem{
		{ID: "a", Status: types.TaskDone},
		{ID: "b", Status: types.TaskPending},
	}
	if got := pickCurrentTaskID(items); got != "b" {
		t.Errorf("pickCurrentTaskID = %q, want b (first pending)", got)
	}
}

func TestPickCurrentTaskID_FallbackToFirst(t *testing.T) {
	items := []types.TaskItem{
		{ID: "a", Status: types.TaskDone},
		{ID: "b", Status: types.TaskDone},
	}
	if got := pickCurrentTaskID(items); got != "a" {
		t.Errorf("pickCurrentTaskID = %q, want a", got)
	}
}

// TestTodoWrite_AnalyzerContractFields verifies that the new fields
// feeding the deterministic pipeline (entities, question_kind,
// answer_shape) round-trip from JSON params into TaskItem exactly as
// the analyzer contract declares.
func TestTodoWrite_AnalyzerContractFields(t *testing.T) {
	tw := &TodoWrite{}
	bus := newMutableBus()

	params := []byte(`{
		"tasks": [{
			"id": "t1",
			"title": "explain ContinuationPrompt",
			"entities": ["ContinuationPrompt", "explorerEvaluator"],
			"question_kind": "mechanism",
			"answer_shape": "step_list",
			"keywords": ["ContinuationPrompt", "explorer", "prompt"]
		}]
	}`)
	res, err := tw.Execute(bus, params)
	if err != nil || !res.Success {
		t.Fatalf("execute failed: err=%v summary=%s", err, res.Summary)
	}
	item := bus.Mutable.TaskList().Tasks[0]
	if len(item.Entities) != 2 || item.Entities[0] != "ContinuationPrompt" {
		t.Errorf("Entities roundtrip failed: %v", item.Entities)
	}
	if item.QuestionKind != "mechanism" {
		t.Errorf("QuestionKind = %q, want mechanism", item.QuestionKind)
	}
	if item.AnswerShape != "step_list" {
		t.Errorf("AnswerShape = %q, want step_list", item.AnswerShape)
	}
}

// TestTodoWrite_EntitiesTrimEmpty verifies that whitespace-only
// entities are dropped, so a sloppy analyzer can't pollute the entity
// set with empty strings that would match any ERM check.
func TestTodoWrite_EntitiesTrimEmpty(t *testing.T) {
	tw := &TodoWrite{}
	bus := newMutableBus()
	params := []byte(`{"tasks":[{"title":"x","entities":["Foo","  ","","Bar"]}]}`)
	if _, err := tw.Execute(bus, params); err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	got := bus.Mutable.TaskList().Tasks[0].Entities
	if len(got) != 2 || got[0] != "Foo" || got[1] != "Bar" {
		t.Errorf("Entities = %v, want [Foo Bar]", got)
	}
}

// TestNormalizeQuestionKind_AllPaths verifies every branch of the
// normalizer including fallbacks for unknown/empty inputs.
func TestNormalizeQuestionKind_AllPaths(t *testing.T) {
	cases := map[string]string{
		"registration":   "registration",
		"REGISTER":       "registration",
		"mechanism":      "mechanism",
		"process":        "mechanism",
		"return_value":   "return_value",
		"return":         "return_value",
		"conditional":    "conditional",
		"config_mapping": "config_mapping",
		"config":         "config_mapping",
		"enumeration":    "enumeration",
		"count":          "enumeration",
		"call_chain":     "call_chain",
		"calls":          "call_chain",
		"":               "unknown",
		"unknown":        "unknown",
		"gibberish":      "unknown",
	}
	for in, want := range cases {
		if got := normalizeQuestionKind(in); got != want {
			t.Errorf("normalizeQuestionKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeAnswerShape_AllPaths(t *testing.T) {
	cases := map[string]string{
		"list_of_symbols": "list_of_symbols",
		"symbol_list":     "list_of_symbols",
		"step_list":       "step_list",
		"steps":           "step_list",
		"value":           "value",
		"literal":         "value",
		"boolean":         "boolean",
		"yes_no":          "boolean",
		"config_value":    "config_value",
		"":                "none",
		"whatever":        "none",
	}
	for in, want := range cases {
		if got := normalizeAnswerShape(in); got != want {
			t.Errorf("normalizeAnswerShape(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestTodoWrite_ParamsRoundTrip just sanity-checks that the schema's
// declared fields match what the tool actually consumes; if a field
// gets renamed in the schema string, this test catches the divergence.
func TestTodoWrite_ParamsRoundTrip(t *testing.T) {
	tw := &TodoWrite{}
	var schema map[string]any
	if err := json.Unmarshal(tw.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters() does not return valid JSON: %v", err)
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["tasks"]; !ok {
		t.Error("schema should declare 'tasks' property")
	}
}

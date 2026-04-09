package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/design/internal/types"
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

func TestNormalizeTodoType(t *testing.T) {
	if got := normalizeTodoType("implementation"); got != types.TaskTypeImplementation {
		t.Errorf("implementation classified as %s", got)
	}
	if got := normalizeTodoType("analysis"); got != types.TaskTypeAnalysis {
		t.Errorf("analysis classified as %s", got)
	}
	if got := normalizeTodoType("wat"); got != types.TaskTypeAnalysis {
		t.Errorf("unknown classified as %s, want analysis (fail-safe)", got)
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

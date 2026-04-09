package agent

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/design/internal/llm"
	"github.com/hanchaoqun/design/internal/types"
)

func TestParseAnalyzeOutput(t *testing.T) {
	cases := []struct {
		name        string
		raw         string
		wantType    types.TaskType
		wantTaskNum int
	}{
		{
			name: "valid analysis JSON with task list",
			raw: `{
				"objective": "explain project",
				"tasks": [
					{"id": "t1", "title": "read README", "description": "...", "type": "analysis"},
					{"id": "t2", "title": "summarize",   "description": "...", "type": "analysis"}
				]
			}`,
			wantType:    types.TaskTypeAnalysis,
			wantTaskNum: 2,
		},
		{
			name: "valid implementation JSON",
			raw: `{
				"objective": "add feature X",
				"tasks": [{"id": "t1", "title": "implement", "type": "implementation"}]
			}`,
			wantType:    types.TaskTypeImplementation,
			wantTaskNum: 1,
		},
		{
			name:        "code-fenced JSON",
			raw:         "```json\n{\"objective\": \"x\", \"tasks\": [{\"id\": \"t1\", \"title\": \"x\", \"type\": \"analysis\"}]}\n```",
			wantType:    types.TaskTypeAnalysis,
			wantTaskNum: 1,
		},
		{
			name:        "JSON with leading prose",
			raw:         `Here is the analysis: {"objective": "x", "tasks": [{"id": "t1", "title": "x", "type": "analysis"}]} done.`,
			wantType:    types.TaskTypeAnalysis,
			wantTaskNum: 1,
		},
		{
			name:        "non-JSON garbage falls back to single analysis task",
			raw:         "I'm not sure how to classify this.",
			wantType:    types.TaskTypeAnalysis,
			wantTaskNum: 1,
		},
		{
			name:        "empty string falls back",
			raw:         "",
			wantType:    types.TaskTypeAnalysis,
			wantTaskNum: 1,
		},
		{
			name:        "empty tasks array falls back",
			raw:         `{"objective": "x", "tasks": []}`,
			wantType:    types.TaskTypeAnalysis,
			wantTaskNum: 1,
		},
		{
			name:        "unknown task type normalizes to analysis",
			raw:         `{"tasks": [{"id": "t1", "title": "x", "type": "wat"}]}`,
			wantType:    types.TaskTypeAnalysis,
			wantTaskNum: 1,
		},
		{
			name:        "implement synonym normalizes to implementation",
			raw:         `{"tasks": [{"id": "t1", "title": "x", "type": "implement"}]}`,
			wantType:    types.TaskTypeImplementation,
			wantTaskNum: 1,
		},
		{
			name:        "missing id gets synthetic value",
			raw:         `{"tasks": [{"title": "x", "type": "analysis"}]}`,
			wantType:    types.TaskTypeAnalysis,
			wantTaskNum: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tl := parseAnalyzeOutput(c.raw)
			if tl == nil {
				t.Fatal("parseAnalyzeOutput returned nil")
			}
			if got := len(tl.Tasks); got != c.wantTaskNum {
				t.Fatalf("task count = %d, want %d", got, c.wantTaskNum)
			}
			if tl.Tasks[0].Type != c.wantType {
				t.Errorf("tasks[0].Type = %s, want %s", tl.Tasks[0].Type, c.wantType)
			}
			if tl.Tasks[0].ID == "" {
				t.Error("tasks[0].ID should not be empty (synthetic ID expected)")
			}
			if tl.CurrentTaskID != tl.Tasks[0].ID {
				t.Errorf("CurrentTaskID = %q, want %q", tl.CurrentTaskID, tl.Tasks[0].ID)
			}
			if tl.Tasks[0].Status != types.TaskPending {
				t.Errorf("tasks[0].Status = %s, want pending", tl.Tasks[0].Status)
			}
		})
	}
}

func TestExtractJSONObject(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"plain object", `{"a":1}`, `{"a":1}`, true},
		{"with surrounding prose", `Here: {"a":1} done.`, `{"a":1}`, true},
		{"json code fence", "```json\n{\"a\":1}\n```", `{"a":1}`, true},
		{"bare code fence", "```\n{\"a\":1}\n```", `{"a":1}`, true},
		{"no braces", `not json at all`, ``, false},
		{"only opening brace", `{"a":1`, ``, false},
		{"empty string", ``, ``, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := extractJSONObject(c.in)
			if ok != c.ok {
				t.Errorf("ok = %v, want %v", ok, c.ok)
			}
			if ok && got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestNormalizeTaskType(t *testing.T) {
	cases := []struct {
		in   types.TaskType
		want types.TaskType
	}{
		{"implementation", types.TaskTypeImplementation},
		{"implement", types.TaskTypeImplementation},
		{"code", types.TaskTypeImplementation},
		{"feature", types.TaskTypeImplementation},
		{"fix", types.TaskTypeImplementation},
		{"bug", types.TaskTypeImplementation},
		{"  Implement  ", types.TaskTypeImplementation},
		{"analysis", types.TaskTypeAnalysis},
		{"explain", types.TaskTypeAnalysis},
		{"", types.TaskTypeAnalysis},
		{"wat", types.TaskTypeAnalysis},
		{"unknown", types.TaskTypeAnalysis},
	}
	for _, c := range cases {
		t.Run(string(c.in), func(t *testing.T) {
			if got := normalizeTaskType(c.in); got != c.want {
				t.Errorf("normalizeTaskType(%q) = %s, want %s", c.in, got, c.want)
			}
		})
	}
}

func TestDefaultAnalysisTaskList(t *testing.T) {
	tl := defaultAnalysisTaskList()
	if tl == nil {
		t.Fatal("defaultAnalysisTaskList returned nil")
	}
	if len(tl.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tl.Tasks))
	}
	if tl.Tasks[0].Type != types.TaskTypeAnalysis {
		t.Errorf("default task type = %s, want analysis", tl.Tasks[0].Type)
	}
	if tl.CurrentTaskID != tl.Tasks[0].ID {
		t.Errorf("CurrentTaskID = %q, want %q", tl.CurrentTaskID, tl.Tasks[0].ID)
	}
	if got := tl.CurrentTask(); got == nil {
		t.Error("CurrentTask() returned nil; CurrentTaskID and Tasks must agree")
	}
}

// TestAnalyzerParseOutputEndToEnd verifies that analyzerEvaluator.ParseOutput
// populates StageOutput.TaskListUpdate from a realistic LLM message.
func TestAnalyzerParseOutputEndToEnd(t *testing.T) {
	e := &analyzerEvaluator{}
	msgs := []llm.Message{
		{Role: "assistant", Content: `{"objective": "explain", "tasks": [{"id": "t1", "title": "explain", "type": "analysis"}]}`},
	}

	out, err := e.ParseOutput(&types.AgentContext{Stage: types.StageAnalyze}, msgs, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.TaskListUpdate == nil {
		t.Fatal("expected TaskListUpdate to be set")
	}
	if got := out.TaskListUpdate.CurrentTask(); got == nil || got.Type != types.TaskTypeAnalysis {
		t.Errorf("got current task %+v, want type=analysis", got)
	}

	// Sanity: Data field still set with the raw assistant content
	var data map[string]string
	if err := json.Unmarshal(out.Data, &data); err != nil {
		t.Fatalf("Data not valid JSON: %v", err)
	}
	if data["result"] == "" {
		t.Error("Data.result should contain raw LLM output")
	}
}

// TestAnalyzerParseOutputFailSafe verifies that a non-JSON LLM response
// still produces a TaskListUpdate (the safe default), so the orchestrator
// can route to the analysis policy instead of falling through to a guess.
func TestAnalyzerParseOutputFailSafe(t *testing.T) {
	e := &analyzerEvaluator{}
	msgs := []llm.Message{
		{Role: "assistant", Content: "I'm thinking about this..."},
	}
	out, err := e.ParseOutput(&types.AgentContext{Stage: types.StageAnalyze}, msgs, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.TaskListUpdate == nil {
		t.Fatal("expected fail-safe TaskListUpdate, got nil")
	}
	if out.TaskListUpdate.CurrentTask().Type != types.TaskTypeAnalysis {
		t.Error("fail-safe default should be analysis")
	}
}

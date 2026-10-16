package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/llm"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

type toolSurfaceTransitionLLM247 struct {
	traceTeachingCaptureLLM
	step func(int, []llm.Message, []llm.ToolSchema) (llm.Response, error)
}

func (l *toolSurfaceTransitionLLM247) Chat(_ context.Context, messages []llm.Message, schemas []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	i := l.calls
	l.calls++
	return l.step(i, messages, schemas)
}

// Real Execute, schema filtering, tool admission, message history, resume and
// query execution. Only the model's choices are scripted; no schema rebuilding
// or tool execution is performed by the test between model requests.
func TestHMC247ToolSurfaceTransitionActualExplorerResume(t *testing.T) {
	bus := traceCapabilitiesDiscoveryBus(t, false)
	bus.WorkDir = t.TempDir()
	bus.TraceInputPreparer = traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: t.TempDir()})
	body := "app-42 (42) [000] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=42 next_prio=120\napp-42 (42) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=42 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"
	if err := os.WriteFile(filepath.Join(bus.RepoRoot, "capture.systrace"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	seed, err := (&toolpkg.TraceCatalog{}).Execute(bus, []byte(`{"queries":[{"view":"window_stats","pid":42,"time_start":1,"time_end":1.02}]}`))
	if err != nil || !seed.Success {
		t.Fatal(err, seed.Summary)
	}
	var page struct {
		SavedIndex string `json:"saved_index"`
	}
	if err := json.Unmarshal([]byte(seed.Summary), &page); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(page.SavedIndex)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := filepath.Join(bus.RepoRoot, "checkpoint.json")
	if err := os.WriteFile(checkpoint, data, 0600); err != nil {
		t.Fatal(err)
	}
	bus.Mutable = types.NewMutableState("continue checkpoint")
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	stop := errors.New("captured current query result")
	call := func(name string, args any) (llm.Response, error) {
		raw, e := json.Marshal(args)
		if e != nil {
			t.Fatal(e)
		}
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: name, Name: name, Params: raw}}, StopReason: "tool_use"}, nil
	}
	model := &toolSurfaceTransitionLLM247{}
	model.step = func(i int, messages []llm.Message, schemas []llm.ToolSchema) (llm.Response, error) {
		switch i {
		case 0:
			if hasToolSchema(schemas, "trace_query") {
				t.Fatal("history-less request gained query visibility")
			}
			return call("trace_query", map[string]any{"path": "capture.systrace", "view": "window_stats", "pid": 42, "time_start": 1, "time_end": 1.02})
		case 1:
			results := ctx.Mutable.DispatchToolResults()
			if len(results) != 1 || results[0].Repair == nil || results[0].Repair.Code != unavailableToolSurfaceCode {
				t.Fatalf("missing real unavailable result: %+v", results)
			}
			return call("read_file", map[string]string{"path": checkpoint})
		case 2:
			if hasToolSchema(schemas, "trace_query") {
				t.Fatal("reading historical JSON granted current query visibility")
			}
			results := ctx.Mutable.DispatchToolResults()
			var nav struct {
				NextCall json.RawMessage `json:"next_call"`
			}
			var line string
			for _, candidate := range strings.Split(results[len(results)-1].Summary, "\n") {
				if strings.HasPrefix(candidate, "Checkpoint navigation (not query evidence): ") {
					line = candidate
				}
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "Checkpoint navigation (not query evidence): ")), &nav); err != nil || len(nav.NextCall) == 0 {
				t.Fatalf("real read lost next_call: %v %s", err, line)
			}
			return call("trace_catalog", nav.NextCall)
		case 3:
			if !hasToolSchema(schemas, "trace_query") {
				t.Fatal("real resume did not reveal query in next Execute iteration")
			}
			found, preserved := false, false
			for _, m := range messages {
				found = found || m.Role == "system" && strings.Contains(m.Content, "[current tool surface changed]") && strings.Contains(m.Content, "`trace_query`")
				preserved = preserved || m.Role == "tool" && strings.Contains(m.Content, "not available in the current explore turn")
			}
			if !preserved {
				t.Error("original unavailable result was erased instead of kept for audit")
			}
			if !found {
				t.Error("current schema includes trace_query but stale unavailable turn has no current-surface correction")
			}
			results := ctx.Mutable.DispatchToolResults()
			var current struct {
				Calls map[string]json.RawMessage `json:"query_calls"`
			}
			if err := json.Unmarshal([]byte(results[len(results)-1].Summary), &current); err != nil || len(current.Calls) != 1 {
				t.Fatalf("missing current executable plan: %v", err)
			}
			for _, raw := range current.Calls {
				return call("trace_query", raw)
			}
		case 4:
			for _, m := range messages {
				if strings.Contains(m.Content, "[current tool surface changed]") {
					t.Error("unchanged next iteration retained transient tool-surface correction")
				}
			}
			return llm.Response{}, stop
		}
		return llm.Response{}, errors.New("unexpected model step")
	}
	registry := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(registry)
	_, err = NewExplorerAgent(&Dependencies{LLM: model, Tools: registry, MaxIterations: 5}).Execute(ctx, traceTeachingSkill(t, "explore-skill"))
	if !errors.Is(err, stop) || model.calls != 5 {
		t.Fatalf("real Execute stopped before current query: calls=%d err=%v", model.calls, err)
	}
	catalogs := ctx.Mutable.TraceCatalogs()
	if len(catalogs) != 1 {
		t.Fatal("missing current catalog")
	}
	for _, q := range catalogs[0].Snapshot().Queries {
		if q.Outcome != tracecatalog.OutcomeSuccess || len(q.Attempts) != 1 {
			t.Fatalf("current plan not actually executed: %+v", q)
		}
	}
}

func TestHMC247ToolSurfaceTransitionExactSetsAndStage(t *testing.T) {
	schemas := func(names ...string) []llm.ToolSchema {
		out := make([]llm.ToolSchema, len(names))
		for i, name := range names {
			out[i] = llm.ToolSchema{Name: name}
		}
		return out
	}
	ctx := &types.AgentContext{Stage: types.StageExplore}
	for _, tc := range []struct {
		name              string
		previous, current []llm.ToolSchema
		stage             types.PipelineStage
		want              bool
	}{
		{"added", schemas("read_file"), schemas("read_file", "trace_catalog"), types.StageExplore, true},
		{"removed", schemas("read_file", "trace_query"), schemas("read_file"), types.StageExplore, true},
		{"same_count_replaced", schemas("read_file"), schemas("trace_catalog"), types.StageExplore, true},
		{"unchanged", schemas("read_file"), schemas("read_file"), types.StageExplore, false},
		{"order_and_duplicate_only", schemas("read_file", "trace_catalog"), schemas("trace_catalog", "read_file", "read_file"), types.StageExplore, false},
		{"different_stage", schemas("read_file"), schemas("emit_answer_document"), types.StageFinalize, false},
		{"empty", schemas("read_file"), nil, types.StageExplore, false},
		{"unknown_stage", schemas("read_file"), schemas("trace_catalog"), "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var state toolSurfaceTransition
			if _, ok := state.message(ctx, tc.previous, tc.previous); ok {
				t.Fatal("initial surface should not duplicate skill/schema")
			}
			got, ok := state.message(&types.AgentContext{Stage: tc.stage}, tc.current, tc.current)
			if ok != tc.want {
				t.Fatalf("transition=%t want=%t: %s", ok, tc.want, got.Content)
			}
			if !ok {
				return
			}
			if got.Role != "system" || !strings.Contains(got.Content, "earlier turn only") {
				t.Fatal("missing precise temporal correction", got)
			}
			for _, name := range sortedToolSchemaNames(tc.current) {
				if !strings.Contains(got.Content, "`"+name+"`") {
					t.Fatal("current admitted name omitted", got.Content)
				}
			}
			for _, name := range []string{"trace_query", "exec_command", "apply_patch"} {
				if !hasToolSchema(tc.current, name) && strings.Contains(got.Content, "`"+name+"`") {
					t.Fatal("unavailable capability invented", got.Content)
				}
			}
		})
	}
	var state toolSurfaceTransition
	base, narrow := schemas("read_file", "emit_investigation_complete"), schemas("emit_investigation_complete")
	_, _ = state.message(ctx, base, base)
	got, ok := state.message(ctx, base, narrow)
	want, oldOK := currentTurnToolSurfaceMessage(base, narrow)
	if !ok || !oldOK || got.Role != want.Role || got.Content != want.Content || strings.Contains(got.Content, "[current tool surface changed]") {
		t.Fatal("existing narrow-filter instruction was replaced or duplicated")
	}
}

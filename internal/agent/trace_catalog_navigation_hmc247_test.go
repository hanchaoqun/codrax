package agent

import (
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

func TestHMC247CheckpointActualAgentNavigationAndHistoricalHandoff(t *testing.T) {
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
	var seedPage struct {
		SavedIndex string `json:"saved_index"`
	}
	if err := json.Unmarshal([]byte(seed.Summary), &seedPage); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(seedPage.SavedIndex)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bus.RepoRoot, "checkpoint.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	bus.Mutable = types.NewMutableState("continue the saved capture queries")
	reg := toolpkg.NewRegistry()
	toolpkg.RegisterDefaults(reg)
	base := NewBaseAgent(types.AgentExplorer, &Dependencies{Tools: reg}, &stubEvaluator{})
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentExplorer, types.StageExplore)
	sk := traceTeachingSkill(t, "explore-skill")
	call := func(name string, raw json.RawMessage) *types.ToolResult {
		schemas := base.buildToolSchemas(sk, ctx)
		if !hasToolSchema(schemas, name) {
			t.Fatalf("%s absent from actual current schema", name)
		}
		r, e := base.executeTool(ctx, llm.ToolCall{Name: name, Params: raw}, toolSchemaNameSet(schemas))
		if e != nil || r == nil || !r.Success {
			t.Fatalf("%s: %v %+v", name, e, r)
		}
		return r
	}
	if hasToolSchema(base.buildToolSchemas(sk, ctx), "trace_query") {
		t.Fatal("history-less source gained native query visibility")
	}
	readArgs, _ := json.Marshal(map[string]any{"path": path})
	read := call("read_file", readArgs)
	const prefix = "Checkpoint navigation (not query evidence): "
	var nav struct {
		NextCall json.RawMessage `json:"next_call"`
	}
	var line string
	for _, candidate := range strings.Split(read.Summary, "\n") {
		if strings.HasPrefix(candidate, prefix) {
			line = candidate
		}
	}
	if !strings.HasPrefix(line, prefix) {
		t.Fatal("actual read lost navigation")
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, prefix)), &nav); err != nil {
		t.Fatal(err)
	}
	if len(nav.NextCall) == 0 || len(ctx.Mutable.TraceCatalogs()) != 0 || hasToolSchema(base.buildToolSchemas(sk, ctx), "trace_query") {
		t.Fatal("read restored authority or lost exact action")
	}
	resumed := call("trace_catalog", nav.NextCall)
	var current struct {
		Calls map[string]json.RawMessage `json:"query_calls"`
	}
	if err := json.Unmarshal([]byte(resumed.Summary), &current); err != nil {
		t.Fatal(err)
	}
	if len(current.Calls) != 1 {
		t.Fatal("resume failed to provide exact query calls")
	}
	for _, raw := range current.Calls {
		r := call("trace_query", raw)
		if len(r.Observations) == 0 {
			t.Fatal("query produced no native observations")
		}
	}
	for _, q := range ctx.Mutable.TraceCatalogs()[0].Snapshot().Queries {
		if q.Outcome != tracecatalog.OutcomeSuccess || len(q.Attempts) != 1 {
			t.Fatal("current execution not recorded")
		}
	}
	// Historical qualification must survive the real stage handoff even if
	// the raw read summary is dropped; do not change its source line anchors.
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{*read}})
	for _, stage := range []types.PipelineStage{types.StageExtract, types.StageFinalize} {
		name, skillName := types.AgentExtractor, "extract-skill"
		if stage == types.StageFinalize {
			name, skillName = types.AgentFinalizer, "answer-document-skill"
		}
		stageCtx := ctxbuilder.BuildAgentContext(bus, name, stage)
		capture := &traceTeachingCaptureLLM{stop: errors.New("captured historical navigation")}
		deps := &Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}
		a := NewExtractorAgent(deps)
		if stage == types.StageFinalize {
			a = NewFinalizerAgent(deps)
		}
		_, err := a.Execute(stageCtx, traceTeachingSkill(t, skillName))
		if !errors.Is(err, capture.stop) || capture.calls != 1 {
			t.Fatalf("%s prompt not captured: %v", stage, err)
		}
		var prompt strings.Builder
		for _, m := range capture.messages {
			prompt.WriteString(m.Content)
		}
		if !strings.Contains(prompt.String(), "historical_results_not_current_execution_evidence") {
			t.Fatalf("%s lost historical role", stage)
		}
	}
}

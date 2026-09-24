package agent

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceEventInventoryActualFinalizerPrompt(t *testing.T, ctx *types.AgentContext) string {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Register(&tool.EmitAnswerDocument{})
	capture := &traceTeachingCaptureLLM{stop: errors.New("captured event inventory finalizer request")}
	agent := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1})
	_, err := agent.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("expected exactly one real adapter request: calls=%d err=%v", capture.calls, err)
	}
	var prompt strings.Builder
	for _, message := range capture.messages {
		if message.Role == "user" && strings.Contains(message.Content, "Trace Event Search Inventories") {
			prompt.WriteString(message.Content)
		}
	}
	return prompt.String()
}

func TestTraceEventInventoryActualFinalizerKeepsMainQueryAfterNarrowDrilldowns(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_io_activity/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	start, end := 2.0, 2.24
	ctx := traceEventInventoryPublicContext(nil)
	ctx.RepoRoot, ctx.WorkDir = t.TempDir(), t.TempDir()
	ctx.Stage, ctx.AgentName, ctx.Language = types.StageFinalize, types.AgentFinalizer, "zh"
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "[2,2.24] seconds",
	}
	var results []types.ToolResult
	// The first valid query is the broad inventory. Four later, independently
	// scoped endpoint lookups used to evict it entirely from the finalizer.
	for _, names := range [][]string{
		nil, {"mmc_request_start"}, {"mmc_request_done"}, {"f2fs_sync_file_enter"}, {"f2fs_sync_file_exit"},
	} {
		params := map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": start, "time_end": end, "limit": 40}
		if names != nil {
			params["event_names"] = names
		}
		args, _ := json.Marshal(params)
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
		if err != nil || !result.Success {
			t.Fatalf("native event inventory failed: %v / %s", err, result.Summary)
		}
		results = append(results, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	beforeLedger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal([]any{beforeLedger, types.CompileTraceCausalProjectionSet(beforeLedger), ctx.Mutable.TraceRootCauseReport()})
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 5 {
		t.Fatalf("actual finalizer has %d query identities, want all five", len(views))
	}
	byScope := make(map[string]traceEventInventoryPromptView)
	for _, view := range views {
		byScope[view.Inventory.QueryScopeID] = view
	}
	mainRows := 0
	for _, record := range beforeLedger.Records {
		if !types.IsValidTraceEventSearchInventoryRecord(record) {
			continue
		}
		view, found := byScope[record.SourceRef.QueryScopeID]
		original := record.EventSearchInventory
		if !found || !reflect.DeepEqual(view.Source, record.SourceRef) || !reflect.DeepEqual(view.Inventory.Query, original.Query) ||
			view.ObservedAt != record.ObservedAt || !reflect.DeepEqual(view.ProducerNotes, record.RichNotes) ||
			view.Inventory.Coverage != original.Coverage || !reflect.DeepEqual(view.Inventory.Rows, original.Rows) || !view.Inventory.RowsComplete || view.PromptRowsOmitted != 0 {
			t.Fatalf("actual model request lost source, original/matched window, exact event-name filter or member detail: %+v", view)
		}
		if len(original.Query.EventNames) == 0 {
			mainRows = len(view.Inventory.Rows)
			write, flush := false, false
			for _, row := range view.Inventory.Rows {
				write = write || (row.EventName == "block_rq_issue" && row.TraceTimeSeconds == 2.2 && strings.Contains(row.Raw, " W 16384 "))
				flush = flush || (row.EventName == "block_rq_issue" && row.TraceTimeSeconds == 2.22 && strings.Contains(row.Raw, " FS 0 "))
			}
			if !write || !flush {
				t.Fatal("late RQ write/flush rows disappeared behind eight-row or latest-four-query limits")
			}
		} else if len(view.Inventory.Rows) != 1 || view.Inventory.Rows[0].EventName != original.Query.EventNames[0] {
			t.Fatal("exact event-name filter was widened or rewritten")
		}
	}
	if mainRows != 19 || !strings.Contains(prompt, "prompt_member_rows=23/32") || !strings.Contains(prompt, "not a request population") {
		t.Fatalf("broad inventory count/lookup boundary incorrect: main rows=%d", mainRows)
	}
	afterLedger := answerDocObservationLedger(ctx)
	after, _ := json.Marshal([]any{afterLedger, types.CompileTraceCausalProjectionSet(afterLedger), ctx.Mutable.TraceRootCauseReport()})
	if string(before) != string(after) {
		t.Fatal("actual finalizer handoff mutated source receipts or causal authority")
	}
}

func TestTraceEventInventoryActualFinalizerKeepsWindowAndSourceGuards(t *testing.T) {
	results := traceEventInventoryPublicResults(t, 20, "2")
	ctx := traceEventInventoryPublicContext(results)
	ctx.RepoRoot, ctx.WorkDir = t.TempDir(), t.TempDir()
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	start, end := 6.001, 6.004
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "[6.001,6.004] seconds",
	}
	// Lookup tolerance is not causal selected_window authority. Each lookup
	// remains visible under its OWN request/matching rulers, never borrowing
	// the narrower user request's denominator or causal authority.
	var path string
	for _, record := range answerDocObservationLedger(ctx).Records {
		if types.IsValidTraceEventSearchInventoryRecord(record) {
			path = record.SourceRef.Path
			break
		}
	}
	results = nil
	for _, window := range [][2]float64{{6, 6.01}, {start, end}} {
		args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "event_names": []string{"tracing_mark_write"}, "time_start": window[0], "time_end": window[1], "limit": 20})
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
		if err != nil || !result.Success {
			t.Fatalf("query failed: %v / %s", err, result.Summary)
		}
		results = append(results, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	beforeLedger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal([]any{beforeLedger, types.CompileTraceCausalProjectionSet(beforeLedger)})
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 2 {
		t.Fatalf("source/window protection changed in actual message: %+v", views)
	}
	for n, view := range views {
		var original types.ObservationRecord
		for _, record := range beforeLedger.Records {
			if record.ID == view.ObservationID {
				original = record
				break
			}
		}
		if original.EventSearchInventory == nil || !reflect.DeepEqual(view.Source, original.SourceRef) || !reflect.DeepEqual(view.Inventory, original.EventSearchInventory) ||
			view.ObservedAt != original.ObservedAt || !reflect.DeepEqual(view.ProducerNotes, original.RichNotes) || view.Source.Path != path {
			t.Fatalf("lookup query %d changed its own source/window/generation: %+v", n, view)
		}
		notes := strings.Join(view.ProducerNotes, "\n")
		requested, matching, count := "requested_window=6.000000..6.010000", "matching_window=6.000000..6.010500", 14
		if n == 1 {
			requested, matching, count = "requested_window=6.001000..6.004000", "matching_window=6.000500..6.004500", 8
		}
		if !strings.Contains(notes, requested) || !strings.Contains(notes, matching) || !strings.Contains(notes, "matching_window_policy=lookup_only_boundary_tolerance") ||
			view.Inventory.Coverage.MatchedTotal != count || !view.Inventory.Query.TimeStartSet || !view.Inventory.Query.TimeEndSet {
			t.Fatalf("lookup query %d lost requested/matching distinction or borrowed another count: %+v", n, view)
		}
	}
	afterLedger := answerDocObservationLedger(ctx)
	after, _ := json.Marshal([]any{afterLedger, types.CompileTraceCausalProjectionSet(afterLedger)})
	if string(before) != string(after) {
		t.Fatal("prompt filtering modified accepted source/window or causal projection")
	}
}

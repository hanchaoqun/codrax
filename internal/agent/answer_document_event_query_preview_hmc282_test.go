package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the producer and actual finalizer adapter, not a model-written
// summary. Equal counts and overlapping rows must keep independent rulers.
func TestHMC282EventQueryPreviewPublicIdentityAndEndpoints(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "members.ftrace")
	var data strings.Builder
	for n := 0; n < 60; n++ {
		fmt.Fprintf(&data, "writer-101 (101) [001] .... 6.%03d000: tracing_mark_write: B|201|batch-%d\n", n, n)
		fmt.Fprintf(&data, "writer-101 (101) [001] .... 6.%03d010: tracing_mark_write: E|201\n", n)
	}
	if err := os.WriteFile(path, []byte(data.String()), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := traceEventInventoryPublicContext(nil)
	ctx.RepoRoot, ctx.WorkDir = dir, dir
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	var results []types.ToolResult
	for _, actions := range [][]string{{"B"}, {"E"}, {"B", "E"}} {
		args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "trace_mark_actions": actions, "limit": 60})
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
		if err != nil || !result.Success {
			t.Fatalf("query: %v / %s", err, result.Summary)
		}
		results = append(results, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ledger := answerDocObservationLedger(ctx)
	before, _ := json.Marshal([]any{ledger, types.CompileTraceCausalProjectionSet(ledger)})
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	summaries := hmc282EventQuerySummaries(t, prompt)
	if len(views) != 3 || len(summaries) != 3 {
		t.Fatalf("query-owned summaries missing: views=%d summaries=%d", len(views), len(summaries))
	}
	for _, view := range views {
		var original types.ObservationRecord
		for _, record := range ledger.Records {
			if record.ID == view.ObservationID {
				original = record
			}
		}
		i := original.EventSearchInventory
		if i == nil || !reflect.DeepEqual(view.Source, original.SourceRef) || !reflect.DeepEqual(view.Inventory.Query, i.Query) || !reflect.DeepEqual(view.Inventory.Coverage, i.Coverage) {
			t.Fatalf("source/query ruler changed: %+v", view)
		}
		s := summaries[view.ObservationID]
		if s["native_retained_rows"] != float64(40) || s["profile_rows"] != float64(40) || s["profile_covers_query_matches"] != false || view.Inventory.QueryScopeID != i.QueryScopeID || view.Inventory.Coverage.Emitted != 60 || view.PromptRowsShown != len(view.Inventory.Rows) {
			t.Fatalf("query count conflated with a retained/display population: %+v", s)
		}
		rows := view.Inventory.Rows
		if len(rows) < 3 || rows[0].Line != i.Rows[0].Line || rows[len(rows)-1].Line != i.Rows[len(i.Rows)-1].Line {
			t.Fatalf("bounded preview lost query endpoints: shown=%d first=%d last=%d native_last=%d", len(rows), rows[0].Line, rows[len(rows)-1].Line, i.Rows[len(i.Rows)-1].Line)
		}
		last := -1
		for n, id := range view.SelectorRowIDs {
			var index int
			if _, err := fmt.Sscanf(id, "event-%d", &index); err != nil || index <= last || index > len(i.Rows) || !reflect.DeepEqual(rows[n], i.Rows[index-1]) {
				t.Fatalf("sampling changed order/selector-to-member binding: %v rows=%+v", view.SelectorRowIDs, rows)
			}
			last = index
		}
	}
	afterLedger := answerDocObservationLedger(ctx)
	after, _ := json.Marshal([]any{afterLedger, types.CompileTraceCausalProjectionSet(afterLedger)})
	if string(before) != string(after) {
		t.Fatal("query preview mutated accepted facts/causal projection")
	}
}

func hmc282EventQuerySummaries(t *testing.T, prompt string) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, line := range strings.Split(prompt, "\n") {
		if !strings.HasPrefix(line, "- {") {
			continue
		}
		var value struct {
			ObservationID string         `json:"observation_id"`
			Summary       map[string]any `json:"query_summary"`
		}
		if err := json.Unmarshal([]byte(line[2:]), &value); err != nil {
			t.Fatal(err)
		}
		if value.Summary != nil {
			out[value.ObservationID] = value.Summary
		}
	}
	return out
}

func TestHMC282EventQueryPreviewDoesNotInventEmptyOrPopulation(t *testing.T) {
	base := traceEventInventoryBudgetRecord(t)
	for _, complete := range []bool{false, true} {
		r := traceEventInventoryDistinctRecord(base, 1)
		i := r.EventSearchInventory
		i.Rows = nil
		i.Coverage.MatchedTotal, i.Coverage.Emitted = 0, 0
		i.Coverage.ScopeComplete, i.Coverage.EnumerationComplete = complete, complete
		i.HandoffRowsOmitted, i.RowsComplete = 0, complete
		prompt := renderAnswerDocTraceEventInventories(types.ObservationLedger{Records: []types.ObservationRecord{r}})
		s := hmc282EventQuerySummaries(t, prompt)[r.ID]
		if complete && s != nil {
			t.Fatal("complete zero needs no redundant partial-layer summary")
		}
		if !complete && (s == nil || s["profile_covers_query_matches"] != false || s["profile_rows"] != float64(0)) {
			t.Fatalf("complete=%t lost known-zero vs incomplete observation: %+v", complete, s)
		}
		views := traceEventInventoryPromptViews(t, prompt)
		if len(views) != 1 || !reflect.DeepEqual(views[0].Inventory.Coverage, i.Coverage) || views[0].Inventory.RowsComplete != complete {
			t.Fatal("empty-query original coverage/completeness was changed")
		}
		// A source coordinate or model origin alone must not mint a query digest.
		r.Producer = "model"
		if got := renderAnswerDocTraceEventInventories(types.ObservationLedger{Records: []types.ObservationRecord{r}}); got != "" {
			t.Fatal("model payload minted a producer-owned query summary")
		}
	}
}

func TestHMC282EventQueryPreviewSpreadsSameSchemaWithoutChangingRows(t *testing.T) {
	base := traceEventInventoryBudgetRecord(t)
	var records []types.ObservationRecord
	for n := 0; n < 3; n++ {
		r := traceEventInventoryDistinctRecord(base, n)
		row := r.EventSearchInventory.Rows[0]
		r.EventSearchInventory.Rows = nil
		for p := 0; p < 40; p++ {
			row.Line = p + 1
			row.Raw = fmt.Sprintf("same event schema payload-%d", p)
			r.EventSearchInventory.Rows = append(r.EventSearchInventory.Rows, row)
		}
		records = append(records, r)
	}
	before, _ := json.Marshal(records)
	got := traceEventInventoryMembers(records)
	for n, positions := range got.positions {
		if len(positions) < 3 || positions[0] != 0 || positions[len(positions)-1] != 39 {
			t.Fatalf("query %d kept only a prefix: %v", n, positions)
		}
		middle := false
		for _, p := range positions {
			middle = middle || p == 20
		}
		if !middle {
			t.Fatalf("query %d lacks middle coverage: %v", n, positions)
		}
	}
	after, _ := json.Marshal(records)
	if string(before) != string(after) {
		t.Fatal("budget selection mutated query members")
	}
}

func TestHMC282EventQueryPreviewProfileKeepsTargetWindowAndConflictSeparate(t *testing.T) {
	result := traceInventoryScopePublicQuery(t,
		"writer-101 (101) [000] .... 9.999900: tracing_mark_write: I|101|outside-left\n"+
			"writer-101 (101) [000] .... 10.000000: tracing_mark_write: I|101|inside\n"+
			"writer-202 (101) [000] .... 10.005000: tracing_mark_write: I|101|other-thread\n"+
			"writer-101 (101) [000] .... 10.050000: tracing_mark_write: I|101|right-boundary\n", 9.9999, 10.05, nil)
	ctx := traceInventoryScopeContext(result, 10, 10.05)
	original := traceInventoryScopeReceipt(t, ctx)
	before := traceInventoryScopeAuthority(t, ctx)
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	s := hmc282EventQuerySummaries(t, prompt)[original.ID]
	views := traceEventInventoryPromptViews(t, prompt)
	if s["native_retained_rows"] != float64(4) || s["profile_rows"] != float64(1) || s["profile_covers_query_matches"] != false || len(views) != 1 || views[0].Inventory.Coverage.MatchedTotal != 4 || views[0].Inventory.Rows[0].Line != 2 {
		t.Fatalf("scope-projected profile became a source population: summary=%+v views=%+v", s, views)
	}
	kinds, _ := s["retained_event_kinds"].([]any)
	if len(kinds) != 1 || kinds[0].(map[string]any)["members"] != float64(1) || traceInventoryScopeAuthority(t, ctx) != before {
		t.Fatal("profile counted excluded members or changed source authority")
	}
	conflict := original
	conflict.EventSearchInventory = types.CloneTraceEventSearchInventory(original.EventSearchInventory)
	conflict.EventSearchInventory.Coverage.MatchedTotal++
	conflict.EventSearchInventory.RowsComplete = false
	conflicted := renderAnswerDocTraceEventInventories(types.ObservationLedger{Records: []types.ObservationRecord{original, conflict}})
	if len(hmc282EventQuerySummaries(t, conflicted)) != 0 || !strings.Contains(conflicted, `"query_identity_conflict":true`) {
		t.Fatal("conflicting payloads minted an unambiguous query digest")
	}
}

func TestHMC282EventQueryPreviewDigestBudgetYieldsToExactReceipt(t *testing.T) {
	r := traceEventInventoryBudgetRecord(t)
	r.RichNotes = []string{"requested_time_window=6..6.1s", "query-owned producer note"}
	i := r.EventSearchInventory
	row := i.Rows[0]
	i.Rows = nil
	for n := 0; n < 40; n++ {
		row.Line = n + 1
		row.EventName = fmt.Sprintf("kind-%d-%s", n, strings.Repeat("long-original-event-name", 500))
		i.Rows = append(i.Rows, row)
	}
	i.Coverage.MatchedTotal, i.Coverage.Emitted = 40, 40
	i.HandoffRowsOmitted, i.RowsComplete = 0, true
	if !types.IsValidTraceEventSearchInventoryRecord(r) {
		t.Fatal("test must reach the accepted typed inventory contract")
	}
	before, _ := json.Marshal(r)
	prompt := renderAnswerDocTraceEventInventories(types.ObservationLedger{Records: []types.ObservationRecord{r}})
	if len(hmc282EventQuerySummaries(t, prompt)) != 0 || len(prompt) > traceEventInventoryPromptByteLimit {
		t.Fatal("oversized redundant digest displaced the source receipt budget")
	}
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 1 || !reflect.DeepEqual(views[0].Source, r.SourceRef) || !reflect.DeepEqual(views[0].Inventory.Query, i.Query) || !reflect.DeepEqual(views[0].Inventory.Coverage, i.Coverage) || !reflect.DeepEqual(views[0].ProducerNotes, r.RichNotes) {
		t.Fatal("digest budget clipped original source, query, coverage or producer notes")
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		t.Fatal("digest omission mutated the accepted receipt")
	}
}

func TestHMC282EventQueryPreviewCompleteProfileIsNotCompleteDisplay(t *testing.T) {
	r := traceEventInventoryBudgetRecord(t)
	r.RichNotes = nil
	i := r.EventSearchInventory
	row := i.Rows[0]
	i.Rows = nil
	for n := 0; n < 40; n++ {
		row.Line, row.EventName = n+1, "same-kind"
		if n >= 30 {
			row.EventName = "other-kind"
		}
		i.Rows = append(i.Rows, row)
	}
	i.Coverage.MatchedTotal, i.Coverage.Emitted = 40, 40
	i.HandoffRowsOmitted, i.RowsComplete = 0, true
	prompt := renderAnswerDocTraceEventInventories(types.ObservationLedger{Records: []types.ObservationRecord{r}})
	s := hmc282EventQuerySummaries(t, prompt)[r.ID]
	views := traceEventInventoryPromptViews(t, prompt)
	if s["profile_covers_query_matches"] != true || s["profile_rows"] != float64(40) || len(views) != 1 || views[0].Inventory.RowsComplete || len(views[0].Inventory.Rows) != 32 || views[0].Inventory.Coverage.MatchedTotal != 40 {
		t.Fatalf("complete retained population was confused with finite display: %+v", s)
	}
	kinds, _ := s["retained_event_kinds"].([]any)
	if len(kinds) != 2 || kinds[0].(map[string]any)["members"] != float64(30) || kinds[1].(map[string]any)["members"] != float64(10) {
		t.Fatalf("query-local retained distribution changed: %+v", kinds)
	}
}

// Opt-in deterministic replay of an already captured artifact. This does not
// call a model, depend on a developer path in ordinary tests, or mutate the
// captured artifact. The finalizer adapter is stopped at its first LLM call.
func TestHMC282EventQueryPreviewReplayCapturedSource(t *testing.T) {
	path := os.Getenv("CODRAX_EVENT_PREVIEW_REPLAY_SOURCE")
	if path == "" {
		t.Skip("set CODRAX_EVENT_PREVIEW_REPLAY_SOURCE for the archived HMC281 A source")
	}
	ctx := traceEventInventoryPublicContext(nil)
	ctx.RepoRoot, ctx.WorkDir = t.TempDir(), t.TempDir()
	ctx.Stage, ctx.AgentName = types.StageFinalize, types.AgentFinalizer
	queries := []map[string]any{
		{"patterns": []string{"clock_set_rate", "clock_events", "trace_clock", "sys_time", "clock_gettime"}, "limit": 60},
		{"patterns": []string{"realtime", "wall", "utc", "1970", "epoch", "ntp"}, "limit": 20},
		{"pattern": "trace_event_clock_sync", "limit": 10},
		{"event_types": []string{"clock_set_rate"}, "limit": 1},
	}
	var results []types.ToolResult
	for _, query := range queries {
		query["source"], query["path"], query["view"] = "path", path, "event_search"
		args, _ := json.Marshal(query)
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
		if err != nil || !result.Success {
			t.Fatalf("replay query: %v / %s", err, result.Summary)
		}
		results = append(results, result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	prompt := traceEventInventoryActualFinalizerPrompt(t, ctx)
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 4 {
		t.Fatalf("independent query roster=%d, want 4", len(views))
	}
	for _, view := range views {
		i := view.Inventory
		t.Logf("query=%s patterns=%v pattern=%s event_types=%v matched=%d emitted=%d shown=%d complete=%t selectors=%v", i.QueryScopeID, i.Query.Patterns, i.Query.Pattern, i.Query.EventTypes, i.Coverage.MatchedTotal, i.Coverage.Emitted, len(i.Rows), i.RowsComplete, view.SelectorRowIDs)
		if i.Query.Pattern == "trace_event_clock_sync" {
			if i.Coverage.MatchedTotal != 2 || len(i.Rows) != 2 || !i.RowsComplete {
				t.Fatal("two clock witnesses lost their independent complete query")
			}
			continue
		}
		if i.Query.Limit != 60 {
			continue
		}
		thermal, first, last := false, false, false
		for _, row := range i.Rows {
			thermal = thermal || row.Line == 1830
			first, last = first || row.Line == 16, last || row.Line == 3088
			t.Logf("broad member line=%d raw=%s", row.Line, row.Raw)
		}
		if i.Coverage.MatchedTotal != 7134 || i.Coverage.Emitted != 60 || i.RowsComplete || !thermal || !first || !last {
			t.Fatal("old broad-query preview still hides the retained counterexample or rewrites its population")
		}
	}
	if out := os.Getenv("CODRAX_EVENT_PREVIEW_REPLAY_OUTPUT"); out != "" {
		if err := os.WriteFile(out, []byte(prompt), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

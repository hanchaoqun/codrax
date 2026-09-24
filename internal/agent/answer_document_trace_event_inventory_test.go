package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func traceEventInventoryPublicResults(t *testing.T, limit int, minimum string) []types.ToolResult {
	t.Helper()
	dir := t.TempDir()
	var source strings.Builder
	for i, item := range []struct{ frames, app, prefix string }{
		{"1", "830", ""}, {"2", "830", ""}, {"8", "830", ""},
		{"4", "830", ""}, {"7", "831", ""}, {"invalid", "830", ""}, {"99", "830", "other "},
	} {
		fmt.Fprintf(&source, "writer-101 (101) [001] .... 6.%03d000: tracing_mark_write: B|201|%sjank_event_sync: start_ts=%d, end_ts=%d, jank_frames=%s, appid=%s\n", i, item.prefix, int64(9007199254740993)+int64(i)*100000000, int64(9007199274740993)+int64(i)*100000000, item.frames, item.app)
		fmt.Fprintf(&source, "writer-101 (101) [001] .... 6.%03d010: tracing_mark_write: E|201\n", i)
	}
	path := filepath.Join(dir, "events.ftrace")
	if err := os.WriteFile(path, []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	var results []types.ToolResult
	for _, filtered := range []bool{false, true} {
		params := map[string]any{"source": "path", "path": path, "view": "event_search", "pattern": "jank_event_sync", "limit": limit}
		if filtered {
			params["event_field_filters"] = []map[string]string{{"field": "appid", "op": "eq", "value": "830"}, {"field": "jank_frames", "op": "gte", "value": minimum}}
		}
		data, _ := json.Marshal(params)
		result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, data)
		if err != nil || !result.Success {
			t.Fatalf("event query failed: %v %s", err, result.Summary)
		}
		results = append(results, result)
	}
	return results
}

func traceEventInventoryPublicContext(results []types.ToolResult) *types.AgentContext {
	ctx := tracePrincipalValueAuthorityTestContext("", 0, nil)
	ctx.AnalysisIR.RequestModel.RuntimeTargets = nil
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	return ctx
}

func TestTraceEventInventoryPublicQuerySurvivesFinalizerHandoff(t *testing.T) {
	ctx := traceEventInventoryPublicContext(traceEventInventoryPublicResults(t, 20, "2"))
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	for name, prompt := range map[string]string{
		"ledger":  renderAnswerDocObservationLedger(ctx),
		"initial": (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil),
	} {
		for _, want := range []string{"Trace Event Search Inventories", `"matched_total":7`, `"matched_total":3`, `"event_field_filters":`, `"field":"jank_frames"`, `"value":"2"`, "9007199354740993", "9007199474740993", "jank_event_fields_invalid", `"time_domain_status":"source_trace_clock"`} {
			if !strings.Contains(prompt, want) {
				t.Errorf("%s handoff lost %q", name, want)
			}
		}
		views := traceEventInventoryPromptViews(t, prompt)
		if len(views) != 2 {
			t.Fatalf("%s query receipts=%d, want separate broad and filtered", name, len(views))
		}
		for _, view := range views {
			i := view.Inventory
			if i.QueryScopeID != view.Source.QueryScopeID || i.QueryScopeID == "" {
				t.Fatal("lost query-bound identity")
			}
			if len(i.Query.EventFieldFilters) == 0 {
				if i.Coverage.MatchedTotal != 7 {
					t.Fatal("narrow count substituted for broad count")
				}
				continue
			}
			if i.Coverage.MatchedTotal != 3 || len(i.Rows) != 3 || !i.RowsComplete {
				t.Fatalf("filtered receipt inconsistent: %+v", i)
			}
			for _, row := range i.Rows {
				if row.JankEvent == nil || row.JankEvent.Values == nil || row.JankEvent.Values.AppID != 830 || row.JankEvent.Values.JankFrames < 2 || row.EmitterTID != 101 || row.MarkerPID != 201 {
					t.Fatalf("filtered membership/identity corrupted: %+v", row)
				}
			}
		}
		if !strings.Contains(prompt, skill.TraceJankClockContract) {
			t.Fatal("clock guidance drifted from exploration")
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("answer-writing view mutated the accepted ledger")
	}
}

type traceEventInventoryPromptView struct {
	ObservationID     string                           `json:"observation_id"`
	ObservedAt        string                           `json:"observed_at"`
	Source            types.ObservationSourceRef       `json:"source"`
	Inventory         *types.TraceEventSearchInventory `json:"inventory"`
	ProducerNotes     []string                         `json:"producer_notes,omitempty"`
	PromptRowsShown   int                              `json:"prompt_rows_shown"`
	PromptRowsOmitted int                              `json:"prompt_rows_omitted"`
}

func traceEventInventoryPromptViews(t *testing.T, prompt string) []traceEventInventoryPromptView {
	t.Helper()
	var views []traceEventInventoryPromptView
	for _, line := range strings.Split(prompt, "\n") {
		if !strings.HasPrefix(line, `- {`) {
			continue
		}
		var view traceEventInventoryPromptView
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "- ")), &view); err != nil {
			t.Fatal(err)
		}
		if view.Inventory == nil {
			continue
		}
		views = append(views, view)
	}
	return views
}

func TestTraceEventInventoryPromptBudgetDoesNotChangeCoverageOrAuthority(t *testing.T) {
	results := traceEventInventoryPublicResults(t, 20, "2")
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: results})
	var base types.ObservationRecord
	for _, r := range ledger.Records {
		if types.IsValidTraceEventSearchInventoryRecord(r) && len(r.EventSearchInventory.Query.EventFieldFilters) > 0 {
			base = r
		}
	}
	if base.EventSearchInventory == nil {
		t.Fatal("public tool did not publish a qualified inventory")
	}
	var all types.ObservationLedger
	for n := 0; n < 6; n++ {
		r := base
		r.ID = fmt.Sprintf("query-%d", n)
		r.EventSearchInventory = types.CloneTraceEventSearchInventory(base.EventSearchInventory)
		i := r.EventSearchInventory
		r.SourceRef.QueryScopeID = fmt.Sprintf("query-%d", n)
		i.QueryScopeID = r.SourceRef.QueryScopeID
		row := i.Rows[0]
		row.Raw = strings.Repeat("证", 400)
		i.Rows = nil
		for m := 0; m < 12; m++ {
			row.Line = m + 1
			i.Rows = append(i.Rows, row)
		}
		i.Coverage.MatchedTotal, i.Coverage.Emitted = 12, 12
		i.Coverage.ScopeTimestampRows = 24
		i.RowsComplete = true
		all.Records = append(all.Records, r)
	}
	before, _ := json.Marshal([]any{all, types.CompileTraceCausalProjectionSet(all)})
	prompt := renderAnswerDocTraceEventInventories(all)
	if strings.Contains(prompt, "query_receipts_omitted=") {
		t.Fatal("six query summaries should fit without discarding scopes")
	}
	views := traceEventInventoryPromptViews(t, prompt)
	if len(views) != 6 {
		t.Fatalf("views=%d", len(views))
	}
	for n, view := range views {
		i := view.Inventory
		wantRows := 5
		if n < 2 {
			wantRows = 6
		}
		if i.Coverage.MatchedTotal != 12 || i.Coverage.Emitted != 12 || !i.Coverage.ScopeComplete || !i.Coverage.EnumerationComplete || i.RowsComplete || view.PromptRowsShown != wantRows || view.PromptRowsOmitted != 12-wantRows || i.HandoffRowsOmitted != 12-wantRows {
			t.Fatalf("budget/count contradiction: %+v", view)
		}
		for _, row := range i.Rows {
			if len(row.Raw) > 512 || !utf8.ValidString(row.Raw) || !row.RawTruncated || row.JankEvent.Values.StartTSNS != 9007199354740993 {
				t.Fatal("raw display clipping damaged exact fields or UTF-8")
			}
		}
	}
	after, _ := json.Marshal([]any{all, types.CompileTraceCausalProjectionSet(all)})
	if string(before) != string(after) {
		t.Fatal("inventory view changed source/causal authority")
	}
	// No special clock teaching on a genuinely generic typed event receipt;
	// neither a raw phrase nor a user/model keyword activates it.
	generic := base
	generic.EventSearchInventory = types.CloneTraceEventSearchInventory(base.EventSearchInventory)
	generic.EventSearchInventory.Query.EventFieldFilters = nil
	for n := range generic.EventSearchInventory.Rows {
		generic.EventSearchInventory.Rows[n].JankEvent = nil
	}
	got := renderAnswerDocTraceEventInventories(types.ObservationLedger{Records: []types.ObservationRecord{generic}})
	if got == "" || strings.Contains(got, skill.TraceJankClockContract) {
		t.Fatal("generic inventory received irrelevant jank teaching or disappeared")
	}
}

func TestTraceEventInventoryPublicZeroAndLimitedCountsSurvive(t *testing.T) {
	for _, tc := range []struct {
		name, minimum, total string
		limit                int
	}{
		{"zero", "100", `"matched_total":0`, 20},
		{"limited", "2", `"matched_total":3`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := traceEventInventoryPublicContext(traceEventInventoryPublicResults(t, tc.limit, tc.minimum))
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			for _, want := range []string{"Trace Event Search Inventories", tc.total, `"event_field_filters":`, "jank_event_fields_invalid"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("lost exact zero/limited query scope: %q", want)
				}
			}
		})
	}
}

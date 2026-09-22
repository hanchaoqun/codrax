package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryEventSearchInventoryPublicBroadAndFiltered(t *testing.T) {
	ctx := eventSearchInventoryPublicContext(t)
	query := func(extra string) types.ToolResult {
		r, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"source":"path","path":"events.systrace","view":"event_search","pattern":"jank_event_sync"`+extra+`}`))
		if err != nil || !r.Success {
			t.Fatalf("query failed: %v %+v", err, r)
		}
		return r
	}
	broad := query("")
	filtered := query(`,"event_field_filters":[{"field":"appid","op":"eq","value":"620"},{"field":"jank_frames","op":"gte","value":"2"}]`)
	a := requireEventSearchInventory(t, broad)
	b := requireEventSearchInventory(t, filtered)
	if a.SourceRef.QueryScopeID == b.SourceRef.QueryScopeID || a.ID == b.ID {
		t.Fatal("different queries coalesced")
	}
	if a.EventSearchInventory.Coverage.MatchedTotal != 7 || b.EventSearchInventory.Coverage.MatchedTotal != 3 {
		t.Fatalf("wrong independent totals: %+v %+v", a, b)
	}
	if !b.EventSearchInventory.RowsComplete || len(b.EventSearchInventory.Rows) != 3 {
		t.Fatalf("incomplete exact result: %+v", b)
	}
	if len(b.EventSearchInventory.Query.EventFieldFilters) != 2 {
		t.Fatal("lost numeric filters")
	}
	for n, want := range []int64{2, 7, 4} {
		row := b.EventSearchInventory.Rows[n]
		if row.JankEvent == nil || row.JankEvent.Values == nil || row.JankEvent.Values.JankFrames != want || row.JankEvent.TimeDomainStatus != "source_trace_clock" || row.EmitterTID != 101 || row.MarkerPID != 201 || row.JankEvent.Values.AppID != 620 {
			t.Fatalf("row identity/value mismatch: %+v", row)
		}
	}
	wire, err := json.Marshal(b)
	if err != nil || !strings.Contains(string(wire), `"start_ts_ns":"9007199254740993"`) {
		t.Fatalf("native int64 must use exact JSON string: %v %s", err, wire)
	}
	if !strings.Contains(strings.Join(b.EventSearchInventory.Caveats, "\n"), "jank_event_fields_invalid=true rows=1") {
		t.Fatal("lost verbatim invalid-field disclosure")
	}
	if j := a.EventSearchInventory.Rows[6].JankEvent; j == nil || j.Values != nil || j.IssueReason == "" {
		t.Fatal("malformed field became numeric zero")
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: []types.ToolResult{broad, filtered}})
	n := 0
	for _, r := range ledger.Records {
		if types.IsValidTraceEventSearchInventoryRecord(r) {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("ledger lost independent query receipts: %d", n)
	}
}

func TestTraceQueryEventSearchInventoryPublicZeroAndDisplayCap(t *testing.T) {
	ctx := eventSearchInventoryPublicContext(t)
	var zeroQueryIDs []string
	for _, tc := range []struct {
		value                 string
		limit, total, emitted int
		complete              bool
	}{{"2", 1, 3, 1, false}, {"100", 1, 0, 0, true}, {"101", 1, 0, 0, true}} {
		params, _ := json.Marshal(map[string]any{"source": "path", "path": "events.systrace", "view": "event_search", "limit": tc.limit, "event_field_filters": []map[string]string{{"field": "appid", "op": "eq", "value": "620"}, {"field": "jank_frames", "op": "gte", "value": tc.value}}})
		r, err := (&TraceQuery{}).Execute(ctx, params)
		if err != nil || !r.Success {
			t.Fatalf("query failed: %v %+v", err, r)
		}
		inv := requireEventSearchInventory(t, r).EventSearchInventory
		if inv.Coverage.MatchedTotal != tc.total || inv.Coverage.Emitted != tc.emitted || inv.RowsComplete != tc.complete || !inv.Coverage.ScopeComplete || !inv.Coverage.EnumerationComplete {
			t.Fatalf("count/display boundary lost: %+v", inv)
		}
		if tc.total == 0 {
			zeroQueryIDs = append(zeroQueryIDs, inv.QueryScopeID)
		}
	}
	if len(zeroQueryIDs) != 2 || zeroQueryIDs[0] == zeroQueryIDs[1] {
		t.Fatal("identical zero payloads merged different numeric filters")
	}
}

func TestTraceQueryEventSearchInventoryPublicHandoffCapAndLineScope(t *testing.T) {
	dir := t.TempDir()
	var source strings.Builder
	for i := 0; i < 55; i++ {
		fmt.Fprintf(&source, "writer-101 (101) [001] .... 5.%06d: print: B|201|jank_event_sync: start_ts=9007199254740993, end_ts=9007199254740995, jank_frames=2, appid=620, extra=%s\n", i, strings.Repeat("x", 4300))
	}
	if err := os.WriteFile(filepath.Join(dir, "events.systrace"), []byte(source.String()), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	r, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"path":"events.systrace","view":"event_search","event_field_filters":[{"field":"jank_frames","op":"gte","value":"2"}],"limit":80}`))
	if err != nil || !r.Success {
		t.Fatalf("query: %v %+v", err, r)
	}
	i := requireEventSearchInventory(t, r).EventSearchInventory
	if i.Coverage.MatchedTotal != 55 || i.Coverage.Emitted != 55 || len(i.Rows) != 40 || i.HandoffRowsOmitted != 15 || i.RowsComplete {
		t.Fatalf("handoff cap rewrote census: %+v", i.Coverage)
	}
	if !i.Rows[0].RawTruncated || len(i.Rows[0].Raw) > types.TraceEventSearchInventoryRawLimit || i.Rows[0].JankEvent.Values.StartTSNS != 9007199254740993 {
		t.Fatal("raw cap lost exact typed field or missing truncation disclosure")
	}
	r, err = (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"path":"events.systrace","view":"event_search","line_start":2,"line_end":3,"event_field_filters":[{"field":"jank_frames","op":"gte","value":"2"}]}`))
	if err != nil || !r.Success {
		t.Fatalf("scoped query: %v %+v", err, r)
	}
	i = requireEventSearchInventory(t, r).EventSearchInventory
	if i.Query.LineStart != 2 || i.Query.LineEnd != 3 || i.Coverage.ScopeKind == "artifact" || i.Coverage.MatchedTotal != 2 {
		t.Fatalf("line-scoped count promoted to whole artifact: %+v", i)
	}
}

func TestTraceQueryEventSearchInventoryProducerRefusesUnboundAndPreservesIncomplete(t *testing.T) {
	ctx := eventSearchInventoryPublicContext(t)
	idx, err := tracequery.BuildIndex(context.Background(), filepath.Join(ctx.RepoRoot, "events.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	q := tracequery.Query{View: "event_search", Limit: 1, EventFieldFilters: []tracequery.EventFieldFilter{{Field: "jank_frames", Op: "gte", Value: "2"}}}
	result := tracequery.Run(idx, q)
	ref := traceQueryObservationSourceRef(result, "path", "/result.json", "/result.json")
	ref.QueryScopeID = traceQueryPublicationScope(result, "/result.json", "/result.json", "", q)
	if got := traceQueryEventSearchInventoryObservation(result, ref, time.Now().Format(time.RFC3339), nil); len(got) != 0 {
		t.Fatal("legacy query-less call minted receipt")
	}
	result.EventSearchCoverage.ScopeComplete = false
	result.EventSearchCoverage.EnumerationComplete = false
	got := traceQueryEventSearchInventoryObservation(result, ref, time.Now().Format(time.RFC3339), []tracequery.Query{q})
	if len(got) != 1 || got[0].EventSearchInventory.RowsComplete || got[0].EventSearchInventory.Coverage.ScopeComplete {
		t.Fatalf("partial census lost its incomplete boundary: %+v", got)
	}
	// A raw reread failure cannot turn FieldText's bounded parsed fragment
	// into a claimed complete physical source line. Canonical and source
	// clocks are separately owned even when an affine bundle differs.
	result.Events[0].Raw = ""
	result.Events[0].RawUnavailableReason = "source_identity_changed"
	result.Events[0].SourcePath = "/physical.trace"
	result.Events[0].LocalLine = 1
	result.Events[0].Ts = 100
	result.Events[0].SourceTs = 5
	got = traceQueryEventSearchInventoryObservation(result, ref, time.Now().Format(time.RFC3339), []tracequery.Query{q})
	if len(got) != 1 {
		t.Fatal("missing raw incorrectly withheld typed facts")
	}
	row := got[0].EventSearchInventory.Rows[0]
	if row.Raw != "" || row.RawUnavailableReason != "source_identity_changed" || row.RawTruncated || row.TraceTimeSeconds != 100 || row.SourceTimeSeconds != 5 || !row.SourceTimeKnown {
		t.Fatalf("raw/source clock fabricated: %+v", row)
	}
	result.Events[0].RawUnavailableReason = "clock_inverse_unsafe"
	result.Events[0].ClockAligned = false
	result.Events[0].SourceTs = result.Events[0].Ts
	got = traceQueryEventSearchInventoryObservation(result, ref, time.Now().Format(time.RFC3339), []tracequery.Query{q})
	if len(got) != 1 || got[0].EventSearchInventory.Rows[0].SourceTimeKnown || got[0].EventSearchInventory.Rows[0].SourceTimeSeconds != 0 {
		t.Fatal("unsafe inverse minted source header clock from canonical fallback")
	}
	result.View = "window_stats"
	if got := traceQueryEventSearchInventoryObservation(result, ref, time.Now().Format(time.RFC3339), []tracequery.Query{q}); len(got) != 0 {
		t.Fatal("non-event view minted event inventory")
	}
}

func requireEventSearchInventory(t *testing.T, result types.ToolResult) types.ObservationRecord {
	t.Helper()
	var found []types.ObservationRecord
	for _, r := range result.Observations {
		if r.EventSearchInventory != nil {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		t.Fatalf("public event_search must publish one query-bound typed inventory; got %d in %d observations", len(found), len(result.Observations))
	}
	if !types.IsValidTraceEventSearchInventoryRecord(found[0]) {
		t.Fatalf("invalid receipt: %+v", found[0])
	}
	return found[0]
}

func eventSearchInventoryPublicContext(t *testing.T) *types.BusContext {
	t.Helper()
	dir := t.TempDir()
	text := strings.Join([]string{
		`writer-101 (101) [001] .... 5.010000: print: B|201|jank_event_sync: start_ts=9007199254740993, end_ts=9007199274740993, jank_frames=2, appid=620`,
		`writer-101 (101) [001] .... 5.020000: print: B|201|jank_event_sync: start_ts=9007199254840993, end_ts=9007199264840993, jank_frames=1, appid=620`,
		`writer-101 (101) [001] .... 5.030000: print: B|201|jank_event_sync: start_ts=9007199254940993, end_ts=9007199344940993, jank_frames=9, appid=621`,
		`writer-101 (101) [001] .... 5.040000: print: B|201|jank_event_sync: start_ts=9007199255740993, end_ts=9007199325740993, jank_frames=7, appid=620`,
		`writer-101 (101) [001] .... 5.050000: print: B|201|jank_event_sync: start_ts=9007199256740993, end_ts=9007199296740993, jank_frames=4, appid=620`,
		`writer-101 (101) [001] .... 5.060000: print: B|201|other jank_event_sync: start_ts=9007199257740993, end_ts=9007199347740993, jank_frames=99, appid=620`,
		`writer-101 (101) [001] .... 5.070000: print: B|201|jank_event_sync: start_ts=9007199258740993, end_ts=9007199268740993, jank_frames=invalid, appid=620`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "events.systrace"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return &types.BusContext{RepoRoot: dir, WorkDir: dir}
}

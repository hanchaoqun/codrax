package tool

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeStackPublicQuery(t *testing.T, path string, args map[string]any) (types.ToolResult, types.ObservationRecord, tracequery.ResourceStackResult) {
	t.Helper()
	ctx, _, _ := hmc17NamedPathContext(t)
	args["source"], args["path"], args["view"] = "path", path, "resource_stack"
	raw, _ := json.Marshal(args)
	r, err := (&TraceQuery{}).Execute(ctx, raw)
	if err != nil || !r.Success {
		t.Fatalf("query %v %s", err, r.Summary)
	}
	for _, record := range r.Observations {
		if p, ok := DecodeTraceResourceStack(record); ok {
			return r, record, p
		}
	}
	t.Fatalf("typed resource stack absent: %s", r.Summary)
	return types.ToolResult{}, types.ObservationRecord{}, tracequery.ResourceStackResult{}
}

func TestNativeResourceStackPublicDefaultPrepareSelectorsAndTampering(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	before, _ := os.ReadFile(path)
	r, record, p := nativeStackPublicQuery(t, path, map[string]any{"pid": 101, "time_start": 10, "time_end": 10.05})
	if p.MatchedEvents != 3 || len(p.Events) != 3 || p.Events[1].UnknownSymbols != 1 || p.Events[2].DuplicateDepths != 1 {
		t.Fatalf("source quality not public %+v", p)
	}
	for _, want := range []string{"ImageCache::reserve", "UIFrame::render", "0xffffffffffffffff", "源事件物理rowid=1", "原始id=1", "pid=101", "CPU未知", "缺深度数=1", "重复深度数=1"} {
		if !strings.Contains(r.Summary, want) {
			t.Errorf("missing %q: %s", want, r.Summary)
		}
	}
	for _, mutate := range []func(*types.ObservationRecord){func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" }, func(r *types.ObservationRecord) { r.SourceRef.QueryTargetScope = "process" }, func(r *types.ObservationRecord) { r.SourceRef.QueryTargetPID = 202 }, func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs = 11 }, func(r *types.ObservationRecord) { r.Negative = true }, func(r *types.ObservationRecord) {
		r.RichNotes = append(append([]string(nil), r.RichNotes...), r.RichNotes[0])
	}} {
		copy := record
		mutate(&copy)
		if _, ok := DecodeTraceResourceStack(copy); ok {
			t.Fatal("tampered handoff admitted")
		}
	}
	_, _, named := nativeStackPublicQuery(t, path, map[string]any{"thread": "gallery-main", "time_start": 10, "time_end": 10.05})
	if named.MatchedEvents != 3 {
		t.Fatal("name-only owner failed")
	}
	_, _, other := nativeStackPublicQuery(t, path, map[string]any{"pid": 202, "time_start": 10, "time_end": 10.05})
	if other.MatchedEvents != 1 || other.Events[0].Source.PID != 202 {
		t.Fatal("foreign owner mixed")
	}
	_, _, all := nativeStackPublicQuery(t, path, map[string]any{"pid": 101})
	if all.MatchedEvents != 4 || !all.Window.EndInclusive {
		t.Fatalf("default last event lost %+v", all)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("source SQLite mutated")
	}
}

func TestNativeResourceStackPublicSignedRowsAndPointWindow(t *testing.T) {
	src, _ := os.ReadFile("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	path := filepath.Join(t.TempDir(), "native.db")
	if err := os.WriteFile(path, src, 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("DELETE FROM native_hook WHERE id<>1; UPDATE native_hook SET rowid=-9223372036854775808; UPDATE native_hook_frame SET rowid=0 WHERE id=1; UPDATE native_hook_frame SET rowid=9223372036854775807 WHERE id=2")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	_, _, p := nativeStackPublicQuery(t, path, map[string]any{"pid": 101})
	if p.MatchedEvents != 1 || p.Events[0].RowID != -9223372036854775808 || p.Events[0].Frames[0].RowID != 0 || p.Events[0].Frames[1].RowID != 9223372036854775807 {
		t.Fatalf("physical identity narrowed %+v", p)
	}
	// The source event timestamp is the only observed timestamp in this text.
	e := p.Events[0]
	wire := tracewire.ResourceStackRecord{TimestampNS: e.TimestampNS, EventRowID: e.RowID, Event: &e.Source}
	body, ok := tracewire.FormatResourceStack(wire)
	if !ok {
		t.Fatal("bad point fixture")
	}
	body += "\n"
	for _, f := range e.Frames {
		wire.Event = nil
		wire.Frame = &f.ResourceFrame
		line, ok := tracewire.FormatResourceStack(wire)
		if !ok {
			t.Fatal("bad point frame")
		}
		body += line + "\n"
	}
	textPath := filepath.Join(t.TempDir(), "point.systrace")
	if err := os.WriteFile(textPath, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, point := nativeStackPublicQuery(t, textPath, map[string]any{"pid": 101})
	if point.MatchedEvents != 1 || point.Window.EndTs != point.Window.StartTs || !point.Window.EndInclusive {
		t.Fatalf("bad source point envelope %+v", point)
	}
}

func TestNativeResourceStackHandoffByteBudgetKeepsWholeSymbolsAndCensus(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	_, record, p := nativeStackPublicQuery(t, path, map[string]any{"pid": 101, "time_start": 10, "time_end": 10.05})
	for i := range p.Events {
		for j := range p.Events[i].Frames {
			p.Events[i].Frames[j].Symbol.Value = strings.Repeat("\n", 4096)
			p.Events[i].Frames[j].Symbol.Status = "known"
		}
		p.Events[i].UnknownSymbols = p.Events[i].Source.FrameCount
	}
	if !tracequery.ValidResourceStack(p) {
		t.Fatal("bad budget test input")
	}
	before, _ := json.Marshal(p)
	records := traceQueryResourceStackObservations(&p, record.SourceRef, "scope", "now")
	if len(records) != 1 {
		t.Fatal("lost bounded handoff")
	}
	bounded, ok := DecodeTraceResourceStack(records[0])
	if !ok {
		t.Fatal("invalid bounded handoff")
	}
	data, _ := json.Marshal(bounded)
	if len(data) > resourceStackHandoffByteBudget || len(TraceResourceStackText(bounded, 8)) > resourceStackHandoffByteBudget {
		t.Fatal("byte cap exceeded")
	}
	if bounded.MatchedEvents != 3 {
		t.Fatal("source census changed")
	}
	summary := traceQuerySummary(tracequery.Result{View: tracequery.ViewResourceStack, ResourceStack: &p}, traceQueryParams{View: tracequery.ViewResourceStack}, "fixture", "payload")
	if !strings.Contains(summary, TraceResourceStackText(bounded, 8)) || len(summary) > resourceStackHandoffByteBudget+8192 {
		t.Fatal("tool preview bypassed whole-frame byte budget")
	}
	omitted := bounded.OmittedEvents
	for _, e := range bounded.Events {
		omitted += e.OmittedFrames
		for _, f := range e.Frames {
			if f.Symbol.Value != strings.Repeat("\n", 4096) {
				t.Fatal("symbol truncated")
			}
		}
	}
	if omitted == 0 {
		t.Fatal("test did not exercise byte omissions")
	}
	after, _ := json.Marshal(p)
	if !bytes.Equal(before, after) {
		t.Fatal("budget mutated native payload")
	}
}

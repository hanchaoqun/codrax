package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// WINFLAG-1 pins, tool half (§29.190④, 2026-07-21): (a) the selected_window
// note producer lifts its 0-start suppression ONLY on a flagged real window;
// (b) the chain-path / window_stats / state-account Spans publish an absent
// ts pair on the line-anchored unset form. Compat arm: unflagged windows keep
// every legacy byte.

// TestWinflagProductionFullChainRebasedZeroWindow is the composition pin: a
// REAL rebased trace (first event at ts 0.000000) through the PRODUCTION
// engine (tracequery.Run) and the production observation mint. The explicit
// [0,end] run (the tracediag `--trace-window 0..X` shape) must declare its
// selected window with the 0-start form; the line-anchored run on the same
// trace must declare nothing and publish no whole-prefix Span pair
// (残余 (a)+(b) 全链正臂/缺席臂 on one physical trace).
func TestWinflagProductionFullChainRebasedZeroWindow(t *testing.T) {
	trace := "# tracer: nop\n" +
		"        app-100 (100) [001] .... 0.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n" +
		"      other-300 (300) [002] .... 0.001000: sched_wakeup: comm=worker pid=200 prio=40 target_cpu=002\n" +
		"     worker-200 (100) [002] .... 0.008000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=40\n" +
		"     worker-200 (100) [002] .... 0.009000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001\n" +
		"     worker-200 (100) [002] .... 0.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120\n" +
		"        app-100 (100) [001] .... 0.011000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52\n" +
		"        app-100 (100) [001] .... 0.020000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120\n"
	path := filepath.Join(t.TempDir(), "rebased.systrace")
	if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	collectNotes := func(rows []types.ObservationRecord) []string {
		var notes []string
		for _, row := range rows {
			for _, note := range row.RichNotes {
				if strings.HasPrefix(note, "selected_window=") {
					notes = append(notes, note)
				}
			}
		}
		return notes
	}
	// 正臂: the explicit [0,end] run declares the real selected window.
	result := tracequery.Run(idx, tracequery.Query{
		View: "window_stats", PID: 100,
		TimeStart: 0, TimeEnd: 0.02, TimeStartSet: true, TimeEndSet: true,
	})
	if !result.WindowStats.Window.StartSet || result.WindowStats.Window.StartTs != 0 {
		t.Fatalf("the engine result window must carry the typed flag: %+v", result.WindowStats.Window)
	}
	rows := traceQueryTypedObservations(result, "trace", "/blobs/winflag-e2e.json", "", "", time.Now())
	notes := collectNotes(rows)
	found := false
	for _, note := range notes {
		if strings.HasPrefix(note, "selected_window=0.000000..") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the rebased [0,end] run must declare its selected window, notes=%v", notes)
	}
	// 缺席臂: the line-anchored run on the same trace declares nothing and
	// mints no whole-prefix Span pair on the q-window copy families.
	result = tracequery.Run(idx, tracequery.Query{View: "window_stats", PID: 100, LineStart: 2})
	if result.WindowStats.Window.StartSet {
		t.Fatalf("the line-anchored result window must stay unset: %+v", result.WindowStats.Window)
	}
	rows = traceQueryTypedObservations(result, "trace", "/blobs/winflag-e2e2.json", "", "", time.Now())
	for _, note := range collectNotes(rows) {
		if strings.HasPrefix(note, "selected_window=0.000000..") {
			t.Fatalf("a line-anchored run must not declare a whole-prefix window: %v", note)
		}
	}
	for _, row := range rows {
		if row.ClaimKey == "compute_supply_balance" && (row.Span.StartTs != 0 || row.Span.EndTs != 0) &&
			row.Span.StartTs == 0 && row.Span.EndTs > 0 {
			t.Fatalf("line-anchored balance Span must not claim (0,end): %+v", row.Span)
		}
	}
}

func TestWinflagSelectedWindowNoteValueArms(t *testing.T) {
	// 正臂: flagged real [0,end] declares itself.
	flagged := tracequery.TimeWindow{StartTs: 0, EndTs: 0.233190, StartSet: true}
	if got := traceQuerySelectedWindowNoteValue(flagged); got != "0.000000..0.233190" {
		t.Fatalf("flagged [0,end] must declare the selected window, got %q", got)
	}
	// 缺席臂: the line-anchored unset (0,end) form stays suppressed.
	unset := tracequery.TimeWindow{StartTs: 0, EndTs: 0.233190}
	if got := traceQuerySelectedWindowNoteValue(unset); got != "" {
		t.Fatalf("unflagged (0,end) must stay suppressed, got %q", got)
	}
	// 兼容臂: positive-start windows are byte-identical to the legacy form,
	// degenerate windows stay empty even when flagged.
	if got := traceQuerySelectedWindowNoteValue(tracequery.TimeWindow{StartTs: 1.5, EndTs: 2.0}); got != "1.500000..2.000000" {
		t.Fatalf("positive window must keep its legacy bytes, got %q", got)
	}
	if got := traceQuerySelectedWindowNoteValue(tracequery.TimeWindow{StartTs: 0, EndTs: 0, StartSet: true}); got != "" {
		t.Fatalf("degenerate flagged window must stay empty, got %q", got)
	}
}

func TestWinflagObservationWindowSpanTsArms(t *testing.T) {
	// 正臂: flagged real [0,end] copies verbatim.
	s, e := traceQueryObservationWindowSpanTs(tracequery.TimeWindow{StartTs: 0, EndTs: 3.5, StartSet: true})
	if s != 0 || e != 3.5 {
		t.Fatalf("flagged [0,end] must copy verbatim, got %v..%v", s, e)
	}
	// 缺席臂: the unset (0,end) form publishes the ABSENT pair — no
	// whole-prefix window claim reaches the evidence-index labels.
	s, e = traceQueryObservationWindowSpanTs(tracequery.TimeWindow{StartTs: 0, EndTs: 3.5})
	if s != 0 || e != 0 {
		t.Fatalf("unflagged (0,end) must publish the absent pair, got %v..%v", s, e)
	}
	// 兼容臂: positive-start windows copy verbatim (including the degenerate
	// (5,0) legacy shape — its bytes are untouched by this batch).
	s, e = traceQueryObservationWindowSpanTs(tracequery.TimeWindow{StartTs: 5.0, EndTs: 0})
	if s != 5.0 || e != 0 {
		t.Fatalf("legacy degenerate shape must keep its bytes, got %v..%v", s, e)
	}
}

// winflagChainResult builds a minimal wakeup-chain result whose window is the
// given result window; the single branch-less node pair yields the legacy
// path record (the chain-path Span family under test).
func winflagChainResult(window tracequery.TimeWindow) *tracequery.ChainResult {
	return &tracequery.ChainResult{
		Target: tracequery.ThreadRef{Comm: "app", PID: 20},
		Window: window,
		Nodes: []tracequery.ChainNode{
			{ID: "n1", Thread: tracequery.ThreadRef{Comm: "app", PID: 20}, Depth: 0},
			{ID: "n2", Thread: tracequery.ThreadRef{Comm: "waker", PID: 21}, Depth: 1},
		},
		Edges: []tracequery.WakeupEdge{{
			Waker: tracequery.ThreadRef{Comm: "waker", PID: 21},
			Wakee: tracequery.ThreadRef{Comm: "app", PID: 20},
			// WakeupLine anchors the record line range.
			WakeupTs: 0.01, WakeupLine: 12,
		}},
	}
}

func TestWinflagChainPathSpanHonestAbsence(t *testing.T) {
	run := func(window tracequery.TimeWindow) types.ObservationRecord {
		result := tracequery.Result{
			View: "wakeup_chain", SourcePath: "/traces/app.systrace",
			WakeupChain: winflagChainResult(window),
		}
		rows := traceQueryTypedObservations(result, "trace", "/blobs/winflag.json", "", "", time.Now())
		for _, row := range rows {
			if row.ClaimKey == "wakeup_chain:path" {
				return row
			}
		}
		t.Fatalf("fixture must publish the wakeup_chain:path record; rows=%d", len(rows))
		return types.ObservationRecord{}
	}
	// 缺席臂: the line-anchored unset (0,end) window publishes an absent
	// Span ts pair (line range untouched).
	row := run(tracequery.TimeWindow{StartTs: 0, EndTs: 4.2})
	if row.Span.StartTs != 0 || row.Span.EndTs != 0 {
		t.Fatalf("unset window must publish an absent Span pair: %+v", row.Span)
	}
	if row.Span.LineStart <= 0 {
		t.Fatalf("line anchor must survive the absent ts pair: %+v", row.Span)
	}
	// 正臂: the flagged real [0,end] window copies verbatim and declares the
	// selected window on the record notes.
	row = run(tracequery.TimeWindow{StartTs: 0, EndTs: 4.2, StartSet: true})
	if row.Span.StartTs != 0 || row.Span.EndTs != 4.2 {
		t.Fatalf("flagged window must copy verbatim: %+v", row.Span)
	}
	// 兼容臂: a positive window keeps its legacy bytes.
	row = run(tracequery.TimeWindow{StartTs: 1.0, EndTs: 4.2})
	if row.Span.StartTs != 1.0 || row.Span.EndTs != 4.2 {
		t.Fatalf("positive window must keep legacy bytes: %+v", row.Span)
	}
}

func TestWinflagComputeSupplyBalanceSpanHonestAbsence(t *testing.T) {
	run := func(window tracequery.TimeWindow) types.ObservationRecord {
		result := tracequery.Result{
			View: "window_stats", SourcePath: "/traces/app.systrace",
			WindowStats: &tracequery.WindowStats{
				Window: window,
				ComputeSupplyBalance: &tracequery.ComputeSupplyBalance{
					SupplyRatio: 0.82, DeliveredComputeMs: 100, WindowMs: 120, CPUCount: 8,
					Summary: "compute supply 0.82",
				},
			},
		}
		rows := traceQueryTypedObservations(result, "trace", "/blobs/winflag2.json", "", "", time.Now())
		for _, row := range rows {
			if row.ClaimKey == "compute_supply_balance" {
				return row
			}
		}
		t.Fatalf("fixture must publish the compute_supply_balance record")
		return types.ObservationRecord{}
	}
	row := run(tracequery.TimeWindow{StartTs: 0, EndTs: 4.2})
	if row.Span.StartTs != 0 || row.Span.EndTs != 0 {
		t.Fatalf("unset window must publish an absent Span pair: %+v", row.Span)
	}
	row = run(tracequery.TimeWindow{StartTs: 0, EndTs: 4.2, StartSet: true})
	if row.Span.StartTs != 0 || row.Span.EndTs != 4.2 {
		t.Fatalf("flagged window must copy verbatim: %+v", row.Span)
	}
}

func TestWinflagTargetWindowStatesSpanAndNote(t *testing.T) {
	run := func(window tracequery.TimeWindow) types.ObservationRecord {
		result := tracequery.Result{
			View: "wakeup_chain", SourcePath: "/traces/app.systrace",
			TargetWindowStates: &tracequery.TargetWindowStateAccount{
				Thread: tracequery.ThreadRef{Comm: "app", PID: 20},
				Window: window, WindowMs: (window.EndTs - window.StartTs) * 1000,
				RunningMs: 10, RunnableMs: 5, SleepMs: 2, TotalMs: 17,
				LineStart: 3, LineEnd: 9,
			},
		}
		rows := traceQueryTypedObservations(result, "trace", "/blobs/winflag3.json", "", "", time.Now())
		for _, row := range rows {
			if strings.HasPrefix(row.ClaimKey, "target_window_states:") {
				return row
			}
		}
		t.Fatalf("fixture must publish the target_window_states record")
		return types.ObservationRecord{}
	}
	// 缺席臂: absent Span pair AND no selected_window note.
	row := run(tracequery.TimeWindow{StartTs: 0, EndTs: 4.2})
	if row.Span.StartTs != 0 || row.Span.EndTs != 0 {
		t.Fatalf("unset window must publish an absent Span pair: %+v", row.Span)
	}
	for _, note := range row.RichNotes {
		if strings.HasPrefix(note, "selected_window=") {
			t.Fatalf("unset window must not declare a selected window: %v", row.RichNotes)
		}
	}
	// 正臂: verbatim Span pair AND the 0-start selected_window declaration —
	// the note round-trips through the ONE strict projection parser.
	row = run(tracequery.TimeWindow{StartTs: 0, EndTs: 4.2, StartSet: true})
	if row.Span.StartTs != 0 || row.Span.EndTs != 4.2 {
		t.Fatalf("flagged window must copy verbatim: %+v", row.Span)
	}
	found := false
	for _, note := range row.RichNotes {
		if note == "selected_window=0.000000..4.200000" {
			found = true
		}
	}
	if !found {
		t.Fatalf("flagged window must declare selected_window: %v", row.RichNotes)
	}
	if ws, we, ok := types.TraceCausalProjectionSelectedWindowNote(row.RichNotes); !ok || ws != 0 || we != 4.2 {
		t.Fatalf("the flag-minted 0-start note must parse through the strict parser: %v %v %v", ws, we, ok)
	}
}

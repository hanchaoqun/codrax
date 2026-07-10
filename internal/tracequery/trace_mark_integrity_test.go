package tracequery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func traceMarkTestLine(comm string, pid int, ts float64, payload string) string {
	return fmt.Sprintf(" %s-%d (%d) [000] .... %.6f: tracing_mark_write: %s", comm, pid, pid, ts, payload)
}

func writeTraceMarkIntegrityTrace(t *testing.T, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(strings.Join(append(lines, ""), "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func caveatsContain(caveats []string, needle string) bool {
	for _, caveat := range caveats {
		if strings.Contains(caveat, needle) {
			return true
		}
	}
	return false
}

func TestTraceMarkEndpointSchemaTruthTable(t *testing.T) {
	valid := []struct {
		payload     string
		action      string
		pid         int
		name, value string
	}{
		{"E", "E", 0, "E", ""},
		{"E|7", "E", 7, "E|7", ""},
		{"E|7|", "E", 7, "E|7|", ""},
		{"E|7|I39", "E", 7, "I39", ""},
		{"E|7|M0538", "E", 7, "M0538", ""},
		{"B|0|zero-owner-compatible", "B", 0, "zero-owner-compatible", ""},
		// Production Berlin witness: the logical name itself ends in an empty
		// pipe component; only the exact rightmost M tag delimits it.
		{"B|1855|H:CheckNeedNotify notifyCdt=1 needR||M0538", "B", 1855, "H:CheckNeedNotify notifyCdt=1 needR|", "M0538"},
		{"B|7|phase||inner|I42", "B", 7, "phase||inner", "I42"},
		{"S|7|async|opaque-cookie", "S", 7, "async", "opaque-cookie"},
		{"F|7|async|opaque-cookie", "F", 7, "async", "opaque-cookie"},
		{"S|7|async|opaque-cookie|I42", "S", 7, "async", "opaque-cookie"},
		{"F|7|async|opaque-cookie|M0538", "F", 7, "async", "opaque-cookie"},
		{"S|7|async||phase|cookie-alpha|I42", "S", 7, "async||phase", "cookie-alpha"},
		{"F|7|async||phase|cookie-alpha|M0538", "F", 7, "async||phase", "cookie-alpha"},
		{"0x0: 7|carved", "B", 7, "carved", ""},
	}
	for _, tc := range valid {
		t.Run("valid_"+strings.NewReplacer("|", "_", ":", "_").Replace(tc.payload), func(t *testing.T) {
			ev, ok := ParseLine(1, traceMarkTestLine("app", 10, 1, tc.payload), newStringInterner())
			if !ok || ev.Type != EventTraceMark || traceMarkEventMalformed(ev) {
				t.Fatalf("valid payload rejected: ok=%v event=%+v", ok, ev)
			}
			if ev.SpanAction != tc.action || ev.SpanPID != tc.pid || ev.SpanName != tc.name || ev.SpanValue != tc.value {
				t.Fatalf("payload %q => %q/%d/%q/%q, want %q/%d/%q/%q", tc.payload,
					ev.SpanAction, ev.SpanPID, ev.SpanName, ev.SpanValue, tc.action, tc.pid, tc.name, tc.value)
			}
		})
	}

	invalid := []struct {
		payload, action, reason string
	}{
		{"B|bad|name", "B", "invalid_payload_pid"},
		{"B|7|", "B", "empty_name"},
		{"B|7", "B", "invalid_arity"},
		{"B|7|name|not-a-tag", "B", "invalid_arity"},
		{"B|7|name||not-a-tag", "B", "invalid_arity"},
		{"B|7|name||X42", "B", "invalid_arity"},
		{"B|7|name||I42|extra", "B", "invalid_arity"},
		{"B|7|||M0538", "B", "empty_name"},
		{"E|bad", "E", "invalid_payload_pid"},
		{"E|7|not-a-tag", "E", "invalid_end_tag"},
		{"E|7|I1|extra", "E", "invalid_arity"},
		{"S|0|name|cookie", "S", "payload_pid_must_be_positive"},
		{"S|7||cookie", "S", "empty_name"},
		{"S|7|name|", "S", "empty_cookie"},
		{"S|7|name|cookie|extra", "S", "invalid_arity"},
		{"S|7|async|phase|cookie", "S", "invalid_arity"},
		{"S|7|async||cookie|X42", "S", "invalid_arity"},
		{"S|7|||cookie|I42", "S", "empty_name"},
		{"F|7|async||I42", "F", "empty_cookie"},
		{"F|bad|name|opaque", "F", "invalid_payload_pid"},
	}
	for i, tc := range invalid {
		t.Run(fmt.Sprintf("invalid_%d_%s", i, tc.action), func(t *testing.T) {
			ev, ok := ParseLine(1, traceMarkTestLine("app", 10, 1, tc.payload), newStringInterner())
			if !ok || ev.Type != EventTraceMark {
				t.Fatalf("malformed mark must remain trace_mark inventory: ok=%v event=%+v", ok, ev)
			}
			action, reason := traceMarkEventInvalidCodes(ev)
			if ev.SpanAction != "" || action.String() != tc.action || reason.String() != tc.reason {
				t.Fatalf("malformed verdict mismatch: %+v action=%q reason=%q", ev, action, reason)
			}
			if ev.FieldText != tc.payload {
				t.Fatalf("event_search payload lost: got %q want %q", ev.FieldText, tc.payload)
			}
		})
	}

	// Non-numeric C stays inventory and reaches CounterQuality; it is not an
	// endpoint and must not have its action cleared by the B/E/S/F validator.
	ev, ok := ParseLine(1, traceMarkTestLine("app", 10, 1, "C|7|depth|not-a-number"), newStringInterner())
	if !ok || ev.SpanAction != "C" || traceMarkEventMalformed(ev) {
		t.Fatalf("non-numeric counter action must remain C inventory: %+v", ev)
	}
}

func TestBerlinPipeNameBeginPairsFromExactRightTag(t *testing.T) {
	path := writeTraceMarkIntegrityTrace(t, "berlin-pipe-name.systrace",
		`  render_service-1855  ( 1855) [002] .... 17729.476873: print: B|1855|H:CheckNeedNotify notifyCdt=1 needR||M0538`,
		`  render_service-1855  ( 1855) [002] .... 17729.476874: print: E|1855|M0538`,
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.traceMarkIntegrityFailures) != 0 {
		t.Fatalf("production pipe-name endpoint was classified malformed: %+v", idx.traceMarkIntegrityFailures)
	}
	spans, _, caveats := computeTraceMarks(idx, Query{TimeStart: 17729.4768, TimeEnd: 17729.4770}, 8)
	if len(spans) != 1 || spans[0].Name != "H:CheckNeedNotify notifyCdt=1 needR|" || spans[0].SpanPID != 1855 || spans[0].Kind != "sync" {
		t.Fatalf("production B/E pair did not retain its right-delimited name: spans=%+v caveats=%v", spans, caveats)
	}
}

func TestMalformedTraceMarksResetOnlyEmitterAndRecover(t *testing.T) {
	path := writeTraceMarkIntegrityTrace(t, "reset.systrace",
		traceMarkTestLine("a", 10, 1.000, "B|10|old-e"),
		traceMarkTestLine("b", 20, 1.001, "B|20|other"),
		traceMarkTestLine("a", 10, 1.002, "E|bad"),
		traceMarkTestLine("a", 10, 1.003, "E"),
		traceMarkTestLine("b", 20, 1.004, "E"),
		traceMarkTestLine("a", 10, 1.005, "B|10|old-b"),
		traceMarkTestLine("a", 10, 1.006, "B|bad|poison"),
		traceMarkTestLine("a", 10, 1.007, "E"),
		traceMarkTestLine("a", 10, 1.008, "B|10|recovered"),
		traceMarkTestLine("a", 10, 1.009, "E"),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.traceMarkIntegrityFailures) != 2 {
		t.Fatalf("typed malformed ledger=%+v", idx.traceMarkIntegrityFailures)
	}
	spans, _, caveats := computeTraceMarks(idx, Query{TimeStart: 1, TimeEnd: 1.01}, 16)
	if len(spans) != 2 || spans[0].Name != "other" && spans[1].Name != "other" {
		t.Fatalf("only unaffected+recovered spans should survive: %+v", spans)
	}
	names := map[string]bool{}
	for _, span := range spans {
		names[span.Name] = true
	}
	if !names["other"] || !names["recovered"] || names["old-e"] || names["old-b"] {
		t.Fatalf("local reset/recovery mismatch: %+v", spans)
	}
	if !caveatsContain(caveats, "trace_mark_integrity_degraded=true") {
		t.Fatalf("missing model-facing integrity caveat: %+v", caveats)
	}
	windows, windowCaveats := FindSpanWindows(idx, Query{TimeStart: 1, TimeEnd: 1.01}, 16)
	if len(windows) != 2 || !caveatsContain(windowCaveats, "trace_mark_integrity_degraded=true") {
		t.Fatalf("span_window parity mismatch: spans=%+v caveats=%+v", windows, windowCaveats)
	}
	marks := EventSearch(idx, Query{EventTypes: []EventType{EventTraceMark}, Pattern: "E|bad", Limit: 10})
	if len(marks) != 1 || marks[0].SpanAction != "" || marks[0].Raw == "" {
		t.Fatalf("malformed row must remain event_search evidence: %+v", marks)
	}
}

func TestMalformedAsyncEndpointResetsAndOpaqueCookiePairs(t *testing.T) {
	path := writeTraceMarkIntegrityTrace(t, "async.systrace",
		traceMarkTestLine("a", 10, 2.000, "S|10|old|opaque-old"),
		traceMarkTestLine("a", 10, 2.001, "F|10|old|"),
		traceMarkTestLine("a", 10, 2.002, "F|10|old|opaque-old"),
		traceMarkTestLine("a", 10, 2.003, "S|10|fresh|cookie-alpha"),
		traceMarkTestLine("a", 10, 2.004, "F|10|fresh|cookie-alpha"),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	spans, _, _ := computeTraceMarks(idx, Query{TimeStart: 2, TimeEnd: 2.01}, 16)
	if len(spans) != 1 || spans[0].Name != "fresh" || spans[0].Kind != "async" {
		t.Fatalf("opaque async cookie/recovery mismatch: %+v", spans)
	}
}

func TestLongMalformedTraceMarkCannotRevalidateAfterInventoryClamp(t *testing.T) {
	// The complete payload has a fourth, invalid B field, but that field starts
	// beyond Event.FieldText's 300-byte inventory bound. Re-validating the
	// retained prefix would see a syntactically valid three-field B and let the
	// later E close the older real begin.
	malformed := "B|10|" + strings.Repeat("x", 320) + "|bad-extra"
	path := writeTraceMarkIntegrityTrace(t, "long-malformed.systrace",
		traceMarkTestLine("a", 10, 1.000, "B|10|must-not-cross-pair"),
		traceMarkTestLine("a", 10, 1.001, malformed),
		traceMarkTestLine("a", 10, 1.002, "E"),
		traceMarkTestLine("a", 10, 1.003, "B|10|recovered"),
		traceMarkTestLine("a", 10, 1.004, "E"),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.traceMarkIntegrityFailures) != 1 {
		t.Fatalf("full-payload ledger lost the long malformed endpoint: %+v", idx.traceMarkIntegrityFailures)
	}
	failure := idx.traceMarkIntegrityFailures[0]
	if failure.Action != "B" || failure.Reason != "invalid_arity" || !failure.EmitterKnown || failure.RowPID != 10 {
		t.Fatalf("exact raw-ledger verdict drifted: %+v", failure)
	}
	var retained Event
	for _, ev := range idx.Events {
		if ev.Line == 2 {
			retained = ev
			break
		}
	}
	if retained.Line == 0 || retained.SpanAction != "" || len(retained.FieldText) != 300 {
		t.Fatalf("adversarial endpoint did not reach the bounded inventory shape: %+v", retained)
	}
	if parsed := parseTraceMarkValidated(retained.FieldText); parsed.invalidAction != traceMarkActionValid || parsed.action != "B" {
		t.Fatalf("test premise failed: bounded prefix should look valid to the old re-parser: %+v", parsed)
	}
	if !traceMarkEventMalformed(retained) {
		t.Fatalf("empty typed action plus retained B prefix must stay malformed: %+v", retained)
	}

	spans, _, caveats := computeTraceMarks(idx, Query{TimeStart: 1, TimeEnd: 1.01}, 16)
	if len(spans) != 1 || spans[0].Name != "recovered" {
		t.Fatalf("long malformed B crossed into a forged sync duration: %+v", spans)
	}
	if !caveatsContain(caveats, "reason=invalid_arity") {
		t.Fatalf("exact full-payload reason must remain model-visible: %+v", caveats)
	}
	windows, windowCaveats := FindSpanWindows(idx, Query{TimeStart: 1, TimeEnd: 1.01}, 16)
	if len(windows) != 1 || windows[0].Name != "recovered" || !caveatsContain(windowCaveats, "reason=invalid_arity") {
		t.Fatalf("span_window long-payload parity mismatch: spans=%+v caveats=%+v", windows, windowCaveats)
	}
}

func TestSharedAsyncDuplicateLaneResetCannotSalvageOneStartByEmitter(t *testing.T) {
	tests := []struct {
		name      string
		resetLine string
	}{
		{
			name:      "malformed endpoint",
			resetLine: traceMarkTestLine("a", 10, 2.002, "E|bad"),
		},
		{
			name:      "task reincarnation",
			resetLine: ` creator-30 (30) [000] .... 2.002000: sched_wakeup_new: comm=a pid=10 prio=120 target_cpu=000`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTraceMarkIntegrityTrace(t, "shared-async.systrace",
				traceMarkTestLine("a", 10, 2.000, "S|500|shared|cookie"),
				traceMarkTestLine("b", 20, 2.001, "S|500|shared|cookie"),
				tc.resetLine,
				traceMarkTestLine("b", 20, 2.003, "F|500|shared|cookie"),
				traceMarkTestLine("b", 20, 2.004, "F|500|shared|cookie"),
				traceMarkTestLine("b", 20, 2.005, "S|500|shared|cookie"),
				traceMarkTestLine("b", 20, 2.006, "F|500|shared|cookie"),
			)
			idx, err := BuildIndex(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			spans, _, caveats := computeTraceMarks(idx, Query{TimeStart: 2, TimeEnd: 2.01}, 16)
			if len(spans) != 1 || spans[0].Kind != "async" || spans[0].Thread.PID != 20 || spans[0].StartTs != 2.005 || spans[0].EndTs != 2.006 {
				t.Fatalf("duplicate cohort was partially salvaged or failed to recover after depth zero: %+v", spans)
			}
			if !caveatsContain(caveats, "trace_mark_async_duplicate_key_fail_closed=true") {
				t.Fatalf("duplicate shared lane was withheld without disclosure: %+v", caveats)
			}
			windows, windowCaveats := FindSpanWindows(idx, Query{TimeStart: 2, TimeEnd: 2.01}, 16)
			if len(windows) != 1 || windows[0].Thread.PID != 20 || windows[0].StartTs != 2.005 || windows[0].EndTs != 2.006 {
				t.Fatalf("span_window shared-lane duplicate/recovery parity mismatch: %+v", windows)
			}
			if !caveatsContain(windowCaveats, "trace_mark_async_duplicate_key_fail_closed=true") {
				t.Fatalf("span_window duplicate shared lane disclosure missing: %+v", windowCaveats)
			}
		})
	}
}

func TestDurationOrderSharedAsyncLaneResetClearsWholeAuditLane(t *testing.T) {
	start := func(pid int, ts float64) Event {
		return Event{Type: EventTraceMark, PID: pid, Ts: ts, SpanAction: "S", SpanPID: 500, SpanName: "shared", SpanValue: "cookie"}
	}
	assertCleared := func(t *testing.T, tracker *durationOrderTracker) {
		t.Helper()
		lane := durationOrderLane{family: durationOrderTraceSpan, key: "async\x00" + traceAsyncSpanKey(start(10, 2))}
		if tracker.depth[lane] != 0 || len(tracker.traceSpanOwners[lane]) != 0 {
			t.Fatalf("shared async audit lane retained partial state: depth=%d owners=%v", tracker.depth[lane], tracker.traceSpanOwners[lane])
		}
		if _, ok := tracker.last[lane]; ok {
			t.Fatalf("shared async audit lane retained timestamp predecessor: %+v", tracker.last)
		}
	}

	for _, tc := range []struct {
		name  string
		reset Event
	}{
		{name: "malformed", reset: Event{Type: EventTraceMark, PID: 10, Ts: 2.002, FieldText: "E|bad"}},
		{name: "lifecycle", reset: Event{Type: EventSchedWakeup, Name: "sched_wakeup_new", WakeePID: 10, Ts: 2.002}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracker := newDurationOrderTracker()
			tracker.observeAll(start(10, 2.000))
			tracker.observeAll(start(20, 2.001))
			tracker.observeAll(tc.reset)
			assertCleared(t, tracker)
		})
	}
}

func TestMalformedTraceMarkClearsIPCInterfaceJoinLocally(t *testing.T) {
	malformed := Event{Line: 2, Ts: 1.01, Type: EventTraceMark, PID: 10, FieldText: "E|bad"}
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 1.00, Type: EventTraceMark, PID: 10, SpanAction: "B", SpanPID: 10, SpanName: "transact[old.Interface:1]"},
		malformed,
		{Line: 3, Ts: 1.02, Type: EventBinderTransaction, PID: 10, BinderFields: &BinderFields{TransactionID: 1, DestProc: 20, DestThread: 21}},
		{Line: 4, Ts: 1.03, Type: EventTraceMark, PID: 10, SpanAction: "B", SpanPID: 10, SpanName: "transact[new.Interface:2]"},
		{Line: 5, Ts: 1.04, Type: EventBinderTransaction, PID: 10, BinderFields: &BinderFields{TransactionID: 2, DestProc: 20, DestThread: 21}},
	}, traceMarkIntegrityFailures: []traceMarkIntegrityFailure{{Action: "E", Reason: "invalid_payload_pid", Line: 2, LocalLine: 2, Ts: 1.01, RowPID: 10, EmitterKnown: true}}}
	graph := BuildIPCGraph(idx, Query{TimeStart: 1, TimeEnd: 1.1})
	if len(graph.Edges) != 2 || graph.Edges[0].Interface != "" || graph.Edges[1].Interface != "new.Interface:2" {
		t.Fatalf("malformed endpoint must clear old interface and allow recovery: %+v", graph.Edges)
	}

	idx.traceMarkIntegrityFailures = []traceMarkIntegrityFailure{{Action: "B", Reason: "invalid_emitter_pid", Line: 2, LocalLine: 2, Ts: 1.01, EmitterKnown: false}}
	graph = BuildIPCGraph(idx, Query{TimeStart: 1, TimeEnd: 1.1})
	if len(graph.Edges) != 2 || graph.Edges[0].Interface != "" || graph.Edges[1].Interface != "" {
		t.Fatalf("unknown emitter must disable only interface joins, not binder edges: %+v", graph.Edges)
	}
	if !caveatsContain(graph.Caveats, "trace_mark_interface_join_fail_closed=true") {
		t.Fatalf("unknown-emitter IPC caveat missing: %+v", graph.Caveats)
	}
}

func TestTraceMarkIdentityVotesUseClosedTypedActions(t *testing.T) {
	badCounter := Event{Line: 1, Ts: 1, Type: EventTraceMark, PID: 10, SpanAction: "C", SpanPID: 500, SpanName: "depth", SpanValue: "bad", FieldText: "C|500|depth|bad"}
	if got := buildTidTgidDerivation([]Event{badCounter}); got != nil {
		t.Fatalf("non-numeric C must not vote in tid/tgid derivation: %+v", got)
	}
	goodCounter := badCounter
	goodCounter.SpanValue, goodCounter.FieldText = "1", "C|500|depth|1"
	if got := buildTidTgidDerivation([]Event{goodCounter}); got == nil || got.tidTgid[10] != 500 {
		t.Fatalf("typed finite C should retain compatibility vote: %+v", got)
	}

	async := Event{Line: 1, Ts: 1, Type: EventTraceMark, PID: 10, TGID: 100, SpanAction: "S", SpanPID: 700, SpanName: "async", SpanValue: "cookie"}
	derived := buildNsSpanDerivation([]Event{async, {Line: 2, Ts: 2, Type: EventTraceMark, PID: 10, TGID: 100, SpanAction: "F", SpanPID: 700, SpanName: "async", SpanValue: "cookie"}, badCounter})
	if len(derived.process) != 0 {
		t.Fatalf("S/F and malformed C must not establish ns process identity: %+v", derived.process)
	}
	goodCounter.TGID = 100
	derived = buildNsSpanDerivation([]Event{goodCounter})
	if derived.process[500] == nil || derived.process[500].HostTGID != 100 {
		t.Fatalf("typed finite C ns vote missing: %+v", derived.process)
	}
}

func TestTraceMarkHeaderPIDOverflowFailsClosedWithoutPIDZero(t *testing.T) {
	overflow := " app-999999999999999999999999 (10) [000] .... 3.000000: tracing_mark_write: B|10|unknown-emitter"
	path := writeTraceMarkIntegrityTrace(t, "overflow.systrace",
		overflow,
		traceMarkTestLine("app", 10, 3.001, "B|10|otherwise-valid"),
		traceMarkTestLine("app", 10, 3.002, "E"),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.traceMarkIntegrityFailures) != 1 || idx.traceMarkIntegrityFailures[0].EmitterKnown || idx.traceMarkIntegrityFailures[0].Reason != "invalid_emitter_pid" {
		t.Fatalf("overflow emitter witness missing: %+v", idx.traceMarkIntegrityFailures)
	}
	for _, ev := range idx.Events {
		if ev.Line == 1 || ev.PID == 0 {
			t.Fatalf("overflow row must not materialize as pid0: %+v", ev)
		}
	}
	spans, _, caveats := computeTraceMarks(idx, Query{TimeStart: 3, TimeEnd: 3.01}, 16)
	if len(spans) != 0 || !caveatsContain(caveats, "trace_mark_span_pairing_fail_closed=true") {
		t.Fatalf("unknown emitter must fail-close trace spans: spans=%+v caveats=%+v", spans, caveats)
	}
	if windows, cs := FindSpanWindows(idx, Query{TimeStart: 3, TimeEnd: 3.01}, 16); len(windows) != 0 || !caveatsContain(cs, "trace_mark_span_pairing_fail_closed=true") {
		t.Fatalf("unknown emitter must fail-close span_window: spans=%+v caveats=%+v", windows, cs)
	}
}

func TestUnmaterializedTraceMarkEndpointGloballyFailsClosed(t *testing.T) {
	unsafeTimestamp := "1" + strings.Repeat("0", 305) + ".0"
	tests := []struct {
		name, badEnd, reason string
	}{
		{
			name:   "invalid header cpu",
			badEnd: ` app-10 (10) [9999] .... 3.001000: tracing_mark_write: E`,
			reason: "invalid_header_cpu",
		},
		{
			name:   "unsafe timestamp",
			badEnd: " app-10 (10) [000] .... " + unsafeTimestamp + `: tracing_mark_write: E`,
			reason: "invalid_timestamp",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTraceMarkIntegrityTrace(t, "unmaterialized.systrace",
				traceMarkTestLine("app", 10, 3.000, "B|10|must-not-cross-pair"),
				tc.badEnd,
				traceMarkTestLine("app", 10, 3.002, "E"),
			)
			idx, err := BuildIndex(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if len(idx.traceMarkIntegrityFailures) != 1 {
				t.Fatalf("unmaterialized marker witness missing: %+v", idx.traceMarkIntegrityFailures)
			}
			failure := idx.traceMarkIntegrityFailures[0]
			if failure.Reason != tc.reason || !failure.Unmaterialized {
				t.Fatalf("unmaterialized marker verdict mismatch: %+v", failure)
			}
			spans, _, caveats := computeTraceMarks(idx, Query{TimeStart: 3, TimeEnd: 3.01}, 16)
			if len(spans) != 0 {
				t.Fatalf("later E crossed an unmaterialized endpoint and minted a span: %+v", spans)
			}
			if !caveatsContain(caveats, "trace_mark_span_pairing_fail_closed=true") ||
				!caveatsContain(caveats, tc.reason) {
				t.Fatalf("unmaterialized endpoint was not disclosed: %+v", caveats)
			}
			graph := BuildIPCGraph(idx, Query{TimeStart: 3, TimeEnd: 3.01})
			if !caveatsContain(graph.Caveats, "trace_mark_interface_join_fail_closed=true") {
				t.Fatalf("interface join did not share the global fail-close verdict: %+v", graph.Caveats)
			}
		})
	}
}

func TestTraceMarkIntegrityLedgerCapWarmCacheAndBundleProvenance(t *testing.T) {
	var cappedLines []string
	for i := 0; i < traceMarkIntegrityFailureCap+1; i++ {
		cappedLines = append(cappedLines, traceMarkTestLine("app", 10, 4+float64(i)/1000, "B|bad|name"))
	}
	cappedPath := writeTraceMarkIntegrityTrace(t, "cap.systrace", cappedLines...)
	capped, err := BuildIndex(context.Background(), cappedPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(capped.traceMarkIntegrityFailures) != traceMarkIntegrityFailureCap || !capped.traceMarkIntegrityFailuresCapped {
		t.Fatalf("bounded ledger mismatch: len=%d capped=%v", len(capped.traceMarkIntegrityFailures), capped.traceMarkIntegrityFailuresCapped)
	}
	if !caveatsContain(traceMarkIntegrityCaveats(capped, Query{}), "additional_rows_unknown=true") {
		t.Fatalf("ledger overflow must be disclosed: %+v", traceMarkIntegrityCaveats(capped, Query{}))
	}

	cachePath := writeTraceMarkIntegrityTrace(t, "cache.systrace",
		traceMarkTestLine("app", 10, 5.000, "B|bad|name"),
		traceMarkTestLine("app", 10, 5.010, "B|10|ok"),
		traceMarkTestLine("app", 10, 5.020, "E"),
	)
	indexCache = newTraceIndexCache(traceIndexCacheBudgetBytes)
	if _, err := BuildIndex(context.Background(), cachePath); err != nil {
		t.Fatal(err)
	}
	warm, err := BuildIndexWithOptions(context.Background(), cachePath, BuildOptions{
		AllowWindowedParse: true, TimeStart: 5.0, TimeStartSet: true, TimeEnd: 5.03, TimeEndSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !warm.Windowed || len(warm.traceMarkIntegrityFailures) != 1 || warm.traceMarkIntegrityFailures[0].Line != 1 {
		t.Fatalf("warm full-cache derive lost malformed witness: %+v", warm.traceMarkIntegrityFailures)
	}

	dir := t.TempDir()
	first := filepath.Join(dir, "first.perftrace")
	second := filepath.Join(dir, "second.systrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleProvenanceFixture(t, first, ` perf-20 (20) [000] .... 6.000000: perf_sample: cpu=0 pid=20 tid=20 period=1 event=cpu-cycles symbol=Warmup dso=lib.so source=test`)
	writeBundleProvenanceFixture(t, second, traceMarkTestLine("bad", 30, 6.002, "E|bad"))
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "artifacts":[
	{"type":"perftrace","path":"first.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}},
    {"type":"systrace","path":"second.systrace"}
  ]
}`)
	bundled, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundled.traceMarkIntegrityFailures) != 1 {
		t.Fatalf("bundle lost malformed witness: failures=%+v artifacts=%+v events=%+v", bundled.traceMarkIntegrityFailures, bundled.TraceArtifacts, bundled.Events)
	}
	failure := bundled.traceMarkIntegrityFailures[0]
	if failure.SourcePath != canonicalTraceIndexPath(second) || failure.LocalLine != 1 || failure.Line == failure.LocalLine {
		t.Fatalf("bundle witness must retain source/local and rebased global line: %+v", failure)
	}
	if !strings.Contains(failure.reason(), "source="+canonicalTraceIndexPath(second)) || !strings.Contains(failure.reason(), "local_line=1") {
		t.Fatalf("bundle-local provenance absent from disclosure: %q", failure.reason())
	}
}

func TestTraceMarkKnownEmitterLedgerOverflowStaysLocallyRecoverable(t *testing.T) {
	var lines []string
	for i := 0; i < traceMarkIntegrityFailureCap+8; i++ {
		lines = append(lines, traceMarkTestLine("app", 10, 7+float64(i)/1000, "B|bad|name"))
	}
	lines = append(lines,
		traceMarkTestLine("app", 10, 7.100, "B|10|healthy-after-overflow"),
		traceMarkTestLine("app", 10, 7.101, "E"),
	)
	path := writeTraceMarkIntegrityTrace(t, "known-overflow.systrace", lines...)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !idx.traceMarkIntegrityFailuresCapped || idx.traceMarkIntegrityDroppedGlobalPoison {
		t.Fatalf("known materialized failures must cap without global poison: capped=%t global=%t", idx.traceMarkIntegrityFailuresCapped, idx.traceMarkIntegrityDroppedGlobalPoison)
	}
	spans, _, caveats := computeTraceMarks(idx, Query{TimeStart: 7, TimeEnd: 7.2}, 16)
	if len(spans) != 1 || spans[0].Name != "healthy-after-overflow" {
		t.Fatalf("known-emitter overflow globally suppressed a recoverable span: spans=%+v caveats=%+v", spans, caveats)
	}
	if !caveatsContain(caveats, "additional_rows_unknown=true") {
		t.Fatalf("bounded witness overflow must remain disclosed: %+v", caveats)
	}
}

func TestTraceMarkDroppedUnmaterializedEndpointStillGloballyFailsClosed(t *testing.T) {
	var lines []string
	for i := 0; i < traceMarkIntegrityFailureCap; i++ {
		lines = append(lines, traceMarkTestLine("app", 10, 8+float64(i)/1000, "B|bad|name"))
	}
	lines = append(lines,
		` app-999999999999999999999999 (10) [000] .... 8.080000: tracing_mark_write: E`,
		traceMarkTestLine("app", 10, 8.100, "B|10|must-be-withheld"),
		traceMarkTestLine("app", 10, 8.101, "E"),
	)
	path := writeTraceMarkIntegrityTrace(t, "dropped-global.systrace", lines...)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !idx.traceMarkIntegrityFailuresCapped || !idx.traceMarkIntegrityDroppedGlobalPoison {
		t.Fatalf("dropped unmaterialized endpoint must retain global poison: capped=%t global=%t", idx.traceMarkIntegrityFailuresCapped, idx.traceMarkIntegrityDroppedGlobalPoison)
	}
	spans, _, caveats := computeTraceMarks(idx, Query{TimeStart: 8, TimeEnd: 8.2}, 16)
	if len(spans) != 0 || !caveatsContain(caveats, "trace_mark_span_pairing_fail_closed=true") {
		t.Fatalf("dropped global endpoint did not fail-close pairing: spans=%+v caveats=%+v", spans, caveats)
	}
}

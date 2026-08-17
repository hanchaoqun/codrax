package tracequery

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestStreamSpanLocatePairsRemoteUnnamedEndBeyondParentWindow(t *testing.T) {
	lines := []string{traceMarkTestLine("app", 42, 1.000, "B|42|Choreographer#doFrame 8002384")}
	for i := 0; i < 200; i++ {
		lines = append(lines, fmt.Sprintf(" worker-%d (%d) [001] .... %.6f: sched_switch: prev_comm=worker prev_pid=%d prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120", 100+i, 100+i, 1.01+float64(i)*.01, 100+i))
	}
	lines = append(lines, traceMarkTestLine("app", 42, 4.250, "E"))
	path := writeWindowDiscoveryTrace(t, strings.Join(lines, "\n"))

	res, err := StreamSpanLocate(context.Background(), path, TraceFlavorAuto, Query{
		View: "span_window", SpanName: "Choreographer#doFrame 8002384", PID: 42,
		TimeStart: .990, TimeEnd: 1.010, TimeStartSet: true, TimeEndSet: true, Limit: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.SpanWindows) != 1 {
		t.Fatalf("remote unnamed E was not paired: spans=%+v caveats=%v", res.SpanWindows, res.Caveats)
	}
	span := res.SpanWindows[0]
	if span.StartTs != 1 || span.EndTs != 4.25 || !near(span.DurationMs, 3250, .001) || span.Thread.PID != 42 {
		t.Fatalf("remote span = %+v", span)
	}
	if res.TimeStart != .990 || res.TimeEnd != 4.25 || !containsSubstring(res.Caveats, "without a fixed time-padding ceiling") {
		t.Fatalf("parent/span union or streaming caliber missing: window=%.6f..%.6f caveats=%v", res.TimeStart, res.TimeEnd, res.Caveats)
	}
}

func TestStreamSpanLocateKeepsNestedLIFOAndTypedSelector(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, strings.Join([]string{
		traceMarkTestLine("other", 7, .500, "B|7|Choreographer#doFrame 8002384"),
		traceMarkTestLine("app", 42, 1.000, "B|42|Choreographer#doFrame 8002384"),
		traceMarkTestLine("app", 42, 1.100, "B|42|inner"),
		traceMarkTestLine("app", 42, 1.200, "E"),
		traceMarkTestLine("app", 42, 3.500, "E"),
		traceMarkTestLine("other", 7, 4.000, "E"),
	}, "\n"))
	res, err := StreamSpanLocate(context.Background(), path, TraceFlavorAuto, Query{
		View: "span_window", SpanName: "Choreographer#doFrame 8002384", PID: 42,
		TimeStart: .99, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.SpanWindows) != 1 || res.SpanWindows[0].StartLine != 2 || res.SpanWindows[0].EndLine != 5 ||
		!near(res.SpanWindows[0].DurationMs, 2500, .001) {
		t.Fatalf("selector/LIFO drift: %+v caveats=%v", res.SpanWindows, res.Caveats)
	}
}

func TestStreamSpanLocateSelectorBudgetIgnoresUnrelatedMarkerTraffic(t *testing.T) {
	req := traceMarkCarryRequest(.99, 4.01, 2, WindowDiscoveryFamilyTraceSync)
	req.EndpointLimit = 4
	req, err := normalizeWindowDiscoveryRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	d := newTraceMarkCarryDiscovery(req, "primary")
	d.spanSelector = &traceMarkCarrySpanSelector{name: "target span"}
	for line := 1; line <= 20; line += 2 {
		d.observe("primary", mustParseTraceMarkCarryEvent(t, line, float64(line)/10, "B|7|unrelated"))
		d.observe("primary", mustParseTraceMarkCarryEvent(t, line+1, float64(line+1)/10, "E"))
	}
	d.observe("primary", mustParseTraceMarkCarryEvent(t, 21, 3.0, "B|42|target span"))
	d.observe("primary", mustParseTraceMarkCarryEvent(t, 22, 4.0, "E"))
	result := d.finalize(&Index{LineCount: 22, LastTs: 4}, TraceSourceVersion{})
	pairs := traceMarkCarryCandidatesByStatus(result, WindowDiscoveryPairingCompleteExact)
	if result.BudgetStopped || len(pairs) != 1 || pairs[0].FirstLine != 21 || pairs[0].LastLine != 22 {
		t.Fatalf("unrelated marker traffic consumed selector budget: %+v", result)
	}
}

func TestStreamSpanLocateSupportsNamespaceProcessMembership(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, strings.Join([]string{
		traceMarkTestLine("container-thread", 501, 2.000, "B|42|namespace span"),
		traceMarkTestLine("container-thread", 501, 2.200, "B|42|nested namespace work"),
		traceMarkTestLine("container-thread", 501, 2.300, "E"),
		traceMarkTestLine("container-thread", 501, 2.800, "E"),
	}, "\n"))
	res, err := StreamSpanLocate(context.Background(), path, TraceFlavorAuto, Query{
		View: "span_window", SpanName: "namespace span", PID: 42, TargetScope: TargetScopeProcess,
		TimeStart: 1.99, TimeEnd: 2.01, TimeStartSet: true, TimeEndSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.SpanWindows) != 1 || res.SpanWindows[0].Thread.PID != 501 || res.SpanWindows[0].SpanPID != 42 ||
		res.SpanWindows[0].TargetScope != TargetScopeProcess || res.SpanWindows[0].ProcessMembershipSource != "trace_mark_span_pid" {
		t.Fatalf("namespace process membership lost: %+v caveats=%v", res.SpanWindows, res.Caveats)
	}
}

func TestStreamSpanLocateLifecycleCutNeverMintsDuration(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, strings.Join([]string{
		traceMarkTestLine("reused", 42, 5.000, "B|42|long work"),
		` creator-20 (20) [000] .... 5.100000: sched_wakeup_new: comm=reused pid=42 prio=120 target_cpu=000`,
		traceMarkTestLine("reused", 42, 6.000, "E"),
	}, "\n"))
	res, err := StreamSpanLocate(context.Background(), path, TraceFlavorAuto, Query{
		View: "span_window", SpanName: "long work", PID: 42,
		TimeStart: 4.99, TimeEnd: 5.01, TimeStartSet: true, TimeEndSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.SpanWindows) != 0 || !containsSubstring(res.Caveats, "trace_mark_carry_identity_complete=false") ||
		!containsSubstring(res.Caveats, "no duration or causal window was inferred") {
		t.Fatalf("lifecycle cut minted a duration or lost typed fail-close: spans=%+v caveats=%v", res.SpanWindows, res.Caveats)
	}
}

func TestStreamSpanLocateParentLifecycleBoundaryWithholdsCompletedIncarnation(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, strings.Join([]string{
		traceMarkTestLine("reused", 42, 5.000, "B|42|completed before reuse"),
		traceMarkTestLine("reused", 42, 5.050, "E"),
		` creator-20 (20) [000] .... 5.100000: sched_wakeup_new: comm=reused pid=42 prio=120 target_cpu=000`,
	}, "\n"))
	res, err := StreamSpanLocate(context.Background(), path, TraceFlavorAuto, Query{
		View: "span_window", SpanName: "completed before reuse", PID: 42,
		TimeStart: 4.99, TimeEnd: 5.11, TimeStartSet: true, TimeEndSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.SpanWindows) != 0 || !containsSubstring(res.Caveats, "thread_identity_fail_closed=true") {
		t.Fatalf("parent lifecycle conflict did not withhold completed old incarnation: spans=%+v caveats=%v", res.SpanWindows, res.Caveats)
	}
}

func TestStreamSpanLocateRecipePublishesTypedEndpointsAndSpan(t *testing.T) {
	path := writeWindowDiscoveryTrace(t, strings.Join([]string{
		traceMarkTestLine("app", 42, 10.000, "B|42|slow phase"),
		traceMarkTestLine("app", 42, 12.000, "E"),
	}, "\n"))
	res, err := StreamSpanLocate(context.Background(), path, TraceFlavorAuto, Query{
		View: "recipe", RecipeName: "span_locate", Pattern: "slow phase", PID: 42,
		TimeStart: 9.99, TimeEnd: 10.01, TimeStartSet: true, TimeEndSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Recipe == nil || res.Recipe.Name != "span_locate" || len(res.SpanWindows) != 1 || len(res.Events) != 2 || len(res.EvidencePack) < 1 {
		t.Fatalf("recipe typed handoff incomplete: %+v", res)
	}
}

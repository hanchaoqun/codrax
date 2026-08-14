package tracequery

import (
	"strings"
	"testing"
)

func TestSpanWindowDisclosesMatchingUnpairedBeginWithoutMintingDuration(t *testing.T) {
	idx := &Index{Events: []Event{
		{
			Line: 41, Ts: 32136.468701, Type: EventTraceMark,
			Comm: "com.tencent.mm", PID: 25827, TGID: 21690,
			SpanAction: "B", SpanPID: 21690, SpanName: "Choreographer#doFrame 8002384",
		},
	}, FirstTs: 32136.468701, LastTs: 32136.468701}

	spans, caveats := FindSpanWindows(idx, Query{
		PID: 25827, SpanName: "Choreographer#doFrame 8002384",
		TimeStart: 32136.460000, TimeEnd: 32136.480000,
	}, 8)
	if len(spans) != 0 {
		t.Fatalf("a begin marker alone must not mint a duration: %+v", spans)
	}
	joined := strings.Join(caveats, "\n")
	for _, want := range []string{
		"trace_span_begin_unpaired=true",
		"row_tid=25827",
		"trace_mark_span_pid=21690",
		"start_ts=32136.468701",
		"span duration and target-span causal scope remain unproven",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in caveats: %s", want, joined)
		}
	}
}

func TestSpanWindowDoesNotDisclosePaddingOnlyUnpairedBegin(t *testing.T) {
	idx := &Index{Events: []Event{
		{Line: 1, Ts: 0.900, Type: EventTraceMark, Comm: "worker", PID: 7, SpanAction: "B", SpanPID: 7, SpanName: "work"},
	}, FirstTs: 0.900, LastTs: 0.900}

	_, caveats := FindSpanWindows(idx, Query{PID: 7, SpanName: "work", TimeStart: 1.000, TimeEnd: 1.100}, 8)
	if strings.Contains(strings.Join(caveats, "\n"), "trace_span_begin_unpaired=true") {
		t.Fatalf("padding-only begin marker must remain context-only: %v", caveats)
	}
}

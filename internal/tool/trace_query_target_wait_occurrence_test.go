package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryTypedObservationsPublishCompleteTargetWaitOccurrenceRoster(t *testing.T) {
	account := &tracequery.TargetWindowStateAccount{
		Thread:                tracequery.ThreadRef{Comm: "main", PID: 59566},
		Window:                tracequery.TimeWindow{StartTs: 10, EndTs: 10.1},
		WindowMs:              100,
		RunningMs:             99.365,
		IOWaitMs:              0.635,
		TotalMs:               100,
		LineStart:             1,
		LineEnd:               3000,
		WaitOccurrenceStatus:  "complete",
		WaitOccurrenceTotal:   3,
		WaitOccurrenceEmitted: 3,
		WaitOccurrences: []tracequery.TargetWindowStateOccurrence{
			{Ordinal: 1, State: tracequery.StateIOWait, StartTs: 10.001, EndTs: 10.001138, DurationMs: 0.138, StartLine: 90, EndLine: 117, IOWait: true, IOWaitKnown: true, Caller: "sync_buffer_read_wi", ReasonLine: 118},
			{Ordinal: 2, State: tracequery.StateIOWait, StartTs: 10.002, EndTs: 10.002147, DurationMs: 0.147, StartLine: 225, EndLine: 249, IOWait: true, IOWaitKnown: true, Caller: "sync_buffer_read_wi", ReasonLine: 250},
			{Ordinal: 3, State: tracequery.StateIOWait, StartTs: 10.020, EndTs: 10.020350, DurationMs: 0.350, StartLine: 2500, EndLine: 2532, IOWait: true, IOWaitKnown: true, Caller: "sync_buffer_read_wi", ReasonLine: 2533},
		},
	}
	records := traceQueryTypedObservations(
		tracequery.Result{TargetWindowStates: account},
		"attached_trace.txt",
		"",
		"attached_trace.txt",
		"",
		time.Unix(1, 0),
	)
	var set *types.ObservationRecord
	var occurrences []types.ObservationRecord
	for i := range records {
		switch records[i].Predicate {
		case "target_window_wait_occurrences":
			set = &records[i]
		case "target_window_wait_occurrence":
			occurrences = append(occurrences, records[i])
		}
	}
	if set == nil || set.ResultCount == nil || *set.ResultCount != 3 ||
		set.Value != "3" || set.Object != "complete" {
		t.Fatalf("complete occurrence-set authority missing: %+v", set)
	}
	for _, want := range []string{
		"#1 state=io_wait 10.001000..10.001138 duration=0.138ms",
		"#2 state=io_wait 10.002000..10.002147 duration=0.147ms",
		"#3 state=io_wait 10.020000..10.020350 duration=0.350ms",
		"caller=sync_buffer_read_wi",
	} {
		if !strings.Contains(set.Summary, want) {
			t.Fatalf("occurrence roster summary missing %q: %s", want, set.Summary)
		}
	}
	if len(occurrences) != 3 {
		t.Fatalf("individual occurrence rows=%d, want 3: %+v", len(occurrences), occurrences)
	}
	var sum float64
	for i, occurrence := range occurrences {
		if occurrence.Span.StartTs == 0 || occurrence.Span.EndTs <= occurrence.Span.StartTs ||
			occurrence.Object == "" || !strings.Contains(occurrence.Object, "iowait=1") {
			t.Fatalf("occurrence %d lost typed interval/caller fields: %+v", i+1, occurrence)
		}
		sum += account.WaitOccurrences[i].DurationMs
	}
	if sum != account.IOWaitMs {
		t.Fatalf("fixture occurrence Σ %.3f != account io_wait %.3f", sum, account.IOWaitMs)
	}
}

func TestTraceQueryTypedObservationsPublishMeasuredZeroWaitRoster(t *testing.T) {
	account := &tracequery.TargetWindowStateAccount{
		Thread:               tracequery.ThreadRef{Comm: "main", PID: 42},
		Window:               tracequery.TimeWindow{StartTs: 1, EndTs: 1.01},
		WindowMs:             10,
		RunningMs:            10,
		TotalMs:              10,
		WaitOccurrenceStatus: "complete",
	}
	records := traceQueryTypedObservations(
		tracequery.Result{TargetWindowStates: account},
		"trace.txt",
		"",
		"trace.txt",
		"",
		time.Unix(1, 0),
	)
	for _, record := range records {
		if record.Predicate != "target_window_wait_occurrences" {
			continue
		}
		if record.ResultCount == nil || *record.ResultCount != 0 ||
			record.Value != "0" || record.Object != "complete" {
			t.Fatalf("measured-zero wait roster was not typed: %+v", record)
		}
		return
	}
	t.Fatal("measured-zero target wait occurrence set missing")
}

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
	if strings.Contains(set.Summary, "roster=[") {
		t.Fatalf("occurrence roster must not ride the truncatable summary field: %s", set.Summary)
	}
	projected := types.ProjectObservationPromptRecords(
		[]types.ObservationRecord{*set},
		nil,
		nil,
		types.DefaultObservationPromptProjectionOptions(1),
	)
	if len(projected) != 1 || len(projected[0].Notes) != 5 {
		t.Fatalf("complete prompt occurrence roster must retain meta + sum + 3 rows: %+v", projected)
	}
	projectedNotes := strings.Join(projected[0].Notes, "\n")
	for _, want := range []string{
		"target_wait_occurrence_prompt=status=complete,emitted=3,total=3",
		"target_wait_occurrence_prompt_sum_ms=0.635",
		"#1 state=io_wait 10.001000..10.001138 duration=0.138ms",
		"#2 state=io_wait 10.002000..10.002147 duration=0.147ms",
		"#3 state=io_wait 10.020000..10.020350 duration=0.350ms",
		"caller=sync_buffer_read_wi",
	} {
		if !strings.Contains(projectedNotes, want) {
			t.Fatalf("prompt occurrence notes missing %q: %s", want, projectedNotes)
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

func TestTraceQueryTypedObservationsKeepCompleteEngineRosterSeparateFromCappedPromptPreview(t *testing.T) {
	account := &tracequery.TargetWindowStateAccount{
		Thread:                tracequery.ThreadRef{Comm: "CompThread_0", PID: 2955},
		Window:                tracequery.TimeWindow{StartTs: 10, EndTs: 10.1},
		WindowMs:              100,
		DStateMs:              11,
		TotalMs:               100,
		WaitOccurrenceStatus:  "complete",
		WaitOccurrenceTotal:   11,
		WaitOccurrenceEmitted: 11,
	}
	for ordinal := 1; ordinal <= 11; ordinal++ {
		start := 10 + float64(ordinal)*0.002
		account.WaitOccurrences = append(account.WaitOccurrences, tracequery.TargetWindowStateOccurrence{
			Ordinal: ordinal, State: tracequery.StateDSleep,
			StartTs: start, EndTs: start + 0.001, DurationMs: 1,
			StartLine: ordinal * 10, EndLine: ordinal*10 + 1,
			IOWaitKnown: true, Caller: "dma_fence_default_w", ReasonLine: ordinal*10 + 2,
		})
	}
	records := traceQueryTypedObservations(
		tracequery.Result{TargetWindowStates: account},
		"attached_trace.txt", "", "attached_trace.txt", "", time.Unix(1, 0),
	)
	var set *types.ObservationRecord
	leafCount := 0
	for i := range records {
		switch records[i].Predicate {
		case "target_window_wait_occurrences":
			set = &records[i]
		case "target_window_wait_occurrence":
			leafCount++
		}
	}
	if set == nil || set.Object != "complete" || set.Value != "11" ||
		set.ResultCount == nil || *set.ResultCount != 11 || leafCount != 11 {
		t.Fatalf("complete engine roster must retain all 11 typed leaves: set=%+v leaves=%d", set, leafCount)
	}
	notes := strings.Join(set.RichNotes, "\n")
	for _, want := range []string{
		"target_wait_occurrence_prompt=status=incomplete,emitted=8,total=11",
		"target_wait_occurrence_prompt_sum_ms=8.000",
		"target_wait_occurrence=#8 ",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("bounded prompt-preview contract missing %q:\n%s", want, notes)
		}
	}
	if strings.Contains(notes, "target_wait_occurrence=#9 ") {
		t.Fatalf("prompt preview must remain bounded without truncating the typed leaf roster:\n%s", notes)
	}
}

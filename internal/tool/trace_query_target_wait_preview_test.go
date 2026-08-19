package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQuerySummaryCarriesCompleteSmallTargetWaitRosterBeforeLongBody(t *testing.T) {
	result := tracequery.Result{
		View:      "thread_timeline",
		TimeStart: 34579.4,
		TimeEnd:   34579.6,
		TargetWindowStates: &tracequery.TargetWindowStateAccount{
			Thread:                tracequery.ThreadRef{Comm: "com.baidu.tieba", PID: 59566},
			Window:                tracequery.TimeWindow{StartTs: 34579.4, EndTs: 34579.6},
			WaitOccurrenceStatus:  "complete",
			WaitOccurrenceTotal:   3,
			WaitOccurrenceEmitted: 3,
			WaitOccurrences: []tracequery.TargetWindowStateOccurrence{
				{Ordinal: 1, State: tracequery.StateIOWait, StartTs: 34579.451701, EndTs: 34579.451839, DurationMs: 0.138, IOWaitKnown: true, IOWait: true, Caller: "sync_buffer_read_wi", StartLine: 91, EndLine: 118, ReasonLine: 119},
				{Ordinal: 2, State: tracequery.StateIOWait, StartTs: 34579.452934, EndTs: 34579.453081, DurationMs: 0.147, IOWaitKnown: true, IOWait: true, Caller: "sync_buffer_read_wi", StartLine: 226, EndLine: 250, ReasonLine: 251},
				{Ordinal: 3, State: tracequery.StateIOWait, StartTs: 34579.471372, EndTs: 34579.471722, DurationMs: 0.350, IOWaitKnown: true, IOWait: true, Caller: "sync_buffer_read_wi", StartLine: 2531, EndLine: 2532, ReasonLine: 2533},
			},
		},
		Timeline: &tracequery.TimelineResult{},
	}
	got := traceQuerySummary(result, traceQueryParams{}, "attached_trace", "/tmp/result.json")
	for _, want := range []string{
		"target_d_io_wait_occurrence_roster status=complete account_status=complete emitted=3 total=3 d_state=0 io_wait=3 sleep_iowait=0 other=0 wall_clock_sum=0.635ms",
		"includes=D|io_wait|S_with_iowait_1 excludes=ordinary_S_and_other_wait_mechanisms",
		"ordinal=1 state=io_wait window=34579.451701..34579.451839 duration=0.138ms iowait=1 caller=sync_buffer_read_wi",
		"ordinal=2 state=io_wait window=34579.452934..34579.453081 duration=0.147ms iowait=1 caller=sync_buffer_read_wi",
		"ordinal=3 state=io_wait window=34579.471372..34579.471722 duration=0.350ms iowait=1 caller=sync_buffer_read_wi",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing head-preview row %q:\n%s", want, got)
		}
	}
	previewAt := strings.Index(got, "target_d_io_wait_occurrence_roster status=complete")
	bodyAt := strings.Index(got, "## Thread timeline")
	if previewAt < 0 || bodyAt < 0 || previewAt > bodyAt {
		t.Fatalf("small target wait roster must precede long timeline body, preview=%d body=%d:\n%s", previewAt, bodyAt, got)
	}
	if strings.Contains(got, "target_d_io_wait_occurrence_roster_continuation") {
		t.Fatalf("complete small roster was falsely marked truncated:\n%s", got)
	}
}

func TestTraceQuerySummaryMarksLargeTargetWaitRosterIncomplete(t *testing.T) {
	occurrences := make([]tracequery.TargetWindowStateOccurrence, 10)
	for i := range occurrences {
		occurrences[i] = tracequery.TargetWindowStateOccurrence{
			Ordinal: i + 1, State: tracequery.StateDSleep,
			StartTs: float64(i + 1), EndTs: float64(i+1) + 0.001, DurationMs: 1,
		}
	}
	result := tracequery.Result{
		View: "thread_timeline",
		TargetWindowStates: &tracequery.TargetWindowStateAccount{
			WaitOccurrenceStatus: "complete", WaitOccurrenceTotal: 10,
			WaitOccurrenceEmitted: 10, WaitOccurrences: occurrences,
		},
	}
	got := traceQuerySummary(result, traceQueryParams{}, "attached_trace", "/tmp/large.json")
	for _, want := range []string{
		"target_d_io_wait_occurrence_roster status=incomplete account_status=complete emitted=8 total=10",
		"target_d_io_wait_occurrence_roster_continuation omitted=2 payload_ref=/tmp/large.json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("large roster disclosure missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "ordinal=9 ") {
		t.Fatalf("large roster exceeded head-preview cap:\n%s", got)
	}
}

func TestTraceQuerySummaryKeepsIncompleteAccountCountsAsObservedLowerBound(t *testing.T) {
	result := tracequery.Result{
		View: "thread_timeline",
		TargetWindowStates: &tracequery.TargetWindowStateAccount{
			WaitOccurrenceStatus:  "incomplete",
			WaitOccurrenceTotal:   40,
			WaitOccurrenceEmitted: 1,
			WaitOccurrences: []tracequery.TargetWindowStateOccurrence{{
				Ordinal: 1, State: tracequery.StateIOWait, StartTs: 1, EndTs: 1.001,
				DurationMs: 1, IOWaitKnown: true, IOWait: true,
			}},
		},
	}
	got := traceQuerySummary(result, traceQueryParams{}, "attached_trace", "/tmp/incomplete.json")
	for _, want := range []string{
		"status=incomplete account_status=incomplete emitted=1 total=40",
		"observed_d_state=0 observed_io_wait=1 observed_sleep_iowait=0 observed_other=0 observed_wall_clock_sum=1.000ms",
		"target_d_io_wait_occurrence_roster_continuation omitted=39 payload_ref=/tmp/incomplete.json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("incomplete account lost lower-bound caliber %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, " io_wait=1 ") || strings.Contains(got, " wall_clock_sum=1.000ms ") {
		t.Fatalf("incomplete account falsely published exact count/sum fields:\n%s", got)
	}
}

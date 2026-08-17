package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryTypedIOLatencyObservationsPreserveBothNonAdditiveRulers(t *testing.T) {
	stats := tracequery.WindowStats{
		Window: tracequery.TimeWindow{StartTs: 10, EndTs: 11},
		IOLatencies: []tracequery.IOLatencySummary{{
			EndpointFamily: "block_rq", Dev: "12,80", Op: "RCVHS", Sector: 923339752, Len: 64,
			IssueThread:    tracequery.ThreadRef{Comm: "com.tencent.mm", PID: 25827},
			CompleteThread: tracequery.ThreadRef{Comm: "udk-irq-4-80", PID: 2},
			IssueTs:        10.100000, CompleteTs: 10.101347, DurationMs: 1.347,
			WaitCaliber:          tracequery.BlockIOWaitCaliberIssueToComplete,
			CompletionWokeIssuer: true,
			IssuerBlockedStartTs: 10.100010, IssuerBlockedEndTs: 10.101347, IssuerBlockedMs: 1.337,
			IssuerBlockedLine: 1944839, IssuerBlockedState: "s_sleep",
			CausalWaitCaliber: tracequery.BlockIOCausalWaitCaliberCompletionClosedIssuerBlocked,
			WakeupTs:          10.101347, WakeupLine: 1944907,
			IssueLine: 1944838, CompleteLine: 1944906,
		}},
		IOLatencyOverflowCount:     190,
		IOLatencyOverflowRequestMs: 41.329,
	}
	rows := traceQueryTypedIOLatencyObservations(stats, types.ObservationSourceRef{}, "io", "now")
	if len(rows) != 2 {
		t.Fatalf("expected one exact pair plus coverage row, got %+v", rows)
	}
	pair := rows[0]
	if pair.Predicate != "io_latency" || pair.Subject != "com.tencent.mm-25827" || pair.Value != "1.347" || pair.Unit != "ms" {
		t.Fatalf("exact IO pair identity/value drifted: %+v", pair)
	}
	notes := strings.Join(pair.RichNotes, "\n")
	for _, want := range []string{
		"io_endpoint_family=block_rq",
		"io_sector=923339752",
		"io_len=64",
		"io_issue_thread=com.tencent.mm-25827",
		"io_complete_ts=10.101347",
		"request_residence=1.347",
		"request_residence_caliber=block_rq_issue_to_complete",
		"request_residence_clock_scope=single_request_elapsed_wall_clock_not_target_blocking",
		"completion_woke_issuer=true",
		"issuer_blocked=1.337",
		"causal_wait_caliber=completion_closed_issuer_blocked",
		"issuer_blocked_clock_scope=target_blocking_elapsed_wall_clock",
		"non_additive_with_issuer_blocked=true",
		"capacity_truncated=true",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("typed IO pair missing %q: %+v", want, pair.RichNotes)
		}
	}
	coverage := rows[1]
	if coverage.Predicate != "io_latency_coverage" || coverage.ResultCount == nil || *coverage.ResultCount != 191 ||
		!strings.Contains(coverage.Summary, "41.329 request·ms is non-wall-clock and non-additive") {
		t.Fatalf("IO pair coverage/caliber drifted: %+v", coverage)
	}
	coverageNotes := strings.Join(coverage.RichNotes, "\n")
	for _, want := range []string{
		"io_latency_emitted=1",
		"io_latency_complete=false",
		"total=191",
		"io_latency_coverage_status=capacity_truncated",
		"io_latency_overflow_pairs=190",
		"io_latency_overflow_request_ms=41.329",
	} {
		if !strings.Contains(coverageNotes, want) {
			t.Fatalf("IO pair coverage missing %q: %+v", want, coverage.RichNotes)
		}
	}
}

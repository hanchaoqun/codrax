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
		"request_residence=1.347",
		"request_residence_caliber=block_rq_issue_to_complete",
		"completion_woke_issuer=true",
		"issuer_blocked=1.337",
		"causal_wait_caliber=completion_closed_issuer_blocked",
		"non_additive_with_issuer_blocked=true",
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
}

package tool

// trace_query_blocked_reason_census_test.go — 件1 census 根修 tool pins
// (修复轮, 2026-07-13): the pid-keyed per-caller census reaches the ledger
// as ONE typed record per thread — census note "sym×N(Σx.xxxms)/…" +
// explicit caller-overflow note; zero census → zero records.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryBlockedReasonCensusObservations(t *testing.T) {
	stats := tracequery.WindowStats{
		Window: tracequery.TimeWindow{StartTs: 34579.472865, EndTs: 34579.587805},
		BlockedReasonCensus: []tracequery.BlockedReasonPIDCensus{{
			Thread: tracequery.ThreadRef{PID: 60555, Comm: "ThreadPoolForeg"},
			Count:  19,
			Callers: []tracequery.BlockedReasonCensusCaller{
				{Caller: "fscache_page_wait_o", Count: 17, DelayTotalMs: 13.905},
				{Caller: "hmfs_read", Count: 1, DelayTotalMs: 0.145},
				{Caller: "hmfs_get_dnode", Count: 1},
				// 件B sync: a partial-delay caller arrives from the engine
				// with Σ withheld — the note renders count only.
				{Caller: "rwsem_down_read", Count: 2},
			},
		}, {
			Thread:         tracequery.ThreadRef{PID: 42, Comm: "capped"},
			Count:          9,
			Callers:        []tracequery.BlockedReasonCensusCaller{{Caller: "sym_a", Count: 7}},
			CallerOverflow: 2,
		}},
	}
	records := traceQueryTypedBlockedReasonCensusObservations(stats, types.ObservationSourceRef{ArtifactID: "tieba.systrace"}, "w1", "now")
	if len(records) != 2 {
		t.Fatalf("one census record per thread expected, got %d", len(records))
	}
	first := records[0]
	if first.Subject != "ThreadPoolForeg-60555" || first.Predicate != "blocked_reason_census" || first.Value != "19" {
		t.Fatalf("census record identity drifted: %+v", first)
	}
	joined := strings.Join(first.RichNotes, "\n")
	// Per-caller FULL enumeration (E1-F1: the old feed carried top-1 only) —
	// Σms rides only the delay-complete callers.
	if !strings.Contains(joined, types.TraceNoteKeyBlockedReasonCensus+"=fscache_page_wait_o×17(Σ13.905ms)/hmfs_read×1(Σ0.145ms)/hmfs_get_dnode×1/rwsem_down_read×2") {
		t.Fatalf("census note must carry the full per-caller enumeration:\n%s", joined)
	}
	if strings.Contains(joined, "hmfs_get_dnode×1(") || strings.Contains(joined, "rwsem_down_read×2(") {
		t.Fatalf("a delay-less or partial-delay caller must not publish a Σms:\n%s", joined)
	}
	second := records[1]
	if !strings.Contains(strings.Join(second.RichNotes, "\n"), types.TraceNoteKeyBlockedReasonCensusOverflow+"=2") {
		t.Fatalf("the caller overflow must ride its typed note: %+v", second.RichNotes)
	}
	// Zero census → zero records (zero-emission anti-noise).
	if got := traceQueryTypedBlockedReasonCensusObservations(tracequery.WindowStats{}, types.ObservationSourceRef{}, "w1", "now"); len(got) != 0 {
		t.Fatalf("empty census must mint nothing, got %+v", got)
	}
}

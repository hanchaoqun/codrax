package tool

// trace_query_lock_single_publication_test.go — BLK §15.C ① (q6 东湖 doFrame
// 复跑, ledger real_trace_campaign_20260705.md §15.C): ONE physical
// monitor_contention span must publish exactly ONE observation. Before the
// fold, the resolved lock rank row (subject=holder, E3) and its
// critical_blocking twin (subject=waiter, E1) — both carved from the SAME
// collectBlockingSpanRows candidate, identical line range :45696-79136 —
// published as two crossed-direction 112.223ms rows: the user-facing "双向锁"
// (mutual-deadlock) misdirection. The rank record is the single publication;
// the twin's display-exclusive note families are ported onto it; and every
// fallback lane (no rank result / rank row beyond the family cap) keeps the
// critical_blocking twin publishing so the span is never lost entirely.

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func blkLockSinglePublicationFixture() tracequery.Result {
	window := tracequery.TimeWindow{StartTs: 33872.290666, EndTs: 33872.409727}
	waiter := tracequery.ThreadRef{Comm: ".ugc.aweme.lite", PID: 16547}
	holder := tracequery.ThreadRef{Comm: "#RxComputationT", PID: 16816}
	return tracequery.Result{
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: window,
			Items: []tracequery.RootCauseRankItem{{
				Rank: 1, Tier: "primary", Type: "blocking_span",
				Thread:   holder,
				ImpactMs: 112.223, CumulativeImpactMs: 112.223, EffectiveImpactMs: 112.223,
				Score: 0.9, Confidence: 0.62,
				LineStart: 45696, LineEnd: 79136,
				Source:    "window_stats.trace_spans.lock_contention",
				Causality: "on_wakeup_chain", ChainRelevance: "on_chain", ChainDepth: 1,
				BlockingKind: "monitor_contention", BlockingPeer: waiter,
				HolderSite:          "AssetManager.list(AssetManager.java:1258)",
				HolderSource:        tracequery.CounterpartSourceContentionPayload,
				SubjectIsLockHolder: true,
				Summary:             "lock holder #RxComputationT-16816 blocked .ugc.aweme.lite-16547 for 112.223ms",
			}},
		},
		CriticalBlocking: &tracequery.CriticalBlockingResult{
			Window: window,
			Items: []tracequery.CriticalBlockingCandidate{
				{
					// The waiter-subject twin of the SAME physical span.
					Type: "blocking_span", Thread: waiter, Peer: holder,
					BlockingKind: "monitor_contention",
					HolderSite:   "AssetManager.list(AssetManager.java:1258)",
					Waiters:      2,
					PeerState: &tracequery.ThreadStateBreakdown{
						DominantState: string(tracequery.StateRunning), TotalMs: 58.9, RunningMs: 58.9,
					},
					DurationMs: 112.223, StartTs: 33872.290666, EndTs: 33872.402889,
					LineStart: 45696, LineEnd: 79136,
					Confidence: 0.62, Summary: "monitor contention with owner #RxComputationT",
				},
				{
					// Unrelated counterpart-lane row — must keep publishing.
					Type: "binder_wait", Thread: waiter,
					Peer:       tracequery.ThreadRef{Comm: "binder_peer", PID: 200},
					DurationMs: 5.5, LineStart: 80000, LineEnd: 80010,
					Confidence: 0.8, Summary: "binder wait",
				},
			},
		},
	}
}

func blkCollectRecords(t *testing.T, result tracequery.Result) []types.ObservationRecord {
	t.Helper()
	records := traceQueryTypedObservations(result, "q6.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
	if len(records) == 0 {
		t.Fatal("fixture produced no observation records")
	}
	return records
}

func blkRecordsByPredicate(records []types.ObservationRecord, predicate string) []types.ObservationRecord {
	var out []types.ObservationRecord
	for _, record := range records {
		if strings.TrimSpace(record.Predicate) == predicate {
			out = append(out, record)
		}
	}
	return out
}

func blkNotesContain(record types.ObservationRecord, note string) bool {
	for _, n := range record.RichNotes {
		if n == note {
			return true
		}
	}
	return false
}

// TestBLKLockSpanPublishesExactlyOnce — §15.C ① core: the rank record is the
// single publication of the physical lock span; the crossed-direction
// critical_blocking twin is skipped; unrelated blocking rows keep publishing.
func TestBLKLockSpanPublishesExactlyOnce(t *testing.T) {
	records := blkCollectRecords(t, blkLockSinglePublicationFixture())

	lockObservations := 0
	for _, record := range records {
		if blkNotesContain(record, "blocking_kind=monitor_contention") &&
			record.Span.LineStart == 45696 && record.Span.LineEnd == 79136 {
			lockObservations++
			if !strings.HasPrefix(strings.TrimSpace(record.Predicate), "root_cause_") {
				t.Fatalf("the surviving lock observation must be the rank record, got predicate %q (id %s)", record.Predicate, record.ID)
			}
			if !blkNotesContain(record, "subject_is_lock_holder=true") {
				t.Fatalf("surviving rank record must keep the holder-subject lane, notes: %v", record.RichNotes)
			}
		}
	}
	if lockObservations != 1 {
		t.Fatalf("ONE physical lock span must publish exactly ONE observation, got %d", lockObservations)
	}

	blocking := blkRecordsByPredicate(records, "critical_blocking")
	if len(blocking) != 1 {
		t.Fatalf("only the unrelated binder row may publish on the critical_blocking lane, got %d records", len(blocking))
	}
	if blocking[0].ClaimKey != "critical_blocking:binder_wait" {
		t.Fatalf("unrelated binder row must keep publishing, got %q", blocking[0].ClaimKey)
	}
}

// TestBLKLockRankRecordPortsTwinDisplayNotes — §15.C ① richer-form-survives:
// the suppressed twin's display-exclusive families (waiters / peer-state)
// ride the surviving rank record; keys the rank record already publishes
// keep the rank row's own values (peer= stays the blocked waiter of the
// holder subject, exactly once).
func TestBLKLockRankRecordPortsTwinDisplayNotes(t *testing.T) {
	records := blkCollectRecords(t, blkLockSinglePublicationFixture())
	rank := blkRecordsByPredicate(records, "root_cause_primary")
	if len(rank) != 1 {
		t.Fatalf("expected exactly one primary rank record, got %d", len(rank))
	}
	record := rank[0]
	for _, expected := range []string{"waiters=2", "peer_state_dominant=running"} {
		if !blkNotesContain(record, expected) {
			t.Fatalf("rank record must port the twin's display note %q, notes: %v", expected, record.RichNotes)
		}
	}
	peers := 0
	for _, n := range record.RichNotes {
		if strings.HasPrefix(n, "peer=") {
			peers++
			if n != "peer=.ugc.aweme.lite-16547" {
				t.Fatalf("rank record's peer= must stay its OWN counterpart (the waiter), got %q", n)
			}
		}
	}
	if peers != 1 {
		t.Fatalf("rank record must carry exactly one peer= note, got %d", peers)
	}
}

// TestBLKLockNextStepNamesTheTrueHolderOnce — §15.C ③ end-to-end (records →
// ledger → compiled projection → next-step hints): the q6 shape yields
// exactly ONE holder drilldown hint and it names the TRUE holder
// (#RxComputationT-16816, the rank node's subject) — never the waiter
// (.ugc.aweme.lite-16547), which is what next-step-1 did before the fix.
func TestBLKLockNextStepNamesTheTrueHolderOnce(t *testing.T) {
	records := blkCollectRecords(t, blkLockSinglePublicationFixture())
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{ToolResults: []types.ToolResult{{
		ToolName: "trace_query", Success: true, Observations: records,
	}}})
	hints := runtimeTraceNextStepResolvedPeerHints(ledger, true)
	holderHints := 0
	for _, hint := range hints {
		if strings.Contains(hint, "对持有者 .ugc.aweme.lite-16547") {
			t.Fatalf("next-step must never name the WAITER as 持有者: %q", hint)
		}
		if strings.Contains(hint, "对持有者 #RxComputationT-16816") {
			holderHints++
			if !strings.Contains(hint, "AssetManager.list(AssetManager.java:1258)") {
				t.Fatalf("holder hint must keep the holding site: %q", hint)
			}
		}
	}
	if holderHints != 1 {
		t.Fatalf("exactly ONE holder drilldown hint must survive, got %d (hints: %v)", holderHints, hints)
	}
}

// TestBLKLockTwinPublishesWithoutRankRow — §15.C ① fallback: a result without
// a rank lane (e.g. a standalone critical_blocking view) keeps the
// waiter-subject twin publishing — the fold never loses the span entirely.
func TestBLKLockTwinPublishesWithoutRankRow(t *testing.T) {
	fixture := blkLockSinglePublicationFixture()
	fixture.RootCauseRank = nil
	records := blkCollectRecords(t, fixture)
	blocking := blkRecordsByPredicate(records, "critical_blocking")
	if len(blocking) != 2 {
		t.Fatalf("without a rank lane both blocking rows must publish, got %d", len(blocking))
	}
}

// TestBLKLockTwinPublishesWhenRankRowBeyondCap — §15.C ① fallback: a rank
// lock row cut by the typed family row cap emits nothing, so it must not
// suppress its twin.
func TestBLKLockTwinPublishesWhenRankRowBeyondCap(t *testing.T) {
	fixture := blkLockSinglePublicationFixture()
	lockRow := fixture.RootCauseRank.Items[0]
	var padded []tracequery.RootCauseRankItem
	for i := 0; i < traceQueryWidthTypedFamilyRowCap(); i++ {
		padded = append(padded, tracequery.RootCauseRankItem{
			Rank: i + 1, Tier: "adjacent", Type: "runnable_wait",
			Thread:   tracequery.ThreadRef{Comm: "pad", PID: 900 + i},
			ImpactMs: 1, LineStart: 100 + i, LineEnd: 100 + i,
			Confidence: 0.5, Summary: "pad row",
		})
	}
	lockRow.Rank = len(padded) + 1
	fixture.RootCauseRank.Items = append(padded, lockRow)

	records := blkCollectRecords(t, fixture)
	suppressedRankPublished := false
	for _, record := range records {
		if strings.HasPrefix(strings.TrimSpace(record.Predicate), "root_cause_") &&
			blkNotesContain(record, "blocking_kind=monitor_contention") {
			suppressedRankPublished = true
		}
	}
	if suppressedRankPublished {
		t.Fatal("the beyond-cap rank lock row must not have published (test premise broken)")
	}
	blocking := blkRecordsByPredicate(records, "critical_blocking")
	found := false
	for _, record := range blocking {
		if blkNotesContain(record, "blocking_kind=monitor_contention") {
			found = true
		}
	}
	if !found {
		t.Fatal("with the rank lock row beyond the cap, the critical_blocking twin must keep publishing")
	}
}

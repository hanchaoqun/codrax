package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// trace_query_q4_bundle_test.go — P0-E1 tool-face mutation pins (ledger
// docs/design/real_trace_campaign_20260705.md §12):
//
//   Q4-K 修1  bundle HEAD region names the top blocking candidates with typed
//             contention semantics (kind/owner/holder_site/duration) instead
//             of a bare blocking=%d count, anchored ahead of the full
//             blocking section (head-24KB blob zone).
//   RCX①     largest-measured-impact undrilled disclosure line + drill_status
//             riding the blocking/rank observation notes and text faces.
//   Q4-A/B   lock-lane rank rows publish blocking_kind/peer/holder_site notes
//             (registered blocking-family keys) and the inherited annotation
//             note inherited_target_blocked_ms.

func q4BundleFixtureResult() tracequery.Result {
	waiter := tracequery.ThreadRef{Comm: "app", PID: 100}
	holder := tracequery.ThreadRef{Comm: "NetworkKit_AssetsUtil_Operate_0", PID: 42067}
	blocking := &tracequery.CriticalBlockingResult{
		Window: tracequery.TimeWindow{StartTs: 5.0, EndTs: 5.115},
		Items: []tracequery.CriticalBlockingCandidate{
			{ // generic row with the LARGER duration but no contention lane —
				// the head block must still lead with the resolved lock row.
				Type:       "blocked_reason",
				Thread:     tracequery.ThreadRef{Comm: "io-thread", PID: 900},
				DurationMs: 200,
				Confidence: 0.82,
				Summary:    "io-thread sched_blocked_reason iowait=1 caller=f2fs count=1",
			},
			{
				Type:           "blocking_span",
				Thread:         waiter,
				Peer:           holder,
				BlockingKind:   "monitor_contention",
				HolderSite:     "AssetManager.list(AssetManager.java:1258)",
				Waiters:        1,
				DrillStatus:    tracequery.DrillStatusUndrilledPeerKnown,
				ChainRelevance: "on_chain",
				DurationMs:     112.223,
				StartTs:        5.0,
				EndTs:          5.112223,
				LineStart:      1,
				LineEnd:        5,
				Confidence:     0.72,
				Summary:        "blocking-like trace span lasted 112.223ms blocking_kind=monitor_contention owner=NetworkKit_AssetsUtil_Operate_0-42067",
			},
		},
	}
	rank := &tracequery.RootCauseRankResult{
		Window: tracequery.TimeWindow{StartTs: 5.0, EndTs: 5.115},
		Items: []tracequery.RootCauseRankItem{
			{
				Rank:               1,
				Tier:               "primary",
				Type:               "blocking_span",
				Thread:             holder,
				BlockingKind:       "monitor_contention",
				BlockingPeer:       waiter,
				HolderSite:         "AssetManager.list(AssetManager.java:1258)",
				DrillStatus:        tracequery.DrillStatusUndrilledPeerKnown,
				ImpactMs:           112.223,
				CumulativeImpactMs: 112.223,
				Score:              64.6,
				Confidence:         0.72,
				ChainRelevance:     "on_chain",
				Causality:          "on_wakeup_chain",
				LineStart:          1,
				LineEnd:            5,
				Summary:            "lock holder NetworkKit_AssetsUtil_Operate_0-42067 blocked app-100 for 112.223ms",
			},
			{
				Rank:                     2,
				Tier:                     "secondary",
				Type:                     "io_latency",
				Thread:                   tracequery.ThreadRef{Comm: "worker", PID: 200},
				ImpactMs:                 50,
				CumulativeImpactMs:       50,
				InheritedTargetBlockedMs: 112.175,
				Score:                    49.4,
				Confidence:               0.86,
				ChainRelevance:           "on_chain",
				Causality:                "on_wakeup_chain",
				LineStart:                7,
				LineEnd:                  9,
				Summary:                  "block IO took 50.000ms",
			},
		},
	}
	bundle := &tracequery.FrameRootCauseBundle{
		Target:           waiter,
		Window:           tracequery.TimeWindow{StartTs: 5.0, EndTs: 5.115},
		RootCauseRank:    rank,
		CriticalBlocking: blocking,
	}
	return tracequery.Result{
		View:                 "frame_root_cause_bundle",
		FrameRootCauseBundle: bundle,
		RootCauseRank:        rank,
		CriticalBlocking:     blocking,
	}
}

func TestBundleHeadNamesTopBlockingWithUndrilledDisclosure(t *testing.T) {
	summary := traceQuerySummary(q4BundleFixtureResult(), traceQueryParams{View: "frame_root_cause_bundle"}, "path", "/tmp/payload.json")

	lockLine := "- bundle_top_blocking type=blocking_span thread=app-100 kind=monitor_contention owner=NetworkKit_AssetsUtil_Operate_0-42067 holder_site=AssetManager.list(AssetManager.java:1258) duration=112.223ms chain_relevance=on_chain drill_status=undrilled_peer_known"
	if !strings.Contains(summary, lockLine) {
		t.Fatalf("bundle head must name the resolved lock candidate with typed semantics, got:\n%s", summary)
	}
	genericIdx := strings.Index(summary, "- bundle_top_blocking type=blocked_reason")
	lockIdx := strings.Index(summary, lockLine)
	if genericIdx < 0 {
		t.Fatalf("generic blocking row should still be listed in the head block:\n%s", summary)
	}
	if lockIdx > genericIdx {
		t.Fatalf("resolved on-chain contention must lead the head block even against a larger generic duration (lock=%d generic=%d)", lockIdx, genericIdx)
	}
	// RCX① disclosure: the largest measured drill-lane impact is undrilled.
	if !strings.Contains(summary, "- bundle_largest_impact_undrilled: app-100 112.223ms peer=NetworkKit_AssetsUtil_Operate_0-42067") {
		t.Fatalf("bundle head must disclose the largest undrilled impact, got:\n%s", summary)
	}
	// Head anchoring: the head block precedes the full blocking section.
	blockingSection := strings.Index(summary, "## Critical blocking calls")
	if blockingSection >= 0 && lockIdx > blockingSection {
		t.Fatalf("bundle_top_blocking must live in the head region before the blocking section (lock=%d section=%d)", lockIdx, blockingSection)
	}
	// Blocking section rows carry the drill verdict on the text face too.
	if !strings.Contains(summary, " drill_status=undrilled_peer_known chain_relevance=on_chain") {
		t.Fatalf("blocking row text must carry drill_status, got:\n%s", summary)
	}
	// Rank text face: the indented lock-lane detail line.
	if !strings.Contains(summary, "  rank_blocking_detail rank=1 kind=monitor_contention peer=app-100 holder_site=AssetManager.list(AssetManager.java:1258) drill_status=undrilled_peer_known") {
		t.Fatalf("rank text must carry the lock-lane detail line, got:\n%s", summary)
	}
	if !strings.Contains(summary, "  rank_blocking_detail rank=2 inherited_target_blocked=112.175ms(annotation_only)") {
		t.Fatalf("rank text must disclose the inherited annotation as annotation-only, got:\n%s", summary)
	}
}

func TestTraceQueryObservationsCarryLockLaneAndDrillNotes(t *testing.T) {
	records := traceQueryTypedObservations(q4BundleFixtureResult(), "full.systrace", "payload-ref", "raw-ref", "", time.Unix(1751600000, 0).UTC())
	joinNotes := func(notes []string) string { return strings.Join(notes, "\n") }

	var lockRank, ioRank, blockingRow string
	for _, record := range records {
		switch {
		case record.Predicate == "root_cause_primary" && record.Object == "blocking_span":
			lockRank = joinNotes(record.RichNotes)
		case record.Predicate == "root_cause_secondary" && record.Object == "io_latency":
			ioRank = joinNotes(record.RichNotes)
		case record.Predicate == "critical_blocking" && strings.Contains(joinNotes(record.RichNotes), "blocking_kind=monitor_contention"):
			blockingRow = joinNotes(record.RichNotes)
		}
	}
	if lockRank == "" || ioRank == "" || blockingRow == "" {
		t.Fatalf("expected lock rank, io rank and blocking observations, got %d records", len(records))
	}
	for _, want := range []string{
		"blocking_kind=monitor_contention",
		"peer=app-100",
		"holder_site=AssetManager.list(AssetManager.java:1258)",
		"drill_status=undrilled_peer_known",
	} {
		if !strings.Contains(lockRank, want) {
			t.Fatalf("lock rank observation notes missing %q:\n%s", want, lockRank)
		}
	}
	if !strings.Contains(ioRank, "inherited_target_blocked_ms=112.175") {
		t.Fatalf("attributed resource rank observation must carry the inherited annotation note:\n%s", ioRank)
	}
	if strings.Contains(ioRank, "drill_status=") {
		t.Fatalf("rows without a counterpart lane must not claim a drill verdict:\n%s", ioRank)
	}
	if !strings.Contains(blockingRow, "drill_status=undrilled_peer_known") {
		t.Fatalf("critical_blocking observation must carry drill_status:\n%s", blockingRow)
	}
}

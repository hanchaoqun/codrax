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
					// The waiter-subject twin of the SAME physical span. Its
					// PeerState / PeerChain / PeerSource all describe the twin's
					// peer — the HOLDER — which on the rank record is the SUBJECT
					// (BLK-2 P1 re-keyed port; peer_source not ported at all).
					Type: "blocking_span", Thread: waiter, Peer: holder,
					BlockingKind: "monitor_contention",
					HolderSite:   "AssetManager.list(AssetManager.java:1258)",
					Waiters:      2,
					PeerSource:   tracequery.CounterpartSourceContentionPayload,
					WaitObject:   "monitor of AssetManager",
					PeerState: &tracequery.ThreadStateBreakdown{
						DominantState: string(tracequery.StateRunning), TotalMs: 58.9, RunningMs: 58.9,
					},
					PeerChain: &tracequery.PeerChainStep{
						Peer:                holder,
						State:               &tracequery.ThreadStateBreakdown{DominantState: string(tracequery.StateRunning), TotalMs: 58.9, RunningMs: 58.9},
						DirectBlocker:       tracequery.ThreadRef{Comm: "upstream", PID: 130},
						DirectBlockerState:  string(tracequery.StateSSleep),
						DirectBlockerSource: tracequery.CounterpartSourceWakeupEdge,
						Presumptive:         false, Confidence: 0.62, Summary: "continuation off holder",
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

// TestBLKLockRankRecordPortsTwinDisplayNotes — §15.C ① richer-form-survives,
// re-keyed to the rank record's orientation (BLK-2 P1 pin ①): the suppressed
// twin's display-exclusive families ride the surviving rank record, but the
// twin's PeerState/PeerChain describe the twin's peer — the HOLDER, i.e. THIS
// record's SUBJECT — so they port as subject_state_* / subject_chain_*, never
// as peer_state_* / peer_chain_* (which the evaluator teaches the LLM to pair
// with peer=, and peer= here is the WAITER: same-key porting minted the
// "等待方 running 主导" false fact). peer_source is not ported at all
// (holder-resolution origin — the rank row already publishes holder_source).
// Direction-neutral families (waiters / wait_object) keep their keys; peer=
// stays the blocked waiter of the holder subject, exactly once.
func TestBLKLockRankRecordPortsTwinDisplayNotes(t *testing.T) {
	records := blkCollectRecords(t, blkLockSinglePublicationFixture())
	rank := blkRecordsByPredicate(records, "root_cause_primary")
	if len(rank) != 1 {
		t.Fatalf("expected exactly one primary rank record, got %d", len(rank))
	}
	record := rank[0]
	for _, expected := range []string{
		"waiters=2",
		"wait_object=monitor of AssetManager",
		// The holder's state/continuation, spelled as the record's SUBJECT.
		"subject_state_dominant=running",
		"subject_state_running=58.900",
		"subject_chain_state=running",
		"subject_chain_blocker=upstream-130",
		// BLK-2 P2: the precise fold witness rides the surviving record.
		"lock_twin_folded=true",
	} {
		if !blkNotesContain(record, expected) {
			t.Fatalf("rank record must port the twin's display note %q, notes: %v", expected, record.RichNotes)
		}
	}
	for _, n := range record.RichNotes {
		// Pin ① negative half: NO peer_state_*/peer_chain_* note may ride a
		// holder-subject rank record (they would pair with peer=<waiter>), and
		// the twin's peer_source must not be ported under any spelling.
		if strings.HasPrefix(n, "peer_state_") || strings.HasPrefix(n, "peer_chain_") || strings.HasPrefix(n, "peer_source=") {
			t.Fatalf("rank record carries twin note %q under a peer-oriented key — on this record those families describe the SUBJECT (the holder), notes: %v", n, record.RichNotes)
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

// TestBLKLockTwinFoldDoesNotFakeMissingBlockingCoverage — BLK-2 P2 pin ②:
// when the folded twin was the window's ONLY critical_blocking row, the
// blocking dimension count is zero BY DESIGN — the coverage view must count
// the lock_twin_folded rank record as blocking coverage instead of reporting
// a "critical_blocking_calls" soft gap that pushes the LLM to re-run a query
// which structurally cannot add rows. The counterfactual half strips the
// typed marker and requires the gap back, so this pin exercises the marker
// mechanism, not a fixture accident.
func TestBLKLockTwinFoldDoesNotFakeMissingBlockingCoverage(t *testing.T) {
	fixture := blkLockSinglePublicationFixture()
	// Drop the unrelated binder row: the folded twin is the only blocking row.
	fixture.CriticalBlocking.Items = fixture.CriticalBlocking.Items[:1]
	records := blkCollectRecords(t, fixture)

	coverage := types.TraceObservationCoverageFromObservationRecords(records)
	if !coverage.Active {
		t.Fatal("coverage compile produced no active view — the pin is checking nothing")
	}
	for _, dimension := range coverage.Dimensions {
		if dimension.Dimension == types.TraceObservationDimensionCriticalBlocking {
			t.Fatalf("test premise broken: the twin must have folded, but the critical_blocking dimension has %d record(s)", dimension.Count)
		}
	}
	for _, missing := range coverage.SoftMissingDimensions {
		if missing == "critical_blocking_calls" {
			t.Fatalf("folded-twin window must not fake a critical_blocking_calls gap, soft missing: %v", coverage.SoftMissingDimensions)
		}
	}

	// Counterfactual: without the typed fold witness the gap MUST come back —
	// the coverage relief keys on the precise marker, nothing else.
	stripped := make([]types.ObservationRecord, 0, len(records))
	for _, record := range records {
		clone := record
		var notes []string
		for _, n := range record.RichNotes {
			if n == "lock_twin_folded=true" {
				continue
			}
			notes = append(notes, n)
		}
		clone.RichNotes = notes
		stripped = append(stripped, clone)
	}
	counterfactual := types.TraceObservationCoverageFromObservationRecords(stripped)
	found := false
	for _, missing := range counterfactual.SoftMissingDimensions {
		if missing == "critical_blocking_calls" {
			found = true
		}
	}
	if !found {
		t.Fatalf("counterfactual (marker stripped) must report the critical_blocking_calls gap, got: %v", counterfactual.SoftMissingDimensions)
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

// TestBLKLockRankSeatSurvivesTransportCap — §15.C ① + B1260: the engine
// has already bounded and seated the rank board, so the observation transport
// cap must preserve a positive-ordinal lock row and suppress its physical twin.
func TestBLKLockRankSeatSurvivesTransportCap(t *testing.T) {
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
	rankPublished := false
	for _, record := range records {
		if strings.HasPrefix(strings.TrimSpace(record.Predicate), "root_cause_") &&
			blkNotesContain(record, "blocking_kind=monitor_contention") {
			rankPublished = true
		}
	}
	if !rankPublished {
		t.Fatal("the beyond-cap engine-ranked lock row must survive the transport cap")
	}
	blocking := blkRecordsByPredicate(records, "critical_blocking")
	foundTwin := false
	for _, record := range blocking {
		if blkNotesContain(record, "blocking_kind=monitor_contention") {
			foundTwin = true
		}
	}
	if foundTwin {
		t.Fatal("the published rank lock row must suppress its physical critical_blocking twin")
	}
}

// TestBLKRankBlockingDetailNamesSubjectRoleHolder — BLK-2 P3b pin ③: the
// LLM-facing rank_blocking_detail text line names the row's lock orientation
// explicitly — subject_role=holder rides every holder-subject row so peer=
// can never be misread as the holder again; waiter-subject rows carry no
// subject_role token.
func TestBLKRankBlockingDetailNamesSubjectRoleHolder(t *testing.T) {
	item := tracequery.RootCauseRankItem{
		Rank: 1, Type: "blocking_span",
		BlockingKind:        "monitor_contention",
		BlockingPeer:        tracequery.ThreadRef{Comm: ".ugc.aweme.lite", PID: 16547},
		HolderSite:          "AssetManager.list(AssetManager.java:1258)",
		SubjectIsLockHolder: true,
	}
	var holderFace strings.Builder
	writeTraceRootCauseBlockingDetail(&holderFace, item)
	line := holderFace.String()
	if !strings.Contains(line, "subject_role=holder") {
		t.Fatalf("holder-subject detail line must carry subject_role=holder, got %q", line)
	}
	if !strings.Contains(line, "peer=.ugc.aweme.lite-16547") {
		t.Fatalf("detail line must keep naming the waiter as peer=, got %q", line)
	}

	item.SubjectIsLockHolder = false
	var waiterFace strings.Builder
	writeTraceRootCauseBlockingDetail(&waiterFace, item)
	if strings.Contains(waiterFace.String(), "subject_role=") {
		t.Fatalf("waiter-subject detail line must carry no subject_role token, got %q", waiterFace.String())
	}
}

// TestBLKLockFoldContractAsymmetryFailsOpenToDoublePublish — BLK-2 P3b pin ④
// (备案项 direction-safety pin): the §15.C ① fold folds ONLY on an exact
// physical-span key match (kind + exact line range + unordered thread pair).
// When the two faces of one physical span disagree by even one line, the keys
// diverge and the contract fails OPEN — BOTH faces publish (the pre-BLK
// double-publication shape) — never CLOSED (a suppressed twin whose notes were
// ported nowhere, i.e. silent span loss). If a future refactor fuzzes the key
// match or flips the fail direction, this pin is the tripwire.
func TestBLKLockFoldContractAsymmetryFailsOpenToDoublePublish(t *testing.T) {
	fixture := blkLockSinglePublicationFixture()
	// One-line asymmetry between the two views of the "same" physical span.
	fixture.CriticalBlocking.Items[0].LineEnd = fixture.RootCauseRank.Items[0].LineEnd + 1
	records := blkCollectRecords(t, fixture)

	var rankFace, blockingFace int
	for _, record := range records {
		if !blkNotesContain(record, "blocking_kind=monitor_contention") {
			continue
		}
		switch {
		case strings.HasPrefix(strings.TrimSpace(record.Predicate), "root_cause_"):
			rankFace++
			if blkNotesContain(record, "lock_twin_folded=true") {
				t.Fatalf("no fold happened — the rank record must not claim the fold witness, notes: %v", record.RichNotes)
			}
			for _, n := range record.RichNotes {
				if strings.HasPrefix(n, "subject_state_") || strings.HasPrefix(n, "subject_chain_") {
					t.Fatalf("no fold happened — nothing may have been ported onto the rank record, found %q", n)
				}
			}
		case strings.TrimSpace(record.Predicate) == "critical_blocking":
			blockingFace++
		}
	}
	if rankFace != 1 || blockingFace != 1 {
		t.Fatalf("key asymmetry must fail OPEN to double publication (rank=1 blocking=1), got rank=%d blocking=%d", rankFace, blockingFace)
	}
}

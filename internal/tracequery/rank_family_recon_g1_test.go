package tracequery

// rank_family_recon_g1_test.go — G1 跨车道对账 engine pins (§27.2-G1, user
// ruling 收口批准 §28.1, 2026-07-09, real_trace_campaign_20260705.md).
//
// End-to-end shapes are ENGINE-MINTED from trace text (§28.7 复核纪律②:
// fixture 应取引擎实铸形): the opendir_79 form (×6 io_latency family, raw sum
// 2.858ms, four udk-irq completer peers) and the huadong_79 form (×8, raw sum
// 15.156ms, five peers).
//
// MUTATION self-checks (each gate dimension has a pin that reds when the gate
// is deleted or loosened to a noisy signal — 对账门放宽到嘈声维 must bite):
//   - drop the window-equality arm            → TestG1ReconCrossWindowNegative;
//   - drop the thread-key arm                 → TestG1ReconDifferentThreadNegative;
//   - replace member-union containment with
//     the merged-row hull                     → TestG1ReconIntervalInsideHullGapNegative
//     (the probe interval sits INSIDE the hull but OUTSIDE the union);
//   - loosen the exact type-token map to a
//     substring/lane heuristic                → TestG1ReconNonAdjudicatedTypeNegative
//     + TestG1ReconTypeUniversePinned;
//   - drop the family requirement (absorb
//     against single rank rows)               → TestG1EngineSingleEventNoFamilyNegative;
//   - drop the reset-first idempotency        → TestG1EngineRunQueryFrameBundleIdempotent
//     (the double reconcile would double AbsorbedChainRows).

import (
	"math"
	"strings"
	"testing"
)

// g1OpendirShapeTrace: six block IOs issued by work-500, completed by FOUR
// distinct udk-irq peers; durations 5×0.500ms + 0.358ms = 2.858ms raw sum
// (the opendir_79 form). Disjoint intervals → sum_disjoint family caliber.
const g1OpendirShapeTrace = `
        work-500   (  500) [001] .... 10.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=work next_pid=500 next_prio=100
        work-500   (  500) [001] .... 10.100000: block_rq_issue: 8,0 R 4096 () 1000 + 8 [work]
      udk-irq-0-71   (    2) [000] .... 10.100500: block_rq_complete: 8,0 R () 1000 + 8 [0]
        work-500   (  500) [001] .... 10.110000: block_rq_issue: 8,0 R 4096 () 1008 + 8 [work]
      udk-irq-1-72   (    2) [000] .... 10.110500: block_rq_complete: 8,0 R () 1008 + 8 [0]
        work-500   (  500) [001] .... 10.120000: block_rq_issue: 8,0 R 4096 () 1016 + 8 [work]
      udk-irq-2-73   (    2) [000] .... 10.120500: block_rq_complete: 8,0 R () 1016 + 8 [0]
        work-500   (  500) [001] .... 10.130000: block_rq_issue: 8,0 R 4096 () 1024 + 8 [work]
      udk-irq-3-74   (    2) [000] .... 10.130500: block_rq_complete: 8,0 R () 1024 + 8 [0]
        work-500   (  500) [001] .... 10.140000: block_rq_issue: 8,0 R 4096 () 1032 + 8 [work]
      udk-irq-0-71   (    2) [000] .... 10.140500: block_rq_complete: 8,0 R () 1032 + 8 [0]
        work-500   (  500) [001] .... 10.150000: block_rq_issue: 8,0 R 4096 () 1040 + 8 [work]
      udk-irq-1-72   (    2) [000] .... 10.150358: block_rq_complete: 8,0 R () 1040 + 8 [0]
        work-500   (  500) [001] .... 10.900000: sched_switch: prev_comm=work prev_pid=500 prev_prio=100 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`

// g1HuadongShapeTrace: eight block IOs on one thread, FIVE distinct peers;
// durations 7×2.000ms + 1.156ms = 15.156ms raw sum (the huadong_79 form).
const g1HuadongShapeTrace = `
        hmfs-600   (  600) [002] .... 20.010000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=hmfs next_pid=600 next_prio=100
        hmfs-600   (  600) [002] .... 20.100000: block_rq_issue: 12,48 RS 4096 () 2000 + 8 [hmfs]
      udk-irq-0-81   (    2) [000] .... 20.102000: block_rq_complete: 12,48 RS () 2000 + 8 [0]
        hmfs-600   (  600) [002] .... 20.110000: block_rq_issue: 12,48 RS 4096 () 2008 + 8 [hmfs]
      udk-irq-1-82   (    2) [000] .... 20.112000: block_rq_complete: 12,48 RS () 2008 + 8 [0]
        hmfs-600   (  600) [002] .... 20.120000: block_rq_issue: 12,48 RS 4096 () 2016 + 8 [hmfs]
      udk-irq-2-83   (    2) [000] .... 20.122000: block_rq_complete: 12,48 RS () 2016 + 8 [0]
        hmfs-600   (  600) [002] .... 20.130000: block_rq_issue: 12,48 RS 4096 () 2024 + 8 [hmfs]
      udk-irq-3-84   (    2) [000] .... 20.132000: block_rq_complete: 12,48 RS () 2024 + 8 [0]
        hmfs-600   (  600) [002] .... 20.140000: block_rq_issue: 12,48 RS 4096 () 2032 + 8 [hmfs]
      udk-irq-4-85   (    2) [000] .... 20.142000: block_rq_complete: 12,48 RS () 2032 + 8 [0]
        hmfs-600   (  600) [002] .... 20.150000: block_rq_issue: 12,48 RS 4096 () 2040 + 8 [hmfs]
      udk-irq-0-81   (    2) [000] .... 20.152000: block_rq_complete: 12,48 RS () 2040 + 8 [0]
        hmfs-600   (  600) [002] .... 20.160000: block_rq_issue: 12,48 RS 4096 () 2048 + 8 [hmfs]
      udk-irq-1-82   (    2) [000] .... 20.162000: block_rq_complete: 12,48 RS () 2048 + 8 [0]
        hmfs-600   (  600) [002] .... 20.170000: block_rq_issue: 12,48 RS 4096 () 2056 + 8 [hmfs]
      udk-irq-2-83   (    2) [000] .... 20.171156: block_rq_complete: 12,48 RS () 2056 + 8 [0]
        hmfs-600   (  600) [002] .... 20.900000: sched_switch: prev_comm=hmfs prev_pid=600 prev_prio=100 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
`

// g1SingleEventTrace: ONE block IO — a single rank row is NOT a family row,
// so nothing may absorb (负向保护: peer 行渲染照旧).
const g1SingleEventTrace = `
        work-500   (  500) [001] .... 10.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=work next_pid=500 next_prio=100
        work-500   (  500) [001] .... 10.100000: block_rq_issue: 8,0 R 4096 () 1000 + 8 [work]
      udk-irq-0-71   (    2) [000] .... 10.100500: block_rq_complete: 8,0 R () 1000 + 8 [0]
        work-500   (  500) [001] .... 10.900000: sched_switch: prev_comm=work prev_pid=500 prev_prio=100 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`

func g1BuildIndex(t *testing.T, content string) *Index {
	t.Helper()
	return buildTraceIndex(t, "g1_recon.systrace", content)
}

func g1FamilyRow(t *testing.T, rank RootCauseRankResult) *RootCauseRankItem {
	t.Helper()
	var fam *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "io_latency" {
			if fam != nil {
				t.Fatalf("expected ONE folded io_latency family row, found a second: %+v", rank.Items[i])
			}
			fam = &rank.Items[i]
		}
	}
	if fam == nil {
		t.Fatalf("no io_latency rank row minted; rank items: %+v", rank.Items)
	}
	return fam
}

func TestG1EngineAbsorbsOpendirShape(t *testing.T) {
	idx := g1BuildIndex(t, g1OpendirShapeTrace)
	bundle := BuildFrameRootCauseBundle(idx, Query{PID: 500, TimeStart: 10.0, TimeEnd: 11.0})
	if bundle.RootCauseRank == nil || bundle.CriticalBlocking == nil {
		t.Fatal("bundle must carry both lanes")
	}
	fam := g1FamilyRow(t, *bundle.RootCauseRank)
	if fam.MemberCount != 6 {
		t.Fatalf("opendir shape must fold ×6, got member_count=%d", fam.MemberCount)
	}
	if fam.MemberFoldCaliber != RootCauseMemberFoldCaliberSumDisjoint {
		t.Fatalf("disjoint members must publish sum_disjoint, got %q", fam.MemberFoldCaliber)
	}
	if math.Abs(fam.CumulativeImpactMs-2.858) > 1e-6 {
		t.Fatalf("family combined value must be the 2.858ms raw sum, got %.6f", fam.CumulativeImpactMs)
	}
	if fam.RankFamilyKey == "" {
		t.Fatal("absorbing family row must carry its canonical rank_family_key")
	}
	if fam.AbsorbedChainRows != 6 {
		t.Fatalf("family must record 6 absorbed chain rows, got %d", fam.AbsorbedChainRows)
	}
	blocking := bundle.CriticalBlocking
	ioRows, peers := 0, map[string]bool{}
	for _, item := range blocking.Items {
		if item.Type != "io_latency" {
			continue
		}
		ioRows++
		peers[item.Peer.Comm] = true
		if !item.AbsorbedByRankFamily {
			t.Fatalf("io_latency blocking row must be absorbed: %+v", item)
		}
		if item.AbsorbedIntoFamily != fam.RankFamilyKey {
			t.Fatalf("absorbed_into %q must equal the family's rank_family_key %q", item.AbsorbedIntoFamily, fam.RankFamilyKey)
		}
	}
	// 观测照发不删: absorption marks rows, it never removes them.
	if ioRows != 6 {
		t.Fatalf("all 6 io_latency blocking rows must keep publishing, got %d", ioRows)
	}
	if len(peers) != 4 {
		t.Fatalf("opendir shape carries FOUR distinct completer peers, got %v", peers)
	}
	for peer := range peers {
		if !strings.HasPrefix(peer, "udk-irq-") {
			t.Fatalf("unexpected peer %q", peer)
		}
	}
}

func TestG1EngineAbsorbsHuadongShape(t *testing.T) {
	idx := g1BuildIndex(t, g1HuadongShapeTrace)
	bundle := BuildFrameRootCauseBundle(idx, Query{PID: 600, TimeStart: 20.0, TimeEnd: 21.0})
	if bundle.RootCauseRank == nil || bundle.CriticalBlocking == nil {
		t.Fatal("bundle must carry both lanes")
	}
	fam := g1FamilyRow(t, *bundle.RootCauseRank)
	if fam.MemberCount != 8 {
		t.Fatalf("huadong shape must fold ×8, got member_count=%d", fam.MemberCount)
	}
	if math.Abs(fam.CumulativeImpactMs-15.156) > 1e-6 {
		t.Fatalf("family combined value must be the 15.156ms raw sum, got %.6f", fam.CumulativeImpactMs)
	}
	if fam.AbsorbedChainRows != 8 {
		t.Fatalf("family must record 8 absorbed chain rows, got %d", fam.AbsorbedChainRows)
	}
	peers := map[string]bool{}
	for _, item := range bundle.CriticalBlocking.Items {
		if item.Type != "io_latency" {
			continue
		}
		if !item.AbsorbedByRankFamily || item.AbsorbedIntoFamily != fam.RankFamilyKey {
			t.Fatalf("huadong io_latency blocking row must absorb into the family: %+v", item)
		}
		peers[item.Peer.Comm] = true
	}
	if len(peers) != 5 {
		t.Fatalf("huadong shape carries FIVE distinct completer peers, got %v", peers)
	}
}

// TestG1EngineRunQueryFrameBundleIdempotent drives the RunQuery path: the
// frame bundle reconciles once inside BuildFrameRootCauseBundle and the
// post-switch pass reconciles the SAME backing slices again — reset-first
// recomputation must converge (a naive re-run would double AbsorbedChainRows
// to 12).
func TestG1EngineRunQueryFrameBundleIdempotent(t *testing.T) {
	idx := g1BuildIndex(t, g1OpendirShapeTrace)
	res := Run(idx, Query{View: "frame_root_cause_bundle", PID: 500, TimeStart: 10.0, TimeEnd: 11.0})
	if res.RootCauseRank == nil || res.CriticalBlocking == nil {
		t.Fatal("frame bundle result must expose both lanes")
	}
	fam := g1FamilyRow(t, *res.RootCauseRank)
	if fam.AbsorbedChainRows != 6 {
		t.Fatalf("reset-first idempotency: absorbed count must stay 6 after the double reconcile, got %d", fam.AbsorbedChainRows)
	}
	absorbed := 0
	for _, item := range res.CriticalBlocking.Items {
		if item.Type == "io_latency" && item.AbsorbedByRankFamily {
			absorbed++
		}
	}
	if absorbed != 6 {
		t.Fatalf("expected 6 absorbed blocking rows on the RunQuery face, got %d", absorbed)
	}
}

// TestG1EngineSingleEventNoFamilyNegative: one io_latency event mints a
// SINGLE rank row (never family-folded) — no absorption may fire (§27.2-G1
// scope: the ruling reconciles against FAMILY rows only; 负向保护).
func TestG1EngineSingleEventNoFamilyNegative(t *testing.T) {
	idx := g1BuildIndex(t, g1SingleEventTrace)
	bundle := BuildFrameRootCauseBundle(idx, Query{PID: 500, TimeStart: 10.0, TimeEnd: 11.0})
	fam := g1FamilyRow(t, *bundle.RootCauseRank)
	if fam.MemberCount >= 2 {
		t.Fatalf("single event must not family-fold, got member_count=%d", fam.MemberCount)
	}
	if fam.RankFamilyKey != "" || fam.AbsorbedChainRows != 0 {
		t.Fatalf("single rank row must never wear absorption marks: %+v", fam)
	}
	for _, item := range bundle.CriticalBlocking.Items {
		if item.AbsorbedByRankFamily || item.AbsorbedIntoFamily != "" {
			t.Fatalf("no family row → blocking row must stay unmarked: %+v", item)
		}
	}
}

// --- unit-level gate pins (hand-built rows; same package so the unexported
// familyMemberIntervals inventory can be staged directly) --------------------

func g1UnitFamily() RootCauseRankItem {
	fam := RootCauseRankItem{
		Type:               "io_latency",
		Thread:             ThreadRef{Comm: "work", PID: 500},
		MemberCount:        2,
		MemberFoldCaliber:  RootCauseMemberFoldCaliberSumDisjoint,
		CumulativeImpactMs: 1.0,
		StatsWindowStartTs: 10.0,
		StatsWindowEndTs:   11.0,
	}
	fam.familyMemberIntervals = []foldInterval{{start: 10.10, end: 10.20}, {start: 10.30, end: 10.40}}
	return fam
}

func g1UnitBlocking(window TimeWindow, item CriticalBlockingCandidate) CriticalBlockingResult {
	return CriticalBlockingResult{Window: window, Items: []CriticalBlockingCandidate{item}}
}

func TestG1ReconUnitPositiveControl(t *testing.T) {
	rank := RootCauseRankResult{Items: []RootCauseRankItem{g1UnitFamily()}}
	blocking := g1UnitBlocking(TimeWindow{StartTs: 10.0, EndTs: 11.0}, CriticalBlockingCandidate{
		Type: "io_latency", Thread: ThreadRef{Comm: "work", PID: 500},
		StartTs: 10.10, EndTs: 10.20, DurationMs: 100,
	})
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if !blocking.Items[0].AbsorbedByRankFamily || rank.Items[0].AbsorbedChainRows != 1 {
		t.Fatalf("positive control must absorb: %+v / %+v", blocking.Items[0], rank.Items[0])
	}
	if blocking.Items[0].AbsorbedIntoFamily != rank.Items[0].RankFamilyKey || rank.Items[0].RankFamilyKey == "" {
		t.Fatalf("both sides must carry the SAME canonical key: %q vs %q",
			blocking.Items[0].AbsorbedIntoFamily, rank.Items[0].RankFamilyKey)
	}
	// Reset-first idempotency at the unit grain.
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if rank.Items[0].AbsorbedChainRows != 1 {
		t.Fatalf("double reconcile must converge to 1, got %d", rank.Items[0].AbsorbedChainRows)
	}
}

func TestG1ReconCrossWindowNegative(t *testing.T) {
	rank := RootCauseRankResult{Items: []RootCauseRankItem{g1UnitFamily()}}
	// Same thread, same type, interval inside the union — but the blocking
	// result was measured over a DIFFERENT query window. 跨窗不对账.
	blocking := g1UnitBlocking(TimeWindow{StartTs: 11.0, EndTs: 12.0}, CriticalBlockingCandidate{
		Type: "io_latency", Thread: ThreadRef{Comm: "work", PID: 500},
		StartTs: 10.10, EndTs: 10.20, DurationMs: 100,
	})
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if blocking.Items[0].AbsorbedByRankFamily || rank.Items[0].AbsorbedChainRows != 0 {
		t.Fatalf("cross-window rows must never reconcile: %+v", blocking.Items[0])
	}
}

func TestG1ReconDifferentThreadNegative(t *testing.T) {
	rank := RootCauseRankResult{Items: []RootCauseRankItem{g1UnitFamily()}}
	blocking := g1UnitBlocking(TimeWindow{StartTs: 10.0, EndTs: 11.0}, CriticalBlockingCandidate{
		Type: "io_latency", Thread: ThreadRef{Comm: "other", PID: 501},
		StartTs: 10.10, EndTs: 10.20, DurationMs: 100,
	})
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if blocking.Items[0].AbsorbedByRankFamily {
		t.Fatalf("different-thread rows must never reconcile: %+v", blocking.Items[0])
	}
}

// TestG1ReconIntervalInsideHullGapNegative is the anti-hull mutation pin: the
// probe interval [10.22,10.28] sits INSIDE the family hull (10.10–10.40) but
// in the GAP between the two members — a hull-based membership test (the
// noisy signal the design forbids) would absorb it; the member-union test
// must not.
func TestG1ReconIntervalInsideHullGapNegative(t *testing.T) {
	rank := RootCauseRankResult{Items: []RootCauseRankItem{g1UnitFamily()}}
	blocking := g1UnitBlocking(TimeWindow{StartTs: 10.0, EndTs: 11.0}, CriticalBlockingCandidate{
		Type: "io_latency", Thread: ThreadRef{Comm: "work", PID: 500},
		StartTs: 10.22, EndTs: 10.28, DurationMs: 60,
	})
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if blocking.Items[0].AbsorbedByRankFamily {
		t.Fatal("an interval in the hull GAP is not a member — hull-based absorption is the forbidden noisy gate")
	}
}

// TestG1ReconNonAdjudicatedTypeNegative bites the token-loosening mutation:
// a d_state_or_io_wait blocking row with matching thread/window/interval must
// NOT absorb — only the adjudicated io_latency↔io_latency pair reconciles
// (其他族缺 witness, 立账观察).
func TestG1ReconNonAdjudicatedTypeNegative(t *testing.T) {
	rank := RootCauseRankResult{Items: []RootCauseRankItem{g1UnitFamily()}}
	blocking := g1UnitBlocking(TimeWindow{StartTs: 10.0, EndTs: 11.0}, CriticalBlockingCandidate{
		Type: "d_state_or_io_wait", Thread: ThreadRef{Comm: "work", PID: 500},
		StartTs: 10.10, EndTs: 10.20, DurationMs: 100,
	})
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if blocking.Items[0].AbsorbedByRankFamily {
		t.Fatal("non-adjudicated type must never reconcile against the io_latency family")
	}
}

func TestG1ReconMissingIntervalNegative(t *testing.T) {
	rank := RootCauseRankResult{Items: []RootCauseRankItem{g1UnitFamily()}}
	blocking := g1UnitBlocking(TimeWindow{StartTs: 10.0, EndTs: 11.0}, CriticalBlockingCandidate{
		Type: "io_latency", Thread: ThreadRef{Comm: "work", PID: 500},
		DurationMs: 100, // no typed StartTs/EndTs — membership unprovable
	})
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if blocking.Items[0].AbsorbedByRankFamily {
		t.Fatal("a row without typed interval identity must never absorb (absence never guesses)")
	}
}

// TestG1ReconLaneSplitFamiliesDistinctKeysP2a (收尾 P2-a, 对抗复核 2026-07-09):
// the family fold splits same-(thread,type,window) rows into per-LANE
// families — the recon key must carry that lane dimension (同构 with the fold
// key), or two lane-split families share one key and the display's
// first-claimed family row swallows both families' peers. Each absorbed row
// must point at ITS OWN family's key.
func TestG1ReconLaneSplitFamiliesDistinctKeysP2a(t *testing.T) {
	famOn := g1UnitFamily()
	famOn.ChainRelevance = "on_chain"
	famOn.familyMemberIntervals = []foldInterval{{start: 10.10, end: 10.20}, {start: 10.30, end: 10.40}}
	famBg := g1UnitFamily()
	famBg.ChainRelevance = "background"
	famBg.familyMemberIntervals = []foldInterval{{start: 10.50, end: 10.60}, {start: 10.70, end: 10.80}}
	rank := RootCauseRankResult{Items: []RootCauseRankItem{famOn, famBg}}
	blocking := CriticalBlockingResult{Window: TimeWindow{StartTs: 10.0, EndTs: 11.0}, Items: []CriticalBlockingCandidate{
		{Type: "io_latency", Thread: ThreadRef{Comm: "work", PID: 500}, StartTs: 10.10, EndTs: 10.20, DurationMs: 100},
		{Type: "io_latency", Thread: ThreadRef{Comm: "work", PID: 500}, StartTs: 10.50, EndTs: 10.60, DurationMs: 100},
	}}
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	keyOn, keyBg := rank.Items[0].RankFamilyKey, rank.Items[1].RankFamilyKey
	if keyOn == "" || keyBg == "" || keyOn == keyBg {
		t.Fatalf("lane-split families must mint DISTINCT keys, got %q vs %q", keyOn, keyBg)
	}
	if !strings.Contains(keyOn, "|on_chain|") || !strings.Contains(keyBg, "|background|") {
		t.Fatalf("keys must carry the lane dimension, got %q / %q", keyOn, keyBg)
	}
	if blocking.Items[0].AbsorbedIntoFamily != keyOn || blocking.Items[1].AbsorbedIntoFamily != keyBg {
		t.Fatalf("each absorbed row must point at ITS OWN family: %q / %q",
			blocking.Items[0].AbsorbedIntoFamily, blocking.Items[1].AbsorbedIntoFamily)
	}
	if rank.Items[0].AbsorbedChainRows != 1 || rank.Items[1].AbsorbedChainRows != 1 {
		t.Fatalf("each family absorbs exactly its own row: %d / %d",
			rank.Items[0].AbsorbedChainRows, rank.Items[1].AbsorbedChainRows)
	}
}

// TestG1ReconTypeUniversePinned is the §28.8 教训⑥ mechanical universe pin:
// the adjudicated cross-lane type map holds EXACTLY the adjudicated pairs,
// and both sides must stay registered row tokens with the source side
// family-foldable (the recon presupposes a family row can exist). Extending
// the map requires a new adjudication — this pin forces that conversation.
//
// EVOLUTION RECORD (CASE-1, §29.52 独立立案 → v5 P1 批, 2026-07-13): the
// universe was EXACTLY {io_latency: io_latency} (§27.2-G1). The row-level
// witnesses the original ruling waited for arrived (SMR-S7 + S4-TPF + 42729
// E9↔E15; reports 31693/42729/45903, smr_audit_report_20260712 CASE-1), so
// the SECOND adjudicated pair set {d_state_or_io_wait, io_wait}² joined
// through THIS authority channel — both lanes mint from the same
// stats.DStateTop/IOWaitTop ledger groups; the family token flips to
// io_wait when dStateMs==0 (gap (c)), so all four combinations enter
// together. The D/IO pairs use the same-source-identity membership arm
// (exact interval + exact value; hull containment forbidden —
// rank_family_recon_case1_test.go pins the split); io_latency membership is
// unchanged. Every other pair still requires a new adjudication here.
func TestG1ReconTypeUniversePinned(t *testing.T) {
	if len(criticalBlockingRankFamilyReconTypes) != 3 {
		t.Fatalf("recon type universe must hold exactly the adjudicated pairs, got %v", criticalBlockingRankFamilyReconTypes)
	}
	wantUniverse := map[string][]string{
		"io_latency":         {"io_latency"},
		"d_state_or_io_wait": {"d_state_or_io_wait", "io_wait"},
		"io_wait":            {"d_state_or_io_wait", "io_wait"},
	}
	for from, wantTos := range wantUniverse {
		tos, ok := criticalBlockingRankFamilyReconTypes[from]
		if !ok || len(tos) != len(wantTos) {
			t.Fatalf("adjudicated universe drifted for %q: want %v got %v", from, wantTos, tos)
		}
		for k, to := range wantTos {
			if tos[k] != to {
				t.Fatalf("adjudicated universe drifted for %q: want %v got %v", from, wantTos, tos)
			}
		}
	}
	for from, tos := range criticalBlockingRankFamilyReconTypes {
		for _, to := range tos {
			for _, token := range []string{from, to} {
				spec, ok := CausalTokenSpecFor(token)
				if !ok || !spec.RowToken {
					t.Fatalf("recon token %q must be a registered row token", token)
				}
			}
			if CausalTokenFamilyFoldLane(to) != CausalFamilyFoldSameThreadType {
				t.Fatalf("recon target %q must be same-thread-type family-foldable (a family row must be mintable)", to)
			}
		}
	}
	// The membership arm split itself is part of the adjudication: io_latency
	// keeps two-pass membership, every same-source D/IO pair is
	// same-source-identity only.
	if criticalBlockingRankFamilyReconSameSourceOnly("io_latency") {
		t.Fatal("io_latency must keep the original two-pass membership")
	}
	if !criticalBlockingRankFamilyReconSameSourceOnly("d_state_or_io_wait") ||
		!criticalBlockingRankFamilyReconSameSourceOnly("io_wait") {
		t.Fatal("the D/IO pairs must use the same-source-identity membership arm")
	}
}

// TestG1FamilyMemberIntervalsSurviveEngineFold pins the engine-internal
// inventory the reconciliation depends on: the merged family row carries one
// validated interval per member (the union test's precise input).
func TestG1FamilyMemberIntervalsSurviveEngineFold(t *testing.T) {
	idx := g1BuildIndex(t, g1OpendirShapeTrace)
	bundle := BuildFrameRootCauseBundle(idx, Query{PID: 500, TimeStart: 10.0, TimeEnd: 11.0})
	fam := g1FamilyRow(t, *bundle.RootCauseRank)
	if len(fam.familyMemberIntervals) != 6 {
		t.Fatalf("family row must keep all 6 validated member intervals, got %d", len(fam.familyMemberIntervals))
	}
}

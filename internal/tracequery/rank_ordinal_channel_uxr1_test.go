package tracequery

// rank_ordinal_channel_uxr1_test.go — UXR-1 件① engine pins (§29.36.2/§29.36.3
// 3+1 通道终形, user rulings 2026-07-11, ledger real_trace_campaign_20260705.md).
//
// Witness (real_trace_a5 eval log codrax-20260711-141009-000-4165): the single
// rankPos ordinal space seated 根因排序#1/#2 on ▒ background rows whose own
// caliber note says 不计入链上归因 (same-page contradiction), interleaved
// #3/#4/#6/#7/#9/#10 across the ◇ adjacent stanza — one ordinal sequence over
// incomparable calibers (两把尺红线的序数版).
//
// Pinned behavior:
//   - channel 1 (on-chain 根因排序#N): ordinals allocate ONLY inside the
//     on-chain population, contiguous 1..K;
//   - channel 2 (◇ 邻近影响#N): an independent contiguous ordinal space over
//     adjacent rows, ordered by published effective (§29.22.1 序数键==发布 eff);
//   - channel 3 (▒ background): NO ordinal — Rank stays 0 on every background
//     row regardless of magnitude;
//   - the (rank, chain_relevance) pair is the wire carrier: no new note key.
//
// MUTATION self-checks (recorded in the batch report):
//   - M-U1 revert takeOrdinal to the single rankPos counter → both tests red
//     (background rows re-mint ordinals / adjacent ordinals leave 1..K);
//   - M-U2 route adjacent rows through the chain counter → both red;
//   - M-U3 make rootCauseOrdinalChannel return chain for empty relevance
//     off-chain rows → TestUXR1ThreeChannelOrdinalSplit4165Shape red.
//   - M-U4 (复核 P1-1(a)) revert the union pass to hull StartTs/EndTs for
//     folded members → TestUXR1AdjacentIOFacetFamilyHullGapFailOpen red;
//   - M-U5 (复核 P1-1(b)) re-admit block_io_by_inode as a union facet →
//     TestUXR1AdjacentIOFacetFamilyBlockIORosterOnly red (fake bridge +
//     envelope union value).

import (
	"fmt"
	"strings"
	"testing"
)

func uxr1AdjacentItem(typ, comm string, pid int, eff float64, line int) RootCauseRankItem {
	item := RootCauseRankItem{
		Type: typ, Thread: ThreadRef{Comm: comm, PID: pid},
		ImpactMs: eff, CumulativeImpactMs: eff, EffectiveImpactMs: eff,
		Confidence:     0.6,
		ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain",
		LineStart: line, LineEnd: line + 1,
	}
	// The effective-impact canonicaliser derives some facet lanes from their
	// state scalars — keep them coherent with the published value.
	switch typ {
	case "io_wait", "d_state_or_io_wait":
		item.IOWaitMs = eff
		item.DominantState = string(StateIOWait)
	case "runnable_wait":
		item.RunnableMs = eff
		item.DominantState = string(StateRunnable)
	}
	return item
}

func uxr1BackgroundItem(typ, comm string, pid int, eff float64, state string, line int) RootCauseRankItem {
	item := RootCauseRankItem{
		Type: typ, Thread: ThreadRef{Comm: comm, PID: pid},
		ImpactMs: eff, CumulativeImpactMs: eff, EffectiveImpactMs: eff,
		Confidence:     0.6,
		ChainRelevance: "background", Causality: "background",
		LineStart: line, LineEnd: line + 1,
	}
	switch state {
	case "runnable":
		item.RunnableMs = eff
		item.DominantState = string(StateRunnable)
	case "d_state":
		item.DStateMs = eff
		item.DominantState = string(StateDSleep)
	}
	return item
}

// TestUXR1ThreeChannelOrdinalSplit4165Shape replays the real_trace_a5 witness
// board shape (zero on-chain rows; six ◇ adjacent contenders including the
// five IO facets; four ▒ background contenders, two of them larger than every
// adjacent row): background rows must publish NO ordinal, and the adjacent
// channel numbers 1..6 contiguously by published effective.
func TestUXR1ThreeChannelOrdinalSplit4165Shape(t *testing.T) {
	items := []RootCauseRankItem{
		// ◇ adjacent (same thread 36644): five IO facets + one runnable.
		uxr1AdjacentItem("page_cache_churn", "[GT]ColdPool#6", 36644, 0.600, 100),
		uxr1AdjacentItem("block_io_by_inode", "[GT]ColdPool#6", 36644, 0.198, 110),
		uxr1AdjacentItem("io_latency", "[GT]ColdPool#6", 36644, 0.099, 120),
		uxr1AdjacentItem("io_burst_episode", "[GT]ColdPool#6", 36644, 0.099, 130),
		uxr1AdjacentItem("io_wait", "[GT]ColdPool#6", 36644, 0.091, 140),
		uxr1AdjacentItem("runnable_wait", "[GT]ColdPool#6", 36644, 0.088, 150),
		// ▒ background — the witness's fake #1/#2 rows (largest magnitudes).
		uxr1BackgroundItem("runnable_wait", "SearchDaemon", 37188, 54.701, "runnable", 200),
		uxr1BackgroundItem("d_state_or_io_wait", "wc_srvinit_0", 37254, 54.608, "d_state", 210),
		uxr1BackgroundItem("runnable_wait", "mars::comm", 36700, 0.146, "runnable", 220),
		uxr1BackgroundItem("d_state_or_io_wait", "wc_srvinit_3", 37253, 0.093, "d_state", 230),
	}
	// The witness window had no resolved wakeup chain → the flat (non
	// chain-aware) sort, published-eff descending across the whole list.
	sortRootCauseRankItems(items, false)
	assignRootCauseRanksAndTiers(items)

	for _, item := range items {
		switch item.ChainRelevance {
		case "background":
			if item.Rank != 0 {
				t.Fatalf("§29.36.2 通道3 无序数 — background row %s wears ordinal #%d: %+v",
					item.Thread.Comm, item.Rank, item)
			}
		case "adjacent":
			// V2-P0 (行级尺守卫): caliber-side rows are ordinal-less by
			// design — the wall-clock contenders below keep the obligation.
			if CausalTokenCaliberSideClass(item.Type) != CausalCaliberSideNone {
				if item.Rank != 0 || item.Tier != RootCauseTierCaliberSide {
					t.Fatalf("caliber-side row %s/%s must carry Rank=0 + tier=caliber_side: %+v",
						item.Thread.Comm, item.Type, item)
				}
				continue
			}
			if item.Rank <= 0 {
				t.Fatalf("adjacent contender %s/%s lost its 邻近影响 ordinal: %+v",
					item.Thread.Comm, item.Type, item)
			}
		}
	}
	// Channel-2 ordinal order == published effective descending (§29.22.1).
	// EVOLUTION RECORD (V2-P0 行级尺守卫, rank_order_v2_design_20260712.md
	// §6.1 新裁定 A, 2026-07-12): the pre-V2-P0 self-violating sequence —
	// count 计数当量 0.600 at #1 and composite score 0.198 at #2 ABOVE the
	// wall-clock 0.099/0.091 rows — is exactly the 4165 witness the design
	// quotes (rank_boards.txt:113 自违序列). Count/composite rows now leave
	// the ordinal space (⌗ 口径旁栏, rank=0 asserted above) and the
	// wall-clock contenders renumber contiguously.
	wantAdjacent := []struct {
		typ  string
		rank int
	}{
		{"page_cache_churn", 0},
		{"block_io_by_inode", 0},
		{"io_latency", 1},
		{"io_burst_episode", 2},
		{"io_wait", 3},
		{"runnable_wait", 4},
	}
	for _, want := range wantAdjacent {
		found := false
		for _, item := range items {
			if item.ChainRelevance == "adjacent" && item.Type == want.typ {
				found = true
				if item.Rank != want.rank {
					t.Fatalf("adjacent %s: want 邻近影响#%d, got #%d (%+v)", want.typ, want.rank, item.Rank, items)
				}
			}
		}
		if !found {
			t.Fatalf("fixture drifted: adjacent %s missing", want.typ)
		}
	}
	assertRankOrdinalsContiguous(t, items)
}

// TestUXR1OnChainOrdinalSpaceUnpollutedByStanzaRows: with all three channels
// populated, the on-chain 根因排序 space numbers 1..K over on-chain rows only
// — a background row with a magnitude larger than every on-chain row neither
// takes #1 nor shifts the on-chain ordinals, and the adjacent channel numbers
// independently from 1.
func TestUXR1OnChainOrdinalSpaceUnpollutedByStanzaRows(t *testing.T) {
	items := []RootCauseRankItem{
		{
			Type: "runnable_wait", Thread: ThreadRef{Comm: "waiter", PID: 100},
			ImpactMs: 26.392, CumulativeImpactMs: 26.392, EffectiveImpactMs: 26.392,
			RunnableMs: 26.392, DominantState: string(StateRunnable), Confidence: 0.8,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", LineStart: 10, LineEnd: 11,
		},
		{
			Type: "d_state_or_io_wait", Thread: ThreadRef{Comm: "io-peer", PID: 101},
			ImpactMs: 12.001, CumulativeImpactMs: 12.001, EffectiveImpactMs: 12.001,
			DStateMs: 12.001, DominantState: string(StateDSleep), Confidence: 0.8,
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", LineStart: 20, LineEnd: 21,
		},
		uxr1AdjacentItem("io_latency", "adj-thread", 300, 3.500, 30),
		uxr1BackgroundItem("runnable_wait", "bg-huge", 400, 99.000, "runnable", 40),
	}
	sortRootCauseRankItems(items, true)
	assignRootCauseRanksAndTiers(items)

	byType := func(typ string, pid int) *RootCauseRankItem {
		for i := range items {
			if items[i].Type == typ && items[i].Thread.PID == pid {
				return &items[i]
			}
		}
		return nil
	}
	waiter := byType("runnable_wait", 100)
	ioPeer := byType("d_state_or_io_wait", 101)
	adjacent := byType("io_latency", 300)
	background := byType("runnable_wait", 400)
	if waiter == nil || ioPeer == nil || adjacent == nil || background == nil {
		t.Fatalf("fixture drifted: %+v", items)
	}
	if waiter.Rank != 1 || ioPeer.Rank != 2 {
		t.Fatalf("on-chain 根因排序 space polluted: waiter=#%d ioPeer=#%d (%+v)",
			waiter.Rank, ioPeer.Rank, items)
	}
	if adjacent.Rank != 1 {
		t.Fatalf("adjacent channel must number independently from #1, got #%d", adjacent.Rank)
	}
	if background.Rank != 0 {
		t.Fatalf("background row must hold no ordinal even at 99ms, got #%d", background.Rank)
	}
	assertRankOrdinalsContiguous(t, items)
}

// --- §29.36③ ◇ IO facet family aggregate seat ---------------------------------

func uxr1FacetItem(typ string, eff, start, end float64, line int, memberKey string) RootCauseRankItem {
	item := uxr1AdjacentItem(typ, "[GT]ColdPool#6", 36644, eff, line)
	item.StartTs, item.EndTs = start, end
	item.StatsWindowStartTs, item.StatsWindowEndTs = 10.0, 10.2
	item.MemberKey = memberKey
	item.Source = "window_stats." + typ
	return item
}

func uxr1FacetFamilyFixture() []RootCauseRankItem {
	return []RootCauseRankItem{
		// The 4165 five-facet shape: one physical wait, five adjacent seats.
		uxr1FacetItem("page_cache_churn", 0.600, 10.00990, 10.01020, 100, "inode=0x81251 dev=260:84"),
		uxr1FacetItem("block_io_by_inode", 0.198, 10.00995, 10.010148, 110, "inode=0x1 dev=260:84"),
		uxr1FacetItem("io_latency", 0.099, 10.01000, 10.010099, 120, ""),
		uxr1FacetItem("io_burst_episode", 0.099, 10.00999, 10.010089, 130, ""),
		uxr1FacetItem("io_wait", 0.091, 10.01000, 10.010091, 140, ""),
		// A same-thread adjacent runnable stays its own channel-2 contender.
		uxr1AdjacentItem("runnable_wait", "[GT]ColdPool#6", 36644, 0.088, 150),
	}
}

// TestUXR1AdjacentIOFacetFamilyOneSeat4165 pins the §29.36③ convergence: the
// five-facet witness shape folds into ONE interval-union family seat, the
// members move to the lossless absorbed carrier (明细不占席), and the ◇
// channel numbers the family seat and the runnable row contiguously.
func TestUXR1AdjacentIOFacetFamilyOneSeat4165(t *testing.T) {
	rank := RootCauseRankResult{Items: uxr1FacetFamilyFixture()}
	reconcileAdjacentIOFacetFamilySeats(&rank)
	sortRootCauseRankItems(rank.Items, false)
	assignRootCauseRanksAndTiers(rank.Items)

	var family *RootCauseRankItem
	facetRows := 0
	for i := range rank.Items {
		if rank.Items[i].Source == adjacentIOFacetFamilySource {
			family = &rank.Items[i]
		}
		if adjacentIOFacetFamilyTypes[rank.Items[i].Type] {
			facetRows++
		}
	}
	if family == nil || facetRows != 1 {
		t.Fatalf("五席形 must converge to ONE family seat, got %d facet rows: %+v", facetRows, rank.Items)
	}
	// interval-union wall clock over the UNION facets only (复核 P1-1(b):
	// block_io_by_inode is roster-only — its composite score and inode
	// envelope enter neither connectivity nor value): io_burst [10.00999,
	// 10.010089] ∪ io_wait [10.01000,10.010091] ∪ io_latency [10.01000,
	// 10.010099] = [10.00999,10.010099] → 0.109ms (never the raw Σ, never the
	// block_io envelope's 0.198, never the 0.600 count equivalent).
	if !near(family.EffectiveImpactMs, 0.109, 0.0005) || !near(family.ImpactMs, 0.109, 0.0005) {
		t.Fatalf("family seat must publish the union-facet interval union (0.109ms), got %+v", family)
	}
	if family.MemberCount != 5 || family.MemberFoldCaliber != RootCauseMemberFoldCaliberIntervalUnion {
		t.Fatalf("family must fold all five facets under interval_union: %+v", family)
	}
	roster := strings.Join(family.MemberRoster, " | ")
	for _, want := range []string{"io_wait", "io_latency",
		"block_io_by_inode inode=0x1 dev=260:84 0.198ms(不计入墙钟合计)",
		"io_burst_episode", "page_cache_churn inode=0x81251 dev=260:84 计数当量 0.600(非墙钟)"} {
		if !strings.Contains(roster, want) {
			t.Fatalf("family roster must keep the facet distinguishing keys (%q): %q", want, roster)
		}
	}
	// The raw union-member Σ discloses losslessly beside the union (原始和;
	// roster-only values excluded — they never entered the wall-clock lane).
	if !near(family.MemberSumMs, 0.289, 0.001) {
		t.Fatalf("the union-member Σ must disclose beside the union: %+v", family)
	}
	// 复核 P1-1(c) invariant: the published union never exceeds what the
	// union members' published values can account for.
	if family.ImpactMs > family.MemberSumMs+0.0005 {
		t.Fatalf("union > Σ(published union members) — unaccountable wall clock: %+v", family)
	}
	// The family's recon inventory carries the union facets' TRUE intervals.
	if len(family.familyMemberIntervals) != 3 {
		t.Fatalf("family must carry the 3 union facets' validated intervals, got %d", len(family.familyMemberIntervals))
	}
	if family.Rank != 1 {
		t.Fatalf("the family seat holds ONE 邻近影响 ordinal (#1 at 0.109ms), got #%d", family.Rank)
	}
	// Members are lossless absorbed carriers with no seats.
	if len(rank.AbsorbedItems) != 5 {
		t.Fatalf("all five members must move to the absorbed carrier, got %d: %+v",
			len(rank.AbsorbedItems), rank.AbsorbedItems)
	}
	for _, member := range rank.AbsorbedItems {
		if member.Rank != 0 || member.Tier != RootCauseTierAbsorbed ||
			!strings.HasPrefix(member.AbsorbedIntoFamily, adjacentIOFacetFamilyKeyPrefix) {
			t.Fatalf("absorbed member must carry no seat and the family key: %+v", member)
		}
	}
	// Channel-2 ordinals stay contiguous: family #1, runnable #2.
	for _, item := range rank.Items {
		if item.Type == "runnable_wait" && item.Rank != 2 {
			t.Fatalf("the runnable contender must hold 邻近影响#2, got #%d", item.Rank)
		}
	}
	assertRankOrdinalsContiguous(t, rank.Items)
}

// TestUXR1AdjacentIOFacetFamilyIdempotent pins the reset-first recomputation:
// a second pass over the already-folded result is byte-stable (the enrich
// pipeline reruns the pass).
func TestUXR1AdjacentIOFacetFamilyIdempotent(t *testing.T) {
	// The pipeline sorts right after the pass — the idempotency contract is
	// over the SORTED publication (row identity/values), matching what the
	// enrich rerun actually republishes.
	rank := RootCauseRankResult{Items: uxr1FacetFamilyFixture()}
	reconcileAdjacentIOFacetFamilySeats(&rank)
	sortRootCauseRankItems(rank.Items, false)
	first := fmt.Sprintf("%+v||%+v", rank.Items, rank.AbsorbedItems)
	reconcileAdjacentIOFacetFamilySeats(&rank)
	sortRootCauseRankItems(rank.Items, false)
	second := fmt.Sprintf("%+v||%+v", rank.Items, rank.AbsorbedItems)
	if first != second {
		t.Fatalf("facet family fold must be idempotent:\nfirst:  %s\nsecond: %s", first, second)
	}
}

// TestUXR1AdjacentIOFacetFamilyFailOpenArms pins the precision boundaries:
// disconnected segments, on-chain facets and duplicate facet types never fold.
func TestUXR1AdjacentIOFacetFamilyFailOpenArms(t *testing.T) {
	// Disconnected wall-clock segments (two separate bursts) — fail open.
	disconnected := []RootCauseRankItem{
		uxr1FacetItem("io_wait", 0.091, 10.0100, 10.0101, 100, ""),
		uxr1FacetItem("io_latency", 0.099, 10.1500, 10.1501, 110, ""),
	}
	rank := RootCauseRankResult{Items: disconnected}
	reconcileAdjacentIOFacetFamilySeats(&rank)
	if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("disconnected segments must not fold (同段 proof): %+v", rank.Items)
	}
	// On-chain facets are out of scope (◇ channel only).
	onChain := uxr1FacetFamilyFixture()[:3]
	for i := range onChain {
		onChain[i].ChainRelevance = "on_chain"
		onChain[i].Causality = "on_wakeup_chain"
	}
	rank = RootCauseRankResult{Items: onChain}
	reconcileAdjacentIOFacetFamilySeats(&rank)
	if len(rank.AbsorbedItems) != 0 {
		t.Fatalf("on-chain facet rows must never enter the adjacent family fold: %+v", rank.AbsorbedItems)
	}
	// Duplicate facet type = ambiguity — the whole group fails open.
	dup := []RootCauseRankItem{
		uxr1FacetItem("io_wait", 0.091, 10.0100, 10.0101, 100, ""),
		uxr1FacetItem("io_wait", 0.050, 10.0100, 10.0101, 105, ""),
		uxr1FacetItem("io_latency", 0.099, 10.0100, 10.0102, 110, ""),
	}
	rank = RootCauseRankResult{Items: dup}
	reconcileAdjacentIOFacetFamilySeats(&rank)
	if len(rank.Items) != 3 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("a duplicate facet type must fail the whole group open: %+v", rank.Items)
	}
}

// uxr1FoldedFacetItem builds the engine-actual MERGED facet row form: the
// same-thread same-type fold (which runs BEFORE the facet pass) publishes the
// members' HULL on StartTs/EndTs and keeps the validated per-member intervals
// on familyMemberIntervals (rank_family_fold.go G1 carrier).
func uxr1FoldedFacetItem(typ string, eff float64, intervals []foldInterval, line int) RootCauseRankItem {
	hullStart, hullEnd := 0.0, 0.0
	for _, iv := range intervals {
		if hullStart == 0 || iv.start < hullStart {
			hullStart = iv.start
		}
		if iv.end > hullEnd {
			hullEnd = iv.end
		}
	}
	item := uxr1FacetItem(typ, eff, hullStart, hullEnd, line, "")
	item.MemberCount = len(intervals)
	item.MemberFoldCaliber = RootCauseMemberFoldCaliberIntervalUnion
	item.familyMemberIntervals = append([]foldInterval(nil), intervals...)
	return item
}

// TestUXR1AdjacentIOFacetFamilyHullGapFailOpen — 复核 P1-1(a)/(c) pin: a
// same-type MERGED row's hull spans two disjoint bursts. The hull must
// neither fake 同段 connectivity with a facet sitting INSIDE the gap nor let
// the gap wall clock reach the published union — the true member intervals
// are three disjoint components, so the group fails open.
//
// The primary arm deliberately OVER-publishes the folded member (io_burst
// latency calibers legitimately exceed the envelope), so the P1-1(c)
// Σ(published) ceiling is TRANSPARENT here and cannot mask an (a) revert —
// only the true-inventory connectivity proof rejects this shape (M-U4 bites
// exactly here). The gross Σ=2ms/hull=160ms witness form follows as a second
// arm (there (a) and (c) both reject — defense in depth).
func TestUXR1AdjacentIOFacetFamilyHullGapFailOpen(t *testing.T) {
	items := []RootCauseRankItem{
		// Two 1ms bursts, 9ms gap; published 15.0 (block latency caliber).
		uxr1FoldedFacetItem("io_burst_episode", 15.0,
			[]foldInterval{{start: 10.0100, end: 10.0110}, {start: 10.0200, end: 10.0210}}, 100),
		uxr1FacetItem("io_wait", 0.100, 10.0150, 10.0151, 110, ""), // inside the hull GAP
	}
	rank := RootCauseRankResult{Items: items}
	reconcileAdjacentIOFacetFamilySeats(&rank)
	if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("hull-with-gap must fail open (真成员区间三段不连通): %+v || %+v",
			rank.Items, rank.AbsorbedItems)
	}
	for _, item := range rank.Items {
		if item.Source == adjacentIOFacetFamilySource {
			t.Fatalf("no family row may mint over a hull gap: %+v", item)
		}
	}
	// Gross witness arm (双 burst Σ=2ms hull=160ms 病理形): both the (a)
	// connectivity proof and the (c) published-value ceiling reject it.
	gross := []RootCauseRankItem{
		uxr1FoldedFacetItem("io_burst_episode", 2.0,
			[]foldInterval{{start: 10.0100, end: 10.0110}, {start: 10.1690, end: 10.1700}}, 100),
		uxr1FacetItem("io_wait", 0.100, 10.0500, 10.0501, 110, ""),
	}
	rank = RootCauseRankResult{Items: gross}
	reconcileAdjacentIOFacetFamilySeats(&rank)
	if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("the gross 160ms-hull witness form must fail open: %+v", rank.Items)
	}

	// Inventory-missing arm: the SAME shape whose merged row lost its interval
	// inventory fails open too — the hull alone can never prove 同段.
	noInventory := uxr1FoldedFacetItem("io_burst_episode", 2.0,
		[]foldInterval{{start: 10.0100, end: 10.0110}, {start: 10.1690, end: 10.1700}}, 100)
	noInventory.familyMemberIntervals = nil
	rank = RootCauseRankResult{Items: []RootCauseRankItem{
		noInventory,
		uxr1FacetItem("io_wait", 0.100, 10.0500, 10.0501, 110, ""),
	}}
	reconcileAdjacentIOFacetFamilySeats(&rank)
	if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("a folded union facet without interval inventory must fail the group open: %+v",
			rank.Items)
	}
}

// TestUXR1AdjacentIOFacetFamilyBlockIORosterOnly — 复核 P1-1(b) pin:
// block_io_by_inode (composite score over an inode ENVELOPE) is excluded from
// wall-clock connectivity AND from the union value; it joins overlapping
// families as a roster-only disclosure and keeps its independent seat when it
// does not overlap.
func TestUXR1AdjacentIOFacetFamilyBlockIORosterOnly(t *testing.T) {
	// Arm A — no fake bridge: two DISJOINT union facets whose gap is covered
	// by the block_io envelope must not connect through it.
	bridge := []RootCauseRankItem{
		uxr1FacetItem("io_wait", 0.100, 10.0100, 10.0101, 100, ""),
		uxr1FacetItem("io_latency", 0.100, 10.0150, 10.0151, 110, ""),
		uxr1FacetItem("block_io_by_inode", 0.198, 10.0090, 10.0160, 120, "inode=0x1 dev=260:84"),
	}
	rank := RootCauseRankResult{Items: bridge}
	reconcileAdjacentIOFacetFamilySeats(&rank)
	if len(rank.Items) != 3 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("the block_io envelope must not bridge disjoint union facets: %+v", rank.Items)
	}
	// Arm B — value exclusion: an overlapping block_io joins roster-only; the
	// family publishes the union facet's OWN wall clock, never the envelope.
	value := []RootCauseRankItem{
		uxr1FacetItem("io_wait", 0.091, 10.0100, 10.010091, 100, ""),
		uxr1FacetItem("block_io_by_inode", 0.198, 10.00995, 10.010148, 110, "inode=0x1 dev=260:84"),
	}
	rank = RootCauseRankResult{Items: value}
	reconcileAdjacentIOFacetFamilySeats(&rank)
	var family *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Source == adjacentIOFacetFamilySource {
			family = &rank.Items[i]
		}
	}
	if family == nil || family.MemberCount != 2 || len(rank.AbsorbedItems) != 2 {
		t.Fatalf("overlapping block_io joins the family roster-only: %+v", rank.Items)
	}
	if !near(family.ImpactMs, 0.091, 0.0005) || !near(family.EffectiveImpactMs, 0.091, 0.0005) {
		t.Fatalf("family value = union facets only (0.091ms), never the block_io envelope: %+v", family)
	}
	if !strings.Contains(strings.Join(family.MemberRoster, " | "), "block_io_by_inode inode=0x1 dev=260:84 0.198ms(不计入墙钟合计)") {
		t.Fatalf("block_io roster entry must disclose its excluded caliber: %+v", family.MemberRoster)
	}
	// Arm C — independent seat: a NON-overlapping block_io keeps its own seat
	// beside the folding union facets.
	apart := []RootCauseRankItem{
		uxr1FacetItem("io_wait", 0.091, 10.0100, 10.010091, 100, ""),
		uxr1FacetItem("io_latency", 0.099, 10.0100, 10.010099, 110, ""),
		uxr1FacetItem("block_io_by_inode", 0.198, 10.1500, 10.1510, 120, "inode=0x1 dev=260:84"),
	}
	rank = RootCauseRankResult{Items: apart}
	reconcileAdjacentIOFacetFamilySeats(&rank)
	blockSeat := false
	for _, item := range rank.Items {
		if item.Type == "block_io_by_inode" && item.Source != adjacentIOFacetFamilySource {
			blockSeat = true
		}
	}
	if !blockSeat || len(rank.AbsorbedItems) != 2 {
		t.Fatalf("non-overlapping block_io keeps its independent seat: %+v || %+v",
			rank.Items, rank.AbsorbedItems)
	}
}

// TestUXR1AdjacentIOFacetFamilyUnionExceedsPublishedFailOpen — 复核 P1-1(c)
// invariant pin: union ≤ Σ(union members' PUBLISHED values). Envelopes that
// over-claim (interval length ≫ published wall clock) must not mint a family
// value no member set can account for.
func TestUXR1AdjacentIOFacetFamilyUnionExceedsPublishedFailOpen(t *testing.T) {
	items := []RootCauseRankItem{
		uxr1FacetItem("io_wait", 1.0, 10.000, 10.100, 100, ""),    // 100ms envelope, 1ms published
		uxr1FacetItem("io_latency", 1.0, 10.050, 10.150, 110, ""), // 100ms envelope, 1ms published
	}
	rank := RootCauseRankResult{Items: items}
	reconcileAdjacentIOFacetFamilySeats(&rank)
	if len(rank.Items) != 2 || len(rank.AbsorbedItems) != 0 {
		t.Fatalf("union (150ms) > Σ published (2ms) must fail open: %+v", rank.Items)
	}
}

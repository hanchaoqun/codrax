package tracequery

// rank_direction_axiom.go — AXIOM-V2 批 (user rulings 2026-07-18, ledger
// docs/design/real_trace_campaign_20260705.md):
//
// 公理 v2 (守恒律,按修复方向分键): 同线程·同窗·同板·同口径(墙钟)·同修复方向
// 的物理时间∩>0 ⇒ 恰一全额席; 跨方向重叠 = 合法共存全额,互指披露「同段收益
// 不叠加」。用户语义定谳: 榜=修复方向指导; 席值=折算后可消除提升空间; 跨视角
// 对同段状态的净收益差异合法 (「哪个最大,可以认为根因是他」)。
//
// Three passes, all on the SHARED rank finalize tail (after every sort/
// capacity/ordinal decision — pure attribute/disclosure lanes, no seat can
// move and no value channel changes; §29.104.13 非致命不硬拦):
//
//   1. stampRootCauseFixDirections (件1) — publish the registry fix-direction
//      attribute verbatim on every rank row (unresolved → absent).
//   2. cross-direction overlap disclosure (件2) — the typed pair table behind
//      the 互指句 (「与[E#]作用于同段时间,修其一后另一席空间会缩,收益不
//      叠加」); symmetric entries, 宁漏勿假指.
//   3. direction-conservation audit (件3) — per (thread, direction) the Σ of
//      full-seat support-interval unions must fit the physical window;
//      violations DISCLOSE (typed finding + caveat 立案素材) and never block
//      emission (检查器=纯披露道永不硬拦).
//
// 「链上」硬纪律 (user ruling 2026-07-18): the population for 件2/件3 is the
// STRICT on-chain full-seat set — typed on-chain channel, OnChainBasis in the
// closed set, a COMPETING chain ordinal (Rank>0), positive effective value,
// wall-clock caliber, no ◇ demotion flag, and a PER-SEGMENT support-interval
// inventory (envelope-level rows step out — HULL-CRED: hull endpoints are
// noise). 「Self* 豁免席」出圈 reads as the NON-COMPETING self exemption rows
// (wait-symptom / context-only rows, Rank==0 — the R8 channel-word exemption
// family): the analysis target's own COMPETING seats stay in, which is the
// only reading consistent with the ruling's named acceptance witness
// (cust_span_runnable E26 self_deterministic 类校验席 × E5 self_wall_clock
// running 折算席 —— both self-basis competing seats). 委托默认处置,待人工追认.

import (
	"fmt"
	"sort"
	"strings"
)

// RootCauseCrossDirectionOverlapPartnerCap bounds each seat's cross-direction
// disclosure roster (AXIOM-V2 件2; the XLANE-2 SelfGapSemanticOverlapPartnerCap
// precedent). Pairs beyond the cap keep wire SYMMETRY: the pair is dropped on
// BOTH sides and reported through the undisclosed type-token lane instead
// (件3 closure — no Σ claim rides the roster, so a bounded roster is honest).
// Exported for the tool-side mirror pin against the projection decode cap.
const RootCauseCrossDirectionOverlapPartnerCap = 6

// directionConservationToleranceMs — µs-scale float tolerance on the 件3
// Σ-vs-window comparison (precise-signal discipline: the inequality fires on
// real double-billed wall clock, never on float dust).
const directionConservationToleranceMs = 0.001

// Closed basis set of rootCauseItemDirectionSupport — the per-segment
// support-interval inventories a seat may enter the direction population
// with. Envelope-only rows resolve NOTHING and step out of the population
// (包络级凭证行出圈 — HULL-CRED: hull endpoints must never pose as segments).
// ONCHAIN-FIX-2 件4 (AXIOM-V2 偏离④ 衔接, 2026-07-18): the formal D/IO
// seats enter through their OWN true-segment carrier —
// dioSegmentIntervals, the member ledgers' exact close-site dioIntervals
// pushed down at mintRootCauseDIOStateSeat under an all-or-nothing
// validation (sum_disjoint caliber, whole-td members, no ledger overflow,
// per-member Σ identity); a seat that misses any condition carries nothing
// and stays out (fail-open, 宁漏勿假指).
//
// INTERSECT-FIX (偏离④ 收窄, §29.143 INTERSECT-REG 归因, 2026-07-19): the
// familyMemberIntervals lane is shared by THREE mint sites — the D/IO census
// (hull), the io-facet family (rank_io_facet_family_uxr1.go:417, validated
// real intervals, interval_union — 复核 P3-1: it enters through the same
// µs-identity gate and its carriers are real segments, so admission is
// correct by the same superset theorem) and the D/IO census
// family (query.go, per-(thread,cpu) group HULLS: gaps/overlaps inside, the
// 偏离④ exclusion reason) and the same-thread-type fold pass
// (rank_family_fold.go, member envelopes whose values the interval_union /
// sum_disjoint caliber ladder already proved against their lengths). A
// blanket exclusion of the lane mis-hit the fold-pass TRUE-segment families
// (the §29.136 CHAIN-BUDGET regression: the io_latency single-segment row
// folded into its family, MemberCount>1 killed the single-segment arm, and
// the family's exact segments were refused wholesale — the flagship board
// silently lost its ∩ disclosure). The family_member_segment_intervals arm
// below re-admits ONLY the exact-inventory shape, on PRECISE signals
// (typed caliber enum + µs identity |union−ImpactMs|≤tol, all-or-nothing):
// hull inventories structurally fail the identity (internal gaps ⇒ union ≠
// published value), so 偏离④'s original protection stands unweakened.
const (
	RootCauseDirectionBasisSemanticMembers = "semantic_member_intervals"
	RootCauseDirectionBasisSelfRunning     = "self_running_intervals"
	RootCauseDirectionBasisRunnable        = "runnable_intervals"
	RootCauseDirectionBasisDioSegments     = "dio_segment_intervals"
	RootCauseDirectionBasisFamilySegments  = "family_member_segment_intervals"
	RootCauseDirectionBasisOccurrences     = "occurrence_windows"
	RootCauseDirectionBasisSingleSegment   = "single_segment_identity"
)

// stampRootCauseFixDirections (件1) publishes the registry fix-direction
// attribute on every rank row — Items and AbsorbedItems alike (the absorbed
// lane keeps publishing losslessly). Verbatim single source
// (CausalTokenFixDirectionFor); unresolved publishes NOTHING (fail-open,
// absence never guesses). Attribute axis only.
func stampRootCauseFixDirections(rank *RootCauseRankResult) {
	stamp := func(items []RootCauseRankItem) {
		for i := range items {
			direction := CausalTokenFixDirectionFor(items[i].Type)
			if direction == CausalFixDirectionUnresolved {
				continue
			}
			items[i].FixDirection = string(direction)
		}
	}
	stamp(rank.Items)
	stamp(rank.AbsorbedItems)
}

// rootCauseItemDirectionPopulationEligible is the strict on-chain full-seat
// predicate (see the file header). PRECISE signals only: typed channel/flag/
// registry reads, never prose.
func rootCauseItemDirectionPopulationEligible(item *RootCauseRankItem) bool {
	if item.Rank <= 0 || item.EffectiveImpactMs <= 0 {
		return false // 豁免席 (non-competing) and gated-zero rows step out
	}
	if !rootCauseItemIsOnChain(*item) || rootCauseOrdinalChannel(*item) != rootCauseOrdinalChannelChain {
		return false
	}
	switch item.OnChainBasis {
	case "", RootCauseOnChainBasisSelfDeterministicSpan,
		RootCauseOnChainBasisSelfWallClockInterval, RootCauseOnChainBasisHostWakeupEdge,
		RootCauseOnChainBasisHostWakeupEdgeState:
		// the closed basis set — legacy overlap ("") included; the ONCHAIN-3c
		// state seats enter with their pre-edge-clipped inventories (support
		// union == published value by construction of the bisection).
	default:
		return false // unknown basis never enters (fail-open exclusion)
	}
	if item.ChainAnchorRemainderSeat || item.ChainCredentialLaneDemoted ||
		item.ChainAnchorRepresentedByChainSeat || item.GatedShareConstituentSeat ||
		item.AbsorbedByRankFamily {
		return false // ◇/constituent/absorbed lanes never re-enter as full seats
	}
	if item.Thread.PID <= 0 {
		return false // the axiom keys on thread identity — no identity, no claim
	}
	spec, ok := CausalTokenSpecFor(item.Type)
	if !ok || spec.Additivity != CausalAdditivityWallClockPerThread {
		return false // 同口径(墙钟) — cross-thread/count calibers step out
	}
	if CausalTokenCaliberSideClass(item.Type) != CausalCaliberSideNone {
		return false // 计数当量/复合分数行出圈
	}
	if CausalTokenFixDirectionFor(item.Type) == CausalFixDirectionUnresolved {
		return false // unresolved direction: 不互斥只披露 (nothing to key on)
	}
	return true
}

// rootCauseItemDirectionSupport resolves a seat's PER-SEGMENT support
// inventory (closed basis set above). Returns nil when only the
// StartTs..EndTs envelope is available — envelope-level rows are OUT
// (宁漏勿假指; the single-segment µs-identity arm below is the one precise
// exception: an unfragmented row whose measured wall clock equals its
// envelope length IS its own segment).
func rootCauseItemDirectionSupport(item *RootCauseRankItem) ([]foldInterval, string) {
	if len(item.semanticMemberIntervals) > 0 {
		return item.semanticMemberIntervals, RootCauseDirectionBasisSemanticMembers
	}
	if strings.TrimSpace(item.SemanticClass) != "" && item.MemberCount <= 1 &&
		item.StartTs > 0 && item.EndTs > item.StartTs {
		// The XLANE-2 single-span fallback: one span row's interval IS its
		// one typed segment.
		return []foldInterval{{start: item.StartTs, end: item.EndTs}}, RootCauseDirectionBasisSemanticMembers
	}
	if len(item.selfGapRunningIntervals) > 0 {
		return item.selfGapRunningIntervals, RootCauseDirectionBasisSelfRunning
	}
	if len(item.runnableIntervals) > 0 {
		return item.runnableIntervals, RootCauseDirectionBasisRunnable
	}
	if len(item.dioSegmentIntervals) > 0 {
		// ONCHAIN-FIX-2 件4: the formal D/IO seat's true close-site segments
		// (validated all-or-nothing at the mint — see the carrier comment).
		return item.dioSegmentIntervals, RootCauseDirectionBasisDioSegments
	}
	if len(item.familyMemberIntervals) > 0 &&
		(item.MemberFoldCaliber == RootCauseMemberFoldCaliberIntervalUnion ||
			item.MemberFoldCaliber == RootCauseMemberFoldCaliberSumDisjoint) {
		// INTERSECT-FIX (偏离④ 收窄, 2026-07-19): the fold-pass family whose
		// member inventory provably IS its published wall clock. Admission is
		// all-or-nothing on PRECISE signals — the typed exact-caliber enum
		// (interval_union / sum_disjoint; the MAX-fallback lower bound and
		// count calibers never enter) plus the µs identity union(inventory) ==
		// ImpactMs. Census hull inventories (per-(thread,cpu) group hulls with
		// internal gaps) structurally fail the identity and stay out — 偏离④'s
		// hull protection intact; any miss leaves the seat without a basis
		// (照旧出圈, 宁漏勿假指).
		unionMs, _ := foldIntervalUnionMs(item.familyMemberIntervals)
		if diff := unionMs - item.ImpactMs; diff <= directionConservationToleranceMs && diff >= -directionConservationToleranceMs {
			return item.familyMemberIntervals, RootCauseDirectionBasisFamilySegments
		}
	}
	if len(item.OccurrenceWindows) > 0 {
		var out []foldInterval
		for _, occ := range item.OccurrenceWindows {
			window := occ.ActualWindow
			if window.EndTs <= window.StartTs {
				window = occ.Window
			}
			if window.EndTs <= window.StartTs {
				continue
			}
			// 修补轮 件1 (P1) µs-identity 守卫: an occurrence window is a
			// node-level HULL across whatever states filled it — it counts as
			// a per-segment credential only when the dominant state fills the
			// window to the µs (DominantImpactMs == window length, the same
			// identity discipline as the single-segment arm below). A
			// state-mixture window (tieba 59566 live shape: 8.632ms occ =
			// 0.185 run + 8.282 runnable + 0.113 sleep) is envelope-level and
			// never enters; when no window passes, the basis is absent and
			// the seat steps out of the 件2/件3 population (fail-open,
			// 宁漏勿假指 — 包络级凭证行出圈红线).
			windowMs := (window.EndTs - window.StartTs) * 1000
			if diff := occ.DominantImpactMs - windowMs; diff > directionConservationToleranceMs || diff < -directionConservationToleranceMs {
				continue
			}
			out = append(out, foldInterval{start: window.StartTs, end: window.EndTs})
		}
		if len(out) > 0 {
			return out, RootCauseDirectionBasisOccurrences
		}
	}
	if item.MemberCount <= 1 && item.StartTs > 0 && item.EndTs > item.StartTs && item.ImpactMs > 0 {
		envelopeMs := (item.EndTs - item.StartTs) * 1000
		if diff := item.ImpactMs - envelopeMs; diff <= directionConservationToleranceMs && diff >= -directionConservationToleranceMs {
			return []foldInterval{{start: item.StartTs, end: item.EndTs}}, RootCauseDirectionBasisSingleSegment
		}
	}
	return nil, ""
}

// foldIntervalIntersectionMs is the exact interval-intersection wall clock of
// two merged unions (ms). Pure interval measure — by construction the result
// is ≤ min of the two union lengths (the 件2 identity pin).
func foldIntervalIntersectionMs(a, b []foldInterval) float64 {
	total := 0.0
	for _, x := range a {
		for _, y := range b {
			lo, hi := x.start, x.end
			if y.start > lo {
				lo = y.start
			}
			if y.end < hi {
				hi = y.end
			}
			if hi > lo {
				total += (hi - lo) * 1000
			}
		}
	}
	return total
}

// stampCrossDirectionDisclosureAndConservation runs 件2 + 件3 over ONE rank
// result (one board, one window — 同板同窗 by construction; cross-board pairs
// structurally cannot form here, honoring 跨板→不发). Disclosure lanes only:
// no gate, ordinal, sort or value channel is touched, and emission always
// proceeds (永不硬拦).
func stampCrossDirectionDisclosureAndConservation(rank *RootCauseRankResult) {
	items := rank.Items
	type populationSeat struct {
		idx       int
		union     []foldInterval
		unionMs   float64
		basis     string
		direction CausalTokenFixDirection
	}
	var seats []populationSeat
	// INTERSECT-FIX P3 软披露 (§29.143 归因件4, 2026-07-19): the
	// eligible-but-credential-less exit used to be fully silent — a basis-arm
	// regression could erase a flagship board's ∩ disclosure with zero typed
	// trace (six commits unnoticed). Count the exits and their type tokens;
	// ONE advisory caveat rides the banner face below (软引导面 only — no
	// gate, no typed observation, no render lane reads it).
	populationExits := 0
	var populationExitTypes []string
	notePopulationExit := func(typ string) {
		populationExits++
		for _, existing := range populationExitTypes {
			if existing == typ {
				return
			}
		}
		populationExitTypes = append(populationExitTypes, typ)
	}
	for i := range items {
		if !rootCauseItemDirectionPopulationEligible(&items[i]) {
			continue
		}
		intervals, basis := rootCauseItemDirectionSupport(&items[i])
		if len(intervals) == 0 {
			// envelope-level credential — out of the population
			notePopulationExit(items[i].Type)
			continue
		}
		if rank.Window.EndTs > rank.Window.StartTs {
			// 同窗口径: the support inventory backs the seat's IN-WINDOW claim
			// — clip to the board window so an actual_* audit extent reaching
			// past the window can never inflate the conservation Σ.
			clipped := make([]foldInterval, 0, len(intervals))
			for _, interval := range intervals {
				lo, hi := interval.start, interval.end
				if lo < rank.Window.StartTs {
					lo = rank.Window.StartTs
				}
				if hi > rank.Window.EndTs {
					hi = rank.Window.EndTs
				}
				if hi > lo {
					clipped = append(clipped, foldInterval{start: lo, end: hi})
				}
			}
			intervals = clipped
			if len(intervals) == 0 {
				continue
			}
		}
		merged, _ := foldIntervalUnionWithDisjoint(intervals)
		unionMs := 0.0
		for _, interval := range merged {
			unionMs += (interval.end - interval.start) * 1000
		}
		if unionMs <= 0 {
			continue
		}
		items[i].directionSupportIntervals = merged
		items[i].directionSupportBasis = basis
		seats = append(seats, populationSeat{
			idx: i, union: merged, unionMs: unionMs, basis: basis,
			direction: CausalTokenFixDirectionFor(items[i].Type),
		})
	}
	appendCaveat := func(caveat string) {
		for _, existing := range rank.Caveats {
			if existing == caveat {
				return
			}
		}
		rank.Caveats = append(rank.Caveats, caveat)
	}
	appendUndisclosed := func(idx int, partnerType string) {
		for _, existing := range items[idx].CrossDirectionOverlapUndisclosed {
			if existing == partnerType {
				return
			}
		}
		items[idx].CrossDirectionOverlapUndisclosed =
			append(items[idx].CrossDirectionOverlapUndisclosed, partnerType)
	}
	if populationExits > 0 {
		// INTERSECT-FIX P3: its OWN caveat family prefix
		// (cross_direction_population:) — never cross_direction_overlap: (件2
		// carrier-absent pairs) nor direction_conservation: (件3 violations),
		// so the three audit families keep sorting cleanly in 立案素材 triage.
		sort.Strings(populationExitTypes)
		typeList := populationExitTypes
		const populationExitTypeCap = 6
		suffix := ""
		if len(typeList) > populationExitTypeCap {
			suffix = fmt.Sprintf(" +%d more", len(typeList)-populationExitTypeCap)
			typeList = typeList[:populationExitTypeCap]
		}
		appendCaveat(fmt.Sprintf(
			"cross_direction_population: %d on-chain full seat(s) resolved no per-segment support inventory and stepped out of the cross-direction disclosure population (types: %s%s) — envelope-level credentials; disclosure coverage only, no value or seat changed",
			populationExits, strings.Join(typeList, "/"), suffix))
	}
	// ── 件2: cross-direction overlap pair table ─────────────────────────────
	type pairEntry struct {
		a, b    int // seat indexes
		overlap float64
	}
	var pairs []pairEntry
	for x := 0; x < len(seats); x++ {
		for y := x + 1; y < len(seats); y++ {
			if items[seats[x].idx].Thread.PID != items[seats[y].idx].Thread.PID {
				continue // 同线程 only (tid-first typed identity)
			}
			if seats[x].direction == seats[y].direction {
				continue // same direction: the 件3 conservation lane owns it
			}
			overlap := foldIntervalIntersectionMs(seats[x].union, seats[y].union)
			if overlap <= 0 {
				continue
			}
			pairs = append(pairs, pairEntry{a: x, b: y, overlap: overlap})
		}
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].overlap != pairs[j].overlap {
			return pairs[i].overlap > pairs[j].overlap
		}
		if pairs[i].a != pairs[j].a {
			return pairs[i].a < pairs[j].a
		}
		return pairs[i].b < pairs[j].b
	})
	rosterLen := make(map[int]int)
	for _, pair := range pairs {
		ia, ib := seats[pair.a].idx, seats[pair.b].idx
		envelopeA := items[ia].LineStart > 0 && items[ia].LineEnd >= items[ia].LineStart
		envelopeB := items[ib].LineStart > 0 && items[ib].LineEnd >= items[ib].LineStart
		capacity := rosterLen[ia] < RootCauseCrossDirectionOverlapPartnerCap &&
			rosterLen[ib] < RootCauseCrossDirectionOverlapPartnerCap
		if !envelopeA || !envelopeB || !capacity {
			// 件3 closure (与件2 闭环): the overlap is DETECTED but the
			// mutual-pointer carrier is absent (missing line envelope) or the
			// bounded roster is full — report by honest type token on BOTH
			// sides, never a fake [E#] (宁漏勿假指). Wire symmetry holds: the
			// pair appears on neither roster.
			appendUndisclosed(ia, items[ib].Type)
			appendUndisclosed(ib, items[ia].Type)
			// 修补轮 件8: the carrier-absent 件2 caveat wears its OWN family
			// prefix (cross_direction_overlap:) — the 件3 conservation
			// violations keep direction_conservation:, so the two audit
			// families sort cleanly in 立案素材 triage.
			appendCaveat(fmt.Sprintf(
				"cross_direction_overlap: overlap %.3fms between %s and %s on thread %s lacks a mutual-pointer carrier (line envelope/roster capacity) — disclosed by type token only",
				pair.overlap, items[ia].Type, items[ib].Type, threadLabel(items[ia].Thread)))
			continue
		}
		items[ia].CrossDirectionOverlaps = append(items[ia].CrossDirectionOverlaps,
			RootCauseCrossDirectionOverlap{
				OverlapMs: pair.overlap, LineStart: items[ib].LineStart, LineEnd: items[ib].LineEnd,
				Direction: string(seats[pair.b].direction), Basis: seats[pair.b].basis,
			})
		items[ib].CrossDirectionOverlaps = append(items[ib].CrossDirectionOverlaps,
			RootCauseCrossDirectionOverlap{
				OverlapMs: pair.overlap, LineStart: items[ia].LineStart, LineEnd: items[ia].LineEnd,
				Direction: string(seats[pair.a].direction), Basis: seats[pair.a].basis,
			})
		rosterLen[ia]++
		rosterLen[ib]++
	}
	// ── 件3: per-(thread, direction) conservation audit ─────────────────────
	if rank.Window.EndTs <= rank.Window.StartTs {
		return // no physical window — nothing to conserve against (绝不猜窗)
	}
	windowMs := (rank.Window.EndTs - rank.Window.StartTs) * 1000
	type conservationKey struct {
		pid       int
		direction CausalTokenFixDirection
	}
	groups := make(map[conservationKey][]int)
	sums := make(map[conservationKey]float64)
	for seatIdx, seat := range seats {
		key := conservationKey{pid: items[seat.idx].Thread.PID, direction: seat.direction}
		groups[key] = append(groups[key], seatIdx)
		sums[key] += seat.unionMs
	}
	keys := make([]conservationKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].pid != keys[j].pid {
			return keys[i].pid < keys[j].pid
		}
		return keys[i].direction < keys[j].direction
	})
	for _, key := range keys {
		if len(groups[key]) < 2 || sums[key] <= windowMs+directionConservationToleranceMs {
			continue // 合规形 publishes nothing (负臂)
		}
		for _, seatIdx := range groups[key] {
			finding := RootCauseDirectionConservation{
				Direction: string(key.direction), SumMs: sums[key],
				WindowMs: windowMs, SeatCount: len(groups[key]),
			}
			items[seats[seatIdx].idx].DirectionConservationExcess = &finding
		}
		appendCaveat(fmt.Sprintf(
			"direction_conservation: thread %s direction %s full-seat support unions sum %.3fms > window %.3fms across %d seats — same-direction physical time is double-billed (公理 v2 violation, disclosure only; no value or seat changed)",
			threadLabel(items[seats[groups[key][0]].idx].Thread), key.direction, sums[key], windowMs, len(groups[key])))
	}
}

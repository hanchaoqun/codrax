package tracequery

// rank_p3_measure.go — P3MEASURE-1 (§29.169 user ruling chain, 2026-07-20):
//
// 榜语义定谳 (RULE3-1 件12⑦, §29.185② verbatim, 2026-07-21): 可消除榜=优化
// 归因提醒面 — 「能优化的就是可以优化的,尽量提醒用户去优化」. Counterfactual
// validity (节拍吞噬类) is NOT a hidden gate: when stage two discloses,
// invalid shares gain a NOTE beside the seat, never a hidden-seat or a
// swallow discount (不过度考虑吞噬折扣); stage-two increments (三列命名/
// 白名单) design under this semantics.
// the ONCHAIN-P3 stage-one SILENT two-dimension measurement of every on-chain
// seat. Display-only wire, model/user double-invisible (双不可见) — nothing
// in this file may touch a value, ordinal, seat, lane, caveat or fingerprint
// channel, and the wire keys it feeds are registered display_only with NO
// parsing or rendering consumer anywhere (pinned).
//
// ── The ruled caliber (§29.169, verbatim chain) ─────────────────────────────
//
// The user upgraded the P3 deviation caliber from the structural edge-witness
// reading to the COUNTERFACTUAL reading: 「如果 Worker-200 早完成一点,早一点
// 唤醒 UI-100,那么省出来的时间都应该算做窗内可消除的影响」 — a seat's
// anchor-window time is VALID when shortening it moves the closing edge
// earlier and therefore wakes the target earlier. Exactly TWO counterexample
// families break that inference, and only TYPED criteria may prove one
// (反例族只认 typed 精确判据,禁新启发式):
//
//   family ① periodic/absolute-deadline-pinned edges (周期/绝对期限钉死边):
//     the anchor window's right-endpoint edge is emitted by a measured
//     periodic signal source (timer / vsync / cadence family) — the edge
//     fires on the cadence, not on work completion, so shortening the
//     in-window time does NOT move it. Typed judge = the EXISTING VS-1
//     periodic-source discount classification (detectPeriodicWakeupSource:
//     WakeupCausalImpact/Aggregate.PeriodicSource) — the same machine that
//     already discounts the seat's published attribution. NO new heuristic.
//
//   family ② late-relay (晚到中继形): the worker's outgoing edge time is
//     pinned by its own LATE incoming edge, not by its visible work. No
//     existing typed classification carries a deterministic per-edge
//     criterion for this form (any "relay gap below X" threshold would be a
//     NEW heuristic, which the ruling forbids), and multi-hop chain
//     expansion already absorbs the relay's upstream into its own node
//     windows. THIS ROUND DOES NOT MEASURE family ② — the measurement
//     discloses its own coverage honestly (宁缺勿噪):
//     p3m_coverage=families:[periodic_pinned].
//
// Ruled-legal forms count VALID (已裁合法形计 valid): the 97.8% 兼服/边前继承
// shape (onchain_segment_audit_20260718.md, reading A = R4 pre-edge scope)
// and the self 恒链上 ruling are SETTLED — the measurement records only true
// deviations and NEVER re-litigates a ruling through data (§29.169 red line).
// Concretely: absence of a typed counterexample hit ⇒ VALID; self seats
// record a typed disposition and NO numbers.
//
// ── The two dimensions per seat ─────────────────────────────────────────────
//
//   counterfactual (family-①): valid_ms + invalid_ms == the seat's
//     anchor-window time, µs-exact by integer-µs construction (恒等 pin).
//     invalid = the portion whose anchor window closes on a typed
//     periodic-pinned edge.
//
//   structural (P3 原结构口径, 审计同源): edge_witnessed_ms = the seat's
//     anchor-window segment time in segments that carry a real outgoing
//     edge toward the chain closure ([seg.start, seg.end +
//     rspaIOCompletionClosureTolS], sched_wakeup/waking, wakee ∈
//     {target} ∪ chain nodes — the exact probe caliber of
//     onchain_segment_audit_20260718.md §2 and
//     TestChainBudgetTiebaLeakConvergesAndLegacyAnchorHolds). Pinned
//     edge_witnessed ≤ 席值; a seat whose published value rides a DISCOUNTED
//     caliber (value below its own segment measure) withholds this dimension
//     instead of publishing a >值 number (measured_counterfactual_only).
//
// ── Hard disciplines ────────────────────────────────────────────────────────
//
//   - 值/榜/席位/板指纹零动: this pass writes ONLY the four P3M* fields.
//     Four-flagship-board A/B byte-identity is pinned.
//   - zero new trace scans: the edge inventory joins through the SAME
//     indexed access the wakeup-edge census uses (cache.wakeupsForThread +
//     the idx.Events fallback); anchor windows come from the chain the rank
//     build already holds; support segments come from the seats' own typed
//     carriers (rootCauseItemDirectionSupport — the ONE hardened inventory
//     resolver; no second inventory authority).
//   - 禁截断库存二次聚合: an aggregate seat whose OccurrenceWindows sit at
//     the wire cap is NOT measured (occurrence_inventory_capped) — a capped
//     inventory never re-aggregates.
//   - advisory-only red line (supply_pressure 分离先例): these fields and
//     their p3m_* wire keys are MEASUREMENT ONLY. They MUST NEVER drive a
//     hard gate, a lane/seat/ordinal decision, or any rendered face. Stage
//     two (data-gated disclosure) requires a NEW user ruling and must speak
//     见证下界 semantics, never a ratio form (§29.169).

import (
	"math"
	"sort"
	"strings"
)

// P3MeasureCoverageFamilies is the measurement's own coverage disclosure
// (量测披露自身覆盖): the counterexample families this round actually
// measures. family ② (late-relay) is honestly excluded — see the file
// header. Emitted verbatim on the p3m_coverage display-only wire key.
const P3MeasureCoverageFamilies = "families:[periodic_pinned]"

// The typed measurement dispositions (closed set — the p3m_disposition wire
// token). "measured_*" tokens carry numbers; the rest are honest absence.
const (
	// p3mDispositionSegmentJoin — both dimensions measured from the seat's
	// typed support segments ∩ its pid's merged chain anchor windows.
	p3mDispositionSegmentJoin = "measured_segment_join"
	// p3mDispositionEdgeTerminatedWindow — chain-sourced seats: the windows
	// ARE edge-closed by construction (every node window's right endpoint is
	// the pid's own real sched_wakeup), so the structural dimension is
	// window-granular (min(base, 席值)), not a segment join.
	p3mDispositionEdgeTerminatedWindow = "measured_edge_terminated_window"
	// p3mDispositionCounterfactualOnly — the counterfactual dimension is
	// measured, but the seat's published value rides a discounted caliber
	// below its own segment measure, so a segment-join edge_witnessed would
	// exceed 席值; the structural dimension is withheld (pin-true honesty).
	p3mDispositionCounterfactualOnly = "measured_counterfactual_only"
	// p3mDispositionSelfRuled — the analysis target's own seats (pid ==
	// target, or a self_* on-chain basis): 恒链上 is a ruling (XLANE-1 /
	// R8 / §29.169 已裁合法形计 valid). No numbers — publishing a per-seat
	// deviation figure here would re-litigate the ruling through data.
	p3mDispositionSelfRuled = "self_ruled"
	// p3mDispositionNoTypedInventory — the seat resolves no typed
	// per-segment support inventory (envelope-only rows, count-caliber
	// rows): absence never guesses, nothing is measured.
	p3mDispositionNoTypedInventory = "no_typed_inventory"
	// p3mDispositionNoAnchorWindows — a chain-window-basis seat whose pid
	// holds no depth>0 chain anchor window (and no thread identity forms):
	// there is no anchor time to decompose.
	p3mDispositionNoAnchorWindows = "no_anchor_windows"
	// p3mDispositionOccurrenceCapped — an aggregate seat whose occurrence
	// window inventory sits at the wire cap (possibly trimmed): 禁截断库存
	// 二次聚合 — the measurement refuses a capped inventory.
	p3mDispositionOccurrenceCapped = "occurrence_inventory_capped"
)

// p3mWindowEndEpsS is the window-end ↔ impact-window match tolerance (1µs,
// the codebase-wide µs identity grain) used to map a merged anchor window's
// right endpoint back onto the typed impact that closed it.
const p3mWindowEndEpsS = 1e-6

// p3mWindowFlag is one (window-end, periodic) reading lifted verbatim from a
// typed WakeupCausalImpact — the family-① judge for the anchor window that
// ends at that edge.
type p3mWindowFlag struct {
	endTs    float64
	periodic bool
}

// p3MeasureContext carries the chain-derived typed inputs of the measurement
// from the rank build lane (chain + cache + idx in scope) to the ONE shared
// finalize tail. Unexported, never serialized.
type p3MeasureContext struct {
	targetPID int
	// anchors — chainAnchorWindowsByPID(chain): merged depth>0 node windows
	// per non-target chain pid (the RSPA anchor authority, reused verbatim).
	anchors map[int][]TimeWindow
	// windowFlags — per pid, every typed impact's (window end, PeriodicSource)
	// pair; the per-window family-① judge (VS-1 stamps members in place).
	windowFlags map[int][]p3mWindowFlag
	// pidPeriodic — pid-level OR of the typed VS-1 flags (aggregates ∪
	// impacts). The judge for host-edge-basis seats whose credential edge
	// carries no per-window impact record.
	pidPeriodic map[int]bool
	// edgeTs — per waker pid, the sorted timestamps of raw sched_wakeup /
	// sched_waking rows whose wakee is in the chain closure ({target} ∪
	// node threads). Same indexed inventory access as the wakeup-edge
	// census; zero new trace passes.
	edgeTs map[int][]float64
}

// buildP3MeasureContext assembles the measurement context from inventories
// the rank build already holds. Returns nil when there is no chain universe
// (no chain ⇒ no on-chain seats ⇒ nothing to measure).
func buildP3MeasureContext(idx *Index, cache *chainQueryCache, chain *ChainResult) *p3MeasureContext {
	if idx == nil || chain == nil || (chain.Target.PID <= 0 && len(chain.Nodes) == 0) {
		return nil
	}
	ctx := &p3MeasureContext{
		targetPID:   chain.Target.PID,
		anchors:     chainAnchorWindowsByPID(*chain),
		windowFlags: map[int][]p3mWindowFlag{},
		pidPeriodic: map[int]bool{},
		edgeTs:      map[int][]float64{},
	}
	for i := range chain.CausalImpacts {
		impact := &chain.CausalImpacts[i]
		if impact.Thread.PID <= 0 || impact.Window.EndTs <= impact.Window.StartTs {
			continue
		}
		ctx.windowFlags[impact.Thread.PID] = append(ctx.windowFlags[impact.Thread.PID],
			p3mWindowFlag{endTs: impact.Window.EndTs, periodic: impact.PeriodicSource})
		if impact.PeriodicSource {
			ctx.pidPeriodic[impact.Thread.PID] = true
		}
	}
	for i := range chain.AggregatedImpacts {
		aggregate := &chain.AggregatedImpacts[i]
		if aggregate.Thread.PID > 0 && aggregate.PeriodicSource {
			ctx.pidPeriodic[aggregate.Thread.PID] = true
		}
	}
	// --- edge inventory: census-identical access pattern -------------------
	population := make([]ThreadRef, 0, len(chain.Nodes)+1)
	seenWakee := map[string]bool{}
	addWakee := func(thread ThreadRef) {
		if thread.PID <= 0 && strings.TrimSpace(thread.Comm) == "" {
			return
		}
		key := threadKey(thread)
		if seenWakee[key] {
			return
		}
		seenWakee[key] = true
		population = append(population, thread)
	}
	addWakee(chain.Target)
	for _, node := range chain.Nodes {
		addWakee(node.Thread)
	}
	seenEvent := map[int]bool{}
	visit := func(wakee ThreadRef, i int) {
		if i < 0 || i >= len(idx.Events) || seenEvent[i] {
			return
		}
		ev := &idx.Events[i]
		if ev.Type != EventSchedWakeup && ev.Type != EventSchedWaking {
			return
		}
		if schedWakeupStartsNewIncarnation(*ev) || ev.PID <= 0 {
			return
		}
		if !threadMatches(wakee, ev.WakeePID, ev.WakeeComm) {
			return
		}
		seenEvent[i] = true
		ctx.edgeTs[ev.PID] = append(ctx.edgeTs[ev.PID], ev.Ts)
	}
	for _, wakee := range population {
		if cache != nil && wakee.PID > 0 {
			if ids, indexed := cache.wakeupsForThread(wakee); indexed {
				for _, id := range ids {
					visit(wakee, id)
				}
				continue
			}
		}
		for i := range idx.Events {
			visit(wakee, i)
		}
	}
	for pid := range ctx.edgeTs {
		sort.Float64s(ctx.edgeTs[pid])
	}
	return ctx
}

// p3mRoundUs converts a duration in SECONDS to integer microseconds (the
// measurement's exact-arithmetic grain: valid+invalid==base holds by int64
// construction, and float64(µs)/1000 is an exact ms wire value).
func p3mRoundUs(sec float64) int64 {
	if sec <= 0 {
		return 0
	}
	return int64(math.Round(sec * 1e6))
}

// p3mOverlapS is the overlap, in seconds, of [aStart,aEnd] with [bStart,bEnd].
func p3mOverlapS(aStart, aEnd, bStart, bEnd float64) float64 {
	lo, hi := aStart, aEnd
	if bStart > lo {
		lo = bStart
	}
	if bEnd < hi {
		hi = bEnd
	}
	if hi <= lo {
		return 0
	}
	return hi - lo
}

// p3mSegmentHasEdge reports whether the pid emitted a raw edge toward the
// chain closure inside [start, end + rspaIOCompletionClosureTolS] — the
// audit probe's segment-witness caliber (audit 同源, closure tolerance
// shared with the M-IO completion pairing).
func p3mSegmentHasEdge(edges []float64, start, end float64) bool {
	if len(edges) == 0 || end <= start {
		return false
	}
	hi := end + rspaIOCompletionClosureTolS
	i := sort.SearchFloat64s(edges, start)
	return i < len(edges) && edges[i] <= hi
}

// p3mWindowPeriodic is the family-① judge for one merged anchor window: the
// typed impact whose window END closed this merged span (within 1µs) speaks;
// no matching typed record ⇒ NOT proven ⇒ valid (typed 精确判据 only,
// absence never convicts).
func p3mWindowPeriodic(flags []p3mWindowFlag, windowEnd float64) bool {
	for _, flag := range flags {
		if flag.periodic && math.Abs(flag.endTs-windowEnd) <= p3mWindowEndEpsS {
			return true
		}
	}
	return false
}

// p3mClearSeat resets the measurement fields (idempotent re-stamp base).
func p3mClearSeat(item *RootCauseRankItem) {
	item.P3MCounterfactualValidMs = 0
	item.P3MCounterfactualInvalidMs = 0
	item.P3MEdgeWitnessedMs = 0
	item.P3MDisposition = ""
}

// stampP3CounterfactualMeasure runs the P3MEASURE-1 silent measurement over
// the PUBLISHED board (rank.Items). Mounted on the ONE shared finalize tail
// (attachPerfContextToRootCauseRankWithIndex) AFTER every sort / capacity /
// ordinal / disclosure decision: pure additive stamp, no other channel moves.
func stampP3CounterfactualMeasure(rank *RootCauseRankResult) {
	if rank == nil {
		return
	}
	ctx := rank.p3MeasureCtx
	for i := range rank.Items {
		p3mMeasureSeat(&rank.Items[i], ctx)
	}
}

// p3mMeasureSeat measures ONE seat. Population = typed on-chain relevance ∧
// OnChainBasis in the closed set (§29.169 定形) — everything else stays
// byte-untouched (beyond the idempotency clear).
func p3mMeasureSeat(item *RootCauseRankItem, ctx *p3MeasureContext) {
	p3mClearSeat(item)
	if ctx == nil {
		return
	}
	if !rootCauseItemIsOnChain(*item) {
		return
	}
	// B829: this semantic basis proves a host→target relation and a raw
	// pre-edge occupancy boundary, not a semantic completion dependency. Do
	// not resurrect a positive causal-looking millisecond through the silent
	// counterfactual audit after the priced effective channel was zeroed. The
	// host-edge STATE sibling remains in the measurement population.
	if rootCauseItemIsRelationOnlySemantic(*item) {
		return
	}
	switch item.OnChainBasis {
	case "", RootCauseOnChainBasisSelfDeterministicSpan,
		RootCauseOnChainBasisSelfWallClockInterval,
		RootCauseOnChainBasisSemanticChainIntervalRelation,
		RootCauseOnChainBasisHostWakeupEdge,
		RootCauseOnChainBasisHostWakeupEdgeState:
	default:
		return // unknown basis: outside the ruled closed set, not measured
	}
	// ── self-ruled lane (恒链上 = settled; no numbers, 禁重诉既裁) ──────────
	if item.OnChainBasis == RootCauseOnChainBasisSelfDeterministicSpan ||
		item.OnChainBasis == RootCauseOnChainBasisSelfWallClockInterval ||
		(ctx.targetPID > 0 && item.Thread.PID == ctx.targetPID) {
		item.P3MDisposition = p3mDispositionSelfRuled
		return
	}
	if item.Thread.PID <= 0 {
		item.P3MDisposition = p3mDispositionNoAnchorWindows
		return
	}
	valueUs := p3mRoundUs(rootCauseEffectiveImpactMs(*item) / 1000)
	// ── chain-sourced seats: constructive edge-terminated windows ──────────
	if strings.HasPrefix(item.Source, "wakeup_chain.") {
		var windows []foldInterval
		if item.Source == "wakeup_chain.aggregated_impacts" {
			if len(item.OccurrenceWindows) >= wakeupCausalAggregateOccurrenceCap {
				// len==cap means the wire inventory may be a trimmed view of
				// the occurrence census — a capped inventory never
				// re-aggregates (禁截断库存二次聚合).
				item.P3MDisposition = p3mDispositionOccurrenceCapped
				return
			}
			for _, occ := range item.OccurrenceWindows {
				if occ.Window.EndTs > occ.Window.StartTs {
					windows = append(windows, foldInterval{start: occ.Window.StartTs, end: occ.Window.EndTs})
				}
			}
		} else if item.EndTs > item.StartTs {
			windows = []foldInterval{{start: item.StartTs, end: item.EndTs}}
		}
		if len(windows) == 0 {
			item.P3MDisposition = p3mDispositionNoTypedInventory
			return
		}
		merged, _ := foldIntervalUnionWithDisjoint(windows)
		var baseUs int64
		for _, w := range merged {
			baseUs += p3mRoundUs(w.end - w.start)
		}
		var invalidUs int64
		if item.PeriodicSource {
			// The seat's own typed VS-1 mirror: every member window of a
			// periodic aggregate/impact is cadence-closed (detect stamps the
			// members in place) — the whole anchor time is counterfactually
			// invalid, exactly where the periodic discount machine already
			// sits (校准 pin ②: 两机器同判).
			invalidUs = baseUs
		}
		item.P3MCounterfactualValidMs = float64(baseUs-invalidUs) / 1000
		item.P3MCounterfactualInvalidMs = float64(invalidUs) / 1000
		witnessedUs := baseUs
		if valueUs < witnessedUs {
			// Window-granular constructive witness: the published value (≤
			// the window time by caliber, e.g. a discounted periodic eff)
			// sits entirely inside edge-terminated windows — 席值 is the
			// honest cap (edge_witnessed ≤ 席值 pin).
			witnessedUs = valueUs
		}
		item.P3MEdgeWitnessedMs = float64(witnessedUs) / 1000
		item.P3MDisposition = p3mDispositionEdgeTerminatedWindow
		return
	}
	// ── typed support inventory (the ONE hardened resolver) ────────────────
	support, _ := rootCauseItemDirectionSupport(item)
	if len(support) == 0 {
		item.P3MDisposition = p3mDispositionNoTypedInventory
		return
	}
	segments, _ := foldIntervalUnionWithDisjoint(support)
	var baseUs, invalidUs, witnessedUs int64
	switch item.OnChainBasis {
	case RootCauseOnChainBasisHostWakeupEdge, RootCauseOnChainBasisHostWakeupEdgeState:
		// The host-edge credential clipped the inventory to the pre-edge
		// share at the mint — the support IS the seat's anchor time.
		// 复核收编 (P2-2, 2026-07-20): the family-① judge prefers the seat's
		// OWN typed edge address — a chain-member host carries
		// HostWakeupEdgeAnchorTs, and the pid's per-window impact flags can
		// judge exactly the window that edge closes (the coarse pid-level OR
		// over-convicted mixed-cadence hosts: strictly conservative, but the
		// precise judge was available). A non-node host (no per-window
		// impact record) falls back to the pid-level typed VS-1 flag.
		periodic := ctx.pidPeriodic[item.Thread.PID]
		if flags := ctx.windowFlags[item.Thread.PID]; len(flags) > 0 && item.HostWakeupEdgeAnchorTs > 0 {
			periodic = p3mWindowPeriodic(flags, item.HostWakeupEdgeAnchorTs)
		}
		for _, seg := range segments {
			segUs := p3mRoundUs(seg.end - seg.start)
			baseUs += segUs
			if periodic {
				invalidUs += segUs
			}
			if p3mSegmentHasEdge(ctx.edgeTs[item.Thread.PID], seg.start, seg.end) {
				witnessedUs += segUs
			}
		}
	default: // basis "" — legacy chain-window overlap (RSPA state seats,
		// semantic overlap seats, satellites)
		anchors := ctx.anchors[item.Thread.PID]
		if len(anchors) == 0 {
			item.P3MDisposition = p3mDispositionNoAnchorWindows
			return
		}
		flags := ctx.windowFlags[item.Thread.PID]
		edges := ctx.edgeTs[item.Thread.PID]
		for _, seg := range segments {
			witnessed := p3mSegmentHasEdge(edges, seg.start, seg.end)
			for _, w := range anchors {
				overlapUs := p3mRoundUs(p3mOverlapS(seg.start, seg.end, w.StartTs, w.EndTs))
				if overlapUs == 0 {
					continue
				}
				baseUs += overlapUs
				if p3mWindowPeriodic(flags, w.EndTs) {
					invalidUs += overlapUs
				}
				if witnessed {
					witnessedUs += overlapUs
				}
			}
		}
	}
	item.P3MCounterfactualValidMs = float64(baseUs-invalidUs) / 1000
	item.P3MCounterfactualInvalidMs = float64(invalidUs) / 1000
	if witnessedUs > valueUs+1 {
		// The seat publishes a discounted caliber below its own segment
		// measure — a segment-join edge_witnessed would overshoot 席值.
		// Withhold the structural dimension instead of publishing a lie
		// (edge_witnessed ≤ 席值 stays a hard wire invariant).
		item.P3MEdgeWitnessedMs = 0
		item.P3MDisposition = p3mDispositionCounterfactualOnly
		return
	}
	item.P3MEdgeWitnessedMs = float64(witnessedUs) / 1000
	item.P3MDisposition = p3mDispositionSegmentJoin
}

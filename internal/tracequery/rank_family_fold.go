package tracequery

// rank_family_fold.go — RCM engine half (ledger §24.7.1 / §24.10 / §24.12
// dimension A, user rulings 2026-07-08, real_trace_campaign_20260705.md).
//
// Two folds live here, one merge philosophy:
//
//  1. FoldSemanticSpanFamilies (§24.10): all window-clipped semantic compile
//     spans of one (thread, semantic class, chain lane) fold into ONE
//     SemanticSpanFamily whose participation value is the WINDOW-PROJECTION
//     TOTAL of the member segments (interval union; disjoint == Σ; union < Σ
//     discloses). BOTH consumers — the rank minting loop in
//     buildRootCauseRankFrom and the typed observation channel in
//     internal/tool/trace_query.go — read THIS one function, so the two faces
//     can never publish two different family shapes (§24.12 mandate: 两消费方
//     同一函数). cmp_78_01 witness: (com.xs.fm.lite-6565, class_verification)
//     ×14 same-thread spans, pairwise disjoint, union == Σ == 7.124ms.
//
//  2. foldSameThreadTypeRankFamilies (§24.7.1): the per-instance window-stats
//     rank families (io_latency / block_io_by_inode / file_io_hot_inode /
//     page_cache_churn / io_burst_episode / workqueue_activity /
//     dma_fence_activity / scheduler_latency — declared in the causal-token
//     registry family-fold column, single source) merge same-(thread,type)
//     rows into ONE contender BEFORE sortRootCauseRankItems, so the family
//     competes with its combined magnitude instead of splitting its own vote
//     (拆分参赛=弱化排序). opendir_78 witness: 块设备IO(inode) 1.136ms rank#3
//     + 0.462ms rank#8 → one 1.598ms contender (disjoint segments, legal
//     same-thread sum), with the real distinguishing keys (two different
//     inodes) preserved in the typed MemberRoster (§24.7.1 ①).
//
// 口径红线 (§7.2.1/§7.4/§7.5 untouched): wall clock NEVER sums across
// threads; a same-thread sum is legal only for provably disjoint segments.
// Overlapping/unprovable members publish the interval union or fall back to
// the member MAX (honest lower bound) — see RootCauseMemberFoldCaliber*.
//
// 跨窗纪律 (M3, §24.12 mandate ②): a family only merges rows measured over
// the SAME selected window — the typed StatsWindowStartTs/EndTs identity is
// part of the merge key, so cross-window re-measurements of one physical
// segment never double-sum (RN-12 伪包含 same-type defense). Cross-window
// rows stay as separate lines.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// --- shared interval algebra -------------------------------------------------

type foldInterval struct {
	start, end float64
	// valueMs is the SAME-SOURCE published account of the member this
	// interval was copied from (CASE-1, §29.52 立案, 2026-07-13) — the µs
	// value-identity dimension of the D/IO cross-lane reconciliation, whose
	// per-(thread,cpu) group intervals are segment HULLS (G1 明拒 hull
	// containment; membership is exact same-source float identity instead).
	// Zero = no value carrier (legacy inventories): the value-gated arms
	// fail OPEN (never absorb). The interval-union algebra ignores it.
	valueMs float64
}

// priorityFoldRelationSourcesLexicallyClosed is the engine-side minimum
// provenance closure for a duration-bearing priority family member. Physical
// artifact membership is rechecked against the result universe at the final
// publication boundary; the fold still must reject empty, malformed, or
// mixed compatibility/production identities before sibling union can hide
// them. Whitespace and non-canonical integer spellings are rejected rather
// than normalized because this is an authority token, not display prose.
func priorityFoldRelationSourcesLexicallyClosed(sources []string) bool {
	if len(sources) == 0 {
		return false
	}
	mode := ""
	for _, source := range sources {
		if source == "" || source != strings.TrimSpace(source) {
			return false
		}
		candidateMode := ""
		switch {
		case source == "compat:index":
			candidateMode = "compat"
		case strings.HasPrefix(source, "artifact:"):
			rawIndex := strings.TrimPrefix(source, "artifact:")
			index, err := strconv.Atoi(rawIndex)
			if err != nil || index < 0 || strconv.Itoa(index) != rawIndex {
				return false
			}
			candidateMode = "artifact"
		default:
			return false
		}
		if mode != "" && mode != candidateMode {
			return false
		}
		mode = candidateMode
	}
	return mode != ""
}

func priorityFoldEffectiveMemberAuthorityClosed(member RootCauseRankItem, effective float64) bool {
	if effective <= 0 || math.IsNaN(effective) || math.IsInf(effective, 0) ||
		!priorityEvidenceCaliberIsHard(member.PriorityRelationCaliber) {
		return false
	}
	proven := member.PriorityRelationProvenLowerMs
	if proven <= 0 || math.IsNaN(proven) || math.IsInf(proven, 0) {
		return false
	}
	const accountEpsilonMs = 1e-9
	return effective <= proven+accountEpsilonMs &&
		priorityFoldRelationSourcesLexicallyClosed(member.PriorityRelationArtifactSources)
}

// foldIntervalUnionWithDisjoint returns the sorted, non-overlapping union of valid
// [start,end] intervals plus whether the input intervals were pairwise
// disjoint. Adjacent half-open intervals remain separate (and disjoint); that
// distinction matters to the fold-caliber disclosure even though it does not
// change the total length.
func foldIntervalUnionWithDisjoint(intervals []foldInterval) (merged []foldInterval, disjoint bool) {
	valid := make([]foldInterval, 0, len(intervals))
	for _, interval := range intervals {
		if interval.end > interval.start {
			valid = append(valid, interval)
		}
	}
	if len(valid) == 0 {
		return nil, true
	}
	sort.Slice(valid, func(i, j int) bool {
		if valid[i].start != valid[j].start {
			return valid[i].start < valid[j].start
		}
		return valid[i].end < valid[j].end
	})
	disjoint = true
	current := valid[0]
	for _, interval := range valid[1:] {
		if interval.start < current.end {
			disjoint = false
			if interval.end > current.end {
				current.end = interval.end
			}
			continue
		}
		merged = append(merged, current)
		current = interval
	}
	return append(merged, current), disjoint
}

// foldIntervalUnionMs computes the total length (ms) of the union of the
// given [start,end] second-intervals plus whether they are pairwise disjoint.
// Invalid intervals are ignored rather than acquiring negative duration.
func foldIntervalUnionMs(intervals []foldInterval) (unionMs float64, disjoint bool) {
	merged, disjoint := foldIntervalUnionWithDisjoint(intervals)
	for _, interval := range merged {
		unionMs += (interval.end - interval.start) * 1000
	}
	return unionMs, disjoint
}

type semanticChainWindow struct {
	interval foldInterval
	state    string
	depth    int
	branch   int
}

type semanticChainIntersectionProjection struct {
	projection            semanticTraceSpanProjection
	dominantStateImpactMs float64
}

// semanticTraceSpanChainIntersectionProjection is the shared single/family
// projection primitive. It computes exactly
//
//	union(member intervals) ∩ union(same-thread chain node/impact windows)
//
// by unioning all pairwise intersections. Consequently overlapping family
// members, overlapping chain windows, and duplicate node+impact faces each
// contribute at most once. The wrapper intentionally returns the existing
// projection carrier so the single-span mint can consume the same algebra.
func semanticTraceSpanChainIntersectionProjection(chain *ChainResult, thread ThreadRef, memberIntervals []foldInterval) semanticTraceSpanProjection {
	return semanticTraceSpanChainIntersection(chain, thread, memberIntervals).projection
}

func semanticTraceSpanChainIntersection(chain *ChainResult, thread ThreadRef, memberIntervals []foldInterval) semanticChainIntersectionProjection {
	if chain == nil {
		return semanticChainIntersectionProjection{}
	}
	members, _ := foldIntervalUnionWithDisjoint(memberIntervals)
	if len(members) == 0 {
		return semanticChainIntersectionProjection{}
	}
	var windows []semanticChainWindow
	for _, node := range chain.Nodes {
		if !sameThreadRef(node.Thread, thread) || node.Window.EndTs <= node.Window.StartTs {
			continue
		}
		state := string(node.Dominant)
		depth, branch := node.Depth, node.Branch
		if node.Impact != nil {
			if depth == 0 {
				depth = node.Impact.ChainDepth
			}
			if branch == 0 {
				branch = node.Impact.ChainBranch
			}
			if state == "" {
				state = node.Impact.DominantState
			}
		}
		windows = append(windows, semanticChainWindow{
			interval: foldInterval{start: node.Window.StartTs, end: node.Window.EndTs},
			state:    state, depth: depth, branch: branch,
		})
	}
	for _, impact := range chain.CausalImpacts {
		if !sameThreadRef(impact.Thread, thread) || impact.Window.EndTs <= impact.Window.StartTs {
			continue
		}
		windows = append(windows, semanticChainWindow{
			interval: foldInterval{start: impact.Window.StartTs, end: impact.Window.EndTs},
			state:    impact.DominantState, depth: impact.ChainDepth, branch: impact.ChainBranch,
		})
	}
	if len(windows) == 0 {
		return semanticChainIntersectionProjection{}
	}

	type windowOverlap struct {
		window    semanticChainWindow
		overlapMs float64
	}
	var allIntersections []foldInterval
	stateIntersections := map[string][]foldInterval{}
	var stateOrder []string
	stateSeen := map[string]bool{}
	var windowOverlaps []windowOverlap
	for _, window := range windows {
		var intersections []foldInterval
		for _, member := range members {
			start := maxFloat(member.start, window.interval.start)
			end := minFloat(member.end, window.interval.end)
			if end <= start {
				continue
			}
			intersections = append(intersections, foldInterval{start: start, end: end})
		}
		if len(intersections) == 0 {
			continue
		}
		overlapMs, _ := foldIntervalUnionMs(intersections)
		windowOverlaps = append(windowOverlaps, windowOverlap{window: window, overlapMs: overlapMs})
		allIntersections = append(allIntersections, intersections...)
		if window.state != "" {
			if !stateSeen[window.state] {
				stateSeen[window.state] = true
				stateOrder = append(stateOrder, window.state)
			}
			stateIntersections[window.state] = append(stateIntersections[window.state], intersections...)
		}
	}
	projected, _ := foldIntervalUnionWithDisjoint(allIntersections)
	if len(projected) == 0 {
		return semanticChainIntersectionProjection{}
	}
	result := semanticChainIntersectionProjection{}
	result.projection.OnChain = true
	result.projection.StartTs = projected[0].start
	result.projection.EndTs = projected[len(projected)-1].end
	for _, interval := range projected {
		result.projection.ImpactMs += (interval.end - interval.start) * 1000
	}
	for _, state := range stateOrder {
		stateMs, _ := foldIntervalUnionMs(stateIntersections[state])
		if stateMs > result.dominantStateImpactMs {
			result.dominantStateImpactMs = stateMs
			result.projection.DominantState = state
		}
	}
	bestWindowMs := 0.0
	for _, overlap := range windowOverlaps {
		if result.projection.DominantState != "" && overlap.window.state != result.projection.DominantState {
			continue
		}
		if overlap.overlapMs <= bestWindowMs {
			continue
		}
		bestWindowMs = overlap.overlapMs
		result.projection.ChainDepth = overlap.window.depth
		result.projection.ChainBranch = overlap.window.branch
	}
	return result
}

// --- 1. semantic span families (§24.10) ---------------------------------------

// semanticSpanFamilyAdmits is the single admission predicate shared by the
// fold and by the rank loop's "already family-consumed" skip — byte-for-byte
// the old rootCauseItemFromSemanticTraceSpan admission gate (typed name-class
// classifier; zero-duration and reversed spans stay out).
func semanticSpanFamilyAdmits(span TraceSpanSummary) bool {
	if span.DurationMs <= 0 || span.EndTs <= span.StartTs {
		return false
	}
	_, ok := traceSpanSemanticWorkClass(span.Name)
	return ok
}

// semanticSpanChainLane is the three-valued family 道别 (SELF-SEM, §29.61.1):
// the legacy bool split into a closed enum so the two ON-CHAIN calibers —
// chain-window intersection vs the target's own window-projection union —
// never fold into one family (两把尺禁混折; the overlap form is structurally
// near-empty for self spans, the enum is the defense).
type semanticSpanChainLane int

const (
	semanticSpanLaneOff semanticSpanChainLane = iota
	semanticSpanLaneChainOverlap
	semanticSpanLaneChainSelf
	// semanticSpanLaneChainEdgeAnchored (R3-IMPL, §29.88.1 user ruling
	// 2026-07-14): a NON-target host's span admitted by the host's own
	// in-window typed wakeup edge toward the target, its pre-edge share
	// carrying raw occupancy (relation credential; effective attribution is
	// zero until a stronger binding exists). The FOURTH
	// lane value — the three on-chain calibers (overlap / self union /
	// pre-edge projection) never cross-fold.
	semanticSpanLaneChainEdgeAnchored
)

// selfDeterministicSemanticSpanLane is THE shared SELF-SEM admission predicate
// (§29.61.1 user ruling 2026-07-13; §2.1 of the RANK-U design — ONE
// implementation, two mint-time consumers: the family fold lane and the
// single-span E2 fall-through arm; the enrich pass only KEEPS the verdict).
// All-typed conditions:
//   - a chain universe exists (nodes/edges/causal impacts — no chain, no
//     "on-chain" identity to grant);
//   - chain.Target is resolved (PID or comm — absence never guesses, the
//     stampRootCauseRankAnalysisTargetSubject discipline);
//   - the span's thread IS the target (sameThreadRef — the engine's tid-first
//     identity comparator, never a label heuristic);
//   - the span is a deterministic semantic work class (traceSpanSemanticWorkClass
//     — the ONE six-class registry incl. normalized custom patterns; no second
//     list) with a positive in-window extent.
//
// Callers apply it ONLY on the no-overlap fall-through: a genuine chain-window
// intersection keeps the legacy overlap lane byte-identically (N5 pin; 件3
// M5 突变自检: moving the self arm ahead of the overlap check reds
// TestSelfSemOverlapFamilyKeepsIntersectionLaneOnBothFaces).
// §23.1 道别红线 stays untouched for every NON-target thread: the predicate's
// third condition is exactly the red line's protected boundary (its witness is
// an other-process span — structurally different, ruling ③).
func selfDeterministicSemanticSpanLane(chain *ChainResult, span TraceSpanSummary) bool {
	if chain == nil {
		return false
	}
	if len(chain.Nodes) == 0 && len(chain.Edges) == 0 && len(chain.CausalImpacts) == 0 {
		return false
	}
	if chain.Target.PID <= 0 && strings.TrimSpace(chain.Target.Comm) == "" {
		return false
	}
	if !sameThreadRef(span.Thread, chain.Target) {
		return false
	}
	if span.DurationMs <= 0 || span.EndTs <= span.StartTs {
		return false
	}
	_, ok := traceSpanSemanticWorkClass(span.Name)
	return ok
}

// semanticSpanFamilyLane is the DCS E2 道别 predicate applied at family-fold
// time: on-chain requires a SAME-THREAD chain node / causal-impact window
// overlap with the (already window-clipped) span — thread membership alone
// never flips a lane (道别红线; the huadong E21 adjacent precedent). Returns
// the lane plus the best-overlap dominant state / chain depth for display
// context.
//
// SELF-SEM (§29.61.1): the no-overlap fall-through consults the shared self
// predicate — the target's OWN deterministic spans take the chain_self lane
// (no fabricated overlap: depth/state stay zero/empty).
func semanticSpanFamilyLane(chain *ChainResult, span TraceSpanSummary) (lane semanticSpanChainLane, depth int, state string) {
	projection := semanticTraceSpanChainIntersectionProjection(chain, span.Thread, []foldInterval{{start: span.StartTs, end: span.EndTs}})
	if projection.OnChain {
		return semanticSpanLaneChainOverlap, projection.ChainDepth, projection.DominantState
	}
	if selfDeterministicSemanticSpanLane(chain, span) {
		return semanticSpanLaneChainSelf, 0, ""
	}
	// R3-IMPL (§29.88.1): the no-overlap fall-through's host-edge arm — the
	// span must hold a POSITIVE pre-edge share (a span at/after the boundary
	// is 边后=解除 and keeps the legacy lane; no fabricated overlap: depth/
	// state stay zero/empty, same discipline as the self lane).
	if anchor, ok := hostSemanticSpanEdgeAnchor(chain, span.Thread); ok && span.StartTs < anchor.boundaryTs {
		return semanticSpanLaneChainEdgeAnchored, 0, ""
	}
	return semanticSpanLaneOff, projection.ChainDepth, projection.DominantState
}

// FoldSemanticSpanFamilies folds the window-clipped semantic spans of ONE
// window_stats run into (physical artifact, thread, semantic class, chain
// lane) families. A stats run can contain several attached artifacts, so the
// physical source is an identity dimension even though every member shares
// one selected window (the M3 same-window discipline holds by construction;
// the tool observation face additionally stamps the family record with that
// run's selected_window note, unchanged). stats.TraceSpans is never modified.
// Families emit in FIRST-MEMBER APPEARANCE order (the span inventory is
// already duration-ranked by the stats bound, and appearance order keeps the
// degenerate all-singles shape byte-identical to the pre-RCM per-span
// iteration on both consumers); members inside a family sort largest first
// (Members[0] is the representative).
func FoldSemanticSpanFamilies(chain *ChainResult, spans []TraceSpanSummary) []SemanticSpanFamily {
	type laneKey struct {
		thread string
		class  string
		// SHADERCACHE-1: the refined subcategory joins the key so
		// shader_cache_hit and shader_cache_miss fold into SEPARATE
		// families with their own values (for every other class the
		// subcategory is the classifier's constant — no behavior change).
		subcategory string
		lane        semanticSpanChainLane
		source      string
	}
	type acc struct {
		family  SemanticSpanFamily
		members []TraceSpanSummary
	}
	groups := map[laneKey]*acc{}
	var order []laneKey
	for _, span := range spans {
		if !semanticSpanFamilyAdmits(span) {
			continue
		}
		lane, depth, state := semanticSpanFamilyLane(chain, span)
		key := laneKey{thread: threadKey(span.Thread), class: traceSpanSemanticClass(span.Name), subcategory: span.Subcategory, lane: lane, source: span.SourcePath}
		group, ok := groups[key]
		if !ok {
			basis := ""
			if lane == semanticSpanLaneChainSelf {
				basis = RootCauseOnChainBasisSelfDeterministicSpan
			}
			if lane == semanticSpanLaneChainEdgeAnchored {
				// R3-IMPL (§29.88.1): the host-edge lane's typed basis.
				basis = RootCauseOnChainBasisHostWakeupEdge
			}
			group = &acc{family: SemanticSpanFamily{
				Thread:        span.Thread,
				SemanticClass: key.class,
				SourcePath:    span.SourcePath,
				OnChain:       lane != semanticSpanLaneOff,
				OnChainBasis:  basis,
				ChainDepth:    depth,
				DominantState: state,
			}}
			groups[key] = group
			order = append(order, key)
		}
		if lane == semanticSpanLaneChainOverlap && (group.family.DominantState == "" || span.DurationMs > group.family.MaxMs) {
			group.family.DominantState = state
			group.family.ChainDepth = depth
		}
		if group.family.MaxMs == 0 || span.DurationMs > group.family.MaxMs {
			group.family.MaxMs = span.DurationMs
		}
		if group.family.MinMs == 0 || span.DurationMs < group.family.MinMs {
			group.family.MinMs = span.DurationMs
		}
		group.members = append(group.members, span)
	}
	out := make([]SemanticSpanFamily, 0, len(order))
	for _, key := range order {
		group := groups[key]
		fam := group.family
		sort.SliceStable(group.members, func(i, j int) bool {
			if group.members[i].DurationMs != group.members[j].DurationMs {
				return group.members[i].DurationMs > group.members[j].DurationMs
			}
			return group.members[i].StartLine < group.members[j].StartLine
		})
		fam.Members = group.members
		intervals := make([]foldInterval, 0, len(group.members))
		clipped := false
		for _, member := range group.members {
			firstMember := fam.SumMs <= 0
			fam.SumMs += member.DurationMs
			intervals = append(intervals, foldInterval{start: member.StartTs, end: member.EndTs})
			if firstMember || member.StartTs < fam.StartTs {
				fam.StartTs = member.StartTs
			}
			if member.EndTs > fam.EndTs {
				fam.EndTs = member.EndTs
			}
			if fam.StartLine == 0 || (member.StartLine > 0 && member.StartLine < fam.StartLine) {
				fam.StartLine = member.StartLine
			}
			if member.EndLine > fam.EndLine {
				fam.EndLine = member.EndLine
			}
			if member.ActualDurationMs > 0 {
				clipped = true
			}
		}
		unionMs, disjoint := foldIntervalUnionMs(intervals)
		// The window projection total: disjoint members sum exactly; overlap
		// deduplicates to the interval union (never above the member Σ — the
		// member value IS the clipped interval length, so union ≤ Σ up to
		// float noise, clamped for determinism).
		fam.TotalMs = unionMs
		if fam.TotalMs > fam.SumMs {
			fam.TotalMs = fam.SumMs
		}
		if disjoint {
			fam.FoldCaliber = RootCauseMemberFoldCaliberSumDisjoint
			fam.TotalMs = fam.SumMs
		} else {
			fam.FoldCaliber = RootCauseMemberFoldCaliberIntervalUnion
		}
		if fam.OnChain && fam.OnChainBasis == "" {
			// EVOLUTION RECORD (§24.10 → on-chain intersection caliber; 审计
			// #62 追认, §29.25 处置委托 + §29.26 待主会话落账, 2026-07-10).
			// §24.10 用户裁定原文: "合并键=(线程,语义类),参赛值=窗口投影合计
			// (同线程求和墙钟合法;非单次最大——用户明示'投影合计');链上 tier 道
			// 与非链背景综合排序道同规". EVOLUTION: for the ON-CHAIN lane the
			// participation/published value is the exact member∩chain
			// intersection union (有效归因诚实方向 — the same lane-admission
			// algebra prices the causal magnitude; consistent with the E17
			// 承自/gated 折算 philosophy), while TotalMs deliberately remains
			// the complete selected-window member union (§24.10 窗口投影合计
			// verbatim) on the Value/cumulative disclosure lanes. Off-chain
			// participation keeps the §24.10 original caliber byte-identically.
			// Display half: the ✦ row 行3/明细 disclose BOTH calibers
			// (链上计入 X + 窗口投影合计 Y, 审计 #62 ①).
			chainProjection := semanticTraceSpanChainIntersection(chain, fam.Thread, intervals)
			fam.ProjectedImpactMs = chainProjection.projection.ImpactMs
			if fam.ProjectedImpactMs > fam.TotalMs {
				fam.ProjectedImpactMs = fam.TotalMs
			}
			fam.ProjectedStartTs = chainProjection.projection.StartTs
			fam.ProjectedEndTs = chainProjection.projection.EndTs
			fam.DominantState = chainProjection.projection.DominantState
			fam.DominantStateImpactMs = chainProjection.dominantStateImpactMs
			if fam.DominantStateImpactMs > fam.ProjectedImpactMs {
				fam.DominantStateImpactMs = fam.ProjectedImpactMs
			}
			fam.ChainDepth = chainProjection.projection.ChainDepth
			fam.ChainBranch = chainProjection.projection.ChainBranch
		} else if fam.OnChain && fam.OnChainBasis == RootCauseOnChainBasisHostWakeupEdge {
			// R3-IMPL (§29.88.1): the host-edge lane's participation is the
			// PRE-EDGE member union (边前段窗内投影); the post-edge union is
			// stashed for the mint loop's ◇ remainder clone (跨边按边界二分).
			// TotalMs deliberately remains the complete member union (the
			// §24.10 disclosure caliber, same dual-caliber shape as the
			// overlap lane). No fabricated overlap: depth/state stay 0/"".
			if anchor, ok := hostSemanticSpanEdgeAnchor(chain, fam.Thread); ok {
				fam.EdgeAnchorBoundaryTs = anchor.boundaryTs
				fam.EdgeAnchorVia = anchor.via()
				var pre, post []foldInterval
				for _, interval := range intervals {
					preStart, preEnd, postStart, postEnd := semanticEdgeAnchorSplit(interval.start, interval.end, anchor.boundaryTs)
					if preEnd > preStart {
						pre = append(pre, foldInterval{start: preStart, end: preEnd})
					}
					if postEnd > postStart {
						post = append(post, foldInterval{start: postStart, end: postEnd})
					}
				}
				preMerged, _ := foldIntervalUnionWithDisjoint(pre)
				for _, interval := range preMerged {
					fam.ProjectedImpactMs += (interval.end - interval.start) * 1000
				}
				if len(preMerged) > 0 {
					fam.ProjectedStartTs = preMerged[0].start
					fam.ProjectedEndTs = preMerged[len(preMerged)-1].end
				}
				if fam.ProjectedImpactMs > fam.TotalMs {
					fam.ProjectedImpactMs = fam.TotalMs
				}
				postMerged, _ := foldIntervalUnionWithDisjoint(post)
				for _, interval := range postMerged {
					fam.EdgeAnchorRemainderMs += (interval.end - interval.start) * 1000
				}
				if len(postMerged) > 0 {
					fam.EdgeAnchorRemainderStartTs = postMerged[0].start
					fam.EdgeAnchorRemainderEndTs = postMerged[len(postMerged)-1].end
				}
			}
			// A structurally-unreachable anchor miss leaves
			// ProjectedImpactMs=0 and the family mint fails closed (the same
			// direction as the overlap lane's unreachable arm — the full
			// union must never leak onto the causal lane).
		}
		if clipped {
			// DCS E4 dual basis at family grain: physical member extents, with
			// each un-clipped member contributing its own (raw == clipped)
			// segment. Union-guarded the same way — physical same-thread
			// segments may nest.
			actualIntervals := make([]foldInterval, 0, len(group.members))
			actualSum := 0.0
			for _, member := range group.members {
				start, end, ms := member.StartTs, member.EndTs, member.DurationMs
				if member.ActualDurationMs > 0 {
					start, end, ms = member.ActualStartTs, member.ActualEndTs, member.ActualDurationMs
				}
				actualSum += ms
				if end > start {
					actualIntervals = append(actualIntervals, foldInterval{start: start, end: end})
				}
				if fam.ActualStartTs == 0 || start < fam.ActualStartTs {
					fam.ActualStartTs = start
				}
				if end > fam.ActualEndTs {
					fam.ActualEndTs = end
				}
			}
			actualUnion, actualDisjoint := foldIntervalUnionMs(actualIntervals)
			fam.ActualTotalMs = actualUnion
			if actualDisjoint || fam.ActualTotalMs > actualSum {
				fam.ActualTotalMs = actualSum
			}
		}
		out = append(out, fam)
	}
	return out
}

// rootCauseItemFromSemanticSpanFamily mints the ONE rank contender for a
// multi-member semantic family (§24.10). Off-chain participation remains the
// complete selected-window member union. On-chain participation is the exact
// member∩chain intersection union; the complete member union remains on the
// cumulative/actual disclosure lanes. Single-member families never come here:
// the caller routes them through rootCauseItemFromSemanticTraceSpan verbatim,
// so a family of one stays byte-identical to the pre-RCM single-span row
// (退化不变体). The effective-impact discount runs the SAME single formula
// over the FAMILY total (never Σ of per-member clamped values — §24.12
// mandate: 禁 Σ 各成员 clamp 后值).
func rootCauseItemFromSemanticSpanFamily(q Query, fam SemanticSpanFamily, hasCausalChain bool) (RootCauseRankItem, bool) {
	if len(fam.Members) == 0 || fam.TotalMs <= 0 {
		return RootCauseRankItem{}, false
	}
	rep := fam.Members[0]
	work, ok := traceSpanSemanticWorkClass(rep.Name)
	if !ok {
		return RootCauseRankItem{}, false
	}
	participationMs := fam.TotalMs
	projectionStartTs, projectionEndTs := fam.StartTs, fam.EndTs
	selfBasis := fam.OnChain && fam.OnChainBasis == RootCauseOnChainBasisSelfDeterministicSpan
	edgeBasis := fam.OnChain && fam.OnChainBasis == RootCauseOnChainBasisHostWakeupEdge
	if fam.OnChain && !selfBasis {
		participationMs = fam.ProjectedImpactMs
		projectionStartTs, projectionEndTs = fam.ProjectedStartTs, fam.ProjectedEndTs
		if participationMs <= 0 || projectionEndTs <= projectionStartTs {
			// FoldSemanticSpanFamilies uses the same exact projection primitive
			// for lane admission and magnitude, so this is structurally
			// unreachable for a genuine family. Fail closed rather than letting
			// the full family union leak back into the causal lane.
			return RootCauseRankItem{}, false
		}
	}
	// SELF-SEM (§29.61.1 ②, 2026-07-13): a self-basis family participates and
	// publishes by the complete window-projection member union — the target's
	// own running-segment deterministic work is DEFINED inside the target's
	// window, so the union IS the proven eliminable magnitude (已证可消除量,
	// not a conditional upper bound; the caliber difference from the overlap
	// lane is the PROOF PATH, not the ruler). R13 携值合法性: the value is the
	// target thread's own in-window wall clock — never another thread's
	// borrowed account (the wc_srvinit 54.608 protected shape).
	projection := semanticTraceSpanProjection{
		StartTs:       projectionStartTs,
		EndTs:         projectionEndTs,
		ImpactMs:      participationMs,
		DominantState: fam.DominantState,
		ChainDepth:    fam.ChainDepth,
		ChainBranch:   fam.ChainBranch,
		OnChain:       fam.OnChain,
	}
	// EVOLUTION RECORD (§29.7-2 ② → intersection caliber; 审计 #62 追认,
	// §29.25 处置委托 + §29.26 待主会话落账, 2026-07-10). §29.7-2 ② 原文:
	// "参赛值=家族真实合计(如 102.172),score 乘子(2.10)留引擎排序内部" —
	// same as the single-span mint, the deterministic hidden-cost boost never
	// publishes as EffectiveImpactMs (792-textup witness: the family's
	// 有效归因 read 214.561ms = 102.172 × 2.10); the boost stays
	// engine-internal on RankSortBoostedEffectiveMs and the
	// semantic_multiplier=/hidden_cost_boost= internal tokens left the Summary
	// (红线: internal tokens must never reach answer prose). EVOLUTION on top:
	// for ON-CHAIN families "家族真实合计" evolved to the exact member∩chain
	// intersection union (see the FoldSemanticSpanFamilies OnChain record
	// above); full-overlap families — the §29.22 textup 102.172 witness —
	// are byte-identical because there intersection == union.
	// B829: a host-owned wakeup edge proves that this thread is related to the
	// target and gives the pre-edge span a precise raw-occupancy boundary.  It
	// does NOT prove that the target waited for this semantic operation to
	// finish (or that finishing it triggered the wakeup).  Keep the raw
	// projection and business identity, but publish no eliminable attribution
	// until an exact chain-window intersection or target-self basis exists.
	publishedEffectiveMs := participationMs
	if edgeBasis {
		publishedEffectiveMs = 0
	}
	sortBoostedMs := 0.0
	if publishedEffectiveMs > 0 {
		sortBoostedMs = semanticTraceSpanEffectiveImpactMs(work, projection, TraceSpanSummary{DurationMs: fam.TotalMs})
	}
	// SHADERCACHE-1: the family label speaks the proven cache outcome (the
	// key split guarantees a uniform outcome across members).
	familyLabel := semanticSpanLabelWithCacheOutcome(work.Label, rep.Subcategory)
	var summary string
	if selfBasis {
		summary = fmt.Sprintf("%s family n=%d span(s) on the analysis target's own thread totalled %.3fms window projection (deterministic self work counted on-chain without any wakeup-edge claim; largest %q %.3fms, smallest %.3fms); effective_impact=%.3fms; fold_caliber=%s",
			familyLabel, len(fam.Members), fam.TotalMs, rep.Name, fam.MaxMs, fam.MinMs, fam.TotalMs, fam.FoldCaliber)
	} else if edgeBasis {
		// R3-IMPL: the R4-family edge=credential wording — the participation
		// is the pre-edge share, the complete union stays disclosed.
		summary = fmt.Sprintf("%s family n=%d same-thread span(s) carried %.3fms raw pre-edge occupancy before the host's own in-window wakeup edge toward the analysis target at %.6f (edge=relation credential; semantic completion/delay binding=unproven; via=%s) from %.3fms complete selected-window span union (largest %q %.3fms, smallest %.3fms); effective_impact=0.000ms; fold_caliber=%s",
			familyLabel, len(fam.Members), participationMs, fam.EdgeAnchorBoundaryTs, fam.EdgeAnchorVia, fam.TotalMs, rep.Name, fam.MaxMs, fam.MinMs, fam.FoldCaliber)
	} else if fam.OnChain {
		summary = fmt.Sprintf("%s family n=%d same-thread span(s) attributed %.3fms by exact on-chain interval intersection from %.3fms complete selected-window span union (largest %q %.3fms, smallest %.3fms); effective_impact=%.3fms; fold_caliber=%s",
			familyLabel, len(fam.Members), participationMs, fam.TotalMs, rep.Name, fam.MaxMs, fam.MinMs, participationMs, fam.FoldCaliber)
	} else {
		summary = fmt.Sprintf("%s family n=%d same-thread span(s) totalled %.3fms window projection (largest %q %.3fms, smallest %.3fms); effective_impact=%.3fms; fold_caliber=%s",
			familyLabel, len(fam.Members), fam.TotalMs, rep.Name, fam.MaxMs, fam.MinMs, fam.TotalMs, fam.FoldCaliber)
	}
	if fam.TotalMs < fam.SumMs {
		summary = fmt.Sprintf("%s; interval_union=%.3fms < member_sum=%.3fms (overlapping same-thread segments deduplicated)", summary, fam.TotalMs, fam.SumMs)
	}
	if fam.DominantState != "" {
		summary = fmt.Sprintf("%s; overlapped_chain_state=%s", summary, fam.DominantState)
	}
	item := rootCauseItem(work.RootCauseType, fam.Thread, participationMs, work.Confidence, fam.StartLine, fam.EndLine, "window_stats.trace_spans.semantic", summary)
	item.PhysicalSourcePath = fam.SourcePath
	item.StartTs = projectionStartTs
	item.EndTs = projectionEndTs
	if fam.ActualTotalMs > 0 {
		item.ActualStartTs = fam.ActualStartTs
		item.ActualEndTs = fam.ActualEndTs
		item.ActualImpactMs = fam.ActualTotalMs
		item.ActualTotalMs = fam.ActualTotalMs
	} else {
		item.ActualStartTs = fam.StartTs
		item.ActualEndTs = fam.EndTs
		item.ActualImpactMs = fam.TotalMs
		item.ActualTotalMs = fam.TotalMs
	}
	item.ProjectedImpactMs = participationMs
	// The complete selected-window family union is a disclosure caliber, not
	// an on-chain ranking caliber. Keeping it here preserves the full family
	// total on the existing projected-total/cumulative wire face.
	item.CumulativeImpactMs = fam.TotalMs
	// SEM-LEAD (§29.7-2 ②): published effective = the real causal projection;
	// the boost rides the internal sort channel only.
	item.EffectiveImpactMs = publishedEffectiveMs
	if publishedEffectiveMs > 0 && sortBoostedMs > publishedEffectiveMs {
		item.RankSortBoostedEffectiveMs = sortBoostedMs
	}
	item.SpanName = rep.Name
	item.SpanKind = rep.Kind
	item.SpanCategory = firstNonEmpty(rep.Category, work.Category)
	item.SpanSubcategory = firstNonEmpty(rep.Subcategory, work.Subcategory)
	item.SemanticClass = work.SemanticClass
	item.DominantState = fam.DominantState
	item.ChainDepth = fam.ChainDepth
	item.ChainBranch = fam.ChainBranch
	if selfBasis {
		// SELF-SEM: on-chain channel identity WITHOUT a cross-thread claim —
		// the honest causality token, the typed basis marker (the enrich keep
		// arm and every display qualifier read THIS one field), and NO
		// OverlapMs (不伪造重叠).
		item.Causality = RootCauseCausalitySelfDeterministic
		item.ChainRelevance = "on_chain"
		item.OnChainBasis = RootCauseOnChainBasisSelfDeterministicSpan
	} else if edgeBasis {
		// R3-IMPL (§29.88.1): a REAL typed wakeup edge toward the target
		// exists, so "on_wakeup_chain" is honest; the typed basis + edge
		// disclosure pair ride the row. NO OverlapMs — the pre-edge share is
		// an edge relation, never a chain-window overlap claim (不伪造重叠).
		item.Causality = "on_wakeup_chain"
		item.ChainRelevance = "on_chain"
		item.OnChainBasis = RootCauseOnChainBasisHostWakeupEdge
		item.HostWakeupEdgeAnchorTs = fam.EdgeAnchorBoundaryTs
		item.HostWakeupEdgeAnchorVia = fam.EdgeAnchorVia
		if fam.EdgeAnchorRemainderMs > 0 {
			// 跨边按边界二分: the bipartition trio + the ◇ clone stash (the
			// mint loop emits the remainder seat right after this row).
			item.ChainAnchoredMs = participationMs
			item.ChainAnchorFullMs = participationMs + fam.EdgeAnchorRemainderMs
			item.hostEdgeRemainderStartTs = fam.EdgeAnchorRemainderStartTs
			item.hostEdgeRemainderEndTs = fam.EdgeAnchorRemainderEndTs
		}
	} else if fam.OnChain {
		item.Causality = "on_wakeup_chain"
		item.ChainRelevance = "on_chain"
		item.OverlapMs = participationMs
	}
	stateImpactMs := participationMs
	if fam.OnChain {
		stateImpactMs = fam.DominantStateImpactMs
	}
	applySemanticTraceSpanState(&item, fam.DominantState, stateImpactMs)
	item.MemberCount = len(fam.Members)
	item.MemberRoster = fam.MemberRosterEntries()
	// XLANE-2 件1: the complete typed member line-range set rides beside the
	// bounded roster (all-or-nothing; the display subset judgment consumes it).
	item.MemberLineRanges = fam.MemberLineRangeEntries()
	// SPANTOP-1 件1 (§29.131): the complete typed per-member wall-clock list
	// rides beside it (same order, same all-or-nothing discipline; the display
	// constituent top-3 sub-rows consume it under the µs identity gate).
	item.MemberWallMs = fam.MemberWallMsEntries()
	// XLANE-2 件2: the complete member span intervals (engine-internal,
	// all-or-nothing — the self-gap overlap disclosure claims an EXACT X, so
	// a partial inventory mints nothing rather than an understated overlap).
	item.semanticMemberIntervals = fam.memberIntervalInventory(queryWindowStartsAtDeterminedZero(q))
	item.MemberMaxMs = fam.MaxMs
	item.MemberMinMs = fam.MinMs
	if fam.TotalMs < fam.SumMs {
		item.MemberSumMs = fam.SumMs
	}
	item.MemberFoldCaliber = fam.FoldCaliber
	// SEM-LEAD P1-1 (§29.22 修向(a)): the Score consumes the boosted basis as
	// a same-effective tie-break only — the on-chain ordinal key is the
	// published intersection projection, so the ordinal can never contradict the
	// eff-ordered board/badges.
	item.Score = rootCauseRankScoreBasisMs(item) * item.Confidence * rootCauseItemScoreWeight(item)
	return item, true
}

// rootCauseFamilyRosterCap bounds every typed member roster (§24.7.1 ①
// 可折叠显示: the roster is bounded; overflow is expressed by MemberCount and
// the full inventory stays available on the source view — never a silent
// drop, the count discloses).
const rootCauseFamilyRosterCap = 8

// MemberRosterEntries renders the bounded typed member roster ("<span name>
// <clipped ms>ms", largest first — the count discloses overflow). ONE renderer
// for BOTH consumers (rank item MemberRoster and the observation-channel
// member_roster note), so the two faces can never disagree on the roster
// shape.
// EVOLUTION RECORD (SPANTOP-1 件2, §29.131, 2026-07-18): the SEMANTIC roster
// bound rose from rootCauseFamilyRosterCap to the line-range cap — the detail
// face is the tree top-3's 「全清单见明细」 counterpart, so it must hold the
// complete inventory for every family the line-range carrier can cover (≤32
// members; larger families keep the counted-excerpt honesty, and their
// constituent block never renders because MemberLineRangeEntries is nil there
// anyway). The generic same-thread-type fold keeps rootCauseFamilyRosterCap.
func (fam SemanticSpanFamily) MemberRosterEntries() []string {
	out := make([]string, 0, minIntFold(len(fam.Members), rootCauseFamilyMemberLineRangeCap))
	for i, member := range fam.Members {
		if i >= rootCauseFamilyMemberLineRangeCap {
			break
		}
		out = append(out, fmt.Sprintf("%s %.3fms", member.Name, member.DurationMs))
	}
	return out
}

// MemberWallMsEntries renders the COMPLETE typed per-member in-window
// wall-clock list ("%.3f", member order — same order as Members and
// MemberLineRangeEntries). All-or-nothing (SPANTOP-1 件1, §29.131, same
// discipline as the line-range carrier): any member without a positive
// duration, or a family beyond rootCauseFamilyMemberLineRangeCap, yields nil
// so the display-side constituent decomposition can never run on a partial
// list (a partial Σ would fake the seat identity).
func (fam SemanticSpanFamily) MemberWallMsEntries() []string {
	if len(fam.Members) == 0 || len(fam.Members) > rootCauseFamilyMemberLineRangeCap {
		return nil
	}
	out := make([]string, 0, len(fam.Members))
	for _, member := range fam.Members {
		if member.DurationMs <= 0 {
			return nil
		}
		out = append(out, fmt.Sprintf("%.3f", member.DurationMs))
	}
	return out
}

// rootCauseFamilyMemberLineRangeCap bounds the typed member line-range carrier
// (XLANE-2 件1): a family beyond the cap mints NO line-range note at all —
// never a truncated set (the subset judgment fails open on absence).
const rootCauseFamilyMemberLineRangeCap = 32

// MemberLineRangeEntries renders the COMPLETE typed member line-range set
// ("start..end", member order — same order as Members). All-or-nothing
// (XLANE-2 件1 fail-open discipline): any member without a valid line range,
// a duplicated range (修补轮 件7, 2026-07-17: the display judgment is a SET —
// two members on one physical range would silently collapse, and a
// multiplicity-blind subset verdict is a false pointer risk), or a family
// beyond rootCauseFamilyMemberLineRangeCap, yields nil so the display-side
// 成员子集 judgment can never run on a partial or collapsed set.
func (fam SemanticSpanFamily) MemberLineRangeEntries() []string {
	if len(fam.Members) == 0 || len(fam.Members) > rootCauseFamilyMemberLineRangeCap {
		return nil
	}
	out := make([]string, 0, len(fam.Members))
	seen := make(map[string]bool, len(fam.Members))
	for _, member := range fam.Members {
		if member.StartLine <= 0 || member.EndLine < member.StartLine {
			return nil
		}
		entry := fmt.Sprintf("%d..%d", member.StartLine, member.EndLine)
		if seen[entry] {
			return nil
		}
		seen[entry] = true
		out = append(out, entry)
	}
	return out
}

func minIntFold(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// memberIntervalInventory renders the COMPLETE member span interval set
// (XLANE-2 件2, engine-internal). All-or-nothing: any member without a valid
// [StartTs, EndTs) yields nil — the overlap disclosure claims an exact X and
// must never understate it from a partial inventory (fail-open).
// WINFLAG-1 (c) (§29.190④): validity reads the flag-aware gate — in a
// flagged [0,end] run (zeroStartReal) a span genuinely starting at ts==0 no
// longer voids the whole inventory; without the flag the gate is
// byte-equivalent to the legacy `StartTs <= 0` reject.
func (fam SemanticSpanFamily) memberIntervalInventory(zeroStartReal bool) []foldInterval {
	if len(fam.Members) == 0 {
		return nil
	}
	out := make([]foldInterval, 0, len(fam.Members))
	for _, member := range fam.Members {
		if !rankFoldStartUsable(member.StartTs, member.EndTs, zeroStartReal) {
			return nil
		}
		out = append(out, foldInterval{start: member.StartTs, end: member.EndTs})
	}
	return out
}

// markSemanticSpanConsumed records that the family fold consumed the given
// member span, so the generic trace_span fallback loop never double-mints it.
// Members are verbatim copies of stats.TraceSpans entries — the first
// unconsumed exact-identity match is the one (duplicates consume one seat
// each, exactly like the source inventory).
func markSemanticSpanConsumed(consumed map[int]bool, spans []TraceSpanSummary, member TraceSpanSummary) {
	for i := range spans {
		if consumed[i] {
			continue
		}
		if spans[i].Name == member.Name &&
			spans[i].StartLine == member.StartLine && spans[i].EndLine == member.EndLine &&
			spans[i].StartTs == member.StartTs && spans[i].EndTs == member.EndTs &&
			sameThreadRef(spans[i].Thread, member.Thread) {
			consumed[i] = true
			return
		}
	}
}

// --- 2. generic same-(thread,type) rank families (§24.7.1) --------------------

// rootCauseFamilyFoldLaneKey is the 道别 half of the merge key: the on-chain
// board and the background composite board never cross-merge, and a typed
// adjacent row keeps its own lane.
//
// SELF-ALL (§29.61.2, 2026-07-13): the on-chain lane additionally keys on the
// typed proof basis — an overlap-proven row and a self-basis row never fold
// into one family (the SELF-SEM 两把尺禁混折 enum precedent: mixing proof
// paths inside one seat would let the family speak an overlap claim its
// self-basis members never earned, or under-claim a proven overlap).
func rootCauseFamilyFoldLaneKey(item RootCauseRankItem) string {
	if rootCauseItemIsOnChain(item) {
		if basis := strings.TrimSpace(item.OnChainBasis); basis != "" {
			return "on_chain|" + basis
		}
		return "on_chain"
	}
	if relevance := strings.TrimSpace(item.ChainRelevance); relevance != "" {
		return relevance
	}
	return "background"
}

// rootCauseFamilyFoldWindowKey is the M3 cross-window discipline half of the
// merge key: only rows carrying the SAME typed selected-window identity may
// merge (RN-12 伪包含 same-type defense — cross-window re-measurements of one
// physical segment stay separate lines and never double-sum).
func rootCauseFamilyFoldWindowKey(item RootCauseRankItem) string {
	return fmt.Sprintf("%.6f..%.6f", item.StatsWindowStartTs, item.StatsWindowEndTs)
}

// rootCauseFamilyFoldAnchorFormKey (RNB-1, §29.88 R2/R4, 2026-07-14): the
// re-anchoring account-identity half of the merge key. Precise typed signals
// only; "" on every plain row keeps all pre-RNB merges byte-identical.
func rootCauseFamilyFoldAnchorFormKey(item RootCauseRankItem) string {
	switch {
	case item.ChainAnchorRemainderSeat:
		return "anchor_remainder"
	case item.ChainAnchorFullMs > 0:
		return "anchor_clipped"
	case item.ChainCredentialLaneDemoted:
		return "lane_demoted"
	case item.ChainAnchorRepresentedByChainSeat:
		// XLANE-1 件1: the represented-demoted satellite is its own account
		// form — folding it with a plain (or R4-demoted) row of the same
		// (thread, type, lane) would let one family speak two credential
		// stories.
		return "anchor_represented"
	default:
		return ""
	}
}

// foldSameThreadTypeRankFamilies merges same-(thread,type) rank rows of the
// registry-declared per-instance families (CausalTokenFamilyFoldLane ==
// CausalFamilyFoldSameThreadType) into ONE contender per
// (type, thread, chain lane, selected window) BEFORE the rank sort, so the
// family competes with its combined magnitude (§24.7.1: 拆分参赛=弱化排序;
// 合并量参赛). Groups of one pass through untouched (a family of one IS the
// row — byte-stable), so the pass is idempotent. q/hasCausalChain re-apply the
// §7.5 background window cap ONCE over the family value (single cap on the
// family, mirroring the mint sites — capped members never re-sum above it).
func foldSameThreadTypeRankFamilies(q Query, hasCausalChain bool, items []RootCauseRankItem) []RootCauseRankItem {
	if len(items) < 2 {
		return items
	}
	type familyKey struct {
		typ            string
		thread         string
		lane           string
		window         string
		source         string
		physicalSource string
		cause          string
		// anchorForm (RNB-1, §29.88 R2/R4, 2026-07-14): the re-anchoring
		// account-identity dimension — a ◇ remainder seat, a ⛓ clipped seat
		// and an R4 lane-demoted seat are DIFFERENT accounts from a plain
		// window seat of the same (thread, type, lane) and must never re-Σ
		// with one (§29.50.5 绝不灌根因席 precedent). Witnessed by INV-D S5:
		// the enrich-pass re-fold merged the migrated remainder seat (70,
		// carrying the 97=27+70 decomposition) with the plain adjacent dust
		// seat (0.5) into a 70.5 row whose 行2 still spoke the 97 account.
		anchorForm string
	}
	groups := map[familyKey][]int{}
	var order []familyKey
	for i := range items {
		if CausalTokenFamilyFoldLane(items[i].Type) != CausalFamilyFoldSameThreadType {
			continue
		}
		key := familyKey{
			typ:    items[i].Type,
			thread: threadKey(items[i].Thread),
			lane:   rootCauseFamilyFoldLaneKey(items[i]),
			window: rootCauseFamilyFoldWindowKey(items[i]),
			// ORD (2026-07-10): the mint-source identity — the wakeup-chain
			// lane and the window-stats lane measure the SAME physical time
			// through different rulers (e.g. a RootEvidence sleep_wait row vs
			// the SleepTop bucket of one thread); same-(thread,type) rows
			// from different producers must never Σ into one family (双计
			// 防线). Every pre-ORD family type is minted from exactly ONE
			// source string, so no existing merge splits.
			source:         items[i].Source,
			physicalSource: items[i].PhysicalSourcePath,
			// §29.50.5 (v5 P1 批 件②, 2026-07-13): the root-cause-identity
			// dimension of the participation key — a proven cause seat and
			// its honest-remainder sibling are DIFFERENT accounts and must
			// never re-merge here (聚合键=(线程,状态族,根因身份); 绝不灌根
			// 因席). Rows without a proven wait object keep "" (no split of
			// any pre-§29.50.5 merge).
			cause:      strings.TrimSpace(items[i].BlockedReasonCaller),
			anchorForm: rootCauseFamilyFoldAnchorFormKey(items[i]),
		}
		if len(groups[key]) == 0 {
			order = append(order, key)
		}
		groups[key] = append(groups[key], i)
	}
	merged := map[int]RootCauseRankItem{}
	drop := map[int]bool{}
	for _, key := range order {
		idxs := groups[key]
		if len(idxs) < 2 {
			continue
		}
		merged[idxs[0]] = mergeSameThreadTypeRankFamily(q, hasCausalChain, items, idxs)
		for _, i := range idxs[1:] {
			drop[i] = true
		}
	}
	if len(merged) == 0 {
		return items
	}
	out := make([]RootCauseRankItem, 0, len(items))
	for i := range items {
		if drop[i] {
			continue
		}
		if m, ok := merged[i]; ok {
			out = append(out, m)
			continue
		}
		out = append(out, items[i])
	}
	return out
}

func mergeSameThreadTypeRankFamily(q Query, hasCausalChain bool, items []RootCauseRankItem, idxs []int) RootCauseRankItem {
	// WINFLAG-1 (c) (§29.190④): one flag read for every start>0 guard in this
	// fold — in a flagged [0,end] run a member StartTs==0 is a real timestamp
	// (interval collection, envelope fold, roster identity keys); without the
	// flag every guard below is byte-identical to its legacy form.
	zeroStartReal := queryWindowStartsAtDeterminedZero(q)
	members := make([]RootCauseRankItem, 0, len(idxs))
	for _, i := range idxs {
		members = append(members, items[i])
	}
	// Largest raw member first — the family representative is the biggest
	// member, never "whichever the source iterated first" (the audit's D-3
	// order-dependence complaint applied per-CLASS here).
	sort.SliceStable(members, func(i, j int) bool {
		vi, vj := rootCauseCumulativeImpactMs(members[i]), rootCauseCumulativeImpactMs(members[j])
		if vi != vj {
			return vi > vj
		}
		return members[i].LineStart < members[j].LineStart
	})
	base := members[0]
	runnableCPU, runnableCPUKnown := base.runnableCPU, base.runnableCPUKnown
	var runnableIntervals []foldInterval
	for _, member := range members {
		runnableIntervals = append(runnableIntervals, member.runnableIntervals...)
	}
	for _, member := range members[1:] {
		if !runnableCPUKnown || !member.runnableCPUKnown || member.runnableCPU != runnableCPU {
			runnableCPUKnown = false
			runnableCPU = -1
		}
	}
	spec, _ := CausalTokenSpecFor(base.Type)
	// G3 (§27.2, 2026-07-09): a Count-additivity family's member values are
	// count-derived advisory scalars (registry: "not a physical duration even
	// when printed with an ms suffix") — every face that prints one goes
	// through rootCauseCountEquivalentValue instead of a bare wall-clock "ms".
	// Arm-level on the registry Additivity column, never a per-type carve-out.
	countFamily := spec.Additivity == CausalAdditivityCount
	intervals := make([]foldInterval, 0, len(members))
	intervalsUsable := true
	// reconIntervals (G1, §27.2): the reconciliation-lane member inventory.
	// Separate from `intervals` (which also drives the caliber ladder, kept
	// byte-identical): a re-folded member that already carries its own member
	// inventory contributes THOSE intervals, never its hull — so the 二次 fold
	// hull 退化 corner (§24.22 留账, today unreachable) can never widen the
	// G1 membership union.
	reconIntervals := make([]foldInterval, 0, len(members))
	sum := 0.0
	effectiveSum := 0.0
	effectiveMax := 0.0
	effectiveMatchesRaw := true
	strongestEffective := RootCauseRankItem{}
	gatedRunnableSum, gatedDeficitSum := 0.0, 0.0
	priorityProvenSum, priorityUnknownSum := 0.0, 0.0
	priorityEffectiveMembersAuthorized := true
	priorityAuthorizedContributorSeen := false
	allEffectivePureRunnable, allEffectivePureDeficit := true, true
	memberCount := 0
	roster := make([]string, 0, minIntFold(len(members), rootCauseFamilyRosterCap))
	maxMs, minMs := 0.0, 0.0
	sameInode, sameDev := true, true
	// ORD (§29.11 补充 观察①, 2026-07-10): the producer-disjointness proof —
	// true only when EVERY member carries the mint-site guarantee (a single
	// off-CPU state machine keeps at most one open segment per PID, so the
	// per-CPU bucket rows of one thread partition its own timeline). AND over
	// members: a merged row re-entering the enrich-pass fold keeps the proof
	// only if all its constituents had it.
	producerDisjoint := true
	for _, member := range members {
		if !member.memberSegmentsProducerDisjoint {
			producerDisjoint = false
		}
	}
	for _, member := range members {
		raw := rootCauseCumulativeImpactMs(member)
		effective := rootCauseEffectiveImpactMs(member)
		sum += raw
		effectiveSum += effective
		if effectiveMax == 0 || effective > effectiveMax {
			effectiveMax = effective
			strongestEffective = member
		}
		tolerance := raw * 0.001
		if tolerance < 0.001 {
			tolerance = 0.001
		}
		if effective < raw-tolerance || effective > raw+tolerance {
			effectiveMatchesRaw = false
		}
		gatedRunnableSum += member.GatedRunnableMs
		gatedDeficitSum += member.GatedRunningDeficitMs
		if rootCauseTypeIsPriorityInversion(member.Type) {
			switch {
			case effective > 0 && priorityFoldEffectiveMemberAuthorityClosed(member, effective):
				priorityProvenSum += member.PriorityRelationProvenLowerMs
				priorityUnknownSum += member.PriorityRelationUnknownOrNonLowerMs
				priorityAuthorizedContributorSeen = true
			case effective > 0 || effective < 0 || math.IsNaN(effective) || math.IsInf(effective, 0):
				// Every positive-effective contributor independently owns its hard
				// caliber, finite covering account, and physical source token. Move
				// rejected coverage to the unknown side and fail the whole family;
				// interval union and a healthy sibling's source may not launder it.
				priorityUnknownSum += member.PriorityRelationProvenLowerMs + member.PriorityRelationUnknownOrNonLowerMs
				priorityEffectiveMembersAuthorized = false
			case priorityEvidenceCaliberIsHard(member.PriorityRelationCaliber):
				// A zero-effective context member need not carry its own source
				// proof, but any finite hard coverage remains an auditable part of
				// the family account. It cannot make the contributor-seen guard true.
				priorityProvenSum += member.PriorityRelationProvenLowerMs
				priorityUnknownSum += member.PriorityRelationUnknownOrNonLowerMs
			default:
				priorityUnknownSum += member.PriorityRelationProvenLowerMs + member.PriorityRelationUnknownOrNonLowerMs
			}
		}
		if math.Abs(member.GatedRunnableMs-effective) > tolerance || math.Abs(member.GatedRunningDeficitMs) > tolerance {
			allEffectivePureRunnable = false
		}
		if math.Abs(member.GatedRunningDeficitMs-effective) > tolerance || math.Abs(member.GatedRunnableMs) > tolerance {
			allEffectivePureDeficit = false
		}
		count := member.MemberCount
		if count <= 0 {
			count = 1
		}
		memberCount += count
		if maxMs == 0 || raw > maxMs {
			maxMs = raw
		}
		if member.MemberMaxMs > maxMs {
			maxMs = member.MemberMaxMs
		}
		if minMs == 0 || raw < minMs {
			minMs = raw
		}
		if member.MemberMinMs > 0 && member.MemberMinMs < minMs {
			minMs = member.MemberMinMs
		}
		if rankFoldStartUsable(member.StartTs, member.EndTs, zeroStartReal) {
			intervals = append(intervals, foldInterval{start: member.StartTs, end: member.EndTs})
		} else {
			intervalsUsable = false
		}
		if len(member.familyMemberIntervals) > 0 {
			reconIntervals = append(reconIntervals, member.familyMemberIntervals...)
		} else if rankFoldStartUsable(member.StartTs, member.EndTs, zeroStartReal) {
			reconIntervals = append(reconIntervals, foldInterval{start: member.StartTs, end: member.EndTs})
		}
		if member.Inode != base.Inode {
			sameInode = false
		}
		if member.Dev != base.Dev {
			sameDev = false
		}
		if len(roster) < rootCauseFamilyRosterCap {
			if len(member.MemberRoster) > 0 {
				for _, entry := range member.MemberRoster {
					if len(roster) >= rootCauseFamilyRosterCap {
						break
					}
					roster = append(roster, entry)
				}
			} else if countFamily {
				// G3: honest count-equivalent labelling — the roster face and
				// the summary face share one renderer (两面同源).
				roster = append(roster, fmt.Sprintf("%s %s", rootCauseFamilyMemberRosterKey(member, zeroStartReal), rootCauseCountEquivalentValue(raw)))
			} else {
				roster = append(roster, fmt.Sprintf("%s %.3fms", rootCauseFamilyMemberRosterKey(member, zeroStartReal), raw))
			}
		}
	}
	// Caliber ladder (§24.7.1 终裁 + §24.9 dim-B 复核勘误 — the same-window
	// merge caliber is a NEW lane, never a mechanical reuse of the display's
	// cross-window CWD ladder):
	//   count additivity            → Σ (counts add regardless of overlap);
	//   provably disjoint segments  → Σ (same-thread wall clock, legal —
	//                                 composite/advisory values sum too, the
	//                                 caliber word discloses the ruler);
	//   overlap ∧ value==interval   → interval union (the member value IS its
	//                                 occupied wall-clock length — io_latency
	//                                 shape — so the union deduplicates
	//                                 exactly; < Σ, disclosed);
	//   otherwise                   → member MAX (honest lower bound: overlap
	//                                 or missing interval identity on a value
	//                                 that is NOT an interval length — naive Σ
	//                                 would double-count unprovably).
	caliber := RootCauseMemberFoldCaliberMaxOverlapFallback
	combined := maxMs
	disjoint := false
	unionMs := 0.0
	if intervalsUsable && len(intervals) == len(members) {
		unionMs, disjoint = foldIntervalUnionMs(intervals)
	}
	valuesAreIntervalLengths := intervalsUsable && len(intervals) == len(members)
	if valuesAreIntervalLengths {
		for _, member := range members {
			value := rootCauseCumulativeImpactMs(member)
			length := (member.EndTs - member.StartTs) * 1000
			tolerance := value * 0.001
			if tolerance < 0.001 {
				tolerance = 0.001
			}
			if value < length-tolerance || value > length+tolerance {
				valuesAreIntervalLengths = false
				break
			}
		}
	}
	switch {
	case spec.Additivity == CausalAdditivityCount:
		caliber = RootCauseMemberFoldCaliberCountSum
		combined = sum
	case disjoint:
		caliber = RootCauseMemberFoldCaliberSumDisjoint
		combined = sum
	case producerDisjoint:
		// ORD: envelope overlap with a producer-level disjointness proof —
		// the per-CPU off-CPU buckets of one thread interleave their line/ts
		// ENVELOPES while their underlying segments are pairwise disjoint by
		// construction, so the same-thread Σ is legal (§24.7.1 合并量参赛;
		// same caliber word as the envelope-proven arm above).
		caliber = RootCauseMemberFoldCaliberSumDisjoint
		combined = sum
	case valuesAreIntervalLengths:
		caliber = RootCauseMemberFoldCaliberIntervalUnion
		combined = unionMs
		if combined > sum {
			combined = sum
		}
	}
	merged := base
	merged.runnableCPU = runnableCPU
	merged.runnableCPUKnown = runnableCPUKnown
	merged.runnableIntervals = runnableIntervals
	// ORD: the merged row keeps the producer-disjointness proof only when
	// every member had it (idempotent re-fold in the enrich pass).
	merged.memberSegmentsProducerDisjoint = producerDisjoint
	// RNB-1 T1 (§29.88 R2, 2026-07-14): the census-group ledger anchored
	// stamps Σ across members ONLY under the exact Σ caliber (sum_disjoint —
	// the published family value is the member Σ, so the anchored Σ is the
	// same-segment-set share). Any other caliber (MAX fallback / interval
	// union) publishes a value that is NOT the group Σ — the stamp clears and
	// the re-anchoring fails open for the seat (宁漏勿猜; base-member stamp
	// inheritance would misstate the merged account).
	ledgerStamped := caliber == RootCauseMemberFoldCaliberSumDisjoint
	ledgerRunnable, ledgerD, ledgerIO := 0.0, 0.0, 0.0
	for _, member := range members {
		if !member.ledgerAnchorStamped {
			ledgerStamped = false
			break
		}
		ledgerRunnable += member.ledgerAnchoredRunnableMs
		ledgerD += member.ledgerAnchoredDMs
		ledgerIO += member.ledgerAnchoredIOMs
	}
	merged.ledgerAnchorStamped = ledgerStamped
	merged.ledgerAnchoredRunnableMs, merged.ledgerAnchoredDMs, merged.ledgerAnchoredIOMs = 0, 0, 0
	if ledgerStamped {
		merged.ledgerAnchoredRunnableMs, merged.ledgerAnchoredDMs, merged.ledgerAnchoredIOMs = ledgerRunnable, ledgerD, ledgerIO
	}
	merged.CumulativeImpactMs = combined
	// F3 (对抗复核收尾, 2026-07-08): the per-state scalar channels
	// (running/runnable/sleep/d_state/io_wait) and the categorical dominant
	// state must stay SAME-SOURCE with the published family value — inheriting
	// the base member's slice under a Σ total published "cumulative=2.0 while
	// runnable=1.2" self-contradictions (the audit's D-3 fold-inheritance
	// class). Ruler-aligned re-derivation, honest and minimal:
	//   sum_disjoint / count_sum → Σ each channel over members (disjoint
	//     same-thread segments: per-state wall clock adds under the same
	//     ruling as the published value);
	//   interval_union → channels ZERO (a per-channel union is not derivable
	//     from scalars; absence over a fabricated number);
	//   max_overlap_fallback → keep the base member's channels (the published
	//     value IS that member's own measurement — consistent by
	//     construction).
	// DominantState survives only when every member agrees (a categorical
	// word must not claim a state the family does not share).
	switch caliber {
	case RootCauseMemberFoldCaliberSumDisjoint, RootCauseMemberFoldCaliberCountSum:
		merged.RunningMs, merged.RunnableMs, merged.SleepMs, merged.DStateMs, merged.IOWaitMs = 0, 0, 0, 0, 0
		for _, member := range members {
			merged.RunningMs += member.RunningMs
			merged.RunnableMs += member.RunnableMs
			merged.SleepMs += member.SleepMs
			merged.DStateMs += member.DStateMs
			merged.IOWaitMs += member.IOWaitMs
		}
	case RootCauseMemberFoldCaliberIntervalUnion:
		merged.RunningMs, merged.RunnableMs, merged.SleepMs, merged.DStateMs, merged.IOWaitMs = 0, 0, 0, 0, 0
	}
	for _, member := range members[1:] {
		if member.DominantState != base.DominantState {
			merged.DominantState = ""
			break
		}
	}
	merged.ImpactMs = backgroundImpactMs(q, combined, hasCausalChain, rootCauseItemIsOnChain(merged))
	merged.ProjectedImpactMs = merged.ImpactMs
	foldedEffective := effectiveMax
	switch caliber {
	case RootCauseMemberFoldCaliberSumDisjoint:
		foldedEffective = effectiveSum
	case RootCauseMemberFoldCaliberIntervalUnion:
		if effectiveMatchesRaw {
			foldedEffective = combined
		}
	}
	if rootCauseTypeIsPriorityInversion(merged.Type) {
		var relationArtifactSources []string
		for _, member := range members {
			relationArtifactSources = append(relationArtifactSources, member.PriorityRelationArtifactSources...)
		}
		merged.PriorityRelationArtifactSources = priorityArtifactSourceUnion(relationArtifactSources...)
		if !priorityEffectiveMembersAuthorized || !priorityAuthorizedContributorSeen {
			// Fail the folded inversion family closed as one unit. Keeping a hard
			// base token/effective value while one contributor lacks its own
			// covering proof would let interval arithmetic and sibling provenance
			// become a second priority authority. Zeroing the typed attribution
			// here also makes the row rank-0 context before the publication guard.
			merged.PriorityRelationCaliber = string(priorityCaliberUnknown)
			merged.PriorityRelationProvenLowerMs = 0
			merged.PriorityRelationUnknownOrNonLowerMs = priorityProvenSum + priorityUnknownSum
			merged.GatedRunnableMs = 0
			merged.GatedRunningDeficitMs = 0
			merged.RankSortBoostedEffectiveMs = 0
			foldedEffective = 0
		} else {
			switch caliber {
			case RootCauseMemberFoldCaliberSumDisjoint:
				merged.GatedRunnableMs = gatedRunnableSum
				merged.GatedRunningDeficitMs = gatedDeficitSum
				merged.PriorityRelationProvenLowerMs = priorityProvenSum
				merged.PriorityRelationUnknownOrNonLowerMs = priorityUnknownSum
				if priorityProvenSum > 0 {
					merged.PriorityRelationCaliber = string(priorityCaliberClosedRangeStable)
				}
			case RootCauseMemberFoldCaliberIntervalUnion:
				switch {
				case effectiveMatchesRaw && allEffectivePureRunnable:
					merged.GatedRunnableMs = foldedEffective
					merged.GatedRunningDeficitMs = 0
					merged.PriorityRelationCaliber = string(priorityCaliberClosedRangeStable)
					merged.PriorityRelationProvenLowerMs = foldedEffective
					merged.PriorityRelationUnknownOrNonLowerMs = 0
				case effectiveMatchesRaw && allEffectivePureDeficit:
					merged.GatedRunnableMs = 0
					merged.GatedRunningDeficitMs = foldedEffective
					merged.PriorityRelationCaliber = strongestEffective.PriorityRelationCaliber
					merged.PriorityRelationProvenLowerMs = strongestEffective.PriorityRelationProvenLowerMs
					merged.PriorityRelationUnknownOrNonLowerMs = strongestEffective.PriorityRelationUnknownOrNonLowerMs
				default:
					// Partial gated subsets overlap without endpoint identity. MAX is
					// the only non-double-counting effective ruler; carry the same
					// strongest member's component split.
					foldedEffective = effectiveMax
					merged.GatedRunnableMs = strongestEffective.GatedRunnableMs
					merged.GatedRunningDeficitMs = strongestEffective.GatedRunningDeficitMs
					merged.PriorityRelationCaliber = strongestEffective.PriorityRelationCaliber
					merged.PriorityRelationProvenLowerMs = strongestEffective.PriorityRelationProvenLowerMs
					merged.PriorityRelationUnknownOrNonLowerMs = strongestEffective.PriorityRelationUnknownOrNonLowerMs
				}
			default:
				merged.GatedRunnableMs = strongestEffective.GatedRunnableMs
				merged.GatedRunningDeficitMs = strongestEffective.GatedRunningDeficitMs
				merged.PriorityRelationCaliber = strongestEffective.PriorityRelationCaliber
				merged.PriorityRelationProvenLowerMs = strongestEffective.PriorityRelationProvenLowerMs
				merged.PriorityRelationUnknownOrNonLowerMs = strongestEffective.PriorityRelationUnknownOrNonLowerMs
			}
		}
	}
	foldedEffective = backgroundImpactMs(q, foldedEffective, hasCausalChain, rootCauseItemIsOnChain(merged))
	if rootCauseTypeIsPriorityInversion(merged.Type) {
		componentTotal := merged.GatedRunnableMs + merged.GatedRunningDeficitMs
		if componentTotal > 0 && math.Abs(componentTotal-foldedEffective) > 0.000001 {
			scale := foldedEffective / componentTotal
			merged.GatedRunnableMs *= scale
			merged.GatedRunningDeficitMs *= scale
		}
	}
	if caliber == RootCauseMemberFoldCaliberCountSum {
		// G3 count-family identity (§27.2 audit, 2026-07-09): the Σ计入==V==
		// 引擎发布值 identity broke here — CumulativeImpactMs kept the raw
		// (uncapped) count-equivalent Σ (opendir_79 页缓存抖动 198.300, a
		// single member of 133.200 already exceeding the whole window) while
		// ImpactMs/Score published the §7.5-capped 41.671, and normalize then
		// stamped the raw Σ onto EffectiveImpactMs too — a three-face fork.
		// The published value on ALL value channels is the capped ImpactMs;
		// the raw count-equivalent Σ survives losslessly on the existing
		// member_sum disclosure channel below (the interval_union precedent).
		// Conservative on both sides: attribution faces never exceed the
		// capped published value; the disclosure face keeps the full raw Σ.
		merged.CumulativeImpactMs = merged.ImpactMs
		merged.EffectiveImpactMs = merged.ImpactMs
	} else {
		// Fold attribution with the same provable disjoint/union/MAX ruler as
		// the raw family, but from each member's typed effective scalar. Raw
		// Impact/Cumulative stay on the display lane and cannot leak back into
		// runnable/inversion/running attribution through normalization.
		merged.EffectiveImpactMs = foldedEffective
	}
	// Install the typed family identity before deriving Score. Interval-union
	// folds intentionally clear per-state scalar channels; the effective
	// accessor can recognize the already-adjudicated family scalar only when
	// MemberCount/MemberFoldCaliber are present. Writing them after Score
	// silently turned valid runnable and D/IO union families into score=0.
	merged.MemberCount = memberCount
	merged.MemberFoldCaliber = caliber
	merged.Score = rootCauseRankScoreBasisMs(merged) * merged.Confidence * rootCauseItemScoreWeight(merged)
	// WINFLAG-1 (c) envelope fold: in a flagged [0,end] run the base's real
	// ts==0 start is a determined envelope floor — a later positive member
	// must not raise it (the legacy `merged.StartTs == 0` arm reads 0 as
	// "unset yet"), and a member's real 0 start may lower the envelope to 0.
	// Without the flag both extra arms are dead and the fold is
	// byte-identical to the legacy form.
	mergedZeroStartReal := zeroStartReal && merged.StartTs == 0 && merged.EndTs > merged.StartTs
	for _, member := range members[1:] {
		if member.StartTs > 0 && (merged.StartTs == 0 || member.StartTs < merged.StartTs) {
			if !mergedZeroStartReal {
				merged.StartTs = member.StartTs
			}
		}
		if zeroStartReal && member.StartTs == 0 && member.EndTs > member.StartTs {
			merged.StartTs = 0
			mergedZeroStartReal = true
		}
		if member.EndTs > merged.EndTs {
			merged.EndTs = member.EndTs
		}
		if member.LineStart > 0 && (merged.LineStart == 0 || member.LineStart < merged.LineStart) {
			merged.LineStart = member.LineStart
		}
		if member.LineEnd > merged.LineEnd {
			merged.LineEnd = member.LineEnd
		}
	}
	merged.MemberRoster = roster
	// XLANE-2 件1 fail-open: the re-fold rebuilds a BOUNDED roster from mixed
	// member carriers — the seed's complete line-range claim no longer holds
	// on the merged row, so the typed carrier clears (absence never judges;
	// a partial set could fake a subset verdict). SPANTOP-1 件1: the complete
	// per-member wall-clock claim dies with it (same all-or-nothing lane).
	merged.MemberLineRanges = nil
	merged.MemberWallMs = nil
	merged.MemberMaxMs = maxMs
	merged.MemberMinMs = minMs
	// G1 (§27.2, 2026-07-09): keep the VALIDATED member intervals on the
	// merged row (engine-internal, never serialized) — the cross-lane
	// reconciliation tests a critical_blocking row's interval against this
	// member UNION, never against the merged hull (hull gaps between disjoint
	// members would absorb non-members — a noisy membership signal).
	merged.familyMemberIntervals = reconIntervals
	merged.MemberSumMs = 0
	// The raw-Σ disclosure keys on the PUBLISHED value (CumulativeImpactMs):
	// identical to the legacy `combined < sum` gate on every non-count arm
	// (there Cumulative == combined), and on the count arm it now fires
	// whenever the §7.5 cap bit — the raw count-equivalent Σ must stay
	// disclosed, never silently absorbed by the cap (G3).
	if merged.CumulativeImpactMs < sum {
		merged.MemberSumMs = sum
	}
	merged.MemberKey = ""
	if !sameInode {
		merged.Inode = ""
	}
	if !sameDev {
		merged.Dev = ""
	}
	// Compact by design: the full member roster rides the typed member_roster
	// note / MemberRoster field — the prose face only declares the merge and
	// its caliber (summary byte budgets are shared with every other section).
	// G3: the summary prints the PUBLISHED value (same source as the tree /
	// score faces) and renders count-family magnitudes through the same
	// count-equivalent helper as the roster (两面同源) — a bare wall-clock
	// "ms" on a count-derived scalar is the §27.2 wording over-claim.
	combinedFace := fmt.Sprintf("%.3fms", merged.CumulativeImpactMs)
	sumFace := fmt.Sprintf("%.3fms", sum)
	if countFamily {
		combinedFace = rootCauseCountEquivalentValue(merged.CumulativeImpactMs)
		sumFace = rootCauseCountEquivalentValue(sum)
	}
	merged.Summary = fmt.Sprintf("%s | merged x%d same-thread: combined=%s (%s)",
		base.Summary, memberCount, combinedFace, caliber)
	if merged.MemberSumMs > 0 {
		merged.Summary = fmt.Sprintf("%s member_sum=%s", merged.Summary, sumFace)
	}
	return merged
}

// rootCauseCountEquivalentValue renders a Count-additivity magnitude (a
// count-derived advisory scalar — registry CausalAdditivityCount: "not a
// physical duration even when printed with an ms suffix") for the roster and
// summary faces. The 计数当量 marker keeps the number comparable while
// refusing the bare wall-clock "ms" reading (G3, §27.2/§28.1 收口 2026-07-09;
// both faces render through THIS one helper — 两面同源, and the marker is
// generic per-CLASS: page_cache_churn, file_io_hot_inode and any future
// count-class family share it — never a per-type carve-out).
//
// EVOLUTION RECORD (§29.55 观察③ 两形一裁, WF-2 词面批 2026-07-14):
// 「计数当量Xms」→「计数当量X(非墙钟)」 — the value is NOT wall-clock
// milliseconds, so the ms suffix was a caliber lie (G3/DISP-2 裸 ms 纪律的
// 词面面); the sidebar's (非墙钟) qualifier won the ruling and every
// value-adjacent minting point speaks the one form. OMGCLEAN-1 双复核 件13
// (冷读 CR11 定稿空格, 2026-07-21): the word and the value separate with one
// space (计数当量 11.100) — the display mirror
// (runtimeTraceProjCountEquivalentValueText) co-moves byte-for-byte.
func rootCauseCountEquivalentValue(ms float64) string {
	return fmt.Sprintf("计数当量 %.3f(非墙钟)", ms)
}

// rootCauseFamilyMemberKey is the roster-entry identity fallback for a member
// that carries no mint-time typed MemberKey: the typed time window, then the
// evidence line range — never a Summary re-parse (§24.9 dim-B F3 red line).
// WINFLAG-1 (c) (§29.190④): the window-form gate reads the flag — a flagged
// [0,end] run keeps a real ts==0-starting member on the window-form key
// instead of degrading to the line-range fallback; without the flag the gate
// is byte-identical to the legacy `StartTs > 0` guard.
func rootCauseFamilyMemberKey(member RootCauseRankItem, zeroStartReal bool) string {
	if key := strings.TrimSpace(member.MemberKey); key != "" {
		return key
	}
	if rankFoldStartUsable(member.StartTs, member.EndTs, zeroStartReal) {
		return fmt.Sprintf("%.6f..%.6f", member.StartTs, member.EndTs)
	}
	return fmt.Sprintf("lines %d-%d", member.LineStart, member.LineEnd)
}

// rootCauseFamilyMemberRosterKey is the roster FACE of the member key
// (§29.104.21 DISPLAY-HYG 件4, 2026-07-17): a runnable cpu=unknown member
// whose census bucket carried the UNIFORM window_end_unverified continuity
// reason wears the why word — the capture ended while the thread was still
// queued runnable, so no closing switch-in exists to verify a CPU (§29.107
// XCPU rules make bare unknown the only honest value; the why word explains
// it at the point of reading). Typed gate: the carried reason must BE the
// open-ended reason — mixed/absent/conflict reasons keep the bare
// cpu=unknown (负臂, never a guess). zh word on the single-form engine
// roster face (计数当量 precedent: zh caliber words live on engine roster
// faces byte-identically for both report languages). Identity lanes keep
// consuming MemberKey itself — the why word is display-only bytes.
func rootCauseFamilyMemberRosterKey(member RootCauseRankItem, zeroStartReal bool) string {
	key := rootCauseFamilyMemberKey(member, zeroStartReal)
	if key == "cpu=unknown" && member.runnableCPUUnknownReason == RunnableCPUContinuityOpenEnded {
		return key + "(采集端截断,无收尾切入)"
	}
	return key
}

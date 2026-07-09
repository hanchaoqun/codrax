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
	"sort"
	"strings"
)

// --- shared interval algebra -------------------------------------------------

type foldInterval struct {
	start, end float64
}

// foldIntervalUnionMs computes the total length (ms) of the union of the
// given [start,end] second-intervals plus whether they are pairwise disjoint.
// Callers pass only validated intervals (end > start).
func foldIntervalUnionMs(intervals []foldInterval) (unionMs float64, disjoint bool) {
	if len(intervals) == 0 {
		return 0, true
	}
	sorted := make([]foldInterval, len(intervals))
	copy(sorted, intervals)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].start != sorted[j].start {
			return sorted[i].start < sorted[j].start
		}
		return sorted[i].end < sorted[j].end
	})
	disjoint = true
	curStart, curEnd := sorted[0].start, sorted[0].end
	total := 0.0
	for _, iv := range sorted[1:] {
		if iv.start < curEnd {
			disjoint = false
			if iv.end > curEnd {
				curEnd = iv.end
			}
			continue
		}
		total += curEnd - curStart
		curStart, curEnd = iv.start, iv.end
	}
	total += curEnd - curStart
	return total * 1000, disjoint
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

// semanticSpanFamilyLane is the DCS E2 道别 predicate applied at family-fold
// time: on-chain requires a SAME-THREAD chain node / causal-impact window
// overlap with the (already window-clipped) span — thread membership alone
// never flips a lane (道别红线; the huadong E21 adjacent precedent). Returns
// the lane plus the best-overlap dominant state / chain depth for display
// context.
func semanticSpanFamilyLane(chain *ChainResult, span TraceSpanSummary) (onChain bool, depth int, state string) {
	if chain == nil {
		return false, 0, ""
	}
	best := 0.0
	consider := func(winStart, winEnd float64, nodeState string, nodeDepth int) {
		overlapStart, overlapEnd, ok := overlapTimeWindow(span.StartTs, span.EndTs, winStart, winEnd)
		if !ok {
			return
		}
		overlapMs := (overlapEnd - overlapStart) * 1000
		if overlapMs <= 0 {
			return
		}
		onChain = true
		if overlapMs > best || state == "" {
			best = overlapMs
			state = nodeState
			depth = nodeDepth
		}
	}
	for _, node := range chain.Nodes {
		if !sameThreadRef(node.Thread, span.Thread) {
			continue
		}
		nodeState := string(node.Dominant)
		nodeDepth := 0
		if node.Impact != nil {
			nodeDepth = node.Impact.ChainDepth
			if nodeState == "" {
				nodeState = node.Impact.DominantState
			}
		}
		consider(node.Window.StartTs, node.Window.EndTs, nodeState, nodeDepth)
	}
	if !onChain {
		for _, impact := range chain.CausalImpacts {
			if !sameThreadRef(impact.Thread, span.Thread) {
				continue
			}
			consider(impact.Window.StartTs, impact.Window.EndTs, impact.DominantState, impact.ChainDepth)
		}
	}
	return onChain, depth, state
}

// FoldSemanticSpanFamilies folds the window-clipped semantic spans of ONE
// window_stats run into (thread, semantic class, chain lane) families. The
// input spans MUST come from a single stats run (they already share one
// selected window — the M3 same-window discipline holds by construction; the
// tool observation face additionally stamps the family record with that run's
// selected_window note, unchanged). stats.TraceSpans is never modified.
// Families emit in FIRST-MEMBER APPEARANCE order (the span inventory is
// already duration-ranked by the stats bound, and appearance order keeps the
// degenerate all-singles shape byte-identical to the pre-RCM per-span
// iteration on both consumers); members inside a family sort largest first
// (Members[0] is the representative).
func FoldSemanticSpanFamilies(chain *ChainResult, spans []TraceSpanSummary) []SemanticSpanFamily {
	type laneKey struct {
		thread string
		class  string
		lane   bool
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
		onChain, depth, state := semanticSpanFamilyLane(chain, span)
		key := laneKey{thread: threadKey(span.Thread), class: traceSpanSemanticClass(span.Name), lane: onChain}
		group, ok := groups[key]
		if !ok {
			group = &acc{family: SemanticSpanFamily{
				Thread:        span.Thread,
				SemanticClass: key.class,
				OnChain:       onChain,
				ChainDepth:    depth,
				DominantState: state,
			}}
			groups[key] = group
			order = append(order, key)
		}
		if onChain && (group.family.DominantState == "" || span.DurationMs > group.family.MaxMs) {
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
			fam.SumMs += member.DurationMs
			intervals = append(intervals, foldInterval{start: member.StartTs, end: member.EndTs})
			if fam.StartTs == 0 || member.StartTs < fam.StartTs {
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
// multi-member semantic family (§24.10: 合并量参赛,参赛值=窗口投影合计 — the
// user explicitly rejected 单次最大). Single-member families never come here:
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
	projection := semanticTraceSpanProjection{
		StartTs:       fam.StartTs,
		EndTs:         fam.EndTs,
		ImpactMs:      fam.TotalMs,
		DominantState: fam.DominantState,
		ChainDepth:    fam.ChainDepth,
		OnChain:       fam.OnChain,
	}
	effectiveImpactMs := semanticTraceSpanEffectiveImpactMs(work, projection, TraceSpanSummary{DurationMs: fam.TotalMs})
	summary := fmt.Sprintf("%s family ×%d same-thread span(s) totalled %.3fms window projection (largest %q %.3fms, smallest %.3fms); effective_impact=%.3fms; fold_caliber=%s",
		work.Label, len(fam.Members), fam.TotalMs, rep.Name, fam.MaxMs, fam.MinMs, effectiveImpactMs, fam.FoldCaliber)
	if fam.TotalMs < fam.SumMs {
		summary = fmt.Sprintf("%s; interval_union=%.3fms < member_sum=%.3fms (overlapping same-thread segments deduplicated)", summary, fam.TotalMs, fam.SumMs)
	}
	if fam.OnChain && effectiveImpactMs > fam.TotalMs {
		summary = fmt.Sprintf("%s; semantic_multiplier=%.2f hidden_cost_boost=true", summary, work.ImpactMultiplier)
	}
	if fam.DominantState != "" {
		summary = fmt.Sprintf("%s; overlapped_chain_state=%s", summary, fam.DominantState)
	}
	item := rootCauseItem(work.RootCauseType, fam.Thread, fam.TotalMs, work.Confidence, fam.StartLine, fam.EndLine, "window_stats.trace_spans.semantic", summary)
	item.StartTs = fam.StartTs
	item.EndTs = fam.EndTs
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
	item.ProjectedImpactMs = fam.TotalMs
	item.CumulativeImpactMs = fam.TotalMs
	item.EffectiveImpactMs = effectiveImpactMs
	item.SpanName = rep.Name
	item.SpanKind = rep.Kind
	item.SpanCategory = firstNonEmpty(rep.Category, work.Category)
	item.SpanSubcategory = firstNonEmpty(rep.Subcategory, work.Subcategory)
	item.SemanticClass = work.SemanticClass
	item.DominantState = fam.DominantState
	item.ChainDepth = fam.ChainDepth
	if fam.OnChain {
		item.Causality = "on_wakeup_chain"
		item.ChainRelevance = "on_chain"
	}
	applySemanticTraceSpanState(&item, fam.DominantState, fam.TotalMs)
	item.MemberCount = len(fam.Members)
	item.MemberRoster = fam.MemberRosterEntries()
	item.MemberMaxMs = fam.MaxMs
	item.MemberMinMs = fam.MinMs
	if fam.TotalMs < fam.SumMs {
		item.MemberSumMs = fam.SumMs
	}
	item.MemberFoldCaliber = fam.FoldCaliber
	item.Score = item.EffectiveImpactMs * item.Confidence * rootCauseItemScoreWeight(item)
	return item, true
}

// rootCauseFamilyRosterCap bounds every typed member roster (§24.7.1 ①
// 可折叠显示: the roster is bounded; overflow is expressed by MemberCount and
// the full inventory stays available on the source view — never a silent
// drop, the count discloses).
const rootCauseFamilyRosterCap = 8

// MemberRosterEntries renders the bounded typed member roster ("<span name>
// <clipped ms>ms", largest first, capped at rootCauseFamilyRosterCap — the
// count discloses overflow). ONE renderer for BOTH consumers (rank item
// MemberRoster and the observation-channel member_roster note), so the two
// faces can never disagree on the roster shape.
func (fam SemanticSpanFamily) MemberRosterEntries() []string {
	out := make([]string, 0, minIntFold(len(fam.Members), rootCauseFamilyRosterCap))
	for i, member := range fam.Members {
		if i >= rootCauseFamilyRosterCap {
			break
		}
		out = append(out, fmt.Sprintf("%s %.3fms", member.Name, member.DurationMs))
	}
	return out
}

func minIntFold(a, b int) int {
	if a < b {
		return a
	}
	return b
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
func rootCauseFamilyFoldLaneKey(item RootCauseRankItem) string {
	if rootCauseItemIsOnChain(item) {
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
		typ    string
		thread string
		lane   string
		window string
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
	memberCount := 0
	roster := make([]string, 0, minIntFold(len(members), rootCauseFamilyRosterCap))
	maxMs, minMs := 0.0, 0.0
	sameInode, sameDev := true, true
	for _, member := range members {
		raw := rootCauseCumulativeImpactMs(member)
		sum += raw
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
		if member.StartTs > 0 && member.EndTs > member.StartTs {
			intervals = append(intervals, foldInterval{start: member.StartTs, end: member.EndTs})
		} else {
			intervalsUsable = false
		}
		if len(member.familyMemberIntervals) > 0 {
			reconIntervals = append(reconIntervals, member.familyMemberIntervals...)
		} else if member.StartTs > 0 && member.EndTs > member.StartTs {
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
				roster = append(roster, fmt.Sprintf("%s %s", rootCauseFamilyMemberKey(member), rootCauseCountEquivalentValue(raw)))
			} else {
				roster = append(roster, fmt.Sprintf("%s %.3fms", rootCauseFamilyMemberKey(member), raw))
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
	case valuesAreIntervalLengths:
		caliber = RootCauseMemberFoldCaliberIntervalUnion
		combined = unionMs
		if combined > sum {
			combined = sum
		}
	}
	merged := base
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
	} else if base.EffectiveImpactMs > 0 {
		// Pre-normalize members rarely carry an explicit effective value; when
		// the representative does, keep the family on the same discipline: the
		// combined magnitude, single-capped (normalize fills the rest later).
		merged.EffectiveImpactMs = merged.ImpactMs
	}
	merged.Score = merged.ImpactMs * merged.Confidence * rootCauseItemScoreWeight(merged)
	for _, member := range members[1:] {
		if member.StartTs > 0 && (merged.StartTs == 0 || member.StartTs < merged.StartTs) {
			merged.StartTs = member.StartTs
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
	merged.MemberCount = memberCount
	merged.MemberRoster = roster
	merged.MemberMaxMs = maxMs
	merged.MemberMinMs = minMs
	merged.MemberFoldCaliber = caliber
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
func rootCauseCountEquivalentValue(ms float64) string {
	return fmt.Sprintf("计数当量%.3fms", ms)
}

// rootCauseFamilyMemberKey is the roster-entry identity fallback for a member
// that carries no mint-time typed MemberKey: the typed time window, then the
// evidence line range — never a Summary re-parse (§24.9 dim-B F3 red line).
func rootCauseFamilyMemberKey(member RootCauseRankItem) string {
	if key := strings.TrimSpace(member.MemberKey); key != "" {
		return key
	}
	if member.StartTs > 0 && member.EndTs > member.StartTs {
		return fmt.Sprintf("%.6f..%.6f", member.StartTs, member.EndTs)
	}
	return fmt.Sprintf("lines %d-%d", member.LineStart, member.LineEnd)
}

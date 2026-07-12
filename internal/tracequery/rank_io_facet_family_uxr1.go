package tracequery

// rank_io_facet_family_uxr1.go — UXR-1 件① §29.36③ (user ruling 2026-07-11,
// ledger real_trace_campaign_20260705.md): the ◇ adjacent-channel IO facet
// FAMILY AGGREGATE SEAT.
//
// Witness (real_trace_a5 eval log 4165): ONE physical IO wait of ONE thread
// published FIVE adjacent-channel seats through five facets — 页缓存抖动
// (page_cache_churn) / 块设备IO(inode) (block_io_by_inode) / IO延迟
// (io_latency) / IO突发 (io_burst_episode) / iowait (io_wait) — occupying half
// the board for ~0.6ms of physical wait. Per the ruling the same-thread
// same-segment cross-facet rows hold ONE family seat: the family aggregate row
// publishes the INTERVAL-UNION wall clock (interval_union — an existing wire
// caliber; the display's 合计(共N段,同线程) fifth-word machinery renders it),
// and the member rows move to the lossless AbsorbedItems carrier (成员
// absorbed 明细不占席 — the ORD aggregate/member seat-exclusivity and the B4
// adjudicated-absorption precedents). This pass IS the "B4 裁定表全族扩对":
// the B4 exact matcher cannot express the family form (facets carry different
// values/intervals of one segment), so the family closed set gets its own
// adjudicated pass with the same lossless-carrier discipline.
//
// Precision rules (fail-open to the multi-row publication on every miss):
//   - closed facet set = exactly the five ruling-named types;
//   - population = ◇ adjacent channel only (rootCauseOrdinalChannel — the
//     same chain-relevance single source the ordinal allocator reads);
//   - group key = numeric TID (exact, >0) + typed stats-window identity +
//     chain lane (rootCauseFamilyFoldLaneKey);
//   - each facet type at most ONCE per group (the same-thread type fold runs
//     first); a duplicate facet is ambiguity — the whole group fails open;
//   - 同段 proof: every UNION facet (io_wait / io_latency / io_burst_episode)
//     must prove its TRUE interval inventory, and the inventory must merge
//     into ONE contiguous union (overlap-connected component). 复核 P1-1(a):
//     the same-thread same-type fold runs FIRST (query.go build/enrich order)
//     and overwrites a merged row's StartTs/EndTs with the members' HULL —
//     hull gaps would fake 同段 connectivity AND count gap wall clock into
//     the published union (双 burst Σ=2ms hull=160ms 病理形). A MemberCount>1
//     row therefore contributes its validated familyMemberIntervals, ALL of
//     which enter the connectivity check and the union; a folded union facet
//     WITHOUT that inventory fails the whole group open;
//   - 复核 P1-1(b): block_io_by_inode is EXCLUDED from both the wall-clock
//     connectivity proof and the union value — its published value is a
//     COMPOSITE score (BlockMax+StorageMax+MiB, query.go mint), its interval
//     is the inode ENVELOPE, and its registry wall_clock_per_thread token
//     (untouched — 红线 §7.2.1) would otherwise let the envelope fake-bridge
//     disjoint facets and stand in as a wall-clock union value. It joins the
//     family as a ROSTER-ONLY member (the count-facet treatment) when its own
//     intervals provably overlap the union hull; otherwise it keeps its
//     independent seat;
//   - count facets (page_cache_churn, registry additivity count) join the
//     roster only when their own intervals overlap the union hull — count
//     equivalents are NEVER summed into the wall-clock seat value; a
//     non-overlapping / interval-less roster-only row keeps its own seat;
//   - published-value invariant (复核 P1-1(c)): the union wall clock can
//     never exceed the Σ of the union members' PUBLISHED values — an
//     inventory that over-claims (envelopes wider than the published wall
//     clock) fails the group open instead of minting a value no member set
//     can account for;
//   - ≥2 folded members required (a lone facet is not a family).
//
// Idempotency: the synthesized family row carries the dedicated Source token
// (survives the B4 reset, which clears RankFamilyKey), so the reset-first
// recomputation can always identify and drop a stale family row before
// refolding — both the build and the enrich pipeline call this pass right
// after reconcileExactCrossTypeRankSeats.

import (
	"fmt"
	"sort"
	"strings"
)

// adjacentIOFacetFamilySource is the synthesized family row's provenance
// token (idempotency marker; no adjudicated B4 spec matches it).
const adjacentIOFacetFamilySource = "window_stats.io_facet_family"

// adjacentIOFacetFamilyKeyPrefix marks this pass's absorption ownership on
// the AbsorbedIntoFamily carrier.
const adjacentIOFacetFamilyKeyPrefix = "io_facet_adjacent:"

// adjacentIOFacetFamilyTypes is the §29.36③ closed facet set (ruling-named
// five facets; adding a facet changes seat semantics and requires a witness
// plus an explicit ruling).
var adjacentIOFacetFamilyTypes = map[string]bool{
	"io_wait":           true,
	"io_latency":        true,
	"io_burst_episode":  true,
	"block_io_by_inode": true,
	"page_cache_churn":  true,
}

// adjacentIOFacetLeadOrder picks the family row's carrying type: the formal
// scheduler-state lane first (the B4 seat-owner discipline). Roster-only
// facets (count facets and the 复核 P1-1(b) block_io_by_inode carve) never
// lead — the lead is chosen among the union facets.
var adjacentIOFacetLeadOrder = []string{"io_wait", "io_latency", "io_burst_episode"}

func adjacentIOFacetFamilyGroupKey(item RootCauseRankItem) string {
	return fmt.Sprintf("%d|%.6f..%.6f|%s", item.Thread.PID,
		item.StatsWindowStartTs, item.StatsWindowEndTs, rootCauseFamilyFoldLaneKey(item))
}

// CausalIOFacetFamilyToken exports the closed IO facet family membership —
// the ONE set the ◇ engine fold below and the IOFAM-SELF display fold
// (internal/tool, CAL-1 件② 2026-07-12) both consume; a second hand copy of
// this set is forbidden (§29.38 M-family discipline).
func CausalIOFacetFamilyToken(token string) bool {
	return adjacentIOFacetFamilyTypes[token]
}

func adjacentIOFacetWallClockFacet(typ string) bool {
	spec, ok := CausalTokenSpecFor(typ)
	return ok && spec.Additivity == CausalAdditivityWallClockPerThread
}

// adjacentIOFacetUnionFacet reports whether the facet participates in the
// 同段 connectivity proof and the published interval-union value. 复核
// P1-1(b): block_io_by_inode is carved out despite its registry wall-clock
// token (the token lane itself is untouched — 红线 §7.2.1): its value is a
// composite score over an inode ENVELOPE interval, so it can neither prove
// connectivity nor account for wall clock. It stays a roster-only member.
// V2-P0 (2026-07-12): the carve-out now reads the SHARED typed
// composite-score marker (CausalTokenCaliberSideClass) instead of a local
// type-name literal — behavior identical, single implementation.
func adjacentIOFacetUnionFacet(typ string) bool {
	if CausalTokenCaliberSideClass(typ) != CausalCaliberSideNone {
		return false
	}
	return adjacentIOFacetWallClockFacet(typ)
}

// adjacentIOFacetItemIntervals resolves a candidate row's TRUE interval
// inventory (复核 P1-1(a)): a same-thread same-type MERGED row (the type fold
// runs before this pass) carries its members' hull on StartTs/EndTs — the
// validated per-member inventory lives on familyMemberIntervals and is the
// only admissible connectivity/union input. ok=false when a folded row lacks
// the inventory (or any interval is invalid): the hull alone cannot prove 同段.
func adjacentIOFacetItemIntervals(item RootCauseRankItem) ([]foldInterval, bool) {
	if item.MemberCount > 1 {
		if len(item.familyMemberIntervals) == 0 {
			return nil, false
		}
		for _, iv := range item.familyMemberIntervals {
			if iv.end <= iv.start {
				return nil, false
			}
		}
		return item.familyMemberIntervals, true
	}
	if item.EndTs <= item.StartTs {
		return nil, false
	}
	return []foldInterval{{start: item.StartTs, end: item.EndTs}}, true
}

// reconcileAdjacentIOFacetFamilySeats folds same-thread same-segment adjacent
// IO facet rows into one family aggregate seat (§29.36③). Reset-first, then
// recompute — idempotent across the build and enrich pipelines.
func reconcileAdjacentIOFacetFamilySeats(rank *RootCauseRankResult) {
	if rank == nil {
		return
	}
	// Reset: drop stale synthesized family rows; restore this pass's absorbed
	// members (the B4 reset usually restored them already — both paths end in
	// the same clean state).
	active := make([]RootCauseRankItem, 0, len(rank.Items))
	for _, item := range rank.Items {
		if item.Source == adjacentIOFacetFamilySource {
			continue
		}
		active = append(active, item)
	}
	keptAbsorbed := make([]RootCauseRankItem, 0, len(rank.AbsorbedItems))
	for _, item := range rank.AbsorbedItems {
		if strings.HasPrefix(item.AbsorbedIntoFamily, adjacentIOFacetFamilyKeyPrefix) {
			item.AbsorbedByRankFamily = false
			item.AbsorbedIntoFamily = ""
			if item.Tier == RootCauseTierAbsorbed {
				item.Tier = ""
			}
			item.Rank = 0
			item.BackgroundRank = 0
			active = append(active, item)
			continue
		}
		keptAbsorbed = append(keptAbsorbed, item)
	}

	type group struct {
		order   []int          // candidate indexes in pool order
		byType  map[string]int // facet type -> count
		blocked bool
	}
	groups := map[string]*group{}
	var keys []string
	for i := range active {
		item := active[i]
		if !adjacentIOFacetFamilyTypes[item.Type] || item.Thread.PID <= 0 {
			continue
		}
		if rootCauseOrdinalChannel(item) != rootCauseOrdinalChannelAdjacent {
			continue
		}
		key := adjacentIOFacetFamilyGroupKey(item)
		g := groups[key]
		if g == nil {
			g = &group{byType: map[string]int{}}
			groups[key] = g
			keys = append(keys, key)
		}
		g.order = append(g.order, i)
		g.byType[item.Type]++
		if g.byType[item.Type] > 1 {
			g.blocked = true // duplicate facet = ambiguity, fail open
		}
	}
	sort.Strings(keys)

	folded := map[int]string{} // active index -> family key
	type familyPlan struct {
		key       string
		members   []int
		lead      int
		unionMS   float64
		hull      foldInterval
		intervals map[int][]foldInterval // union members' TRUE intervals
	}
	var plans []familyPlan
	for _, key := range keys {
		g := groups[key]
		if g.blocked || len(g.order) < 2 {
			continue
		}
		// 同段 proof over the UNION facets' true interval inventory (复核
		// P1-1(a)): every interval of every union member enters both the
		// connectivity check and the union; a folded member without its
		// inventory fails the whole group open.
		var unionIdx []int
		var rosterIdx []int
		intervalsOK := true
		var intervals []foldInterval
		memberIntervals := map[int][]foldInterval{}
		for _, idx := range g.order {
			item := active[idx]
			if adjacentIOFacetUnionFacet(item.Type) {
				ivs, ok := adjacentIOFacetItemIntervals(item)
				if !ok {
					intervalsOK = false
					break
				}
				unionIdx = append(unionIdx, idx)
				intervals = append(intervals, ivs...)
				memberIntervals[idx] = ivs
				continue
			}
			rosterIdx = append(rosterIdx, idx)
		}
		if !intervalsOK || len(unionIdx) == 0 {
			continue
		}
		merged, _ := foldIntervalUnionWithDisjoint(intervals)
		if len(merged) != 1 {
			continue // not one overlap-connected segment — fail open
		}
		hull := merged[0]
		unionMS := (hull.end - hull.start) * 1000
		// 复核 P1-1(c) invariant: the published union wall clock must be
		// accountable by the union members' PUBLISHED values — Σ(published) is
		// the ceiling. An inventory whose envelopes over-claim fails open.
		publishedSum := 0.0
		for _, idx := range unionIdx {
			value := rootCauseEffectiveImpactMs(active[idx])
			if value <= 0 {
				value = active[idx].ImpactMs
			}
			publishedSum += value
		}
		if unionMS > publishedSum+0.0005 {
			continue
		}
		members := append([]int(nil), unionIdx...)
		for _, idx := range rosterIdx {
			// A roster-only facet (count facet / block_io_by_inode) joins only
			// when its own TRUE intervals provably overlap the segment hull;
			// otherwise it keeps its seat. A folded roster-only row without
			// its inventory keeps its seat too (hull overlap proves nothing).
			ivs, ok := adjacentIOFacetItemIntervals(active[idx])
			if !ok {
				continue
			}
			for _, iv := range ivs {
				if iv.start < hull.end && iv.end > hull.start {
					members = append(members, idx)
					break
				}
			}
		}
		if len(members) < 2 {
			continue
		}
		// Lead: formal-lane priority among the union facets.
		lead := -1
		for _, typ := range adjacentIOFacetLeadOrder {
			for _, idx := range unionIdx {
				if active[idx].Type == typ {
					lead = idx
					break
				}
			}
			if lead >= 0 {
				break
			}
		}
		if lead < 0 {
			continue
		}
		plans = append(plans, familyPlan{
			key:       adjacentIOFacetFamilyKeyPrefix + key,
			members:   members,
			lead:      lead,
			unionMS:   unionMS,
			hull:      hull,
			intervals: memberIntervals,
		})
		for _, idx := range members {
			folded[idx] = adjacentIOFacetFamilyKeyPrefix + key
		}
	}
	if len(plans) == 0 {
		rank.Items = active
		rank.AbsorbedItems = keptAbsorbed
		return
	}

	out := make([]RootCauseRankItem, 0, len(active))
	absorbed := keptAbsorbed
	planned := map[string]familyPlan{}
	for _, plan := range plans {
		planned[plan.key] = plan
	}
	emitted := map[string]bool{}
	for i := range active {
		key, isMember := folded[i]
		if !isMember {
			out = append(out, active[i])
			continue
		}
		// Every member becomes a lossless absorbed carrier.
		member := active[i]
		member.Rank = 0
		member.BackgroundRank = 0
		member.Tier = RootCauseTierAbsorbed
		member.AbsorbedByRankFamily = true
		member.AbsorbedIntoFamily = key
		absorbed = append(absorbed, member)
		if emitted[key] {
			continue
		}
		emitted[key] = true
		plan := planned[key]
		lead := active[plan.lead]
		fam := lead
		fam.Source = adjacentIOFacetFamilySource
		fam.RankFamilyKey = key
		fam.Rank = 0
		fam.BackgroundRank = 0
		fam.Tier = ""
		fam.StartTs = plan.hull.start
		fam.EndTs = plan.hull.end
		fam.ImpactMs = plan.unionMS
		fam.CumulativeImpactMs = plan.unionMS
		fam.EffectiveImpactMs = plan.unionMS
		fam.RankSortBoostedEffectiveMs = 0
		fam.Score = plan.unionMS * fam.Confidence
		// Interval-union families clear the per-state scalar sums — the union
		// wall clock cannot be reconstructed into a per-state split, and a
		// stale lead-row scalar would fork the effective derivation
		// (rootCauseEffectiveImpactMsUncapped family arm reads the folded
		// field authoritatively).
		fam.RunningMs = 0
		fam.RunnableMs = 0
		fam.SleepMs = 0
		fam.DStateMs = 0
		fam.IOWaitMs = 0
		fam.MemberCount = len(plan.members)
		fam.MemberFoldCaliber = RootCauseMemberFoldCaliberIntervalUnion
		fam.MemberKey = ""
		fam.Inode = ""
		fam.Dev = ""
		fam.familyMemberIntervals = nil
		roster := make([]string, 0, len(plan.members))
		sum := 0.0
		maxMs, minMs := 0.0, 0.0
		lineStart, lineEnd := 0, 0
		for _, idx := range plan.members {
			m := active[idx]
			value := rootCauseEffectiveImpactMs(m)
			if value <= 0 {
				value = m.ImpactMs
			}
			entry := m.Type
			if strings.TrimSpace(m.MemberKey) != "" {
				entry += " " + strings.TrimSpace(m.MemberKey)
			}
			switch {
			case adjacentIOFacetUnionFacet(m.Type):
				entry += fmt.Sprintf(" %.3fms", value)
				sum += value
				if maxMs == 0 || value > maxMs {
					maxMs = value
				}
				if minMs == 0 || value < minMs {
					minMs = value
				}
				// 复核 P1-1(a): the family carries the members' TRUE validated
				// intervals (the cross-lane recon consumes this union — hull
				// gaps must not absorb non-members).
				fam.familyMemberIntervals = append(fam.familyMemberIntervals,
					plan.intervals[idx]...)
			case adjacentIOFacetWallClockFacet(m.Type):
				// block_io_by_inode (复核 P1-1(b)): roster-only — the composite
				// score discloses beside the union but never enters it.
				entry += fmt.Sprintf(" %.3fms(不计入墙钟合计)", value)
			default:
				// Count facets disclose as count equivalents — never summed
				// into the wall-clock seat value (计数当量非墙钟).
				entry += fmt.Sprintf(" 计数当量%.3fms", value)
			}
			roster = append(roster, entry)
			if lineStart == 0 || (m.LineStart > 0 && m.LineStart < lineStart) {
				lineStart = m.LineStart
			}
			if m.LineEnd > lineEnd {
				lineEnd = m.LineEnd
			}
		}
		fam.MemberRoster = roster
		fam.MemberSumMs = sum
		fam.MemberMaxMs = maxMs
		fam.MemberMinMs = minMs
		fam.memberSegmentsProducerDisjoint = false
		if lineStart > 0 {
			fam.LineStart = lineStart
		}
		if lineEnd > 0 {
			fam.LineEnd = lineEnd
		}
		fam.Summary = fmt.Sprintf("%s-%d adjacent IO facet family x%d (interval union %.3fms; facets: %s)",
			fam.Thread.Comm, fam.Thread.PID, len(plan.members), plan.unionMS, strings.Join(roster, " | "))
		out = append(out, fam)
	}
	rank.Items = out
	rank.AbsorbedItems = absorbed
}

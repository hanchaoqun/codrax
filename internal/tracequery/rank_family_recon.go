package tracequery

// rank_family_recon.go — G1 cross-lane duplicate-display reconciliation
// (§27.2-G1, user ruling 收口批准 §28.1, 2026-07-09,
// docs/design/real_trace_campaign_20260705.md).
//
// ── The double publication being reconciled ─────────────────────────────────
//
// ONE batch of source events (opendir_79 / huadong_79 witnesses: a thread's
// io_latency block-IO completions) is published through TWO disjoint lanes of
// the SAME result:
//
//   rank lane   — buildRootCauseRankFrom mints one rank row per
//                 stats.IOLatencies entry; foldSameThreadTypeRankFamilies
//                 merges the same-(thread,type) rows into ONE family
//                 contender (member_fold_caliber=interval_union, e.g.
//                 "IO延迟 ×8 合计11.506ms");
//   chain lane  — buildCriticalBlockingCallsFromStats mints one
//                 critical_blocking candidate per THE SAME stats.IOLatencies
//                 entry (peer = the completer thread, e.g. udk-irq-*).
//
// Both lanes then rendered side by side under one causal-tree parent as
// duplicate child rows (opendir_79: family E3 raw-sum 2.858ms ↔ peer rows
// E6-E9 sum 2.858ms, strictly equal; huadong_79: ×8 15.156ms ↔ five peer
// rows). All three existing fold generations miss the shape by design: the
// display same-segment two-lane fold requires an inversion candidate,
// critical_blocking is not in causalTokenFamilyFoldLanes, and the NEW-3 IO
// caliber token set excludes io_latency.
//
// ── Ruling shape (§27.2 修向, engine half) ──────────────────────────────────
//
// Reconcile the member sets across lanes on the PRECISE identity
// (thread, adjudicated type family, query window, interval ∈ family member
// interval union) and stamp a TYPED absorption marker on the critical row —
// the observations keep publishing unchanged (观测照发不删: evidence index /
// system supplement / audit tokens stay lossless); only the display RENDER
// seat folds (the display half consumes the marker).
//
// Per-dimension precision argument (精确信号才可硬门 red line):
//   thread    — typed ThreadRef equality via threadKey (pid-first; the exact
//               key the family fold itself groups on). Never a label spelling
//               heuristic.
//   type      — exact wire-token equality through the adjudicated
//               criticalBlockingRankFamilyReconTypes map (io_latency ↔
//               io_latency ONLY this ruling; other same-source pairs such as
//               d_state_or_io_wait↔block_io lack a witness and stay OUT —
//               根因修复不过度泛化, 立账观察). Never a substring/lane
//               heuristic.
//   window    — exact typed endpoint equality between the blocking result's
//               own query window and the family row's StatsWindow identity
//               (both copied from the same normalized Query in every
//               coexistence shape; zero==zero is the honest "both unbounded
//               over the same trace" arm, not a guessed window). 跨窗不对账.
//   interval  — the critical row's typed [StartTs,EndTs] must lie INSIDE the
//               family's member interval UNION (familyMemberIntervals — the
//               validated member inventory, never the merged hull whose gaps
//               would admit non-members). Both lanes copy the SAME
//               IssueTs/CompleteTs floats from one stats entry, so containment
//               is exact float arithmetic — no tolerance, and any mismatch
//               fails OPEN to the current two-row render (honest duplicate
//               beats wrong absorption).
//
// Deliberately NOT gate dimensions (noisy — 嘈声维禁作硬门): chain-relevance
// lane words (two independent derivations that may disagree), label
// spellings, value similarity, Summary prose.

import (
	"fmt"
	"strings"
)

// criticalBlockingRankFamilyReconTypes is the adjudicated cross-lane type
// mapping: critical_blocking wire token → rank-lane family tokens. §27.2-G1
// scoped the original ruling to the exact io_latency↔io_latency pair (both
// lanes mint from the SAME stats.IOLatencies entries, so member identity is
// provable); every other same-source pair MUST NOT be added without a new
// adjudication — the universe pin (TestG1ReconTypeUniversePinned) forces
// that conversation, §28.8 教训⑥ 机械化.
//
// EVOLUTION RECORD (CASE-1, §29.52 独立立案 → v5 P1 批, 2026-07-13): the
// universe grew its SECOND adjudicated pair set — {d_state_or_io_wait,
// io_wait}². Both lanes mint from the SAME stats.DStateTop/IOWaitTop
// per-(thread,cpu) ledger groups (rank lane: rootCauseDIOStateFamilyItems,
// one family contender per thread; chain lane:
// buildCriticalBlockingCallsFromStats, one candidate per group), so member
// identity is provable by same-source floats — the row-level witnesses the
// original ruling waited for ("stay OUT · 立账观察") arrived as SMR-S7 +
// S4-TPF + the 42729 E9↔E15 critical↔critical pair (reports
// 31693/42729/45903, smr_audit_report_20260712 CASE-1). The family token
// flips to "io_wait" when dStateMs==0 (the query.go io_wait arm — CASE-1
// gap (c)), so all four token combinations are adjudicated together.
// Membership for these pairs is the SAME-SOURCE identity ONLY (see
// criticalBlockingRankFamilyReconSameSourceOnly) — G1 明拒 hull containment
// on per-(thread,cpu) group intervals. Every OTHER same-source pair
// (…↔block_io_by_inode etc.) still lacks a witness and stays OUT.
var criticalBlockingRankFamilyReconTypes = map[string][]string{
	"io_latency":         {"io_latency"},
	"d_state_or_io_wait": {"d_state_or_io_wait", "io_wait"},
	"io_wait":            {"d_state_or_io_wait", "io_wait"},
}

// criticalBlockingRankFamilyReconSameSourceOnly reports whether the critical
// type's membership arm is the CASE-1 same-source identity ONLY: exact float
// interval equality AND exact float value equality against the family's
// validated member inventory (both lanes copy the same ThreadDuration
// floats). The D/IO ledger groups are per-(thread,cpu) segment HULLS (the
// 42729 witness held 10 discontinuous segments in one io_wait cpu=3 group)
// — union containment would admit non-members through hull gaps, exactly
// the noisy-membership shape G1 rejects; the io_latency pair keeps its
// original two-pass membership (exact member, then union containment over
// true per-IO intervals) byte-identically.
func criticalBlockingRankFamilyReconSameSourceOnly(criticalType string) bool {
	return criticalType != "io_latency"
}

// criticalBlockingReconInterval returns the typed interval identity the
// reconciliation proves membership with: io_latency rows publish their true
// per-IO [StartTs,EndTs] (the original §27.2-G1 wire); the D/IO rows carry
// their group HULL engine-internally only (reconStartTs/reconEndTs — CASE-1
// gap (a), 修复轮 h1 ∿ 回归: the hull must never reach the published wire).
func criticalBlockingReconInterval(item CriticalBlockingCandidate) (float64, float64) {
	if item.StartTs > 0 && item.EndTs > item.StartTs {
		return item.StartTs, item.EndTs
	}
	return item.reconStartTs, item.reconEndTs
}

// rankFamilyReconKey renders the canonical G1 family identity BOTH sides
// carry (family row RankFamilyKey == absorbed row AbsorbedIntoFamily). One
// renderer, engine-side only — the display layer joins the two markers by
// verbatim string equality and never re-derives the key from labels.
//
// 收尾 P2-a (对抗复核, 2026-07-09): the key carries the CHAIN-LANE dimension
// (rootCauseFamilyFoldLaneKey — the SAME 道别 helper the family fold key
// uses, 同构), because the family fold splits same-(thread,type,window) rows
// into per-lane families (an io row's lane can flip on its CompleteThread's
// chain membership): a lane-less key made two lane-split families SHARE one
// key, so the display's first-rendered family row swallowed BOTH families'
// peers (count over-report) while the second family's note starved. With the
// lane aboard the key is UNIQUE per family BY CONSTRUCTION — it is
// isomorphic to the family fold's own merge key (type, thread, lane, window)
// and the fold merges any two rows agreeing on all four, so two distinct
// family rows can never share a key. Single mint point (the reconcile match
// below) keeps both sides consistent automatically.
func rankFamilyReconKey(typ string, thread ThreadRef, lane string, window TimeWindow) string {
	return fmt.Sprintf("%s|%s|%s|%.6f..%.6f", strings.TrimSpace(typ), threadKey(thread), strings.TrimSpace(lane), window.StartTs, window.EndTs)
}

// rankFamilyReconEligibleFamily reports whether a rank item is a G1
// reconciliation family row for rankType: an ENGINE family merge
// (MemberCount ≥ 2 — the precise foldSameThreadTypeRankFamilies signature;
// a single rank row is NOT a family row and never absorbs, §27.2-G1 scope)
// with a validated member interval inventory to test membership against.
func rankFamilyReconEligibleFamily(item RootCauseRankItem, rankType string) bool {
	return item.Type == rankType && item.MemberCount >= 2 && len(item.familyMemberIntervals) > 0
}

// rankFamilyReconIntervalInsideUnion reports whether [start,end] lies inside
// the union of the member intervals. Exact float containment: both lanes copy
// the same source floats, so a true member matches exactly; any drift fails
// open (no absorption). Implementation walks the merged (sorted, coalesced)
// union — the same algebra foldIntervalUnionMs uses.
func rankFamilyReconIntervalInsideUnion(start, end float64, members []foldInterval) bool {
	if end <= start || len(members) == 0 {
		return false
	}
	merged := foldIntervalUnion(members)
	for _, iv := range merged {
		if start >= iv.start && end <= iv.end {
			return true
		}
	}
	return false
}

// foldIntervalUnion returns the sorted, coalesced union of the intervals
// (shared algebra with foldIntervalUnionMs; length-only callers keep using
// that one).
func foldIntervalUnion(intervals []foldInterval) []foldInterval {
	if len(intervals) == 0 {
		return nil
	}
	sorted := make([]foldInterval, len(intervals))
	copy(sorted, intervals)
	sortFoldIntervals(sorted)
	out := make([]foldInterval, 0, len(sorted))
	cur := sorted[0]
	for _, iv := range sorted[1:] {
		if iv.start <= cur.end {
			if iv.end > cur.end {
				cur.end = iv.end
			}
			continue
		}
		out = append(out, cur)
		cur = iv
	}
	return append(out, cur)
}

func sortFoldIntervals(intervals []foldInterval) {
	// Insertion sort keeps this dependency-free; member counts are tiny (≤
	// the family fold's own member counts).
	for i := 1; i < len(intervals); i++ {
		for j := i; j > 0; j-- {
			if intervals[j].start < intervals[j-1].start ||
				(intervals[j].start == intervals[j-1].start && intervals[j].end < intervals[j-1].end) {
				intervals[j], intervals[j-1] = intervals[j-1], intervals[j]
			} else {
				break
			}
		}
	}
}

// reconcileCriticalBlockingWithRankFamilies stamps the G1 typed absorption
// markers across the two lanes of ONE result. Idempotent by reset-first
// recomputation (RunQuery and BuildFrameRootCauseBundle may both run it over
// the same backing slices — the marks converge, counts never double).
// Mutates items in place; row order, values, membership and every published
// account are untouched — this pass ONLY writes the four marker fields.
func reconcileCriticalBlockingWithRankFamilies(rank *RootCauseRankResult, blocking *CriticalBlockingResult) {
	if rank == nil || blocking == nil {
		return
	}
	// Reset-first idempotency: clear every G1 marker before recomputing.
	for i := range rank.Items {
		// B4 reuses the generic projection join key on an exact cross-type
		// rank absorber. AbsorbedRankRows is its ownership marker; G1 must not
		// erase that independent lane while resetting its own chain count.
		if rank.Items[i].AbsorbedRankRows == 0 {
			rank.Items[i].RankFamilyKey = ""
		}
		rank.Items[i].AbsorbedChainRows = 0
	}
	for i := range blocking.Items {
		blocking.Items[i].AbsorbedByRankFamily = false
		blocking.Items[i].AbsorbedIntoFamily = ""
	}
	for i := range blocking.Items {
		item := &blocking.Items[i]
		famTypes, adjudicated := criticalBlockingRankFamilyReconTypes[item.Type]
		if !adjudicated {
			continue // exact-token universe only — never a substring/lane guess
		}
		reconStart, reconEnd := criticalBlockingReconInterval(*item)
		if reconStart <= 0 || reconEnd <= reconStart {
			continue // no typed interval identity → membership unprovable → keep both rows
		}
		// CASE-1 membership arm split: the D/IO pairs prove membership by
		// same-source identity ONLY (single pass) — no union containment.
		sameSourceOnly := criticalBlockingRankFamilyReconSameSourceOnly(item.Type)
		passes := 2
		if sameSourceOnly {
			passes = 1
		}
		// Deterministic family election: first pass prefers a family holding a
		// member interval EXACTLY equal to this row's interval (both lanes copy
		// the same floats from one stats entry — the strongest membership
		// witness; a lane-split pair of families is disambiguated HERE, since
		// each source event's rank twin folded into exactly one lane's family);
		// the union-containment pass covers members whose validated inventory
		// coalesced across overlaps. First match in rank order wins — 收尾 P3-v
		// (对抗复核, 2026-07-09) honest scope note: reachable ambiguity is
		// pass-1 only (one interval inside TWO families' unions without exact
		// membership in either — requires float drift between the lanes'
		// copies of one stats entry, not observed in production), and the
		// deterministic first-match then absorbs into a same-(thread,type,
		// window) family either way.
		matched := -1
		for pass := 0; pass < passes && matched < 0; pass++ {
			for j := range rank.Items {
				fam := &rank.Items[j]
				eligible := false
				for _, famType := range famTypes {
					if rankFamilyReconEligibleFamily(*fam, famType) {
						eligible = true
						break
					}
				}
				if !eligible {
					continue
				}
				if threadKey(fam.Thread) != threadKey(item.Thread) {
					// 同线程 hard gate — the EXACT typed key the family fold
					// itself grouped on (pid-first), so cross-lane membership
					// uses the same identity ruler as in-lane membership.
					continue
				}
				// 跨窗不对账: the family's typed StatsWindow identity must equal
				// the blocking result's own query window exactly (both copied
				// from one normalized Query in every coexistence shape;
				// zero==zero = both unbounded over the same trace).
				if fam.StatsWindowStartTs != blocking.Window.StartTs || fam.StatsWindowEndTs != blocking.Window.EndTs {
					continue
				}
				if pass == 0 {
					exact := false
					for _, iv := range fam.familyMemberIntervals {
						if iv.start != reconStart || iv.end != reconEnd {
							continue
						}
						// CASE-1 µs value-identity dimension (same-source
						// pairs only): both lanes copy one ThreadDuration's
						// DurationMs, so a true member matches EXACTLY; a
						// valueless inventory entry (legacy/foreign) is
						// unprovable and fails open. io_latency keeps the
						// original interval-only exact arm byte-identically.
						if sameSourceOnly && (iv.valueMs <= 0 || iv.valueMs != item.DurationMs) {
							continue
						}
						exact = true
						break
					}
					if !exact {
						continue
					}
				} else if !rankFamilyReconIntervalInsideUnion(reconStart, reconEnd, fam.familyMemberIntervals) {
					continue
				}
				matched = j
				break
			}
		}
		if matched < 0 {
			continue // 负向保护: no family row → the peer row renders as before
		}
		fam := &rank.Items[matched]
		// P2-a: the key's lane dimension comes from the MATCHED family's own
		// 道别 (the same helper its fold key used) — single mint point, both
		// sides consistent by construction. The type dimension is the MATCHED
		// family's own token (== the critical token on the io_latency pair;
		// the CASE-1 pairs join cross-token, e.g. io_wait chain rows into a
		// mixed d_state_or_io_wait family).
		key := rankFamilyReconKey(fam.Type, fam.Thread, rootCauseFamilyFoldLaneKey(*fam),
			TimeWindow{StartTs: fam.StatsWindowStartTs, EndTs: fam.StatsWindowEndTs})
		fam.RankFamilyKey = key
		fam.AbsorbedChainRows++
		item.AbsorbedByRankFamily = true
		item.AbsorbedIntoFamily = key
	}
}

package tool

// answer_document_mutation_runtime_rcm.go — RCM-2 display half (ledger
// docs/design/real_trace_campaign_20260705.md §24.7.1①/§24.10/§24.12 维度A
// 施工图/§24.22 F6, 2026-07-08): the family-merge contenders the RCM engine
// half mints (node.FamilyMember* + Inode/Dev, ISOLATED from the display-side
// Merged* fold lane) get their display shape —
//
//   D1  the FIFTH caliber word 合计(共N段,同线程) (closed-set extension;
//       max_overlap_fallback speaks 成员最大(共N段,重叠未拆), count_sum keeps
//       the counting-semantics word 计数合计(共N项,同线程)); the legacy
//       累计(跨线程) fallback word is BANNED on family rows (F6: a same-thread
//       total mislabeled cross-thread during the merge-batch window);
//   D2  the four-line grammar family form (行1 类型词+×N+合计 value prefix;
//       行2 类别·根因排序#N|背景榜位#N·置信; 行3 有效归因 V = 合计(共N段,
//       同线程) with the identity pin V == 发布值; 子行 = roster top-3 +
//       counted 其余 K 见明细 trailer — §24.7.1① roster 折叠必带计数披露);
//   D3  the three lossless faces (key-metric table ×N token, detail-block
//       family stanza, comparison 确定性优化点 family cell) plus the
//       runtimeTraceProjLeadSelectionValue typed family lane (参赛值=发布值 —
//       NEVER the MergedCount/MergedMaxMS member-MAX discount lane, §24.12
//       dim-A mandate ①, pinned negative);
//   D4  evidence-index member_count/member_fold_caliber audit tokens.
//
// Every word below is a SINGLE-SOURCE helper — the tree 行1/行3, the C00
// fallback slot, the (a) table token, the (b) block stanza and the comparison
// cell all read these functions, so the five faces can never drift.

import (
	"fmt"
	"math"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeTraceProjFamilyRow is THE family gate (§24.7.1 ②): precise typed
// signal only — the engine wrote member_count>1 onto the isolated
// FamilyMember* lane. Never inferred from names, rosters or values.
func runtimeTraceProjFamilyRow(node types.TraceCausalProjectionNode) bool {
	return node.FamilyMemberCount > 1
}

// runtimeTraceProjFamilyCaliberWord maps the engine's typed fold-caliber
// ladder (§24.22 M2 口径梯四臂) onto the display caliber word + its legend
// mark. ok=false on an unknown caliber token — the display then makes NO
// caliber claim (fail-open honesty; it still never falls back to the banned
// 累计(跨线程) word — see runtimeTraceProjRowFallbackCaliberWord).
func runtimeTraceProjFamilyCaliberWord(node types.TraceCausalProjectionNode, zh bool) (string, runtimeTraceProjMark, bool) {
	n := node.FamilyMemberCount
	switch strings.TrimSpace(node.FamilyFoldCaliber) {
	case tracequery.RootCauseMemberFoldCaliberSumDisjoint, tracequery.RootCauseMemberFoldCaliberIntervalUnion:
		// The FIFTH closed-set caliber word (§24.12 维度A ③): same-thread
		// wall-clock segment sum, overlapping segments as their interval union
		// (disjoint == Σ; union < Σ discloses via the sum suffix below).
		if zh {
			return fmt.Sprintf("合计(共%d段,同线程)", n), runtimeTraceProjMarkFamilyTotal, true
		}
		return fmt.Sprintf("total (%d segments, same thread)", n), runtimeTraceProjMarkFamilyTotal, true
	case tracequery.RootCauseMemberFoldCaliberMaxOverlapFallback:
		// Overlap without a usable union deduction: the published value is the
		// member MAX — an honest lower bound, never a Σ (§24.22 M2 fourth arm).
		if zh {
			return fmt.Sprintf("成员最大(共%d段,重叠未拆)", n), runtimeTraceProjMarkFamilyMemberMax, true
		}
		return fmt.Sprintf("member max (%d segments, overlap not deducted)", n), runtimeTraceProjMarkFamilyMemberMax, true
	case tracequery.RootCauseMemberFoldCaliberCountSum:
		// F-7 (冷读, 2026-07-12): the off-chain window cap (engine
		// backgroundImpactMs, 0.35×窗) can clamp the published seat BELOW the
		// member Σ — donghu E29: Σ119.100 → 81.616, single member 84.300 >
		// seat. A clamped seat must not claim the「=计数合计」identity (口径
		// 区承诺"分量计入之和恒等于 V"): the honest word names the truncation;
		// the Σ stays on the 原始和 comparison face. Typed check: published
		// value vs FamilyMemberSumMS beyond the %.3f print tolerance.
		if runtimeTraceProjFamilyCountSumClamped(node) {
			if zh {
				return fmt.Sprintf("计数当量(超上限截断;共%d项,同线程)", n), runtimeTraceProjMarkFamilyCountEquivalent, true
			}
			return fmt.Sprintf("count equivalent (capped; %d items, same thread)", n), runtimeTraceProjMarkFamilyCountEquivalent, true
		}
		// Count-class members keep the counting-semantics word (D1: count_sum
		// 形沿用计数语义词) — counts add regardless of interval overlap.
		if zh {
			return fmt.Sprintf("计数合计(共%d项,同线程)", n), runtimeTraceProjMarkFamilyCountSum, true
		}
		return fmt.Sprintf("count total (%d items, same thread)", n), runtimeTraceProjMarkFamilyCountSum, true
	}
	return "", 0, false
}

// runtimeTraceProjFamilyCountSumClamped — F-7 (冷读 CAL-1 修复轮,
// 2026-07-12): whether a count-sum family seat's PUBLISHED value diverged
// from its member Σ (the engine's off-chain 0.35×window cap is the known
// producer). Exact typed comparison; ok only when the Σ was published.
func runtimeTraceProjFamilyCountSumClamped(node types.TraceCausalProjectionNode) bool {
	if strings.TrimSpace(node.FamilyFoldCaliber) != tracequery.RootCauseMemberFoldCaliberCountSum {
		return false
	}
	if node.FamilyMemberSumMS <= 0 {
		return false
	}
	return math.Abs(runtimeTraceProjFamilyPublishedMS(node)-node.FamilyMemberSumMS) > 0.0005
}

// runtimeTraceProjMarkFamilyCaliber records a family caliber-word mark at a
// render site, plus the 计数当量 companion entry on the count arm (DISP-2 /
// GAP-A P3-6, 2026-07-09): the count family's engine faces (member roster /
// raw-Σ detail note / Summary) print the 计数当量X(非墙钟) marker wherever
// the count caliber word renders, so its teaching entry rides exactly those
// renders. ALL family caliber-word mark sites MUST route through this helper
// — never a bare marks.mark(caliberMark) — so the companion can never drift
// off one face.
func runtimeTraceProjMarkFamilyCaliber(marks *runtimeTraceProjMarkSet, caliberMark runtimeTraceProjMark) {
	marks.mark(caliberMark)
	if caliberMark == runtimeTraceProjMarkFamilyCountSum {
		marks.mark(runtimeTraceProjMarkFamilyCountEquivalent)
	}
}

// runtimeTraceProjCountEquivalentValueText is the ONE display form of a
// count-equivalent magnitude rendered WITH its value (§29.55 观察③ 两形一裁,
// WF-2 词面批 2026-07-14): 计数当量<value>(非墙钟) — the value is not
// wall-clock milliseconds, so it never wears an ms suffix (带 ms=口径谎;
// the G3/DISP-2 bare-ms discipline's wordface half). The sidebar form won
// the ruling; the tree 行1 form 计数当量Xms is retired. Mirrors the engine's
// rootCauseCountEquivalentValue byte-for-byte on the zh face (三面同源).
// 双复核修复 件13 (冷读 CR11 定稿空格, 2026-07-21): 计数当量 and the value
// separate with one space (计数当量 11.100); the engine mint co-moves.
func runtimeTraceProjCountEquivalentValueText(v float64, zh bool) string {
	if zh {
		return fmt.Sprintf("计数当量 %.3f(非墙钟)", v)
	}
	return fmt.Sprintf("count-equivalent %.3f (not wall clock)", v)
}

// runtimeTraceProjCompositeScoreValueText is the ONE display form of a
// composite-score magnitude rendered WITH its value (QH2-A 件2 站①, §29.55
// 观察③ 族裁延伸 2026-07-14): <value>(综合评分,非墙钟) — the value is a
// score over mixed units (block_io_by_inode: max latencies + MiB), not
// wall-clock milliseconds, so it never wears an ms suffix. Extracted from
// the tree 行1 mint (微词面① 2026-07-12) so the roster/树行1 wording stays
// the single source; the 关键指标表 value cells consume the same form.
func runtimeTraceProjCompositeScoreValueText(v float64, zh bool) string {
	if zh {
		return fmt.Sprintf("%.3f(综合评分,非墙钟)", v)
	}
	return fmt.Sprintf("%.3f (composite score, not wall clock)", v)
}

// runtimeTraceProjCompositeValueText keeps io_pressure's authoritative
// activity index distinct from generic composite scores. The value is
// background context only: the wording deliberately carries neither a window
// projection nor a chain-cumulative implication.
func runtimeTraceProjCompositeValueText(node types.TraceCausalProjectionNode, fallback float64, zh bool) string {
	if (runtimeTraceCausalProjectionCanonicalNode(node.TypeToken) == "io_pressure" ||
		runtimeTraceCausalProjectionCanonicalNode(node.Object) == "io_pressure") &&
		node.IOPressureActivityIndex > 0 {
		if zh {
			return fmt.Sprintf("%.3f(IO活动综合指数,非墙钟)", node.IOPressureActivityIndex)
		}
		return fmt.Sprintf("%.3f (IO activity index, not wall clock)", node.IOPressureActivityIndex)
	}
	return runtimeTraceProjCompositeScoreValueText(fallback, zh)
}

// runtimeTraceProjCompositeValueCaliber is the display-only authority for a
// node whose published magnitude is a composite score rather than wall-clock
// milliseconds. RANKDIS-M18 moved io_pressure onto the typed
// ObservationRecord.Unit=composite_score wire without moving it into the
// caliber-side seat/tier class (排序零动); report faces must therefore consume
// the carried Unit instead of reconstructing value caliber from board
// membership. The registry fallback preserves legacy block_io_by_inode
// projections whose persisted records predate the typed Unit. No rank, tier,
// channel, score or fold gate reads this helper.
func runtimeTraceProjCompositeValueCaliber(node types.TraceCausalProjectionNode) bool {
	if strings.TrimSpace(node.Unit) == types.TraceObservationUnitCompositeScore {
		return true
	}
	return tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)) == tracequery.CausalCaliberSideCompositeScore &&
		(node.IsCaliberSideRow() || !runtimeTraceProjFamilyRow(node))
}

// runtimeTraceProjCountEquivalentValueCaliber is the display-only authority
// for a node whose published magnitude renders in the count-equivalent
// 计数当量X(非墙钟) form (CALSIDE-1 件2 = SPANVIS-1 冷读官 F7, §29.147,
// 2026-07-19): the SHARED registry count class
// (tracequery.CausalTokenCaliberSideClass — the same arm the tree 行1 / ◎
// footnote / detail-table value forms already read) or the clamped count-sum
// family seat (the 计数当量 stem's other mint). Exactly the mints of the
// count-equivalent value form — no rank, tier, channel, score or fold gate
// reads this helper (value/bar/% word faces only).
func runtimeTraceProjCountEquivalentValueCaliber(node types.TraceCausalProjectionNode) bool {
	if tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)) == tracequery.CausalCaliberSideCount {
		return true
	}
	return runtimeTraceProjFamilyCountSumClamped(node)
}

// runtimeTraceProjNonWallClockValueCaliber — CALSIDE-1 件2 (F7 假单位修,
// §29.147 独立立案, 2026-07-19): whether the row's published value renders in
// one of the two NON-wall-clock forms (计数当量X(非墙钟) /
// X(综合评分,非墙钟)). Such a value must never be pooled against wall-clock
// denominators: the row draws NO wall-clock bar (the bar's full scale is a
// wall-clock ruler — the CMP-3 cross-thread precedent) and publishes NO
// window-share % (a non-wall-clock numerator over a wall-clock window is a
// cross-unit fake). Display-only; the value channel itself is untouched.
func runtimeTraceProjNonWallClockValueCaliber(node types.TraceCausalProjectionNode) bool {
	return runtimeTraceProjCompositeValueCaliber(node) ||
		runtimeTraceProjCountEquivalentValueCaliber(node)
}

// runtimeTraceProjFamilyValuePrefix is the COMPACT 行1 value qualifier
// (witness form 「合计7.124ms」): the fifth word's stem rides directly on the
// main-line duration so the merged magnitude is identifiable at the point of
// reading; the full parenthesized word lives on 行3 / the C00 slot / the
// detail stanza. "" when the caliber is unknown (no claim, never a guess).
func runtimeTraceProjFamilyValuePrefix(node types.TraceCausalProjectionNode, zh bool) string {
	if !runtimeTraceProjFamilyRow(node) {
		return ""
	}
	switch strings.TrimSpace(node.FamilyFoldCaliber) {
	case tracequery.RootCauseMemberFoldCaliberSumDisjoint, tracequery.RootCauseMemberFoldCaliberIntervalUnion,
		tracequery.RootCauseMemberFoldCaliberCountSum:
		// F-7: a clamped count seat must not claim the sum stem on 行1
		// either — the established 计数当量 marker word carries no identity.
		if runtimeTraceProjFamilyCountSumClamped(node) {
			if zh {
				return "计数当量"
			}
			return "count equivalent "
		}
		if zh {
			return "合计"
		}
		return "total "
	case tracequery.RootCauseMemberFoldCaliberMaxOverlapFallback:
		if zh {
			return "成员最大"
		}
		return "member max "
	}
	return ""
}

// runtimeTraceProjFamilyPublishedMS is the family's PUBLISHED participation
// value (§24.10 合并量参赛; D3 参赛值=发布值): the engine-published effective
// attribution when present, else the row's typed on-chain intersection, else
// the row's display impact. Never a display-side recomputation — the engine's
// fold is the single value authority.
//
// EVOLUTION RECORD (审计 #5/#62, §29.25 处置委托 + §29.26 待主会话落账,
// 2026-07-10): the on-chain semantic intersection arm was inserted between
// the effective arm and the display fallback — an unfolded on-chain semantic
// row (fold fail-open remnant) used to fall back to its display value = the
// COMPLETE member union, publishing the union as 有效归因/lead value while
// the engine's participation is the exact member∩chain intersection (✦ 行
// 「有效归因」标签不得回退 union 裸值). Folded rows already carry the same
// intersection on EffectiveImpactMS — byte-identical for them.
func runtimeTraceProjFamilyPublishedMS(node types.TraceCausalProjectionNode) float64 {
	if node.IsSemanticRelationOnly() {
		return 0
	}
	if node.EffectiveImpactMS > 0 {
		return node.EffectiveImpactMS
	}
	if v := runtimeTraceProjSemanticChainIntersectionMS(node); v > 0 {
		return v
	}
	return runtimeTraceProjNodeDisplayImpact(node)
}

// runtimeTraceProjSemanticChainIntersectionMS returns the typed on-chain
// semantic intersection participation (审计 #5/#62; see
// TraceCausalProjectionNode.SemanticChainProjectedMS). 0 = no typed
// intersection — every consumer fails open to its legacy lane. The
// SemanticClass gate is defensive: the compile only mints the field on
// trace_semantic_span records, which always carry the class token.
func runtimeTraceProjSemanticChainIntersectionMS(node types.TraceCausalProjectionNode) float64 {
	// B829: the host-edge pre-span value is raw occupancy bounded by a
	// relation credential, not a semantic-completion intersection.  This
	// typed basis outranks legacy projected_impact carriers so every display
	// face fails closed to zero effective attribution while preserving raw
	// Impact/Cumulative values.
	if node.IsSemanticRelationOnly() {
		return 0
	}
	if strings.TrimSpace(node.SemanticClass) == "" {
		return 0
	}
	return node.SemanticChainProjectedMS
}

// runtimeTraceProjSemanticChainDualCaliber reports whether the ✦ row must
// speak the DUAL-CALIBER form (审计 #62 ①, §29.25 处置委托 + §29.26 待主会话
// 落账): the published participation is the exact member∩chain intersection
// AND it differs from the row's window-projection union at print precision.
// Full-overlap rows (intersection == union, e.g. the §29.22 textup 102.172
// witness) stay byte-identical on the legacy single-caliber form.
func runtimeTraceProjSemanticChainDualCaliber(node types.TraceCausalProjectionNode) (intersection float64, ok bool) {
	intersection = runtimeTraceProjSemanticChainIntersectionMS(node)
	if intersection <= 0 {
		return 0, false
	}
	union := runtimeTraceProjNodeDisplayImpact(node)
	if union <= 0 || runtimeTraceProjRound3Equal(intersection, union) {
		return 0, false
	}
	return intersection, true
}

// runtimeTraceProjSemanticChainIntersectionWord is the dual-caliber 行3 word
// (assembled from the existing closed-set vocabulary only: 链上 / 计入 /
// (共N段,同线程) — never a coined term): the participation counts ONLY the
// member∩same-thread-chain-window intersection.
func runtimeTraceProjSemanticChainIntersectionWord(node types.TraceCausalProjectionNode, zh bool) string {
	n := node.FamilyMemberCount
	if n <= 1 {
		if zh {
			return "链上计入"
		}
		return "on-chain counted"
	}
	if zh {
		return fmt.Sprintf("链上计入(共%d段,同线程)", n)
	}
	return fmt.Sprintf("on-chain counted (%d segments, same thread)", n)
}

// runtimeTraceProjSemanticChainUnionDisclosure is the dual-caliber union
// disclosure suffix beside the 行3 equation (窗口投影合计 = the complete
// selected-window member union, the §24.10 lossless caliber; existing
// closed-set tokens 窗口投影 / 合计 / 见明细). The stanza variant swaps the
// pointer for the detail block's own 供对照 convention.
func runtimeTraceProjSemanticChainUnionDisclosure(node types.TraceCausalProjectionNode, zh, stanza bool) string {
	union := runtimeTraceProjNodeDisplayImpact(node)
	if union <= 0 {
		return ""
	}
	if zh {
		if stanza {
			return fmt.Sprintf("(窗口投影合计 %.3fms 供对照)", union)
		}
		return fmt.Sprintf("(窗口投影合计 %.3fms 见明细)", union)
	}
	if stanza {
		return fmt.Sprintf(" (complete window-projection total %.3fms for cross-checking)", union)
	}
	return fmt.Sprintf(" (complete window-projection total %.3fms in the detail blocks)", union)
}

// runtimeTraceProjFamilySumDisclosure renders the lossless raw-Σ suffix (D1):
// a union row whose published value sits below the member Σ appends
// 「(重叠段已并,原始和 X ms 见明细)」; the max-fallback form appends the bare
// Σ pointer. "" when the engine disclosed no sum (published == Σ, or count
// caliber).
func runtimeTraceProjFamilySumDisclosure(node types.TraceCausalProjectionNode, zh bool) string {
	if !runtimeTraceProjFamilyRow(node) || node.FamilyMemberSumMS <= 0 {
		return ""
	}
	switch strings.TrimSpace(node.FamilyFoldCaliber) {
	case tracequery.RootCauseMemberFoldCaliberIntervalUnion:
		if zh {
			return fmt.Sprintf("(重叠段已并,原始和 %.3fms 见明细)", node.FamilyMemberSumMS)
		}
		return fmt.Sprintf(" (overlapping segments deduplicated; raw sum %.3fms in the detail blocks)", node.FamilyMemberSumMS)
	case tracequery.RootCauseMemberFoldCaliberMaxOverlapFallback:
		if zh {
			return fmt.Sprintf("(原始和 %.3fms 见明细)", node.FamilyMemberSumMS)
		}
		return fmt.Sprintf(" (raw sum %.3fms in the detail blocks)", node.FamilyMemberSumMS)
	}
	return ""
}

// runtimeTraceProjFamilySumDetailNote is the DETAIL-STANZA raw-Σ note (复核
// F-2, 2026-07-08 — the stanza's hand-written second implementation drifted
// into a self-contradiction: the max_overlap_fallback arm printed 「重叠未拆」
// and 「重叠段已并」 on ONE line). Single caliber fork beside the fence
// disclosure above: union → the deduplication clause; max fallback (and any
// other Σ-bearing caliber) → the bare Σ pointer — its caliber word already
// states 重叠未拆, so a deduplicated claim would contradict it. "" when the
// engine disclosed no sum.
func runtimeTraceProjFamilySumDetailNote(node types.TraceCausalProjectionNode, zh bool) string {
	if !runtimeTraceProjFamilyRow(node) || node.FamilyMemberSumMS <= 0 {
		return ""
	}
	if strings.TrimSpace(node.FamilyFoldCaliber) == tracequery.RootCauseMemberFoldCaliberIntervalUnion {
		if zh {
			return fmt.Sprintf(";原始和 %.3fms 供对照(重叠段已并)", node.FamilyMemberSumMS)
		}
		return fmt.Sprintf("; raw sum %.3fms for cross-checking (overlapping segments deduplicated)", node.FamilyMemberSumMS)
	}
	// 跨批 X2 (GAP-B 收尾, 2026-07-09): a COUNT-class family's Σ is a
	// count-derived advisory scalar, not wall clock — GAP-A's G3 makes the
	// count arm always publish MemberSumMs, and the former fallback arm below
	// printed it in the bare wall-clock form ("原始和 198.300ms 供对照"),
	// contradicting the fence face (which deliberately excludes count) and
	// the engine's roster/Summary faces. The 计数当量 marker mirrors the
	// engine's ONE helper wording (rootCauseCountEquivalentValue) — 三面同源.
	// §29.55 观察③ 两形一裁 (2026-07-14): the value speaks the suffix-free
	// atom 计数当量X(非墙钟); the trailing paren keeps only the class word
	// (非墙钟 now rides the value, never said twice).
	if strings.TrimSpace(node.FamilyFoldCaliber) == tracequery.RootCauseMemberFoldCaliberCountSum {
		if zh {
			return fmt.Sprintf(";原始和 %s 供对照(计数类)", runtimeTraceProjCountEquivalentValueText(node.FamilyMemberSumMS, true))
		}
		return fmt.Sprintf("; raw sum %s for cross-checking (count-class)", runtimeTraceProjCountEquivalentValueText(node.FamilyMemberSumMS, false))
	}
	if zh {
		return fmt.Sprintf(";原始和 %.3fms 供对照", node.FamilyMemberSumMS)
	}
	return fmt.Sprintf("; raw sum %.3fms for cross-checking", node.FamilyMemberSumMS)
}

// runtimeTraceProjFamilyRosterSubRows renders the 行4+ member roster (D2):
// top-3 verbatim engine roster entries (largest first by producer contract —
// the real distinguishing keys: inode/dev/span names, §24.7.1 ① 不能丢), then
// a COUNTED fold trailer (roster 折叠必带计数披露 red line; the full roster
// lives on the detail stanza). The trailer counts against FamilyMemberCount —
// the wire roster itself may already be a bounded excerpt.
func runtimeTraceProjFamilyRosterSubRows(node types.TraceCausalProjectionNode, zh bool) []string {
	roster := node.FamilyMemberRoster
	if len(roster) == 0 {
		return nil
	}
	listed := len(roster)
	if listed > 3 {
		listed = 3
	}
	out := make([]string, 0, listed+1)
	// RSPA (§29.61.10, 2026-07-14): a re-anchored half publishes only its own
	// side of the 同源二分 while the engine roster keeps the FULL-window
	// member inventory (both halves share it) — the member rows say so, so
	// the roster can never read as summing to the row's published value (the
	// split itself is disclosed by the row's 同源二分 line).
	memberWord := runtimeTraceProjFamilyMemberWord(node, zh) + " "
	for _, entry := range roster[:listed] {
		out = append(out, memberWord+entry)
	}
	if rest := node.FamilyMemberCount - listed; rest > 0 {
		if zh {
			out = append(out, fmt.Sprintf("其余 %d 项见明细(成员共%d,列%d)", rest, node.FamilyMemberCount, listed))
		} else {
			out = append(out, fmt.Sprintf("%d more in the detail blocks (%d members, %d listed)", rest, node.FamilyMemberCount, listed))
		}
	}
	return out
}

// runtimeTraceProjFamilySpanTopCap is the customer-named constituent sub-row
// cap (SPANTOP-1, §29.131, user ruling 2026-07-18): a semantic family seat
// lists at most this many member spans as 行4+ constituent sub-rows; the rest
// fold into ONE counted remainder line. Display-only — the wire carriers stay
// complete (rootCauseFamilyMemberLineRangeCap bounds them engine-side) and no
// decode lane caps at this number, so the constant has no cross-package
// mirror to pin.
const runtimeTraceProjFamilySpanTopCap = 3

// runtimeTraceProjFamilySpanTopNameBudget is the member-name cell budget of a
// constituent sub-row (the value + line-range cells must stay on the same
// physical line for the top-3 to read as a ranked list; the verbatim full
// name lives on the detail stanza the remainder line points at).
const runtimeTraceProjFamilySpanTopNameBudget = 56

// runtimeTraceProjFamilySpanTopNameFace renders a member span name into the
// sub-row name cell: verbatim when it fits, otherwise the catalog-B5
// tail-KEEPING middle cut (dex_location/class tails are the distinguishing
// segment of semantic span names — the head keeps the action word, the tail
// survives whole-budget). truncated reports whether the cut fired (the seat
// then drops the (span原文) verbatim chip — a cut name is not verbatim).
func runtimeTraceProjFamilySpanTopNameFace(name string, budget int) (face string, truncated bool) {
	if runewidth.StringWidth(name) <= budget {
		return name, false
	}
	runes := []rune(name)
	// Head keep: up to 12 cells of the span's leading action word.
	headBudget := 12
	var head []rune
	w := 0
	for _, r := range runes {
		rw := runewidth.RuneWidth(r)
		if w+rw > headBudget {
			break
		}
		head = append(head, r)
		w += rw
	}
	tailBudget := budget - w - 1
	if tailBudget < 1 {
		tailBudget = 1
	}
	var tail []rune
	tw := 0
	for i := len(runes) - 1; i >= len(head); i-- {
		rw := runewidth.RuneWidth(runes[i])
		if tw+rw > tailBudget {
			break
		}
		tail = append([]rune{runes[i]}, tail...)
		tw += rw
	}
	return string(head) + "…" + string(tail), true
}

// runtimeTraceProjFamilySpanTopMemberName strictly recovers member i's span
// name from the wire roster entry: the roster's single producer renders
// "<name> %.3fms" with the SAME duration the member_wall_ms carrier holds at
// index i, so the entry must end with that exact suffix — anything else fails
// the whole block (all-or-nothing; never a prose guess).
func runtimeTraceProjFamilySpanTopMemberName(entry string, wallMs float64) (string, bool) {
	suffix := fmt.Sprintf(" %.3fms", wallMs)
	if !strings.HasSuffix(entry, suffix) {
		return "", false
	}
	name := strings.TrimSpace(strings.TrimSuffix(entry, suffix))
	return name, name != ""
}

// runtimeTraceProjFamilySpanTopSubRows renders the SPANTOP-1 constituent
// block (§29.131, user ruling 2026-07-18) for a SEMANTIC family seat: top-3
// member spans by in-window wall-clock contribution (成员 name + 单段 value +
// 行a..b line range) plus ONE counted remainder line pointing at the detail
// stanza's complete inventory. ok=false keeps the caller on the legacy roster
// sub-rows byte-identically (整块不发,席行现状).
//
// EVERY gate below is a precise typed signal (all-or-nothing, 禁凑数):
//   - semantic family seat only (typed SemanticClass; 聚合/同线程 type 族与
//     ◦ trace_span 单段行不进此形);
//   - fold caliber == sum_disjoint (any other caliber's members do not sum to
//     the seat value — an interval_union/max seat keeps its legacy form);
//   - complete aligned carriers: wall list + line ranges + roster all exactly
//     FamilyMemberCount entries (the roster completeness doubles as the
//     remainder line's 全清单见明细 truth condition);
//   - producer order: the wall list is non-increasing (窗内墙钟贡献 desc);
//   - 单次最大 typed source reuse (§29.99 件⑥): the top-1 value must equal
//     the seat's FamilyMemberMaxMS at print precision — the「过长」case IS
//     the typed member max, never a display-side re-derivation;
//   - µs identity: Σ(members) == the seat's 行1 displayed value (integer-µs
//     arithmetic; the remainder value is derived as 行1 − Σ(top3), so
//     top3 + remainder == 席行合计 holds by construction);
//   - a remainder (count > cap) additionally needs the seat's [E#] tag.
//
// The sub-rows are strings on the seat's own subordinate lane — they never
// become model rows, so seats/badges/◎ populations/census denominators are
// structurally unreachable (链上 span 家族席硬纪律②③: zero on-chain claims,
// no ⛓, no credential impersonation — the seat row holds the credential).
func runtimeTraceProjFamilySpanTopSubRows(row runtimeTraceProjTreeRow, zh bool) ([]string, bool) {
	node := row.Node
	if !runtimeTraceProjFamilyRow(node) || strings.TrimSpace(node.SemanticClass) == "" {
		return nil, false
	}
	if strings.TrimSpace(node.FamilyFoldCaliber) != tracequery.RootCauseMemberFoldCaliberSumDisjoint {
		return nil, false
	}
	n := node.FamilyMemberCount
	wall := node.FamilyMemberWallMS
	ranges := node.FamilyMemberLineRanges
	roster := node.FamilyMemberRoster
	if len(wall) != n || len(ranges) != n || len(roster) != n {
		return nil, false
	}
	totalUS := int64(math.Round(runtimeTraceProjNodeDisplayImpact(node) * 1000))
	if totalUS <= 0 {
		return nil, false
	}
	var sumUS int64
	for i, v := range wall {
		if v <= 0 || (i > 0 && v > wall[i-1]) {
			return nil, false
		}
		sumUS += int64(math.Round(v * 1000))
	}
	if sumUS != totalUS {
		return nil, false
	}
	if node.FamilyMemberMaxMS > 0 && !runtimeTraceProjRound3Equal(wall[0], node.FamilyMemberMaxMS) {
		return nil, false
	}
	listed := n
	if listed > runtimeTraceProjFamilySpanTopCap {
		listed = runtimeTraceProjFamilySpanTopCap
	}
	evidenceTag := strings.TrimSpace(row.EvidenceTag)
	if n > listed && evidenceTag == "" {
		return nil, false
	}
	names := make([]string, 0, listed)
	anyTruncated := false
	var topUS int64
	for i := 0; i < listed; i++ {
		name, ok := runtimeTraceProjFamilySpanTopMemberName(roster[i], wall[i])
		if !ok {
			return nil, false
		}
		face, truncated := runtimeTraceProjFamilySpanTopNameFace(name, runtimeTraceProjFamilySpanTopNameBudget)
		anyTruncated = anyTruncated || truncated
		names = append(names, face)
		topUS += int64(math.Round(wall[i] * 1000))
	}
	// A truncated name is no longer verbatim — the whole block drops the C12
	// (span原文) chip (the "…" cut is its own signal; the verbatim bytes live
	// on the detail stanza). The A5 full-window chip keeps its own authority;
	// the untruncated block wears the ONE member-word source unchanged.
	memberWord := runtimeTraceProjFamilyMemberWord(node, zh)
	if anyTruncated {
		if zh {
			memberWord = "成员" + runtimeTraceProjFamilyMemberFullWindowChip(node, true)
		} else {
			memberWord = "member" + runtimeTraceProjFamilyMemberFullWindowChip(node, false)
		}
	}
	out := make([]string, 0, listed+1)
	for i := 0; i < listed; i++ {
		if zh {
			out = append(out, fmt.Sprintf("%s %s 单段%.3fms 行%d..%d",
				memberWord, names[i], wall[i], ranges[i][0], ranges[i][1]))
		} else {
			out = append(out, fmt.Sprintf("%s %s · segment %.3fms · lines %d..%d",
				memberWord, names[i], wall[i], ranges[i][0], ranges[i][1]))
		}
	}
	if rest := n - listed; rest > 0 {
		restMS := float64(totalUS-topUS) / 1000
		// The row's EvidenceTag is the bare id ("E7", "E7(+2)"); sentence
		// faces wrap it in brackets (the 构成段见[E#] convention).
		if zh {
			out = append(out, fmt.Sprintf("另有 %d 段 合计%.3fms · 全清单见明细[%s]", rest, restMS, evidenceTag))
		} else {
			out = append(out, fmt.Sprintf("%d more segments · total %.3fms · full inventory in the detail blocks [%s]", rest, restMS, evidenceTag))
		}
	}
	row.marks.mark(runtimeTraceProjMarkFamilySpanTop)
	return out, true
}

// runtimeTraceProjBusinessSpanMentionNameBudget is the span-name cell budget
// of a ◈ mention row — its own constant with the same display geometry as the
// SPANTOP sub-row cell (the value/count/line cells must stay readable beside
// the name; the verbatim full name lives in the evidence at 行a..b).
const runtimeTraceProjBusinessSpanMentionNameBudget = 56

// runtimeTraceProjBusinessSpanMentionBasisWord maps the closed-set on-chain
// basis token of a mention row to its display word (凭证词如实 — the word
// states exactly how the thread's on-chain status was proven, never more).
// ok=false = unknown token → the row never renders (fail-open).
func runtimeTraceProjBusinessSpanMentionBasisWord(basis string, zh bool) (string, bool) {
	switch basis {
	case tracequery.BusinessSpanMentionBasisSelf:
		if zh {
			return "自身", true
		}
		return "self", true
	case tracequery.BusinessSpanMentionBasisChainMember:
		if zh {
			return "链上节点", true
		}
		return "chain member", true
	case tracequery.BusinessSpanMentionBasisHostWakeupEdge:
		if zh {
			return "唤醒边凭证(边前)", true
		}
		return "wakeup-edge credential (pre-edge)", true
	}
	return "", false
}

// runtimeTraceProjBusinessSpanMentionValid is the shared typed admission of a
// ◈ mention row (factored for OMGCLEAN-1 件5 so the tree roster row and the
// ◎ compact row can never fork on validity). Every gate is a precise typed
// check; false drops the row (fail-open — the face never renders a partially
// valid row).
func runtimeTraceProjBusinessSpanMentionValid(m types.TraceCausalProjectionBusinessSpanMention) bool {
	if strings.TrimSpace(m.Subject) == "" || strings.TrimSpace(m.Name) == "" {
		return false
	}
	if m.Count < 1 || !(m.TotalMS > 0) || !(m.MaxMS > 0) || m.MaxMS > m.TotalMS+0.001 {
		return false
	}
	// POOL2-1 件① (§29.160① ruling): Hidden is informational 0..Count — a
	// fully-visible family (Hidden==0) renders too; only negative/overflow
	// values (impossible from the engine) drop the row.
	if m.StartLine < 1 || m.EndLine < m.StartLine || m.Hidden < 0 || m.Hidden > m.Count {
		return false
	}
	return true
}

// runtimeTraceProjBusinessSpanSelectionRuleWord — OMGCLEAN-1 件5 rider3
// (§29.175.5 ②, 2026-07-20): the selection rule is a PROMISE-FACE word — the
// ◈ heads (◎ zone + tree block) and the legend speak the same sentence
// (词面单点). The engine implements exactly this rule
// (tracequery computeBusinessSpanMentions: 非语义类业务族的 单次最长 TOP-K ∪
// 合计最长 TOP-K 去重).
//
// EVOLUTION RECORD (RULE3-1 件11, §29.185③, 2026-07-21): TOP3 → TOP5 —
// engine cap (BusinessSpanMentionFamilyCap) and this promise word move in
// one batch; the ruled reason is the doFrame crowd-out (扩容而非排除锚).
//
// EVOLUTION RECORD (MENTION8-1, §29.203, 2026-07-21): TOP5 → TOP8 — the
// user widened the display-selection cap (「TOP 5 可能不太够…安全的扩充为
// TOP 8(如果有)」); the union rule and admission gates are unchanged, and
// the engine cap moves with this word in one batch (词面单点).
func runtimeTraceProjBusinessSpanSelectionRuleWord(zh bool) string {
	if zh {
		return "单次最长∪合计最长 TOP8"
	}
	return "max-single ∪ total TOP8"
}

// runtimeTraceProjBusinessSpanPurposeWord — OMGCLEAN-1 件5 rider3 (§29.175.5
// ③ 面目的词): the business-self-check purpose sentence, one source for both
// ◈ heads (zh/EN 单点).
func runtimeTraceProjBusinessSpanPurposeWord(zh bool) string {
	if zh {
		return "业务自查:能否减少这些 span 的时间占用"
	}
	return "business self-check: can these spans' time be reduced"
}

// runtimeTraceProjBusinessSpanMentionNameFace — 双复核修复 件12 (冷读 CR9
// 截断名撞脸, 2026-07-21). EVOLUTION RECORD: the ◈ mention faces truncated
// through the SPANTOP tail-keeping middle cut alone (fixed 12-cell head +
// whole-budget tail) — two donghu_2955 H:CommitLayer span names differing
// only in their MIDDLE rendered the SAME face on both ◈ faces (同名撞脸,
// distinguishable only by the tree line ranges). COLLISION-AWARE two-tier
// cut: the tail-keeping face stays the default (B5b prior ruling —
// dex_location/class tails ARE the distinguishing segment of signature
// names, byte-identical for every non-colliding name), and exactly when the
// default face COLLIDES with a DIFFERENT sibling name's default face the cut
// re-runs head-majority — the head keeps the DISTINGUISHING PREFIX (the
// budget's bulk) and the tail floors at the RUN2FIX-A 件4 6-cell identity
// stub (runtimeTraceProjNameHeadPrefixFloorCells reused as the tail floor).
// Precise trigger (face byte equality across distinct names), never a
// heuristic; the SPANTOP member roster keeps its tail-keeping cut untouched.
func runtimeTraceProjBusinessSpanMentionNameFace(name string, siblings []types.TraceCausalProjectionBusinessSpanMention) string {
	budget := runtimeTraceProjBusinessSpanMentionNameBudget
	face, truncated := runtimeTraceProjFamilySpanTopNameFace(name, budget)
	if !truncated {
		return face
	}
	collision := false
	for _, s := range siblings {
		if s.Name == name {
			continue
		}
		if sibling, cut := runtimeTraceProjFamilySpanTopNameFace(s.Name, budget); cut && sibling == face {
			collision = true
			break
		}
	}
	if !collision {
		return face
	}
	runes := []rune(name)
	var tail []rune
	tw := 0
	for i := len(runes) - 1; i >= 0; i-- {
		rw := runewidth.RuneWidth(runes[i])
		if tw+rw > runtimeTraceProjNameHeadPrefixFloorCells {
			break
		}
		tail = append([]rune{runes[i]}, tail...)
		tw += rw
	}
	headBudget := budget - 1 - tw
	var head []rune
	w := 0
	for i, r := range runes {
		if i >= len(runes)-len(tail) {
			break // never overlap the tail stub
		}
		rw := runewidth.RuneWidth(r)
		if w+rw > headBudget {
			break
		}
		head = append(head, r)
		w += rw
	}
	return string(head) + "…" + string(tail)
}

// runtimeTraceProjBusinessSpanMentionRowText is the word-face source of a ◈
// TREE roster row (行号/凭证细节面 — §29.175.6: line ranges and credential
// words live here; the ◎ zone renders the compact row below). Values print
// verbatim from the typed transports (原始值三问: 提及行值=原始段值; no
// re-derivation, no re-scaling).
func runtimeTraceProjBusinessSpanMentionRowText(m types.TraceCausalProjectionBusinessSpanMention, siblings []types.TraceCausalProjectionBusinessSpanMention, zh bool) (string, bool) {
	if !runtimeTraceProjBusinessSpanMentionValid(m) {
		return "", false
	}
	basisWord, ok := runtimeTraceProjBusinessSpanMentionBasisWord(m.Basis, zh)
	if !ok {
		return "", false
	}
	face := runtimeTraceProjBusinessSpanMentionNameFace(m.Name, siblings)
	if zh {
		return fmt.Sprintf("%s %s 单次最大%.3fms/%d次 合计%.3fms 行%d..%d 凭证:%s",
			m.Subject, face, m.MaxMS, m.Count, m.TotalMS, m.StartLine, m.EndLine, basisWord), true
	}
	return fmt.Sprintf("%s %s · max single %.3fms ×%d · total %.3fms · lines %d..%d · credential: %s",
		m.Subject, face, m.MaxMS, m.Count, m.TotalMS, m.StartLine, m.EndLine, basisWord), true
}

// runtimeTraceProjBusinessSpanMentionCompactRowText is the ◎ ◈ zone's compact
// row (OMGCLEAN-1 件5 + §29.175.6 定谳: 值·线程·span 名·次数(·单次最大) — the
// TotalMS leads on the shared right-aligned value grid, NO bar (名维度视觉
// 区分), no line numbers and no credential words (they stay on the tree ◈
// roster row above). The 单次最大 clause attaches only when the family has
// >1 member (a single member's max IS its total — zero information twice).
func runtimeTraceProjBusinessSpanMentionCompactRowText(m types.TraceCausalProjectionBusinessSpanMention, siblings []types.TraceCausalProjectionBusinessSpanMention, zh bool) (string, bool) {
	if !runtimeTraceProjBusinessSpanMentionValid(m) {
		return "", false
	}
	if _, ok := runtimeTraceProjBusinessSpanMentionBasisWord(m.Basis, true); !ok {
		return "", false // same closed-set basis gate as the roster row
	}
	face := runtimeTraceProjBusinessSpanMentionNameFace(m.Name, siblings)
	var b strings.Builder
	if zh {
		fmt.Fprintf(&b, "%9.3fms %s · %s · %d次", m.TotalMS, m.Subject, face, m.Count)
		if m.Count > 1 {
			fmt.Fprintf(&b, " ·单次最大%.3fms", m.MaxMS)
		}
	} else {
		fmt.Fprintf(&b, "%9.3fms %s · %s · ×%d", m.TotalMS, m.Subject, face, m.Count)
		if m.Count > 1 {
			fmt.Fprintf(&b, " · max single %.3fms", m.MaxMS)
		}
	}
	return b.String(), true
}

// runtimeTraceProjBusinessSpanMentionNonAdditiveClause — F2 (返工轮, 冷读官
// 实证: donghu 全窗三族 Σ 275.8ms > 窗 233.19ms, 嵌套 span): the ONE
// non-additive word-face source shared by the tree block and the ◎ footnote.
// Family totals are per-family wall-clock sums whose intervals may overlap
// or nest (parent/child business spans) — never cross-family additive.
func runtimeTraceProjBusinessSpanMentionNonAdditiveClause(zh bool) string {
	if zh {
		return "各族合计间不可相加(区间可重叠/嵌套)"
	}
	return "family totals are not additive to each other (intervals may overlap or nest)"
}

// runtimeTraceProjBusinessSpanMentionOmittedText — 件3 truncation row (单点,
// both faces). 返工轮 F3: zh 未列出 — the families ARE counted (they cleared
// the floor and were tallied into N); they are merely not LISTED row-by-row;
// no detail-face pointer exists, so none is promised.
func runtimeTraceProjBusinessSpanMentionOmittedText(n int, zh bool) string {
	if n <= 0 {
		return ""
	}
	if zh {
		return fmt.Sprintf("另有 %d 个 span 族(≥显著地板)未列出", n)
	}
	// 双复核修复 件14 (冷读 CR10, 2026-07-21): EN n==1 plural branch.
	if n == 1 {
		return "1 more span family (at/above the significance floor) is not listed"
	}
	return fmt.Sprintf("%d more span families (at/above the significance floor) are not listed", n)
}

// runtimeTraceProjBusinessSpanMentionLines renders the ◈ tree-fence advisory
// block (SPANVIS-1 面1): a head line, the F2 non-additive promise line (same
// ◈ mark, zero indent — 用户 2026-07-19 多行同佩形), one wrapped "· " row
// per valid mention, plus the honest truncation row (件3). nil = no valid
// row = no block, no head, no mark (zero bytes). The rows are strings only —
// no row model, no seat, no ordinal, no bar; census/conservation populations
// are structurally unreachable.
func runtimeTraceProjBusinessSpanMentionLines(model runtimeTraceProjTreeModel, zh bool) []string {
	var rows []string
	for _, m := range model.BusinessSpanMentions {
		text, ok := runtimeTraceProjBusinessSpanMentionRowText(m, model.BusinessSpanMentions, zh)
		if !ok {
			continue
		}
		rows = append(rows, runtimeTraceProjSubordinateLines("", text)...)
	}
	if len(rows) == 0 {
		return nil
	}
	var out []string
	// OMGCLEAN-1 件5 rider3 (§29.175.5 ②③): the tree ◈ head speaks the
	// selection-rule promise word and the business-self-check purpose (same
	// single sources as the ◎ zone head — 词面单点).
	if zh {
		out = append(out, "◈ 业务span提示(不参与根因排序 · "+runtimeTraceProjBusinessSpanSelectionRuleWord(true)+" · "+runtimeTraceProjBusinessSpanPurposeWord(true)+"):")
	} else {
		out = append(out, "◈ business span leads (not in root-cause ranking · "+runtimeTraceProjBusinessSpanSelectionRuleWord(false)+" · "+runtimeTraceProjBusinessSpanPurposeWord(false)+"):")
	}
	out = append(out, "◈ "+runtimeTraceProjBusinessSpanMentionNonAdditiveClause(zh))
	out = append(out, rows...)
	if text := runtimeTraceProjBusinessSpanMentionOmittedText(model.BusinessSpanMentionOmitted, zh); text != "" {
		out = append(out, runtimeTraceProjSubordinateLines("", text)...)
	}
	return out
}

// runtimeTraceProjFamilyMemberFullWindowChip — catalog A5 (DISPLAY-HYG 二轮,
// §29.104.18.1, 2026-07-17): the ONE full-window-account chip authority
// shared by the tree 行4+ roster sub-rows and the detail stanza's 成员/members
// field key. A re-anchored 同源二分 half carries the chip (ChainAnchorFullMS
// > 0 — the exact predicate the tree face always used) on BOTH faces, so the
// detail roster can never read as a per-side split while the tree face says
// 全窗账 (the witness held 「单段 2.197~3.853ms」 beside a 16.064ms
// full-window member with the chip only on the tree). "" on plain rows —
// their faces stay byte-identical.
func runtimeTraceProjFamilyMemberFullWindowChip(node types.TraceCausalProjectionNode, zh bool) string {
	if node.ChainAnchorFullMS <= 0 {
		return ""
	}
	if zh {
		return "(全窗账)"
	}
	return " (full-window account)"
}

// runtimeTraceProjFamilyMemberVerbatimSpanChip — catalog C12 (DISPLAY-HYG
// 二轮, §29.104.18.1, 2026-07-17): a SEMANTIC family's roster entries are
// engine-verbatim span names (dex_location tails with source-side unbalanced
// parens, wrap-split argument lists) — the member word says so, so a reader
// meeting a lone `)` or a wrapped `(int)` knows the bytes are quoted source,
// not display damage. Typed gate = SemanticClass (minted exclusively on
// trace_semantic_span records); every other family keeps its bare word
// byte-identically.
func runtimeTraceProjFamilyMemberVerbatimSpanChip(node types.TraceCausalProjectionNode, zh bool) string {
	if strings.TrimSpace(node.SemanticClass) == "" {
		return ""
	}
	if zh {
		return "(span原文)"
	}
	return " (verbatim span)"
}

// runtimeTraceProjFamilyMemberWord is the tree-face member-row word
// (成员/member + the A5 full-window chip + the C12 verbatim-span chip).
func runtimeTraceProjFamilyMemberWord(node types.TraceCausalProjectionNode, zh bool) string {
	if zh {
		return "成员" + runtimeTraceProjFamilyMemberFullWindowChip(node, true) +
			runtimeTraceProjFamilyMemberVerbatimSpanChip(node, true)
	}
	return "member" + runtimeTraceProjFamilyMemberFullWindowChip(node, false) +
		runtimeTraceProjFamilyMemberVerbatimSpanChip(node, false)
}

// runtimeTraceProjFamilyTableToken is the (a) key-metric table's family token
// (D3 表面: ×N token + 合计 stem beside the node name; the full caliber word
// and roster live on the (b) stanza). "" on non-family rows.
func runtimeTraceProjFamilyTableToken(node types.TraceCausalProjectionNode, zh bool) string {
	if !runtimeTraceProjFamilyRow(node) {
		return ""
	}
	prefix := runtimeTraceProjFamilyValuePrefix(node, zh)
	// EVOLUTION RECORD (WF-xn §29.52.1, 2026-07-12): 「×N合计」→「N次合计」/
	// en 「n=N total」-family — the count chip speaks the same vocabulary as
	// the data tokens (tracefence display-table ⑥).
	if prefix == "" {
		// Unknown caliber: the count is still typed truth — token without a
		// caliber claim.
		if zh {
			return fmt.Sprintf("%d次", node.FamilyMemberCount)
		}
		return fmt.Sprintf("n=%d", node.FamilyMemberCount)
	}
	if zh {
		return fmt.Sprintf("%d次%s", node.FamilyMemberCount, prefix)
	}
	return fmt.Sprintf("n=%d %s", node.FamilyMemberCount, strings.TrimSpace(prefix))
}

// runtimeTraceProjFamilySemanticClassWord is the semantic family's 行1 词位
// (D2 类型词行1词位, witness ✦ VerifyClass ×14): ONE member's span name must
// not impersonate the whole family, so the row speaks the typed semantic
// class's display word (zh 类校验/JIT编译…, EN raw token); the member span
// names ride the roster sub-rows and the detail stanza. "" when the node
// carries no semantic class (caller keeps its own lane).
func runtimeTraceProjFamilySemanticClassWord(node types.TraceCausalProjectionNode, zh bool) string {
	class := strings.TrimSpace(node.SemanticClass)
	if class == "" {
		class = strings.TrimSpace(node.Object)
	}
	if class == "" {
		return ""
	}
	return strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseName(class, zh))
}

// runtimeTraceProjAbsorbedChainNote renders the family stanza's 链上并入
// disclosure (G1 跨车道对账, §27.2-G1 ruling wording, 2026-07-09): "链上车道
// N 条同源观测已并入本行(E#,E#…)". The E# list is bounded with a counted
// overflow (§24.7.1 ① roster 折叠必带计数披露); tag-less peers still count in
// N (identity survives on the raw observation even when the evidence index
// could not mint a locator). The note carries identity ONLY — the absorbed
// values are already inside this row's combined account (engine membership
// proof), so re-printing an ms here would invite double-reading.
func runtimeTraceProjAbsorbedChainNote(peers []runtimeTraceProjAbsorbedChainPeer, zh bool) string {
	const tagCap = 8
	tags := make([]string, 0, len(peers))
	for _, peer := range peers {
		if tag := strings.TrimSpace(peer.EvidenceTag); tag != "" && len(tags) < tagCap {
			tags = append(tags, tag)
		}
	}
	sep := ","
	if zh {
		sep = "、"
	}
	list := strings.Join(tags, sep)
	if rest := len(peers) - len(tags); rest > 0 && list != "" {
		if zh {
			list += fmt.Sprintf("%s等共%d条", sep, len(peers))
		} else {
			list += fmt.Sprintf("%s %d total", sep, len(peers))
		}
	}
	if zh {
		if list == "" {
			return fmt.Sprintf("链上通道 %d 条同源观测已并入本行", len(peers))
		}
		return fmt.Sprintf("链上通道 %d 条同源观测已并入本行(%s)", len(peers), list)
	}
	if list == "" {
		return fmt.Sprintf("%d on-chain-channel same-source observation(s) absorbed into this row", len(peers))
	}
	return fmt.Sprintf("%d on-chain-channel same-source observation(s) absorbed into this row (%s)", len(peers), list)
}

// runtimeTraceProjSemanticCellParts renders the shared name/value pair of the
// LEAD-SEM consumers (comparison 确定性优化点 cell, the zero-chain presence
// note and the semantic-fallback conclusion line — one wording source, D3
// 零链括注同步): a family node speaks 「<类型词> ×N」 + 「合计<V>ms」; a
// plain span keeps the legacy name + bare ms byte-identically.
func runtimeTraceProjSemanticCellParts(node types.TraceCausalProjectionNode, ms float64, zh bool) (string, string) {
	name := strings.TrimSpace(node.SpanName)
	if name == "" {
		name = strings.TrimSpace(node.Object)
	}
	value := fmt.Sprintf("%.3fms", ms)
	if runtimeTraceProjFamilyRow(node) {
		if word := runtimeTraceProjFamilySemanticClassWord(node, zh); word != "" {
			name = word
		}
		name += runtimeTraceProjMergeCountChip(node.FamilyMemberCount, zh)
		if runtimeTraceProjFamilyCountSumClamped(node) {
			// §29.55 观察③ 两形一裁 (2026-07-14): the 计数当量 stem never
			// composes with an ms-suffixed value — suffix-free atom.
			value = runtimeTraceProjCountEquivalentValueText(ms, zh)
		} else if prefix := runtimeTraceProjFamilyValuePrefix(node, zh); prefix != "" {
			value = prefix + value
		}
	}
	return name, value
}

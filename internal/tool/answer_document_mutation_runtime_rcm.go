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
func runtimeTraceProjCountEquivalentValueText(v float64, zh bool) string {
	if zh {
		return fmt.Sprintf("计数当量%.3f(非墙钟)", v)
	}
	return fmt.Sprintf("count-equivalent %.3f (not wall clock)", v)
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
	for _, entry := range roster[:listed] {
		if zh {
			out = append(out, "成员 "+entry)
		} else {
			out = append(out, "member "+entry)
		}
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
			return fmt.Sprintf("链上车道 %d 条同源观测已并入本行", len(peers))
		}
		return fmt.Sprintf("链上车道 %d 条同源观测已并入本行(%s)", len(peers), list)
	}
	if list == "" {
		return fmt.Sprintf("%d chain-lane same-source observation(s) absorbed into this row", len(peers))
	}
	return fmt.Sprintf("%d chain-lane same-source observation(s) absorbed into this row (%s)", len(peers), list)
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

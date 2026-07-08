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
		// Count-class members keep the counting-semantics word (D1: count_sum
		// 形沿用计数语义词) — counts add regardless of interval overlap.
		if zh {
			return fmt.Sprintf("计数合计(共%d项,同线程)", n), runtimeTraceProjMarkFamilyCountSum, true
		}
		return fmt.Sprintf("count total (%d items, same thread)", n), runtimeTraceProjMarkFamilyCountSum, true
	}
	return "", 0, false
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
// attribution when present, else the row's display impact. Never a display-
// side recomputation — the engine's fold is the single value authority.
func runtimeTraceProjFamilyPublishedMS(node types.TraceCausalProjectionNode) float64 {
	if node.EffectiveImpactMS > 0 {
		return node.EffectiveImpactMS
	}
	return runtimeTraceProjNodeDisplayImpact(node)
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
	if prefix == "" {
		// Unknown caliber: the count is still typed truth — token without a
		// caliber claim.
		return fmt.Sprintf("×%d", node.FamilyMemberCount)
	}
	if zh {
		return fmt.Sprintf("×%d%s", node.FamilyMemberCount, prefix)
	}
	return fmt.Sprintf("×%d %s", node.FamilyMemberCount, strings.TrimSpace(prefix))
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
		name = fmt.Sprintf("%s ×%d", name, node.FamilyMemberCount)
		if prefix := runtimeTraceProjFamilyValuePrefix(node, zh); prefix != "" {
			value = prefix + value
		}
	}
	return name, value
}

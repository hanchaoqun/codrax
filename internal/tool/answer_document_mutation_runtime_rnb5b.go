package tool

// answer_document_mutation_runtime_rnb5b.go — RNB-5B 件⑦ (§29.96.2 终判⑦,
// 2026-07-15): the micro anchored-cut-seat fold.
//
// Chain-lane anchored bipartition CUT seats (⛓ 剪切席 — the credential-
// anchored halves of the 同源二分, ChainAnchoredMS > 0 ∧ !remainder) whose
// published effective attribution sits below the micro threshold fold into
// ONE counted row 「其余 N 项微额锚定席」 on both the projection tree's chain
// lane AND the ◎ overview board (witness donghu 2955 全窗: the 0.026/0.018/
// 0.016 anchored slivers held 根因排序#5/#6/#7 and a board slot while their
// ◇ remainder twins carried the readable accounts). The fold row:
//
//   - keeps the credential semantics — it stays on the ⛓ on-chain channel
//     (凭证语义保留, per the ruling);
//   - publishes the members' account Σ under the explicit 合计 word (the
//     ruling's 「(合计 X)见明细」 form — an ACCOUNT sum over the sub-threshold
//     anchored shares, never a wall-clock union claim; the caliber tag says
//     账目合计);
//   - follows the R9 fold-row discipline (§29.93.2): line 1 carries only the
//     bare counted label, the member preview sinks to line 2;
//   - carries every member's evidence id ([E#+E#+…] bracket + merged ids), so
//     the ◇ remainder twins' 同源二分 sentences keep resolvable on-page refs
//     (零静默消失 — the RNB-1 D1 dangling-reference lesson).
//
// The fold runs at the display-model level only (values, ranks, ordinals and
// every engine lane untouched); it fires only for ≥2 micro seats — folding a
// single row would be a pure rename that loses its identity line for nothing
// (工程判断, documented deviation from the bare threshold wording).

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeTraceProjMicroAnchorFoldMs is the single-point micro threshold
// (§29.96.2 终判⑦ 阈值常量单点). Display-face fold selection only — never a
// gate, score or rank input. Deliberately NOT borrowed from any other
// tolerance constant (容差常量禁跨语义借用).
const runtimeTraceProjMicroAnchorFoldMs = 0.1

// runtimeTraceProjMicroAnchorCutSeat reports whether a node is a chain-lane
// anchored CUT seat below the micro threshold: the ⛓ half of a bipartition
// (typed ChainAnchoredMS > 0, not the ◇ remainder half) on the on-chain
// channel with 0 < eff < 0.1ms. Pure typed comparisons.
func runtimeTraceProjMicroAnchorCutSeat(node types.TraceCausalProjectionNode) bool {
	return node.ChainAnchoredMS > 0 && !node.ChainAnchorRemainderSeat &&
		strings.TrimSpace(node.ChainRelevance) == "on_chain" &&
		node.EffectiveImpactMS > 0 &&
		node.EffectiveImpactMS < runtimeTraceProjMicroAnchorFoldMs
}

// runtimeTraceProjFoldMicroAnchorSeats folds the chain-lane micro anchored
// cut seats of model.TreeRows into one fold row seated at the first folded
// row's position. Runs after every relation/attach pass (members' relation
// sentences on their ◇ twins are already minted and their E# refs stay
// on-page via the fold bracket) and before bar scaling / badge assignment.
func runtimeTraceProjFoldMicroAnchorSeats(model *runtimeTraceProjTreeModel) {
	if model == nil {
		return
	}
	var members []runtimeTraceProjTreeRow
	memberIdx := map[int]bool{}
	firstIdx := -1
	for i := range model.TreeRows {
		row := model.TreeRows[i]
		if !row.HasData || row.Node.OnChainOverflowFold {
			continue
		}
		if row.Kind != runtimeTraceProjTreeRowChain && row.Kind != runtimeTraceProjTreeRowDepthless {
			continue
		}
		if runtimeTraceProjMicroAnchorCutSeat(row.Node) {
			if firstIdx < 0 {
				firstIdx = i
			}
			memberIdx[i] = true
			members = append(members, row)
		}
	}
	if len(members) < 2 {
		return // a single micro seat keeps its own row (fold-of-one = rename)
	}
	fold := runtimeTraceProjTreeRow{
		Kind:    runtimeTraceProjTreeRowChain,
		Edge:    runtimeTraceProjTreeEdgeChainUnresolved,
		HasData: true,
	}
	node := types.TraceCausalProjectionNode{
		Role:            types.TraceCausalRoleRootCauseContext,
		ChainRelevance:  "on_chain",
		MicroAnchorFold: true,
		// OnChainOverflowFold rides along so every established fold-row guard
		// (cause-grammar exclusion, census skips, pointer-pass subject skip)
		// treats this row as the fold it is; the micro word/caliber arms fork
		// BEFORE the generic overflow-fold arms at each emission site.
		OnChainOverflowFold: true,
		MergedCount:         len(members),
	}
	sum := 0.0
	var tags []string
	depthless := 0
	rankLo, rankHi := 0, 0
	allRanked := true
	uniformState := ""
	uniformStateOK := true
	for _, member := range members {
		mn := member.Node
		sum += mn.EffectiveImpactMS
		// 修复轮 P1-1 (F8): the census counts fold rows by member — record how
		// many folded members were Depthless rows (individually counted
		// pre-fold).
		if member.Kind == runtimeTraceProjTreeRowDepthless {
			depthless++
		}
		// 修复轮 U3 (席位记忆): the folded members' rank ordinal range for the
		// detail block's honest seat note (all-ranked folds only — a rank-less
		// member voids the claim, 宁缺勿假).
		if mn.Rank <= 0 {
			allRanked = false
		} else {
			if rankLo == 0 || mn.Rank < rankLo {
				rankLo = mn.Rank
			}
			if mn.Rank > rankHi {
				rankHi = mn.Rank
			}
		}
		// 修复轮 U4 (全员一致态词): a uniform member StateKind survives onto
		// the fold node so the detail faces keep the honest state word
		// (三 runnable 折叠不写「未分类」); any mix or absence stays wordless.
		if state := strings.TrimSpace(mn.StateKind); state == "" {
			uniformStateOK = false
		} else if uniformState == "" {
			uniformState = state
		} else if uniformState != state {
			uniformStateOK = false
		}
		if node.MergedMinMS == 0 || (mn.EffectiveImpactMS > 0 && mn.EffectiveImpactMS < node.MergedMinMS) {
			node.MergedMinMS = mn.EffectiveImpactMS
		}
		if mn.EffectiveImpactMS > node.MergedMaxMS {
			node.MergedMaxMS = mn.EffectiveImpactMS
		}
		if subject := strings.TrimSpace(mn.Subject); subject != "" {
			node.MergedSubjects = append(node.MergedSubjects, subject)
		}
		if id := strings.TrimSpace(mn.EvidenceID); id != "" {
			node.MergedEvidenceIDs = append(node.MergedEvidenceIDs, id)
		}
		for _, id := range mn.MergedEvidenceIDs {
			node.MergedEvidenceIDs = append(node.MergedEvidenceIDs, id)
		}
		if tag := strings.TrimSpace(member.EvidenceTag); tag != "" {
			tags = append(tags, tag)
		}
		if mn.LineStart > 0 && (node.LineStart <= 0 || mn.LineStart < node.LineStart) {
			node.LineStart = mn.LineStart
		}
		if mn.LineEnd > node.LineEnd {
			node.LineEnd = mn.LineEnd
		}
		if mn.Confidence > 0 && (node.Confidence <= 0 || mn.Confidence < node.Confidence) {
			node.Confidence = mn.Confidence
		}
	}
	// The value channel carries the account Σ (per the ruling's explicit
	// (合计 X) form) on BOTH the display and effective channels — the board
	// orders and prints EffectiveImpactMS, the tree bar reads the display
	// impact. The caliber tag names the 账目合计 ruler.
	node.ImpactMS = sum
	node.CumulativeImpactMS = sum
	node.EffectiveImpactMS = sum
	if uniformStateOK && uniformState != "" {
		node.StateKind = uniformState // U4: 全员一致态词保留
	}
	fold.Node = node
	fold.EvidenceTag = strings.Join(tags, "+")
	fold.MicroAnchorFoldDepthlessMembers = depthless
	if allRanked && rankLo > 0 {
		fold.MicroAnchorFoldRankLo = rankLo
		fold.MicroAnchorFoldRankHi = rankHi
	}
	kept := make([]runtimeTraceProjTreeRow, 0, len(model.TreeRows)-len(members)+1)
	for i := range model.TreeRows {
		if memberIdx[i] {
			if i == firstIdx {
				kept = append(kept, fold)
			}
			continue
		}
		kept = append(kept, model.TreeRows[i])
	}
	model.TreeRows = kept
}

// runtimeTraceProjStampSelfWallClockQualifiers — RNB-5B 默认小件c (§29.95
// UX-4 对称, 2026-07-15): the 「自身·墙钟席」 qualifier covers the WHOLE self
// wall-clock cause-seat family, not only the rows whose lane was minted by
// the SELF-ALL basis arm (witness donghu 17267: the io_latency family seat
// E5 wore the chip while the sibling self seats E6/E9/E10 — scheduler
// satellite, chain-lane io seat, io_burst seat — rendered bare; the reader
// had no way to tell why four same-thread seats split into worded/unworded).
// Typed inputs only: SelfRows membership (subject == target by construction),
// chain ordinal channel, ranked participation, a positive published eff, and
// none of the symptom/context/caliber-side/semantic exclusions (each of those
// families wears its OWN qualifier or footnote). Wording input only.
func runtimeTraceProjStampSelfWallClockQualifiers(model *runtimeTraceProjTreeModel) {
	if model == nil {
		return
	}
	for i := range model.SelfRows {
		row := &model.SelfRows[i]
		node := row.Node
		if !row.HasData || node.EffectiveImpactMS <= 0 {
			continue
		}
		if node.IsTargetSelfStateRow() || node.IsContextOnlyRow() || node.IsCaliberSideRow() {
			continue
		}
		if strings.TrimSpace(node.SemanticClass) != "" {
			continue // the semantic self family wears 自身·确定性优化
		}
		if node.Rank <= 0 && len(row.RankFoldPeers) == 0 {
			continue
		}
		if runtimeTraceProjRowOrdinalChannel(*row) != runtimeTraceProjOrdinalChannelChain {
			continue
		}
		row.SelfWallClockQualifier = true
	}
}

// runtimeTraceProjMicroAnchorFoldName is the fold row's line-1 label (R9
// 行1 短标签纪律 — the bare counted label only; the member preview sinks to
// line 2 via the shared fold sink line).
func runtimeTraceProjMicroAnchorFoldName(node types.TraceCausalProjectionNode, zh bool) string {
	if zh {
		return fmt.Sprintf("其余 %d 项微额锚定席", node.MergedCount)
	}
	return fmt.Sprintf("%d more micro anchored seats", node.MergedCount)
}

// runtimeTraceProjMicroAnchorFoldTagText is the fold row's caliber tag — the
// 合计/见明细 半 of the ruling's 「其余 N 项微额锚定席(合计 X)见明细」 form:
// the value channel already printed Σ, this tag names the ruler (账目合计
// over sub-threshold anchored shares), re-affirms the preserved ⛓ credential
// semantics, and points at the detail blocks.
func runtimeTraceProjMicroAnchorFoldTagText(node types.TraceCausalProjectionNode, zh bool) string {
	if zh {
		return fmt.Sprintf("%d席合计(账目合计,单席%s,均<%.1fms)·凭证锚定段,仍属⛓链上通道·见明细",
			node.MergedCount, runtimeTraceProjMergedRangeText(node), runtimeTraceProjMicroAnchorFoldMs)
	}
	return fmt.Sprintf("%d-seat total (account sum, each %s, all <%.1fms) · anchored credential shares, still the ⛓ on-chain channel · see the detail blocks",
		node.MergedCount, runtimeTraceProjMergedRangeText(node), runtimeTraceProjMicroAnchorFoldMs)
}

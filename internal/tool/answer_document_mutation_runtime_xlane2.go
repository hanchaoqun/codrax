package tool

// answer_document_mutation_runtime_xlane2.go — XLANE-2 件1: semantic family
// member-subset demotion (§29.104.1 立案 / §29.104.2 定谳④, customer witness
// runnable2.txt, 2026-07-17).
//
// Disease: the same physical semantic spans double-mint across query steps
// through the three semantic lanes (SELF-SEM / overlap-basis / non-chain —
// twin keys carry line boundaries, so cross-step families never dedupe): the
// witness E34 (类校验 8次 9.586ms, 榜#1) = E35 (4次, on-chain overlap lane) ∪
// E49 (4次, non-chain self lane), with E35's members verbatim inside E34's —
// three seats of one physical account, zero cross-references.
//
// Fix (display-side — the fusion point is the only place all steps meet):
// within one board, a semantic family seat whose COMPLETE typed member
// line-range set is a PROPER SUBSET of another semantic seat's set demotes —
// it wears the 为[E#]成员子集(整席降道) pointer at its row and
// leaves the ◎ population/census into a dedicated footnote (XLANE-1 裁定①
// precedent: ◎ 排除 + 专用披露脚注; 值面与引擎序数零动). The complementary
// split form (E35 ∪ E49 = E34) needs no special casing: each sub-seat is
// itself a proper subset of the union seat and both demote toward it.
//
// Precision discipline (CLAUDE.md 红线): the judgment consumes ONLY the
// engine-typed member line-range sets (FamilyMemberLineRanges — minted
// all-or-nothing, parsed strictly against the member count); prose/name/value
// matching is forbidden. A seat without a complete set never judges and never
// demotes (fail-open 保原状+不记账). Board identity consumes the ONE shared
// board index (runtimeTraceProjRankBoardIndexFor — 板身份单值源, XLANE-3);
// provably different boards never compare (cross-board double-mints stay on
// the XLANE-3 cross-board mutual-pointer lane).

import "strings"

// runtimeTraceProjSemanticSubsetCandidate is one judgeable seat: a semantic
// family row carrying its complete typed member line-range set.
type runtimeTraceProjSemanticSubsetCandidate struct {
	row *runtimeTraceProjTreeRow
	set map[[2]int]bool
}

// runtimeTraceProjMarkSemanticMemberSubsets stamps the 件1 verdicts on the
// model. Runs BEFORE the generic relation passes (the subset relation is the
// most typed judgment of the family; later passes skip related rows through
// runtimeTraceProjSMR1RelationFree).
func runtimeTraceProjMarkSemanticMemberSubsets(model *runtimeTraceProjTreeModel) {
	all := runtimeTraceProjSMR1AllRows(model)
	var cands []runtimeTraceProjSemanticSubsetCandidate
	for _, row := range all {
		if !row.HasData || strings.TrimSpace(row.Node.SemanticClass) == "" {
			continue
		}
		node := row.Node
		if node.FamilyMemberCount <= 1 || len(node.FamilyMemberLineRanges) != node.FamilyMemberCount {
			// 行号缺席/不完整的席 fail-open 保原状 (the engine mints the set
			// all-or-nothing; the strict decode enforces count equality — this
			// arm is the display-side belt of the same discipline).
			continue
		}
		set := make(map[[2]int]bool, len(node.FamilyMemberLineRanges))
		for _, lineRange := range node.FamilyMemberLineRanges {
			set[lineRange] = true
		}
		cands = append(cands, runtimeTraceProjSemanticSubsetCandidate{row: row, set: set})
	}
	if len(cands) < 2 {
		return
	}
	boardIndex := runtimeTraceProjRankBoardIndexFor(all)
	for i := range cands {
		sub := cands[i]
		if !runtimeTraceProjSMR1RelationFree(sub.row) {
			continue // an already-related row keeps its earlier relation (one relation, one sentence)
		}
		// Pick the LARGEST proper superset (deterministic; on a chain
		// A ⊂ B ⊂ C every subset points at C directly — the ref target can
		// never itself be a demoted seat, single-hop by construction).
		var best *runtimeTraceProjSemanticSubsetCandidate
		for j := range cands {
			if i == j {
				continue
			}
			super := &cands[j]
			if runtimeTraceCausalProjectionCanonicalNode(sub.row.Node.Subject) !=
				runtimeTraceCausalProjectionCanonicalNode(super.row.Node.Subject) {
				continue
			}
			if !runtimeTraceProjSemanticSubsetSameBoard(boardIndex, sub.row, super.row) {
				continue
			}
			if !runtimeTraceProjSemanticSubsetProper(sub.set, super.set) {
				continue
			}
			if best == nil || len(super.set) > len(best.set) {
				best = super
			}
		}
		if best == nil {
			continue
		}
		ref := strings.TrimSpace(best.row.EvidenceTag)
		if ref == "" {
			continue // 宁漏勿假指: no guessed E#
		}
		sub.row.NonAdditiveRef = ref
		sub.row.NonAdditiveKind = runtimeTraceProjNonAdditiveMemberSubset
		sub.row.SemanticMemberSubsetOf = ref
	}
}

// runtimeTraceProjSemanticSubsetProper reports whether sub is a PROPER subset
// of super on the typed line-range sets (every member range of sub appears in
// super, and super holds at least one range sub lacks). Equal sets are NOT a
// subset relation — neither side demotes (lane twins of one identical account
// stay on their existing mirror/fold lanes).
func runtimeTraceProjSemanticSubsetProper(sub, super map[[2]int]bool) bool {
	if len(sub) == 0 || len(sub) >= len(super) {
		return false
	}
	for lineRange := range sub {
		if !super[lineRange] {
			return false
		}
	}
	return true
}

// runtimeTraceProjSelfGapOverlapClause is one resolved clause of the XLANE-2
// 件2 disclosure: the partner semantic seat's evidence tag + the typed
// overlap wall clock.
type runtimeTraceProjSelfGapOverlapClause struct {
	Ref       string
	OverlapMS float64
}

// runtimeTraceProjStampSelfGapSemanticOverlaps (XLANE-2 件2, user ruling
// §29.104.17 ④ 披露式拆分, 2026-07-17) resolves the self-gap seat's typed
// overlap roster into display clauses: each partner is matched by its VERBATIM
// typed line envelope (node.LineStart/LineEnd equality) on a same-subject
// semantic row — never a name/value guess. An unresolvable partner drops its
// clause (宁漏勿假指); zero resolved clauses leave the row byte-identical.
func runtimeTraceProjStampSelfGapSemanticOverlaps(model *runtimeTraceProjTreeModel) {
	all := runtimeTraceProjSMR1AllRows(model)
	for _, row := range all {
		if len(row.Node.SelfGapSemanticOverlaps) == 0 {
			continue
		}
		for _, overlap := range row.Node.SelfGapSemanticOverlaps {
			var partner *runtimeTraceProjTreeRow
			ambiguous := false
			for _, cand := range all {
				if cand == row || !cand.HasData || strings.TrimSpace(cand.Node.SemanticClass) == "" {
					continue
				}
				if cand.Node.LineStart != overlap.LineStart || cand.Node.LineEnd != overlap.LineEnd {
					continue
				}
				if runtimeTraceCausalProjectionCanonicalNode(cand.Node.Subject) !=
					runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) {
					continue
				}
				if partner != nil {
					ambiguous = true
					break
				}
				partner = cand
			}
			if partner == nil || ambiguous || strings.TrimSpace(partner.EvidenceTag) == "" {
				continue
			}
			row.SelfGapSemanticOverlapClauses = append(row.SelfGapSemanticOverlapClauses,
				runtimeTraceProjSelfGapOverlapClause{
					Ref: strings.TrimSpace(partner.EvidenceTag), OverlapMS: overlap.OverlapMS,
				})
		}
	}
}

// runtimeTraceProjSemanticSubsetSameBoard is the 件1 同板 gate over the ONE
// shared board index (板身份单值源 — never a second board-identity
// implementation): equal full triple IDs are one board; different window
// clusters are different boards; two NAMED rows with different triples are
// provably different boards (target or params fork — XLANE-3 discipline).
// A named/unnamed pair inside ONE window cluster is compatible only when the
// cluster hosts at most one named target and that board carries at most one
// params fingerprint — an identity-less row never guesses between two named
// boards (宁漏勿假指; the witness single-window single-target report is
// exactly the compatible form).
func runtimeTraceProjSemanticSubsetSameBoard(index *runtimeTraceProjRankBoardIndex, a, b *runtimeTraceProjTreeRow) bool {
	if index.ids[a] == index.ids[b] {
		return true
	}
	windowID := index.windowIDs[a]
	if windowID != index.windowIDs[b] {
		return false
	}
	aTarget := strings.TrimSpace(a.Node.RankBoardTarget)
	bTarget := strings.TrimSpace(b.Node.RankBoardTarget)
	if aTarget != "" && bTarget != "" {
		return false
	}
	named := ""
	namedCount := 0
	for target := range index.targetsInWindow[windowID] {
		if target != "" {
			named = target
			namedCount++
		}
	}
	if namedCount > 1 {
		return false
	}
	if len(index.paramsInBoard[windowID+"\x00"+named]) > 1 {
		return false
	}
	// 修补轮 件5 (2026-07-17): fingerprint blind-spot gate — an identity-less
	// row carrying its OWN non-empty params fingerprint that differs from the
	// named board's single fingerprint is provably another board (精确信号:
	// two verbatim typed fingerprints, both present, unequal → 宁漏勿假指).
	// An absent fingerprint on either side never proves a fork and stays
	// compatible (the witness form: named board fingerprinted, span-family
	// rows fingerprint-less).
	for _, row := range []*runtimeTraceProjTreeRow{a, b} {
		if strings.TrimSpace(row.Node.RankBoardTarget) != "" {
			continue // a named row's fingerprint is the named board's own (indexed above)
		}
		fp := strings.TrimSpace(row.Node.RankBoardParamsFingerprint)
		if fp == "" {
			continue
		}
		for namedFP := range index.paramsInBoard[windowID+"\x00"+named] {
			if namedFP != fp {
				return false
			}
		}
	}
	return true
}

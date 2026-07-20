package tool

// answer_document_mutation_runtime_axiomv2.go — AXIOM-V2 批 display side
// (user rulings 2026-07-18, ledger docs/design/real_trace_campaign_20260705.md):
//
//   件1  the fix-direction attribute word (方向词落行2 + legend): a single
//        zh/EN word table keyed on the registry's wire token — the display
//        NEVER re-derives a direction from type names (the engine publishes
//        the registry value verbatim; 单一权威).
//   件2  the cross-direction mutual-overlap clause resolution: each wire
//        entry's partner is matched by its VERBATIM typed line envelope on a
//        same-subject same-board row (the XLANE-2 resolution discipline), and
//        the pair renders BOTH-OR-NEITHER (reciprocity prune — a one-sided
//        pointer would break the 互指 promise; 宁漏勿假指).
//
// 根因排序三护栏之② (件1 discipline): direction is an ATTRIBUTE AXIS — no
// ordinal, chip, sort or value face forks on it; the words land on 行2 and
// the legend only.

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
)

// runtimeTraceProjFixDirectionWord is the display's accessor for the registry
// fix-direction word table. FREQDIR-1 件1 (§29.149, 2026-07-19): the table
// bytes moved verbatim to tracefence.FixDirectionWord (Table ⑦) so the
// LLM-facing board feed (internal/context) and this renderer consume ONE
// mapping — behavior byte-identical, only the ownership moved. ok=false on
// unresolved/unknown tokens: absence never guesses (fail-open).
func runtimeTraceProjFixDirectionWord(direction string, zh bool) (string, bool) {
	return tracefence.FixDirectionWord(direction, zh)
}

// runtimeTraceProjCrossDirectionClause is one resolved clause of the 件2
// mutual disclosure: the partner seat's evidence tag + the typed overlap wall
// clock + the partner's fix direction.
type runtimeTraceProjCrossDirectionClause struct {
	Ref       string
	OverlapMS float64
	Direction string
}

// runtimeTraceProjStampCrossDirectionOverlaps (件2) resolves the wire pair
// rosters into display clauses. Resolution discipline (all typed, never a
// name/value guess):
//   - the partner is the ONE row whose node carries the entry's VERBATIM line
//     envelope, the SAME canonical subject (the axiom keys on one thread) and
//     the SAME rank board (跨板→不发; the shared XLANE-2 board gate);
//   - ambiguity, a missing evidence tag on EITHER side, or a missing reverse
//     entry drops the pair on BOTH sides (both-or-neither — the 互指句 is a
//     PAIR promise; a one-sided sentence would fake reciprocity);
//   - a partner whose own published fix_direction contradicts the entry's
//     direction token is a stale-artifact replay and drops the pair
//     (verbatim typed identity, 宁漏勿假指).
func runtimeTraceProjStampCrossDirectionOverlaps(model *runtimeTraceProjTreeModel) {
	all := runtimeTraceProjSMR1AllRows(model)
	boardIndex := runtimeTraceProjRankBoardIndexFor(all)
	type tentativeClause struct {
		row, partner *runtimeTraceProjTreeRow
		overlapMS    float64
		direction    string
	}
	var tentative []tentativeClause
	for _, row := range all {
		if !row.HasData || len(row.Node.CrossDirectionOverlaps) == 0 {
			continue
		}
		if strings.TrimSpace(row.EvidenceTag) == "" {
			continue // the partner could never point back (宁漏勿假指)
		}
		for _, overlap := range row.Node.CrossDirectionOverlaps {
			var partner *runtimeTraceProjTreeRow
			ambiguous := false
			for _, cand := range all {
				if cand == row || !cand.HasData {
					continue
				}
				if cand.Node.LineStart != overlap.LineStart || cand.Node.LineEnd != overlap.LineEnd {
					continue
				}
				if runtimeTraceCausalProjectionCanonicalNode(cand.Node.Subject) !=
					runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) {
					continue
				}
				if !runtimeTraceProjSemanticSubsetSameBoard(boardIndex, row, cand) {
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
			if fd := strings.TrimSpace(partner.Node.FixDirection); fd != "" && fd != strings.TrimSpace(overlap.Direction) {
				continue // stale persisted form: the entry and the partner disagree
			}
			tentative = append(tentative, tentativeClause{
				row: row, partner: partner,
				overlapMS: overlap.OverlapMS, direction: strings.TrimSpace(overlap.Direction),
			})
		}
	}
	// Both-or-neither: keep a clause only when the PARTNER also resolved a
	// clause pointing back at this row (the engine mints symmetric entries;
	// any one-sided display survival — a hidden/ambiguous/tag-less side —
	// must drop the pair entirely).
	reciprocal := make(map[[2]*runtimeTraceProjTreeRow]bool, len(tentative))
	for _, clause := range tentative {
		reciprocal[[2]*runtimeTraceProjTreeRow{clause.row, clause.partner}] = true
	}
	for _, clause := range tentative {
		if !reciprocal[[2]*runtimeTraceProjTreeRow{clause.partner, clause.row}] {
			continue
		}
		next := runtimeTraceProjCrossDirectionClause{
			Ref:       strings.TrimSpace(clause.partner.EvidenceTag),
			OverlapMS: clause.overlapMS,
			Direction: clause.direction,
		}
		// ANSWERFACE-1 件7 (§29.144 P3-3 「61839 同 chip 双条」, 2026-07-19):
		// a display row that aggregates more than one engine item's overlap
		// roster (family merge / ×N) can resolve two wire entries to the SAME
		// partner with identical values — every per-row emitter (the ·∩[E#]
		// chip and the 同段重叠 tree sentence) would then print the same text
		// twice. Typed same-chip key = the full clause identity (Ref +
		// OverlapMS + Direction) — the same key the footnote's seen-map
		// already collapses; clauses with DIVERGING values are different
		// accounts and both stay (honest, never summed).
		dup := false
		for _, have := range clause.row.CrossDirectionOverlapClauses {
			if have.Ref == next.Ref && have.OverlapMS == next.OverlapMS && have.Direction == next.Direction {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		clause.row.CrossDirectionOverlapClauses = append(clause.row.CrossDirectionOverlapClauses, next)
	}
}

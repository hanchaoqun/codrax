package tool

// answer_document_mutation_runtime_partsplit.go — PARTSPLIT-1 (§29.150④ user
// ruling, 2026-07-19): the R4-mirror-refused gated composite seat's pre-edge-
// share DISCLOSURE faces (LEVELMERGE 披露拆分范式 — split the MEASURE for
// disclosure, never the published authority).
//
// Background: the ONCHAIN-3c inversion arm (rank_state_edge_anchor.go)
// bisects a gated composite seat's runnable census inventory at the host's
// own credential-edge boundary; any post-edge share makes the R4-mirror gate
// REFUSE the lane conversion (the gated eff is an indivisible composite —
// RSPA R4/§29.83 件③), which used to leave the pre-edge share invisible (the
// tieba 23088 live form: 13.959ms pre-edge of a 13.979ms account, whole seat
// in the candidate pool). This file renders the refusal record WITHOUT
// touching the seat's value/lane/ordinal:
//   面1 — the seat-row 行2 分账 sub-line (when the refused seat renders);
//   面2 — the ◎ chain-section NON-SEAT mention rows (the SPANVIS ◈ two-face
//         precedent: the disclosure survives even when the refused seat died
//         at the publication cap), no ordinal, never inside a section head's
//         max-eliminable, never in any conservation/census denominator.
//
// This file is deliberately self-contained (CALSIDE-1 battlefield isolation:
// runtime_elim.go's footnote composer area and runtime_tree.go's ⌗ value
// column area stay untouched; the two host files carry minimal call-site
// hunks only). One wording family, one mark
// (runtimeTraceProjMarkGatedCompositeEdgeShare) for both faces.

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeTraceProjGatedCompositeEdgeShareIdentityTolMs is the print-quantum
// identity tolerance for X + Y == account (each of the three values prints at
// "%.3f", so the combined rounding noise is bounded by 2µs — print-quantum
// noise only, never an identity tolerance borrowed across semantics; the
// engine identity is exact by construction).
const runtimeTraceProjGatedCompositeEdgeShareIdentityTolMs = 0.002

// runtimeTraceProjGatedCompositeEdgeShareNodeAdmitted is the seat-face typed
// admission gate (PRECISE signals only): the atomic four-field refusal record
// is present AND the X+Y identity re-validates against the row's OWN runnable
// account (宁漏勿假指 — an inherited or diverged record silently never
// renders instead of lying).
func runtimeTraceProjGatedCompositeEdgeShareNodeAdmitted(node types.TraceCausalProjectionNode) bool {
	if node.GatedCompositeEdgePreShareMS <= 0 || node.GatedCompositeEdgePostShareMS <= 0 ||
		node.GatedCompositeEdgeAnchorTS <= 0 {
		return false
	}
	switch strings.TrimSpace(node.GatedCompositeEdgeAnchorVia) {
	case "direct", "chain_hop", "direct+chain_hop": // the R3 closed set
	default:
		return false
	}
	if node.RunnableMS <= 0 {
		return false
	}
	diff := node.GatedCompositeEdgePreShareMS + node.GatedCompositeEdgePostShareMS - node.RunnableMS
	return diff <= runtimeTraceProjGatedCompositeEdgeShareIdentityTolMs &&
		diff >= -runtimeTraceProjGatedCompositeEdgeShareIdentityTolMs
}

// runtimeTraceProjGatedCompositeEdgeShareHead is the shared family head — the
// ONE word face both the 行2 sub-line and the ◎ mention block open with (词面
// 单点; the legend probe token).
func runtimeTraceProjGatedCompositeEdgeShareHead(zh bool) string {
	if zh {
		return "边前份披露(R4拒转·整席不拆)"
	}
	return "pre-edge share disclosure (R4 refused conversion · whole seat unsplit)"
}

// runtimeTraceProjGatedCompositeEdgeShareTagText builds the seat-row 行2 分账
// sub-line (the 有效归因 V=… decomposition-line family: value facts + the
// non-addition rider on one sub-line). ok=false renders nothing (admission
// gate above). Marks the shared legend entry on emission.
func runtimeTraceProjGatedCompositeEdgeShareTagText(row runtimeTraceProjTreeRow, zh bool) (string, bool) {
	node := row.Node
	if !runtimeTraceProjGatedCompositeEdgeShareNodeAdmitted(node) {
		return "", false
	}
	if row.marks != nil {
		row.marks.mark(runtimeTraceProjMarkGatedCompositeEdgeShare)
	}
	via := strings.TrimSpace(node.GatedCompositeEdgeAnchorVia)
	if zh {
		return fmt.Sprintf("%s:其中边前份 %.3fms(唤醒边前,凭证:R3 边凭证,最晚凭证边 %.6fs·%s)· 边后份 %.3fms(边界后,不入链上)· 边前+边后=本席 runnable 账 %.3fms 恒等;边前份与本席已发布值同段,不相加",
			runtimeTraceProjGatedCompositeEdgeShareHead(true),
			node.GatedCompositeEdgePreShareMS, node.GatedCompositeEdgeAnchorTS,
			runtimeTraceProjHostEdgeViaWordZH(via), node.GatedCompositeEdgePostShareMS,
			node.RunnableMS), true
	}
	return fmt.Sprintf("%s: %.3fms pre-edge (before the wakeup edge; credential: the R3 edge credential, latest credential edge %.6fs, via=%s) · %.3fms post-edge (after the boundary — never on-chain) · pre + post == this seat's runnable account %.3fms; the pre-edge share covers the same segments as the seat's published value, never additive",
		runtimeTraceProjGatedCompositeEdgeShareHead(false),
		node.GatedCompositeEdgePreShareMS, node.GatedCompositeEdgeAnchorTS, via,
		node.GatedCompositeEdgePostShareMS, node.RunnableMS), true
}

// runtimeTraceProjGatedCompositeEdgeShareDisclosureAdmitted re-validates one
// side-channel record at render time (the projection parser already enforced
// the identity; re-checking here keeps the render honest against any future
// carrier).
func runtimeTraceProjGatedCompositeEdgeShareDisclosureAdmitted(d types.TraceCausalProjectionGatedCompositeEdgeShareDisclosure) bool {
	if strings.TrimSpace(d.Subject) == "" || d.PreMS <= 0 || d.PostMS <= 0 ||
		d.AccountMS <= 0 || d.AnchorTS <= 0 {
		return false
	}
	switch strings.TrimSpace(d.Via) {
	case "direct", "chain_hop", "direct+chain_hop":
	default:
		return false
	}
	diff := d.PreMS + d.PostMS - d.AccountMS
	return diff <= runtimeTraceProjGatedCompositeEdgeShareIdentityTolMs &&
		diff >= -runtimeTraceProjGatedCompositeEdgeShareIdentityTolMs
}

// runtimeTraceProjGatedCompositeEdgeShareResolveRef resolves the disclosure
// subject to a rendered seat row's [E#] tag — all-or-nothing (宁漏勿假指): a
// pointer mints only when a rendered row carries the SAME refusal record
// (typed anchor match, canonical-subject match) and a non-empty tag.
func runtimeTraceProjGatedCompositeEdgeShareResolveRef(model runtimeTraceProjTreeModel, d types.TraceCausalProjectionGatedCompositeEdgeShareDisclosure) string {
	subjectKey := runtimeTraceCausalProjectionCanonicalNode(d.Subject)
	if subjectKey == "" {
		return ""
	}
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for i := range rows {
			node := rows[i].Node
			if node.GatedCompositeEdgeAnchorTS != d.AnchorTS ||
				!runtimeTraceProjGatedCompositeEdgeShareNodeAdmitted(node) {
				continue
			}
			if runtimeTraceCausalProjectionCanonicalNode(node.Subject) != subjectKey {
				continue
			}
			if tag := strings.TrimSpace(rows[i].EvidenceTag); tag != "" {
				return tag
			}
		}
	}
	return ""
}

// runtimeTraceProjElimGatedCompositeEdgeShareMentionLines builds the ◎
// chain-section NON-SEAT mention block (面2): a head line, the shared
// non-addition rider, and one indented row per admitted disclosure (the ◈
// footnote family form). The rows were never board entries, so section
// max-eliminable heads, subtotals, conservation and census denominators stay
// structurally untouched. Empty admission → zero bytes (absence silent).
func runtimeTraceProjElimGatedCompositeEdgeShareMentionLines(model runtimeTraceProjTreeModel, zh bool) []string {
	if len(model.GatedCompositeEdgeShareDisclosures) == 0 {
		return nil
	}
	var rows []string
	for _, d := range model.GatedCompositeEdgeShareDisclosures {
		if !runtimeTraceProjGatedCompositeEdgeShareDisclosureAdmitted(d) {
			continue
		}
		subject := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(d.Subject, zh))
		if subject == "" {
			continue
		}
		ref := runtimeTraceProjGatedCompositeEdgeShareResolveRef(model, d)
		var b strings.Builder
		if zh {
			fmt.Fprintf(&b, "  %s 边前份 %.3fms · 边后份 %.3fms · 边前+边后=该席 runnable 账 %.3fms 恒等(凭证:R3 边凭证,最晚凭证边 %.6fs·%s)",
				subject, d.PreMS, d.PostMS, d.AccountMS, d.AnchorTS,
				runtimeTraceProjHostEdgeViaWordZH(strings.TrimSpace(d.Via)))
			if ref != "" {
				b.WriteString(" [" + ref + "]")
			}
			if !d.SeatPublished {
				b.WriteString("(该席未入发布榜,账在候选池,席值/车道/序数零动)")
			}
		} else {
			fmt.Fprintf(&b, "  %s pre-edge %.3fms · post-edge %.3fms · pre + post == the seat's runnable account %.3fms (credential: the R3 edge credential, latest credential edge %.6fs, via=%s)",
				subject, d.PreMS, d.PostMS, d.AccountMS, d.AnchorTS, strings.TrimSpace(d.Via))
			if ref != "" {
				b.WriteString(" [" + ref + "]")
			}
			if !d.SeatPublished {
				b.WriteString(" (the seat itself is off the published board — account kept in the candidate pool; seat value/lane/ordinal untouched)")
			}
		}
		rows = append(rows, b.String())
	}
	if len(rows) == 0 {
		return nil
	}
	model.Marks.mark(runtimeTraceProjMarkGatedCompositeEdgeShare)
	var lines []string
	if zh {
		// Wording note: the cross-direction 收益不叠加 word family stays OFF
		// this rider (its probe/legend belong to the ∩ overlap pair) — the
		// non-addition promise here speaks the same-segment fact only.
		lines = append(lines, "· "+runtimeTraceProjGatedCompositeEdgeShareHead(true)+"——非席披露行:不占序数、不参与节头「最大可消」:")
		lines = append(lines, "  边前份与该席自身值同段:仅披露,不与该席值或任何行相加")
	} else {
		lines = append(lines, "· "+runtimeTraceProjGatedCompositeEdgeShareHead(false)+" — non-seat disclosure rows: no ordinal, never inside a section head's max-eliminable:")
		lines = append(lines, "  the pre-edge share covers the same segments as the seat's own value: disclosure only, never additive to the seat or any row")
	}
	return append(lines, rows...)
}

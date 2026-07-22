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
	"sort"
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
// ONE word face the 行2 sub-line opens with and the legend probe token.
// OMGCLEAN-1 件10 (§29.175.12 用户裁定, UX-15 转正, 2026-07-20). EVOLUTION
// RECORD: the head spoke the internal rule number and lane words
// (「R4拒转·整席不拆」/"R4 refused conversion") — user-visible faces carry
// ZERO rule numbers and zero internal-lane vocabulary; the ruled replacement
// closed set applies: R4拒转·整席不拆 → 按口径不拆段入榜. Rule numbers stay in
// code/tests/ledgers only.
func runtimeTraceProjGatedCompositeEdgeShareHead(zh bool) string {
	if zh {
		return "边前份披露(按口径不拆段入榜)"
	}
	return "pre-edge share disclosure (kept whole per its caliber; not split into a board row)"
}

// runtimeTraceProjGatedCompositeEdgeShareTagText builds the seat-row 行2 分账
// sub-line (the 有效归因 V=… decomposition-line family: value facts + the
// non-addition rider on one sub-line). ok=false renders nothing (admission
// gate above). Marks the shared legend entry on emission.
// 件10: 凭证:R3 边凭证 → 凭证:唤醒边 (the via word beside the timestamp keeps
// stating 直接裸边/链上跳边 exactly — the credential strength never inflates).
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
		return fmt.Sprintf("%s:其中边前份 %.3fms(唤醒边前,凭证:唤醒边,最晚凭证边 %.6fs·%s)· 边后份 %.3fms(边界后,不入链上)· 边前+边后=本席 runnable 账 %.3fms 恒等;边前份与本席已发布值同段,不相加",
			runtimeTraceProjGatedCompositeEdgeShareHead(true),
			node.GatedCompositeEdgePreShareMS, node.GatedCompositeEdgeAnchorTS,
			runtimeTraceProjHostEdgeViaWordZH(via), node.GatedCompositeEdgePostShareMS,
			node.RunnableMS), true
	}
	return fmt.Sprintf("%s: %.3fms pre-edge (before the wakeup edge; credential: the wakeup edge, latest credential edge %.6fs, via=%s) · %.3fms post-edge (after the boundary — never on-chain) · pre + post == this seat's runnable account %.3fms; the pre-edge share covers the same segments as the seat's published value, never additive",
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

// runtimeTraceProjElimGatedCompositeEdgeShareMentionRows builds the ◎ face's
// 未入榜最大 auxiliary rows (面2). OMGCLEAN-1 件6+件9 (§29.175.10/.12/.14
// user rulings, 2026-07-20). EVOLUTION RECORD — 涉既裁位移② (§29.150④ 原裁
// verbatim: 「请按推荐的 开 LEVELMERGE 式披露拆分批」; §29.156 定形 put the
// 恒等(凭证…) sentence in ◎): the §29.150④ LEVELMERGE-style 5-line ◎ mention
// block (head + rider + per-disclosure identity line, landed per §29.156 定形
// WITH the 恒等(凭证…) sentence in ◎)
// compresses into ONE 「未入榜最大」 auxiliary row per admitted disclosure —
// the §29.150④ core (⛓-face visibility of the largest
// should-have-seated-but-unranked account) survives as the aux row; the
// pre/post identity rides the row's own inline 括注 (双复核 件2 — see the
// row comment below), the full 分账 sentence additionally renders on the 行2
// face whenever the seat publishes (runtimeTraceProjGatedCompositeEdgeShareTagText),
// and the legend entry carries the full semantics. 件10 word sweep: 候选池
// 最大 → 未入榜最大; R4拒转 → 按口径不拆段入榜; the internal 席值/车道/序数
// 零动 sentence retires (the label itself states the unranked fact). The rows
// were never board entries, so section max-eliminable heads, subtotals,
// conservation and census denominators stay structurally untouched. Empty
// admission → zero rows (absence silent).
// SMALL3-1 件5 (§29.197② user ruling, 2026-07-21): the 未入榜最大 family is a
// proliferable aux family and caps at TOP5 by value — the customer report
// witness (cust_report_xx.txt:253-273) minted ×21 per-seat rows and drowned
// the low-priority auxiliary zone. Value order = the row's own published lead
// value (the seat's typed AccountMS — the §29.175.14 定稿 value), descending,
// stable on ties; the tail folds into ONE ruled 「另有 N 项见明细」 same-level
// row (§29.175.14 文法, the A2 件12 构成拆解 precedent); ≤5 rows render
// without a tail. The kept rows' identity 括注 and word forms are untouched.
const runtimeTraceProjElimGatedCompositeMentionTopN = 5

func runtimeTraceProjElimGatedCompositeEdgeShareMentionRows(model runtimeTraceProjTreeModel, zh bool) []runtimeTraceProjElimAuxRow {
	if len(model.GatedCompositeEdgeShareDisclosures) == 0 {
		return nil
	}
	eligible := make([]types.TraceCausalProjectionGatedCompositeEdgeShareDisclosure, 0, len(model.GatedCompositeEdgeShareDisclosures))
	for _, d := range model.GatedCompositeEdgeShareDisclosures {
		if !runtimeTraceProjGatedCompositeEdgeShareDisclosureAdmitted(d) {
			continue
		}
		if strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(d.Subject, zh)) == "" {
			continue
		}
		eligible = append(eligible, d)
	}
	// 件5: value desc (typed AccountMS), stable — the pre-§29.197② emission
	// order was the disclosure arrival order (pre-edge share desc), which let
	// a 54.896ms account rank below a 33.981ms one on the reading face.
	sort.SliceStable(eligible, func(i, j int) bool {
		return eligible[i].AccountMS > eligible[j].AccountMS
	})
	folded := 0
	if len(eligible) > runtimeTraceProjElimGatedCompositeMentionTopN {
		folded = len(eligible) - runtimeTraceProjElimGatedCompositeMentionTopN
		eligible = eligible[:runtimeTraceProjElimGatedCompositeMentionTopN]
	}
	var rows []runtimeTraceProjElimAuxRow
	for _, d := range eligible {
		subject := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(d.Subject, zh))
		ref := ""
		if tag := runtimeTraceProjGatedCompositeEdgeShareResolveRef(model, d); tag != "" {
			ref = " [" + tag + "]"
		}
		// §29.175.14 row form + OMGCLEAN-1 双复核修复 件2 (冷读 CR2, 主会话裁定
		// 2026-07-21). EVOLUTION RECORD: the row VALUE spoke the pre-edge share
		// (13.982) — the 定稿 value is the SEAT'S OWN ACCOUNT (AccountMS,
		// 14.002 形), and the pre/post identity had evaporated from every user
		// face (the 行2 分账句 renders only for PUBLISHED seats while this
		// lane's subject is exactly the unpublished one). The ruled inline form
		// carries the compact identity 括注 right after the value —
		// 「(唤醒边前 X + 边后 Y)」 — so value + identity live on the ONE row;
		// the 「有唤醒凭证」 word now rides inside the 括注 (唤醒边前 = the
		// wakeup-edge credential word), keeping the row inside the 100-cell
		// same-level-row budget (§29.175.14 禁续行 discipline). Values
		// transcribe the typed disclosure verbatim; 席值/榜序零动.
		if zh {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: "未入榜最大",
				content: fmt.Sprintf("%s %.3fms(唤醒边前 %.3f + 边后 %.3f)· 按口径不拆段入榜%s",
					subject, d.AccountMS, d.PreMS, d.PostMS, ref)})
		} else {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: "unranked max",
				content: fmt.Sprintf("%s %.3fms (pre %.3f + post %.3f) · kept whole per caliber%s",
					subject, d.AccountMS, d.PreMS, d.PostMS, ref)})
		}
	}
	// 件5 (§29.197②): the honest folded tail — same-level row, ruled words.
	if folded > 0 {
		if zh {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: "未入榜最大",
				content: fmt.Sprintf("另有 %d 项见明细", folded)})
		} else {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: "unranked max",
				content: fmt.Sprintf("%d more — see the detail blocks", folded)})
		}
	}
	if len(rows) > 0 {
		model.Marks.mark(runtimeTraceProjMarkGatedCompositeEdgeShare)
	}
	return rows
}

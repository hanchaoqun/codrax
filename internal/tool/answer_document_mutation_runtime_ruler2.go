package tool

// answer_document_mutation_runtime_ruler2.go — RULER2-1 (§29.150② user
// ruling / R-19-b, 2026-07-19): the self runnable account TWO-RULER cross-row
// disclosure sentence.
//
// Background (§29.136 CHAIN-BUDGET, cb_rework P3③): the donghu17267 flagship
// board's former single self runnable seat (5.604ms) split under the
// CHAIN-BUDGET default tier into three published seats on two different
// closed rulers — 3.956 + 1.193 [self_wall_clock] and 1.648 [on_wakeup_chain
// edge-anchored]. Each seat's value/caliber word is honest in isolation; the
// reader-facing gap is the missing CROSS-ROW sentence explaining the split
// (the old ledger said 5.604, the new board's three rows suggest Σ6.797 —
// a mixed-ruler number that must never be printed). This file renders the
// typed engine record (tracequery harvestSelfRunnableTwoRulerAccounting →
// self_runnable_two_ruler observation → projection side channel) as ONE 行2
// sub-line under the LEAD seat row:
//
//   自身runnable账按两把尺记账:自身墙钟尺 2 席(3.956+1.193=5.149ms,同尺内可加)
//   ·唤醒边锚尺 1 席(1.648ms,另一把尺);跨尺不相加,无合计数
//
// M3 禁混尺 red line: NO cross-ruler sum is ever computed or rendered here —
// same-ruler subtotals only (µs identity re-validated before rendering).
// Single-ruler boards stay silent (the §29.136 single-ruler fold precedent's
// faces own that shape); absent carriers stay silent (缺载体静默); the fence
// is recomposed from the typed model on every render, so a replay/re-render
// never stacks the sentence (咽喉幂等纪律).
//
// This file is deliberately self-contained (CALSIDE-1/PARTSPLIT-1 battlefield
// isolation): the host file answer_document_mutation_runtime_tree.go carries
// minimal call-site hunks only (row field, model field, stamp call, composer
// call, mark + legend entry).

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeTraceProjSelfTwoRulerIdentityTolPerValueMs is the per-participating-
// value print-quantum slack of the same-ruler Σ identity re-check (every
// value prints at "%.3f" upstream — n addends + the subtotal bound the honest
// rounding drift by (n+1)×0.5µs; 1µs per value is headroom, never an identity
// tolerance borrowed across semantics).
const runtimeTraceProjSelfTwoRulerIdentityTolPerValueMs = 0.001

// runtimeTraceProjSelfTwoRulerEdgeCausality is the wakeup-edge ruler's
// causality token (the closed-set literal every edge-proven on-chain row
// publishes; the self ruler token is tracequery.RootCauseCausalitySelfWallClock).
const runtimeTraceProjSelfTwoRulerEdgeCausality = "on_wakeup_chain"

// runtimeTraceProjSelfTwoRulerHead is the ONE word face of the family (词面
// 单点; the legend probe token). Wording note: the zh face deliberately
// avoids the substring 分账 — that word is the LEVELMERGE gated-share family
// token (one word, one family; the bidirectional legend sweep enforces it).
func runtimeTraceProjSelfTwoRulerHead(zh bool) string {
	if zh {
		return "自身runnable账按两把尺记账"
	}
	return "self runnable account split across two rulers"
}

// runtimeTraceProjSelfTwoRulerRulerIdentityHolds re-validates ONE ruler's
// same-ruler Σ identity at render time (µs; the projection parser already
// enforced it — re-checking keeps the render honest against any future
// carrier).
func runtimeTraceProjSelfTwoRulerRulerIdentityHolds(effs []float64, ranks []int, subtotal float64) bool {
	if len(effs) == 0 || len(effs) != len(ranks) || !(subtotal > 0) {
		return false
	}
	sum := 0.0
	for i := range effs {
		if !(effs[i] > 0) || ranks[i] <= 0 {
			return false
		}
		sum += effs[i]
	}
	tol := float64(len(effs)+1) * runtimeTraceProjSelfTwoRulerIdentityTolPerValueMs
	diff := sum - subtotal
	return diff <= tol && diff >= -tol
}

// runtimeTraceProjSelfTwoRulerRecordAdmitted is the typed render-time
// admission gate (PRECISE signals only): both rulers occupied, parallel
// eff/rank lists, every value positive, and BOTH same-ruler Σ identities
// hold (宁漏勿假指 — a diverged record silently never renders instead of
// lying).
func runtimeTraceProjSelfTwoRulerRecordAdmitted(record types.TraceCausalProjectionSelfRunnableTwoRuler) bool {
	return types.TraceCausalProjectionSelfRunnableTwoRulerValid(record)
}

// runtimeTraceProjSelfTwoRulerLead returns the record's LEAD seat — the
// lowest board ordinal across both rulers — as (rank, eff, causality token).
// The lead seat's row hosts the cross-row sentence (三席中首席行下披露行).
func runtimeTraceProjSelfTwoRulerLead(record types.TraceCausalProjectionSelfRunnableTwoRuler) (int, float64, string) {
	leadRank, leadEff, leadCausality := 0, 0.0, ""
	scan := func(effs []float64, ranks []int, causality string) {
		for i := range ranks {
			if leadRank == 0 || ranks[i] < leadRank {
				leadRank, leadEff, leadCausality = ranks[i], effs[i], causality
			}
		}
	}
	scan(record.WallEffsMS, record.WallRanks, tracequery.RootCauseCausalitySelfWallClock)
	scan(record.EdgeEffsMS, record.EdgeRanks, runtimeTraceProjSelfTwoRulerEdgeCausality)
	return leadRank, leadEff, leadCausality
}

// runtimeTraceProjSelfTwoRulerNodeEff is the row's published display value
// (the duration triad's first positive member — eff==cum==imp on runnable
// wall-clock seats; the µs match against the record's lead value is part of
// the host resolution).
func runtimeTraceProjSelfTwoRulerNodeEff(node types.TraceCausalProjectionNode) float64 {
	for _, v := range []float64{node.EffectiveImpactMS, node.CumulativeImpactMS, node.ImpactMS} {
		if v > 0 {
			return v
		}
	}
	return 0
}

// runtimeTraceProjStampSelfRunnableTwoRuler resolves each admitted record's
// LEAD seat row and stamps the record onto it — all-or-nothing (宁漏勿假指):
// the stamp mints only when EXACTLY ONE rendered row matches the typed host
// key (canonical subject ∧ on_chain lane ∧ the lead ordinal ∧ the lead ruler
// causality ∧ µs value match). No match or an ambiguous match stamps nothing
// (缺载体静默 — the record never guesses a host).
func runtimeTraceProjStampSelfRunnableTwoRuler(model *runtimeTraceProjTreeModel) {
	for i := range model.SelfRunnableTwoRulerAccountings {
		record := model.SelfRunnableTwoRulerAccountings[i]
		if !runtimeTraceProjSelfTwoRulerRecordAdmitted(record) {
			continue
		}
		subjectKey := runtimeTraceCausalProjectionCanonicalNode(record.Subject)
		if subjectKey == "" {
			continue
		}
		leadRank, leadEff, leadCausality := runtimeTraceProjSelfTwoRulerLead(record)
		var host *runtimeTraceProjTreeRow
		hosts := 0
		for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
			for j := range rows {
				node := rows[j].Node
				if node.Rank != leadRank || strings.TrimSpace(node.ChainRelevance) != "on_chain" ||
					strings.TrimSpace(node.Causality) != leadCausality {
					continue
				}
				if runtimeTraceCausalProjectionCanonicalNode(node.Subject) != subjectKey {
					continue
				}
				eff := runtimeTraceProjSelfTwoRulerNodeEff(node)
				if diff := eff - leadEff; diff > runtimeTraceProjSelfTwoRulerIdentityTolPerValueMs ||
					diff < -runtimeTraceProjSelfTwoRulerIdentityTolPerValueMs {
					continue
				}
				host = &rows[j]
				hosts++
			}
		}
		if hosts != 1 {
			continue
		}
		host.SelfRunnableTwoRuler = &model.SelfRunnableTwoRulerAccountings[i]
		// DISPHYG-3 件6: the sentence's non-lead participants gain their
		// compact-face cross-reference markers (lead-stamped records only).
		runtimeTraceProjStampSelfTwoRulerParticipants(model, record, leadRank)
	}
}

// runtimeTraceProjSelfTwoRulerRulerClause renders ONE ruler's value clause:
// a lone seat prints its value; ≥2 seats print the addends and the
// same-ruler subtotal (X+Y=S — arithmetic self-consistent at the print
// quantum by the admission gate above). NEVER a cross-ruler computation.
func runtimeTraceProjSelfTwoRulerRulerClause(effs []float64, subtotal float64, zh bool) string {
	if len(effs) == 1 {
		return fmt.Sprintf("%.3fms", effs[0])
	}
	parts := make([]string, 0, len(effs))
	for _, eff := range effs {
		parts = append(parts, fmt.Sprintf("%.3f", eff))
	}
	if zh {
		return strings.Join(parts, "+") + fmt.Sprintf("=%.3fms,同尺内可加", subtotal)
	}
	return strings.Join(parts, "+") + fmt.Sprintf("=%.3fms, additive within one ruler", subtotal)
}

// runtimeTraceProjSelfRunnableTwoRulerTagText builds the 行2 cross-row
// two-ruler accounting sentence for the stamped LEAD seat row (the 有效归因
// decomposition-line family). ok=false renders nothing (absent stamp or a
// record failing the re-validation). Marks the shared legend entry on
// emission.
func runtimeTraceProjSelfRunnableTwoRulerTagText(row runtimeTraceProjTreeRow, zh bool) (string, bool) {
	record := row.SelfRunnableTwoRuler
	if record == nil || !runtimeTraceProjSelfTwoRulerRecordAdmitted(*record) {
		return "", false
	}
	if row.marks != nil {
		row.marks.mark(runtimeTraceProjMarkSelfRunnableTwoRuler)
	}
	wallClause := runtimeTraceProjSelfTwoRulerRulerClause(record.WallEffsMS, record.WallSubtotalMS, zh)
	edgeClause := runtimeTraceProjSelfTwoRulerRulerClause(record.EdgeEffsMS, record.EdgeSubtotalMS, zh)
	if zh {
		return fmt.Sprintf("%s:自身墙钟尺 %d 席(%s)·唤醒边锚尺 %d 席(%s,另一把尺);跨尺不相加,无合计数",
			runtimeTraceProjSelfTwoRulerHead(true),
			len(record.WallEffsMS), wallClause, len(record.EdgeEffsMS), edgeClause), true
	}
	seatWord := func(n int) string {
		if n == 1 {
			return "seat"
		}
		return "seats"
	}
	return fmt.Sprintf("%s: self wall-clock ruler %d %s (%s) · wakeup-edge-anchored ruler %d %s (%s, the other ruler); never additive across rulers, no combined total",
		runtimeTraceProjSelfTwoRulerHead(false),
		len(record.WallEffsMS), seatWord(len(record.WallEffsMS)), wallClause,
		len(record.EdgeEffsMS), seatWord(len(record.EdgeEffsMS)), edgeClause), true
}

// runtimeTraceProjStampSelfTwoRulerParticipants (DISPHYG-3 件6, §29.158 P3-2
// 紧凑面对照摩擦, 2026-07-20) stamps the record's NON-lead participant seats
// onto their rendered rows so the compact face (a merged state row that
// absorbed a board seat WITHOUT its ordinal — node.Rank==0, no 行2 identity
// line) gains a minimal cross-reference marker against the sentence's
// 「N 席」 claim. Typed keys, all-or-nothing per participant (宁漏勿假指):
// canonical subject ∧ on_chain lane ∧ runnable state class (the record IS the
// self runnable account) ∧ µs value match ∧ node.Rank==0 (a row already
// wearing its own board ordinal never gets a second chip — the §29.36.2
// one-chip discipline) — and EXACTLY ONE rendered row may match; ambiguity
// stamps nothing. Causality is deliberately NOT a key here: the merged
// survivor row keeps ITS OWN causality token (donghu E5 witness:
// on_wakeup_chain on the state survivor absorbing the self_wall_clock seat),
// so requiring ruler-causality equality would silently kill the flagship
// shape. Runs only for records whose LEAD host stamped — the marker exists
// solely as the sentence's cross-reference and never renders alone.
func runtimeTraceProjStampSelfTwoRulerParticipants(model *runtimeTraceProjTreeModel,
	record types.TraceCausalProjectionSelfRunnableTwoRuler, leadRank int) {
	subjectKey := runtimeTraceCausalProjectionCanonicalNode(record.Subject)
	stampOne := func(rank int, eff float64) {
		if rank <= 0 || rank == leadRank {
			return
		}
		var host *runtimeTraceProjTreeRow
		hosts := 0
		for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
			for j := range rows {
				node := rows[j].Node
				if node.Rank != 0 || strings.TrimSpace(node.ChainRelevance) != "on_chain" {
					continue
				}
				if types.TraceCausalProjectionStateClass(node.StateKind) != "runnable" {
					continue
				}
				if runtimeTraceCausalProjectionCanonicalNode(node.Subject) != subjectKey {
					continue
				}
				v := runtimeTraceProjSelfTwoRulerNodeEff(node)
				if diff := v - eff; diff > runtimeTraceProjSelfTwoRulerIdentityTolPerValueMs ||
					diff < -runtimeTraceProjSelfTwoRulerIdentityTolPerValueMs {
					continue
				}
				host = &rows[j]
				hosts++
			}
		}
		// Exactly one host, not already stamped by another participant.
		if hosts != 1 || host.SelfTwoRulerParticipantRank != 0 {
			return
		}
		host.SelfTwoRulerParticipantRank = rank
	}
	for i := range record.WallRanks {
		stampOne(record.WallRanks[i], record.WallEffsMS[i])
	}
	for i := range record.EdgeRanks {
		stampOne(record.EdgeRanks[i], record.EdgeEffsMS[i])
	}
}

// runtimeTraceProjSelfTwoRulerParticipantChip renders the stamped compact-face
// cross-reference marker — the CHANNEL-worded board ordinal (根因排序#N; the
// §29.36.2 禁裸 #N invariant applies to this chip too, so the minimal form
// still carries the chain-channel word). "" without a stamp.
func runtimeTraceProjSelfTwoRulerParticipantChip(row runtimeTraceProjTreeRow, zh bool) string {
	if row.SelfTwoRulerParticipantRank <= 0 {
		return ""
	}
	if zh {
		return fmt.Sprintf("%s#%d", tracefence.SeatChannelChainZH, row.SelfTwoRulerParticipantRank)
	}
	return fmt.Sprintf("%s #%d", tracefence.SeatChannelChainEN, row.SelfTwoRulerParticipantRank)
}

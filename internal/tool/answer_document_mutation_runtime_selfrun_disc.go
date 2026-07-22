package tool

// answer_document_mutation_runtime_selfrun_disc.go — SELFRUN-DISC (§29.192①
// (b) user ruling; A2 件11(b) handoff §29.194, 2026-07-21): the self
// supply-fold 「量不了」 absence disclosure face.
//
// Background: the ELIM-SELF-FIX self running fold seat (§29.93) mints ONLY on
// a positive deficit, and the zero-deficit path folded TWO different zeros
// into one silence — the truly-full-frequency zero (KnownMs>0, gap 0: real
// "no loss") and the fully-unknown-basis zero (KnownMs==0 ∧ UnknownMs>0:
// every slice booked at ratio 1 because frequency data was never collected).
// The second form proves NOTHING about losses, and its silence dressed
// "unmeasurable" up as "no loss". The engine now mints the typed
// SelfRunningFoldUnmeasuredDisclosure on exactly that form
// (rank_self_running_fold.go), the wire carries it as the NON-SEAT
// self_running_fold_unmeasured side-channel record, and THIS file renders it
// as one ◎ auxiliary 另账 row — no seat, no ordinal, never inside a section
// maximum, never in any conservation/census denominator.
//
// This file is deliberately self-contained (PARTSPLIT battlefield-isolation
// precedent): runtime_elim.go carries a minimal call-site hunk per aux
// assembly branch and runtime_tree.go carries the model field + population
// only. One wording family, single-sourced below (词面单点 zh/EN).

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeTraceProjSelfFoldUnmeasuredIdentityTolMs is the print-quantum
// identity tolerance for running == unknown (both values print at "%.3f", so
// two roundings bound the honest drift by 1µs; 2µs allows headroom — never an
// identity tolerance borrowed across semantics; the engine identity is exact
// by construction: KnownMs==0 ⇒ UnknownMs==RunningMs).
const runtimeTraceProjSelfFoldUnmeasuredIdentityTolMs = 0.002

// runtimeTraceProjSelfFoldUnmeasuredSentence is THE single word face of the
// disclosure (§29.192① (b) ruled wording): the absence sentence that
// distinguishes 「量不了」 from 「无损失」. Every consumer speaks these exact
// bytes; no re-spelling.
func runtimeTraceProjSelfFoldUnmeasuredSentence(zh bool) string {
	if zh {
		return "运行频点未采集,自身降频折算不可量"
	}
	return "running-frequency samples were not collected; the self down-clock fold is unmeasurable"
}

// runtimeTraceProjSelfFoldUnmeasuredAdmitted re-validates one side-channel
// record at render time (the projection parser already enforced the identity;
// re-checking keeps the render honest against any future carrier): subject
// present, both values positive, and the fold identity running == unknown
// within the print quantum — a partially-known basis (running > unknown)
// must NEVER wear the absence sentence (宁缺勿错).
func runtimeTraceProjSelfFoldUnmeasuredAdmitted(d types.TraceCausalProjectionSelfRunningFoldUnmeasured) bool {
	if strings.TrimSpace(d.Subject) == "" || d.RunningMS <= 0 || d.UnknownMS <= 0 {
		return false
	}
	diff := d.RunningMS - d.UnknownMS
	return diff <= runtimeTraceProjSelfFoldUnmeasuredIdentityTolMs &&
		diff >= -runtimeTraceProjSelfFoldUnmeasuredIdentityTolMs
}

// runtimeTraceProjElimSelfFoldUnmeasuredRows builds the ◎ auxiliary zone's
// 折算不可量 另账 rows (§29.175.8 two-column grammar: conditional disclosure
// label + value-first one-sentence content). One row per admitted disclosure;
// empty admission → zero rows (absence silent — the negative arms live at the
// engine mint: a deficit seat or a truly-full-frequency zero never reaches
// this list). The row transcribes the typed values verbatim and mints no
// ordinal, joins no census/conservation denominator.
func runtimeTraceProjElimSelfFoldUnmeasuredRows(model runtimeTraceProjTreeModel, zh bool) []runtimeTraceProjElimAuxRow {
	if len(model.SelfRunningFoldUnmeasured) == 0 {
		return nil
	}
	var rows []runtimeTraceProjElimAuxRow
	for _, d := range model.SelfRunningFoldUnmeasured {
		if !runtimeTraceProjSelfFoldUnmeasuredAdmitted(d) {
			continue
		}
		subject := strings.TrimSpace(runtimeTraceCausalProjectionDisplayNodeName(d.Subject, zh))
		if subject == "" {
			continue
		}
		if zh {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: "折算不可量",
				content: fmt.Sprintf("%s 窗内 running %.3fms:%s",
					subject, d.RunningMS, runtimeTraceProjSelfFoldUnmeasuredSentence(true))})
		} else {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: "fold unmeasurable",
				content: fmt.Sprintf("%s ran %.3fms in-window: %s",
					subject, d.RunningMS, runtimeTraceProjSelfFoldUnmeasuredSentence(false))})
		}
	}
	return rows
}

package orchestrator

import (
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// finalize_repair_arbitration.go — S3' ④ (§29.47.1 user ruling, 2026-07-12,
// docs/design/real_trace_campaign_20260705.md; evolved from the S3 two-draft
// arbitration: the soft prose lanes now dispatch ZERO repair rounds, so the
// arbitration guards the STRICT finalize-local repair lane instead).
//
// A strict repair round must apply the smallest possible edit (the retry
// surface's standing block-patch directive); its patch draft replaces the
// first draft ONLY when strictly better — all three precise criteria hold:
//
//	① the named violations are cleared: no violation kind from the round
//	  that triggered the repair persists on the patch draft;
//	② zero NEW strict-for-bus violations (typed kind-set comparison
//	  against the first draft's full contract result; information-lane
//	  soft findings never drive draft selection — P2-2);
//	③ no block / facet-coverage loss (block-id and facet-id set
//	  comparison; required-block regressions additionally surface as
//	  ②-visible kinds).
//
// Any criterion failing → the FIRST draft ships (发第一稿+附注): the patch
// is discarded, the retained draft is restored through the production
// chokepoint, and the system cross-check appendix carries the one-line
// arbitration note (③ of the ruling: the first-draft retention region
// never duplicates the shipped answer — the note replaces it).
//
// 红线: the arbitration is precise-signal SELECTION between two
// model-authored drafts — the system edits neither. Only finalize-local
// repair rounds arm it; upstream (re-explore / re-extract) fallbacks never
// do: fresh evidence legitimately reshapes the answer.

// answerDocFacetSet returns the typed facet-id union over a document's
// blocks (criterion ③'s coverage face).
func answerDocFacetSet(doc *types.AnswerDocumentV2) map[string]bool {
	out := map[string]bool{}
	if doc == nil {
		return out
	}
	for _, blk := range doc.Blocks {
		for _, facet := range blk.FacetIDs {
			if facet = strings.TrimSpace(facet); facet != "" {
				out[facet] = true
			}
		}
	}
	return out
}

// answerDocBlockIDSet returns the block-id set (criterion ③'s 丢块 face).
func answerDocBlockIDSet(doc *types.AnswerDocumentV2) map[string]bool {
	out := map[string]bool{}
	if doc == nil {
		return out
	}
	for _, blk := range doc.Blocks {
		if id := strings.TrimSpace(blk.ID); id != "" {
			out[id] = true
		}
	}
	return out
}

// violationKindSet folds a violation list into its kind set.
func violationKindSet(violations []types.Violation) map[types.ViolationKind]bool {
	out := map[types.ViolationKind]bool{}
	for _, v := range violations {
		out[v.Kind] = true
	}
	return out
}

// finalizeRepairArbitration is the armed state of one finalize-local strict
// repair lane: the retained first draft plus its precise comparison
// baselines. Armed once per task — later rounds keep comparing against the
// FIRST draft (发第一稿 names the original, §29.47.1 ④).
type finalizeRepairArbitration struct {
	// record indexes the FRCAP draft ledger entry retaining the first draft
	// (answer + doc snapshot + violations).
	record int
	// named is the kind set of the violations that triggered the repair
	// (criterion ① baseline).
	named map[types.ViolationKind]bool
	// kinds is the first draft's full contract-violation kind set
	// (criterion ② baseline).
	kinds map[types.ViolationKind]bool
	// facets is the first draft's block facet union (criterion ③).
	facets map[string]bool
	// blocks is the first draft's block-id set (criterion ③, 丢块).
	blocks map[string]bool
}

// armFinalizeRepairArbitration arms the arbitration for the finalize-local
// repair round being dispatched, or returns nil when the first draft was
// not retained (blank-answer bound etc. — nothing safe to compare against).
func armFinalizeRepairArbitration(retryViolations, allViolations []types.Violation, ledger []finalizeRepairDraftRecord, out *agent.StageOutput, doc *types.AnswerDocumentV2) *finalizeRepairArbitration {
	if len(retryViolations) == 0 || out == nil || len(ledger) == 0 {
		return nil
	}
	rec := len(ledger) - 1
	if ledger[rec].Hash != finalizeRepairDraftHash(out.FinalAnswer) {
		return nil
	}
	return &finalizeRepairArbitration{
		record: rec,
		named:  violationKindSet(retryViolations),
		kinds:  violationKindSet(allViolations),
		facets: answerDocFacetSet(doc),
		blocks: answerDocBlockIDSet(doc),
	}
}

// arbitrateFinalizeRepairDraft applies the ①②③ criteria to the accepted
// patch draft (the current out/doc + its full contract violations). Returns
// restored=true when the FIRST draft was restored into out (the caller
// accepts it immediately); note carries the one-line arbitration note the
// system cross-check appendix renders. Advisory log both ways.
func (o *Orchestrator) arbitrateFinalizeRepairDraft(arb *finalizeRepairArbitration, ledger []finalizeRepairDraftRecord, out *agent.StageOutput, secondViolations []types.Violation) (string, bool) {
	if o == nil || arb == nil || out == nil || arb.record < 0 || arb.record >= len(ledger) {
		return "", false
	}
	doc := o.busCtx.Mutable.AnswerDocumentV2()
	var reasons []string
	secondKinds := violationKindSet(secondViolations)
	// ① the named violations are cleared on the patch draft.
	for kind := range arb.named {
		if secondKinds[kind] {
			reasons = append(reasons, "named violation kind persists: "+string(kind))
		}
	}
	// ② zero NEW strict-for-bus violations (P2-2 narrowing, 2026-07-12):
	// information-lane soft findings jitter round to round (reviewer kinds,
	// prose scans) and must never drive draft selection — they disclose on
	// the appendix either way. Only a typed strict-for-bus newcomer vetoes
	// the patch draft.
	newStrict := map[types.ViolationKind]bool{}
	for _, v := range secondViolations {
		if arb.kinds[v.Kind] || newStrict[v.Kind] || !isStrictViolationForBus(v, o.busCtx) {
			continue
		}
		newStrict[v.Kind] = true
		reasons = append(reasons, "new strict violation kind "+string(v.Kind))
	}
	// ③ block / facet coverage never shrinks.
	secondFacets := answerDocFacetSet(doc)
	for facet := range arb.facets {
		if !secondFacets[facet] {
			reasons = append(reasons, "lost facet coverage "+facet)
		}
	}
	secondBlocks := answerDocBlockIDSet(doc)
	for id := range arb.blocks {
		if !secondBlocks[id] {
			reasons = append(reasons, "lost block "+id)
		}
	}
	sort.Strings(reasons)
	if len(reasons) == 0 {
		logging.Info("[orchestrator] finalize repair arbitration: patch draft is strictly better (named cleared, no new violations, coverage kept) — shipping the patch")
		return "", false
	}
	rec := &ledger[arb.record]
	if !o.restoreFinalizeRepairDraft(out, rec) {
		logging.Warning("[orchestrator] finalize repair arbitration: patch draft degraded (%s) but the first draft could not be restored — shipping the patch", strings.Join(reasons, "; "))
		return "", false
	}
	logging.Info("[orchestrator] finalize repair arbitration: patch draft degraded (%s) — shipping the FIRST draft (发第一稿+附注)", strings.Join(reasons, "; "))
	return finalizeRepairArbitrationNote(o.busCtx.Language), true
}

// finalizeRepairArbitrationNote is the one-line note the system cross-check
// appendix renders when the arbitration kept the first draft.
func finalizeRepairArbitrationNote(lang string) string {
	if isChineseLang(lang) {
		return "修复稿在对照校验中未能严格改善（点名问题未清除、丢失内容或引入新问题），已保留第一稿作为最终答案。"
	}
	return "The repair draft did not strictly improve on the first draft (named findings persisting, content lost, or new issues introduced); the first draft ships as the final answer."
}

// finishArbitrationRestoredAnswer applies the standard accept-path caveat
// appends + live-preview cleanup to the restored FIRST draft (the caller
// marks the node done and breaks — same shape as the pass path).
func (o *Orchestrator) finishArbitrationRestoredAnswer(out *agent.StageOutput) {
	if o == nil || out == nil {
		return
	}
	// The restored draft's residual STRICT concerns disclose on the system
	// cross-check appendix (P2-2, injectResidualConcernsCaveat family); its
	// soft findings re-scan there directly. No extra caveat channel here.
	out.FinalAnswer = o.appendSystemCaveatsToAnswer(out.FinalAnswer)
	o.emit(render.Event{
		Kind:            render.EventLivePreviewClear,
		Timestamp:       time.Now(),
		Stage:           types.StageFinalize,
		PreviewRejected: false,
	})
}

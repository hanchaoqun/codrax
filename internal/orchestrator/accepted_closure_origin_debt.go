package orchestrator

// accepted_closure_origin_debt.go — the mixed-origin evidence-lane debt gate
// for accepted investigation closures (split out of orchestrator.go under the
// IR delivery hot-file ratchet, COMPLETE-2 / §29.140 GAP-4).
//
// When the model's emit_investigation_complete is ACCEPTED, the scheduler may
// still refuse to auto-complete a mixed-origin explore window / reconcile node
// when a required typed evidence lane (e.g. current_source next to
// runtime_artifact) has zero ledger observations — that refusal redispatches
// the explorer. Everything in this file decides that debt; the waiver lane
// (§29.140 GAP-4) narrows it with precise typed signals only.

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) acceptedClosureMissingRequiredOriginsForAutoComplete() []types.AnswerEvidenceOrigin {
	required := o.acceptedClosureRequiredOriginLanesBeforeDebtMint()
	if len(required) == 0 {
		return nil
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(o.busCtx, types.ObservationExtractLedgerEvidenceLimit))
	return o.dropWaivedCurrentSourceOriginDebt(missingObservationOrigins(required, ledger))
}

// acceptedClosureRequiredOriginLanesBeforeDebtMint computes the required
// mixed-origin lanes for the two auto-complete gates and applies the
// current_source waiver BEFORE any debt is minted (§29.146 UPSTREAM-3 件1):
// when the typed exclusion lane holds, the current_source obligation is never
// minted at all instead of being minted and then post-filtered away. The
// intent contract is compiled with the Run-entry preflight carrier (件2), so
// a large attached artifact whose triage bundle was size-gated away cannot
// bypass the user's typed exclusion carve either.
func (o *Orchestrator) acceptedClosureRequiredOriginLanesBeforeDebtMint() []types.AnswerEvidenceOrigin {
	if o == nil || o.busCtx == nil || o.busCtx.AnalysisIR == nil {
		return nil
	}
	if o.runtimeSourceAuthoritySuppressesAcceptedClosureOriginDebt() {
		return nil
	}
	rm := o.busCtx.AnalysisIR.RequestModel
	contract := &o.busCtx.AnalysisIR.AnswerContract
	if !parallelExploreMixedOriginNeedsSiblingHandoffs(rm, contract, o.busCtx.RuntimeArtifactPreflight) {
		return nil
	}
	intentContract := types.CompileAnswerIntentContractWithPreflight(rm, contract, o.busCtx.RuntimeArtifactPreflight)
	required := requiredMixedOriginAutoCompleteLanes(intentContract)
	if len(required) == 0 {
		return nil
	}
	return o.withholdWaivedCurrentSourceOriginLaneBeforeDebtMint(required)
}

// withholdWaivedCurrentSourceOriginLaneBeforeDebtMint is the pre-mint half of
// the double defense (§29.146 UPSTREAM-3 件1). It shares the SAME typed
// predicate and reason strings as the post-filter
// (acceptedClosureCurrentSourceOriginDebtWaiverReason — single source of
// truth): when either waiver arm holds, the current_source lane is removed
// from the required set before missingObservationOrigins ever runs, so the
// debt is not minted rather than minted-then-dropped. Every other lane keeps
// its redispatch pressure unchanged — this is a pure relaxation lane and
// never adds a block.
func (o *Orchestrator) withholdWaivedCurrentSourceOriginLaneBeforeDebtMint(required []types.AnswerEvidenceOrigin) []types.AnswerEvidenceOrigin {
	if len(required) == 0 {
		return required
	}
	reason, waived := o.acceptedClosureCurrentSourceOriginDebtWaiverReason()
	if !waived {
		return required
	}
	kept := required[:0]
	withheld := false
	for _, origin := range required {
		if origin == types.AnswerEvidenceOriginCurrentSource {
			withheld = true
			continue
		}
		kept = append(kept, origin)
	}
	if withheld {
		logging.Info("[orchestrator] accepted investigation closure withholds current_source origin debt before minting; reason=%s", reason)
	}
	return kept
}

// dropWaivedCurrentSourceOriginDebt removes ONLY the current_source arm from
// the mixed-origin auto-complete debt when that arm is waived for this run
// (§29.140 GAP-4); every other missing lane keeps its redispatch pressure
// unchanged. Before this filter, an accepted trace-only closure whose request
// explicitly forbade source analysis was still redispatched to hunt
// current_source evidence (witness frame_multicausal-20260719-030504: 3
// emit_investigation_complete rounds, 21 trace_query calls, ~180s extra burn
// per run) even though the terminal completion landed with the identical
// runtime-only caveat — a soft, downgradable lane driving a hard redispatch.
//
// §29.146 UPSTREAM-3 件1: this post-filter is now the INVARIANT BACKSTOP of a
// double defense. The shared waiver predicate is evaluated pre-mint by
// withholdWaivedCurrentSourceOriginLaneBeforeDebtMint, so under normal
// operation this filter never sees a waived current_source lane and its hit
// count stays at 0 — a non-zero post_filter_hits log line means a new minting
// path bypassed the pre-mint decision and must be investigated.
func (o *Orchestrator) dropWaivedCurrentSourceOriginDebt(missing []types.AnswerEvidenceOrigin) []types.AnswerEvidenceOrigin {
	if len(missing) == 0 {
		return missing
	}
	reason, waived := o.acceptedClosureCurrentSourceOriginDebtWaiverReason()
	if !waived {
		return missing
	}
	kept := missing[:0]
	dropped := false
	for _, origin := range missing {
		if origin == types.AnswerEvidenceOriginCurrentSource {
			dropped = true
			continue
		}
		kept = append(kept, origin)
	}
	if dropped {
		logging.Info("[orchestrator] accepted investigation closure waives current_source origin debt; reason=%s post_filter_hits=1 (backstop: pre-mint withhold should have decided this lane)", reason)
	}
	return kept
}

// acceptedClosureCurrentSourceOriginDebtWaiverReason reports whether the
// current_source arm of the mixed-origin auto-complete debt is waived, on two
// precise typed signals that mirror the emit-side completion bypasses:
//
//	① explicit_current_source_exclusion — the analyzer emitted the anchored
//	  user scope boundary (current_source_mode=exclude AND
//	  exclusion_kind=explicit_user_exclusion AND a verbatim SourceQuote;
//	  ExternalObservationPolicy.ExcludesCurrentSource, the same typed signal
//	  behind explicitCurrentSourceExclusionCompletionBypassLabel). The user
//	  instructed "don't analyze code", so redispatching the explore window to
//	  mint current_source observations re-opens work the request forbids.
//	  Under this policy the requirement precision is structurally never
//	  precise (runtimeSourceAuthorityPreciseCurrentSourceRequirement carves
//	  the exclusion first), so the terminal completion already lands with the
//	  runtime-only caveat — first-round acceptance is caveat-equivalent.
//	② zero_current_source_repo — the deterministic run-entry census proved the
//	  checkout holds zero current-source files (RuntimeArtifactPreflight,
//	  same signal as zeroCurrentSourceRepoCompletionBypassLabel): no sequence
//	  of redispatched tool calls can ever produce a current_source line-span
//	  observation, so the debt is structurally unpayable.
//
// Mixed requests that genuinely ask for code+runtime analysis carry neither
// signal (an active CurrentSourceExplanationProfile / typed scope request is
// how that shape is emitted, and a valid exclude policy never coexists with a
// precise source obligation), so their redispatch pressure is preserved —
// negative arm pinned by
// TestAcceptedClosureAutoCompleteBlocksUntilRuntimeCurrentSourceOriginsPresent.
// "Downgradable" alone is deliberately NOT a waiver arm: it is soft/noisy and
// would flip that genuine mixed shape (precise-signals red line).
func (o *Orchestrator) acceptedClosureCurrentSourceOriginDebtWaiverReason() (string, bool) {
	if o == nil || o.busCtx == nil {
		return "", false
	}
	if o.busCtx.AnalysisIR != nil {
		rm := o.busCtx.AnalysisIR.RequestModel
		if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
			return "explicit_current_source_exclusion", true
		}
	}
	if o.busCtx.RuntimeArtifactPreflight.ZeroCurrentSourceRepo() {
		return "zero_current_source_repo", true
	}
	return "", false
}

func (o *Orchestrator) runtimeSourceAuthoritySuppressesAcceptedClosureOriginDebt() bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || o.busCtx.AnalysisIR == nil {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(o.busCtx, types.ObservationExtractLedgerEvidenceLimit))
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(o.busCtx, ledger)
	if !authority.Active || !authority.HasRuntimeCarrier() ||
		authority.CurrentSourceRequired ||
		authority.CanHardBlockCompletion ||
		authority.CurrentSourceRequirement == types.RuntimeSourceRequirementPrecise {
		return false
	}
	if authority.CurrentSourceLane == types.CurrentSourceLaneExcluded {
		return true
	}
	if authority.KeepsCurrentSourceLaneLoadBearing() {
		return false
	}
	return authority.AllowsRuntimeEvidenceWithoutCurrentSource()
}

func requiredMixedOriginAutoCompleteLanes(contract types.AnswerIntentContract) []types.AnswerEvidenceOrigin {
	hasCurrent := false
	hasNonSource := false
	for _, origin := range contract.Origins {
		switch {
		case origin == types.AnswerEvidenceOriginCurrentSource:
			hasCurrent = true
		case origin != types.AnswerEvidenceOriginUnknown && types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin):
			hasNonSource = true
		}
	}
	if !hasCurrent || !hasNonSource {
		return nil
	}
	var out []types.AnswerEvidenceOrigin
	for _, origin := range contract.Origins {
		if origin == types.AnswerEvidenceOriginCurrentSource || types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
			out = append(out, origin)
		}
	}
	return out
}

func missingObservationOrigins(required []types.AnswerEvidenceOrigin, ledger types.ObservationLedger) []types.AnswerEvidenceOrigin {
	if len(required) == 0 {
		return nil
	}
	present := make(map[types.AnswerEvidenceOrigin]bool, len(required))
	for _, record := range ledger.Records {
		switch record.Origin {
		case types.AnswerEvidenceOriginUnknown:
			continue
		case types.AnswerEvidenceOriginCurrentSource:
			// CSP #63 same-family raw side channel, documented deliberately
			// unwired (§29.121): an engine blob-session read (current_source
			// origin, blob path, line span) can count as current_source
			// presence for the mixed-origin auto-complete lanes here. Review
			// 2026-07-17 proved the outcome flow-identical either way — the
			// authority-side suppressor upstream
			// (runtimeSourceAuthoritySuppressesAcceptedClosureOriginDebt)
			// and the mixed-contract gate own the reachable flows — so this
			// is not a behavioral defect. Carving blob paths here would flip
			// presence→missing in the mixed-contract corner, a
			// TIGHTEN-direction change the CSP63-FIX accept-direction batch
			// must not smuggle through the shared
			// ObservationRecordHasCurrentSourceLineSpan lane. If a reachable
			// divergence ever appears, wire types.IsCodraxBlobSessionPath
			// beside the line-span check and pin the flip.
			if types.ObservationRecordHasCurrentSourceLineSpan(record) {
				present[record.Origin] = true
			}
		default:
			if types.AnswerEvidenceOriginCarriesOriginSpecificSupport(record.Origin) {
				present[record.Origin] = true
			}
		}
	}
	var missing []types.AnswerEvidenceOrigin
	for _, origin := range required {
		if origin == types.AnswerEvidenceOriginUnknown || present[origin] {
			continue
		}
		missing = append(missing, origin)
	}
	return missing
}

func formatAnswerEvidenceOriginsForLog(origins []types.AnswerEvidenceOrigin) string {
	if len(origins) == 0 {
		return ""
	}
	parts := make([]string, 0, len(origins))
	for _, origin := range origins {
		if origin == types.AnswerEvidenceOriginUnknown {
			continue
		}
		parts = append(parts, string(origin))
	}
	return strings.Join(parts, ",")
}

package orchestrator

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// checkTier1Floor runs the Tier-1-proven-ratio gate against the
// accumulated evidence buffer, right before the orchestrator would
// dispatch the pre-finalize extract + finalize. It is the
// orchestrator-level mirror of tool.tier1GateReject: that gate only
// fires when the LLM explicitly calls emit_investigation_complete,
// but the explorer can also exit via ShouldStop / idle force-stop /
// soft-stop acceptance, and those paths bypass the tool entirely.
// This check catches them all — every exit path converges here
// because we gate the extract + finalize dispatch.
//
// Return values:
//
//	msg      — bounded diagnostic for logs and the typed
//	           TerminationProfile.Detail (not user-facing text)
//	arm      — WHICH detection fired (件1): tier1_ratio /
//	           followup_coverage; drives the arm-specific disclosure
//	proceed  — true when the gate passes; caller continues to extract
//	exhausted — true when the gate fails AND the retry budget has no
//	           more room
//
// §29.60 + 件4 (2026-07-13): this is a DETECTION function only. On
// proceed=false the caller routes by the completion signal — an ACCEPTED
// model completion is terminal (mark the termination profile floor-degraded,
// continue to finalize, degradedTerminationSystemCaveat renders the
// arm-specific disclosure); exit paths WITHOUT any completion signal
// (ShouldStop / idle-stop / soft-stop) keep the old bounded recovery requeue
// via requeueTier1FloorWithoutCompletion. The removed unconditional requeue
// loop is the endless_loop.txt witness (3 full re-collection cycles on a
// first-pass-correct answer) and the donghu.ftrace 4/4 local budget-burn
// shape (codrax-3afc32b5/20260713).
func (o *Orchestrator) checkTier1Floor(ir *types.AnalysisIR, state *graphState) (msg string, arm types.TerminationFloorArm, proceed bool, exhausted bool) {
	if followup := readLocalizerFollowupForTier1(o.busCtx, ir); followup != nil {
		traceDrill := traceObservationDrillRetryLensActive(o.busCtx, ir)
		logging.Info("[orchestrator] pre-finalize read localizer follow-up: reason=%s paths=%d missing_routes=%d trace_drill_lens=%v — will disclose",
			followup.ReasonCode, len(followup.CandidatePaths), len(followup.MissingRoutes), traceDrill)
		msg := renderReadLocalizerFollowupRetryMessage(followup, traceDrill)
		return msg, types.TerminationFloorArmFollowupCoverage, false, state.retryBudgetExhausted()
	}
	floor := tool.CurrentGroundingPolicy().Tier1Floor
	if floor <= 0 {
		return "", "", true, false
	}
	if o.busCtx == nil || o.busCtx.Mutable == nil {
		return "", "", true, false
	}
	if tier1FloorSuppressedByRuntimeSourceAuthority(o.busCtx) {
		logging.Info("[orchestrator] pre-finalize Tier-1 floor suppressed: reason=runtime_source_authority")
		return "", "", true, false
	}
	evidence := o.busCtx.Mutable.EmittedEvidence()
	if len(evidence) == 0 {
		// No evidence emitted at all — tool-only investigation
		// (exec_command / grep-only answer). Accept; downstream
		// absence checks will handle.
		return "", "", true, false
	}
	contract := types.BuildExactResolutionContract(ir.RequestModel)
	stableAbsent := strings.EqualFold(strings.TrimSpace(o.busCtx.Mutable.StableInvestigationResultKind()), "absence") &&
		strings.TrimSpace(o.busCtx.Mutable.StableAbsenceJustification()) != ""
	requiredFiles := types.ExactResolutionRequiredContextFiles(contract, o.busCtx.Mutable)
	tier1, total := countTier1Evidence(evidence, contract, ir.RequestModel.Scenario, stableAbsent, requiredFiles, ir.RequestModel)
	if total == 0 {
		return "", "", true, false
	}
	ratio := float64(tier1) / float64(total)
	if ratio >= floor {
		return "", "", true, false
	}
	logging.Info("[orchestrator] pre-finalize Tier-1 floor: ratio=%.0f%% (%d/%d) < floor=%.0f%% — will disclose",
		ratio*100, tier1, total, floor*100)
	var b strings.Builder
	fmt.Fprintf(&b, "Only %.0f%% of citation anchors (%d of %d) were proven by text from files already opened; the required minimum is %.0f%%.",
		ratio*100, tier1, total, floor*100)
	b.WriteString(" Some anchors were recovered from indexes and may fail later citation checks — ")
	b.WriteString("call read_file on the recovered sources before declaring the investigation complete.")
	return b.String(), types.TerminationFloorArmTier1Ratio, false, state.retryBudgetExhausted()
}

// discloseTier1FloorGap is the §29.60 (2026-07-13) terminal handling for a
// failed pre-finalize floor detection: model completion is terminal for
// quality-class arms, so the scheduler no longer requeues the explore DAG,
// resets the model's completion, or burns retry budget — it marks the typed
// termination profile floor-degraded and continues to finalize, where
// degradedTerminationSystemCaveat renders the user-facing disclosure.
// Witness for the removed requeue loop: donghu.ftrace 4/4 local runs
// (codrax-3afc32b5/20260713) burned the full RetryBudget here and shipped the
// identical disclosure anyway; the customer endless_loop.txt burned 3 full
// re-collection cycles whose re-anchored evidence reshaped a
// first-pass-correct answer.
func (o *Orchestrator) discloseTier1FloorGap(msg string, arm types.TerminationFloorArm, exhausted bool) {
	logging.Warning("[orchestrator] pre-finalize grounding floor detected a gap; continuing to finalize with disclosure (arm=%s exhausted=%v): %s", arm, exhausted, msg)
	if o != nil && o.busCtx != nil && o.busCtx.Mutable != nil {
		o.busCtx.Mutable.MarkTerminationFloorDegradedArm(arm, msg)
	}
}

// requeueTier1FloorWithoutCompletion is the 件4 (2026-07-13) recovery lane:
// checkTier1Floor also guards explore exits that carry NO model completion
// signal (ShouldStop / idle force-stop / soft-stop) — §29.60 rules only
// ACCEPTED completions terminal, so on those exits the old bounded requeue
// stays legal and useful (§29.21 CSP-RM F-1 deliberately keeps the trace
// drill-down retry alive there; cmp_792: the answer's core attribution
// evidence landed in exactly that retry round). Both completion signals are
// precise typed fields; the retry count keeps the old ExecutionPolicy
// RetryBudget bound. Returns true when it requeued (caller sets
// pendingViolation=msg and continues the loop).
func (o *Orchestrator) requeueTier1FloorWithoutCompletion(ir *types.AnalysisIR, state *graphState, finID string, exhausted bool) bool {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || ir == nil || state == nil {
		return false
	}
	if o.busCtx.Mutable.IsInvestigationComplete() ||
		strings.TrimSpace(o.busCtx.Mutable.StableInvestigationCompleteReason()) != "" {
		return false
	}
	if exhausted {
		return false
	}
	if tp := o.busCtx.Mutable.TerminationProfile(); tp != nil && tp.Kind == types.TerminationHardStall {
		// Hard stall: the evidence fingerprint is pinned — requeueing
		// replays the identical state and re-stalls (pre-§29.60 behavior
		// preserved).
		return false
	}
	if finID != "" {
		state.requeue(finID)
	}
	for _, n := range ir.TaskGraph.Nodes {
		if n.Type == types.NodeFinalize {
			continue
		}
		if state.nodeStatus(n.ID) == nodeDone {
			state.requeue(n.ID)
		}
	}
	state.recordRetry()
	logging.Info("[orchestrator] pre-finalize floor gap with no completion signal — bounded recovery requeue (件4)")
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeRetry,
		Reasoning:  softRetryHintMessage(o.busCtx.Language),
	})
	return true
}

func tier1FloorSuppressedByRuntimeSourceAuthority(busCtx *types.BusContext) bool {
	if busCtx == nil || busCtx.Mutable == nil || busCtx.AnalysisIR == nil {
		return false
	}
	if !types.RuntimeArtifactContextActiveFromBus(busCtx) {
		return false
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(busCtx, types.ObservationLedger{})
	if readLocalizerRuntimeSourceAuthorityKeepsFollowup(authority) {
		return false
	}
	return readLocalizerRuntimeSourceAuthoritySuppressesFollowup(authority)
}

func readLocalizerFollowupForTier1(busCtx *types.BusContext, ir *types.AnalysisIR) *types.ReadLocalizerFollowup {
	if busCtx == nil || busCtx.Mutable == nil || ir == nil {
		return nil
	}
	// CSP-RM F-1 (§29.21 review, 2026-07-10): the source-navigation waiver is
	// a SOURCE-lane advisory statement; it must not kill the pending
	// trace-drill retry (cmp_792, ledger :1815: the final projection's core
	// evidence — root_cause_rank hot windows / frame_root_cause_bundle — was
	// all produced by the retry round after the first pass landed only
	// window_stats/window_sweep/wakeup_chain-level rows truncated by
	// index_event_limit). While the trace drill lens is active and the typed
	// drill DEPTH is still zero, this arm yields. §29.60 件4 (2026-07-13)
	// scope: the retry this keeps alive now exists ONLY on exit paths with
	// no model completion signal (requeueTier1FloorWithoutCompletion,
	// bounded by the old RetryBudget); an accepted completion turns this
	// detection into the arm-specific disclosure instead.
	if types.RuntimeArtifactReadSourceNavigationNotRequiredForBusContext(busCtx) &&
		!traceDrillRetryPending(busCtx, ir) {
		return nil
	}
	// A runtime-artifact-only checkout has no source to localize: the
	// "Source localization is not yet narrow enough / Missing repo_map
	// lenses" follow-up is definitionally unsatisfiable there and only
	// steers the model back into repo_map/grep loops. This must run BEFORE
	// the runtime-source-authority keep/suppress heuristics below — those
	// consume the (census-blind) lane decision and would keep the follow-up
	// alive on the very repos it can never help.
	if busCtx.RuntimeArtifactPreflight.ZeroCurrentSourceRepo() {
		logging.Info("[orchestrator] pre-finalize read localizer follow-up suppressed: reason=zero_current_source_repo")
		return nil
	}
	if runtimeObservationClosureSuppressesReadLocalizerFollowup(busCtx, ir) {
		logging.Info("[orchestrator] pre-finalize read localizer follow-up suppressed: reason=runtime_observation_closure")
		return nil
	}
	turnA := busCtx.Mutable.TurnAArtifacts()
	if turnA == nil {
		return nil
	}
	review := types.SourceLocalizationReviewFromTurnAArtifacts(turnA)
	if readPrincipalMemberSetLocalizationComplete(busCtx, ir, turnA) {
		logging.Info("[orchestrator] pre-finalize read localizer follow-up suppressed: reason=principal_member_set_localization_complete")
		return nil
	}
	var coveragePtr *types.RepoMapNavigationCoverage
	coverage := types.RepoMapNavigationCoverageFromReadArtifacts(ir, busCtx.ExploreLanePlan, turnA)
	coverage = types.NormalizeRepoMapNavigationCoverage(coverage)
	if coverage.State != "" && coverage.State != types.RepoMapNavigationCoverageNotRequired {
		coveragePtr = &coverage
	}
	followup := types.DeriveReadLocalizerFollowup(review, coveragePtr)
	if followup == nil || followup.State != types.ReadLocalizerFollowupNeeded {
		return nil
	}
	return followup
}

func runtimeObservationClosureSuppressesReadLocalizerFollowup(busCtx *types.BusContext, ir *types.AnalysisIR) bool {
	if busCtx == nil || busCtx.Mutable == nil || ir == nil {
		return false
	}
	if !types.RuntimeArtifactContextActiveFromBus(busCtx) {
		return false
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(busCtx, types.ObservationLedger{})
	if readLocalizerRuntimeSourceAuthorityKeepsFollowup(authority) {
		return false
	}
	// CSP-RM F-1 (§29.21 review): for the trace-drill lens shape the closure
	// criterion is the typed drill DEPTH, not the mere existence of
	// deterministic observations. "One wakeup-level observation exists" plus
	// an insufficient sufficiency status is not a closed investigation
	// (cmp_792: the first pass had exactly that shape and the answer's core
	// attribution evidence only landed in the retry round). Depth==0 keeps
	// the follow-up alive (the §29.20 trace directive fires); depth>0 falls
	// through to the arms below, which suppress — one drill round converges.
	// Arm ORDER below is untouched; this is a typed precise carve-out after
	// the keep arm.
	if traceDrillRetryPending(busCtx, ir) {
		return false
	}
	if readLocalizerRuntimeSourceAuthoritySuppressesFollowup(authority) {
		return true
	}
	// A runtime-artifact-only checkout (deterministic census: zero
	// current-source files) has no current source to localize, so a tier-1
	// current-source requirement is structurally unsatisfiable there and must
	// not keep the followup loop alive.
	if !busCtx.RuntimeArtifactPreflight.ZeroCurrentSourceRepo() &&
		readLocalizerTier1CurrentSourceRequired(ir.RequestModel) {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(busCtx, types.ObservationExtractLedgerEvidenceLimit))
	if suff := types.AssessExternalObservationSufficiency(ledger.Records, &ir.RequestModel, busCtx.TurnRouteHint); suff.Status.Sufficient() {
		return true
	}
	if ledger.HasDeterministicRuntimeQueryObservation() {
		return true
	}
	return busCtx.Mutable.TraceQueryRuntimeObservationCount() > 0
}

// traceDrillRetryPending reports whether the §29.20 beneficial trace-drill
// retry is still pending: the trace drill lens shape is active (typed
// conditions in traceObservationDrillRetryLensActive) AND the typed drill
// depth is zero — no deterministic root-cause-rank-family observation has
// landed yet. Both inputs are precise signals: the lens is a five-way typed
// conjunction, and the depth reads the trace observation coverage taxonomy's
// root_cause_rank dimension over hard deterministic trace_query records
// (trace_observation_coverage.go — system-authored predicates/claim keys, not
// model prose). Convergence is bounded by the existing retry budget: the
// drill round publishes rank rows, depth flips >0, and the pending gate turns
// itself off (witness cmp_792 converged in exactly one pass).
func traceDrillRetryPending(busCtx *types.BusContext, ir *types.AnalysisIR) bool {
	if !traceObservationDrillRetryLensActive(busCtx, ir) {
		return false
	}
	return traceDrillDepthObservationCount(busCtx) == 0
}

// traceDrillDepthObservationCount is the typed "drill depth" face for the
// pending gate: the count of deterministic trace_query observations in the
// root_cause_rank coverage dimension (root_cause_* predicates/claim keys —
// the heavy hot-window attribution family that frame_root_cause_bundle rows
// also classify into). Window/sweep/wakeup-level rows deliberately do not
// count: the cmp_792 first pass had those and still produced a weak answer.
func traceDrillDepthObservationCount(busCtx *types.BusContext) int {
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(busCtx, types.ObservationExtractLedgerEvidenceLimit))
	coverage := types.TraceObservationCoverageFromObservationRecords(ledger.Records)
	for _, dim := range coverage.Dimensions {
		if dim.Dimension == types.TraceObservationDimensionRootCauseRank {
			return dim.Count
		}
	}
	return 0
}

func readLocalizerRuntimeSourceAuthorityKeepsFollowup(authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	if authority.CurrentSourceLane == types.CurrentSourceLaneExcluded &&
		!authority.CanHardBlockCompletion &&
		authority.HasRuntimeCarrier() {
		return false
	}
	return authority.KeepsCurrentSourceLaneLoadBearing()
}

func readLocalizerRuntimeSourceAuthoritySuppressesFollowup(authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	if authority.CurrentSourceLane == types.CurrentSourceLaneExcluded &&
		!authority.CanHardBlockCompletion &&
		authority.HasRuntimeCarrier() {
		return true
	}
	return authority.AllowsRuntimeEvidenceWithoutCurrentSource()
}

func readLocalizerTier1CurrentSourceRequired(rm types.RequestModel) bool {
	if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return false
	}
	if rm.ExternalObservationAllowsCurrentSource() {
		return true
	}
	if rm.CurrentSourceExplanationProfile != nil && rm.CurrentSourceExplanationProfile.Active() {
		return true
	}
	if rm.HasTypedCurrentSourceScopeRequest() || rm.HasCurrentSourceObligationSignal() {
		return true
	}
	if rm.LogTriage != nil && len(rm.LogTriage.ResolvedFiles) > 0 {
		return true
	}
	if rm.PerfTrace != nil && len(rm.PerfTrace.ResolvedFiles) > 0 {
		return true
	}
	// §29.151 UPTAIL-1 件4: the bare Required∧Role word-face used to flip the
	// tier-1 floor here with no precise-anchor check (a local copy named
	// readLocalizerHasRequiredCurrentKeyCodeDimension — same noisy→hard shape
	// as §29.146 UPSTREAM-3 件3). It now rides the single-point precise form
	// (dimensionHasPreciseCurrentSourceAnchor behind the exported types
	// method): a prose-only required current_key_code dimension no longer
	// keeps the read-localizer follow-up loop alive or disables the
	// trace-drill retry lens. The exclusion carve at the top of this function
	// stays first, unchanged.
	if rm.HasRequiredCurrentKeyCodeDimensionWithPreciseAnchor() {
		return true
	}
	return readLocalizerHasDiagnosticMechanismBridge(rm)
}

func readLocalizerHasDiagnosticMechanismBridge(rm types.RequestModel) bool {
	if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return false
	}
	if !rm.DiagnosticProfile.CurrentVersionCheck ||
		!rm.Predicates.IsDiagnosticQuestion ||
		!rm.Predicates.IsCrossComponent {
		return false
	}
	switch rm.Intent {
	case types.IntentRootCause, types.IntentExplain:
		return true
	default:
		return false
	}
}

func readPrincipalMemberSetLocalizationComplete(busCtx *types.BusContext, ir *types.AnalysisIR, turnA *types.TurnAArtifacts) bool {
	if busCtx == nil || busCtx.Mutable == nil || ir == nil || turnA == nil {
		return false
	}
	facts := busCtx.Mutable.StableInvestigationAggregateFacts()
	refs := types.PrincipalAggregateMemberSetFactRefsForRequest(facts, &ir.RequestModel)
	if len(refs) == 0 {
		return false
	}
	readFiles := make(map[string]bool, len(turnA.ReadFiles))
	for _, raw := range turnA.ReadFiles {
		if p := canonicalTier1LocationPath(raw); p != "" {
			readFiles[p] = true
		}
	}
	if len(readFiles) == 0 && len(turnA.EvidenceItems) == 0 {
		return false
	}
	covered := 0
	for _, ref := range refs {
		if len(ref.Fact.Members) == 0 {
			continue
		}
		for idx, member := range ref.Fact.Members {
			loc, ok := principalMemberSetLocationForTier1(ref.Fact, idx, member)
			if !ok || !tier1LocationReadBacked(loc, readFiles, turnA.EvidenceItems) {
				return false
			}
			covered++
		}
	}
	return covered > 0
}

func readLocalizerExplicitMissingOwnerForTier1(review *types.SourceLocalizationReview) bool {
	if !types.SourceLocalizationReviewHasSignal(review) {
		return false
	}
	normalized := types.NormalizeSourceLocalizationReview(*review)
	return len(normalized.MissingPaths) > 0 || len(normalized.OwnerMissingPaths) > 0
}

func principalMemberSetLocationForTier1(fact types.AnswerAggregateFact, memberIdx int, member string) (types.AnswerSourceLocationSurface, bool) {
	member = strings.TrimSpace(member)
	if member == "" {
		return types.AnswerSourceLocationSurface{}, false
	}
	if _, loc, ok := types.ParseAnswerSupportRefMemberLocation(member); ok && loc.File != "" && loc.LineStart > 0 {
		return loc, true
	}
	if loc, ok := types.ParseAnswerSourceLocationSurface(member); ok && loc.File != "" && loc.LineStart > 0 {
		return loc, true
	}
	if memberIdx >= 0 && memberIdx < len(fact.SupportRefs) {
		if _, loc, ok := types.ParseAnswerSupportRefMemberLocation(fact.SupportRefs[memberIdx]); ok && loc.File != "" && loc.LineStart > 0 {
			return loc, true
		}
		if loc, ok := types.ParseAnswerSourceLocationSurface(fact.SupportRefs[memberIdx]); ok && loc.File != "" && loc.LineStart > 0 {
			return loc, true
		}
	}
	return types.AnswerSourceLocationSurface{}, false
}

func tier1LocationReadBacked(loc types.AnswerSourceLocationSurface, readFiles map[string]bool, evidence []types.EvidenceItem) bool {
	path := canonicalTier1LocationPath(loc.File)
	if path == "" || loc.LineStart <= 0 {
		return false
	}
	if tier1ReadFilesContainPath(path, readFiles) {
		return true
	}
	if resolved, ok := tier1UniqueReadFileSuffix(path, readFiles); ok && resolved != "" {
		return true
	}
	for _, ev := range evidence {
		evPath := canonicalTier1LocationPath(ev.Source)
		if !tier1PathEqual(evPath, path) && !tier1PathHasSuffix(evPath, path) {
			continue
		}
		if ev.LineStart <= 0 {
			continue
		}
		lineEnd := ev.LineEnd
		if lineEnd <= 0 {
			lineEnd = ev.LineStart
		}
		if loc.LineStart < ev.LineStart || loc.LineStart > lineEnd {
			continue
		}
		if ev.GroundingStatus == types.GroundingGrounded && ev.GroundingTier == types.TierLineText {
			return true
		}
	}
	return false
}

func canonicalTier1LocationPath(raw string) string {
	p := strings.TrimSpace(strings.ReplaceAll(raw, `\`, `/`))
	p = strings.TrimPrefix(p, "./")
	return p
}

func tier1ReadFilesContainPath(path string, readFiles map[string]bool) bool {
	path = canonicalTier1LocationPath(path)
	if path == "" {
		return false
	}
	if readFiles[path] {
		return true
	}
	for candidate := range readFiles {
		if tier1PathEqual(candidate, path) {
			return true
		}
	}
	return false
}

func tier1UniqueReadFileSuffix(path string, readFiles map[string]bool) (string, bool) {
	path = canonicalTier1LocationPath(path)
	if path == "" || len(readFiles) == 0 {
		return "", false
	}
	var match string
	for candidate := range readFiles {
		candidate = canonicalTier1LocationPath(candidate)
		if candidate == "" || !tier1PathHasSuffix(candidate, path) {
			continue
		}
		if match != "" && match != candidate {
			return "", false
		}
		match = candidate
	}
	return match, match != ""
}

func tier1PathHasSuffix(candidate, suffix string) bool {
	candidate = canonicalTier1LocationPath(candidate)
	suffix = canonicalTier1LocationPath(suffix)
	if candidate == "" || suffix == "" {
		return false
	}
	return tier1PathEqual(candidate, suffix) ||
		strings.HasSuffix(strings.ToLower(candidate), "/"+strings.ToLower(suffix))
}

func tier1PathEqual(left, right string) bool {
	left = canonicalTier1LocationPath(left)
	right = canonicalTier1LocationPath(right)
	if left == "" || right == "" {
		return false
	}
	return strings.EqualFold(left, right)
}

// traceObservationDrillRetryLensActive reports whether the pre-finalize
// localizer retry directive should steer the next exploration pass at the
// runtime-trace drill-down surface instead of repository source navigation.
//
// CSP #63 回探臂 (cmp_792 witness, ledger real_trace_campaign §29.8 P2④): a
// pure-trace comparison run tripped the pre-finalize follow-up gate and the
// retry directive named repo_map lenses — structurally unsatisfiable guidance
// for a session whose entire evidence pool is the attached trace (the very
// prompt that dispatched the explorer had told it NOT to run repo breadth
// search). The repair loop itself is design-final (gate intercept → one more
// exploration pass → convergence — the witness converged through trace_query
// window_sweep/rank/bundle drills); only the directive wording is forked here.
// Suppression-arm order is deliberately untouched: re-ordering the
// TraceQueryRuntimeObservationCount arm ahead of the authority keep arm would
// remove the beneficial retry entirely. The keep-arm veto in the original
// witness traced back to CurrentSourceSatisfied minted from model-authored
// aggregate facts (plain-RM terminal current_source fallback); the §29.21
// ruling (CSP-RM, 2026-07-10) retired that fallback — pure model claims now
// classify onto the advisory lane (system_inference/model_claim) and the
// keep arm only rides deterministic witnesses (read coverage / evidence /
// negative search) or a typed precise requirement.
//
// Every condition is a typed, deterministic signal (the fork drives soft
// retry-hint wording only):
//   - no typed precise/typed-carrier current-source requirement is on the
//     request (readLocalizerTier1CurrentSourceRequired — the same signal the
//     closure suppressor consumes): when the request genuinely demands
//     current-source proof, "repository breadth search is not required"
//     would contradict the very reason the follow-up stayed alive and
//     strand the retry loop on an unsatisfiable directive;
//   - the run has a typed trace-query surface (attachment, preflight trace
//     artifact, perf bundle, or typed trace path carrier);
//   - deterministic trace_query runtime observations already exist, so the
//     drill-down lens is demonstrably productive for this session;
//   - no current-source read/evidence work happened: zero CURRENT-SOURCE
//     read files (runtime-artifact reads — the trace_query blob escape
//     lane, .codrax materializations, artifact-shaped paths, and the
//     attached-trace spelling — do not count as source work; the read_file
//     tool already types that decision via ToolRuntimeArtifactRead /
//     nil ReadCoverage, and the path filter here is the consumption-side
//     defense should an artifact path ever reach ReadFiles), and no
//     emitted evidence outside the runtime log/perf origins — a session
//     that did source work keeps the source-navigation wording.
func traceObservationDrillRetryLensActive(busCtx *types.BusContext, ir *types.AnalysisIR) bool {
	if busCtx == nil || busCtx.Mutable == nil || ir == nil {
		return false
	}
	if readLocalizerTier1CurrentSourceRequired(ir.RequestModel) {
		return false
	}
	if !types.TraceQueryContextActiveFromBus(busCtx) {
		return false
	}
	if busCtx.Mutable.TraceQueryRuntimeObservationCount() == 0 {
		return false
	}
	if turnA := busCtx.Mutable.TurnAArtifacts(); turnA != nil {
		for _, file := range turnA.ReadFiles {
			if !traceRetryLensReadFileIsRuntimeArtifact(busCtx, file) {
				return false
			}
		}
	}
	for _, ev := range busCtx.Mutable.EmittedEvidence() {
		switch ev.Origin {
		case types.ClaimOriginLog, types.ClaimOriginPerf:
		default:
			// Current-repo, cross-source, or legacy empty-origin evidence
			// means source-lane work exists; keep the source wording.
			return false
		}
	}
	return true
}

// traceRetryLensReadFileIsRuntimeArtifact reports whether a TurnA read-file
// entry names runtime-artifact bytes rather than current repository source.
// Reading the attached artifact is a design-internal escape lane and must not
// flip the retry directive back to source-navigation wording. The predicate
// consumes the same typed authorities as the read_file tool's own
// classification (readFileTypedSourcePath): the shared artifact path-shape
// lane (types.RuntimeArtifactPathKind — reserved blob basenames + artifact
// extensions, case-folded), the .codrax runtime-state root, and the
// attached-trace user spelling (case-folded path or basename, mirroring the
// typed citation spelling set semantics).
func traceRetryLensReadFileIsRuntimeArtifact(busCtx *types.BusContext, path string) bool {
	p := strings.TrimSpace(strings.ReplaceAll(path, `\`, `/`))
	if p == "" {
		return true // empty entry proves no source work
	}
	if types.RuntimeArtifactPathKind(p) != "" {
		return true
	}
	trimmed := strings.TrimPrefix(p, "./")
	if trimmed == ".codrax" || strings.HasPrefix(trimmed, ".codrax/") || strings.Contains(trimmed, "/.codrax/") {
		return true
	}
	attached := strings.TrimSpace(busCtx.AttachedHitraceSource)
	if attached == "" {
		return false
	}
	attachedFold := strings.ToLower(filepath.ToSlash(filepath.Clean(attached)))
	pathFold := strings.ToLower(filepath.ToSlash(filepath.Clean(p)))
	return pathFold == attachedFold || filepath.Base(pathFold) == filepath.Base(attachedFold)
}

func renderReadLocalizerFollowupRetryMessage(followup *types.ReadLocalizerFollowup, traceDrill bool) string {
	if followup == nil {
		return ""
	}
	normalized := types.NormalizeReadLocalizerFollowup(*followup)
	if normalized.State != types.ReadLocalizerFollowupNeeded {
		return ""
	}
	if traceDrill {
		// Pure-trace session: repo_map lens words are structurally
		// unsatisfiable here and are stripped entirely; the directive points
		// at the trace drill-down views the session can actually execute
		// (LLM-facing wording — tool/view names only, no internal tokens).
		var b strings.Builder
		b.WriteString("The verification pass needs deeper runtime-trace drill-down before the investigation can finish safely.")
		b.WriteString(" Stay on trace_query: for each attached trace, use window_sweep over that trace's already-selected span to locate the densest sub-windows,")
		b.WriteString(" then run heavy views (root_cause_rank, critical_blocking_calls, window_stats, frame_root_cause_bundle)")
		b.WriteString(" on those 80-150ms hotspot windows, carrying that trace's own time_start/time_end into every follow-up call.")
		b.WriteString(" Preserve the verified findings through emit_investigation_complete (reason and, for counts/scalars, aggregate_facts) before declaring completion.")
		b.WriteString(" Repository breadth search is not required for this pass; the attached trace remains the evidence pool.")
		return b.String()
	}
	var b strings.Builder
	b.WriteString("Source localization is not yet narrow enough to finish safely.")
	if len(normalized.CandidatePaths) > 0 {
		b.WriteString(" Candidate paths: ")
		b.WriteString(strings.Join(normalized.CandidatePaths, ", "))
		b.WriteString(".")
	}
	if len(normalized.MissingRoutes) > 0 {
		var routes []string
		for _, route := range normalized.MissingRoutes {
			if route.Valid() {
				routes = append(routes, string(route))
			}
		}
		if len(routes) > 0 {
			b.WriteString(" Missing repo_map lenses: ")
			b.WriteString(strings.Join(routes, ", "))
			b.WriteString(".")
		}
	}
	b.WriteString(" Run a focused read-only pass: use repo_map for the missing lenses, then read_file on the strongest owner/source paths and emit evidence before declaring completion.")
	return b.String()
}

// countTier1Evidence returns (tier1, total) where tier1 is the count
// of items the LLM grounded via TierLineText (actually read the
// file) and total is the overall evidence count. Legacy empty-
// GroundingStatus items (deterministic concrete_value scans) count
// toward tier1 — they are facts extracted from source, not LLM
// speculation, and should not push the floor down.
func countTier1Evidence(
	evidence []types.EvidenceItem,
	contract *types.ExactResolutionContract,
	scenario types.Scenario,
	stableAbsent bool,
	requiredFiles []string,
	rm types.RequestModel,
) (tier1, total int) {
	for _, e := range evidence {
		if !types.EvidenceCountsTowardTier1FloorInContext(e, contract, scenario, stableAbsent, requiredFiles, rm) {
			continue
		}
		total++
		switch e.GroundingStatus {
		case types.GroundingGrounded:
			if e.GroundingTier == types.TierLineText {
				tier1++
			}
		case types.GroundingRecovered, types.GroundingUngrounded:
			// not counted toward Tier-1
		default:
			// legacy / deterministic
			tier1++
		}
	}
	return tier1, total
}

type groundingHealth struct {
	total     int
	accepted  int
	tier1     int
	recovered int
}

func (h groundingHealth) groundingRatio() float64 {
	if h.total == 0 {
		return 1
	}
	return float64(h.accepted) / float64(h.total)
}

func (h groundingHealth) tier1Ratio() float64 {
	if h.total == 0 {
		return 1
	}
	return float64(h.tier1) / float64(h.total)
}

func countGroundingHealth(
	evidence []types.EvidenceItem,
	contract *types.ExactResolutionContract,
	scenario types.Scenario,
	stableAbsent bool,
	requiredFiles []string,
	rm types.RequestModel,
) groundingHealth {
	var h groundingHealth
	for _, e := range evidence {
		if !types.EvidenceCountsTowardTier1FloorInContext(e, contract, scenario, stableAbsent, requiredFiles, rm) {
			continue
		}
		h.total++
		switch e.GroundingStatus {
		case types.GroundingGrounded:
			h.accepted++
			if e.GroundingTier == types.TierLineText {
				h.tier1++
			}
		case types.GroundingRecovered:
			h.accepted++
			h.recovered++
		case types.GroundingUngrounded:
			// Advisory warning target.
		default:
			// Legacy deterministic facts predate the grounding fields;
			// match the hard gates and count them as Tier-1.
			h.accepted++
			h.tier1++
		}
	}
	return h
}

func (o *Orchestrator) warnLowGroundingIfNeeded(ir *types.AnalysisIR, warned *bool) {
	if warned != nil && *warned {
		return
	}
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || ir == nil {
		return
	}
	policy := tool.CurrentGroundingPolicy()
	if policy.WarnGroundingFloor <= 0 && policy.WarnTier1Floor <= 0 {
		return
	}
	evidence := o.busCtx.Mutable.EmittedEvidence()
	if len(evidence) == 0 {
		return
	}
	contract := types.BuildExactResolutionContract(ir.RequestModel)
	stableAbsent := strings.EqualFold(strings.TrimSpace(o.busCtx.Mutable.StableInvestigationResultKind()), "absence") &&
		strings.TrimSpace(o.busCtx.Mutable.StableAbsenceJustification()) != ""
	requiredFiles := types.ExactResolutionRequiredContextFiles(contract, o.busCtx.Mutable)
	health := countGroundingHealth(evidence, contract, ir.RequestModel.Scenario, stableAbsent, requiredFiles, ir.RequestModel)
	if health.total == 0 {
		return
	}
	groundingRatio := health.groundingRatio()
	tier1Ratio := health.tier1Ratio()
	lowGrounding := policy.WarnGroundingFloor > 0 && groundingRatio < policy.WarnGroundingFloor
	lowTier1 := policy.WarnTier1Floor > 0 && tier1Ratio < policy.WarnTier1Floor
	if !lowGrounding && !lowTier1 {
		return
	}
	if warned != nil {
		*warned = true
	}
	profile := strings.TrimSpace(string(policy.Profile))
	if profile == "" {
		profile = string(tool.GroundingProfileCustom)
	}
	groundingPct := int(groundingRatio*100 + 0.5)
	tier1Pct := int(tier1Ratio*100 + 0.5)
	logging.Warning("[orchestrator] low grounding health: profile=%s grounding_ratio=%.2f tier1_ratio=%.2f accepted=%d recovered=%d tier1=%d total=%d warn_grounding_floor=%.2f warn_tier1_floor=%.2f",
		profile, groundingRatio, tier1Ratio, health.accepted, health.recovered, health.tier1, health.total, policy.WarnGroundingFloor, policy.WarnTier1Floor)
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeLowGrounding,
		Reasoning:  lowGroundingWarningMessage(o.busCtx.Language, profile, groundingPct, tier1Pct),
	})
}

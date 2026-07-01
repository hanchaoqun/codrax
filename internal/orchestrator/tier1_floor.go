package orchestrator

import (
	"fmt"
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
//	msg      — human-readable diagnostic suitable for pendingViolation
//	proceed  — true when the gate passes; caller continues to extract
//	exhausted — true when the gate fails AND the retry budget has no
//	           more room. Caller falls through to finalize fail-loud.
//
// When proceed=false and exhausted=false, the caller is expected to
// requeue nodes + record a retry + set pendingViolation=msg and
// continue the main loop.
func (o *Orchestrator) checkTier1Floor(ir *types.AnalysisIR, state *graphState) (msg string, proceed bool, exhausted bool) {
	if followup := readLocalizerFollowupForTier1(o.busCtx, ir); followup != nil {
		logging.Info("[orchestrator] pre-finalize read localizer follow-up: reason=%s paths=%d missing_routes=%d — will requeue explorer",
			followup.ReasonCode, len(followup.CandidatePaths), len(followup.MissingRoutes))
		msg := renderReadLocalizerFollowupRetryMessage(followup)
		if exhausted := state.retryBudgetExhausted(); exhausted {
			return msg, false, true
		}
		return msg, false, false
	}
	floor := tool.CurrentGroundingPolicy().Tier1Floor
	if floor <= 0 {
		return "", true, false
	}
	if o.busCtx == nil || o.busCtx.Mutable == nil {
		return "", true, false
	}
	if tier1FloorSuppressedByRuntimeSourceAuthority(o.busCtx) {
		logging.Info("[orchestrator] pre-finalize Tier-1 floor suppressed: reason=runtime_source_authority")
		return "", true, false
	}
	evidence := o.busCtx.Mutable.EmittedEvidence()
	if len(evidence) == 0 {
		// No evidence emitted at all — tool-only investigation
		// (exec_command / grep-only answer). Accept; downstream
		// absence checks will handle.
		return "", true, false
	}
	contract := types.BuildExactResolutionContract(ir.RequestModel)
	stableAbsent := strings.EqualFold(strings.TrimSpace(o.busCtx.Mutable.StableInvestigationResultKind()), "absence") &&
		strings.TrimSpace(o.busCtx.Mutable.StableAbsenceJustification()) != ""
	requiredFiles := types.ExactResolutionRequiredContextFiles(contract, o.busCtx.Mutable)
	tier1, total := countTier1Evidence(evidence, contract, ir.RequestModel.Scenario, stableAbsent, requiredFiles, ir.RequestModel)
	if total == 0 {
		return "", true, false
	}
	ratio := float64(tier1) / float64(total)
	if ratio >= floor {
		return "", true, false
	}
	logging.Info("[orchestrator] pre-finalize Tier-1 floor: ratio=%.0f%% (%d/%d) < floor=%.0f%% — will requeue explorer",
		ratio*100, tier1, total, floor*100)
	var b strings.Builder
	fmt.Fprintf(&b, "Only %.0f%% of citation anchors (%d of %d) were proven by text from files already opened; the required minimum is %.0f%%.",
		ratio*100, tier1, total, floor*100)
	b.WriteString(" Some anchors were recovered from indexes and may fail later citation checks — ")
	b.WriteString("call read_file on the recovered sources before declaring the investigation complete.")
	if exhausted := state.retryBudgetExhausted(); exhausted {
		return b.String(), false, true
	}
	return b.String(), false, false
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
	if types.RuntimeArtifactReadSourceNavigationNotRequiredForBusContext(busCtx) {
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
	if readLocalizerRuntimeSourceAuthoritySuppressesFollowup(authority) {
		return true
	}
	if readLocalizerTier1CurrentSourceRequired(ir.RequestModel) {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(busCtx, 128))
	if suff := types.AssessExternalObservationSufficiency(ledger.Records, &ir.RequestModel, busCtx.TurnRouteHint); suff.Status.Sufficient() {
		return true
	}
	if ledger.HasDeterministicRuntimeQueryObservation() {
		return true
	}
	return busCtx.Mutable.TraceQueryRuntimeObservationCount() > 0
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
	if readLocalizerHasRequiredCurrentKeyCodeDimension(rm) {
		return true
	}
	return readLocalizerHasDiagnosticMechanismBridge(rm)
}

func readLocalizerHasRequiredCurrentKeyCodeDimension(rm types.RequestModel) bool {
	if rm.RequestedAnswerDimensions == nil || !rm.RequestedAnswerDimensions.Active() {
		return false
	}
	for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
		if dim.Required && dim.Role == types.RequestedAnswerDimensionCurrentKeyCode {
			return true
		}
	}
	return false
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

func renderReadLocalizerFollowupRetryMessage(followup *types.ReadLocalizerFollowup) string {
	if followup == nil {
		return ""
	}
	normalized := types.NormalizeReadLocalizerFollowup(*followup)
	if normalized.State != types.ReadLocalizerFollowupNeeded {
		return ""
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

package orchestrator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// repair_caveat_materializer.go — Plan W1.2 (2026-05-05).
//
// Translates unresolved Violation lists into user-facing natural-
// language caveats by way of the ViolKindSpec.CaveatFamilyID +
// CaveatFamilyTemplate registry. The output goes into
// AnswerDocumentV2.Caveats[] so the user sees a polite "the system
// could not fully verify X" sentence — never the orchestration's
// internal vocabulary (ViolKind names, IR fields, confidence
// scores, yield-kill markers, etc.).
//
// Replaces the prependFailLoudWarning code path that pre-2026-05-05
// dumped closure ledger contents directly into the user-visible
// answer string. See docs/design/iteration_inflation_remediation.md
// W1 for design rationale.
//
// The function is pure: input violations + language → output
// caveat strings. No state, no side effects. Called from the
// orchestrator at the points where retry has exhausted and the
// answer is shipping with unresolved gaps.

// MaxMaterializedCaveats caps the number of system-generated caveats
// per answer to avoid drowning the user. Beyond this, additional
// families are dropped (the LLM-authored caveats already in the
// answer document take precedence as a separate channel).
const MaxMaterializedCaveats = 3

// MaterializeUnresolvedViolationsAsCaveats groups violations by
// CaveatFamilyID, looks up the user-facing template per language,
// and returns up to MaxMaterializedCaveats deduplicated caveat
// strings ready to append to AnswerDocumentV2.Caveats.
//
// Empty / nil input returns nil. Violations whose ViolKindSpec
// either is unregistered or has empty CaveatFamilyID are skipped
// silently — those are operator-only telemetry signals.
//
// lang: "zh" / "zh-cn" / "cn" / "chinese" → ZH template; anything
// else → EN. Empty defaults to ZH (default project language).
//
// Output stability: family declaration order via
// AllCaveatFamilies, then alphabetical by family ID for any
// families whose declaration order is undefined.
func MaterializeUnresolvedViolationsAsCaveats(violations []types.Violation, lang string) []string {
	if len(violations) == 0 {
		return nil
	}
	hit := make(map[string]bool, len(violations))
	for _, v := range violations {
		spec, ok := types.ViolKindSpecFor(v.Kind)
		if !ok {
			continue
		}
		if spec.CaveatFamilyID == "" {
			continue
		}
		hit[spec.CaveatFamilyID] = true
	}
	if len(hit) == 0 {
		return nil
	}
	useChinese := isChineseLang(lang)
	out := make([]string, 0, len(hit))
	// AllCaveatFamilies gives stable order via the registry's
	// declaration sequence (map iteration in Go is randomized, but
	// the registry's All accessor returns in registration order via
	// its slice mirror — see types/violation_registry.go).
	for _, fam := range types.AllCaveatFamilies() {
		if !hit[fam.ID] {
			continue
		}
		body := fam.EN
		if useChinese {
			body = fam.ZH
		}
		if fam.ID == types.CaveatFamilyConsistency {
			if specific := materializeSelfContradictionCaveat(violations, useChinese); specific != "" {
				body = specific
			}
		}
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		out = append(out, body)
		if len(out) >= MaxMaterializedCaveats {
			break
		}
	}
	return out
}

func materializeSelfContradictionCaveat(violations []types.Violation, useChinese bool) string {
	for _, v := range violations {
		if v.Kind != types.ViolSelfContradiction {
			continue
		}
		summary, body := parseSelfContradictionClaims(v.Detail)
		if summary == "" && body == "" {
			continue
		}
		topic := residualClusterValue(v.ClusterKey, "topic")
		if useChinese {
			prefix := "答案有一处前后不一致"
			if topic != "" {
				prefix += "（" + trimUserVisibleQuote(topic, 40) + "）"
			}
			if summary != "" && body != "" {
				return prefix + "：摘要写到“" + trimUserVisibleQuote(summary, 96) + "”，正文/表格写到“" + trimUserVisibleQuote(body, 96) + "”；请按引用和表格核对该项。"
			}
			if summary != "" {
				return prefix + "：摘要写到“" + trimUserVisibleQuote(summary, 120) + "”；请按引用和正文核对该项。"
			}
			return prefix + "：正文写到“" + trimUserVisibleQuote(body, 120) + "”；请按引用和摘要核对该项。"
		}
		prefix := "One answer passage is internally inconsistent"
		if topic != "" {
			prefix += " (" + trimUserVisibleQuote(topic, 40) + ")"
		}
		if summary != "" && body != "" {
			return prefix + ": the summary says \"" + trimUserVisibleQuote(summary, 96) + "\", while the body/table says \"" + trimUserVisibleQuote(body, 96) + "\"; verify that item against the citations and table."
		}
		if summary != "" {
			return prefix + ": the summary says \"" + trimUserVisibleQuote(summary, 120) + "\"; verify that item against the citations and body."
		}
		return prefix + ": the body says \"" + trimUserVisibleQuote(body, 120) + "\"; verify that item against the citations and summary."
	}
	return ""
}

func parseSelfContradictionClaims(detail string) (summary, body string) {
	detail = strings.TrimSpace(detail)
	const summaryMarker = "SUMMARY: "
	const bodyMarker = " ⇄ BODY: "
	si := strings.Index(detail, summaryMarker)
	bi := strings.Index(detail, bodyMarker)
	if si < 0 || bi < 0 || bi <= si {
		return "", ""
	}
	summary = strings.TrimSpace(detail[si+len(summaryMarker) : bi])
	body = strings.TrimSpace(detail[bi+len(bodyMarker):])
	return trimQuotedClaim(summary), trimQuotedClaim(body)
}

func trimQuotedClaim(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if unquoted, err := strconv.Unquote(s); err == nil {
			return strings.TrimSpace(unquoted)
		}
	}
	return strings.Trim(s, "\"")
}

func trimUserVisibleQuote(s string, maxRunes int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if maxRunes <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes <= 1 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-1]) + "…"
}

// AppendUserCaveatsToAnswer renders materialized caveats as a
// trailing markdown section appended to the answer text. Replaces
// prependFailLoudWarning's hostile header pattern: caveats sit at
// the END of the answer (after the body the user just read), in
// natural language, with no internal jargon.
//
// The heading text is "补充说明：" (ZH) / "Additional notes:" (EN) —
// deliberately different from the answer document's own LLM-
// authored "**说明**：" / "**Caveats:**" section so the two channels
// remain visually distinguishable. LLM-authored caveats describe
// content gaps the LLM identified; system caveats describe
// orchestration constraints the retry loop could not resolve.
//
// If MaterializeUnresolvedViolationsAsCaveats returns no caveats
// (all violations are operator-only telemetry, or the input is
// empty), the answer is returned unchanged.
func AppendUserCaveatsToAnswer(answer string, violations []types.Violation, lang string) string {
	caveats := MaterializeUnresolvedViolationsAsCaveats(violations, lang)
	if len(caveats) == 0 {
		return answer
	}
	return appendSystemCaveatBullets(answer, caveats, lang)
}

func AppendUserCaveatsToAnswerForBus(answer string, violations []types.Violation, lang string, ctx *types.BusContext) string {
	filtered := FilterDerivedViolations(violations)
	filtered = suppressRuntimeObservationOnlyLowPrecisionCaveats(filtered, ctx)
	filtered = suppressGenericSoftCaveatsForAcceptedSurface(filtered, ctx)
	if len(filtered) == 0 {
		return answer
	}
	return AppendUserCaveatsToAnswer(answer, filtered, lang)
}

// AppendSoftContractCaveatsToAnswer appends user-facing caveats only
// for soft root violations under the current runtime policy.
//
// This is the accept-path companion to FilterActionableRootViolations:
// default-soft contract concerns should not burn another finalizer
// retry, but if the registry has a CaveatFamilyID the shipped answer
// should still tell the user about the residual boundary. Operator
// promotion via pipeline_contract_strict_kinds removes the kind from
// this soft set, keeping promoted gates actionable.
func AppendSoftContractCaveatsToAnswer(answer string, violations []types.Violation, lang string) string {
	return AppendUserCaveatsToAnswer(answer, softContractCaveatViolations(violations), lang)
}

func AppendSoftContractCaveatsToAnswerForBus(answer string, violations []types.Violation, lang string, ctx *types.BusContext) string {
	soft := softContractCaveatViolations(violations)
	if len(soft) == 0 {
		return answer
	}
	soft = suppressRuntimeObservationOnlyLowPrecisionCaveats(soft, ctx)
	if len(soft) == 0 {
		return answer
	}
	if runtimeObservationOnlyCaveatContext(ctx) {
		remaining, needsBoundary := splitRuntimeObservationOnlyCaveats(soft)
		caveats := make([]string, 0, len(remaining)+1)
		if needsBoundary {
			if boundary := runtimeObservationBoundaryCaveat(ctx, lang); boundary != "" {
				caveats = append(caveats, boundary)
			}
		}
		caveats = append(caveats, MaterializeUnresolvedViolationsAsCaveats(remaining, lang)...)
		return appendSystemCaveatBullets(answer, caveats, lang)
	}
	if historyNarrativeCaveatContext(ctx) {
		remaining := filterPureHistoryNarrativeCaveats(soft)
		remaining = suppressGenericSoftCaveatsForAcceptedSurface(remaining, ctx)
		if len(remaining) == 0 {
			return answer
		}
		return AppendUserCaveatsToAnswer(answer, remaining, lang)
	}
	soft = suppressGenericSoftCaveatsForAcceptedSurface(soft, ctx)
	if len(soft) == 0 {
		return answer
	}
	return AppendUserCaveatsToAnswer(answer, soft, lang)
}

func suppressRuntimeObservationOnlyLowPrecisionCaveats(violations []types.Violation, ctx *types.BusContext) []types.Violation {
	if len(violations) == 0 || !runtimeLowPrecisionCaveatSuppressionContext(ctx) {
		return violations
	}
	out := make([]types.Violation, 0, len(violations))
	for _, v := range violations {
		if runtimeObservationOnlyLowPrecisionCaveat(v, ctx) {
			continue
		}
		out = append(out, v)
	}
	return out
}

func runtimeLowPrecisionCaveatSuppressionContext(ctx *types.BusContext) bool {
	return runtimeObservationOnlyCaveatContext(ctx) || runtimeArtifactPrincipalAnswerSurfaceContext(ctx)
}

func runtimeObservationOnlyLowPrecisionCaveat(v types.Violation, ctx *types.BusContext) bool {
	switch v.Kind {
	case types.ViolSelfContradiction:
		summary, body := parseSelfContradictionClaims(v.Detail)
		return summary == "" && body == ""
	case types.ViolEnumerationEvidenceUnderspecified:
		return runtimeArtifactPrincipalAnswerSurfaceContext(ctx) && !runtimeSurfaceRequiresPrincipalEnumeration(ctx)
	case types.ViolEnumerationLabelUngrounded,
		types.ViolEnumerationItemLabelExtractorDrift,
		types.ViolEnumerationLabelHallucinated:
		return runtimeArtifactPrincipalAnswerSurfaceContext(ctx)
	case types.ViolDeniedTokenUndeclared:
		return runtimeArtifactPrincipalAnswerSurfaceContext(ctx)
	case types.ViolUncertaintyBlockMissing:
		return runtimeArtifactPrincipalAnswerSurfaceContext(ctx)
	default:
		return false
	}
}

func runtimeArtifactPrincipalAnswerSurfaceContext(ctx *types.BusContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	doc := ctx.Mutable.AnswerDocumentV2()
	if doc == nil || len(doc.Blocks) == 0 {
		return false
	}
	blocks := runtimeAnswerSurfaceCandidateBlocks(doc)
	if len(blocks) == 0 {
		return false
	}
	external := 0
	for _, block := range blocks {
		if !answerBlockHasOnlyExternalObservationClaimUses(block) {
			return false
		}
		if answerBlockUsesCitation(block, doc.Citations) {
			return false
		}
		external++
	}
	return external == len(blocks)
}

func runtimeAnswerSurfaceCandidateBlocks(doc *types.AnswerDocumentV2) []types.AnswerBlock {
	if doc == nil {
		return nil
	}
	var principal []types.AnswerBlock
	for _, block := range doc.Blocks {
		if block.SurfaceRole == types.SurfacePrincipal {
			principal = append(principal, block)
		}
	}
	if len(principal) > 0 {
		return principal
	}
	var visible []types.AnswerBlock
	for _, block := range doc.Blocks {
		if !runtimePrincipalLikeAnswerBlockKind(block.Kind) {
			continue
		}
		visible = append(visible, block)
	}
	return visible
}

func runtimePrincipalLikeAnswerBlockKind(kind types.AnswerBlockKind) bool {
	switch kind {
	case types.BlockSummary,
		types.BlockSection,
		types.BlockOrderedList,
		types.BlockBulletList,
		types.BlockScalar,
		types.BlockDecision,
		types.BlockTable,
		types.BlockDiagram:
		return true
	default:
		return false
	}
}

func requestModelHasRuntimeOrExternalObservation(rm types.RequestModel) bool {
	return rm.HasExternalOnlyRuntimeArtifact() ||
		rm.HasExternalObservationArtifactReference() ||
		rm.LogTriage != nil ||
		rm.PerfTrace != nil ||
		rm.ArtifactObservationProfile != nil
}

func answerBlockHasOnlyExternalObservationClaimUses(block types.AnswerBlock) bool {
	seen := false
	for _, claim := range block.ClaimUses {
		if claim.IsEmpty() {
			continue
		}
		seen = true
		if claim.ClaimForm != types.ClaimExternalObservation {
			return false
		}
	}
	return seen
}

func answerBlockUsesCitation(block types.AnswerBlock, citations []types.Citation) bool {
	for _, item := range block.Items {
		if item.CitationRef >= 0 && item.CitationRef < len(citations) {
			return true
		}
	}
	return false
}

func runtimeSurfaceRequiresPrincipalEnumeration(ctx *types.BusContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	rm := ctx.Mutable.RequestModel()
	if rm == nil {
		return false
	}
	return rm.Intent == types.IntentEnumerate ||
		rm.Predicates.IsCategoryEnumeration ||
		types.RequiresExhaustiveEnumerationMemberSetHandoff(*rm) ||
		types.RequiresRelationMemberSetHandoff(*rm) ||
		rm.CompletenessObligation.IsActive()
}

func softContractCaveatViolations(violations []types.Violation) []types.Violation {
	roots := FilterDerivedViolations(violations)
	if len(roots) == 0 {
		return nil
	}
	out := make([]types.Violation, 0, len(roots))
	for _, v := range roots {
		if !isSoftViolationKind(v.Kind) {
			continue
		}
		out = append(out, v)
	}
	return out
}

func runtimeObservationOnlyCaveatContext(ctx *types.BusContext) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	if rm := ctx.Mutable.RequestModel(); rm != nil && rm.HasObservationOnlyRuntimeArtifact() {
		return true
	}
	if runtimeWaiverObservationOnly(ctx.Mutable.EvidenceFloorWaiver()) {
		return true
	}
	return runtimeWaiverObservationOnly(ctx.Mutable.StableEvidenceFloorWaiver())
}

func historyNarrativeCaveatContext(ctx *types.BusContext) bool {
	rm, contract := requestModelAndAnswerContractForBus(ctx)
	return rm != nil && types.HistoryLookupPrefersVCSNarrativePrincipal(*rm, contract)
}

func filterPureHistoryNarrativeCaveats(violations []types.Violation) []types.Violation {
	if len(violations) == 0 {
		return nil
	}
	out := make([]types.Violation, 0, len(violations))
	for _, v := range violations {
		if pureHistoryNarrativeSuppressibleCaveat(v.Kind) {
			continue
		}
		out = append(out, v)
	}
	return out
}

func pureHistoryNarrativeSuppressibleCaveat(kind types.ViolationKind) bool {
	switch kind {
	case types.ViolBlockCoverageMissing,
		types.ViolFacetUncovered,
		types.ViolRichnessGlaringGap,
		types.ViolEnumerationLabelUngrounded,
		types.ViolAnswerSemanticUnderfilled:
		return true
	default:
		return false
	}
}

func suppressGenericSoftCaveatsForAcceptedSurface(violations []types.Violation, ctx *types.BusContext) []types.Violation {
	if len(violations) == 0 {
		return nil
	}
	rm, answerContract := requestModelAndAnswerContractForBus(ctx)
	if rm == nil {
		return violations
	}
	intentContract := types.CompileAnswerIntentContract(*rm, answerContract)
	out := make([]types.Violation, 0, len(violations))
	for _, v := range violations {
		if genericAcceptedPathCaveatIsTelemetry(v, rm, intentContract) {
			continue
		}
		out = append(out, v)
	}
	return out
}

func genericAcceptedPathCaveatIsTelemetry(v types.Violation, rm *types.RequestModel, contract types.AnswerIntentContract) bool {
	switch v.Kind {
	case types.ViolPrincipalClaimUseMissing,
		types.ViolLaneBlockKindMismatch,
		types.ViolRichnessRegression,
		types.ViolRichnessGlaringGap,
		types.ViolPrincipalProseUnderfilled,
		types.ViolAnswerSemanticUnderfilled:
		return true
	case types.ViolEnumerationLabelUngrounded,
		types.ViolEnumerationEvidenceUnderspecified,
		types.ViolEnumerationItemLabelExtractorDrift,
		types.ViolEnumerationLabelHallucinated,
		types.ViolPrincipalSupportMemberOmitted,
		types.ViolExhaustiveMemberSetCoverageDrift:
		return !principalEnumerationSurfaceRequested(rm, contract)
	case types.ViolStructuralEnumerationDivergence:
		return structuralEnumerationDivergenceCaveatIsTelemetry(rm, contract)
	case types.ViolUncertaintyBlockMissing:
		return !contract.HasOutput(types.AnswerRequestedOutputAbsence) &&
			!contract.HasOutput(types.AnswerRequestedOutputDiagnostic)
	case types.ViolFacetUncovered:
		return facetUncoveredCaveatIsTelemetry(v, rm, contract)
	case types.ViolBlockCoverageMissing:
		return blockCoverageCaveatIsTelemetry(v, rm, contract)
	case types.ViolDiagramEdgeUnsupported,
		types.ViolDiagramEdgeLabelMismatch,
		types.ViolDiagramRelationLabelOnly,
		types.ViolDiagramEdgeEndpointHallucinated:
		return typedCallChainAnswerSurface(rm, contract)
	case types.ViolCitation,
		types.ViolGhostAnchor,
		types.ViolChainDemoted,
		types.ViolLiteralFormFailed,
		types.ViolSymbolAnchorMismatch:
		return !acceptedPathNeedsPreciseGroundingDisclosure(rm, contract)
	case types.ViolFamilyMismatch,
		types.ViolAcceptance,
		types.ViolSuccessCriterion,
		types.ViolClaimFormUnsupported:
		// These legacy acceptance-family violations are useful
		// operator telemetry and strict-promotion inputs, but their
		// default caveat text is intentionally generic. On the soft
		// accept path, emitting "some acceptance checks failed" gives
		// users no concrete next action and makes an otherwise good
		// answer look unstable. Specific grounding/coverage caveats
		// remain handled by the citation, facet, and block branches
		// above.
		return true
	default:
		return false
	}
}

func structuralEnumerationDivergenceCaveatIsTelemetry(rm *types.RequestModel, contract types.AnswerIntentContract) bool {
	if rm == nil {
		return false
	}
	if contract.HasOutput(types.AnswerRequestedOutputComparison) {
		return true
	}
	if rm.Predicates.IsCrossComponent || len(rm.QuestionStructure().Buckets) >= 2 {
		return true
	}
	return false
}

func facetUncoveredCaveatIsTelemetry(v types.Violation, rm *types.RequestModel, contract types.AnswerIntentContract) bool {
	root := residualClusterValue(v.ClusterKey, "root")
	facet := types.AnswerFacetKind(residualClusterValue(v.ClusterKey, "facet"))
	if facet == "" {
		return unclusteredFacetCaveatIsTelemetry(rm, contract)
	}
	if root == "answer_richness_facet_coverage" {
		return true
	}
	if root != "answer_facet_coverage" {
		return false
	}
	switch facet {
	case types.FacetDiagramSpine:
		return !contract.HasOutput(types.AnswerRequestedOutputDiagram) &&
			!contract.HasOutput(types.AnswerRequestedOutputTrace)
	case types.FacetBranchGuard:
		kind := types.RequirementKind("")
		if rm != nil {
			kind = types.NormalizeRequirementKind(rm.AnalyzerHints.Kind)
		}
		return typedCallChainAnswerSurface(rm, contract) && kind != types.ReqConditional
	case types.FacetComponentRelation:
		if rm != nil && rm.Predicates.IsRelationalLookup {
			return false
		}
		return true
	case types.FacetNearestMechanism, types.FacetUncertaintyBoundary:
		return !acceptedPathNeedsPreciseGroundingDisclosure(rm, contract)
	case types.FacetCurrentCodePath:
		if principalEnumerationSurfaceRequested(rm, contract) ||
			contract.HasOutput(types.AnswerRequestedOutputComparison) {
			return true
		}
		return !acceptedPathNeedsPreciseGroundingDisclosure(rm, contract)
	default:
		return false
	}
}

func unclusteredFacetCaveatIsTelemetry(rm *types.RequestModel, contract types.AnswerIntentContract) bool {
	if rm == nil {
		return false
	}
	if acceptedPathNeedsPreciseGroundingDisclosure(rm, contract) {
		return false
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return false
	}
	switch types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case types.ReqMechanism:
		return true
	}
	return rm.Intent == types.IntentExplain && rm.Scenario == types.ScenarioArchitectureExplain
}

func acceptedPathNeedsPreciseGroundingDisclosure(rm *types.RequestModel, contract types.AnswerIntentContract) bool {
	if rm == nil {
		return true
	}
	if principalEnumerationSurfaceRequested(rm, contract) {
		return true
	}
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsRoleLocateLookup {
		return true
	}
	switch rm.Intent {
	case types.IntentConfigQuery, types.IntentReturnValue, types.IntentTrace:
		return true
	}
	if contract.HasOutput(types.AnswerRequestedOutputScalar) ||
		contract.HasOutput(types.AnswerRequestedOutputKeyValue) ||
		contract.HasOutput(types.AnswerRequestedOutputCount) ||
		contract.HasOutput(types.AnswerRequestedOutputEnumeration) ||
		contract.HasOutput(types.AnswerRequestedOutputTrace) ||
		contract.HasOutput(types.AnswerRequestedOutputAbsence) {
		return true
	}
	switch types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case types.ReqReturnValue,
		types.ReqConditional,
		types.ReqConfigMapping,
		types.ReqRegistration,
		types.ReqCallChain:
		return true
	}
	return false
}

func blockCoverageCaveatIsTelemetry(v types.Violation, rm *types.RequestModel, contract types.AnswerIntentContract) bool {
	blockKind := types.AnswerBlockKind(residualClusterValue(v.ClusterKey, "block_kind"))
	switch blockKind {
	case types.BlockDiagram:
		return !contract.HasOutput(types.AnswerRequestedOutputDiagram)
	case types.BlockScalar, types.BlockDecision:
		return !contract.HasOutput(types.AnswerRequestedOutputScalar) &&
			!contract.HasOutput(types.AnswerRequestedOutputKeyValue) &&
			!contract.HasOutput(types.AnswerRequestedOutputCount)
	case types.BlockOrderedList, types.BlockBulletList, types.BlockTable:
		if principalEnumerationSurfaceRequested(rm, contract) {
			return false
		}
		if blockKind == types.BlockTable && contract.HasOutput(types.AnswerRequestedOutputComparison) {
			return false
		}
		return true
	case types.BlockSummary, types.BlockSection, types.BlockCaveat, "":
		return true
	default:
		return false
	}
}

func principalEnumerationSurfaceRequested(rm *types.RequestModel, contract types.AnswerIntentContract) bool {
	if rm == nil {
		return contract.HasOutput(types.AnswerRequestedOutputEnumeration)
	}
	if typedCallChainAnswerSurface(rm, contract) &&
		rm.Intent != types.IntentEnumerate &&
		!rm.Predicates.IsCategoryEnumeration &&
		!rm.Predicates.IsCountQuestion {
		return false
	}
	if types.RequiresExhaustiveEnumerationMemberSetHandoff(*rm) ||
		types.RequiresRelationMemberSetHandoff(*rm) {
		return true
	}
	if rm.SourceInventoryProfile != nil && rm.SourceInventoryProfile.Active() &&
		(rm.Intent == types.IntentEnumerate || rm.Predicates.IsCategoryEnumeration || rm.QuestionStructure().HasAnyObligation()) {
		return true
	}
	if rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsScalarAnswer {
		return false
	}
	if rm.Intent == types.IntentEnumerate || rm.Predicates.IsCategoryEnumeration {
		return contract.HasOutput(types.AnswerRequestedOutputEnumeration)
	}
	if rm.Predicates.IsCrossComponent ||
		types.IsArchitectureNarrativeExplanation(*rm) ||
		types.IsSingleTopicMechanismExplanation(*rm) {
		return false
	}
	return contract.HasOutput(types.AnswerRequestedOutputEnumeration) ||
		rm.Intent == types.IntentEnumerate ||
		rm.Predicates.IsCategoryEnumeration
}

func typedCallChainAnswerSurface(rm *types.RequestModel, contract types.AnswerIntentContract) bool {
	if rm == nil {
		return contract.HasOutput(types.AnswerRequestedOutputTrace)
	}
	kind := types.NormalizeRequirementKind(rm.AnalyzerHints.Kind)
	return kind == types.ReqCallChain ||
		rm.Intent == types.IntentTrace ||
		contract.HasOutput(types.AnswerRequestedOutputTrace)
}

func runtimeWaiverObservationOnly(w *types.EvidenceFloorWaiver) bool {
	if !w.IsActive() {
		return false
	}
	switch w.Reason {
	case types.EvidenceFloorWaiverExternalLog,
		types.EvidenceFloorWaiverExternalTrace,
		types.EvidenceFloorWaiverNoRepoIntersection,
		types.EvidenceFloorWaiverInformationalRuntime:
		return true
	default:
		return false
	}
}

func splitRuntimeObservationOnlyCaveats(violations []types.Violation) (remaining []types.Violation, needsBoundary bool) {
	for _, v := range violations {
		switch v.Kind {
		case types.ViolUncertaintyBlockMissing:
			needsBoundary = true
			continue
		case types.ViolBlockCoverageMissing:
			if runtimeObservationOnlyBlockCoverageIsImplicit(v) {
				continue
			}
		}
		remaining = append(remaining, v)
	}
	return remaining, needsBoundary
}

func runtimeObservationOnlyBlockCoverageIsImplicit(v types.Violation) bool {
	blockKind := residualClusterValue(v.ClusterKey, "block_kind")
	switch types.AnswerBlockKind(blockKind) {
	case types.BlockSummary, types.BlockCaveat:
		return true
	default:
		return false
	}
}

func runtimeObservationBoundaryCaveat(ctx *types.BusContext, lang string) string {
	kind := "log"
	if ctx != nil && ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil && rm.PerfTrace != nil && rm.PerfTrace.IsExternalSource() {
			kind = "trace"
		}
		if w := ctx.Mutable.EvidenceFloorWaiver(); w != nil && w.Reason == types.EvidenceFloorWaiverExternalTrace {
			kind = "trace"
		}
		if w := ctx.Mutable.StableEvidenceFloorWaiver(); w != nil && w.Reason == types.EvidenceFloorWaiverExternalTrace {
			kind = "trace"
		}
	}
	if isChineseLang(lang) {
		if kind == "trace" {
			return "这份运行时 trace 的帧或 span 未映射到当前仓库；上面的结论只说明附件中实际观察到的现象，不证明当前 checkout 仍存在相同源码路径。"
		}
		return "这份运行时日志的栈帧未映射到当前仓库；上面的结论只说明附件中实际观察到的错误，不证明当前 checkout 仍存在相同源码路径。"
	}
	if kind == "trace" {
		return "The runtime trace frames or spans did not resolve to the current repository; the answer states what the attachment observed and does not prove the current checkout still contains the same source path."
	}
	return "The runtime log stack frames did not resolve to the current repository; the answer states what the attachment observed and does not prove the current checkout still contains the same source path."
}

// AppendSystemCaveatString renders ONE pre-formatted system caveat
// as a trailing markdown section appended to the answer text. Same
// channel as AppendUserCaveatsToAnswer (the "**补充说明：**" /
// "**Additional notes:**" heading) but bypasses the
// MaterializeUnresolvedViolationsAsCaveats Violation→template
// path: callers that already have a final-form caveat string
// (e.g. P6's softFinalizeRepairCapMessage) write it directly.
//
// Why this matters (P7, 2026-05-10): the AnswerDocumentV2.Caveats[]
// channel is for LLM-authored caveats — content gaps the LLM
// itself identified. System-injected caveats (orchestrator decisions
// about repair-loop termination, scope boundaries, etc.) MUST NOT
// pollute the LLM-authored slot — that violates
// feedback_no_system_backfill_to_user_panel. They go through this
// system-side helper instead, keeping the two channels visually
// distinguishable in the rendered answer.
//
// Empty / whitespace-only caveat returns the answer unchanged. Use
// AppendUserCaveatsToAnswer when the input is a list of
// types.Violation; use this when the input is a single already-
// composed user-facing string.
func AppendSystemCaveatString(answer, caveat, lang string) string {
	caveat = strings.TrimSpace(caveat)
	if caveat == "" {
		return answer
	}
	return appendSystemCaveatBullets(answer, []string{caveat}, lang)
}

// AppendResidualConcernDetailsToAnswer renders the finalize-repair
// hard-cap disclosure. Unlike AppendUserCaveatsToAnswer, this path
// intentionally preserves per-violation detail from the typed
// contract/reviewer results so "N residual concern(s)" is followed
// by the actual unresolved items. It still stays in system-caveat
// vocabulary: no ViolKind names, no IR fields, no confidence scores.
func AppendResidualConcernDetailsToAnswer(answer string, violations []types.Violation, lang string) string {
	bullets := MaterializeResidualConcernDetails(violations, lang)
	if len(bullets) == 0 {
		return answer
	}
	return appendSystemCaveatBullets(answer, bullets, lang)
}

// MaterializeResidualConcernDetails converts unresolved violations
// into the detailed variant used only when the finalize-stage repair
// hard cap is reached. Inputs are typed system/model outputs; this
// helper does not infer answer facts or repair missing answer content.
func MaterializeResidualConcernDetails(violations []types.Violation, lang string) []string {
	if len(violations) == 0 {
		return nil
	}
	useChinese := isChineseLang(lang)
	limit := len(violations)
	if limit > MaxMaterializedCaveats {
		limit = MaxMaterializedCaveats
	}
	out := make([]string, 0, limit+1)
	if useChinese {
		if len(violations) > limit {
			out = append(out, fmt.Sprintf("质量审阅仍有 %d 项未完全解决，以下列出前 %d 项可见边界。", len(violations), limit))
		} else {
			out = append(out, fmt.Sprintf("质量审阅仍有 %d 项未完全解决，以下列出可见边界。", len(violations)))
		}
	} else {
		if len(violations) > limit {
			out = append(out, fmt.Sprintf("Quality review still has %d unresolved concern(s); showing the first %d visible boundary item(s).", len(violations), limit))
		} else {
			out = append(out, fmt.Sprintf("Quality review still has %d unresolved concern(s); the visible boundary item(s) are listed below.", len(violations)))
		}
	}

	seen := map[string]bool{}
	for _, v := range violations {
		if len(out) > limit {
			break
		}
		line := residualConcernLine(v, useChinese)
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		out = append(out, line)
	}
	if len(out) == 1 {
		for _, c := range MaterializeUnresolvedViolationsAsCaveats(violations, lang) {
			c = strings.TrimSpace(c)
			if c == "" || seen[c] {
				continue
			}
			seen[c] = true
			out = append(out, c)
			if len(out) > limit {
				break
			}
		}
	}
	if len(out) == 1 {
		return nil
	}
	return out
}

func residualConcernLine(v types.Violation, useChinese bool) string {
	switch v.Kind {
	case types.ViolMustInclude:
		if term := residualClusterValue(v.ClusterKey, "term"); term != "" {
			if useChinese {
				return "答案仍需明确提及 " + inlineCode(term) + "。"
			}
			return "The answer still needs to explicitly mention " + inlineCode(term) + "."
		}
	case types.ViolAnswerSemanticUnderfilled, types.ViolAnswerTopicMismatch:
		topic := residualClusterValue(v.ClusterKey, "topic")
		observation := residualObservation(v.Detail)
		suggestion := residualSuggestion(v.Repair)
		if topic != "" || observation != "" || suggestion != "" {
			return residualTopicLine(topic, observation, suggestion, useChinese)
		}
	}

	subject := firstResidualClusterValue(v.ClusterKey, "term", "symbol", "topic", "label", "file", "path", "block", "block_kind", "relation")
	if subject != "" {
		if useChinese {
			return "与 " + inlineCode(subject) + " 相关的检查仍需补充验证。"
		}
		return "The check related to " + inlineCode(subject) + " still needs additional verification."
	}
	if caveats := MaterializeUnresolvedViolationsAsCaveats([]types.Violation{v}, langFromChinese(useChinese)); len(caveats) > 0 {
		return caveats[0]
	}
	return ""
}

func residualTopicLine(topic, observation, suggestion string, useChinese bool) string {
	subject := strings.TrimSpace(topic)
	body := strings.TrimSpace(suggestion)
	if observation != "" {
		body = observation
	}
	if subject == "" {
		if useChinese {
			return "仍有一处答案覆盖问题：" + trimSentence(body)
		}
		return "One answer-coverage concern remains: " + trimSentence(body)
	}
	if body == "" {
		if useChinese {
			return "关于 " + inlineCode(subject) + " 的覆盖仍需补充。"
		}
		return "Coverage for " + inlineCode(subject) + " still needs to be completed."
	}
	if useChinese {
		return "关于 " + inlineCode(subject) + "：" + trimSentence(body)
	}
	return "For " + inlineCode(subject) + ": " + trimSentence(body)
}

func residualClusterValue(clusterKey, key string) string {
	prefix := strings.TrimSpace(key) + ":"
	for _, part := range strings.Split(clusterKey, "|") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(part, prefix))
		}
	}
	return ""
}

func firstResidualClusterValue(clusterKey string, keys ...string) string {
	for _, key := range keys {
		if v := residualClusterValue(clusterKey, key); v != "" {
			return v
		}
	}
	return ""
}

func residualObservation(detail string) string {
	const marker = "observation:"
	idx := strings.Index(detail, marker)
	if idx < 0 {
		return ""
	}
	return trimSentence(detail[idx+len(marker):])
}

func residualSuggestion(repair string) string {
	repair = strings.TrimSpace(repair)
	if repair == "" {
		return ""
	}
	if idx := strings.Index(repair, "Reviewer rationale:"); idx >= 0 {
		repair = strings.TrimSpace(repair[:idx])
	}
	if idx := strings.Index(repair, ". "); idx >= 0 && idx+2 < len(repair) {
		repair = strings.TrimSpace(repair[idx+2:])
	}
	return trimSentence(repair)
}

func trimSentence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, " \t\r\n—-")
	return s
}

func inlineCode(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "`", "")
	return "`" + s + "`"
}

func langFromChinese(useChinese bool) string {
	if useChinese {
		return "zh"
	}
	return "en"
}

func appendSystemCaveatBullets(answer string, caveats []string, lang string) string {
	filtered := make([]string, 0, len(caveats))
	seen := map[string]bool{}
	for _, caveat := range caveats {
		caveat = strings.TrimSpace(caveat)
		if caveat == "" || seen[caveat] {
			continue
		}
		seen[caveat] = true
		filtered = append(filtered, caveat)
	}
	if len(filtered) == 0 {
		return answer
	}
	heading := "**Additional notes:**"
	if isChineseLang(lang) {
		heading = "**补充说明：**"
	}
	base := strings.TrimRight(answer, "\n")
	var b strings.Builder
	b.WriteString(base)
	if strings.Contains(base, heading) {
		b.WriteString("\n")
	} else {
		b.WriteString("\n\n")
		b.WriteString(heading)
		b.WriteString("\n\n")
	}
	for _, c := range filtered {
		b.WriteString("- ")
		b.WriteString(c)
		b.WriteString("\n")
	}
	return b.String()
}

// isChineseLang accepts the same set of variants the renderer
// already recognises in answerDocumentRequiresChinese (the canonical
// gate is in tool/emit_answer_document.go). Duplicated here as a
// pure helper so the materializer has no cross-package import on
// the tool layer.
func isChineseLang(lang string) bool {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh", "zh-cn", "cn", "chinese":
		return true
	case "":
		// Default project language is zh — see config defaultLang.
		// Empty input most commonly means BusContext.Language was
		// never populated, which currently corresponds to the zh
		// default.
		return true
	}
	return false
}

package tool

import (
	"regexp"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

var (
	runtimeCitationRefSentinelRe = regexp.MustCompile("`?citation_ref`?\\s*(?:=|:|：)\\s*`?-1`?")
	runtimeCitationRefMarkedRe   = regexp.MustCompile("`?citation_ref`?\\s*(?:被)?(?:标记|设置|置|设|记)为\\s*`?-1`?")
)

func answerDocumentRuntimeObservationOnly(ctx *types.BusContext) bool {
	plan := answerSurfacePlan(ctx)
	if plan == nil ||
		!plan.RuntimeGroundingDisposition.IsActive() ||
		plan.CurrentStatusDiagnosticRequired ||
		plan.CurrentSourceEvidenceOrigin {
		return false
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if !authority.Active {
		return true
	}
	return runtimeSourceAuthorityAllowsObservationOnlyRuntimeSurface(authority)
}

func answerDocumentRuntimeArtifactWithoutRequiredCurrentSource(ctx *types.BusContext) bool {
	if answerDocumentRuntimeObservationOnly(ctx) {
		return true
	}
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if runtimeSourceAuthorityAppliesToArtifactCitationCleanup(ctx, authority) {
		return runtimeSourceAuthorityAllowsArtifactCitationCleanup(authority)
	}
	if !ctx.AnalysisIR.RequestModel.HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext(types.RuntimeArtifactContextActiveFromBus(ctx)) {
		return false
	}
	return !answerDocumentHasCurrentSourceObservationSupport(ctx)
}

func runtimeSourceAuthorityAppliesToArtifactCitationCleanup(ctx *types.BusContext, authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	if !authority.Active {
		return false
	}
	if authority.HasRuntimeCarrier() || authority.CurrentSourceSatisfied {
		return true
	}
	return false
}

func runtimeSourceAuthorityAllowsArtifactCitationCleanup(authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	return authority.AllowsRuntimeEvidenceWithoutCurrentSource()
}

func runtimeSourceAuthorityAllowsObservationOnlyRuntimeSurface(authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	if !authority.Active {
		return true
	}
	if authority.KeepsCurrentSourceLaneLoadBearing() {
		return false
	}
	return authority.AllowsRuntimeEvidenceWithoutCurrentSource()
}

func answerDocumentHasCurrentSourceObservationSupport(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, 64))
	for _, record := range ledger.Records {
		if record.Origin == types.AnswerEvidenceOriginCurrentSource {
			return true
		}
	}
	return false
}

func answerDocumentHasLoadBearingCurrentSourceLane(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if authority.Active {
		return authority.KeepsCurrentSourceLaneLoadBearing()
	}
	return false
}

func answerDocumentExternalObservationOnly(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx, types.ObservationLedger{})
	if runtimeSourceAuthorityDisablesExternalObservationOnly(authority) {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	contract := types.CompileAnswerIntentContract(rm, &ctx.AnalysisIR.AnswerContract)
	hasExternal := false
	for _, origin := range contract.Origins {
		switch origin {
		case types.AnswerEvidenceOriginCurrentSource:
			// The compiled contract can include current_source as a conservative
			// default for generic/count shapes. Do not let that default alone
			// suppress cleanup when the accepted ledger is purely external; real
			// current-source answers are blocked by the answer surface plan or
			// by current-source ledger records below.
			continue
		case types.AnswerEvidenceOriginUnknown, types.AnswerEvidenceOriginSystemInference:
			continue
		default:
			if types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin) {
				hasExternal = true
			}
		}
	}
	if plan := answerSurfacePlan(ctx); plan != nil {
		if plan.CurrentStatusDiagnosticRequired {
			return false
		}
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, 64))
	for _, record := range ledger.Records {
		switch record.Origin {
		case types.AnswerEvidenceOriginCurrentSource:
			return false
		case types.AnswerEvidenceOriginUnknown, types.AnswerEvidenceOriginSystemInference:
			continue
		default:
			if types.AnswerEvidenceOriginCarriesOriginSpecificSupport(record.Origin) {
				hasExternal = true
			}
		}
	}
	return hasExternal
}

func runtimeSourceAuthorityDisablesExternalObservationOnly(authority types.RuntimeSourceAnswerAuthoritySnapshot) bool {
	return authority.KeepsCurrentSourceLaneLoadBearing()
}

func answerDocumentHasExternalObservationSupport(ctx *types.BusContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	contract := types.CompileAnswerIntentContract(rm, &ctx.AnalysisIR.AnswerContract)
	for _, origin := range contract.Origins {
		if answerEvidenceOriginIsExternalObservationSupport(origin) {
			return true
		}
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, 64))
	for _, record := range ledger.Records {
		if answerEvidenceOriginIsExternalObservationSupport(record.Origin) {
			return true
		}
	}
	return false
}

func answerEvidenceOriginIsExternalObservationSupport(origin types.AnswerEvidenceOrigin) bool {
	switch origin {
	case types.AnswerEvidenceOriginCurrentSource,
		types.AnswerEvidenceOriginUnknown,
		types.AnswerEvidenceOriginSystemInference:
		return false
	default:
		return types.AnswerEvidenceOriginCarriesOriginSpecificSupport(origin)
	}
}

// normalizeRuntimeArtifactCitationRefs removes citation-pool entries that
// would make an attached runtime artifact look like current-repo source proof.
// The visible answer content is preserved; only citation_ref carriers that
// pointed at the artifact-side coordinate are downgraded to -1.
func normalizeRuntimeArtifactCitationRefs(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil || len(doc.Citations) == 0 {
		return 0
	}
	if answerDocumentRuntimeArtifactWithoutRequiredCurrentSource(ctx) {
		remove := make(map[int]bool)
		for i, cit := range doc.Citations {
			if types.LooksLikeRuntimeArtifactPath(cit.File) {
				remove[i] = true
			}
		}
		if len(remove) > 0 {
			return dropAnswerDocumentCitationsByIndex(doc, remove)
		}
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || !plan.IsCrashSourcedRootCause() {
		return 0
	}
	remove := make(map[int]bool)
	if plan.RuntimeGroundingDisposition.IsActive() &&
		!plan.CurrentStatusDiagnosticRequired &&
		!plan.CurrentSourceEvidenceOrigin &&
		!answerDocumentHasLoadBearingCurrentSourceLane(ctx) &&
		!answerDocumentHasCurrentSourceObservationSupport(ctx) {
		for i := range doc.Citations {
			remove[i] = true
		}
	} else {
		for i, cit := range doc.Citations {
			if _, _, ok := citationMatchesDriftedArtifactFrame(plan, cit); ok {
				remove[i] = true
			}
		}
	}
	if len(remove) == 0 {
		return 0
	}
	return dropAnswerDocumentCitationsByIndex(doc, remove)
}

// normalizeExternalObservationPseudoCitations removes non-positive line-shaped
// citation carriers when the run has typed external observation support. A
// current-source citation must be a real file:line (or an explicit non-line
// scope such as ScopeFile/ScopeSection); VCS paths, command rows, log lines, and
// trace spans should travel through ObservationSourceRef/ObservationSpan instead
// of masquerading as `file:0` citations. The model-authored answer surface is
// preserved; only invalid citation carriers are detached.
func normalizeExternalObservationPseudoCitations(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || len(doc.Citations) == 0 || !answerDocumentHasExternalObservationSupport(ctx) {
		return 0
	}
	remove := make(map[int]bool)
	for i, cit := range doc.Citations {
		if citationIsPseudoCurrentSourceCarrier(cit) {
			remove[i] = true
		}
	}
	if len(remove) == 0 {
		return 0
	}
	return dropAnswerDocumentCitationsByIndex(doc, remove)
}

func citationIsPseudoCurrentSourceCarrier(cit types.Citation) bool {
	if cit.Line > 0 {
		return false
	}
	switch cit.Scope {
	case "", types.ScopeLine, types.ScopeLineRange:
		return true
	case types.ScopeSection:
		return strings.TrimSpace(cit.SectionPath) == ""
	default:
		return false
	}
}

func dropAnswerDocumentCitationsByIndex(doc *types.AnswerDocumentV2, remove map[int]bool) int {
	if doc == nil || len(remove) == 0 {
		return 0
	}
	oldLen := len(doc.Citations)
	remap := make(map[int]int, oldLen)
	next := make([]types.Citation, 0, oldLen-len(remove))
	for i, cit := range doc.Citations {
		if remove[i] {
			continue
		}
		remap[i] = len(next)
		next = append(next, cit)
	}
	changed := oldLen - len(next)
	doc.Citations = next
	for bi := range doc.Blocks {
		for ii := range doc.Blocks[bi].Items {
			ref := doc.Blocks[bi].Items[ii].CitationRef
			if ref < 0 {
				continue
			}
			if remove[ref] {
				doc.Blocks[bi].Items[ii].CitationRef = -1
				changed++
				continue
			}
			if mapped, ok := remap[ref]; ok && mapped != ref {
				doc.Blocks[bi].Items[ii].CitationRef = mapped
				changed++
			}
		}
	}
	return changed
}

func normalizeUnusedCitationPoolEntries(doc *types.AnswerDocumentV2) int {
	if doc == nil || len(doc.Citations) == 0 {
		return 0
	}
	used := make(map[int]bool)
	for _, block := range doc.Blocks {
		for _, item := range block.Items {
			if item.CitationRef >= 0 && item.CitationRef < len(doc.Citations) {
				used[item.CitationRef] = true
			}
		}
	}
	if len(used) == 0 || len(used) == len(doc.Citations) {
		return 0
	}
	remove := make(map[int]bool, len(doc.Citations)-len(used))
	for i := range doc.Citations {
		if !used[i] {
			remove[i] = true
		}
	}
	return dropAnswerDocumentCitationsByIndex(doc, remove)
}

// normalizeRuntimeArtifactVisibleCitationSentinels removes internal
// citation-carrier vocabulary that a model copied into visible runtime-artifact
// prose. The decision is typed: only observation-only attached log/trace
// answers enter this path. Current-source and Codrax-internal answers keep their
// literal text untouched.
func normalizeRuntimeArtifactVisibleCitationSentinels(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || !answerDocumentRuntimeArtifactWithoutRequiredCurrentSource(ctx) {
		return 0
	}
	return normalizeVisibleCitationSentinels(doc, runtimeArtifactVisibleCitationBoundaryText(ctx))
}

// normalizeRuntimeObservationOnlyDecisionBlocks removes optional current-status
// style decision carriers from observation-only runtime answers. The decision is
// typed: it fires only when the answer surface plan says runtime artifact facts
// are sufficient and current-source/status verdict lanes are inactive. This
// keeps trace/log answers from rendering stale `still_present`/`fixed` verdict
// prose as a principal conclusion while preserving the model's summary/list
// payload.
func normalizeRuntimeObservationOnlyDecisionBlocks(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctx *types.BusContext) int {
	if doc == nil || view == nil || !answerDocumentRuntimeArtifactWithoutRequiredCurrentSource(ctx) {
		return 0
	}
	if view.CurrentStatusDiagnostic != nil && view.CurrentStatusDiagnostic.Required {
		return 0
	}
	if view.ErrorGranularityProfile != nil && view.ErrorGranularityProfile.Active() {
		return 0
	}
	if answerDocumentHasCurrentSourceCitationCarrier(doc) {
		return 0
	}
	if !answerDocumentHasNonDecisionVisiblePayload(doc) {
		return 0
	}
	removed := 0
	blocks := doc.Blocks[:0]
	for _, block := range doc.Blocks {
		if block.Kind == types.BlockDecision {
			removed++
			continue
		}
		blocks = append(blocks, block)
	}
	doc.Blocks = blocks
	return removed
}

func answerDocumentHasNonDecisionVisiblePayload(doc *types.AnswerDocumentV2) bool {
	if doc == nil {
		return false
	}
	for _, block := range doc.Blocks {
		if block.Kind == types.BlockDecision {
			continue
		}
		if preEmitBlockHasVisiblePayload(block) {
			return true
		}
	}
	return false
}

func answerDocumentHasCurrentSourceCitationCarrier(doc *types.AnswerDocumentV2) bool {
	if doc == nil {
		return false
	}
	for _, cit := range doc.Citations {
		if cit.Line <= 0 || strings.TrimSpace(cit.File) == "" {
			continue
		}
		if !types.LooksLikeRuntimeArtifactPath(cit.File) {
			return true
		}
	}
	return false
}

func normalizeExternalObservationVisibleCitationSentinels(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || answerDocumentRuntimeObservationOnly(ctx) || !answerDocumentExternalObservationOnly(ctx) {
		return 0
	}
	return normalizeVisibleCitationSentinels(doc, externalObservationVisibleCitationBoundaryText(ctx))
}

func normalizeVisibleCitationSentinels(doc *types.AnswerDocumentV2, replacement string) int {
	if doc == nil {
		return 0
	}
	changed := 0
	normalize := func(ptr *string) {
		if ptr == nil || strings.TrimSpace(*ptr) == "" {
			return
		}
		next := sanitizeRuntimeArtifactVisibleCitationSentinel(*ptr, replacement)
		if next != *ptr {
			*ptr = next
			changed++
		}
	}
	for i := range doc.Caveats {
		normalize(&doc.Caveats[i])
	}
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		normalize(&block.Title)
		normalize(&block.Text)
		for ii := range block.Items {
			item := &block.Items[ii]
			normalize(&item.Label)
			normalize(&item.Text)
			for ci := range item.Cells {
				normalize(&item.Cells[ci])
			}
		}
	}
	return changed
}

func sanitizeRuntimeArtifactVisibleCitationSentinel(s, replacement string) string {
	if s == "" {
		return s
	}
	out := runtimeCitationRefMarkedRe.ReplaceAllString(s, replacement)
	out = runtimeCitationRefSentinelRe.ReplaceAllString(out, replacement)
	return out
}

func runtimeArtifactVisibleCitationBoundaryText(ctx *types.BusContext) string {
	if ctx != nil && strings.HasPrefix(strings.ToLower(strings.TrimSpace(ctx.Language)), "en") {
		return "runtime-artifact observation, not a current-repository source citation"
	}
	return "来自附件运行时材料，未绑定当前仓库源码引用"
}

func externalObservationVisibleCitationBoundaryText(ctx *types.BusContext) string {
	if ctx != nil && strings.HasPrefix(strings.ToLower(strings.TrimSpace(ctx.Language)), "en") {
		return "external observation, not a current-repository source citation"
	}
	return "来自外部观察结果，未绑定当前仓库源码引用"
}

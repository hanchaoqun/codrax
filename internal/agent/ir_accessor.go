package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The ir* helpers read analyzer-sourced hints from the AnalysisIR
// populated by the analyze stage. They return zero values when the IR
// is nil (analyze failed, or unit tests that skip the analyze stage).
//
// Before batch B5b-β these helpers fell back to legacy CurrentTask*
// fields on AgentContext; those fields were deleted along with their
// TaskItem siblings.

func irKeywords(ctx *types.AgentContext) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return ctx.AnalysisIR.RequestModel.AnalyzerHints.Keywords
}

func irEntities(ctx *types.AgentContext) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return ctx.AnalysisIR.RequestModel.AnalyzerHints.Entities
}

func irEntityProvenance(ctx *types.AgentContext) []types.EntityProvenance {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return ctx.AnalysisIR.RequestModel.AnalyzerHints.EntityProvenance
}

// irPrimaryEntities returns the pre-merge top-level entity snapshot
// captured by the analyzer before sub-topic entities were unioned into
// Entities for breadth ranking. Returns nil when no sub-topic merge
// ran (single-topic questions) or when IR is missing — callers should
// fall back to irEntities for the merged list.
func irPrimaryEntities(ctx *types.AgentContext) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return ctx.AnalysisIR.RequestModel.AnalyzerHints.PrimaryEntities
}

func irMentionedEntities(ctx *types.AgentContext) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return ctx.AnalysisIR.RequestModel.AnalyzerHints.MentionedEntities
}

// irDomainHints returns the unique non-empty Domain tags attached to
// every TermSymbol in the analyzer's TermGraph. Populated by the
// normalizer's SymbolResolver when a term is repo-grounded; empty when
// the resolver was unavailable or the TermGraph has no symbol terms.
// Consumed by keyword_search to boost files whose FileInfo.Package
// matches any hint — i.e., siblings of the answer symbol in the same
// package — without naming the symbol directly.
func irDomainHints(ctx *types.AgentContext) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, c := range ctx.AnalysisIR.RequestModel.TermGraph.Canonical {
		if c.Kind != types.TermSymbol || c.Domain == "" || seen[c.Domain] {
			continue
		}
		seen[c.Domain] = true
		out = append(out, c.Domain)
	}
	return out
}

func irQuestionKind(ctx *types.AgentContext) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	return ctx.AnalysisIR.RequestModel.AnalyzerHints.Kind
}

// irQuestionFamily returns the resolved QuestionFamily for the
// current request as a string. Replaces the legacy irAnswerShape
// helper per docs/migration/answer_shape_retirement.md — downstream
// stage-report consumers see the typed family classification rather
// than the LLM-emitted shape hint.
func irQuestionFamily(ctx *types.AgentContext) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	return string(types.ResolveQuestionFamily(ctx.AnalysisIR.RequestModel))
}

// irComplexity returns the analyzer-classified complexity for the
// current request, or the empty string when the IR is missing (unit
// tests, or an analyze-phase failure that still let the pipeline
// continue). Callers that branch on this value should treat "" the
// same as ComplexityModerate so the pre-complexity-aware behavior
// is the safe default.
//
// Complexity flows from the analyzer's emit_analysis call (field
// `complexity`) through analyzer_complexity.reconcileComplexity
// (which can override the LLM choice on structural signals) into
// RequestModel.Complexity. The explorer reads it via this helper
// to scale its own runtime budgets: keyword-search top-N cap
// (T1b), ERM requirement-satisfaction thresholds (T1c), and
// potential future per-complexity behaviors.
func irComplexity(ctx *types.AgentContext) types.Complexity {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	return ctx.AnalysisIR.RequestModel.Complexity
}

func irExactResolutionContract(ctx *types.AgentContext) *types.ExactResolutionContract {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return ctx.AnalysisIR.AnswerContract.ExactResolution
}

func irSourceScopeProfile(ctx *types.AgentContext) *types.SourceScopeProfile {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return sourceScopeProfileForRequestModel(ctx.AnalysisIR.RequestModel)
}

func sourceScopeProfileForRequestModel(rm types.RequestModel) *types.SourceScopeProfile {
	if rm.SourceScopeProfile != nil {
		return rm.SourceScopeProfile
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		switch rm.ChangeImpactProfile.Scope {
		case types.ImpactScopeTest:
			return &types.SourceScopeProfile{RequestedScope: types.SourceScopeTest, IncludeAuxiliaryAsPrincipal: true, Confidence: 1}
		case types.ImpactScopeAll:
			return &types.SourceScopeProfile{RequestedScope: types.SourceScopeAll, IncludeAuxiliaryAsPrincipal: true, Confidence: 1}
		case types.ImpactScopeProduction:
			return &types.SourceScopeProfile{RequestedScope: types.SourceScopeProduction, Confidence: 1}
		}
	}
	if requestModelMentionsAuxiliaryPath(rm) {
		return &types.SourceScopeProfile{RequestedScope: types.SourceScopeAll, IncludeAuxiliaryAsPrincipal: true, Confidence: 1}
	}
	return &types.SourceScopeProfile{RequestedScope: types.SourceScopeProduction, Confidence: 0.5}
}

func requestModelMentionsAuxiliaryPath(rm types.RequestModel) bool {
	for _, target := range rm.AnalyzerHints.ExactTargets {
		if types.SourcePathRoleIsAuxiliary(types.ClassifySourcePathRole(target)) {
			return true
		}
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if types.SourcePathRoleIsAuxiliary(types.ClassifySourcePathRole(hint.Path)) {
			return true
		}
	}
	if rm.ConversationReferenceProfile != nil {
		for _, subject := range rm.ConversationReferenceProfile.ResolvedSubjects {
			if types.SourcePathRoleIsAuxiliary(types.ClassifySourcePathRole(subject.Surface)) {
				return true
			}
		}
	}
	return false
}

func observationOnlyRuntimeArtifactForExplorer(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	return ctx.AnalysisIR.RequestModel.HasObservationOnlyRuntimeArtifact()
}

func runtimeSourceHardCurrentSourceObligationForExplorer(ctx *types.AgentContext) bool {
	authority := runtimeSourceAnswerAuthorityForExplorer(ctx)
	if authority.Active {
		return authority.CanHardBlockCompletion
	}
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	return ctx.AnalysisIR.RequestModel.CurrentSourceLaneDecision().RequiresCurrentSource()
}

func runtimeSourceSoftCurrentSourceObligationForExplorer(ctx *types.AgentContext) bool {
	authority := runtimeSourceAnswerAuthorityForExplorer(ctx)
	return authority.Active &&
		authority.CurrentSourceRequirement == types.RuntimeSourceRequirementSoft &&
		authority.NeedsCurrentSourceEvidence
}

func runtimeSourceAnswerAuthorityForExplorer(ctx *types.AgentContext) types.RuntimeSourceAnswerAuthoritySnapshot {
	if ctx == nil || ctx.AnalysisIR == nil {
		return types.RuntimeSourceAnswerAuthoritySnapshot{}
	}
	return types.BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx, types.ObservationLedger{})
}

func externalObservationFirstSourceOptionalForExplorer(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil || !ctx.TurnRouteHint.ExternalObservationFirst() {
		return false
	}
	authority := runtimeSourceAnswerAuthorityForExplorer(ctx)
	if authority.Active {
		if authority.CanHardBlockCompletion ||
			authority.CurrentSourceRequirement == types.RuntimeSourceRequirementPrecise ||
			authority.CurrentSourceSatisfied {
			return false
		}
		return true
	}
	return true
}

func runtimeArtifactWithoutRequiredSourceForExplorer(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if runtimeSourceHardCurrentSourceObligationForExplorer(ctx) {
		return false
	}
	return rm.HasRuntimeArtifactWithoutRequiredCurrentSource()
}

func runtimeArtifactObservationOnlySurfaceForExplorer(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if runtimeSourceHardCurrentSourceObligationForExplorer(ctx) {
		return false
	}
	if runtimeSourceSoftCurrentSourceObligationForExplorer(ctx) {
		return false
	}
	return rm.HasRuntimeArtifactObservationOnlySurface()
}

func runtimeArtifactSourceOptionalMixedSurfaceForExplorer(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if runtimeSourceHardCurrentSourceObligationForExplorer(ctx) {
		return false
	}
	if runtimeSourceSoftCurrentSourceObligationForExplorer(ctx) &&
		rm.HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext(types.RuntimeArtifactContextActiveFromAgent(ctx)) {
		return true
	}
	return rm.HasRuntimeArtifactSourceOptionalMixedSurface()
}

func explorerHasTraceQueryRuntimeTraceCarrier(ctx *types.AgentContext) bool {
	if ctx == nil {
		return false
	}
	if strings.TrimSpace(ctx.AttachedHitrace) != "" || ctx.PerfTrace != nil {
		return true
	}
	if ctx.Mutable != nil && ctx.Mutable.PerfTrace() != nil {
		return true
	}
	if ctx.AnalysisIR == nil {
		return false
	}
	return ctx.AnalysisIR.RequestModel.PerfTrace != nil
}

func originSpecificObservationWithoutRequiredSourceForExplorer(ctx *types.AgentContext, facts []types.AnswerAggregateFact) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if originSpecificCompletionCurrentSourceBlockedForExplorer(ctx) {
		return false
	}
	for _, fact := range facts {
		origins := types.AnswerAggregateFactEvidenceOrigins(fact, &rm)
		if !types.AnswerEvidenceOriginsAreOriginSpecificOnly(origins) {
			continue
		}
		for _, origin := range origins {
			if origin == types.AnswerEvidenceOriginRuntimeArtifact ||
				origin == types.AnswerEvidenceOriginUnknown {
				continue
			}
			return true
		}
	}
	return false
}

func originSpecificCompletionCurrentSourceBlockedForExplorer(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	authority := runtimeSourceAnswerAuthorityForExplorer(ctx)
	if authority.Active && authority.CurrentSourceRequirement != types.RuntimeSourceRequirementNone {
		return authority.CanHardBlockCompletion ||
			authority.CurrentSourceRequirement == types.RuntimeSourceRequirementPrecise ||
			authority.CurrentSourceSatisfied
	}
	return rm.RequiresCurrentSourceForExternalObservation(&ctx.AnalysisIR.AnswerContract)
}

func observationOnlyRuntimeArtifactForAnalyzer(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.Stage != types.StageAnalyze {
		return false
	}
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			return rm.HasObservationOnlyRuntimeArtifact()
		}
	}
	return false
}

func runtimeArtifactWithoutRequiredSourceForAnalyzer(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.Stage != types.StageAnalyze {
		return false
	}
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			return rm.HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext(types.RuntimeArtifactContextActiveFromAgent(ctx))
		}
	}
	if ctx.AnalysisIR != nil {
		return ctx.AnalysisIR.RequestModel.HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext(types.RuntimeArtifactContextActiveFromAgent(ctx))
	}
	return false
}

func runtimeArtifactAttachmentPendingAnalysisForAnalyzer(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.Stage != types.StageAnalyze {
		return false
	}
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			return false
		}
	}
	if ctx.AnalysisIR != nil {
		return false
	}
	return strings.TrimSpace(ctx.AttachedLog) != "" ||
		strings.TrimSpace(ctx.AttachedHitrace) != "" ||
		ctx.LogTriage != nil ||
		ctx.PerfTrace != nil
}

func analyzerHasAttachedTraceContext(ctx *types.AgentContext) bool {
	if ctx == nil {
		return false
	}
	if strings.TrimSpace(ctx.AttachedHitrace) != "" || ctx.PerfTrace != nil {
		return true
	}
	if ctx.Mutable == nil {
		return false
	}
	return ctx.Mutable.PerfTrace() != nil
}

// analyzerRequiredFilesFromIR returns the analyzer's EvidencePlan
// RequiredFiles list (T3a), or nil when the IR is unavailable. This
// is a deliberate thin wrapper so the explorer's consumer loop
// reads from a single, documented accessor instead of walking the
// nested IR → EvidencePlan → RequiredFiles path inline.
func analyzerRequiredFilesFromIR(ctx *types.AgentContext) []string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return ctx.AnalysisIR.EvidencePlan.RequiredFiles
}

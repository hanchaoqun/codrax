package types

type answerPlanCacheKey struct {
	analysisIR               *AnalysisIR
	mutableRevision          uint64
	evidenceLen              int
	flowFindingsLen          int
	answerChainsLen          int
	answerSymbolsLen         int
	answerSymbolCompleteness CompletenessClaim
}

func answerPlanCacheKeyForBus(ctx *BusContext) answerPlanCacheKey {
	if ctx == nil {
		return answerPlanCacheKey{}
	}
	key := answerPlanCacheKey{
		analysisIR:               ctx.AnalysisIR,
		evidenceLen:              len(ctx.EvidenceItems),
		flowFindingsLen:          len(ctx.FlowFindings),
		answerChainsLen:          len(ctx.AnswerChains),
		answerSymbolsLen:         len(ctx.AnswerSymbols),
		answerSymbolCompleteness: ctx.AnswerSymbolCompleteness,
	}
	if ctx.Mutable != nil {
		key.mutableRevision = ctx.Mutable.answerSurfaceRevisionValue()
	}
	return key
}

func (ctx *AgentContext) cachedAnswerSurfacePlan() *AnswerSurfacePlan {
	if ctx == nil {
		return nil
	}
	ctx.answerSurfaceCacheMu.Lock()
	defer ctx.answerSurfaceCacheMu.Unlock()
	return cloneAnswerSurfacePlan(ctx.answerSurfacePlan)
}

func (ctx *AgentContext) storeAnswerSurfacePlan(plan *AnswerSurfacePlan) {
	if ctx == nil {
		return
	}
	ctx.answerSurfaceCacheMu.Lock()
	defer ctx.answerSurfaceCacheMu.Unlock()
	ctx.answerSurfacePlan = cloneAnswerSurfacePlan(plan)
}

func (ctx *AgentContext) cachedAnswerSemanticView() *AnswerSemanticView {
	if ctx == nil {
		return nil
	}
	ctx.answerSurfaceCacheMu.Lock()
	defer ctx.answerSurfaceCacheMu.Unlock()
	return cloneAnswerSemanticView(ctx.answerSemanticView)
}

func (ctx *AgentContext) storeAnswerSemanticView(view *AnswerSemanticView) {
	if ctx == nil {
		return
	}
	ctx.answerSurfaceCacheMu.Lock()
	defer ctx.answerSurfaceCacheMu.Unlock()
	ctx.answerSemanticView = cloneAnswerSemanticView(view)
}

func (ctx *AgentContext) cachedAnswerSupportPlan() *AnswerSupportPlan {
	if ctx == nil {
		return nil
	}
	ctx.answerSurfaceCacheMu.Lock()
	defer ctx.answerSurfaceCacheMu.Unlock()
	return cloneAnswerSupportPlan(ctx.answerSupportPlan)
}

func (ctx *AgentContext) storeAnswerSupportPlan(plan *AnswerSupportPlan) {
	if ctx == nil {
		return
	}
	ctx.answerSurfaceCacheMu.Lock()
	defer ctx.answerSurfaceCacheMu.Unlock()
	ctx.answerSupportPlan = cloneAnswerSupportPlan(plan)
}

func (ctx *BusContext) cachedAnswerSurfacePlan() *AnswerSurfacePlan {
	if ctx == nil {
		return nil
	}
	key := answerPlanCacheKeyForBus(ctx)
	ctx.answerSurfaceCacheMu.Lock()
	defer ctx.answerSurfaceCacheMu.Unlock()
	if !ctx.answerSurfacePlanCached || ctx.answerSurfaceCacheKey != key {
		return nil
	}
	return cloneAnswerSurfacePlan(ctx.answerSurfacePlan)
}

func (ctx *BusContext) storeAnswerSurfacePlan(plan *AnswerSurfacePlan) {
	if ctx == nil {
		return
	}
	key := answerPlanCacheKeyForBus(ctx)
	ctx.answerSurfaceCacheMu.Lock()
	defer ctx.answerSurfaceCacheMu.Unlock()
	ctx.resetAnswerPlanCacheForKeyLocked(key)
	ctx.answerSurfaceCacheKey = key
	ctx.answerSurfacePlan = cloneAnswerSurfacePlan(plan)
	ctx.answerSurfacePlanCached = plan != nil
}

func (ctx *BusContext) cachedAnswerSemanticView() *AnswerSemanticView {
	if ctx == nil {
		return nil
	}
	key := answerPlanCacheKeyForBus(ctx)
	ctx.answerSurfaceCacheMu.Lock()
	defer ctx.answerSurfaceCacheMu.Unlock()
	if !ctx.answerSemanticViewCached || ctx.answerSurfaceCacheKey != key {
		return nil
	}
	return cloneAnswerSemanticView(ctx.answerSemanticView)
}

func (ctx *BusContext) storeAnswerSemanticView(view *AnswerSemanticView) {
	if ctx == nil {
		return
	}
	key := answerPlanCacheKeyForBus(ctx)
	ctx.answerSurfaceCacheMu.Lock()
	defer ctx.answerSurfaceCacheMu.Unlock()
	ctx.resetAnswerPlanCacheForKeyLocked(key)
	ctx.answerSurfaceCacheKey = key
	ctx.answerSemanticView = cloneAnswerSemanticView(view)
	ctx.answerSemanticViewCached = view != nil
}

func (ctx *BusContext) cachedAnswerSupportPlan() *AnswerSupportPlan {
	if ctx == nil {
		return nil
	}
	key := answerPlanCacheKeyForBus(ctx)
	ctx.answerSurfaceCacheMu.Lock()
	defer ctx.answerSurfaceCacheMu.Unlock()
	if !ctx.answerSupportPlanCached || ctx.answerSurfaceCacheKey != key {
		return nil
	}
	return cloneAnswerSupportPlan(ctx.answerSupportPlan)
}

func (ctx *BusContext) storeAnswerSupportPlan(plan *AnswerSupportPlan) {
	if ctx == nil {
		return
	}
	key := answerPlanCacheKeyForBus(ctx)
	ctx.answerSurfaceCacheMu.Lock()
	defer ctx.answerSurfaceCacheMu.Unlock()
	ctx.resetAnswerPlanCacheForKeyLocked(key)
	ctx.answerSurfaceCacheKey = key
	ctx.answerSupportPlan = cloneAnswerSupportPlan(plan)
	ctx.answerSupportPlanCached = plan != nil
}

// InvalidateAnswerPlanCache drops BusContext-level compiled answer
// projections. The orchestrator calls this when truth-set slices are
// replaced or enlarged; MutableState's answer-surface revision handles
// tool-time mutations that do not pass through applyStageOutput.
func (ctx *BusContext) InvalidateAnswerPlanCache() {
	if ctx == nil {
		return
	}
	ctx.answerSurfaceCacheMu.Lock()
	defer ctx.answerSurfaceCacheMu.Unlock()
	ctx.answerSurfacePlanCached = false
	ctx.answerSemanticViewCached = false
	ctx.answerSupportPlanCached = false
	ctx.answerSurfacePlan = nil
	ctx.answerSemanticView = nil
	ctx.answerSupportPlan = nil
}

func (ctx *BusContext) resetAnswerPlanCacheForKeyLocked(key answerPlanCacheKey) {
	if ctx.answerSurfaceCacheKey == key {
		return
	}
	ctx.answerSurfacePlanCached = false
	ctx.answerSemanticViewCached = false
	ctx.answerSupportPlanCached = false
	ctx.answerSurfacePlan = nil
	ctx.answerSemanticView = nil
	ctx.answerSupportPlan = nil
}

func cloneAnswerSurfacePlan(in *AnswerSurfacePlan) *AnswerSurfacePlan {
	if in == nil {
		return nil
	}
	out := *in
	out.StepBackbone = append([]StepSurfaceAnchor(nil), in.StepBackbone...)
	out.ExplanationAnchorBackbone = append([]StepSurfaceAnchor(nil), in.ExplanationAnchorBackbone...)
	out.ExplanationAnchorMissingTopics = append([]string(nil), in.ExplanationAnchorMissingTopics...)
	out.ExternalObservationSeeds = append([]ExternalObservationSeed(nil), in.ExternalObservationSeeds...)
	out.LogSourceDriftAnchors = append([]LogSourceDriftAnchor(nil), in.LogSourceDriftAnchors...)
	out.LogObservedAnchors = append([]LogSourceDriftAnchor(nil), in.LogObservedAnchors...)
	out.DriftBoundedSurfaceItems = append([]EvidenceItem(nil), in.DriftBoundedSurfaceItems...)
	out.StableAggregateFacts = cloneAnswerAggregateFacts(in.StableAggregateFacts)
	out.ExactContextRequiredFiles = append([]string(nil), in.ExactContextRequiredFiles...)
	out.CapabilityAuthorityFiles = append([]string(nil), in.CapabilityAuthorityFiles...)
	out.SurfaceEvidence = append([]EvidenceItem(nil), in.SurfaceEvidence...)
	out.AllowedExactContextItems = append([]EvidenceItem(nil), in.AllowedExactContextItems...)
	out.CitationGradeExactContextItems = append([]EvidenceItem(nil), in.CitationGradeExactContextItems...)
	out.ProseOnlyExactContextItems = append([]EvidenceItem(nil), in.ProseOnlyExactContextItems...)
	out.DiagramGradeExactContextItems = append([]EvidenceItem(nil), in.DiagramGradeExactContextItems...)
	out.ForbiddenExactContextItems = append([]EvidenceItem(nil), in.ForbiddenExactContextItems...)
	out.AllowedExactContextLabels = append([]ExactContextSurfaceLabel(nil), in.AllowedExactContextLabels...)
	out.CitationGradeExactContextLabels = append([]ExactContextSurfaceLabel(nil), in.CitationGradeExactContextLabels...)
	out.ForbiddenExactContextLabels = append([]ExactContextSurfaceLabel(nil), in.ForbiddenExactContextLabels...)
	out.RelatedContextCitationCandidates = append([]ConfigTraceRelatedContextCitationCandidate(nil), in.RelatedContextCitationCandidates...)
	out.ConfigTraceDiagramAnchors = append([]ConfigTraceDiagramAnchor(nil), in.ConfigTraceDiagramAnchors...)
	out.FacetCoverage = cloneFacetCoverageContract(in.FacetCoverage)
	if in.Diagram != nil {
		diagram := *in.Diagram
		diagram.PreferredKinds = append([]DiagramKind(nil), in.Diagram.PreferredKinds...)
		diagram.Reasons = append([]string(nil), in.Diagram.Reasons...)
		out.Diagram = &diagram
	}
	if in.ExactResolution != nil {
		exact := *in.ExactResolution
		exact.Targets = append([]string(nil), in.ExactResolution.Targets...)
		exact.RelatedContextTerms = append([]string(nil), in.ExactResolution.RelatedContextTerms...)
		exact.RequestedContextRoles = append([]EvidenceDiagramRole(nil), in.ExactResolution.RequestedContextRoles...)
		out.ExactResolution = &exact
	}
	if in.PreferredExactResolution != nil {
		exact := *in.PreferredExactResolution
		out.PreferredExactResolution = &exact
	}
	if in.CapabilitySurface != nil {
		capability := *in.CapabilitySurface
		capability.AuthorityFiles = append([]string(nil), in.CapabilitySurface.AuthorityFiles...)
		out.CapabilitySurface = &capability
	}
	if in.ChangeImpactProfile != nil {
		profile := *in.ChangeImpactProfile
		profile.AffectedSiteKinds = append([]ImpactAffectedSiteKind(nil), in.ChangeImpactProfile.AffectedSiteKinds...)
		out.ChangeImpactProfile = &profile
	}
	if in.RuntimeGroundingDisposition != nil {
		disposition := *in.RuntimeGroundingDisposition
		out.RuntimeGroundingDisposition = &disposition
	}
	if in.RequestedEnumerationBoundary != nil {
		boundary := *in.RequestedEnumerationBoundary
		out.RequestedEnumerationBoundary = &boundary
	}
	return &out
}

func cloneAnswerSemanticView(in *AnswerSemanticView) *AnswerSemanticView {
	if in == nil {
		return nil
	}
	out := *in
	out.FacetCoverage = cloneFacetCoverageContract(in.FacetCoverage)
	out.RequiredBlocks = cloneBlockRequirements(in.RequiredBlocks)
	out.OptionalBlocks = cloneBlockRequirements(in.OptionalBlocks)
	out.RequiredCandidateRoles = append([]AnswerCandidateRole(nil), in.RequiredCandidateRoles...)
	out.RequiredMechanismAnchors = append([]AnswerRequiredAnchor(nil), in.RequiredMechanismAnchors...)
	out.MissingRequestedRoles = append([]AnswerMissingRequestedRole(nil), in.MissingRequestedRoles...)
	out.UncertaintyRules = append([]UncertaintyRule(nil), in.UncertaintyRules...)
	out.RichnessCandidates = append([]RichnessCandidate(nil), in.RichnessCandidates...)
	if in.DiagramPlan != nil {
		diagram := *in.DiagramPlan
		diagram.NodeFacets = append([]string(nil), in.DiagramPlan.NodeFacets...)
		diagram.EdgeFacets = append([]string(nil), in.DiagramPlan.EdgeFacets...)
		diagram.EdgeRelations = append([]DiagramEdgeRelationContract(nil), in.DiagramPlan.EdgeRelations...)
		out.DiagramPlan = &diagram
	}
	if in.ExactResolution != nil {
		exact := *in.ExactResolution
		exact.Targets = append([]string(nil), in.ExactResolution.Targets...)
		exact.RelatedContextTerms = append([]string(nil), in.ExactResolution.RelatedContextTerms...)
		exact.RequestedContextRoles = append([]EvidenceDiagramRole(nil), in.ExactResolution.RequestedContextRoles...)
		out.ExactResolution = &exact
	}
	if in.CurrentStatusDiagnostic != nil {
		current := *in.CurrentStatusDiagnostic
		current.AllowedVerdicts = append([]CurrentStatusVerdict(nil), in.CurrentStatusDiagnostic.AllowedVerdicts...)
		out.CurrentStatusDiagnostic = &current
	}
	if in.ErrorGranularityProfile != nil {
		profile := *in.ErrorGranularityProfile
		profile.RequestedVerdictOptions = append([]ErrorGranularityVerdict(nil), in.ErrorGranularityProfile.RequestedVerdictOptions...)
		profile.SourceQuotes = append([]string(nil), in.ErrorGranularityProfile.SourceQuotes...)
		out.ErrorGranularityProfile = &profile
	}
	return &out
}

func cloneBlockRequirements(in []BlockRequirement) []BlockRequirement {
	if len(in) == 0 {
		return nil
	}
	out := append([]BlockRequirement(nil), in...)
	for i := range out {
		out[i].FacetIDs = append([]string(nil), in[i].FacetIDs...)
		out[i].AcceptableClaimForms = append([]ClaimForm(nil), in[i].AcceptableClaimForms...)
	}
	return out
}

func cloneFacetCoverageContract(in *FacetCoverageContract) *FacetCoverageContract {
	if in == nil {
		return nil
	}
	out := *in
	out.Required = cloneFacetRequirements(in.Required)
	out.Optional = cloneFacetRequirements(in.Optional)
	return &out
}

func cloneFacetRequirements(in []FacetRequirement) []FacetRequirement {
	if len(in) == 0 {
		return nil
	}
	out := append([]FacetRequirement(nil), in...)
	for i := range out {
		out[i].AcceptableForms = append([]ClaimForm(nil), in[i].AcceptableForms...)
		out[i].SourceCandidate = append([]string(nil), in[i].SourceCandidate...)
	}
	return out
}

func cloneAnswerSupportPlan(in *AnswerSupportPlan) *AnswerSupportPlan {
	if in == nil {
		return nil
	}
	out := *in
	out.Lanes = append([]AnswerSupportLane(nil), in.Lanes...)
	for i := range out.Lanes {
		out.Lanes[i].AllowedBlocks = append([]string(nil), in.Lanes[i].AllowedBlocks...)
		out.Lanes[i].Entries = append([]AnswerSupportEntry(nil), in.Lanes[i].Entries...)
		for j := range out.Lanes[i].Entries {
			out.Lanes[i].Entries[j].EquivalentLocations = append([]string(nil), in.Lanes[i].Entries[j].EquivalentLocations...)
			out.Lanes[i].Entries[j].SurfaceTerms = append([]string(nil), in.Lanes[i].Entries[j].SurfaceTerms...)
		}
	}
	if in.ChangeImpactProfile != nil {
		profile := *in.ChangeImpactProfile
		profile.AffectedSiteKinds = append([]ImpactAffectedSiteKind(nil), in.ChangeImpactProfile.AffectedSiteKinds...)
		out.ChangeImpactProfile = &profile
	}
	return &out
}

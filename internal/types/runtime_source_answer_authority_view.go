package types

// RuntimeSourceAnswerAuthoritySnapshot is the answer-facing authority view for
// mixed runtime/log/trace artifact plus current-source questions. It composes
// existing typed carriers; it must not inspect user prose, model rationale, or
// rendered answer text.
type RuntimeSourceAnswerAuthoritySnapshot struct {
	Active                         bool                                 `json:"active,omitempty"`
	CurrentSourceLane              CurrentSourceLaneDecision            `json:"current_source_lane,omitempty"`
	CurrentSourceRequirement       RuntimeSourceRequirementPrecision    `json:"current_source_requirement,omitempty"`
	ExternalObservationSufficiency ExternalObservationSufficiencyStatus `json:"external_observation_sufficiency,omitempty"`
	RuntimeObservationCount        int                                  `json:"runtime_observation_count,omitempty"`
	AddressableRuntimeCount        int                                  `json:"addressable_runtime_count,omitempty"`
	DeterministicRuntimeQueryCount int                                  `json:"deterministic_runtime_query_count,omitempty"`
	CurrentSourceRecordCount       int                                  `json:"current_source_record_count,omitempty"`
	ExactCurrentSourceSupportCount int                                  `json:"exact_current_source_support_count,omitempty"`
	CurrentSourceRequired          bool                                 `json:"current_source_required,omitempty"`
	CurrentSourceSatisfied         bool                                 `json:"current_source_satisfied,omitempty"`
	RuntimeOnlySufficient          bool                                 `json:"runtime_only_sufficient,omitempty"`
	CanCompleteWithCombinedProof   bool                                 `json:"can_complete_with_combined_proof,omitempty"`
	CanUseRuntimeOnlyWithCaveat    bool                                 `json:"can_use_runtime_only_with_caveat,omitempty"`
	NeedsCurrentSourceEvidence     bool                                 `json:"needs_current_source_evidence,omitempty"`
	CanHardBlockCompletion         bool                                 `json:"can_hard_block_completion,omitempty"`
	CanDowngradeToCaveat           bool                                 `json:"can_downgrade_to_caveat,omitempty"`
	RuntimeCitationPolicy          RuntimeGroundingCitationPolicy       `json:"runtime_citation_policy,omitempty"`
	ReasonCodes                    []RuntimeSourceAuthorityReasonCode   `json:"reason_codes,omitempty"`
}

type RuntimeSourceRequirementPrecision string

const (
	RuntimeSourceRequirementNone    RuntimeSourceRequirementPrecision = ""
	RuntimeSourceRequirementSoft    RuntimeSourceRequirementPrecision = "soft"
	RuntimeSourceRequirementPrecise RuntimeSourceRequirementPrecision = "precise"
)

type RuntimeSourceAuthorityReasonCode string

const (
	RuntimeSourceAuthorityRuntimeObservationAvailable RuntimeSourceAuthorityReasonCode = "runtime_observation_available"
	RuntimeSourceAuthorityDeterministicRuntimeQuery   RuntimeSourceAuthorityReasonCode = "deterministic_runtime_query"
	RuntimeSourceAuthorityCurrentSourceRequired       RuntimeSourceAuthorityReasonCode = "current_source_required"
	RuntimeSourceAuthorityCurrentSourcePrecise        RuntimeSourceAuthorityReasonCode = "current_source_precise"
	RuntimeSourceAuthorityCurrentSourceSoft           RuntimeSourceAuthorityReasonCode = "current_source_soft"
	RuntimeSourceAuthorityCurrentSourceSatisfied      RuntimeSourceAuthorityReasonCode = "current_source_satisfied"
	RuntimeSourceAuthorityCurrentSourceMissing        RuntimeSourceAuthorityReasonCode = "current_source_missing"
	RuntimeSourceAuthorityCurrentSourceDowngradable   RuntimeSourceAuthorityReasonCode = "current_source_downgradable"
	RuntimeSourceAuthorityRuntimeOnlySufficient       RuntimeSourceAuthorityReasonCode = "runtime_only_sufficient"
	RuntimeSourceAuthorityRuntimeOnlyCaveat           RuntimeSourceAuthorityReasonCode = "runtime_only_caveat"
	RuntimeSourceAuthorityCombinedProofReady          RuntimeSourceAuthorityReasonCode = "combined_proof_ready"
)

type RuntimeSourceAnswerAuthorityInput struct {
	RequestModel      *RequestModel
	RouteHint         TurnRouteHint
	Ledger            ObservationLedger
	AnswerSurfacePlan *AnswerSurfacePlan
}

// BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext compiles the shared
// mixed runtime/current-source authority for prompt consumers. Callers may pass
// a precompiled ledger to avoid rebuilding a large prompt view; when omitted the
// helper uses the standard typed AgentContext ledger projection.
func BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext(ctx *AgentContext, ledger ObservationLedger) RuntimeSourceAnswerAuthoritySnapshot {
	if ctx == nil {
		return RuntimeSourceAnswerAuthoritySnapshot{}
	}
	if ledger.Empty() {
		ledger = CompileObservationLedger(ObservationLedgerInputFromAgentContext(ctx, 128))
	}
	return BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel:      RuntimeSourceAuthorityRequestModelFromAgentContext(ctx),
		RouteHint:         ctx.TurnRouteHint,
		Ledger:            ledger,
		AnswerSurfacePlan: BuildAnswerSurfacePlanForAgentContext(ctx),
	})
}

// BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext is the tool/reviewer
// companion to BuildRuntimeSourceAnswerAuthoritySnapshotForAgentContext. It
// keeps completion gates, status views, and prompt handoff on the same typed
// authority surface instead of each consumer repacking runtime/source state.
func BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(ctx *BusContext, ledger ObservationLedger) RuntimeSourceAnswerAuthoritySnapshot {
	if ctx == nil {
		return RuntimeSourceAnswerAuthoritySnapshot{}
	}
	if ledger.Empty() {
		ledger = CompileObservationLedger(ObservationLedgerInputFromBusContext(ctx, 128))
	}
	return BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel:      RuntimeSourceAuthorityRequestModelFromBusContext(ctx),
		RouteHint:         ctx.TurnRouteHint,
		Ledger:            ledger,
		AnswerSurfacePlan: BuildAnswerSurfacePlanForBusContext(ctx),
	})
}

// RuntimeSourceAuthorityRequestModelFromAgentContext returns a request-model
// clone enriched with already-validated runtime bundles from the narrowed agent
// view. The returned pointer is detached from AnalysisIR so consumers cannot
// mutate analyzer output while filling missing LogTriage/PerfTrace carriers.
func RuntimeSourceAuthorityRequestModelFromAgentContext(ctx *AgentContext) *RequestModel {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.LogTriage == nil {
		if ctx.LogTriage != nil {
			rm.LogTriage = ctx.LogTriage
		} else if ctx.Mutable != nil {
			rm.LogTriage = ctx.Mutable.LogTriage()
		}
	}
	if rm.PerfTrace == nil {
		if ctx.PerfTrace != nil {
			rm.PerfTrace = ctx.PerfTrace
		} else if ctx.Mutable != nil {
			rm.PerfTrace = ctx.Mutable.PerfTrace()
		}
	}
	return &rm
}

// RuntimeSourceAuthorityRequestModelFromBusContext is the BusContext equivalent
// of RuntimeSourceAuthorityRequestModelFromAgentContext.
func RuntimeSourceAuthorityRequestModelFromBusContext(ctx *BusContext) *RequestModel {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	rm := ctx.AnalysisIR.RequestModel
	if ctx.Mutable != nil {
		if rm.LogTriage == nil {
			rm.LogTriage = ctx.Mutable.LogTriage()
		}
		if rm.PerfTrace == nil {
			rm.PerfTrace = ctx.Mutable.PerfTrace()
		}
	}
	return &rm
}

func BuildRuntimeSourceAnswerAuthoritySnapshot(in RuntimeSourceAnswerAuthorityInput) RuntimeSourceAnswerAuthoritySnapshot {
	out := RuntimeSourceAnswerAuthoritySnapshot{}
	rm := in.RequestModel
	if rm != nil {
		out.CurrentSourceLane = rm.CurrentSourceLaneDecision()
	} else if RouteBackedExternalObservationRequiresCurrentSource(nil, in.RouteHint) {
		out.CurrentSourceLane = CurrentSourceLaneRequired
	} else {
		out.CurrentSourceLane = CurrentSourceLaneAllowedOptional
	}
	suff := AssessExternalObservationSufficiency(in.Ledger.Records, rm, in.RouteHint)
	out.ExternalObservationSufficiency = suff.Status
	out.RuntimeOnlySufficient = suff.Status.Sufficient()
	out.CurrentSourceRequired = runtimeSourceAuthorityCurrentSourceRequired(rm, in.RouteHint, suff)
	out.CurrentSourceRequirement = runtimeSourceAuthorityRequirementPrecision(rm, in.RouteHint, out.CurrentSourceRequired)
	out.RuntimeCitationPolicy = runtimeSourceAuthorityCitationPolicy(in.AnswerSurfacePlan)

	for _, record := range in.Ledger.Records {
		switch {
		case runtimeSourceAuthorityRuntimeRecord(record):
			out.RuntimeObservationCount++
			if runtimeSourceAuthorityAddressableRuntimeRecord(record) {
				out.AddressableRuntimeCount++
			}
			if RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
				out.DeterministicRuntimeQueryCount++
			}
		case runtimeSourceAuthorityCurrentSourceRecord(record):
			out.CurrentSourceRecordCount++
			if runtimeSourceAuthorityExactCurrentSourceSupport(record) {
				out.ExactCurrentSourceSupportCount++
			}
		}
	}

	out.Active = out.RuntimeObservationCount > 0 ||
		out.CurrentSourceRecordCount > 0 ||
		out.CurrentSourceRequired ||
		out.RuntimeOnlySufficient
	out.CurrentSourceSatisfied = out.CurrentSourceRecordCount > 0 || out.ExactCurrentSourceSupportCount > 0
	out.NeedsCurrentSourceEvidence = out.CurrentSourceRequired && !out.CurrentSourceSatisfied
	out.CanHardBlockCompletion = out.NeedsCurrentSourceEvidence &&
		out.CurrentSourceRequirement == RuntimeSourceRequirementPrecise
	externalObservationAvailable := out.RuntimeObservationCount > 0 || out.RuntimeOnlySufficient
	out.CanDowngradeToCaveat = out.NeedsCurrentSourceEvidence &&
		out.CurrentSourceRequirement == RuntimeSourceRequirementSoft &&
		externalObservationAvailable
	out.CanUseRuntimeOnlyWithCaveat = externalObservationAvailable &&
		!out.CurrentSourceSatisfied &&
		(!out.CurrentSourceRequired || out.CanDowngradeToCaveat)
	out.CanCompleteWithCombinedProof = out.RuntimeObservationCount > 0 &&
		(!out.CurrentSourceRequired || out.CurrentSourceSatisfied)
	out.ReasonCodes = runtimeSourceAuthorityReasonCodes(out)
	return out
}

// HasRuntimeCarrier reports whether the authority has answer-grade runtime or
// external observation support. It deliberately consumes only typed authority
// fields; consumers should not re-derive this from prompt prose or rendered
// trace/log text.
func (s RuntimeSourceAnswerAuthoritySnapshot) HasRuntimeCarrier() bool {
	return s.RuntimeObservationCount > 0 ||
		s.DeterministicRuntimeQueryCount > 0 ||
		s.RuntimeOnlySufficient ||
		s.CanUseRuntimeOnlyWithCaveat
}

// HasCurrentSourceCarrier reports whether a current-source proof lane is
// present, required, or already satisfied. Soft obligations count as carriers
// for handoff/navigation, but only CanHardBlockCompletion may drive hard gates.
func (s RuntimeSourceAnswerAuthoritySnapshot) HasCurrentSourceCarrier() bool {
	return s.CurrentSourceRequired ||
		s.CurrentSourceSatisfied ||
		s.CurrentSourceRecordCount > 0 ||
		s.ExactCurrentSourceSupportCount > 0 ||
		s.CurrentSourceRequirement != RuntimeSourceRequirementNone
}

// HasMixedRuntimeCurrentSourceCarrier is the shared typed predicate for mixed
// trace/log/runtime plus current-source turns. It is a handoff/readiness helper,
// not a hard completion gate.
func (s RuntimeSourceAnswerAuthoritySnapshot) HasMixedRuntimeCurrentSourceCarrier() bool {
	return s.Active && s.HasRuntimeCarrier() && s.HasCurrentSourceCarrier()
}

// RuntimeSourceAuthorityRequestCarrierActiveForAgentContext reports whether an
// agent prompt/budget consumer should treat the current turn as an intentional
// runtime/source mixed surface. This is prompt and handoff budgeting guidance,
// not a completion hard gate.
func RuntimeSourceAuthorityRequestCarrierActiveForAgentContext(ctx *AgentContext, authority RuntimeSourceAnswerAuthoritySnapshot) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	return RuntimeSourceAuthorityRequestCarrierActive(
		ctx.TurnRouteHint,
		RuntimeSourceAuthorityRequestModelFromAgentContext(ctx),
		authority,
	)
}

// RuntimeSourceAuthorityRequestCarrierActive centralizes the request-side
// carrier predicate used by finalizer/context prompt-budget consumers. It keeps
// local consumers from re-deriving the boundary from individual profile fields.
func RuntimeSourceAuthorityRequestCarrierActive(hint TurnRouteHint, rm *RequestModel, authority RuntimeSourceAnswerAuthoritySnapshot) bool {
	if rm == nil || !authority.HasRuntimeCarrier() {
		return false
	}
	if hint.ExternalObservationParticipates() ||
		rm.HasExternalOnlyRuntimeArtifact() ||
		rm.HasExternalObservationArtifactReference() ||
		rm.HasRuntimeArtifactPathReference() ||
		rm.LogTriage != nil ||
		rm.PerfTrace != nil {
		return true
	}
	return rm.HasCurrentSourceObligationSignal() ||
		rm.HasTypedCurrentSourceScopeRequest() ||
		rm.CurrentSourceExplanationProfile != nil &&
			rm.CurrentSourceExplanationProfile.Active()
}

func runtimeSourceAuthorityCurrentSourceRequired(rm *RequestModel, hint TurnRouteHint, suff ExternalObservationSufficiency) bool {
	if suff.Status == ExternalObservationSufficiencyBlockedByCurrentSource {
		return true
	}
	if RouteBackedExternalObservationRequiresCurrentSource(rm, hint) {
		return true
	}
	if rm == nil {
		return false
	}
	return rm.CurrentSourceLaneDecision().RequiresCurrentSource()
}

func runtimeSourceAuthorityRequirementPrecision(rm *RequestModel, hint TurnRouteHint, required bool) RuntimeSourceRequirementPrecision {
	if !required {
		return RuntimeSourceRequirementNone
	}
	if runtimeSourceAuthorityPreciseCurrentSourceRequirement(rm) {
		return RuntimeSourceRequirementPrecise
	}
	if RouteBackedExternalObservationRequiresCurrentSource(rm, hint) || rm != nil {
		return RuntimeSourceRequirementSoft
	}
	return RuntimeSourceRequirementSoft
}

func runtimeSourceAuthorityPreciseCurrentSourceRequirement(rm *RequestModel) bool {
	if rm == nil {
		return false
	}
	if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return false
	}
	if rm.currentSourceExplanationHasPreciseCurrentSourceQuote() {
		return true
	}
	if rm.HasTypedCurrentSourceScopeRequest() {
		return true
	}
	if rm.LogTriage != nil && len(rm.LogTriage.ResolvedFiles) > 0 {
		return true
	}
	if rm.PerfTrace != nil && len(rm.PerfTrace.ResolvedFiles) > 0 {
		return true
	}
	if rm.RequestedAnswerDimensions != nil && rm.RequestedAnswerDimensions.Active() {
		for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
			if !dim.Required {
				continue
			}
			switch dim.Role {
			case RequestedAnswerDimensionCurrentKeyCode:
				if rm.dimensionHasPreciseCurrentSourceAnchor(dim) {
					return true
				}
			case RequestedAnswerDimensionFunctionOrPurpose:
				if rm.dimensionHasPreciseCurrentSourceAnchor(dim) {
					return true
				}
			}
		}
	}
	for _, target := range rm.AnalyzerHints.ExactTargets {
		if targetLooksLikeCurrentSourceAnchor(target) {
			return true
		}
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if targetLooksLikeCurrentSourceAnchor(hint.Path) {
			return true
		}
	}
	return false
}

func runtimeSourceAuthorityCitationPolicy(plan *AnswerSurfacePlan) RuntimeGroundingCitationPolicy {
	if plan == nil || plan.RuntimeGroundingDisposition == nil {
		return ""
	}
	return plan.RuntimeGroundingDisposition.CitationPolicy
}

func runtimeSourceAuthorityRuntimeRecord(record ObservationRecord) bool {
	return record.Origin == AnswerEvidenceOriginRuntimeArtifact &&
		record.SourceRef.Kind == ObservationSourceRuntimeArtifact
}

func runtimeSourceAuthorityAddressableRuntimeRecord(record ObservationRecord) bool {
	if !runtimeSourceAuthorityRuntimeRecord(record) {
		return false
	}
	if record.SourceRef.PayloadRef != "" ||
		record.SourceRef.RowSetRef != "" ||
		record.SourceRef.RawRef != "" ||
		record.SourceRef.ArtifactID != "" {
		return true
	}
	return record.Span.LineStart > 0 ||
		record.Span.StartTs > 0 ||
		record.Span.StartTsMs > 0 ||
		record.Span.Selector != "" ||
		record.Span.JSONPointer != "" ||
		record.Span.Row > 0
}

func runtimeSourceAuthorityCurrentSourceRecord(record ObservationRecord) bool {
	if RuntimeArtifactPathKind(record.SourceRef.Path) != "" {
		return false
	}
	return record.Origin == AnswerEvidenceOriginCurrentSource ||
		record.SourceRef.Kind == ObservationSourceCurrentSource
}

func runtimeSourceAuthorityExactCurrentSourceSupport(record ObservationRecord) bool {
	if !runtimeSourceAuthorityCurrentSourceRecord(record) {
		return false
	}
	if record.SourceRef.Path == "" {
		return false
	}
	if record.Span.LineStart > 0 {
		return true
	}
	for _, ref := range record.SupportRefs {
		if answerSupportRefHasSourceLine(ref) {
			return true
		}
	}
	return false
}

func runtimeSourceAuthorityReasonCodes(s RuntimeSourceAnswerAuthoritySnapshot) []RuntimeSourceAuthorityReasonCode {
	var out []RuntimeSourceAuthorityReasonCode
	add := func(code RuntimeSourceAuthorityReasonCode) {
		if code == "" {
			return
		}
		for _, existing := range out {
			if existing == code {
				return
			}
		}
		out = append(out, code)
	}
	if s.RuntimeObservationCount > 0 {
		add(RuntimeSourceAuthorityRuntimeObservationAvailable)
	}
	if s.DeterministicRuntimeQueryCount > 0 {
		add(RuntimeSourceAuthorityDeterministicRuntimeQuery)
	}
	if s.CurrentSourceRequired {
		add(RuntimeSourceAuthorityCurrentSourceRequired)
	}
	switch s.CurrentSourceRequirement {
	case RuntimeSourceRequirementPrecise:
		add(RuntimeSourceAuthorityCurrentSourcePrecise)
	case RuntimeSourceRequirementSoft:
		add(RuntimeSourceAuthorityCurrentSourceSoft)
	}
	if s.CurrentSourceSatisfied {
		add(RuntimeSourceAuthorityCurrentSourceSatisfied)
	}
	if s.NeedsCurrentSourceEvidence {
		add(RuntimeSourceAuthorityCurrentSourceMissing)
	}
	if s.CanDowngradeToCaveat {
		add(RuntimeSourceAuthorityCurrentSourceDowngradable)
	}
	if s.RuntimeOnlySufficient {
		add(RuntimeSourceAuthorityRuntimeOnlySufficient)
	}
	if s.CanUseRuntimeOnlyWithCaveat {
		add(RuntimeSourceAuthorityRuntimeOnlyCaveat)
	}
	if s.CanCompleteWithCombinedProof {
		add(RuntimeSourceAuthorityCombinedProofReady)
	}
	return out
}

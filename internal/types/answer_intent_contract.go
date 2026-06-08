package types

import (
	"sort"
	"strings"
)

// AnswerEvidenceOrigin names the typed provenance lane a downstream claim may
// rely on. It is deliberately orthogonal to the visible answer shape: the same
// VCS fact can support a scalar commit lookup, a narrative feature summary, a
// comparison, or a diagram-backed explanation.
type AnswerEvidenceOrigin string

const (
	AnswerEvidenceOriginUnknown            AnswerEvidenceOrigin = ""
	AnswerEvidenceOriginCurrentSource      AnswerEvidenceOrigin = "current_source"
	AnswerEvidenceOriginVCSMetadata        AnswerEvidenceOrigin = "vcs_metadata"
	AnswerEvidenceOriginVCSDiff            AnswerEvidenceOrigin = "vcs_diff"
	AnswerEvidenceOriginRuntimeArtifact    AnswerEvidenceOrigin = "runtime_artifact"
	AnswerEvidenceOriginCommandMeasurement AnswerEvidenceOrigin = "command_measurement"
	AnswerEvidenceOriginRepoNegativeSearch AnswerEvidenceOrigin = "repo_negative_search"
	AnswerEvidenceOriginCrossRepoIndex     AnswerEvidenceOrigin = "cross_repo_index"
	AnswerEvidenceOriginExternalDocument   AnswerEvidenceOrigin = "external_document"
	AnswerEvidenceOriginWebPage            AnswerEvidenceOrigin = "web_page"
	AnswerEvidenceOriginMCPResource        AnswerEvidenceOrigin = "mcp_resource"
	AnswerEvidenceOriginConnectorResource  AnswerEvidenceOrigin = "connector_resource"
	AnswerEvidenceOriginSystemInference    AnswerEvidenceOrigin = "system_inference"
)

// AllAnswerEvidenceOrigins returns the canonical non-empty origin list.
func AllAnswerEvidenceOrigins() []AnswerEvidenceOrigin {
	return []AnswerEvidenceOrigin{
		AnswerEvidenceOriginCurrentSource,
		AnswerEvidenceOriginVCSMetadata,
		AnswerEvidenceOriginVCSDiff,
		AnswerEvidenceOriginRuntimeArtifact,
		AnswerEvidenceOriginCommandMeasurement,
		AnswerEvidenceOriginRepoNegativeSearch,
		AnswerEvidenceOriginCrossRepoIndex,
		AnswerEvidenceOriginExternalDocument,
		AnswerEvidenceOriginWebPage,
		AnswerEvidenceOriginMCPResource,
		AnswerEvidenceOriginConnectorResource,
		AnswerEvidenceOriginSystemInference,
	}
}

func (o AnswerEvidenceOrigin) IsValid() bool {
	if o == AnswerEvidenceOriginUnknown {
		return true
	}
	for _, declared := range AllAnswerEvidenceOrigins() {
		if o == declared {
			return true
		}
	}
	return false
}

// AnswerRequestedOutput names the visible answer surface the user requested.
// The analyzer may emit several outputs for a mixed request, for example VCS
// history + current-source mechanism + diagram. Origins must not be treated as
// requested outputs.
type AnswerRequestedOutput string

const (
	AnswerRequestedOutputUnknown      AnswerRequestedOutput = ""
	AnswerRequestedOutputSummary      AnswerRequestedOutput = "summary"
	AnswerRequestedOutputScalar       AnswerRequestedOutput = "scalar"
	AnswerRequestedOutputKeyValue     AnswerRequestedOutput = "key_value"
	AnswerRequestedOutputCount        AnswerRequestedOutput = "count"
	AnswerRequestedOutputEnumeration  AnswerRequestedOutput = "enumeration"
	AnswerRequestedOutputComparison   AnswerRequestedOutput = "comparison"
	AnswerRequestedOutputMechanism    AnswerRequestedOutput = "mechanism"
	AnswerRequestedOutputTrace        AnswerRequestedOutput = "trace"
	AnswerRequestedOutputDiagram      AnswerRequestedOutput = "diagram"
	AnswerRequestedOutputDiagnostic   AnswerRequestedOutput = "diagnostic"
	AnswerRequestedOutputChangeImpact AnswerRequestedOutput = "change_impact"
	AnswerRequestedOutputAbsence      AnswerRequestedOutput = "absence"
)

func AllAnswerRequestedOutputs() []AnswerRequestedOutput {
	return []AnswerRequestedOutput{
		AnswerRequestedOutputSummary,
		AnswerRequestedOutputScalar,
		AnswerRequestedOutputKeyValue,
		AnswerRequestedOutputCount,
		AnswerRequestedOutputEnumeration,
		AnswerRequestedOutputComparison,
		AnswerRequestedOutputMechanism,
		AnswerRequestedOutputTrace,
		AnswerRequestedOutputDiagram,
		AnswerRequestedOutputDiagnostic,
		AnswerRequestedOutputChangeImpact,
		AnswerRequestedOutputAbsence,
	}
}

func (o AnswerRequestedOutput) IsValid() bool {
	if o == AnswerRequestedOutputUnknown {
		return true
	}
	for _, declared := range AllAnswerRequestedOutputs() {
		if o == declared {
			return true
		}
	}
	return false
}

// ClaimGroundingPolicy is the downstream action level for a claim/origin pair.
// Batch A only defines the enum; later batches will compile per-claim policies
// from AnswerIntentContract + support refs.
type ClaimGroundingPolicy string

const (
	ClaimGroundingUnknown     ClaimGroundingPolicy = ""
	ClaimGroundingHard        ClaimGroundingPolicy = "hard"
	ClaimGroundingRepairable  ClaimGroundingPolicy = "repairable"
	ClaimGroundingSoft        ClaimGroundingPolicy = "soft"
	ClaimGroundingDisplayOnly ClaimGroundingPolicy = "display_only"
)

func (p ClaimGroundingPolicy) IsValid() bool {
	switch p {
	case ClaimGroundingUnknown,
		ClaimGroundingHard,
		ClaimGroundingRepairable,
		ClaimGroundingSoft,
		ClaimGroundingDisplayOnly:
		return true
	default:
		return false
	}
}

// AnswerIntentContract is the request-level projection that keeps evidence
// provenance separate from visible answer shape. It is compiled from existing
// typed RequestModel / AnswerContract fields and intentionally does not inspect
// raw user prose or model free text.
type AnswerIntentContract struct {
	Origins          []AnswerEvidenceOrigin
	RequestedOutputs []AnswerRequestedOutput
}

func (c AnswerIntentContract) HasOrigin(origin AnswerEvidenceOrigin) bool {
	for _, existing := range c.Origins {
		if existing == origin {
			return true
		}
	}
	return false
}

func (c AnswerIntentContract) HasOutput(output AnswerRequestedOutput) bool {
	for _, existing := range c.RequestedOutputs {
		if existing == output {
			return true
		}
	}
	return false
}

// CompileAnswerIntentContract builds the first unified evidence/output
// contract. It is intentionally conservative and side-effect free; Batch A
// callers use it for tests/telemetry only, so existing routing remains intact.
func CompileAnswerIntentContract(rm RequestModel, contract *AnswerContract) AnswerIntentContract {
	var origins []AnswerEvidenceOrigin
	var outputs []AnswerRequestedOutput
	addOrigin := func(origin AnswerEvidenceOrigin) {
		if origin == AnswerEvidenceOriginUnknown || !origin.IsValid() {
			return
		}
		for _, existing := range origins {
			if existing == origin {
				return
			}
		}
		origins = append(origins, origin)
	}
	addOutput := func(output AnswerRequestedOutput) {
		if output == AnswerRequestedOutputUnknown || !output.IsValid() {
			return
		}
		for _, existing := range outputs {
			if existing == output {
				return
			}
		}
		outputs = append(outputs, output)
	}

	if rm.Predicates.IsHistoryLookup {
		addOrigin(AnswerEvidenceOriginVCSMetadata)
		if historyRequestNeedsVCSDiffOrigin(rm, contract) {
			addOrigin(AnswerEvidenceOriginVCSDiff)
		}
	}
	if rm.HasExternalOnlyRuntimeArtifact() ||
		rm.LogTriage != nil ||
		rm.PerfTrace != nil ||
		rm.ArtifactObservationProfile != nil {
		addOrigin(AnswerEvidenceOriginRuntimeArtifact)
	}
	if requestModelHasMCPResourceReference(rm) {
		addOrigin(AnswerEvidenceOriginMCPResource)
	}
	if rm.Predicates.IsCountQuestion {
		addOrigin(AnswerEvidenceOriginCommandMeasurement)
	}
	if shouldIncludeCurrentSourceOrigin(rm, contract) {
		addOrigin(AnswerEvidenceOriginCurrentSource)
	}

	addOutput(AnswerRequestedOutputSummary)
	if rm.Predicates.IsCountQuestion {
		addOutput(AnswerRequestedOutputCount)
	}
	if requestModelShouldRequestScalarOutput(rm) {
		addOutput(AnswerRequestedOutputScalar)
	}
	if rm.FieldValueProfile != nil && rm.FieldValueProfile.Active() {
		addOutput(AnswerRequestedOutputKeyValue)
	}
	if shouldRequestEnumerationOutput(rm) {
		addOutput(AnswerRequestedOutputEnumeration)
	}
	if historyLookupHasMultipleNonScalarTargets(rm) {
		addOutput(AnswerRequestedOutputEnumeration)
	}
	if len(rm.QuestionStructure().Buckets) >= 2 ||
		(rm.Predicates.IsCrossComponent && !IsSingleTopicMechanismExplanation(rm)) {
		addOutput(AnswerRequestedOutputComparison)
	}
	if rm.Intent == IntentTrace {
		addOutput(AnswerRequestedOutputTrace)
	}
	if rm.Intent == IntentExplain && !rm.Predicates.IsScalarAnswer && !rm.Predicates.IsCountQuestion {
		addOutput(AnswerRequestedOutputMechanism)
	}
	if rm.Predicates.IsDiagnosticQuestion ||
		rm.DiagnosticProfile.RequiresDiagnosticRootCause() ||
		rm.DiagnosticProfile.RequiresCurrentStatusDiagnostic() ||
		rm.Intent == IntentRootCause ||
		rm.HasExternalOnlyRuntimeArtifact() {
		addOutput(AnswerRequestedOutputDiagnostic)
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		addOutput(AnswerRequestedOutputChangeImpact)
	}
	if contract != nil && contract.ExactResolution != nil && contract.ExactResolution.AllowAbsence {
		addOutput(AnswerRequestedOutputAbsence)
	}
	if rm.DiagramHint != nil && rm.DiagramHint.Kind != "" {
		addOutput(AnswerRequestedOutputDiagram)
	}
	if contract != nil && contract.Diagram != nil && contract.Diagram.Required {
		addOutput(AnswerRequestedOutputDiagram)
	}

	if len(origins) == 0 {
		addOrigin(AnswerEvidenceOriginCurrentSource)
	}
	sort.SliceStable(origins, func(i, j int) bool {
		return answerEvidenceOriginRank(origins[i]) < answerEvidenceOriginRank(origins[j])
	})
	sort.SliceStable(outputs, func(i, j int) bool {
		return answerRequestedOutputRank(outputs[i]) < answerRequestedOutputRank(outputs[j])
	})
	return AnswerIntentContract{Origins: origins, RequestedOutputs: outputs}
}

func requestModelShouldRequestScalarOutput(rm RequestModel) bool {
	if historyLookupHasMultipleNonScalarTargets(rm) {
		return false
	}
	return rm.Predicates.IsScalarAnswer || rm.Predicates.IsRoleLocateLookup || rm.Intent == IntentReturnValue
}

func historyLookupHasMultipleNonScalarTargets(rm RequestModel) bool {
	return rm.Predicates.IsHistoryLookup &&
		!rm.Predicates.IsCountQuestion &&
		!rm.Predicates.IsRoleLocateLookup &&
		HistoryLookupScalarTargetCount(rm) > 1
}

func historyRequestNeedsVCSDiffOrigin(rm RequestModel, contract *AnswerContract) bool {
	if !rm.Predicates.IsHistoryLookup {
		return false
	}
	if rm.Predicates.IsScalarAnswer || rm.Predicates.IsCountQuestion {
		return false
	}
	if IsHistoryBackedCurrentCodeExplanation(rm) {
		return true
	}
	if rm.CurrentSourceExplanationProfile != nil && rm.CurrentSourceExplanationProfile.Active() {
		return true
	}
	if rm.Predicates.IsCrossComponent || len(rm.QuestionStructure().Buckets) >= 2 {
		return true
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return true
	}
	if rm.DiagramHint != nil && rm.DiagramHint.Kind != "" {
		return true
	}
	return contract != nil && contract.Diagram != nil && contract.Diagram.Required
}

func shouldIncludeCurrentSourceOrigin(rm RequestModel, contract *AnswerContract) bool {
	if rm.ExternalObservationPolicy != nil &&
		rm.ExternalObservationPolicy.ExcludesCurrentSource() &&
		(rm.HasExternalOnlyRuntimeArtifact() || requestModelHasMCPResourceReference(rm)) {
		return false
	}
	if typesContractRequiresCurrentSource(contract) {
		return true
	}
	if rm.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		return false
	}
	if rm.CurrentSourceExplanationProfile != nil && rm.CurrentSourceExplanationProfile.Active() {
		return true
	}
	if rm.Predicates.IsHistoryLookup {
		return IsHistoryBackedCurrentCodeExplanation(rm) ||
			rm.Predicates.IsRelationalLookup ||
			rm.Intent == IntentTrace ||
			(rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active()) ||
			(rm.DiagramHint != nil && rm.DiagramHint.Kind != "")
	}
	return true
}

// RequiresCurrentSourceForExternalObservation reports whether a request that
// already has typed non-current-source observations must still block completion
// on current-checkout evidence. This is intentionally stricter than
// shouldIncludeCurrentSourceOrigin: source exploration is allowed by default,
// but hard gates should only require it when a typed source-oriented contract
// says the current checkout is part of the answer.
func (rm RequestModel) RequiresCurrentSourceForExternalObservation(contract *AnswerContract) bool {
	if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
		return false
	}
	if typesContractRequiresCurrentSource(contract) {
		return true
	}
	if rm.HasRuntimeArtifactCurrentVerificationAnchor() {
		return true
	}
	if rm.HasTypedCurrentSourceScopeRequest() {
		return true
	}
	if rm.CurrentSourceExplanationProfile != nil && rm.CurrentSourceExplanationProfile.Active() {
		return true
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return true
	}
	if rm.Predicates.IsHistoryLookup {
		return IsHistoryBackedCurrentCodeExplanation(rm) ||
			rm.Predicates.IsRelationalLookup ||
			rm.Intent == IntentTrace ||
			(rm.DiagramHint != nil && rm.DiagramHint.Kind != "")
	}
	return false
}

func requestModelHasMCPResourceReference(rm RequestModel) bool {
	if textHasMCPResourceURI(rm.RawRequest) {
		return true
	}
	for _, term := range rm.TermGraph.Canonical {
		if textHasMCPResourceURI(term.Surface) {
			return true
		}
	}
	for _, target := range rm.AnalyzerHints.ExactTargets {
		if textHasMCPResourceURI(target) {
			return true
		}
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if textHasMCPResourceURI(hint.Path) {
			return true
		}
	}
	return false
}

func textHasMCPResourceURI(raw string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(raw)), "mcp://")
}

func typesContractRequiresCurrentSource(contract *AnswerContract) bool {
	if contract == nil {
		return false
	}
	if exactResolutionContractHasTarget(contract.ExactResolution) {
		return true
	}
	if contract.CurrentStatusDiagnostic != nil && contract.CurrentStatusDiagnostic.Required {
		return true
	}
	return false
}

func exactResolutionContractHasTarget(contract *ExactResolutionContract) bool {
	if contract == nil {
		return false
	}
	return contract.TargetKind != "" ||
		contract.TargetLabel != "" ||
		len(contract.Targets) > 0 ||
		len(contract.RequestedContextRoles) > 0
}

func shouldRequestEnumerationOutput(rm RequestModel) bool {
	if RequiresExhaustiveEnumerationMemberSetHandoff(rm) ||
		RequiresRelationMemberSetHandoff(rm) {
		return true
	}
	if IsArchitectureNarrativeExplanation(rm) || IsSingleTopicMechanismExplanation(rm) {
		return false
	}
	if rm.Predicates.IsCategoryEnumeration || rm.Intent == IntentEnumerate {
		return true
	}
	return false
}

func answerEvidenceOriginRank(origin AnswerEvidenceOrigin) int {
	for i, declared := range AllAnswerEvidenceOrigins() {
		if origin == declared {
			return i
		}
	}
	return len(AllAnswerEvidenceOrigins())
}

func answerRequestedOutputRank(output AnswerRequestedOutput) int {
	for i, declared := range AllAnswerRequestedOutputs() {
		if output == declared {
			return i
		}
	}
	return len(AllAnswerRequestedOutputs())
}

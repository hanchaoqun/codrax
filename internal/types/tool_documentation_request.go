package types

import (
	"errors"
	"fmt"
)

// ToolDocumentationRequest is an optional, orthogonal answer domain, not a
// tool-call receipt or permission to treat documentation as observed evidence.
// Nil preserves the complete pre-existing source/runtime contract.
type ToolDocumentationRequest struct {
	Scope            ToolDocumentationRequestScope `json:"scope"`
	DimensionIndices []int                         `json:"dimension_indices,omitempty"`
}

func CloneToolDocumentationRequest(in *ToolDocumentationRequest) *ToolDocumentationRequest {
	if in == nil {
		return nil
	}
	out := *in
	out.DimensionIndices = append([]int(nil), in.DimensionIndices...)
	return &out
}

type ToolDocumentationRequestScope string

const (
	ToolDocumentationRequestOnly  ToolDocumentationRequestScope = "only"
	ToolDocumentationRequestMixed ToolDocumentationRequestScope = "mixed"
)

// ToolDocumentationRequestTeaching is shared by the JSON schema and agent
// guidance. The classifier chooses the requested domain, never its truth.
const ToolDocumentationRequestTeaching = "Optional tool_documentation_request declares questions about the host's documented tool capabilities, input/output contracts, units, prerequisites, or usage; it does not ask where those tools are implemented in the target repository. Omit it for the legacy source/runtime domain. Use scope=only when the entire request is static documentation and omit dimension_indices. Use scope=mixed with the exact explicit indices of required requested_answer_dimensions that ask only for documentation; all other source/runtime obligations remain intact. Do not assign actual source locations, current implementation behavior, measured values, target effects, causal attribution, or an explicit Trace window to documentation. Documentation must come from a successful current tool-documentation call and be retained for the answer; this declaration proves no capability, source code, measurement, or causal relation."

// ValidateToolDocumentationRequest checks precise model-owned declarations.
// No keyword scan or confidence score can create or erase an obligation.
func ValidateToolDocumentationRequest(rm *RequestModel) error {
	return errors.Join(CollectToolDocumentationRequestViolations(rm)...)
}

// CollectToolDocumentationRequestViolations reports every independent domain
// violation without rewriting the request. Invalid selections do not suppress
// later selections; repeated selections do not repeat their semantic errors.
func CollectToolDocumentationRequestViolations(rm *RequestModel) []error {
	if rm == nil || rm.ToolDocumentationRequest == nil {
		return nil
	}
	var violations []error
	p := rm.ToolDocumentationRequest
	switch p.Scope {
	case ToolDocumentationRequestOnly:
		if len(p.DimensionIndices) != 0 {
			violations = append(violations, fmt.Errorf("tool_documentation_request scope=only must omit dimension_indices; use mixed for selected dimensions"))
		}
		if reason := toolDocumentationIndependentObligation(rm); reason != "" {
			violations = append(violations, fmt.Errorf("tool_documentation_request scope=only conflicts with %s; preserve the independent obligation and use mixed with documentation-only dimension indices, or omit the documentation domain", reason))
		}
	case ToolDocumentationRequestMixed:
		if len(p.DimensionIndices) == 0 {
			violations = append(violations, fmt.Errorf("tool_documentation_request scope=mixed requires nonempty dimension_indices"))
		}
		seen := map[int]bool{}
		invalidIndexShapeReported := false
		for _, index := range p.DimensionIndices {
			if index <= 0 || seen[index] {
				if !invalidIndexShapeReported {
					violations = append(violations, fmt.Errorf("tool_documentation_request dimension_indices must be unique positive indices"))
					invalidIndexShapeReported = true
				}
				continue
			}
			seen[index] = true
			var selected *RequestedAnswerDimension
			matches := 0
			if rm.RequestedAnswerDimensions != nil && rm.RequestedAnswerDimensions.Active() {
				for i := range rm.RequestedAnswerDimensions.Dimensions {
					dim := &rm.RequestedAnswerDimensions.Dimensions[i]
					if dim.Index == index {
						matches++
						selected = dim
					}
				}
			}
			if matches > 1 {
				violations = append(violations, fmt.Errorf("tool_documentation_request index %d is ambiguous in requested_answer_dimensions", index))
				continue
			}
			if selected == nil || !selected.Required {
				violations = append(violations, fmt.Errorf("tool_documentation_request index %d must refer to one retained required requested_answer_dimensions row", index))
				continue
			}
			if toolDocumentationDimensionHasIndependentObligation(rm, *selected) {
				violations = append(violations, fmt.Errorf("tool_documentation_request index %d carries an independent source/runtime obligation; do not relabel that obligation as documentation", index))
			}
		}
	default:
		violations = append(violations, fmt.Errorf("tool_documentation_request.scope must be only or mixed"))
	}
	return violations
}

// ToolDocumentationOnlyRequested fails closed for invalid/replayed profiles.
// It does not establish that any documentation was actually read.
func ToolDocumentationOnlyRequested(rm *RequestModel) bool {
	return rm != nil && rm.ToolDocumentationRequest != nil &&
		rm.ToolDocumentationRequest.Scope == ToolDocumentationRequestOnly && ValidateToolDocumentationRequest(rm) == nil
}

// ToolDocumentationDimensionRequested is the shared applicability projection
// for mixed explanation seats. It never mutates the dimension list or source
// bindings, and an invalid declaration cannot waive a single seat.
func ToolDocumentationDimensionRequested(rm *RequestModel, index int) bool {
	if rm == nil || rm.ToolDocumentationRequest == nil || ValidateToolDocumentationRequest(rm) != nil {
		return false
	}
	if rm.ToolDocumentationRequest.Scope == ToolDocumentationRequestOnly {
		return true
	}
	for _, selected := range rm.ToolDocumentationRequest.DimensionIndices {
		if selected == index {
			return true
		}
	}
	return false
}

func toolDocumentationIndependentObligation(rm *RequestModel) string {
	if len(rm.UserPinnedFiles) > 0 || rm.HasCurrentSourceObligationSignal() ||
		rm.CurrentSourceExplanationProfile.Active() || rm.HasTypedCurrentSourceScopeRequest() ||
		rm.SourceInventoryProfile.Active() || rm.ChangeImpactProfile.Active() || rm.FieldValueProfile.Active() || rm.HistorySelectionProfile.Active() {
		return "a typed current-source request"
	}
	for _, target := range rm.AnalyzerHints.ExactTargets {
		if targetLooksLikeCurrentSourceAnchor(target) || textHasPreciseCurrentSourceAnchor(target) {
			return "an exact current-source target"
		}
	}
	if p := rm.SourceScopeProfile; p != nil && p.RequestedScope.IsValid() && p.RequestedScope != SourceScopeUnknown && len(p.SourceQuotes) > 0 {
		return "an explicitly scoped repository-source request"
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if len(hint.RequestedDimensionIndices) > 0 && CanonicalRequestedDimensionSource(hint.Path) != "" {
			return "a required source-file binding"
		}
	}
	if rm.RuntimeArtifactScopeProfile.Active() || rm.RuntimeTargetProfile.NamedTarget() || len(rm.RuntimeTargets) > 0 || rm.RuntimeArtifactValueProfile.Active() {
		return "a typed runtime scope or target"
	}
	if p := rm.RuntimeQuestionProfile; p != nil &&
		((p.Scope != "" && p.Scope != RuntimeQuestionScopeNotApplicable && p.Scope != RuntimeQuestionScopeUnspecified) ||
			p.RuntimeWorkRelationRequested || p.FrameCausalityRequested || len(p.FactFamilies) > 0) {
		return "a typed runtime answer obligation"
	}
	if rm.Intent == IntentRootCause || rm.Scenario == ScenarioRootCause || rm.DiagnosticProfile.RequiresCurrentStatusDiagnostic() {
		return "a diagnostic/source-status request"
	}
	if rm.RequestedAnswerDimensions != nil && rm.RequestedAnswerDimensions.Active() {
		for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
			if dim.Required && toolDocumentationDimensionHasIndependentObligation(rm, dim) {
				return fmt.Sprintf("required source/runtime dimension %d", dim.Index)
			}
		}
	}
	return ""
}

func toolDocumentationDimensionHasIndependentObligation(rm *RequestModel, dim RequestedAnswerDimension) bool {
	switch dim.Role {
	case RequestedAnswerDimensionCurrentKeyCode, RequestedAnswerDimensionSourceLocation,
		RequestedAnswerDimensionSourceAttribute, RequestedAnswerDimensionRuntimeWorkRelation,
		RequestedAnswerDimensionTargetEffectVerdict, RequestedAnswerDimensionCausalAttribution,
		RequestedAnswerDimensionCausalContributorSet:
		return true
	}
	if rm.dimensionHasPreciseCurrentSourceAnchor(dim) {
		return true
	}
	for _, hint := range rm.AnalyzerHints.RequiredFileHints {
		if CanonicalRequestedDimensionSource(hint.Path) == "" {
			continue
		}
		for _, index := range hint.RequestedDimensionIndices {
			if index == dim.Index {
				return true
			}
		}
	}
	return false
}

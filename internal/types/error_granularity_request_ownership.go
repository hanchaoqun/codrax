package types

import "strings"

// ErrorGranularityQuotesBelongToOtherDimensions only softens a duplicate
// optional classification. Every already-anchored profile quote must exactly
// equal a required dimension's quote whose typed role is not a failure-scope
// verdict. This does not classify raw text, grant evidence authority, or turn
// presentation dimensions into an additional mandatory declaration.
// Broader/differently quoted model classifications deliberately do not match.
func ErrorGranularityQuotesBelongToOtherDimensions(rm RequestModel) bool {
	p := rm.ErrorGranularityProfile
	if p == nil || !p.Active() || len(p.SourceQuotes) == 0 || rm.Intent == IntentReturnValue ||
		NormalizeRequirementKind(rm.AnalyzerHints.Kind) == ReqReturnValue {
		return false
	}
	owners := errorGranularityOtherDimensionQuotes(rm)
	for _, quote := range p.SourceQuotes {
		if !owners[strings.TrimSpace(quote)] {
			return false
		}
	}
	return true
}

func errorGranularityHasIndependentRequestQuote(rm RequestModel) bool {
	if rm.ErrorGranularityProfile == nil {
		return false
	}
	owners := errorGranularityOtherDimensionQuotes(rm)
	if len(owners) == 0 {
		return false
	}
	for _, quote := range rm.ErrorGranularityProfile.SourceQuotes {
		if quote = strings.TrimSpace(quote); quote != "" && !owners[quote] {
			return true
		}
	}
	return false
}

func errorGranularityOtherDimensionQuotes(rm RequestModel) map[string]bool {
	owners := make(map[string]bool)
	otherRoles := make(map[string]bool)
	if rm.RequestedAnswerDimensions == nil || !rm.RequestedAnswerDimensions.Active() {
		return owners
	}
	for _, dim := range rm.RequestedAnswerDimensions.Dimensions {
		quote := strings.TrimSpace(dim.SourceQuote)
		if !dim.Required || quote == "" {
			continue
		}
		switch dim.Role {
		case RequestedAnswerDimensionMemberSet, RequestedAnswerDimensionCount,
			RequestedAnswerDimensionFunctionOrPurpose, RequestedAnswerDimensionObservedValue,
			RequestedAnswerDimensionRelationPath, RequestedAnswerDimensionRuntimeWorkRelation,
			RequestedAnswerDimensionCausalAttribution, RequestedAnswerDimensionCausalContributorSet,
			RequestedAnswerDimensionSourceLocation, RequestedAnswerDimensionSourceAttribute,
			RequestedAnswerDimensionEvidenceSource, RequestedAnswerDimensionDiagram,
			RequestedAnswerDimensionStageWorkflow:
			owners[quote] = true
		default:
			// A shared quote with a branch/comparison/verdict or unspecified
			// role is ambiguous; an event role cannot erase the other meaning.
			otherRoles[quote] = true
		}
	}
	for quote := range otherRoles {
		delete(owners, quote)
	}
	return owners
}

package types

// EvidenceMechanismAuthorityBoundary returns a compact, typed source-shape
// boundary for evidence whose cited source can prove visible content but
// cannot, by itself, prove executable control flow.  It deliberately keys off
// ClaimForm rather than repository path, language, filename, or model prose:
// a prompt string, SQL literal, config key, comment, generated header and
// ordinary documentation line all receive the same honest authority.
//
// Empty means this helper adds no extra boundary; it does not positively
// certify every other anchor as a complete implementation proof.  Calls,
// assignments, guards, returns and other executable shapes retain their
// existing, more specific ClaimForm contracts.
func EvidenceMechanismAuthorityBoundary(item EvidenceItem) string {
	return MechanismAuthorityBoundaryForClaimForm(ClaimFormOf(item))
}

// MechanismAuthorityBoundaryForClaimForm projects the same boundary from an
// already-derived ClaimForm. Cross-stage identity carriers intentionally keep
// ClaimForm rather than copying a free-form sentence; prompt renderers can use
// this helper without reconstructing or weakening the original EvidenceItem.
func MechanismAuthorityBoundaryForClaimForm(form ClaimForm) string {
	switch form {
	case ClaimDefinitionFact:
		return "source_shape_authority=definition_site_only executable_body=unproven"
	case ClaimTextReferenceFact:
		return "source_shape_authority=visible_text_only executable_mechanism=unproven"
	case ClaimLiteralValueFact:
		return "source_shape_authority=literal_value_only executable_mechanism=unproven"
	default:
		return ""
	}
}

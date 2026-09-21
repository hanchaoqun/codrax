package tracefinding

import "github.com/hanchaoqun/codrax/internal/types"

// RootCauseNodeValueDescription is a read-only view of this exact node's
// candidate value. It reuses the candidate compiler, not a subject/rank lookup
// into another board or a reconstruction of magnitude components by a caller.
// No contract, candidate identity, selection, or projection is published or
// modified. Missing/ineligible rows provide no extra value description.
func RootCauseNodeValueDescription(projection types.TraceCausalProjection, node types.TraceCausalProjectionNode, language string) string {
	registryHash, err := RegistryHash()
	if err != nil {
		return ""
	}
	candidate, ok := compileCandidate(projection, node, registryHash, SeatFrameCausalityAuthority{})
	if !ok || !candidate.PrimaryEligible {
		return ""
	}
	return RootCauseValueDescriptionForLanguage(candidate.Decision, language)
}

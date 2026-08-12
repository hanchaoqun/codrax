package types

import "strings"

// DiagramParticipantIdentitySurfaces resolves a schema-valid participant's
// presentation label to a code-identity surface for SOFT planning consumers.
// Analyzer models sometimes preserve a useful display qualifier (for example
// "Analyzer agent" or "Mutable (in BusContext)") while also publishing the
// canonical identity in AnalyzerHints.Entities. A raw code-identity comparator
// must reject the decorated label, so prompt routing and exploration checklists
// need this typed bridge.
//
// The bridge is deliberately narrow:
//   - it reads only DiagramParticipantHint plus typed analyzer entities;
//   - a native code identity is returned unchanged;
//   - otherwise exactly one boundary-complete entity must occur in the primary
//     display segment (before a parenthetical qualifier);
//   - ambiguous/no-match labels remain unresolved.
//
// The result is planning identity only. It MUST NOT be used as source evidence,
// to mint a relation edge, or to relax an answer-document relation gate.
func DiagramParticipantIdentitySurfaces(rm RequestModel, participant DiagramParticipantHint) []string {
	identity := strings.TrimSpace(participant.Identity)
	if identity == "" {
		return nil
	}
	if IsCodeIdentitySurface(identity) {
		return []string{identity}
	}
	primary := identity
	if idx := strings.IndexRune(primary, '('); idx >= 0 {
		primary = strings.TrimSpace(primary[:idx])
	}
	if primary == "" {
		return nil
	}

	seen := map[string]bool{}
	var matched []string
	for _, entity := range rm.AnalyzerHints.Entities {
		entity = strings.TrimSpace(entity)
		if entity == "" || !IsCodeIdentitySurface(entity) || seen[entity] {
			continue
		}
		if !AnswerCodeIdentitySurfacesCompatible(primary, entity) &&
			!AnswerCodeSurfaceAppearsInText(primary, entity) {
			continue
		}
		seen[entity] = true
		matched = append(matched, entity)
	}
	if len(matched) != 1 {
		return nil
	}
	return matched
}

// DiagramParticipantHasPreciseSourceOperationIdentity reports whether a
// request-visible participant is precise enough to drive a hard source
// operation search. Participant role and request provenance alone establish
// display/coverage intent, not source identity. Once typed entity provenance
// is available, only a uniquely resolved symbol may become a hard operation
// endpoint obligation; scopes, concepts, prescan-only names, and ambiguous
// same-name symbols stay visible planning boundaries without forcing an
// impossible source hunt.
//
// Empty provenance is a compatibility lane for hand-built/legacy RequestModel
// values. Production analyzer output always carries provenance; callers with
// no lane retain the pre-provenance behavior.
func DiagramParticipantHasPreciseSourceOperationIdentity(rm RequestModel, participant DiagramParticipantHint) bool {
	surfaces := DiagramParticipantIdentitySurfaces(rm, participant)
	if len(surfaces) == 0 {
		return false
	}
	provenance := rm.AnalyzerHints.EntityProvenance
	if len(provenance) == 0 {
		return true
	}
	for _, surface := range surfaces {
		for _, candidate := range provenance {
			if candidate.Resolution != EntityResolutionSymbol || !candidate.Resolved || !candidate.UseForShape {
				continue
			}
			resolvedSurface := strings.TrimSpace(candidate.ResolvedAs)
			if resolvedSurface == "" {
				resolvedSurface = strings.TrimSpace(candidate.Surface)
			}
			if AnswerCodeIdentitySurfacesEquivalent(surface, resolvedSurface) ||
				AnswerCodeIdentitySurfacesCompatible(surface, resolvedSurface) {
				return true
			}
		}
	}
	return false
}

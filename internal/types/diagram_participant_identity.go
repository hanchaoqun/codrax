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

package types

import "strings"

// DiagramPrimaryVisibleIdentity projects one Mermaid display label onto its
// exact first-line identity carrier. Later lines commonly contain file/line or
// type metadata and must not make two structural consumers disagree about
// whether a requested participant is present. This is presentation identity
// only: it never resolves source symbols, proves relations, or scans prose.
func DiagramPrimaryVisibleIdentity(raw string) string {
	label := strings.TrimSpace(raw)
	cut := len(label)
	for _, separator := range []string{"<br/>", "<br>", `\n`, "\n"} {
		if idx := strings.Index(label, separator); idx >= 0 && idx < cut {
			cut = idx
		}
	}
	label = strings.TrimSpace(label[:cut])
	if symbol, _, ok := ParseAnswerSupportRefMemberLocation(label); ok && strings.TrimSpace(symbol) != "" {
		label = strings.TrimSpace(symbol)
	}
	if len(label) >= 3 && label[0] == '`' && label[len(label)-1] == '`' &&
		strings.Count(label, "`") == 2 {
		identity := label[1 : len(label)-1]
		if IsCodeIdentitySurface(identity) {
			return identity
		}
	}
	return label
}

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
//   - a native code identity remains the presentation surface, while one
//     uniquely resolved symbol provenance may add its canonical source name;
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
		return diagramParticipantCanonicalProvenanceSurfaces(rm, []string{identity})
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
	return diagramParticipantCanonicalProvenanceSurfaces(rm, matched)
}

// diagramParticipantCanonicalProvenanceSurfaces preserves the request-visible
// participant label and, when analyzer entity provenance resolves that label
// to exactly one source symbol, adds the canonical parser identity. This is a
// typed identity projection only: it does not inspect request/answer prose,
// create evidence, choose an edge, or authorize a relation. Downstream
// relation consumers still require their ordinary citable operation rows.
//
// Keeping both surfaces is important. The display label is the answer contract
// (for example `Extractor`), while the source operation may be owned by
// `extractorEvaluator`. Dropping ResolvedAs makes navigation, completion, and
// diagram validation disagree even though all three consume the same precise
// analyzer resolution.
func diagramParticipantCanonicalProvenanceSurfaces(rm RequestModel, base []string) []string {
	if len(base) == 0 || len(rm.AnalyzerHints.EntityProvenance) == 0 {
		return base
	}
	var resolved []string
	for _, candidate := range rm.AnalyzerHints.EntityProvenance {
		if candidate.Resolution != EntityResolutionSymbol || !candidate.Resolved || !candidate.UseForShape {
			continue
		}
		sourceSurface := strings.TrimSpace(candidate.Surface)
		resolvedSurface := strings.TrimSpace(candidate.ResolvedAs)
		if resolvedSurface == "" {
			resolvedSurface = sourceSurface
		}
		if resolvedSurface == "" || !IsCodeIdentitySurface(resolvedSurface) {
			continue
		}
		matched := false
		for _, surface := range base {
			if AnswerCodeIdentitySurfacesEquivalent(surface, sourceSurface) ||
				AnswerCodeIdentitySurfacesCompatible(surface, sourceSurface) ||
				AnswerCodeIdentitySurfacesEquivalent(surface, resolvedSurface) ||
				AnswerCodeIdentitySurfacesCompatible(surface, resolvedSurface) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		duplicate := false
		for _, current := range resolved {
			if AnswerCodeIdentitySurfacesEquivalent(current, resolvedSurface) || strings.EqualFold(current, resolvedSurface) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			resolved = append(resolved, resolvedSurface)
		}
	}
	if len(resolved) != 1 {
		// Ambiguous provenance remains presentation-only and cannot widen a
		// source operation identity.
		return base
	}
	out := append([]string(nil), base...)
	for _, current := range out {
		if AnswerCodeIdentitySurfacesEquivalent(current, resolved[0]) || strings.EqualFold(current, resolved[0]) {
			return out
		}
	}
	return append(out, resolved[0])
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

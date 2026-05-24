package types

import (
	"fmt"
	"sort"
	"strings"
)

// EvidenceRelationCandidateSource adapts accepted structured evidence into the
// shared typed-relation provider boundary.
//
// This is deliberately evidence-driven rather than framework-pattern-driven:
// registration / binding / observer relations vary across languages and
// frameworks, but emit_evidence already gives us a grounded, typed contract.
// The provider never inspects raw user prose or model free-form text.
type EvidenceRelationCandidateSource struct {
	Items []EvidenceItem
}

func (s EvidenceRelationCandidateSource) TypedRelationCandidates(q TypedRelationQuery) []TypedRelationCandidate {
	if (!q.AllowsKind(TypedRelationRegisters) && !q.AllowsKind(TypedRelationConfigures)) ||
		len(s.Items) == 0 || len(q.Sources) == 0 {
		return nil
	}
	var out []TypedRelationCandidate
	seen := map[string]bool{}
	for _, item := range s.Items {
		for _, source := range q.Sources {
			if q.AllowsKind(TypedRelationRegisters) && evidenceRelationRegistrationItemUsable(item, q.Purpose) {
				out = appendEvidenceRelationCandidates(out, seen, evidenceRegistrationCandidatesForSource(item, source, q.Purpose)...)
			}
			if q.AllowsKind(TypedRelationConfigures) && evidenceRelationConfigItemUsable(item, q.Purpose) {
				out = appendEvidenceRelationCandidates(out, seen, evidenceConfigCandidatesForSource(item, source, q.Purpose)...)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SourceName != out[j].SourceName {
			return out[i].SourceName < out[j].SourceName
		}
		if out[i].Member.Name != out[j].Member.Name {
			return out[i].Member.Name < out[j].Member.Name
		}
		if out[i].Member.File != out[j].Member.File {
			return out[i].Member.File < out[j].Member.File
		}
		return out[i].Member.Line < out[j].Member.Line
	})
	if q.MaxMembers > 0 && len(out) > q.MaxMembers {
		out = out[:q.MaxMembers]
	}
	return out
}

func appendEvidenceRelationCandidates(dst []TypedRelationCandidate, seen map[string]bool, candidates ...TypedRelationCandidate) []TypedRelationCandidate {
	for _, candidate := range candidates {
		key := evidenceRelationCandidateKey(candidate)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		dst = append(dst, candidate)
	}
	return dst
}

func evidenceRelationRegistrationItemUsable(item EvidenceItem, purpose TypedRelationPurpose) bool {
	if item.Kind != EvidenceRegistration {
		return false
	}
	if strings.TrimSpace(item.Source) == "" || item.LineStart <= 0 {
		return false
	}
	if item.GroundingStatus == GroundingUngrounded {
		return false
	}
	switch item.ContextRole {
	case EvidenceContextRoleIllustrativeOnly, EvidenceContextRoleRelatedContext:
		return false
	}
	if purpose != TypedRelationPurposeCoverageGate {
		return true
	}
	if !item.IsCitable() {
		return false
	}
	if item.ContextRole == EvidenceContextRoleDefining {
		return true
	}
	return item.Salience == SalienceLoadBearing || item.Salience == SalienceExhaustListed
}

func evidenceRelationConfigItemUsable(item EvidenceItem, purpose TypedRelationPurpose) bool {
	switch item.Kind {
	case EvidenceDirect, EvidenceMechanism, EvidenceRelationship, EvidenceConcrete:
	default:
		return false
	}
	if strings.TrimSpace(item.Source) == "" || item.LineStart <= 0 {
		return false
	}
	if item.GroundingStatus == GroundingUngrounded {
		return false
	}
	switch item.ContextRole {
	case EvidenceContextRoleIllustrativeOnly, EvidenceContextRoleRelatedContext:
		return false
	}
	if purpose != TypedRelationPurposeCoverageGate {
		return true
	}
	if !item.IsCitable() {
		return false
	}
	if item.ContextRole == EvidenceContextRoleDefining {
		return true
	}
	return item.Salience == SalienceLoadBearing || item.Salience == SalienceExhaustListed
}

func evidenceRegistrationCandidatesForSource(item EvidenceItem, rawSource string, purpose TypedRelationPurpose) []TypedRelationCandidate {
	source := strings.TrimSpace(rawSource)
	if source == "" {
		return nil
	}
	var out []TypedRelationCandidate
	if evidenceRelationSurfaceMatches(source, append([]string{item.Object}, item.SurfaceTerms...)...) {
		if member := evidenceRegistrationMember(item, item.Subject, "registration_site"); member.Name != "" {
			out = append(out, evidenceRegistrationCandidate(item, source, "registration_target", member, purpose))
		}
	}
	if evidenceRelationSurfaceMatches(source, item.Subject, item.OwnerSymbol) {
		if member := evidenceRegistrationMember(item, item.Object, "registered_target"); member.Name != "" {
			out = append(out, evidenceRegistrationCandidate(item, source, "registrar", member, purpose))
		}
	}
	return out
}

func evidenceConfigCandidatesForSource(item EvidenceItem, rawSource string, purpose TypedRelationPurpose) []TypedRelationCandidate {
	source := strings.TrimSpace(rawSource)
	if source == "" {
		return nil
	}
	configSurfaces := append([]string{item.Object, item.AnchorSymbol}, item.SurfaceTerms...)
	var out []TypedRelationCandidate
	if evidenceRelationSurfaceMatches(source, configSurfaces...) {
		if member := evidenceConfigMember(item, item.Subject, "config_site"); member.Name != "" &&
			!evidenceRelationSurfaceMatches(member.Name, source) {
			out = append(out, evidenceConfigCandidate(item, source, "config_key", member, purpose))
		}
	}
	if evidenceRelationSurfaceMatches(source, item.Subject, item.OwnerSymbol) {
		if member := evidenceConfigMember(item, item.Object, "config_key"); member.Name != "" &&
			!evidenceRelationSurfaceMatches(member.Name, source) {
			out = append(out, evidenceConfigCandidate(item, source, "config_site", member, purpose))
		}
	}
	return out
}

func evidenceRegistrationCandidate(item EvidenceItem, sourceName, sourceKind string, member TypedRelationMember, purpose TypedRelationPurpose) TypedRelationCandidate {
	precision := TypedRelationPrecisionExactEvidence
	if purpose == TypedRelationPurposePromptHint && item.GroundingStatus == GroundingRecovered {
		precision = TypedRelationPrecisionNameOnly
	}
	return TypedRelationCandidate{
		Relation:   TypedRelationRegisters,
		SourceName: strings.TrimSpace(sourceName),
		SourceKind: sourceKind,
		SourceFile: cleanEvidenceRelationPath(item.Source),
		SourceLine: item.LineStart,
		Member:     member,
		Carrier:    TypedRelationCarrierEvidence,
		Precision:  precision,
	}
}

func evidenceConfigCandidate(item EvidenceItem, sourceName, sourceKind string, member TypedRelationMember, purpose TypedRelationPurpose) TypedRelationCandidate {
	precision := TypedRelationPrecisionExactEvidence
	if purpose == TypedRelationPurposePromptHint && item.GroundingStatus == GroundingRecovered {
		precision = TypedRelationPrecisionNameOnly
	}
	return TypedRelationCandidate{
		Relation:   TypedRelationConfigures,
		SourceName: strings.TrimSpace(sourceName),
		SourceKind: sourceKind,
		SourceFile: cleanEvidenceRelationPath(item.Source),
		SourceLine: item.LineStart,
		Member:     member,
		Carrier:    TypedRelationCarrierEvidence,
		Precision:  precision,
	}
}

func evidenceRegistrationMember(item EvidenceItem, preferred, fallbackKind string) TypedRelationMember {
	name := strings.TrimSpace(preferred)
	if name == "" {
		name = strings.TrimSpace(item.AnchorSymbol)
	}
	if name == "" {
		return TypedRelationMember{}
	}
	kind := strings.TrimSpace(string(item.AnchorKind))
	if kind == "" {
		kind = fallbackKind
	}
	return TypedRelationMember{
		Name:     name,
		File:     cleanEvidenceRelationPath(item.Source),
		Line:     item.LineStart,
		Kind:     kind,
		Distance: 1,
	}
}

func evidenceConfigMember(item EvidenceItem, preferred, fallbackKind string) TypedRelationMember {
	name := strings.TrimSpace(preferred)
	if name == "" {
		name = strings.TrimSpace(item.OwnerSymbol)
	}
	if name == "" {
		name = strings.TrimSpace(item.AnchorSymbol)
	}
	if name == "" {
		return TypedRelationMember{}
	}
	kind := strings.TrimSpace(string(item.AnchorKind))
	if kind == "" {
		kind = fallbackKind
	}
	return TypedRelationMember{
		Name:     name,
		File:     cleanEvidenceRelationPath(item.Source),
		Line:     item.LineStart,
		Kind:     kind,
		Distance: 1,
	}
}

func evidenceRelationSurfaceMatches(source string, surfaces ...string) bool {
	sourceKey := evidenceRelationStableKey(source)
	if sourceKey == "" {
		return false
	}
	for _, surface := range surfaces {
		for _, candidate := range evidenceRelationSurfaceCandidates(surface) {
			if sourceKey == evidenceRelationStableKey(candidate) {
				return true
			}
		}
	}
	return false
}

func evidenceRelationSurfaceCandidates(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := []string{raw}
	if trimmed := strings.Trim(raw, "`'\""); trimmed != "" && trimmed != raw {
		out = append(out, trimmed)
	}
	if tail := NormalizedSurfaceSymbolTail(raw); tail != "" && tail != raw {
		out = append(out, tail)
	}
	for _, sep := range []string{" @ ", " | ", "\t", ":", "::", ".", "/"} {
		if idx := strings.LastIndex(raw, sep); idx >= 0 && idx+len(sep) < len(raw) {
			out = append(out, strings.TrimSpace(raw[idx+len(sep):]))
		}
	}
	return dedupEvidenceRelationStrings(out)
}

func evidenceRelationStableKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.Trim(raw, "`'\"")
	if tail := NormalizedSurfaceSymbolTail(raw); tail != "" {
		raw = tail
	}
	return strings.ToLower(raw)
}

func evidenceRelationCandidateKey(candidate TypedRelationCandidate) string {
	if candidate.Relation == "" || candidate.SourceName == "" || candidate.Member.Name == "" {
		return ""
	}
	return strings.ToLower(fmt.Sprintf("%s|%s|%s|%s|%d",
		candidate.Relation,
		evidenceRelationStableKey(candidate.SourceName),
		evidenceRelationStableKey(candidate.Member.Name),
		cleanEvidenceRelationPath(candidate.Member.File),
		candidate.Member.Line))
}

func cleanEvidenceRelationPath(raw string) string {
	raw = strings.ReplaceAll(strings.TrimSpace(raw), `\`, `/`)
	raw = strings.TrimPrefix(raw, "./")
	return strings.TrimSpace(raw)
}

func dedupEvidenceRelationStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

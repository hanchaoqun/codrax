package types

import (
	"fmt"
	"sort"
	"strconv"
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
	if (!q.AllowsKind(TypedRelationRegisters) && !q.AllowsKind(TypedRelationConfigures) && !q.AllowsKind(TypedRelationRoutesTo)) ||
		len(s.Items) == 0 || len(q.Sources) == 0 {
		return nil
	}
	var out []TypedRelationCandidate
	seen := map[string]bool{}
	itemsByID := make(map[string]EvidenceItem, len(s.Items))
	for _, item := range s.Items {
		if id := strings.TrimSpace(item.ID); id != "" {
			itemsByID[id] = item
		}
	}
	for _, item := range s.Items {
		for _, source := range q.Sources {
			if q.AllowsKind(TypedRelationRegisters) && evidenceRelationRegistrationItemUsable(item, q.Purpose) {
				out = appendEvidenceRelationCandidates(out, seen, evidenceRegistrationCandidatesForSource(item, source, q.Purpose)...)
			}
			if q.AllowsKind(TypedRelationRegisters) {
				out = appendEvidenceRelationCandidates(out, seen, evidenceBridgeLiteralRegistrationCandidatesForSource(item, itemsByID, source, q.Purpose)...)
			}
			if q.AllowsKind(TypedRelationConfigures) && evidenceRelationConfigItemUsable(item, q.Purpose) {
				out = appendEvidenceRelationCandidates(out, seen, evidenceConfigCandidatesForSource(item, source, q.Purpose)...)
			}
			if q.AllowsKind(TypedRelationRoutesTo) && evidenceRelationRouteItemUsable(item, q.Purpose) {
				out = appendEvidenceRelationCandidates(out, seen, evidenceRouteCandidatesForSource(item, source, q.Purpose)...)
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

// evidenceBridgeLiteralRegistrationCandidatesForSource projects one exact
// registry member only when the deterministic bridge producer preserved both
// ends of the join: the registration-family binding site and the terminal
// identity return. Generic dataflow prose, a lone literal, or a same-named
// method cannot mint this relation. The projection consumes structured fields
// and stable evidence IDs only; it never parses the bridge summary or any
// request/model/final-answer prose.
func evidenceBridgeLiteralRegistrationCandidatesForSource(
	bridge EvidenceItem,
	itemsByID map[string]EvidenceItem,
	rawSource string,
	purpose TypedRelationPurpose,
) []TypedRelationCandidate {
	if bridge.Kind != EvidenceDataflowPath ||
		bridge.Producer != "bridge_literal" ||
		bridge.Predicate != "resolution_chain" ||
		strings.TrimSpace(bridge.Source) == "" || bridge.LineStart <= 0 ||
		strings.TrimSpace(bridge.OwnerSymbol) == "" ||
		strings.TrimSpace(bridge.AnchorSymbol) == "" ||
		strings.TrimSpace(bridge.Object) == "" ||
		len(bridge.DerivedFrom) != 1 ||
		bridge.GroundingStatus == GroundingUngrounded ||
		!evidenceRelationContextRoleUsable(bridge.ContextRole) {
		return nil
	}
	if purpose == TypedRelationPurposeCoverageGate && !bridge.IsCitable() {
		return nil
	}
	source := strings.TrimSpace(rawSource)
	if source == "" || !evidenceRelationSurfaceMatches(source, bridge.OwnerSymbol) {
		return nil
	}
	terminal, ok := itemsByID[strings.TrimSpace(bridge.DerivedFrom[0])]
	if !ok || terminal.Kind != EvidenceConcrete ||
		terminal.Producer != "bridge_literal_terminal" ||
		terminal.Predicate != "returns" ||
		strings.TrimSpace(terminal.Source) == "" || terminal.LineStart <= 0 ||
		terminal.GroundingStatus == GroundingUngrounded ||
		!evidenceRelationContextRoleUsable(terminal.ContextRole) ||
		!strings.EqualFold(strings.TrimSpace(terminal.Subject), strings.TrimSpace(bridge.AnchorSymbol)) ||
		strings.TrimSpace(terminal.Object) != strings.TrimSpace(bridge.Object) {
		return nil
	}
	if purpose == TypedRelationPurposeCoverageGate && !terminal.IsCitable() {
		return nil
	}
	memberName, ok := evidenceRelationQuotedLiteral(terminal.Object)
	if !ok {
		return nil
	}
	return []TypedRelationCandidate{{
		Relation:   TypedRelationRegisters,
		SourceName: source,
		SourceKind: "registrar_identity_chain",
		SourceFile: cleanEvidenceRelationPath(bridge.Source),
		SourceLine: bridge.LineStart,
		Member: TypedRelationMember{
			Name:     memberName,
			File:     cleanEvidenceRelationPath(terminal.Source),
			Line:     terminal.LineStart,
			Kind:     "registered_identity",
			Distance: 1,
		},
		Carrier:   TypedRelationCarrierEvidence,
		Precision: TypedRelationPrecisionExactEvidence,
	}}
}

func evidenceRelationContextRoleUsable(role EvidenceContextRole) bool {
	switch role {
	case EvidenceContextRoleIllustrativeOnly, EvidenceContextRoleRelatedContext:
		return false
	default:
		return true
	}
}

func evidenceRelationQuotedLiteral(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 {
		return "", false
	}
	value, err := strconv.Unquote(raw)
	if err != nil || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

func appendEvidenceRelationCandidates(dst []TypedRelationCandidate, seen map[string]bool, candidates ...TypedRelationCandidate) []TypedRelationCandidate {
	for _, candidate := range candidates {
		candidate = NormalizeTypedRelationCandidateSourceRole(candidate)
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

func evidenceRelationRouteItemUsable(item EvidenceItem, purpose TypedRelationPurpose) bool {
	switch item.Kind {
	case EvidenceDirect, EvidenceMechanism, EvidenceRelationship, EvidenceRegistration:
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

func evidenceRouteCandidatesForSource(item EvidenceItem, rawSource string, purpose TypedRelationPurpose) []TypedRelationCandidate {
	source := strings.TrimSpace(rawSource)
	if source == "" {
		return nil
	}
	routeSurfaces := append([]string{item.Object, item.AnchorSymbol}, item.SurfaceTerms...)
	var out []TypedRelationCandidate
	if evidenceRelationSurfaceMatches(source, routeSurfaces...) {
		if member := evidenceRouteMember(item, item.Subject, "route_handler"); member.Name != "" &&
			!evidenceRelationSurfaceMatches(member.Name, source) {
			out = append(out, evidenceRouteCandidate(item, source, "route", member, purpose))
		}
	}
	if evidenceRelationSurfaceMatches(source, item.Subject, item.OwnerSymbol) {
		if member := evidenceRouteMember(item, item.Object, "route"); member.Name != "" &&
			!evidenceRelationSurfaceMatches(member.Name, source) {
			out = append(out, evidenceRouteCandidate(item, source, "route_handler", member, purpose))
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

func evidenceRouteCandidate(item EvidenceItem, sourceName, sourceKind string, member TypedRelationMember, purpose TypedRelationPurpose) TypedRelationCandidate {
	precision := TypedRelationPrecisionExactEvidence
	if purpose == TypedRelationPurposePromptHint && item.GroundingStatus == GroundingRecovered {
		precision = TypedRelationPrecisionNameOnly
	}
	return TypedRelationCandidate{
		Relation:   TypedRelationRoutesTo,
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

func evidenceRouteMember(item EvidenceItem, preferred, fallbackKind string) TypedRelationMember {
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

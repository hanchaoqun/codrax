package types

import (
	"strconv"
	"strings"
)

// AnswerPrincipalMemberSurface classifies the visible identity of an
// enumeration member after facet evidence has selected the principal
// evidence pool. It is deliberately typed over EvidenceItem fields and
// ClaimForm, not request prose: source/path/location questions,
// relationship enumerations, and symbol enumerations all share this
// boundary instead of each validator rediscovering it locally.
type AnswerPrincipalMemberSurface string

const (
	PrincipalMemberSurfaceUnknown        AnswerPrincipalMemberSurface = ""
	PrincipalMemberSurfaceSymbolLike     AnswerPrincipalMemberSurface = "symbol_like"
	PrincipalMemberSurfaceDisplayLabel   AnswerPrincipalMemberSurface = "display_label"
	PrincipalMemberSurfaceSourceLocation AnswerPrincipalMemberSurface = "source_location"
)

func (s AnswerPrincipalMemberSurface) IsNonSymbol() bool {
	return s == PrincipalMemberSurfaceDisplayLabel || s == PrincipalMemberSurfaceSourceLocation
}

func (s AnswerPrincipalMemberSurface) IsSymbolLike() bool {
	return s == PrincipalMemberSurfaceSymbolLike
}

// PrincipalMemberSurfaceForRequest layers typed request intent over the
// evidence-only surface classifier. It is intentionally narrow: only
// structured profiles that explicitly ask for file/site impact members can
// override a definition-like evidence item into a source-location member.
// No raw request text or keyword scoring is consulted.
func PrincipalMemberSurfaceForRequest(rm RequestModel, item EvidenceItem, peers []EvidenceItem) AnswerPrincipalMemberSurface {
	if requestWantsSourceLocationMemberSurface(rm, item) {
		return PrincipalMemberSurfaceSourceLocation
	}
	return PrincipalMemberSurfaceForEvidenceSet(item, peers)
}

func requestWantsSourceLocationMemberSurface(rm RequestModel, item EvidenceItem) bool {
	if strings.TrimSpace(item.Source) == "" {
		return false
	}
	if rm.ChangeImpactProfile == nil || !rm.ChangeImpactProfile.Active() {
		return false
	}
	switch rm.ChangeImpactProfile.RequestedOutput {
	case ImpactOutputFiles, ImpactOutputSites:
		return true
	default:
		return false
	}
}

// PrincipalMemberSurfaceForEvidenceSet returns the answer-visible member
// surface for item within its typed peer set. The peer set lets relation
// evidence orient itself without keyword checks:
//
//   - many sources pointing at one endpoint => the source/location is the
//     principal member (reverse dependency, call-site, assignment-site);
//   - one source pointing at many endpoints => the endpoint/display label
//     is the principal member (imports from a file, precedence labels);
//   - symbol facts stay symbol-like and still use symbol-specific gates.
func PrincipalMemberSurfaceForEvidenceSet(item EvidenceItem, peers []EvidenceItem) AnswerPrincipalMemberSurface {
	form := ClaimFormOf(item)
	switch form {
	case ClaimImportEdge:
		return relationPrincipalMemberSurface(item, peers, relationEndpointImport)
	case ClaimCallEdge, ClaimAssignmentFact, ClaimGuardCondition, ClaimReturnFact:
		return relationPrincipalMemberSurface(item, peers, relationEndpointSubjectObject)
	case ClaimPrecedenceRole, ClaimExternalObservation:
		return PrincipalMemberSurfaceDisplayLabel
	case ClaimDefinitionFact, ClaimAbsenceFact:
		return PrincipalMemberSurfaceSymbolLike
	default:
		if form.UsesNonSymbolLabelSurface() {
			return PrincipalMemberSurfaceDisplayLabel
		}
		if form.LabelSurfaceKind() == ClaimLabelSurfaceSymbolLike {
			return PrincipalMemberSurfaceSymbolLike
		}
		return PrincipalMemberSurfaceUnknown
	}
}

type relationEndpointMode int

const (
	relationEndpointSubjectObject relationEndpointMode = iota
	relationEndpointImport
)

func relationPrincipalMemberSurface(item EvidenceItem, peers []EvidenceItem, mode relationEndpointMode) AnswerPrincipalMemberSurface {
	if len(peers) < 2 {
		if ClaimFormOf(item).UsesNonSymbolLabelSurface() {
			return PrincipalMemberSurfaceDisplayLabel
		}
		return PrincipalMemberSurfaceSymbolLike
	}
	form := ClaimFormOf(item)
	sourceSet := make(map[string]bool)
	endpointSet := make(map[string]bool)
	for _, peer := range peers {
		if ClaimFormOf(peer) != form {
			continue
		}
		if src := principalMemberSourceIdentity(peer, mode); src != "" {
			sourceSet[src] = true
		}
		if endpoint := principalMemberEndpointIdentity(peer, mode); endpoint != "" {
			endpointSet[endpoint] = true
		}
	}
	if len(sourceSet) > 1 && len(endpointSet) <= 1 {
		return PrincipalMemberSurfaceSourceLocation
	}
	if form.UsesNonSymbolLabelSurface() {
		return PrincipalMemberSurfaceDisplayLabel
	}
	return PrincipalMemberSurfaceSymbolLike
}

func principalMemberSourceIdentity(item EvidenceItem, mode relationEndpointMode) string {
	source := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
	if source == "" {
		return ""
	}
	if mode == relationEndpointImport {
		return strings.ToLower(source)
	}
	if item.LineStart > 0 {
		return strings.ToLower(source) + ":" + strings.TrimSpace(intString(item.LineStart))
	}
	return strings.ToLower(source)
}

func principalMemberEndpointIdentity(item EvidenceItem, mode relationEndpointMode) string {
	var parts []string
	switch mode {
	case relationEndpointImport:
		parts = append(parts, item.Object)
		for _, term := range item.SurfaceTerms {
			parts = append(parts, term)
		}
		parts = append(parts, item.AnchorSymbol)
	default:
		parts = append(parts, item.Subject, item.Predicate, item.Object, item.AnchorSymbol, item.Condition)
	}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			return strings.ToLower(part)
		}
	}
	return ""
}

func intString(v int) string {
	if v <= 0 {
		return ""
	}
	return strconv.Itoa(v)
}

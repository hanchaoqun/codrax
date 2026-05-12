package types

import (
	"fmt"
	"strconv"
	"strings"
)

const definitionSupportMemberCitationWindow = 5

// AnswerSupportMemberObligation is a principal support-lane member
// that must survive into an enumeration answer's principal list/table
// surface. It is derived from typed support entries, not from raw
// request text, so the finalizer cannot silently drop answer-grade
// evidence that the explorer already structured.
type AnswerSupportMemberObligation struct {
	EvidenceID   string
	Location     string
	Label        string
	ClaimForm    ClaimForm
	SurfaceTerms []string
	Source       string
	LineStart    int

	// EquivalentLocations holds other file:line anchors that describe
	// the same typed principal member. Definition facts commonly have
	// both declaration-line and field/body-line evidence, especially
	// across Go, C/C++, ArkTS, Cangjie, and generated bindings. These
	// anchors are equivalent for member-preservation purposes; keeping
	// them typed prevents the finalizer from bouncing between adjacent
	// valid citations.
	EquivalentLocations []string
}

// LocationHint returns the citation shape a repair prompt should ask
// for. It deliberately exposes equivalent definition anchors instead
// of pretending there is only one exact line when the typed evidence
// says several adjacent lines describe the same member.
func (ob AnswerSupportMemberObligation) LocationHint() string {
	locations := supportMemberCoverageLocations(ob)
	if len(locations) == 0 {
		return strings.TrimSpace(ob.Location)
	}
	if len(locations) == 1 {
		return locations[0]
	}
	const maxShown = 4
	if len(locations) <= maxShown {
		return "one of " + strings.Join(locations, ", ")
	}
	return fmt.Sprintf("one of %s, ... (%d equivalent typed anchors)", strings.Join(locations[:maxShown], ", "), len(locations))
}

// PrincipalSupportMemberObligations returns the answer-grade
// principal evidence entries whose locations should appear as
// cited list/table items in QFEnumeration answers. Other families can
// use principal support lanes for enrichment or context; forcing all
// entries there to render as one item each would be too strict.
func PrincipalSupportMemberObligations(plan *AnswerSupportPlan) []AnswerSupportMemberObligation {
	if plan == nil || plan.Family != QFEnumeration {
		return nil
	}
	seen := make(map[string]int)
	var out []AnswerSupportMemberObligation
	for _, lane := range plan.Lanes {
		if lane.Kind != SupportLanePrincipalEvidence {
			continue
		}
		for _, entry := range lane.Entries {
			if !principalSupportEntryRequiresMemberCoverage(entry) {
				continue
			}
			ob := principalSupportMemberObligation(entry)
			key := principalSupportMemberDedupKey(ob)
			if key == "" {
				continue
			}
			if idx, ok := seen[key]; ok {
				mergePrincipalSupportMemberObligation(&out[idx], ob)
				continue
			}
			seen[key] = len(out)
			out = append(out, ob)
		}
	}
	return out
}

func principalSupportEntryRequiresMemberCoverage(entry AnswerSupportEntry) bool {
	if strings.TrimSpace(entry.Location) == "" {
		return false
	}
	switch entry.ClaimForm {
	case ClaimDefinitionFact, ClaimCallEdge, ClaimGuardCondition,
		ClaimAssignmentFact, ClaimReturnFact, ClaimPrecedenceRole,
		ClaimExternalObservation, ClaimImportEdge:
		return true
	case ClaimAbsenceFact, ClaimUnknown:
		return false
	default:
		return false
	}
}

func principalSupportMemberObligation(entry AnswerSupportEntry) AnswerSupportMemberObligation {
	source, line := supportMemberSourceLine(entry.Source, entry.LineStart, entry.Location)
	ob := AnswerSupportMemberObligation{
		EvidenceID: strings.TrimSpace(entry.EvidenceID),
		Location:   normalizeAnswerSupportLocation(entry.Location),
		Label:      principalSupportMemberLabel(entry),
		ClaimForm:  entry.ClaimForm,
		Source:     source,
		LineStart:  line,
	}
	seen := make(map[string]bool)
	addTerm := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		key := strings.ToLower(s)
		if seen[key] {
			return
		}
		seen[key] = true
		ob.SurfaceTerms = append(ob.SurfaceTerms, s)
	}
	for _, term := range entry.SurfaceTerms {
		addTerm(term)
	}
	addTerm(entry.Subject)
	addTerm(entry.Object)
	addTerm(entry.AnchorSymbol)
	addTerm(entry.OwnerSymbol)
	if ob.Label == "" && len(ob.SurfaceTerms) > 0 {
		ob.Label = ob.SurfaceTerms[0]
	}
	if ob.Label == "" {
		ob.Label = strings.TrimSpace(entry.Text)
	}
	return ob
}

func principalSupportMemberDedupKey(ob AnswerSupportMemberObligation) string {
	label := normalizedSupportMemberIdentity(ob.Label)
	form := string(ob.ClaimForm)
	if form == "" {
		form = string(ClaimUnknown)
	}
	if ob.ClaimForm == ClaimDefinitionFact && strings.TrimSpace(ob.Source) != "" && label != "" {
		return form + "\x00" + normalizeAnswerSupportPath(ob.Source) + "\x00" + label
	}
	location := strings.TrimSpace(ob.Location)
	if location == "" && label == "" {
		return ""
	}
	return form + "\x00" + location + "\x00" + label
}

func mergePrincipalSupportMemberObligation(dst *AnswerSupportMemberObligation, src AnswerSupportMemberObligation) {
	if dst == nil {
		return
	}
	if strings.TrimSpace(dst.EvidenceID) == "" {
		dst.EvidenceID = src.EvidenceID
	}
	if strings.TrimSpace(dst.Location) == "" {
		dst.Location = src.Location
	}
	for _, location := range supportMemberCoverageLocations(src) {
		appendEquivalentSupportMemberLocation(dst, location)
	}
	if strings.TrimSpace(dst.Label) == "" {
		dst.Label = src.Label
	}
	if strings.TrimSpace(dst.Source) == "" {
		dst.Source = src.Source
	}
	if dst.LineStart <= 0 {
		dst.LineStart = src.LineStart
	}
	seen := make(map[string]bool, len(dst.SurfaceTerms)+len(src.SurfaceTerms))
	for _, term := range dst.SurfaceTerms {
		if key := strings.ToLower(strings.TrimSpace(term)); key != "" {
			seen[key] = true
		}
	}
	for _, term := range src.SurfaceTerms {
		term = strings.TrimSpace(term)
		key := strings.ToLower(term)
		if term == "" || seen[key] {
			continue
		}
		seen[key] = true
		dst.SurfaceTerms = append(dst.SurfaceTerms, term)
	}
}

func appendEquivalentSupportMemberLocation(ob *AnswerSupportMemberObligation, location string) {
	if ob == nil {
		return
	}
	location = normalizeAnswerSupportLocation(location)
	if location == "" || location == ob.Location {
		return
	}
	for _, existing := range ob.EquivalentLocations {
		if normalizeAnswerSupportLocation(existing) == location {
			return
		}
	}
	ob.EquivalentLocations = append(ob.EquivalentLocations, location)
}

func principalSupportMemberLabel(entry AnswerSupportEntry) string {
	if entry.ClaimForm == ClaimImportEdge && strings.TrimSpace(entry.Subject) != "" && strings.TrimSpace(entry.Object) != "" {
		return fmt.Sprintf("%s -> %s", strings.TrimSpace(entry.Subject), strings.TrimSpace(entry.Object))
	}
	for _, s := range []string{entry.AnchorSymbol, entry.Subject, entry.Object} {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	for _, s := range entry.SurfaceTerms {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// MissingPrincipalSupportMembers returns the member obligations that
// are not represented by any principal list/table row item with a
// citation to the same typed support-entry location.
func MissingPrincipalSupportMembers(doc *AnswerDocumentV2, plan *AnswerSupportPlan) []AnswerSupportMemberObligation {
	if doc == nil {
		return nil
	}
	obligations := PrincipalSupportMemberObligations(plan)
	if len(obligations) == 0 {
		return nil
	}
	var out []AnswerSupportMemberObligation
	for _, ob := range obligations {
		if answerDocumentCoversSupportMember(doc, ob) {
			continue
		}
		out = append(out, ob)
	}
	return out
}

func answerDocumentCoversSupportMember(doc *AnswerDocumentV2, ob AnswerSupportMemberObligation) bool {
	if strings.TrimSpace(ob.Location) == "" {
		return false
	}
	for _, block := range doc.Blocks {
		if !answerBlockCanCarryPrincipalMember(block) {
			continue
		}
		for _, item := range block.Items {
			if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
				continue
			}
			if !citationCoversSupportMember(doc.Citations[item.CitationRef], ob) {
				continue
			}
			if len(ob.SurfaceTerms) == 0 || answerItemSurfaceMentionsSupportMember(item, ob) {
				return true
			}
		}
	}
	if answerDocumentCaveatsSupportMember(doc, ob) {
		return true
	}
	return false
}

func answerDocumentCaveatsSupportMember(doc *AnswerDocumentV2, ob AnswerSupportMemberObligation) bool {
	for _, block := range doc.Blocks {
		if block.Kind != BlockCaveat {
			continue
		}
		for _, item := range block.Items {
			if item.CitationRef < 0 || item.CitationRef >= len(doc.Citations) {
				continue
			}
			if !citationCoversSupportMember(doc.Citations[item.CitationRef], ob) {
				continue
			}
			if len(ob.SurfaceTerms) == 0 || answerItemSurfaceMentionsSupportMember(item, ob) {
				return true
			}
		}
	}
	return false
}

func answerBlockCanCarryPrincipalMember(block AnswerBlock) bool {
	switch block.Kind {
	case BlockOrderedList, BlockBulletList, BlockTable:
	default:
		return false
	}
	if block.SurfaceRole == SurfacePrincipal {
		return true
	}
	for _, facet := range block.FacetIDs {
		if strings.TrimSpace(facet) == string(FacetEnumerationItem) {
			return true
		}
	}
	for _, cu := range block.ClaimUses {
		if strings.TrimSpace(cu.FacetID) == string(FacetEnumerationItem) {
			return true
		}
	}
	// Enumeration answers frequently use the principal list/table
	// block before all annotations are filled in. Treat the block
	// shape itself as a carrier; other validators still enforce
	// missing facet/claim annotations.
	return true
}

func answerItemSurfaceMentionsSupportMember(item AnswerBlockItem, ob AnswerSupportMemberObligation) bool {
	surface := strings.TrimSpace(item.Label + "\n" + item.Text)
	if surface == "" {
		return false
	}
	for _, term := range ob.SurfaceTerms {
		if supportMemberTermAppears(term, surface) {
			return true
		}
	}
	if supportMemberTermAppears(ob.Label, surface) {
		return true
	}
	return false
}

func supportMemberTermAppears(term, surface string) bool {
	term = strings.TrimSpace(term)
	surface = strings.TrimSpace(surface)
	if term == "" || surface == "" {
		return false
	}
	if displaySurfaceAppears(term, surface) || CodeSurfaceAppearsAsToken(term, surface) {
		return true
	}
	if tail := NormalizedSurfaceSymbolTail(term); tail != "" && tail != term {
		return displaySurfaceAppears(tail, surface) || CodeSurfaceAppearsAsToken(tail, surface)
	}
	return false
}

func citationLocationKeyForSupportMember(cit Citation) string {
	file := strings.TrimSpace(cit.File)
	if file == "" || cit.Line <= 0 {
		return ""
	}
	return normalizeAnswerSupportLocation(fmt.Sprintf("%s:%d", file, cit.Line))
}

func citationCoversSupportMember(cit Citation, ob AnswerSupportMemberObligation) bool {
	key := citationLocationKeyForSupportMember(cit)
	if key == "" {
		return false
	}
	for _, location := range supportMemberCoverageLocations(ob) {
		if key == location {
			return true
		}
	}
	return citationWithinDefinitionSupportMemberWindow(cit, ob)
}

func citationWithinDefinitionSupportMemberWindow(cit Citation, ob AnswerSupportMemberObligation) bool {
	if ob.ClaimForm != ClaimDefinitionFact {
		return false
	}
	if strings.TrimSpace(ob.Source) == "" || ob.LineStart <= 0 {
		return false
	}
	if cit.Line <= 0 || normalizeAnswerSupportPath(cit.File) != normalizeAnswerSupportPath(ob.Source) {
		return false
	}
	return absInt(cit.Line-ob.LineStart) <= definitionSupportMemberCitationWindow
}

func supportMemberCoverageLocations(ob AnswerSupportMemberObligation) []string {
	var out []string
	add := func(location string) {
		location = normalizeAnswerSupportLocation(location)
		if location == "" {
			return
		}
		for _, existing := range out {
			if existing == location {
				return
			}
		}
		out = append(out, location)
	}
	add(ob.Location)
	for _, location := range ob.EquivalentLocations {
		add(location)
	}
	return out
}

func normalizeAnswerSupportLocation(location string) string {
	location = strings.TrimSpace(location)
	if location == "" {
		return ""
	}
	location = strings.ReplaceAll(location, `\`, `/`)
	return strings.ToLower(location)
}

func normalizeAnswerSupportPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = strings.ReplaceAll(path, `\`, `/`)
	return strings.ToLower(path)
}

func supportMemberSourceLine(source string, line int, location string) (string, int) {
	source = strings.TrimSpace(strings.ReplaceAll(source, `\`, `/`))
	if line > 0 && source != "" {
		return normalizeAnswerSupportPath(source), line
	}
	locSource, locLine := splitAnswerSupportLocation(location)
	if source == "" {
		source = locSource
	}
	if line <= 0 {
		line = locLine
	}
	return normalizeAnswerSupportPath(source), line
}

func splitAnswerSupportLocation(location string) (string, int) {
	location = strings.TrimSpace(strings.ReplaceAll(location, `\`, `/`))
	if location == "" {
		return "", 0
	}
	idx := strings.LastIndex(location, ":")
	if idx < 0 || idx == len(location)-1 {
		return normalizeAnswerSupportPath(location), 0
	}
	line, err := strconv.Atoi(location[idx+1:])
	if err != nil || line <= 0 {
		return normalizeAnswerSupportPath(location), 0
	}
	return normalizeAnswerSupportPath(location[:idx]), line
}

func normalizedSupportMemberIdentity(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	if tail := NormalizedSurfaceSymbolTail(label); tail != "" {
		return tail
	}
	return strings.ToLower(label)
}

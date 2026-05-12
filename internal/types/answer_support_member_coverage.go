package types

import (
	"fmt"
	"strings"
)

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
	seen := make(map[string]bool)
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
			key := strings.ToLower(ob.Location) + "\x00" + strings.ToLower(ob.Label)
			if key == "\x00" || seen[key] {
				continue
			}
			seen[key] = true
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
	ob := AnswerSupportMemberObligation{
		EvidenceID: strings.TrimSpace(entry.EvidenceID),
		Location:   normalizeAnswerSupportLocation(entry.Location),
		Label:      principalSupportMemberLabel(entry),
		ClaimForm:  entry.ClaimForm,
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
			if citationLocationKeyForSupportMember(doc.Citations[item.CitationRef]) != ob.Location {
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
			if citationLocationKeyForSupportMember(doc.Citations[item.CitationRef]) != ob.Location {
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

func normalizeAnswerSupportLocation(location string) string {
	location = strings.TrimSpace(location)
	if location == "" {
		return ""
	}
	location = strings.ReplaceAll(location, `\`, `/`)
	return strings.ToLower(location)
}

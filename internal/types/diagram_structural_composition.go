package types

import (
	"sort"
	"strings"
)

// DiagramOwnershipDeclaration is a parser- or checkout-owned field/property
// ownership fact. It authorizes visual containment only; it never creates a
// call, transfer, order, or runtime relation.
type DiagramOwnershipDeclaration struct {
	Owner  string
	Member string
	Type   string
	Source string
	Line   int
}

// DiagramNoArrowOwnershipGroup is the exact requested-participant projection
// of one ownership declaration.
type DiagramNoArrowOwnershipGroup struct {
	Owner  string
	Member string
	Type   string
	Source string
	Line   int
}

// DiagramOwnershipDeclarationsFromEvidence keeps declaration extraction in
// one package so the finalizer handoff and pre-emit validator cannot disagree
// about which typed rows authorize a no-arrow grouping.
func DiagramOwnershipDeclarationsFromEvidence(evidence []EvidenceItem) []DiagramOwnershipDeclaration {
	var out []DiagramOwnershipDeclaration
	for _, item := range evidence {
		if !item.IsCitable() || item.AnchorKind != AnchorDefinition ||
			strings.TrimSpace(item.DeclaredOwner) == "" || strings.TrimSpace(item.DeclaredBinding) == "" ||
			strings.TrimSpace(item.DeclaredType) == "" || RuntimeArtifactPathKind(item.Source) != "" {
			continue
		}
		binding := strings.TrimSpace(item.DeclaredBinding)
		member := binding
		if dot := strings.LastIndex(binding, "."); dot >= 0 && dot+1 < len(binding) {
			member = binding[dot+1:]
		}
		out = append(out, DiagramOwnershipDeclaration{
			Owner: strings.TrimSpace(item.DeclaredOwner), Member: member,
			Type: strings.TrimSpace(item.DeclaredType), Source: strings.TrimSpace(item.Source), Line: item.LineStart,
		})
	}
	return out
}

// RequestedDiagramNoArrowOwnershipGroups resolves declaration identities only
// against distinct incident_required participants from one typed flow request.
// No request prose, model prose, diagram label, or answer text participates.
func RequestedDiagramNoArrowOwnershipGroups(rm RequestModel, declarations []DiagramOwnershipDeclaration) []DiagramNoArrowOwnershipGroup {
	if rm.Intent == IntentTrace || ResolveQuestionFamily(rm) == QFRootCauseTrace ||
		rm.PredicateAxis != AxisFlow || rm.DiagramHint == nil || !rm.DiagramHint.Required {
		return nil
	}
	participants := make([][]string, 0, len(rm.DiagramHint.Participants))
	for _, participant := range rm.DiagramHint.Participants {
		if participant.Role != DiagramParticipantIncidentRequired {
			continue
		}
		surfaces := DiagramParticipantIdentitySurfaces(rm, participant)
		if identity := strings.TrimSpace(participant.Identity); identity != "" {
			surfaces = append([]string{identity}, surfaces...)
		}
		participants = append(participants, surfaces)
	}
	if len(participants) < 2 {
		return nil
	}
	matchesParticipant := func(identity string) []int {
		var matches []int
		for i, surfaces := range participants {
			for _, surface := range surfaces {
				if AnswerCodeIdentitySurfacesCompatible(surface, identity) {
					matches = append(matches, i)
					break
				}
			}
		}
		return matches
	}
	var rows []DiagramNoArrowOwnershipGroup
	seen := make(map[string]bool)
	for _, declaration := range declarations {
		row := DiagramNoArrowOwnershipGroup{
			Owner: strings.TrimSpace(declaration.Owner), Member: strings.TrimSpace(declaration.Member),
			Type: strings.TrimSpace(declaration.Type), Source: strings.TrimSpace(declaration.Source), Line: declaration.Line,
		}
		ownerMatches := matchesParticipant(row.Owner)
		memberMatches := matchesParticipant(row.Member)
		if len(memberMatches) != 1 && row.Type != "" {
			memberMatches = matchesParticipant(row.Type)
		}
		if row.Owner == "" || row.Member == "" || row.Source == "" || row.Line <= 0 ||
			len(ownerMatches) != 1 || len(memberMatches) != 1 || ownerMatches[0] == memberMatches[0] {
			continue
		}
		key := strings.ToLower(row.Owner + "\x00" + row.Member + "\x00" + row.Type)
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Source != rows[j].Source {
			return rows[i].Source < rows[j].Source
		}
		if rows[i].Line != rows[j].Line {
			return rows[i].Line < rows[j].Line
		}
		if rows[i].Owner != rows[j].Owner {
			return rows[i].Owner < rows[j].Owner
		}
		return rows[i].Member < rows[j].Member
	})
	return rows
}

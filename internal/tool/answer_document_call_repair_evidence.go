package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// diagramCallEdgeExactOrUniqueShortEvidence preserves the call gate's two
// original row-backed lanes. Exact matches have priority. Short identities
// are checked side-by-side against the COMPLETE citable pool before selecting
// rows; checking each row as a singleton would erase multi-owner ambiguity.
// Each returned row owns both endpoints and the direction. Other call-gate
// bridges remain separate and do not receive an inferred exact-row receipt.
func diagramCallEdgeExactOrUniqueShortEvidence(evidence []types.EvidenceItem, from, to string, collect bool) (bool, []types.EvidenceItem) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" || to == "" {
		return false, nil
	}
	var matched []types.EvidenceItem
	for _, ev := range evidence {
		if ev.IsCitable() && types.ClaimFormOf(ev) == types.ClaimCallEdge &&
			diagramCallEvidenceEndpointMatches(ev, ev.Subject, from) &&
			(diagramCallEvidenceEndpointMatches(ev, ev.Object, to) || diagramCallEvidenceEndpointMatches(ev, ev.AnchorSymbol, to)) {
			if !collect {
				return true, nil // Preserve the ordinary boolean gate's early return/allocation cost.
			}
			matched = append(matched, ev)
		}
	}
	if len(matched) > 0 {
		return true, matched
	}
	accept := func(ev types.EvidenceItem) bool { return types.ClaimFormOf(ev) == types.ClaimCallEdge }
	sources := func(ev types.EvidenceItem) []string { return []string{ev.Subject} }
	targets := func(ev types.EvidenceItem) []string { return []string{ev.Object, ev.AnchorSymbol} }
	if !diagramRelationEdgeHasExactOrUniqueShortProjection(evidence, from, to, accept, sources, targets) {
		return false, nil
	}
	if !collect {
		return true, nil
	}
	for _, ev := range evidence {
		if ev.IsCitable() && accept(ev) &&
			diagramRelationEndpointCandidateSetMatches(sources(ev), from) &&
			diagramRelationEndpointCandidateSetMatches(targets(ev), to) {
			matched = append(matched, ev)
		}
	}
	return len(matched) > 0, matched
}

// Strings are copied from one current citable row. Source includes its actual
// line range; neither an evidence id nor an endpoint alone can select a row.
type diagramCallRepairEvidence struct {
	evidenceID, source, from, to string
	unresolved                   bool
}

func diagramCallRepairEvidenceForMismatch(issue string, evidence []types.EvidenceItem, from, to string) []diagramCallRepairEvidence {
	if issue != diagramCallEdgeIssueMissingGroundedAnchor {
		return nil
	}
	var out []diagramCallRepairEvidence
	_, matched := diagramCallEdgeExactOrUniqueShortEvidence(evidence, from, to, true)
	for _, ev := range matched {
		resolved := false
		for _, candidate := range preEmitStandaloneRelationCandidatesFromEvidence(ev) {
			if candidate.relation != types.DiagramRelCall || candidate.evidenceID == "" ||
				strings.TrimSpace(ev.Source) == "" || ev.LineStart <= 0 || candidate.source == "" {
				continue
			}
			// Object and AnchorSymbol can be equivalent qualified/short names,
			// but an unrelated Object must not replace the AnchorSymbol that
			// actually matched the diagram. Preserve the old boolean outcome
			// without issuing a repair receipt for that contradictory row.
			if !(diagramCallEvidenceEndpointMatches(ev, candidate.from, from) || diagramRelationEndpointCandidateMatches(candidate.from, from)) ||
				!(diagramCallEvidenceEndpointMatches(ev, candidate.to, to) || diagramRelationEndpointCandidateMatches(candidate.to, to)) {
				continue
			}
			out = append(out, diagramCallRepairEvidence{evidenceID: candidate.evidenceID, source: candidate.source, from: candidate.from, to: candidate.to})
			resolved = true
		}
		if !resolved {
			// Do not erase a matched-but-unclassifiable row and then falsely
			// declare the remaining bounded candidates a unique identity pair.
			out = append(out, diagramCallRepairEvidence{unresolved: true})
		}
	}
	return out
}

func (m DiagramCallEdgeEvidenceMismatch) matchesCallRepairCandidate(candidate preEmitStandaloneRelationRepairCandidate) bool {
	if m.Issue != diagramCallEdgeIssueMissingGroundedAnchor || candidate.relation != types.DiagramRelCall {
		return false
	}
	for _, receipt := range m.matchedCallEvidence {
		if !receipt.unresolved && receipt.evidenceID == candidate.evidenceID && receipt.source == candidate.source &&
			receipt.from == candidate.from && receipt.to == candidate.to {
			return true
		}
	}
	return false
}

// Refine only the repair locator, never the diagram or diagnostic label.
// Uniqueness uses every matched row before the display/candidate budget is
// applied. Distinct call sites for one pair remain independent choices.
func (m DiagramCallEdgeEvidenceMismatch) uniqueCallRepairPair() (string, string, bool) {
	if m.Issue != diagramCallEdgeIssueMissingGroundedAnchor || len(m.matchedCallEvidence) == 0 {
		return "", "", false
	}
	first := m.matchedCallEvidence[0]
	if first.unresolved {
		return "", "", false
	}
	for _, receipt := range m.matchedCallEvidence[1:] {
		if receipt.unresolved || !types.AnswerCodeIdentitySurfacesEquivalent(first.from, receipt.from) ||
			!types.AnswerCodeIdentitySurfacesEquivalent(first.to, receipt.to) {
			return "", "", false
		}
	}
	return first.from, first.to, true
}

package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// renderAnswerDocFlowParticipantCoverageGuidance compares the analyzer's
// original typed participant shortlist with the current citable relation
// endpoints. This is intentionally a SOFT finalizer input: PrimaryEntities may
// include conceptual roles, so absence never rejects an answer or manufactures
// an edge. It gives the model a compact checklist for deciding which named
// participants remain independent/unproven without scanning request or answer
// prose. Language-native identity separators share one compatibility helper.
func renderAnswerDocFlowParticipantCoverageGuidance(b *strings.Builder, rm types.RequestModel, edges []answerDocMechanismRelationEdge) {
	if b == nil || rm.PredicateAxis != types.AxisFlow || rm.Intent == types.IntentTrace ||
		types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace {
		return
	}
	candidates := make([]string, 0, 8)
	seen := map[string]bool{}
	for _, raw := range rm.AnalyzerHints.PrimaryEntities {
		participant := strings.TrimSpace(raw)
		key := strings.ToLower(participant)
		if participant == "" || seen[key] || !types.IsCodeIdentitySurface(participant) ||
			types.HasCodeOrConfigPathSuffix(participant) {
			continue
		}
		seen[key] = true
		candidates = append(candidates, participant)
		if len(candidates) >= 8 {
			break
		}
	}
	if len(candidates) < 2 {
		return
	}
	incident := make([]string, 0, len(candidates))
	unproved := make([]string, 0, len(candidates))
	for _, participant := range candidates {
		covered := false
		for _, edge := range edges {
			if types.AnswerCodeIdentitySurfacesCompatible(participant, edge.from) ||
				types.AnswerCodeIdentitySurfacesCompatible(participant, edge.to) {
				covered = true
				break
			}
		}
		if covered {
			incident = append(incident, participant)
		} else {
			unproved = append(unproved, participant)
		}
	}
	fmt.Fprintf(b, "- soft_named_participant_relation_coverage: incident=%v; no_incident_typed_relation=%v.\n", incident, unproved)
	b.WriteString("- This participant checklist is soft exploration context, not completeness authority and not permission to add an edge. An incident match proves only that one listed typed relation touches the participant; it does not prove a complete path. Keep every no-incident participant independent/unproven, or inspect and emit its real operation site before connecting it.\n")
}

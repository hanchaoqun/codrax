package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// renderAnswerDocFlowParticipantCoverageGuidance compares the analyzer's
// optional schema-validated participant obligations (or the legacy broad
// PrimaryEntities fallback) with current citable relation endpoints. This is
// intentionally a SOFT finalizer input: the planning identity is not evidence,
// so absence never rejects an answer or manufactures an edge. It gives the
// model a compact checklist without scanning request or answer prose.
func renderAnswerDocFlowParticipantCoverageGuidance(b *strings.Builder, rm types.RequestModel, edges []answerDocMechanismRelationEdge, evidence []types.EvidenceItem) {
	if b == nil || rm.PredicateAxis != types.AxisFlow || rm.Intent == types.IntentTrace ||
		types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace {
		return
	}
	typed := rm.DiagramHint != nil && len(rm.DiagramHint.Participants) > 0
	candidates := make([]string, 0, 12)
	contextOnly := make([]string, 0, 4)
	seen := map[string]bool{}
	if typed {
		for _, obligation := range rm.DiagramHint.Participants {
			participant := strings.TrimSpace(obligation.Identity)
			key := strings.ToLower(participant)
			if participant == "" || seen[key] || !obligation.Role.IsValid() {
				continue
			}
			seen[key] = true
			if obligation.Role == types.DiagramParticipantContextOnly {
				contextOnly = append(contextOnly, participant)
				continue
			}
			candidates = append(candidates, participant)
		}
	} else {
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
	}
	if len(candidates) == 0 && len(contextOnly) == 0 {
		return
	}
	incident := make([]string, 0, len(candidates))
	unproved := make([]string, 0, len(candidates))
	for _, participant := range candidates {
		covered := false
		for _, edge := range edges {
			if types.AnswerCodeIdentitySurfacesCompatible(participant, edge.from) ||
				types.AnswerCodeIdentitySurfacesCompatible(participant, edge.to) ||
				types.AnswerCodeIdentityOwnsEndpoint(participant, edge.from) ||
				types.AnswerCodeIdentityOwnsEndpoint(participant, edge.to) ||
				types.AnswerCodeIdentityIncidentViaDeclaredBinding(participant, edge.from, edge.sourceItem, evidence) ||
				types.AnswerCodeIdentityIncidentViaDeclaredBinding(participant, edge.to, edge.sourceItem, evidence) {
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
	if typed {
		fmt.Fprintf(b, "- typed_named_participant_relation_coverage: incident=%v; no_incident_typed_relation=%v; context_only=%v.\n", incident, unproved, contextOnly)
		b.WriteString("- These schema-validated participant roles are planning/coverage guidance, not relation evidence and not permission to add an edge. For each `incident_required` participant with no incident typed relation, inspect and emit its real operation site or disclose that participant as an unproven boundary. Keep `context_only` participants outside the path unless independent evidence proves a relation.\n")
		b.WriteString("- A typed call/member endpoint may be incident to its exact receiver/owner participant (for example `ctx.Mutable.Reset` is incident to `Mutable`). Keep the model-authored participant as the visible business/component label while preserving the exact operation endpoint in `edge_anchors.from_identity/to_identity`; this owner projection changes no relation kind, direction, or evidence authority.\n")
		b.WriteString("- A parser-stamped static declaration may also align its declared type participant with the exact binding inside an operation endpoint (for example `state *RequestState` plus `handler.state.Items = value`). The declaration is identity-only; keep the operation's exact endpoint and do not turn the declaration into an edge. Untyped, ambiguous, or differently-owned bindings remain unproven.\n")
		return
	}
	fmt.Fprintf(b, "- soft_named_participant_relation_coverage: incident=%v; no_incident_typed_relation=%v.\n", incident, unproved)
	b.WriteString("- This participant checklist is soft exploration context, not completeness authority and not permission to add an edge. An incident match proves only that one listed typed relation touches the participant; it does not prove a complete path. Keep every no-incident participant independent/unproven, or inspect and emit its real operation site before connecting it.\n")
}

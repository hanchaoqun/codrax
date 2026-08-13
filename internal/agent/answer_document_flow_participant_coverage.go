package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type answerDocFlowParticipantCoverage struct {
	incident    []string
	unproved    []string
	contextOnly []string
}

// answerDocResolveFlowParticipantCoverage is the single participant-state
// projection shared by the Diagram Contract and the later current-source
// authority capsule. Both consumers must observe the same final evidence
// closure: publishing an early "uncovered" recipe beside a later "incident"
// receipt gives the answer model mutually exclusive instructions.
//
// This helper projects identity only. It cannot add an edge, change endpoint
// direction, or turn a participant obligation into relation evidence.
func answerDocResolveFlowParticipantCoverage(rm types.RequestModel, edges []answerDocMechanismRelationEdge, evidence []types.EvidenceItem) answerDocFlowParticipantCoverage {
	result := answerDocFlowParticipantCoverage{}
	typed := rm.DiagramHint != nil && len(rm.DiagramHint.Participants) > 0
	candidates := make([]string, 0, 12)
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
				result.contextOnly = append(result.contextOnly, participant)
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
	for _, participant := range candidates {
		covered := types.AnswerCodeParticipantHasFlowOperation(participant, evidence)
		for _, edge := range edges {
			if covered {
				break
			}
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
			result.incident = append(result.incident, participant)
		} else {
			result.unproved = append(result.unproved, participant)
		}
	}
	return result
}

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
	coverage := answerDocResolveFlowParticipantCoverage(rm, edges, evidence)
	if len(coverage.incident) == 0 && len(coverage.unproved) == 0 && len(coverage.contextOnly) == 0 {
		return
	}
	if typed {
		fmt.Fprintf(b, "- typed_named_participant_relation_coverage: incident=%v; no_incident_typed_relation=%v; context_only=%v.\n", coverage.incident, coverage.unproved, coverage.contextOnly)
		sourceOperationMissing, requestVisibleBoundary := answerDocPartitionUnprovedParticipants(rm, coverage.unproved)
		fmt.Fprintf(b, "- typed_participant_source_identity: source_operation_missing=%v; request_visible_boundary_only=%v.\n", sourceOperationMissing, requestVisibleBoundary)
		b.WriteString("- An `incident` row proves only that one local typed relation touches that participant. It does not prove that all requested participants form one connected flow, that a carrier transfers a value between those local relations, or that the requested relation is complete.\n")
		b.WriteString("- These schema-validated participant roles are planning/coverage guidance, not relation evidence and not permission to add an edge. Inspect a real operation only for each uniquely resolved `source_operation_missing` participant. A `request_visible_boundary_only` label has no unique typed source identity: keep it independent/unproven when useful, and do not search for or connect a homonymous operation merely for visual completeness. Keep `context_only` participants outside the path unless independent evidence proves a relation.\n")
		b.WriteString("- A typed call/member endpoint may be incident to its exact receiver/owner participant (for example `ctx.Mutable.Reset` is incident to `Mutable`). Keep the model-authored participant as the visible business/component label while preserving the exact operation endpoint in `edge_anchors.from_identity/to_identity`; this owner projection changes no relation kind, direction, or evidence authority.\n")
		b.WriteString("- A parser-stamped static declaration may also align its declared type participant with the exact binding inside an operation endpoint (for example `state *RequestState` plus `handler.state.Items = value`). The declaration is identity-only; keep the operation's exact endpoint and do not turn the declaration into an edge. Untyped, ambiguous, or differently-owned bindings remain unproven.\n")
		return
	}
	fmt.Fprintf(b, "- soft_named_participant_relation_coverage: incident=%v; no_incident_typed_relation=%v.\n", coverage.incident, coverage.unproved)
	b.WriteString("- This participant checklist is soft exploration context, not completeness authority and not permission to add an edge. An incident match proves only that one listed typed relation touches the participant; it does not prove a complete path. Keep every no-incident participant independent/unproven, or inspect and emit its real operation site before connecting it.\n")
}

func answerDocPartitionUnprovedParticipants(rm types.RequestModel, unproved []string) (sourceOperationMissing, requestVisibleBoundary []string) {
	if rm.DiagramHint == nil || len(unproved) == 0 {
		return nil, nil
	}
	byIdentity := make(map[string]types.DiagramParticipantHint, len(rm.DiagramHint.Participants))
	for _, participant := range rm.DiagramHint.Participants {
		key := strings.ToLower(strings.TrimSpace(participant.Identity))
		if key != "" {
			byIdentity[key] = participant
		}
	}
	for _, identity := range unproved {
		participant, ok := byIdentity[strings.ToLower(strings.TrimSpace(identity))]
		if ok && types.DiagramParticipantHasPreciseSourceOperationIdentity(rm, participant) {
			sourceOperationMissing = append(sourceOperationMissing, identity)
		} else {
			requestVisibleBoundary = append(requestVisibleBoundary, identity)
		}
	}
	return sourceOperationMissing, requestVisibleBoundary
}

package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

type answerDocFlowParticipantCoverage struct {
	incident                      []string
	localOnly                     []string
	unproved                      []string
	contextOnly                   []string
	requestScopedSubsetIncomplete bool
}

func (c answerDocFlowParticipantCoverage) boundaryParticipants() []string {
	result := make([]string, 0, len(c.localOnly)+len(c.unproved))
	result = append(result, c.localOnly...)
	result = append(result, c.unproved...)
	return result
}

func answerDocParticipantIncidentToMechanismEdge(participant string, edge answerDocMechanismRelationEdge, evidence []types.EvidenceItem) bool {
	return types.AnswerCodeIdentitySurfacesCompatible(participant, edge.from) ||
		types.AnswerCodeIdentitySurfacesCompatible(participant, edge.to) ||
		types.AnswerCodeIdentityOwnsEndpoint(participant, edge.from) ||
		types.AnswerCodeIdentityOwnsEndpoint(participant, edge.to) ||
		types.AnswerCodeIdentityIncidentViaDeclaredBinding(participant, edge.from, edge.sourceItem, evidence) ||
		types.AnswerCodeIdentityIncidentViaDeclaredBinding(participant, edge.to, edge.sourceItem, evidence)
}

// answerDocResolveFlowParticipantCoverage is the single participant-state
// projection shared by the Diagram Contract and the later current-source
// authority capsule. Both consumers must observe the same final evidence
// closure: publishing an early "uncovered" recipe beside a later "incident"
// receipt gives the answer model mutually exclusive instructions.
//
// This helper projects identity only. It cannot add an edge, change endpoint
// direction, or turn a participant obligation into relation evidence.
func answerDocResolveFlowParticipantCoverage(
	rm types.RequestModel,
	edges []answerDocMechanismRelationEdge,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
) answerDocFlowParticipantCoverage {
	result := answerDocFlowParticipantCoverage{}
	requestScopedRelations := 0
	requestSpineRelations := 0
	for _, edge := range edges {
		if edge.requestScoped {
			requestScopedRelations++
		}
		if edge.requestSpine {
			requestSpineRelations++
		}
	}
	typed := rm.DiagramHint != nil && len(rm.DiagramHint.Participants) > 0
	candidates := make([]string, 0, 12)
	typedParticipants := make([]types.DiagramParticipantHint, 0, 12)
	typedParticipantSurfaces := make([][]string, 0, 12)
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
			typedParticipants = append(typedParticipants, obligation)
			surfaces := []string{participant}
			for _, surface := range types.DiagramParticipantIdentitySurfaces(rm, obligation) {
				if strings.TrimSpace(surface) != "" {
					surfaces = append(surfaces, surface)
				}
			}
			typedParticipantSurfaces = append(typedParticipantSurfaces, surfaces)
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
	sharedCoverage := tool.FlowParticipantRelationCoverage{}
	if typed {
		sharedCoverage = tool.ResolveFlowParticipantRelationCoverage(
			rm, typedParticipants, typedParticipantSurfaces, evidence, stagePrecedence,
		)
		result.requestScopedSubsetIncomplete = sharedCoverage.RequestScopedSubsetIncomplete
	} else {
		result.requestScopedSubsetIncomplete = requestScopedRelations > 0 && requestSpineRelations == 0
	}
	for participantIndex, participant := range candidates {
		requestScopedCovered := false
		localCovered := types.AnswerCodeParticipantHasFlowOperation(participant, evidence)
		for _, edge := range edges {
			if !answerDocParticipantIncidentToMechanismEdge(participant, edge, evidence) {
				continue
			}
			if edge.requestScoped {
				requestScopedCovered = true
			} else {
				localCovered = true
			}
		}
		generalCovered := requestScopedCovered || localCovered
		if typed {
			requestScopedCovered = participantIndex < len(sharedCoverage.ParticipantRequestScopedCovered) &&
				sharedCoverage.ParticipantRequestScopedCovered[participantIndex]
			generalCovered = participantIndex < len(sharedCoverage.ParticipantCovered) &&
				sharedCoverage.ParticipantCovered[participantIndex]
			// A strict request-scoped provider subset must not inherit a broad
			// repository-wide operation match.  Local-only authoring is available
			// exactly when the shared typed scope can publish a concrete candidate;
			// otherwise keep the requested participant honestly unproved.
			localCovered = participantIndex < len(sharedCoverage.ParticipantLocalCandidateAvailable) &&
				sharedCoverage.ParticipantLocalCandidateAvailable[participantIndex]
		}
		if result.requestScopedSubsetIncomplete && requestScopedCovered {
			result.incident = append(result.incident, participant)
		} else if result.requestScopedSubsetIncomplete && localCovered {
			result.localOnly = append(result.localOnly, participant)
		} else if !result.requestScopedSubsetIncomplete && generalCovered {
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
// model a compact checklist without scanning request or answer prose. Despite
// the legacy helper name, typed diagram participants apply to every required
// source-relation diagram (call, flow, registration, etc.); only runtime
// root-cause trace diagrams keep their independent causal authority.
func renderAnswerDocFlowParticipantCoverageGuidance(
	b *strings.Builder,
	ctx *types.AgentContext,
	edges []answerDocMechanismRelationEdge,
	evidence []types.EvidenceItem,
) {
	if ctx == nil || ctx.AnalysisIR == nil {
		return
	}
	rm := ctx.AnalysisIR.RequestModel
	typedDiagramRequired := rm.DiagramHint != nil && rm.DiagramHint.Required
	if b == nil || types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace ||
		(!typedDiagramRequired && rm.PredicateAxis != types.AxisFlow) {
		return
	}
	typed := rm.DiagramHint != nil && len(rm.DiagramHint.Participants) > 0
	coverage := answerDocResolveFlowParticipantCoverage(
		rm, edges, evidence, answerDocVerifiedReadModeStagePrecedenceForRequest(ctx),
	)
	if len(coverage.incident) == 0 && len(coverage.localOnly) == 0 && len(coverage.unproved) == 0 && len(coverage.contextOnly) == 0 {
		return
	}
	if typed {
		if coverage.requestScopedSubsetIncomplete {
			fmt.Fprintf(b, "- typed_named_participant_relation_coverage: request_scoped_incident=%v; local_typed_incident_only=%v; no_incident_typed_relation=%v; context_only=%v.\n", coverage.incident, coverage.localOnly, coverage.unproved, coverage.contextOnly)
		} else {
			fmt.Fprintf(b, "- typed_named_participant_relation_coverage: incident=%v; no_incident_typed_relation=%v; context_only=%v.\n", coverage.incident, coverage.unproved, coverage.contextOnly)
		}
		sourceOperationMissing, requestVisibleBoundary := answerDocPartitionUnprovedParticipants(rm, coverage.unproved)
		fmt.Fprintf(b, "- typed_participant_source_identity: source_operation_missing=%v; request_visible_boundary_only=%v.\n", sourceOperationMissing, requestVisibleBoundary)
		if coverage.requestScopedSubsetIncomplete {
			b.WriteString("- `request_scoped_incident` is covered by the exact request-scoped typed provider. `local_typed_incident_only` has an exact local operation but does not close the requested participant graph, prove a carrier transfer, or remove that participant's `unproven` requested-relation boundary. `no_incident_typed_relation` has neither form of typed incidence.\n")
			renderAnswerDocLocalOnlyParticipantBindings(b, coverage.localOnly, edges, evidence)
			b.WriteString("- When a `local_typed_incident_only` fact is useful, preserve its exact technical endpoints and direction inside a visible group/subgraph named for the participant; do not retarget the operation directly to the abstract participant node. Keep the model-authored participant boundary visibly `unproven` for the requested relation. This is representation guidance only: the system supplies no visible node, edge, diagram, or conclusion.\n")
		} else {
			b.WriteString("- An `incident` row proves only that one local typed relation touches that participant. It does not prove that all requested participants form one connected flow, that a carrier transfers a value between those local relations, or that the requested relation is complete.\n")
		}
		b.WriteString("- These schema-validated participant roles are planning/coverage guidance, not relation evidence and not permission to add an edge. Inspect a real operation only for each uniquely resolved `source_operation_missing` participant. A `request_visible_boundary_only` label has no unique typed source identity: keep it independent/unproven when useful, and do not search for or connect a homonymous operation merely for visual completeness. Keep `context_only` participants outside the path unless independent evidence proves a relation.\n")
		b.WriteString("- A typed call/member endpoint may be incident to its exact receiver/owner participant (for example `ctx.Mutable.Reset` is incident to `Mutable`). Keep the model-authored participant as the visible business/component label while preserving the exact operation endpoint in `edge_anchors.from_identity/to_identity`; this owner projection changes no relation kind, direction, or evidence authority.\n")
		b.WriteString("- A parser-stamped static declaration may also align its declared type participant with the exact binding inside an operation endpoint (for example `state *RequestState` plus `handler.state.Items = value`). The declaration is identity-only; keep the operation's exact endpoint and do not turn the declaration into an edge. Untyped, ambiguous, or differently-owned bindings remain unproven.\n")
		candidateGuidance := tool.FlowParticipantTypedIncidentCandidateGuidance(
			rm, evidence, answerDocVerifiedReadModeStagePrecedenceForRequest(ctx), 2,
		)
		if candidateGuidance != "" {
			b.WriteString("- First-pass typed participant endpoint choices (bounded authoring input, not a system-authored graph): " + candidateGuidance + "\n")
			b.WriteString("- For every incident-required participant that has first-pass choices, select one relevant existing candidate when representing that participant's directed relation. One edge may satisfy two participants when its declared sides match both. In `diagram.body`, map only `participant_endpoint_side` to the exact `participant_node_id`; in the sibling edge anchor keep `edge_anchor_identity_fields` byte-for-byte and use `visible_arrow_label` as reader-facing relationship wording. Do not render every candidate, join candidates into a new path, or add a bridge. Participants with no candidate keep the separately published honest boundary. The model still authors every visible node, edge, label, diagram, and conclusion.\n")
			b.WriteString("- A candidate marked `candidate_scope=local_operation_only` proves only that local operation. It may be shown inside the participant's visible group, but it does not close or join the requested participant relation: keep the model-authored `unproven` participant boundary exactly as directed by `retain_participant_boundary=true`.\n")
		}
		return
	}
	fmt.Fprintf(b, "- soft_named_participant_relation_coverage: incident=%v; no_incident_typed_relation=%v.\n", coverage.incident, coverage.unproved)
	b.WriteString("- This participant checklist is soft exploration context, not completeness authority and not permission to add an edge. An incident match proves only that one listed typed relation touches the participant; it does not prove a complete path. Keep every no-incident participant independent/unproven, or inspect and emit its real operation site before connecting it.\n")
}

func renderAnswerDocLocalOnlyParticipantBindings(b *strings.Builder, participants []string, edges []answerDocMechanismRelationEdge, evidence []types.EvidenceItem) {
	if b == nil || len(participants) == 0 {
		return
	}
	for _, participant := range participants {
		emitted := 0
		total := 0
		for _, edge := range edges {
			if edge.requestScoped || !answerDocParticipantIncidentToMechanismEdge(participant, edge, evidence) {
				continue
			}
			total++
			if emitted >= 2 {
				continue
			}
			emitted++
			fmt.Fprintf(b, "- local_operation_binding[%s][%d]: relation_kind=`%s`; from_identity=`%s`; to_identity=`%s`; source=`%s`; representation=`exact_technical_endpoint_inside_participant_group`; requested_relation_closure=`unproven`; retain_participant_boundary=true.\n",
				participant, emitted, edge.relation, edge.from, edge.to, edge.loc)
		}
		if total > 0 {
			fmt.Fprintf(b, "- local_operation_binding_coverage[%s]: emitted=%d; total=%d; complete=%t.\n", participant, emitted, total, emitted == total)
		}
	}
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

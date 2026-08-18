package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// flowParticipantRelationScope is a typed, weak-connectivity view of the
// source operations available for a requested participant slate. It answers
// only whether existing parser-owned operations put multiple requested
// participants in the same relation component. It never chooses an edge,
// direction, label, or conclusion for the model.
//
// A local operation incident to one participant is intentionally not enough
// for a multi-participant request. This prevents unrelated member calls or
// returns on A and B from masquerading as evidence for the requested A/B
// relation. Multi-hop evidence remains valid: A -> helper -> B places both
// participants in one component without requiring a direct edge.
type flowParticipantRelationScope struct {
	participantCovered              []bool
	participantRequestScopedCovered []bool
	participantLocalOperation       []bool
	operationRelevant               []bool
	requestScopedSubsetIncomplete   bool
}

// FlowParticipantRelationCoverage is the package-boundary projection consumed
// by both exploration/validation and the finalizer's authoring context.  It
// carries coverage decisions only; operation relevance and candidate choice
// remain private to the tool package.
type FlowParticipantRelationCoverage struct {
	ParticipantCovered              []bool
	ParticipantRequestScopedCovered []bool
	RequestScopedSubsetIncomplete   bool
}

// ResolveFlowParticipantRelationCoverage exposes the one typed relation-scope
// computation to downstream prompt construction. Returning copies prevents a
// consumer from mutating validator authority.
func ResolveFlowParticipantRelationCoverage(
	rm types.RequestModel,
	participants []types.DiagramParticipantHint,
	participantSurfaces [][]string,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
) FlowParticipantRelationCoverage {
	scope := buildFlowParticipantRelationScope(rm, participants, participantSurfaces, evidence, stagePrecedence)
	return FlowParticipantRelationCoverage{
		ParticipantCovered:              append([]bool(nil), scope.participantCovered...),
		ParticipantRequestScopedCovered: append([]bool(nil), scope.participantRequestScopedCovered...),
		RequestScopedSubsetIncomplete:   scope.requestScopedSubsetIncomplete,
	}
}

// effectiveParticipantCovered keeps a request-scoped typed provider from
// being silently widened by disconnected local operations.  Before the model
// has authored one fully anchored requested-participant graph, a strict
// provider subset covers only the participants that provider itself names.
// Once the authored graph is genuinely complete, the ordinary typed
// component result is sufficient.  This is identity/coverage authority only;
// it never creates an edge or decides the requested relation for the model.
func (s flowParticipantRelationScope) effectiveParticipantCovered(participantIndex int, authoredRequestedGraphComplete bool) bool {
	if participantIndex < 0 || participantIndex >= len(s.participantCovered) {
		return false
	}
	if s.requestScopedSubsetIncomplete && !authoredRequestedGraphComplete {
		return participantIndex < len(s.participantRequestScopedCovered) &&
			s.participantRequestScopedCovered[participantIndex]
	}
	return s.participantCovered[participantIndex]
}

type flowParticipantRelationEdge struct {
	from      string
	to        string
	operation *types.EvidenceItem
	stageFrom *stageauthority.StageRow
	stageTo   *stageauthority.StageRow
	index     int
}

type flowParticipantRelationNode struct {
	endpoint string
	// A returned value is an output of one exact returning operation, not a
	// globally shared actor merely because another function returns the same
	// spelling (for example nil, false, an empty value, or a common type name).
	// Keep that sink local to its edge. The edge can still directly cover a
	// requested participant on either endpoint; it just cannot become an
	// evidence-free bridge between otherwise unrelated operations.
	returnSink bool
}

func buildFlowParticipantRelationScope(
	rm types.RequestModel,
	participants []types.DiagramParticipantHint,
	participantSurfaces [][]string,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
) flowParticipantRelationScope {
	scope := flowParticipantRelationScope{
		participantCovered:              make([]bool, len(participants)),
		participantRequestScopedCovered: make([]bool, len(participants)),
		participantLocalOperation:       make([]bool, len(participants)),
	}
	operations := types.FlowOperationEvidenceForRequest(evidence, rm)
	scope.operationRelevant = make([]bool, len(operations))
	if len(participants) == 0 {
		return scope
	}

	edges := make([]flowParticipantRelationEdge, 0, len(operations)+len(stagePrecedence))
	for i := range operations {
		operation := &operations[i]
		from, to := strings.TrimSpace(operation.Subject), strings.TrimSpace(operation.Object)
		if operation.AnchorKind == types.AnchorAssignment || operation.AnchorKind == types.AnchorInitializer {
			if receiver, value, ok := types.AssignmentEvidenceEndpoints(*operation); ok {
				from, to = value, receiver
			}
		}
		if from == "" || to == "" {
			continue
		}
		edges = append(edges, flowParticipantRelationEdge{
			from: from, to: to, operation: operation, index: i,
		})
	}
	for i := range stagePrecedence {
		relation := &stagePrecedence[i]
		from, to := strings.TrimSpace(relation.From.AgentValue), strings.TrimSpace(relation.To.AgentValue)
		if from == "" || to == "" {
			continue
		}
		edges = append(edges, flowParticipantRelationEdge{
			from: from, to: to, stageFrom: &relation.From, stageTo: &relation.To, index: -1,
		})
	}
	if len(edges) == 0 {
		return scope
	}

	// Endpoint equivalence is deliberately stricter than participant-to-endpoint
	// incidence. Complete typed identities may differ only by presentation
	// separators; short-tail compatibility is not allowed to join two otherwise
	// unrelated operation components.
	nodes := make([]flowParticipantRelationNode, 0, len(edges)*2)
	nodeFor := func(endpoint string, returnSink bool) int {
		if returnSink {
			nodes = append(nodes, flowParticipantRelationNode{endpoint: endpoint, returnSink: true})
			return len(nodes) - 1
		}
		for i, current := range nodes {
			if current.returnSink {
				continue
			}
			if types.AnswerCodeIdentitySurfacesEquivalent(current.endpoint, endpoint) ||
				strings.EqualFold(strings.TrimSpace(current.endpoint), strings.TrimSpace(endpoint)) {
				return i
			}
		}
		nodes = append(nodes, flowParticipantRelationNode{endpoint: endpoint})
		return len(nodes) - 1
	}
	parent := make([]int, 0, len(edges)*2)
	ensureParent := func() {
		for len(parent) < len(nodes) {
			parent = append(parent, len(parent))
		}
	}
	var find func(int) int
	find = func(v int) int {
		if parent[v] != v {
			parent[v] = find(parent[v])
		}
		return parent[v]
	}
	union := func(a, b int) {
		a, b = find(a), find(b)
		if a != b {
			parent[b] = a
		}
	}
	edgeNodes := make([][2]int, len(edges))
	for i, edge := range edges {
		returnSink := edge.operation != nil && edge.operation.AnchorKind == types.AnchorReturn
		from, to := nodeFor(edge.from, false), nodeFor(edge.to, returnSink)
		ensureParent()
		union(from, to)
		edgeNodes[i] = [2]int{from, to}
	}

	componentParticipants := make(map[int]map[int]bool)
	componentRequestScoped := make(map[int]bool)
	participantComponents := make([]map[int]bool, len(participants))
	for i := range participants {
		participantComponents[i] = make(map[int]bool)
	}
	for edgeIndex, edge := range edges {
		root := find(edgeNodes[edgeIndex][0])
		if edge.stageFrom != nil && edge.stageTo != nil {
			componentRequestScoped[root] = true
		}
		fromMatches := make([]int, 0, 1)
		toMatches := make([]int, 0, 1)
		for participantIndex, participant := range participants {
			if participant.Role != types.DiagramParticipantIncidentRequired ||
				participantIndex >= len(participantSurfaces) {
				continue
			}
			if edge.operation != nil {
				if diagramParticipantCandidateEndpointMatches(participantSurfaces[participantIndex], edge.from, *edge.operation, evidence) ||
					stageauthority.ParticipantMatchesStageEndpoint(rm, participant, edge.from, stagePrecedence) {
					fromMatches = append(fromMatches, participantIndex)
				}
				if diagramParticipantCandidateEndpointMatches(participantSurfaces[participantIndex], edge.to, *edge.operation, evidence) ||
					stageauthority.ParticipantMatchesStageEndpoint(rm, participant, edge.to, stagePrecedence) {
					toMatches = append(toMatches, participantIndex)
				}
			} else if edge.stageFrom != nil && edge.stageTo != nil {
				if stageauthority.ParticipantMatchesStageRow(rm, participant, *edge.stageFrom) {
					fromMatches = append(fromMatches, participantIndex)
				}
				if stageauthority.ParticipantMatchesStageRow(rm, participant, *edge.stageTo) {
					toMatches = append(toMatches, participantIndex)
				}
			}
		}
		// Ambiguous endpoint-to-participant matches fail closed. A short label
		// and a qualified sibling must not both acquire relation authority from
		// one endpoint merely because the presentation comparator can resolve
		// either spelling in isolation.
		incidentParticipants := make(map[int]bool)
		if len(fromMatches) == 1 {
			incidentParticipants[fromMatches[0]] = true
		}
		if len(toMatches) == 1 {
			incidentParticipants[toMatches[0]] = true
		}
		for participantIndex := range incidentParticipants {
			if edge.operation != nil {
				scope.participantLocalOperation[participantIndex] = true
			}
			if componentParticipants[root] == nil {
				componentParticipants[root] = make(map[int]bool)
			}
			componentParticipants[root][participantIndex] = true
			participantComponents[participantIndex][root] = true
		}
	}

	// A one-participant request asks only for an operation incident to that
	// participant, so preserve the established behavior. A multi-participant
	// request requires a component shared by at least two requested identities.
	minimumParticipants := 2
	if len(participants) == 1 {
		minimumParticipants = 1
	}
	// A uniquely resolved requested participant is itself a typed identity
	// bridge between its provider endpoint and its local technical endpoint.
	// Propagate request scope only through that exact alternating
	// component/participant graph. This admits a real carrier joined through a
	// verified stage participant, without promoting an unrelated local pair.
	for changed := true; changed; {
		changed = false
		for participantIndex, components := range participantComponents {
			if scope.participantRequestScopedCovered[participantIndex] {
				continue
			}
			for root := range components {
				if componentRequestScoped[root] {
					scope.participantRequestScopedCovered[participantIndex] = true
					changed = true
					break
				}
			}
		}
		for root, members := range componentParticipants {
			if componentRequestScoped[root] {
				continue
			}
			for participantIndex := range members {
				if scope.participantRequestScopedCovered[participantIndex] {
					componentRequestScoped[root] = true
					changed = true
					break
				}
			}
		}
	}
	for participantIndex, components := range participantComponents {
		for root := range components {
			if len(componentParticipants[root]) >= minimumParticipants {
				scope.participantCovered[participantIndex] = true
				// Verified stage precedence is currently the precise
				// request-scoped provider. Propagate that role through the same
				// exact typed component so a real carrier bridge may close the
				// request, while disconnected local pairs cannot inherit it.
				if componentRequestScoped[root] {
					scope.participantRequestScopedCovered[participantIndex] = true
				}
			}
		}
	}
	requestScopedCovered, incidentParticipants := 0, 0
	for i, participant := range participants {
		if participant.Role != types.DiagramParticipantIncidentRequired {
			continue
		}
		incidentParticipants++
		if i < len(scope.participantRequestScopedCovered) && scope.participantRequestScopedCovered[i] {
			requestScopedCovered++
		}
	}
	scope.requestScopedSubsetIncomplete = requestScopedCovered > 0 &&
		requestScopedCovered < incidentParticipants
	for edgeIndex, edge := range edges {
		if edge.operation == nil || edge.index < 0 || edge.index >= len(scope.operationRelevant) {
			continue
		}
		if len(componentParticipants[find(edgeNodes[edgeIndex][0])]) >= minimumParticipants {
			scope.operationRelevant[edge.index] = true
		}
	}
	return scope
}

// flowMissingParticipantsHaveLocalOperations distinguishes a relation-only
// deficit from an operation-discovery deficit. It is true only when every
// currently missing, uniquely resolved participant already owns at least one
// citable principal-source operation, while the caller has separately proved
// that those operations do not share a requested relation component.
//
// This signal may shorten or redirect SOFT repair navigation. It never closes
// participant coverage and never authorizes an answer edge.
func flowMissingParticipantsHaveLocalOperations(
	ctx *types.BusContext,
	evidence []types.EvidenceItem,
	missing []string,
) bool {
	if ctx == nil || ctx.AnalysisIR == nil || len(missing) < 2 {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	wanted := make(map[string]bool, len(missing))
	for _, identity := range missing {
		if key := strings.ToLower(strings.TrimSpace(identity)); key != "" {
			wanted[key] = true
		}
	}
	participants := make([]types.DiagramParticipantHint, 0, len(wanted))
	participantSurfaces := make([][]string, 0, len(wanted))
	seen := make(map[string]bool, len(wanted))
	if rm.DiagramHint == nil {
		return false
	}
	for _, participant := range rm.DiagramHint.Participants {
		key := strings.ToLower(strings.TrimSpace(participant.Identity))
		if !wanted[key] || seen[key] || participant.Role != types.DiagramParticipantIncidentRequired {
			continue
		}
		seen[key] = true
		resolved := flowResolveParticipantIdentity(ctx, rm, participant)
		if len(resolved.surfaces) == 0 {
			return false
		}
		participants = append(participants, participant)
		participantSurfaces = append(participantSurfaces, resolved.surfaces)
	}
	if len(participants) != len(wanted) {
		return false
	}
	scope := buildFlowParticipantRelationScope(rm, participants, participantSurfaces, evidence, nil)
	for i := range participants {
		if i >= len(scope.participantLocalOperation) || !scope.participantLocalOperation[i] {
			return false
		}
	}
	return true
}

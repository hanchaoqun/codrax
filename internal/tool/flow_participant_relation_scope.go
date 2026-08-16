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
	participantCovered        []bool
	participantLocalOperation []bool
	operationRelevant         []bool
}

type flowParticipantRelationEdge struct {
	from      string
	to        string
	operation *types.EvidenceItem
	stageFrom *stageauthority.StageRow
	stageTo   *stageauthority.StageRow
	index     int
}

func buildFlowParticipantRelationScope(
	rm types.RequestModel,
	participants []types.DiagramParticipantHint,
	participantSurfaces [][]string,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
) flowParticipantRelationScope {
	scope := flowParticipantRelationScope{
		participantCovered:        make([]bool, len(participants)),
		participantLocalOperation: make([]bool, len(participants)),
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
	nodes := make([]string, 0, len(edges)*2)
	nodeFor := func(endpoint string) int {
		for i, current := range nodes {
			if types.AnswerCodeIdentitySurfacesEquivalent(current, endpoint) ||
				strings.EqualFold(strings.TrimSpace(current), strings.TrimSpace(endpoint)) {
				return i
			}
		}
		nodes = append(nodes, endpoint)
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
		from, to := nodeFor(edge.from), nodeFor(edge.to)
		ensureParent()
		union(from, to)
		edgeNodes[i] = [2]int{from, to}
	}

	componentParticipants := make(map[int]map[int]bool)
	participantComponents := make([]map[int]bool, len(participants))
	for i := range participants {
		participantComponents[i] = make(map[int]bool)
	}
	for edgeIndex, edge := range edges {
		root := find(edgeNodes[edgeIndex][0])
		fromMatches := make([]int, 0, 1)
		toMatches := make([]int, 0, 1)
		for participantIndex, participant := range participants {
			if participant.Role != types.DiagramParticipantIncidentRequired ||
				participantIndex >= len(participantSurfaces) {
				continue
			}
			if edge.operation != nil {
				if diagramParticipantCandidateEndpointMatches(participantSurfaces[participantIndex], edge.from, *edge.operation, evidence) {
					fromMatches = append(fromMatches, participantIndex)
				}
				if diagramParticipantCandidateEndpointMatches(participantSurfaces[participantIndex], edge.to, *edge.operation, evidence) {
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
	for participantIndex, components := range participantComponents {
		for root := range components {
			if len(componentParticipants[root]) >= minimumParticipants {
				scope.participantCovered[participantIndex] = true
				break
			}
		}
	}
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

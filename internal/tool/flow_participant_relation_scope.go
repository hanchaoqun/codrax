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
	// participantPrincipalComponentCovered is completion-navigation
	// authority only. It identifies the largest typed connected group of
	// requested participants after ordinary per-participant coverage succeeds.
	// Final answer validation deliberately keeps participantCovered plus
	// explicit unproven boundaries, so an honest disconnected diagram can
	// still converge without the system inventing a bridge.
	participantPrincipalComponentCovered []bool
	participantLocalOperation            []bool
	operationRelevant                    []bool
	requestScopedSubsetIncomplete        bool
	requestScopedRelationComplete        bool
}

// FlowParticipantRelationCoverage is the package-boundary projection consumed
// by both exploration/validation and the finalizer's authoring context.  It
// carries coverage decisions only; operation relevance and candidate choice
// remain private to the tool package.
type FlowParticipantRelationCoverage struct {
	ParticipantCovered              []bool
	ParticipantRequestScopedCovered []bool
	// ParticipantLocalCandidateAvailable is true only when the same
	// request-scoped typed operation pool used by diagram authoring can publish
	// at least one exact local-only candidate for that participant.  A broader
	// repository evidence row may remain useful context, but it must not make a
	// downstream prompt promise an operation that the candidate carrier cannot
	// actually expose.
	ParticipantLocalCandidateAvailable []bool
	RequestScopedSubsetIncomplete      bool
	RequestScopedRelationComplete      bool
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
	localCandidates := make([]bool, len(participants))
	if scope.requestScopedSubsetIncomplete {
		for i, participant := range participants {
			if i >= len(scope.participantLocalOperation) || !scope.participantLocalOperation[i] ||
				scope.effectiveParticipantCovered(i) {
				continue
			}
			candidates := diagramParticipantTypedIncidentCandidateValuesWithScope(
				rm, participant, evidence, stagePrecedence, 1,
				participants, participantSurfaces, i, scope,
			)
			localCandidates[i] = len(candidates) > 0 && candidates[0].localOnly
		}
	}
	return FlowParticipantRelationCoverage{
		ParticipantCovered:                 append([]bool(nil), scope.participantCovered...),
		ParticipantRequestScopedCovered:    append([]bool(nil), scope.participantRequestScopedCovered...),
		ParticipantLocalCandidateAvailable: localCandidates,
		RequestScopedSubsetIncomplete:      scope.requestScopedSubsetIncomplete,
		RequestScopedRelationComplete:      scope.requestScopedRelationComplete,
	}
}

// effectiveParticipantCovered keeps a request-scoped typed provider from
// being silently widened by local operations or by the model-authored visible
// graph. A strict parser-owned provider subset covers only the participants
// that provider itself names. The visible graph may faithfully present typed
// local facts, but it cannot promote those facts into request-scope authority;
// doing so makes the same local-only candidate alternately require and reject
// an unproven boundary across repair rounds. This is identity/coverage
// authority only; it never creates an edge or decides the requested relation
// for the model.
func (s flowParticipantRelationScope) effectiveParticipantCovered(participantIndex int) bool {
	if participantIndex < 0 || participantIndex >= len(s.participantCovered) {
		return false
	}
	if s.requestScopedSubsetIncomplete {
		return participantIndex < len(s.participantRequestScopedCovered) &&
			s.participantRequestScopedCovered[participantIndex]
	}
	return s.participantCovered[participantIndex]
}

// completionParticipantConnectedCovered is stricter than ordinary incident
// coverage only for the completion navigator. A required multi-participant
// flow is not fully investigated merely because every participant belongs to
// some two-party island. The largest stable typed component remains the
// explored frontier and participants outside it receive one bounded bridge
// pass. This method never creates relation evidence and is not consumed by the
// final answer validator.
func (s flowParticipantRelationScope) completionParticipantConnectedCovered(participantIndex int) bool {
	return participantIndex >= 0 && participantIndex < len(s.participantPrincipalComponentCovered) &&
		s.participantPrincipalComponentCovered[participantIndex]
}

type flowParticipantRelationEdge struct {
	from         string
	to           string
	fromCallable bool
	toCallable   bool
	operation    *types.EvidenceItem
	stageFrom    *stageauthority.StageRow
	stageTo      *stageauthority.StageRow
	index        int
}

type flowParticipantRelationNode struct {
	endpoint string
	callable bool
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
		participantCovered:                   make([]bool, len(participants)),
		participantRequestScopedCovered:      make([]bool, len(participants)),
		participantPrincipalComponentCovered: make([]bool, len(participants)),
		participantLocalOperation:            make([]bool, len(participants)),
	}
	// Requested-relation coverage is principal answer-path authority. Ordinary
	// flow operations stay on the rows the Explorer explicitly selected for
	// this investigation. Exact request-scoped parser type relations share the
	// same lane because the regular diagram edge gate already treats those rows
	// as hard authority; excluding them here creates an impossible contract in
	// which one validator accepts a model-authored relation and participant
	// coverage simultaneously requires an unproven boundary for that relation.
	operations := diagramRequestedRelationEvidenceForRequest(evidence, rm)
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
		fromCallable, toCallable := flowRelationOperationCallableEndpoints(*operation)
		edges = append(edges, flowParticipantRelationEdge{
			from: from, to: to, fromCallable: fromCallable, toCallable: toCallable,
			operation: operation, index: i,
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
	callableTailAuthority := flowRelationUniqueQualifiedCallableTails(edges)

	// Endpoint equivalence is deliberately stricter than participant-to-endpoint
	// incidence. Complete typed identities may differ by presentation separators.
	// A qualified/bare callable tail may also join only under the graph-local
	// single-owner authority above; general short-tail compatibility remains
	// forbidden for values, fields, ambiguous callables, and unrelated components.
	nodes := make([]flowParticipantRelationNode, 0, len(edges)*2)
	nodeFor := func(endpoint string, returnSink, callable bool) int {
		if returnSink {
			nodes = append(nodes, flowParticipantRelationNode{endpoint: endpoint, callable: callable, returnSink: true})
			return len(nodes) - 1
		}
		for i, current := range nodes {
			if current.returnSink {
				continue
			}
			if types.AnswerCodeIdentitySurfacesEquivalent(current.endpoint, endpoint) ||
				strings.EqualFold(strings.TrimSpace(current.endpoint), strings.TrimSpace(endpoint)) ||
				(current.callable && callable && flowRelationUniqueCallableTailEquivalent(
					current.endpoint, endpoint, callableTailAuthority)) {
				nodes[i].callable = current.callable || callable
				return i
			}
		}
		nodes = append(nodes, flowParticipantRelationNode{endpoint: endpoint, callable: callable})
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
		from, to := nodeFor(edge.from, false, edge.fromCallable), nodeFor(edge.to, returnSink, edge.toCallable)
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
		var fromMatches, toMatches []int
		if edge.operation != nil {
			fromMatches, toMatches = flowResolvedOperationParticipantEndpoints(
				rm, participants, participantSurfaces, edge.from, edge.to, *edge.operation, evidence, stagePrecedence,
			)
		} else if edge.stageFrom != nil && edge.stageTo != nil {
			for participantIndex, participant := range participants {
				if participant.Role != types.DiagramParticipantIncidentRequired || participantIndex >= len(participantSurfaces) {
					continue
				}
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
	// Build a second, participant-level connectivity view. A uniquely matched
	// participant may legitimately bridge multiple technical endpoint
	// components, so union participant identities that occur in the same typed
	// relation component. Keep only the largest group as the stable explored
	// frontier; ties resolve to the earliest request participant. The caller
	// uses this only after ordinary coverage is complete.
	participantParent := make([]int, len(participants))
	for i := range participantParent {
		participantParent[i] = i
	}
	var participantFind func(int) int
	participantFind = func(v int) int {
		if participantParent[v] != v {
			participantParent[v] = participantFind(participantParent[v])
		}
		return participantParent[v]
	}
	participantUnion := func(a, b int) {
		a, b = participantFind(a), participantFind(b)
		if a != b {
			participantParent[b] = a
		}
	}
	for _, members := range componentParticipants {
		if len(members) < minimumParticipants {
			continue
		}
		first := -1
		for participantIndex := range members {
			if participantIndex < 0 || participantIndex >= len(scope.participantCovered) ||
				!scope.participantCovered[participantIndex] {
				continue
			}
			if first < 0 {
				first = participantIndex
				continue
			}
			participantUnion(first, participantIndex)
		}
	}
	type participantGroup struct {
		members  []int
		earliest int
	}
	groups := make(map[int]*participantGroup)
	for i, participant := range participants {
		if participant.Role != types.DiagramParticipantIncidentRequired ||
			i >= len(scope.participantCovered) || !scope.participantCovered[i] {
			continue
		}
		root := participantFind(i)
		group := groups[root]
		if group == nil {
			group = &participantGroup{earliest: i}
			groups[root] = group
		}
		group.members = append(group.members, i)
		if i < group.earliest {
			group.earliest = i
		}
	}
	var principal *participantGroup
	for _, group := range groups {
		if principal == nil || len(group.members) > len(principal.members) ||
			(len(group.members) == len(principal.members) && group.earliest < principal.earliest) {
			principal = group
		}
	}
	if principal != nil {
		for _, participantIndex := range principal.members {
			scope.participantPrincipalComponentCovered[participantIndex] = true
		}
	}
	scope.requestScopedRelationComplete = incidentParticipants >= 2 && requestScopedCovered == incidentParticipants
	if scope.requestScopedRelationComplete {
		for i, participant := range participants {
			if participant.Role != types.DiagramParticipantIncidentRequired {
				continue
			}
			if !scope.completionParticipantConnectedCovered(i) {
				scope.requestScopedRelationComplete = false
				break
			}
		}
	}
	// A request-scoped provider that names every participant but leaves them in
	// separate typed components is still partial. Counting names alone made an
	// honest disconnected answer look complete and suppressed the model-owned
	// whole-diagram scope disclosure. Conversely, one connected typed graph is
	// complete even when its exact operation endpoints need participant carrier
	// nodes for reader-facing composition.
	scope.requestScopedSubsetIncomplete = requestScopedCovered > 0 && !scope.requestScopedRelationComplete
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

// flowResolvedOperationParticipantEndpoints resolves endpoint incidence at
// the whole-edge level. A qualified member endpoint may precisely carry two
// identities at once: its declared receiver/container type and its field/value
// type. Resolving each side independently treated that legitimate overlap as
// ambiguity and discarded relations such as `bus.Mutable -> Mutable` even
// when the opposite endpoint uniquely selected Mutable. We now admit only the
// unique distinct pairing across both endpoints. A lone ambiguous endpoint
// (for example Foo versus pkg.Foo with no opposite typed participant) remains
// fail-closed. No prose, labels, or model output participates.
func flowResolvedOperationParticipantEndpoints(
	rm types.RequestModel,
	participants []types.DiagramParticipantHint,
	participantSurfaces [][]string,
	from, to string,
	operation types.EvidenceItem,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
) ([]int, []int) {
	fromCandidates := flowOperationEndpointParticipantCandidates(
		rm, participants, participantSurfaces, from, operation, evidence, stagePrecedence,
	)
	toCandidates := flowOperationEndpointParticipantCandidates(
		rm, participants, participantSurfaces, to, operation, evidence, stagePrecedence,
	)
	if len(participants) <= 1 {
		return uniqueParticipantEndpointCandidate(fromCandidates), uniqueParticipantEndpointCandidate(toCandidates)
	}
	fromResolved := uniqueParticipantEndpointCandidate(fromCandidates)
	toResolved := uniqueParticipantEndpointCandidate(toCandidates)
	if len(fromResolved) == 0 && len(fromCandidates) > 1 && len(toResolved) == 1 {
		fromResolved = uniqueParticipantEndpointCandidateExcluding(fromCandidates, toResolved[0])
	}
	if len(toResolved) == 0 && len(toCandidates) > 1 && len(fromResolved) == 1 {
		toResolved = uniqueParticipantEndpointCandidateExcluding(toCandidates, fromResolved[0])
	}
	if len(fromResolved) == 0 && len(toResolved) == 0 && len(fromCandidates) > 1 && len(toCandidates) > 1 {
		type pair struct{ from, to int }
		var pairs []pair
		for _, fromIndex := range fromCandidates {
			for _, toIndex := range toCandidates {
				if fromIndex != toIndex {
					pairs = append(pairs, pair{from: fromIndex, to: toIndex})
				}
			}
		}
		if len(pairs) == 1 {
			fromResolved, toResolved = []int{pairs[0].from}, []int{pairs[0].to}
		}
	}
	return fromResolved, toResolved
}

func flowOperationEndpointParticipantCandidates(
	rm types.RequestModel,
	participants []types.DiagramParticipantHint,
	participantSurfaces [][]string,
	endpoint string,
	operation types.EvidenceItem,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
) []int {
	var out []int
	for i, participant := range participants {
		if participant.Role != types.DiagramParticipantIncidentRequired || i >= len(participantSurfaces) {
			continue
		}
		if diagramParticipantCandidateEndpointMatches(participantSurfaces[i], endpoint, operation, evidence) ||
			stageauthority.ParticipantMatchesStageEndpoint(rm, participant, endpoint, stagePrecedence) {
			out = append(out, i)
		}
	}
	return out
}

func uniqueParticipantEndpointCandidate(candidates []int) []int {
	if len(candidates) == 1 {
		return []int{candidates[0]}
	}
	return nil
}

func uniqueParticipantEndpointCandidateExcluding(candidates []int, excluded int) []int {
	selected := -1
	for _, candidate := range candidates {
		if candidate == excluded {
			continue
		}
		if selected >= 0 && selected != candidate {
			return nil
		}
		selected = candidate
	}
	if selected < 0 {
		return nil
	}
	return []int{selected}
}

// diagramRequestedRelationEvidenceForRequest is the single typed evidence
// lane shared by participant coverage, component checks, and repair guidance.
// It does not inspect request prose or diagram text and never creates an edge.
//
// General operations remain Explorer-selected. The only deterministic rows
// admitted in addition are exact, citable repo-map type relationships whose
// typed relation family and target are selected by the current RequestModel.
// Name-only/heuristic rows, unrelated targets, and unsupported relation kinds
// remain unable to satisfy a hard gate.
func diagramRequestedRelationEvidenceForRequest(evidence []types.EvidenceItem, rm types.RequestModel) []types.EvidenceItem {
	out := append([]types.EvidenceItem(nil), types.ExplorerAuthoredFlowOperationEvidenceForRequest(evidence, rm)...)
	query := types.BuildTypedRelationQuery(rm, types.TypedRelationPurposeCoverageGate, 0)
	if len(query.Kinds) == 0 || len(query.Sources) == 0 {
		return out
	}
	seen := make(map[string]bool, len(out)+len(evidence))
	for _, item := range out {
		seen[diagramRequestedRelationEvidenceKey(item)] = true
	}
	for _, item := range evidence {
		kind, ok := diagramRepoMapTypeRelationKind(item)
		if !ok || !item.IsCitable() || !query.AllowsKind(kind) ||
			!diagramTypedRelationTargetMatchesQuery(item.Object, query.Sources) {
			continue
		}
		key := diagramRequestedRelationEvidenceKey(item)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func diagramRepoMapTypeRelationKind(item types.EvidenceItem) (types.TypedRelationKind, bool) {
	if !types.IsRepoMapTypeRelationEvidence(item) {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(item.Predicate)) {
	case "implements", "implementation":
		return types.TypedRelationImplements, true
	case "inheritance", "extends", "embedding":
		return types.TypedRelationExtends, true
	case "overrides":
		return types.TypedRelationOverrides, true
	default:
		return "", false
	}
}

func diagramTypedRelationTargetMatchesQuery(target string, sources []string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, source := range sources {
		if strings.EqualFold(target, strings.TrimSpace(source)) ||
			types.AnswerCodeIdentitySurfacesEquivalent(target, source) {
			return true
		}
	}
	return false
}

func diagramRequestedRelationEvidenceKey(item types.EvidenceItem) string {
	if id := strings.TrimSpace(item.ID); id != "" {
		return "id\x00" + id
	}
	return strings.ToLower(strings.Join([]string{
		strings.TrimSpace(item.Producer),
		strings.TrimSpace(item.Subject),
		strings.TrimSpace(item.Predicate),
		strings.TrimSpace(item.Object),
		strings.TrimSpace(item.Source),
	}, "\x00"))
}

func diagramRequestedEvidenceRelation(item types.EvidenceItem) types.DiagramRelationKind {
	if _, ok := diagramRepoMapTypeRelationKind(item); ok {
		return types.DiagramRelTypeRelation
	}
	return types.RelationForClaimForm(types.ClaimFormOf(item))
}

func flowRelationOperationCallableEndpoints(operation types.EvidenceItem) (bool, bool) {
	switch operation.AnchorKind {
	case types.AnchorCall, types.AnchorCallback:
		return true, true
	case types.AnchorArgument:
		// The complete source argument is a value. The receiving API is a
		// callable. Keeping those roles distinct prevents a field/value tail
		// from being joined merely because it happens to share a method name.
		return false, true
	case types.AnchorReturn:
		return true, false
	default:
		return false, false
	}
}

// flowRelationUniqueQualifiedCallableTails records graph-local authority for
// joining one qualified callable spelling (ctxbuilder.BuildAgentContext) with
// its bare spelling (BuildAgentContext). Parser providers legitimately expose
// those two surfaces at a caller boundary and inside the callee body. The tail
// is usable only when exactly one qualified callable owns it in this evidence
// graph; two packages with the same method/function tail remain ambiguous and
// fail closed.
func flowRelationUniqueQualifiedCallableTails(edges []flowParticipantRelationEdge) map[string]string {
	qualified := make(map[string]map[string]bool)
	add := func(endpoint string, callable bool) {
		if !callable {
			return
		}
		full, tail, isQualified, ok := flowRelationCallableIdentity(endpoint)
		if !ok || !isQualified {
			return
		}
		if qualified[tail] == nil {
			qualified[tail] = make(map[string]bool)
		}
		qualified[tail][full] = true
	}
	for _, edge := range edges {
		add(edge.from, edge.fromCallable)
		add(edge.to, edge.toCallable)
	}
	out := make(map[string]string)
	for tail, owners := range qualified {
		if len(owners) != 1 {
			continue
		}
		for owner := range owners {
			out[tail] = owner
		}
	}
	return out
}

func flowRelationUniqueCallableTailEquivalent(left, right string, authority map[string]string) bool {
	leftFull, leftTail, leftQualified, leftOK := flowRelationCallableIdentity(left)
	rightFull, rightTail, rightQualified, rightOK := flowRelationCallableIdentity(right)
	if !leftOK || !rightOK || leftTail != rightTail || leftQualified == rightQualified {
		return false
	}
	qualifiedFull := leftFull
	if rightQualified {
		qualifiedFull = rightFull
	}
	return authority[leftTail] != "" && authority[leftTail] == qualifiedFull
}

func flowRelationCallableIdentity(raw string) (full, tail string, qualified, ok bool) {
	raw = strings.Trim(strings.TrimSpace(raw), "`'\"")
	raw = strings.TrimSuffix(raw, "()")
	if raw == "" || strings.ContainsAny(raw, "\n\r\t ") {
		return "", "", false, false
	}
	raw = strings.NewReplacer(
		"::", ".",
		"->", ".",
		"#", ".",
		"/", ".",
		`\`, ".",
	).Replace(raw)
	parts := strings.Split(raw, ".")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.Trim(strings.TrimSpace(part), "*&( )"))
		if part == "" || !types.IsCodeIdentitySurface(part) {
			return "", "", false, false
		}
		segments = append(segments, part)
	}
	if len(segments) == 0 {
		return "", "", false, false
	}
	return strings.Join(segments, "."), segments[len(segments)-1], len(segments) > 1, true
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

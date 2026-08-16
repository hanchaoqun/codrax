package tool

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/types"
)

type DiagramParticipantCoverageIssue string

const (
	DiagramParticipantCoverageMissingBoundary   DiagramParticipantCoverageIssue = "missing_unproven_boundary"
	DiagramParticipantCoverageStaleBoundary     DiagramParticipantCoverageIssue = "stale_boundary_for_connected_participant"
	DiagramParticipantCoverageNodeMissing       DiagramParticipantCoverageIssue = "boundary_participant_not_visible"
	DiagramParticipantCoverageUnknownBoundary   DiagramParticipantCoverageIssue = "unknown_or_context_only_boundary"
	DiagramParticipantCoverageDuplicate         DiagramParticipantCoverageIssue = "duplicate_unproven_boundary"
	DiagramParticipantCoverageTypedEdgeMissing  DiagramParticipantCoverageIssue = "available_typed_incident_edge_not_rendered"
	DiagramParticipantCoverageIdentityMissing   DiagramParticipantCoverageIssue = "required_participant_identity_not_visible"
	DiagramParticipantCoverageBoundaryConnected DiagramParticipantCoverageIssue = "unproven_boundary_has_visible_incident_edge"
)

// DiagramParticipantCoverageMismatch is derived only from the analyzer's
// typed participant slate, model-authored boundary rows, parsed Mermaid
// identities/edges, and typed relation anchors. No request or answer prose is
// read as authority.
type DiagramParticipantCoverageMismatch struct {
	BlockID     string
	Participant string
	Issue       DiagramParticipantCoverageIssue
}

func preCheckDiagramParticipantCoverage(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, pctx *preEmitCheckContext) []emitFixHint {
	if pctx == nil || pctx.ctx == nil || pctx.ctx.AnalysisIR == nil {
		return nil
	}
	evidence := pctx.evidenceItems()
	if view != nil && view.RelationAxis == types.AxisFlow {
		if pctx.groundCtx == nil {
			pctx.groundCtx = ground.BuildContext(pctx.ctx)
		}
		evidence = preEmitEvidenceWithGroundedDiagramPrecedence(doc, view, evidence, pctx.groundCtx)
	}
	mismatches := DiagramParticipantCoverageMismatches(
		doc, view, pctx.ctx.AnalysisIR.RequestModel, evidence,
		diagramVerifiedReadModeStagePrecedence(pctx.ctx, view)...,
	)
	if len(mismatches) == 0 {
		return nil
	}
	parts := make([]string, 0, len(mismatches))
	for _, mismatch := range mismatches {
		parts = append(parts, "participant="+strings.TrimSpace(mismatch.Participant)+" issue="+string(mismatch.Issue))
	}
	candidates := diagramParticipantCoverageCandidateGuidance(
		pctx.ctx.AnalysisIR.RequestModel,
		mismatches,
		evidence,
		diagramVerifiedReadModeStagePrecedence(pctx.ctx, view),
	)
	actions := diagramParticipantCoverageRepairActions(mismatches)
	endpointConflicts := diagramParticipantEndpointConflictGuidance(
		doc,
		pctx.ctx.AnalysisIR.RequestModel,
		mismatches,
	)
	// This precheck runs only after the document has already parsed into the
	// block-level ParticipantBoundaries field. Do not prepend a generic JSON
	// placement warning here: doing so falsely tells a model with valid JSON to
	// move a correctly placed field and hides the actual typed coverage issue.
	// Schema/decoder failures own malformed-placement teaching; this lane names
	// only the participant mismatch it actually observed.
	expected := "Typed participant coverage mismatch: " + strings.Join(parts, "; ") + ". For every typed incident_required participant with an available request-scoped evidence-backed relation, render one matching typed visible incident edge authored from that relation; do not replace that request-scoped evidence with an unproven boundary. A local operation that merely touches a participant is an independent fact: when the typed request-scope authority says the complete requested relation is unproved, that local operation may coexist with the participant's unproven requested-relation boundary and does not eliminate it. The requested participant identity must itself remain present as the exact Mermaid endpoint node id or as a visible node/subgraph/group label; an internal operation endpoint that is merely owned by or statically bound to that participant does not replace the business/component identity. A stable exact participant node id may carry a concise business-facing visible label when the proved edge terminates on that same node id and edge_anchors preserve the exact technical endpoint identities. The candidate map publishes participant_endpoint_side=from|to|from_or_to. Reuse the selected candidate as one edge and set only that declared side's Mermaid node id to the exact participant identity; keep the candidate's technical from_identity/to_identity unchanged. Do not draw the technical method as a separate endpoint and then add an unanchored bridge edge to the participant. Alternatively, place the exact technical endpoint inside that participant's visible group. When the requested directed relation is unproved, retain exactly one {participant:<typed identity>,status:\"unproven\"} row. That participant must not be a visible directed-edge endpoint, but independently proved local technical facts or no-arrow containment/grouping may coexist. For a bounded participant, make the exact typed identity the Mermaid node id or the first visible node label, including a visible subgraph/group label; a short node id whose typed identity appears only in a secondary parenthetical or later multiline label is not that primary identity. Omit participant_boundaries or emit [] when all required participants have the requested typed incidence. Remove stale, unknown, context_only, or already-covered boundary rows. The system does not create or choose an edge."
	if endpointConflicts != "" {
		expected = "Resolve these exact endpoint-ID/label collisions first. Preserve each existing visible edge, canonical from_identity/to_identity, relation_kind, and direction. Choose one fresh non-participant Mermaid node ID for the technical endpoint; change only that side of the visible body edge and the matching edge_anchor from_node/to_node field. If that endpoint's visible label also equals the unproven participant, relabel the same technical node with concise technical wording derived from the already-published exact from_identity/to_identity; do not alter those anchor identities. Keep the exact participant separate from that directed edge and retain its unproven boundary; an independently grounded, already-authored no-arrow subgraph/grouping remains valid and must not be flattened or removed. The system is identifying an already model-authored unique edge/anchor pair and its parsed visible label, not creating or selecting a relation or choosing replacement wording: " + endpointConflicts + ". " + expected
	}
	if actions != "" {
		expected += ". Typed repair actions (apply only the row for each failed participant; these actions preserve model ownership of the visible diagram): " + actions
	}
	if candidates != "" {
		expected += ". Existing typed candidate map (choose one relevant candidate per failed participant; this list is not a requirement to render every candidate and does not create an edge). Each candidate's edge_anchor_identity_fields are the exact schema fields to copy alongside model-authored from_node/to_node IDs; never replace those exact identities with the broader participant name: " + candidates
	}
	return []emitFixHint{{
		Field:               "blocks[kind=diagram].participant_boundaries AND blocks[kind=diagram].diagram.body",
		HardSignal:          preEmitHardSignalTypedDiagramParticipantCoverage,
		OffendingBlockKinds: []types.AnswerBlockKind{types.BlockDiagram},
		ExpectedShape:       expected,
		Reason:              "the typed participant slate is precise completeness authority for participant presence, but never relation evidence; an explicit typed boundary preserves honesty when exploration did not prove an incident relation.",
	}}
}

// diagramParticipantEndpointConflictGuidance publishes the exact already
// model-authored edge/anchor tuple behind a boundary-connected mismatch. It
// never chooses a replacement node ID or creates a relation. Guidance is
// emitted only when one body edge and one exact typed anchor uniquely own the
// conflicting participant endpoint; ambiguous pairs retain the generic
// fail-open repair text.
func diagramParticipantEndpointConflictGuidance(
	doc *types.AnswerDocumentV2,
	rm types.RequestModel,
	mismatches []DiagramParticipantCoverageMismatch,
) string {
	if doc == nil || rm.DiagramHint == nil || len(mismatches) == 0 {
		return ""
	}
	participantSurfaces := make(map[string][]string)
	for _, participant := range rm.DiagramHint.Participants {
		identity := strings.TrimSpace(participant.Identity)
		if identity == "" || participant.Role != types.DiagramParticipantIncidentRequired {
			continue
		}
		surfaces := []string{identity}
		for _, resolved := range types.DiagramParticipantIdentitySurfaces(rm, participant) {
			if !diagramParticipantSurfaceListContainsExact(surfaces, resolved) {
				surfaces = append(surfaces, resolved)
			}
		}
		participantSurfaces[strings.ToLower(identity)] = surfaces
	}
	blockCounts := diagramEvidenceBodyEdgeBlockCounts(doc)
	rows := make([]string, 0)
	seen := make(map[string]bool)
	for _, mismatch := range mismatches {
		if mismatch.Issue != DiagramParticipantCoverageBoundaryConnected {
			continue
		}
		participant := strings.TrimSpace(mismatch.Participant)
		surfaces := participantSurfaces[strings.ToLower(participant)]
		if participant == "" || len(surfaces) == 0 {
			continue
		}
		type endpointConflictCandidate struct {
			blockID              string
			fromNode             string
			toNode               string
			side                 string
			conflictSurface      string
			visibleLabelConflict bool
			anchor               types.DiagramEdgeAnchor
		}
		var candidates []endpointConflictCandidate
		for blockIndex := range doc.Blocks {
			block := &doc.Blocks[blockIndex]
			if block.Kind != types.BlockDiagram || block.Diagram == nil ||
				(mismatch.BlockID != "" && block.ID != mismatch.BlockID) {
				continue
			}
			labels := diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind)
			anchors := diagramEvidenceEffectiveAnchorsForBlock(doc, blockIndex, blockCounts)
			for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
				fromConflict, fromSurface, fromLabelConflict := diagramParticipantEndpointConflictSurface(surfaces, edge.From, labels)
				toConflict, toSurface, toLabelConflict := diagramParticipantEndpointConflictSurface(surfaces, edge.To, labels)
				if fromConflict == toConflict { // neither side, or both sides: not unique
					continue
				}
				var exact []types.DiagramEdgeAnchor
				for _, anchor := range anchors {
					if diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode) != diagramEvidenceEdgeKey(edge.From, edge.To) ||
						strings.TrimSpace(anchor.FromIdentity) == "" || strings.TrimSpace(anchor.ToIdentity) == "" ||
						!anchor.RelationKind.IsValid() {
						continue
					}
					exact = append(exact, anchor)
				}
				if len(exact) != 1 {
					continue
				}
				side := "to"
				conflictSurface := toSurface
				visibleLabelConflict := toLabelConflict
				if fromConflict {
					side = "from"
					conflictSurface = fromSurface
					visibleLabelConflict = fromLabelConflict
				}
				candidates = append(candidates, endpointConflictCandidate{
					blockID: block.ID, fromNode: edge.From, toNode: edge.To, side: side,
					conflictSurface: conflictSurface, visibleLabelConflict: visibleLabelConflict, anchor: exact[0],
				})
			}
		}
		if len(candidates) != 1 {
			continue
		}
		selected := candidates[0]
		key := strings.ToLower(participant + "\x00" + selected.blockID + "\x00" + selected.fromNode + "\x00" + selected.toNode)
		if seen[key] {
			continue
		}
		seen[key] = true
		nodeFields := "body.to_node+edge_anchor.to_node"
		if selected.side == "from" {
			nodeFields = "body.from_node+edge_anchor.from_node"
		}
		rows = append(rows, fmt.Sprintf(
			"typed_endpoint_collision[%s]={block_id:%s,body_edge:{from_node:%s,to_node:%s},conflict_endpoint_side:%s,conflict_visible_surface:%s,visible_label_collision:%t,node_fields_to_change:%s,edge_anchor_identity_fields:{from_identity:%s,to_identity:%s,relation_kind:%s}}",
			strconv.Quote(participant), strconv.Quote(selected.blockID),
			strconv.Quote(selected.fromNode), strconv.Quote(selected.toNode), strconv.Quote(selected.side),
			strconv.Quote(selected.conflictSurface), selected.visibleLabelConflict, strconv.Quote(nodeFields), strconv.Quote(selected.anchor.FromIdentity),
			strconv.Quote(selected.anchor.ToIdentity), strconv.Quote(string(selected.anchor.RelationKind)),
		))
	}
	return strings.Join(rows, "; ")
}

// diagramParticipantEndpointConflictSurface uses the same two precise Mermaid
// identity lanes as the boundary-connected gate: stable node ID and parsed
// primary visible label. The former implementation inspected only node IDs,
// so a common `n6["Mutable"]` shape was rejected by the label-aware gate but
// could not receive its unique endpoint-collision repair tuple. This helper
// changes guidance only; it creates no edge, alias, label, or relation.
func diagramParticipantEndpointConflictSurface(
	surfaces []string,
	node string,
	labels map[string]string,
) (matched bool, conflictSurface string, visibleLabelConflict bool) {
	node = strings.TrimSpace(node)
	visible := diagramParticipantVisibleEndpoint(node, labels)
	nodeMatch := diagramParticipantSurfaceListContainsExact(surfaces, node)
	labelMatch := diagramParticipantSurfaceListContainsExact(surfaces, visible)
	switch {
	case labelMatch:
		return true, visible, true
	case nodeMatch:
		return true, node, false
	default:
		return false, "", false
	}
}

// diagramParticipantCoverageRepairActions translates each precise typed
// mismatch into one bounded structural action.  It deliberately does not
// choose a candidate, node id, label, layout, or edge: those remain model
// authored.  Separating edge authority, visible participant identity, and
// boundary state prevents a repair from fixing one lane by corrupting another
// (for example retargeting o.busCtx.PipelineStage to BusContext).
func diagramParticipantCoverageRepairActions(mismatches []DiagramParticipantCoverageMismatch) string {
	if len(mismatches) == 0 {
		return ""
	}
	rows := make([]string, 0, len(mismatches))
	seen := make(map[string]bool, len(mismatches))
	for _, mismatch := range mismatches {
		participant := strings.TrimSpace(mismatch.Participant)
		if participant == "" {
			continue
		}
		key := strings.ToLower(participant) + "\x00" + string(mismatch.Issue)
		if seen[key] {
			continue
		}
		seen[key] = true
		var edgeAction, identityAction, boundaryAction string
		switch mismatch.Issue {
		case DiagramParticipantCoverageTypedEdgeMissing:
			edgeAction = "reuse_one_existing_typed_candidate_as_one_edge_and_map_only_its_declared_participant_endpoint_side_to_the_exact_participant_node_id_without_adding_a_bridge_edge"
			identityAction = "use_the_exact_participant_as_that_edge_endpoint_node_id_with_a_business_label_or_group_the_exact_technical_endpoint_inside_it"
			boundaryAction = "omit_unproven_boundary"
		case DiagramParticipantCoverageIdentityMissing:
			edgeAction = "retain_an_already_rendered_valid_candidate_or_select_one_existing_typed_candidate"
			identityAction = "add_only_the_missing_visible_participant_label_or_group_without_retargeting_canonical_endpoints"
			boundaryAction = "omit_unproven_boundary"
		case DiagramParticipantCoverageStaleBoundary:
			edgeAction = "retain_existing_typed_incident_edge"
			identityAction = "retain_existing_visible_participant_identity"
			boundaryAction = "remove_stale_boundary"
		case DiagramParticipantCoverageMissingBoundary:
			edgeAction = "none_for_missing_requested_relation_keep_independent_typed_local_facts_if_any"
			identityAction = "ensure_exact_visible_participant_without_directed_incident_edge_and_preserve_any_existing_grounded_no_arrow_grouping"
			boundaryAction = "add_exactly_one_unproven_boundary"
		case DiagramParticipantCoverageNodeMissing:
			edgeAction = "none_for_missing_requested_relation_keep_independent_typed_local_facts_if_any"
			identityAction = "ensure_exact_visible_participant_without_directed_incident_edge_and_preserve_any_existing_grounded_no_arrow_grouping"
			boundaryAction = "retain_exactly_one_unproven_boundary"
		case DiagramParticipantCoverageDuplicate:
			edgeAction = "none"
			identityAction = "retain_exact_visible_participant_without_directed_incident_edge_and_preserve_any_existing_grounded_no_arrow_grouping"
			boundaryAction = "deduplicate_to_exactly_one_unproven_boundary"
		case DiagramParticipantCoverageBoundaryConnected:
			edgeAction = "move_existing_typed_edge_to_its_exact_technical_endpoint_and_keep_participant_out_of_that_directed_edge"
			identityAction = "retain_exact_visible_participant_and_preserve_any_existing_grounded_no_arrow_grouping"
			boundaryAction = "retain_exactly_one_unproven_boundary"
		case DiagramParticipantCoverageUnknownBoundary:
			edgeAction = "none"
			identityAction = "none"
			boundaryAction = "remove_unknown_or_context_only_boundary"
		default:
			continue
		}
		rows = append(rows, fmt.Sprintf(
			"repair_action[%s]={issue:%s,edge_action:%s,identity_action:%s,boundary_action:%s}",
			strconv.Quote(participant), strconv.Quote(string(mismatch.Issue)), strconv.Quote(edgeAction),
			strconv.Quote(identityAction), strconv.Quote(boundaryAction),
		))
	}
	return strings.Join(rows, "; ")
}

func DiagramParticipantCoverageMismatches(
	doc *types.AnswerDocumentV2,
	view *types.AnswerSemanticView,
	rm types.RequestModel,
	evidence []types.EvidenceItem,
	stagePrecedence ...stageauthority.PrecedenceRelation,
) []DiagramParticipantCoverageMismatch {
	if doc == nil || view == nil || view.DiagramPlan == nil || !view.DiagramPlan.Required ||
		types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace {
		return nil
	}
	obligations := diagramIncidentParticipantObligations(view)
	if len(obligations) == 0 || !answerDocumentContainsDiagramPayload(doc) {
		return nil
	}

	type state struct {
		obligation                   types.DiagramParticipantHint
		surfaces                     []string
		visibleCovered               bool
		identityVisible              bool
		typedEdgeAvailable           bool
		localEvidenceBoundaryAllowed bool
		bounded                      bool
	}
	states := make([]state, 0, len(obligations))
	allSurfaces := make([][]string, 0, len(obligations))
	for _, obligation := range obligations {
		// The analyzer's typed display identity is always its own exact
		// presentation surface, including schema-valid labels with spaces such
		// as "Analyzer Agent". Resolved code identities are additional aliases;
		// they must not make the original typed identity impossible to name.
		surfaces := []string{strings.TrimSpace(obligation.Identity)}
		for _, resolved := range types.DiagramParticipantIdentitySurfaces(rm, obligation) {
			if !diagramParticipantSurfaceListContainsExact(surfaces, resolved) {
				surfaces = append(surfaces, resolved)
			}
		}
		allSurfaces = append(allSurfaces, surfaces)
		states = append(states, state{
			obligation:      obligation,
			surfaces:        surfaces,
			visibleCovered:  diagramParticipantHasTypedVisibleIncident(doc, surfaces, evidence, stagePrecedence),
			identityVisible: diagramParticipantIdentityVisible(doc, surfaces, stagePrecedence),
		})
	}
	relationScope := buildFlowParticipantRelationScope(rm, obligations, allSurfaces, evidence, stagePrecedence)
	requestedParticipantGraphComplete := diagramParticipantRequestedGraphConnected(doc, allSurfaces, evidence)
	for i := range states {
		requestScopedEdgeAvailable := i < len(relationScope.participantCovered) && relationScope.participantCovered[i]
		states[i].typedEdgeAvailable = requestScopedEdgeAvailable
		states[i].localEvidenceBoundaryAllowed = len(obligations) > 1 &&
			!requestedParticipantGraphComplete && !requestScopedEdgeAvailable
	}

	var out []DiagramParticipantCoverageMismatch
	for _, block := range doc.Blocks {
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		for _, boundary := range block.ParticipantBoundaries {
			if boundary.Status != types.DiagramParticipantBoundaryUnproven {
				out = append(out, DiagramParticipantCoverageMismatch{
					BlockID: block.ID, Participant: strings.TrimSpace(boundary.Participant),
					Issue: DiagramParticipantCoverageUnknownBoundary,
				})
				continue
			}
			// Exact typed identity has precedence over short/qualified alias
			// compatibility. Without this two obligations such as Foo and
			// pkg.Foo can make the exact boundary "pkg.Foo" ambiguous; decorated
			// identities can be rejected as unknown and simultaneously reported
			// missing. Alias matching remains the fail-closed fallback only when
			// no exact typed identity exists.
			matches := make([]int, 0, 1)
			for i := range states {
				if strings.EqualFold(
					strings.TrimSpace(states[i].obligation.Identity),
					strings.TrimSpace(boundary.Participant),
				) {
					matches = append(matches, i)
				}
			}
			if len(matches) == 0 {
				for i := range states {
					if diagramParticipantSurfaceMatches(states[i].surfaces, boundary.Participant) {
						matches = append(matches, i)
					}
				}
			}
			if len(matches) != 1 {
				out = append(out, DiagramParticipantCoverageMismatch{
					BlockID: block.ID, Participant: strings.TrimSpace(boundary.Participant),
					Issue: DiagramParticipantCoverageUnknownBoundary,
				})
				continue
			}
			idx := matches[0]
			if states[idx].bounded {
				out = append(out, DiagramParticipantCoverageMismatch{
					BlockID: block.ID, Participant: states[idx].obligation.Identity,
					Issue: DiagramParticipantCoverageDuplicate,
				})
				continue
			}
			if states[idx].visibleCovered && states[idx].identityVisible && !states[idx].localEvidenceBoundaryAllowed {
				out = append(out, DiagramParticipantCoverageMismatch{
					BlockID: block.ID, Participant: states[idx].obligation.Identity,
					Issue: DiagramParticipantCoverageStaleBoundary,
				})
				continue
			}
			if states[idx].typedEdgeAvailable && !states[idx].localEvidenceBoundaryAllowed {
				issue := DiagramParticipantCoverageTypedEdgeMissing
				if !states[idx].identityVisible {
					issue = DiagramParticipantCoverageIdentityMissing
				}
				out = append(out, DiagramParticipantCoverageMismatch{
					BlockID: block.ID, Participant: states[idx].obligation.Identity,
					Issue: issue,
				})
				states[idx].bounded = true
				continue
			}
			states[idx].bounded = true
			if diagramParticipantDocumentHasVisibleIncidentEdge(doc, states[idx].surfaces) {
				out = append(out, DiagramParticipantCoverageMismatch{
					BlockID: block.ID, Participant: states[idx].obligation.Identity,
					Issue: DiagramParticipantCoverageBoundaryConnected,
				})
				continue
			}
			if !diagramParticipantBlockHasVisibleNode(block, states[idx].surfaces) &&
				!diagramParticipantBlockHasVisibleSubgraph(block, states[idx].surfaces) {
				out = append(out, DiagramParticipantCoverageMismatch{
					BlockID: block.ID, Participant: states[idx].obligation.Identity,
					Issue: DiagramParticipantCoverageNodeMissing,
				})
			}
		}
	}
	for _, current := range states {
		if current.bounded || (current.visibleCovered && current.identityVisible && !current.localEvidenceBoundaryAllowed) {
			continue
		}
		issue := DiagramParticipantCoverageMissingBoundary
		if current.typedEdgeAvailable && !current.localEvidenceBoundaryAllowed {
			issue = DiagramParticipantCoverageTypedEdgeMissing
			if !current.identityVisible {
				issue = DiagramParticipantCoverageIdentityMissing
			}
		}
		out = append(out, DiagramParticipantCoverageMismatch{
			Participant: current.obligation.Identity,
			Issue:       issue,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].BlockID != out[j].BlockID {
			return out[i].BlockID < out[j].BlockID
		}
		if out[i].Participant != out[j].Participant {
			return out[i].Participant < out[j].Participant
		}
		return out[i].Issue < out[j].Issue
	})
	return out
}

// diagramParticipantRequestedGraphConnected checks only model-authored visible
// edges that already carry a valid typed anchor. Each endpoint must map to one
// unique requested participant through the same exact node/visible-group rules
// used by participant coverage. Local operations confined to one participant
// therefore cannot masquerade as a complete requested flow, while a real
// typed cross-participant graph may close the boundary without a special case.
// Direction and relation kind remain owned by the existing edge validators;
// this helper only establishes weak connectivity across the requested roster.
func diagramParticipantRequestedGraphConnected(
	doc *types.AnswerDocumentV2,
	participantSurfaces [][]string,
	evidence []types.EvidenceItem,
) bool {
	if doc == nil || len(participantSurfaces) < 2 {
		return false
	}
	parent := make([]int, len(participantSurfaces))
	for i := range parent {
		parent[i] = i
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

	blockCounts := diagramEvidenceBodyEdgeBlockCounts(doc)
	for blockIndex, block := range doc.Blocks {
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		labels := diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind)
		anchors := diagramEvidenceEffectiveAnchorsForBlock(doc, blockIndex, blockCounts)
		typedRelations := diagramTypedAnchorRelationSet(anchors)
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			if !diagramHasValidTypedRelation(typedRelations[diagramEvidenceEdgeKey(edge.From, edge.To)]) {
				continue
			}
			from, to := diagramEvidenceEdgeEndpointSymbols(edge.From, edge.To, anchors, labels, evidence)
			fromParticipants := diagramParticipantEndpointRosterMatches(block, edge.From, from, labels, participantSurfaces, evidence)
			toParticipants := diagramParticipantEndpointRosterMatches(block, edge.To, to, labels, participantSurfaces, evidence)
			if len(fromParticipants) == 1 && len(toParticipants) == 1 && fromParticipants[0] != toParticipants[0] {
				union(fromParticipants[0], toParticipants[0])
			}
		}
	}
	root := find(0)
	for i := 1; i < len(parent); i++ {
		if find(i) != root {
			return false
		}
	}
	return true
}

func diagramParticipantEndpointRosterMatches(
	block types.AnswerBlock,
	node, typedEndpoint string,
	labels map[string]string,
	participantSurfaces [][]string,
	evidence []types.EvidenceItem,
) []int {
	matches := make([]int, 0, 1)
	for i, surfaces := range participantSurfaces {
		if diagramParticipantIncidentEndpointMatches(surfaces, typedEndpoint, evidence) &&
			diagramParticipantVisibleEndpointCarriesIdentity(block, surfaces, node, typedEndpoint, labels) {
			matches = append(matches, i)
		}
	}
	return matches
}

// diagramParticipantIdentityVisible is deliberately independent from typed
// relation incidence. A parser-owned operation may prove that an internal
// endpoint belongs to BusContext/MutableState, but that identity bridge cannot
// make the user-requested component disappear from the visual layer.
func diagramParticipantIdentityVisible(
	doc *types.AnswerDocumentV2,
	surfaces []string,
	stagePrecedence []stageauthority.PrecedenceRelation,
) bool {
	if doc == nil {
		return false
	}
	blockCounts := diagramEvidenceBodyEdgeBlockCounts(doc)
	for blockIndex, block := range doc.Blocks {
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		if diagramParticipantBlockHasVisibleNode(block, surfaces) ||
			diagramParticipantBlockHasVisibleSubgraph(block, surfaces) {
			return true
		}
		labels := diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind)
		anchors := diagramEvidenceEffectiveAnchorsForBlock(doc, blockIndex, blockCounts)
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			fromIdentity, toIdentity, present, conflict := diagramEvidenceEdgeIdentityPair(edge.From, edge.To, anchors)
			if present && !conflict &&
				(diagramParticipantSurfaceListContainsExact(surfaces, fromIdentity) ||
					diagramParticipantSurfaceListContainsExact(surfaces, toIdentity)) {
				// An explicit endpoint identity is model-authored metadata for
				// this visible edge. Unlike an inferred owner/static binding, it
				// is sufficient to bind a business label to the typed identity.
				return true
			}
			if diagramParticipantHasVerifiedStageIncident(
				surfaces,
				diagramParticipantVisibleEndpoint(edge.From, labels),
				diagramParticipantVisibleEndpoint(edge.To, labels),
				stagePrecedence,
			) {
				return true
			}
		}
	}
	return false
}

func diagramParticipantBlockHasVisibleSubgraph(block types.AnswerBlock, surfaces []string) bool {
	if block.Kind != types.BlockDiagram || block.Diagram == nil {
		return false
	}
	for _, line := range strings.Split(block.Diagram.Body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(trimmed), "subgraph ") {
			continue
		}
		rest := strings.TrimSpace(trimmed[len("subgraph "):])
		if rest == "" {
			continue
		}
		candidates := []string{rest}
		if idx := strings.IndexAny(rest, "[\""); idx > 0 {
			candidates = append(candidates, strings.TrimSpace(rest[:idx]))
		}
		if open := strings.IndexAny(rest, "[\""); open >= 0 {
			label := strings.TrimSpace(rest[open:])
			label = strings.Trim(label, "[](){}\"' ")
			if label != "" {
				candidates = append(candidates, label)
			}
		}
		for _, candidate := range candidates {
			if diagramParticipantSurfaceMatches(surfaces, candidate) {
				return true
			}
		}
	}
	return false
}

func diagramParticipantCoverageCandidateGuidance(
	rm types.RequestModel,
	mismatches []DiagramParticipantCoverageMismatch,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
) string {
	if rm.DiagramHint == nil || len(mismatches) == 0 {
		return ""
	}
	failed := make(map[string]bool)
	for _, mismatch := range mismatches {
		if mismatch.Issue == DiagramParticipantCoverageTypedEdgeMissing ||
			mismatch.Issue == DiagramParticipantCoverageIdentityMissing {
			failed[strings.ToLower(strings.TrimSpace(mismatch.Participant))] = true
		}
	}
	if len(failed) == 0 {
		return ""
	}
	rows := make([]string, 0, len(failed)*2)
	for _, obligation := range rm.DiagramHint.Participants {
		participant := strings.TrimSpace(obligation.Identity)
		if !failed[strings.ToLower(participant)] || obligation.Role != types.DiagramParticipantIncidentRequired {
			continue
		}
		candidates := diagramParticipantTypedIncidentCandidates(rm, obligation, evidence, stagePrecedence, 3)
		for i, candidate := range candidates {
			rows = append(rows, fmt.Sprintf("typed_candidate[%s][%d]=%s", participant, i+1, candidate))
		}
	}
	return strings.Join(rows, "; ")
}

func diagramParticipantTypedIncidentCandidates(
	rm types.RequestModel,
	obligation types.DiagramParticipantHint,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
	limit int,
) []string {
	if limit <= 0 {
		return nil
	}
	obligations := make([]types.DiagramParticipantHint, 0)
	allSurfaces := make([][]string, 0)
	obligationIndex := -1
	if rm.DiagramHint != nil {
		for _, participant := range rm.DiagramHint.Participants {
			if participant.Role != types.DiagramParticipantIncidentRequired || strings.TrimSpace(participant.Identity) == "" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(participant.Identity), strings.TrimSpace(obligation.Identity)) {
				obligationIndex = len(obligations)
			}
			obligations = append(obligations, participant)
			participantIdentitySurfaces := []string{strings.TrimSpace(participant.Identity)}
			participantIdentitySurfaces = append(participantIdentitySurfaces, types.DiagramParticipantIdentitySurfaces(rm, participant)...)
			allSurfaces = append(allSurfaces, participantIdentitySurfaces)
		}
	}
	relationScope := buildFlowParticipantRelationScope(rm, obligations, allSurfaces, evidence, stagePrecedence)
	if len(obligations) > 1 && (obligationIndex < 0 ||
		obligationIndex >= len(relationScope.participantCovered) || !relationScope.participantCovered[obligationIndex]) {
		// Local operations remain valid evidence, but are not repair candidates
		// for a relationship that no typed component currently proves.
		return nil
	}
	surfaces := []string{strings.TrimSpace(obligation.Identity)}
	surfaces = append(surfaces, types.DiagramParticipantIdentitySurfaces(rm, obligation)...)
	seen := make(map[string]bool)
	rows := make([]string, 0, limit)
	add := func(relation types.DiagramRelationKind, from, to, location, participantEndpointSide string) {
		from, to = strings.TrimSpace(from), strings.TrimSpace(to)
		participantEndpointSide = strings.TrimSpace(participantEndpointSide)
		if len(rows) >= limit || !relation.IsValid() || from == "" || to == "" || participantEndpointSide == "" {
			return
		}
		key := strings.ToLower(string(relation) + "\x00" + from + "\x00" + to + "\x00" + location)
		if seen[key] {
			return
		}
		seen[key] = true
		rows = append(rows, fmt.Sprintf("{relation_kind:%s,from_identity:%s,to_identity:%s,participant_endpoint_side:%s,source:%s,edge_anchor_identity_fields:{from_identity:%s,to_identity:%s,relation_kind:%s}}",
			strconv.Quote(string(relation)), strconv.Quote(from), strconv.Quote(to), strconv.Quote(participantEndpointSide), strconv.Quote(location),
			strconv.Quote(from), strconv.Quote(to), strconv.Quote(string(relation))))
	}
	for _, relation := range stagePrecedence {
		fromIncident := stageauthority.ParticipantMatchesStageRow(rm, obligation, relation.From)
		toIncident := stageauthority.ParticipantMatchesStageRow(rm, obligation, relation.To)
		if fromIncident || toIncident {
			add(types.DiagramRelPrecedence, relation.From.AgentValue, relation.To.AgentValue,
				fmt.Sprintf("%s:%d-%d", relation.SourceFile, relation.LineStart, relation.LineEnd),
				diagramParticipantCandidateEndpointSide(fromIncident, toIncident))
		}
	}
	for operationIndex, operation := range types.FlowOperationEvidenceForRequest(evidence, rm) {
		if len(obligations) > 1 && (operationIndex >= len(relationScope.operationRelevant) ||
			!relationScope.operationRelevant[operationIndex]) {
			continue
		}
		from, to := strings.TrimSpace(operation.Subject), strings.TrimSpace(operation.Object)
		relation := types.RelationForClaimForm(types.ClaimFormOf(operation))
		if operation.AnchorKind == types.AnchorAssignment || operation.AnchorKind == types.AnchorInitializer {
			receiver, value, ok := types.AssignmentEvidenceEndpoints(operation)
			if !ok {
				continue
			}
			relation, from, to = types.DiagramRelDataFlow, value, receiver
		}
		fromIncident := diagramParticipantCandidateEndpointMatches(surfaces, from, operation, evidence)
		toIncident := diagramParticipantCandidateEndpointMatches(surfaces, to, operation, evidence)
		if fromIncident || toIncident {
			add(relation, from, to, operation.DisplayLocation(true),
				diagramParticipantCandidateEndpointSide(fromIncident, toIncident))
		}
	}
	return rows
}

func diagramParticipantCandidateEndpointMatches(
	surfaces []string,
	endpoint string,
	operation types.EvidenceItem,
	evidence []types.EvidenceItem,
) bool {
	for _, surface := range surfaces {
		if types.AnswerCodeIdentitySurfacesCompatible(surface, endpoint) ||
			types.AnswerCodeIdentityOwnsEndpoint(surface, endpoint) ||
			types.AnswerCodeIdentityIncidentViaDeclaredBinding(surface, endpoint, operation, evidence) {
			return true
		}
	}
	return false
}

func diagramParticipantCandidateEndpointSide(fromIncident, toIncident bool) string {
	switch {
	case fromIncident && toIncident:
		return "from_or_to"
	case fromIncident:
		return "from"
	case toIncident:
		return "to"
	default:
		return ""
	}
}

func diagramIncidentParticipantObligations(view *types.AnswerSemanticView) []types.DiagramParticipantHint {
	if view == nil {
		return nil
	}
	out := make([]types.DiagramParticipantHint, 0, len(view.DiagramParticipantObligations))
	seen := make(map[string]bool)
	for _, obligation := range view.DiagramParticipantObligations {
		identity := strings.TrimSpace(obligation.Identity)
		key := strings.ToLower(identity)
		if identity == "" || obligation.Role != types.DiagramParticipantIncidentRequired || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, obligation)
	}
	return out
}

func answerDocumentContainsDiagramPayload(doc *types.AnswerDocumentV2) bool {
	for _, block := range doc.Blocks {
		if block.Kind == types.BlockDiagram && block.Diagram != nil && strings.TrimSpace(block.Diagram.Body) != "" {
			return true
		}
	}
	return false
}

func diagramParticipantHasTypedVisibleIncident(doc *types.AnswerDocumentV2, surfaces []string, evidence []types.EvidenceItem, stagePrecedence []stageauthority.PrecedenceRelation) bool {
	blockCounts := diagramEvidenceBodyEdgeBlockCounts(doc)
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		labels := diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind)
		anchors := diagramEvidenceEffectiveAnchorsForBlock(doc, i, blockCounts)
		typedRelations := diagramTypedAnchorRelationSet(anchors)
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			relations := typedRelations[diagramEvidenceEdgeKey(edge.From, edge.To)]
			if !diagramHasValidTypedRelation(relations) {
				continue
			}
			from, to := diagramEvidenceEdgeEndpointSymbols(edge.From, edge.To, anchors, labels, evidence)
			// A technical endpoint owned by a requested component proves the
			// relation, but it does not by itself prove that the visible graph
			// connects that component. Keep technical authority and display
			// identity on the same endpoint: the endpoint must either carry the
			// requested participant label or sit inside its visible subgraph.
			// Otherwise a disconnected `BusContext` node plus an unrelated
			// `o.busCtx.Mutable.SetResult` edge would be accepted as a connected
			// BusContext, even though the user-visible graph contains no such
			// relation.
			if diagramParticipantIncidentEndpointMatches(surfaces, from, evidence) &&
				diagramParticipantVisibleEndpointCarriesIdentity(*block, surfaces, edge.From, from, labels) {
				return true
			}
			if diagramParticipantIncidentEndpointMatches(surfaces, to, evidence) &&
				diagramParticipantVisibleEndpointCarriesIdentity(*block, surfaces, edge.To, to, labels) {
				return true
			}
			if relations[types.DiagramRelPrecedence] && diagramParticipantHasVerifiedStageIncident(
				surfaces,
				diagramParticipantVisibleEndpoint(edge.From, labels),
				diagramParticipantVisibleEndpoint(edge.To, labels),
				stagePrecedence,
			) {
				return true
			}
		}
	}
	return false
}

func diagramParticipantVisibleEndpointCarriesIdentity(
	block types.AnswerBlock,
	surfaces []string,
	node string,
	typedEndpoint string,
	labels map[string]string,
) bool {
	// Mermaid node IDs are the stable structured identity lane while labels are
	// intentionally free to use concise business language.  When the model
	// makes the exact typed participant the endpoint node ID, and the same edge
	// retains its canonical technical identity in edge_anchors, that is an
	// explicit model-authored participant mapping.  Requiring the visible label
	// to repeat the internal identity would contradict the display contract and
	// can strand an otherwise valid evidence-backed edge in a retry loop.
	if diagramParticipantSurfaceMatches(surfaces, node) {
		return true
	}
	// An exact structured endpoint-to-participant identity is already the
	// model-authored mapping for this visible node. Its label may therefore be
	// business language (for example "理解请求" for Analyzer) without exposing
	// an internal symbol. The stricter label/group requirement below applies
	// only to broader owner/static-binding bridges.
	for _, surface := range surfaces {
		if types.AnswerCodeIdentitySurfacesCompatible(surface, typedEndpoint) {
			return true
		}
	}
	if diagramParticipantSurfaceMatches(surfaces, diagramParticipantVisibleEndpoint(node, labels)) {
		return true
	}
	return diagramParticipantNodeInsideVisibleSubgraph(block, surfaces, node)
}

// diagramParticipantNodeInsideVisibleSubgraph keeps the component/group
// presentation lane precise. A model may preserve an exact technical node
// label for evidence matching and place it inside a business-facing component
// subgraph. That is a visible incident relation. Merely declaring the same
// business participant as a disconnected node elsewhere is not.
func diagramParticipantNodeInsideVisibleSubgraph(block types.AnswerBlock, surfaces []string, node string) bool {
	if block.Kind != types.BlockDiagram || block.Diagram == nil || strings.TrimSpace(node) == "" {
		return false
	}
	var matchingStack []bool
	for _, line := range strings.Split(block.Diagram.Body, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "subgraph ") {
			parentMatches := len(matchingStack) > 0 && matchingStack[len(matchingStack)-1]
			matchingStack = append(matchingStack, parentMatches || diagramParticipantSubgraphLineMatches(trimmed, surfaces))
			continue
		}
		if lower == "end" {
			if len(matchingStack) > 0 {
				matchingStack = matchingStack[:len(matchingStack)-1]
			}
			continue
		}
		if len(matchingStack) == 0 || !matchingStack[len(matchingStack)-1] {
			continue
		}
		for _, decl := range mermaidcompat.NodeDeclarationsAll(line) {
			if strings.EqualFold(strings.TrimSpace(decl.Ident), strings.TrimSpace(node)) {
				return true
			}
		}
		for _, edge := range mermaidcompat.ParseEdges("flowchart TD\n" + line) {
			if strings.EqualFold(strings.TrimSpace(edge.From), strings.TrimSpace(node)) ||
				strings.EqualFold(strings.TrimSpace(edge.To), strings.TrimSpace(node)) {
				return true
			}
		}
	}
	return false
}

func diagramParticipantSubgraphLineMatches(line string, surfaces []string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(strings.ToLower(trimmed), "subgraph ") {
		return false
	}
	rest := strings.TrimSpace(trimmed[len("subgraph "):])
	if rest == "" {
		return false
	}
	candidates := []string{rest}
	if idx := strings.IndexAny(rest, "[\""); idx > 0 {
		candidates = append(candidates, strings.TrimSpace(rest[:idx]))
	}
	if open := strings.IndexAny(rest, "[\""); open >= 0 {
		label := strings.TrimSpace(rest[open:])
		label = strings.Trim(label, "[](){}\"' ")
		if label != "" {
			candidates = append(candidates, label)
		}
	}
	for _, candidate := range candidates {
		if diagramParticipantSurfaceMatches(surfaces, candidate) {
			return true
		}
	}
	return false
}

// diagramParticipantIncidentEndpointMatches lets a model-authored component
// node be covered by an already-proved operation on that exact receiver. The
// typed edge still owns its technical endpoint identity and direction; this
// helper only reconciles the broader participant display layer. Keep it out of
// boundary-node visibility checks, where a method label must not impersonate a
// missing disconnected participant node.
func diagramParticipantIncidentEndpointMatches(surfaces []string, endpoint string, evidence []types.EvidenceItem) bool {
	if diagramParticipantSurfaceMatches(surfaces, endpoint) {
		return true
	}
	for _, surface := range surfaces {
		if types.AnswerCodeIdentityOwnsEndpoint(surface, endpoint) {
			return true
		}
		for _, operation := range types.FlowOperationEvidence(evidence) {
			if !types.AnswerCodeIdentitySurfacesEquivalent(endpoint, operation.Subject) &&
				!types.AnswerCodeIdentitySurfacesEquivalent(endpoint, operation.Object) {
				continue
			}
			if types.AnswerCodeIdentityIncidentViaDeclaredBinding(surface, endpoint, operation, evidence) {
				return true
			}
		}
	}
	return false
}

func diagramParticipantVisibleEndpoint(node string, labels map[string]string) string {
	if label := strings.TrimSpace(labels[strings.ToLower(strings.TrimSpace(node))]); label != "" {
		return diagramEvidenceLabelSymbol(label)
	}
	return strings.TrimSpace(node)
}

func diagramParticipantHasVerifiedStageIncident(surfaces []string, from, to string, relations []stageauthority.PrecedenceRelation) bool {
	matched := make([]stageauthority.PrecedenceRelation, 0, 1)
	for _, relation := range relations {
		if diagramStageRowIdentityMatches(relation.From, from) && diagramStageRowIdentityMatches(relation.To, to) {
			matched = append(matched, relation)
		}
	}
	if len(matched) != 1 {
		return false
	}
	for _, row := range []stageauthority.StageRow{matched[0].From, matched[0].To} {
		for _, alias := range row.IdentityAliases() {
			if diagramParticipantSurfaceMatches(surfaces, alias) {
				return true
			}
		}
	}
	return false
}

// DiagramParticipantCoverageMismatchesWithRuntimeContext keeps pre-emit and
// post-finalizer participant coverage on the same checkout-verified stage
// authority. It remains document-local and does not persist derived evidence.
func DiagramParticipantCoverageMismatchesWithRuntimeContext(
	ctx *types.BusContext,
	doc *types.AnswerDocumentV2,
	view *types.AnswerSemanticView,
	evidence []types.EvidenceItem,
) []DiagramParticipantCoverageMismatch {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	return DiagramParticipantCoverageMismatches(
		doc, view, ctx.AnalysisIR.RequestModel, evidence,
		diagramVerifiedReadModeStagePrecedence(ctx, view)...,
	)
}

func diagramParticipantBlockHasVisibleNode(block types.AnswerBlock, surfaces []string) bool {
	if block.Kind != types.BlockDiagram || block.Diagram == nil {
		return false
	}
	for nodeID, label := range diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind) {
		for _, candidate := range []string{nodeID, diagramEvidenceLabelSymbol(label)} {
			if diagramParticipantSurfaceMatches(surfaces, candidate) {
				return true
			}
		}
	}
	return false
}

// diagramParticipantBlockHasVisibleIncidentEdge enforces the literal
// semantics of an unproven participant boundary. A participant with no typed
// incident candidate may remain visible, but that exact business identity
// must not be used as either endpoint of a visible edge. A separately labelled
// technical operation may still own its existing typed edge.
func diagramParticipantBlockHasVisibleIncidentEdge(block types.AnswerBlock, surfaces []string) bool {
	if block.Kind != types.BlockDiagram || block.Diagram == nil {
		return false
	}
	labels := diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind)
	for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
		for _, endpoint := range []string{edge.From, edge.To} {
			for _, candidate := range []string{
				strings.TrimSpace(endpoint),
				diagramParticipantVisibleEndpoint(endpoint, labels),
			} {
				if diagramParticipantSurfaceMatches(surfaces, candidate) {
					return true
				}
			}
		}
	}
	return false
}

func diagramParticipantDocumentHasVisibleIncidentEdge(doc *types.AnswerDocumentV2, surfaces []string) bool {
	if doc == nil {
		return false
	}
	for _, block := range doc.Blocks {
		if diagramParticipantBlockHasVisibleIncidentEdge(block, surfaces) {
			return true
		}
	}
	return false
}

func diagramParticipantSurfaceMatches(surfaces []string, candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false
	}
	for _, surface := range surfaces {
		if strings.EqualFold(strings.TrimSpace(surface), candidate) {
			return true
		}
		if types.AnswerCodeIdentitySurfacesCompatible(surface, candidate) {
			return true
		}
	}
	return false
}

func diagramParticipantSurfaceListContainsExact(surfaces []string, candidate string) bool {
	for _, surface := range surfaces {
		if strings.EqualFold(strings.TrimSpace(surface), strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

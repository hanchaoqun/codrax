package tool

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/types"
)

type DiagramParticipantCoverageIssue string

const (
	DiagramParticipantCoverageMissingBoundary    DiagramParticipantCoverageIssue = "missing_unproven_boundary"
	DiagramParticipantCoverageStaleBoundary      DiagramParticipantCoverageIssue = "stale_boundary_for_connected_participant"
	DiagramParticipantCoverageNodeMissing        DiagramParticipantCoverageIssue = "boundary_participant_not_visible"
	DiagramParticipantCoverageUnknownBoundary    DiagramParticipantCoverageIssue = "unknown_or_context_only_boundary"
	DiagramParticipantCoverageDuplicate          DiagramParticipantCoverageIssue = "duplicate_unproven_boundary"
	DiagramParticipantCoverageTypedEdgeMissing   DiagramParticipantCoverageIssue = "available_typed_incident_edge_not_rendered"
	DiagramParticipantCoverageIdentityMissing    DiagramParticipantCoverageIssue = "required_participant_identity_not_visible"
	DiagramParticipantCoverageBoundaryConnected  DiagramParticipantCoverageIssue = "unproven_boundary_has_visible_incident_edge"
	DiagramParticipantCoverageEndpointRetargeted DiagramParticipantCoverageIssue = "participant_visible_on_nonincident_endpoint"
	DiagramParticipantCoverageComponentSplit     DiagramParticipantCoverageIssue = "typed_requested_component_not_connected"
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

// diagramParticipantTypedIncidentCandidate is an already-grounded relation
// row plus the one requested-participant endpoint it may display. Keeping the
// typed value separate from its retry-string rendering lets the component
// gate select an executable join frontier without parsing its own prose.
type diagramParticipantTypedIncidentCandidate struct {
	participant             string
	relation                types.DiagramRelationKind
	anchor                  types.AnchorKind
	from                    string
	to                      string
	location                string
	participantEndpointSide string
	stageAuthority          bool
}

func preCheckDiagramParticipantCoverage(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, pctx *preEmitCheckContext) []emitFixHint {
	if pctx == nil || pctx.ctx == nil || pctx.ctx.AnalysisIR == nil {
		return nil
	}
	// Keep the first emit-time gate and the later orchestrator contract on one
	// evidence provider. In particular, BusContext.EvidenceItems may contain
	// lossless deterministic relation rows that are intentionally wider than
	// Mutable.EmittedEvidence/TurnA prompt caps. Letting only the later gate see
	// that pool creates an impossible contract where one unproven boundary is
	// required here and rejected after persistence.
	evidence := DiagramEvidenceForValidation(pctx.ctx, doc, view, pctx.evidenceItems())
	mismatches := DiagramParticipantCoverageMismatchesWithRuntimeContext(
		pctx.ctx, doc, view, evidence,
	)
	if len(mismatches) == 0 {
		return nil
	}
	parts := make([]string, 0, len(mismatches))
	for _, mismatch := range mismatches {
		parts = append(parts, "participant="+strings.TrimSpace(mismatch.Participant)+" issue="+string(mismatch.Issue))
	}
	candidates := diagramParticipantCoverageCandidateGuidance(
		doc,
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
	expected := "Typed participant coverage mismatch: " + strings.Join(parts, "; ") + ". For every typed incident_required participant with an available request-scoped evidence-backed relation, render one matching typed visible incident edge authored from that relation; do not replace that request-scoped evidence with an unproven boundary. A local operation that merely touches a participant is an independent fact: when the typed request-scope authority says the complete requested relation is unproved, that local operation may coexist with the participant's unproven requested-relation boundary and does not eliminate it. The requested participant identity must itself remain present as the exact Mermaid endpoint node id or as a visible node/subgraph/group label; an internal operation endpoint that is merely owned by or statically bound to that participant does not replace the business/component identity. A stable exact participant node id may carry a concise business-facing visible label when the proved edge terminates on that same node id and edge_anchors preserve the exact technical endpoint identities. The candidate map publishes participant_endpoint_side=from|to|from_or_to plus participant_node_id and participant_node_side. Reuse the selected candidate as one edge and set only that declared side's Mermaid node id to participant_node_id; keep the candidate's technical from_identity/to_identity unchanged in edge_anchor_identity_fields. A different requested participant may appear on the opposite visible endpoint only when that opposite typed endpoint is independently incident to it; anchor metadata does not license retargeting the reader-visible endpoint to another component. technical_endpoint_identity_stays_in_edge_anchor=true means the broader reader-facing participant node replaces that side only in diagram.body and edge_anchors[].from_node/to_node, never in from_identity/to_identity. visible_arrow_label is safe reader wording for the immutable relation_kind; use it instead of displaying the raw relation enum. Do not draw the technical method as a separate endpoint and then add an unanchored bridge edge to the participant. Alternatively, place the exact technical endpoint inside that participant's visible group. When the requested directed relation is unproved, retain exactly one {participant:<typed identity>,status:\"unproven\"} row. That boundary forbids using an unproved directed edge as the requested relation; it does not forbid an independently proved local technical edge, exact local endpoint, or no-arrow containment/grouping. For a bounded participant, make the exact typed identity the Mermaid node id or the first visible node label, including a visible subgraph/group label; a short node id whose typed identity appears only in a secondary parenthetical or later multiline label is not that primary identity. Omit participant_boundaries or emit [] when all required participants have the requested typed incidence. Remove stale, unknown, context_only, or already-covered boundary rows. The system does not create or choose an edge."
	if endpointConflicts != "" {
		expected = "Resolve these exact endpoint-ID/label collisions first. Preserve each existing visible edge, canonical from_identity/to_identity, relation_kind, and direction. Choose one fresh non-participant Mermaid node ID for the technical endpoint; change only that side of the visible body edge and the matching edge_anchor from_node/to_node field. If that endpoint's visible label also equals the unproven participant, relabel the same technical node with concise technical wording derived from the already-published exact from_identity/to_identity; do not alter those anchor identities. Keep the exact participant separate from that directed edge and retain its unproven boundary; an independently grounded, already-authored no-arrow subgraph/grouping remains valid and must not be flattened or removed. The system is identifying an already model-authored unique edge/anchor pair and its parsed visible label, not creating or selecting a relation or choosing replacement wording: " + endpointConflicts + ". " + expected
	}
	if actions != "" {
		expected += ". Typed repair actions (apply only the row for each failed participant; these actions preserve model ownership of the visible diagram): " + actions
	}
	if candidates != "" {
		expected += ". Existing typed candidate map (choose only the relevant candidate edges needed for the failed participant or its typed join frontier; this list is not a requirement to render every candidate and does not create an edge). Each candidate's edge_anchor_identity_fields are the exact schema fields to copy alongside model-authored from_node/to_node IDs; never replace those exact identities with the broader participant name: " + candidates
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
		case DiagramParticipantCoverageEndpointRetargeted:
			edgeAction = "keep_the_existing_typed_edge_and_anchor_direction_but_remove_the_requested_participant_identity_from_the_nonincident_endpoint"
			identityAction = "map_each_requested_participant_only_to_an_endpoint_that_is_typed_incident_to_it_or_keep_it_in_an_honest_unproven_group"
			boundaryAction = "recompute_from_the_corrected_visible_incidence_without_inventing_an_edge"
		case DiagramParticipantCoverageComponentSplit:
			edgeAction = "retain_every_valid_visible_edge_and_reuse_existing_typed_candidate_edges_that_join_the_current_visible_components"
			identityAction = "keep_each_requested_participant_on_its_typed_incident_endpoint_and_keep_shared_technical_handoff_nodes_visible"
			boundaryAction = "omit_unproven_boundary_because_the_typed_evidence_graph_already_proves_the_join"
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
		obligation                       types.DiagramParticipantHint
		surfaces                         []string
		visibleCovered                   bool
		identityVisible                  bool
		typedEdgeAvailable               bool
		independentLocalIncidenceAllowed bool
		bounded                          bool
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
	requestedParticipantGraphComplete := diagramParticipantRequestedGraphConnected(doc, allSurfaces, evidence, stagePrecedence)
	evidenceParticipantGraphComplete := len(obligations) > 1
	for i := range states {
		requestScopedEdgeAvailable := relationScope.effectiveParticipantCovered(i, requestedParticipantGraphComplete)
		states[i].typedEdgeAvailable = requestScopedEdgeAvailable
		if !requestScopedEdgeAvailable || !relationScope.completionParticipantConnectedCovered(i) {
			evidenceParticipantGraphComplete = false
		}
		// Requested-relation coverage and local technical incidence are
		// orthogonal. When the complete requested graph remains unproved and
		// this participant has no request-scoped candidate, an independently
		// typed local edge cannot make its required unproven boundary stale or
		// contradictory merely because the technical endpoint shares an exact
		// identity with the participant.
		states[i].independentLocalIncidenceAllowed = len(obligations) > 1 &&
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
			if states[idx].visibleCovered && states[idx].identityVisible && !states[idx].independentLocalIncidenceAllowed {
				out = append(out, DiagramParticipantCoverageMismatch{
					BlockID: block.ID, Participant: states[idx].obligation.Identity,
					Issue: DiagramParticipantCoverageStaleBoundary,
				})
				continue
			}
			if states[idx].typedEdgeAvailable && !states[idx].independentLocalIncidenceAllowed {
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
			if !states[idx].independentLocalIncidenceAllowed &&
				diagramParticipantDocumentHasVisibleIncidentEdge(doc, states[idx].surfaces) {
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
		if current.bounded || (current.visibleCovered && current.identityVisible && !current.independentLocalIncidenceAllowed) {
			continue
		}
		issue := DiagramParticipantCoverageMissingBoundary
		if current.typedEdgeAvailable && !current.independentLocalIncidenceAllowed {
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
	// Per-participant incidence is insufficient when the exact typed evidence
	// graph already proves one connected requested flow: a finalizer can choose
	// one valid local edge per participant and accidentally re-split that graph
	// into several reader-visible islands. Require a visible join only under the
	// precise evidence-complete signal. If source evidence itself is split or
	// unavailable, explicit unproven boundaries remain the valid fallback.
	// A component-split hard reject is actionable only when the same typed
	// provider can publish at least one exact candidate that crosses two of the
	// model's current visible components. Participant-level connectivity may be
	// proved through an identity shared by several technical components; that
	// is useful exploration authority but is not itself a copyable diagram edge.
	// Without a crossing candidate, keep the split as an honest bounded result
	// instead of forcing the model to guess a bridge through repeated retries.
	joinCandidates := diagramParticipantTypedJoinCandidates(doc, rm, evidence, stagePrecedence, 1)
	if evidenceParticipantGraphComplete && !requestedParticipantGraphComplete && len(joinCandidates) > 0 {
		principal := diagramParticipantRequestedGraphPrincipalCovered(doc, allSurfaces, evidence, stagePrecedence)
		existing := make(map[string]bool, len(out))
		for _, mismatch := range out {
			existing[strings.ToLower(strings.TrimSpace(mismatch.Participant))] = true
		}
		for i, current := range states {
			participant := strings.TrimSpace(current.obligation.Identity)
			if i < len(principal) && principal[i] || existing[strings.ToLower(participant)] {
				continue
			}
			out = append(out, DiagramParticipantCoverageMismatch{
				Participant: participant,
				Issue:       DiagramParticipantCoverageComponentSplit,
			})
		}
	}
	out = append(out, diagramParticipantVisibleEndpointRetargetMismatches(
		doc, obligations, allSurfaces, evidence,
	)...)
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

// diagramParticipantVisibleEndpointRetargetMismatches keeps the reader-facing
// participant identity on the same side as its typed endpoint incidence. Edge
// anchors intentionally allow concise business labels, so this check does not
// require every visible label to repeat an internal symbol. It rejects only a
// much narrower contradiction: an endpoint node id or primary label explicitly
// names one of the analyzer's requested participants while the exact typed
// endpoint on that side is not incident to that participant. A visible group
// may legitimately contain several internal endpoints, so grouping alone is
// not treated as endpoint retargeting.
//
// This closes the shape where an honest o.busCtx -> BuildAgentContext anchor is
// displayed as BusContext -> Analyzer. The typed metadata still proves the
// operation, but it cannot authorize moving the Analyzer identity onto the
// BuildAgentContext endpoint. No request text, answer prose, fuzzy label, or
// inferred relation participates.
func diagramParticipantVisibleEndpointRetargetMismatches(
	doc *types.AnswerDocumentV2,
	obligations []types.DiagramParticipantHint,
	allSurfaces [][]string,
	evidence []types.EvidenceItem,
) []DiagramParticipantCoverageMismatch {
	if doc == nil || len(obligations) == 0 || len(obligations) != len(allSurfaces) {
		return nil
	}
	blockCounts := diagramEvidenceBodyEdgeBlockCounts(doc)
	boundedParticipants := make(map[string]bool)
	for _, block := range doc.Blocks {
		for _, boundary := range block.ParticipantBoundaries {
			if boundary.Status == types.DiagramParticipantBoundaryUnproven {
				boundedParticipants[strings.ToLower(strings.TrimSpace(boundary.Participant))] = true
			}
		}
	}
	seen := make(map[string]bool)
	var out []DiagramParticipantCoverageMismatch
	for blockIndex := range doc.Blocks {
		block := &doc.Blocks[blockIndex]
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
			fromIdentity, toIdentity, present, conflict := diagramEvidenceEdgeIdentityPair(edge.From, edge.To, anchors)
			if !present || conflict || strings.TrimSpace(fromIdentity) == "" || strings.TrimSpace(toIdentity) == "" {
				continue
			}
			fromVisible := make([]bool, len(obligations))
			toVisible := make([]bool, len(obligations))
			fromIncident := make([]bool, len(obligations))
			toIncident := make([]bool, len(obligations))
			fromHasAlignedParticipant := false
			toHasAlignedParticipant := false
			for participantIndex, obligation := range obligations {
				if obligation.Role != types.DiagramParticipantIncidentRequired {
					continue
				}
				surfaces := allSurfaces[participantIndex]
				fromVisible[participantIndex] = diagramParticipantEndpointExplicitlyDisplaysIdentity(surfaces, edge.From, labels)
				toVisible[participantIndex] = diagramParticipantEndpointExplicitlyDisplaysIdentity(surfaces, edge.To, labels)
				fromIncident[participantIndex] = diagramParticipantIncidentEndpointMatches(surfaces, fromIdentity, evidence)
				toIncident[participantIndex] = diagramParticipantIncidentEndpointMatches(surfaces, toIdentity, evidence)
				fromHasAlignedParticipant = fromHasAlignedParticipant || (fromVisible[participantIndex] && fromIncident[participantIndex])
				toHasAlignedParticipant = toHasAlignedParticipant || (toVisible[participantIndex] && toIncident[participantIndex])
			}
			for participantIndex, obligation := range obligations {
				participant := strings.TrimSpace(obligation.Identity)
				if participant == "" || obligation.Role != types.DiagramParticipantIncidentRequired ||
					boundedParticipants[strings.ToLower(participant)] {
					continue
				}
				fromRetargeted := fromVisible[participantIndex] && !fromIncident[participantIndex] && toHasAlignedParticipant
				toRetargeted := toVisible[participantIndex] && !toIncident[participantIndex] && fromHasAlignedParticipant
				if !fromRetargeted && !toRetargeted {
					continue
				}
				key := strings.ToLower(strings.TrimSpace(block.ID) + "\x00" + participant)
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, DiagramParticipantCoverageMismatch{
					BlockID: block.ID, Participant: participant,
					Issue: DiagramParticipantCoverageEndpointRetargeted,
				})
			}
		}
	}
	return out
}

func diagramParticipantEndpointExplicitlyDisplaysIdentity(
	surfaces []string,
	node string,
	labels map[string]string,
) bool {
	node = strings.TrimSpace(node)
	if node == "" {
		return false
	}
	if diagramParticipantSurfaceListContainsExact(surfaces, node) {
		return true
	}
	if label := strings.TrimSpace(labels[strings.ToLower(node)]); label != "" {
		for _, candidate := range diagramEvidenceLabelIdentityCandidates(label) {
			if diagramParticipantSurfaceListContainsExact(surfaces, candidate) {
				return true
			}
		}
	}
	return false
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
	stagePrecedence []stageauthority.PrecedenceRelation,
) bool {
	principal := diagramParticipantRequestedGraphPrincipalCovered(doc, participantSurfaces, evidence, stagePrecedence)
	if len(principal) < 2 {
		return false
	}
	for _, covered := range principal {
		if !covered {
			return false
		}
	}
	return true
}

// diagramParticipantRequestedGraphPrincipalCovered computes weak connectivity
// over model-authored visible edges that already carry typed anchors. Unlike a
// direct participant-pair check, it keeps shared technical handoff nodes in
// the graph, so A -> builder <- B is recognized as one reader-visible
// component without pretending that builder is itself a requested component.
// It returns only the largest stable participant group; callers may request a
// missing bridge but never synthesize one.
func diagramParticipantRequestedGraphPrincipalCovered(
	doc *types.AnswerDocumentV2,
	participantSurfaces [][]string,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
) []bool {
	covered := make([]bool, len(participantSurfaces))
	if doc == nil || len(participantSurfaces) == 0 {
		return covered
	}
	participantParent := make([]int, len(participantSurfaces))
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

	blockCounts := diagramEvidenceBodyEdgeBlockCounts(doc)
	for blockIndex, block := range doc.Blocks {
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		type visibleNode struct {
			parent       int
			participants map[int]bool
		}
		nodes := make(map[string]*visibleNode)
		nodeIndex := make(map[string]int)
		ordered := make([]string, 0)
		ensureNode := func(raw string) int {
			key := strings.ToLower(strings.TrimSpace(raw))
			if index, ok := nodeIndex[key]; ok {
				return index
			}
			index := len(ordered)
			nodeIndex[key] = index
			ordered = append(ordered, key)
			nodes[key] = &visibleNode{parent: index, participants: make(map[int]bool)}
			return index
		}
		var nodeFind func(int) int
		nodeFind = func(v int) int {
			key := ordered[v]
			if nodes[key].parent != v {
				nodes[key].parent = nodeFind(nodes[key].parent)
			}
			return nodes[key].parent
		}
		nodeUnion := func(a, b int) {
			a, b = nodeFind(a), nodeFind(b)
			if a != b {
				nodes[ordered[b]].parent = a
			}
		}

		labels := diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind)
		anchors := diagramEvidenceEffectiveAnchorsForBlock(doc, blockIndex, blockCounts)
		typedRelations := diagramTypedAnchorRelationSet(anchors)
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			if !diagramHasValidTypedRelation(typedRelations[diagramEvidenceEdgeKey(edge.From, edge.To)]) {
				continue
			}
			fromIndex, toIndex := ensureNode(edge.From), ensureNode(edge.To)
			nodeUnion(fromIndex, toIndex)
			from, to := diagramEvidenceEdgeEndpointSymbols(edge.From, edge.To, anchors, labels, evidence)
			fromParticipants := diagramParticipantEndpointRosterMatches(block, edge.From, from, labels, participantSurfaces, evidence)
			toParticipants := diagramParticipantEndpointRosterMatches(block, edge.To, to, labels, participantSurfaces, evidence)
			if typedRelations[diagramEvidenceEdgeKey(edge.From, edge.To)][types.DiagramRelPrecedence] {
				stageFrom, stageTo := diagramParticipantVerifiedStageEndpointRosterMatches(
					diagramParticipantVisibleEndpoint(edge.From, labels),
					diagramParticipantVisibleEndpoint(edge.To, labels),
					participantSurfaces,
					stagePrecedence,
				)
				fromParticipants = appendUniqueParticipantIndexes(fromParticipants, stageFrom...)
				toParticipants = appendUniqueParticipantIndexes(toParticipants, stageTo...)
			}
			if len(fromParticipants) == 1 {
				nodes[ordered[fromIndex]].participants[fromParticipants[0]] = true
			}
			if len(toParticipants) == 1 {
				nodes[ordered[toIndex]].participants[toParticipants[0]] = true
			}
		}
		componentParticipants := make(map[int]map[int]bool)
		for i, key := range ordered {
			root := nodeFind(i)
			if componentParticipants[root] == nil {
				componentParticipants[root] = make(map[int]bool)
			}
			for participantIndex := range nodes[key].participants {
				componentParticipants[root][participantIndex] = true
			}
		}
		for _, members := range componentParticipants {
			first := -1
			for participantIndex := range members {
				if first < 0 {
					first = participantIndex
					continue
				}
				participantUnion(first, participantIndex)
			}
		}
	}

	type participantGroup struct {
		members  []int
		earliest int
	}
	groups := make(map[int]*participantGroup)
	for i := range participantSurfaces {
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
			covered[participantIndex] = true
		}
	}
	return covered
}

func diagramParticipantVerifiedStageEndpointRosterMatches(
	from, to string,
	participantSurfaces [][]string,
	stagePrecedence []stageauthority.PrecedenceRelation,
) (fromParticipants, toParticipants []int) {
	matched := make([]stageauthority.PrecedenceRelation, 0, 1)
	for _, relation := range stagePrecedence {
		if diagramStageRowIdentityMatches(relation.From, from) && diagramStageRowIdentityMatches(relation.To, to) {
			matched = append(matched, relation)
		}
	}
	if len(matched) != 1 {
		return nil, nil
	}
	for participantIndex, surfaces := range participantSurfaces {
		for _, alias := range matched[0].From.IdentityAliases() {
			if diagramParticipantSurfaceMatches(surfaces, alias) {
				fromParticipants = append(fromParticipants, participantIndex)
				break
			}
		}
		for _, alias := range matched[0].To.IdentityAliases() {
			if diagramParticipantSurfaceMatches(surfaces, alias) {
				toParticipants = append(toParticipants, participantIndex)
				break
			}
		}
	}
	return fromParticipants, toParticipants
}

func appendUniqueParticipantIndexes(dst []int, values ...int) []int {
	for _, value := range values {
		found := false
		for _, existing := range dst {
			if existing == value {
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, value)
		}
	}
	return dst
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
	doc *types.AnswerDocumentV2,
	rm types.RequestModel,
	mismatches []DiagramParticipantCoverageMismatch,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
) string {
	if rm.DiagramHint == nil || len(mismatches) == 0 {
		return ""
	}
	failed := make(map[string]bool)
	componentSplit := false
	for _, mismatch := range mismatches {
		if mismatch.Issue == DiagramParticipantCoverageTypedEdgeMissing ||
			mismatch.Issue == DiagramParticipantCoverageIdentityMissing ||
			mismatch.Issue == DiagramParticipantCoverageEndpointRetargeted ||
			mismatch.Issue == DiagramParticipantCoverageComponentSplit {
			failed[strings.ToLower(strings.TrimSpace(mismatch.Participant))] = true
		}
		componentSplit = componentSplit || mismatch.Issue == DiagramParticipantCoverageComponentSplit
	}
	if len(failed) == 0 {
		return ""
	}
	joinGuidance := ""
	if componentSplit {
		// A bridge can be incident to the already-visible principal component
		// rather than to the participants reported outside it. Publish the same
		// bounded typed roster for the whole requested frontier, but lead with the
		// subset that actually crosses two current visible components. This still
		// creates no edge or wording.
		joinGuidance = diagramParticipantTypedJoinCandidateGuidance(doc, rm, evidence, stagePrecedence, 4)
		failed = nil
	}
	incidentGuidance := flowParticipantTypedIncidentCandidateGuidance(rm, evidence, stagePrecedence, failed, 3)
	if joinGuidance == "" {
		return incidentGuidance
	}
	if incidentGuidance == "" {
		return joinGuidance
	}
	return joinGuidance + "; " + incidentGuidance
}

type diagramParticipantVisibleComponentIndex struct {
	blockID       string
	nodeComponent map[string]int
	labels        map[string]string
	anchors       []types.DiagramEdgeAnchor
}

// diagramParticipantTypedJoinCandidateGuidance publishes only candidates that
// cross two components already visible in the current model-authored diagram.
// The crossing predicate is computed from parsed node IDs, typed anchor
// identities, and the analyzer's typed participant surfaces; it never reads
// labels or prose as relation authority and never chooses an edge for the
// model.
func diagramParticipantTypedJoinCandidateGuidance(
	doc *types.AnswerDocumentV2,
	rm types.RequestModel,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
	limit int,
) string {
	candidates := diagramParticipantTypedJoinCandidates(doc, rm, evidence, stagePrecedence, limit)
	rows := make([]string, 0, len(candidates))
	for i, candidate := range candidates {
		rows = append(rows, fmt.Sprintf("typed_join_candidate[%d]=%s", i+1,
			renderDiagramParticipantTypedIncidentCandidate(candidate, rm.Language)))
	}
	return strings.Join(rows, "; ")
}

func diagramParticipantTypedJoinCandidates(
	doc *types.AnswerDocumentV2,
	rm types.RequestModel,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
	limit int,
) []diagramParticipantTypedIncidentCandidate {
	if doc == nil || rm.DiagramHint == nil || limit <= 0 {
		return nil
	}
	obligations, allSurfaces := diagramParticipantCandidateObligations(rm)
	if len(obligations) < 2 {
		return nil
	}
	indexes := diagramParticipantVisibleComponentIndexes(doc)
	if len(indexes) == 0 {
		return nil
	}
	relationScope := buildFlowParticipantRelationScope(rm, obligations, allSurfaces, evidence, stagePrecedence)
	// Build the complete internal candidate pool, then bound only the exact
	// crossing rows published to the model. This avoids hiding a useful bridge
	// behind three higher-ranked local incident edges.
	poolLimit := len(evidence) + len(stagePrecedence) + 1
	if poolLimit < 8 {
		poolLimit = 8
	}
	seen := make(map[string]bool)
	out := make([]diagramParticipantTypedIncidentCandidate, 0, limit)
	for obligationIndex, obligation := range obligations {
		candidates := diagramParticipantTypedIncidentCandidateValuesWithScope(
			rm, obligation, evidence, stagePrecedence, poolLimit,
			obligations, allSurfaces, obligationIndex, relationScope,
		)
		for _, candidate := range candidates {
			if !diagramParticipantTypedCandidateCrossesVisibleComponents(candidate, allSurfaces[obligationIndex], indexes) {
				continue
			}
			key := strings.ToLower(candidate.participant + "\x00" + string(candidate.relation) + "\x00" +
				candidate.from + "\x00" + candidate.to + "\x00" + candidate.participantEndpointSide)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, candidate)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func diagramParticipantVisibleComponentIndexes(doc *types.AnswerDocumentV2) []diagramParticipantVisibleComponentIndex {
	if doc == nil {
		return nil
	}
	blockCounts := diagramEvidenceBodyEdgeBlockCounts(doc)
	out := make([]diagramParticipantVisibleComponentIndex, 0)
	for blockIndex := range doc.Blocks {
		block := &doc.Blocks[blockIndex]
		if block.Kind != types.BlockDiagram || block.Diagram == nil {
			continue
		}
		anchors := diagramEvidenceEffectiveAnchorsForBlock(doc, blockIndex, blockCounts)
		typedRelations := diagramTypedAnchorRelationSet(anchors)
		nodeIndex := make(map[string]int)
		ordered := make([]string, 0)
		parent := make([]int, 0)
		ensureNode := func(raw string) int {
			key := strings.ToLower(strings.TrimSpace(raw))
			if index, ok := nodeIndex[key]; ok {
				return index
			}
			index := len(ordered)
			nodeIndex[key] = index
			ordered = append(ordered, key)
			parent = append(parent, index)
			return index
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
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			if !diagramHasValidTypedRelation(typedRelations[diagramEvidenceEdgeKey(edge.From, edge.To)]) {
				continue
			}
			union(ensureNode(edge.From), ensureNode(edge.To))
		}
		if len(ordered) == 0 {
			continue
		}
		components := make(map[string]int, len(ordered))
		for i, node := range ordered {
			components[node] = find(i)
		}
		out = append(out, diagramParticipantVisibleComponentIndex{
			blockID: strings.TrimSpace(block.ID), nodeComponent: components,
			labels: diagramEvidenceNodeLabels(block.Diagram.Body, block.Diagram.Kind), anchors: anchors,
		})
	}
	return out
}

func diagramParticipantTypedCandidateCrossesVisibleComponents(
	candidate diagramParticipantTypedIncidentCandidate,
	participantSurfaces []string,
	indexes []diagramParticipantVisibleComponentIndex,
) bool {
	for _, index := range indexes {
		participantComponents := diagramParticipantVisibleComponentsForParticipant(index, participantSurfaces)
		if len(participantComponents) == 0 {
			continue
		}
		fromComponents := diagramParticipantVisibleComponentsForIdentity(index, candidate.from)
		toComponents := diagramParticipantVisibleComponentsForIdentity(index, candidate.to)
		switch candidate.participantEndpointSide {
		case "from":
			fromComponents = participantComponents
		case "to":
			toComponents = participantComponents
		case "from_or_to":
			if diagramParticipantComponentSetsDiffer(participantComponents, toComponents) ||
				diagramParticipantComponentSetsDiffer(fromComponents, participantComponents) {
				return true
			}
			continue
		default:
			continue
		}
		if diagramParticipantComponentSetsDiffer(fromComponents, toComponents) {
			return true
		}
	}
	return false
}

func diagramParticipantVisibleComponentsForParticipant(
	index diagramParticipantVisibleComponentIndex,
	surfaces []string,
) map[int]bool {
	out := make(map[int]bool)
	for node, component := range index.nodeComponent {
		if diagramParticipantEndpointExplicitlyDisplaysIdentity(surfaces, node, index.labels) {
			out[component] = true
		}
	}
	return out
}

func diagramParticipantVisibleComponentsForIdentity(
	index diagramParticipantVisibleComponentIndex,
	identity string,
) map[int]bool {
	out := make(map[int]bool)
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return out
	}
	add := func(node string) {
		if component, ok := index.nodeComponent[strings.ToLower(strings.TrimSpace(node))]; ok {
			out[component] = true
		}
	}
	for _, anchor := range index.anchors {
		if diagramParticipantTypedIdentityEquivalent(identity, anchor.FromIdentity) {
			add(anchor.FromNode)
		}
		if diagramParticipantTypedIdentityEquivalent(identity, anchor.ToIdentity) {
			add(anchor.ToNode)
		}
	}
	for node := range index.nodeComponent {
		if diagramParticipantTypedIdentityEquivalent(identity, node) ||
			diagramParticipantTypedIdentityEquivalent(identity, index.labels[node]) {
			add(node)
		}
	}
	return out
}

func diagramParticipantTypedIdentityEquivalent(left, right string) bool {
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	return left != "" && right != "" &&
		(strings.EqualFold(left, right) || types.AnswerCodeIdentitySurfacesEquivalent(left, right))
}

func diagramParticipantComponentSetsDiffer(left, right map[int]bool) bool {
	for leftComponent := range left {
		for rightComponent := range right {
			if leftComponent != rightComponent {
				return true
			}
		}
	}
	return false
}

// FlowParticipantTypedIncidentCandidateGuidance publishes the same bounded,
// typed participant-to-endpoint choices used by the hard participant gate,
// but early enough for first-pass authoring. It is deliberately a candidate
// roster: it does not choose a row, create a visible edge, assign Mermaid node
// IDs on the model's behalf, or change the relation/boundary decision.
//
// Keeping this exported seam in the tool package prevents prompt construction
// from growing a second language-specific endpoint matcher that can drift from
// validation. No request prose or answer text participates.
func FlowParticipantTypedIncidentCandidateGuidance(
	rm types.RequestModel,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
	limitPerParticipant int,
) string {
	return flowParticipantTypedIncidentCandidateGuidance(rm, evidence, stagePrecedence, nil, limitPerParticipant)
}

func flowParticipantTypedIncidentCandidateGuidance(
	rm types.RequestModel,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
	participants map[string]bool,
	limitPerParticipant int,
) string {
	if rm.DiagramHint == nil || limitPerParticipant <= 0 {
		return ""
	}
	obligations, allSurfaces := diagramParticipantCandidateObligations(rm)
	if len(obligations) == 0 {
		return ""
	}
	relationScope := buildFlowParticipantRelationScope(rm, obligations, allSurfaces, evidence, stagePrecedence)
	rows := make([]string, 0, len(rm.DiagramHint.Participants)*limitPerParticipant)
	for obligationIndex, obligation := range obligations {
		participant := strings.TrimSpace(obligation.Identity)
		if participants != nil && !participants[strings.ToLower(participant)] {
			continue
		}
		candidates := diagramParticipantTypedIncidentCandidatesWithScope(
			rm, obligation, evidence, stagePrecedence, limitPerParticipant,
			obligations, allSurfaces, obligationIndex, relationScope,
		)
		for i, candidate := range candidates {
			rows = append(rows, fmt.Sprintf("typed_candidate[%s][%d]=%s", participant, i+1, candidate))
		}
	}
	return strings.Join(rows, "; ")
}

func diagramParticipantCandidateObligations(rm types.RequestModel) ([]types.DiagramParticipantHint, [][]string) {
	if rm.DiagramHint == nil {
		return nil, nil
	}
	obligations := make([]types.DiagramParticipantHint, 0, len(rm.DiagramHint.Participants))
	allSurfaces := make([][]string, 0, len(rm.DiagramHint.Participants))
	for _, participant := range rm.DiagramHint.Participants {
		if participant.Role != types.DiagramParticipantIncidentRequired || strings.TrimSpace(participant.Identity) == "" {
			continue
		}
		obligations = append(obligations, participant)
		surfaces := []string{strings.TrimSpace(participant.Identity)}
		surfaces = append(surfaces, types.DiagramParticipantIdentitySurfaces(rm, participant)...)
		allSurfaces = append(allSurfaces, surfaces)
	}
	return obligations, allSurfaces
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
	obligations, allSurfaces := diagramParticipantCandidateObligations(rm)
	obligationIndex := -1
	for i, participant := range obligations {
		if strings.EqualFold(strings.TrimSpace(participant.Identity), strings.TrimSpace(obligation.Identity)) {
			obligationIndex = i
			break
		}
	}
	relationScope := buildFlowParticipantRelationScope(rm, obligations, allSurfaces, evidence, stagePrecedence)
	return diagramParticipantTypedIncidentCandidatesWithScope(
		rm, obligation, evidence, stagePrecedence, limit,
		obligations, allSurfaces, obligationIndex, relationScope,
	)
}

func diagramParticipantTypedIncidentCandidatesWithScope(
	rm types.RequestModel,
	obligation types.DiagramParticipantHint,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
	limit int,
	obligations []types.DiagramParticipantHint,
	allSurfaces [][]string,
	obligationIndex int,
	relationScope flowParticipantRelationScope,
) []string {
	candidates := diagramParticipantTypedIncidentCandidateValuesWithScope(
		rm, obligation, evidence, stagePrecedence, limit,
		obligations, allSurfaces, obligationIndex, relationScope,
	)
	rows := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		rows = append(rows, renderDiagramParticipantTypedIncidentCandidate(candidate, rm.Language))
	}
	return rows
}

func diagramParticipantTypedIncidentCandidateValuesWithScope(
	rm types.RequestModel,
	obligation types.DiagramParticipantHint,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
	limit int,
	obligations []types.DiagramParticipantHint,
	allSurfaces [][]string,
	obligationIndex int,
	relationScope flowParticipantRelationScope,
) []diagramParticipantTypedIncidentCandidate {
	if limit <= 0 {
		return nil
	}
	if len(obligations) > 1 && (obligationIndex < 0 ||
		!relationScope.effectiveParticipantCovered(obligationIndex, false)) {
		// Local operations remain valid evidence, but are not repair candidates
		// for a relationship that no typed component currently proves.
		return nil
	}
	var surfaces []string
	if obligationIndex >= 0 && obligationIndex < len(allSurfaces) {
		surfaces = allSurfaces[obligationIndex]
	} else {
		surfaces = []string{strings.TrimSpace(obligation.Identity)}
		surfaces = append(surfaces, types.DiagramParticipantIdentitySurfaces(rm, obligation)...)
	}
	seen := make(map[string]bool)
	candidates := make([]diagramParticipantTypedIncidentCandidate, 0, limit)
	add := func(relation types.DiagramRelationKind, anchor types.AnchorKind, from, to, location, participantEndpointSide string, stageAuthority bool) {
		from, to = strings.TrimSpace(from), strings.TrimSpace(to)
		participantEndpointSide = strings.TrimSpace(participantEndpointSide)
		if !relation.IsValid() || from == "" || to == "" || participantEndpointSide == "" {
			return
		}
		key := strings.ToLower(string(relation) + "\x00" + from + "\x00" + to + "\x00" + location)
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, diagramParticipantTypedIncidentCandidate{
			participant: strings.TrimSpace(obligation.Identity),
			relation:    relation, anchor: anchor, from: from, to: to, location: location,
			participantEndpointSide: participantEndpointSide, stageAuthority: stageAuthority,
		})
	}
	for _, relation := range stagePrecedence {
		fromIncident := stageauthority.ParticipantMatchesStageRow(rm, obligation, relation.From)
		toIncident := stageauthority.ParticipantMatchesStageRow(rm, obligation, relation.To)
		if fromIncident || toIncident {
			add(types.DiagramRelPrecedence, types.AnchorPrecedence, relation.From.AgentValue, relation.To.AgentValue,
				fmt.Sprintf("%s:%d-%d", relation.SourceFile, relation.LineStart, relation.LineEnd),
				diagramParticipantCandidateEndpointSide(fromIncident, toIncident), true)
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
			add(relation, operation.AnchorKind, from, to, operation.DisplayLocation(true),
				diagramParticipantCandidateEndpointSide(fromIncident, toIncident), false)
		}
	}
	// Candidate ordering is selection guidance only: it never creates, removes,
	// reverses, or rewrites a typed relation. Prefer a verified stage edge when
	// the participant is itself a verified stage, then rank the already-typed
	// operations by the analyzer's typed predicate axis. Deterministic typed
	// endpoint/location keys make the bounded roster independent of evidence
	// arrival order; no request or answer prose participates in the ranking.
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		leftRank := diagramParticipantTypedCandidateRank(rm.PredicateAxis, left.relation, left.anchor, left.stageAuthority)
		rightRank := diagramParticipantTypedCandidateRank(rm.PredicateAxis, right.relation, right.anchor, right.stageAuthority)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if left.location != right.location {
			return left.location < right.location
		}
		if left.relation != right.relation {
			return left.relation < right.relation
		}
		if left.from != right.from {
			return left.from < right.from
		}
		if left.to != right.to {
			return left.to < right.to
		}
		return left.participantEndpointSide < right.participantEndpointSide
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates
}

func renderDiagramParticipantTypedIncidentCandidate(candidate diagramParticipantTypedIncidentCandidate, language string) string {
	return fmt.Sprintf("{relation_kind:%s,visible_arrow_label:%s,from_identity:%s,to_identity:%s,participant_endpoint_side:%s,participant_node_id:%s,participant_node_side:%s,technical_endpoint_identity_stays_in_edge_anchor:true,source:%s,edge_anchor_identity_fields:{from_identity:%s,to_identity:%s,relation_kind:%s}}",
		strconv.Quote(string(candidate.relation)), strconv.Quote(diagramParticipantReaderArrowLabel(candidate.relation, language)), strconv.Quote(candidate.from), strconv.Quote(candidate.to), strconv.Quote(candidate.participantEndpointSide), strconv.Quote(candidate.participant), strconv.Quote(candidate.participantEndpointSide), strconv.Quote(candidate.location),
		strconv.Quote(candidate.from), strconv.Quote(candidate.to), strconv.Quote(string(candidate.relation)))
}

// diagramParticipantTypedCandidateRank orders only already-grounded typed
// candidates. The tens digit represents predicate-axis affinity; the units
// digit is a stable relation-family preference within that axis. It is not a
// proof score and never decides whether a candidate exists.
func diagramParticipantTypedCandidateRank(
	axis types.PredicateAxis,
	relation types.DiagramRelationKind,
	anchor types.AnchorKind,
	stageAuthority bool,
) int {
	if stageAuthority {
		return 0
	}
	axisMatch := 1
	for _, allowed := range types.PredicateAxisToAnchorKinds(axis) {
		if allowed == anchor {
			axisMatch = 0
			break
		}
	}
	relationRank := 8
	switch axis {
	case types.AxisCall:
		switch relation {
		case types.DiagramRelCall:
			relationRank = 0
		case types.DiagramRelCallback, types.DiagramRelArgumentFlow:
			relationRank = 1
		case types.DiagramRelReturn:
			relationRank = 2
		}
	case types.AxisRegister:
		switch relation {
		case types.DiagramRelRegister:
			relationRank = 0
		case types.DiagramRelDataFlow, types.DiagramRelAssignment:
			relationRank = 1
		case types.DiagramRelCall:
			relationRank = 2
		}
	case types.AxisReturn:
		if relation == types.DiagramRelReturn {
			relationRank = 0
		}
	case types.AxisConfigure:
		switch relation {
		case types.DiagramRelDataFlow, types.DiagramRelAssignment:
			relationRank = 0
		case types.DiagramRelCall:
			relationRank = 1
		}
	case types.AxisCondition:
		switch relation {
		case types.DiagramRelGuard, types.DiagramRelControlFlow:
			relationRank = 0
		}
	case types.AxisImplement:
		if relation == types.DiagramRelTypeRelation {
			relationRank = 0
		}
	case types.AxisFlow:
		switch relation {
		case types.DiagramRelDataFlow, types.DiagramRelAssignment:
			relationRank = 0
		case types.DiagramRelArgumentFlow:
			relationRank = 1
		case types.DiagramRelReturn, types.DiagramRelCallback:
			relationRank = 2
		case types.DiagramRelCall:
			relationRank = 3
		case types.DiagramRelPrecedence:
			relationRank = 4
		}
	default:
		switch relation {
		case types.DiagramRelCall:
			relationRank = 0
		case types.DiagramRelDataFlow, types.DiagramRelAssignment, types.DiagramRelArgumentFlow:
			relationRank = 1
		case types.DiagramRelReturn, types.DiagramRelCallback, types.DiagramRelPrecedence:
			relationRank = 2
		}
	}
	return 10 + axisMatch*10 + relationRank
}

// diagramParticipantReaderArrowLabel supplies reader wording for an already
// typed immutable relation. It does not infer a relation from prose and it is
// never called to mint, select, reverse, or connect an edge. Containment is a
// no-arrow grouping facet, so it deliberately has no arrow label here.
func diagramParticipantReaderArrowLabel(relation types.DiagramRelationKind, lang string) string {
	zh := strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh")
	if zh {
		switch relation {
		case types.DiagramRelCall:
			return "调用"
		case types.DiagramRelCallback:
			return "传递回调函数（未证明已执行）"
		case types.DiagramRelArgumentFlow:
			return "作为参数传递"
		case types.DiagramRelGuard:
			return "受此条件约束"
		case types.DiagramRelControlFlow:
			return "条件成立时执行"
		case types.DiagramRelImport:
			return "依赖"
		case types.DiagramRelPrecedence:
			return "随后进入"
		case types.DiagramRelTypeRelation:
			return "具有声明的类型关系"
		case types.DiagramRelObserve:
			return "由此观测到"
		case types.DiagramRelRegister:
			return "注册或绑定"
		case types.DiagramRelAssignment:
			return "赋值给"
		case types.DiagramRelDataFlow:
			return "数据写入"
		case types.DiagramRelReturn:
			return "返回"
		case types.DiagramRelTemporal:
			return "随后发生（未证明因果）"
		}
		return ""
	}
	switch relation {
	case types.DiagramRelCall:
		return "invoke"
	case types.DiagramRelCallback:
		return "pass callback (execution not proven)"
	case types.DiagramRelArgumentFlow:
		return "pass as an argument"
	case types.DiagramRelGuard:
		return "governed by this condition"
	case types.DiagramRelControlFlow:
		return "execute when the condition holds"
	case types.DiagramRelImport:
		return "depend on"
	case types.DiagramRelPrecedence:
		return "then continue to"
	case types.DiagramRelTypeRelation:
		return "have a declared type relation to"
	case types.DiagramRelObserve:
		return "observe from"
	case types.DiagramRelRegister:
		return "register or bind"
	case types.DiagramRelAssignment:
		return "assign to"
	case types.DiagramRelDataFlow:
		return "write data to"
	case types.DiagramRelReturn:
		return "return value to"
	case types.DiagramRelTemporal:
		return "occur before (causality unproven)"
	default:
		return ""
	}
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

// DiagramParticipantUnprovenBoundaryShapeComplete reports whether one
// model-authored node-only diagram has discharged every typed
// incident_required participant with the exact explicit no-relation shape:
//
//   - the diagram contains no directed Mermaid edge;
//   - every required participant is visibly present by its exact typed
//     identity; and
//   - every required participant has exactly one status=unproven boundary,
//     with no extra boundary rows.
//
// This is presentation-shape authority only. It does not inspect or replace
// evidence and cannot authorize a relation. The evidence-aware participant
// validator still rejects the boundary as stale when a request-scoped typed
// relation is available. Keeping this predicate beside the participant parser
// ensures the emit-time and orchestrator contracts agree on the same exact
// disconnected-boundary shape instead of demanding an invented edge after the
// model has already disclosed that the requested relation is unproved.
func DiagramParticipantUnprovenBoundaryShapeComplete(
	block *types.AnswerBlock,
	view *types.AnswerSemanticView,
) bool {
	if block == nil || block.Kind != types.BlockDiagram || block.Diagram == nil ||
		strings.TrimSpace(block.Diagram.Body) == "" ||
		len(mermaidcompat.ParseEdges(block.Diagram.Body)) != 0 {
		return false
	}
	obligations := diagramIncidentParticipantObligations(view)
	if len(obligations) == 0 || len(block.ParticipantBoundaries) != len(obligations) {
		return false
	}

	required := make(map[string]string, len(obligations))
	for _, obligation := range obligations {
		identity := strings.TrimSpace(obligation.Identity)
		required[strings.ToLower(identity)] = identity
	}
	seen := make(map[string]bool, len(block.ParticipantBoundaries))
	for _, boundary := range block.ParticipantBoundaries {
		identity := strings.TrimSpace(boundary.Participant)
		key := strings.ToLower(identity)
		exact, ok := required[key]
		if !ok || seen[key] || boundary.Status != types.DiagramParticipantBoundaryUnproven ||
			!strings.EqualFold(identity, exact) {
			return false
		}
		if !diagramParticipantBlockHasVisibleNode(*block, []string{exact}) &&
			!diagramParticipantBlockHasVisibleSubgraph(*block, []string{exact}) {
			return false
		}
		seen[key] = true
	}
	return len(seen) == len(required)
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
	evidence = DiagramEvidenceForValidation(ctx, doc, view, evidence)
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

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
	DiagramParticipantCoverageMissingBoundary  DiagramParticipantCoverageIssue = "missing_unproven_boundary"
	DiagramParticipantCoverageStaleBoundary    DiagramParticipantCoverageIssue = "stale_boundary_for_connected_participant"
	DiagramParticipantCoverageNodeMissing      DiagramParticipantCoverageIssue = "boundary_participant_not_visible"
	DiagramParticipantCoverageUnknownBoundary  DiagramParticipantCoverageIssue = "unknown_or_context_only_boundary"
	DiagramParticipantCoverageDuplicate        DiagramParticipantCoverageIssue = "duplicate_unproven_boundary"
	DiagramParticipantCoverageTypedEdgeMissing DiagramParticipantCoverageIssue = "available_typed_incident_edge_not_rendered"
	DiagramParticipantCoverageIdentityMissing  DiagramParticipantCoverageIssue = "required_participant_identity_not_visible"
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
	expected := "JSON placement: participant_boundaries is block-level, as a sibling of diagram and edge_anchors: {kind:\"diagram\", diagram:{kind,language,body}, participant_boundaries:[...]}; never nest it inside diagram. For every typed incident_required participant with an available evidence-backed relation, render one matching typed visible incident edge authored from that relation; do not replace available evidence with an unproven boundary. The requested participant identity must itself remain visible as a Mermaid node/participant or subgraph/group label; an internal operation endpoint that is merely owned by or statically bound to that participant does not replace the business/component identity. Prefer placing the exact technical endpoint inside that participant's visible group, or use the participant as the visible node label while preserving exact technical identities in edge_anchors. Only when no typed incident relation exists may the participant remain a disconnected visible node with exactly one {participant:<typed identity>,status:\"unproven\"} row. For a disconnected boundary, make the exact typed identity the Mermaid node id or the first visible node label; a short node id whose typed identity appears only in a secondary parenthetical or later multiline label is not that primary identity. Omit participant_boundaries or emit [] when all required participants are connected. Remove stale, unknown, context_only, or already-connected boundary rows. The system does not create or choose an edge: " + strings.Join(parts, "; ")
	if candidates != "" {
		expected += ". Existing typed candidate map (choose one relevant candidate per failed participant; this list is not a requirement to render every candidate and does not create an edge): " + candidates
	}
	return []emitFixHint{{
		Field:               "blocks[kind=diagram].participant_boundaries AND blocks[kind=diagram].diagram.body",
		HardSignal:          preEmitHardSignalTypedDiagramParticipantCoverage,
		OffendingBlockKinds: []types.AnswerBlockKind{types.BlockDiagram},
		ExpectedShape:       expected,
		Reason:              "the typed participant slate is precise completeness authority for participant presence, but never relation evidence; an explicit typed boundary preserves honesty when exploration did not prove an incident relation.",
	}}
}

func DiagramParticipantCoverageMismatches(
	doc *types.AnswerDocumentV2,
	view *types.AnswerSemanticView,
	rm types.RequestModel,
	evidence []types.EvidenceItem,
	stagePrecedence ...stageauthority.PrecedenceRelation,
) []DiagramParticipantCoverageMismatch {
	if doc == nil || view == nil || view.DiagramPlan == nil || !view.DiagramPlan.Required ||
		view.RelationAxis != types.AxisFlow || rm.Intent == types.IntentTrace ||
		types.ResolveQuestionFamily(rm) == types.QFRootCauseTrace {
		return nil
	}
	obligations := diagramIncidentParticipantObligations(view)
	if len(obligations) == 0 || !answerDocumentContainsDiagramPayload(doc) {
		return nil
	}

	type state struct {
		obligation         types.DiagramParticipantHint
		surfaces           []string
		visibleCovered     bool
		identityVisible    bool
		typedEdgeAvailable bool
		bounded            bool
	}
	states := make([]state, 0, len(obligations))
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
		states = append(states, state{
			obligation:         obligation,
			surfaces:           surfaces,
			visibleCovered:     diagramParticipantHasTypedVisibleIncident(doc, surfaces, evidence, stagePrecedence),
			identityVisible:    diagramParticipantIdentityVisible(doc, surfaces, stagePrecedence),
			typedEdgeAvailable: diagramParticipantHasAvailableTypedIncident(rm, obligation, surfaces, evidence, stagePrecedence),
		})
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
			if states[idx].visibleCovered && states[idx].identityVisible {
				out = append(out, DiagramParticipantCoverageMismatch{
					BlockID: block.ID, Participant: states[idx].obligation.Identity,
					Issue: DiagramParticipantCoverageStaleBoundary,
				})
				continue
			}
			if states[idx].typedEdgeAvailable {
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
			if !diagramParticipantBlockHasVisibleNode(block, states[idx].surfaces) {
				out = append(out, DiagramParticipantCoverageMismatch{
					BlockID: block.ID, Participant: states[idx].obligation.Identity,
					Issue: DiagramParticipantCoverageNodeMissing,
				})
			}
		}
	}
	for _, current := range states {
		if (current.visibleCovered && current.identityVisible) || current.bounded {
			continue
		}
		issue := DiagramParticipantCoverageMissingBoundary
		if current.typedEdgeAvailable {
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
	surfaces := []string{strings.TrimSpace(obligation.Identity)}
	surfaces = append(surfaces, types.DiagramParticipantIdentitySurfaces(rm, obligation)...)
	seen := make(map[string]bool)
	rows := make([]string, 0, limit)
	add := func(relation types.DiagramRelationKind, from, to, location string) {
		from, to = strings.TrimSpace(from), strings.TrimSpace(to)
		if len(rows) >= limit || !relation.IsValid() || from == "" || to == "" {
			return
		}
		key := strings.ToLower(string(relation) + "\x00" + from + "\x00" + to + "\x00" + location)
		if seen[key] {
			return
		}
		seen[key] = true
		rows = append(rows, fmt.Sprintf("{relation_kind:%s,from_identity:%s,to_identity:%s,source:%s}",
			strconv.Quote(string(relation)), strconv.Quote(from), strconv.Quote(to), strconv.Quote(location)))
	}
	for _, relation := range stagePrecedence {
		if stageauthority.ParticipantMatchesStageRow(rm, obligation, relation.From) ||
			stageauthority.ParticipantMatchesStageRow(rm, obligation, relation.To) {
			add(types.DiagramRelPrecedence, relation.From.AgentValue, relation.To.AgentValue,
				fmt.Sprintf("%s:%d-%d", relation.SourceFile, relation.LineStart, relation.LineEnd))
		}
	}
	for _, operation := range types.FlowOperationEvidence(evidence) {
		from, to := strings.TrimSpace(operation.Subject), strings.TrimSpace(operation.Object)
		relation := types.RelationForClaimForm(types.ClaimFormOf(operation))
		if operation.AnchorKind == types.AnchorAssignment || operation.AnchorKind == types.AnchorInitializer {
			receiver, value, ok := types.AssignmentEvidenceEndpoints(operation)
			if !ok {
				continue
			}
			relation, from, to = types.DiagramRelDataFlow, value, receiver
		}
		incident := false
		for _, surface := range surfaces {
			if types.AnswerCodeIdentitySurfacesCompatible(surface, from) ||
				types.AnswerCodeIdentitySurfacesCompatible(surface, to) ||
				types.AnswerCodeIdentityOwnsEndpoint(surface, from) ||
				types.AnswerCodeIdentityOwnsEndpoint(surface, to) ||
				types.AnswerCodeIdentityIncidentViaDeclaredBinding(surface, from, operation, evidence) ||
				types.AnswerCodeIdentityIncidentViaDeclaredBinding(surface, to, operation, evidence) {
				incident = true
				break
			}
		}
		if incident {
			add(relation, from, to, operation.DisplayLocation(true))
		}
	}
	return rows
}

func diagramParticipantHasAvailableTypedIncident(
	rm types.RequestModel,
	obligation types.DiagramParticipantHint,
	surfaces []string,
	evidence []types.EvidenceItem,
	stagePrecedence []stageauthority.PrecedenceRelation,
) bool {
	if stageauthority.ParticipantHasIncidentPrecedence(rm, obligation, stagePrecedence) {
		return true
	}
	for _, surface := range surfaces {
		if types.AnswerCodeParticipantHasFlowOperation(surface, evidence) {
			return true
		}
	}
	return false
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
			if diagramParticipantIncidentEndpointMatches(surfaces, from, evidence) || diagramParticipantIncidentEndpointMatches(surfaces, to, evidence) {
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

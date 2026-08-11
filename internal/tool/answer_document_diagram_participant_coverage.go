package tool

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/types"
)

type DiagramParticipantCoverageIssue string

const (
	DiagramParticipantCoverageMissingBoundary DiagramParticipantCoverageIssue = "missing_unproven_boundary"
	DiagramParticipantCoverageStaleBoundary   DiagramParticipantCoverageIssue = "stale_boundary_for_connected_participant"
	DiagramParticipantCoverageNodeMissing     DiagramParticipantCoverageIssue = "boundary_participant_not_visible"
	DiagramParticipantCoverageUnknownBoundary DiagramParticipantCoverageIssue = "unknown_or_context_only_boundary"
	DiagramParticipantCoverageDuplicate       DiagramParticipantCoverageIssue = "duplicate_unproven_boundary"
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
	return []emitFixHint{{
		Field:               "blocks[kind=diagram].participant_boundaries AND blocks[kind=diagram].diagram.body",
		HardSignal:          preEmitHardSignalTypedDiagramParticipantCoverage,
		OffendingBlockKinds: []types.AnswerBlockKind{types.BlockDiagram},
		ExpectedShape:       "JSON placement: participant_boundaries is block-level, as a sibling of diagram and edge_anchors: {kind:\"diagram\", diagram:{kind,language,body}, participant_boundaries:[...]}; never nest it inside diagram. For every typed incident_required participant, keep one evidence-backed typed visible incident edge; when no such relation exists, keep that participant as a disconnected visible node and add exactly one {participant:<typed identity>,status:\"unproven\"} row. For a disconnected boundary, make the exact typed identity the Mermaid node id or the first visible node label; a short node id whose typed identity appears only in a secondary parenthetical or later multiline label is not that primary identity. Omit participant_boundaries or emit [] when all required participants are connected. Remove stale, unknown, context_only, or already-connected boundary rows. Do not invent an edge: " + strings.Join(parts, "; "),
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
		obligation types.DiagramParticipantHint
		surfaces   []string
		covered    bool
		bounded    bool
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
			obligation: obligation,
			surfaces:   surfaces,
			covered:    diagramParticipantHasTypedVisibleIncident(doc, surfaces, evidence, stagePrecedence),
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
			if states[idx].covered {
				out = append(out, DiagramParticipantCoverageMismatch{
					BlockID: block.ID, Participant: states[idx].obligation.Identity,
					Issue: DiagramParticipantCoverageStaleBoundary,
				})
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
		if current.covered || current.bounded {
			continue
		}
		out = append(out, DiagramParticipantCoverageMismatch{
			Participant: current.obligation.Identity,
			Issue:       DiagramParticipantCoverageMissingBoundary,
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
			if diagramParticipantIncidentEndpointMatches(surfaces, from) || diagramParticipantIncidentEndpointMatches(surfaces, to) {
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
func diagramParticipantIncidentEndpointMatches(surfaces []string, endpoint string) bool {
	if diagramParticipantSurfaceMatches(surfaces, endpoint) {
		return true
	}
	for _, surface := range surfaces {
		if types.AnswerCodeIdentityOwnsEndpoint(surface, endpoint) {
			return true
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

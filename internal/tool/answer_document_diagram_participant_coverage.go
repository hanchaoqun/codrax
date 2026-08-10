package tool

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
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
	mismatches := DiagramParticipantCoverageMismatches(doc, view, pctx.ctx.AnalysisIR.RequestModel, evidence)
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
		ExpectedShape:       "for every typed incident_required participant, keep one evidence-backed typed visible incident edge; when no such relation exists, keep that participant as a disconnected visible node and add exactly one {participant:<typed identity>,status:\"unproven\"} row. Omit participant_boundaries or emit [] when all required participants are connected. Remove stale, unknown, context_only, or already-connected boundary rows. Do not invent an edge: " + strings.Join(parts, "; "),
		Reason:              "the typed participant slate is precise completeness authority for participant presence, but never relation evidence; an explicit typed boundary preserves honesty when exploration did not prove an incident relation.",
	}}
}

func DiagramParticipantCoverageMismatches(
	doc *types.AnswerDocumentV2,
	view *types.AnswerSemanticView,
	rm types.RequestModel,
	evidence []types.EvidenceItem,
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
		surfaces := types.DiagramParticipantIdentitySurfaces(rm, obligation)
		if len(surfaces) == 0 {
			surfaces = []string{strings.TrimSpace(obligation.Identity)}
		}
		states = append(states, state{
			obligation: obligation,
			surfaces:   surfaces,
			covered:    diagramParticipantHasTypedVisibleIncident(doc, surfaces, evidence),
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
			matches := make([]int, 0, 1)
			for i := range states {
				if diagramParticipantSurfaceMatches(states[i].surfaces, boundary.Participant) {
					matches = append(matches, i)
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

func diagramParticipantHasTypedVisibleIncident(doc *types.AnswerDocumentV2, surfaces []string, evidence []types.EvidenceItem) bool {
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
			if !diagramHasValidTypedRelation(typedRelations[diagramEvidenceEdgeKey(edge.From, edge.To)]) {
				continue
			}
			from := diagramEvidenceEndpointSymbol(edge.From, labels, evidence)
			to := diagramEvidenceEndpointSymbol(edge.To, labels, evidence)
			if diagramParticipantSurfaceMatches(surfaces, from) || diagramParticipantSurfaceMatches(surfaces, to) {
				return true
			}
		}
	}
	return false
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
		if types.AnswerCodeIdentitySurfacesCompatible(surface, candidate) {
			return true
		}
	}
	return false
}

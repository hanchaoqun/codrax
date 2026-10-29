package tool

import (
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// RuntimeDiagramUnmappedRequestedParticipants reports only an absence of an
// exact requested-identity binding. Labels, role names and adjacency cannot
// create that binding. It does not deny independently verified native arrows.
func RuntimeDiagramUnmappedRequestedParticipants(rows []RuntimeDiagramRelation, rm *types.RequestModel) []string {
	if !runtimeDiagramRowsHaveObservedThreadLanes(rows) || rm == nil || rm.DiagramHint == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, p := range rm.DiagramHint.Participants {
		if p.Role == types.DiagramParticipantIncidentRequired && p.Identity != "" && !seen[p.Identity] &&
			len(diagramParticipantRuntimeRelations(requestedRuntimeParticipantSurfaces(p.Identity, rows), rows)) == 0 {
			out = append(out, p.Identity)
			seen[p.Identity] = true
		}
	}
	return out
}

func runtimeDiagramUnmappedParticipantTeaching(rows []RuntimeDiagramRelation, rm *types.RequestModel) string {
	participants := RuntimeDiagramUnmappedRequestedParticipants(rows, rm)
	if len(participants) == 0 {
		return ""
	}
	data, _ := json.Marshal(participants)
	return fmt.Sprintf("requested_native_identity_unverified=%s: the missing proof is the mapping of these requested business roles to observed threads, not the existence of the separately listed verified event pairs. Keep the unproven participant boundary; do not describe it as missing wakeup evidence or delete proved arrows. For an already-authored lane that independently has an exact verified event anchor, the exact requested identity may occupy its first label line with the complete observed thread label on a second <br/> line; this is presentation only and still requires the unproven role boundary. Preserve that lane's node ID, notes and anchors rather than creating a duplicate. A label alone cannot choose a native thread or certify its business role.\n", data)
}

// Called only after participant coverage accepts the document. This is a
// display receipt, not a new way to satisfy that gate or infer a business role.
func bindNativeDiagramCoverage(doc *types.AnswerDocumentV2, ctx *types.BusContext) {
	if doc == nil || ctx == nil || ctx.AnalysisIR == nil {
		return
	}
	rows := runtimeDiagramRelationsForContext(ctx)
	unmapped := map[string]bool{}
	for _, p := range RuntimeDiagramUnmappedRequestedParticipants(rows, &ctx.AnalysisIR.RequestModel) {
		unmapped[p] = true
	}
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		block.BindNativeDiagramCoverage(nil, 0)
		if block.Kind != types.BlockDiagram || block.Diagram == nil || block.SystemGeneratedKind != types.AnswerSystemGeneratedBlockUnknown {
			continue
		}
		var participants []string
		for _, b := range block.ParticipantBoundaries {
			if b.Status == types.DiagramParticipantBoundaryUnproven && unmapped[b.Participant] {
				participants = append(participants, b.Participant)
			}
		}
		visible := map[string]bool{}
		for _, edge := range mermaidcompat.ParseEdges(block.Diagram.Body) {
			visible[diagramEvidenceEdgeKey(edge.From, edge.To)] = true
		}
		verified := map[[3]string]bool{}
		// A block-local snapshot cannot certify mutable sibling-block legacy
		// anchors. Those keep their existing validators but get no receipt here.
		for _, anchor := range block.EdgeAnchors {
			if visible[diagramEvidenceEdgeKey(anchor.FromNode, anchor.ToNode)] && anchor.HasEndpointIdentityPair() &&
				!runtimeDiagramAnchorAliasConflict(rows, anchor) && runtimeThreadDiagramRelationProved(rows, anchor) {
				verified[[3]string{anchor.FromIdentity, anchor.ToIdentity, string(anchor.RelationKind)}] = true
			}
		}
		block.BindNativeDiagramCoverage(participants, len(verified))
	}
}

func runtimeThreadDiagramRelationProved(rows []RuntimeDiagramRelation, anchor types.DiagramEdgeAnchor) bool {
	for _, row := range rows {
		if row.threadEndpoints != nil && row.Kind == anchor.RelationKind && row.FromIdentity == anchor.FromIdentity && row.ToIdentity == anchor.ToIdentity {
			return true
		}
	}
	return false
}

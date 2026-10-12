package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Native alias binding is an explicit model choice, not recognition of a
// readable name. It reuses a syntax-owned sequence lane and therefore all its
// notes without moving any statement or conferring a business-role mapping.
func projectNativeParticipantBindingBranch(branch map[string]any, base *types.AnswerDocumentV2, c types.AnswerDiagramRelationRepairCandidate) {
	if base == nil {
		return
	}
	sequence := false
	for _, b := range base.Blocks {
		if b.ID == c.BlockID && b.Diagram != nil && b.Diagram.Kind == types.DiagramSequence {
			sequence = true
		}
	}
	if !sequence {
		return
	}
	p, _ := branch["properties"].(map[string]any)
	for _, side := range [][2]string{{"from_native_participant", c.FromNativeParticipant}, {"to_native_participant", c.ToNativeParticipant}} {
		if side[1] != "" {
			p[side[0]] = map[string]any{"type": "string", "enum": []string{side[1]}, "description": "Explicitly bind this side's existing declared sequence node to the selected native participant. Reuses its state notes without deriving identity from labels. Does not prove a requested business role or any additional relation."}
		}
	}
}

func validateAtomicNativeParticipantBinding(block *types.AnswerBlock, edit emitAnswerDiagramEdgeEdit, lease *types.AnswerDiagramRelationRepairLease) error {
	if edit.FromNativeParticipant == "" && edit.ToNativeParticipant == "" {
		return nil
	}
	if edit.Action != "add" || edit.additionCandidate == nil || edit.Edge == nil || lease == nil || block == nil || block.Diagram == nil || block.Diagram.Kind != types.DiagramSequence {
		return fmt.Errorf("native participant binding requires a live native addition_ref on a sequence diagram")
	}
	c := edit.additionCandidate
	for _, side := range [][3]string{{edit.FromNativeParticipant, c.FromNativeParticipant, edit.Edge.FromNode}, {edit.ToNativeParticipant, c.ToNativeParticipant, edit.Edge.ToNode}} {
		if side[0] == "" {
			continue
		}
		if side[1] == "" || side[0] != side[1] {
			return fmt.Errorf("native participant binding is stale or does not match the selected event side")
		}
		count := 0
		for _, d := range diagramExplicitSourceNodeDeclarations(block.Diagram.Body, block.Diagram.Kind) {
			if d.ID == side[2] {
				count++
			}
		}
		if count != 1 {
			return fmt.Errorf("native participant binding requires one exact existing declared node_id=%q", side[2])
		}
	}
	return nil
}

func nativeParticipantBoundCandidate(edit emitAnswerDiagramEdgeEdit) *types.AnswerDiagramRelationRepairCandidate {
	if edit.additionCandidate == nil {
		return nil
	}
	c := *edit.additionCandidate
	if edit.FromNativeParticipant != "" {
		c.FromNodeIDs = append(append([]string(nil), c.FromNodeIDs...), edit.Edge.FromNode)
	}
	if edit.ToNativeParticipant != "" {
		c.ToNodeIDs = append(append([]string(nil), c.ToNodeIDs...), edit.Edge.ToNode)
	}
	return &c
}

// A readable sequence node cannot represent two different producer lanes.
// Repeated event-local endpoints can share it only when the provider supplies
// exactly the same query/source/clock/thread lane. Names never participate.
func runtimeDiagramAliasBindingsConflict(rows []RuntimeDiagramRelation, anchors []types.DiagramEdgeAnchor, node string) bool {
	lane := ""
	for _, a := range anchors {
		for _, r := range rows {
			if r.Kind != a.RelationKind || r.FromIdentity != a.FromIdentity || r.ToIdentity != a.ToIdentity {
				continue
			}
			for _, side := range [][2]string{{a.FromNode, r.FromNode}, {a.ToNode, r.ToNode}} {
				if side[0] != node || !strings.HasPrefix(side[1], "rt_thread_") {
					continue
				}
				if lane != "" && lane != side[1] {
					return true
				}
				lane = side[1]
			}
		}
	}
	return false
}

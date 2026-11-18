package tool

import (
	"fmt"
	"slices"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Native identity is not a Mermaid node ID. All three surfaces use this
// exact, case-sensitive roster from the current sequence declarations.
func nativeBindingLocalSequenceNodes(block *types.AnswerBlock) []string {
	if block == nil || block.Diagram == nil || block.Diagram.Kind != types.DiagramSequence {
		return nil
	}
	declarations := diagramExplicitSourceNodeDeclarations(block.Diagram.Body, block.Diagram.Kind)
	ids := make([]string, 0, len(declarations))
	for _, declaration := range declarations {
		ids = append(ids, declaration.ID)
	}
	return ids
}

// Native alias binding is an explicit model choice, not recognition of a
// readable name. It reuses a syntax-owned sequence lane and therefore all its
// notes without moving any statement or conferring a business-role mapping.
func projectNativeParticipantBindingBranch(branch map[string]any, base *types.AnswerDocumentV2, c types.AnswerDiagramRelationRepairCandidate) {
	if base == nil {
		return
	}
	var block *types.AnswerBlock
	for i := range base.Blocks {
		if base.Blocks[i].ID == c.BlockID {
			if block != nil {
				return
			}
			block = &base.Blocks[i]
		}
	}
	nodes := nativeBindingLocalSequenceNodes(block)
	if len(nodes) == 0 {
		return
	}
	p, _ := branch["properties"].(map[string]any)
	allOf, _ := branch["allOf"].([]any)
	for _, side := range [][2]string{{"from", c.FromNativeParticipant}, {"to", c.ToNativeParticipant}} {
		if side[1] == "" {
			continue
		}
		field := side[0] + "_native_participant"
		p[field] = map[string]any{"type": "string", "enum": []string{side[1]}, "description": "Use only with this side's listed existing diagram node ID in edge. The native ID selects the observed thread, not a diagram node or a business-role mapping."}
		allOf = append(allOf, map[string]any{
			"if": map[string]any{"required": []any{field}},
			"then": map[string]any{"properties": map[string]any{
				"edge": map[string]any{"properties": map[string]any{side[0] + "_node": map[string]any{"enum": nodes}}},
			}},
		})
	}
	if len(allOf) > 0 {
		branch["allOf"] = allOf
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
	nodes := nativeBindingLocalSequenceNodes(block)
	for _, side := range [][3]string{{edit.FromNativeParticipant, c.FromNativeParticipant, edit.Edge.FromNode}, {edit.ToNativeParticipant, c.ToNativeParticipant, edit.Edge.ToNode}} {
		if side[0] == "" {
			continue
		}
		if side[1] == "" || side[0] != side[1] {
			return fmt.Errorf("native participant binding is stale or does not match the selected event side")
		}
		if !slices.Contains(nodes, side[2]) {
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

// A readable sequence node is one lifeline of one recorded thread. It may host
// every published lane of that thread (lane ids differ per lookup and recorded
// precision), never lanes of two different threads. A lane without typed
// thread endpoints is only ever the same as itself. Names never participate.
func runtimeDiagramAliasBindingsConflict(rows []RuntimeDiagramRelation, anchors []types.DiagramEdgeAnchor, node string) bool {
	return len(runtimeDiagramLifelineOddSides(rows, runtimeDiagramThreadGroups(rows), anchors, node)) > 0
}

package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

var nativeParticipantNodeID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func runtimeNativeParticipantLanes(rows []RuntimeDiagramRelation) map[string]string {
	lanes := map[string]string{}
	for _, r := range rows {
		for _, side := range [][2]string{{r.FromNode, r.FromLabel}, {r.ToNode, r.ToLabel}} {
			if strings.HasPrefix(side[0], "rt_thread_") {
				lanes[side[0]] = side[1]
			}
		}
	}
	return lanes
}

// Reselection repairs only a unique, already declared selector whose native
// lane is absent from the current provider. A display edit never infers its
// replacement from an ID resemblance, label, requested role, or graph shape.
// Both schema and executor use this exact roster; valid selectors are not
// delegated to this local repair capability.
func runtimeNativeParticipantReselectionNodes(block types.AnswerBlock, rows []RuntimeDiagramRelation) map[string]int {
	eligible := map[string]int{}
	if block.Kind != types.BlockDiagram || block.Diagram == nil || block.Diagram.Kind != types.DiagramSequence || len(block.Diagram.RuntimeParticipantsV1) > 64 {
		return eligible
	}
	lanes := runtimeNativeParticipantLanes(rows)
	if len(lanes) == 0 {
		return eligible
	}
	declared, selected := map[string]int{}, map[string]int{}
	for _, d := range diagramExplicitSourceNodeDeclarations(block.Diagram.Body, block.Diagram.Kind) {
		declared[d.ID]++
	}
	for _, s := range block.Diagram.RuntimeParticipantsV1 {
		selected[s.NodeID]++
	}
	for i, s := range block.Diagram.RuntimeParticipantsV1 {
		if _, current := lanes[s.NativeParticipant]; !current && nativeParticipantNodeID.MatchString(s.NodeID) && declared[s.NodeID] == 1 && selected[s.NodeID] == 1 {
			eligible[s.NodeID] = i
		}
	}
	return eligible
}

// Extend only visibility branches that survived the ordinary lease projection.
// This does not create a lease, widen whole-block replacement, or expose a
// participant operation during an orphan/relation-only phase.
func projectNativeParticipantReselectionSchema(raw json.RawMessage, prev *types.AnswerDocumentV2, lease *types.AnswerDiagramRelationRepairLease, rows []RuntimeDiagramRelation) json.RawMessage {
	if prev == nil || lease == nil || lease.OrphanDispositionOnly || len(rows) == 0 {
		return raw
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return raw
	}
	props, _ := root["properties"].(map[string]any)
	edits, _ := props["diagram_participant_edits"].(map[string]any)
	items, _ := edits["items"].(map[string]any)
	branches, _ := items["oneOf"].([]any)
	if len(branches) == 0 {
		return raw
	}
	visibleRefs := map[string]bool{}
	for _, branch := range branches {
		b, _ := branch.(map[string]any)
		p, _ := b["properties"].(map[string]any)
		a, _ := p["action"].(map[string]any)
		r, _ := p["participant_ref"].(map[string]any)
		actions, refs := schemaStringValues(a["enum"]), schemaStringValues(r["enum"])
		if len(actions) == 1 && actions[0] == "ensure_visible" && len(refs) == 1 {
			visibleRefs[refs[0]] = true
		}
	}
	lanes := runtimeNativeParticipantLanes(rows)
	natives := make([]string, 0, len(lanes))
	for native := range lanes {
		natives = append(natives, native)
	}
	sort.Strings(natives)
	added := false
	for _, f := range lease.ParticipantVisibilityFailures {
		if !visibleRefs[f.ParticipantRef] {
			continue
		}
		var block types.AnswerBlock
		count := 0
		for _, b := range prev.Blocks {
			if b.ID == f.BlockID {
				block, count = b, count+1
			}
		}
		if count != 1 || f.BaseDiagramFingerprint == "" || types.AnswerDiagramParticipantVisibilityFingerprint(block) != f.BaseDiagramFingerprint {
			continue
		}
		var nodes []string
		for node := range runtimeNativeParticipantReselectionNodes(block, rows) {
			nodes = append(nodes, node)
		}
		if len(nodes) == 0 {
			continue
		}
		sort.Strings(nodes)
		branches = append(branches, map[string]any{
			"type": "object", "additionalProperties": false,
			"description": fmt.Sprintf("Explicitly reselect one listed unavailable native binding while making participant %q visible. Choose a current native thread ID; only that selector and declaration label change, not messages, notes, anchors, other selectors, or business-role/relationship authority. Without native_participant, ensure_visible never changes a selector.", f.Participant),
			"properties": map[string]any{
				"participant_ref":    map[string]any{"type": "string", "enum": []any{f.ParticipantRef}},
				"action":             map[string]any{"type": "string", "enum": []any{"ensure_visible"}},
				"node_id":            map[string]any{"type": "string", "enum": stringsToAny(nodes)},
				"native_participant": map[string]any{"type": "string", "enum": stringsToAny(natives)},
				"visible_label":      map[string]any{"type": "string", "minLength": 1, "maxLength": 512},
			},
			"required": []any{"participant_ref", "action", "node_id", "native_participant", "visible_label"},
		})
		added = true
	}
	if !added {
		return raw
	}
	items["oneOf"] = branches
	out, err := json.Marshal(root)
	if err != nil {
		return raw
	}
	return out
}

func projectNativeDiagramParticipants(blockProps map[string]any, view *types.AnswerSemanticView, rows ...[]RuntimeDiagramRelation) {
	diagram, _ := blockProps["diagram"].(map[string]any)
	props, _ := diagram["properties"].(map[string]any)
	if props == nil {
		return
	}
	delete(props, "runtime_participants_v1")
	if view == nil || view.DiagramPlan == nil || view.DiagramPlan.Kind != types.DiagramSequence || len(rows) != 1 {
		return
	}
	lanes := runtimeNativeParticipantLanes(rows[0])
	ids := make([]string, 0, len(lanes))
	for id := range lanes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var choices []any
	for _, id := range ids {
		choices = append(choices, map[string]any{"type": "object", "additionalProperties": false, "required": []string{"node_id", "native_participant"}, "description": fmt.Sprintf("Current observed thread %q. Display choice only, not a business-role mapping or causal permission.", lanes[id]), "properties": map[string]any{
			"node_id":            map[string]any{"type": "string", "pattern": `^[A-Za-z_][A-Za-z0-9_]*$`, "description": "One existing uniquely declared participant. The first choice for this native thread becomes its shared display lane; its notes keep their original wording, which is not certified as a native fact."},
			"native_participant": map[string]any{"type": "string", "enum": []string{id}},
		}})
	}
	if len(choices) == 0 {
		return
	}
	props["runtime_participants_v1"] = map[string]any{"type": "array", "maxItems": 64, "items": map[string]any{"oneOf": choices}, "description": "Explicitly reuse existing sequence participants for current native threads, including note-only lanes. The system puts exactly bound messages and notes on one lane. Select every duplicate alias you intend to merge; labels alone never bind an unselected node. This does not prove note contents, requested business roles or extra edges. Stale selections must be reselected from current choices."}
}

// The map is block-local display identity. It accepts only current native
// endpoints, complete validated edge tuples, and an explicit display selection.
// It never derives an identity from labels, note prose or a requested role.
func runtimeDiagramParticipantBindings(block types.AnswerBlock, rows []RuntimeDiagramRelation) (map[string]string, error) {
	lanes := runtimeNativeParticipantLanes(rows)
	bindings := map[string]string{}
	bind := func(node, lane string) error {
		if previous := bindings[node]; previous != "" && previous != lane {
			return fmt.Errorf("participant %q selects conflicting native threads", node)
		}
		bindings[node] = lane
		return nil
	}
	for lane := range lanes {
		bindings[lane] = lane
	}
	for _, a := range block.EdgeAnchors {
		for _, r := range rows {
			if a.RelationKind != r.Kind || a.FromIdentity != r.FromIdentity || a.ToIdentity != r.ToIdentity {
				continue
			}
			for _, side := range [][2]string{{a.FromNode, r.FromNode}, {a.ToNode, r.ToNode}} {
				if _, ok := lanes[side[1]]; ok {
					if err := bind(side[0], side[1]); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	if block.Diagram == nil {
		return bindings, nil
	}
	selections := block.Diagram.RuntimeParticipantsV1
	if len(selections) > 64 || len(selections) > 0 && block.Diagram.Kind != types.DiagramSequence {
		return nil, fmt.Errorf("runtime_participants_v1 requires a sequence diagram and at most 64 current choices")
	}
	declared := map[string]int{}
	for _, d := range diagramExplicitSourceNodeDeclarations(block.Diagram.Body, block.Diagram.Kind) {
		declared[d.ID]++
	}
	seen := map[string]bool{}
	for _, s := range selections {
		if _, ok := lanes[s.NativeParticipant]; !ok {
			return nil, fmt.Errorf("runtime_participants_v1: native participant %q is unavailable or stale", s.NativeParticipant)
		}
		if !nativeParticipantNodeID.MatchString(s.NodeID) || declared[s.NodeID] != 1 || seen[s.NodeID] {
			return nil, fmt.Errorf("runtime_participants_v1: node_id=%q must select one unique existing declaration once", s.NodeID)
		}
		seen[s.NodeID] = true
		if err := bind(s.NodeID, s.NativeParticipant); err != nil {
			return nil, err
		}
	}
	return bindings, nil
}

// Validate every selected block before committing any change. Retained wire
// selectors are rebound on every emit/patch, including JSON-restored drafts.
func normalizeNativeDiagramParticipants(doc *types.AnswerDocumentV2, rows []RuntimeDiagramRelation, requests ...*types.RequestModel) (int, error) {
	if doc == nil {
		return 0, nil
	}
	changed := map[int]types.AnswerBlock{}
	for i, b := range doc.Blocks {
		if b.Diagram == nil || len(b.Diagram.RuntimeParticipantsV1) == 0 {
			continue
		}
		b = cloneAtomicDiagramPatchBlock(b)
		bindings, err := runtimeDiagramParticipantBindings(b, rows)
		if err != nil {
			return 0, fmt.Errorf("block %q: %w", b.ID, err)
		}
		if len(requests) == 1 && requests[0] != nil && requests[0].DiagramHint != nil {
			for _, s := range b.Diagram.RuntimeParticipantsV1 {
				for _, p := range requests[0].DiagramHint.Participants {
					if p.Identity == s.NodeID && !requestedRuntimeParticipantNode(p.Identity, s.NativeParticipant, rows) {
						return 0, fmt.Errorf("block %q: display selection for requested participant %q has no exact request-to-native binding", b.ID, p.Identity)
					}
				}
			}
		}
		representatives := map[string]string{}
		var selections []types.RuntimeDiagramParticipantSelection
		for _, s := range b.Diagram.RuntimeParticipantsV1 {
			if representatives[s.NativeParticipant] == "" {
				representatives[s.NativeParticipant] = s.NodeID
				selections = append(selections, s)
			}
		}
		rewrite := map[string]string{}
		for node, lane := range bindings {
			if to := representatives[lane]; to != "" && node != to {
				rewrite[node] = to
			}
		}
		body, lines, err := rewriteNativeSequenceParticipants(b.Diagram.Body, rewrite)
		if err != nil {
			return 0, fmt.Errorf("block %q: %w", b.ID, err)
		}
		for j := range b.EdgeAnchors {
			a := &b.EdgeAnchors[j]
			if to := rewrite[a.FromNode]; to != "" {
				a.FromNode = to
			}
			if to := rewrite[a.ToNode]; to != "" {
				a.ToNode = to
			}
		}
		for j := range b.Diagram.RuntimeNotesV1 {
			n := &b.Diagram.RuntimeNotesV1[j]
			if n.BodyLine <= 0 || lines[n.BodyLine] == 0 {
				return 0, fmt.Errorf("block %q: selected native Note line is unavailable", b.ID)
			}
			n.BodyLine = lines[n.BodyLine]
			if to := rewrite[n.NodeID]; to != "" {
				n.NodeID = to
			}
		}
		b.Diagram.Body, b.Diagram.RuntimeParticipantsV1 = body, selections
		if !reflect.DeepEqual(doc.Blocks[i], b) {
			changed[i] = b
		}
	}
	for i, b := range changed {
		doc.Blocks[i] = b
	}
	return len(changed), nil
}

func preCheckNativeDiagramParticipants(doc *types.AnswerDocumentV2, ctx *types.BusContext) []emitFixHint {
	if _, err := normalizeNativeDiagramParticipants(doc, runtimeDiagramRelationsForContext(ctx), nativeDiagramParticipantRequest(ctx)); err != nil {
		return []emitFixHint{{Field: "diagram.runtime_participants_v1", ExpectedShape: "Select only current native_participant choices and unique existing sequence node_id declarations. Keep different native threads separate. Use an exact current schema branch to explicitly reselect an unavailable binding; display-only ensure_visible does not change selectors. Re-emit or replace this diagram only when the current schema permits it. Labels and Note text cannot supply identity.", Reason: err.Error(), ForceHard: true, HardSignal: preEmitHardSignalExactReceiptBinding}}
	}
	return nil
}

func nativeDiagramParticipantRequest(ctx *types.BusContext) *types.RequestModel {
	if ctx != nil && ctx.AnalysisIR != nil {
		return &ctx.AnalysisIR.RequestModel
	}
	return nil
}

package tool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	maxModelAuthoredDiagramRelationScopeEdits = 16

	answerDiagramRelationScopeActionSet    = "set_partial_unproven"
	answerDiagramRelationScopeActionRemove = "remove_scope"
)

type answerDiagramRelationScopeEditCapability struct {
	BlockID string
	Action  string
}

func answerDiagramRelationScopeEditMaxItems(capabilities []answerDiagramRelationScopeEditCapability) int {
	if len(capabilities) == 0 {
		return 0
	}
	for _, capability := range capabilities {
		if capability.Action == answerDiagramRelationScopeActionSet {
			// A missing whole-relation disclosure needs exactly one model-owned
			// carrier. Multiple branches are alternative diagram choices, not a
			// roster to apply together.
			return 1
		}
	}
	// Stale/duplicate declarations are independent exact removals and may be
	// repaired transactionally in one patch.
	return len(capabilities)
}

// answerDocumentPatchRelationScopeCapabilities projects only typed
// request-spine mismatches into model-selectable local field edits. It never
// reads request/answer prose or Mermaid labels, and the model still chooses
// whether and where to apply one of the exact branches.
func answerDocumentPatchRelationScopeCapabilities(
	ctx *types.BusContext,
	doc *types.AnswerDocumentV2,
	view *types.AnswerSemanticView,
) []answerDiagramRelationScopeEditCapability {
	if ctx == nil || ctx.AnalysisIR == nil || doc == nil || view == nil {
		return nil
	}
	evidence := DiagramEvidenceForValidation(ctx, doc, view, nil)
	evidence = preEmitEvidenceWithExactTypedDiagramRelations(doc, ctx, evidence)
	mismatches := DiagramRequestedRelationScopeMismatches(
		doc,
		view,
		ctx.AnalysisIR.RequestModel,
		evidence,
		diagramVerifiedReadModeStagePrecedence(ctx, view)...,
	)
	if len(mismatches) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	out := make([]answerDiagramRelationScopeEditCapability, 0, len(mismatches))
	appendCapability := func(blockID, action string) {
		blockID = strings.TrimSpace(blockID)
		if blockID == "" || action == "" {
			return
		}
		key := blockID + "\x00" + action
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, answerDiagramRelationScopeEditCapability{BlockID: blockID, Action: action})
	}
	for _, mismatch := range mismatches {
		switch mismatch.Issue {
		case DiagramRequestedRelationScopeStale, DiagramRequestedRelationScopeDuplicate:
			appendCapability(mismatch.BlockID, answerDiagramRelationScopeActionRemove)
		case DiagramRequestedRelationScopeMissing:
			// Missing scope does not preselect a presentation block. Publish one
			// exact branch per existing diagram and let the model choose; the
			// ordinary typed scope gate validates that exactly one declaration
			// lands on the requested relation surface.
			for _, block := range doc.Blocks {
				if block.Kind == types.BlockDiagram && block.Diagram != nil &&
					strings.TrimSpace(block.ID) != "" {
					appendCapability(block.ID, answerDiagramRelationScopeActionSet)
				}
			}
		}
	}
	return out
}

func agentBusContextForAnswerPatchScope(ctx *types.AgentContext) *types.BusContext {
	if ctx == nil {
		return nil
	}
	return &types.BusContext{
		Mode:          ctx.Mode,
		RepoRoot:      ctx.RepoRoot,
		MainRepoRoot:  ctx.MainRepoRoot,
		Branch:        ctx.Branch,
		Commit:        ctx.Commit,
		WorkDir:       ctx.WorkDir,
		AnalysisIR:    ctx.AnalysisIR,
		Mutable:       ctx.Mutable,
		EvidenceItems: append([]types.EvidenceItem(nil), ctx.EvidenceItems...),
		FlowFindings:  append([]types.FlowFindingDigest(nil), ctx.FlowFindings...),
		AnswerChains:  append([]types.AnswerChain(nil), ctx.AnswerChains...),
		MCPResponses:  append([]types.MCPResponse(nil), ctx.MCPResponses...),
	}
}

// projectAnswerDocumentPatchRelationScopeEdits narrows the schema to the
// current typed mismatch. No generic scope-edit surface is left active when
// the current patch base already agrees with request-spine authority.
func projectAnswerDocumentPatchRelationScopeEdits(
	raw json.RawMessage,
	ctx *types.AgentContext,
	prev *types.AnswerDocumentV2,
) json.RawMessage {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return raw
	}
	properties, _ := root["properties"].(map[string]any)
	if properties == nil {
		return raw
	}
	field, _ := properties["diagram_relation_scope_edits"].(map[string]any)
	if field == nil {
		return raw
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	capabilities := answerDocumentPatchRelationScopeCapabilities(
		agentBusContextForAnswerPatchScope(ctx), prev, view,
	)
	if len(capabilities) == 0 {
		delete(properties, "diagram_relation_scope_edits")
	} else {
		branches := make([]any, 0, len(capabilities))
		for _, capability := range capabilities {
			branches = append(branches, map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"block_id": map[string]any{"type": "string", "enum": []any{capability.BlockID}},
					"action":   map[string]any{"type": "string", "enum": []any{capability.Action}},
				},
				"required": []any{"block_id", "action"},
			})
		}
		field["minItems"] = 1
		field["maxItems"] = answerDiagramRelationScopeEditMaxItems(capabilities)
		field["uniqueItems"] = true
		field["items"] = map[string]any{"oneOf": branches}
		field["description"] = "Choose one exact current block_id/action branch to repair only requested_relation_scope. The current typed request-spine mismatch is the sole authority. No Mermaid line, edge, relation, label, layout, or conclusion is changed."
	}
	out, err := json.Marshal(root)
	if err != nil || !json.Valid(out) {
		return raw
	}
	return out
}

// applyModelAuthoredDiagramRelationScopeEdits compiles model-selected scope
// actions into the same full-block replacements used by other atomic diagram
// edits. If another atomic operation already created a replacement, this
// function changes only its typed scope field; otherwise it clones the exact
// previous block. Every action is revalidated against the live typed mismatch.
func applyModelAuthoredDiagramRelationScopeEdits(
	prev *types.AnswerDocumentV2,
	patch *types.AnswerDocumentV2Patch,
	edits []emitAnswerDiagramRelationScopeEdit,
	ctx *types.BusContext,
) error {
	if len(edits) == 0 {
		return nil
	}
	if len(edits) > maxModelAuthoredDiagramRelationScopeEdits {
		return fmt.Errorf("too many diagram relation-scope edits: got %d, max %d", len(edits), maxModelAuthoredDiagramRelationScopeEdits)
	}
	if prev == nil || patch == nil || ctx == nil {
		return fmt.Errorf("previous answer, patch, and runtime context are required")
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	capabilities := answerDocumentPatchRelationScopeCapabilities(ctx, prev, view)
	allowed := make(map[string]bool, len(capabilities))
	for _, capability := range capabilities {
		allowed[capability.BlockID+"\x00"+capability.Action] = true
	}
	if len(allowed) == 0 {
		return fmt.Errorf("no live typed requested-relation scope mismatch permits a local scope edit")
	}
	setCount := 0
	for _, edit := range edits {
		if strings.TrimSpace(edit.Action) == answerDiagramRelationScopeActionSet {
			setCount++
		}
	}
	if setCount > 0 && (setCount != 1 || len(edits) != 1) {
		return fmt.Errorf("missing requested-relation scope requires exactly one model-selected diagram carrier")
	}
	previous := make(map[string]types.AnswerBlock, len(prev.Blocks))
	ambiguous := make(map[string]bool)
	for _, block := range prev.Blocks {
		id := strings.TrimSpace(block.ID)
		if id == "" {
			continue
		}
		if _, exists := previous[id]; exists {
			ambiguous[id] = true
			continue
		}
		previous[id] = block
	}
	seen := make(map[string]bool, len(edits))
	for i, edit := range edits {
		blockID := strings.TrimSpace(edit.BlockID)
		action := strings.TrimSpace(edit.Action)
		key := blockID + "\x00" + action
		if blockID == "" || action == "" || !allowed[key] {
			return fmt.Errorf("diagram_relation_scope_edits[%d] block_id=%q action=%q is stale or not permitted by the current typed mismatch", i, blockID, action)
		}
		if seen[blockID] {
			return fmt.Errorf("diagram_relation_scope_edits[%d] duplicates block_id=%q", i, blockID)
		}
		seen[blockID] = true
		if ambiguous[blockID] {
			return fmt.Errorf("diagram_relation_scope_edits[%d] block_id=%q is ambiguous", i, blockID)
		}

		var target *types.AnswerBlock
		for j := range patch.ReplaceBlocks {
			if strings.TrimSpace(patch.ReplaceBlocks[j].ID) == blockID {
				if target != nil {
					return fmt.Errorf("diagram_relation_scope_edits[%d] block_id=%q has duplicate replacement carriers", i, blockID)
				}
				target = &patch.ReplaceBlocks[j]
			}
		}
		if target == nil {
			base, ok := previous[blockID]
			if !ok || base.Kind != types.BlockDiagram || base.Diagram == nil {
				return fmt.Errorf("diagram_relation_scope_edits[%d] block_id=%q is not one existing diagram", i, blockID)
			}
			for _, id := range patch.RemoveBlockIDs {
				if strings.TrimSpace(id) == blockID {
					return fmt.Errorf("diagram_relation_scope_edits[%d] block_id=%q conflicts with remove_block_ids", i, blockID)
				}
			}
			patch.ReplaceBlocks = append(patch.ReplaceBlocks, cloneAtomicDiagramPatchBlock(base))
			target = &patch.ReplaceBlocks[len(patch.ReplaceBlocks)-1]
		}
		if target.Kind != types.BlockDiagram || target.Diagram == nil {
			return fmt.Errorf("diagram_relation_scope_edits[%d] block_id=%q is not a diagram carrier", i, blockID)
		}
		switch action {
		case answerDiagramRelationScopeActionSet:
			target.RequestedRelationScope = types.DiagramRelationScopePartialUnproven
		case answerDiagramRelationScopeActionRemove:
			target.RequestedRelationScope = types.DiagramRelationScopeUnknown
		default:
			return fmt.Errorf("diagram_relation_scope_edits[%d] action=%q is invalid", i, action)
		}
	}
	if len(seen) > 0 && len(patch.UnchangedBlockIDs) > 0 {
		kept := patch.UnchangedBlockIDs[:0]
		for _, id := range patch.UnchangedBlockIDs {
			if seen[strings.TrimSpace(id)] {
				continue
			}
			kept = append(kept, id)
		}
		patch.UnchangedBlockIDs = kept
	}
	return nil
}

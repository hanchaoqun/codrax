package tool

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/types"
)

// BuildAnswerDocumentParametersFor projects the canonical
// emit_answer_document JSON schema onto the per-dispatch
// AnswerSemanticView. Conditional fields and enum subsets disappear
// when the view says this dispatch will not need them, so the LLM
// only sees what the contract actually expects.
//
// Projections:
//   - block.kind enum is restricted to view.RequiredBlocks ∪
//     view.OptionalBlocks kinds (canonical set when both are empty).
//   - block.diagram is dropped entirely when view.DiagramPlan is nil.
//     When present, diagram.kind is pinned to view.DiagramPlan.Kind.
//   - block.edge_anchors is dropped entirely when view.DiagramPlan
//     is nil (no diagram block can be emitted, so edge anchors have
//     nothing to anchor to).
//   - block.claim_uses[].claim_form enum is restricted to the union
//     of every BlockRequirement's AcceptableClaimForms (canonical
//     set when none are declared).
//   - missing_requested_roles is dropped when
//     len(view.MissingRequestedRoles) == 0.
//   - exact_resolution is dropped when view.ExactResolution is nil.
//   - All other fields and the description prose stay byte-identical
//     to the canonical schema.
//
// view==nil falls back to the canonical static schema so test
// callers and any future codepath without a compiled view still
// see the full surface.
func BuildAnswerDocumentParametersFor(view *types.AnswerSemanticView) json.RawMessage {
	canonical := (&EmitAnswerDocument{}).canonicalParameters()
	if view == nil {
		return canonical
	}
	var root map[string]any
	if err := json.Unmarshal(canonical, &root); err != nil {
		return canonical
	}
	properties, _ := root["properties"].(map[string]any)
	if properties == nil {
		return canonical
	}

	// Block-level field set on each blocks[] item.
	blocksField, _ := properties["blocks"].(map[string]any)
	if blockItems, ok := blocksField["items"].(map[string]any); ok {
		if blockProps, ok := blockItems["properties"].(map[string]any); ok {
			projectBlockKindEnum(blockProps, view)
			projectClaimUsesEnum(blockProps, view)
			projectDiagramField(blockProps, view)
			projectEdgeAnchorsField(blockProps, view)
		}
	}

	// Document-level conditional fields.
	if view.ExactResolution == nil {
		delete(properties, "exact_resolution")
	}
	if len(view.MissingRequestedRoles) == 0 {
		delete(properties, "missing_requested_roles")
	}

	out, err := json.Marshal(root)
	if err != nil {
		return canonical
	}
	return out
}

// projectBlockKindEnum restricts the block.kind enum to the kinds
// declared in view.RequiredBlocks ∪ view.OptionalBlocks. When the
// view declares no kinds at all, the enum is left at the canonical
// full list so the schema stays usable.
func projectBlockKindEnum(blockProps map[string]any, view *types.AnswerSemanticView) {
	kindField, _ := blockProps["kind"].(map[string]any)
	if kindField == nil {
		return
	}
	seen := make(map[string]bool, 8)
	for _, br := range view.RequiredBlocks {
		if br.Kind != "" {
			seen[string(br.Kind)] = true
		}
	}
	for _, br := range view.OptionalBlocks {
		if br.Kind != "" {
			seen[string(br.Kind)] = true
		}
	}
	if len(seen) == 0 {
		return
	}
	// Iterate the canonical list to preserve order.
	enum := make([]string, 0, len(seen))
	for _, k := range types.AllAnswerBlockKinds() {
		if seen[string(k)] {
			enum = append(enum, string(k))
		}
	}
	kindField["enum"] = enum
}

// projectClaimUsesEnum restricts the claim_uses[].claim_form enum
// to the union of AcceptableClaimForms declared by any
// BlockRequirement. When no requirement declares forms, the enum
// stays at the canonical full list.
func projectClaimUsesEnum(blockProps map[string]any, view *types.AnswerSemanticView) {
	claimUsesField, _ := blockProps["claim_uses"].(map[string]any)
	if claimUsesField == nil {
		return
	}
	seen := make(map[types.ClaimForm]bool, 9)
	for _, br := range view.RequiredBlocks {
		for _, f := range br.AcceptableClaimForms {
			if f != "" {
				seen[f] = true
			}
		}
	}
	for _, br := range view.OptionalBlocks {
		for _, f := range br.AcceptableClaimForms {
			if f != "" {
				seen[f] = true
			}
		}
	}
	if len(seen) == 0 {
		return
	}
	enum := make([]string, 0, len(seen))
	for _, f := range types.AllClaimForms() {
		if seen[f] {
			enum = append(enum, string(f))
		}
	}
	// The description string carries the inline enum list verbatim;
	// keep it as-is to preserve LLM teaching, but rewrite the JSON
	// items[].properties.claim_form.enum if such a node exists. The
	// canonical schema declares claim_uses as a typed array but
	// leaves the inner shape opaque (description prose carries the
	// enum); injecting an items.properties.claim_form.enum lets a
	// strict-mode schema reader see the projected list while keeping
	// the description backward-compatible.
	itemsNode, ok := claimUsesField["items"].(map[string]any)
	if !ok {
		itemsNode = map[string]any{"type": "object"}
		claimUsesField["items"] = itemsNode
	}
	innerProps, ok := itemsNode["properties"].(map[string]any)
	if !ok {
		innerProps = map[string]any{}
		itemsNode["properties"] = innerProps
	}
	innerProps["claim_form"] = map[string]any{
		"type": "string",
		"enum": enum,
	}
}

// projectDiagramField drops the block.diagram payload entirely when
// the view declares no diagram contract. When a diagram is required,
// pin diagram.kind to the view-declared value so the LLM cannot
// pick the wrong semantic family.
func projectDiagramField(blockProps map[string]any, view *types.AnswerSemanticView) {
	if view.DiagramPlan == nil {
		delete(blockProps, "diagram")
		return
	}
	diagramField, _ := blockProps["diagram"].(map[string]any)
	if diagramField == nil {
		return
	}
	innerProps, _ := diagramField["properties"].(map[string]any)
	if innerProps == nil {
		return
	}
	kindField, _ := innerProps["kind"].(map[string]any)
	if kindField == nil {
		return
	}
	if dk := view.DiagramPlan.Kind; dk != "" {
		kindField["enum"] = []string{string(dk)}
	}
}

// projectEdgeAnchorsField drops block.edge_anchors entirely when
// the view declares no diagram contract — without a diagram, edge
// anchors have nothing to anchor to.
func projectEdgeAnchorsField(blockProps map[string]any, view *types.AnswerSemanticView) {
	if view.DiagramPlan == nil {
		delete(blockProps, "edge_anchors")
	}
}

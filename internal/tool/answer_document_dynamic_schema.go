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
//     view.OptionalBlocks ∪ view.Presentation.AllowedBlocks kinds
//     (canonical set when all are empty).
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
			projectTypedDecisionVerdictFields(blockProps, blockItems, view)
		}
		projectKindPayloadConditionals(blockItems, view)
	}
	projectRequiredBlockArrayCardinality(blocksField, view)

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
// declared in view.RequiredBlocks ∪ view.OptionalBlocks ∪
// view.Presentation.AllowedBlocks. When the view declares no kinds at
// all, the enum is left at the canonical full list so the schema stays usable.
func projectBlockKindEnum(blockProps map[string]any, view *types.AnswerSemanticView) {
	kindField, _ := blockProps["kind"].(map[string]any)
	if kindField == nil {
		return
	}
	seen := allowedKindSet(view)
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
	if presentationAddsUnconstrainedClaimCarrier(view) {
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

func presentationAddsUnconstrainedClaimCarrier(view *types.AnswerSemanticView) bool {
	if view == nil || len(view.Presentation.AllowedBlocks) == 0 {
		return false
	}
	contractKinds := make(map[types.AnswerBlockKind]bool, len(view.RequiredBlocks)+len(view.OptionalBlocks))
	for _, br := range view.RequiredBlocks {
		if br.Kind != "" {
			contractKinds[br.Kind] = true
		}
	}
	for _, br := range view.OptionalBlocks {
		if br.Kind != "" {
			contractKinds[br.Kind] = true
		}
	}
	for _, kind := range view.Presentation.AllowedBlocks {
		if kind == "" {
			continue
		}
		if kind == types.BlockDiagram && view.DiagramPlan == nil {
			continue
		}
		if !contractKinds[kind] {
			return true
		}
	}
	return false
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
	// Inside the diagram object: kind + body are load-bearing; mark
	// them required so the LLM cannot emit `{}` for the payload.
	// language defaults to "mermaid" downstream; not required.
	diagramField["required"] = []string{"kind", "body"}
}

// blockKindPayloadField maps each block kind to the load-bearing
// payload field the LLM MUST fill on that kind. The runtime
// normalize / validator layers expect these fields populated; making
// them schema-required teaches the LLM the per-kind shape contract
// up front instead of waiting for a downstream reject.
//
//   - summary / section / scalar / decision / caveat → block.text
//     (the prose / literal / verdict / scope-note carrier)
//   - ordered_list / bullet_list → block.items[]
//     (rows / list entries — empty list is a structural failure)
//   - diagram → block.diagram (the {kind, language, body} payload
//     object — diagram body never goes in block.text)
//
// Table is intentionally omitted: it has three compatible visible carriers
// (block.text markdown table, columns[] + items[].cells[] structured rows,
// and legacy label/text rows). Requiring one fixed field at schema level made
// valid markdown-table emits fail before the renderer could preserve them.
var blockKindPayloadField = map[string]string{
	"summary":      "text",
	"section":      "text",
	"scalar":       "text",
	"decision":     "text",
	"caveat":       "text",
	"ordered_list": "items",
	"bullet_list":  "items",
	"diagram":      "diagram",
}

// projectKindPayloadConditionals teaches the schema, via JSON Schema's
// allOf+if/then construct, that each block-kind requires its load-
// bearing payload field. The block.required array stays at
// ["id", "kind"] (those are universal); the conditional then-clause
// adds the per-kind field. Only kinds present in the projected enum
// (i.e. the view's allowed RequiredBlocks ∪ OptionalBlocks) get
// conditionals — projecting irrelevant kinds wastes prompt budget.
//
// Pre-fix only kind=diagram had a normalize-layer hard reject for
// missing payload (and the customer reported it as a frequent retry
// cause). Other kinds silently produced empty / broken renders.
// Centralising the requirement here makes the per-kind contract
// uniform: the LLM sees up front what each kind expects, the
// runtime normalize / validators have less to catch later, and
// retries on "I forgot the payload" disappear from the budget.
func projectKindPayloadConditionals(blockItems map[string]any, view *types.AnswerSemanticView) {
	allowed := allowedKindSet(view)
	if len(allowed) == 0 {
		return
	}
	conditionals := schemaAllOfEntries(blockItems)
	for _, k := range allowedKindOrder() {
		if !allowed[k] {
			continue
		}
		payload, ok := blockKindPayloadField[k]
		if !ok {
			continue
		}
		conditionals = append(conditionals, map[string]any{
			"if": map[string]any{
				"properties": map[string]any{
					"kind": map[string]any{"const": k},
				},
			},
			"then": map[string]any{
				"required": []string{"id", "kind", payload},
			},
		})
	}
	if len(conditionals) > 0 {
		blockItems["allOf"] = conditionals
	}
}

func projectTypedDecisionVerdictFields(blockProps map[string]any, blockItems map[string]any, view *types.AnswerSemanticView) {
	currentActive := view.CurrentStatusDiagnostic != nil && view.CurrentStatusDiagnostic.Required
	errorActive := view.ErrorGranularityProfile != nil && view.ErrorGranularityProfile.Active()
	if !currentActive {
		delete(blockProps, "current_status_verdict")
	} else {
		projectCurrentStatusVerdictEnum(blockProps, view.CurrentStatusDiagnostic)
		appendKindRequiredFieldsConditional(blockItems, string(types.BlockDecision), "current_status_verdict")
	}
	if !errorActive {
		delete(blockProps, "error_granularity_verdict")
	} else {
		appendKindRequiredFieldsConditional(blockItems, string(types.BlockDecision), "error_granularity_verdict")
	}
}

func projectCurrentStatusVerdictEnum(blockProps map[string]any, contract *types.CurrentStatusDiagnosticContract) {
	node, _ := blockProps["current_status_verdict"].(map[string]any)
	if node == nil {
		return
	}
	allowed := types.CurrentStatusAllowedVerdicts(contract)
	enum := make([]string, 0, len(allowed))
	for _, verdict := range allowed {
		enum = append(enum, string(verdict))
	}
	if len(enum) > 0 {
		node["enum"] = enum
	}
}

func appendKindRequiredFieldsConditional(blockItems map[string]any, kind string, fields ...string) {
	if len(fields) == 0 {
		return
	}
	required := append([]string{"id", "kind"}, fields...)
	conditionals := schemaAllOfEntries(blockItems)
	conditionals = append(conditionals, map[string]any{
		"if": map[string]any{
			"properties": map[string]any{
				"kind": map[string]any{"const": kind},
			},
		},
		"then": map[string]any{
			"required": required,
		},
	})
	blockItems["allOf"] = conditionals
}

func schemaAllOfEntries(node map[string]any) []map[string]any {
	switch raw := node["allOf"].(type) {
	case []map[string]any:
		return append([]map[string]any(nil), raw...)
	case []any:
		out := make([]map[string]any, 0, len(raw))
		for _, entry := range raw {
			if m, ok := entry.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func projectRequiredBlockArrayCardinality(blocksField map[string]any, view *types.AnswerSemanticView) {
	if blocksField == nil || view == nil {
		return
	}
	type bounds struct {
		min int
		max int
	}
	byKind := make(map[types.AnswerBlockKind]bounds)
	for _, req := range view.RequiredBlocks {
		if !req.Required || req.Kind == "" {
			continue
		}
		b := byKind[req.Kind]
		b.min += req.MinCount
		if req.MaxCount > 0 && (b.max == 0 || req.MaxCount < b.max) {
			b.max = req.MaxCount
		}
		byKind[req.Kind] = b
	}
	if len(byKind) == 0 {
		return
	}
	conditionals := schemaAllOfEntries(blocksField)
	for _, kind := range types.AllAnswerBlockKinds() {
		b, ok := byKind[kind]
		if !ok || (b.min <= 0 && b.max <= 0) {
			continue
		}
		entry := map[string]any{
			"contains": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{"const": string(kind)},
				},
				"required": []string{"kind"},
			},
		}
		if b.min > 0 {
			entry["minContains"] = b.min
		}
		if b.max > 0 {
			entry["maxContains"] = b.max
		}
		conditionals = append(conditionals, entry)
	}
	if len(conditionals) > 0 {
		blocksField["allOf"] = conditionals
	}
}

// allowedKindSet returns the set of block-kind strings this dispatch
// allows the LLM to emit, as restricted by the view's RequiredBlocks,
// OptionalBlocks, and Presentation.AllowedBlocks. Empty when the view
// declares none — the caller then treats every canonical kind as allowed.
func allowedKindSet(view *types.AnswerSemanticView) map[string]bool {
	out := make(map[string]bool, 9)
	for _, br := range view.RequiredBlocks {
		if br.Kind != "" {
			out[string(br.Kind)] = true
		}
	}
	for _, br := range view.OptionalBlocks {
		if br.Kind != "" {
			out[string(br.Kind)] = true
		}
	}
	for _, kind := range view.Presentation.AllowedBlocks {
		if kind == "" {
			continue
		}
		if kind == types.BlockDiagram && view.DiagramPlan == nil {
			continue
		}
		out[string(kind)] = true
	}
	return out
}

// allowedKindOrder returns the canonical block-kind enumeration order
// (matches AllAnswerBlockKinds). Used by the conditionals projector
// so the resulting schema's allOf list is deterministic across runs.
func allowedKindOrder() []string {
	return []string{
		"summary", "section",
		"ordered_list", "bullet_list",
		"scalar", "decision",
		"table", "diagram", "caveat",
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

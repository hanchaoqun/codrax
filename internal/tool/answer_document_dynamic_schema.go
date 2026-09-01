package tool

import (
	"encoding/json"
	"fmt"
	"strings"

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
//     view.OptionalBlocks ∪ view.Presentation.AllowedBlocks kinds.
//     When all three are empty, canonical kinds remain available except
//     diagram when no DiagramPlan owns its payload.
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
//   - exact_resolution is dropped when view.ExactResolution is nil or its
//     answer surface is explicitly suppressed.
//   - source_inventory_family and items[].source_inventory_row_id are dropped
//     unless the view has a typed source-inventory observation.
//   - items[].evidence_ids is dropped unless the view has a typed
//     current-source evidence origin.
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
			projectParticipantBoundariesField(blockProps, view)
			projectRequestedRelationScopeField(blockProps, view)
			projectTypedDecisionVerdictFields(blockProps, blockItems, view)
			projectTraceCausalClaimCaliberField(blockProps, blockItems, view)
			projectRuntimeWorkRelationField(blockProps, view)
			projectConceptualTerminalResolutionField(blockProps, view)
			projectSourceInventoryIdentityFields(blockProps, view)
			projectItemEvidenceIdentityField(blockProps, view)
			projectDiagramPayloadOwnership(blockItems, blockProps)
		}
		// The projected field set is the executable contract. Leaving the
		// object open let an omitted payload (notably diagram) be attached to
		// another kind and then revived by the compatibility normalizer. That
		// made the JSON schema and its runtime meaning disagree. Full canonical
		// callers remain unchanged; dispatch-local schemas are deliberately
		// closed over the exact fields they publish.
		blockItems["additionalProperties"] = false
		projectKindPayloadConditionals(blockItems, view)
		projectSourceInventoryPrincipalTableItems(blockItems, view)
	}
	projectRequiredBlockArrayCardinality(blocksField, view)

	// Document-level conditional fields.
	if view.ExactResolution == nil || view.SuppressExactResolutionAnswerSurface {
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

func projectRuntimeWorkRelationField(blockProps map[string]any, view *types.AnswerSemanticView) {
	contract := view.RuntimeWorkRelationContract
	if contract == nil || !contract.Active() {
		delete(blockProps, "runtime_work_relation")
		return
	}
	choices := make([]any, 0, len(contract.Rows))
	for _, row := range contract.Rows {
		for _, conclusion := range row.AllowedConclusions {
			if !conclusion.IsValid() {
				continue
			}
			choices = append(choices, map[string]any{
				"type": "object",
				"properties": map[string]any{
					"observation_id": map[string]any{"const": row.ObservationID},
					"conclusion":     map[string]any{"const": string(conclusion)},
				},
				"required":             []string{"observation_id", "conclusion"},
				"additionalProperties": false,
			})
		}
	}
	if len(choices) == 0 {
		delete(blockProps, "runtime_work_relation")
		return
	}
	blockProps["runtime_work_relation"] = map[string]any{
		"description": "Select exactly one typed runtime-work row and its evidence-bounded conclusion. The system resolves the selected id to exact measured facts for visible rendering; it never chooses the row or conclusion and never scans block text. Do not repeat machine conclusion tokens in visible block text or caveats; the renderer localizes the selected conclusion.",
		"oneOf":       choices,
	}
}

func projectConceptualTerminalResolutionField(blockProps map[string]any, view *types.AnswerSemanticView) {
	contract := view.ConceptualTerminalResolutionContract
	if contract == nil || !contract.Active() {
		delete(blockProps, "conceptual_terminal_resolution")
		return
	}
	choices := make([]any, 0, len(contract.Rows)*3)
	for _, row := range contract.Rows {
		for _, conclusion := range row.AllowedConclusions {
			if !conclusion.IsValid() {
				continue
			}
			choices = append(choices, map[string]any{
				"type":        "object",
				"description": fmt.Sprintf("evidence_id=%s maps to exact terminal operation %s -> %s at %s. %s The system publishes the operation but never chooses this conclusion.", row.EvidenceID, row.TerminalCallable, row.ExactOperation, row.Source, conclusion.SchemaDescription()),
				"properties": map[string]any{
					"evidence_id": map[string]any{"const": row.EvidenceID},
					"conclusion":  map[string]any{"const": string(conclusion)},
				},
				"required":             []string{"evidence_id", "conclusion"},
				"additionalProperties": false,
			})
		}
	}
	if len(choices) == 0 {
		choices = append(choices, map[string]any{
			"type": "object",
			"properties": map[string]any{
				"conclusion": map[string]any{"const": string(types.ConceptualTerminalResolutionDestinationUnproven)},
			},
			"required":             []string{"conclusion"},
			"additionalProperties": false,
		})
	}
	blockProps["conceptual_terminal_resolution"] = map[string]any{
		"description": "Select one exact parser-grounded terminal operation and a model-owned conclusion about whether it satisfies the requested conceptual destination. The system binds and renders the selected operation but never infers this conclusion from names, request wording, or answer prose. When no terminal operation is published, select the schema's destination_unproven form without evidence_id.",
		"oneOf":       choices,
	}
}

// projectDiagramPayloadOwnership makes the native diagram carrier
// unambiguous whenever this dispatch exposes block.diagram. A fused
// section/list + diagram payload remains tolerated by the runtime recovery
// path for historical callers, but a live model can no longer be taught or
// schema-admitted into that compatibility shape: if it chooses diagram, it
// must choose kind=diagram. No Mermaid text, label, request prose, or model
// answer is inspected here.
func projectDiagramPayloadOwnership(blockItems, blockProps map[string]any) {
	if blockItems == nil || blockProps == nil {
		return
	}
	if _, exposed := blockProps["diagram"]; !exposed {
		return
	}
	conditionals := schemaAllOfEntries(blockItems)
	conditionals = append(conditionals, map[string]any{
		"if": map[string]any{
			"required": []string{"diagram"},
		},
		"then": map[string]any{
			"properties": map[string]any{
				"kind": map[string]any{"const": string(types.BlockDiagram)},
			},
		},
	})
	blockItems["allOf"] = conditionals
}

func projectItemEvidenceIdentityField(blockProps map[string]any, view *types.AnswerSemanticView) {
	if view != nil && view.ItemEvidenceIdentityAvailable {
		return
	}
	itemsNode, _ := blockProps["items"].(map[string]any)
	itemSchema, _ := itemsNode["items"].(map[string]any)
	itemProps, _ := itemSchema["properties"].(map[string]any)
	delete(itemProps, "evidence_ids")
}

// projectTraceFindingContract adds the typed sidecar only for explicitly
// activated batch child runs. Ordinary read requests retain byte-identical
// canonical schemas.
func projectTraceFindingContract(schema json.RawMessage, contract *types.TraceFindingContract, patch bool) json.RawMessage {
	if contract == nil || !contract.Required {
		return schema
	}
	var root map[string]any
	if err := json.Unmarshal(schema, &root); err != nil {
		return schema
	}
	properties, _ := root["properties"].(map[string]any)
	if properties == nil {
		return schema
	}
	name := "trace_finding"
	if patch {
		name = "replace_trace_finding"
	}
	properties[name] = traceFindingJSONSchema(contract)
	if !patch {
		required, _ := root["required"].([]any)
		required = append(required, name)
		root["required"] = required
	}
	out, err := json.Marshal(root)
	if err != nil {
		return schema
	}
	return out
}

// projectTraceRootCauseReport adds the user-facing JSON report to trace
// root-cause finalizer calls. Unlike TraceFindingV1, this report does not
// depend on deterministic candidate IDs, so it remains usable when the model
// can diagnose the trace but no root_cause_rank row was published.
func projectTraceRootCauseReport(schema json.RawMessage, contract *types.TraceFindingContract, patch bool) json.RawMessage {
	if contract == nil || !contract.RootCauseReportRequired {
		return schema
	}
	var root map[string]any
	if err := json.Unmarshal(schema, &root); err != nil {
		return schema
	}
	properties, _ := root["properties"].(map[string]any)
	if properties == nil {
		return schema
	}
	name := "trace_root_causes"
	if patch {
		name = "replace_trace_root_causes"
	}
	properties[name] = traceRootCauseReportJSONSchema()
	if !patch {
		required, _ := root["required"].([]any)
		required = append(required, name)
		root["required"] = required
	}
	out, err := json.Marshal(root)
	if err != nil {
		return schema
	}
	return out
}

func traceRootCauseReportJSONSchema() map[string]any {
	categories := make([]string, 0, len(types.AllTraceRootCauseCategories()))
	for _, category := range types.AllTraceRootCauseCategories() {
		categories = append(categories, string(category))
	}
	item := map[string]any{
		"type":        "object",
		"description": "One independently supported structured root cause. Evidence is concise free text grounded in the trace; rank and summary are normalized by the runtime from array order and category identity.",
		"properties": map[string]any{
			"category":      map[string]any{"type": "string", "enum": categories},
			"thread_name":   map[string]any{"type": "string", "description": "Exact thread name; required for thread-scoped categories."},
			"resource_name": map[string]any{"type": "string", "description": "Exact lock or resource name; required for lock_contention."},
			"phase_name":    map[string]any{"type": "string", "description": "Exact phase name; required for phase_high_load."},
			"impact_seconds": map[string]any{
				"type": "number", "exclusiveMinimum": 0,
				"description": "Positive effective impact attributable to this cause on the target analysis window, in seconds. Convert a published effective/attributable millisecond value by dividing by 1000; never use raw occupancy or cross-thread aggregate CPU-ms.",
			},
			"summary": map[string]any{"type": "string", "description": "Short root-cause phrase. Runtime rewrites it to the fixed category format."},
			"evidence": map[string]any{
				"type": "array", "minItems": 1, "maxItems": 4,
				"items":       map[string]any{"type": "string", "maxLength": 240},
				"description": "1-4 concise trace-specific evidence statements; free text, not fixed vocabulary and not internal ids.",
			},
		},
		"required": []string{"category", "impact_seconds", "evidence"},
	}
	return map[string]any{
		"type":        "object",
		"description": "Separate JSON report accompanying the unchanged full trace analysis.",
		"properties": map[string]any{
			"schema_version": map[string]any{"type": "integer", "enum": []int{types.TraceRootCauseReportSchemaVersion}},
			"root_causes": map[string]any{
				"type": "array", "items": item,
				"description": "Evidence-sized root-cause list ordered strongest to weakest. Emit every independently supported cause with a positive trace-grounded impact, or [] when none exists. Runtime assigns contiguous rank values from this order.",
			},
		},
		"required": []string{"schema_version", "root_causes"},
	}
}

func traceFindingJSONSchema(contract *types.TraceFindingContract) map[string]any {
	primaryIDs := append([]string(nil), contract.PrimaryCandidateIDs...)
	contributorIDs := append([]string(nil), contract.ContributorCandidateIDs...)
	decision := func(ids []string) map[string]any {
		statuses := []string{"proven", "supported_candidate"}
		if contract.CausalCeiling == "unproven" {
			statuses = []string{"supported_candidate"}
		}
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"candidate_id": map[string]any{"type": "string", "enum": ids},
				"status":       map[string]any{"type": "string", "enum": statuses},
				"token": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"token":         map[string]any{"type": "string"},
						"lane":          map[string]any{"type": "string"},
						"additivity":    map[string]any{"type": "string"},
						"subject_kind":  map[string]any{"type": "string"},
						"fix_direction": map[string]any{"type": "string"},
						"registry_hash": map[string]any{"type": "string", "enum": []string{contract.RegistryHash}},
					},
					"required": []string{"token", "lane", "additivity", "subject_kind", "registry_hash"},
				},
				"subject_role":         map[string]any{"type": "string"},
				"upstream_role":        map[string]any{"type": "string"},
				"causal_shape":         map[string]any{"type": "string"},
				"phase":                map[string]any{"type": "string"},
				"rank":                 map[string]any{"type": "integer"},
				"tier":                 map[string]any{"type": "string"},
				"board_fingerprint":    map[string]any{"type": "string"},
				"normalized_event_key": map[string]any{"type": "string"},
				"normalized_stack_key": map[string]any{"type": "string"},
				"magnitude": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"value":              map[string]any{"type": "number"},
						"unit":               map[string]any{"type": "string"},
						"additivity":         map[string]any{"type": "string"},
						"caliber":            map[string]any{"type": "string"},
						"window_duration_ms": map[string]any{"type": "number"},
					},
					"required": []string{"value", "unit", "additivity", "caliber"},
				},
				"evidence_refs": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": contract.AcceptedEvidenceIDs}},
				"confidence":    map[string]any{"type": "string"},
			},
			"required": []string{"candidate_id", "status", "token", "subject_role", "causal_shape", "phase", "evidence_refs", "confidence"},
		}
	}
	return map[string]any{
		"type":        "object",
		"description": "Structured conclusion for this trace. Choose only the injected candidate and evidence ids; do not infer it from answer prose.",
		"properties": map[string]any{
			"schema_version": map[string]any{"type": "integer", "enum": []int{types.TraceFindingSchemaVersion}},
			"finding_id":     map[string]any{"type": "string", "enum": []string{contract.FindingID}},
			"analysis_key":   map[string]any{"type": "string", "enum": []string{contract.AnalysisKey}},
			"artifact": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"artifact_id":   map[string]any{"type": "string", "enum": []string{contract.Artifact.ArtifactID}},
					"content_hash":  map[string]any{"type": "string", "enum": []string{contract.Artifact.ContentHash}},
					"display_label": map[string]any{"type": "string", "enum": []string{contract.Artifact.DisplayLabel}},
				},
				"required": []string{"artifact_id", "content_hash"},
			},
			"scope": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"profile_family": map[string]any{"type": "string", "enum": []string{contract.Scope.ProfileFamily}},
					"target_role":    map[string]any{"type": "string", "enum": []string{contract.Scope.TargetRole}},
					"phase":          map[string]any{"type": "string", "enum": []string{contract.Scope.Phase}},
				},
				"required": []string{"profile_family", "target_role", "phase"},
			},
			"revision": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"codrax_commit": map[string]any{"type": "string"},
					"contract_hash": map[string]any{"type": "string", "enum": []string{contract.ContractHash}},
				},
				"required": []string{"contract_hash"},
			},
			"symptom": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":  map[string]any{"type": "string", "enum": []string{contract.Symptom.Kind}},
					"value": map[string]any{"type": "number"},
					"unit":  map[string]any{"type": "string"},
				},
				"required": []string{"kind"},
			},
			"primary_cause": decision(primaryIDs),
			"contributors":  map[string]any{"type": "array", "items": decision(contributorIDs)},
			"unresolved": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"reason":    map[string]any{"type": "string"},
					"raw_label": map[string]any{"type": "string"},
				},
				"required": []string{"reason"},
			},
			"evidence_refs":         map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": contract.AcceptedEvidenceIDs}},
			"counter_evidence_refs": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": contract.AcceptedEvidenceIDs}},
			"coverage": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"complete": map[string]any{"type": "boolean"},
					"caveats":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"complete"},
			},
		},
		"required": []string{"schema_version", "finding_id", "analysis_key", "artifact", "scope", "revision", "symptom", "evidence_refs", "coverage"},
	}
}

// projectSourceInventoryPrincipalTableItems makes the dispatch-projected
// schema tell the whole truth about source-inventory tables. A generic table
// may be carried entirely by block.text, but an authoritative source inventory
// also needs row-local identity/citation sidecars. Requiring items only for a
// principal table in a view backed by typed source-inventory rows preserves
// Markdown presentation freedom for every other table while avoiding a
// schema-accepted first emit that the precise row oracle must reject later.
func projectSourceInventoryPrincipalTableItems(blockItems map[string]any, view *types.AnswerSemanticView) {
	if view == nil || !view.SourceInventoryRowIdentityAvailable {
		return
	}
	appendPrincipalKindRequiredFieldsConditional(blockItems, string(types.BlockTable), "items")
}

func projectSourceInventoryIdentityFields(blockProps map[string]any, view *types.AnswerSemanticView) {
	if view != nil && view.SourceInventoryRowIdentityAvailable {
		return
	}
	delete(blockProps, "source_inventory_family")
	itemsNode, _ := blockProps["items"].(map[string]any)
	itemSchema, _ := itemsNode["items"].(map[string]any)
	itemProps, _ := itemSchema["properties"].(map[string]any)
	delete(itemProps, "source_inventory_row_id")
}

func projectTraceCausalClaimCaliberField(blockProps map[string]any, blockItems map[string]any, view *types.AnswerSemanticView) {
	contract := view.TraceCausalClaimContract
	if !contract.Active() {
		delete(blockProps, "trace_causal_claim_caliber")
		return
	}
	node, _ := blockProps["trace_causal_claim_caliber"].(map[string]any)
	if node == nil {
		return
	}
	enum := make([]string, 0, len(contract.Allowed))
	for _, caliber := range contract.Allowed {
		enum = append(enum, string(caliber))
	}
	node["enum"] = enum
	node["description"] = traceCausalClaimCaliberSchemaDescription(contract)
	appendPrincipalKindExclusiveFieldConditional(blockItems, string(types.BlockSummary), "trace_causal_claim_caliber")
}

func traceCausalClaimCaliberSchemaDescription(contract *types.TraceCausalClaimContract) string {
	if contract == nil || !contract.Active() {
		return ""
	}
	allowed := make(map[types.TraceCausalClaimCaliber]bool, len(contract.Allowed))
	values := make([]string, 0, len(contract.Allowed))
	for _, caliber := range contract.Allowed {
		allowed[caliber] = true
		values = append(values, string(caliber))
	}
	parts := []string{
		"Allowed for this dispatch: " + strings.Join(values, ", ") + ".",
		TraceCausalClaimPrincipalSummaryShape(contract.Allowed),
		types.AnswerControlMetadataVisibilityGuide,
	}
	if allowed[types.TraceCausalClaimNoConclusion] {
		parts = append(parts, "Use no_causal_conclusion only when the principal summary makes no cause or candidate attribution.")
	}
	if allowed[types.TraceCausalClaimBoundedWindow] {
		parts = append(parts, "Use bounded_window_candidate when the summary names or ranks selected-window candidates without claiming proven frame/deadline causality.")
	}
	if allowed[types.TraceCausalClaimTypedChain] {
		parts = append(parts, "Use typed_chain_cause only for a causal attribution bounded by a typed chain.")
	}
	if allowed[types.TraceCausalClaimTypedFrame] {
		parts = append(parts, "Use typed_frame_cause only when typed frame/deadline causality supports that claim.")
	}
	parts = append(parts, "Evidence-status values such as unproven are not enum values for this field. The selected literal stays only in the JSON field; user-facing prose states its meaning in the answer language. You choose the conclusion and caliber; the value is checked only against the provided typed Trace evidence ceiling and neither choice is derived from prose.")
	return strings.Join(parts, " ")
}

// appendPrincipalKindRequiredFieldsConditional keeps report-level typed
// declarations on the principal carrier only. Requiring them on every block
// of the same kind would make a supporting summary repeat a contract it does
// not own, increasing schema load and retry risk without adding authority.
func appendPrincipalKindRequiredFieldsConditional(blockItems map[string]any, kind string, fields ...string) {
	if blockItems == nil || strings.TrimSpace(kind) == "" || len(fields) == 0 {
		return
	}
	required := append([]string{"id", "kind", "surface_role"}, fields...)
	conditionals := schemaAllOfEntries(blockItems)
	conditionals = append(conditionals, map[string]any{
		"if": map[string]any{
			"required": []string{"kind", "surface_role"},
			"properties": map[string]any{
				"kind":         map[string]any{"const": kind},
				"surface_role": map[string]any{"const": string(types.SurfacePrincipal)},
			},
		},
		"then": map[string]any{"required": required},
	})
	blockItems["allOf"] = conditionals
}

// appendPrincipalKindExclusiveFieldConditional keeps a report-level control
// field on exactly one structural owner. The ordinary required-field helper
// only says that a principal block of kind K must carry the field; without the
// else/false-property arm the same projected JSON schema also admits that field on every
// sibling block and leaves the runtime normalizer to reject it later. That is
// a self-contradictory model contract and burns an avoidable repair round.
//
// This predicate reads typed block discriminators only. It neither inspects
// nor rewrites answer prose, and the model remains the sole author of the
// field value allowed by its dispatch-local enum.
func appendPrincipalKindExclusiveFieldConditional(blockItems map[string]any, kind string, fields ...string) {
	if blockItems == nil || strings.TrimSpace(kind) == "" || len(fields) == 0 {
		return
	}
	required := append([]string{"id", "kind", "surface_role"}, fields...)
	forbiddenProperties := make(map[string]any, len(fields))
	for _, field := range fields {
		forbiddenProperties[field] = false
	}
	conditionals := schemaAllOfEntries(blockItems)
	conditionals = append(conditionals, map[string]any{
		"if": map[string]any{
			"required": []string{"kind", "surface_role"},
			"properties": map[string]any{
				"kind":         map[string]any{"const": kind},
				"surface_role": map[string]any{"const": string(types.SurfacePrincipal)},
			},
		},
		"then": map[string]any{"required": required},
		"else": map[string]any{
			"properties": forbiddenProperties,
		},
	})
	blockItems["allOf"] = conditionals
}

// projectBlockKindEnum restricts the block.kind enum to the kinds
// declared in view.RequiredBlocks ∪ view.OptionalBlocks ∪
// view.Presentation.AllowedBlocks. When the view declares no kinds at all,
// the canonical non-diagram list keeps the schema usable; diagram still needs
// a live DiagramPlan.
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
	// Availability comes only from the projected JSON enum. The field
	// description deliberately does not advertise individual conditional
	// forms, so a strict-mode reader cannot be taught a value that this
	// dispatch then rejects.
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
		for _, kind := range br.AcceptedKinds() {
			if kind != "" {
				contractKinds[kind] = true
			}
		}
	}
	for _, br := range view.OptionalBlocks {
		for _, kind := range br.AcceptedKinds() {
			if kind != "" {
				contractKinds[kind] = true
			}
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
	type requirementBounds struct {
		kinds []types.AnswerBlockKind
		min   int
		max   int
	}
	var requirements []requirementBounds
	for _, req := range view.RequiredBlocks {
		if !req.Required {
			continue
		}
		kinds := req.AcceptedKinds()
		if len(kinds) == 0 || (req.MinCount <= 0 && req.MaxCount <= 0) {
			continue
		}
		requirements = append(requirements, requirementBounds{
			kinds: kinds,
			min:   req.MinCount,
			max:   req.MaxCount,
		})
	}
	if len(requirements) == 0 {
		return
	}
	conditionals := schemaAllOfEntries(blocksField)
	for _, b := range requirements {
		kindSchema := map[string]any{}
		if len(b.kinds) == 1 {
			kindSchema["const"] = string(b.kinds[0])
		} else {
			enum := make([]string, 0, len(b.kinds))
			for _, kind := range b.kinds {
				enum = append(enum, string(kind))
			}
			kindSchema["enum"] = enum
		}
		entry := map[string]any{
			"contains": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": kindSchema,
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
// OptionalBlocks, and Presentation.AllowedBlocks. A view with no explicit
// roster receives the canonical set minus diagram when no DiagramPlan exists.
func allowedKindSet(view *types.AnswerSemanticView) map[string]bool {
	out := make(map[string]bool, 9)
	for _, kind := range types.AnswerSemanticViewAllowedBlockKinds(view) {
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

// projectParticipantBoundariesField exposes the explicit no-edge decision
// only for a precise required source-flow participant contract. Ordinary
// diagrams and every Trace lane keep the field out of the model schema.
func projectParticipantBoundariesField(blockProps map[string]any, view *types.AnswerSemanticView) {
	incident := make([]string, 0, len(view.DiagramParticipantObligations))
	for _, obligation := range view.DiagramParticipantObligations {
		if obligation.Role != types.DiagramParticipantIncidentRequired || strings.TrimSpace(obligation.Identity) == "" {
			continue
		}
		incident = append(incident, strings.TrimSpace(obligation.Identity))
	}
	if view.DiagramPlan == nil || !view.DiagramPlan.Required || view.RelationAxis != types.AxisFlow || len(incident) == 0 {
		delete(blockProps, "participant_boundaries")
		return
	}
	field, _ := blockProps["participant_boundaries"].(map[string]any)
	if field != nil {
		field["description"] = "BLOCK-LEVEL sibling of diagram and edge_anchors; NEVER put participant_boundaries inside diagram. Exact shape: {kind:\"diagram\", diagram:{kind,language,body}, participant_boundaries:[{participant,status:\"unproven\"}]}. Explicit coverage decision for the typed incident participants [" + strings.Join(incident, ", ") + "]. Omit or emit [] when every participant has a typed visible incident edge for the requested directed relation. When that requested relation is unproved, list exactly the uncovered participant identities with status=unproven. A separately proved local operation touching the participant does not by itself prove or eliminate this requested-relation boundary. An independently proved local technical edge, exact endpoint, or no-arrow containment/grouping may coexist with the boundary, but none may be presented as the missing requested relation. This array never creates or authorizes an edge."
	}
}

// projectRequestedRelationScopeField exposes the whole-diagram disclosure only
// on the same precise required source-flow lane that can produce a typed
// request-spine coverage decision. The field remains optional in JSON Schema
// because the final evidence pool is dispatch-local; the typed pre-emit and
// post-emit checks require it only when that exact pool proves a partial spine.
func projectRequestedRelationScopeField(blockProps map[string]any, view *types.AnswerSemanticView) {
	incident := 0
	for _, obligation := range view.DiagramParticipantObligations {
		if obligation.Role == types.DiagramParticipantIncidentRequired && strings.TrimSpace(obligation.Identity) != "" {
			incident++
		}
	}
	if view.DiagramPlan == nil || !view.DiagramPlan.Required || view.RelationAxis != types.AxisFlow || incident < 2 ||
		view.Family == types.QFRootCauseTrace {
		delete(blockProps, "requested_relation_scope")
		return
	}
	field, _ := blockProps["requested_relation_scope"].(map[string]any)
	if field != nil {
		field["description"] = "BLOCK-LEVEL sibling of diagram and edge_anchors. When the typed relation context publishes requested_relation_spine_status=unproven, copy requested_relation_scope=partial_unproven onto exactly one diagram block that presents the requested relation. This model-authored declaration tells readers that proved visible segments do not yet establish one complete end-to-end requested relation. It creates no edge and does not replace your conclusion. Omit it when no typed partial-spine obligation is published; never copy the raw enum into visible prose or labels."
	}
}

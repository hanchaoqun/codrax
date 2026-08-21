package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// emit_answer_document_patch — protocol-level retry preservation
// tool. On retry paths the LLM emits a *delta* (this tool) rather
// than the full document (emit_answer_document). The system applies
// the patch to the previous emit (read from
// the retry-local staged patch, MutableState.AnswerDocumentV2, or the typed
// retry-state snapshot)
// and writes the resulting full doc to Mutable.
//
// Why this tool exists despite emit_answer_document already
// supporting full re-emit: prior real-eval traces show generative
// LLMs ignore "preserve byte-identical" prompt directives roughly
// 50% of the time on retry paths, dropping typed annotation fields
// they already emitted correctly. Protocol-level patch makes the
// preservation **structurally guaranteed** — LLM never has the
// chance to drop a field on a block it didn't touch, because the
// system clones unchanged blocks from prev verbatim.
//
// Tool calling discipline (taught via skill prompt):
//   - First dispatch / no prev emit: use emit_answer_document.
//   - Retry path with prev emit on Mutable: PREFER
//     emit_answer_document_patch when only a few blocks need
//     editing. Fall back to full emit_answer_document for big
//     rewrites.

// EmitAnswerDocumentPatch is the patch tool. Mirrors the shape of
// EmitAnswerDocument (NonEvidenceTool — the payload is the final
// answer slate, not factual claims about the repo) so the agent
// layer registers them uniformly.
type EmitAnswerDocumentPatch struct {
	ReadOnly
	NonEvidenceTool
}

func (t *EmitAnswerDocumentPatch) Name() string { return "emit_answer_document_patch" }

func (t *EmitAnswerDocumentPatch) Description() string {
	return "Emit a DELTA against your previous `emit_answer_document` call instead of re-emitting the whole document. Use ONLY on retry paths (when `## Hard Rule (retry attempt N)` appears in the system prompt and a `## Previous Emit` section is present). On first dispatches, use `emit_answer_document` instead.\n\n" +
		"Patch fields (all optional, but at least one MUST be non-empty):\n\n" +
		"- `unchanged_block_ids`: ids of blocks from the previous emit to copy over byte-identical. Use this to assert preservation of every typed annotation/display field (columns, claim_uses, edge_anchors, relation_claims, facet_ids, surface_role, source_inventory_family, items[].cells, items[].candidate_role, items[].source_inventory_row_id, items[].evidence_ids, items[].citation_ref, items[].citation_refs) on blocks you do NOT need to edit. If an id is also targeted by `diagram_edge_edits`, `diagram_boundary_replacements`, or `diagram_participant_edits`, that unchanged entry is redundant and is absorbed because atomic editing already preserves every unmentioned carrier from the immutable base.\n" +
		"- `replace_blocks`: FULL block payloads, not general field merges. Each entry replaces the previous block with the same id and must carry a non-empty existing id. Copy every previous display/typed field that the required repair does not name (especially title, text, columns, diagram, facet_ids, claim_uses, surface_role), then change only the named field. One narrow retry-safety exception applies: when the exact previous block id and kind are retained, at least one unique stable item id overlaps, and `facet_ids` or `surface_role` is truly omitted, the system retains only those omitted carrier fields; an explicit empty/value remains model-owned. Block payload shape matches the canonical block contract — see below.\n" +
		"- `add_blocks`: new block payloads to append. Each id must NOT already exist in the previous emit. Block payload shape matches the canonical block contract — see below.\n" +
		"- `remove_block_ids`: ids that must be absent from the resulting document. Repeating an already-satisfied removal is an idempotent no-op.\n" +
		"- `diagram_edge_edits`: model-authored atomic relation edits against one existing block. Visible relabel/remove/replace/add operations require an existing diagram carrier; a non-diagram block may only remove one exact live prior_anchor_metadata row while preserving all visible block fields. Use this instead of `replace_blocks` for a local typed relation retry. Every live failures[] row publishes `target_carrier` and `allowed_actions`; when using its `failure_ref`, choose only an action listed on that row and prefer `{failure_ref, action}` over retyping block/node/identity/relation/occurrence/body_occurrence coordinates. The live lease resolves only that exact failed carrier; legacy match/occurrence mirrors are ignored after the ref is validated, while an explicit cross-block conflict still fails. prior_anchor identifies one mapped anchor/body pair; prior_anchor_metadata identifies exact anchor metadata with no unique visible body occurrence and is remove-only without changing visible content; visible_body_edge identifies an unanchored Mermaid edge; stale_anchor identifies metadata with no body edge; label_pair is relabel-only. If several live failure rows name the same positive body_occurrence and you choose remove for all of them, submit every `{failure_ref, action:\"remove\"}` in the same patch; the executor removes the shared visible statement once and every selected typed anchor transactionally. replace requires the complete model-authored edge/visible_label. For add, prefer one live allowed_additions[].addition_ref: the ref selects only that typed relation candidate while you still author from_node, to_node, visible_label, ordering, and layout; omit edge.relation_kind/from_identity/to_identity and block_id because the executor restores those invisible fields from the selected row and ignores legacy hidden-field mirrors. In a sequenceDiagram that already declares participants, use those exact declared participant ids as from_node/to_node instead of creating implicit duplicates under stage, agent, or operation aliases. Legacy add without addition_ref must supply one complete new anchor and still cannot use failure_ref. The system preserves every unmentioned model-authored line, node, edge, label, and block field. The model still chooses every operation/relation and writes every visible label.\n" +
		"- `diagram_boundary_replacements`: model-authored replacement of only `participant_boundaries` on an existing diagram block. Use this for a participant coverage retry so the prior Mermaid body, relations, labels, and other block fields remain untouched.\n" +
		"- `diagram_participant_edits`: optional model-authored cleanup of one explicit participant/node declaration during a live local relation repair. Use only an `optional_orphan_cleanups` row and action=remove_if_isolated, in the same patch that removes every failed incident edge. The executor rejects requested participants, unproven-boundary participants, declarations whose original incident edges are not all covered by remove-capable live failures, declarations that remain connected after edge edits, and ambiguous/non-standalone declarations. Omitting this operation keeps the declaration as model-authored context; the system never chooses deletion.\n" +
		"- `replace_citations`: when present, REPLACES the citation pool entirely. Otherwise the previous citations are inherited. Prefer `append_citations` for additive citation repairs. If you accidentally replace the pool while preserving previous citation-bearing blocks, the tool will keep the previous pool, append genuinely new citations, and remap citation_ref values inside your replace/add blocks.\n" +
		"- `append_citations`: when present and `replace_citations` is absent, appended to the inherited pool.\n" +
		"- `replace_exact_resolution` / `replace_missing_requested_roles` / `replace_caveats` / `replace_snippets`: when present, replace the corresponding document-level field.\n\n" +
		"Validation: every id named in `unchanged_block_ids` / `replace_blocks` MUST exist in the previous emit; `remove_block_ids` is idempotent and may name an already-absent block; every `add_blocks` id MUST NOT exist. Cross-op conflicts (Replace + Remove same id, etc.) are rejected. A live local diagram lease exposes its target only through atomic diagram operations; whole replace/add/remove of that target is unavailable, while unrelated blocks remain editable. Block kind is validated against the canonical block-kind enum. The merged document is stored as if you had called `emit_answer_document` with the full payload.\n\n" +
		"Transactional rejection: if any patch validation fails, NONE of that patch's edits become the accepted answer. When a patch was structurally applicable and only a merged-document validator rejected it, that exact model-authored merged draft remains a retry-local staging base for the next patch; it is never user-visible until a later patch passes every validator. Earlier accepted/rejected-full documents remain unchanged. The retry prompt publishes the live staging block roster, so correct the named failure against that current base.\n\n" +
		"Empty patches are rejected — every retry MUST declare some change (set `unchanged_block_ids` to assert preservation if no edits are needed).\n\n" +
		"BLOCK CONTRACT (same shape replace_blocks / add_blocks payloads must follow as a full emit):\n\n" +
		BuildAnswerDocumentSemanticContractDescription()
}

func (t *EmitAnswerDocumentPatch) Parameters() json.RawMessage {
	const schema = `{
  "type": "object",
  "properties": {
    "unchanged_block_ids": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Block ids from the previous emit to copy over verbatim. Every id must exist in the previous emit. Use this to assert preservation of typed annotation/display fields (columns / claim_uses / edge_anchors / facet_ids / surface_role / source_inventory_family / items[].cells / items[].candidate_role / items[].source_inventory_row_id / items[].evidence_ids / items[].citation_ref / items[].citation_refs) on blocks you are not editing — the system clones the prev block byte-identical, so the LLM cannot accidentally drop a field."
    },
    "replace_blocks": {
      "type": "array",
      "description": "FULL replacement payloads, not general field merges. Copy all previous display and typed fields not named by the repair (especially title/text/columns/diagram/facet_ids/claim_uses/surface_role), then change only the requested field. Narrow retry safety: for the exact same block id/kind with a unique stable item-id overlap, truly omitted facet_ids/surface_role retain their prior carrier values; explicit empty/value is never inherited. Each entry has the full block shape and the same id as an existing block.",
      "items": {"type": "object"}
    },
    "add_blocks": {
      "type": "array",
      "description": "New block payloads to append after the existing blocks. Each entry has the full block shape (id, kind, title, text, items, diagram, claim_uses, edge_anchors, facet_ids, surface_role); id MUST NOT already exist in the previous emit (use replace_blocks for editing).",
      "items": {"type": "object"}
    },
    "remove_block_ids": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Block ids that must be absent from the resulting document. An id already absent from the previous emit is an idempotent no-op."
    },
    "diagram_edge_edits": {
      "type": "array",
      "maxItems": 128,
      "description": "Atomic model-authored relation edits for an existing block (maximum 128 operations). Visible relabel/remove/replace/add requires a diagram; a non-diagram block accepts only a live remove-only prior_anchor_metadata ref and preserves visible content. Prefer this during a local typed relation retry so unmentioned content stays byte-identical. A live failures[] failure_ref may replace block_id+match+occurrence+body_occurrence only with an action listed by that row's allowed_actions; target_carrier states whether it selects a mapped prior anchor/body pair, remove-only prior anchor metadata with no unique visible body occurrence, visible body-only edge, stale anchor, or label pair. If multiple live rows share one positive body_occurrence and you choose remove for all, submit every ref in the same patch; the shared statement is removed once with all selected anchors. replace requires a complete model-authored edge and visible_label. For add, prefer a live allowed_additions[].addition_ref plus edge.from_node/to_node/visible_label; omit edge.relation_kind/from_identity/to_identity and block_id because the ref supplies those hidden fields. When a sequenceDiagram already has explicit participant declarations, reuse their exact ids for edge.from_node/to_node instead of introducing implicit duplicate participants. Legacy add requires block_id+complete edge. add always rejects failure_ref. occurrence is 1-based among exact duplicate anchors and defaults to 1. Without failure_ref, body_occurrence selects the 1-based visible Mermaid edge for an otherwise ambiguous from_node/to_node pair. The system applies only the declared operation and then runs the ordinary typed relation/evidence gates; it never chooses an edge or visible label.",
      "items": {
        "type": "object",
        "properties": {
          "block_id": {"type": "string"},
          "action": {"type": "string", "enum": ["relabel", "remove", "replace", "add"]},
          "failure_ref": {"type": "string", "description": "Opaque selector copied exactly from the live failures[] row. Use only an action listed in that row's allowed_actions. It replaces match, occurrence, and body_occurrence; omit those coordinates because any legacy copies are quarantined after the live ref/action is validated. Omit failure_ref for add. Unknown, stale, disallowed-action, explicit cross-block, ambiguous, or reused refs fail closed."},
          "addition_ref": {"type": "string", "description": "Opaque selector copied exactly from one live allowed_additions[] row. Use only with action=add. It supplies that selected candidate's block_id, relation_kind, from_identity, and to_identity; you still author edge.from_node, edge.to_node, and edge.visible_label. If the sequence body already declares participants, copy those exact declared ids into edge.from_node/to_node. Omit failure_ref and hidden technical fields; legacy hidden-field copies are quarantined after the live ref/action is validated. Unknown, stale, duplicate, explicit cross-block, or wrong-action refs fail closed."},
          "occurrence": {"type": "integer", "minimum": 1},
          "body_occurrence": {"type": "integer", "minimum": 1, "description": "1-based visible Mermaid edge occurrence for the selected from_node/to_node pair. Omit when the pair is unique or body edges map one-to-one to exact prior anchors; required when the body pair is otherwise ambiguous."},
          "match": {
            "type": "object",
            "properties": {
              "from_node": {"type": "string"},
              "to_node": {"type": "string"},
              "from_identity": {"type": "string"},
              "to_identity": {"type": "string"},
              "relation_kind": {"type": "string", "enum": ["call", "callback", "argument_flow", "guard", "control_flow", "import", "precedence", "contain", "type_relation", "observe", "register", "assignment", "data_flow", "return", "temporal"]}
            },
            "required": ["from_node", "to_node", "relation_kind"]
          },
          "edge": {
            "type": "object",
            "properties": {
              "from_node": {"type": "string"},
              "to_node": {"type": "string"},
              "visible_label": {"type": "string"},
              "from_identity": {"type": "string"},
              "to_identity": {"type": "string"},
              "relation_kind": {"type": "string", "enum": ["call", "callback", "argument_flow", "guard", "control_flow", "import", "precedence", "contain", "type_relation", "observe", "register", "assignment", "data_flow", "return", "temporal"]}
            },
            "required": ["from_node", "to_node"]
          },
          "visible_label": {"type": "string", "description": "Model-authored reader-facing message for relabel. It updates the matched Mermaid message and anchor.visible_label together."}
        },
        "required": ["action"],
        "anyOf": [
          {"required": ["failure_ref"]},
          {"required": ["addition_ref"]},
          {"required": ["block_id"]}
        ]
      }
    },
    "diagram_boundary_replacements": {
      "type": "array",
      "description": "Replace only participant_boundaries on an existing diagram block while preserving its model-authored Mermaid body, edge anchors, labels, and all other fields.",
      "items": {
        "type": "object",
        "properties": {
          "block_id": {"type": "string"},
          "participant_boundaries": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "participant": {"type": "string"},
                "status": {"type": "string", "enum": ["unproven"]}
              },
              "required": ["participant", "status"]
            }
          }
        },
        "required": ["block_id", "participant_boundaries"]
      }
    },
    "diagram_participant_edits": {
      "type": "array",
      "maxItems": 64,
      "description": "Optional model-authored cleanup during a live local relation-repair lease. Copy one exact optional_orphan_cleanups row as block_id+participant_id and choose action=remove_if_isolated only when the same patch removes every failed incident edge. The executor removes only one unique standalone declaration line after rechecking that it is not requested or boundary-protected and is actually isolated. Omitting this operation retains the declaration as context; no edge, label, relation, or conclusion is inferred.",
      "items": {
        "type": "object",
        "properties": {
          "block_id": {"type": "string"},
          "participant_id": {"type": "string"},
          "action": {"type": "string", "enum": ["remove_if_isolated"]}
        },
        "required": ["block_id", "participant_id", "action"]
      }
    },
    "replace_citations": {
      "type": "array",
      "description": "OPTIONAL. When present, REPLACES the citation pool entirely. Use this when re-picking citations holistically.",
      "items": {"type": "object"}
    },
    "append_citations": {
      "type": "array",
      "description": "OPTIONAL. When present (and replace_citations is absent), appended to the inherited citation pool. Useful for adding a single new cite without rewriting the pool.",
      "items": {"type": "object"}
    },
    "replace_exact_resolution": {"type": "object", "description": "OPTIONAL. When present, replaces previous exact_resolution. Otherwise inherited from previous emit."},
    "replace_missing_requested_roles": {
      "type": "array",
      "description": "OPTIONAL. When present, replaces previous missing_requested_roles[]. Use this when a retry needs to add / remove / correct typed missing requested precedence layers for an exact-absence config-precedence answer.",
      "items": {
        "type": "object",
        "properties": {
          "role": {"type": "string", "enum": ["default", "config", "runtime", "override"]},
          "label": {"type": "string"}
        },
        "required": ["role"]
      }
    },
    "replace_caveats":  {"type": "array", "items": {"type": "string"}, "description": "OPTIONAL. When present, replaces previous caveats."},
    "replace_snippets": {"type": "array", "items": {"type": "object"}, "description": "OPTIONAL. When present, replaces previous snippets."}
  }
}`
	return json.RawMessage(schema)
}

// ParametersFor gives replace_blocks/add_blocks the same dispatch-projected
// block schema as a full emit. The patch envelope remains delta-specific, but
// a model repairing one block must not lose the structural teaching it had on
// the first emit (notably edge_anchors.from_identity/to_identity and
// participant_boundaries). Keeping the nested block item byte-derived from the
// full schema also prevents the two tool surfaces from drifting as new block
// fields or per-dispatch projections are added.
func (t *EmitAnswerDocumentPatch) ParametersFor(ctx *types.AgentContext) json.RawMessage {
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	raw := BuildAnswerDocumentPatchParametersFor(view)
	if ctx == nil || ctx.Mutable == nil {
		return raw
	}
	return narrowAnswerDocumentPatchParametersForLocalDiagramLease(raw, ctx.Mutable.AnswerDiagramRelationRepairLease())
}

// narrowAnswerDocumentPatchParametersForLocalDiagramLease removes one
// contradictory capability from the model-facing tool schema. A live relation
// lease authorizes only atomic edits for its target diagram blocks; offering a
// whole replace/add/remove for those same ids invites a transaction the lease
// must later reject. The JSON-schema exclusion is derived solely from the
// typed same-generation lease. Other block ids and all atomic operations stay
// available, so citation/table repairs can coexist with the local graph fix.
func narrowAnswerDocumentPatchParametersForLocalDiagramLease(raw json.RawMessage, lease *types.AnswerDiagramRelationRepairLease) json.RawMessage {
	targets := localDiagramLeaseTargetBlockIDs(lease)
	if len(targets) == 0 {
		return raw
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return raw
	}
	properties, _ := root["properties"].(map[string]any)
	if properties == nil {
		return raw
	}
	forbidden := make([]any, 0, len(targets))
	for _, id := range targets {
		forbidden = append(forbidden, id)
	}
	for _, field := range []string{"replace_blocks", "add_blocks"} {
		array, _ := properties[field].(map[string]any)
		item, _ := array["items"].(map[string]any)
		itemProperties, _ := item["properties"].(map[string]any)
		idSchema, _ := itemProperties["id"].(map[string]any)
		if idSchema == nil {
			return raw
		}
		idSchema["not"] = map[string]any{"enum": forbidden}
	}
	remove, _ := properties["remove_block_ids"].(map[string]any)
	removeItems, _ := remove["items"].(map[string]any)
	if removeItems == nil {
		return raw
	}
	removeItems["not"] = map[string]any{"enum": forbidden}
	if unchanged, ok := properties["unchanged_block_ids"].(map[string]any); ok {
		unchanged["description"] = "Block ids from the previous emit to preserve. A block also named by diagram_edge_edits, diagram_boundary_replacements, or diagram_participant_edits may be listed redundantly; the atomic compiler absorbs that id because every unmentioned carrier is already preserved from the immutable base."
	}
	out, err := json.Marshal(root)
	if err != nil || !json.Valid(out) {
		return raw
	}
	return out
}

func localDiagramLeaseTargetBlockIDs(lease *types.AnswerDiagramRelationRepairLease) []string {
	if lease == nil || lease.Version != 1 || len(lease.Failures) == 0 {
		return nil
	}
	diagramBlocks := make(map[string]bool, len(lease.Blocks))
	ambiguousBlocks := make(map[string]bool)
	for _, block := range lease.Blocks {
		id := strings.TrimSpace(block.BlockID)
		if id == "" {
			continue
		}
		if _, exists := diagramBlocks[id]; exists {
			ambiguousBlocks[id] = true
		}
		diagramBlocks[id] = block.Kind == types.BlockDiagram
	}
	seen := make(map[string]bool, len(lease.Failures))
	out := make([]string, 0, len(lease.Failures))
	for _, failure := range lease.Failures {
		id := strings.TrimSpace(failure.BlockID)
		if id == "" || strings.TrimSpace(failure.FailureRef) == "" ||
			!types.AnswerDiagramRelationRepairFailureHasCompleteLocator(failure) {
			// A malformed lease must not narrow a hard tool surface.
			return nil
		}
		// Relation anchors may also live on list/table carriers. B1285 narrows
		// only an actual diagram block, whose body and participant carriers have
		// dedicated atomic operations. Non-diagram relation blocks keep their
		// established whole-block repair path and remain guarded by the lease's
		// post-merge topology validator.
		if ambiguousBlocks[id] || !diagramBlocks[id] {
			continue
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// BuildAnswerDocumentPatchParametersFor projects the canonical full-document
// block item into both patch block arrays. It deliberately does not project or
// copy the full document envelope: citations and document-level delta fields
// retain their patch-specific replacement/append semantics.
func BuildAnswerDocumentPatchParametersFor(view *types.AnswerSemanticView) json.RawMessage {
	var patchRoot map[string]any
	if err := json.Unmarshal((&EmitAnswerDocumentPatch{}).Parameters(), &patchRoot); err != nil {
		return (&EmitAnswerDocumentPatch{}).Parameters()
	}
	var fullRoot map[string]any
	if err := json.Unmarshal(BuildAnswerDocumentParametersFor(view), &fullRoot); err != nil {
		return (&EmitAnswerDocumentPatch{}).Parameters()
	}
	patchProperties, _ := patchRoot["properties"].(map[string]any)
	fullProperties, _ := fullRoot["properties"].(map[string]any)
	fullBlocks, _ := fullProperties["blocks"].(map[string]any)
	blockItem, _ := fullBlocks["items"].(map[string]any)
	if patchProperties == nil || blockItem == nil {
		return (&EmitAnswerDocumentPatch{}).Parameters()
	}
	for _, field := range []string{"replace_blocks", "add_blocks"} {
		arraySchema, _ := patchProperties[field].(map[string]any)
		if arraySchema == nil {
			return (&EmitAnswerDocumentPatch{}).Parameters()
		}
		arraySchema["items"] = blockItem
	}
	out, err := json.Marshal(patchRoot)
	if err != nil || !json.Valid(out) {
		return (&EmitAnswerDocumentPatch{}).Parameters()
	}
	return out
}

// emitAnswerDocumentPatchParams mirrors AnswerDocumentV2Patch
// one-to-one for JSON unmarshalling. CitationRef and AnswerBlockItem
// fields use the same FlexInt typed approach as the V2 emit so
// citation_ref values can be int OR string from the LLM.
type emitAnswerDocumentPatchParams struct {
	UnchangedBlockIDs            []string                               `json:"unchanged_block_ids,omitempty"`
	ReplaceBlocks                []emitAnswerBlockV2                    `json:"replace_blocks,omitempty"`
	AddBlocks                    []emitAnswerBlockV2                    `json:"add_blocks,omitempty"`
	RemoveBlockIDs               []string                               `json:"remove_block_ids,omitempty"`
	DiagramEdgeEdits             []emitAnswerDiagramEdgeEdit            `json:"diagram_edge_edits,omitempty"`
	DiagramBoundaryReplacements  []emitAnswerDiagramBoundaryReplacement `json:"diagram_boundary_replacements,omitempty"`
	DiagramParticipantEdits      []emitAnswerDiagramParticipantEdit     `json:"diagram_participant_edits,omitempty"`
	ReplaceCitations             []emitAnswerCitationV2                 `json:"replace_citations,omitempty"`
	AppendCitations              []emitAnswerCitationV2                 `json:"append_citations,omitempty"`
	ReplaceExactResolution       *types.AnswerExactResolution           `json:"replace_exact_resolution,omitempty"`
	ReplaceMissingRequestedRoles []types.AnswerMissingRequestedRole     `json:"replace_missing_requested_roles,omitempty"`
	ReplaceCaveats               []string                               `json:"replace_caveats,omitempty"`
	ReplaceSnippets              []emitCodeSnippetV2                    `json:"replace_snippets,omitempty"`
}

// emitAnswerDiagramEdgeEdit is a model-authored semantic delta over one
// existing diagram edge. The tool renders the declared operation into the
// previous model-authored Mermaid carrier; it never chooses an endpoint,
// relation kind, or reader-facing label.
type emitAnswerDiagramEdgeEdit struct {
	BlockID            string                   `json:"block_id"`
	Action             string                   `json:"action"`
	FailureRef         string                   `json:"failure_ref,omitempty"`
	AdditionRef        string                   `json:"addition_ref,omitempty"`
	Occurrence         int                      `json:"occurrence,omitempty"`
	BodyOccurrence     int                      `json:"body_occurrence,omitempty"`
	Match              *types.DiagramEdgeAnchor `json:"match,omitempty"`
	Edge               *types.DiagramEdgeAnchor `json:"edge,omitempty"`
	VisibleLabel       string                   `json:"visible_label,omitempty"`
	failureRefResolved bool
	failureRefCarrier  types.AnswerDiagramRelationRepairTargetCarrier
}

type emitAnswerDiagramBoundaryReplacement struct {
	BlockID               string                             `json:"block_id"`
	ParticipantBoundaries []types.DiagramParticipantBoundary `json:"participant_boundaries"`
}

type emitAnswerDiagramParticipantEdit struct {
	BlockID       string `json:"block_id"`
	ParticipantID string `json:"participant_id"`
	Action        string `json:"action"`
}

// localDiagramLeaseWholeBlockMutationViolation guards the execution path as
// well as the projected schema. Tool callers can bypass or lag the current
// schema, but they still cannot widen a typed local diagram lease into a whole
// block replacement/removal. Atomic compiler-generated ReplaceBlocks are not
// inspected here: this function runs on the model's decoded envelope before
// atomic edits are compiled, so it distinguishes the authorized internal
// carrier from a model-authored whole-block mutation without reading prose or
// Mermaid labels.
func localDiagramLeaseWholeBlockMutationViolation(
	p *emitAnswerDocumentPatchParams,
	lease *types.AnswerDiagramRelationRepairLease,
) *types.AnswerDiagramRelationRepairScopeViolation {
	if p == nil {
		return nil
	}
	targets := localDiagramLeaseTargetBlockIDs(lease)
	if len(targets) == 0 {
		return nil
	}
	targetSet := make(map[string]bool, len(targets))
	for _, id := range targets {
		targetSet[id] = true
	}
	for _, block := range p.ReplaceBlocks {
		if id := strings.TrimSpace(block.ID); targetSet[id] {
			return &types.AnswerDiagramRelationRepairScopeViolation{BlockID: id, Issue: "whole_replace_not_authorized"}
		}
	}
	for _, block := range p.AddBlocks {
		if id := strings.TrimSpace(block.ID); targetSet[id] {
			return &types.AnswerDiagramRelationRepairScopeViolation{BlockID: id, Issue: "whole_add_not_authorized"}
		}
	}
	for _, rawID := range p.RemoveBlockIDs {
		if id := strings.TrimSpace(rawID); targetSet[id] {
			return &types.AnswerDiagramRelationRepairScopeViolation{BlockID: id, Issue: "whole_remove_not_authorized"}
		}
	}
	return nil
}

// Execute applies the patch to the previous V2 emit. Failure paths
// surface as Success=false ToolResult so the LLM sees the error
// and can retry with corrected params (the patch validator's
// reject messages name the offending id / op verbatim).
func (t *EmitAnswerDocumentPatch) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	now := time.Now()
	if ctx == nil || ctx.Mutable == nil {
		return failEmit(t.Name(), now,
			"emit_answer_document_patch requires a writable context")
	}

	// Locate the previous emit. Prefer the retry-local staged patch when one
	// exists: it is the exact model-authored merged candidate that produced the
	// latest validator failures and relation refs, not accepted answer state.
	// Otherwise prefer Mutable.AnswerDocumentV2() (the live state — most recent
	// successful emit). Fall back to
	// RetryState.PrevEmitJSON (snapshot taken at retry-decision
	// time) when AnswerDocumentV2 has been cleared by ResetForFallback.
	prev := ctx.Mutable.PendingAnswerDocumentPatchBase()
	if prev == nil {
		prev = ctx.Mutable.AnswerDocumentV2()
	}
	if prev == nil {
		prev = recoverPrevFromRetryState(ctx.Mutable)
	}
	if prev == nil {
		prev = recoverPrevFromRejectedDraft(ctx.Mutable)
		if prev != nil {
			logging.Warning("[emit_answer_document_patch] using previous rejected answer draft as patch base; merged document will be fully revalidated")
		}
	}
	if prev == nil {
		return failEmit(t.Name(), now,
			"emit_answer_document_patch: no previous emit found. The patch tool is only valid on retry paths after a successful emit_answer_document call. First dispatches must use emit_answer_document.")
	}
	if answerDocumentHasTopLevelField(params, "relation_claims") {
		return failEmit(t.Name(), now,
			"top-level field %q is not accepted; place the exact typed claim object(s) under replace_blocks[i].relation_claims or add_blocks[i].relation_claims on the model-authored block that uses the values (never at $.relation_claims)",
			"relation_claims")
	}
	if paths := answerDocumentStructuralCarrierCorruptionPaths(params); len(paths) > 0 {
		return failEmitWithRepair(t.Name(), now, answerDocumentStructuralCarrierCorruptionRepair(paths),
			"answer_document patch carrier contains serialized JSON boundary text in field name(s): %s; retry the patch without dropping any requested block",
			strings.Join(paths, ", "))
	}

	// Flat-mode tolerance for the streaming-bug pattern where an LLM
	// stringifies an array field instead of emitting a real JSON
	// array. Mirrors the protection emit_answer_document already has
	// — applied here so add_blocks / replace_blocks / replace_citations
	// / append_citations / replace_missing_requested_roles /
	// replace_caveats / replace_snippets / unchanged_block_ids /
	// remove_block_ids all get the same Path A/C recovery without
	// forcing an LLM retry round-trip.
	// Repair exact nested aliases before generic schema normalization; see the
	// full-emit path for why this ordering preserves the single-value
	// claim_uses[].facet_ids compatibility lane without accepting ambiguity.
	if repaired, paths, ok := repairNestedArraysInPatch(params); ok {
		logging.Warning("[emit_answer_document_patch] nested block fields normalized via local-model JSON tolerance: %s",
			strings.Join(paths, ", "))
		params = repaired
	}
	if !answerDocumentHasUnresolvedClaimUsePluralFacetIDs(params, "replace_blocks", "add_blocks") {
		params = applyStructuredPayloadCompatWithLegacyStringFieldRepair(t.Name(), params, t.Parameters(), "replace_exact_resolution")
		// The generic pass can turn top-level singleton replace/add block
		// objects into arrays, making their nested fields reachable only now.
		if repaired, paths, ok := repairNestedArraysInPatch(params); ok {
			logging.Warning("[emit_answer_document_patch] nested block fields normalized after top-level JSON tolerance: %s",
				strings.Join(paths, ", "))
			params = repaired
		}
	}
	if repaired, paths, ok := quarantineUnknownAnswerDocumentFields(params, answerDocumentPatchQuarantineProfile); ok {
		logging.Warning("[emit_answer_document_patch] quarantined schema-unknown answer-document patch field(s) without retry: %s",
			strings.Join(paths, ", "))
		params = repaired
	}

	// Decode params.
	dec := json.NewDecoder(bytes.NewReader(params))
	dec.DisallowUnknownFields()
	var p emitAnswerDocumentPatchParams
	if err := dec.Decode(&p); err != nil {
		// Reuse the V2 emit's misplaced-field hint table (same
		// schema for blocks); R4 sanitization always applies.
		return failStrictDecode(t.Name(), now, err, answerDocumentV2MisplacedHints, params)
	}
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	if violation := localDiagramLeaseWholeBlockMutationViolation(&p, lease); violation != nil {
		return failEmitWithRepair(t.Name(), now, answerDiagramRelationRepairScopeRepair(lease, []types.AnswerDiagramRelationRepairScopeViolation{*violation}),
			"local diagram repair lease requires atomic diagram operations for block=%q; whole-block operation=%s is not authorized",
			violation.BlockID, violation.Issue)
	}
	if changed, fields := inheritMissingPatchReplacementKinds(prev, p.ReplaceBlocks); changed {
		logging.Warning("[emit_answer_document_patch] inherited omitted replacement block kind from exact previous block id: %s",
			strings.Join(fields, ", "))
	}
	if changed, fields := inheritMissingPatchReplacementCarrierMetadata(prev, params, p.ReplaceBlocks); changed {
		logging.Warning("[emit_answer_document_patch] inherited omitted stable replacement carrier metadata: %s",
			strings.Join(fields, ", "))
	}

	// Fused-block split runs on the raw lists BEFORE typed
	// conversion (the normalize loop's discriminator repair destroys
	// the declared kind). The diagram half of a fused REPLACE entry
	// moves to add_blocks: replace merges one block per replaced id.
	// The budget bounds block-ADDING splits to the merged doc's
	// remaining headroom under maxBlocksPerDoc — system-inserted
	// halves must never push a within-cap patch into the cap
	// hard-reject. prev + the model's remove/unchanged claims make
	// the derived ids collision-disciplined: a half may only ever
	// collide with (and thereby refresh, budget-free) a prior
	// split's unclaimed kind=diagram block.
	p.ReplaceBlocks, p.AddBlocks = splitFusedDiagramPatchBlocks(t.Name(),
		fusedPatchSplitBudget(prev, p.RemoveBlockIDs, p.ReplaceBlocks, p.AddBlocks),
		prev, p.RemoveBlockIDs, p.UnchangedBlockIDs,
		p.ReplaceBlocks, p.AddBlocks)

	// Build typed AnswerDocumentV2Patch from the decoded params.
	patch := &types.AnswerDocumentV2Patch{
		UnchangedBlockIDs:            append([]string(nil), p.UnchangedBlockIDs...),
		RemoveBlockIDs:               append([]string(nil), p.RemoveBlockIDs...),
		ReplaceCitations:             convertEmitCitationsToTyped(p.ReplaceCitations),
		AppendCitations:              convertEmitCitationsToTyped(p.AppendCitations),
		ReplaceExactResolution:       p.ReplaceExactResolution,
		ReplaceMissingRequestedRoles: p.ReplaceMissingRequestedRoles,
		ReplaceCaveats:               p.ReplaceCaveats,
		ReplaceSnippets:              convertEmitCodeSnippetsToTyped(p.ReplaceSnippets),
	}
	if len(p.ReplaceBlocks) > 0 {
		converted, err := convertEmitBlocksToTyped(t.Name(), p.ReplaceBlocks, "replace_blocks")
		if err != nil {
			return failEmit(t.Name(), now, "%s", err.Error())
		}
		patch.ReplaceBlocks = converted
	}
	if len(p.AddBlocks) > 0 {
		converted, err := convertEmitBlocksToTyped(t.Name(), p.AddBlocks, "add_blocks")
		if err != nil {
			return failEmit(t.Name(), now, "%s", err.Error())
		}
		patch.AddBlocks = converted
	}
	if len(p.DiagramEdgeEdits) > 0 || len(p.DiagramBoundaryReplacements) > 0 || len(p.DiagramParticipantEdits) > 0 {
		view := types.BuildAnswerSemanticViewForBusContext(ctx)
		stagePrecedence := diagramVerifiedReadModeStagePrecedence(ctx, view)
		protectedParticipants := make([]string, 0)
		if view != nil {
			for _, obligation := range view.DiagramParticipantObligations {
				if identity := strings.TrimSpace(obligation.Identity); identity != "" {
					protectedParticipants = append(protectedParticipants, identity)
				}
			}
		}
		if err := applyModelAuthoredDiagramAtomicEditsWithParticipants(
			prev, patch, p.DiagramEdgeEdits, p.DiagramBoundaryReplacements,
			p.DiagramParticipantEdits, protectedParticipants, lease, stagePrecedence,
		); err != nil {
			// A live relation lease is the current generation's complete
			// capability surface. Returning only the first executor error here
			// strands the retry on an old failure_ref/action/selector while hiding
			// every current ref that could replace it. Re-publish the unchanged
			// typed lease together with the exact error summary. The evaluator can
			// then render one bounded delta again; it still does not select, drop,
			// or rewrite any model-authored operation or visible relation.
			if lease != nil {
				repair := answerDiagramRelationRepairScopeRepair(lease, nil)
				repair.Fields = []string{"diagram_edge_edits", "diagram_boundary_replacements", "diagram_participant_edits"}
				repair.Hint = "The submitted atomic diagram operation is not executable under the current relation-repair lease. Re-read the complete current typed delta and choose only live failure_ref/actions or listed additions; do not guess, silently drop, or widen operations."
				return failEmitWithRepair(t.Name(), now, repair, "diagram atomic edits: %s", err.Error())
			}
			return failEmit(t.Name(), now, "diagram atomic edits: %s", err.Error())
		}
	}
	// Stamp only blocks the model submitted in this patch. Unchanged blocks
	// retain the internal provenance captured on their original full/patch
	// emit, while citation refs added by later deterministic normalizers remain
	// explicitly non-model-owned.
	markModelSubmittedItemCitationRefs(&types.AnswerDocumentV2{Blocks: patch.ReplaceBlocks})
	markModelSubmittedItemCitationRefs(&types.AnswerDocumentV2{Blocks: patch.AddBlocks})
	if changed, fields := normalizeSparsePatchRelationMetadataEdits(prev, params, patch); changed {
		logging.Warning("[emit_answer_document_patch] preserved prior model-authored block content for typed relation-metadata-only replacement(s): %s",
			strings.Join(fields, ", "))
	}
	if changed, fields := normalizeAnswerDocumentPatchIDSurface(patch); changed {
		logging.Warning("[emit_answer_document_patch] id/op duplicate(s) normalized via transactional tolerance: %s",
			strings.Join(fields, ", "))
	}
	if changed, fields := normalizeAnswerDocumentPatchNestedItemIDs(prev, patch); changed {
		logging.Warning("[emit_answer_document_patch] nested item id(s) removed from block-level preservation surface: %s",
			strings.Join(fields, ", "))
	}
	if changed, fields := normalizeAnswerDocumentPatchBlockOps(prev, patch); changed {
		logging.Warning("[emit_answer_document_patch] block op(s) normalized via prev-id tolerance: %s",
			strings.Join(fields, ", "))
	}
	// Operation recovery can move an existing id from add_blocks into
	// replace_blocks. The raw replacement-carrier inheritance above runs before
	// that recovery and therefore cannot see the moved block. Reapply the same
	// narrow omitted-field rule over the normalized typed replacement set so a
	// misfiled add cannot silently shed principal/facet ownership and escape the
	// merged-document relation checks. Explicit empty/value fields remain
	// model-owned because this pass still consults their raw JSON presence.
	if changed, fields := inheritMissingNormalizedPatchReplacementCarrierMetadata(prev, params, patch.ReplaceBlocks); changed {
		logging.Warning("[emit_answer_document_patch] inherited omitted stable carrier metadata after block-op normalization: %s",
			strings.Join(fields, ", "))
	}
	if changed, fields := normalizeAnswerDocumentPatchCitationOps(prev, patch); changed {
		logging.Warning("[emit_answer_document_patch] citation op(s) normalized via preserved-pool tolerance: %s",
			strings.Join(fields, ", "))
	}
	if changed, fields := preservePatchReplacementStableItemCitationRefs(prev, patch); changed {
		logging.Warning("[emit_answer_document_patch] preserved stable item citation_ref value(s) across row insertion/removal: %s",
			strings.Join(fields, ", "))
	}
	if changed, fields := preservePatchReplacementTableTails(prev, patch); changed {
		logging.Warning("[emit_answer_document_patch] preserved visible table-tail prose from previous block(s): %s",
			strings.Join(fields, ", "))
	}
	if changed, fields := normalizeAnswerDocumentPatchCitationRefs(prev, patch, ctx); changed {
		logging.Warning("[emit_answer_document_patch] citation_ref value(s) rebound by typed citation evidence: %s",
			strings.Join(fields, ", "))
	}

	// v3 B4 (2026-05-04): route the patch-emit write through the
	// unified mutation runtime — same chokepoint as the full path.
	// Partial Apply runs ApplyAnswerDocumentV2Patch internally;
	// merged-doc invariants (id uniqueness / diagram payload /
	// max blocks) live in ApplyAndPersistMutation.
	mutation := types.NewPartialMutation(patch)
	dropExplicitlyRemovedModelDiagrams := false

	// P1 (2026-05-10) — emit-time pre-validation chokepoint, mirror
	// of the full-emit path. Run on the merged doc shape that the
	// patch produces: dry-run Apply once, run pre-emit checks, then
	// hand off to ApplyAndPersistMutation which re-runs Apply
	// internally. Apply is pure (no side effects on the doc clone)
	// so the dry-run is safe.
	if merged, applyErr := mutation.Apply(prev); applyErr == nil && merged != nil {
		preEmitCtx := newPreEmitCheckContext(ctx)
		// B1265: a local relation lease names canonical typed identities,
		// while the model owns and may submit only the visible node ids and
		// relation it selected from allowed_additions. Restore uniquely matched
		// recipe metadata before the lease compares tuples. The repair cannot
		// create or choose a visible relation, and an unlisted/ambiguous edge is
		// still rejected by the unchanged lease below.
		if ctx.Mutable.AnswerDiagramRelationRepairLease() != nil {
			normalizeDiagramEdgeAnchorIdentitiesFromFinalizerTypedRecipes(t.Name(), merged, ctx, preEmitCtx)
		}
		if lease, violations := validateAndConsumeAnswerDiagramRelationRepairLease(ctx.Mutable, merged); len(violations) > 0 {
			return failEmitWithRepair(t.Name(), now, answerDiagramRelationRepairScopeRepair(lease, violations),
				"answer_document relation repair escaped its local typed scope: %s",
				answerDiagramRelationRepairScopeSummary(violations))
		}
		dropExplicitlyRemovedModelDiagrams = preserveExplicitDiagramRemovalIntent(ctx, mutation, prev)
		if view := types.BuildAnswerSemanticViewForBusContext(ctx); view != nil {
			// Freeze the adoption decision on the merged pre-normalize payload.
			// The per-item raw index snapshot was captured above from only the
			// model-submitted replace/add blocks; inherited/system-owned refs stay
			// outside this obligation.
			markModelSubmittedItemEvidenceIDAdoptionRequired(merged, view, preEmitCtx)
			normalizeAnswerDocumentForPreEmit(t.Name(), merged, view, ctx, preEmitCtx)
			if hints := runPreEmitChecksWithContext(merged, view, preEmitOracleFromCtx(ctx), preEmitCtx); len(hints) > 0 {
				if fixed := materializeRequiredCaveatWhenOnlyMissing(merged, view, hints, ctx); fixed > 0 {
					logging.Warning("[emit_answer_document_patch] materialized %d required caveat block(s) from uncertainty contract", fixed)
					hints = runPreEmitChecksWithContext(merged, view, preEmitOracleFromCtx(ctx), preEmitCtx)
				}
				hardHints, advisoryHints := splitPreEmitHintsByGate(hints)
				if len(advisoryHints) > 0 {
					logSoftPreEmitAdvisory(t.Name(), "pre-emit structural", advisoryHints)
				}
				if len(hardHints) > 0 {
					// Keep accepted state and the initial rejected-full base unchanged,
					// but retain this exact normalized merged candidate as retry-local
					// staging state. The model authored every visible change; the next
					// patch only refines it. This keeps live failure refs and block
					// rosters on one generation without making a rejected answer visible.
					ctx.Mutable.SetPendingAnswerDocumentPatchBase(merged)
					return failEmitWithRepair(t.Name(), now, emitFixHintsRepair(hardHints),
						"%s", formatEmitFixHints(hardHints))
				}
			}
			if hints := preCheckModelSurfaceTerms(merged, ctx); len(hints) > 0 {
				if len(hints) > 0 {
					logSoftPreEmitAdvisory(t.Name(), "model-emitted surface_terms", hints)
				}
			}
			return persistMergedAnswerDocumentWithAttachmentPolicy(
				ctx,
				t.Name(),
				types.MutationPartial,
				mutation.Summary(),
				merged,
				now,
				dropExplicitlyRemovedModelDiagrams,
			)
		}
		// No semantic view means there are no view-specific pre-emit checks, but
		// the exact pre-lease identity repair above is still part of the merged
		// carrier and must not be lost by re-applying the original patch.
		return persistMergedAnswerDocumentWithAttachmentPolicy(
			ctx,
			t.Name(),
			types.MutationPartial,
			mutation.Summary(),
			merged,
			now,
			dropExplicitlyRemovedModelDiagrams,
		)
	}

	return ApplyAndPersistMutation(ctx, t.Name(), mutation, prev, now)
}

// validateAndConsumeAnswerDiagramRelationRepairLease applies one precise
// retry-generation contract. A scope violation retains the lease so the model
// can retry the same local correction. Once a merged patch satisfies that
// scope, the old lease is consumed before any independent pre-emit contract is
// evaluated; a later participant/citation/cardinality failure must establish
// its own typed repair authority instead of inheriting stale edge prohibitions.
func validateAndConsumeAnswerDiagramRelationRepairLease(
	mut *types.MutableState,
	merged *types.AnswerDocumentV2,
) (*types.AnswerDiagramRelationRepairLease, []types.AnswerDiagramRelationRepairScopeViolation) {
	if mut == nil {
		return nil, nil
	}
	lease := mut.AnswerDiagramRelationRepairLease()
	if lease == nil {
		return nil, nil
	}
	violations := types.ValidateAnswerDiagramRelationRepairLease(lease, merged)
	if len(violations) == 0 {
		mut.SetAnswerDiagramRelationRepairLease(nil)
	}
	return lease, violations
}

func answerDiagramRelationRepairScopeRepair(
	lease *types.AnswerDiagramRelationRepairLease,
	violations []types.AnswerDiagramRelationRepairScopeViolation,
) *types.ToolRepair {
	fields := make([]string, 0, len(violations))
	for _, violation := range violations {
		if strings.TrimSpace(violation.FromNode) == "" && strings.TrimSpace(violation.ToNode) == "" {
			fields = append(fields, fmt.Sprintf("blocks[%q].kind:%s", violation.BlockID, violation.Issue))
			continue
		}
		fields = append(fields, fmt.Sprintf("blocks[%q].edge_anchors[%s->%s]:%s",
			violation.BlockID, violation.FromNode, violation.ToNode, violation.Issue))
	}
	metadata := map[string]string{}
	if lease != nil {
		delta := struct {
			Version               int                                          `json:"version"`
			Failures              []types.AnswerDiagramRelationRepairFailure   `json:"failures"`
			PreserveUnlistedEdges bool                                         `json:"preserve_unlisted_edges"`
			AllowedAdditions      []types.AnswerDiagramRelationRepairCandidate `json:"allowed_additions,omitempty"`
		}{
			Version: 1, Failures: append([]types.AnswerDiagramRelationRepairFailure(nil), lease.Failures...),
			PreserveUnlistedEdges: true,
			AllowedAdditions:      append([]types.AnswerDiagramRelationRepairCandidate(nil), lease.AllowedAdditions...),
		}
		if raw, err := json.Marshal(delta); err == nil {
			metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON] = string(raw)
		}
	}
	return &types.ToolRepair{
		Code:     types.ToolRepairCodeAnswerDocRelationRepairScope,
		Hint:     "Keep the existing required diagram block ids, kinds, and count unchanged. Keep every unlisted edge_anchor tuple unchanged; remove or correct only failures[] on the same endpoint pair. You may choose a listed allowed_additions[] row through its addition_ref at most once and still author the visible nodes/label; do not add any other relation.",
		Fields:   fields,
		Metadata: metadata,
	}
}

func answerDiagramRelationRepairScopeSummary(violations []types.AnswerDiagramRelationRepairScopeViolation) string {
	parts := make([]string, 0, len(violations))
	for _, violation := range violations {
		if strings.TrimSpace(violation.FromNode) == "" && strings.TrimSpace(violation.ToNode) == "" {
			parts = append(parts, fmt.Sprintf("block=%s issue=%s", violation.BlockID, violation.Issue))
			continue
		}
		parts = append(parts, fmt.Sprintf("block=%s issue=%s edge=%s->%s",
			violation.BlockID, violation.Issue, violation.FromNode, violation.ToNode))
	}
	return strings.Join(parts, "; ")
}

// normalizeSparsePatchRelationMetadataEdits absorbs one precise retry shape
// that otherwise destroys the answer it is trying to repair. A relation
// validator often asks the model only to add edge_anchors (and sometimes the
// matching claim_uses) to an existing principal block. Models then naturally
// emit {id, edge_anchors} even though replace_blocks is normally a full-block
// replacement. Treating that exact annotation-only object as a full
// replacement erases the model's visible items, kind, role, and facets; the
// later orphan-anchor normalizer then deletes the newly supplied anchors too.
//
// This is deliberately not a general merge operation. The raw replacement may
// contain only id, optional kind, edge_anchors, and optional claim_uses;
// edge_anchors must be explicitly present and non-empty; the id must uniquely
// select an existing block. All visible content remains byte-for-byte from the
// model's previous draft, while the only model-authored delta is copied from
// the typed replacement. Explicit content edits, relation deletion, unknown
// ids, fused blocks, and every other sparse shape keep the normal full-replace
// semantics. No request text, answer prose, or heuristic signal is read.
func normalizeSparsePatchRelationMetadataEdits(prev *types.AnswerDocumentV2, raw json.RawMessage, patch *types.AnswerDocumentV2Patch) (bool, []string) {
	if prev == nil || patch == nil || len(patch.ReplaceBlocks) == 0 || len(raw) == 0 {
		return false, nil
	}
	var envelope struct {
		ReplaceBlocks []map[string]json.RawMessage `json:"replace_blocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.ReplaceBlocks) != len(patch.ReplaceBlocks) {
		return false, nil
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
	allowed := map[string]bool{
		"id": true, "kind": true, "edge_anchors": true, "claim_uses": true,
	}
	var fields []string
	for i := range patch.ReplaceBlocks {
		declared := envelope.ReplaceBlocks[i]
		if _, ok := declared["edge_anchors"]; !ok || len(patch.ReplaceBlocks[i].EdgeAnchors) == 0 {
			continue
		}
		exact := true
		for key := range declared {
			if !allowed[key] {
				exact = false
				break
			}
		}
		id := strings.TrimSpace(patch.ReplaceBlocks[i].ID)
		old, ok := previous[id]
		if !exact || !ok || id == "" || ambiguous[id] {
			continue
		}
		if declaredKind, present := declared["kind"]; present {
			var kind string
			if json.Unmarshal(declaredKind, &kind) != nil || strings.TrimSpace(kind) != string(old.Kind) {
				continue
			}
		}
		replacement := old
		replacement.EdgeAnchors = append([]types.DiagramEdgeAnchor(nil), patch.ReplaceBlocks[i].EdgeAnchors...)
		if _, present := declared["claim_uses"]; present {
			replacement.ClaimUses = append([]types.RenderedClaimUse(nil), patch.ReplaceBlocks[i].ClaimUses...)
		}
		patch.ReplaceBlocks[i] = replacement
		fields = append(fields, fmt.Sprintf("replace_blocks[%q].edge_anchors", id))
	}
	return len(fields) > 0, fields
}

// inheritMissingPatchReplacementKinds removes one recurrent retry-only JSON
// burden without weakening block validation. A replacement already names the
// exact previous block whose display/typed payload it supersedes; when its kind
// is omitted, retaining that previous enum is the only non-invented shape. An
// explicit valid/invalid kind remains model-owned, unknown ids still flow to
// the add-block tolerance and fail without a kind, and add_blocks never inherit.
func inheritMissingPatchReplacementKinds(prev *types.AnswerDocumentV2, blocks []emitAnswerBlockV2) (bool, []string) {
	if prev == nil || len(blocks) == 0 {
		return false, nil
	}
	previousKinds := make(map[string]types.AnswerBlockKind, len(prev.Blocks))
	ambiguous := make(map[string]bool)
	for _, block := range prev.Blocks {
		id := strings.TrimSpace(block.ID)
		if id == "" {
			continue
		}
		if _, exists := previousKinds[id]; exists {
			ambiguous[id] = true
			continue
		}
		previousKinds[id] = block.Kind
	}
	var fields []string
	for i := range blocks {
		id := strings.TrimSpace(blocks[i].ID)
		if strings.TrimSpace(blocks[i].Kind) != "" || id == "" || ambiguous[id] {
			continue
		}
		kind, ok := previousKinds[id]
		if !ok || !types.IsValidAnswerBlockKind(kind) {
			continue
		}
		blocks[i].Kind = string(kind)
		fields = append(fields, fmt.Sprintf("replace_blocks[%d].kind=%s", i, kind))
	}
	return len(fields) > 0, fields
}

// inheritMissingPatchReplacementCarrierMetadata protects two structural
// carrier fields from retry-only omission without turning replace_blocks into
// a general merge operation. The previous block must be uniquely selected by
// its exact id, retain the same typed kind, and share at least one item id that
// is unique in both block versions. Raw JSON field presence distinguishes an
// omitted field from an explicit empty/value, so clearing or changing either
// field remains model-owned. No visible content, evidence claim, relation,
// diagram, citation, or answer conclusion is inherited here.
func inheritMissingPatchReplacementCarrierMetadata(prev *types.AnswerDocumentV2, raw json.RawMessage, blocks []emitAnswerBlockV2) (bool, []string) {
	if prev == nil || len(blocks) == 0 || len(raw) == 0 {
		return false, nil
	}
	var envelope struct {
		ReplaceBlocks []map[string]json.RawMessage `json:"replace_blocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.ReplaceBlocks) != len(blocks) {
		return false, nil
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
	var fields []string
	for i := range blocks {
		id := strings.TrimSpace(blocks[i].ID)
		old, ok := previous[id]
		if !ok || id == "" || ambiguous[id] || string(old.Kind) != strings.TrimSpace(blocks[i].Kind) ||
			!patchReplacementHasUniqueStableItemOverlap(old.Items, blocks[i].Items) {
			continue
		}
		declared := envelope.ReplaceBlocks[i]
		if _, present := declared["facet_ids"]; !present {
			blocks[i].FacetIDs = append([]string(nil), old.FacetIDs...)
			fields = append(fields, fmt.Sprintf("replace_blocks[%d].facet_ids", i))
		}
		if _, present := declared["surface_role"]; !present {
			blocks[i].SurfaceRole = string(old.SurfaceRole)
			fields = append(fields, fmt.Sprintf("replace_blocks[%d].surface_role", i))
		}
	}
	return len(fields) > 0, fields
}

func patchReplacementHasUniqueStableItemOverlap(previous []types.AnswerBlockItem, replacement []emitAnswerBlockItemV2) bool {
	previousCounts := make(map[string]int, len(previous))
	replacementCounts := make(map[string]int, len(replacement))
	for _, item := range previous {
		if id := strings.TrimSpace(item.ID); id != "" {
			previousCounts[id]++
		}
	}
	for _, item := range replacement {
		if id := strings.TrimSpace(item.ID); id != "" {
			replacementCounts[id]++
		}
	}
	for id, count := range previousCounts {
		if count == 1 && replacementCounts[id] == 1 {
			return true
		}
	}
	return false
}

// inheritMissingNormalizedPatchReplacementCarrierMetadata mirrors
// inheritMissingPatchReplacementCarrierMetadata after add/replace operation
// recovery. A model may put an existing id under add_blocks; normalization
// correctly recovers it as a replacement, but must not change whether omitted
// facet_ids/surface_role inherit from the previous block. This helper reads
// schema fields only and never inherits visible content, relation claims,
// anchors, citations, diagrams, or conclusions.
func inheritMissingNormalizedPatchReplacementCarrierMetadata(prev *types.AnswerDocumentV2, raw json.RawMessage, blocks []types.AnswerBlock) (bool, []string) {
	if prev == nil || len(blocks) == 0 || len(raw) == 0 {
		return false, nil
	}
	var envelope struct {
		ReplaceBlocks []map[string]json.RawMessage `json:"replace_blocks"`
		AddBlocks     []map[string]json.RawMessage `json:"add_blocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return false, nil
	}
	declared := make(map[string]map[string]json.RawMessage)
	ambiguousDeclared := make(map[string]bool)
	for _, candidates := range [][]map[string]json.RawMessage{envelope.ReplaceBlocks, envelope.AddBlocks} {
		for _, candidate := range candidates {
			var id string
			if idRaw, ok := candidate["id"]; ok {
				_ = json.Unmarshal(idRaw, &id)
			}
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, exists := declared[id]; exists {
				ambiguousDeclared[id] = true
				continue
			}
			declared[id] = candidate
		}
	}
	previous := make(map[string]types.AnswerBlock, len(prev.Blocks))
	ambiguousPrevious := make(map[string]bool)
	for _, block := range prev.Blocks {
		id := strings.TrimSpace(block.ID)
		if id == "" {
			continue
		}
		if _, exists := previous[id]; exists {
			ambiguousPrevious[id] = true
			continue
		}
		previous[id] = block
	}
	var fields []string
	for i := range blocks {
		id := strings.TrimSpace(blocks[i].ID)
		old, ok := previous[id]
		shape := declared[id]
		if !ok || shape == nil || id == "" || ambiguousPrevious[id] || ambiguousDeclared[id] ||
			old.Kind != blocks[i].Kind || !patchTypedReplacementHasUniqueStableItemOverlap(old.Items, blocks[i].Items) {
			continue
		}
		if _, present := shape["facet_ids"]; !present {
			blocks[i].FacetIDs = append([]string(nil), old.FacetIDs...)
			fields = append(fields, fmt.Sprintf("replace_blocks[%q].facet_ids", id))
		}
		if _, present := shape["surface_role"]; !present {
			blocks[i].SurfaceRole = old.SurfaceRole
			fields = append(fields, fmt.Sprintf("replace_blocks[%q].surface_role", id))
		}
	}
	return len(fields) > 0, fields
}

func patchTypedReplacementHasUniqueStableItemOverlap(previous, replacement []types.AnswerBlockItem) bool {
	previousCounts := make(map[string]int, len(previous))
	replacementCounts := make(map[string]int, len(replacement))
	for _, item := range previous {
		if id := strings.TrimSpace(item.ID); id != "" {
			previousCounts[id]++
		}
	}
	for _, item := range replacement {
		if id := strings.TrimSpace(item.ID); id != "" {
			replacementCounts[id]++
		}
	}
	for id, count := range previousCounts {
		if count == 1 && replacementCounts[id] == 1 {
			return true
		}
	}
	return false
}

// unchanged_block_ids is deliberately a block-level preservation surface.
// Models sometimes copy item ids from the previous-emit JSON into that list
// because item ids are shown beside block ids in the same payload. Treating a
// uniquely owned nested item id as an unknown block makes an otherwise valid
// replacement fail even though the parent block is inherited by default.
//
// This tolerance is structural, not fuzzy: only ids that are absent from the
// previous block-id set and occur as a non-empty item id under exactly one
// previous block are removed. Unknown strings, ambiguous duplicate item ids,
// actual block ids, and every replace/add/remove target retain strict behavior.
func normalizeAnswerDocumentPatchNestedItemIDs(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch) (bool, []string) {
	if prev == nil || patch == nil || len(patch.UnchangedBlockIDs) == 0 {
		return false, nil
	}
	blockIDs := make(map[string]bool, len(prev.Blocks))
	itemOwnerCount := make(map[string]int)
	for _, block := range prev.Blocks {
		if id := strings.TrimSpace(block.ID); id != "" {
			blockIDs[id] = true
		}
		seenInBlock := map[string]bool{}
		for _, item := range block.Items {
			id := strings.TrimSpace(item.ID)
			if id == "" || seenInBlock[id] {
				continue
			}
			seenInBlock[id] = true
			itemOwnerCount[id]++
		}
	}
	out := make([]string, 0, len(patch.UnchangedBlockIDs))
	var fields []string
	for _, raw := range patch.UnchangedBlockIDs {
		id := strings.TrimSpace(raw)
		if !blockIDs[id] && itemOwnerCount[id] == 1 {
			fields = append(fields, fmt.Sprintf("unchanged_block_ids[%q] nested item dropped", id))
			continue
		}
		out = append(out, raw)
	}
	if len(fields) == 0 {
		return false, nil
	}
	patch.UnchangedBlockIDs = out
	return true, fields
}

// preservePatchReplacementStableItemCitationRefs repairs a precise patch-only
// failure mode: a model replaces a list/table block, inserts or removes rows,
// and shifts citation_ref by the row-position delta even though the citation
// pool is inherited and the stable item itself is byte-identical. citation_ref
// indexes the document citation pool, never the item row, so that shift silently
// binds an unchanged claim to a neighbouring source line.
//
// The repair intentionally requires all of the following:
//   - the citation pool is inherited (replace_citations is absent),
//   - previous and replacement blocks share a stable non-empty block id,
//   - previous and replacement items share a unique stable non-empty item id,
//   - every item field except citation_ref is exactly equal, and
//   - the new ref equals old ref plus that item's actual row-position delta.
//
// It does not inspect user/model prose, infer symbol identity, or choose a new
// citation. Intentional citation edits that are not the exact row-delta pattern
// remain model-owned and continue through the normal typed citation checks.
func preservePatchReplacementStableItemCitationRefs(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch) (bool, []string) {
	if prev == nil || patch == nil || patch.ReplaceCitations != nil || len(prev.Citations) == 0 || len(patch.ReplaceBlocks) == 0 {
		return false, nil
	}
	prevBlocks := make(map[string]types.AnswerBlock, len(prev.Blocks))
	duplicateBlocks := map[string]bool{}
	for _, block := range prev.Blocks {
		id := strings.TrimSpace(block.ID)
		if id == "" {
			continue
		}
		if _, exists := prevBlocks[id]; exists {
			duplicateBlocks[id] = true
			continue
		}
		prevBlocks[id] = block
	}

	var fields []string
	for bi := range patch.ReplaceBlocks {
		replacement := &patch.ReplaceBlocks[bi]
		blockID := strings.TrimSpace(replacement.ID)
		previous, ok := prevBlocks[blockID]
		if !ok || duplicateBlocks[blockID] || len(previous.Items) == 0 || len(replacement.Items) == 0 {
			continue
		}
		prevItems := make(map[string]struct {
			index int
			item  types.AnswerBlockItem
		}, len(previous.Items))
		duplicateItems := map[string]bool{}
		for ii, item := range previous.Items {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			if _, exists := prevItems[id]; exists {
				duplicateItems[id] = true
				continue
			}
			prevItems[id] = struct {
				index int
				item  types.AnswerBlockItem
			}{index: ii, item: item}
		}
		for ii := range replacement.Items {
			item := &replacement.Items[ii]
			id := strings.TrimSpace(item.ID)
			old, exists := prevItems[id]
			if id == "" || !exists || duplicateItems[id] || old.item.CitationRef < 0 || old.item.CitationRef >= len(prev.Citations) {
				continue
			}
			oldComparable := old.item
			newComparable := *item
			oldComparable.CitationRef = 0
			newComparable.CitationRef = 0
			oldComparable.CitationRefsModelSubmitted = false
			newComparable.CitationRefsModelSubmitted = false
			oldComparable.CitationRefsModelSubmittedValues = nil
			newComparable.CitationRefsModelSubmittedValues = nil
			oldComparable.CitationRefsEvidenceIDAdoptionRequired = false
			newComparable.CitationRefsEvidenceIDAdoptionRequired = false
			if !reflect.DeepEqual(oldComparable, newComparable) {
				continue
			}
			rowDelta := ii - old.index
			if rowDelta == 0 || item.CitationRef != old.item.CitationRef+rowDelta {
				continue
			}
			item.CitationRef = old.item.CitationRef
			item.CitationRefsModelSubmitted = old.item.CitationRefsModelSubmitted
			item.CitationRefsModelSubmittedValues = append([]int(nil), old.item.CitationRefsModelSubmittedValues...)
			item.CitationRefsEvidenceIDAdoptionRequired = old.item.CitationRefsEvidenceIDAdoptionRequired
			fields = append(fields, fmt.Sprintf("replace_blocks[%q].items[%q]→%d", blockID, id, old.item.CitationRef))
		}
	}
	return len(fields) > 0, fields
}

func normalizeAnswerDocumentPatchIDSurface(patch *types.AnswerDocumentV2Patch) (bool, []string) {
	if patch == nil {
		return false, nil
	}
	var fields []string
	patch.UnchangedBlockIDs = normalizePatchIDList("unchanged_block_ids", patch.UnchangedBlockIDs, &fields)
	patch.RemoveBlockIDs = normalizePatchIDList("remove_block_ids", patch.RemoveBlockIDs, &fields)
	patch.ReplaceBlocks = normalizePatchBlockList("replace_blocks", patch.ReplaceBlocks, &fields)
	patch.AddBlocks = normalizePatchBlockList("add_blocks", patch.AddBlocks, &fields)
	return len(fields) > 0, fields
}

func normalizeAnswerDocumentBlockIDSurface(doc *types.AnswerDocumentV2) (bool, []string) {
	if doc == nil || len(doc.Blocks) == 0 {
		return false, nil
	}
	var fields []string
	doc.Blocks = normalizePatchBlockList("blocks", doc.Blocks, &fields)
	return len(fields) > 0, fields
}

func normalizePatchIDList(field string, ids []string, fields *[]string) []string {
	if len(ids) == 0 {
		return ids
	}
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			*fields = append(*fields, field+"[empty] dropped")
			continue
		}
		if id != raw {
			*fields = append(*fields, fmt.Sprintf("%s[%q] trimmed", field, id))
		}
		if seen[id] {
			*fields = append(*fields, fmt.Sprintf("%s[%q] duplicate dropped", field, id))
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func normalizePatchBlockList(field string, blocks []types.AnswerBlock, fields *[]string) []types.AnswerBlock {
	if len(blocks) == 0 {
		return blocks
	}
	out := make([]types.AnswerBlock, 0, len(blocks))
	seen := map[string]int{}
	for _, block := range blocks {
		rawID := block.ID
		block.ID = strings.TrimSpace(block.ID)
		if block.ID != rawID {
			*fields = append(*fields, fmt.Sprintf("%s[%q] id trimmed", field, block.ID))
		}
		if block.ID == "" {
			out = append(out, block)
			continue
		}
		if idx, ok := seen[block.ID]; ok {
			if reflect.DeepEqual(out[idx], block) {
				*fields = append(*fields, fmt.Sprintf("%s[%q] identical duplicate dropped", field, block.ID))
				continue
			}
			out = append(out, block)
			continue
		}
		seen[block.ID] = len(out)
		out = append(out, block)
	}
	return out
}

func preservePatchReplacementTableTails(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch) (bool, []string) {
	if prev == nil || patch == nil || len(patch.ReplaceBlocks) == 0 {
		return false, nil
	}
	prevByID := make(map[string]types.AnswerBlock, len(prev.Blocks))
	usedIDs := make(map[string]bool, len(prev.Blocks)+len(patch.AddBlocks)+len(patch.ReplaceBlocks))
	for _, block := range prev.Blocks {
		prevByID[block.ID] = block
		if strings.TrimSpace(block.ID) != "" {
			usedIDs[block.ID] = true
		}
	}
	for _, block := range patch.AddBlocks {
		if strings.TrimSpace(block.ID) != "" {
			usedIDs[block.ID] = true
		}
	}
	for _, block := range patch.ReplaceBlocks {
		if strings.TrimSpace(block.ID) != "" {
			usedIDs[block.ID] = true
		}
	}
	var fields []string
	for _, replacement := range patch.ReplaceBlocks {
		if replacement.Kind != types.BlockTable ||
			strings.TrimSpace(replacement.Text) != "" ||
			(len(replacement.Items) == 0 && len(replacement.Columns) == 0) {
			continue
		}
		prevBlock, ok := prevByID[replacement.ID]
		if !ok || prevBlock.Kind != types.BlockTable {
			continue
		}
		tail := markdownTableTrailingProse(prevBlock.Text)
		if tail == "" || answerPatchSurfaceAlreadyCarriesText(patch, tail) {
			continue
		}
		id := uniquePatchBlockID(usedIDs, replacement.ID+"_tail")
		usedIDs[id] = true
		patch.AddBlocks = append(patch.AddBlocks, types.AnswerBlock{
			ID:          id,
			Kind:        types.BlockSection,
			Text:        tail,
			FacetIDs:    append([]string(nil), replacement.FacetIDs...),
			ClaimUses:   append([]types.RenderedClaimUse(nil), replacement.ClaimUses...),
			SurfaceRole: replacement.SurfaceRole,
		})
		fields = append(fields, fmt.Sprintf("replace_blocks[%q].tail→add_blocks[%q]", replacement.ID, id))
	}
	return len(fields) > 0, fields
}

func markdownTableTrailingProse(text string) string {
	text = strings.TrimRight(strings.TrimSpace(text), "\n")
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || !isMarkdownTableLine(lines[i]) {
		return ""
	}
	tableLines := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			break
		}
		if !isMarkdownTableLine(line) {
			break
		}
		tableLines++
		i++
	}
	if tableLines < 2 {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[i:], "\n"))
}

func isMarkdownTableLine(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "|") && strings.HasSuffix(line, "|") && strings.Count(line, "|") >= 2
}

func answerPatchSurfaceAlreadyCarriesText(patch *types.AnswerDocumentV2Patch, text string) bool {
	needle := strings.TrimSpace(text)
	if needle == "" || patch == nil {
		return false
	}
	for _, block := range append(append([]types.AnswerBlock(nil), patch.ReplaceBlocks...), patch.AddBlocks...) {
		if strings.Contains(types.AnswerBlockVisibleSurface(block), needle) {
			return true
		}
	}
	return false
}

func uniquePatchBlockID(used map[string]bool, base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "preserved_tail"
	}
	id := base
	for i := 2; used[id]; i++ {
		id = fmt.Sprintf("%s_%d", base, i)
	}
	return id
}

func normalizeAnswerDocumentPatchBlockOps(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch) (bool, []string) {
	if prev == nil || patch == nil {
		return false, nil
	}
	prevIDs := make(map[string]bool, len(prev.Blocks))
	for _, block := range prev.Blocks {
		id := strings.TrimSpace(block.ID)
		if id != "" {
			prevIDs[id] = true
		}
	}
	if len(prevIDs) == 0 {
		return false, nil
	}
	removeIDs := make(map[string]bool, len(patch.RemoveBlockIDs))
	for _, id := range patch.RemoveBlockIDs {
		removeIDs[strings.TrimSpace(id)] = true
	}
	addIDs := make(map[string]bool, len(patch.AddBlocks))
	for _, block := range patch.AddBlocks {
		addIDs[strings.TrimSpace(block.ID)] = true
	}
	replaceIDs := make(map[string]bool, len(patch.ReplaceBlocks))
	for _, block := range patch.ReplaceBlocks {
		replaceIDs[strings.TrimSpace(block.ID)] = true
	}

	var fields []string
	var keptReplace []types.AnswerBlock
	for _, block := range patch.ReplaceBlocks {
		id := strings.TrimSpace(block.ID)
		if id != "" && !prevIDs[id] && !addIDs[id] && !removeIDs[id] {
			patch.AddBlocks = append(patch.AddBlocks, block)
			addIDs[id] = true
			fields = append(fields, fmt.Sprintf("replace_blocks[%q]→add_blocks", id))
			continue
		}
		keptReplace = append(keptReplace, block)
	}
	patch.ReplaceBlocks = keptReplace

	var keptAdd []types.AnswerBlock
	dropRemoveIDs := make(map[string]bool)
	for _, block := range patch.AddBlocks {
		id := strings.TrimSpace(block.ID)
		if id != "" && prevIDs[id] && !replaceIDs[id] {
			patch.ReplaceBlocks = append(patch.ReplaceBlocks, block)
			replaceIDs[id] = true
			if removeIDs[id] {
				dropRemoveIDs[id] = true
				fields = append(fields, fmt.Sprintf("remove_block_ids[%q]+add_blocks[%q]→replace_blocks", id, id))
			} else {
				fields = append(fields, fmt.Sprintf("add_blocks[%q]→replace_blocks", id))
			}
			continue
		}
		keptAdd = append(keptAdd, block)
	}
	patch.AddBlocks = keptAdd
	if len(dropRemoveIDs) > 0 {
		var keptRemove []string
		for _, id := range patch.RemoveBlockIDs {
			if dropRemoveIDs[strings.TrimSpace(id)] {
				continue
			}
			keptRemove = append(keptRemove, id)
		}
		patch.RemoveBlockIDs = keptRemove
	}

	return len(fields) > 0, fields
}

func normalizeAnswerDocumentPatchCitationOps(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch) (bool, []string) {
	if prev == nil || patch == nil {
		return false, nil
	}
	var fields []string
	if patch.ReplaceCitations != nil && len(patch.AppendCitations) > 0 {
		merged := append([]types.Citation(nil), patch.ReplaceCitations...)
		added := 0
		for _, cit := range patch.AppendCitations {
			if findEquivalentCitation(merged, cit) >= 0 {
				continue
			}
			merged = append(merged, cit)
			added++
		}
		patch.ReplaceCitations = merged
		patch.AppendCitations = nil
		if added > 0 {
			fields = append(fields, fmt.Sprintf("append_citations→replace_citations merged=%d", added))
		} else {
			fields = append(fields, "append_citations duplicate of replace_citations dropped")
		}
	}
	if patch.ReplaceCitations == nil && len(patch.AppendCitations) > 0 {
		// append_citations addresses the inherited document pool. Repeating
		// the same repair must therefore be idempotent: otherwise every failed
		// patch round appends the same source coordinates again and a degraded
		// answer can expose dozens of duplicate references. Deduplicate by the
		// same exact-location identity used by the citation binder and remap
		// only refs that pointed into the old appended suffix. Existing refs and
		// citations with distinct coordinates remain untouched.
		pool := append([]types.Citation(nil), prev.Citations...)
		kept := make([]types.Citation, 0, len(patch.AppendCitations))
		remap := make(map[int]int, len(patch.AppendCitations))
		for i, cit := range patch.AppendCitations {
			oldIndex := len(prev.Citations) + i
			idx := answerDocumentPatchCitationIndex(pool, cit)
			if idx < 0 {
				pool = append(pool, cit)
				kept = append(kept, cit)
				idx = len(pool) - 1
			}
			remap[oldIndex] = idx
		}
		remapped := remapPatchBlockCitationRefs(patch.ReplaceBlocks, remap) +
			remapPatchBlockCitationRefs(patch.AddBlocks, remap)
		if len(kept) != len(patch.AppendCitations) {
			fields = append(fields, fmt.Sprintf("append_citations deduplicated=%d", len(patch.AppendCitations)-len(kept)))
		}
		if remapped > 0 {
			fields = append(fields, fmt.Sprintf("items[].citation_ref remapped=%d", remapped))
		}
		patch.AppendCitations = kept
	}
	if patch.ReplaceCitations == nil {
		return len(fields) > 0, fields
	}
	if !patchPreservesCitationBearingBlock(prev, patch) {
		return len(fields) > 0, fields
	}
	mergedPool := append([]types.Citation(nil), prev.Citations...)
	remap := make(map[int]int, len(patch.ReplaceCitations))
	var appendCitations []types.Citation
	for i, cit := range patch.ReplaceCitations {
		idx := findEquivalentCitation(mergedPool, cit)
		if idx < 0 {
			mergedPool = append(mergedPool, cit)
			appendCitations = append(appendCitations, cit)
			idx = len(mergedPool) - 1
		}
		remap[i] = idx
	}
	remapped := remapPatchBlockCitationRefs(patch.ReplaceBlocks, remap) +
		remapPatchBlockCitationRefs(patch.AddBlocks, remap)
	patch.ReplaceCitations = nil
	patch.AppendCitations = appendCitations
	fields = append(fields, "replace_citations→append_citations")
	if remapped > 0 {
		fields = append(fields, fmt.Sprintf("items[].citation_ref remapped=%d", remapped))
	}
	return true, fields
}

func patchPreservesCitationBearingBlock(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch) bool {
	removed := make(map[string]bool, len(patch.RemoveBlockIDs))
	for _, id := range patch.RemoveBlockIDs {
		removed[strings.TrimSpace(id)] = true
	}
	replaced := make(map[string]bool, len(patch.ReplaceBlocks))
	for _, block := range patch.ReplaceBlocks {
		replaced[strings.TrimSpace(block.ID)] = true
	}
	for _, block := range prev.Blocks {
		id := strings.TrimSpace(block.ID)
		if removed[id] || replaced[id] {
			continue
		}
		if answerBlockHasCitationRefsForPatchTool(block) {
			return true
		}
	}
	return false
}

func answerBlockHasCitationRefsForPatchTool(block types.AnswerBlock) bool {
	for _, item := range block.Items {
		if len(types.AnswerBlockItemCitationRefs(item)) > 0 {
			return true
		}
	}
	return false
}

func findEquivalentCitation(pool []types.Citation, cit types.Citation) int {
	for i, existing := range pool {
		if equivalentAnswerCitation(existing, cit) {
			return i
		}
	}
	return -1
}

func equivalentAnswerCitation(a, b types.Citation) bool {
	return normalizePatchCitationFile(a.File) == normalizePatchCitationFile(b.File) &&
		a.Line == b.Line &&
		a.LineEnd == b.LineEnd &&
		strings.TrimSpace(a.Quote) == strings.TrimSpace(b.Quote) &&
		a.Scope == b.Scope &&
		strings.TrimSpace(a.SectionPath) == strings.TrimSpace(b.SectionPath) &&
		a.FileRoleLabel == b.FileRoleLabel &&
		strings.TrimSpace(a.CrossfileSummary) == strings.TrimSpace(b.CrossfileSummary) &&
		strings.TrimSpace(a.NegativePattern) == strings.TrimSpace(b.NegativePattern) &&
		strings.TrimSpace(a.EnclosingFunction) == strings.TrimSpace(b.EnclosingFunction)
}

func normalizePatchCitationFile(file string) string {
	return strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
}

func remapPatchBlockCitationRefs(blocks []types.AnswerBlock, remap map[int]int) int {
	changed := 0
	for bi := range blocks {
		for ii := range blocks[bi].Items {
			item := &blocks[bi].Items[ii]
			refs := types.AnswerBlockItemCitationRefs(*item)
			mappedRefs := make([]int, 0, len(refs))
			for _, ref := range refs {
				mapped, ok := remap[ref]
				if !ok {
					mappedRefs = append(mappedRefs, ref)
					continue
				}
				mappedRefs = append(mappedRefs, mapped)
				if mapped != ref {
					changed++
				}
			}
			types.SetAnswerBlockItemCitationRefs(item, mappedRefs)
		}
	}
	return changed
}

func normalizeAnswerDocumentPatchCitationRefs(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch, ctx *types.BusContext) (bool, []string) {
	if prev == nil || patch == nil || ctx == nil || ctx.Mutable == nil {
		return false, nil
	}
	pctx := newPreEmitCheckContext(ctx)
	sourceInventorySets := preEmitSourceInventoryTypedPrincipalSets(ctx)
	replacePool := patch.ReplaceCitations != nil
	pool := answerDocumentPatchEffectiveCitationPool(prev, patch)
	if len(pool) == 0 {
		pool = []types.Citation{}
	}
	patchCitations := answerDocumentPatchDeclaredCitationPool(patch)
	var fields []string
	rebindBlocks := func(field string, blocks []types.AnswerBlock) {
		for bi := range blocks {
			block := &blocks[bi]
			if !preEmitBlockRendersItemSurface(block.Kind) || preEmitBlockUsesNonSymbolLabelSurface(*block, nil) {
				continue
			}
			for ii := range block.Items {
				item := &block.Items[ii]
				// source_inventory_row_id is the exact compiler-owned row
				// selector and therefore outranks every label/evidence candidate,
				// including in a family-less mixed table. If that exact typed row
				// intentionally has no citation, detach any stale/model-supplied
				// reference and do not let a weaker label binder invent one.
				if row, ok := preEmitPatchSourceInventoryRowForItem(*block, *item, sourceInventorySets); ok {
					if !row.HasCitation || strings.TrimSpace(row.Source) == "" || row.LineStart <= 0 {
						if len(types.AnswerBlockItemCitationRefs(*item)) > 0 {
							types.SetAnswerBlockItemCitationRefs(item, nil)
							fields = append(fields, fmt.Sprintf("%s[%q].items[%q]→uncited-row", field, block.ID, item.ID))
						}
						continue
					}
					cit := types.Citation{File: row.Source, Line: row.LineStart, LineEnd: row.LineEnd}
					target := answerDocumentPatchCitationIndex(pool, cit)
					if target < 0 {
						if replacePool {
							patch.ReplaceCitations = append(patch.ReplaceCitations, cit)
						} else {
							patch.AppendCitations = append(patch.AppendCitations, cit)
						}
						pool = append(pool, cit)
						target = len(pool) - 1
					}
					if target != item.CitationRef || len(item.CitationRefs) > 0 {
						types.SetAnswerBlockItemCitationRefs(item, []int{target})
						fields = append(fields, fmt.Sprintf("%s[%q].items[%q]→%d", field, block.ID, item.ID, target))
					}
					continue
				}
				label := strings.TrimSpace(item.Label)
				text := preEmitItemNonLabelSurface(*item)
				if label == "" ||
					(!preEmitLabelNeedsCitationAlignment(label) &&
						!preEmitItemMatchesPrincipalAggregateMemberWithContext(pctx, label, text)) {
					continue
				}
				// An exact model-authored source_inventory_family plus one exact
				// typed member row is a stronger selector than the generic
				// candidate-role binder below. Resolve it first on the patch
				// carrier itself: a same-name declaration in another family must
				// never replace the selected row merely because its coarse role is
				// easier to classify. No title, item prose, or request text mints
				// this authority.
				if cit, ok := preEmitPatchSourceInventoryCitationForItem(*block, *item, sourceInventorySets); ok {
					target := answerDocumentPatchCitationIndex(pool, cit)
					if target < 0 {
						if replacePool {
							patch.ReplaceCitations = append(patch.ReplaceCitations, cit)
						} else {
							patch.AppendCitations = append(patch.AppendCitations, cit)
						}
						pool = append(pool, cit)
						target = len(pool) - 1
					}
					if target != item.CitationRef {
						item.CitationRef = target
						fields = append(fields, fmt.Sprintf("%s[%q].items[%q]→%d", field, block.ID, item.ID, target))
					}
					continue
				}
				if item.CitationRef >= 0 && item.CitationRef < len(pool) &&
					preEmitItemCitationAlignedWithContext(pctx, label, text, pool[item.CitationRef]) {
					continue
				}
				cit, ok := preEmitPatchCitationCandidateForItem(pctx, label, text, patchCitations)
				if !ok {
					continue
				}
				target := answerDocumentPatchCitationIndex(pool, cit)
				if target < 0 {
					if replacePool {
						patch.ReplaceCitations = append(patch.ReplaceCitations, cit)
					} else {
						patch.AppendCitations = append(patch.AppendCitations, cit)
					}
					pool = append(pool, cit)
					target = len(pool) - 1
				}
				if target == item.CitationRef {
					continue
				}
				item.CitationRef = target
				fields = append(fields, fmt.Sprintf("%s[%q].items[%q]→%d", field, block.ID, item.ID, target))
			}
		}
	}
	rebindBlocks("replace_blocks", patch.ReplaceBlocks)
	rebindBlocks("add_blocks", patch.AddBlocks)
	return len(fields) > 0, fields
}

func preEmitPatchSourceInventoryRowForItem(
	block types.AnswerBlock,
	item types.AnswerBlockItem,
	sets []types.EnumerationDisplaySet,
) (types.EnumerationDisplayRow, bool) {
	rowID := strings.TrimSpace(item.SourceInventoryRowID)
	if rowID == "" || len(sets) == 0 {
		return types.EnumerationDisplayRow{}, false
	}
	row, ok := preEmitSourceInventoryRowsByIDFromSets(sets)[rowID]
	if !ok {
		return types.EnumerationDisplayRow{}, false
	}
	allowed, invalidFamily := preEmitSourceInventoryBindingRowsForBlock(block, sets)
	if invalidFamily || !preEmitSourceInventoryRowsContainAliasIdentity(allowed, row) {
		return types.EnumerationDisplayRow{}, false
	}
	return row, true
}

func preEmitPatchSourceInventoryCitationForItem(
	block types.AnswerBlock,
	item types.AnswerBlockItem,
	sets []types.EnumerationDisplaySet,
) (types.Citation, bool) {
	if types.SourceInventorySurfaceTermKey(block.SourceInventoryFamily) == "" || len(sets) == 0 {
		return types.Citation{}, false
	}
	rows, invalidFamily := preEmitSourceInventoryBindingRowsForBlock(block, sets)
	if invalidFamily || len(rows) == 0 {
		return types.Citation{}, false
	}
	matches := preEmitSourceInventoryExactLabelRows(item, rows)
	if len(matches) != 1 {
		return types.Citation{}, false
	}
	row := matches[0]
	if !row.HasCitation || strings.TrimSpace(row.Source) == "" || row.LineStart <= 0 {
		return types.Citation{}, false
	}
	return types.Citation{File: row.Source, Line: row.LineStart, LineEnd: row.LineEnd}, true
}

func answerDocumentPatchDeclaredCitationPool(patch *types.AnswerDocumentV2Patch) []types.Citation {
	if patch == nil {
		return nil
	}
	if patch.ReplaceCitations != nil {
		return append([]types.Citation(nil), patch.ReplaceCitations...)
	}
	return append([]types.Citation(nil), patch.AppendCitations...)
}

func answerDocumentPatchEffectiveCitationPool(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch) []types.Citation {
	if patch == nil {
		return nil
	}
	if patch.ReplaceCitations != nil {
		return append([]types.Citation(nil), patch.ReplaceCitations...)
	}
	var pool []types.Citation
	if prev != nil && len(prev.Citations) > 0 {
		pool = append(pool, prev.Citations...)
	}
	if len(patch.AppendCitations) > 0 {
		pool = append(pool, patch.AppendCitations...)
	}
	return pool
}

func answerDocumentPatchCitationIndex(pool []types.Citation, cit types.Citation) int {
	for i, existing := range pool {
		if equivalentAnswerCitation(existing, cit) || preEmitCitationSameLocation(existing, cit) {
			return i
		}
	}
	return -1
}

func preEmitPatchCitationCandidateForItem(pctx *preEmitCheckContext, label, text string, patchCitations []types.Citation) (types.Citation, bool) {
	if cit, ok := preEmitExplicitSourceLocationCitationForPatchItem(pctx, label, text); ok {
		return cit, true
	}
	if cit, ok := preEmitUniqueAlignedPatchCitationForItem(pctx, label, text, patchCitations); ok {
		return cit, true
	}
	if cit, ok := preEmitUniqueCandidateCitationForItemWithContext(pctx, label, text); ok {
		return cit, true
	}
	return types.Citation{}, false
}

func preEmitUniqueAlignedPatchCitationForItem(pctx *preEmitCheckContext, label, text string, citations []types.Citation) (types.Citation, bool) {
	var out []types.Citation
	seen := map[string]bool{}
	for _, cit := range citations {
		if pctx != nil {
			cit = pctx.canonicalCitation(cit)
		}
		if cit.File == "" || cit.Line <= 0 ||
			(!preEmitItemCitationAlignedWithContext(pctx, label, text, cit) &&
				!preEmitPatchCitationMatchesTypedCandidateLocation(pctx, label, text, cit)) {
			continue
		}
		key := preEmitCitationLocationKey(cit)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, cit)
	}
	if len(out) != 1 {
		return types.Citation{}, false
	}
	return out[0], true
}

func preEmitPatchCitationMatchesTypedCandidateLocation(pctx *preEmitCheckContext, label, text string, cit types.Citation) bool {
	if pctx == nil || strings.TrimSpace(cit.File) == "" || cit.Line <= 0 {
		return false
	}
	for _, loc := range preEmitCandidateCitationLocationsForAggregateItemWithContext(pctx, label, text, 8) {
		if preEmitLocationMatchesCitation(loc, cit) {
			return true
		}
	}
	for _, loc := range preEmitCandidateCitationLocationsForLabelWithContext(pctx, label, 8) {
		if preEmitLocationMatchesCitation(loc, cit) {
			return true
		}
	}
	return false
}

func preEmitExplicitSourceLocationCitationForPatchItem(pctx *preEmitCheckContext, label, text string) (types.Citation, bool) {
	surfaces := preEmitExplicitSourceLocationSurfaces(label, text)
	if len(surfaces) != 1 {
		return types.Citation{}, false
	}
	surface := surfaces[0]
	cit := types.Citation{
		File:    surface.File,
		Line:    surface.LineStart,
		LineEnd: surface.LineEnd,
	}
	if pctx != nil {
		cit = pctx.canonicalCitation(cit)
	}
	if !preEmitItemCitationAlignedWithContext(pctx, label, text, cit) {
		return types.Citation{}, false
	}
	return cit, true
}

// recoverPrevFromRetryState attempts to decode the prev emit JSON
// stashed by R14 RetryState. Returns nil when not available or
// decode fails.
func recoverPrevFromRetryState(mut *types.MutableState) *types.AnswerDocumentV2 {
	rs := mut.RetryState()
	if rs == nil || len(rs.PrevEmitJSON) == 0 {
		return nil
	}
	var doc types.AnswerDocumentV2
	if err := json.Unmarshal(rs.PrevEmitJSON, &doc); err != nil {
		logging.Warning("[emit_answer_document_patch] RetryState.PrevEmitJSON decode failed: %v", err)
		return nil
	}
	if len(doc.Blocks) == 0 {
		return nil
	}
	// Re-authenticate the system-side snapshot (marker-stripping class
	// root fix, audit 2026-07-10): PrevEmitJSON lost the json:"-"
	// SystemGeneratedKind markers at snapshot time. Without the re-stamp
	// the patch base's GENUINE system blocks fail RuntimeTraceSystemBlock
	// at persist, get renamed to model_runtime_trace_* by
	// normalizeRuntimeTraceReservedBlockIDCollisions, and the
	// materializers re-mint fresh copies — duplicate runtime-trace
	// chapters with the stale ones laundered as model content. The
	// sidecar was captured from the same in-memory document, so only
	// authority that provably existed is restored (model-direct JSON
	// never reaches this lane).
	types.ReauthenticateSystemSnapshotBlockKinds(&doc, rs.PrevEmitSystemBlockKinds)
	return &doc
}

func recoverPrevFromRejectedDraft(mut *types.MutableState) *types.AnswerDocumentV2 {
	if mut == nil {
		return nil
	}
	doc := mut.LastRejectedAnswerDocumentV2()
	if doc == nil || len(doc.Blocks) == 0 {
		return nil
	}
	return doc
}

// convertEmitBlocksToTyped converts the JSON emitAnswerBlockV2
// shape (FlexInt etc.) into the typed AnswerBlock used by the
// patch. Reuses the same per-block validation surface that V2
// emit goes through, so every dimension the V2 carrier rejects
// (invalid kind, duplicate id within emit, etc.) is also rejected
// at patch time.
//
// Returns ([]AnswerBlock, nil) on success; ("", error) names the
// offending block (with field path) so the LLM can fix the patch.
// convertEmitBlocksToTyped routes through the unified
// NormalizeEmitAnswerBlock so the patch path picks up every typed
// annotation field automatically (G2 post_v2_runtime_gap_remediation,
// 2026-05-04 — pre-G2 this loop silently dropped EdgeAnchors).
func convertEmitBlocksToTyped(toolName string, in []emitAnswerBlockV2, fieldName string) ([]types.AnswerBlock, error) {
	out := make([]types.AnswerBlock, 0, len(in))
	for i, raw := range in {
		blk, err := NormalizeEmitAnswerBlock(raw, fmt.Sprintf("%s: %s[%d]", toolName, fieldName, i))
		if err != nil {
			return nil, err
		}
		out = append(out, blk)
	}
	return out, nil
}

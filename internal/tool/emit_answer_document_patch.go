package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
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

const maxModelAuthoredBlockFieldEditsV1 = 128

func (t *EmitAnswerDocumentPatch) Name() string { return "emit_answer_document_patch" }

func (t *EmitAnswerDocumentPatch) Description() string {
	return "Emit a DELTA against your previous `emit_answer_document` call instead of re-emitting the whole document. Use ONLY on retry paths (when `## Hard Rule (retry attempt N)` appears in the system prompt and a `## Previous Emit` section is present). On first dispatches, use `emit_answer_document` instead.\n\n" +
		"Patch fields (all optional, but at least one MUST be non-empty):\n\n" +
		"- `unchanged_block_ids`: ids of blocks from the previous emit to copy over byte-identical. Use this to assert preservation of every typed annotation/display field (columns, claim_uses, edge_anchors, relation_claims, facet_ids, surface_role, source_inventory_family, items[].cells, items[].candidate_role, items[].source_inventory_row_id, items[].evidence_ids, items[].citation_ref, items[].citation_refs) on blocks you do NOT need to edit. If an id is also targeted by `diagram_edge_edits`, `diagram_boundary_replacements`, `diagram_boundary_edits`, `diagram_relation_scope_edits`, or `diagram_participant_edits`, that unchanged entry is redundant and is absorbed because atomic editing already preserves every unmentioned carrier from the immutable base.\n" +
		"- `replace_blocks`: FULL block payloads, not general field merges. Each entry replaces the previous block with the same id and must carry a non-empty existing id. Copy every previous display/typed field that the required repair does not name (especially title, text, columns, diagram, facet_ids, claim_uses, surface_role), then change only the named field. One narrow retry-safety exception applies: when the exact previous block id and kind are retained, at least one unique stable item id overlaps, and `facet_ids` or `surface_role` is truly omitted, the system retains only those omitted carrier fields; an explicit empty/value remains model-owned. Block payload shape matches the canonical block contract — see below.\n" +
		"- `block_field_edits_v1`: lossless local assignment for one projected closed-enum block metadata field. Choose the exact existing block_id, field, and value branch shown by the current schema. Every other field on that block is copied from the immutable patch base. This branch never edits text, title, items, diagrams, relations, labels, evidence, citations, or layout, and the system never chooses the value. Do not combine it with replace_blocks or remove_block_ids for the same block.\n" +
		"- `add_blocks`: new block payloads to append. Each id must NOT already exist in the previous emit. Block payload shape matches the canonical block contract — see below.\n" +
		"- `remove_block_ids`: ids that must be absent from the resulting document. Repeating an already-satisfied removal is an idempotent no-op.\n" +
		"- `diagram_edge_edits`: model-authored atomic relation edits against one existing block. Visible relabel/remove/replace/add operations require an existing diagram carrier; a non-diagram block may only remove one exact live prior_anchor_metadata row while preserving all visible block fields. Use this instead of `replace_blocks` for a local typed relation retry. Every live failures[] row publishes `target_carrier` and `allowed_actions`; when using its `failure_ref`, choose only an action listed on that row. The live lease resolves only the selected carriers; legacy coordinates and hidden fields are ignored after validation. prior_anchor identifies one mapped anchor/body pair; prior_anchor_metadata identifies exact anchor metadata with no unique visible body occurrence and is remove-only without changing visible content; visible_body_edge identifies an unanchored Mermaid edge; stale_anchor identifies metadata with no body edge; label_pair is relabel-only. If several live failure rows name the same positive body_occurrence and you choose remove for all of them, submit every `{failure_ref, action:\"remove\"}` in the same patch; the executor removes the shared visible statement once and every selected typed anchor transactionally. replace requires the complete model-authored edge/visible_label. For add, prefer one live allowed_additions[].addition_ref: the ref selects only that typed relation candidate while you still author from_node, to_node, visible_label, ordering, and layout; omit edge.relation_kind/from_identity/to_identity and block_id. For add/replace, reuse every endpoint that already has an explicit declaration. The live branch schema derives endpoint declaration state from the exact current patch base: it requires from_node_visible_label/to_node_visible_label for a new endpoint, permits omission for an existing endpoint, and accepts an exact current-label replay after a staged retry. It never derives a name from technical identity. In a sequenceDiagram that already declares participants, use those exact declared participant ids as from_node/to_node instead of creating implicit duplicates. The system preserves every unmentioned model-authored line, node, edge, label, and block field. The model still chooses every operation/relation and writes every visible label.\n" +
		"- `diagram_boundary_replacements`: model-authored replacement of the complete `participant_boundaries` array on an existing diagram block. Use only when the current schema does not publish a narrower boundary ref.\n" +
		"- `diagram_boundary_edits`: model-selected local participant-boundary action published by a live typed repair lease. Copy one exact boundary_ref/action branch from the current schema; the executor changes only that participant row and preserves every unmentioned boundary, Mermaid line, relation, and label.\n" +
		"- `diagram_relation_scope_edits`: model-selected local edit of the block-level `requested_relation_scope` disclosure. This field is exposed only when the current typed request-spine authority proves an exact missing/stale/duplicate scope mismatch. Choose one exact block_id/action branch from the current schema; the executor changes only that typed disclosure and preserves the diagram, relations, labels, layout, and conclusion.\n" +
		"- `diagram_participant_edits`: generation-scoped model-authored declaration edits. For an `optional_orphan_cleanups` row, choose remove_if_isolated or retain_as_context with visible_label. For an exact participant-visibility row, copy participant_ref, choose ensure_visible, and author node_id plus visible_label; the executor adds one disconnected standalone declaration only and creates no edge, anchor, relation, or conclusion. The executor rejects stale refs, unsafe/used node ids, unsupported Mermaid families, protected participants, remaining edges, and ambiguous declarations. The system never chooses the action, node id, or wording.\n" +
		"- `replace_citations`: when present, REPLACES the citation pool entirely. Otherwise the previous citations are inherited. Prefer `append_citations` for additive citation repairs. If you accidentally replace the pool while preserving previous citation-bearing blocks, the tool will keep the previous pool, append genuinely new citations, and remap citation_ref values inside your replace/add blocks.\n" +
		"- `append_citations`: when present and `replace_citations` is absent, appended to the inherited pool.\n" +
		"- `replace_exact_resolution` / `replace_missing_requested_roles` / `replace_caveats`: when present, replace the corresponding document-level field. `replace_snippets` replaces only document-level code snippets shaped as {file,start_line,end_line,language?,code}; use a full `replace_blocks` entry for block items, diagrams, evidence_ids, or any other answer-block field.\n\n" +
		"Validation: every id named in `unchanged_block_ids` / `replace_blocks` MUST exist in the previous emit; `remove_block_ids` is idempotent and may name an already-absent block; every `add_blocks` id MUST NOT exist. Cross-op conflicts (Replace + Remove same id, etc.) are rejected. A live local diagram lease exposes its target through atomic diagram operations; whole replace/add is unavailable, and exact target removal is available only when the typed presentation contract marks that diagram optional. Unrelated blocks remain editable. Block kind is validated against the canonical block-kind enum. The merged document is stored as if you had called `emit_answer_document` with the full payload.\n\n" +
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
    "block_field_edits_v1": {
      "type": "array",
      "maxItems": 128,
      "uniqueItems": true,
      "description": "Version-1 lossless local assignments for projected closed-enum block metadata. Select an exact branch; every unmentioned block field is preserved from the immutable patch base. The model selects the value. Free-form text, diagrams, relations, labels, evidence, citations, items, and layout are not representable here.",
      "items": {
        "oneOf": [
          {"type":"object","additionalProperties":false,"properties":{"block_id":{"type":"string"},"field":{"const":"trace_causal_claim_caliber"},"value":{"type":"string","enum":["no_causal_conclusion","bounded_window_candidate","typed_chain_cause","typed_frame_cause"]}},"required":["block_id","field","value"]},
          {"type":"object","additionalProperties":false,"properties":{"block_id":{"type":"string"},"field":{"const":"current_status_verdict"},"value":{"type":"string","enum":["still_present","fixed","not_enough_evidence"]}},"required":["block_id","field","value"]},
          {"type":"object","additionalProperties":false,"properties":{"block_id":{"type":"string"},"field":{"const":"error_granularity_verdict"},"value":{"type":"string","enum":["per_item_rejection","whole_batch_failure","partial_success","fail_fast","collect_errors","not_enough_evidence"]}},"required":["block_id","field","value"]},
          {"type":"object","additionalProperties":false,"properties":{"block_id":{"type":"string"},"field":{"const":"scope_disclosure"},"value":{"type":"string","enum":["inactive_scope_named","out_of_active_scope","requires_workspace_adjust"]}},"required":["block_id","field","value"]},
          {"type":"object","additionalProperties":false,"properties":{"block_id":{"type":"string"},"field":{"const":"surface_role"},"value":{"type":"string","enum":["principal"]}},"required":["block_id","field","value"]}
        ]
      }
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
      "description": "Atomic model-authored relation edits for an existing block (maximum 128 operations). Visible relabel/remove/replace/add requires a diagram; a non-diagram block accepts only a live remove-only prior_anchor_metadata ref and preserves visible content. Prefer this during a local typed relation retry so unmentioned content stays byte-identical. A live failures[] failure_ref may replace block_id+match+occurrence+body_occurrence only with an action listed by that row's allowed_actions; target_carrier states whether it selects a mapped prior anchor/body pair, remove-only prior anchor metadata with no unique visible body occurrence, visible body-only edge, stale anchor, or label pair. If multiple live rows share one positive body_occurrence and you choose remove for all, submit every ref in the same patch; the shared statement is removed once with all selected anchors. replace requires a complete model-authored edge and visible_label. For add, prefer a live allowed_additions[].addition_ref plus edge.from_node/to_node/visible_label; omit edge.relation_kind/from_identity/to_identity and block_id because the ref supplies those hidden fields. For add/replace, reuse explicitly declared endpoint ids; when an endpoint is new/implicit, provide its matching from_node_visible_label/to_node_visible_label so the reader never sees an unlabeled technical id. When a sequenceDiagram already has explicit participant declarations, reuse their exact ids for edge.from_node/to_node instead of introducing implicit duplicate participants. Legacy add requires block_id+complete edge. add always rejects failure_ref. occurrence is 1-based among exact duplicate anchors and defaults to 1. Without failure_ref, body_occurrence selects the 1-based visible Mermaid edge for an otherwise ambiguous from_node/to_node pair. The system applies only the declared operation and then runs the ordinary typed relation/evidence gates; it never chooses an edge or visible label.",
      "items": {
        "type": "object",
        "properties": {
          "block_id": {"type": "string"},
          "action": {"type": "string", "enum": ["relabel", "remove", "replace", "add"]},
          "failure_ref": {"type": "string", "description": "Opaque selector copied exactly from the live failures[] row. Use only an action listed in that row's allowed_actions. It replaces match, occurrence, and body_occurrence; omit those coordinates because any legacy copies are quarantined after the live ref/action is validated. Omit failure_ref for add. Unknown, stale, disallowed-action, explicit cross-block, ambiguous, or reused refs fail closed."},
          "addition_ref": {"type": "string", "description": "Opaque selector copied exactly from one live allowed_additions[] row. Use only with action=add. It supplies that selected candidate's block_id, relation_kind, from_identity, and to_identity; you still author edge.from_node, edge.to_node, and edge.visible_label. Reuse explicit endpoint declarations; for a new/implicit endpoint also author its corresponding from_node_visible_label/to_node_visible_label. If the sequence body already declares participants, copy those exact declared ids into edge.from_node/to_node. Omit failure_ref and hidden technical fields; legacy hidden-field copies are quarantined after the live ref/action is validated. Unknown, stale, duplicate, explicit cross-block, or wrong-action refs fail closed."},
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
          "visible_label": {"type": "string", "description": "Model-authored reader-facing message for relabel. It updates the matched Mermaid message and anchor.visible_label together."},
          "from_node_visible_label": {"type": "string", "description": "Model-authored reader-facing node name. Required by the executor for add/replace only when edge.from_node has no explicit declaration in the current diagram; omit for an already-declared endpoint."},
          "to_node_visible_label": {"type": "string", "description": "Model-authored reader-facing node name. Required by the executor for add/replace only when edge.to_node has no explicit declaration in the current diagram; omit for an already-declared endpoint."}
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
    "diagram_boundary_edits": {
      "type": "array",
      "maxItems": 128,
      "description": "Generation-scoped local participant-boundary edits. Available only when the current schema publishes exact boundary_ref/action branches; the system changes no edge, relation, label, layout, or conclusion.",
      "items": {
        "type": "object",
        "properties": {
          "boundary_ref": {"type": "string"},
          "action": {"type": "string", "enum": ["add_unproven", "remove_boundary", "deduplicate_boundary"]}
        },
        "required": ["boundary_ref", "action"]
      }
    },
    "diagram_relation_scope_edits": {
      "type": "array",
      "maxItems": 16,
      "description": "Generation-scoped model-selected edits of one diagram block's typed requested_relation_scope disclosure. This surface is projected to exact block_id/action branches only when the current typed request-spine authority reports a missing, stale, or duplicate declaration. It changes no Mermaid content, relation, label, layout, or conclusion.",
      "items": {
        "type": "object",
        "properties": {
          "block_id": {"type": "string"},
          "action": {"type": "string", "enum": ["set_partial_unproven", "remove_scope"]}
        },
        "required": ["block_id", "action"]
      }
    },
    "diagram_participant_edits": {
      "type": "array",
      "maxItems": 64,
      "description": "Generation-scoped model-authored declaration edits. For optional_orphan_cleanups, copy block_id+participant_id and choose remove_if_isolated or retain_as_context with visible_label. For an exact participant visibility failure, copy participant_ref, choose ensure_visible, and author node_id+visible_label. The latter inserts only one disconnected standalone declaration and never creates an edge, anchor, relation, or conclusion.",
      "items": {
        "type": "object",
        "properties": {
          "block_id": {"type": "string"},
          "participant_id": {"type": "string"},
          "participant_ref": {"type": "string"},
          "node_id": {"type": "string"},
          "action": {"type": "string", "enum": ["remove_if_isolated", "retain_as_context", "ensure_visible"]},
          "visible_label": {"type": "string", "description": "Required and model-authored only for action=retain_as_context; omitted for remove_if_isolated."}
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
    "replace_snippets": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "file": {"type": "string"},
          "start_line": {"type": "integer"},
          "end_line": {"type": "integer"},
          "language": {"type": "string"},
          "code": {"type": "string"}
        },
        "required": ["file", "start_line", "end_line", "code"],
        "additionalProperties": false
      },
      "description": "OPTIONAL. Replaces only document-level code snippets. Never place block_id/id/kind/items/diagram/evidence_ids or other answer-block fields here—use a full replace_blocks entry for an existing block."
    }
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
	prev := ctx.Mutable.PendingAnswerDocumentPatchBase()
	if prev == nil {
		prev = ctx.Mutable.AnswerDocumentV2()
	}
	if prev == nil {
		prev = recoverPrevFromRetryState(ctx.Mutable)
	}
	if prev == nil {
		prev = recoverPrevFromRejectedDraft(ctx.Mutable)
	}
	raw = projectAnswerDocumentPatchRelationScopeEdits(raw, ctx, prev)
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	if lease == nil || !types.AnswerDiagramRelationRepairLeaseIsLocallyExecutable(lease) {
		raw = projectAnswerDocumentPatchFieldEditTargets(raw, prev, nil)
		return narrowAnswerDocumentPatchParametersWithoutRelationLease(raw)
	}
	raw = projectAnswerDocumentPatchFieldEditTargets(raw, prev, localDiagramLeaseTargetBlockIDs(lease))
	return narrowAnswerDocumentPatchParametersForLocalDiagramLease(raw, lease, prev)
}

// DescriptionFor keeps the prose surface aligned with the per-dispatch schema.
// A live local lease deliberately exposes a much smaller capability set than
// the compatibility patch envelope, so repeating the legacy whole-block and
// coordinate-selector teaching would ask the model to call operations that the
// same dispatch cannot execute. The lease is typed producer state; no request,
// reasoning, answer prose, or Mermaid label participates in this projection.
func (t *EmitAnswerDocumentPatch) DescriptionFor(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return t.Description()
	}
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	if lease == nil || !types.AnswerDiagramRelationRepairLeaseIsLocallyExecutable(lease) {
		return "Repair the previous structured answer using the executable compatibility operations shown in this tool's current parameter schema. " +
			"`replace_snippets` is only for code snippets {file,start_line,end_line,language?,code}; block items, diagrams, evidence_ids, and other block fields belong in `replace_blocks`. " +
			"For one projected closed-enum block metadata field, prefer `block_field_edits_v1`; it preserves all unmentioned content and you still select the value. " +
			"Atomic diagram edge edits identify an existing block and carry the complete model-authored local match or replacement/addition edge. " +
			"Live opaque selectors and participant cleanup choices are unavailable until a typed relation-repair lease publishes them. " +
			"Whole-block edits remain available for broader model-authored repairs. The system selects no action, relation, visible wording, layout, or conclusion."
	}
	targets := localDiagramLeaseTargetBlockIDs(lease)
	if len(targets) == 0 || !localDiagramLeaseRowsAllTargeted(lease, targets) {
		return "Repair the previous structured answer using the executable compatibility operations shown in this tool's current parameter schema. " +
			"For live relation rows, use a failure_ref only with an action listed in that row, or use one addition_ref with action=add and model-authored visible endpoints and label. " +
			"This broad compatibility schema publishes no paired attach branch: never combine a failure_ref and addition_ref in one edit. Whole-block edits remain available for broader model-authored repairs. " +
			"Unmentioned answer content is preserved from the previous draft. The system selects no action, relation, visible wording, layout, or conclusion."
	}
	description := "Repair the previous structured answer using only the exact current relation-repair choices shown in this tool's parameter schema. " +
		"Select one exact schema branch. A branch may use one published failure_ref, one published addition_ref, or one boundary_ref/action pair that changes only a named participant-boundary row; author every visible endpoint and label required by relation branches. "
	if types.AnswerDiagramRelationRepairHasExecutableAttachPair(lease.Failures, lease.AllowedAdditions) {
		description += "Only an exact action=attach schema branch that fixes both opaque ref values may bind a typed relation to one existing visible edge; never infer a pair from adjacent rows. "
	}
	if lease.AllowTargetDiagramRemoval {
		description += "The optional target diagram may instead be removed only through the exact remove_block_ids enum published in this schema. "
	}
	return description +
		"The current schema is the sole capability authority: omitted legacy coordinates, hidden endpoint identities, and relation kinds are unavailable. When `diagram_relation_scope_edits` is present, use its exact block_id/action branch for the block-level coverage disclosure instead of whole replacement. Whole replacement/addition of a lease-target diagram is unavailable; when `replace_blocks` is present, its id enum contains only unrelated existing blocks that may be repaired alongside the local relation delta. " +
		"Unmentioned answer content is preserved from the previous draft. The system selects no action, relation, visible wording, layout, or conclusion."
}

// narrowAnswerDocumentPatchParametersWithoutRelationLease removes operations
// whose executor requires a live typed relation-repair lease. Historical
// failure/addition refs and orphan-cleanup candidates are generation-scoped;
// advertising them after that generation has been consumed makes the schema
// promise a call that the runtime must reject. The no-lease compatibility lane
// keeps whole-block repairs and legacy block-local edge coordinates because
// those remain executable and model-authored.
func narrowAnswerDocumentPatchParametersWithoutRelationLease(raw json.RawMessage) json.RawMessage {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return raw
	}
	properties, _ := root["properties"].(map[string]any)
	if properties == nil {
		return raw
	}
	edgeEdits, _ := properties["diagram_edge_edits"].(map[string]any)
	items, _ := edgeEdits["items"].(map[string]any)
	itemProperties, _ := items["properties"].(map[string]any)
	if edgeEdits == nil || items == nil || itemProperties == nil {
		return raw
	}
	delete(itemProperties, "failure_ref")
	delete(itemProperties, "addition_ref")
	items["required"] = []any{"block_id", "action"}
	delete(items, "anyOf")
	edgeEdits["description"] = "Atomic model-authored relation edits for an existing block. block_id and action are required. Supply the complete local match for relabel/remove/replace and the complete model-authored edge for replace/add. No generation-scoped opaque selector or participant-cleanup choice is available in this dispatch. The system applies only the declared operation and never chooses an edge, relation, visible label, layout, or conclusion."
	delete(properties, "diagram_participant_edits")
	delete(properties, "diagram_boundary_edits")
	out, err := json.Marshal(root)
	if err != nil || !json.Valid(out) {
		return raw
	}
	return out
}

// narrowAnswerDocumentPatchParametersForLocalDiagramLease projects one live
// typed lease into the operations that its executor can actually perform. The
// model sees exact opaque refs crossed only with their allowed actions, while
// still authoring every visible endpoint, label, layout, and disposition. This
// removes the contradictory legacy coordinate/hidden-identity/whole-block
// surface that previously burned retries after a precise lease had already
// been published. A malformed or mixed non-diagram lease keeps the broad
// compatibility schema and remains fail-closed in the executor.
func narrowAnswerDocumentPatchParametersForLocalDiagramLease(raw json.RawMessage, lease *types.AnswerDiagramRelationRepairLease, prev *types.AnswerDocumentV2) json.RawMessage {
	targets := localDiagramLeaseTargetBlockIDs(lease)
	if len(targets) == 0 || !localDiagramLeaseRowsAllTargeted(lease, targets) {
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
	branches, edgeOK := localDiagramLeaseExecutableEdgeBranches(lease, targets, prev)
	boundaryBranches, boundaryOK := localDiagramLeaseExecutableBoundaryBranches(lease, targets)
	participantOK := len(lease.OptionalOrphanCleanups) > 0 || len(lease.ParticipantVisibilityFailures) > 0
	if !edgeOK && !boundaryOK && !participantOK {
		return raw
	}
	allowedReplacementIDs := unrelatedAnswerDocumentPatchBlockIDs(prev, targets)
	if len(allowedReplacementIDs) == 0 || !narrowAnswerDocumentPatchReplacementIDs(properties, allowedReplacementIDs) {
		delete(properties, "replace_blocks")
	}
	// Adding blocks is not needed for a local relation repair and can change the
	// document roster. Exact replacement or removal of an unrelated existing
	// model-authored block remains available because the same validation round
	// may publish a non-diagram structural failure alongside the relation lease.
	// The exact existing-id enums keep that permission bounded. An optional
	// diagram contract additionally exposes its exact target-removal branch;
	// required diagrams keep target removal absent.
	delete(properties, "add_blocks")
	removableIDs := append([]string(nil), allowedReplacementIDs...)
	if lease.AllowTargetDiagramRemoval {
		removableIDs = append(removableIDs, targets...)
	}
	if len(removableIDs) > 0 {
		sort.Strings(removableIDs)
		removeIDs, _ := properties["remove_block_ids"].(map[string]any)
		if removeIDs == nil {
			return raw
		}
		removeIDs["items"] = map[string]any{"type": "string", "enum": stringsToAny(removableIDs)}
		removeIDs["maxItems"] = len(removableIDs)
		removeIDs["uniqueItems"] = true
		removeIDs["description"] = "Remove only an exact existing model-authored block selected from this dispatch's id enum. Unrelated blocks may be removed for a simultaneously reported structural correction; a lease-target diagram appears only when its typed presentation contract makes that diagram optional."
	} else {
		delete(properties, "remove_block_ids")
	}
	edgeEdits, _ := properties["diagram_edge_edits"].(map[string]any)
	if edgeOK && len(branches) > 0 {
		if edgeEdits == nil {
			return raw
		}
		edgeEdits["minItems"] = 1
		edgeEdits["maxItems"] = localDiagramLeaseTargetSelectorCapacity(lease, targets)
		edgeEdits["uniqueItems"] = true
		edgeDescription := "Choose one or more exact current branches. failure_ref/addition_ref and action are lease-owned choices; replacement/addition endpoints and all visible labels remain model-authored. Omitted legacy coordinates and hidden identity fields are unavailable."
		if types.AnswerDiagramRelationRepairHasExecutableAttachPair(lease.Failures, lease.AllowedAdditions) {
			edgeDescription += " An exact action=attach branch fixes both opaque ref values and binds that selected typed relation to one existing visible edge without adding a duplicate body edge."
		}
		edgeEdits["description"] = edgeDescription
		edgeEdits["items"] = map[string]any{"oneOf": branches}
	} else {
		delete(properties, "diagram_edge_edits")
	}
	if boundaryOK && len(boundaryBranches) > 0 {
		boundaryEdits, _ := properties["diagram_boundary_edits"].(map[string]any)
		if boundaryEdits == nil {
			return raw
		}
		boundaryEdits["minItems"] = 1
		boundaryEdits["maxItems"] = len(lease.ParticipantBoundaryFailures)
		boundaryEdits["uniqueItems"] = true
		boundaryEdits["description"] = "Choose exact current boundary_ref/action branches. Each branch changes only one typed participant-boundary row on the immutable patch base; unmentioned boundaries and every visible graph carrier are preserved."
		boundaryEdits["items"] = map[string]any{"oneOf": boundaryBranches}
		// A live local boundary capability supersedes the error-prone whole-array
		// replacement surface for the same generation.
		delete(properties, "diagram_boundary_replacements")
	} else {
		delete(properties, "diagram_boundary_edits")
	}

	if boundaries, ok := properties["diagram_boundary_replacements"].(map[string]any); ok {
		item, _ := boundaries["items"].(map[string]any)
		itemProperties, _ := item["properties"].(map[string]any)
		blockID, _ := itemProperties["block_id"].(map[string]any)
		if blockID == nil {
			return raw
		}
		blockID["enum"] = stringsToAny(targets)
	}
	if !narrowLocalDiagramParticipantEditSchema(properties, lease) {
		return raw
	}
	if unchanged, ok := properties["unchanged_block_ids"].(map[string]any); ok {
		unchanged["description"] = "Block ids from the previous emit to preserve. A block also named by diagram_edge_edits, diagram_boundary_replacements, diagram_boundary_edits, diagram_relation_scope_edits, or diagram_participant_edits may be listed redundantly; the atomic compiler absorbs that id because every unmentioned carrier is already preserved from the immutable base."
	}
	out, err := json.Marshal(root)
	if err != nil || !json.Valid(out) {
		return raw
	}
	return out
}

func unrelatedAnswerDocumentPatchBlockIDs(prev *types.AnswerDocumentV2, targets []string) []string {
	if prev == nil {
		return nil
	}
	targetSet := make(map[string]bool, len(targets))
	for _, id := range targets {
		if id = strings.TrimSpace(id); id != "" {
			targetSet[id] = true
		}
	}
	counts := map[string]int{}
	for _, block := range prev.Blocks {
		if id := strings.TrimSpace(block.ID); id != "" {
			counts[id]++
		}
	}
	var out []string
	for _, block := range prev.Blocks {
		id := strings.TrimSpace(block.ID)
		if id != "" && counts[id] == 1 && !targetSet[id] && block.SystemGeneratedKind == types.AnswerSystemGeneratedBlockUnknown {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func narrowAnswerDocumentPatchReplacementIDs(properties map[string]any, allowed []string) bool {
	replaceBlocks, _ := properties["replace_blocks"].(map[string]any)
	items, _ := replaceBlocks["items"].(map[string]any)
	blockProperties, _ := items["properties"].(map[string]any)
	idSchema, _ := blockProperties["id"].(map[string]any)
	if replaceBlocks == nil || items == nil || blockProperties == nil || idSchema == nil || len(allowed) == 0 {
		return false
	}
	values := make([]any, 0, len(allowed))
	for _, id := range allowed {
		values = append(values, id)
	}
	idSchema["enum"] = values
	replaceBlocks["maxItems"] = len(allowed)
	replaceBlocks["description"] = "FULL replacement payloads for unrelated existing blocks only. The id enum is the complete executable roster for this live relation-repair dispatch; lease-target diagram blocks must use the published atomic relation/boundary operations."
	return true
}

func localDiagramLeaseRowsAllTargeted(lease *types.AnswerDiagramRelationRepairLease, targets []string) bool {
	if lease == nil || len(targets) == 0 || !types.AnswerDiagramRelationRepairLeaseIsLocallyExecutable(lease) {
		return false
	}
	targetSet := make(map[string]bool, len(targets))
	for _, target := range targets {
		targetSet[target] = true
	}
	for _, failure := range lease.Failures {
		// A single validation round can contain both an actual diagram
		// mismatch and relation metadata on a sibling list/table. The diagram
		// row uses an opaque atomic selector; the non-diagram row remains
		// repairable through an exact unrelated whole-block replacement. Do not
		// make that mixed typed batch fall back to legacy diagram coordinates.
		if !targetSet[strings.TrimSpace(failure.BlockID)] {
			continue
		}
	}
	for _, candidate := range lease.AllowedAdditions {
		if !targetSet[strings.TrimSpace(candidate.BlockID)] {
			continue
		}
	}
	for _, failure := range lease.ParticipantBoundaryFailures {
		if !targetSet[strings.TrimSpace(failure.BlockID)] || strings.TrimSpace(failure.BoundaryRef) == "" ||
			len(failure.AllowedBoundaryActions) == 0 {
			return false
		}
	}
	for _, failure := range lease.ParticipantVisibilityFailures {
		if !targetSet[strings.TrimSpace(failure.BlockID)] || strings.TrimSpace(failure.ParticipantRef) == "" ||
			len(failure.AllowedParticipantActions) == 0 {
			return false
		}
	}
	return len(lease.Failures)+len(lease.AllowedAdditions)+len(lease.ParticipantBoundaryFailures)+len(lease.ParticipantVisibilityFailures) > 0
}

func localDiagramLeaseExecutableBoundaryBranches(
	lease *types.AnswerDiagramRelationRepairLease,
	targets []string,
) ([]any, bool) {
	if lease == nil || !localDiagramLeaseRowsAllTargeted(lease, targets) || len(lease.ParticipantBoundaryFailures) == 0 {
		return nil, false
	}
	seen := make(map[string]bool, len(lease.ParticipantBoundaryFailures))
	branches := make([]any, 0, len(lease.ParticipantBoundaryFailures))
	for _, failure := range lease.ParticipantBoundaryFailures {
		ref := strings.TrimSpace(failure.BoundaryRef)
		if ref == "" || seen[ref] || len(failure.AllowedBoundaryActions) != 1 {
			return nil, false
		}
		seen[ref] = true
		action := strings.TrimSpace(string(failure.AllowedBoundaryActions[0]))
		branches = append(branches, map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"boundary_ref": map[string]any{"type": "string", "enum": []any{ref}},
				"action":       map[string]any{"type": "string", "enum": []any{action}},
			},
			"required": []any{"boundary_ref", "action"},
		})
	}
	return branches, len(branches) > 0
}

func localDiagramLeaseExecutableEdgeBranches(
	lease *types.AnswerDiagramRelationRepairLease,
	targets []string,
	prev *types.AnswerDocumentV2,
) ([]any, bool) {
	if lease == nil || !localDiagramLeaseRowsAllTargeted(lease, targets) {
		return nil, false
	}
	seenRefs := make(map[string]bool, len(lease.Failures)+len(lease.AllowedAdditions))
	targetSet := make(map[string]bool, len(targets))
	for _, target := range targets {
		targetSet[strings.TrimSpace(target)] = true
	}
	branches := make([]any, 0, len(lease.Failures)*2+len(lease.AllowedAdditions))
	for _, failure := range lease.Failures {
		if !targetSet[strings.TrimSpace(failure.BlockID)] {
			continue
		}
		ref := strings.TrimSpace(failure.FailureRef)
		if ref == "" || seenRefs[ref] || len(failure.AllowedActions) == 0 {
			return nil, false
		}
		seenRefs[ref] = true
		added := 0
		for _, allowed := range failure.AllowedActions {
			action := strings.TrimSpace(string(allowed))
			endpointDeclarations := explicitDiagramEndpointDeclarations(prev, failure.BlockID)
			var branch map[string]any
			switch action {
			case string(types.AnswerDiagramRelationRepairActionRemove):
				branch = exactLocalDiagramEdgeBranch("failure_ref", ref, action, false, false)
			case string(types.AnswerDiagramRelationRepairActionRelabel):
				branch = exactLocalDiagramEdgeBranch("failure_ref", ref, action, false, true)
			case string(types.AnswerDiagramRelationRepairActionReplace):
				branch = exactLocalDiagramEdgeBranch("failure_ref", ref, action, true, false, endpointDeclarations)
			case string(types.AnswerDiagramRelationRepairActionAttach):
				for _, candidate := range lease.AllowedAdditions {
					if !types.AnswerDiagramRelationRepairFailureCanAttachCandidate(failure, candidate) {
						continue
					}
					branches = append(branches, exactLocalDiagramAttachBranch(ref, candidate.AdditionRef))
					added++
				}
				continue
			default:
				return nil, false
			}
			branches = append(branches, branch)
			added++
		}
		if added == 0 {
			return nil, false
		}
	}
	for _, candidate := range lease.AllowedAdditions {
		if !targetSet[strings.TrimSpace(candidate.BlockID)] {
			continue
		}
		ref := strings.TrimSpace(candidate.AdditionRef)
		if ref == "" || seenRefs[ref] {
			return nil, false
		}
		seenRefs[ref] = true
		attachedToExistingBody := false
		for _, failure := range lease.Failures {
			if types.AnswerDiagramRelationRepairFailureCanAttachCandidate(failure, candidate) {
				attachedToExistingBody = true
				break
			}
		}
		if attachedToExistingBody {
			continue
		}
		branches = append(branches, exactLocalDiagramEdgeBranch(
			"addition_ref", ref, "add", true, false,
			explicitDiagramEndpointDeclarations(prev, candidate.BlockID),
		))
	}
	return branches, true
}

func localDiagramLeaseTargetSelectorCapacity(lease *types.AnswerDiagramRelationRepairLease, targets []string) int {
	if lease == nil {
		return 0
	}
	targetSet := make(map[string]bool, len(targets))
	for _, target := range targets {
		targetSet[strings.TrimSpace(target)] = true
	}
	count := 0
	for _, failure := range lease.Failures {
		if targetSet[strings.TrimSpace(failure.BlockID)] {
			count++
		}
	}
	for _, candidate := range lease.AllowedAdditions {
		if targetSet[strings.TrimSpace(candidate.BlockID)] {
			count++
		}
	}
	if count < 1 {
		return 1
	}
	return count
}

func exactLocalDiagramAttachBranch(failureRef, additionRef string) map[string]any {
	properties := map[string]any{
		"failure_ref":  map[string]any{"type": "string", "enum": []any{strings.TrimSpace(failureRef)}},
		"addition_ref": map[string]any{"type": "string", "enum": []any{strings.TrimSpace(additionRef)}},
		"action":       map[string]any{"type": "string", "enum": []any{string(types.AnswerDiagramRelationRepairActionAttach)}},
		"edge": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"from_node":     map[string]any{"type": "string", "minLength": 1},
				"to_node":       map[string]any{"type": "string", "minLength": 1},
				"visible_label": map[string]any{"type": "string", "minLength": 1},
			},
			"required": []any{"from_node", "to_node", "visible_label"},
		},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
		"required":             []any{"failure_ref", "addition_ref", "action", "edge"},
	}
}

type explicitDiagramEndpointDeclaration struct {
	ID    string
	Label string
}

// explicitDiagramEndpointDeclarations projects only syntax-owned declarations
// from the exact immutable patch base. It never reads request prose, answer
// prose, relation messages, or inferred identities. Conflicting declarations
// stay absent, matching the executor's fail-closed label registry.
func explicitDiagramEndpointDeclarations(prev *types.AnswerDocumentV2, blockID string) []explicitDiagramEndpointDeclaration {
	if prev == nil || strings.TrimSpace(blockID) == "" {
		return nil
	}
	var diagram *types.AnswerDiagramBlock
	count := 0
	for i := range prev.Blocks {
		if strings.TrimSpace(prev.Blocks[i].ID) != strings.TrimSpace(blockID) {
			continue
		}
		count++
		diagram = prev.Blocks[i].Diagram
	}
	if count != 1 || diagram == nil {
		return nil
	}
	uniqueLabels := diagramEvidenceNodeLabels(diagram.Body, diagram.Kind)
	if len(uniqueLabels) == 0 {
		return nil
	}
	byFoldedID := make(map[string]explicitDiagramEndpointDeclaration, len(uniqueLabels))
	sequenceSyntax := diagramEvidenceUsesSequenceSyntax(diagram.Body, diagram.Kind)
	for _, line := range strings.Split(diagram.Body, "\n") {
		var declarations []mermaidcompat.NodeDecl
		if sequenceSyntax {
			declarations = mermaidcompat.SequenceParticipantDeclarations(line)
		} else {
			declarations = mermaidcompat.NodeDeclarationsAll(line)
		}
		for _, declaration := range declarations {
			id := strings.TrimSpace(declaration.Ident)
			key := strings.ToLower(id)
			label, ok := uniqueLabels[key]
			if id == "" || !ok || strings.TrimSpace(declaration.Label) != strings.TrimSpace(label) {
				continue
			}
			if _, exists := byFoldedID[key]; !exists {
				byFoldedID[key] = explicitDiagramEndpointDeclaration{ID: id, Label: label}
			}
		}
	}
	declarations := make([]explicitDiagramEndpointDeclaration, 0, len(byFoldedID))
	for _, declaration := range byFoldedID {
		declarations = append(declarations, declaration)
	}
	sort.Slice(declarations, func(i, j int) bool {
		return declarations[i].ID < declarations[j].ID
	})
	return declarations
}

func exactLocalDiagramEdgeBranch(
	refField, ref, action string,
	needsEdge, needsLabel bool,
	explicitEndpoints ...[]explicitDiagramEndpointDeclaration,
) map[string]any {
	properties := map[string]any{
		refField: map[string]any{"type": "string", "enum": []any{ref}},
		"action": map[string]any{"type": "string", "enum": []any{action}},
	}
	required := []any{refField, "action"}
	if needsEdge {
		declarations := []explicitDiagramEndpointDeclaration(nil)
		if len(explicitEndpoints) > 0 {
			declarations = explicitEndpoints[0]
		}
		properties["edge"] = map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"from_node":     map[string]any{"type": "string", "minLength": 1},
				"to_node":       map[string]any{"type": "string", "minLength": 1},
				"visible_label": map[string]any{"type": "string", "minLength": 1},
			},
			"required": []any{"from_node", "to_node", "visible_label"},
		}
		required = append(required, "edge")
		properties["from_node_visible_label"] = map[string]any{
			"type": "string", "minLength": 1,
			"description": "Model-authored reader-facing name for edge.from_node. The current branch requires it for a new endpoint. For an already explicit endpoint, omit it or replay only the exact current label.",
		}
		properties["to_node_visible_label"] = map[string]any{
			"type": "string", "minLength": 1,
			"description": "Model-authored reader-facing name for edge.to_node. The current branch requires it for a new endpoint. For an already explicit endpoint, omit it or replay only the exact current label.",
		}
		return exactLocalDiagramEndpointLabelContract(map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           properties,
			"required":             required,
		}, declarations)
	}
	if needsLabel {
		properties["visible_label"] = map[string]any{"type": "string", "minLength": 1}
		required = append(required, "visible_label")
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
		"required":             required,
	}
}

// exactLocalDiagramEndpointLabelContract makes the endpoint-name obligation
// state-dependent in the tool schema itself. New endpoints require a visible
// name. Existing exact IDs may omit it; a retry may also replay only the exact
// current label. The executor repeats the same check transactionally.
func exactLocalDiagramEndpointLabelContract(
	branch map[string]any,
	declarations []explicitDiagramEndpointDeclaration,
) map[string]any {
	if len(declarations) == 0 {
		branch["required"] = append(branch["required"].([]any), "from_node_visible_label", "to_node_visible_label")
		return branch
	}
	explicitIDs := make([]any, 0, len(declarations))
	allOf := make([]any, 0, 2+2*len(declarations))
	for _, declaration := range declarations {
		explicitIDs = append(explicitIDs, declaration.ID)
		for _, endpoint := range []string{"from", "to"} {
			allOf = append(allOf, map[string]any{
				"if": map[string]any{
					"properties": map[string]any{
						"edge": map[string]any{
							"properties": map[string]any{endpoint + "_node": map[string]any{"enum": []any{declaration.ID}}},
							"required":   []any{endpoint + "_node"},
						},
					},
					"required": []any{"edge"},
				},
				"then": map[string]any{
					"properties": map[string]any{
						endpoint + "_node_visible_label": map[string]any{"enum": []any{declaration.Label}},
					},
				},
			})
		}
	}
	for _, endpoint := range []string{"from", "to"} {
		allOf = append(allOf, map[string]any{
			"if": map[string]any{
				"properties": map[string]any{
					"edge": map[string]any{
						"properties": map[string]any{endpoint + "_node": map[string]any{"enum": explicitIDs}},
						"required":   []any{endpoint + "_node"},
					},
				},
				"required": []any{"edge"},
			},
			"else": map[string]any{"required": []any{endpoint + "_node_visible_label"}},
		})
	}
	branch["allOf"] = allOf
	branch["description"] = "Endpoint declaration state is derived from the exact current diagram base. Existing endpoint IDs may omit a visible-label field or replay their exact current label; every other endpoint ID requires a model-authored visible label."
	return branch
}

func narrowLocalDiagramParticipantEditSchema(properties map[string]any, lease *types.AnswerDiagramRelationRepairLease) bool {
	if len(lease.OptionalOrphanCleanups) == 0 && len(lease.ParticipantVisibilityFailures) == 0 {
		delete(properties, "diagram_participant_edits")
		return true
	}
	participantEdits, _ := properties["diagram_participant_edits"].(map[string]any)
	if participantEdits == nil {
		return false
	}
	branches := make([]any, 0, len(lease.OptionalOrphanCleanups)*2+len(lease.ParticipantVisibilityFailures))
	for _, candidate := range lease.OptionalOrphanCleanups {
		blockID := strings.TrimSpace(candidate.BlockID)
		participantID := strings.TrimSpace(candidate.ParticipantID)
		if blockID == "" || participantID == "" || len(candidate.AllowedActions) == 0 {
			return false
		}
		for _, allowed := range candidate.AllowedActions {
			action := strings.TrimSpace(string(allowed))
			properties := map[string]any{
				"block_id":       map[string]any{"type": "string", "enum": []any{blockID}},
				"participant_id": map[string]any{"type": "string", "enum": []any{participantID}},
				"action":         map[string]any{"type": "string", "enum": []any{action}},
			}
			required := []any{"block_id", "participant_id", "action"}
			switch action {
			case string(types.AnswerDiagramOrphanDispositionRemove):
			case string(types.AnswerDiagramOrphanDispositionRetain):
				properties["visible_label"] = map[string]any{"type": "string", "minLength": 1}
				required = append(required, "visible_label")
			default:
				return false
			}
			branches = append(branches, map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           properties,
				"required":             required,
			})
		}
	}
	for _, failure := range lease.ParticipantVisibilityFailures {
		ref := strings.TrimSpace(failure.ParticipantRef)
		if ref == "" || len(failure.AllowedParticipantActions) != 1 ||
			failure.AllowedParticipantActions[0] != types.AnswerDiagramParticipantVisibilityRepairEnsureVisible {
			return false
		}
		branches = append(branches, map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"participant_ref": map[string]any{"type": "string", "enum": []any{ref}},
				"action":          map[string]any{"type": "string", "enum": []any{string(types.AnswerDiagramParticipantVisibilityRepairEnsureVisible)}},
				"node_id":         map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
				"visible_label":   map[string]any{"type": "string", "minLength": 1, "maxLength": 512},
			},
			"required": []any{"participant_ref", "action", "node_id", "visible_label"},
		})
	}
	participantEdits["minItems"] = 1
	participantEdits["maxItems"] = len(lease.OptionalOrphanCleanups) + len(lease.ParticipantVisibilityFailures)
	participantEdits["uniqueItems"] = true
	participantEdits["items"] = map[string]any{"oneOf": branches}
	return true
}

func stringsToAny(in []string) []any {
	out := make([]any, 0, len(in))
	for _, value := range in {
		out = append(out, value)
	}
	return out
}

func localDiagramLeaseTargetBlockIDs(lease *types.AnswerDiagramRelationRepairLease) []string {
	if lease == nil || lease.Version != 1 ||
		(len(lease.Failures) == 0 && len(lease.AllowedAdditions) == 0 && len(lease.ParticipantBoundaryFailures) == 0 && len(lease.ParticipantVisibilityFailures) == 0) {
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
	seen := make(map[string]bool, len(lease.Failures)+len(lease.AllowedAdditions)+len(lease.ParticipantBoundaryFailures)+len(lease.ParticipantVisibilityFailures))
	out := make([]string, 0, len(lease.Failures)+len(lease.AllowedAdditions)+len(lease.ParticipantBoundaryFailures)+len(lease.ParticipantVisibilityFailures))
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
	for _, candidate := range lease.AllowedAdditions {
		id := strings.TrimSpace(candidate.BlockID)
		if id == "" || strings.TrimSpace(candidate.AdditionRef) == "" ||
			!candidate.RelationKind.IsValid() || strings.TrimSpace(candidate.FromIdentity) == "" ||
			strings.TrimSpace(candidate.ToIdentity) == "" || strings.TrimSpace(candidate.Source) == "" {
			// A malformed lease must not narrow a hard tool surface.
			return nil
		}
		if ambiguousBlocks[id] || !diagramBlocks[id] {
			continue
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, failure := range lease.ParticipantBoundaryFailures {
		id := strings.TrimSpace(failure.BlockID)
		if id == "" || strings.TrimSpace(failure.BoundaryRef) == "" || len(failure.AllowedBoundaryActions) == 0 {
			return nil
		}
		if ambiguousBlocks[id] || !diagramBlocks[id] {
			continue
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, failure := range lease.ParticipantVisibilityFailures {
		id := strings.TrimSpace(failure.BlockID)
		if id == "" || strings.TrimSpace(failure.ParticipantRef) == "" || len(failure.AllowedParticipantActions) == 0 {
			return nil
		}
		if ambiguousBlocks[id] || !diagramBlocks[id] {
			continue
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
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
	projectAnswerDocumentPatchFieldEditBranches(patchProperties, blockItem)
	out, err := json.Marshal(patchRoot)
	if err != nil || !json.Valid(out) {
		return (&EmitAnswerDocumentPatch{}).Parameters()
	}
	return out
}

// projectAnswerDocumentPatchFieldEditBranches derives the local field-edit
// vocabulary from the same dispatch-projected block schema used by a full
// emit. A field omitted by the active typed contract cannot be resurrected by
// the patch surface. Values are copied from the projected enum; no request or
// answer prose participates.
func projectAnswerDocumentPatchFieldEditBranches(patchProperties map[string]any, blockItem map[string]any) {
	edits, _ := patchProperties["block_field_edits_v1"].(map[string]any)
	items, _ := edits["items"].(map[string]any)
	blockProps, _ := blockItem["properties"].(map[string]any)
	if edits == nil || items == nil || blockProps == nil {
		delete(patchProperties, "block_field_edits_v1")
		return
	}
	fields := []string{
		string(types.AnswerBlockFieldTraceCausalClaimCaliber),
		string(types.AnswerBlockFieldCurrentStatusVerdict),
		string(types.AnswerBlockFieldErrorGranularityVerdict),
		string(types.AnswerBlockFieldScopeDisclosure),
		string(types.AnswerBlockFieldSurfaceRole),
	}
	branches := make([]any, 0, len(fields))
	for _, field := range fields {
		node, _ := blockProps[field].(map[string]any)
		values, _ := node["enum"].([]any)
		if node == nil || len(values) == 0 {
			continue
		}
		branches = append(branches, map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"block_id": map[string]any{"type": "string"},
				"field":    map[string]any{"const": field},
				"value":    map[string]any{"type": "string", "enum": append([]any(nil), values...)},
			},
			"required": []any{"block_id", "field", "value"},
		})
	}
	if len(branches) == 0 {
		delete(patchProperties, "block_field_edits_v1")
		return
	}
	items["oneOf"] = branches
}

// projectAnswerDocumentPatchFieldEditTargets narrows every field branch to
// exact existing model-owned blocks of a compatible kind. excluded contains
// lease-target diagram ids whose current repair capability must stay atomic.
// The projection is typed state only and never reads visible content.
func projectAnswerDocumentPatchFieldEditTargets(raw json.RawMessage, prev *types.AnswerDocumentV2, excluded []string) json.RawMessage {
	if prev == nil {
		return raw
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return raw
	}
	properties, _ := root["properties"].(map[string]any)
	edits, _ := properties["block_field_edits_v1"].(map[string]any)
	items, _ := edits["items"].(map[string]any)
	branches, _ := items["oneOf"].([]any)
	if properties == nil || edits == nil || items == nil || len(branches) == 0 {
		return raw
	}
	excludedSet := make(map[string]bool, len(excluded))
	for _, id := range excluded {
		if id = strings.TrimSpace(id); id != "" {
			excludedSet[id] = true
		}
	}
	var projected []any
	for _, rawBranch := range branches {
		branch, _ := rawBranch.(map[string]any)
		branchProps, _ := branch["properties"].(map[string]any)
		fieldNode, _ := branchProps["field"].(map[string]any)
		field, _ := fieldNode["const"].(string)
		if branch == nil || branchProps == nil || field == "" {
			continue
		}
		var ids []any
		for _, block := range prev.Blocks {
			id := strings.TrimSpace(block.ID)
			if id == "" || excludedSet[id] || block.SystemGeneratedKind != types.AnswerSystemGeneratedBlockUnknown ||
				!answerBlockFieldEditV1KindCompatible(field, block.Kind) {
				continue
			}
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			continue
		}
		blockID, _ := branchProps["block_id"].(map[string]any)
		if blockID == nil {
			continue
		}
		blockID["enum"] = ids
		projected = append(projected, branch)
	}
	if len(projected) == 0 {
		delete(properties, "block_field_edits_v1")
	} else {
		items["oneOf"] = projected
	}
	out, err := json.Marshal(root)
	if err != nil || !json.Valid(out) {
		return raw
	}
	return out
}

func answerBlockFieldEditV1KindCompatible(field string, kind types.AnswerBlockKind) bool {
	switch types.AnswerBlockEditableFieldV1(field) {
	case types.AnswerBlockFieldTraceCausalClaimCaliber:
		return kind == types.BlockSummary
	case types.AnswerBlockFieldCurrentStatusVerdict, types.AnswerBlockFieldErrorGranularityVerdict:
		return kind == types.BlockDecision
	case types.AnswerBlockFieldScopeDisclosure, types.AnswerBlockFieldSurfaceRole:
		return types.IsValidAnswerBlockKind(kind)
	default:
		return false
	}
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
	BlockFieldEditsV1            []types.AnswerBlockFieldEditV1         `json:"block_field_edits_v1,omitempty"`
	DiagramEdgeEdits             []emitAnswerDiagramEdgeEdit            `json:"diagram_edge_edits,omitempty"`
	DiagramBoundaryReplacements  []emitAnswerDiagramBoundaryReplacement `json:"diagram_boundary_replacements,omitempty"`
	DiagramBoundaryEdits         []emitAnswerDiagramBoundaryEdit        `json:"diagram_boundary_edits,omitempty"`
	DiagramRelationScopeEdits    []emitAnswerDiagramRelationScopeEdit   `json:"diagram_relation_scope_edits,omitempty"`
	DiagramParticipantEdits      []emitAnswerDiagramParticipantEdit     `json:"diagram_participant_edits,omitempty"`
	ReplaceCitations             []emitAnswerCitationV2                 `json:"replace_citations,omitempty"`
	AppendCitations              []emitAnswerCitationV2                 `json:"append_citations,omitempty"`
	ReplaceExactResolution       *types.AnswerExactResolution           `json:"replace_exact_resolution,omitempty"`
	ReplaceMissingRequestedRoles []types.AnswerMissingRequestedRole     `json:"replace_missing_requested_roles,omitempty"`
	ReplaceCaveats               []string                               `json:"replace_caveats,omitempty"`
	ReplaceSnippets              []emitCodeSnippetV2                    `json:"replace_snippets,omitempty"`
}

type answerDocumentPatchFieldEditSchemaViolation struct {
	Index           int
	BlockID         string
	Field           string
	Reason          string
	AllowedFields   []string
	AllowedBlockIDs []string
	AllowedValues   []string
}

// validateAnswerDocumentPatchFieldEditsAgainstSchema closes the executor side
// of the dispatch-projected block-field protocol. Tool schemas are normally
// enforced by the provider, but malformed model calls must not widen the
// executable surface when a provider forwards them anyway. The check consumes
// only structured patch JSON plus the exact projected schema: no request,
// reasoning, visible answer text, or Mermaid carrier participates.
func validateAnswerDocumentPatchFieldEditsAgainstSchema(raw, schema json.RawMessage) *answerDocumentPatchFieldEditSchemaViolation {
	var envelope map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &envelope) != nil {
		return nil
	}
	editsRaw, present := envelope["block_field_edits_v1"]
	if !present {
		return nil
	}
	var edits []json.RawMessage
	if json.Unmarshal(editsRaw, &edits) != nil {
		return nil
	}

	type branch struct {
		blockIDs []string
		values   []string
	}
	branches := map[string]branch{}
	var allowedFields []string
	var root map[string]any
	if json.Unmarshal(schema, &root) == nil {
		properties, _ := root["properties"].(map[string]any)
		editsNode, _ := properties["block_field_edits_v1"].(map[string]any)
		items, _ := editsNode["items"].(map[string]any)
		rawBranches, _ := items["oneOf"].([]any)
		for _, rawBranch := range rawBranches {
			branchNode, _ := rawBranch.(map[string]any)
			branchProperties, _ := branchNode["properties"].(map[string]any)
			fieldNode, _ := branchProperties["field"].(map[string]any)
			field, _ := fieldNode["const"].(string)
			if field == "" {
				continue
			}
			if _, duplicate := branches[field]; !duplicate {
				allowedFields = append(allowedFields, field)
			}
			blockIDNode, _ := branchProperties["block_id"].(map[string]any)
			valueNode, _ := branchProperties["value"].(map[string]any)
			branches[field] = branch{
				blockIDs: schemaStringValues(blockIDNode["enum"]),
				values:   schemaStringValues(valueNode["enum"]),
			}
		}
	}
	for i, editRaw := range edits {
		var edit map[string]json.RawMessage
		if json.Unmarshal(editRaw, &edit) != nil {
			continue
		}
		var field, blockID string
		if json.Unmarshal(edit["field"], &field) != nil {
			continue
		}
		_ = json.Unmarshal(edit["block_id"], &blockID)
		allowed, ok := branches[field]
		if !ok {
			return &answerDocumentPatchFieldEditSchemaViolation{
				Index: i, BlockID: blockID, Field: field, Reason: "field_not_published",
				AllowedFields: append([]string(nil), allowedFields...),
			}
		}
		if len(allowed.blockIDs) > 0 && !stringInSlice(blockID, allowed.blockIDs) {
			return &answerDocumentPatchFieldEditSchemaViolation{
				Index: i, BlockID: blockID, Field: field, Reason: "block_id_not_published",
				AllowedFields:   append([]string(nil), allowedFields...),
				AllowedBlockIDs: append([]string(nil), allowed.blockIDs...),
				AllowedValues:   append([]string(nil), allowed.values...),
			}
		}
		var value string
		if json.Unmarshal(edit["value"], &value) != nil {
			return &answerDocumentPatchFieldEditSchemaViolation{
				Index: i, BlockID: blockID, Field: field, Reason: "value_must_be_string",
				AllowedFields:   append([]string(nil), allowedFields...),
				AllowedBlockIDs: append([]string(nil), allowed.blockIDs...),
				AllowedValues:   append([]string(nil), allowed.values...),
			}
		}
		if len(allowed.values) > 0 && !stringInSlice(value, allowed.values) {
			return &answerDocumentPatchFieldEditSchemaViolation{
				Index: i, BlockID: blockID, Field: field, Reason: "value_not_published",
				AllowedFields:   append([]string(nil), allowedFields...),
				AllowedBlockIDs: append([]string(nil), allowed.blockIDs...),
				AllowedValues:   append([]string(nil), allowed.values...),
			}
		}
	}
	return nil
}

func schemaStringValues(raw any) []string {
	values, _ := raw.([]any)
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func stringInSlice(value string, allowed []string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func answerDocumentPatchFieldEditSchemaRepair(v answerDocumentPatchFieldEditSchemaViolation) *types.ToolRepair {
	fields := []string{fmt.Sprintf("block_field_edits_v1[%d].field", v.Index)}
	if v.Reason == "value_must_be_string" || v.Reason == "value_not_published" {
		fields = append(fields, fmt.Sprintf("block_field_edits_v1[%d].value", v.Index))
	}
	metadata := map[string]string{
		"reason":           v.Reason,
		"field":            v.Field,
		"block_id":         v.BlockID,
		"available_fields": strings.Join(v.AllowedFields, ","),
	}
	if len(v.AllowedBlockIDs) > 0 {
		metadata["available_block_ids"] = strings.Join(v.AllowedBlockIDs, ",")
	}
	if len(v.AllowedValues) > 0 {
		metadata["available_values"] = strings.Join(v.AllowedValues, ",")
	}
	hint := "Use only one exact block_id/field/value branch published by the current block_field_edits_v1 schema, then resubmit the complete intended transaction."
	switch v.Reason {
	case "field_not_published":
		hint += " This field has no local-edit branch. Preserve the intended metadata and use a complete replace_blocks entry only when that operation and block id are present in the current schema; otherwise choose another exact published operation. Arrays or objects must not be placed in this string-value protocol; no entry is silently dropped and no content is chosen automatically."
	case "block_id_not_published":
		hint += " The selected field is valid, but this block id is not an executable target for it in the current dispatch. Do not substitute another block unless it is the block you intend to edit."
	case "value_must_be_string":
		hint += " The selected local field accepts one native JSON string enum value; array/object values belong to a different published operation or a complete block replacement."
	case "value_not_published":
		hint += " Copy one exact enum value from the current branch; do not invent, paraphrase, or coerce it."
	}
	return &types.ToolRepair{
		Code:     "answer_doc_patch_field_branch_not_published",
		Fields:   fields,
		Hint:     hint,
		Metadata: metadata,
	}
}

// emitAnswerDiagramEdgeEdit is a model-authored semantic delta over one
// existing diagram edge. The tool renders the declared operation into the
// previous model-authored Mermaid carrier; it never chooses an endpoint,
// relation kind, or reader-facing label.
type emitAnswerDiagramEdgeEdit struct {
	BlockID              string                   `json:"block_id"`
	Action               string                   `json:"action"`
	FailureRef           string                   `json:"failure_ref,omitempty"`
	AdditionRef          string                   `json:"addition_ref,omitempty"`
	Occurrence           int                      `json:"occurrence,omitempty"`
	BodyOccurrence       int                      `json:"body_occurrence,omitempty"`
	Match                *types.DiagramEdgeAnchor `json:"match,omitempty"`
	Edge                 *types.DiagramEdgeAnchor `json:"edge,omitempty"`
	VisibleLabel         string                   `json:"visible_label,omitempty"`
	FromNodeVisibleLabel string                   `json:"from_node_visible_label,omitempty"`
	ToNodeVisibleLabel   string                   `json:"to_node_visible_label,omitempty"`
	failureRefResolved   bool
	failureRefCarrier    types.AnswerDiagramRelationRepairTargetCarrier
	failureIssue         string
	attachPairResolving  bool
	additionCandidate    *types.AnswerDiagramRelationRepairCandidate
}

type emitAnswerDiagramBoundaryReplacement struct {
	BlockID               string                             `json:"block_id"`
	ParticipantBoundaries []types.DiagramParticipantBoundary `json:"participant_boundaries"`
}

type emitAnswerDiagramBoundaryEdit struct {
	BoundaryRef string `json:"boundary_ref"`
	Action      string `json:"action"`
}

type emitAnswerDiagramRelationScopeEdit struct {
	BlockID string `json:"block_id"`
	Action  string `json:"action"`
}

type emitAnswerDiagramParticipantEdit struct {
	BlockID        string `json:"block_id"`
	ParticipantID  string `json:"participant_id"`
	ParticipantRef string `json:"participant_ref"`
	NodeID         string `json:"node_id"`
	Action         string `json:"action"`
	VisibleLabel   string `json:"visible_label,omitempty"`
}

// localDiagramLeaseWholeBlockMutationViolation guards the execution path as
// well as the projected schema. Tool callers can bypass or lag the current
// schema, but they still cannot widen a typed local diagram lease into roster
// mutation outside the projected capabilities: unrelated exact replacement or
// removal is preserved, additions are unavailable, and target removal is
// limited to an exact optional target when explicitly granted. Atomic compiler-generated
// ReplaceBlocks are not
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
		id := strings.TrimSpace(block.ID)
		return &types.AnswerDiagramRelationRepairScopeViolation{BlockID: id, Issue: "whole_add_not_authorized"}
	}
	for _, edit := range p.BlockFieldEditsV1 {
		if id := strings.TrimSpace(edit.BlockID); targetSet[id] {
			return &types.AnswerDiagramRelationRepairScopeViolation{BlockID: id, Issue: "block_field_edit_not_authorized"}
		}
	}
	for _, rawID := range p.RemoveBlockIDs {
		id := strings.TrimSpace(rawID)
		if !targetSet[id] {
			continue
		}
		if targetSet[id] && lease != nil && lease.AllowTargetDiagramRemoval {
			continue
		}
		return &types.AnswerDiagramRelationRepairScopeViolation{BlockID: id, Issue: "whole_remove_not_authorized"}
	}
	return nil
}

type splitCompanionDispositionFailure struct {
	Kind             types.AnswerBlockCompanionLineageKind
	RemovedBlockID   string
	CompanionBlockID string
}

// splitCompanionDispositionViolation requires an explicit model choice for
// both halves of a system-created fused-block split whenever one half is
// removed. The exact typed lineage is the only trigger: block titles, prose,
// Mermaid bodies/messages, and id suffix guesses are never inspected.
func splitCompanionDispositionViolation(prev *types.AnswerDocumentV2, p *emitAnswerDocumentPatchParams) *splitCompanionDispositionFailure {
	if prev == nil || p == nil || len(prev.BlockCompanionLineages) == 0 || len(p.RemoveBlockIDs) == 0 {
		return nil
	}
	removed := make(map[string]bool, len(p.RemoveBlockIDs))
	explicit := make(map[string]bool, len(p.RemoveBlockIDs)+len(p.UnchangedBlockIDs)+len(p.ReplaceBlocks))
	for _, rawID := range p.RemoveBlockIDs {
		if id := strings.TrimSpace(rawID); id != "" {
			removed[id] = true
			explicit[id] = true
		}
	}
	for _, rawID := range p.UnchangedBlockIDs {
		if id := strings.TrimSpace(rawID); id != "" {
			explicit[id] = true
		}
	}
	for _, block := range p.ReplaceBlocks {
		if id := strings.TrimSpace(block.ID); id != "" {
			explicit[id] = true
		}
	}
	for _, edit := range p.BlockFieldEditsV1 {
		if id := strings.TrimSpace(edit.BlockID); id != "" {
			explicit[id] = true
		}
	}
	// Atomic operations are explicit retain/edit decisions for their exact
	// carrier. Failure-ref-only operations can still name the sibling in
	// unchanged_block_ids, which is deliberately accepted as redundant.
	for _, edit := range p.DiagramEdgeEdits {
		if id := strings.TrimSpace(edit.BlockID); id != "" {
			explicit[id] = true
		}
	}
	for _, edit := range p.DiagramBoundaryReplacements {
		if id := strings.TrimSpace(edit.BlockID); id != "" {
			explicit[id] = true
		}
	}
	for _, edit := range p.DiagramParticipantEdits {
		if id := strings.TrimSpace(edit.BlockID); id != "" {
			explicit[id] = true
		}
	}
	for _, lineage := range types.NormalizeAnswerBlockCompanionLineages(prev.BlockCompanionLineages) {
		pairs := [][2]string{
			{lineage.VisibleBlockID, lineage.DiagramBlockID},
			{lineage.DiagramBlockID, lineage.VisibleBlockID},
		}
		for _, pair := range pairs {
			if removed[pair[0]] && !explicit[pair[1]] {
				return &splitCompanionDispositionFailure{
					Kind:             lineage.Kind,
					RemovedBlockID:   pair[0],
					CompanionBlockID: pair[1],
				}
			}
		}
	}
	return nil
}

func splitCompanionDispositionRepair(failure splitCompanionDispositionFailure) *types.ToolRepair {
	metadata := map[string]string{
		"lineage_kind":       string(failure.Kind),
		"removed_block_id":   failure.RemovedBlockID,
		"companion_block_id": failure.CompanionBlockID,
	}
	return &types.ToolRepair{
		Code:     "answer_doc_split_companion_disposition_required",
		Fields:   []string{"remove_block_ids", "unchanged_block_ids", "replace_blocks"},
		Metadata: metadata,
		Hint: fmt.Sprintf(
			"Blocks %q and %q are the two model-visible halves of one earlier lossless fused prose/diagram split. You chose to remove %q. In the same patch, explicitly choose the sibling's disposition: add %q to unchanged_block_ids to retain it byte-identical, replace it with a complete model-authored block, or add it to remove_block_ids. The system will not cascade-delete, rewrite its title/text, or choose the disposition for you.",
			failure.RemovedBlockID, failure.CompanionBlockID, failure.RemovedBlockID, failure.CompanionBlockID),
	}
}

// Execute applies the patch to the previous V2 emit. Failure paths
// surface as Success=false ToolResult so the LLM sees the error
// and can retry with corrected params (the patch validator's
// reject messages name the offending id / op verbatim).
func (t *EmitAnswerDocumentPatch) Execute(ctx *types.BusContext, params json.RawMessage) (result types.ToolResult, err error) {
	now := time.Now()
	stagedByThisCall := false
	defer func() {
		result = annotateAnswerDocumentPatchFailureOutcome(result, stagedByThisCall)
	}()
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
	if repaired, blockIDs, violation := normalizeMisroutedPatchBlockOperations(params, prev); violation != "" {
		return failEmitWithRepair(t.Name(), now, &types.ToolRepair{
			Code:   "answer_doc_patch_block_operation_misrouted",
			Fields: []string{"replace_snippets", "replace_blocks"},
			Hint:   "`replace_snippets` accepts only code-snippet objects {file,start_line,end_line,language?,code}. To edit an existing answer block or its items/diagram/typed annotations, use `replace_blocks` with that existing block id and the complete block payload. Do not mix snippet and block shapes or submit both replacement fields.",
		}, "answer_document patch placed a block operation in replace_snippets, but it could not be remapped losslessly: %s", violation)
	} else if len(blockIDs) > 0 {
		logging.Warning("[emit_answer_document_patch] losslessly remapped block operation(s) from replace_snippets to replace_blocks: %s",
			strings.Join(blockIDs, ", "))
		params = repaired
	}
	if repaired, paths, ok := quarantineUnknownAnswerDocumentFields(params, answerDocumentPatchQuarantineProfile); ok {
		logging.Warning("[emit_answer_document_patch] quarantined schema-unknown answer-document patch field(s) without retry: %s",
			strings.Join(paths, ", "))
		params = repaired
	}
	fieldEditSchema := BuildAnswerDocumentPatchParametersFor(types.BuildAnswerSemanticViewForBusContext(ctx))
	leaseForFieldProjection := ctx.Mutable.AnswerDiagramRelationRepairLease()
	var excludedFieldEditTargets []string
	if types.AnswerDiagramRelationRepairLeaseIsLocallyExecutable(leaseForFieldProjection) {
		excludedFieldEditTargets = localDiagramLeaseTargetBlockIDs(leaseForFieldProjection)
	}
	fieldEditSchema = projectAnswerDocumentPatchFieldEditTargets(fieldEditSchema, prev, excludedFieldEditTargets)
	if violation := validateAnswerDocumentPatchFieldEditsAgainstSchema(params, fieldEditSchema); violation != nil {
		return failEmitWithRepair(t.Name(), now, answerDocumentPatchFieldEditSchemaRepair(*violation),
			"block_field_edits_v1[%d] does not match any exact field-edit branch published for this dispatch: reason=%s field=%q block_id=%q",
			violation.Index, violation.Reason, violation.Field, violation.BlockID)
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
	if len(p.BlockFieldEditsV1) > maxModelAuthoredBlockFieldEditsV1 {
		return failEmit(t.Name(), now, "too many block_field_edits_v1: got %d, max %d",
			len(p.BlockFieldEditsV1), maxModelAuthoredBlockFieldEditsV1)
	}
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	if !types.AnswerDiagramRelationRepairLeaseIsLocallyExecutable(lease) {
		lease = nil
	}
	if violation := localDiagramLeaseWholeBlockMutationViolation(&p, lease); violation != nil {
		return failEmitWithRepair(t.Name(), now, answerDiagramRelationRepairScopeRepair(lease, []types.AnswerDiagramRelationRepairScopeViolation{*violation}),
			"local diagram repair lease requires atomic diagram operations for block=%q; whole-block operation=%s is not authorized",
			violation.BlockID, violation.Issue)
	}
	if violation := splitCompanionDispositionViolation(prev, &p); violation != nil {
		return failEmitWithRepair(t.Name(), now, splitCompanionDispositionRepair(*violation),
			"patch removes split companion block %q but does not explicitly dispose sibling %q",
			violation.RemovedBlockID, violation.CompanionBlockID)
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
	var addedCompanionLineages []types.AnswerBlockCompanionLineage
	p.ReplaceBlocks, p.AddBlocks, addedCompanionLineages = splitFusedDiagramPatchBlocksWithLineage(t.Name(),
		fusedPatchSplitBudget(prev, p.RemoveBlockIDs, p.ReplaceBlocks, p.AddBlocks),
		prev, p.RemoveBlockIDs, p.UnchangedBlockIDs,
		p.ReplaceBlocks, p.AddBlocks)

	// Build typed AnswerDocumentV2Patch from the decoded params.
	patch := &types.AnswerDocumentV2Patch{
		UnchangedBlockIDs:            append([]string(nil), p.UnchangedBlockIDs...),
		RemoveBlockIDs:               append([]string(nil), p.RemoveBlockIDs...),
		BlockFieldEditsV1:            append([]types.AnswerBlockFieldEditV1(nil), p.BlockFieldEditsV1...),
		ReplaceCitations:             convertEmitCitationsToTyped(p.ReplaceCitations),
		AppendCitations:              convertEmitCitationsToTyped(p.AppendCitations),
		ReplaceExactResolution:       p.ReplaceExactResolution,
		ReplaceMissingRequestedRoles: p.ReplaceMissingRequestedRoles,
		ReplaceCaveats:               p.ReplaceCaveats,
		ReplaceSnippets:              convertEmitCodeSnippetsToTyped(p.ReplaceSnippets),
		AddBlockCompanionLineages:    addedCompanionLineages,
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
	if changed, fields := normalizeRedundantPatchBlockFieldEditsV1(params, patch); changed {
		logging.Warning("[emit_answer_document_patch] absorbed exact block-field assignment(s) already carried by full replacement: %s",
			strings.Join(fields, ", "))
	}
	if len(p.DiagramEdgeEdits) > 0 || len(p.DiagramBoundaryReplacements) > 0 || len(p.DiagramBoundaryEdits) > 0 ||
		len(p.DiagramRelationScopeEdits) > 0 || len(p.DiagramParticipantEdits) > 0 {
		view := types.BuildAnswerSemanticViewForBusContext(ctx)
		stagePrecedence := diagramVerifiedReadModeStageEdgeAuthority(ctx, view)
		protectedParticipants := make([]string, 0)
		if view != nil {
			for _, obligation := range view.DiagramParticipantObligations {
				if identity := strings.TrimSpace(obligation.Identity); identity != "" {
					protectedParticipants = append(protectedParticipants, identity)
				}
			}
		}
		if err := applyModelAuthoredDiagramAtomicEditsWithParticipantsAndBoundaries(
			prev, patch, p.DiagramEdgeEdits, p.DiagramBoundaryReplacements,
			p.DiagramBoundaryEdits, p.DiagramParticipantEdits, protectedParticipants, lease, stagePrecedence,
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
				repair.Fields = []string{"diagram_edge_edits", "diagram_boundary_replacements", "diagram_boundary_edits", "diagram_relation_scope_edits", "diagram_participant_edits"}
				if rosterJSON, progressSignature := atomicDiagramParticipantDispositionRosterMetadata(err); rosterJSON != "" {
					repair.Metadata[types.ToolRepairMetaDiagramParticipantDispositionRosterJSON] = rosterJSON
					repair.Metadata[types.ToolRepairMetaDiagramRelationProgressSignature] = progressSignature
				}
				repair.Hint = "The submitted atomic diagram operation is not executable under the current relation-repair lease. The whole rejected patch transaction was rolled back: none of its edge, boundary, participant, block, or citation operations were committed. Re-read the complete current typed delta and resubmit every operation you still choose together in one new atomic patch; do not assume a valid sibling operation from the rejected call already applied, and do not guess, silently drop, or widen operations."
				return failEmitWithRepair(t.Name(), now, repair, "diagram atomic edits: %s", err.Error())
			}
			if repair := answerDiagramRelationRepairLeaseAbsentRepair(p.DiagramEdgeEdits); repair != nil {
				return failEmitWithRepair(t.Name(), now, repair, "diagram atomic edits: %s", err.Error())
			}
			if repair := answerDiagramBoundaryRepairLeaseAbsentRepair(p.DiagramBoundaryEdits); repair != nil {
				return failEmitWithRepair(t.Name(), now, repair, "diagram atomic edits: %s", err.Error())
			}
			return failEmit(t.Name(), now, "diagram atomic edits: %s", err.Error())
		}
		if err := applyModelAuthoredDiagramRelationScopeEdits(prev, patch, p.DiagramRelationScopeEdits, ctx); err != nil {
			return failEmitWithRepair(t.Name(), now, &types.ToolRepair{
				Code:   "answer_doc_relation_scope_edit_invalid",
				Fields: []string{"diagram_relation_scope_edits"},
				Hint:   "Use only one exact block_id/action branch currently published in diagram_relation_scope_edits. The branch changes only requested_relation_scope; do not replace the lease-target diagram or replay older refs.",
			}, "diagram relation-scope edits: %s", err.Error())
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
					stagedByThisCall = true
					return failEmitWithRepair(t.Name(), now, emitFixHintsRepair(hardHints),
						"%s", formatEmitFixHintsWithRetryCompanions(hardHints, advisoryHints))
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

// normalizeRedundantPatchBlockFieldEditsV1 absorbs only an exact assignment
// already made explicitly by a full replacement of the same block. It is a
// compatibility normalization, not merge semantics: a missing replacement
// field, a different value, an invalid value, or any non-whitelisted field is
// left intact so ApplyAnswerDocumentV2Patch rejects the cross-op conflict.
// No prose or nested carrier is inspected and no value is inferred.
func normalizeRedundantPatchBlockFieldEditsV1(raw json.RawMessage, patch *types.AnswerDocumentV2Patch) (bool, []string) {
	if patch == nil || len(patch.BlockFieldEditsV1) == 0 || len(patch.ReplaceBlocks) == 0 {
		return false, nil
	}
	explicitFields := explicitPatchReplacementFields(raw)
	replacements := make(map[string]types.AnswerBlock, len(patch.ReplaceBlocks))
	duplicates := map[string]bool{}
	for _, block := range patch.ReplaceBlocks {
		id := strings.TrimSpace(block.ID)
		if id == "" {
			continue
		}
		if _, exists := replacements[id]; exists {
			duplicates[id] = true
			continue
		}
		replacements[id] = block
	}
	kept := make([]types.AnswerBlockFieldEditV1, 0, len(patch.BlockFieldEditsV1))
	var fields []string
	for _, edit := range patch.BlockFieldEditsV1 {
		id := strings.TrimSpace(edit.BlockID)
		replacement, exists := replacements[id]
		if !exists || duplicates[id] || !explicitFields[id][edit.Field] ||
			!patchBlockFieldEditV1EqualsExplicitReplacement(edit, replacement) {
			kept = append(kept, edit)
			continue
		}
		fields = append(fields, fmt.Sprintf("block_field_edits_v1[%q].%s", id, edit.Field))
	}
	if len(fields) == 0 {
		return false, nil
	}
	patch.BlockFieldEditsV1 = kept
	return true, fields
}

// explicitPatchReplacementFields preserves the distinction between a field
// authored on this replacement and one inherited by retry compatibility.
// Duplicate replacement ids intentionally publish no explicit-field grant.
func explicitPatchReplacementFields(raw json.RawMessage) map[string]map[types.AnswerBlockEditableFieldV1]bool {
	result := make(map[string]map[types.AnswerBlockEditableFieldV1]bool)
	if len(raw) == 0 {
		return result
	}
	var envelope struct {
		ReplaceBlocks []map[string]json.RawMessage `json:"replace_blocks"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return result
	}
	duplicates := make(map[string]bool)
	for _, candidate := range envelope.ReplaceBlocks {
		var id string
		if idRaw, ok := candidate["id"]; ok {
			_ = json.Unmarshal(idRaw, &id)
		}
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := result[id]; exists {
			duplicates[id] = true
			continue
		}
		fields := make(map[types.AnswerBlockEditableFieldV1]bool, 5)
		for _, field := range []types.AnswerBlockEditableFieldV1{
			types.AnswerBlockFieldTraceCausalClaimCaliber,
			types.AnswerBlockFieldCurrentStatusVerdict,
			types.AnswerBlockFieldErrorGranularityVerdict,
			types.AnswerBlockFieldScopeDisclosure,
			types.AnswerBlockFieldSurfaceRole,
		} {
			_, fields[field] = candidate[string(field)]
		}
		result[id] = fields
	}
	for id := range duplicates {
		delete(result, id)
	}
	return result
}

func patchBlockFieldEditV1EqualsExplicitReplacement(edit types.AnswerBlockFieldEditV1, block types.AnswerBlock) bool {
	value := strings.TrimSpace(edit.Value)
	switch edit.Field {
	case types.AnswerBlockFieldTraceCausalClaimCaliber:
		v, ok := types.NormalizeTraceCausalClaimCaliber(value)
		return ok && block.Kind == types.BlockSummary && block.TraceCausalClaimCaliber != "" && block.TraceCausalClaimCaliber == v
	case types.AnswerBlockFieldCurrentStatusVerdict:
		v, ok := types.NormalizeCurrentStatusVerdict(value)
		return ok && block.Kind == types.BlockDecision && block.CurrentStatusVerdict != "" && block.CurrentStatusVerdict == v
	case types.AnswerBlockFieldErrorGranularityVerdict:
		v, ok := types.NormalizeErrorGranularityVerdict(value)
		return ok && block.Kind == types.BlockDecision && block.ErrorGranularityVerdict != "" && block.ErrorGranularityVerdict == v
	case types.AnswerBlockFieldScopeDisclosure:
		v, ok := types.NormalizeScopeDisclosureKind(value)
		return ok && block.ScopeDisclosure != "" && block.ScopeDisclosure == v
	case types.AnswerBlockFieldSurfaceRole:
		v, ok := types.NormalizeSurfaceRole(value)
		return ok && block.SurfaceRole != "" && block.SurfaceRole == v
	default:
		return false
	}
}

// annotateAnswerDocumentPatchFailureOutcome exposes the executor's exact
// transaction phase on every rejected patch call. A structural/apply failure
// leaves the live retry base unchanged and requires the complete intended
// patch to be resubmitted. A merged-document hard failure stores the exact
// merged draft as the new retry base and requires only new corrections. This
// is retry guidance from a precise boolean; it never reads request/model prose,
// never changes the accepted answer, and never selects diagram content.
func annotateAnswerDocumentPatchFailureOutcome(result types.ToolResult, stagedByThisCall bool) types.ToolResult {
	if result.Success || strings.TrimSpace(result.ToolName) != (&EmitAnswerDocumentPatch{}).Name() {
		return result
	}
	repair := &types.ToolRepair{}
	if result.Repair != nil {
		copyRepair := *result.Repair
		copyRepair.Fields = append([]string(nil), result.Repair.Fields...)
		copyRepair.Targets = append([]types.ToolRepairTarget(nil), result.Repair.Targets...)
		copyRepair.Metadata = make(map[string]string, len(result.Repair.Metadata)+1)
		for key, value := range result.Repair.Metadata {
			copyRepair.Metadata[key] = value
		}
		repair = &copyRepair
	} else {
		repair.Metadata = make(map[string]string, 1)
	}
	if repair.Metadata == nil {
		repair.Metadata = make(map[string]string, 1)
	}

	outcome := types.AnswerDocumentPatchOutcomeNotStaged
	prefix := "Patch transaction state: this call was not staged; the live retry base is unchanged. Resubmit the complete intended patch. "
	if stagedByThisCall {
		outcome = types.AnswerDocumentPatchOutcomeStagedForRetry
		prefix = "Patch transaction state: this call's exact merged draft is the live retry base. Submit only new corrections. "
	}
	repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] = outcome
	result.Repair = repair
	result.Summary = prefix + result.Summary
	return result
}

// answerDiagramRelationRepairLeaseAbsentRepair classifies only structured
// ref-bearing operations when the caller has already established lease=nil.
// It never parses the executor error, request, Mermaid text, or model prose,
// and it cannot mint a replacement ref. The retrying model can preserve the
// staged patch base for one ordinary revalidation cycle, which will publish a
// fresh typed lease only if relation failures still exist.
func answerDiagramRelationRepairLeaseAbsentRepair(edits []emitAnswerDiagramEdgeEdit) *types.ToolRepair {
	fields := make([]string, 0, len(edits))
	for i, edit := range edits {
		if strings.TrimSpace(edit.FailureRef) != "" {
			fields = append(fields, fmt.Sprintf("diagram_edge_edits[%d].failure_ref", i))
		}
		if strings.TrimSpace(edit.AdditionRef) != "" {
			fields = append(fields, fmt.Sprintf("diagram_edge_edits[%d].addition_ref", i))
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return &types.ToolRepair{
		Code:   types.ToolRepairCodeAnswerDocRelationRepairLeaseAbsent,
		Fields: fields,
		Hint:   "No relation-repair lease is active for the current patch base. Every historical failure_ref/addition_ref is invalid; do not guess or reuse one.",
		Metadata: map[string]string{
			types.ToolRepairMetaDiagramRelationRepairLeaseStatus: "absent",
		},
	}
}

func answerDiagramBoundaryRepairLeaseAbsentRepair(edits []emitAnswerDiagramBoundaryEdit) *types.ToolRepair {
	fields := make([]string, 0, len(edits))
	for i, edit := range edits {
		if strings.TrimSpace(edit.BoundaryRef) != "" {
			fields = append(fields, fmt.Sprintf("diagram_boundary_edits[%d].boundary_ref", i))
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return &types.ToolRepair{
		Code:   types.ToolRepairCodeAnswerDocRelationRepairLeaseAbsent,
		Fields: fields,
		Hint:   "No participant-boundary repair lease is active for the current patch base. Every historical boundary_ref is invalid; do not guess or reuse one.",
		Metadata: map[string]string{
			types.ToolRepairMetaDiagramRelationRepairLeaseStatus: "absent",
		},
	}
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
	if !types.AnswerDiagramRelationRepairLeaseIsLocallyExecutable(lease) {
		mut.SetAnswerDiagramRelationRepairLease(nil)
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
			Version                int                                          `json:"version"`
			Failures               []types.AnswerDiagramRelationRepairFailure   `json:"failures"`
			PreserveUnlistedEdges  bool                                         `json:"preserve_unlisted_edges"`
			AllowedAdditions       []types.AnswerDiagramRelationRepairCandidate `json:"allowed_additions,omitempty"`
			OptionalOrphanCleanups []types.AnswerDiagramOrphanCleanupCandidate  `json:"optional_orphan_cleanups,omitempty"`
		}{
			Version: 1, Failures: append([]types.AnswerDiagramRelationRepairFailure(nil), lease.Failures...),
			PreserveUnlistedEdges: true,
			AllowedAdditions:      append([]types.AnswerDiagramRelationRepairCandidate(nil), lease.AllowedAdditions...),
			OptionalOrphanCleanups: append(
				[]types.AnswerDiagramOrphanCleanupCandidate(nil), lease.OptionalOrphanCleanups...,
			),
		}
		if raw, err := json.Marshal(delta); err == nil {
			metadata[types.ToolRepairMetaDiagramRelationRepairDeltaJSON] = string(raw)
		}
		if len(lease.ParticipantBoundaryFailures) > 0 || len(lease.ParticipantVisibilityFailures) > 0 {
			participantDelta := struct {
				Version    int `json:"version"`
				Mismatches []struct {
					BlockID                   string                                                 `json:"block_id"`
					Participant               string                                                 `json:"participant"`
					Issue                     string                                                 `json:"issue"`
					BoundaryRef               string                                                 `json:"boundary_ref,omitempty"`
					AllowedBoundaryActions    []types.AnswerDiagramParticipantBoundaryRepairAction   `json:"allowed_boundary_actions,omitempty"`
					ParticipantRef            string                                                 `json:"participant_ref,omitempty"`
					AllowedParticipantActions []types.AnswerDiagramParticipantVisibilityRepairAction `json:"allowed_participant_actions,omitempty"`
				} `json:"mismatches"`
			}{
				Version: 1,
			}
			for _, failure := range lease.ParticipantBoundaryFailures {
				participantDelta.Mismatches = append(participantDelta.Mismatches, struct {
					BlockID                   string                                                 `json:"block_id"`
					Participant               string                                                 `json:"participant"`
					Issue                     string                                                 `json:"issue"`
					BoundaryRef               string                                                 `json:"boundary_ref,omitempty"`
					AllowedBoundaryActions    []types.AnswerDiagramParticipantBoundaryRepairAction   `json:"allowed_boundary_actions,omitempty"`
					ParticipantRef            string                                                 `json:"participant_ref,omitempty"`
					AllowedParticipantActions []types.AnswerDiagramParticipantVisibilityRepairAction `json:"allowed_participant_actions,omitempty"`
				}{BlockID: failure.BlockID, Participant: failure.Participant, Issue: failure.Issue,
					BoundaryRef: failure.BoundaryRef, AllowedBoundaryActions: failure.AllowedBoundaryActions})
			}
			for _, failure := range lease.ParticipantVisibilityFailures {
				participantDelta.Mismatches = append(participantDelta.Mismatches, struct {
					BlockID                   string                                                 `json:"block_id"`
					Participant               string                                                 `json:"participant"`
					Issue                     string                                                 `json:"issue"`
					BoundaryRef               string                                                 `json:"boundary_ref,omitempty"`
					AllowedBoundaryActions    []types.AnswerDiagramParticipantBoundaryRepairAction   `json:"allowed_boundary_actions,omitempty"`
					ParticipantRef            string                                                 `json:"participant_ref,omitempty"`
					AllowedParticipantActions []types.AnswerDiagramParticipantVisibilityRepairAction `json:"allowed_participant_actions,omitempty"`
				}{BlockID: failure.BlockID, Participant: failure.Participant, Issue: failure.Issue,
					ParticipantRef: failure.ParticipantRef, AllowedParticipantActions: failure.AllowedParticipantActions})
			}
			if raw, err := json.Marshal(participantDelta); err == nil {
				metadata[types.ToolRepairMetaDiagramParticipantRepairDeltaJSON] = string(raw)
			}
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

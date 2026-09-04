package tool

import (
	"errors"
	"strconv"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_patch_structure_repair.go — V2-4 (§40.51): the repair for a
// patch-structure reject is keyed on the TYPED violation kind carried by
// types.AnswerDocumentPatchStructureError, never on the message prose.
// EVOLUTION RECORD: answerDocumentMutationRepair used to switch on
// strings.Contains(err.Error(), …) for three messages and returned nil for the
// other ~27, so a wording change silently dropped a repair route and most
// structural rejects reached the model with no typed route at all (the §40.50
// "识别器按字面量键控" class, retired here).

// answerDocumentPatchStructureRepairRow is one row of the single-source table:
// Code is the typed route, Fields the schema fields the model must revisit,
// Hint the executable teaching. Every AnswerDocumentPatchViolationKind has
// exactly one row (pinned by TestAnswerDocumentPatchStructureRepairTableCoversEveryKind).
type answerDocumentPatchStructureRepairRow struct {
	kind   types.AnswerDocumentPatchViolationKind
	code   string
	fields []string
	hint   string
}

// answerDocumentPatchStructureRepairTable lists the rows in PRECEDENCE order:
// when one reject carries several kinds, the first row present decides the
// code and leads the hint; Fields are the union over every present row. The
// three historical codes lead so their consumers keep routing on the same
// spelling.
var answerDocumentPatchStructureRepairTable = []answerDocumentPatchStructureRepairRow{
	{
		kind:   types.AnswerDocumentPatchViolationCitationModeConflict,
		code:   types.ToolRepairCodeAnswerDocPatchCitationModeConflict,
		fields: []string{"replace_citations", "append_citations"},
		hint:   "Re-emit `emit_answer_document_patch` with exactly one citation-pool operation: use `append_citations` when only adding citations, or `replace_citations` only when every citation_ref-bearing block is also replaced/removed. If many citations change, switch to a full `emit_answer_document` payload.",
	},
	{
		kind:   types.AnswerDocumentPatchViolationExistingBlock,
		code:   types.ToolRepairCodeAnswerDocPatchExistingBlock,
		fields: []string{"add_blocks", "replace_blocks", "unchanged_block_ids"},
		hint:   "Move any block id that already exists in the previous emit out of `add_blocks`. Put the edited block in `replace_blocks`, or list it in `unchanged_block_ids` when it should stay byte-identical.",
	},
	{
		kind:   types.AnswerDocumentPatchViolationPreservedCitationBlock,
		code:   types.ToolRepairCodeAnswerDocPatchReplaceCitationsPreservedBlocks,
		fields: []string{"replace_citations", "append_citations", "replace_blocks", "remove_block_ids"},
		hint:   "Do not replace the citation pool while preserving old blocks that still contain citation_ref values. Use `append_citations`, replace/remove every citation-bearing block, or switch to a full `emit_answer_document` payload with a complete zero-based citation pool.",
	},
	{
		kind:   types.AnswerDocumentPatchViolationUnknownID,
		code:   types.ToolRepairCodeAnswerDocPatchStructure,
		fields: []string{"unchanged_block_ids", "replace_blocks", "block_field_edits_v1", "block_receipt_edits_v1", "model_block_order"},
		hint:   "Every id in `unchanged_block_ids`, `replace_blocks`, `block_field_edits_v1`, `block_receipt_edits_v1` and `model_block_order` must be an exact block id of the current retry base; use `add_blocks` for a block that does not exist yet, and copy ids verbatim from the published block roster.",
	},
	{
		kind:   types.AnswerDocumentPatchViolationEmptyID,
		code:   types.ToolRepairCodeAnswerDocPatchStructure,
		fields: []string{"unchanged_block_ids", "remove_block_ids", "replace_blocks", "add_blocks", "block_field_edits_v1", "block_receipt_edits_v1", "model_block_order"},
		hint:   "Every block id must be non-empty and copied exactly, without surrounding whitespace.",
	},
	{
		kind:   types.AnswerDocumentPatchViolationDuplicate,
		code:   types.ToolRepairCodeAnswerDocPatchStructure,
		fields: []string{"remove_block_ids", "replace_blocks", "add_blocks", "block_field_edits_v1", "block_receipt_edits_v1", "model_block_order"},
		hint:   "List each block id (and each block_id/field pair) at most once inside one operation.",
	},
	{
		kind:   types.AnswerDocumentPatchViolationCrossOpConflict,
		code:   types.ToolRepairCodeAnswerDocPatchStructure,
		fields: []string{"replace_blocks", "remove_block_ids", "add_blocks", "block_field_edits_v1", "block_receipt_edits_v1"},
		hint:   "Target each block id with exactly one operation: replace it, remove it, edit one of its published fields, or add it — never two of these for the same id.",
	},
	{
		kind:   types.AnswerDocumentPatchViolationSystemBlock,
		code:   types.ToolRepairCodeAnswerDocPatchStructure,
		fields: []string{"block_field_edits_v1", "block_receipt_edits_v1"},
		hint:   "Local field/receipt edits apply only to model-authored blocks; system-generated supplements are not editable targets.",
	},
	{
		kind:   types.AnswerDocumentPatchViolationInvalidKind,
		code:   types.ToolRepairCodeAnswerDocPatchStructure,
		fields: []string{"replace_blocks", "add_blocks"},
		hint:   "Every replace/add block needs a `kind` from the canonical block-kind enum published in the schema.",
	},
	{
		kind:   types.AnswerDocumentPatchViolationInvalidField,
		code:   types.ToolRepairCodeAnswerDocPatchStructure,
		fields: []string{"block_field_edits_v1", "block_receipt_edits_v1"},
		hint:   "Choose only a `field` branch the current schema publishes for local edits; any other block field needs a complete `replace_blocks` entry.",
	},
	{
		kind:   types.AnswerDocumentPatchViolationFieldKindMismatch,
		code:   types.ToolRepairCodeAnswerDocPatchStructure,
		fields: []string{"block_field_edits_v1"},
		hint:   "The selected field exists only on blocks of the kind the schema names for it; edit a block of that kind or choose another published branch.",
	},
	{
		kind:   types.AnswerDocumentPatchViolationInvalidValue,
		code:   types.ToolRepairCodeAnswerDocPatchStructure,
		fields: []string{"block_field_edits_v1", "block_receipt_edits_v1"},
		hint:   "Copy one exact value / receipt pair from the current schema branch; do not invent, paraphrase, or coerce it.",
	},
	{
		kind:   types.AnswerDocumentPatchViolationRosterMismatch,
		code:   types.ToolRepairCodeAnswerDocPatchStructure,
		fields: []string{"model_block_order"},
		hint:   "`model_block_order` must list every model-authored block id of the current retry base exactly once and nothing else; system-generated blocks keep their slots.",
	},
	{
		kind:   types.AnswerDocumentPatchViolationRosterChangeCombined,
		code:   types.ToolRepairCodeAnswerDocPatchStructure,
		fields: []string{"model_block_order", "add_blocks", "remove_block_ids"},
		hint:   "Do not combine `model_block_order` with `add_blocks` or `remove_block_ids`: settle the block roster in one patch and submit the complete permutation in another.",
	},
}

const answerDocumentPatchStructureListSentence = " The summary lists every structural violation of this patch; fix ALL of them in one resubmission."

// answerDocumentPatchStructureRepair builds the typed repair for a
// structure reject from its violation kinds (precedence table above).
func answerDocumentPatchStructureRepair(structure *types.AnswerDocumentPatchStructureError) *types.ToolRepair {
	if structure == nil || len(structure.Violations) == 0 {
		return nil
	}
	present := make(map[types.AnswerDocumentPatchViolationKind]bool, len(structure.Violations))
	for _, v := range structure.Violations {
		present[v.Kind] = true
	}
	var lead *answerDocumentPatchStructureRepairRow
	var fields []string
	seenField := map[string]bool{}
	for i := range answerDocumentPatchStructureRepairTable {
		row := &answerDocumentPatchStructureRepairTable[i]
		if !present[row.kind] {
			continue
		}
		if lead == nil {
			lead = row
		}
		for _, field := range row.fields {
			if seenField[field] {
				continue
			}
			seenField[field] = true
			fields = append(fields, field)
		}
	}
	if lead == nil {
		// Closed set: every kind has a row (pinned); an unknown kind is a
		// programming error, and the generic route still guides.
		lead = &answerDocumentPatchStructureRepairRow{code: types.ToolRepairCodeAnswerDocPatchStructure, hint: "Fix every structural violation named in the summary and resubmit the complete intended patch."}
	}
	hint := lead.hint
	if len(structure.Violations) > 1 {
		hint += answerDocumentPatchStructureListSentence
	}
	return &types.ToolRepair{
		Code:   lead.code,
		Fields: fields,
		Hint:   hint,
		Metadata: map[string]string{
			types.ToolRepairMetaAnswerDocPatchViolationCount: strconv.Itoa(len(structure.Violations)),
		},
	}
}

// answerDocumentMutationRepair maps a mutation apply rejection to its typed
// repair. Only the typed patch-structure carrier yields a repair; every
// other apply error (invalid mutation shape, internal post-validation
// invariants) stays unstructured.
func answerDocumentMutationRepair(err error) *types.ToolRepair {
	var structure *types.AnswerDocumentPatchStructureError
	if errors.As(err, &structure) {
		return answerDocumentPatchStructureRepair(structure)
	}
	return nil
}

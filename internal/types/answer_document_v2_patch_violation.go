package types

import "fmt"

// AnswerDocumentPatchOp is the closed set of emit_answer_document_patch
// operations a structural violation is attributed to. The values are the
// schema field names (the vocabulary the model already knows); internal
// routing only — never serialized into a prompt.
type AnswerDocumentPatchOp string

const (
	AnswerDocumentPatchOpUnchangedBlockIDs   AnswerDocumentPatchOp = "unchanged_block_ids"
	AnswerDocumentPatchOpRemoveBlockIDs      AnswerDocumentPatchOp = "remove_block_ids"
	AnswerDocumentPatchOpModelBlockOrder     AnswerDocumentPatchOp = "model_block_order"
	AnswerDocumentPatchOpReplaceBlocks       AnswerDocumentPatchOp = "replace_blocks"
	AnswerDocumentPatchOpBlockFieldEditsV1   AnswerDocumentPatchOp = "block_field_edits_v1"
	AnswerDocumentPatchOpBlockReceiptEditsV1 AnswerDocumentPatchOp = "block_receipt_edits_v1"
	AnswerDocumentPatchOpAddBlocks           AnswerDocumentPatchOp = "add_blocks"
	AnswerDocumentPatchOpCitations           AnswerDocumentPatchOp = "citations"
)

// AnswerDocumentPatchViolationKind is the closed, typed reason a patch entry
// is structurally rejected (V2-4, §40.51). The tool layer keys its repair
// table on this kind — never on the message prose — so a wording change can
// never silently drop a repair route.
type AnswerDocumentPatchViolationKind string

const (
	// AnswerDocumentPatchViolationEmptyID: an entry names an empty or
	// whitespace-padded block id.
	AnswerDocumentPatchViolationEmptyID AnswerDocumentPatchViolationKind = "empty_id"
	// AnswerDocumentPatchViolationUnknownID: an entry names a block id that is
	// not present in the previous emit.
	AnswerDocumentPatchViolationUnknownID AnswerDocumentPatchViolationKind = "unknown_id"
	// AnswerDocumentPatchViolationDuplicate: the same id / field is listed
	// twice inside one operation.
	AnswerDocumentPatchViolationDuplicate AnswerDocumentPatchViolationKind = "duplicate"
	// AnswerDocumentPatchViolationExistingBlock: add_blocks names an id that
	// already exists in the previous emit.
	AnswerDocumentPatchViolationExistingBlock AnswerDocumentPatchViolationKind = "existing_block"
	// AnswerDocumentPatchViolationCrossOpConflict: one id is targeted by two
	// mutually exclusive operations (replace + remove, edit + replace, …).
	AnswerDocumentPatchViolationCrossOpConflict AnswerDocumentPatchViolationKind = "cross_op_conflict"
	// AnswerDocumentPatchViolationSystemBlock: a local edit targets a
	// system-generated block.
	AnswerDocumentPatchViolationSystemBlock AnswerDocumentPatchViolationKind = "system_block"
	// AnswerDocumentPatchViolationInvalidKind: a replace/add block carries a
	// kind outside the canonical block-kind enum.
	AnswerDocumentPatchViolationInvalidKind AnswerDocumentPatchViolationKind = "invalid_kind"
	// AnswerDocumentPatchViolationInvalidField: a local edit names a field
	// outside the v1 whitelist.
	AnswerDocumentPatchViolationInvalidField AnswerDocumentPatchViolationKind = "invalid_field"
	// AnswerDocumentPatchViolationFieldKindMismatch: a local edit names a
	// whitelisted field the target block's kind does not carry.
	AnswerDocumentPatchViolationFieldKindMismatch AnswerDocumentPatchViolationKind = "field_kind_mismatch"
	// AnswerDocumentPatchViolationInvalidValue: a local edit value is outside
	// the field's closed enum or receipt shape.
	AnswerDocumentPatchViolationInvalidValue AnswerDocumentPatchViolationKind = "invalid_value"
	// AnswerDocumentPatchViolationRosterMismatch: model_block_order is not an
	// exact permutation of the previous model-owned roster.
	AnswerDocumentPatchViolationRosterMismatch AnswerDocumentPatchViolationKind = "roster_mismatch"
	// AnswerDocumentPatchViolationRosterChangeCombined: model_block_order was
	// combined with add_blocks / remove_block_ids.
	AnswerDocumentPatchViolationRosterChangeCombined AnswerDocumentPatchViolationKind = "roster_change_combined"
	// AnswerDocumentPatchViolationCitationModeConflict: replace_citations and
	// append_citations were both set.
	AnswerDocumentPatchViolationCitationModeConflict AnswerDocumentPatchViolationKind = "citation_mode_conflict"
	// AnswerDocumentPatchViolationPreservedCitationBlock: replace_citations
	// would strand a preserved block's citation_ref values.
	AnswerDocumentPatchViolationPreservedCitationBlock AnswerDocumentPatchViolationKind = "preserved_citation_block"
)

// AllAnswerDocumentPatchViolationKinds is the closed set in declaration order
// (single source for the tool repair table's completeness pin).
func AllAnswerDocumentPatchViolationKinds() []AnswerDocumentPatchViolationKind {
	return []AnswerDocumentPatchViolationKind{
		AnswerDocumentPatchViolationEmptyID,
		AnswerDocumentPatchViolationUnknownID,
		AnswerDocumentPatchViolationDuplicate,
		AnswerDocumentPatchViolationExistingBlock,
		AnswerDocumentPatchViolationCrossOpConflict,
		AnswerDocumentPatchViolationSystemBlock,
		AnswerDocumentPatchViolationInvalidKind,
		AnswerDocumentPatchViolationInvalidField,
		AnswerDocumentPatchViolationFieldKindMismatch,
		AnswerDocumentPatchViolationInvalidValue,
		AnswerDocumentPatchViolationRosterMismatch,
		AnswerDocumentPatchViolationRosterChangeCombined,
		AnswerDocumentPatchViolationCitationModeConflict,
		AnswerDocumentPatchViolationPreservedCitationBlock,
	}
}

// AnswerDocumentPatchViolation is one structural violation of an
// emit_answer_document_patch payload: the operation and typed kind carry the
// routing; Message is the exact model-facing text the historical serial gate
// produced (built once, at the arm that detects the violation, so the
// single-violation error and the numbered listing can never diverge).
type AnswerDocumentPatchViolation struct {
	Op      AnswerDocumentPatchOp
	Kind    AnswerDocumentPatchViolationKind
	BlockID string
	Field   string
	Message string
}

func newAnswerDocumentPatchViolation(op AnswerDocumentPatchOp, kind AnswerDocumentPatchViolationKind, blockID, field, format string, args ...any) *AnswerDocumentPatchViolation {
	return &AnswerDocumentPatchViolation{
		Op:      op,
		Kind:    kind,
		BlockID: blockID,
		Field:   field,
		Message: fmt.Sprintf(format, args...),
	}
}

// AnswerDocumentPatchStructureError is the single typed carrier every patch
// structure rejection travels in (V2-4, §40.51): ApplyAnswerDocumentV2Patch
// returns it with EVERY independent violation of the payload, so one reject
// round teaches every fix instead of burning one finalize retry per arm.
// Error() keeps the historical single-violation text byte-identical and
// renders two or more through the shared numbered formatter.
type AnswerDocumentPatchStructureError struct {
	Violations []AnswerDocumentPatchViolation
}

func (e *AnswerDocumentPatchStructureError) Error() string {
	if e == nil || len(e.Violations) == 0 {
		return "patch: structural validation failed"
	}
	messages := make([]string, 0, len(e.Violations))
	for _, v := range e.Violations {
		messages = append(messages, v.Message)
	}
	return ViolationListMessage(messages, func(n int) string {
		return fmt.Sprintf("patch: %d structural violations — fix ALL of them in this one patch: ", n)
	})
}

// Kinds returns the distinct violation kinds in first-seen order.
func (e *AnswerDocumentPatchStructureError) Kinds() []AnswerDocumentPatchViolationKind {
	if e == nil {
		return nil
	}
	seen := make(map[AnswerDocumentPatchViolationKind]bool, len(e.Violations))
	var out []AnswerDocumentPatchViolationKind
	for _, v := range e.Violations {
		if seen[v.Kind] {
			continue
		}
		seen[v.Kind] = true
		out = append(out, v.Kind)
	}
	return out
}

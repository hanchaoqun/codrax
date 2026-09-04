package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// emit_answer_document_patch_violation_list.go — V2-4 (§40.51): the patch
// executor's own pre-apply validators (field-edit / receipt-edit schema
// projection, local-lease whole-block mutation, split-companion disposition)
// now return every violation. These helpers render the accumulated list
// behind the historical first-violation summary (the roster-keyed message
// prefix stays verbatim; a single violation adds nothing) and build ONE
// repair whose Fields are the union over every violation.

// patchViolationListTail renders the "; this patch has N such violations —
// fix ALL of them …: [1] …; [2] …" suffix for two or more violations and the
// empty string for one, through the shared numbered formatter.
func patchViolationListTail(items []string) string {
	if len(items) < 2 {
		return ""
	}
	return fmt.Sprintf("; this patch has %d such violations — fix ALL of them in this one resubmission: %s",
		len(items), types.NumberedViolationList(items))
}

func answerDocumentPatchFieldEditSchemaViolationTail(violations []answerDocumentPatchFieldEditSchemaViolation) string {
	items := make([]string, 0, len(violations))
	for _, v := range violations {
		items = append(items, fmt.Sprintf("block_field_edits_v1[%d]: reason=%s field=%q block_id=%q", v.Index, v.Reason, v.Field, v.BlockID))
	}
	return patchViolationListTail(items)
}

// answerDocumentPatchFieldEditSchemaRepairAll leads with the first
// violation's repair (its reason-specific hint and metadata) and unions the
// Fields of every violation so the model revisits each entry at once.
func answerDocumentPatchFieldEditSchemaRepairAll(violations []answerDocumentPatchFieldEditSchemaViolation) *types.ToolRepair {
	if len(violations) == 0 {
		return nil
	}
	repair := answerDocumentPatchFieldEditSchemaRepair(violations[0])
	for _, v := range violations[1:] {
		repair.Fields = unionRepairFields(repair.Fields, answerDocumentPatchFieldEditSchemaRepair(v).Fields)
	}
	return attachPatchViolationCount(repair, len(violations))
}

func answerDocumentPatchReceiptEditSchemaViolationTail(violations []answerDocumentPatchReceiptEditSchemaViolation) string {
	items := make([]string, 0, len(violations))
	for _, v := range violations {
		items = append(items, fmt.Sprintf("block_receipt_edits_v1[%d]: reason=%s field=%q block_id=%q", v.Index, v.Reason, v.Field, v.BlockID))
	}
	return patchViolationListTail(items)
}

func answerDocumentPatchReceiptEditSchemaRepairAll(violations []answerDocumentPatchReceiptEditSchemaViolation) *types.ToolRepair {
	if len(violations) == 0 {
		return nil
	}
	repair := answerDocumentPatchReceiptEditSchemaRepair(violations[0])
	for _, v := range violations[1:] {
		repair.Fields = unionRepairFields(repair.Fields, answerDocumentPatchReceiptEditSchemaRepair(v).Fields)
	}
	return attachPatchViolationCount(repair, len(violations))
}

func localDiagramLeaseWholeBlockMutationViolationTail(violations []types.AnswerDiagramRelationRepairScopeViolation) string {
	items := make([]string, 0, len(violations))
	for _, v := range violations {
		items = append(items, fmt.Sprintf("block=%q operation=%s", v.BlockID, v.Issue))
	}
	return patchViolationListTail(items)
}

func splitCompanionDispositionViolationTail(failures []splitCompanionDispositionFailure) string {
	items := make([]string, 0, len(failures))
	for _, f := range failures {
		items = append(items, fmt.Sprintf("removed %q without disposing sibling %q", f.RemovedBlockID, f.CompanionBlockID))
	}
	return patchViolationListTail(items)
}

// splitCompanionDispositionRepairAll leads with the first pair's repair and
// lists every undisposed pair in metadata (removed_block_ids /
// companion_block_ids, comma-joined, in lineage order).
func splitCompanionDispositionRepairAll(failures []splitCompanionDispositionFailure) *types.ToolRepair {
	if len(failures) == 0 {
		return nil
	}
	repair := splitCompanionDispositionRepair(failures[0])
	if len(failures) > 1 {
		removed := make([]string, 0, len(failures))
		companions := make([]string, 0, len(failures))
		for _, f := range failures {
			removed = append(removed, f.RemovedBlockID)
			companions = append(companions, f.CompanionBlockID)
		}
		repair.Metadata["removed_block_ids"] = strings.Join(removed, ",")
		repair.Metadata["companion_block_ids"] = strings.Join(companions, ",")
		repair.Hint += " The summary lists every removed half whose sibling still needs an explicit disposition; decide all of them in the same patch."
	}
	return attachPatchViolationCount(repair, len(failures))
}

func unionRepairFields(base, extra []string) []string {
	seen := make(map[string]bool, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, field := range append(append([]string(nil), base...), extra...) {
		if seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}

func attachPatchViolationCount(repair *types.ToolRepair, count int) *types.ToolRepair {
	if repair == nil {
		return nil
	}
	if repair.Metadata == nil {
		repair.Metadata = map[string]string{}
	}
	repair.Metadata[types.ToolRepairMetaAnswerDocPatchViolationCount] = fmt.Sprintf("%d", count)
	return repair
}

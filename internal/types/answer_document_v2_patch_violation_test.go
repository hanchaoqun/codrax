package types

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// answer_document_v2_patch_violation_test.go — V2-4 (§40.51): the patch
// structure gate returns EVERY independent violation through one typed
// carrier. Red on the pre-V2-4 tree (validatePatchStructure returned the
// first fmt.Errorf; errors.As found no carrier; the dependent-arm pin saw
// only one violation) — proven with a `go test -overlay` of the 3080934fd
// file; green after the collector landed.

func patchStructureErrorOf(t *testing.T, prev *AnswerDocumentV2, p *AnswerDocumentV2Patch) *AnswerDocumentPatchStructureError {
	t.Helper()
	_, err := ApplyAnswerDocumentV2Patch(prev, p)
	if err == nil {
		t.Fatalf("patch must be rejected: %+v", p)
	}
	var structure *AnswerDocumentPatchStructureError
	if !errors.As(err, &structure) {
		t.Fatalf("patch structure reject must travel in *AnswerDocumentPatchStructureError, got %T: %v", err, err)
	}
	return structure
}

func TestApplyPatch_StructureRejectListsEveryIndependentViolation(t *testing.T) {
	prev := samplePrevDoc()
	structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{
		UnchangedBlockIDs: []string{"phantom"},
		ReplaceBlocks:     []AnswerBlock{{ID: "s1", Kind: BlockSummary}, {ID: "s1", Kind: BlockSummary}},
		AddBlocks:         []AnswerBlock{{ID: "lifecycle", Kind: BlockSection}},
		ReplaceCitations:  []Citation{{File: "z.go", Line: 1}},
		AppendCitations:   []Citation{{File: "w.go", Line: 2}},
	})
	want := []struct {
		op   AnswerDocumentPatchOp
		kind AnswerDocumentPatchViolationKind
		id   string
	}{
		{AnswerDocumentPatchOpUnchangedBlockIDs, AnswerDocumentPatchViolationUnknownID, "phantom"},
		{AnswerDocumentPatchOpReplaceBlocks, AnswerDocumentPatchViolationDuplicate, "s1"},
		{AnswerDocumentPatchOpAddBlocks, AnswerDocumentPatchViolationExistingBlock, "lifecycle"},
		{AnswerDocumentPatchOpCitations, AnswerDocumentPatchViolationCitationModeConflict, ""},
	}
	if len(structure.Violations) != len(want) {
		t.Fatalf("expected %d violations in field order, got %d: %+v", len(want), len(structure.Violations), structure.Violations)
	}
	for i, w := range want {
		got := structure.Violations[i]
		if got.Op != w.op || got.Kind != w.kind || got.BlockID != w.id {
			t.Fatalf("violation[%d] = {op=%s kind=%s id=%q}, want {op=%s kind=%s id=%q}", i, got.Op, got.Kind, got.BlockID, w.op, w.kind, w.id)
		}
		if got.Message == "" || !strings.Contains(structure.Error(), fmt.Sprintf("[%d] %s", i+1, got.Message)) {
			t.Fatalf("Error() must list violation %d verbatim under its number:\n%s", i+1, structure.Error())
		}
	}
	if !strings.HasPrefix(structure.Error(), "patch: 4 structural violations — fix ALL of them in this one patch: ") {
		t.Fatalf("multi-violation message must lead with the count sentence: %s", structure.Error())
	}
	if kinds := structure.Kinds(); len(kinds) != 4 {
		t.Fatalf("Kinds() must return the distinct kinds in order, got %v", kinds)
	}
}

// Every arm's single-violation message is byte-identical to the historical
// serial gate text (the first element of the list IS the serial error).
func TestApplyPatch_StructureRejectSingleViolationMessageIsSerialByteIdentical(t *testing.T) {
	prev := samplePrevDoc()
	system := samplePrevDoc()
	system.Blocks = append(system.Blocks, AnswerBlock{ID: "sys", Kind: BlockSection, SystemGeneratedKind: AnswerSystemGeneratedBlockKind("runtime_trace_supplement")})
	cases := []struct {
		name string
		prev *AnswerDocumentV2
		p    *AnswerDocumentV2Patch
		kind AnswerDocumentPatchViolationKind
		want string
	}{
		{"unchanged empty", prev, &AnswerDocumentV2Patch{UnchangedBlockIDs: []string{""}}, AnswerDocumentPatchViolationEmptyID, "patch: unchanged_block_ids contains empty id"},
		{"unchanged unknown", prev, &AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"phantom"}}, AnswerDocumentPatchViolationUnknownID, `patch: unchanged_block_ids["phantom"] not present in previous emit`},
		{"remove empty", prev, &AnswerDocumentV2Patch{RemoveBlockIDs: []string{""}}, AnswerDocumentPatchViolationEmptyID, "patch: remove_block_ids contains empty id"},
		{"remove duplicate", prev, &AnswerDocumentV2Patch{RemoveBlockIDs: []string{"s1", "s1"}}, AnswerDocumentPatchViolationDuplicate, `patch: remove_block_ids["s1"] duplicated`},
		{"order combined", prev, &AnswerDocumentV2Patch{ModelBlockOrder: []string{"s1", "lifecycle", "list1"}, RemoveBlockIDs: []string{"list1"}}, AnswerDocumentPatchViolationRosterChangeCombined, "patch: model_block_order cannot be combined with add_blocks or remove_block_ids; first settle the block roster, then submit one complete model-owned permutation"},
		{"order count", prev, &AnswerDocumentV2Patch{ModelBlockOrder: []string{"s1"}}, AnswerDocumentPatchViolationRosterMismatch, "patch: model_block_order must list every model-authored previous block exactly once: got 1 ids, want 3"},
		{"order padded", prev, &AnswerDocumentV2Patch{ModelBlockOrder: []string{" s1", "lifecycle", "list1"}}, AnswerDocumentPatchViolationEmptyID, "patch: model_block_order contains an empty or whitespace-padded id"},
		{"order duplicate", prev, &AnswerDocumentV2Patch{ModelBlockOrder: []string{"s1", "s1", "list1"}}, AnswerDocumentPatchViolationDuplicate, `patch: model_block_order["s1"] duplicated`},
		{"order unknown", prev, &AnswerDocumentV2Patch{ModelBlockOrder: []string{"s1", "lifecycle", "nope"}}, AnswerDocumentPatchViolationUnknownID, `patch: model_block_order["nope"] is not a model-authored block in the previous emit`},
		{"replace empty", prev, &AnswerDocumentV2Patch{ReplaceBlocks: []AnswerBlock{{Kind: BlockSummary}}}, AnswerDocumentPatchViolationEmptyID, "patch: replace_blocks entry with empty id (kind=summary)"},
		{"replace unknown", prev, &AnswerDocumentV2Patch{ReplaceBlocks: []AnswerBlock{{ID: "phantom", Kind: BlockSummary}}}, AnswerDocumentPatchViolationUnknownID, `patch: replace_blocks["phantom"] not present in previous emit (use add_blocks for new blocks)`},
		{"replace duplicate", prev, &AnswerDocumentV2Patch{ReplaceBlocks: []AnswerBlock{{ID: "s1", Kind: BlockSummary}, {ID: "s1", Kind: BlockSummary}}}, AnswerDocumentPatchViolationDuplicate, `patch: replace_blocks["s1"] duplicated`},
		{"replace+remove", prev, &AnswerDocumentV2Patch{ReplaceBlocks: []AnswerBlock{{ID: "s1", Kind: BlockSummary}}, RemoveBlockIDs: []string{"s1"}}, AnswerDocumentPatchViolationCrossOpConflict, `patch: replace_blocks["s1"] also in remove_block_ids — pick one`},
		{"field edit empty", prev, &AnswerDocumentV2Patch{BlockFieldEditsV1: []AnswerBlockFieldEditV1{{Field: AnswerBlockFieldSurfaceRole, Value: "principal"}}}, AnswerDocumentPatchViolationEmptyID, "patch: block_field_edits_v1 contains empty block_id"},
		{"field edit padded", prev, &AnswerDocumentV2Patch{BlockFieldEditsV1: []AnswerBlockFieldEditV1{{BlockID: " s1", Field: AnswerBlockFieldSurfaceRole, Value: "principal"}}}, AnswerDocumentPatchViolationEmptyID, `patch: block_field_edits_v1 block_id=" s1" must match the exact previous block id without surrounding whitespace`},
		{"field edit unknown", prev, &AnswerDocumentV2Patch{BlockFieldEditsV1: []AnswerBlockFieldEditV1{{BlockID: "phantom", Field: AnswerBlockFieldSurfaceRole, Value: "principal"}}}, AnswerDocumentPatchViolationUnknownID, `patch: block_field_edits_v1["phantom"] not present in previous emit`},
		{"field edit system", system, &AnswerDocumentV2Patch{BlockFieldEditsV1: []AnswerBlockFieldEditV1{{BlockID: "sys", Field: AnswerBlockFieldSurfaceRole, Value: "principal"}}}, AnswerDocumentPatchViolationSystemBlock, `patch: block_field_edits_v1["sys"] cannot edit a system-generated block`},
		{"field edit+remove", prev, &AnswerDocumentV2Patch{BlockFieldEditsV1: []AnswerBlockFieldEditV1{{BlockID: "s1", Field: AnswerBlockFieldSurfaceRole, Value: "principal"}}, RemoveBlockIDs: []string{"s1"}}, AnswerDocumentPatchViolationCrossOpConflict, `patch: block_field_edits_v1["s1"] also in remove_block_ids — pick one`},
		{"field edit+replace", prev, &AnswerDocumentV2Patch{BlockFieldEditsV1: []AnswerBlockFieldEditV1{{BlockID: "s1", Field: AnswerBlockFieldSurfaceRole, Value: "principal"}}, ReplaceBlocks: []AnswerBlock{{ID: "s1", Kind: BlockSummary}}}, AnswerDocumentPatchViolationCrossOpConflict, `patch: block_field_edits_v1["s1"] also in replace_blocks — pick one`},
		{"field edit duplicate", prev, &AnswerDocumentV2Patch{BlockFieldEditsV1: []AnswerBlockFieldEditV1{{BlockID: "s1", Field: AnswerBlockFieldSurfaceRole, Value: "principal"}, {BlockID: "s1", Field: AnswerBlockFieldSurfaceRole, Value: "principal"}}}, AnswerDocumentPatchViolationDuplicate, `patch: block_field_edits_v1["s1"].surface_role duplicated`},
		{"field edit kind mismatch", prev, &AnswerDocumentV2Patch{BlockFieldEditsV1: []AnswerBlockFieldEditV1{{BlockID: "lifecycle", Field: AnswerBlockFieldCurrentStatusVerdict, Value: "x"}}}, AnswerDocumentPatchViolationFieldKindMismatch, `patch: block_field_edits_v1["lifecycle"].current_status_verdict requires kind=decision`},
		{"field edit invalid value", prev, &AnswerDocumentV2Patch{BlockFieldEditsV1: []AnswerBlockFieldEditV1{{BlockID: "s1", Field: AnswerBlockFieldSurfaceRole, Value: "nope"}}}, AnswerDocumentPatchViolationInvalidValue, `patch: block_field_edits_v1["s1"].surface_role="nope" is invalid`},
		{"field edit invalid field", prev, &AnswerDocumentV2Patch{BlockFieldEditsV1: []AnswerBlockFieldEditV1{{BlockID: "s1", Field: AnswerBlockEditableFieldV1("text"), Value: "x"}}}, AnswerDocumentPatchViolationInvalidField, `patch: block_field_edits_v1["s1"].field="text" is not in the v1 whitelist`},
		{"receipt edit empty", prev, &AnswerDocumentV2Patch{BlockReceiptEditsV1: []AnswerBlockReceiptEditV1{{Field: AnswerBlockReceiptFieldRuntimeWorkRelation}}}, AnswerDocumentPatchViolationEmptyID, "patch: block_receipt_edits_v1 contains empty block_id"},
		{"receipt edit unknown", prev, &AnswerDocumentV2Patch{BlockReceiptEditsV1: []AnswerBlockReceiptEditV1{{BlockID: "phantom", Field: AnswerBlockReceiptFieldRuntimeWorkRelation}}}, AnswerDocumentPatchViolationUnknownID, `patch: block_receipt_edits_v1["phantom"] not present in previous emit`},
		{"receipt edit invalid value", prev, &AnswerDocumentV2Patch{BlockReceiptEditsV1: []AnswerBlockReceiptEditV1{{BlockID: "s1", Field: AnswerBlockReceiptFieldRuntimeWorkRelation}}}, AnswerDocumentPatchViolationInvalidValue, `patch: block_receipt_edits_v1["s1"].runtime_work_relation requires only observation_id and conclusion`},
		{"receipt edit invalid field", prev, &AnswerDocumentV2Patch{BlockReceiptEditsV1: []AnswerBlockReceiptEditV1{{BlockID: "s1", Field: AnswerBlockReceiptEditableFieldV1("other")}}}, AnswerDocumentPatchViolationInvalidField, `patch: block_receipt_edits_v1["s1"].field="other" is not in the v1 whitelist`},
		{"add empty", prev, &AnswerDocumentV2Patch{AddBlocks: []AnswerBlock{{Kind: BlockSummary}}}, AnswerDocumentPatchViolationEmptyID, "patch: add_blocks entry with empty id (kind=summary)"},
		{"add existing", prev, &AnswerDocumentV2Patch{AddBlocks: []AnswerBlock{{ID: "s1", Kind: BlockSummary}}}, AnswerDocumentPatchViolationExistingBlock, `patch: add_blocks["s1"] already exists in previous emit (use replace_blocks to modify)`},
		{"add duplicate", prev, &AnswerDocumentV2Patch{AddBlocks: []AnswerBlock{{ID: "n", Kind: BlockSummary}, {ID: "n", Kind: BlockSummary}}}, AnswerDocumentPatchViolationDuplicate, `patch: add_blocks["n"] duplicated`},
		{"add+remove", prev, &AnswerDocumentV2Patch{AddBlocks: []AnswerBlock{{ID: "n", Kind: BlockSummary}}, RemoveBlockIDs: []string{"n"}}, AnswerDocumentPatchViolationCrossOpConflict, `patch: add_blocks["n"] also in remove_block_ids — pick one`},
		{"add invalid kind", prev, &AnswerDocumentV2Patch{AddBlocks: []AnswerBlock{{ID: "n", Kind: AnswerBlockKind("nope")}}}, AnswerDocumentPatchViolationInvalidKind, `patch: add_blocks["n"] kind="nope" is not valid`},
		{"replace invalid kind", prev, &AnswerDocumentV2Patch{ReplaceBlocks: []AnswerBlock{{ID: "s1", Kind: AnswerBlockKind("nope")}}}, AnswerDocumentPatchViolationInvalidKind, `patch: replace_blocks["s1"] kind="nope" is not valid`},
		{"citation mode", prev, &AnswerDocumentV2Patch{ReplaceCitations: []Citation{{File: "z.go"}}, AppendCitations: []Citation{{File: "w.go"}}, RemoveBlockIDs: []string{"list1"}}, AnswerDocumentPatchViolationCitationModeConflict, "patch: replace_citations and append_citations are mutually exclusive (contract invariant 5); set exactly one"},
		{"preserved citation block", prev, &AnswerDocumentV2Patch{ReplaceCitations: []Citation{{File: "z.go"}}}, AnswerDocumentPatchViolationPreservedCitationBlock, `patch: replace_citations cannot preserve citation-bearing block "list1"; replace/remove that block too, use append_citations, or re-emit a full emit_answer_document so every citation_ref is renumbered against the new pool`},
	}
	seenKinds := map[AnswerDocumentPatchViolationKind]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			structure := patchStructureErrorOf(t, tc.prev, tc.p)
			if len(structure.Violations) != 1 {
				t.Fatalf("expected exactly one violation, got %d: %+v", len(structure.Violations), structure.Violations)
			}
			if structure.Violations[0].Kind != tc.kind {
				t.Fatalf("kind = %s, want %s", structure.Violations[0].Kind, tc.kind)
			}
			if got := structure.Error(); got != tc.want {
				t.Fatalf("single-violation Error() must be the serial text:\n got: %s\nwant: %s", got, tc.want)
			}
			seenKinds[tc.kind] = true
		})
	}
	for _, kind := range AllAnswerDocumentPatchViolationKinds() {
		if !seenKinds[kind] {
			t.Errorf("closed set member %s has no serial-parity case above", kind)
		}
	}
}

// A gating violation ends only its own entry's walk: the list never reports
// a follow-up the fix for the gating violation would re-shape anyway.
func TestApplyPatch_DependentArmsSkipAfterGatingViolation(t *testing.T) {
	prev := samplePrevDoc()
	t.Run("unknown block field edit reports only unknown_id", func(t *testing.T) {
		structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{BlockFieldEditsV1: []AnswerBlockFieldEditV1{
			{BlockID: "phantom", Field: AnswerBlockFieldSurfaceRole, Value: "nope"},
		}})
		if len(structure.Violations) != 1 || structure.Violations[0].Kind != AnswerDocumentPatchViolationUnknownID {
			t.Fatalf("unknown target must gate the value check, got %+v", structure.Violations)
		}
	})
	t.Run("order combined with add reports only roster_change_combined", func(t *testing.T) {
		structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{
			ModelBlockOrder: []string{"s1", "nope"},
			AddBlocks:       []AnswerBlock{{ID: "n", Kind: BlockSummary}},
		})
		if len(structure.Violations) != 1 || structure.Violations[0].Kind != AnswerDocumentPatchViolationRosterChangeCombined {
			t.Fatalf("roster change must gate the permutation checks, got %+v", structure.Violations)
		}
	})
	t.Run("citation mode conflict skips the preserved-citation scan", func(t *testing.T) {
		structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{
			ReplaceCitations: []Citation{{File: "z.go"}},
			AppendCitations:  []Citation{{File: "w.go"}},
		})
		if len(structure.Violations) != 1 || structure.Violations[0].Kind != AnswerDocumentPatchViolationCitationModeConflict {
			t.Fatalf("mode conflict must gate the preserved-citation scan, got %+v", structure.Violations)
		}
	})
	t.Run("independent entries keep reporting", func(t *testing.T) {
		structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{BlockFieldEditsV1: []AnswerBlockFieldEditV1{
			{BlockID: "phantom", Field: AnswerBlockFieldSurfaceRole, Value: "principal"},
			{BlockID: "s1", Field: AnswerBlockFieldSurfaceRole, Value: "nope"},
		}})
		if len(structure.Violations) != 2 ||
			structure.Violations[0].Kind != AnswerDocumentPatchViolationUnknownID ||
			structure.Violations[1].Kind != AnswerDocumentPatchViolationInvalidValue {
			t.Fatalf("a sibling entry's violation must still be listed, got %+v", structure.Violations)
		}
	})
}

func TestNumberedViolationListCapsAtTenPlusCount(t *testing.T) {
	var items []string
	for i := 0; i < 13; i++ {
		items = append(items, fmt.Sprintf("v%d", i))
	}
	got := NumberedViolationList(items)
	if !strings.HasPrefix(got, "[1] v0; [2] v1") || !strings.Contains(got, "[10] v9") ||
		strings.Contains(got, "v10") || !strings.HasSuffix(got, "; ... and 3 more violation(s)") {
		t.Fatalf("cap/format drifted: %s", got)
	}
	if ViolationListMessage([]string{"only"}, func(int) string { return "never: " }) != "only" {
		t.Fatal("a single violation must be returned verbatim")
	}
	if ViolationListMessage(nil, func(int) string { return "never: " }) != "" {
		t.Fatal("no violation renders nothing")
	}
}

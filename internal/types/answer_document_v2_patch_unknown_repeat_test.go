package types

import "testing"

// TestApplyPatch_RepeatedUnknownIDReportsOnceThenDuplicated — 收编复核再收编
// (§40.51, b6f2 #11): the duplicate gate runs BEFORE the unknown-id arm in
// every id-keyed arm, so a repeated id that never existed reports its
// `not present` line once and then one `duplicated` line — never two
// byte-identical `not present` lines occupying the 10-item cap. Red on
// 381f36cc9: replace_blocks=[{ghost},{ghost}] minted `[1] replace_blocks
// ["ghost"] not present …; [2] replace_blocks["ghost"] not present …`
// because the unknown-id `continue` ran before the id entered the seen set
// (block_field_edits_v1 / block_receipt_edits_v1 had the same order).
// unchanged_block_ids has no duplicate violation (a redundant Unchanged is
// intentionally harmless), so there the repeat of an unknown id is simply
// not re-reported: one line per distinct id.
func TestApplyPatch_RepeatedUnknownIDReportsOnceThenDuplicated(t *testing.T) {
	prev := samplePrevDoc()
	assertLines := func(t *testing.T, structure *AnswerDocumentPatchStructureError, want ...AnswerDocumentPatchViolationKind) {
		t.Helper()
		if len(structure.Violations) != len(want) {
			t.Fatalf("expected exactly %d violation line(s), got %d:\n%s", len(want), len(structure.Violations), structure.Error())
		}
		seen := map[string]bool{}
		for i, v := range structure.Violations {
			if v.Kind != want[i] {
				t.Fatalf("violation[%d] kind=%s, want %s:\n%s", i, v.Kind, want[i], structure.Error())
			}
			if seen[v.Message] {
				t.Fatalf("byte-identical violation line minted twice: %q\n%s", v.Message, structure.Error())
			}
			seen[v.Message] = true
		}
	}
	t.Run("replace_blocks repeated unknown id", func(t *testing.T) {
		structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{
			ReplaceBlocks: []AnswerBlock{{ID: "ghost", Kind: BlockSummary, Text: "a"}, {ID: "ghost", Kind: BlockSummary, Text: "b"}},
		})
		assertLines(t, structure, AnswerDocumentPatchViolationUnknownID, AnswerDocumentPatchViolationDuplicate)
	})
	t.Run("replace_blocks repeated unknown id plus remove of the same id", func(t *testing.T) {
		// The Execute-face analogue the review reached: remove of an absent
		// id is a valid no-op, so the executor normalizer keeps both replace
		// entries and the structural gate is what the model reads.
		structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{
			ReplaceBlocks:  []AnswerBlock{{ID: "ghost", Kind: BlockSummary, Text: "a"}, {ID: "ghost", Kind: BlockSummary, Text: "b"}},
			RemoveBlockIDs: []string{"ghost"},
		})
		assertLines(t, structure, AnswerDocumentPatchViolationUnknownID, AnswerDocumentPatchViolationDuplicate)
	})
	t.Run("replace_blocks unknown id does not enter the cross-op roster", func(t *testing.T) {
		// Dependency rule: the unknown-id line already tells the model to use
		// add_blocks; a phantom `add_blocks["ghost"] also in replace_blocks`
		// follow-up would be re-shaped by that same fix.
		structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{
			ReplaceBlocks: []AnswerBlock{{ID: "ghost", Kind: BlockSummary, Text: "a"}},
			AddBlocks:     []AnswerBlock{{ID: "ghost", Kind: BlockSummary, Text: "b"}},
		})
		assertLines(t, structure, AnswerDocumentPatchViolationUnknownID)
	})
	t.Run("block_field_edits_v1 repeated unknown id with differing values", func(t *testing.T) {
		structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{
			BlockFieldEditsV1: []AnswerBlockFieldEditV1{
				{BlockID: "ghost", Field: AnswerBlockFieldSurfaceRole, Value: string(SurfacePrincipal)},
				{BlockID: "ghost", Field: AnswerBlockFieldSurfaceRole, Value: "nope"},
			},
		})
		assertLines(t, structure, AnswerDocumentPatchViolationUnknownID, AnswerDocumentPatchViolationDuplicate)
	})
	t.Run("block_field_edits_v1 unknown block on distinct fields reports the block once", func(t *testing.T) {
		// The duplicate key is block+field, so two fields of one unknown
		// block are distinct entries — but the `not present` line names the
		// block, not the field, and would render byte-identical twice; the
		// missing block is one violation and is minted once (first field).
		structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{
			BlockFieldEditsV1: []AnswerBlockFieldEditV1{
				{BlockID: "ghost", Field: AnswerBlockFieldSurfaceRole, Value: string(SurfacePrincipal)},
				{BlockID: "ghost", Field: AnswerBlockFieldSurfaceRole + "_other", Value: "x"},
			},
		})
		assertLines(t, structure, AnswerDocumentPatchViolationUnknownID)
		if structure.Violations[0].Field != string(AnswerBlockFieldSurfaceRole) {
			t.Fatalf("the unknown-block line carries the first entry's field:\n%s", structure.Error())
		}
	})
	t.Run("block_receipt_edits_v1 unknown block on distinct fields reports the block once", func(t *testing.T) {
		structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{
			BlockReceiptEditsV1: []AnswerBlockReceiptEditV1{
				{BlockID: "ghost", Field: AnswerBlockReceiptFieldRuntimeWorkRelation},
				{BlockID: "ghost", Field: AnswerBlockReceiptFieldRuntimeWorkRelation + "_other"},
			},
		})
		assertLines(t, structure, AnswerDocumentPatchViolationUnknownID)
	})
	t.Run("block_receipt_edits_v1 repeated unknown id with differing values", func(t *testing.T) {
		structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{
			BlockReceiptEditsV1: []AnswerBlockReceiptEditV1{
				{BlockID: "ghost", Field: AnswerBlockReceiptFieldRuntimeWorkRelation, Value: AnswerBlockReceiptEditValueV1{
					ObservationID: "trace_query:test#trace_semantic_span:1",
					Conclusion:    string(RuntimeWorkRelationConclusionRelatedCausalityUnproven),
				}},
				{BlockID: "ghost", Field: AnswerBlockReceiptFieldRuntimeWorkRelation, Value: AnswerBlockReceiptEditValueV1{
					ObservationID: "trace_query:test#trace_semantic_span:2",
					Conclusion:    "nope",
				}},
			},
		})
		assertLines(t, structure, AnswerDocumentPatchViolationUnknownID, AnswerDocumentPatchViolationDuplicate)
	})
	t.Run("model_block_order repeated unknown id", func(t *testing.T) {
		// This arm was already duplicate-first; pinned so the symmetry holds
		// across every id-keyed arm. The roster-count mismatch line is an
		// independent violation of the same payload and is kept.
		structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{
			ModelBlockOrder: []string{"ghost", "ghost"},
		})
		seen := map[string]bool{}
		unknown, dup := 0, 0
		for _, v := range structure.Violations {
			if seen[v.Message] {
				t.Fatalf("byte-identical violation line minted twice: %q\n%s", v.Message, structure.Error())
			}
			seen[v.Message] = true
			switch v.Kind {
			case AnswerDocumentPatchViolationUnknownID:
				unknown++
			case AnswerDocumentPatchViolationDuplicate:
				dup++
			}
		}
		if unknown != 1 || dup != 1 {
			t.Fatalf("model_block_order repeat must report unknown once and duplicated once:\n%s", structure.Error())
		}
	})
	t.Run("unchanged_block_ids repeated unknown id reports one line", func(t *testing.T) {
		structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{
			UnchangedBlockIDs: []string{"ghost", "ghost"},
		})
		assertLines(t, structure, AnswerDocumentPatchViolationUnknownID)
	})
	t.Run("add_blocks repeated id stays duplicate-first", func(t *testing.T) {
		structure := patchStructureErrorOf(t, prev, &AnswerDocumentV2Patch{
			AddBlocks: []AnswerBlock{{ID: "n1", Kind: BlockSummary, Text: "a"}, {ID: "n1", Kind: "bogus", Text: "b"}},
		})
		assertLines(t, structure, AnswerDocumentPatchViolationDuplicate)
	})
}

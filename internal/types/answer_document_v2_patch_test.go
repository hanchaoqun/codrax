package types

import (
	"strings"
	"testing"
)

// answer_document_v2_patch_test.go — R16-c1 lock tests.
// Pins the protocol-level retry preservation contract:
// patch must preserve every typed annotation field (claim_use,
// facet_ids, surface_role, items[].claim_use) on Unchanged blocks
// — the bug R14 prompt-level Hard Rule could not consistently
// prevent (m1a r2 真证据).

func samplePrevDoc() *AnswerDocumentV2 {
	return &AnswerDocumentV2{
		DocumentModel: "v2",
		Citations: []Citation{
			{File: "x.go", Line: 10},
			{File: "y.go", Line: 20},
		},
		MissingRequestedRoles: []AnswerMissingRequestedRole{
			{Role: EvidenceDiagramRoleOverride, Label: "CLI"},
		},
		Blocks: []AnswerBlock{
			{
				ID: "s1", Kind: BlockSummary,
				SurfaceRole: SurfacePrincipal,
				Text:        "summary text",
				FacetIDs:    []string{"current_code_path"},
				ClaimUses: []RenderedClaimUse{
					{ClaimForm: ClaimDefinitionFact, EvidenceID: "ev1"},
				},
			},
			{
				ID: "lifecycle", Kind: BlockSection,
				SurfaceRole: SurfacePrincipal,
				Title:       "Lifecycle",
				Text:        "lifecycle prose",
				FacetIDs:    []string{"component_relation"},
				ClaimUses: []RenderedClaimUse{
					{ClaimForm: ClaimDefinitionFact, EvidenceID: "ev2"},
				},
			},
			{
				ID: "list1", Kind: BlockOrderedList,
				Items: []AnswerBlockItem{
					{ID: "i1", Label: "A", CitationRef: 0,
						ClaimUse: &RenderedClaimUse{ClaimForm: ClaimCallEdge}},
					{ID: "i2", Label: "B", CitationRef: 1},
				},
			},
		},
	}
}

// TestApplyPatch_NilPrevRejects covers the nil-prev guard.
func TestApplyPatch_NilPrevRejects(t *testing.T) {
	patch := &AnswerDocumentV2Patch{UnchangedBlockIDs: []string{"x"}}
	if _, err := ApplyAnswerDocumentV2Patch(nil, patch); err == nil {
		t.Error("nil prev must reject — first emit MUST use emit_answer_document, not patch")
	}
}

// TestApplyPatch_EmptyPatchRejects pins the "every retry must
// declare some change" invariant.
func TestApplyPatch_EmptyPatchRejects(t *testing.T) {
	prev := samplePrevDoc()
	if _, err := ApplyAnswerDocumentV2Patch(prev, nil); err == nil {
		t.Error("nil patch must reject")
	}
	if _, err := ApplyAnswerDocumentV2Patch(prev, &AnswerDocumentV2Patch{}); err == nil {
		t.Error("empty patch must reject")
	}
}

// TestApplyPatch_PureUnchangedPreservesAllFields is the **R16
// load-bearing test**: when the LLM declares "all blocks unchanged",
// every typed annotation field on every block flows through
// structurally — claim_use / facet_ids / surface_role / items[]
// claim_use all preserved. This is exactly what R14 prompt-level
// Hard Rule failed to guarantee in m1a r2 (LLM dropped block-level
// claim_use across retries).
func TestApplyPatch_PureUnchangedPreservesAllFields(t *testing.T) {
	prev := samplePrevDoc()
	patch := &AnswerDocumentV2Patch{
		UnchangedBlockIDs: []string{"s1", "lifecycle", "list1"},
	}
	got, err := ApplyAnswerDocumentV2Patch(prev, patch)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if got.DocumentModel != "v2" {
		t.Errorf("DocumentModel = %q, want v2", got.DocumentModel)
	}
	if len(got.Blocks) != len(prev.Blocks) {
		t.Fatalf("block count = %d, want %d", len(got.Blocks), len(prev.Blocks))
	}
	// Block-level claim_use preservation
	if len(got.Blocks[0].ClaimUses) == 0 {
		t.Error("s1 ClaimUses lost (load-bearing R16 invariant)")
	}
	if got.Blocks[0].ClaimUses[0].ClaimForm != ClaimDefinitionFact {
		t.Errorf("s1 ClaimForm changed; got %q", got.Blocks[0].ClaimUses[0].ClaimForm)
	}
	// FacetIDs preservation
	if len(got.Blocks[0].FacetIDs) != 1 || got.Blocks[0].FacetIDs[0] != "current_code_path" {
		t.Errorf("s1 FacetIDs lost; got %+v", got.Blocks[0].FacetIDs)
	}
	// SurfaceRole preservation
	if got.Blocks[1].SurfaceRole != SurfacePrincipal {
		t.Errorf("lifecycle SurfaceRole lost")
	}
	// Item-level claim_use preservation
	if got.Blocks[2].Items[0].ClaimUse == nil ||
		got.Blocks[2].Items[0].ClaimUse.ClaimForm != ClaimCallEdge {
		t.Errorf("list1.i1 item-level claim_use lost")
	}
	// Citation pool preservation
	if len(got.Citations) != 2 {
		t.Errorf("citations dropped; got %d, want 2", len(got.Citations))
	}
	if len(got.MissingRequestedRoles) != 1 || got.MissingRequestedRoles[0].Role != EvidenceDiagramRoleOverride {
		t.Errorf("missing requested roles not preserved; got %+v", got.MissingRequestedRoles)
	}
}

// TestApplyPatch_ReplaceOneBlockKeepsOthers covers the typical
// retry path: LLM replaces one buggy block, keeps the rest.
func TestApplyPatch_ReplaceOneBlockKeepsOthers(t *testing.T) {
	prev := samplePrevDoc()
	patch := &AnswerDocumentV2Patch{
		UnchangedBlockIDs: []string{"s1", "list1"},
		ReplaceBlocks: []AnswerBlock{
			{
				ID: "lifecycle", Kind: BlockSection,
				Title:    "Lifecycle (revised)",
				Text:     "fixed prose",
				FacetIDs: []string{"component_relation", "nearest_mechanism"},
				ClaimUses: []RenderedClaimUse{
					{ClaimForm: ClaimDefinitionFact, EvidenceID: "ev2"},
				},
				SurfaceRole: SurfacePrincipal,
			},
		},
	}
	got, err := ApplyAnswerDocumentV2Patch(prev, patch)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(got.Blocks) != 3 {
		t.Fatalf("block count = %d, want 3", len(got.Blocks))
	}
	// Block order preserved (replace at original position)
	if got.Blocks[0].ID != "s1" || got.Blocks[1].ID != "lifecycle" || got.Blocks[2].ID != "list1" {
		t.Errorf("block order broken: %s, %s, %s",
			got.Blocks[0].ID, got.Blocks[1].ID, got.Blocks[2].ID)
	}
	// Replacement applied
	if got.Blocks[1].Title != "Lifecycle (revised)" {
		t.Errorf("replacement not applied; got Title=%q", got.Blocks[1].Title)
	}
	// Unchanged blocks preserved
	if got.Blocks[0].Text != "summary text" {
		t.Error("s1 unchanged block text changed")
	}
}

// TestApplyPatch_AddNewBlock appends to the tail.
func TestApplyPatch_AddNewBlock(t *testing.T) {
	prev := samplePrevDoc()
	patch := &AnswerDocumentV2Patch{
		AddBlocks: []AnswerBlock{
			{ID: "diag1", Kind: BlockDiagram, Text: "new diagram block"},
		},
	}
	got, err := ApplyAnswerDocumentV2Patch(prev, patch)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(got.Blocks) != 4 {
		t.Fatalf("block count = %d, want 4", len(got.Blocks))
	}
	if got.Blocks[3].ID != "diag1" {
		t.Errorf("added block at wrong position; last id = %q", got.Blocks[3].ID)
	}
}

// TestApplyPatch_RemoveBlock drops by id.
func TestApplyPatch_RemoveBlock(t *testing.T) {
	prev := samplePrevDoc()
	patch := &AnswerDocumentV2Patch{
		RemoveBlockIDs: []string{"lifecycle"},
	}
	got, err := ApplyAnswerDocumentV2Patch(prev, patch)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(got.Blocks) != 2 {
		t.Fatalf("block count = %d, want 2", len(got.Blocks))
	}
	for _, b := range got.Blocks {
		if b.ID == "lifecycle" {
			t.Error("lifecycle block not removed")
		}
	}
}

// TestApplyPatch_ReplaceCitations swaps the citation pool.
func TestApplyPatch_ReplaceCitations(t *testing.T) {
	prev := samplePrevDoc()
	patch := &AnswerDocumentV2Patch{
		ReplaceCitations: []Citation{{File: "z.go", Line: 99}},
	}
	got, err := ApplyAnswerDocumentV2Patch(prev, patch)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(got.Citations) != 1 || got.Citations[0].File != "z.go" {
		t.Errorf("citations not replaced; got %+v", got.Citations)
	}
}

// TestApplyPatch_AppendCitations adds without rewriting.
func TestApplyPatch_AppendCitations(t *testing.T) {
	prev := samplePrevDoc()
	patch := &AnswerDocumentV2Patch{
		AppendCitations: []Citation{
			{File: "z.go", Line: 99, Scope: ScopeNegative, NegativePattern: "rg foo"},
		},
	}
	got, err := ApplyAnswerDocumentV2Patch(prev, patch)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(got.Citations) != 3 {
		t.Errorf("citations not appended; got %d, want 3", len(got.Citations))
	}
}

func TestApplyPatch_ReplaceMissingRequestedRoles(t *testing.T) {
	prev := samplePrevDoc()
	patch := &AnswerDocumentV2Patch{
		ReplaceMissingRequestedRoles: []AnswerMissingRequestedRole{
			{Role: EvidenceDiagramRoleConfig, Label: "codrax.yaml"},
			{Role: EvidenceDiagramRoleOverride, Label: "CLI"},
		},
	}
	got, err := ApplyAnswerDocumentV2Patch(prev, patch)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(got.MissingRequestedRoles) != 2 {
		t.Fatalf("missing requested roles count = %d, want 2", len(got.MissingRequestedRoles))
	}
	if got.MissingRequestedRoles[0].Role != EvidenceDiagramRoleConfig || got.MissingRequestedRoles[1].Role != EvidenceDiagramRoleOverride {
		t.Fatalf("missing requested roles replaced incorrectly: %+v", got.MissingRequestedRoles)
	}
}

// ── Validation rejection tests ───────────────────────────────

// TestApplyPatch_RejectsUnknownBlockID covers the unknown-id guard.
func TestApplyPatch_RejectsUnknownBlockID(t *testing.T) {
	prev := samplePrevDoc()
	cases := []*AnswerDocumentV2Patch{
		{UnchangedBlockIDs: []string{"phantom"}},
		{RemoveBlockIDs: []string{"phantom"}},
		{ReplaceBlocks: []AnswerBlock{{ID: "phantom", Kind: BlockSummary}}},
	}
	for i, p := range cases {
		if _, err := ApplyAnswerDocumentV2Patch(prev, p); err == nil {
			t.Errorf("case %d: must reject unknown id", i)
		} else if !strings.Contains(err.Error(), "phantom") {
			t.Errorf("case %d: error must name phantom; got %q", i, err)
		}
	}
}

// TestApplyPatch_RejectsConflictingOps covers cross-op conflict
// detection — the 3 mutual-exclusion invariants from the godoc:
//
//	(1) Replace∩Remove ∅
//	(2) Replace∩Add ∅
//	(3) Add∩Remove ∅
func TestApplyPatch_RejectsConflictingOps(t *testing.T) {
	prev := samplePrevDoc()
	cases := []struct {
		name  string
		patch *AnswerDocumentV2Patch
	}{
		{
			name: "(1) Replace + Remove same id",
			patch: &AnswerDocumentV2Patch{
				RemoveBlockIDs: []string{"s1"},
				ReplaceBlocks:  []AnswerBlock{{ID: "s1", Kind: BlockSummary}},
			},
		},
		{
			name: "(2) Replace + Add same id",
			patch: &AnswerDocumentV2Patch{
				ReplaceBlocks: []AnswerBlock{{ID: "s1", Kind: BlockSummary}},
				AddBlocks:     []AnswerBlock{{ID: "s1", Kind: BlockSummary}},
			},
		},
		{
			name: "(3) Add + Remove same id (Add MUST be NEW; cannot also be Removed)",
			patch: &AnswerDocumentV2Patch{
				AddBlocks:      []AnswerBlock{{ID: "newAndRemoved", Kind: BlockSummary, Text: "x"}},
				RemoveBlockIDs: []string{"newAndRemoved"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ApplyAnswerDocumentV2Patch(prev, tc.patch); err == nil {
				t.Errorf("conflicting ops must reject")
			}
		})
	}
}

// TestApplyPatch_CitationsReplaceXorAppend pins mutation-contract
// invariant (5): ReplaceCitations and AppendCitations are mutually
// exclusive. Setting BOTH must reject — the LLM is signalling
// contradictory intents (replace pool wholesale vs additive).
func TestApplyPatch_CitationsReplaceXorAppend(t *testing.T) {
	prev := samplePrevDoc()
	patch := &AnswerDocumentV2Patch{
		ReplaceCitations: []Citation{{File: "a.go", Line: 1}},
		AppendCitations:  []Citation{{File: "b.go", Line: 2}},
	}
	if _, err := ApplyAnswerDocumentV2Patch(prev, patch); err == nil {
		t.Error("ReplaceCitations + AppendCitations both set must reject (mutation contract invariant 5)")
	}
}

// TestApplyPatch_RejectsAddWithExistingID covers the add+exists
// guard.
func TestApplyPatch_RejectsAddWithExistingID(t *testing.T) {
	prev := samplePrevDoc()
	patch := &AnswerDocumentV2Patch{
		AddBlocks: []AnswerBlock{{ID: "s1", Kind: BlockSummary}},
	}
	if _, err := ApplyAnswerDocumentV2Patch(prev, patch); err == nil {
		t.Error("add_blocks with existing id must reject")
	}
}

// TestApplyPatch_RejectsDuplicateInOp covers per-op dup id detection.
func TestApplyPatch_RejectsDuplicateInOp(t *testing.T) {
	prev := samplePrevDoc()
	cases := []*AnswerDocumentV2Patch{
		// Replace dup
		{ReplaceBlocks: []AnswerBlock{
			{ID: "s1", Kind: BlockSummary},
			{ID: "s1", Kind: BlockSummary},
		}},
		// Remove dup
		{RemoveBlockIDs: []string{"s1", "s1"}},
		// Add dup
		{AddBlocks: []AnswerBlock{
			{ID: "new1", Kind: BlockSummary},
			{ID: "new1", Kind: BlockSummary},
		}},
	}
	for i, p := range cases {
		if _, err := ApplyAnswerDocumentV2Patch(prev, p); err == nil {
			t.Errorf("case %d: dup id must reject", i)
		}
	}
}

// TestApplyPatch_RejectsInvalidKind covers the kind-validity gate.
func TestApplyPatch_RejectsInvalidKind(t *testing.T) {
	prev := samplePrevDoc()
	cases := []*AnswerDocumentV2Patch{
		{ReplaceBlocks: []AnswerBlock{{ID: "s1", Kind: "bogus_kind"}}},
		{AddBlocks: []AnswerBlock{{ID: "new1", Kind: "bogus_kind"}}},
	}
	for i, p := range cases {
		if _, err := ApplyAnswerDocumentV2Patch(prev, p); err == nil {
			t.Errorf("case %d: invalid kind must reject", i)
		}
	}
}

// TestApplyPatch_RejectsEmptyBlockID covers empty-id detection.
func TestApplyPatch_RejectsEmptyBlockID(t *testing.T) {
	prev := samplePrevDoc()
	cases := []*AnswerDocumentV2Patch{
		{UnchangedBlockIDs: []string{""}},
		{RemoveBlockIDs: []string{""}},
		{ReplaceBlocks: []AnswerBlock{{ID: "", Kind: BlockSummary}}},
		{AddBlocks: []AnswerBlock{{ID: "", Kind: BlockSummary}}},
	}
	for i, p := range cases {
		if _, err := ApplyAnswerDocumentV2Patch(prev, p); err == nil {
			t.Errorf("case %d: empty id must reject", i)
		}
	}
}

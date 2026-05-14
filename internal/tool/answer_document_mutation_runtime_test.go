package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_mutation_runtime_test.go — B4 v3 (2026-05-04).
// Tests that both full and patch paths converge on
// ApplyAndPersistMutation: shared merged-doc validation, shared
// setter, shared telemetry summary.

func newBusForMutationTest() *types.BusContext {
	return &types.BusContext{
		Mutable: types.NewMutableState("test"),
	}
}

// TestApplyAndPersistMutation_ReplaceAllPersistsDocAndClearsPatchFlag
// — happy path: ReplaceAll → SetAnswerDocumentV2WithMutation called
// with MutationReplaceAll → LastEmitFromPatch=false.
func TestApplyAndPersistMutation_ReplaceAllPersistsDocAndClearsPatchFlag(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "answer"},
		},
	}
	mutation := types.NewReplaceAllMutation(doc)
	res, err := ApplyAndPersistMutation(bus, "test_emit", mutation, nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	if got := bus.Mutable.AnswerDocumentV2(); got == nil || len(got.Blocks) != 1 {
		t.Fatalf("merged doc not persisted; got %+v", got)
	}
	if bus.Mutable.LastEmitFromPatch() {
		t.Errorf("ReplaceAll must clear LastEmitFromPatch")
	}
}

// TestApplyAndPersistMutation_PartialPersistsDocAndSetsPatchFlag
// — patch path: NewPartialMutation → LastEmitFromPatch=true.
func TestApplyAndPersistMutation_PartialPersistsDocAndSetsPatchFlag(t *testing.T) {
	bus := newBusForMutationTest()
	prev := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "old"},
		},
	}
	patch := &types.AnswerDocumentV2Patch{
		ReplaceBlocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "new"},
		},
	}
	mutation := types.NewPartialMutation(patch)
	res, err := ApplyAndPersistMutation(bus, "test_patch", mutation, prev, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	if got := bus.Mutable.AnswerDocumentV2(); got == nil || got.Blocks[0].Text != "new" {
		t.Fatalf("patched doc not persisted correctly; got %+v", got)
	}
	if !bus.Mutable.LastEmitFromPatch() {
		t.Errorf("Partial mutation must set LastEmitFromPatch=true")
	}
}

// TestApplyAndPersistMutation_DuplicateBlockIDRejected — merged-doc
// validation enforces unique block ids. Both paths get the same
// rejection message.
func TestApplyAndPersistMutation_DuplicateBlockIDRejected(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "x", Kind: types.BlockSummary, Text: "a"},
			{ID: "x", Kind: types.BlockSection, Text: "b"},
		},
	}
	mutation := types.NewReplaceAllMutation(doc)
	res, _ := ApplyAndPersistMutation(bus, "test_emit", mutation, nil, time.Now())
	if res.Success {
		t.Fatalf("expected rejection on duplicate id; got Success=true")
	}
	if !strings.Contains(res.Summary, "duplicate id") {
		t.Errorf("rejection should name 'duplicate id'; got %q", res.Summary)
	}
}

// TestApplyAndPersistMutation_DiagramWithNilPayloadRejected — diagram
// kind requires a non-nil Diagram payload.
func TestApplyAndPersistMutation_DiagramWithNilPayloadRejected(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "d", Kind: types.BlockDiagram, Diagram: nil},
		},
	}
	mutation := types.NewReplaceAllMutation(doc)
	res, _ := ApplyAndPersistMutation(bus, "test_emit", mutation, nil, time.Now())
	if res.Success {
		t.Fatalf("expected rejection; got Success=true")
	}
	if !strings.Contains(res.Summary, "diagram") {
		t.Errorf("rejection should name 'diagram'; got %q", res.Summary)
	}
}

func TestApplyAndPersistMutation_DiagramPayloadOnSectionRejected(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "s",
			Kind: types.BlockSection,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramFlow,
				Body: "flowchart TD\n  A --> B",
			},
		}},
	}
	mutation := types.NewReplaceAllMutation(doc)
	res, _ := ApplyAndPersistMutation(bus, "test_emit", mutation, nil, time.Now())
	if res.Success {
		t.Fatalf("expected rejection; got Success=true")
	}
	if !strings.Contains(res.Summary, "kind=diagram") {
		t.Errorf("rejection should steer to kind=diagram; got %q", res.Summary)
	}
}

func TestApplyAndPersistMutation_CanonicalizesSummaryLeadBlock(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:    "items",
				Kind:  types.BlockOrderedList,
				Items: []types.AnswerBlockItem{{ID: "i1", Label: "A"}},
			},
			{ID: "caveat", Kind: types.BlockCaveat, Text: "scope"},
			{ID: "summary", Kind: types.BlockSummary, Text: "lead"},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit",
		types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected summary-order canonicalization to accept doc; got %+v", res)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 3 {
		t.Fatalf("merged doc not persisted; got %+v", got)
	}
	if got.Blocks[0].ID != "summary" || got.Blocks[1].ID != "items" || got.Blocks[2].ID != "caveat" {
		t.Fatalf("summary should move to lead while preserving detail order, got block ids: %v",
			[]string{got.Blocks[0].ID, got.Blocks[1].ID, got.Blocks[2].ID})
	}
}

// TestApplyAndPersistMutation_FullAndPatchProduceByteIdenticalMerged
// — same logical doc reached via full vs patch paths produces an
// identical merged AnswerDocumentV2 in MutableState.
func TestApplyAndPersistMutation_FullAndPatchProduceByteIdenticalMerged(t *testing.T) {
	target := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "the answer"},
		},
		Citations: []types.Citation{{File: "foo.go", Line: 1}},
	}

	// Full path.
	busFull := newBusForMutationTest()
	if _, err := ApplyAndPersistMutation(busFull, "full",
		types.NewReplaceAllMutation(target), nil, time.Now()); err != nil {
		t.Fatalf("full apply: %v", err)
	}

	// Patch path: prev has different text; patch replaces.
	busPatch := newBusForMutationTest()
	prev := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "old"},
		},
		Citations: []types.Citation{{File: "foo.go", Line: 1}},
	}
	busPatch.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, prev)
	patch := &types.AnswerDocumentV2Patch{
		ReplaceBlocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "the answer"},
		},
	}
	if _, err := ApplyAndPersistMutation(busPatch, "patch",
		types.NewPartialMutation(patch), prev, time.Now()); err != nil {
		t.Fatalf("patch apply: %v", err)
	}

	full := busFull.Mutable.AnswerDocumentV2()
	patched := busPatch.Mutable.AnswerDocumentV2()
	if full == nil || patched == nil {
		t.Fatalf("docs not persisted")
	}
	if full.Blocks[0].Text != patched.Blocks[0].Text {
		t.Errorf("merged doc text differs:\n  full=%q\n  patch=%q",
			full.Blocks[0].Text, patched.Blocks[0].Text)
	}
}

// TestBuildAnswerDocumentSemanticContractDescription_SharedBetweenTools
// — sanity check the SST helper renders content both Description()
// outputs include.
func TestBuildAnswerDocumentSemanticContractDescription_SharedBetweenTools(t *testing.T) {
	body := BuildAnswerDocumentSemanticContractDescription()
	if !strings.Contains(body, "Block kinds") || !strings.Contains(body, "summary") || !strings.Contains(body, "ordered_list") {
		t.Errorf("body missing canonical block-kind list; got: %.200s...", body)
	}
	if !strings.Contains(body, "claim_form values") {
		t.Errorf("body missing claim_form enum guidance")
	}
	full := (&EmitAnswerDocument{}).Description()
	patch := (&EmitAnswerDocumentPatch{}).Description()
	if !strings.Contains(full, body) {
		t.Errorf("full Description() missing the shared SST body")
	}
	if !strings.Contains(patch, body) {
		t.Errorf("patch Description() missing the shared SST body")
	}
}

// TestApplyAndPersistMutation_SummaryReportsMutationKind — ToolResult
// Summary names the mutation surface so operators can grep telemetry.
func TestApplyAndPersistMutation_SummaryReportsMutationKind(t *testing.T) {
	bus := newBusForMutationTest()
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
		},
	}
	res, _ := ApplyAndPersistMutation(bus, "tool",
		types.NewReplaceAllMutation(doc), nil, time.Now())
	if !strings.Contains(res.Summary, "replace_all") {
		t.Errorf("Summary should name mutation kind; got %q", res.Summary)
	}
}

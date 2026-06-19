package tool

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestApplyAndPersistMutation_StampsReadOwnerAnchorsFromTurnA(t *testing.T) {
	bus := newBusForMutationTest()
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		SourceLocalization: &types.SourceLocalizationReview{
			Source:      "read_turn_a",
			Status:      types.SourceLocalizationObserved,
			SourcePaths: []string{"pkg/observed.py", "pkg/owner.py"},
			Anchors: []types.SourceLocalizationAnchor{{
				Path:     "pkg/observed.py",
				Kind:     types.SourceLocalizationAnchorReadFile,
				Strength: types.SourceLocalizationAnchorObserved,
			}, {
				Path:        "pkg/owner.py",
				Kind:        types.SourceLocalizationAnchorGroundedEvidence,
				Strength:    types.SourceLocalizationAnchorOwner,
				OwnerSymbol: "Owner.Handle",
				EvidenceRef: &types.WriteExplorationEvidenceRef{
					ID:          "ev-owner",
					Source:      "pkg/owner.py",
					LineStart:   12,
					OwnerSymbol: "Owner.Handle",
				},
			}},
		},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "answer"},
		},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.ReadOwnerAnchors) != 1 {
		t.Fatalf("read owner anchors not stamped: %+v", got)
	}
	if got.ReadSourceLocalization == nil || got.ReadSourceLocalization.Status != types.SourceLocalizationObserved {
		t.Fatalf("read source localization not stamped: %+v", got.ReadSourceLocalization)
	}
	if len(got.ReadSourceLocalization.SourcePaths) == 0 || got.ReadSourceLocalization.SourcePaths[0] != "pkg/observed.py" {
		t.Fatalf("read source localization lost observed path: %+v", got.ReadSourceLocalization)
	}
	if got.ReadOwnerAnchors[0].Path != "pkg/owner.py" || got.ReadOwnerAnchors[0].OwnerSymbol != "Owner.Handle" {
		t.Fatalf("wrong stamped anchor: %+v", got.ReadOwnerAnchors[0])
	}
	if got.ReadOwnerAnchors[0].EvidenceRef == nil || got.ReadOwnerAnchors[0].EvidenceRef.ID != "ev-owner" {
		t.Fatalf("stamped anchor lost evidence ref: %+v", got.ReadOwnerAnchors[0])
	}
}

func TestApplyAndPersistMutation_StampsStructuralReadOwnerAnchorsFromLineEvidence(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := []byte(`class Documenter:
    def filter_members(self):
        has_doc = bool(doc)
`)
	if err := os.WriteFile(filepath.Join(repo, "pkg", "owner.py"), src, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	bus := newBusForMutationTest()
	bus.RepoRoot = repo
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		SourceLocalization: &types.SourceLocalizationReview{
			Source:      "read_turn_a",
			SourcePaths: []string{"pkg/owner.py"},
			EvidenceRefs: []types.WriteExplorationEvidenceRef{{
				ID:        "ev-line",
				Kind:      "relationship",
				Source:    "pkg/owner.py",
				LineStart: 3,
			}},
		},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks:        []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "answer"}},
	}

	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.ReadOwnerAnchors) != 1 {
		t.Fatalf("read owner anchors not stamped from structural evidence: %+v", got)
	}
	if got.ReadOwnerAnchors[0].OwnerSymbol != "Documenter.filter_members" ||
		got.ReadOwnerAnchors[0].Strength != types.SourceLocalizationAnchorOwner {
		t.Fatalf("wrong structural owner anchor: %+v", got.ReadOwnerAnchors[0])
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

func TestApplyAndPersistMutation_AcceptedDocDropsRejectedTextAttachments(t *testing.T) {
	bus := newBusForMutationTest()
	bus.Mutable.SetAnswerDisplayAttachments([]types.AnswerDisplayAttachment{
		{
			Kind:   types.AnswerDisplayAttachmentMarkdown,
			Body:   "stale rejected prose",
			Source: "emit_answer_document.rejected_payload",
			Reason: "rejected structured answer draft contained user-visible text",
		},
		{
			Kind:     types.AnswerDisplayAttachmentDiagram,
			Language: "mermaid",
			Body:     "flowchart TD\n  A --> B",
			Source:   "emit_answer_document.rejected_payload",
		},
	})
	prev := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "old"},
		},
	}
	patch := &types.AnswerDocumentV2Patch{
		ReplaceBlocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "accepted"},
		},
	}
	res, err := ApplyAndPersistMutation(bus, "test_patch", types.NewPartialMutation(patch), prev, time.Now())
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ToolResult.Success = false: %s", res.Summary)
	}
	attachments := bus.Mutable.AnswerDisplayAttachments()
	if len(attachments) != 1 || attachments[0].Kind != types.AnswerDisplayAttachmentDiagram {
		t.Fatalf("accepted structured doc should drop stale text but preserve diagram attachments, got %+v", attachments)
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

func TestApplyAndPersistMutation_DiagramPayloadOnSectionNormalizesKind(t *testing.T) {
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
	if !res.Success {
		t.Fatalf("expected diagram discriminator repair to succeed; got %q", res.Summary)
	}
	got := bus.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 1 {
		t.Fatalf("persisted doc missing: %+v", got)
	}
	if got.Blocks[0].Kind != types.BlockDiagram {
		t.Fatalf("persisted kind = %q, want diagram", got.Blocks[0].Kind)
	}
	if got.Blocks[0].Diagram == nil || !strings.Contains(got.Blocks[0].Diagram.Body, "A --> B") {
		t.Fatalf("diagram payload should be preserved, got %+v", got.Blocks[0].Diagram)
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
	for _, surface := range []struct {
		name string
		text string
	}{
		{name: "shared description", text: body},
		{name: "full description", text: full},
		{name: "patch description", text: patch},
		{name: "full parameters", text: string((&EmitAnswerDocument{}).Parameters())},
	} {
		for _, forbidden := range []string{"citation_ref=-1", "citation_ref = -1", "-1 / omitted", "or -1 / omitted"} {
			if strings.Contains(surface.text, forbidden) {
				t.Fatalf("%s should not teach no-citation sentinel %q:\n%s", surface.name, forbidden, surface.text)
			}
		}
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

func TestAnswerDocumentMutationRepair_MapsDeterministicPatchRejects(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantCode   string
		wantFields []string
		wantHint   string
	}{
		{
			name:       "citation mode conflict",
			err:        fmt.Errorf("patch: replace_citations and append_citations are mutually exclusive (contract invariant 5); set exactly one"),
			wantCode:   "answer_doc_patch_citation_mode_conflict",
			wantFields: []string{"replace_citations", "append_citations"},
			wantHint:   "exactly one citation-pool operation",
		},
		{
			name:       "existing block added",
			err:        fmt.Errorf("patch: add_blocks[%q] already exists in previous emit (use replace_blocks to modify)", "s1"),
			wantCode:   "answer_doc_patch_existing_block",
			wantFields: []string{"add_blocks", "replace_blocks", "unchanged_block_ids"},
			wantHint:   "already exists",
		},
		{
			name:       "replace citations preserves old cited block",
			err:        fmt.Errorf("patch: replace_citations cannot preserve citation-bearing block %q; replace/remove that block too, use append_citations, or re-emit a full emit_answer_document so every citation_ref is renumbered against the new pool", "list1"),
			wantCode:   "answer_doc_patch_replace_citations_with_preserved_blocks",
			wantFields: []string{"replace_citations", "append_citations", "replace_blocks", "remove_block_ids"},
			wantHint:   "citation_ref",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repair := answerDocumentMutationRepair(tt.err)
			if repair == nil {
				t.Fatalf("expected structured repair for %v", tt.err)
			}
			if repair.Code != tt.wantCode {
				t.Fatalf("repair code = %q, want %q", repair.Code, tt.wantCode)
			}
			for _, field := range tt.wantFields {
				if !mutationRepairStringSliceContains(repair.Fields, field) {
					t.Fatalf("repair fields missing %q: %+v", field, repair.Fields)
				}
			}
			if !strings.Contains(repair.Hint, tt.wantHint) {
				t.Fatalf("repair hint %q does not contain %q", repair.Hint, tt.wantHint)
			}
		})
	}
}

func TestAnswerDocumentMutationRepair_UnknownErrorStaysUnstructured(t *testing.T) {
	if repair := answerDocumentMutationRepair(fmt.Errorf("patch: unrelated validation failure")); repair != nil {
		t.Fatalf("unknown mutation error should not fabricate repair metadata: %+v", repair)
	}
}

func mutationRepairStringSliceContains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

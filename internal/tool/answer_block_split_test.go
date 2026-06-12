package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// fullyFusedBlock returns a fused table+diagram raw block with EVERY
// emitAnswerBlockV2 field populated — the reflection partition test
// below fails when a future field addition is not explicitly homed
// on one half of the split.
func fullyFusedBlock() emitAnswerBlockV2 {
	return emitAnswerBlockV2{
		ID:                      "t1",
		Kind:                    string(types.BlockTable),
		Title:                   "各阶段输入输出",
		Text:                    "lead",
		Caveat:                  "",
		ErrorGranularityVerdict: "",
		CurrentStatusVerdict:    "",
		ScopeDisclosure:         "",
		Columns:                 []string{"stage", "输入", "输出", "载体"},
		Items: []emitAnswerBlockItemV2{
			{ID: "r1", Label: "StageAnalyze", Cells: []string{"请求", "AnalysisIR", "MutableState"}},
		},
		Diagram: &emitAnswerDiagramV2{
			Kind: string(types.DiagramSequence), Language: "mermaid", Body: "sequenceDiagram\n  A->>B: x",
		},
		ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact, FacetID: "f1"}},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge}},
		FacetIDs:    []string{"current_code_path"},
		SurfaceRole: string(types.SurfacePrincipal),
	}
}

// The forensic shape (read_combo_pipeline_sequence_table 2026-06-12):
// one block fusing kind=table rows with a diagram payload must split
// into adjacent table + diagram halves, both normalizable, with the
// declared kind surviving on the visible half.
func TestSplitFusedDiagramBlocksForensicShape(t *testing.T) {
	in := []emitAnswerBlockV2{
		{ID: "s1", Kind: string(types.BlockSummary), Text: "prose"},
		fullyFusedBlock(),
	}
	out := splitFusedDiagramBlocks("test", in)
	if len(out) != 3 {
		t.Fatalf("expected 3 blocks after split, got %d", len(out))
	}
	if out[0].raw.ID != "s1" || out[0].modelIndex != 0 {
		t.Fatalf("untouched block must stay first with its own model index")
	}
	// Both halves carry the SOURCE block's model index (the fused
	// block was the model's blocks[1]).
	if out[1].modelIndex != 1 || out[2].modelIndex != 1 {
		t.Fatalf("split halves must inherit the source model index 1, got %d/%d",
			out[1].modelIndex, out[2].modelIndex)
	}
	visible, diagramHalf := out[1].raw, out[2].raw
	if visible.Kind != string(types.BlockTable) || visible.ID != "t1" {
		t.Fatalf("visible half lost identity: kind=%s id=%s", visible.Kind, visible.ID)
	}
	if visible.Diagram != nil || len(visible.EdgeAnchors) != 0 {
		t.Fatalf("visible half must not carry the diagram payload")
	}
	if len(visible.Items) != 1 || len(visible.Columns) != 4 {
		t.Fatalf("visible half lost rows: items=%d columns=%d", len(visible.Items), len(visible.Columns))
	}
	if diagramHalf.Kind != string(types.BlockDiagram) || diagramHalf.Diagram == nil || len(diagramHalf.EdgeAnchors) != 1 {
		t.Fatalf("diagram half malformed: %+v", diagramHalf)
	}
	if diagramHalf.ID == visible.ID || diagramHalf.ID == "" {
		t.Fatalf("diagram half needs a derived unique id, got %q", diagramHalf.ID)
	}

	// Both halves must pass the normalizer, and the table half must
	// KEEP kind=table (no discriminator overwrite once Diagram=nil).
	tblTyped, err := NormalizeEmitAnswerBlock(visible, "blocks[1]")
	if err != nil {
		t.Fatalf("visible half failed normalize: %v", err)
	}
	if tblTyped.Kind != types.BlockTable {
		t.Fatalf("table kind overwritten after split: %s", tblTyped.Kind)
	}
	if _, err := NormalizeEmitAnswerBlock(diagramHalf, "blocks[2]"); err != nil {
		t.Fatalf("diagram half failed normalize: %v", err)
	}
}

// Reflection partition lock: every emitAnswerBlockV2 field populated
// on a fused block must surface on EXACTLY one half (ID/Kind exist on
// both by construction). A future field addition fails here until the
// split explicitly homes it.
func TestSplitFusedDiagramBlocksFieldPartition(t *testing.T) {
	in := fullyFusedBlock()
	out := splitFusedDiagramBlocks("test", []emitAnswerBlockV2{in})
	if len(out) != 2 {
		t.Fatalf("expected 2 halves, got %d", len(out))
	}
	visible, diagramHalf := reflect.ValueOf(out[0].raw), reflect.ValueOf(out[1].raw)
	src := reflect.ValueOf(in)
	typ := src.Type()
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if src.Field(i).IsZero() {
			continue // unpopulated on the fixture; nothing to partition
		}
		onVisible := !visible.Field(i).IsZero()
		onDiagram := !diagramHalf.Field(i).IsZero()
		switch name {
		case "ID", "Kind":
			if !onVisible || !onDiagram {
				t.Errorf("%s must exist on both halves", name)
			}
		case "Diagram", "EdgeAnchors":
			if onVisible || !onDiagram {
				t.Errorf("%s must live on the diagram half only (visible=%t diagram=%t)", name, onVisible, onDiagram)
			}
		default:
			if !onVisible || onDiagram {
				t.Errorf("%s must live on the visible half only (visible=%t diagram=%t)", name, onVisible, onDiagram)
			}
		}
	}
}

// Negative space: anything that is not the precise fused shape passes
// through byte-identical.
func TestSplitFusedDiagramBlocksNoOps(t *testing.T) {
	cases := []struct {
		name string
		b    emitAnswerBlockV2
	}{
		{"kind=diagram", emitAnswerBlockV2{ID: "d1", Kind: string(types.BlockDiagram), Items: []emitAnswerBlockItemV2{{Label: "x"}}, Diagram: &emitAnswerDiagramV2{Kind: "flow", Body: "flowchart LR\n A-->B"}}},
		{"empty diagram body", emitAnswerBlockV2{ID: "t1", Kind: string(types.BlockTable), Items: []emitAnswerBlockItemV2{{Label: "x"}}, Diagram: &emitAnswerDiagramV2{Kind: "flow", Body: "  "}}},
		{"no rows", emitAnswerBlockV2{ID: "s1", Kind: string(types.BlockSummary), Text: "prose", Diagram: &emitAnswerDiagramV2{Kind: "flow", Body: "flowchart LR\n A-->B"}}},
		{"invalid kind", emitAnswerBlockV2{ID: "x1", Kind: "bogus", Items: []emitAnswerBlockItemV2{{Label: "x"}}, Diagram: &emitAnswerDiagramV2{Kind: "flow", Body: "flowchart LR\n A-->B"}}},
		{"no diagram", emitAnswerBlockV2{ID: "t1", Kind: string(types.BlockTable), Items: []emitAnswerBlockItemV2{{Label: "x"}}}},
	}
	for _, tc := range cases {
		out := splitFusedDiagramBlocks("test", []emitAnswerBlockV2{tc.b})
		if len(out) != 1 {
			t.Errorf("%s: expected pass-through, got %d blocks", tc.name, len(out))
			continue
		}
		if !reflect.DeepEqual(out[0].raw, tc.b) {
			t.Errorf("%s: block mutated on pass-through", tc.name)
		}
	}
}

// Patch ops: a fused REPLACE entry keeps the visible half in
// replace_blocks (one block per replaced id); a fused ADD entry keeps
// its slot as the visible half. ALL system-derived diagram halves
// land in add_blocks AFTER the model's own add entries so
// add_blocks[i] error indices stay the model's.
func TestSplitFusedDiagramPatchBlocks(t *testing.T) {
	fusedReplace := fullyFusedBlock()
	fusedAdd := fullyFusedBlock()
	fusedAdd.ID = "t2"
	replaceOut, addOut := splitFusedDiagramPatchBlocks("test", maxBlocksPerDoc, nil, nil, nil,
		[]emitAnswerBlockV2{fusedReplace},
		[]emitAnswerBlockV2{{ID: "c1", Kind: string(types.BlockCaveat), Text: "warn"}, fusedAdd},
	)
	if len(replaceOut) != 1 || replaceOut[0].ID != "t1" || replaceOut[0].Diagram != nil {
		t.Fatalf("replace list must hold only the visible half: %+v", replaceOut)
	}
	if len(addOut) != 4 {
		t.Fatalf("add list must gain replace's diagram half + split add (got %d)", len(addOut))
	}
	ids := map[string]int{}
	for _, b := range addOut {
		ids[b.ID]++
	}
	for id, n := range ids {
		if n > 1 {
			t.Fatalf("duplicate id %q in add list", id)
		}
	}
	diagrams := 0
	for _, b := range addOut {
		if b.Kind == string(types.BlockDiagram) && b.Diagram != nil {
			diagrams++
		}
	}
	if diagrams != 2 {
		t.Fatalf("expected 2 diagram halves in add list, got %d", diagrams)
	}
}

// nearCapBlocks builds an n-block emit with fused table+diagram
// blocks at the given indexes and plain summary blocks elsewhere.
func nearCapBlocks(n int, fusedAt map[int]bool) []emitAnswerBlockV2 {
	out := make([]emitAnswerBlockV2, 0, n)
	for i := 0; i < n; i++ {
		if fusedAt[i] {
			b := fullyFusedBlock()
			b.ID = fmt.Sprintf("f%d", i)
			out = append(out, b)
			continue
		}
		out = append(out, emitAnswerBlockV2{
			ID: fmt.Sprintf("b%d", i), Kind: string(types.BlockSummary), Text: "prose",
		})
	}
	return out
}

// TestSplitFusedDiagramBlocks_AtCapPassesThroughUnsplit pins the cap
// fallback: a maxBlocksPerDoc-sized emit containing a fused block —
// previously accepted (lossily) — must NOT be split past the cap into
// a hard reject naming a block count the model never emitted. The
// fused block passes through with its payload intact and takes the
// discriminator-repair (lossy-accept) path, position-independent.
func TestSplitFusedDiagramBlocks_AtCapPassesThroughUnsplit(t *testing.T) {
	in := nearCapBlocks(maxBlocksPerDoc, map[int]bool{0: true})
	out := splitFusedDiagramBlocks("test", in)
	if len(out) != maxBlocksPerDoc {
		t.Fatalf("at-cap emit must stay at %d blocks after split pass, got %d (split past the cap → fabricated hard reject)",
			maxBlocksPerDoc, len(out))
	}
	if out[0].raw.Diagram == nil {
		t.Fatal("fused block must pass through with its diagram payload intact (lossy-accept path), not be stripped")
	}
	// End to end through the same hard gate that rejected: the typed
	// doc built from the split output must be accepted.
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	for _, entry := range out {
		blk, err := NormalizeEmitAnswerBlock(entry.raw, fmt.Sprintf("blocks[%d]", entry.modelIndex))
		if err != nil {
			t.Fatalf("blocks[%d] failed normalize: %v", entry.modelIndex, err)
		}
		doc.Blocks = append(doc.Blocks, blk)
	}
	if err := validateMergedV2Doc(doc); err != nil {
		t.Fatalf("at-cap emit with a fused block must stay accepted: %v", err)
	}
}

// One below the cap, the split still runs — the budget only suppresses
// splits that would not fit.
func TestSplitFusedDiagramBlocks_OneBelowCapStillSplits(t *testing.T) {
	in := nearCapBlocks(maxBlocksPerDoc-1, map[int]bool{0: true})
	out := splitFusedDiagramBlocks("test", in)
	if len(out) != maxBlocksPerDoc {
		t.Fatalf("one-below-cap emit should split to exactly %d blocks, got %d", maxBlocksPerDoc, len(out))
	}
	if out[0].raw.Diagram != nil || out[1].raw.Kind != string(types.BlockDiagram) || out[1].raw.Diagram == nil {
		t.Fatalf("split should have occurred: out[0].Diagram=%v out[1]=%+v", out[0].raw.Diagram, out[1].raw)
	}
}

// With more fused blocks than budget, exactly budget-many split and
// the rest pass through — the final count lands on the cap, never
// over it.
func TestSplitFusedDiagramBlocks_BudgetSplitsOnlyWhatFits(t *testing.T) {
	in := nearCapBlocks(maxBlocksPerDoc-2, map[int]bool{0: true, 1: true, 2: true})
	out := splitFusedDiagramBlocks("test", in)
	if len(out) != maxBlocksPerDoc {
		t.Fatalf("expected exactly %d blocks (budget-bounded), got %d", maxBlocksPerDoc, len(out))
	}
	splitHalves, passedThrough := 0, 0
	for _, entry := range out {
		if entry.raw.Kind == string(types.BlockDiagram) && entry.raw.Diagram != nil {
			splitHalves++
		}
		if entry.raw.Kind == string(types.BlockTable) && entry.raw.Diagram != nil {
			passedThrough++
		}
	}
	if splitHalves != 2 || passedThrough != 1 {
		t.Fatalf("budget=2 should split 2 and pass 1 through; split=%d passthrough=%d", splitHalves, passedThrough)
	}
}

// prevDocWithBlocks builds a prev emit with n summary blocks p0..pN-1.
func prevDocWithBlocks(n int) *types.AnswerDocumentV2 {
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	for i := 0; i < n; i++ {
		doc.Blocks = append(doc.Blocks, types.AnswerBlock{
			ID: fmt.Sprintf("p%d", i), Kind: types.BlockSummary, Text: "prose",
		})
	}
	return doc
}

// mergeFusedPatchForTest mirrors the production patch sequence from
// emit_answer_document_patch.go Execute: split → typed conversion →
// id-surface normalization (trim + identical-duplicate dedup) →
// prev-id tolerance → Apply → merged-doc hard gate. Returns the
// merged document and the hard gate's verdict.
func mergeFusedPatchForTest(t *testing.T, prev *types.AnswerDocumentV2, removeIDs, unchangedIDs []string, replaceBlocks, addBlocks []emitAnswerBlockV2) (*types.AnswerDocumentV2, error) {
	t.Helper()
	replaceBlocks, addBlocks = splitFusedDiagramPatchBlocks("test",
		fusedPatchSplitBudget(prev, removeIDs, replaceBlocks, addBlocks),
		prev, removeIDs, unchangedIDs,
		replaceBlocks, addBlocks)
	patch := &types.AnswerDocumentV2Patch{
		RemoveBlockIDs:    append([]string(nil), removeIDs...),
		UnchangedBlockIDs: append([]string(nil), unchangedIDs...),
	}
	if len(replaceBlocks) > 0 {
		converted, err := convertEmitBlocksToTyped("test", replaceBlocks, "replace_blocks")
		if err != nil {
			t.Fatalf("replace conversion failed: %v", err)
		}
		patch.ReplaceBlocks = converted
	}
	if len(addBlocks) > 0 {
		converted, err := convertEmitBlocksToTyped("test", addBlocks, "add_blocks")
		if err != nil {
			t.Fatalf("add conversion failed: %v", err)
		}
		patch.AddBlocks = converted
	}
	normalizeAnswerDocumentPatchIDSurface(patch)
	normalizeAnswerDocumentPatchBlockOps(prev, patch)
	merged, err := types.ApplyAnswerDocumentV2Patch(prev, patch)
	if err != nil {
		return nil, err
	}
	return merged, validateMergedV2Doc(merged)
}

// mergedBlockByID returns the block with the given id, or nil.
func mergedBlockByID(doc *types.AnswerDocumentV2, id string) *types.AnswerBlock {
	if doc == nil {
		return nil
	}
	for i := range doc.Blocks {
		if doc.Blocks[i].ID == id {
			return &doc.Blocks[i]
		}
	}
	return nil
}

// TestSplitFusedDiagramPatchBlocks_AtCapPassesThroughUnsplit pins the
// patch-path cap fallback: prev at the cap + a fused REPLACE entry
// (count-neutral as emitted) must stay accepted — splitting would
// append a system-generated diagram block and push the merged doc
// over maxBlocksPerDoc, hard-rejecting with a count the model never
// emitted.
func TestSplitFusedDiagramPatchBlocks_AtCapPassesThroughUnsplit(t *testing.T) {
	prev := prevDocWithBlocks(maxBlocksPerDoc)
	fusedReplace := fullyFusedBlock()
	fusedReplace.ID = "p0"
	if _, err := mergeFusedPatchForTest(t, prev, nil, nil, []emitAnswerBlockV2{fusedReplace}, nil); err != nil {
		t.Fatalf("at-cap patch with a fused replace must stay accepted (pass-through, lossy repair): %v", err)
	}
}

// One below the cap the patch split still runs and the merged doc
// lands exactly on the cap.
func TestSplitFusedDiagramPatchBlocks_OneBelowCapStillSplits(t *testing.T) {
	prev := prevDocWithBlocks(maxBlocksPerDoc - 1)
	fusedReplace := fullyFusedBlock()
	fusedReplace.ID = "p0"
	replaceOut, addOut := splitFusedDiagramPatchBlocks("test",
		fusedPatchSplitBudget(prev, nil, []emitAnswerBlockV2{fusedReplace}, nil),
		prev, nil, nil,
		[]emitAnswerBlockV2{fusedReplace}, nil)
	if len(replaceOut) != 1 || replaceOut[0].Diagram != nil {
		t.Fatalf("replace half should be visible-only after split: %+v", replaceOut)
	}
	if len(addOut) != 1 || addOut[0].Kind != string(types.BlockDiagram) {
		t.Fatalf("diagram half should land in add_blocks: %+v", addOut)
	}
	if _, err := mergeFusedPatchForTest(t, prev, nil, nil, []emitAnswerBlockV2{fusedReplace}, nil); err != nil {
		t.Fatalf("one-below-cap patch should merge cleanly at the cap: %v", err)
	}
}

// A remove in the same patch frees budget: prev at cap + remove one
// block + fused replace → split fits exactly.
func TestSplitFusedDiagramPatchBlocks_RemoveFreesBudget(t *testing.T) {
	prev := prevDocWithBlocks(maxBlocksPerDoc)
	fusedReplace := fullyFusedBlock()
	fusedReplace.ID = "p0"
	budget := fusedPatchSplitBudget(prev, []string{"p1"}, []emitAnswerBlockV2{fusedReplace}, nil)
	if budget != 1 {
		t.Fatalf("remove of one prev block should free exactly 1 split, got budget=%d", budget)
	}
	if _, err := mergeFusedPatchForTest(t, prev, []string{"p1"}, nil, []emitAnswerBlockV2{fusedReplace}, nil); err != nil {
		t.Fatalf("patch with freed budget must merge cleanly: %v", err)
	}
}

// Model-authored adds consume budget before system splits may: prev
// one below cap + one new add + fused replace → no split headroom.
func TestSplitFusedDiagramPatchBlocks_AddsConsumeBudget(t *testing.T) {
	prev := prevDocWithBlocks(maxBlocksPerDoc - 1)
	fusedReplace := fullyFusedBlock()
	fusedReplace.ID = "p0"
	newAdd := emitAnswerBlockV2{ID: "n1", Kind: string(types.BlockSummary), Text: "new"}
	if budget := fusedPatchSplitBudget(prev, nil, []emitAnswerBlockV2{fusedReplace}, []emitAnswerBlockV2{newAdd}); budget != 0 {
		t.Fatalf("model add should consume the last slot, got budget=%d", budget)
	}
	if _, err := mergeFusedPatchForTest(t, prev, nil, nil, []emitAnswerBlockV2{fusedReplace}, []emitAnswerBlockV2{newAdd}); err != nil {
		t.Fatalf("at-cap-after-add patch must stay accepted via pass-through: %v", err)
	}
}

// TestSplitFusedDiagramPatchBlocks_PrevNonDiagramNeverClobbered pins
// the collision discipline's hardest case: prev contains a
// model-authored NON-diagram block whose id happens to equal the
// would-be derived id. The derivation must suffix-number past it —
// pre-fix the half collided, the add→replace tolerance demoted it,
// and the prose block became kind=diagram with its text gone.
func TestSplitFusedDiagramPatchBlocks_PrevNonDiagramNeverClobbered(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "p0", Kind: types.BlockTable, Title: "tbl"},
		{ID: "p0_diagram", Kind: types.BlockSummary, Text: "IMPORTANT MODEL PROSE"},
	}}
	fused := fullyFusedBlock()
	fused.ID = "p0"
	merged, err := mergeFusedPatchForTest(t, prev, nil, nil, []emitAnswerBlockV2{fused}, nil)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	prose := mergedBlockByID(merged, "p0_diagram")
	if prose == nil || prose.Kind != types.BlockSummary || prose.Text != "IMPORTANT MODEL PROSE" {
		t.Fatalf("model-authored non-diagram block clobbered by a derived id: %+v", prose)
	}
	// The half landed under a suffixed fresh id as a true add.
	suffixed := mergedBlockByID(merged, "p0_diagram2")
	if suffixed == nil || suffixed.Kind != types.BlockDiagram || suffixed.Diagram == nil {
		t.Fatalf("derived half should land under the suffixed id as a fresh add: %+v", suffixed)
	}
	// And the visible half kept its rows.
	if tbl := mergedBlockByID(merged, "p0"); tbl == nil || tbl.Kind != types.BlockTable || len(tbl.Items) != 1 {
		t.Fatalf("visible half lost identity or rows: %+v", tbl)
	}
}

// TestSplitFusedDiagramPatchBlocks_UnchangedDiagramHalfStaysByteIdentical
// pins R16 for the derived-id path: a prev diagram half the model
// explicitly lists in unchanged_block_ids must flow through
// byte-identical — the derivation suffix-numbers past the claimed id
// instead of refreshing it.
func TestSplitFusedDiagramPatchBlocks_UnchangedDiagramHalfStaysByteIdentical(t *testing.T) {
	stale := &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n KEEP-->ME"}
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "t1", Kind: types.BlockTable, Title: "tbl"},
		{ID: "t1_diagram", Kind: types.BlockDiagram, Diagram: stale},
	}}
	fused := fullyFusedBlock() // id t1, carries a DIFFERENT diagram body
	merged, err := mergeFusedPatchForTest(t, prev, nil, []string{"t1_diagram"}, []emitAnswerBlockV2{fused}, nil)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	kept := mergedBlockByID(merged, "t1_diagram")
	if kept == nil || kept.Diagram == nil || kept.Diagram.Body != "flowchart LR\n KEEP-->ME" {
		t.Fatalf("unchanged-declared diagram half was not preserved byte-identical: %+v", kept)
	}
	// The fresh payload landed under a suffixed id instead.
	if fresh := mergedBlockByID(merged, "t1_diagram2"); fresh == nil || fresh.Diagram == nil {
		t.Fatalf("fresh diagram payload should land under a suffixed id: %+v", fresh)
	}
}

// TestSplitFusedDiagramPatchBlocks_RemoveExecutes pins that a model
// remove of a prior diagram half is never absorbed by a derived-id
// collision: the remove executes, and the new half lands under a
// fresh suffixed id. Pre-fix the tolerance converted the remove+add
// pair into a replace and silently dropped the remove op.
func TestSplitFusedDiagramPatchBlocks_RemoveExecutes(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "t1", Kind: types.BlockTable, Title: "tbl"},
		{ID: "t1_diagram", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n OLD-->GONE"}},
	}}
	fused := fullyFusedBlock() // id t1
	merged, err := mergeFusedPatchForTest(t, prev, []string{"t1_diagram"}, nil, []emitAnswerBlockV2{fused}, nil)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if gone := mergedBlockByID(merged, "t1_diagram"); gone != nil {
		t.Fatalf("model's remove op was absorbed — t1_diagram still present: %+v", gone)
	}
	if fresh := mergedBlockByID(merged, "t1_diagram2"); fresh == nil || fresh.Kind != types.BlockDiagram {
		t.Fatalf("new half should land under a fresh suffixed id: %+v", fresh)
	}
}

// TestSplitFusedDiagramPatchBlocks_AtCapRefreshRunsBudgetFree pins
// the count-neutral exemption: prev at the cap already holding the
// prior split's stale diagram half; a fresh fused replace of the
// source block must split budget-free (derived id collides with the
// unclaimed half → demoted to a replace → merged count unchanged),
// refreshing the diagram and keeping the model's rows. Pre-fix the
// budget projected +1, suppressed the split, the discriminator
// repair destroyed the rows, and the stale diagram survived.
func TestSplitFusedDiagramPatchBlocks_AtCapRefreshRunsBudgetFree(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2"}
	prev.Blocks = append(prev.Blocks,
		types.AnswerBlock{ID: "t1", Kind: types.BlockTable, Title: "tbl"},
		types.AnswerBlock{ID: "t1_diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n STALE-->OLD"}})
	for i := 2; i < maxBlocksPerDoc; i++ {
		prev.Blocks = append(prev.Blocks, types.AnswerBlock{
			ID: fmt.Sprintf("p%d", i), Kind: types.BlockSummary, Text: "x"})
	}
	fused := fullyFusedBlock() // id t1: table rows + fresh sequence diagram
	merged, err := mergeFusedPatchForTest(t, prev, nil, nil, []emitAnswerBlockV2{fused}, nil)
	if err != nil {
		t.Fatalf("at-cap refresh must merge cleanly: %v", err)
	}
	if got := len(merged.Blocks); got != maxBlocksPerDoc {
		t.Fatalf("refresh must be count-neutral: %d blocks, want %d", got, maxBlocksPerDoc)
	}
	tbl := mergedBlockByID(merged, "t1")
	if tbl == nil || tbl.Kind != types.BlockTable || len(tbl.Items) != 1 {
		t.Fatalf("model's refreshed rows destroyed at cap: %+v", tbl)
	}
	dg := mergedBlockByID(merged, "t1_diagram")
	if dg == nil || dg.Diagram == nil || strings.Contains(dg.Diagram.Body, "STALE") {
		t.Fatalf("stale diagram half not refreshed: %+v", dg)
	}
}

// TestSplitFusedDiagramPatchBlocks_IdenticalDuplicatesNoStraddle pins
// "identical duplicates get identical treatment": a model stutter-emit
// of two byte-identical fused replaces at the cap must NOT be split
// for one copy and passed through for the other — that de-identicalizes
// the pair, defeats the downstream identical-duplicate dedup, and
// fabricates a 'replace_blocks duplicated' reject for a patch that
// merges cleanly as emitted.
func TestSplitFusedDiagramPatchBlocks_IdenticalDuplicatesNoStraddle(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2"}
	prev.Blocks = append(prev.Blocks,
		types.AnswerBlock{ID: "t1", Kind: types.BlockTable, Title: "tbl"},
		types.AnswerBlock{ID: "t1_diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n STALE-->OLD"}})
	for i := 2; i < maxBlocksPerDoc; i++ {
		prev.Blocks = append(prev.Blocks, types.AnswerBlock{
			ID: fmt.Sprintf("p%d", i), Kind: types.BlockSummary, Text: "x"})
	}
	fused := fullyFusedBlock() // id t1
	merged, err := mergeFusedPatchForTest(t, prev, nil, nil,
		[]emitAnswerBlockV2{fused, fused}, nil) // byte-identical stutter pair
	if err != nil {
		t.Fatalf("stutter-emitted identical fused replaces must merge cleanly: %v", err)
	}
	if got := len(merged.Blocks); got != maxBlocksPerDoc {
		t.Fatalf("refresh must stay count-neutral with duplicates: %d blocks, want %d", got, maxBlocksPerDoc)
	}
	if tbl := mergedBlockByID(merged, "t1"); tbl == nil || tbl.Kind != types.BlockTable {
		t.Fatalf("rows destroyed on the duplicate pair: %+v", tbl)
	}
	if dg := mergedBlockByID(merged, "t1_diagram"); dg == nil || dg.Diagram == nil || strings.Contains(dg.Diagram.Body, "STALE") {
		t.Fatalf("stale half not refreshed exactly once: %+v", dg)
	}
}

// TestSplitFusedDiagramBlocks_IdenticalDuplicatesNoStraddle is the
// full-emit mirror: 63-block emit with two identical fused blocks and
// budget 1 — pre-fix the first split and the second passed through,
// de-identicalizing the pair past the duplicate dedup into a
// 'duplicate id' hard reject. Both copies must get the split
// treatment (duplicate emits the identical visible half only), so the
// dedup collapses them and the doc passes the gate.
func TestSplitFusedDiagramBlocks_IdenticalDuplicatesNoStraddle(t *testing.T) {
	in := make([]emitAnswerBlockV2, 0, maxBlocksPerDoc-1)
	f := fullyFusedBlock()
	in = append(in, f, f) // byte-identical stutter pair
	for i := 2; i < maxBlocksPerDoc-1; i++ {
		in = append(in, emitAnswerBlockV2{
			ID: fmt.Sprintf("b%d", i), Kind: string(types.BlockSummary), Text: "x"})
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	for _, entry := range splitFusedDiagramBlocks("test", in) {
		blk, err := NormalizeEmitAnswerBlock(entry.raw, fmt.Sprintf("blocks[%d]", entry.modelIndex))
		if err != nil {
			t.Fatalf("blocks[%d]: %v", entry.modelIndex, err)
		}
		doc.Blocks = append(doc.Blocks, blk)
	}
	// Production runs the identical-duplicate dedup before the gate.
	normalizeAnswerDocumentBlockIDSurface(doc)
	if err := validateMergedV2Doc(doc); err != nil {
		t.Fatalf("identical fused duplicates must not fabricate a reject: %v", err)
	}
	if tbl := mergedBlockByID(doc, "t1"); tbl == nil || tbl.Kind != types.BlockTable {
		t.Fatalf("visible half lost: %+v", tbl)
	}
	if dg := mergedBlockByID(doc, "t1_diagram"); dg == nil || dg.Diagram == nil {
		t.Fatalf("diagram half lost: %+v", dg)
	}
}

// TestSplitFusedDiagramPatchBlocks_SuffixExhaustionPassesThrough pins
// the exhaustion guard: when every <base>_diagram[N] candidate up to
// the suffix bound is claimed, the split must fall back to the
// lossy-accept pass-through — committing the still-used _diagram99
// candidate fabricated an 'add_blocks duplicated' reject for ids the
// model never emitted.
func TestSplitFusedDiagramPatchBlocks_SuffixExhaustionPassesThrough(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2"}
	prev.Blocks = append(prev.Blocks, types.AnswerBlock{ID: "t1", Kind: types.BlockTable, Title: "tbl"})
	removeIDs := []string{"t1_diagram"}
	prev.Blocks = append(prev.Blocks, types.AnswerBlock{ID: "t1_diagram", Kind: types.BlockSummary, Text: "x"})
	for i := 2; i <= 62; i++ {
		id := fmt.Sprintf("t1_diagram%d", i)
		prev.Blocks = append(prev.Blocks, types.AnswerBlock{ID: id, Kind: types.BlockSummary, Text: "x"})
		removeIDs = append(removeIDs, id)
	}
	adds := make([]emitAnswerBlockV2, 0, 37)
	for i := 63; i <= 99; i++ {
		adds = append(adds, emitAnswerBlockV2{
			ID: fmt.Sprintf("t1_diagram%d", i), Kind: string(types.BlockSummary), Text: "new"})
	}
	fused := fullyFusedBlock() // id t1 — every derived candidate is claimed
	merged, err := mergeFusedPatchForTest(t, prev, removeIDs, nil,
		[]emitAnswerBlockV2{fused}, adds)
	if err != nil {
		t.Fatalf("suffix exhaustion must degrade to pass-through, not a fabricated reject: %v", err)
	}
	// The fused entry passed through whole (discriminator repair).
	if blk := mergedBlockByID(merged, "t1"); blk == nil || blk.Kind != types.BlockDiagram {
		t.Fatalf("exhausted split should pass through to the lossy-accept path: %+v", blk)
	}
}

// TestSplitFusedDiagramPatchBlocks_RefreshFindsSuffixedHalf pins the
// chain-scan refresh: after an earlier turn's remove/unchanged claim
// landed the persisted half at a SUFFIXED id (t1_diagram2), the next
// re-fuse must refresh THAT block — pre-fix it minted t1_diagram as a
// fresh id, stranding the persisted half as a stale duplicate and, at
// the cap, re-opening the budget-starved row loss.
func TestSplitFusedDiagramPatchBlocks_RefreshFindsSuffixedHalf(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2"}
	prev.Blocks = append(prev.Blocks,
		types.AnswerBlock{ID: "t1", Kind: types.BlockTable, Title: "tbl"},
		// The prior turn's half landed at the suffixed id; no
		// t1_diagram block exists.
		types.AnswerBlock{ID: "t1_diagram2", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n STALE-->OLD"}})
	for i := 2; i < maxBlocksPerDoc; i++ {
		prev.Blocks = append(prev.Blocks, types.AnswerBlock{
			ID: fmt.Sprintf("p%d", i), Kind: types.BlockSummary, Text: "x"})
	}
	fused := fullyFusedBlock() // id t1
	merged, err := mergeFusedPatchForTest(t, prev, nil, nil, []emitAnswerBlockV2{fused}, nil)
	if err != nil {
		t.Fatalf("at-cap suffixed refresh must merge cleanly: %v", err)
	}
	if got := len(merged.Blocks); got != maxBlocksPerDoc {
		t.Fatalf("suffixed refresh must be count-neutral: %d blocks, want %d", got, maxBlocksPerDoc)
	}
	if tbl := mergedBlockByID(merged, "t1"); tbl == nil || tbl.Kind != types.BlockTable || len(tbl.Items) != 1 {
		t.Fatalf("rows destroyed despite available suffixed refresh target: %+v", tbl)
	}
	if dg := mergedBlockByID(merged, "t1_diagram2"); dg == nil || dg.Diagram == nil || strings.Contains(dg.Diagram.Body, "STALE") {
		t.Fatalf("suffixed persisted half not refreshed: %+v", dg)
	}
	if stray := mergedBlockByID(merged, "t1_diagram"); stray != nil {
		t.Fatalf("fresh t1_diagram minted instead of refreshing the suffixed half: %+v", stray)
	}
}

// fencedTwin returns a fused block normalization-identical to
// fullyFusedBlock() but byte-DIFFERENT: the diagram body is wrapped
// in ```mermaid fences that stripOuterDiagramFence erases. Each call
// allocates its own Diagram pointer — NormalizeEmitAnswerBlock
// normalizes payloads in place, so twins must never share one.
func fencedTwin() emitAnswerBlockV2 {
	b := fullyFusedBlock()
	b.Diagram = &emitAnswerDiagramV2{
		Kind: string(types.DiagramSequence), Language: "mermaid",
		Body: "```mermaid\nsequenceDiagram\n  A->>B: x\n```",
	}
	return b
}

// TestSplitFusedDiagramPatchBlocks_NormalizationErasedTwinsNoStraddle
// pins the canonical-identity memo: a stutter pair that is byte-
// DIFFERENT but identical after normalization (bare vs fence-wrapped
// diagram body) must get identical treatment at the refresh/budget
// boundary — the downstream dedup compares post-normalization typed
// blocks, so a raw-only memo let one copy split and the other pass
// through, fabricating a 'replace_blocks duplicated' reject for a
// patch that merges cleanly as emitted.
func TestSplitFusedDiagramPatchBlocks_NormalizationErasedTwinsNoStraddle(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2"}
	prev.Blocks = append(prev.Blocks,
		types.AnswerBlock{ID: "t1", Kind: types.BlockTable, Title: "tbl"},
		types.AnswerBlock{ID: "t1_diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n STALE-->OLD"}})
	for i := 2; i < maxBlocksPerDoc; i++ {
		prev.Blocks = append(prev.Blocks, types.AnswerBlock{
			ID: fmt.Sprintf("p%d", i), Kind: types.BlockSummary, Text: "x"})
	}
	merged, err := mergeFusedPatchForTest(t, prev, nil, nil,
		[]emitAnswerBlockV2{fullyFusedBlock(), fencedTwin()}, nil)
	if err != nil {
		t.Fatalf("normalization-erased twins must merge cleanly at the refresh boundary: %v", err)
	}
	if got := len(merged.Blocks); got != maxBlocksPerDoc {
		t.Fatalf("twin refresh must stay count-neutral: %d blocks, want %d", got, maxBlocksPerDoc)
	}
	if tbl := mergedBlockByID(merged, "t1"); tbl == nil || tbl.Kind != types.BlockTable {
		t.Fatalf("rows destroyed on the twin pair: %+v", tbl)
	}
}

// TestSplitFusedDiagramBlocks_NormalizationErasedTwinsNoStraddle is
// the full-emit mirror at the pure budget boundary (63 blocks,
// budget 1): pre-fix the bare copy split and the fenced copy passed
// through — 'merged doc: duplicate id' for an emit that dedups
// cleanly as emitted.
func TestSplitFusedDiagramBlocks_NormalizationErasedTwinsNoStraddle(t *testing.T) {
	in := make([]emitAnswerBlockV2, 0, maxBlocksPerDoc-1)
	in = append(in, fullyFusedBlock(), fencedTwin())
	for i := 2; i < maxBlocksPerDoc-1; i++ {
		in = append(in, emitAnswerBlockV2{
			ID: fmt.Sprintf("b%d", i), Kind: string(types.BlockSummary), Text: "x"})
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	for _, entry := range splitFusedDiagramBlocks("test", in) {
		blk, err := NormalizeEmitAnswerBlock(entry.raw, fmt.Sprintf("blocks[%d]", entry.modelIndex))
		if err != nil {
			t.Fatalf("blocks[%d]: %v", entry.modelIndex, err)
		}
		doc.Blocks = append(doc.Blocks, blk)
	}
	normalizeAnswerDocumentBlockIDSurface(doc)
	if err := validateMergedV2Doc(doc); err != nil {
		t.Fatalf("normalization-erased twins must not fabricate a reject: %v", err)
	}
}

// TestSplitFusedDiagramPatchBlocks_RefreshReachesDiagram99 pins the
// chain-scan/peek id-space parity: a persisted half at the LAST
// candidate (_diagram99) with every lower candidate claimed must
// still classify as a budget-free refresh — the pre-fix scan stopped
// at _diagram98 while the fresh-mint walk extends to _diagram99.
func TestSplitFusedDiagramPatchBlocks_RefreshReachesDiagram99(t *testing.T) {
	prev := &types.AnswerDocumentV2{DocumentModel: "v2"}
	prev.Blocks = append(prev.Blocks, types.AnswerBlock{ID: "t1", Kind: types.BlockTable, Title: "tbl"})
	// Claim _diagram and _diagram2.._diagram98 via prev non-diagram
	// blocks; leave _diagram99 as the only refreshable target.
	prev.Blocks = append(prev.Blocks, types.AnswerBlock{ID: "t1_diagram", Kind: types.BlockSummary, Text: "x"})
	for i := 2; i <= 98; i++ {
		prev.Blocks = append(prev.Blocks, types.AnswerBlock{
			ID: fmt.Sprintf("t1_diagram%d", i), Kind: types.BlockSummary, Text: "x"})
	}
	prev.Blocks = append(prev.Blocks, types.AnswerBlock{ID: "t1_diagram99", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n STALE-->OLD"}})
	fused := fullyFusedBlock() // id t1
	replaceOut, addOut := splitFusedDiagramPatchBlocks("test",
		0, // no fresh-id budget: only a refresh may split
		prev, nil, nil, []emitAnswerBlockV2{fused}, nil)
	if len(replaceOut) != 1 || replaceOut[0].Diagram != nil {
		t.Fatalf("refresh at _diagram99 should have split the entry: %+v", replaceOut)
	}
	if len(addOut) != 1 || addOut[0].ID != "t1_diagram99" || addOut[0].Kind != string(types.BlockDiagram) {
		t.Fatalf("half should refresh the _diagram99 target: %+v", addOut)
	}
}

// TestDecodeRecoveredAnswerDocument_AbsorbsIdenticalDuplicates pins
// the recovery caller's dedup: a byte-identical fused stutter pair
// must survive LOSSLESS text recovery — the split's duplicate memo
// relies on the identical-duplicate tolerance running before the
// merged-doc gate, which the recovery path previously skipped.
func TestDecodeRecoveredAnswerDocument_AbsorbsIdenticalDuplicates(t *testing.T) {
	f := fullyFusedBlock()
	raw, err := json.Marshal(map[string]any{"blocks": []emitAnswerBlockV2{f, f}})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := decodeRecoveredAnswerDocumentV2(raw, "test")
	if err != nil {
		t.Fatalf("identical fused stutter pair must survive lossless recovery: %v", err)
	}
	if tbl := mergedBlockByID(rec.Document, "t1"); tbl == nil || tbl.Kind != types.BlockTable {
		t.Fatalf("visible half lost in recovery: %+v", tbl)
	}
	if dg := mergedBlockByID(rec.Document, "t1_diagram"); dg == nil || dg.Diagram == nil {
		t.Fatalf("diagram half lost in recovery: %+v", dg)
	}
}

// TestSplitFusedDiagramBlocks_ErrorNamesModelIndex pins index
// attribution end-to-end through a real caller
// (decodeRecoveredAnswerDocumentV2): a validation failure on a model
// block that sits AFTER a split must be reported at the index the
// model emitted it at, not its system-shifted post-split position.
// Pre-fix this reported blocks[2] for the model's blocks[1].
func TestSplitFusedDiagramBlocks_ErrorNamesModelIndex(t *testing.T) {
	raw := json.RawMessage(`{"blocks":[
	  {"id":"t1","kind":"table","columns":["a"],"items":[{"id":"r1","label":"x"}],
	   "diagram":{"kind":"flow","language":"mermaid","body":"flowchart LR\n A-->B"}},
	  {"id":"b2","kind":"bogus","text":"x"}
	]}`)
	_, err := decodeRecoveredAnswerDocumentV2(raw, "test")
	if err == nil {
		t.Fatal("expected validation error for kind=bogus")
	}
	if !strings.HasPrefix(err.Error(), "blocks[1]:") {
		t.Fatalf("error must name the MODEL's index blocks[1], got: %v", err)
	}
}

// TestSplitFusedDiagramPatchBlocks_AddIndicesUnshifted pins the patch
// side: system-derived diagram halves (from BOTH fused replaces and
// fused adds) land AFTER the model's own add_blocks entries, so
// `add_blocks[i]` validation-error fieldPaths keep the model's
// indices. Pre-fix a replace-derived half prepended (shifting every
// model add by one) and an add-in-place split shifted later adds.
func TestSplitFusedDiagramPatchBlocks_AddIndicesUnshifted(t *testing.T) {
	fusedReplace := fullyFusedBlock() // id t1
	fusedAdd := fullyFusedBlock()
	fusedAdd.ID = "t2"
	modelAdd0 := emitAnswerBlockV2{ID: "a0", Kind: string(types.BlockSummary), Text: "first"}
	badModelAdd2 := emitAnswerBlockV2{ID: "a2", Kind: "bogus", Text: "broken"}

	replaceOut, addOut := splitFusedDiagramPatchBlocks("test", maxBlocksPerDoc, nil, nil, nil,
		[]emitAnswerBlockV2{fusedReplace},
		[]emitAnswerBlockV2{modelAdd0, fusedAdd, badModelAdd2},
	)
	if len(replaceOut) != 1 {
		t.Fatalf("replace list must stay 1:1: %+v", replaceOut)
	}
	// Model adds keep their emitted indices: a0@0, t2(visible)@1, a2@2;
	// the two system halves sit at indices >= 3.
	if addOut[0].ID != "a0" || addOut[1].ID != "t2" || addOut[2].ID != "a2" {
		t.Fatalf("model add entries shifted: %q %q %q", addOut[0].ID, addOut[1].ID, addOut[2].ID)
	}
	if len(addOut) != 5 {
		t.Fatalf("expected 3 model adds + 2 system halves, got %d", len(addOut))
	}
	for i := 3; i < len(addOut); i++ {
		if addOut[i].Kind != string(types.BlockDiagram) || addOut[i].Diagram == nil {
			t.Fatalf("tail entry %d must be a system diagram half: %+v", i, addOut[i])
		}
	}
	// The fieldPath the model sees for its own broken add names index 2.
	_, err := convertEmitBlocksToTyped("test", addOut, "add_blocks")
	if err == nil {
		t.Fatal("expected validation error for the bogus add")
	}
	if !strings.Contains(err.Error(), "add_blocks[2]:") {
		t.Fatalf("error must name the MODEL's add index 2, got: %v", err)
	}
}

// TestIsFusedDiagramBlock_FenceOnlyBodyPassesThrough pins mechanic 3:
// a fused-looking block whose diagram body is only a code fence
// (empty once stripped) must NOT split — the split would manufacture
// a system diagram half that fails "diagram.body is empty" on an
// entry the model never authored. Pass-through routes the same
// reject to the model's own blocks[0] via the discriminator repair.
func TestIsFusedDiagramBlock_FenceOnlyBodyPassesThrough(t *testing.T) {
	fenceOnly := fullyFusedBlock()
	fenceOnly.Diagram.Body = "```mermaid\n\n```"
	if isFusedDiagramBlock(fenceOnly) {
		t.Fatal("fence-only body must not qualify as a fused block")
	}
	out := splitFusedDiagramBlocks("test", []emitAnswerBlockV2{fenceOnly})
	if len(out) != 1 || out[0].modelIndex != 0 {
		t.Fatalf("fence-only block must pass through whole: %+v", out)
	}
	// End to end: the reject lands on the model's own index.
	raw, errM := json.Marshal(map[string]any{"blocks": []emitAnswerBlockV2{fenceOnly}})
	if errM != nil {
		t.Fatal(errM)
	}
	_, err := decodeRecoveredAnswerDocumentV2(raw, "test")
	if err == nil {
		t.Fatal("expected the empty-after-fences diagram reject")
	}
	if !strings.HasPrefix(err.Error(), "blocks[0]:") || !strings.Contains(err.Error(), "diagram.body is empty") {
		t.Fatalf("reject must land on the model's blocks[0] with the empty-body message, got: %v", err)
	}
}

// TestIsFusedDiagramBlock_NormalizeEmptiedBodyPassesThrough pins the
// stronger predicate congruence: the split judges body emptiness on
// the SAME normalization pipeline NormalizeEmitAnswerBlock applies,
// not just the fence strip. "graph codraxNode1[hidden]" survives the
// fence strip but mermaidcompat's hidden-marker pass deletes the
// line, emptying the body — splitting it would manufacture a system
// half failing "diagram.body is empty" at an add_blocks index the
// model never emitted (verified pre-fix: add_blocks[0] with ZERO
// model adds). The predicate must also not mutate the model's
// payload while probing.
func TestIsFusedDiagramBlock_NormalizeEmptiedBodyPassesThrough(t *testing.T) {
	markerOnly := fullyFusedBlock()
	markerOnly.Diagram.Body = "graph codraxNode1[hidden]"
	originalBody := markerOnly.Diagram.Body
	if isFusedDiagramBlock(markerOnly) {
		t.Fatal("body that normalizes to empty must not qualify as fused")
	}
	if markerOnly.Diagram.Body != originalBody {
		t.Fatalf("predicate probe mutated the model's payload: %q", markerOnly.Diagram.Body)
	}
	// Patch path: no system half is manufactured; the fused replace
	// passes through whole and the reject (discriminator repair →
	// empty body) lands on the model's own replace_blocks[0].
	replaceOut, addOut := splitFusedDiagramPatchBlocks("test", maxBlocksPerDoc, nil, nil, nil,
		[]emitAnswerBlockV2{markerOnly}, nil)
	if len(addOut) != 0 || len(replaceOut) != 1 || replaceOut[0].Diagram == nil {
		t.Fatalf("normalize-emptied fused block must pass through whole: replace=%+v add=%+v", replaceOut, addOut)
	}
	_, err := convertEmitBlocksToTyped("test", replaceOut, "replace_blocks")
	if err == nil {
		t.Fatal("expected the empty-after-normalization diagram reject")
	}
	if !strings.Contains(err.Error(), "replace_blocks[0]:") {
		t.Fatalf("reject must land on the model's own replace index, got: %v", err)
	}
	// Control: a genuinely renderable body still splits.
	renderable := fullyFusedBlock()
	if !isFusedDiagramBlock(renderable) {
		t.Fatal("control broken: renderable fused block must still split")
	}
}

// TestValidateMergedV2Doc_DuplicateIDMessageCarriesNoIndex pins the
// duplicate-id reject shape: the merged doc's physical layout
// includes system-inserted blocks, so a position would not map to
// the model's emission — the id is the handle, the index is dropped.
func TestValidateMergedV2Doc_DuplicateIDMessageCarriesNoIndex(t *testing.T) {
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "a", Kind: types.BlockSummary, Text: "1"},
		{ID: "x", Kind: types.BlockSection, Text: "2"},
		{ID: "x", Kind: types.BlockSection, Text: "3"},
	}}
	err := validateMergedV2Doc(doc)
	if err == nil {
		t.Fatal("expected duplicate-id reject")
	}
	if !strings.Contains(err.Error(), `duplicate id "x"`) {
		t.Fatalf("reject must name the id: %v", err)
	}
	if strings.Contains(err.Error(), "blocks[") {
		t.Fatalf("reject must not carry a physical index the model cannot map: %v", err)
	}
}

// Derived ids never collide within one emit, even when the model
// already used the natural suffix.
func TestDeriveSplitDiagramBlockIDCollision(t *testing.T) {
	used := map[string]bool{"t1": true, "t1_diagram": true}
	got := deriveSplitDiagramBlockID("t1", used)
	if got == "t1_diagram" || got == "" {
		t.Fatalf("collision not avoided: %q", got)
	}
	if !used[got] {
		t.Fatalf("derived id must be recorded as used")
	}
}

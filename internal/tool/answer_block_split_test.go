package tool

import (
	"reflect"
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
	if out[0].ID != "s1" {
		t.Fatalf("untouched block must stay first")
	}
	visible, diagramHalf := out[1], out[2]
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
	visible, diagramHalf := reflect.ValueOf(out[0]), reflect.ValueOf(out[1])
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
		if !reflect.DeepEqual(out[0], tc.b) {
			t.Errorf("%s: block mutated on pass-through", tc.name)
		}
	}
}

// Patch ops: a fused REPLACE entry keeps the visible half in
// replace_blocks (one block per replaced id) and moves the diagram
// half to add_blocks; a fused ADD entry splits in place.
func TestSplitFusedDiagramPatchBlocks(t *testing.T) {
	fusedReplace := fullyFusedBlock()
	fusedAdd := fullyFusedBlock()
	fusedAdd.ID = "t2"
	replaceOut, addOut := splitFusedDiagramPatchBlocks("test",
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

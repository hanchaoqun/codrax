package tool

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// G2 (post_v2_runtime_gap_remediation, 2026-05-04). NormalizeEmitAnswerBlock
// is the single-source converter from emitAnswerBlockV2 to
// types.AnswerBlock. The tests here lock:
//   - happy-path field propagation (every typed field on the JSON
//     shape lands on the typed result)
//   - error wording for the per-block validation failures (id, kind,
//     surface_role, diagram body)
//   - field-path prefixing so full-emit and patch-emit produce
//     comparable error sites
//   - the EdgeAnchors regression — pre-G2 the patch path silently
//     dropped this field, this test fails immediately if the helper
//     ever reverts to omitting it.

func TestNormalizeEmitAnswerBlock_HappyPathFullProjection(t *testing.T) {
	in := emitAnswerBlockV2{
		ID:    "b1",
		Kind:  string(types.BlockSummary),
		Title: "Title",
		Text:  "body text",
		Items: []emitAnswerBlockItemV2{
			{ID: "i1", Label: "L", Text: "T", CitationRef: 3},
		},
		ClaimUses: []types.RenderedClaimUse{
			{ClaimForm: types.ClaimDefinitionFact, FacetID: "f1"},
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
		FacetIDs:    []string{"f1", "f2"},
		SurfaceRole: string(types.SurfacePrincipal),
	}
	got, err := NormalizeEmitAnswerBlock(in, "blocks[0]")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.ID != "b1" || got.Kind != types.BlockSummary || got.Title != "Title" || got.Text != "body text" {
		t.Errorf("scalar fields lost: %+v", got)
	}
	if len(got.Items) != 1 || got.Items[0].CitationRef != 3 {
		t.Errorf("items[0].CitationRef lost: %+v", got.Items)
	}
	if len(got.ClaimUses) != 1 || got.ClaimUses[0].FacetID != "f1" {
		t.Errorf("claim_uses lost: %+v", got.ClaimUses)
	}
	if len(got.EdgeAnchors) != 1 || got.EdgeAnchors[0].FromNode != "A" {
		t.Errorf("EdgeAnchors lost: %+v", got.EdgeAnchors)
	}
	if got.SurfaceRole != types.SurfacePrincipal {
		t.Errorf("surface_role lost: %v", got.SurfaceRole)
	}
}

// G2 regression lock — pre-G2 the patch path's
// convertEmitBlocksToTyped silently dropped EdgeAnchors. Single-source
// normalizer fixes this. Lock it here so it cannot regress.
func TestNormalizeEmitAnswerBlock_EdgeAnchorsRegressionLock(t *testing.T) {
	in := emitAnswerBlockV2{
		ID:   "b1",
		Kind: string(types.BlockDiagram),
		Diagram: &emitAnswerDiagramV2{
			Kind: string(types.DiagramFlow),
			Body: "flowchart LR\n  A --> B",
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
	}
	got, err := NormalizeEmitAnswerBlock(in, "replace_blocks[0]")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got.EdgeAnchors) != 1 {
		t.Fatalf("EdgeAnchors must propagate (G2 patch-path bug regression lock): got %+v", got.EdgeAnchors)
	}
	if got.EdgeAnchors[0].RelationKind != types.DiagramRelCall {
		t.Errorf("RelationKind lost: %+v", got.EdgeAnchors[0])
	}
}

func TestNormalizeEmitAnswerBlock_RejectsEmptyID(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{Kind: string(types.BlockSummary)}, "blocks[2]")
	if err == nil {
		t.Fatal("must reject empty id")
	}
	if !strings.Contains(err.Error(), "blocks[2]") {
		t.Errorf("err must include fieldPath; got %q", err.Error())
	}
}

func TestNormalizeEmitAnswerBlock_RejectsInvalidKind(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{ID: "b1", Kind: "bogus"}, "blocks[0]")
	if err == nil {
		t.Fatal("must reject bogus kind")
	}
	// R4: error must NOT contain Go-internal type name.
	if strings.Contains(err.Error(), "AnswerBlockKind") {
		t.Errorf("R4 violation: err leaks Go type name: %q", err.Error())
	}
	// MUST name the bad value + allowed list.
	if !strings.Contains(err.Error(), `"bogus"`) {
		t.Errorf("err must name bad value verbatim: %q", err.Error())
	}
}

func TestNormalizeEmitAnswerBlock_RejectsInvalidSurfaceRole(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID: "b1", Kind: string(types.BlockSummary), SurfaceRole: "garbage",
	}, "blocks[0]")
	if err == nil {
		t.Fatal("must reject bogus surface_role")
	}
	// R4 cleanup: surface role error uses lowercase contract phrasing.
	if strings.Contains(err.Error(), "SurfaceRole") {
		t.Errorf("R4 violation: err leaks Go type name: %q", err.Error())
	}
}

func TestNormalizeEmitAnswerBlock_DiagramBodyRequired(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID: "b1", Kind: string(types.BlockDiagram),
		Diagram: &emitAnswerDiagramV2{Kind: string(types.DiagramFlow), Body: "  "},
	}, "blocks[0]")
	if err == nil {
		t.Fatal("must reject empty diagram body")
	}
}

func TestNormalizeEmitAnswerBlock_DiagramKindRequiresPayload(t *testing.T) {
	_, err := NormalizeEmitAnswerBlock(emitAnswerBlockV2{
		ID: "b1", Kind: string(types.BlockDiagram),
		// No Diagram payload.
	}, "blocks[0]")
	if err == nil {
		t.Fatal("must reject kind=diagram without diagram payload")
	}
}

// TestNormalizeEmitAnswerBlock_AllFieldsPropagate uses reflection to
// verify every field on emitAnswerBlockV2 surfaces as a non-zero
// value on the resulting types.AnswerBlock when populated from a
// fully-filled fixture. This is the structural lock that prevents a
// future field addition from silently being dropped — the test fails
// until the new field is wired into NormalizeEmitAnswerBlock.
func TestNormalizeEmitAnswerBlock_AllFieldsPropagate(t *testing.T) {
	in := emitAnswerBlockV2{
		ID:    "b1",
		Kind:  string(types.BlockDiagram),
		Title: "Title",
		Text:  "body text",
		Items: []emitAnswerBlockItemV2{
			{ID: "i1", Label: "L", Text: "T", CitationRef: 3},
		},
		Diagram: &emitAnswerDiagramV2{
			Kind: string(types.DiagramFlow), Body: "flowchart LR\n  A --> B",
		},
		ClaimUses: []types.RenderedClaimUse{
			{ClaimForm: types.ClaimDefinitionFact, FacetID: "f1"},
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge},
		},
		FacetIDs:    []string{"f1", "f2"},
		SurfaceRole: string(types.SurfacePrincipal),
	}
	got, err := NormalizeEmitAnswerBlock(in, "blocks[0]")
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	// Per-field lock: every emitAnswerBlockV2 input field must surface
	// a corresponding non-zero typed field. When a new field is added
	// to emitAnswerBlockV2, fixturise it above AND extend this map.
	checks := map[string]func() bool{
		"ID":          func() bool { return got.ID != "" },
		"Kind":        func() bool { return got.Kind != "" },
		"Title":       func() bool { return got.Title != "" },
		"Text":        func() bool { return got.Text != "" },
		"Items":       func() bool { return len(got.Items) > 0 },
		"Diagram":     func() bool { return got.Diagram != nil && got.Diagram.Body != "" },
		"ClaimUses":   func() bool { return len(got.ClaimUses) > 0 },
		"EdgeAnchors": func() bool { return len(got.EdgeAnchors) > 0 },
		"FacetIDs":    func() bool { return len(got.FacetIDs) > 0 },
		"SurfaceRole": func() bool { return got.SurfaceRole != "" },
	}
	for name, check := range checks {
		if !check() {
			t.Errorf("field %q dropped by normalizer (got AnswerBlock = %+v)", name, got)
		}
	}

	// Sanity: the JSON shape and the typed shape must have field-name
	// agreement on the load-bearing fields. Reflection over the JSON
	// shape catches a future emitAnswerBlockV2 field rename.
	jsonShape := reflect.TypeOf(emitAnswerBlockV2{})
	typedShape := reflect.TypeOf(types.AnswerBlock{})
	for i := 0; i < jsonShape.NumField(); i++ {
		f := jsonShape.Field(i)
		// The JSON-shape has fields "Diagram" (pointer to emit shape)
		// and a few items[].* sub-fields that don't appear by name on
		// the typed AnswerBlock surface. Limit the reflection check to
		// fields whose names also exist on AnswerBlock.
		if _, ok := typedShape.FieldByName(f.Name); !ok {
			continue
		}
		if _, ok := checks[f.Name]; !ok {
			t.Errorf("emitAnswerBlockV2 field %q has a typed counterpart but is NOT covered by the propagation lock; add a fixture+check for it", f.Name)
		}
	}
}

package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ── B4-T5 V2 block oracle 单测 (4 validator × 4 case = 16+ + AllViolationKinds 完整性) ──

func minimalSummaryView() *types.AnswerSemanticView {
	return &types.AnswerSemanticView{
		Family: types.QFGeneric,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, MinCount: 1, MaxCount: 1, Required: true, Rationale: "summary required"},
		},
	}
}

func summaryBlock(id string) types.AnswerBlock {
	return types.AnswerBlock{ID: id, Kind: types.BlockSummary, Text: "x"}
}

// ── validateRequiredBlockCoverage ──────────────────────────

func TestRequiredBlockCoverage_MeetsMinPasses(t *testing.T) {
	view := minimalSummaryView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{summaryBlock("b1")}}
	if vs := validateRequiredBlockCoverage(doc, view); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestRequiredBlockCoverage_MissingBlockFires(t *testing.T) {
	view := minimalSummaryView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "x1", Kind: types.BlockOrderedList},
	}}
	vs := validateRequiredBlockCoverage(doc, view)
	if len(vs) != 1 || vs[0].Kind != types.ViolBlockCoverageMissing {
		t.Fatalf("expected ViolBlockCoverageMissing; got %+v", vs)
	}
	if !strings.Contains(vs[0].Detail, "summary") {
		t.Errorf("detail should name missing kind; got %q", vs[0].Detail)
	}
}

func TestRequiredBlockCoverage_OverEmittedFires(t *testing.T) {
	view := minimalSummaryView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		summaryBlock("b1"),
		summaryBlock("b2"),
	}}
	vs := validateRequiredBlockCoverage(doc, view)
	if len(vs) != 1 || vs[0].Kind != types.ViolBlockCoverageMissing {
		t.Fatalf("expected violation on over-emit; got %+v", vs)
	}
}

func TestRequiredBlockCoverage_NilGuards(t *testing.T) {
	if vs := validateRequiredBlockCoverage(nil, minimalSummaryView()); vs != nil {
		t.Errorf("nil doc → nil violations; got %+v", vs)
	}
	if vs := validateRequiredBlockCoverage(&types.AnswerDocumentV2{}, nil); vs != nil {
		t.Errorf("nil view → nil violations; got %+v", vs)
	}
}

// ── validatePrincipalClaimUse ──────────────────────────────

func principalClaimUseView() *types.AnswerSemanticView {
	return &types.AnswerSemanticView{
		Family: types.QFRoleLookup,
		RequiredBlocks: []types.BlockRequirement{
			{
				Kind:                 types.BlockScalar,
				MinCount:             1,
				MaxCount:             1,
				Required:             true,
				AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact},
				SurfaceRoleHint:      types.SurfacePrincipal,
				Rationale:            "scalar required",
			},
		},
	}
}

func TestPrincipalClaimUse_PresentPasses(t *testing.T) {
	view := principalClaimUseView()
	cu := types.RenderedClaimUse{ClaimForm: types.ClaimDefinitionFact, SurfaceRole: types.SurfacePrincipal}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:        "s1",
			Kind:      types.BlockScalar,
			Text:      "42",
			ClaimUses: []types.RenderedClaimUse{cu},
		},
	}}
	if vs := validatePrincipalClaimUse(doc, view); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestPrincipalClaimUse_MissingFires(t *testing.T) {
	view := principalClaimUseView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "s1", Kind: types.BlockScalar, Text: "42", SurfaceRole: types.SurfacePrincipal},
	}}
	vs := validatePrincipalClaimUse(doc, view)
	if len(vs) != 1 || vs[0].Kind != types.ViolPrincipalClaimUseMissing {
		t.Fatalf("expected ViolPrincipalClaimUseMissing; got %+v", vs)
	}
}

func TestPrincipalClaimUse_NonPrincipalSkipped(t *testing.T) {
	view := principalClaimUseView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "s1", Kind: types.BlockScalar, Text: "42", SurfaceRole: types.SurfaceSupport},
	}}
	if vs := validatePrincipalClaimUse(doc, view); len(vs) != 0 {
		t.Errorf("non-principal block must skip; got %+v", vs)
	}
}

func TestPrincipalClaimUse_ItemLevelClaimSatisfies(t *testing.T) {
	view := principalClaimUseView()
	cu := types.RenderedClaimUse{ClaimForm: types.ClaimDefinitionFact}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:          "s1",
			Kind:        types.BlockScalar,
			SurfaceRole: types.SurfacePrincipal,
			Items:       []types.AnswerBlockItem{{Label: "v", ClaimUse: &cu}},
		},
	}}
	if vs := validatePrincipalClaimUse(doc, view); len(vs) != 0 {
		t.Errorf("item-level ClaimUse must satisfy; got %+v", vs)
	}
}

// ── validateDiagramEdgeSupport ─────────────────────────────

func diagramRequiredView(kind types.DiagramKind) *types.AnswerSemanticView {
	return &types.AnswerSemanticView{
		Family: types.QFCallChain,
		DiagramPlan: &types.DiagramFacetGraph{
			Required: true,
			Kind:     kind,
		},
	}
}

func TestDiagramEdgeSupport_RequiredAndPresentPasses(t *testing.T) {
	view := diagramRequiredView(types.DiagramSequence)
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "d1",
			Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramSequence,
				Body: "sequenceDiagram",
			},
		},
	}}
	if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestDiagramEdgeSupport_RequiredAbsentFires(t *testing.T) {
	view := diagramRequiredView(types.DiagramSequence)
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{summaryBlock("b1")}}
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) != 1 || vs[0].Kind != types.ViolDiagramEdgeUnsupported {
		t.Fatalf("expected ViolDiagramEdgeUnsupported; got %+v", vs)
	}
}

func TestDiagramEdgeSupport_KindMismatchFires(t *testing.T) {
	view := diagramRequiredView(types.DiagramSequence)
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "d1",
			Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramFlow, // 故意不匹配
				Body: "flowchart LR",
			},
		},
	}}
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) != 1 {
		t.Fatalf("expected 1 violation on kind mismatch; got %+v", vs)
	}
}

func TestDiagramEdgeSupport_NoDiagramPlanSkipped(t *testing.T) {
	view := &types.AnswerSemanticView{Family: types.QFGeneric}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{summaryBlock("b1")}}
	if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
		t.Errorf("no DiagramPlan should skip; got %+v", vs)
	}
}

// ── R4.3 deepening: per-edge endpoint grounding ─────────────

func diagramFlowView() *types.AnswerSemanticView {
	return &types.AnswerSemanticView{
		Family: types.QFRootCauseTrace,
		DiagramPlan: &types.DiagramFacetGraph{
			Required: true,
			Kind:     types.DiagramFlow,
		},
	}
}

// TestDiagramEdgeSupport_AllEdgesGroundedInBodyDecls — every edge's
// endpoint is a declared node in the same body, so grounding succeeds
// purely from the body itself with no other doc content needed.
func TestDiagramEdgeSupport_AllEdgesGroundedInBodyDecls(t *testing.T) {
	view := diagramFlowView()
	body := "flowchart LR\n" +
		"  A[\"Login\"] --> B[\"AuthCheck\"]\n" +
		"  B --> C[\"Dashboard\"]\n"
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "d1",
			Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramFlow,
				Body: body,
			},
		},
	}}
	if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
		t.Errorf("all-grounded body edges should pass; got %+v", vs)
	}
}

// TestDiagramEdgeSupport_AllEdgesGroundedInItemsAndTitles —
// endpoints are bare identifiers (no in-body decl) but each is named
// in an item label or block title elsewhere in the doc. Grounding
// succeeds via the cross-doc support set.
func TestDiagramEdgeSupport_AllEdgesGroundedInItemsAndTitles(t *testing.T) {
	view := diagramFlowView()
	body := "flowchart LR\n  Login --> AuthCheck\n  AuthCheck --> Dashboard\n"
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:    "ol1",
			Kind:  types.BlockOrderedList,
			Title: "Login",
			Items: []types.AnswerBlockItem{
				{Label: "AuthCheck"},
				{Label: "Dashboard"},
			},
		},
		{
			ID:   "d1",
			Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramFlow,
				Body: body,
			},
		},
	}}
	if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
		t.Errorf("items/title-grounded edges should pass; got %+v", vs)
	}
}

// TestDiagramEdgeSupport_HallucinatedMiddleNodeFires — Login and
// Dashboard are grounded but InternalGate is not. Both edges
// (Login -> InternalGate) and (InternalGate -> Dashboard) cite
// InternalGate as an endpoint, so both fire.
func TestDiagramEdgeSupport_HallucinatedMiddleNodeFires(t *testing.T) {
	view := diagramFlowView()
	body := "flowchart LR\n  Login --> InternalGate\n  InternalGate --> Dashboard\n"
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:    "ol1",
			Kind:  types.BlockOrderedList,
			Title: "Login",
			Items: []types.AnswerBlockItem{
				{Label: "Dashboard"},
			},
		},
		{
			ID:   "d1",
			Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramFlow,
				Body: body,
			},
		},
	}}
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) != 1 || vs[0].Kind != types.ViolDiagramEdgeUnsupported {
		t.Fatalf("expected one ViolDiagramEdgeUnsupported; got %+v", vs)
	}
	if !strings.Contains(vs[0].Detail, "InternalGate") {
		t.Errorf("Detail should name the hallucinated endpoint; got %q", vs[0].Detail)
	}
	if !strings.Contains(vs[0].Detail, "Login -> InternalGate") {
		t.Errorf("Detail should list the unsupported (from -> to) pair; got %q", vs[0].Detail)
	}
	if !strings.Contains(vs[0].Detail, "InternalGate -> Dashboard") {
		t.Errorf("Detail should list both unsupported pairs; got %q", vs[0].Detail)
	}
}

// TestDiagramEdgeSupport_FullyHallucinatedEdgesFire — neither
// endpoint of either edge is grounded; every edge is reported.
func TestDiagramEdgeSupport_FullyHallucinatedEdgesFire(t *testing.T) {
	view := diagramFlowView()
	body := "flowchart LR\n  Phantom1 --> Phantom2\n  Phantom2 --> Phantom3\n"
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "d1",
			Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramFlow,
				Body: body,
			},
		},
	}}
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) != 1 {
		t.Fatalf("expected exactly one aggregated violation; got %d (%+v)", len(vs), vs)
	}
	d := vs[0].Detail
	if !strings.Contains(d, "2 edge(s)") {
		t.Errorf("Detail should report 2 unsupported edges; got %q", d)
	}
}

// TestDiagramEdgeSupport_BodyWithLabelsButEmptyClaimsPasses — the
// body declares the labels via shape wrappers; doc has nothing else.
// Grounding via the body's own node decls is sufficient.
func TestDiagramEdgeSupport_BodyWithLabelsButEmptyClaimsPasses(t *testing.T) {
	view := diagramFlowView()
	body := "flowchart TD\n" +
		"  N1((\"Start\")) --> N2{Decision}\n" +
		"  N2 --> N3[\"End\"]\n"
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "d1",
			Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramFlow,
				Body: body,
			},
		},
	}}
	if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
		t.Errorf("self-declared body should pass; got %+v", vs)
	}
}

// TestDiagramEdgeSupport_SequenceDiagramArrowsAreParsed — the
// sequence-diagram arrow operators (->>, -->>) are recognised as
// edges by the parser.
func TestDiagramEdgeSupport_SequenceDiagramArrowsAreParsed(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFCallChain,
		DiagramPlan: &types.DiagramFacetGraph{
			Required: true,
			Kind:     types.DiagramSequence,
		},
	}
	body := "sequenceDiagram\n  Client->>Server: GET /x\n  Server-->>PhantomDB: query\n  PhantomDB-->>Server: row\n  Server-->>Client: 200 OK\n"
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:    "ol1",
			Kind:  types.BlockBulletList,
			Title: "Trace",
			Items: []types.AnswerBlockItem{
				{Label: "Client"},
				{Label: "Server"},
			},
		},
		{
			ID:   "d1",
			Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramSequence,
				Body: body,
			},
		},
	}}
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) != 1 {
		t.Fatalf("expected one violation listing the PhantomDB edges; got %+v", vs)
	}
	if !strings.Contains(vs[0].Detail, "PhantomDB") {
		t.Errorf("Detail should name PhantomDB; got %q", vs[0].Detail)
	}
}

// TestDiagramEdgeSupport_NilClaimUsesDoesNotPanic — the per-edge
// path must handle a doc whose blocks have nil ClaimUses / nil item
// ClaimUse pointers without panicking.
func TestDiagramEdgeSupport_NilClaimUsesDoesNotPanic(t *testing.T) {
	view := diagramFlowView()
	body := "flowchart LR\n  A --> B\n"
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:        "ol1",
			Kind:      types.BlockOrderedList,
			Title:     "A",
			Items:     []types.AnswerBlockItem{{Label: "B"}},
			ClaimUses: nil,
		},
		{
			ID:   "d1",
			Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind:      types.DiagramFlow,
				Body:      body,
				ClaimUses: nil,
			},
		},
	}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil claim_uses caused panic: %v", r)
		}
	}()
	if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
		t.Errorf("nil claim_uses with grounded items should pass; got %+v", vs)
	}
}

// TestDiagramEdgeSupport_EdgesWithLabelsParseCorrectly — the
// `A -->|cond| B` and `A -- text --> B` shapes must split as
// (A, B) so the labels do not leak into endpoint tokens.
func TestDiagramEdgeSupport_EdgesWithLabelsParseCorrectly(t *testing.T) {
	view := diagramFlowView()
	body := "flowchart LR\n  A -->|matched| B\n  B -- when_x_is_true --> C\n"
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:    "ol1",
			Kind:  types.BlockOrderedList,
			Title: "A",
			Items: []types.AnswerBlockItem{{Label: "B"}, {Label: "C"}},
		},
		{
			ID:   "d1",
			Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramFlow,
				Body: body,
			},
		},
	}}
	if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
		t.Errorf("labelled edges with grounded endpoints should pass; got %+v", vs)
	}
}

// TestDiagramEdgeSupport_EmptyBodyShortCircuits — an empty body must
// not cause the parser to misclassify it as fully-hallucinated. The
// validator returns nil so the existing kind/presence violations
// remain the only signal.
func TestDiagramEdgeSupport_EmptyBodyShortCircuits(t *testing.T) {
	view := diagramFlowView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "d1",
			Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramFlow,
				Body: "   \n  \n",
			},
		},
	}}
	if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
		t.Errorf("empty body should not fire edge oracle; got %+v", vs)
	}
}

// ── validateUncertaintyBlockPresence ───────────────────────

func uncertaintyView() *types.AnswerSemanticView {
	return &types.AnswerSemanticView{
		Family: types.QFRootCauseTrace,
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: types.FacetObservedArtifactFact, Required: types.FacetHardRequired},
			},
		},
		UncertaintyRules: []types.UncertaintyRule{
			{
				TriggerFacet:      string(types.FacetObservedArtifactFact),
				ExpectedBlockKind: types.BlockCaveat,
				MissingMessage:    "emit caveat for log drift",
			},
		},
	}
}

func TestUncertaintyBlockPresence_PresentPasses(t *testing.T) {
	view := uncertaintyView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		summaryBlock("b1"),
		{ID: "c1", Kind: types.BlockCaveat, Text: "log drift"},
	}}
	if vs := validateUncertaintyBlockPresence(doc, view); len(vs) != 0 {
		t.Errorf("expected pass; got %+v", vs)
	}
}

func TestUncertaintyBlockPresence_MissingFires(t *testing.T) {
	view := uncertaintyView()
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{summaryBlock("b1")}}
	vs := validateUncertaintyBlockPresence(doc, view)
	if len(vs) != 1 || vs[0].Kind != types.ViolUncertaintyBlockMissing {
		t.Fatalf("expected ViolUncertaintyBlockMissing; got %+v", vs)
	}
	if !strings.Contains(vs[0].Repair, "caveat") {
		t.Errorf("repair should mention caveat; got %q", vs[0].Repair)
	}
}

func TestUncertaintyBlockPresence_TriggerNotInRequiredSkipped(t *testing.T) {
	view := uncertaintyView()
	view.FacetCoverage.Required = nil // remove the trigger facet
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{summaryBlock("b1")}}
	if vs := validateUncertaintyBlockPresence(doc, view); len(vs) != 0 {
		t.Errorf("trigger absent → skip; got %+v", vs)
	}
}

func TestUncertaintyBlockPresence_NoRulesSkipped(t *testing.T) {
	view := &types.AnswerSemanticView{Family: types.QFGeneric}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{summaryBlock("b1")}}
	if vs := validateUncertaintyBlockPresence(doc, view); len(vs) != 0 {
		t.Errorf("no UncertaintyRules → skip; got %+v", vs)
	}
}

// ── runV2BlockOracles 联合 dispatch ────────────────────────

func TestRunV2BlockOracles_AggregatesAllViolations(t *testing.T) {
	view := uncertaintyView()
	view.RequiredBlocks = []types.BlockRequirement{
		{Kind: types.BlockSummary, MinCount: 1, MaxCount: 1, Required: true, Rationale: "summary"},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		// no summary, no caveat — both missing.
	}}
	vs := runV2BlockOracles(doc, view)
	if len(vs) < 2 {
		t.Errorf("expected ≥2 violations (coverage + uncertainty); got %d (%+v)", len(vs), vs)
	}
}

func TestRunV2BlockOracles_NilGuards(t *testing.T) {
	if vs := runV2BlockOracles(nil, &types.AnswerSemanticView{}); vs != nil {
		t.Errorf("nil doc → nil; got %+v", vs)
	}
	if vs := runV2BlockOracles(&types.AnswerDocumentV2{}, nil); vs != nil {
		t.Errorf("nil view → nil; got %+v", vs)
	}
}

// TestRunV2BlockOracles_PatchMergedDocTriggersSameValidators is a
// Phase 2-B2 regression guard. It pins the contract that V2 oracle
// validation is INVARIANT to whether the AnswerDocumentV2 reached
// MutableState via fresh full emit (SetAnswerDocumentV2) or via
// patch merge (SetAnswerDocumentV2FromPatch + ApplyAnswerDocumentV2Patch).
//
// The orchestrator's runContractCheck reads mut.AnswerDocumentV2()
// post-emit; both emit paths funnel through the same MutableState
// surface, so the V2 oracle suite sees identical input regardless
// of provenance. If a future commit short-circuits patch emit to
// bypass MutableState (or routes patch through a different
// pre-validation that strips fields), this test will fire the
// validators on a known-violating merged doc and prove the breach.
func TestRunV2BlockOracles_PatchMergedDocTriggersSameValidators(t *testing.T) {
	// Step 1: build a prev doc lacking a required summary block.
	// (Pre-patch shape passes patch's structural validator —
	// missing required block kind is a CONTRACT-level violation,
	// not a STRUCTURAL one.)
	prev := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{ID: "list1", Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{{ID: "i1", Label: "A"}}},
		},
	}
	// Step 2: apply an empty-mutation patch (UnchangedBlockIDs only).
	// Merged doc == prev structurally. Critical: ApplyAnswerDocumentV2Patch
	// only checks structural invariants — it does NOT enforce
	// "must have a summary block". That gate belongs to runV2BlockOracles.
	patch := &types.AnswerDocumentV2Patch{
		UnchangedBlockIDs: []string{"list1"},
	}
	merged, err := types.ApplyAnswerDocumentV2Patch(prev, patch)
	if err != nil {
		t.Fatalf("patch apply failed: %v", err)
	}

	// Step 3: feed the merged doc into runV2BlockOracles with a view
	// that DOES require a summary. The validators must fire on the
	// patch-merged doc the same way they would fire on a fresh emit
	// of the same shape.
	view := uncertaintyView()
	view.RequiredBlocks = []types.BlockRequirement{
		{Kind: types.BlockSummary, MinCount: 1, MaxCount: 1, Required: true, Rationale: "summary"},
	}
	violations := runV2BlockOracles(merged, view)
	if len(violations) == 0 {
		t.Fatalf("patch-merged doc lacking required summary must trigger ViolBlockCoverageMissing; got 0 violations")
	}
	// Verify the expected violation kind is present.
	foundCoverageMissing := false
	for _, v := range violations {
		if v.Kind == types.ViolBlockCoverageMissing {
			foundCoverageMissing = true
			break
		}
	}
	if !foundCoverageMissing {
		t.Errorf("expected ViolBlockCoverageMissing from patch-merged doc; got %+v", violations)
	}
}

// ── R2.3 V2 重接 facet_uncovered + richness_regression tests ──────

// TestValidateFacetCoverage_AllRequiredCovered confirms no fire
// when every Required facet has a block declaring its FacetID.
func TestValidateFacetCoverage_AllRequiredCovered(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.intro"}},
			{Kind: types.BlockOrderedList, FacetIDs: []string{"facet.steps"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: "facet.intro", Tier: types.TierEssential},
				{Kind: "facet.steps", Tier: types.TierExpected},
			},
		},
	}
	if vs := validateFacetCoverage(doc, view); len(vs) > 0 {
		t.Errorf("all required facets covered: got false fire %+v", vs)
	}
}

// TestValidateFacetCoverage_MissingFires confirms ViolFacetUncovered
// fires when a Required facet has no block declaring its FacetID.
func TestValidateFacetCoverage_MissingFires(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.intro"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: "facet.intro", Tier: types.TierEssential},
				{Kind: "facet.steps", Tier: types.TierExpected},
			},
		},
	}
	vs := validateFacetCoverage(doc, view)
	if len(vs) != 1 {
		t.Fatalf("want 1 violation; got %d (%+v)", len(vs), vs)
	}
	if vs[0].Kind != types.ViolFacetUncovered {
		t.Errorf("kind = %q, want ViolFacetUncovered", vs[0].Kind)
	}
	if !strings.Contains(vs[0].Detail, "facet.steps") {
		t.Errorf("Detail must name uncovered facet; got %q", vs[0].Detail)
	}
}

// TestValidateFacetCoverage_OptionalFacetSkipped confirms
// Tier=Enrichment is NOT a violation here (handled by
// validateRichnessRegression below).
func TestValidateFacetCoverage_OptionalFacetSkipped(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.intro"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: "facet.intro", Tier: types.TierEssential},
				{Kind: "facet.optional_extra", Tier: types.TierEnrichment},
			},
		},
	}
	if vs := validateFacetCoverage(doc, view); len(vs) > 0 {
		t.Errorf("Tier=Enrichment must skip facet_uncovered; got %+v", vs)
	}
}

// TestValidateFacetCoverage_ClaimUseFacetIDCounts confirms FacetID
// declared via item.ClaimUse.FacetID also satisfies coverage.
func TestValidateFacetCoverage_ClaimUseFacetIDCounts(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{
				{Label: "step1", ClaimUse: &types.RenderedClaimUse{FacetID: "facet.steps"}},
			},
		}},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: "facet.steps", Tier: types.TierEssential},
			},
		},
	}
	if vs := validateFacetCoverage(doc, view); len(vs) > 0 {
		t.Errorf("item.ClaimUse.FacetID must satisfy coverage; got %+v", vs)
	}
}

// TestValidateFacetCoverage_NilGuards covers nil cases.
func TestValidateFacetCoverage_NilGuards(t *testing.T) {
	if vs := validateFacetCoverage(nil, &types.AnswerSemanticView{}); vs != nil {
		t.Errorf("nil doc → nil")
	}
	if vs := validateFacetCoverage(&types.AnswerDocumentV2{}, nil); vs != nil {
		t.Errorf("nil view → nil")
	}
	if vs := validateFacetCoverage(&types.AnswerDocumentV2{}, &types.AnswerSemanticView{}); vs != nil {
		t.Errorf("nil FacetCoverage → nil")
	}
}

// TestValidateRichnessRegression_FiresWhenOptionalUnsurfaced
// confirms optional facets with evidence available but no block
// coverage fire ViolRichnessRegression.
func TestValidateRichnessRegression_FiresWhenOptionalUnsurfaced(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.intro"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Optional: []types.FacetRequirement{
				{
					Kind:            "facet.tip",
					Tier:            types.TierEnrichment,
					SourceCandidate: []string{"ev1", "ev2"},
				},
			},
		},
	}
	vs := validateRichnessRegression(doc, view)
	if len(vs) != 1 {
		t.Fatalf("want 1 richness regression; got %d (%+v)", len(vs), vs)
	}
	if vs[0].Kind != types.ViolRichnessRegression {
		t.Errorf("kind = %q, want ViolRichnessRegression", vs[0].Kind)
	}
}

// TestValidateRichnessRegression_NoEvidenceNoFire confirms an
// optional facet with empty SourceCandidate is NOT a regression.
func TestValidateRichnessRegression_NoEvidenceNoFire(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{Kind: types.BlockSummary}},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Optional: []types.FacetRequirement{
				{Kind: "facet.tip", Tier: types.TierEnrichment, SourceCandidate: nil},
			},
		},
	}
	if vs := validateRichnessRegression(doc, view); len(vs) > 0 {
		t.Errorf("no evidence available → not a regression; got %+v", vs)
	}
}

// TestValidateRichnessRegression_CoveredOptionalNoFire confirms
// covered Optional facet does not fire.
func TestValidateRichnessRegression_CoveredOptionalNoFire(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.tip"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Optional: []types.FacetRequirement{
				{Kind: "facet.tip", Tier: types.TierEnrichment, SourceCandidate: []string{"ev1"}},
			},
		},
	}
	if vs := validateRichnessRegression(doc, view); len(vs) > 0 {
		t.Errorf("covered optional must not fire; got %+v", vs)
	}
}

// ── R2.3 V2 重接 ClaimFormUnsupported tests ──────────────────────

// TestValidateClaimFormSupport_MatchPasses confirms when the LLM-
// declared ClaimForm matches ClaimFormOf(evidence), no fire.
func TestValidateClaimFormSupport_MatchPasses(t *testing.T) {
	mut := &types.MutableState{}
	// Definition-anchor evidence projects to ClaimDefinitionFact.
	mut.AppendEvidence([]types.EvidenceItem{{
		ID: "ev1", AnchorKind: types.AnchorDefinition,
		Source: "x.go", LineStart: 10, Subject: "Foo",
	}})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "b1",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				Label: "step1",
				ClaimUse: &types.RenderedClaimUse{
					EvidenceID: "ev1",
					ClaimForm:  types.ClaimDefinitionFact,
				},
			}},
		}},
	}
	if vs := validateClaimFormSupport(doc, mut); len(vs) > 0 {
		t.Errorf("matching ClaimForm must not fire; got %+v", vs)
	}
}

// TestValidateClaimFormSupport_MismatchFires confirms a typed-
// projection mismatch fires ViolClaimFormUnsupported.
func TestValidateClaimFormSupport_MismatchFires(t *testing.T) {
	mut := &types.MutableState{}
	// Call-anchor evidence → ClaimCallEdge.
	mut.AppendEvidence([]types.EvidenceItem{{
		ID: "ev1", AnchorKind: types.AnchorCall,
		Source: "x.go", LineStart: 50, Subject: "callerFn", Object: "calleeFn",
	}})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "b1",
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{{
				Label: "x",
				ClaimUse: &types.RenderedClaimUse{
					EvidenceID: "ev1",
					ClaimForm:  types.ClaimDefinitionFact, // wrong — evidence is a call
				},
			}},
		}},
	}
	vs := validateClaimFormSupport(doc, mut)
	if len(vs) != 1 {
		t.Fatalf("want 1 violation; got %d (%+v)", len(vs), vs)
	}
	if vs[0].Kind != types.ViolClaimFormUnsupported {
		t.Errorf("kind = %q, want ViolClaimFormUnsupported", vs[0].Kind)
	}
	if !strings.Contains(vs[0].Detail, "definition_fact") || !strings.Contains(vs[0].Detail, "call_edge") {
		t.Errorf("Detail must name both forms; got %q", vs[0].Detail)
	}
}

// TestValidateClaimFormSupport_GeneralisationOK confirms when
// ClaimFormOf(evidence) is ClaimUnknown (projection couldn't lock
// a form), the LLM is allowed to declare a more specific form.
func TestValidateClaimFormSupport_GeneralisationOK(t *testing.T) {
	mut := &types.MutableState{}
	// Bare evidence with no AnchorKind → ClaimUnknown.
	mut.AppendEvidence([]types.EvidenceItem{{ID: "ev1"}})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{{
				Label: "x",
				ClaimUse: &types.RenderedClaimUse{
					EvidenceID: "ev1",
					ClaimForm:  types.ClaimAssignmentFact,
				},
			}},
		}},
	}
	if vs := validateClaimFormSupport(doc, mut); len(vs) > 0 {
		t.Errorf("ClaimUnknown projection must allow generalisation; got %+v", vs)
	}
}

// TestValidateClaimFormSupport_UnknownEvidenceIDSkipped confirms
// EvidenceID not in pool is silently skipped (no spurious fire).
func TestValidateClaimFormSupport_UnknownEvidenceIDSkipped(t *testing.T) {
	mut := &types.MutableState{}
	mut.AppendEvidence([]types.EvidenceItem{{ID: "ev1", AnchorKind: types.AnchorDefinition}})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{{
				Label: "x",
				ClaimUse: &types.RenderedClaimUse{
					EvidenceID: "ev_phantom",
					ClaimForm:  types.ClaimDefinitionFact,
				},
			}},
		}},
	}
	if vs := validateClaimFormSupport(doc, mut); len(vs) > 0 {
		t.Errorf("unknown EvidenceID must be silently skipped; got %+v", vs)
	}
}

// TestValidateClaimFormSupport_NilGuards covers nil guards.
func TestValidateClaimFormSupport_NilGuards(t *testing.T) {
	if vs := validateClaimFormSupport(nil, &types.MutableState{}); vs != nil {
		t.Errorf("nil doc → nil")
	}
	if vs := validateClaimFormSupport(&types.AnswerDocumentV2{}, nil); vs != nil {
		t.Errorf("nil mut → nil")
	}
	if vs := validateClaimFormSupport(&types.AnswerDocumentV2{}, &types.MutableState{}); vs != nil {
		t.Errorf("empty pool → nil")
	}
}

// TestValidateClaimFormSupport_EmptyClaimFormSkipped confirms when
// LLM didn't declare a ClaimForm (empty), there's nothing to check.
func TestValidateClaimFormSupport_EmptyClaimFormSkipped(t *testing.T) {
	mut := &types.MutableState{}
	mut.AppendEvidence([]types.EvidenceItem{{ID: "ev1", AnchorKind: types.AnchorDefinition}})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			Kind: types.BlockBulletList,
			Items: []types.AnswerBlockItem{{
				Label:    "x",
				ClaimUse: &types.RenderedClaimUse{EvidenceID: "ev1"}, // no ClaimForm declared
			}},
		}},
	}
	if vs := validateClaimFormSupport(doc, mut); len(vs) > 0 {
		t.Errorf("empty ClaimForm must be silently skipped; got %+v", vs)
	}
}

// ── R2.3 V2 重接 AbsenceScopeBound tests ─────────────────────────

// TestValidateAbsenceScopeBound_BoundedPasses confirms an absent
// claim with at least one Scope=Negative citation + non-empty
// NegativePattern is bounded → no fire.
func TestValidateAbsenceScopeBound_BoundedPasses(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		Citations: []types.Citation{
			{File: "internal", Scope: types.ScopeNegative, NegativePattern: "rg -n FooBar internal/"},
		},
	}
	if vs := validateAbsenceScopeBound(doc); len(vs) > 0 {
		t.Errorf("bounded absence must not fire; got %+v", vs)
	}
}

// TestValidateAbsenceScopeBound_UnboundedFires confirms an absent
// claim with NO Scope=Negative citation fires
// ViolAbsenceScopeExceeded.
func TestValidateAbsenceScopeBound_UnboundedFires(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		Citations: []types.Citation{
			{File: "x.go", Line: 10},
		},
	}
	vs := validateAbsenceScopeBound(doc)
	if len(vs) != 1 {
		t.Fatalf("want 1 violation; got %d (%+v)", len(vs), vs)
	}
	if vs[0].Kind != types.ViolAbsenceScopeExceeded {
		t.Errorf("kind = %q, want ViolAbsenceScopeExceeded", vs[0].Kind)
	}
}

// TestValidateAbsenceScopeBound_NegativeWithoutPatternFires confirms
// the NegativePattern emptiness check: a Citation with
// Scope=Negative but empty NegativePattern is NOT a bound (the
// system has no audit trail of the actual search query).
func TestValidateAbsenceScopeBound_NegativeWithoutPatternFires(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		Citations: []types.Citation{
			{File: "x.go", Scope: types.ScopeNegative, NegativePattern: "  "}, // whitespace
		},
	}
	if vs := validateAbsenceScopeBound(doc); len(vs) != 1 {
		t.Errorf("whitespace-only NegativePattern must NOT count as bounded; got %d (%+v)", len(vs), vs)
	}
}

// TestValidateAbsenceScopeBound_NonAbsentSkipped confirms the
// oracle skips when the resolution is resolved / unknown / nil.
func TestValidateAbsenceScopeBound_NonAbsentSkipped(t *testing.T) {
	for _, status := range []types.AnswerExactResolutionStatus{
		"",                  // empty
		"resolved",          // typed
		"unknown",
	} {
		doc := &types.AnswerDocumentV2{
			ExactResolution: &types.AnswerExactResolution{Status: status},
		}
		if vs := validateAbsenceScopeBound(doc); len(vs) > 0 {
			t.Errorf("status=%q must skip; got %+v", status, vs)
		}
	}
}

// TestValidateAbsenceScopeBound_NilGuards covers nil cases.
func TestValidateAbsenceScopeBound_NilGuards(t *testing.T) {
	if vs := validateAbsenceScopeBound(nil); vs != nil {
		t.Errorf("nil doc → nil")
	}
	if vs := validateAbsenceScopeBound(&types.AnswerDocumentV2{}); vs != nil {
		t.Errorf("nil ExactResolution → nil")
	}
}

// ── AllViolationKinds 完整性 (4 新 kind 在 covered + kindSymbols 双表) ──
func TestB4ViolationKindsRegistered(t *testing.T) {
	want := []types.ViolationKind{
		types.ViolBlockCoverageMissing,
		types.ViolPrincipalClaimUseMissing,
		types.ViolDiagramEdgeUnsupported,
		types.ViolUncertaintyBlockMissing,
		types.ViolEnumerationLabelUngrounded,
	}
	all := types.AllViolationKinds()
	seen := map[types.ViolationKind]bool{}
	for _, k := range all {
		seen[k] = true
	}
	for _, k := range want {
		if !seen[k] {
			t.Errorf("AllViolationKinds() missing %q (B4 oracle should be registered)", k)
		}
	}
}

// ── R-Hallu post-shape s1a-20260504-064754 forensic:
// validateEnumerationItemLabelGrounding lock tests ───────

func mutWithEvidence(items []types.EvidenceItem) *types.MutableState {
	mut := &types.MutableState{}
	mut.SetTurnAArtifacts(types.TurnAArtifacts{EvidenceItems: items})
	return mut
}

// TestEnumerationLabelGrounding_AllLabelsMatchAnchorPasses — every
// items[].label is the verbatim AnchorSymbol of an evidence item;
// oracle returns no violations.
func TestEnumerationLabelGrounding_AllLabelsMatchAnchorPasses(t *testing.T) {
	mut := mutWithEvidence([]types.EvidenceItem{
		{ID: "e1", AnchorSymbol: "checkCoverage"},
		{ID: "e2", AnchorSymbol: "checkDAGClosure"},
		{ID: "e3", AnchorSymbol: "checkContractComplete"},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:   "list",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "checkCoverage"},
					{ID: "i2", Label: "checkDAGClosure"},
					{ID: "i3", Label: "checkContractComplete"},
				},
			},
		},
	}
	if vs := validateEnumerationItemLabelGrounding(doc, mut); len(vs) != 0 {
		t.Errorf("all-grounded labels should pass; got %+v", vs)
	}
}

// TestEnumerationLabelGrounding_HallucinatedLabelFires — the s1a
// failure mode reproduced as a unit test: 3 grounded labels +
// 2 fabricated labels (no evidence anchor); oracle reports the 2.
func TestEnumerationLabelGrounding_HallucinatedLabelFires(t *testing.T) {
	mut := mutWithEvidence([]types.EvidenceItem{
		{ID: "e1", AnchorSymbol: "checkCoverage"},
		{ID: "e2", AnchorSymbol: "checkDAGClosure"},
		{ID: "e3", AnchorSymbol: "checkContractComplete"},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:   "list",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "checkCoverage"},
					{ID: "i2", Label: "checkCrossSignalCoherence"}, // hallucinated
					{ID: "i3", Label: "checkAnswerSubjectKindIsValid"}, // hallucinated
				},
			},
		},
	}
	vs := validateEnumerationItemLabelGrounding(doc, mut)
	if len(vs) != 1 {
		t.Fatalf("expected one aggregated violation; got %d (%+v)", len(vs), vs)
	}
	if vs[0].Kind != types.ViolEnumerationLabelUngrounded {
		t.Errorf("kind = %q, want %q", vs[0].Kind, types.ViolEnumerationLabelUngrounded)
	}
	if !strings.Contains(vs[0].Detail, "checkCrossSignalCoherence") {
		t.Errorf("Detail should name first hallucinated label; got %q", vs[0].Detail)
	}
	if !strings.Contains(vs[0].Detail, "checkAnswerSubjectKindIsValid") {
		t.Errorf("Detail should name second hallucinated label; got %q", vs[0].Detail)
	}
	if !strings.Contains(vs[0].Detail, "2 enumeration item label(s)") {
		t.Errorf("Detail should report the count; got %q", vs[0].Detail)
	}
}

// TestEnumerationLabelGrounding_SubjectAndObjectAlsoSupport —
// evidence Subject and Object also count as support tokens.
func TestEnumerationLabelGrounding_SubjectAndObjectAlsoSupport(t *testing.T) {
	mut := mutWithEvidence([]types.EvidenceItem{
		{ID: "e1", Subject: "Login", Object: "AuthCheck"},
		{ID: "e2", AnchorSymbol: "Dashboard"},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:   "list",
				Kind: types.BlockBulletList,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "Login"},
					{ID: "i2", Label: "AuthCheck"},
					{ID: "i3", Label: "Dashboard"},
				},
			},
		},
	}
	if vs := validateEnumerationItemLabelGrounding(doc, mut); len(vs) != 0 {
		t.Errorf("subject/object should support items; got %+v", vs)
	}
}

// TestEnumerationLabelGrounding_NonListBlocksSkipped — scalar /
// decision / summary / section / diagram blocks are NOT subject to
// the oracle.
func TestEnumerationLabelGrounding_NonListBlocksSkipped(t *testing.T) {
	mut := mutWithEvidence([]types.EvidenceItem{
		{ID: "e1", AnchorSymbol: "answer"},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:    "summ",
				Kind:  types.BlockSummary,
				Title: "ungroundedTitle",
				Text:  "any prose here, oracle does not check this",
			},
			{
				ID:   "tab",
				Kind: types.BlockTable,
				Items: []types.AnswerBlockItem{
					{ID: "r1", Label: "ungroundedRowLabel"},
				},
			},
		},
	}
	if vs := validateEnumerationItemLabelGrounding(doc, mut); len(vs) != 0 {
		t.Errorf("non-list blocks should be skipped; got %+v", vs)
	}
}

// TestEnumerationLabelGrounding_ProseOnlyAndDiagramOnlySkipped —
// SurfaceRole gates prose_only / diagram_only out of the oracle.
func TestEnumerationLabelGrounding_ProseOnlyAndDiagramOnlySkipped(t *testing.T) {
	mut := mutWithEvidence([]types.EvidenceItem{
		{ID: "e1", AnchorSymbol: "X"},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:          "prose",
				Kind:        types.BlockBulletList,
				SurfaceRole: types.SurfaceProseOnly,
				Items: []types.AnswerBlockItem{
					{ID: "p1", Label: "ungrounded_in_prose_only"},
				},
			},
			{
				ID:          "diagOnly",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfaceDiagramOnly,
				Items: []types.AnswerBlockItem{
					{ID: "d1", Label: "ungrounded_in_diagram_only"},
				},
			},
		},
	}
	if vs := validateEnumerationItemLabelGrounding(doc, mut); len(vs) != 0 {
		t.Errorf("prose_only / diagram_only blocks should be skipped; got %+v", vs)
	}
}

// TestEnumerationLabelGrounding_EmptyLabelSkipped — empty / whitespace
// labels do not fire the oracle (the prose lives in `text`).
func TestEnumerationLabelGrounding_EmptyLabelSkipped(t *testing.T) {
	mut := mutWithEvidence([]types.EvidenceItem{
		{ID: "e1", AnchorSymbol: "X"},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:   "list",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "X"},
					{ID: "i2", Label: "  "},
					{ID: "i3", Label: ""},
					{ID: "i4", Text: "describes something but no label"},
				},
			},
		},
	}
	if vs := validateEnumerationItemLabelGrounding(doc, mut); len(vs) != 0 {
		t.Errorf("empty labels should not fire; got %+v", vs)
	}
}

// TestEnumerationLabelGrounding_NilMutDisablesOracle — nil mut means
// no evidence pool wired (unit-test mode); oracle returns no
// violations rather than false-positives on legitimate items.
func TestEnumerationLabelGrounding_NilMutDisablesOracle(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:   "list",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "anything"},
				},
			},
		},
	}
	if vs := validateEnumerationItemLabelGrounding(doc, nil); len(vs) != 0 {
		t.Errorf("nil mut should disable oracle; got %+v", vs)
	}
}

// TestEnumerationLabelGrounding_EmptyEvidencePoolSkipsOracle —
// when the evidence pool is empty the oracle skips (the LLM may
// legitimately rely on extractor-derived data only).
func TestEnumerationLabelGrounding_EmptyEvidencePoolSkipsOracle(t *testing.T) {
	mut := mutWithEvidence(nil)
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:   "list",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "fabricated_but_no_pool"},
				},
			},
		},
	}
	if vs := validateEnumerationItemLabelGrounding(doc, mut); len(vs) != 0 {
		t.Errorf("empty evidence pool should disable oracle; got %+v", vs)
	}
}

// TestEnumerationLabelGrounding_BidirectionalSubstring — a label
// that is shorter / longer than the anchor should still match
// (e.g. anchor "checkCoverage(ir,th)" supports label "checkCoverage").
func TestEnumerationLabelGrounding_BidirectionalSubstring(t *testing.T) {
	mut := mutWithEvidence([]types.EvidenceItem{
		{ID: "e1", AnchorSymbol: "checkCoverage(ir, th)"},
		{ID: "e2", AnchorSymbol: "DAGClosure"},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:   "list",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "checkCoverage"},   // shorter ⊂ anchor
					{ID: "i2", Label: "checkDAGClosure"}, // longer ⊃ anchor
				},
			},
		},
	}
	if vs := validateEnumerationItemLabelGrounding(doc, mut); len(vs) != 0 {
		t.Errorf("bidirectional substring should match; got %+v", vs)
	}
}

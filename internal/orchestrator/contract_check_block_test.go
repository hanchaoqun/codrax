package orchestrator

import (
	"fmt"
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
	if got := vs[0].ClusterKey; got != "block_kind:summary|root:answer_block_coverage" {
		t.Fatalf("missing-block violation cluster_key = %q, want block_kind:summary|root:answer_block_coverage", got)
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

func TestMissingRequestedRoleDisclosure_RequiredAndPresentPasses(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFConfigPrecedence,
		MissingRequestedRoles: []types.AnswerMissingRequestedRole{
			{Role: types.EvidenceDiagramRoleConfig, Label: "codrax.yaml"},
			{Role: types.EvidenceDiagramRoleOverride, Label: "CLI"},
		},
	}
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		MissingRequestedRoles: []types.AnswerMissingRequestedRole{
			{Role: types.EvidenceDiagramRoleConfig, Label: "codrax.yaml"},
			{Role: types.EvidenceDiagramRoleOverride, Label: "CLI"},
		},
	}
	if vs := validateMissingRequestedRoleDisclosure(doc, view); len(vs) != 0 {
		t.Fatalf("expected pass; got %+v", vs)
	}
}

func TestMissingRequestedRoleDisclosure_MissingEntryFires(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFConfigPrecedence,
		MissingRequestedRoles: []types.AnswerMissingRequestedRole{
			{Role: types.EvidenceDiagramRoleConfig, Label: "codrax.yaml"},
			{Role: types.EvidenceDiagramRoleOverride, Label: "CLI"},
		},
	}
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		MissingRequestedRoles: []types.AnswerMissingRequestedRole{
			{Role: types.EvidenceDiagramRoleConfig, Label: "codrax.yaml"},
		},
	}
	vs := validateMissingRequestedRoleDisclosure(doc, view)
	if len(vs) != 1 || vs[0].Kind != types.ViolMissingRequestedRoleUndisclosed {
		t.Fatalf("expected ViolMissingRequestedRoleUndisclosed; got %+v", vs)
	}
}

func TestMissingRequestedRoleDisclosure_OutsideExactAbsenceFires(t *testing.T) {
	view := &types.AnswerSemanticView{
		Family: types.QFGeneric,
	}
	doc := &types.AnswerDocumentV2{
		MissingRequestedRoles: []types.AnswerMissingRequestedRole{
			{Role: types.EvidenceDiagramRoleOverride, Label: "CLI"},
		},
	}
	vs := validateMissingRequestedRoleDisclosure(doc, view)
	if len(vs) != 1 || vs[0].Kind != types.ViolMissingRequestedRoleUndisclosed {
		t.Fatalf("expected ViolMissingRequestedRoleUndisclosed outside exact-absence surface; got %+v", vs)
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
	if got := vs[0].ClusterKey; got != "block:s1|root:block_claim_use" {
		t.Fatalf("principal-claim-use violation cluster_key = %q, want block:s1|root:block_claim_use", got)
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
				Kind: types.DiagramFlow,
				Body: body,
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
				// Phase 5-E1: TierExpected with SourceCandidate
				// populated (typed evidence available) — demand
				// coverage. Without SourceCandidate the gate would
				// skip; that branch is covered by
				// TestValidateFacetCoverage_ExpectedWithoutEvidenceSkipped.
				{Kind: "facet.steps", Tier: types.TierExpected, SourceCandidate: []string{"ev-1"}},
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
	if got := vs[0].ClusterKey; got != "facet:facet.steps|root:answer_facet_coverage" {
		t.Fatalf("facet-uncovered violation cluster_key = %q, want facet:facet.steps|root:answer_facet_coverage", got)
	}
}

// Phase 5-E1: Essential always demands coverage even when
// SourceCandidate is empty (analyzer template pinned it; cannot ship
// without it).
func TestValidateFacetCoverage_EssentialWithoutEvidenceStillFires(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.other"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				// Essential, empty SourceCandidate — MUST still fire.
				{Kind: "facet.essential", Tier: types.TierEssential, SourceCandidate: nil},
			},
		},
	}
	vs := validateFacetCoverage(doc, view)
	if len(vs) != 1 || vs[0].Kind != types.ViolFacetUncovered {
		t.Fatalf("Essential MUST always demand coverage; got %+v", vs)
	}
}

// Phase 5-E1: Expected with empty SourceCandidate is SKIPPED — no
// typed evidence binds to the facet; demanding coverage would force
// the LLM to invent unsupported claims (R3 noisy-signal avoidance).
func TestValidateFacetCoverage_ExpectedWithoutEvidenceSkipped(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.other"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				// Expected, empty SourceCandidate — evidence-sufficient gate skips.
				{Kind: "facet.expected", Tier: types.TierExpected, SourceCandidate: nil},
			},
		},
	}
	if vs := validateFacetCoverage(doc, view); len(vs) != 0 {
		t.Errorf("Expected without typed evidence MUST NOT fire; got %+v", vs)
	}
}

// Phase 5-E1: Expected with non-empty SourceCandidate AND covered →
// no violation (positive baseline for the gate).
func TestValidateFacetCoverage_ExpectedWithEvidenceAndCoveredPasses(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.expected"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: "facet.expected", Tier: types.TierExpected, SourceCandidate: []string{"ev-1"}},
			},
		},
	}
	if vs := validateFacetCoverage(doc, view); len(vs) != 0 {
		t.Errorf("Expected with evidence + covered MUST pass; got %+v", vs)
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
// declared via block.ClaimUses[].FacetID satisfies coverage.
func TestValidateFacetCoverage_ClaimUseFacetIDCounts(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			Kind: types.BlockOrderedList,
			ClaimUses: []types.RenderedClaimUse{
				{FacetID: "facet.steps"},
			},
			Items: []types.AnswerBlockItem{
				{Label: "step1"},
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
		t.Errorf("block.ClaimUses FacetID must satisfy coverage; got %+v", vs)
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

// G6-2: Essential Detail label surfaces "essential" — the LLM /
// operator can tell why this facet was promoted (always-hard).
func TestValidateFacetCoverage_DetailNamesEssentialPromotion(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.other"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: "facet.essential", Required: types.FacetHardRequired,
					PromotionPolicy: types.PromotionAlwaysHard, SourceCandidate: nil},
			},
		},
	}
	vs := validateFacetCoverage(doc, view)
	if len(vs) != 1 {
		t.Fatalf("want 1 violation; got %d (%+v)", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "essential") {
		t.Errorf("Detail must surface 'essential' promotion label; got %q", vs[0].Detail)
	}
}

// G6-2: Expected promoted (evidence-sufficient) Detail surfaces
// the typed evidence count.
func TestValidateFacetCoverage_DetailNamesEvidenceSufficientPromotion(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.other"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: "facet.expected", Required: types.FacetSoftRequired,
					PromotionPolicy:         types.PromotionWhenEvidenceSufficient,
					MinEvidenceForPromotion: 1,
					SourceCandidate:         []string{"ev-a", "ev-b", "ev-c"}},
			},
		},
	}
	vs := validateFacetCoverage(doc, view)
	if len(vs) != 1 {
		t.Fatalf("want 1 violation; got %d (%+v)", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "evidence-sufficient") {
		t.Errorf("Detail must surface 'evidence-sufficient' label; got %q", vs[0].Detail)
	}
	if !strings.Contains(vs[0].Detail, "3 typed evidence rows") {
		t.Errorf("Detail must surface evidence count (=3); got %q", vs[0].Detail)
	}
}

// G6-2: AdvisoryOnly facet in Required list is defensively skipped —
// this exercises the EffectivePromotionPolicy=AdvisoryOnly guard.
func TestValidateFacetCoverage_AdvisoryOnlyInRequiredSkipped(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{Kind: types.BlockSummary, FacetIDs: []string{"facet.other"}},
		},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{
				{Kind: "facet.optional_in_required", Required: types.FacetOptional,
					PromotionPolicy: types.PromotionAdvisoryOnly, SourceCandidate: []string{"ev-a"}},
			},
		},
	}
	if vs := validateFacetCoverage(doc, view); len(vs) != 0 {
		t.Errorf("AdvisoryOnly in Required must be skipped; got %+v", vs)
	}
}

// G6-2: Repair text is free of internal jargon (Go field/type names).
// Existing R4 self-check during refactor swapped "AcceptableForms"
// → "acceptable forms" and "ClaimForm" → "claim_form". Lock that.
func TestValidateFacetCoverage_RepairNoInternalJargon(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{Kind: types.BlockSummary, FacetIDs: []string{"facet.other"}},
	}}
	view := &types.AnswerSemanticView{FacetCoverage: &types.FacetCoverageContract{
		Required: []types.FacetRequirement{
			{Kind: "facet.x", Required: types.FacetHardRequired},
		},
	}}
	vs := validateFacetCoverage(doc, view)
	if len(vs) != 1 {
		t.Fatalf("want 1 violation; got %+v", vs)
	}
	for _, banned := range []string{"AcceptableForms", "ClaimForm", "FacetCoverage", "FacetRequirement", "PromotionPolicy"} {
		if strings.Contains(vs[0].Repair, banned) {
			t.Errorf("Repair contains internal jargon %q: %q", banned, vs[0].Repair)
		}
		if strings.Contains(vs[0].Detail, banned) {
			t.Errorf("Detail contains internal jargon %q: %q", banned, vs[0].Detail)
		}
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
			ClaimUses: []types.RenderedClaimUse{{
				EvidenceID: "ev1",
				ClaimForm:  types.ClaimDefinitionFact,
			}},
			Items: []types.AnswerBlockItem{{Label: "step1"}},
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
			ClaimUses: []types.RenderedClaimUse{{
				EvidenceID: "ev1",
				ClaimForm:  types.ClaimDefinitionFact, // wrong — evidence is a call
			}},
			Items: []types.AnswerBlockItem{{Label: "x"}},
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
			ClaimUses: []types.RenderedClaimUse{{
				EvidenceID: "ev1",
				ClaimForm:  types.ClaimAssignmentFact,
			}},
			Items: []types.AnswerBlockItem{{Label: "x"}},
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
			ClaimUses: []types.RenderedClaimUse{{
				EvidenceID: "ev_phantom",
				ClaimForm:  types.ClaimDefinitionFact,
			}},
			Items: []types.AnswerBlockItem{{Label: "x"}},
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
			ClaimUses: []types.RenderedClaimUse{
				{EvidenceID: "ev1"}, // no ClaimForm declared
			},
			Items: []types.AnswerBlockItem{{Label: "x"}},
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
		"",         // empty
		"resolved", // typed
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
					{ID: "i2", Label: "checkCrossSignalCoherence"},     // hallucinated
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
	if got, want := vs[0].ClusterKey, `block:list|root:block_items_label`; got != want {
		t.Errorf("ClusterKey = %q, want %q", got, want)
	}
}

func TestEnumerationLabelGrounding_MultiBlockSplitsViolationsPerBlock(t *testing.T) {
	mut := mutWithEvidence([]types.EvidenceItem{
		{ID: "e1", AnchorSymbol: "checkCoverage"},
		{ID: "e2", AnchorSymbol: "checkDAGClosure"},
		{ID: "e3", AnchorSymbol: "checkContractComplete"},
	})
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:   "listA",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "a1", Label: "checkCoverage"},
					{ID: "a2", Label: "missingA"},
				},
			},
			{
				ID:   "listB",
				Kind: types.BlockBulletList,
				Items: []types.AnswerBlockItem{
					{ID: "b1", Label: "checkDAGClosure"},
					{ID: "b2", Label: "missingB"},
				},
			},
		},
	}
	vs := validateEnumerationItemLabelGrounding(doc, mut)
	if len(vs) != 2 {
		t.Fatalf("expected one violation per affected block; got %d (%+v)", len(vs), vs)
	}
	if got, want := vs[0].ClusterKey, `block:listA|root:block_items_label`; got != want {
		t.Errorf("first ClusterKey = %q, want %q", got, want)
	}
	if got, want := vs[1].ClusterKey, `block:listB|root:block_items_label`; got != want {
		t.Errorf("second ClusterKey = %q, want %q", got, want)
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

// ── Phase 3-C2: parseMermaidEdges label capture ───────────────────

// TestParseMermaidEdges_PipeLabelCaptured — flowchart `A -->|cond| B`
// puts the relation marker between pipes; the verbatim text MUST
// land in mermaidEdge.label.
func TestParseMermaidEdges_PipeLabelCaptured(t *testing.T) {
	body := "flowchart TD\n  A -->|guard| B\n"
	got := parseMermaidEdges(body)
	if len(got) != 1 {
		t.Fatalf("want 1 edge, got %d (%+v)", len(got), got)
	}
	if got[0].label != "guard" {
		t.Errorf("label = %q, want %q", got[0].label, "guard")
	}
	if got[0].from != "A" || got[0].to != "B" {
		t.Errorf("endpoint regression: from=%q to=%q (want A→B)", got[0].from, got[0].to)
	}
}

// TestParseMermaidEdges_SequenceMessageCapturedAsLabel — sequence
// diagrams encode the relation as the trailing `: message`.
func TestParseMermaidEdges_SequenceMessageCapturedAsLabel(t *testing.T) {
	body := "sequenceDiagram\n  A->>B: handleRequest\n"
	got := parseMermaidEdges(body)
	if len(got) != 1 {
		t.Fatalf("want 1 edge, got %d", len(got))
	}
	if got[0].label != "handleRequest" {
		t.Errorf("label = %q, want %q", got[0].label, "handleRequest")
	}
	if got[0].from != "A" || got[0].to != "B" {
		t.Errorf("endpoint regression: from=%q to=%q", got[0].from, got[0].to)
	}
}

// TestParseMermaidEdges_UnlabelledEdgeHasEmptyLabel — bare arrows
// MUST yield label == "" so InferRelationFromLabel returns
// DiagramRelUnknown ("label-free edge", legitimate state).
func TestParseMermaidEdges_UnlabelledEdgeHasEmptyLabel(t *testing.T) {
	body := "flowchart LR\n  A --> B\n  X --> Y\n"
	got := parseMermaidEdges(body)
	if len(got) != 2 {
		t.Fatalf("want 2 edges, got %d", len(got))
	}
	for _, e := range got {
		if e.label != "" {
			t.Errorf("unlabelled edge has label = %q, want empty", e.label)
		}
	}
}

// TestParseMermaidEdges_MultiEdgeWithMixedLabels — labels are
// per-edge; capture must not leak across edges in the same body.
func TestParseMermaidEdges_MultiEdgeWithMixedLabels(t *testing.T) {
	body := "flowchart TD\n" +
		"  A -->|call| B\n" +
		"  B --> C\n" +
		"  C -->|guard| D\n"
	got := parseMermaidEdges(body)
	if len(got) != 3 {
		t.Fatalf("want 3 edges, got %d", len(got))
	}
	wantLabels := []string{"call", "", "guard"}
	for i, e := range got {
		if e.label != wantLabels[i] {
			t.Errorf("edge[%d] label = %q, want %q", i, e.label, wantLabels[i])
		}
	}
}

// TestParseMermaidEdges_PipeLabelStripsBeforeNodeShape — endpoint
// extraction MUST still strip node-shape brackets even when a label
// is present (`A["Label A"] -->|cond| B["Label B"]`).
func TestParseMermaidEdges_PipeLabelStripsBeforeNodeShape(t *testing.T) {
	body := "flowchart TD\n  A[\"Auth\"] -->|invoke| B[\"Worker\"]\n"
	got := parseMermaidEdges(body)
	if len(got) != 1 {
		t.Fatalf("want 1 edge, got %d", len(got))
	}
	if got[0].from != "A" || got[0].to != "B" {
		t.Errorf("endpoint regression with label present: from=%q to=%q", got[0].from, got[0].to)
	}
	if got[0].label != "invoke" {
		t.Errorf("label = %q, want invoke", got[0].label)
	}
}

// TestSplitMermaidEdgeLine_SignatureCarriesLabel — defensive
// signature test: the new 4-return form MUST be the only callable
// surface. (Compile-time check; if signature drifts back, this fails
// to build.)
func TestSplitMermaidEdgeLine_SignatureCarriesLabel(t *testing.T) {
	from, to, label, ok := splitMermaidEdgeLine("A -->|cond| B")
	if !ok || from != "A" || to != "B" || label != "cond" {
		t.Errorf("splitMermaidEdgeLine = (%q,%q,%q,%v), want (A,B,cond,true)",
			from, to, label, ok)
	}
}

// ── Phase 3-C5: validateDiagramRelationLegality (Layer 2) ─────────

// callChainViewWithDiagram returns a sequence-family view that
// requires a diagram (so EdgeRelations is populated to
// [{call, Min:1, ClaimCallEdge}]).
func callChainViewWithDiagram() *types.AnswerSemanticView {
	return types.BuildAnswerSemanticView(
		&types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentTrace,
				Scenario: types.ScenarioGeneric,
			},
		},
		&types.AnswerSurfacePlan{Diagram: &types.DiagramContract{Required: true}},
	)
}

// docWithDiagramBody constructs a minimal V2 doc holding a diagram
// block + body, with optional edge anchors. Includes an ordered_list
// block whose item labels match the test edge nodes "Auth" /
// "Worker" so Layer 1 endpoint grounding always passes — the tests
// below exercise Layer 2 (relation legality) only.
//
// Phase 1-B (V2 runtime eval followup, 2026-05-04): edge anchors
// moved from RenderedClaimUse fields to typed AnswerBlock.EdgeAnchors[].
func docWithDiagramBody(body string, edgeAnchors ...types.DiagramEdgeAnchor) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "anchors",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "n1", Label: "Auth"},
					{ID: "n2", Label: "Worker"},
				},
			},
			{
				ID:   "d1",
				Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramSequence,
					Language: "mermaid",
					Body:     body,
				},
				EdgeAnchors: edgeAnchors,
			},
		},
	}
}

// Layer 2 fires when a labelled edge has no anchored claim_use.
func TestValidateDiagramEdgeSupport_LabelledEdgeMissingClaimUseFires(t *testing.T) {
	view := callChainViewWithDiagram()
	doc := docWithDiagramBody("sequenceDiagram\n  Auth->>Worker: invoke\n")
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) == 0 {
		t.Fatal("expected violation: labelled call-edge has no anchored claim_use")
	}
	if vs[0].Kind != types.ViolDiagramEdgeUnsupported {
		t.Errorf("kind = %q, want ViolDiagramEdgeUnsupported", vs[0].Kind)
	}
	if !strings.Contains(vs[0].Detail, "call_edge") {
		t.Errorf("detail does not name expected claim_form; got %q", vs[0].Detail)
	}
}

// Layer 2 passes when an anchored edge entry covers the labelled edge.
//
// B3 v3 (2026-05-04): the anchor must declare BOTH RelationKind AND
// ClaimForm to count as typed-first. RelationKind is what the v3
// validator reads to fill EdgeRelations.Min via typed declarations;
// without it the contract is satisfied via label-only inference and
// the SOFT advisory ViolDiagramRelationLabelOnly fires.
func TestValidateDiagramEdgeSupport_LabelledEdgeWithAnchoredClaimUsePasses(t *testing.T) {
	view := callChainViewWithDiagram()
	doc := docWithDiagramBody(
		"sequenceDiagram\n  Auth->>Worker: invoke\n",
		types.DiagramEdgeAnchor{
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
			FromNode:     "Auth",
			ToNode:       "Worker",
		},
	)
	if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
		t.Errorf("typed anchored edge entry must satisfy Layer 2 cleanly; got %+v", vs)
	}
}

// Case-folded matching: edge token "auth" matches anchor "Auth".
func TestValidateDiagramEdgeSupport_AnchorMatchingIsCaseFolded(t *testing.T) {
	view := callChainViewWithDiagram()
	doc := docWithDiagramBody(
		"sequenceDiagram\n  AUTH->>WORKER: invoke\n",
		types.DiagramEdgeAnchor{
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
			FromNode:     "Auth",
			ToNode:       "worker",
		},
	)
	if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
		t.Errorf("case-folded match must succeed; got %+v", vs)
	}
}

// Unlabelled edges skip Layer 2 entirely (legitimate label-free
// state — Layer 1 endpoint grounding already passed for these).
func TestValidateDiagramEdgeSupport_UnlabelledEdgeSkipsLayer2(t *testing.T) {
	view := callChainViewWithDiagram()
	// Body has 1 unlabelled edge + 1 labelled edge with typed anchor —
	// the EdgeRelations.Min=1 contract is satisfied by the typed edge.
	doc := docWithDiagramBody(
		"sequenceDiagram\n  Auth->>Worker\n  Auth->>Worker: invoke\n",
		types.DiagramEdgeAnchor{
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
			FromNode:     "Auth",
			ToNode:       "Worker",
		},
	)
	if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
		t.Errorf("unlabelled edge must not trip Layer 2 when min satisfied; got %+v", vs)
	}
}

// Claim-form-only edge anchors now count as typed relation authority:
// when edge_anchors[] carries an edge-capable claim_form, the
// validator derives the relation kind from that typed claim instead of
// downgrading to label-only inference.
func TestValidateDiagramEdgeSupport_LabelOnlySatisfiesMinFiresAdvisory(t *testing.T) {
	view := callChainViewWithDiagram()
	doc := docWithDiagramBody(
		"sequenceDiagram\n  Auth->>Worker: invoke\n",
		types.DiagramEdgeAnchor{
			// Note: NO RelationKind set — only ClaimForm. The validator
			// should derive relation=call from ClaimCallEdge.
			ClaimForm: types.ClaimCallEdge,
			FromNode:  "Auth",
			ToNode:    "Worker",
		},
	)
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) != 0 {
		t.Fatalf("claim-form-only edge anchor should satisfy the relation contract without advisory; got %+v", vs)
	}
}

// B3 v3 (2026-05-04): typed satisfies AND extra label-only edges
// also exist for the same contract → no advisory (typed is already
// authoritative; label-only edges that fall outside the contract
// don't trigger).
func TestValidateDiagramEdgeSupport_TypedSatisfiesNoAdvisoryOnExtraLabels(t *testing.T) {
	view := callChainViewWithDiagram()
	doc := docWithDiagramBody(
		"sequenceDiagram\n  Auth->>Worker: invoke\n",
		types.DiagramEdgeAnchor{
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
			FromNode:     "Auth",
			ToNode:       "Worker",
		},
	)
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) != 0 {
		t.Errorf("typed-satisfied contract must not fire any violation; got %+v", vs)
	}
}

// EdgeRelations.Min shortfall fires its own violation.
func TestValidateDiagramEdgeSupport_MinCountShortfallFires(t *testing.T) {
	view := callChainViewWithDiagram()
	// Body has only unlabelled edges — Min=1 for relation=call not met.
	doc := docWithDiagramBody("sequenceDiagram\n  Auth->>Worker\n")
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) == 0 {
		t.Fatal("expected violation: EdgeRelations.Min not met by labelled edges")
	}
	var found bool
	for _, v := range vs {
		if strings.Contains(v.Detail, "expected at least 1") &&
			strings.Contains(v.Detail, "kind=call") {
			found = true
		}
	}
	if !found {
		t.Errorf("missing min-count violation in %+v", vs)
	}
}

// Layer 1 failure short-circuits Layer 2 — when endpoints aren't
// grounded, ask the LLM to fix the bigger problem first.
func TestValidateDiagramEdgeSupport_Layer1FailureShortCircuitsLayer2(t *testing.T) {
	view := callChainViewWithDiagram()
	// "Ghost" endpoint not in any block / item / declaration.
	doc := docWithDiagramBody("sequenceDiagram\n  Ghost->>Phantom: invoke\n")
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) != 1 {
		t.Fatalf("want exactly 1 violation (Layer 1); got %d (%+v)", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "endpoints are not grounded") {
		t.Errorf("expected Layer 1 violation; got %q", vs[0].Detail)
	}
}

// ── G3 typed RelationKind 优先 (post_v2_runtime_gap_remediation, 2026-05-04) ──

// G3 step 2: typed RelationKind on the EdgeAnchor overrides label
// inference when both differ. The legacy 1-cluster contract
// (EdgeRelations.Min=1 for relation=call) is satisfied by the typed
// declaration even when the label parses to a different relation.
func TestValidateDiagramRelationLegality_TypedRelationOverridesLabel(t *testing.T) {
	view := callChainViewWithDiagram()
	// Label "if call ready" parses to DiagramRelGuard via priority
	// (guard > call). Typed RelationKind=Call should override.
	doc := docWithDiagramBody(
		"sequenceDiagram\n  Auth->>Worker: if call ready\n",
		types.DiagramEdgeAnchor{
			FromNode:     "Auth",
			ToNode:       "Worker",
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
		},
	)
	vs := validateDiagramEdgeSupport(doc, view)
	// Must NOT fire ViolDiagramEdgeUnsupported — typed Call satisfies
	// EdgeRelations.Min=1 + the anchored claim_use is present.
	for _, v := range vs {
		if v.Kind == types.ViolDiagramEdgeUnsupported {
			t.Errorf("typed Call should satisfy EdgeRelations.Min — got unexpected: %+v", v)
		}
	}
	// MUST fire ViolDiagramEdgeLabelMismatch — typed=Call, label=Guard.
	var foundMismatch bool
	for _, v := range vs {
		if v.Kind == types.ViolDiagramEdgeLabelMismatch {
			foundMismatch = true
			if !strings.Contains(v.Detail, "call") || !strings.Contains(v.Detail, "guard") {
				t.Errorf("mismatch detail must name both relations; got %q", v.Detail)
			}
		}
	}
	if !foundMismatch {
		t.Errorf("expected ViolDiagramEdgeLabelMismatch when typed≠label; got %+v", vs)
	}
}

// G3 step 2: when typed RelationKind is set AND label is empty (or
// parses to Unknown), no mismatch fires — the typed declaration
// authoritatively drives the relation; the (absent) label is not a
// drift.
func TestValidateDiagramRelationLegality_TypedAloneNoMismatch(t *testing.T) {
	view := callChainViewWithDiagram()
	doc := docWithDiagramBody(
		"sequenceDiagram\n  Auth->>Worker\n", // unlabelled
		types.DiagramEdgeAnchor{
			FromNode:     "Auth",
			ToNode:       "Worker",
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
		},
	)
	vs := validateDiagramEdgeSupport(doc, view)
	for _, v := range vs {
		if v.Kind == types.ViolDiagramEdgeLabelMismatch {
			t.Errorf("unlabelled edge with typed relation must NOT fire mismatch; got %+v", v)
		}
	}
}

// G3 step 2: legacy label-only path stays valid when no typed
// RelationKind set. Back-compat: existing answers must keep passing.
func TestValidateDiagramRelationLegality_LabelOnlyPathPreserved(t *testing.T) {
	view := callChainViewWithDiagram()
	doc := docWithDiagramBody(
		"sequenceDiagram\n  Auth->>Worker: invoke\n",
		types.DiagramEdgeAnchor{
			FromNode:  "Auth",
			ToNode:    "Worker",
			ClaimForm: types.ClaimCallEdge,
			// RelationKind unset → fall back to label inference.
		},
	)
	vs := validateDiagramEdgeSupport(doc, view)
	for _, v := range vs {
		if v.Kind == types.ViolDiagramEdgeLabelMismatch {
			t.Errorf("typed unset must NOT fire mismatch (label-only legacy path); got %+v", v)
		}
		if v.Kind == types.ViolDiagramEdgeUnsupported {
			t.Errorf("legacy label=invoke + ClaimCallEdge anchor must satisfy Layer 2; got %+v", v)
		}
	}
}

// G3 step 2: typed RelationKind satisfies EdgeRelations.Min even when
// the label parses to Unknown (vocabulary doesn't know the word).
func TestValidateDiagramRelationLegality_TypedSatisfiesMinWithUnknownLabel(t *testing.T) {
	view := callChainViewWithDiagram()
	doc := docWithDiagramBody(
		"sequenceDiagram\n  Auth->>Worker: ☆☆☆\n", // label not in vocabulary
		types.DiagramEdgeAnchor{
			FromNode:     "Auth",
			ToNode:       "Worker",
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
		},
	)
	vs := validateDiagramEdgeSupport(doc, view)
	for _, v := range vs {
		if v.Kind == types.ViolDiagramEdgeUnsupported {
			t.Errorf("typed Call with unknown-vocabulary label must still satisfy Min; got %+v", v)
		}
	}
}

// When relation_kind is omitted but claim_form already names an
// edge-capable typed relation, the validator should treat that edge as
// typed-first rather than falling back to label vocabulary.
func TestValidateDiagramRelationLegality_ClaimFormOnlySatisfiesMinWithUnknownLabel(t *testing.T) {
	view := callChainViewWithDiagram()
	doc := docWithDiagramBody(
		"sequenceDiagram\n  Auth->>Worker: ☆☆☆\n",
		types.DiagramEdgeAnchor{
			FromNode:  "Auth",
			ToNode:    "Worker",
			ClaimForm: types.ClaimCallEdge,
		},
	)
	vs := validateDiagramEdgeSupport(doc, view)
	for _, v := range vs {
		if v.Kind == types.ViolDiagramEdgeUnsupported {
			t.Errorf("claim_form-only Call edge with unknown-vocabulary label must still satisfy Min; got %+v", v)
		}
	}
}

// G3 step 2: ViolDiagramEdgeLabelMismatch is permanently SOFT — even
// when the operator promotes via pipeline_contract_strict_kinds, the
// validator emits SOFT (no STRICT route). Locked via DeriveSeverity
// returning SeveritySoft regardless of isStrict.
func TestViolDiagramEdgeLabelMismatch_PermanentlySoft(t *testing.T) {
	for _, isStrict := range []bool{false, true} {
		got := types.DeriveSeverity(types.ViolDiagramEdgeLabelMismatch, isStrict)
		if got != types.SeveritySoft {
			t.Errorf("isStrict=%v: DeriveSeverity = %v, want SeveritySoft (permanent SOFT, R3 noisy-signal red line)", isStrict, got)
		}
	}
}

// ── 修 B: validateEnumerationItemLabelExtractorMatch lock tests ──

// helper: build a minimal V2 doc with an ordered_list block carrying
// the supplied item labels.
func docWithEnumItems(blockID string, labels ...string) *types.AnswerDocumentV2 {
	items := make([]types.AnswerBlockItem, len(labels))
	for i, l := range labels {
		items[i] = types.AnswerBlockItem{ID: fmt.Sprintf("i%d", i+1), Label: l}
	}
	return &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: blockID, Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal, Items: items},
		},
	}
}

// helper: MutableState with N AnswerSymbols.
func mutWithSymbols(names ...string) *types.MutableState {
	mut := &types.MutableState{}
	syms := make([]types.AnswerSymbol, len(names))
	for i, n := range names {
		syms[i] = types.AnswerSymbol{Name: n, File: "x.go", Line: i + 1, Kind: types.KindMethod}
	}
	mut.SetEmittedAnswerSymbols(syms, types.CompletenessComplete)
	return mut
}

func enumView() *types.AnswerSemanticView {
	return &types.AnswerSemanticView{Family: types.QFEnumeration}
}

func boundedCallChainView() *types.AnswerSemanticView {
	return &types.AnswerSemanticView{
		Family: types.QFCallChain,
		FacetCoverage: &types.FacetCoverageContract{
			Family: types.QFCallChain,
			Required: []types.FacetRequirement{
				{Kind: types.FacetEnumerationItem, Required: types.FacetHardRequired},
			},
		},
	}
}

// All 9 items[].label literal-equal AnswerSymbols → no violation.
func TestEnumerationItemLabelExtractorMatch_AllVerbatimPasses(t *testing.T) {
	mut := mutWithSymbols("checkCoverage", "checkDAGClosure", "checkBudgetSanity",
		"checkContractComplete", "checkHypothesisCoverage", "checkSubtopicCoherence",
		"checkShapeSubjectCoherence", "checkCriterionResolvable", "checkPendingFieldsWellformed")
	doc := docWithEnumItems("list1",
		"checkCoverage", "checkDAGClosure", "checkBudgetSanity",
		"checkContractComplete", "checkHypothesisCoverage", "checkSubtopicCoherence",
		"checkShapeSubjectCoherence", "checkCriterionResolvable", "checkPendingFieldsWellformed")
	if vs := validateEnumerationItemLabelExtractorMatch(doc, enumView(), mut); len(vs) != 0 {
		t.Errorf("verbatim labels MUST pass; got %+v", vs)
	}
}

// s1a-2 reproduction: items[].label = "check 1 (gate.go:148)" abstract
// placeholders, AnswerSymbols are the 9 real method names → fires.
func TestEnumerationItemLabelExtractorMatch_AbstractPlaceholdersFire(t *testing.T) {
	mut := mutWithSymbols("checkCoverage", "checkDAGClosure", "checkBudgetSanity",
		"checkContractComplete", "checkHypothesisCoverage", "checkSubtopicCoherence",
		"checkShapeSubjectCoherence", "checkCriterionResolvable", "checkPendingFieldsWellformed")
	doc := docWithEnumItems("list1",
		"check 1 (gate.go:148)", "check 2 (gate.go:149)", "check 3 (gate.go:150)",
		"check 4 (gate.go:156)", "check 5 (gate.go:157)", "check 6 (gate.go:168)",
		"check 7 (gate.go:169)", "check 8 (gate.go:171)", "check 9 (gate.go:172)")
	vs := validateEnumerationItemLabelExtractorMatch(doc, enumView(), mut)
	if len(vs) != 1 {
		t.Fatalf("abstract placeholder labels MUST fire; got %d violations", len(vs))
	}
	if vs[0].Kind != types.ViolEnumerationItemLabelExtractorDrift {
		t.Errorf("kind = %q, want ViolEnumerationItemLabelExtractorDrift", vs[0].Kind)
	}
	if !strings.Contains(vs[0].Detail, "checkCoverage") {
		t.Errorf("detail must list verbatim names; got %q", vs[0].Detail)
	}
	if got, want := vs[0].ClusterKey, `block:list1|root:block_items_label`; got != want {
		t.Errorf("ClusterKey = %q, want %q", got, want)
	}
}

func TestEnumerationItemLabelExtractorMatch_MultiBlockSplitsViolationsPerBlock(t *testing.T) {
	mut := mutWithSymbols(
		"checkCoverage", "checkDAGClosure", "checkBudgetSanity",
		"checkContractComplete", "checkHypothesisCoverage", "checkSubtopicCoherence",
	)
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:   "list1",
				Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "check 1"},
					{ID: "i2", Label: "check 2"},
					{ID: "i3", Label: "check 3"},
				},
			},
			{
				ID:   "list2",
				Kind: types.BlockBulletList,
				Items: []types.AnswerBlockItem{
					{ID: "j1", Label: "check 4"},
					{ID: "j2", Label: "check 5"},
					{ID: "j3", Label: "check 6"},
				},
			},
		},
	}
	vs := validateEnumerationItemLabelExtractorMatch(doc, enumView(), mut)
	if len(vs) != 2 {
		t.Fatalf("expected one violation per drifted block; got %d (%+v)", len(vs), vs)
	}
	if got, want := vs[0].ClusterKey, `block:list1|root:block_items_label`; got != want {
		t.Errorf("first ClusterKey = %q, want %q", got, want)
	}
	if got, want := vs[1].ClusterKey, `block:list2|root:block_items_label`; got != want {
		t.Errorf("second ClusterKey = %q, want %q", got, want)
	}
}

// Relaxed substring match: "checkCoverage — 资源检查" contains the
// verbatim name → counts as match.
func TestEnumerationItemLabelExtractorMatch_RelaxedSubstringPasses(t *testing.T) {
	mut := mutWithSymbols("checkCoverage", "checkDAGClosure", "checkBudgetSanity")
	doc := docWithEnumItems("list1",
		"checkCoverage — 资源检查", "checkDAGClosure (closure check)", "checkBudgetSanity")
	if vs := validateEnumerationItemLabelExtractorMatch(doc, enumView(), mut); len(vs) != 0 {
		t.Errorf("substring containment MUST pass; got %+v", vs)
	}
}

// Skip: family is not enumeration-shaped.
func TestEnumerationItemLabelExtractorMatch_NonEnumFamilySkipped(t *testing.T) {
	mut := mutWithSymbols("X", "Y", "Z")
	doc := docWithEnumItems("list1", "abstract 1", "abstract 2", "abstract 3")
	view := &types.AnswerSemanticView{Family: types.QFGeneric}
	if vs := validateEnumerationItemLabelExtractorMatch(doc, view, mut); len(vs) != 0 {
		t.Errorf("non-enumeration family MUST skip; got %+v", vs)
	}
}

func TestEnumerationItemLabelExtractorMatch_PlainCallChainSkipped(t *testing.T) {
	mut := mutWithSymbols("stepA", "stepB", "stepC")
	doc := docWithEnumItems("list1", "placeholder 1", "placeholder 2", "placeholder 3")
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	if vs := validateEnumerationItemLabelExtractorMatch(doc, view, mut); len(vs) != 0 {
		t.Errorf("plain call-chain without bounded enumeration facet MUST skip; got %+v", vs)
	}
}

func TestEnumerationItemLabelExtractorMatch_BoundedCallChainStillChecksVerbatimNames(t *testing.T) {
	mut := mutWithSymbols("stepA", "stepB", "stepC")
	doc := docWithEnumItems("list1", "placeholder 1", "placeholder 2", "placeholder 3")
	vs := validateEnumerationItemLabelExtractorMatch(doc, boundedCallChainView(), mut)
	if len(vs) != 1 || vs[0].Kind != types.ViolEnumerationItemLabelExtractorDrift {
		t.Fatalf("bounded call-chain should still enforce extractor-backed labels; got %+v", vs)
	}
}

// Skip: less than 3 AnswerSymbols (small slate, noise-prone).
func TestEnumerationItemLabelExtractorMatch_SmallSymbolSetSkipped(t *testing.T) {
	mut := mutWithSymbols("A", "B")
	doc := docWithEnumItems("list1", "abstract 1", "abstract 2", "abstract 3")
	if vs := validateEnumerationItemLabelExtractorMatch(doc, enumView(), mut); len(vs) != 0 {
		t.Errorf("symbols<3 MUST skip; got %+v", vs)
	}
}

// 80% threshold: 8/10 items match → passes; 7/10 → fires.
// Use distinguishable identifiers so substring containment doesn't
// false-positive on the placeholder labels.
func TestEnumerationItemLabelExtractorMatch_EightyPercentThreshold(t *testing.T) {
	mut := mutWithSymbols("alphaFn", "betaFn", "gammaFn", "deltaFn", "epsilonFn",
		"zetaFn", "etaFn", "thetaFn", "iotaFn", "kappaFn")
	// 8 verbatim + 2 unrelated → 80% — passes
	doc8 := docWithEnumItems("list1",
		"alphaFn", "betaFn", "gammaFn", "deltaFn", "epsilonFn",
		"zetaFn", "etaFn", "thetaFn", "row 1 (xyz)", "row 2 (xyz)")
	if vs := validateEnumerationItemLabelExtractorMatch(doc8, enumView(), mut); len(vs) != 0 {
		t.Errorf("80%% match MUST pass; got %+v", vs)
	}
	// 7 verbatim + 3 unrelated → 70% — fires
	doc7 := docWithEnumItems("list1",
		"alphaFn", "betaFn", "gammaFn", "deltaFn", "epsilonFn",
		"zetaFn", "etaFn", "row 1 (xyz)", "row 2 (xyz)", "row 3 (xyz)")
	if vs := validateEnumerationItemLabelExtractorMatch(doc7, enumView(), mut); len(vs) == 0 {
		t.Errorf("<80%% match MUST fire")
	}
}

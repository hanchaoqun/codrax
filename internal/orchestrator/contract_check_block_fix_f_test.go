package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// docWithDiagramKind builds a V2 doc with a single BlockDiagram whose
// kind is the supplied DiagramKind. Used by Fix F tests that need to
// vary the kind to exercise the code-context gate.
func docWithDiagramKind(blockID, body string, kind types.DiagramKind) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          blockID,
				Kind:        types.BlockDiagram,
				SurfaceRole: types.SurfacePrincipal,
				Diagram: &types.AnswerDiagramBlock{
					Kind: kind,
					Body: body,
				},
			},
		},
	}
}

// docWithDiagramAndEdgeAnchors builds a V2 doc carrying typed
// EdgeAnchors so the per-edge RelationKind path of the Fix F gate
// can be exercised independently of the diagram-kind path. Each
// anchor maps a (from, to) pair to a typed RelationKind.
func docWithDiagramAndEdgeAnchors(blockID, body string, kind types.DiagramKind, anchors []types.DiagramEdgeAnchor) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          blockID,
				Kind:        types.BlockDiagram,
				SurfaceRole: types.SurfacePrincipal,
				Diagram: &types.AnswerDiagramBlock{
					Kind: kind,
					Body: body,
				},
				EdgeAnchors: anchors,
			},
		},
	}
}

// TestFixF_M1aR1Reproduction pins the exact false-positive scenario
// that motivated Fix F. m1a r1 produced an architecture diagram
// with 7 abstract role-name endpoints (`ExplorerAgent`,
// `EmitEvidence_TA`, etc.) connected by untyped edges. Fix D
// initially reported all 7 as hallucinations because the names
// were ≥10 chars + identifier-shape and oracle Tier 0. Fix F
// recognises that DiagramFlow / DiagramArchitecture diagrams
// without typed `call` relations are role-context, not
// code-context, and skips the gate.
func TestFixF_M1aR1Reproduction_FlowKindWithoutTypedRelationsSkips(t *testing.T) {
	body := "graph TD\n" +
		"    ExplorerAgent --> EmitEvidence_TA\n" +
		"    ExplorerAgent --> EmitInvestigationComplete_TA\n" +
		"    ExtractorAgent --> EmitEvidence_TB\n" +
		"    AnalyzeStage --> ExplorerAgent\n" +
		"    ExtractorAgent --> FinalizerStage\n"
	doc := docWithDiagramKind("arch_diagram", body, types.DiagramFlow)
	// All endpoints absent from oracle (they're abstract roles, not
	// codebase symbols). Without Fix F, every endpoint would fire
	// hallucinated. With Fix F, the diagram kind is `flow` AND no
	// edge declared `relation_kind=call`, so the whole block skips.
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateDiagramEdgeEndpointHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("flow-kind diagram with role-label endpoints MUST skip Fix F gate; got %d violations:\n  %+v", len(vs), vs)
	}
}

// TestFixF_CallDAGKind_FiresOnHallucinations confirms the diagram-
// kind branch of Fix F: when the LLM declares the diagram as a
// CallDAG, every edge endpoint is in code context (regardless of
// whether the edge has a typed RelationKind), so fabricated
// identifiers fire.
func TestFixF_CallDAGKind_FiresOnHallucinations(t *testing.T) {
	body := "graph TD\n" +
		"    realCheckCoverage --> validateFakeCoherenceCheck\n"
	doc := docWithDiagramKind("call_dag", body, types.DiagramCallDAG)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"realCheckCoverage": 1,
	}}
	vs := validateDiagramEdgeEndpointHallucination(doc, oracle)
	if len(vs) != 1 {
		t.Fatalf("CallDAG-kind hallucination MUST fire; got %d:\n  %+v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "validateFakeCoherenceCheck") {
		t.Errorf("violation must list the hallucinated endpoint; got: %s", vs[0].Detail)
	}
}

func TestFixF_CallDAGKind_ExplicitDisplayLabelSkipsInternalAlias(t *testing.T) {
	body := "graph TD\n" +
		"    readNode[\"读取文件\"] --> check_size[\"行数检查\\nDEFAULT_READ_LIMIT=2000\"]\n" +
		"    check_size --> check_bytes[\"字节检查\\nMAX_BYTES=50KB\"]\n"
	doc := docWithDiagramKind("call_dag", body, types.DiagramCallDAG)
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateDiagramEdgeEndpointHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("Mermaid internal node ids with explicit non-code display labels must not force finalizer rewrite; got %+v", vs)
	}
}

func TestFixF_CallDAGKind_ExplicitCodeLabelStillFires(t *testing.T) {
	body := "graph TD\n" +
		"    n1[\"realCheckCoverage\"] --> n2[\"validateFakeCoherenceCheck\"]\n"
	doc := docWithDiagramKind("call_dag", body, types.DiagramCallDAG)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"realCheckCoverage": 1,
	}}
	vs := validateDiagramEdgeEndpointHallucination(doc, oracle)
	if len(vs) != 1 {
		t.Fatalf("explicit code-shaped display-label hallucination must still fire; got %d:\n  %+v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "validateFakeCoherenceCheck") {
		t.Errorf("violation must list visible fake label; got: %s", vs[0].Detail)
	}
}

// TestFixF_FlowKindWithTypedCallRelation_Skips confirms the
// 2026-05-18 intent-preservation refinement: relation_kind=call is
// not, by itself, a precise enough signal to treat a flow/architecture
// diagram as code-endpoint context. Component-level "calls" are common
// in presentation diagrams, and forcing every endpoint through the
// symbol oracle causes finalizer rewrites for valid role diagrams.
func TestFixF_FlowKindWithTypedCallRelation_Skips(t *testing.T) {
	body := "graph TD\n" +
		"    realCheckCoverage --> validateFakeCoherenceCheck\n" +
		"    AbstractRoleNode --> AnotherAbstractRole\n"
	// Only the first edge declared as a call relation; the second
	// edge is left untyped (abstract role flow).
	anchors := []types.DiagramEdgeAnchor{
		{
			FromNode:     "realCheckCoverage",
			ToNode:       "validateFakeCoherenceCheck",
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
		},
	}
	doc := docWithDiagramAndEdgeAnchors("flow_with_call", body, types.DiagramFlow, anchors)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"realCheckCoverage": 1,
		// AbstractRoleNode / AnotherAbstractRole / validateFakeCoherenceCheck absent.
	}}
	vs := validateDiagramEdgeEndpointHallucination(doc, oracle)
	if len(vs) != 0 {
		t.Fatalf("flow-kind typed-call presentation diagram must not force symbol-oracle endpoint checks; got %d:\n  %+v", len(vs), vs)
	}
}

// TestFixF_FlowKindWithImportRelation_Skips confirms that non-call
// typed relations (Import / Contain / Guard / Precedence /
// Observe) do NOT enter code context, even though they are typed.
// Their endpoints are packages / role labels / states, not
// function names — applying the oracle gate would false-positive.
func TestFixF_FlowKindWithImportRelation_Skips(t *testing.T) {
	body := "graph TD\n    pkgFooHelper --> pkgBarHelper\n"
	anchors := []types.DiagramEdgeAnchor{
		{
			FromNode:     "pkgFooHelper",
			ToNode:       "pkgBarHelper",
			RelationKind: types.DiagramRelImport,
			ClaimForm:    types.ClaimImportEdge,
		},
	}
	doc := docWithDiagramAndEdgeAnchors("import_graph", body, types.DiagramFlow, anchors)
	// Both names absent from oracle (they're package paths, not Go symbols).
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateDiagramEdgeEndpointHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("import-relation edges MUST skip Fix F gate (pkg paths != Go symbols); got %+v", vs)
	}
}

// TestFixF_ArchitectureKindWithoutTypedRelations_Skips confirms
// the architecture diagram surface skips entirely when no edge
// declares `relation_kind=call`. Architecture nodes are subsystem
// names / role labels by convention — they're not Go symbols.
func TestFixF_ArchitectureKindWithoutTypedRelations_Skips(t *testing.T) {
	body := "graph TD\n" +
		"    AnalyzerSubsystem --> ExplorerSubsystem\n" +
		"    ExplorerSubsystem --> ExtractorSubsystem\n"
	doc := docWithDiagramKind("arch", body, types.DiagramArchitecture)
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateDiagramEdgeEndpointHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("untyped architecture diagram MUST skip; got %+v", vs)
	}
}

// TestFixF_SequenceKindWithoutTypedRelations_Skips confirms
// sequence diagrams (actor-to-actor message flows) skip the gate.
func TestFixF_SequenceKindWithoutTypedRelations_Skips(t *testing.T) {
	body := "sequenceDiagram\n" +
		"    UserActorAlpha->>SystemActorBeta: requestPayload\n"
	doc := docWithDiagramKind("seq", body, types.DiagramSequence)
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateDiagramEdgeEndpointHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("untyped sequence diagram MUST skip; got %+v", vs)
	}
}

// TestFixF_CallDAGKind_TypedCallEdgeStillFires_AdditiveSemantics
// confirms that the two Fix F signals are additive (OR), not
// exclusive: a CallDAG-kind diagram with typed call edges still
// applies the gate (no double-counting / no signal cancellation).
func TestFixF_CallDAGKind_TypedCallEdgeStillFires_AdditiveSemantics(t *testing.T) {
	body := "graph TD\n    realCheckCoverage --> fabricatedCheckLogic\n"
	anchors := []types.DiagramEdgeAnchor{
		{
			FromNode:     "realCheckCoverage",
			ToNode:       "fabricatedCheckLogic",
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
		},
	}
	doc := docWithDiagramAndEdgeAnchors("call_dag", body, types.DiagramCallDAG, anchors)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"realCheckCoverage": 1,
	}}
	vs := validateDiagramEdgeEndpointHallucination(doc, oracle)
	if len(vs) != 1 {
		t.Fatalf("CallDAG + typed-call hallucination MUST fire; got %d:\n  %+v", len(vs), vs)
	}
}

// TestFixF_FlowKindMixedEdges_AllPresentationEdgesSkip covers a
// mixed-edge flow diagram. Even when one edge carries relation_kind=call,
// the diagram's semantic kind remains presentation/role context; only
// DiagramCallDAG asserts code endpoints strongly enough for a hard
// hallucination gate.
func TestFixF_FlowKindMixedEdges_AllPresentationEdgesSkip(t *testing.T) {
	body := "graph TD\n" +
		"    realCheckCoverage --> fabricatedCallTarget\n" + // typed call, but flow kind → presentation skip
		"    StartEventNode --> ProcessingState\n" + // untyped → skip
		"    ProcessingState --> EndEventNode\n" // untyped → skip
	anchors := []types.DiagramEdgeAnchor{
		{
			FromNode:     "realCheckCoverage",
			ToNode:       "fabricatedCallTarget",
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
		},
	}
	doc := docWithDiagramAndEdgeAnchors("mixed_flow", body, types.DiagramFlow, anchors)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"realCheckCoverage": 1,
		// fabricatedCallTarget / StartEventNode / ProcessingState / EndEventNode all absent
	}}
	vs := validateDiagramEdgeEndpointHallucination(doc, oracle)
	if len(vs) != 0 {
		t.Fatalf("mixed flow presentation diagram must not force endpoint symbol checks; got %d:\n  %+v", len(vs), vs)
	}
}

// TestFixF_IsCodeContextDiagramKind_Table pins the diagram-kind
// branch of the Fix F decision.
func TestFixF_IsCodeContextDiagramKind_Table(t *testing.T) {
	cases := []struct {
		kind types.DiagramKind
		want bool
		note string
	}{
		{types.DiagramCallDAG, true, "call_dag is the only code-context kind by declaration"},
		{types.DiagramFlow, false, "flow nodes are typically state/role labels"},
		{types.DiagramSequence, false, "sequence nodes are actors, often roles"},
		{types.DiagramArchitecture, false, "architecture nodes are subsystem names / roles"},
		{types.DiagramNone, false, "no diagram kind declared → not code context"},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			if got := isCodeContextDiagramKind(tc.kind); got != tc.want {
				t.Errorf("isCodeContextDiagramKind(%q) = %v, want %v\n  note: %s",
					tc.kind, got, tc.want, tc.note)
			}
		})
	}
}

func TestDiagramKindBodyCoherence_SequenceBodyRequiresSequenceKind(t *testing.T) {
	doc := docWithDiagramKind(
		"seq",
		"sequenceDiagram\n",
		types.DiagramArchitecture,
	)
	doc.Blocks[0].Diagram.Language = "mermaid"
	view := &types.AnswerSemanticView{
		DiagramPlan: &types.DiagramFacetGraph{
			Kind:     types.DiagramArchitecture,
			Required: false,
		},
	}
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) != 1 {
		t.Fatalf("sequence body + architecture kind must fail exactly once, got %+v", vs)
	}
	if vs[0].Kind != types.ViolDiagramEdgeUnsupported {
		t.Fatalf("kind = %q, want %q", vs[0].Kind, types.ViolDiagramEdgeUnsupported)
	}
	if !strings.Contains(vs[0].Detail, "sequence") || !strings.Contains(vs[0].Detail, "architecture") {
		t.Fatalf("detail should explain kind/body mismatch, got %q", vs[0].Detail)
	}
}

func TestDiagramKindBodyCoherence_FlowBodyAllowsArchitectureKind(t *testing.T) {
	doc := docWithDiagramKind(
		"arch",
		"flowchart TD\n",
		types.DiagramArchitecture,
	)
	doc.Blocks[0].Diagram.Language = "mermaid"
	view := &types.AnswerSemanticView{
		DiagramPlan: &types.DiagramFacetGraph{
			Kind:     types.DiagramArchitecture,
			Required: false,
		},
	}
	if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
		t.Fatalf("flowchart syntax is valid for architecture semantic diagrams; got %+v", vs)
	}
}

func TestDiagramKindBodyCoherence_FlowBodyRejectsSequenceKind(t *testing.T) {
	doc := docWithDiagramKind(
		"flow",
		"graph LR\n",
		types.DiagramSequence,
	)
	doc.Blocks[0].Diagram.Language = "mermaid"
	view := &types.AnswerSemanticView{
		DiagramPlan: &types.DiagramFacetGraph{
			Kind:     types.DiagramSequence,
			Required: false,
		},
	}
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) != 1 {
		t.Fatalf("flow body + sequence kind must fail exactly once, got %+v", vs)
	}
	if !strings.Contains(vs[0].Detail, "flow") || !strings.Contains(vs[0].Detail, "sequence") {
		t.Fatalf("detail should explain kind/body mismatch, got %q", vs[0].Detail)
	}
}

func TestDiagramKindBodyCoherence_UnsupportedKnownMermaidDirectiveFails(t *testing.T) {
	doc := docWithDiagramKind(
		"class",
		"classDiagram\n  class AnalyzerAgent\n",
		types.DiagramArchitecture,
	)
	doc.Blocks[0].Diagram.Language = "mermaid"
	view := &types.AnswerSemanticView{
		DiagramPlan: &types.DiagramFacetGraph{
			Kind:     types.DiagramArchitecture,
			Required: false,
		},
	}
	vs := validateDiagramEdgeSupport(doc, view)
	if len(vs) != 1 {
		t.Fatalf("known unsupported Mermaid syntax must fail exactly once, got %+v", vs)
	}
	if !strings.Contains(vs[0].Detail, "unsupported") {
		t.Fatalf("detail should name unsupported syntax family, got %q", vs[0].Detail)
	}
	if !strings.Contains(vs[0].Repair, "flowchart/graph") {
		t.Fatalf("repair should direct the model to a supported carrier syntax, got %q", vs[0].Repair)
	}
}

func TestFixF_CallRelationDoesNotUpgradeArchitectureDiagram(t *testing.T) {
	body := "graph TD\n" +
		"    ResSchedService --> ResSchedMgr\n" +
		"    ResSchedMgr --> PluginMgr\n"
	anchors := []types.DiagramEdgeAnchor{
		{
			FromNode:     "ResSchedService",
			ToNode:       "ResSchedMgr",
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
		},
		{
			FromNode:     "ResSchedMgr",
			ToNode:       "PluginMgr",
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimCallEdge,
		},
	}
	doc := docWithDiagramAndEdgeAnchors("arch_component_calls", body, types.DiagramArchitecture, anchors)
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateDiagramEdgeEndpointHallucination(doc, oracle); len(vs) != 0 {
		t.Fatalf("architecture component-call diagram must not be treated as a code call DAG; got %+v", vs)
	}
}

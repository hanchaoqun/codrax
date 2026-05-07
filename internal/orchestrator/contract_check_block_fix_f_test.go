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

// TestFixF_FlowKindWithTypedCallRelation_Fires confirms the
// edge-level branch of Fix F: even when the diagram kind is
// non-call (here Flow), an individual edge that the LLM typed as
// `relation_kind=call` enters code context for THAT edge — so
// hallucinations on that edge fire.
func TestFixF_FlowKindWithTypedCallRelation_Fires(t *testing.T) {
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
	if len(vs) != 1 {
		t.Fatalf("typed-call edge hallucination MUST fire; got %d:\n  %+v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "validateFakeCoherenceCheck") {
		t.Errorf("violation must list the hallucinated CALL endpoint; got: %s", vs[0].Detail)
	}
	if strings.Contains(vs[0].Detail, "AbstractRoleNode") || strings.Contains(vs[0].Detail, "AnotherAbstractRole") {
		t.Errorf("untyped abstract-role endpoints MUST NOT fire (Fix F per-edge skip); got: %s", vs[0].Detail)
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

// TestFixF_FlowKindMixedEdges_ScopedToCallEdgesOnly is the most
// nuanced case: a single flow-kind diagram with multiple edges,
// some typed as `call` and some untyped. The Fix F gate applies
// per-edge, so hallucinations on `call` edges fire and abstract
// role labels on untyped edges are tolerated — even within the
// same diagram block.
func TestFixF_FlowKindMixedEdges_ScopedToCallEdgesOnly(t *testing.T) {
	body := "graph TD\n" +
		"    realCheckCoverage --> fabricatedCallTarget\n" + // typed call → check
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
	if len(vs) != 1 {
		t.Fatalf("mixed-flow Fix F: only typed-call edge hallucination should fire; got %d:\n  %+v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "fabricatedCallTarget") {
		t.Errorf("violation must list the typed-call hallucination; got: %s", vs[0].Detail)
	}
	for _, untyped := range []string{"StartEventNode", "ProcessingState", "EndEventNode"} {
		if strings.Contains(vs[0].Detail, untyped) {
			t.Errorf("untyped flow node %q MUST NOT fire (per-edge code-context skip); got: %s",
				untyped, vs[0].Detail)
		}
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

// TestFixF_IsCodeContextRelationKind_Table pins the relation-kind
// branch.
func TestFixF_IsCodeContextRelationKind_Table(t *testing.T) {
	cases := []struct {
		rel  types.DiagramRelationKind
		want bool
		note string
	}{
		{types.DiagramRelCall, true, "call edges connect function/method endpoints"},
		{types.DiagramRelImport, false, "import edges connect package paths, not Go symbols"},
		{types.DiagramRelContain, false, "contain edges connect subsystems/roles, not Go symbols"},
		{types.DiagramRelGuard, false, "guard edges are conditions, endpoints are flow states"},
		{types.DiagramRelPrecedence, false, "precedence edges are config/role layering"},
		{types.DiagramRelObserve, false, "observe edges connect log frames to nodes"},
		{types.DiagramRelUnknown, false, "unknown / unlabelled edges have no typed assertion"},
	}
	for _, tc := range cases {
		t.Run(string(tc.rel)+"_relation", func(t *testing.T) {
			if got := isCodeContextRelationKind(tc.rel); got != tc.want {
				t.Errorf("isCodeContextRelationKind(%q) = %v, want %v\n  note: %s",
					tc.rel, got, tc.want, tc.note)
			}
		})
	}
}

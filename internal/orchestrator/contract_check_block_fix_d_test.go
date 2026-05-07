package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// docWithDiagram builds a minimal V2 doc carrying a single
// BlockDiagram with the supplied mermaid body.
func docWithDiagram(blockID, body string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          blockID,
				Kind:        types.BlockDiagram,
				SurfaceRole: types.SurfacePrincipal,
				Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramFlow,
					Body: body,
				},
			},
		},
	}
}

// TestDiagramEdgeEndpointHallucination_FabricatedNodeFires pins the
// production-shape scenario: LLM emits a mermaid edge whose endpoint
// is a long compound identifier that no source file declares (e.g.
// `validateFakeCoherenceCheck`). The graph oracle Tier=0 verdict
// fires the violation even though the existing
// validateDiagramEdgeSupport's substring vouch may have passed.
func TestDiagramEdgeEndpointHallucination_FabricatedNodeFires(t *testing.T) {
	body := "graph TD\n" +
		"    realCheckCoverage --> validateFakeCoherenceCheck\n" +
		"    validateFakeCoherenceCheck --> realCheckCoverage\n"
	doc := docWithDiagram("call_dag", body)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"realCheckCoverage": 1,
		// validateFakeCoherenceCheck absent → Tier 0 → fire
	}}
	vs := validateDiagramEdgeEndpointHallucination(doc, oracle)
	if len(vs) != 1 {
		t.Fatalf("expected 1 violation; got %d:\n  %+v", len(vs), vs)
	}
	if vs[0].Kind != types.ViolDiagramEdgeEndpointHallucinated {
		t.Errorf("kind = %q, want ViolDiagramEdgeEndpointHallucinated", vs[0].Kind)
	}
	if !strings.Contains(vs[0].Detail, "validateFakeCoherenceCheck") {
		t.Errorf("violation must list the hallucinated endpoint; got: %s", vs[0].Detail)
	}
	// Dedupe: even though the fake name appears in 2 edges (forward
	// + reverse), it should be reported as a single pair per block
	// — count distinct `ident="..."` occurrences, not raw substrings,
	// because each pair contains the name twice (ident + endpoint
	// when they're identical for bare mermaid identifiers).
	if strings.Count(vs[0].Detail, `ident="validateFakeCoherenceCheck"`) != 1 {
		t.Errorf("hallucinated endpoint should appear in exactly one pair per block; got Detail=%s", vs[0].Detail)
	}
}

// TestDiagramEdgeEndpointHallucination_AllRealSymbolsPass confirms
// the validator does NOT fire when every endpoint is a graph-
// confirmed Tier 1-2 symbol.
func TestDiagramEdgeEndpointHallucination_AllRealSymbolsPass(t *testing.T) {
	body := "graph TD\n" +
		"    realCheckCoverage --> realCheckBudgetSanity\n" +
		"    realCheckBudgetSanity --> realCheckDAGClosure\n"
	doc := docWithDiagram("call_dag", body)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"realCheckCoverage":     1,
		"realCheckBudgetSanity": 1,
		"realCheckDAGClosure":   1,
	}}
	if vs := validateDiagramEdgeEndpointHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("real graph-confirmed endpoints MUST pass; got %+v", vs)
	}
}

// TestDiagramEdgeEndpointHallucination_NilOracleSkips confirms the
// validator is dormant when no oracle is wired (unit-test path /
// no-graph runs).
func TestDiagramEdgeEndpointHallucination_NilOracleSkips(t *testing.T) {
	body := "graph TD\n    fakeFunctionName --> anotherFakeOne\n"
	doc := docWithDiagram("call_dag", body)
	if vs := validateDiagramEdgeEndpointHallucination(doc, nil); len(vs) != 0 {
		t.Errorf("nil oracle MUST disable validator; got %+v", vs)
	}
}

// TestDiagramEdgeEndpointHallucination_ShortIdentSkipsFloor confirms
// the length floor: short mermaid stub identifiers (`A`, `Foo`,
// `node1`) bypass the validator. These are typically mermaid scaffold
// node ids, not source-file symbol references.
func TestDiagramEdgeEndpointHallucination_ShortIdentSkipsFloor(t *testing.T) {
	body := "graph TD\n" +
		"    A[Analyzer] --> B[Explorer]\n" +
		"    B --> Foo\n" +
		"    Foo --> node1\n"
	doc := docWithDiagram("flow", body)
	// Empty oracle would fail every endpoint, but all are below the
	// 10-char floor, so none should fire.
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateDiagramEdgeEndpointHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("short identifiers MUST skip the gate; got %+v", vs)
	}
}

// TestDiagramEdgeEndpointHallucination_NonDiagramBlocksSkip
// confirms only BlockDiagram blocks are scanned.
func TestDiagramEdgeEndpointHallucination_NonDiagramBlocksSkip(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID: "summary1", Kind: types.BlockSummary,
				Text: "A diagram with fakeFunctionName --> anotherFakeOne would not be scanned here",
			},
			{
				ID: "list1", Kind: types.BlockOrderedList,
				Items: []types.AnswerBlockItem{
					{ID: "i1", Label: "fakeFunctionName"},
				},
			},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateDiagramEdgeEndpointHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("non-diagram blocks MUST skip the gate; got %+v", vs)
	}
}

// TestDiagramEdgeEndpointHallucination_OracleTier3Rejected confirms
// Tier 3+ matches don't vouch (precision contract — same as Fix A/B/C).
func TestDiagramEdgeEndpointHallucination_OracleTier3Rejected(t *testing.T) {
	body := "graph TD\n    fakeCompoundName --> realFunction\n"
	doc := docWithDiagram("call_dag", body)
	oracle := &stubOracleFixB{tiers: map[string]int{
		"fakeCompoundName": 3, // low-confidence parse → reject
		"realFunction":     1,
	}}
	vs := validateDiagramEdgeEndpointHallucination(doc, oracle)
	if len(vs) != 1 {
		t.Fatalf("Tier 3 MUST NOT vouch; got %d:\n  %+v", len(vs), vs)
	}
	if !strings.Contains(vs[0].Detail, "fakeCompoundName") {
		t.Errorf("violation must list the Tier-3 hallucination; got: %s", vs[0].Detail)
	}
}

// TestDiagramEdgeEndpointHallucination_EmptyDiagramBodySkips ensures
// blocks with empty Diagram body don't trigger the validator.
func TestDiagramEdgeEndpointHallucination_EmptyDiagramBodySkips(t *testing.T) {
	doc := docWithDiagram("call_dag", "")
	oracle := &stubOracleFixB{tiers: map[string]int{}}
	if vs := validateDiagramEdgeEndpointHallucination(doc, oracle); len(vs) != 0 {
		t.Errorf("empty diagram body MUST skip; got %+v", vs)
	}
}

// TestDiagramEdgeEndpointHallucination_MultiBlockSplits confirms
// each affected diagram block produces its own per-block violation.
func TestDiagramEdgeEndpointHallucination_MultiBlockSplits(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID: "diagram1", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramFlow,
					Body: "graph TD\n    fakeFunctionOne --> realFunction\n",
				},
			},
			{
				ID: "diagram2", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{
					Kind: types.DiagramFlow,
					Body: "graph TD\n    fakeFunctionTwo --> realFunction\n",
				},
			},
		},
	}
	oracle := &stubOracleFixB{tiers: map[string]int{
		"realFunction": 1,
	}}
	vs := validateDiagramEdgeEndpointHallucination(doc, oracle)
	if len(vs) != 2 {
		t.Fatalf("expected one violation per diagram block; got %d (%+v)", len(vs), vs)
	}
	if vs[0].ClusterKey == vs[1].ClusterKey {
		t.Errorf("each block should have a distinct ClusterKey; got %q == %q", vs[0].ClusterKey, vs[1].ClusterKey)
	}
}

// TestDiagramEdgeEndpointHallucination_MultiLanguage locks the
// multi-language equivalence promise. SymbolOracle is repomap-
// backed and indexes every language uniformly; the validator should
// treat each language's node-name conventions identically.
func TestDiagramEdgeEndpointHallucination_MultiLanguage(t *testing.T) {
	cases := []struct {
		lang     string
		realSym  string // present in oracle, len ≥ 10
		fakeSym  string // absent in oracle, len ≥ 10
		note     string
	}{
		{"go", "validateChecker", "validateFakeName",
			"Go camelCase + camelCase fake"},
		{"python", "DataPipelineRunner", "FakeDataLoader",
			"Python PascalCase + Pascal fake"},
		{"rust", "TaskGraphBuilder", "FakeTaskBuilder",
			"Rust PascalCase + Pascal fake"},
		{"swift", "userSessionManager", "fakeUserSession",
			"Swift camelCase + camel fake"},
		{"java", "RequestHandlerImpl", "FakeRequestImpl",
			"Java PascalCase + Pascal fake"},
		{"kotlin", "HomeViewModelImpl", "FakeViewModel",
			"Kotlin PascalCase + Pascal fake"},
		{"ruby", "UserAccountClass", "FakeUserAccount",
			"Ruby PascalCase + Pascal fake"},
		{"ts", "requestHandlerFn", "fakeRequestHandler",
			"TS camelCase + camel fake"},
		{"arkts", "UserProfileView", "FakeUserProfile",
			"ArkTS PascalCase + Pascal fake"},
		{"cangjie", "TaskRunnerEntry", "FakeTaskRunner",
			"Cangjie PascalCase + Pascal fake"},
	}
	for _, tc := range cases {
		t.Run(tc.lang+"/"+tc.realSym, func(t *testing.T) {
			body := "graph TD\n    " + tc.fakeSym + " --> " + tc.realSym + "\n"
			doc := docWithDiagram("call_dag", body)
			oracle := &stubOracleFixB{tiers: map[string]int{
				tc.realSym: 1,
				// fake absent
			}}
			vs := validateDiagramEdgeEndpointHallucination(doc, oracle)
			if len(vs) != 1 {
				t.Fatalf("[%s] expected 1 violation; got %d:\n  %+v\n  note: %s",
					tc.lang, len(vs), vs, tc.note)
			}
			if !strings.Contains(vs[0].Detail, tc.fakeSym) {
				t.Errorf("[%s] violation must list the fake endpoint %q; got: %s",
					tc.lang, tc.fakeSym, vs[0].Detail)
			}
			if strings.Contains(vs[0].Detail, tc.realSym) {
				t.Errorf("[%s] violation MUST NOT list the real endpoint %q; got: %s",
					tc.lang, tc.realSym, vs[0].Detail)
			}
		})
	}
}

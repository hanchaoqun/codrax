package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeDiagramEdgeAnchorMetadata_NormalizesOnlyTypedMetadata(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "d1",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     types.DiagramArchitecture,
			Language: "mermaid",
			Body: strings.Join([]string{
				"flowchart TD",
				"    A[\"Caller\"] -->|calls| B[\"Callee\"]",
				"    B -->|imports| C[\"Module\"]",
			}, "\n"),
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode:     "Caller",
			ToNode:       "Callee",
			RelationKind: types.DiagramRelCall,
			ClaimForm:    types.ClaimImportEdge,
		}},
	}}}
	originalBody := doc.Blocks[0].Diagram.Body

	fixed := normalizeDiagramEdgeAnchorMetadata(doc)
	if fixed != 3 {
		t.Fatalf("fixed=%d, want 3; anchors=%+v", fixed, doc.Blocks[0].EdgeAnchors)
	}
	if doc.Blocks[0].Diagram.Body != originalBody {
		t.Fatalf("typed metadata normalization must not rewrite model Mermaid body:\nbefore=%s\nafter=%s", originalBody, doc.Blocks[0].Diagram.Body)
	}
	anchors := doc.Blocks[0].EdgeAnchors
	if len(anchors) != 1 {
		t.Fatalf("len(edge_anchors)=%d, want 1: label text must not mint typed authority: %+v", len(anchors), anchors)
	}
	if anchors[0].FromNode != "A" || anchors[0].ToNode != "B" ||
		anchors[0].RelationKind != types.DiagramRelCall ||
		anchors[0].ClaimForm != types.ClaimCallEdge {
		t.Fatalf("existing anchor not normalized: %+v", anchors[0])
	}
}

func TestNormalizeDiagramEdgeAnchorMetadata_RewritesExactSiblingCarrierAfterMermaidAliasing(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "relations", Kind: types.BlockOrderedList,
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "Orchestrator.dispatchStage", ToNode: "ag.Execute",
				RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
			}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: strings.Join([]string{
				"flowchart TD",
				`codraxNode1["Orchestrator.dispatchStage"] --> codraxNode2["ag.Execute"]`,
			}, "\n")},
		},
	}}
	if fixed := normalizeDiagramEdgeAnchorMetadata(doc); fixed != 2 {
		t.Fatalf("fixed=%d, want exact sibling endpoint pair rewrite", fixed)
	}
	anchor := doc.Blocks[0].EdgeAnchors[0]
	if anchor.FromNode != "codraxNode1" || anchor.ToNode != "codraxNode2" {
		t.Fatalf("sibling anchor=%+v, want syntax-repair aliases", anchor)
	}
	if anchor.RelationKind != types.DiagramRelCall || anchor.ClaimForm != types.ClaimCallEdge {
		t.Fatalf("endpoint rewrite changed relation semantics: %+v", anchor)
	}
}

func TestNormalizeDiagramEdgeAnchorMetadata_LeavesAmbiguousSiblingAliasUnchanged(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "relations", Kind: types.BlockOrderedList,
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "Caller.run", ToNode: "Worker.exec",
			RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
		}},
	}}}
	for _, id := range []string{"first", "second"} {
		doc.Blocks = append(doc.Blocks, types.AnswerBlock{
			ID: id, Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: strings.Join([]string{
				"flowchart TD",
				`A["Caller.run"] --> B["Worker.exec"]`,
			}, "\n")},
		})
	}
	if fixed := normalizeDiagramEdgeAnchorMetadata(doc); fixed != 0 {
		t.Fatalf("fixed=%d, reused pair across diagrams must stay ambiguous", fixed)
	}
	anchor := doc.Blocks[0].EdgeAnchors[0]
	if anchor.FromNode != "Caller.run" || anchor.ToNode != "Worker.exec" {
		t.Fatalf("ambiguous sibling anchor was guessed: %+v", anchor)
	}
}

func TestNormalizeDiagramEdgeAnchorMetadata_DoesNotUseDisconnectedLabelsAsEdge(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "relations", Kind: types.BlockOrderedList,
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "Caller.run", ToNode: "Worker.exec",
				RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
			}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n A[\"Caller.run\"]\n B[\"Worker.exec\"]"},
		},
	}}
	if fixed := normalizeDiagramEdgeAnchorMetadata(doc); fixed != 0 {
		t.Fatalf("fixed=%d, disconnected labels must not authorize metadata", fixed)
	}
}

func TestNormalizeDiagramEdgeAnchorIdentitiesFromTypedRecipesPreservesBusinessLabels(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "business", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: strings.Join([]string{
			"flowchart TD",
			`  n11["提交阶段请求"] -->|call| n5["分发当前阶段"]`,
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "n11", ToNode: "n5", RelationKind: types.DiagramRelCall}},
	}}}
	recipes := []types.DiagramEdgeAnchor{{
		FromNode: "n11", ToNode: "n5",
		FromIdentity: "Orchestrator.executeStageRequest", ToIdentity: "Orchestrator.dispatchStage",
		RelationKind: types.DiagramRelCall,
	}}
	originalBody := doc.Blocks[0].Diagram.Body
	if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes); fixed != 1 {
		t.Fatalf("fixed=%d, want one exact typed identity-pair restore", fixed)
	}
	got := doc.Blocks[0].EdgeAnchors[0]
	if got.FromIdentity != recipes[0].FromIdentity || got.ToIdentity != recipes[0].ToIdentity {
		t.Fatalf("restored anchor=%+v, want recipe identity pair", got)
	}
	if doc.Blocks[0].Diagram.Body != originalBody {
		t.Fatalf("typed identity repair must not rewrite business display copy:\n%s", doc.Blocks[0].Diagram.Body)
	}
	if mismatches := DiagramCallEdgeEvidenceMismatches(doc,
		&types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow},
		[]types.EvidenceItem{diagramEvidenceTestCall(recipes[0].FromIdentity, recipes[0].ToIdentity)},
	); len(mismatches) != 0 {
		t.Fatalf("restored exact identity pair must pass the unchanged evidence gate: %+v", mismatches)
	}
}

func TestNormalizeDiagramEdgeAnchorIdentitiesFromTypedRecipesMapsUniqueBusinessTopology(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "business-topology", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: strings.Join([]string{
			"flowchart TD",
			`  prepare["理解请求"] -->|precedence| inspect["查找证据"]`,
			`  inspect -->|precedence| distill["整理事实"]`,
			`  distill -->|precedence| answer["形成回答"]`,
			`  dispatch["安排阶段"] -->|call| worker["执行阶段"]`,
			`  value["分析结果"] -->|data_flow| context["共享上下文"]`,
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "prepare", ToNode: "inspect", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "inspect", ToNode: "distill", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "distill", ToNode: "answer", RelationKind: types.DiagramRelPrecedence},
			{FromNode: "dispatch", ToNode: "worker", RelationKind: types.DiagramRelCall},
			{FromNode: "value", ToNode: "context", RelationKind: types.DiagramRelDataFlow},
		},
	}}}
	recipes := []types.DiagramEdgeAnchor{
		{FromNode: "n1", ToNode: "n2", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence},
		{FromNode: "n2", ToNode: "n3", FromIdentity: "Explorer", ToIdentity: "Extractor", RelationKind: types.DiagramRelPrecedence},
		{FromNode: "n3", ToNode: "n4", FromIdentity: "Extractor", ToIdentity: "Finalizer", RelationKind: types.DiagramRelPrecedence},
		{FromNode: "n5", ToNode: "n6", FromIdentity: "Orchestrator.Run", ToIdentity: "Orchestrator.runAnalyzePhase", RelationKind: types.DiagramRelCall},
		{FromNode: "n7", ToNode: "n8", FromIdentity: "out.AnalysisIR", ToIdentity: "o.busCtx.AnalysisIR", RelationKind: types.DiagramRelDataFlow},
	}
	originalBody := doc.Blocks[0].Diagram.Body
	if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes); fixed != 5 {
		t.Fatalf("fixed=%d, want all five identities restored by unique typed topology", fixed)
	}
	for i, want := range recipes {
		got := doc.Blocks[0].EdgeAnchors[i]
		if got.FromIdentity != want.FromIdentity || got.ToIdentity != want.ToIdentity {
			t.Fatalf("anchor[%d]=%+v, want identity pair %q -> %q", i, got, want.FromIdentity, want.ToIdentity)
		}
	}
	if doc.Blocks[0].Diagram.Body != originalBody {
		t.Fatalf("topology receipt must not rewrite model-authored business labels:\n%s", doc.Blocks[0].Diagram.Body)
	}
}

func TestNormalizeDiagramEdgeAnchorIdentitiesFromTypedRecipesTopologyAmbiguityFailsClosed(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "ambiguous-business-edge", Kind: types.BlockDiagram,
		Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n  caller --> callee\n"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "caller", ToNode: "callee", RelationKind: types.DiagramRelCall}},
	}}}
	recipes := []types.DiagramEdgeAnchor{
		{FromNode: "n1", ToNode: "n2", FromIdentity: "A.run", ToIdentity: "B.run", RelationKind: types.DiagramRelCall},
		{FromNode: "n3", ToNode: "n4", FromIdentity: "C.run", ToIdentity: "D.run", RelationKind: types.DiagramRelCall},
	}
	if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes); fixed != 0 || doc.Blocks[0].EdgeAnchors[0].HasEndpointIdentityPair() {
		t.Fatalf("two isomorphic typed components must remain fail-closed: fixed=%d anchor=%+v", fixed, doc.Blocks[0].EdgeAnchors[0])
	}
}

func TestNormalizeDiagramEdgeAnchorIdentitiesFromTypedRecipesPartialTopologyFailsClosed(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "partial-chain", Kind: types.BlockDiagram,
		Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n  first --> second\n"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "first", ToNode: "second", RelationKind: types.DiagramRelPrecedence}},
	}}}
	recipes := []types.DiagramEdgeAnchor{
		{FromNode: "n1", ToNode: "n2", FromIdentity: "Analyzer", ToIdentity: "Explorer", RelationKind: types.DiagramRelPrecedence},
		{FromNode: "n2", ToNode: "n3", FromIdentity: "Explorer", ToIdentity: "Extractor", RelationKind: types.DiagramRelPrecedence},
	}
	if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes); fixed != 0 {
		t.Fatalf("a strict subgraph must not borrow an edge identity from a larger typed component: fixed=%d", fixed)
	}
}

func TestNormalizeDiagramEdgeAnchorIdentitiesFromTypedRecipesFailsClosed(t *testing.T) {
	newDoc := func() *types.AnswerDocumentV2 {
		return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "ambiguous", Kind: types.BlockDiagram,
			Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart TD\n  n1 --> n2\n"},
			EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "n1", ToNode: "n2", RelationKind: types.DiagramRelCall}},
		}}}
	}
	base := types.DiagramEdgeAnchor{FromNode: "n1", ToNode: "n2", FromIdentity: "A.run", ToIdentity: "B.run", RelationKind: types.DiagramRelCall}
	t.Run("ambiguous exact pairs", func(t *testing.T) {
		doc := newDoc()
		other := base
		other.ToIdentity = "Other.run"
		if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, []types.DiagramEdgeAnchor{base, other}); fixed != 0 || doc.Blocks[0].EdgeAnchors[0].HasEndpointIdentityPair() {
			t.Fatalf("ambiguous recipes must remain unmodified: fixed=%d anchor=%+v", fixed, doc.Blocks[0].EdgeAnchors[0])
		}
	})
	t.Run("relation mismatch", func(t *testing.T) {
		doc := newDoc()
		doc.Blocks[0].EdgeAnchors[0].RelationKind = types.DiagramRelDataFlow
		if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, []types.DiagramEdgeAnchor{base}); fixed != 0 {
			t.Fatalf("different relation kind must not receive call identities: fixed=%d", fixed)
		}
	})
	t.Run("no visible body edge", func(t *testing.T) {
		doc := newDoc()
		doc.Blocks[0].Diagram.Body = "flowchart TD\n  n1\n  n2\n"
		if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, []types.DiagramEdgeAnchor{base}); fixed != 0 {
			t.Fatalf("metadata-only anchor must not be authorized: fixed=%d", fixed)
		}
	})
	t.Run("partial model identity", func(t *testing.T) {
		doc := newDoc()
		doc.Blocks[0].EdgeAnchors[0].FromIdentity = "Wrong.run"
		if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, []types.DiagramEdgeAnchor{base}); fixed != 0 || doc.Blocks[0].EdgeAnchors[0].FromIdentity != "Wrong.run" {
			t.Fatalf("partial model identity must remain fail-closed: fixed=%d anchor=%+v", fixed, doc.Blocks[0].EdgeAnchors[0])
		}
	})
}

func TestNormalizeAnswerDocumentForPreEmitWiresTypedRecipeIdentityRepair(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("render an architecture diagram")}
	recipe := types.DiagramEdgeAnchor{
		FromNode: "n5", ToNode: "n6",
		FromIdentity: "Orchestrator.dispatchStage", ToIdentity: "o.agents.Get",
		RelationKind: types.DiagramRelCall,
	}
	bus.Mutable.SetFinalizerTypedRelationRecipeAvailable(true)
	bus.Mutable.SetFinalizerTypedRelationRecipeAnchors([]types.DiagramEdgeAnchor{recipe})
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "dispatch", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid",
			Body: "flowchart TD\n  n5[\"阶段分发\"] -->|call| n6[\"获取代理\"]\n"},
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "n5", ToNode: "n6", RelationKind: types.DiagramRelCall}},
	}}}
	pctx := newPreEmitCheckContext(bus)
	normalizeAnswerDocumentForPreEmit("emit_answer_document", doc,
		&types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}, bus, pctx)
	got := doc.Blocks[0].EdgeAnchors[0]
	if got.FromIdentity != recipe.FromIdentity || got.ToIdentity != recipe.ToIdentity {
		t.Fatalf("production normalizer did not consume the dispatch-scoped typed receipt: %+v", got)
	}
	if pctx.repairCounts["normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes"] != 1 {
		t.Fatalf("production repair accounting missing: %+v", pctx.repairCounts)
	}
}

func TestNormalizeAnswerDocumentForPreEmitWiresUniqueTopologyAfterMermaidAliasRepair(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("render a business-facing architecture diagram")}
	recipes := []types.DiagramEdgeAnchor{
		{FromNode: "n5", ToNode: "n6", FromIdentity: "Orchestrator.Run", ToIdentity: "Orchestrator.runAnalyzePhase", RelationKind: types.DiagramRelCall},
		{FromNode: "n7", ToNode: "n8", FromIdentity: "out.AnalysisIR", ToIdentity: "o.busCtx.AnalysisIR", RelationKind: types.DiagramRelDataFlow},
	}
	bus.Mutable.SetFinalizerTypedRelationRecipeAvailable(true)
	bus.Mutable.SetFinalizerTypedRelationRecipeAnchors(recipes)
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "dispatch", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: strings.Join([]string{
			"flowchart TD",
			`  dispatch["安排分析"] -->|call| worker["执行分析"]`,
			`  result["分析结果"] -->|data_flow| codraxNode1["共享上下文"]`,
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{FromNode: "dispatch", ToNode: "worker", RelationKind: types.DiagramRelCall},
			{FromNode: "result", ToNode: "codraxNode1", RelationKind: types.DiagramRelDataFlow},
		},
	}}}
	pctx := newPreEmitCheckContext(bus)
	normalizeAnswerDocumentForPreEmit("emit_answer_document_patch", doc,
		&types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}, bus, pctx)
	for i, want := range recipes {
		got := doc.Blocks[0].EdgeAnchors[i]
		if got.FromIdentity != want.FromIdentity || got.ToIdentity != want.ToIdentity {
			t.Fatalf("production topology receipt anchor[%d]=%+v, want %q -> %q", i, got, want.FromIdentity, want.ToIdentity)
		}
	}
	if pctx.repairCounts["normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes"] != 2 {
		t.Fatalf("production topology repair accounting missing: %+v", pctx.repairCounts)
	}
}

func TestNormalizeOrphanDiagramEdgeAnchors_RemovedOptionalDiagramClearsSiblingMetadata(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "summary", Kind: types.BlockSummary,
			Text: "The optional diagram was removed; the grounded text remains.",
		},
		{
			ID: "path", Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{ID: "hop", Label: "run_pipeline"}},
			EdgeAnchors: []types.DiagramEdgeAnchor{
				{FromNode: "run_pipeline", ToNode: "resolve", RelationKind: types.DiagramRelCall},
				{FromNode: "loop", ToNode: "handle", RelationKind: types.DiagramRelCallback},
			},
		},
	}}
	if removed := normalizeOrphanDiagramEdgeAnchors(doc); removed != 2 {
		t.Fatalf("removed=%d, want 2: %+v", removed, doc.Blocks)
	}
	if len(doc.Blocks[1].EdgeAnchors) != 0 {
		t.Fatalf("orphan anchors survived after all typed diagrams were removed: %+v", doc.Blocks[1].EdgeAnchors)
	}
}

func TestNormalizeOrphanDiagramEdgeAnchors_PreservesSiblingCarrierForExistingDiagram(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "path", Kind: types.BlockOrderedList,
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall,
			}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{
				Kind: types.DiagramSequence, Language: "mermaid",
				Body: "sequenceDiagram\n  participant A as Caller\n  participant B as Callee\n  A->>B: call\n",
			},
		},
	}}
	if removed := normalizeOrphanDiagramEdgeAnchors(doc); removed != 0 {
		t.Fatalf("removed=%d, want 0 while a typed diagram can consume sibling anchors", removed)
	}
	if len(doc.Blocks[0].EdgeAnchors) != 1 {
		t.Fatalf("valid sibling diagram anchor was removed: %+v", doc.Blocks[0].EdgeAnchors)
	}
}

func TestNormalizeAnswerDocumentForPreEmit_RecordsOrphanAnchorRepair(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "path", Kind: types.BlockOrderedList,
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "loop", ToNode: "handle", RelationKind: types.DiagramRelCallback,
		}},
	}}}
	pctx := newPreEmitCheckContext()
	normalizeAnswerDocumentForPreEmit("test", doc, &types.AnswerSemanticView{Family: types.QFCallChain}, nil, pctx)
	if got := pctx.repairCounts["normalizeOrphanDiagramEdgeAnchors"]; got != 1 {
		t.Fatalf("repair count=%d, want 1: %+v", got, pctx.repairCounts)
	}
	if len(doc.Blocks[0].EdgeAnchors) != 0 {
		t.Fatalf("pre-emit normalization retained orphan metadata: %+v", doc.Blocks[0].EdgeAnchors)
	}
}

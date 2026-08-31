package tool

import (
	"fmt"
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

func TestNormalizeDiagramEdgeAnchorMetadata_ExactBodyNodeIDWinsDisplayLabelAlias(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind types.DiagramKind
		body string
	}{
		{
			name: "sequence",
			kind: types.DiagramSequence,
			body: strings.Join([]string{
				"sequenceDiagram",
				"  participant Ex as Explorer",
				"  participant Et as Extractor",
				"  explorer->>extractor: model-authored stage handoff",
			}, "\n"),
		},
		{
			name: "flow",
			kind: types.DiagramFlow,
			body: strings.Join([]string{
				"flowchart TD",
				`  Ex["Explorer"]`,
				`  Et["Extractor"]`,
				"  explorer --> extractor",
			}, "\n"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "d1", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: tc.kind, Language: "mermaid", Body: tc.body},
				EdgeAnchors: []types.DiagramEdgeAnchor{{
					FromNode: "explorer", ToNode: "extractor",
					RelationKind: types.DiagramRelPrecedence, ClaimForm: types.ClaimPrecedenceRole,
				}},
			}}}
			if fixed := normalizeDiagramEdgeAnchorMetadata(doc); fixed != 0 {
				t.Fatalf("fixed=%d, exact body node ids must not be rewritten through old display labels: %+v", fixed, doc.Blocks[0].EdgeAnchors)
			}
			anchor := doc.Blocks[0].EdgeAnchors[0]
			if anchor.FromNode != "explorer" || anchor.ToNode != "extractor" || doc.Blocks[0].Diagram.Body != tc.body {
				t.Fatalf("exact body identity or model-authored diagram changed: anchor=%+v body=%q", anchor, doc.Blocks[0].Diagram.Body)
			}
		})
	}
}

func TestDiagramNodeAliasIndex_AmbiguousDisplayLabelFailsClosed(t *testing.T) {
	body := strings.Join([]string{
		"flowchart TD",
		`  A["Worker"] --> B["Worker"]`,
	}, "\n")
	aliases := diagramNodeAliasIndex(body)
	if got := aliases[diagramSurfaceKey("Worker")]; got != "" {
		t.Fatalf("ambiguous display label must not select an exact node id: %q", got)
	}
	if aliases[diagramSurfaceKey("A")] != "A" || aliases[diagramSurfaceKey("B")] != "B" {
		t.Fatalf("ambiguous labels must not erase exact node ids: %+v", aliases)
	}
}

func TestNormalizeDiagramEdgeAnchorMetadata_RepairsUniqueOneSidedCopiedIdentityNodeRef(t *testing.T) {
	body := strings.Join([]string{
		"flowchart TD",
		`  Mutable["Mutable"] -->|data_flow| codraxNode1["AgentContext.Mutable"]`,
	}, "\n")
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "flow", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: body},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "bus.Mutable", ToNode: "AgentContext.Mutable",
			FromIdentity: "bus.Mutable", ToIdentity: "AgentContext.Mutable",
			RelationKind: types.DiagramRelDataFlow, ClaimForm: types.ClaimAssignmentFact,
		}},
	}}}
	if fixed := normalizeDiagramEdgeAnchorMetadata(doc); fixed != 2 {
		t.Fatalf("fixed=%d, want target alias plus one-sided source node repair: %+v", fixed, doc.Blocks[0].EdgeAnchors)
	}
	anchor := doc.Blocks[0].EdgeAnchors[0]
	if anchor.FromNode != "Mutable" || anchor.ToNode != "codraxNode1" {
		t.Fatalf("one-sided node refs not aligned to the unchanged visible edge: %+v", anchor)
	}
	if anchor.FromIdentity != "bus.Mutable" || anchor.ToIdentity != "AgentContext.Mutable" ||
		anchor.RelationKind != types.DiagramRelDataFlow || doc.Blocks[0].Diagram.Body != body {
		t.Fatalf("metadata-only repair changed relation authority or Mermaid source: anchor=%+v body=%q", anchor, doc.Blocks[0].Diagram.Body)
	}
	initializer := diagramEvidenceTestCall("AgentContext.Mutable", "bus.Mutable")
	initializer.AnchorKind = types.AnchorInitializer
	initializer.Predicate = "assigns"
	initializer.AnchorSymbol = "Mutable"
	initializer.InitializerContainer = "AgentContext"
	initializer.Snippet = "Mutable: bus.Mutable,"
	if mismatches := DiagramCallEdgeEvidenceMismatches(doc,
		&types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow},
		[]types.EvidenceItem{initializer},
	); len(mismatches) != 0 {
		t.Fatalf("one-sided node-ref repair must preserve the exact typed data-flow through the relation gate: %+v", mismatches)
	}
}

func TestNormalizeDiagramEdgeAnchorMetadata_OneSidedRepairFailsClosedOnAmbiguousOrOccupiedEdge(t *testing.T) {
	baseAnchor := types.DiagramEdgeAnchor{
		FromNode: "bus.Mutable", ToNode: "T",
		FromIdentity: "bus.Mutable", ToIdentity: "AgentContext.Mutable",
		RelationKind: types.DiagramRelDataFlow, ClaimForm: types.ClaimAssignmentFact,
	}
	for _, tc := range []struct {
		name    string
		body    string
		anchors []types.DiagramEdgeAnchor
	}{
		{
			name:    "two visible sources share the matched target",
			body:    "flowchart TD\n  A --> T\n  B --> T",
			anchors: []types.DiagramEdgeAnchor{baseAnchor},
		},
		{
			name: "the only visible edge is already owned",
			body: "flowchart TD\n  A --> T",
			anchors: []types.DiagramEdgeAnchor{baseAnchor, {
				FromNode: "A", ToNode: "T", FromIdentity: "producer.A", ToIdentity: "consumer.T",
				RelationKind: types.DiagramRelDataFlow, ClaimForm: types.ClaimAssignmentFact,
			}},
		},
		{
			name: "mismatching node is not a copied typed identity",
			body: "flowchart TD\n  A --> T",
			anchors: []types.DiagramEdgeAnchor{{
				FromNode: "wrong-node", ToNode: "T", FromIdentity: "bus.Mutable", ToIdentity: "AgentContext.Mutable",
				RelationKind: types.DiagramRelDataFlow, ClaimForm: types.ClaimAssignmentFact,
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "flow", Kind: types.BlockDiagram,
				Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: tc.body},
				EdgeAnchors: append([]types.DiagramEdgeAnchor(nil), tc.anchors...),
			}}}
			before := doc.Blocks[0].EdgeAnchors[0]
			if fixed := normalizeDiagramEdgeAnchorMetadata(doc); fixed != 0 {
				t.Fatalf("fixed=%d, ambiguous/non-owned shape must fail closed: %+v", fixed, doc.Blocks[0].EdgeAnchors)
			}
			if got := doc.Blocks[0].EdgeAnchors[0]; got != before {
				t.Fatalf("orphan anchor changed without unique structural authority: before=%+v after=%+v", before, got)
			}
		})
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

func TestNormalizeDiagramEdgeAnchorMetadata_PreservesStandaloneReaderLabels(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID: "relations", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "py.tokenize_bytes", ToNode: "tokenize_bytes (Rust)",
				FromIdentity: "py.tokenize_bytes", ToIdentity: "tokenize_bytes",
				RelationKind: types.DiagramRelCall, VisibleLabel: "转发调用",
			}},
		},
		{
			ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
				"sequenceDiagram",
				"  participant Wr as py.tokenize_bytes",
				"  participant Fn as tokenize_bytes (Rust)",
				"  Wr->>Fn: tokenize_bytes(...) ",
			}, "\n")},
		},
	}}
	if fixed := normalizeDiagramEdgeAnchorMetadata(doc); fixed != 0 {
		t.Fatalf("fixed=%d, standalone reader labels must not be rewritten", fixed)
	}
	got := doc.Blocks[0].EdgeAnchors[0]
	if got.FromNode != "py.tokenize_bytes" || got.ToNode != "tokenize_bytes (Rust)" {
		t.Fatalf("standalone reader labels leaked Mermaid aliases: %+v", got)
	}
	counts := diagramEvidenceBodyEdgeBlockCounts(doc)
	effective := diagramEvidenceEffectiveAnchorsForBlock(doc, 1, counts)
	if len(effective) != 1 || effective[0].FromNode != "Wr" || effective[0].ToNode != "Fn" {
		t.Fatalf("diagram validation did not receive ephemeral aliases: %+v", effective)
	}
	// Removing the optional visual must leave the reader-facing relation
	// exactly as the model authored it.
	doc.Blocks = doc.Blocks[:1]
	if removed := normalizeOrphanDiagramEdgeAnchors(doc, &types.AnswerSemanticView{Family: types.QFCallChain}); removed != 0 {
		t.Fatalf("standalone relation removed with optional diagram: %d", removed)
	}
	got = doc.Blocks[0].EdgeAnchors[0]
	if got.FromNode != "py.tokenize_bytes" || got.ToNode != "tokenize_bytes (Rust)" {
		t.Fatalf("reader labels changed after optional diagram removal: %+v", got)
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

func TestNormalizeDiagramEdgeAnchorIdentitiesFromTypedRecipesCompletesUniqueOneSidedReceipt(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "business", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: strings.Join([]string{
			"flowchart TD",
			`  buildInit["准备分析"] -->|call| Mutable["可变调查状态"]`,
		}, "\n")},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "buildInit", ToNode: "Mutable",
			ToIdentity: "ctx.Mutable.SetPrescanRoundLimit", RelationKind: types.DiagramRelCall,
		}},
	}}}
	recipes := []types.DiagramEdgeAnchor{
		{FromIdentity: "analyzerEvaluator.BuildInitialInstruction", ToIdentity: "ctx.Mutable.SetPrescanRoundLimit", RelationKind: types.DiagramRelCall},
		{FromIdentity: "analyzerEvaluator.BuildInitialInstruction", ToIdentity: "ctx.Mutable.SetSearchGraph", RelationKind: types.DiagramRelCall},
	}
	originalBody := doc.Blocks[0].Diagram.Body
	if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes); fixed != 1 {
		t.Fatalf("fixed=%d, want one unique one-sided receipt completion", fixed)
	}
	got := doc.Blocks[0].EdgeAnchors[0]
	if got.FromIdentity != recipes[0].FromIdentity || got.ToIdentity != recipes[0].ToIdentity {
		t.Fatalf("one-sided receipt was not completed exactly: %+v", got)
	}
	if got.FromNode != "buildInit" || got.ToNode != "Mutable" || got.RelationKind != types.DiagramRelCall || doc.Blocks[0].Diagram.Body != originalBody {
		t.Fatalf("identity completion changed model-authored topology or relation: anchor=%+v body=%q", got, doc.Blocks[0].Diagram.Body)
	}
}

func TestNormalizeDiagramEdgeAnchorIdentitiesFromTypedRecipesKeepsAmbiguousOrReplayedOneSidedRowsFailClosed(t *testing.T) {
	newDoc := func() *types.AnswerDocumentV2 {
		return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "business", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramArchitecture, Language: "mermaid", Body: strings.Join([]string{
				"flowchart TD", `  A -->|call| B`, `  C -->|call| D`,
			}, "\n")},
			EdgeAnchors: []types.DiagramEdgeAnchor{
				{FromNode: "A", ToNode: "B", FromIdentity: "Caller", RelationKind: types.DiagramRelCall},
				{FromNode: "C", ToNode: "D", ToIdentity: "Unique.Target", RelationKind: types.DiagramRelCall},
			},
		}}}
	}
	recipes := []types.DiagramEdgeAnchor{
		{FromIdentity: "Caller", ToIdentity: "First.Target", RelationKind: types.DiagramRelCall},
		{FromIdentity: "Caller", ToIdentity: "Second.Target", RelationKind: types.DiagramRelCall},
		{FromIdentity: "Unique.Caller", ToIdentity: "Unique.Target", RelationKind: types.DiagramRelCall},
	}

	t.Run("ambiguous populated side", func(t *testing.T) {
		doc := newDoc()
		doc.Blocks[0].EdgeAnchors = doc.Blocks[0].EdgeAnchors[:1]
		if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes); fixed != 0 || doc.Blocks[0].EdgeAnchors[0].ToIdentity != "" {
			t.Fatalf("ambiguous one-sided receipt must stay untouched: fixed=%d anchor=%+v", fixed, doc.Blocks[0].EdgeAnchors[0])
		}
	})

	t.Run("one receipt cannot authorize duplicate visible consumers", func(t *testing.T) {
		doc := newDoc()
		doc.Blocks[0].EdgeAnchors[0] = types.DiagramEdgeAnchor{FromNode: "A", ToNode: "B", ToIdentity: "Unique.Target", RelationKind: types.DiagramRelCall}
		if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes); fixed != 0 {
			t.Fatalf("duplicated one-sided consumers must not replay one receipt: fixed=%d anchors=%+v", fixed, doc.Blocks[0].EdgeAnchors)
		}
		for _, anchor := range doc.Blocks[0].EdgeAnchors {
			if anchor.FromIdentity != "" {
				t.Fatalf("receipt was replayed onto duplicate consumer: %+v", anchor)
			}
		}
	})
}

func TestNormalizeDiagramEdgeAnchorIdentitiesFromTypedRecipesRemovesUniqueDisplayQualifierFromStructuredCarrier(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "path", Kind: types.BlockOrderedList,
		Items: []types.AnswerBlockItem{{ID: "hop-1", Label: "tokenize_bytes (core)", Text: "business-facing copy"}},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{
				FromNode: "wrapper", ToNode: "core",
				FromIdentity: "py.tokenize_bytes", ToIdentity: "tokenize_bytes (core)",
				RelationKind: types.DiagramRelCall,
			},
			{
				FromNode: "core", ToNode: "merge",
				FromIdentity: "tokenize_bytes (core)", ToIdentity: "best_merge",
				RelationKind: types.DiagramRelCall,
			},
		},
	}}}
	recipes := []types.DiagramEdgeAnchor{
		{FromIdentity: "py.tokenize_bytes", ToIdentity: "tokenize_bytes", RelationKind: types.DiagramRelCall},
		{FromIdentity: "tokenize_bytes", ToIdentity: "best_merge", RelationKind: types.DiagramRelCall},
	}
	originalItems := append([]types.AnswerBlockItem(nil), doc.Blocks[0].Items...)
	originalNodes := [][2]string{{"wrapper", "core"}, {"core", "merge"}}
	if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes); fixed != 2 {
		t.Fatalf("fixed=%d, want two unique display-qualifier receipts", fixed)
	}
	for i, anchor := range doc.Blocks[0].EdgeAnchors {
		if anchor.FromIdentity != recipes[i].FromIdentity || anchor.ToIdentity != recipes[i].ToIdentity {
			t.Fatalf("anchor[%d]=%+v, want exact recipe identities", i, anchor)
		}
		if anchor.FromNode != originalNodes[i][0] || anchor.ToNode != originalNodes[i][1] {
			t.Fatalf("typed receipt changed business nodes: %+v", anchor)
		}
	}
	if doc.Blocks[0].Items[0].ID != originalItems[0].ID ||
		doc.Blocks[0].Items[0].Label != originalItems[0].Label ||
		doc.Blocks[0].Items[0].Text != originalItems[0].Text {
		t.Fatalf("typed receipt changed visible item copy: before=%+v after=%+v", originalItems[0], doc.Blocks[0].Items[0])
	}
}

func TestNormalizeDisplayQualifiedEdgeAnchorIdentitiesFailsClosedOnRecipeAmbiguity(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "path", Kind: types.BlockOrderedList,
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromIdentity: "pkg.foo (role)", ToIdentity: "pkg.bar (target)", RelationKind: types.DiagramRelCall,
		}},
	}}}
	recipes := []types.DiagramEdgeAnchor{
		{FromIdentity: "pkg.foo", ToIdentity: "pkg.bar", RelationKind: types.DiagramRelCall},
		{FromIdentity: "pkg::foo", ToIdentity: "pkg::bar", RelationKind: types.DiagramRelCall},
	}
	before := doc.Blocks[0].EdgeAnchors[0]
	if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes); fixed != 0 || doc.Blocks[0].EdgeAnchors[0] != before {
		t.Fatalf("multiple exact recipe surfaces must remain fail-closed: fixed=%d anchor=%+v", fixed, doc.Blocks[0].EdgeAnchors[0])
	}
}

func TestNormalizeDisplayQualifiedEdgeAnchorIdentitiesDoesNotStripFunctionSignature(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "path", Kind: types.BlockOrderedList,
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromIdentity: "foo(arg)", ToIdentity: "bar", RelationKind: types.DiagramRelCall,
		}},
	}}}
	recipes := []types.DiagramEdgeAnchor{{
		FromIdentity: "foo", ToIdentity: "bar", RelationKind: types.DiagramRelCall,
	}}
	before := doc.Blocks[0].EdgeAnchors[0]
	if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes); fixed != 0 || doc.Blocks[0].EdgeAnchors[0] != before {
		t.Fatalf("function signatures are code identity, not display qualifiers: fixed=%d anchor=%+v", fixed, doc.Blocks[0].EdgeAnchors[0])
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

func TestNormalizeDiagramEdgeAnchorIdentitiesFromTypedRecipesDoesNotReuseOneTypedComponentAcrossDisconnectedAliases(t *testing.T) {
	recipes := []types.DiagramEdgeAnchor{
		{FromNode: "n1", ToNode: "n2", FromIdentity: "Pipeline.run", ToIdentity: "Pipeline.dispatch", RelationKind: types.DiagramRelCall},
		{FromNode: "n3", ToNode: "n4", FromIdentity: "Pipeline.apply", ToIdentity: "appendState", RelationKind: types.DiagramRelCall},
		{FromNode: "n5", ToNode: "n4", FromIdentity: "ctx.Mutable", ToIdentity: "appendState", RelationKind: types.DiagramRelArgumentFlow},
	}
	newDoc := func(firstFrom, firstTo string) *types.AnswerDocumentV2 {
		return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
			ID: "disconnected-call-aliases", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: strings.Join([]string{
				"flowchart TD",
				"  " + firstFrom + " -->|call| " + firstTo,
				"  applyAlias -->|call| appendAlias",
			}, "\n")},
			EdgeAnchors: []types.DiagramEdgeAnchor{
				{FromNode: firstFrom, ToNode: firstTo, RelationKind: types.DiagramRelCall},
				{FromNode: "applyAlias", ToNode: "appendAlias", RelationKind: types.DiagramRelCall},
			},
		}}}
	}

	t.Run("two aliases remain unowned", func(t *testing.T) {
		doc := newDoc("dispatchAlias", "workerAlias")
		if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes); fixed != 0 {
			t.Fatalf("one typed call receipt must not be replayed onto disconnected aliases: fixed=%d anchors=%+v", fixed, doc.Blocks[0].EdgeAnchors)
		}
	})

	t.Run("exact node receipt is reserved from alias repair", func(t *testing.T) {
		doc := newDoc("n1", "n2")
		if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes); fixed != 1 {
			t.Fatalf("exact recipe nodes should repair once while the disconnected alias stays empty: fixed=%d anchors=%+v", fixed, doc.Blocks[0].EdgeAnchors)
		}
		if !doc.Blocks[0].EdgeAnchors[0].HasEndpointIdentityPair() || doc.Blocks[0].EdgeAnchors[1].HasEndpointIdentityPair() {
			t.Fatalf("typed receipt was not consumed injectively: anchors=%+v", doc.Blocks[0].EdgeAnchors)
		}
	})
}

func TestNormalizeDiagramEdgeAnchorIdentitiesFromTypedRecipesSkipsFullyIdentifiedWideSymmetricComponent(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "identified-wide-star", Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: "sequenceDiagram\n"},
	}}}
	recipes := make([]types.DiagramEdgeAnchor, 0, 12)
	for i := 0; i < 12; i++ {
		leaf := "leaf" + string(rune('a'+i))
		doc.Blocks[0].Diagram.Body += "  root->>" + leaf + ": call\n"
		anchor := types.DiagramEdgeAnchor{
			FromNode: "root", ToNode: leaf,
			FromIdentity: "Analyzer.buildAnalysisIR", ToIdentity: "Analyzer.helper." + leaf,
			RelationKind: types.DiagramRelCall,
		}
		doc.Blocks[0].EdgeAnchors = append(doc.Blocks[0].EdgeAnchors, anchor)
		recipe := anchor
		recipe.FromNode = "typed-root"
		recipe.ToNode = "typed-" + leaf
		recipes = append(recipes, recipe)
	}
	if fixed := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes); fixed != 0 {
		t.Fatalf("fully identified component must bypass optional topology repair: fixed=%d", fixed)
	}
}

func TestDiagramComponentIsomorphismsWideSymmetryExhaustionFailsClosed(t *testing.T) {
	modelEdges := make([]diagramModelAnchorEdge, 0, 12)
	recipeEdges := make([]diagramTypedRecipeEdge, 0, 12)
	for i := 0; i < 12; i++ {
		leaf := "leaf" + string(rune('a'+i))
		modelEdges = append(modelEdges, diagramModelAnchorEdge{from: "root", to: leaf, relation: types.DiagramRelCall})
		recipeEdges = append(recipeEdges, diagramTypedRecipeEdge{from: "typed-root", to: "typed-" + leaf, relation: types.DiagramRelCall})
	}
	model := diagramModelAnchorComponents(modelEdges)[0]
	recipe := diagramTypedRecipeComponents(recipeEdges)[0]
	if mappings, exhaustive := diagramComponentIsomorphisms(model, modelEdges, recipe, recipeEdges); exhaustive || len(mappings) != 0 {
		t.Fatalf("budget exhaustion must discard partial mappings: exhaustive=%t mappings=%d", exhaustive, len(mappings))
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

func TestNormalizeAnswerDocumentForPreEmitWiresDisplayQualifierReceiptOnStructuredCarrier(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("render the verified path")}
	recipe := types.DiagramEdgeAnchor{
		FromIdentity: "tokenize_bytes", ToIdentity: "best_merge", RelationKind: types.DiagramRelCall,
	}
	bus.Mutable.SetFinalizerTypedRelationRecipeAvailable(true)
	bus.Mutable.SetFinalizerTypedRelationRecipeAnchors([]types.DiagramEdgeAnchor{recipe})
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
		Items:     []types.AnswerBlockItem{{ID: "hop", Label: "tokenize_bytes (core)"}},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "core", ToNode: "merge",
			FromIdentity: "tokenize_bytes (core)", ToIdentity: "best_merge",
			RelationKind: types.DiagramRelCall,
		}},
	}}}
	pctx := newPreEmitCheckContext(bus)
	normalizeAnswerDocumentForPreEmit("emit_answer_document_patch", doc,
		&types.AnswerSemanticView{Family: types.QFArchitecture, RelationAxis: types.AxisFlow}, bus, pctx)
	got := doc.Blocks[0].EdgeAnchors[0]
	if got.FromIdentity != recipe.FromIdentity || got.ToIdentity != recipe.ToIdentity {
		t.Fatalf("production normalizer did not consume the unique display qualifier receipt: %+v", got)
	}
	if doc.Blocks[0].Items[0].Label != "tokenize_bytes (core)" || got.FromNode != "core" || got.ToNode != "merge" {
		t.Fatalf("production receipt changed visible business surface: item=%+v anchor=%+v", doc.Blocks[0].Items[0], got)
	}
	if pctx.repairCounts["normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes"] != 1 {
		t.Fatalf("production display-qualifier repair accounting missing: %+v", pctx.repairCounts)
	}
}

func TestNormalizeAnswerDocumentForPreEmitWiresStandaloneSemanticHandoffIdentityRepair(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("render the selected cross-language path")}
	recipe := types.DiagramEdgeAnchor{
		FromNode: "native", ToNode: "wrapper",
		FromIdentity: "_fastlex.tokenize_bytes", ToIdentity: "py::tokenize_bytes",
		RelationKind: types.DiagramRelRegister,
	}
	bus.Mutable.SetFinalizerTypedRelationRecipeAvailable(true)
	bus.Mutable.SetFinalizerTypedRelationSemanticHandoffAnchors([]types.DiagramEdgeAnchor{recipe})
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimRegistrationEdge}},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "native", ToNode: "wrapper", RelationKind: types.DiagramRelRegister,
		}},
	}}}
	pctx := newPreEmitCheckContext(bus)
	normalizeAnswerDocumentForPreEmit("emit_answer_document_patch", doc,
		&types.AnswerSemanticView{Family: types.QFCallChain}, bus, pctx)
	got := doc.Blocks[0].EdgeAnchors[0]
	if got.FromIdentity != recipe.FromIdentity || got.ToIdentity != recipe.ToIdentity {
		t.Fatalf("production normalizer did not consume the exact semantic handoff receipt: %+v", got)
	}
	if got.FromNode != "native" || got.ToNode != "wrapper" || got.RelationKind != types.DiagramRelRegister {
		t.Fatalf("identity repair changed model-authored topology or relation: %+v", got)
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

func TestNormalizeAnswerDocumentForPreEmitPreservesSelectedRelationsAcrossFusedCompanionSplit(t *testing.T) {
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall("main", "run"),
		diagramEvidenceTestCall("run", "walker::collect_files"),
		diagramEvidenceTestCall("collect_files", "walk"),
		diagramEvidenceTestCall("run", "index_file"),
		diagramEvidenceTestCall("index_file", "Matcher.is_match"),
	}
	for i := range evidence {
		evidence[i].ID = fmt.Sprintf("ev-%d", i+1)
		evidence[i].LineStart = 10 + i
		evidence[i].AnchorSymbol = evidence[i].Object
	}
	recipes := []types.DiagramEdgeAnchor{
		{FromNode: "n1", ToNode: "n2", FromIdentity: "main", ToIdentity: "run", RelationKind: types.DiagramRelCall},
		{FromNode: "n2", ToNode: "n3", FromIdentity: "run", ToIdentity: "walker::collect_files", RelationKind: types.DiagramRelCall},
		{FromNode: "n3", ToNode: "n4", FromIdentity: "collect_files", ToIdentity: "walk", RelationKind: types.DiagramRelCall},
		{FromNode: "n2", ToNode: "n5", FromIdentity: "run", ToIdentity: "index_file", RelationKind: types.DiagramRelCall},
		{FromNode: "n5", ToNode: "n6", FromIdentity: "index_file", ToIdentity: "Matcher.is_match", RelationKind: types.DiagramRelCall},
	}
	anchors := []types.DiagramEdgeAnchor{
		{FromNode: "main", ToNode: "run", RelationKind: types.DiagramRelCall, VisibleLabel: "run(&pattern, fixed)"},
		{FromNode: "run", ToNode: "walker", RelationKind: types.DiagramRelCall, VisibleLabel: "collect_files"},
		{FromNode: "walker", ToNode: "walk", RelationKind: types.DiagramRelCall, VisibleLabel: "walk(root, &mut out)"},
		{FromNode: "run", ToNode: "index_file", RelationKind: types.DiagramRelCall, VisibleLabel: "index_file(f, m.as_ref())"},
		{FromNode: "index_file", ToNode: "matcher", RelationKind: types.DiagramRelCall, VisibleLabel: "is_match (逐行)"},
	}
	uses := make([]types.RenderedClaimUse, len(evidence))
	for i := range evidence {
		uses[i] = types.RenderedClaimUse{ClaimForm: types.ClaimCallEdge, EvidenceID: evidence[i].ID}
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "chain", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
				ClaimUses: uses, EdgeAnchors: append([]types.DiagramEdgeAnchor(nil), anchors...)},
			{ID: "chain_diagram", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
					"sequenceDiagram",
					"  participant main",
					"  participant run as run (main.rs)",
					"  participant walker as walker::collect_files",
					"  participant walk as walk (walker.rs)",
					"  participant index_file as index_file (main.rs)",
					"  participant matcher as Matcher (matcher.rs)",
					"  main->>run: run(&pattern, fixed)",
					"  run->>walker: collect_files",
					"  walker->>walk: walk(root, &mut out)",
					"  run->>index_file: index_file(f, m.as_ref())",
					"  index_file->>matcher: is_match (逐行)",
				}, "\n")},
				EdgeAnchors: append([]types.DiagramEdgeAnchor(nil), anchors...)},
		},
		BlockCompanionLineages: []types.AnswerBlockCompanionLineage{{
			Kind: types.AnswerBlockCompanionLineageFusedDiagramSplit, VisibleBlockID: "chain", DiagramBlockID: "chain_diagram",
		}},
	}
	bus := &types.BusContext{Mutable: types.NewMutableState("render selected Rust call chain"), EvidenceItems: evidence}
	bus.Mutable.SetFinalizerTypedRelationRecipeAvailable(true)
	bus.Mutable.SetFinalizerTypedRelationRecipeAnchors(recipes)
	pctx := newPreEmitCheckContext(bus)
	normalizeAnswerDocumentForPreEmit("emit_answer_document", doc,
		&types.AnswerSemanticView{Family: types.QFCallChain}, bus, pctx)
	for bi := range doc.Blocks {
		for ai := range doc.Blocks[bi].EdgeAnchors {
			got, want := doc.Blocks[bi].EdgeAnchors[ai], recipes[ai]
			if got.FromIdentity != want.FromIdentity || got.ToIdentity != want.ToIdentity {
				t.Fatalf("block[%d] anchor[%d]=%+v, want selected identity %q -> %q", bi, ai, got, want.FromIdentity, want.ToIdentity)
			}
		}
	}
	if pctx.repairCounts["normalizeFusedDiagramCompanionEdgeAnchorIdentitiesFromClaimUses"] != 10 {
		t.Fatalf("fused companion repair accounting missing: %+v", pctx.repairCounts)
	}
	if got := DiagramCallEdgeEvidenceMismatches(doc, &types.AnswerSemanticView{Family: types.QFCallChain}, evidence); len(got) != 0 {
		t.Fatalf("exact model-selected relations must survive fused split and ordinary validation: %+v", got)
	}
}

func TestNormalizeFusedDiagramCompanionEdgeAnchorIdentitiesRequiresExactSplitLineage(t *testing.T) {
	evidence := diagramEvidenceTestCall("run", "index_file")
	evidence.ID = "ev-call"
	evidence.LineStart = 20
	evidence.AnchorSymbol = evidence.Object
	anchor := types.DiagramEdgeAnchor{
		FromNode: "run", ToNode: "index_file", RelationKind: types.DiagramRelCall,
		VisibleLabel: "index_file(f, matcher)",
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "chain", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			ClaimUses:   []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge, EvidenceID: evidence.ID}},
			EdgeAnchors: []types.DiagramEdgeAnchor{anchor}},
		{ID: "chain_diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
				"sequenceDiagram",
				"  participant run as run (main.rs)",
				"  participant index_file as index_file (main.rs)",
				"  run->>index_file: index_file(f, matcher)",
			}, "\n")},
			EdgeAnchors: []types.DiagramEdgeAnchor{anchor}},
	}}
	bus := &types.BusContext{Mutable: types.NewMutableState("no split lineage"), EvidenceItems: []types.EvidenceItem{evidence}}
	pctx := newPreEmitCheckContext(bus)
	recipes := []types.DiagramEdgeAnchor{{
		FromNode: "n1", ToNode: "n2", FromIdentity: "run", ToIdentity: "index_file", RelationKind: types.DiagramRelCall,
	}}
	if fixed := normalizeFusedDiagramCompanionEdgeAnchorIdentitiesFromClaimUses(doc, pctx, recipes); fixed != 0 {
		t.Fatalf("blocks without executor split lineage must not be repaired, fixed=%d doc=%+v", fixed, doc)
	}
	for i := range doc.Blocks {
		if got := doc.Blocks[i].EdgeAnchors[0]; got.FromIdentity != "" || got.ToIdentity != "" {
			t.Fatalf("block[%d] gained identity without split lineage: %+v", i, got)
		}
	}
}

func TestNormalizeFusedDiagramCompanionEdgeAnchorIdentitiesRejectsAmbiguousSelectedEvidence(t *testing.T) {
	first := diagramEvidenceTestCall("run", "index_file")
	first.ID = "ev-call-a"
	first.LineStart = 20
	first.AnchorSymbol = first.Object
	second := first
	second.ID = "ev-call-b"
	second.LineStart = 21
	anchor := types.DiagramEdgeAnchor{
		FromNode: "run", ToNode: "index_file", RelationKind: types.DiagramRelCall,
		VisibleLabel: "index_file(f, matcher)",
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "chain", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
				ClaimUses: []types.RenderedClaimUse{
					{ClaimForm: types.ClaimCallEdge, EvidenceID: first.ID},
					{ClaimForm: types.ClaimCallEdge, EvidenceID: second.ID},
				},
				EdgeAnchors: []types.DiagramEdgeAnchor{anchor}},
			{ID: "chain_diagram", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: strings.Join([]string{
					"sequenceDiagram",
					"  participant run as run (main.rs)",
					"  participant index_file as index_file (main.rs)",
					"  run->>index_file: index_file(f, matcher)",
				}, "\n")},
				EdgeAnchors: []types.DiagramEdgeAnchor{anchor}},
		},
		BlockCompanionLineages: []types.AnswerBlockCompanionLineage{{
			Kind: types.AnswerBlockCompanionLineageFusedDiagramSplit, VisibleBlockID: "chain", DiagramBlockID: "chain_diagram",
		}},
	}
	bus := &types.BusContext{Mutable: types.NewMutableState("ambiguous selected evidence"), EvidenceItems: []types.EvidenceItem{first, second}}
	pctx := newPreEmitCheckContext(bus)
	recipes := []types.DiagramEdgeAnchor{{
		FromNode: "n1", ToNode: "n2", FromIdentity: "run", ToIdentity: "index_file", RelationKind: types.DiagramRelCall,
	}}
	if fixed := normalizeFusedDiagramCompanionEdgeAnchorIdentitiesFromClaimUses(doc, pctx, recipes); fixed != 0 {
		t.Fatalf("ambiguous selected evidence must remain fail-closed, fixed=%d doc=%+v", fixed, doc)
	}
	for i := range doc.Blocks {
		if got := doc.Blocks[i].EdgeAnchors[0]; got.FromIdentity != "" || got.ToIdentity != "" {
			t.Fatalf("block[%d] gained identity from ambiguous evidence: %+v", i, got)
		}
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

func TestNormalizeOrphanDiagramEdgeAnchors_PreservesStandalonePrincipalRelationCarrier(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
		Items:     []types.AnswerBlockItem{{ID: "hop", Label: "业务调用"}},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "entry", ToNode: "worker",
			FromIdentity: "Entry.run", ToIdentity: "Worker.handle",
			RelationKind: types.DiagramRelCall,
		}},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	if removed := normalizeOrphanDiagramEdgeAnchors(doc, view); removed != 0 {
		t.Fatalf("standalone typed relation removed=%d: %+v", removed, doc.Blocks)
	}
	if len(doc.Blocks[0].EdgeAnchors) != 1 {
		t.Fatalf("standalone principal relation carrier was dropped: %+v", doc.Blocks[0])
	}

	trace := &types.AnswerSemanticView{Family: types.QFRootCauseTrace}
	if removed := normalizeOrphanDiagramEdgeAnchors(doc, trace); removed != 1 {
		t.Fatalf("Trace non-diagram source relation removed=%d, want 1", removed)
	}
}

func TestNormalizeOrphanDiagramEdgeAnchors_MixedStandaloneCarrierPreservesModelRelations(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
		ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
		Items:     []types.AnswerBlockItem{{ID: "hop", Label: "业务调用"}},
		EdgeAnchors: []types.DiagramEdgeAnchor{
			{
				FromNode: "entry", ToNode: "worker",
				FromIdentity: "Entry.run", ToIdentity: "Worker.handle",
				RelationKind: types.DiagramRelCall,
			},
			{
				FromNode: "native", ToNode: "binding",
				FromIdentity: "native.entry", ToIdentity: "binding.entry",
				RelationKind: types.DiagramRelRegister,
			},
		},
	}}}
	view := &types.AnswerSemanticView{Family: types.QFCallChain}
	if removed := normalizeOrphanDiagramEdgeAnchors(doc, view); removed != 0 {
		t.Fatalf("mixed carrier removed=%d, want no system-authored relation deletion: %+v", removed, doc.Blocks)
	}
	if got := doc.Blocks[0].EdgeAnchors; len(got) != 2 {
		t.Fatalf("mixed standalone carrier lost model-authored relation anchors: %+v", got)
	}
	if answerBlockCarriesStandaloneTypedRelations(doc.Blocks[0]) {
		t.Fatalf("mixed carrier must await an explicit registration claim: %+v", doc.Blocks[0])
	}
}

func TestNormalizeOrphanDiagramEdgeAnchors_StandaloneCarrierNeedsMatchingTypedClaim(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind types.AnswerBlockKind
		role types.SurfaceRole
		form types.ClaimForm
	}{
		{name: "not principal", kind: types.BlockOrderedList, form: types.ClaimCallEdge},
		{name: "not structured", kind: types.BlockSummary, role: types.SurfacePrincipal, form: types.ClaimCallEdge},
		{name: "wrong relation form", kind: types.BlockTable, role: types.SurfacePrincipal, form: types.ClaimDefinitionFact},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
				ID: "carrier", Kind: tc.kind, SurfaceRole: tc.role,
				ClaimUses: []types.RenderedClaimUse{{ClaimForm: tc.form}},
				EdgeAnchors: []types.DiagramEdgeAnchor{{
					FromNode: "A", ToNode: "B", RelationKind: types.DiagramRelCall,
				}},
			}}}
			if removed := normalizeOrphanDiagramEdgeAnchors(doc, &types.AnswerSemanticView{Family: types.QFCallChain}); removed != 1 {
				t.Fatalf("removed=%d, want 1: %+v", removed, doc.Blocks)
			}
		})
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

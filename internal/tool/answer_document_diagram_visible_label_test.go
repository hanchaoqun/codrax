package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func diagramVisibleLabelTestDocument(kind types.DiagramKind, body, anchorLabel string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "relations",
		Kind: types.BlockDiagram,
		Diagram: &types.AnswerDiagramBlock{
			Kind:     kind,
			Language: "mermaid",
			Body:     body,
		},
		EdgeAnchors: []types.DiagramEdgeAnchor{{
			FromNode: "A", ToNode: "B",
			FromIdentity: "Alpha.run", ToIdentity: "Beta.run",
			RelationKind: types.DiagramRelCall,
			VisibleLabel: anchorLabel,
		}},
	}}}
}

func TestDiagramVisibleLabelConsistencyRejectsModelAuthoredDisplayConflictAcrossFamilies(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind types.DiagramKind
		body string
	}{
		{name: "flow", kind: types.DiagramFlow, body: "flowchart TD\n  A -->|call| B"},
		{name: "architecture", kind: types.DiagramArchitecture, body: "flowchart LR\n  A -->|call| B"},
		{name: "call_dag", kind: types.DiagramCallDAG, body: "flowchart LR\n  A -->|call| B"},
		{name: "sequence", kind: types.DiagramSequence, body: "sequenceDiagram\n  participant A\n  participant B\n  A->>B: call"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := diagramVisibleLabelTestDocument(tc.kind, tc.body, "调用业务能力")
			hints := preCheckDiagramVisibleLabelConsistency(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, nil)
			if len(hints) != 1 {
				t.Fatalf("hints=%d want 1: %+v", len(hints), hints)
			}
			if !strings.Contains(hints[0].ExpectedShape, `diagram_label="call" visible_label="调用业务能力"`) ||
				len(hints[0].DiagramRelationFailureIssues) != 1 ||
				hints[0].DiagramRelationFailureIssues[0] != diagramVisibleLabelMismatch {
				t.Fatalf("unexpected exact display-consistency hint: %+v", hints[0])
			}
		})
	}
}

func TestDiagramVisibleLabelConsistencyAcceptsExactModelAuthoredWording(t *testing.T) {
	doc := diagramVisibleLabelTestDocument(
		types.DiagramFlow,
		"flowchart TD\n  A -->|调用业务能力| B",
		"调用业务能力",
	)
	if hints := preCheckDiagramVisibleLabelConsistency(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, nil); len(hints) != 0 {
		t.Fatalf("matching model-authored labels must pass: %+v", hints)
	}
}

func TestDiagramVisibleLabelConsistencyAcceptsRenderabilityQuotedWording(t *testing.T) {
	for _, body := range []string{
		`flowchart TD
  A -->|"调用业务能力"| B`,
		`flowchart TD
  A -->|'调用业务能力'| B`,
	} {
		doc := diagramVisibleLabelTestDocument(types.DiagramArchitecture, body, "调用业务能力")
		if hints := preCheckDiagramVisibleLabelConsistency(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, nil); len(hints) != 0 {
			t.Fatalf("Mermaid syntax quotes must not change visible-label identity: %+v", hints)
		}
		if doc.Blocks[0].EdgeAnchors[0].VisibleLabel != "调用业务能力" || doc.Blocks[0].Diagram.Body != body {
			t.Fatalf("comparison must not rewrite either model-authored surface: %+v", doc.Blocks[0])
		}
	}
}

func TestDiagramVisibleLabelConsistencyRejectsRawRelationEnumWithoutChoosingFinalCopy(t *testing.T) {
	doc := diagramVisibleLabelTestDocument(
		types.DiagramFlow,
		"flowchart TD\n  A -->|call| B",
		"call",
	)
	pctx := &preEmitCheckContext{ctx: &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh-CN"}}}}
	hints := preCheckDiagramVisibleLabelConsistency(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, pctx)
	if len(hints) != 1 ||
		!strings.Contains(hints[0].ExpectedShape, `visible_label repeats raw relation_kind="call" suggested_reader_label="调用"`) ||
		len(hints[0].DiagramRelationFailureIssues) != 1 ||
		hints[0].DiagramRelationFailureIssues[0] != diagramVisibleLabelRawRelationKind {
		t.Fatalf("raw relation enum did not receive exact structured-field repair guidance: %+v", hints)
	}
	if doc.Blocks[0].EdgeAnchors[0].VisibleLabel != "call" || !strings.Contains(doc.Blocks[0].Diagram.Body, "|call|") {
		t.Fatalf("display gate rewrote model-authored diagram instead of rejecting: %+v", doc.Blocks[0])
	}
}

func TestRunPreEmitChecksWiresDiagramVisibleLabelConsistency(t *testing.T) {
	doc := diagramVisibleLabelTestDocument(
		types.DiagramFlow,
		"flowchart TD\n  A -->|call| B",
		"调用业务能力",
	)
	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, nil)
	found := false
	for _, hint := range hints {
		for _, issue := range hint.DiagramRelationFailureIssues {
			if issue == diagramVisibleLabelMismatch {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("pre-emit chokepoint did not publish exact visible-label mismatch: %+v", hints)
	}
}

func TestDiagramVisibleLabelConsistencyFailsOpenWithoutUniqueStructuredJoin(t *testing.T) {
	doc := diagramVisibleLabelTestDocument(
		types.DiagramFlow,
		"flowchart TD\n  A -->|call| B",
		"调用业务能力",
	)
	doc.Blocks[0].EdgeAnchors = append(doc.Blocks[0].EdgeAnchors, types.DiagramEdgeAnchor{
		FromNode: "A", ToNode: "B",
		FromIdentity: "Alpha.run", ToIdentity: "Beta.guard",
		RelationKind: types.DiagramRelGuard,
		VisibleLabel: "条件成立",
	})
	if hints := preCheckDiagramVisibleLabelConsistency(doc, &types.AnswerSemanticView{Family: types.QFGeneric}, nil); len(hints) != 0 {
		t.Fatalf("compound edge with no one-to-one display join must fail open: %+v", hints)
	}
}

func TestDiagramVisibleLabelConsistencyDoesNotEnterRootCauseTraceAuthority(t *testing.T) {
	doc := diagramVisibleLabelTestDocument(
		types.DiagramSequence,
		"sequenceDiagram\n  participant A\n  participant B\n  A->>B: temporal adjacency",
		"不同显示",
	)
	if hints := preCheckDiagramVisibleLabelConsistency(doc, &types.AnswerSemanticView{Family: types.QFRootCauseTrace}, nil); len(hints) != 0 {
		t.Fatalf("runtime root-cause trace diagram must retain independent authority: %+v", hints)
	}
}

func TestEmitAnswerDocumentSchemaPublishesVisibleLabelConsistencyWithoutPlaceholder(t *testing.T) {
	raw := string((&EmitAnswerDocument{}).Parameters())
	if strings.Contains(raw, "__DIAGRAM_VISIBLE_LABEL_CONSISTENCY__") {
		t.Fatalf("visible-label consistency placeholder leaked into schema")
	}
	if !strings.Contains(raw, types.DiagramVisibleLabelConsistencyContract) {
		t.Fatalf("schema missing shared visible-label consistency contract")
	}
}

package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// b1649ActualCall exercises ReadFile and EmitEvidence against a real
// temporary Go source file; it does not fabricate a grounded evidence row.
// A static relation is not a runtime execution count. These examples retain
// repeated presentations of that relation, but assert no observed invocation
// count, branch outcome, exact retry limit, or message payload value.
func b1712StaticCallDocument(t *testing.T, kind types.DiagramKind, body string) (*types.BusContext, *types.AnswerDocumentV2) {
	t.Helper()
	bus, evidence := b1649ActualCall(t)
	if len(bus.EvidenceItems) != 1 || !evidence.IsCitable() ||
		evidence.GroundingStatus != types.GroundingGrounded || types.ClaimFormOf(evidence) != types.ClaimCallEdge {
		t.Fatalf("fixture must contain exactly one real grounded static call: %+v", bus.EvidenceItems)
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary,
			Text: "The diagram reuses one source-backed call relation; it is not an observed execution count."},
		{ID: "diagram", Kind: types.BlockDiagram,
			Diagram: &types.AnswerDiagramBlock{Kind: kind, Language: "mermaid", Body: strings.TrimSpace(body)},
			EdgeAnchors: []types.DiagramEdgeAnchor{{
				FromNode: "A", ToNode: "B", FromIdentity: "p.Caller", ToIdentity: "Callee",
				RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge,
			}},
		},
	}}
	return bus, doc
}

const b1712SequenceHeader = "sequenceDiagram\n  participant A as \"p.Caller\"\n  participant B as Callee\n"
const b1712GraphHeader = "flowchart TD\n  A[\"p.Caller\"]\n  B[\"Callee\"]\n"

func TestB1712PublicStaticCallRelationMayAppearRepeatedly(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind types.DiagramKind
		body string
	}{
		{"single_control", types.DiagramSequence, b1712SequenceHeader + "  A->>B: operation alpha\n"},
		{"sequence_plain", types.DiagramSequence, b1712SequenceHeader + "  A->>B: operation alpha\n  A->>B: operation beta\n"},
		{"sequence_opt", types.DiagramSequence, b1712SequenceHeader + "  A->>B: operation alpha\n  opt selected continuation\n    A->>B: operation beta\n  end\n"},
		{"sequence_loop", types.DiagramSequence, b1712SequenceHeader + "  A->>B: operation alpha\n  loop selected continuation\n    A->>B: operation beta\n  end\n"},
		{"sequence_alt", types.DiagramSequence, b1712SequenceHeader + "  alt selected branch\n    A->>B: operation alpha\n  else other branch\n    A->>B: operation beta\n  end\n"},
		{"sequence_paired_replies", types.DiagramSequence, b1712SequenceHeader + "  A->>B: operation alpha\n  B-->>A: response alpha\n  A->>B: operation beta\n  B-->>A: response beta\n"},
		{"flow", types.DiagramFlow, b1712GraphHeader + "  A -->|operation alpha| B\n  A -->|operation beta| B\n"},
		{"call_dag", types.DiagramCallDAG, b1712GraphHeader + "  A -->|operation alpha| B\n  A -->|operation beta| B\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus, doc := b1712StaticCallDocument(t, tc.kind, tc.body)
			beforeEvidence, err := json.Marshal(bus.Mutable.EmittedEvidence())
			if err != nil {
				t.Fatal(err)
			}
			result := standaloneCompleteRowExecute(t, bus, doc, false)
			if !result.Success {
				t.Fatalf("one source call proves a relation, not a cap on its visible presentations: %s", result.Summary)
			}
			published := bus.Mutable.AnswerDocumentV2()
			if published == nil {
				t.Fatal("accepted public emit did not publish a document")
			}
			got := blockByID(t, published, "diagram")
			if got.Diagram == nil || got.Diagram.Body != doc.Blocks[1].Diagram.Body || !reflect.DeepEqual(got.EdgeAnchors, doc.Blocks[1].EdgeAnchors) {
				t.Fatalf("acceptance must not collapse model-authored repetitions or mint relation anchors: got=%+v got_diagram=%+v want_diagram=%+v want_anchors=%+v", got, got.Diagram, doc.Blocks[1].Diagram, doc.Blocks[1].EdgeAnchors)
			}
			afterEvidence, err := json.Marshal(bus.Mutable.EmittedEvidence())
			if err != nil || string(beforeEvidence) != string(afterEvidence) {
				t.Fatalf("a repeated display must not mint evidence, execution events, or counts: err=%v", err)
			}
		})
	}
}

func TestB1712PublicStaticRepetitionDoesNotAuthorizeOtherRelations(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*types.AnswerBlock)
		want string
	}{
		{"unproved_target", func(block *types.AnswerBlock) {
			block.Diagram.Body = b1712SequenceHeader + "  participant C as Stranger\n  A->>C: operation alpha\n"
			block.EdgeAnchors[0].ToNode, block.EdgeAnchors[0].ToIdentity = "C", "Stranger"
		}, "call_edge_unproven"},
		{"reverse_call", func(block *types.AnswerBlock) {
			block.Diagram.Body = b1712SequenceHeader + "  B->>A: operation alpha\n"
			anchor := &block.EdgeAnchors[0]
			anchor.FromNode, anchor.ToNode = "B", "A"
			anchor.FromIdentity, anchor.ToIdentity = "Callee", "p.Caller"
		}, "call_edge_unproven"},
		{"wrong_relation_kind", func(block *types.AnswerBlock) {
			block.EdgeAnchors[0].RelationKind = types.DiagramRelReturn
			block.EdgeAnchors[0].ClaimForm = ""
		}, "return_edge_unproven"},
		{"orphan_reply", func(block *types.AnswerBlock) {
			block.Diagram.Body = b1712SequenceHeader + "  B-->>A: response alpha\n"
			block.EdgeAnchors = nil
		}, "missing_call_anchor"},
		{"extra_reply", func(block *types.AnswerBlock) {
			block.Diagram.Body += "\n  B-->>A: response alpha\n  B-->>A: response beta\n"
		}, "missing_call_anchor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus, doc := b1712StaticCallDocument(t, types.DiagramSequence, b1712SequenceHeader+"  A->>B: operation alpha\n")
			tc.edit(&doc.Blocks[1])
			result := standaloneCompleteRowExecute(t, bus, doc, false)
			if result.Success || result.Repair == nil || bus.Mutable.AnswerDocumentV2() != nil {
				t.Fatalf("static relation reuse must not bypass exact relation/reply authority: %+v", result)
			}
			if !strings.Contains(result.Summary, "issue="+tc.want) {
				t.Fatalf("must reject for the actual relation defect %q, not a fixture error: %+v", tc.want, result)
			}
		})
	}
}

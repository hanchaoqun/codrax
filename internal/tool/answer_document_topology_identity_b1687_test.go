package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1687TopologyFixture(from, to, other, unrelatedFrom, unrelatedTo, fromNode, toNode string) (*types.BusContext, *types.AnswerDocumentV2) {
	evidence := []types.EvidenceItem{
		diagramEvidenceTestCall(from, to),
		diagramEvidenceTestCall(from, other),
		diagramEvidenceTestCall(unrelatedFrom, unrelatedTo),
	}
	for i := range evidence {
		evidence[i].AnchorSymbol = evidence[i].Object
		evidence[i].LineStart += i
	}
	bus := &types.BusContext{
		Mutable:    types.NewMutableState("Explain the selected relation."),
		AnalysisIR: &types.AnalysisIR{}, EvidenceItems: evidence,
	}
	bus.Mutable.SetFinalizerTypedRelationRecipeAvailable(true)
	bus.Mutable.SetFinalizerTypedRelationRecipeAnchors([]types.DiagramEdgeAnchor{
		{FromNode: "n1", ToNode: "n2", FromIdentity: from, ToIdentity: to, RelationKind: types.DiagramRelCall},
		{FromNode: "n1", ToNode: "n3", FromIdentity: from, ToIdentity: other, RelationKind: types.DiagramRelCall},
		{FromNode: "n4", ToNode: "n5", FromIdentity: unrelatedFrom, ToIdentity: unrelatedTo, RelationKind: types.DiagramRelCall},
	})
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "The selected operation invokes its output operation."},
		{ID: "diag", Kind: types.BlockDiagram,
			Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Language: "mermaid", Body: fmt.Sprintf("sequenceDiagram\n participant %s as %q\n participant %s as %q\n %s->>%s: output", fromNode, from, toNode, to, fromNode, toNode)},
			EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: fromNode, ToNode: toNode, RelationKind: types.DiagramRelCall, ClaimForm: types.ClaimCallEdge, VisibleLabel: "output"}},
		},
	}}
	return bus, doc
}

// Both actual public entry points must preserve a valid partial view of a
// larger component. The old normalizer borrowed the sole complete one-edge
// component and made the unchanged visible endpoints contradict its own pair.
func TestB1687PublicPartialComponentDoesNotBorrowUnrelatedIdentity(t *testing.T) {
	for _, lane := range []string{"emit", "patch"} {
		for _, names := range [][5]string{
			{"ConsoleSink.write", "std::fputs", "string.c_str", "make_sink", "SinkRegistry.create"},
			{"Order.send", "Transport.write", "Buffer.bytes", "Session.open", "Registry.create"},
		} {
			for _, ids := range [][2]string{{"C", "F"}, {"businessSend", "businessOutput"}} {
				t.Run(lane+"/"+names[0]+"/"+ids[0], func(t *testing.T) {
					bus, doc := b1687TopologyFixture(names[0], names[1], names[2], names[3], names[4], ids[0], ids[1])
					view := types.BuildAnswerSemanticViewForBusContext(bus)
					if view == nil {
						t.Fatal("public validation must be enabled")
					}
					if issues := DiagramCallEdgeEvidenceMismatches(doc, view, bus.EvidenceItems); len(issues) != 0 {
						t.Fatalf("original model relation must already be valid: %+v", issues)
					}
					before := doc.Blocks[1]
					var result types.ToolResult
					var err error
					if lane == "emit" {
						raw, _ := json.Marshal(doc)
						result, err = (&EmitAnswerDocument{}).Execute(bus, raw)
					} else {
						base := *doc
						base.Blocks = append([]types.AnswerBlock(nil), doc.Blocks...)
						base.Blocks[1].EdgeAnchors = append([]types.DiagramEdgeAnchor(nil), doc.Blocks[1].EdgeAnchors...)
						base.Blocks[1].EdgeAnchors[0].FromIdentity, base.Blocks[1].EdgeAnchors[0].ToIdentity = names[0], names[1]
						diagram := *doc.Blocks[1].Diagram
						diagram.Body += fmt.Sprintf("\n participant Other as %q\n %s->>Other: inspect", names[2], ids[0])
						base.Blocks[1].Diagram = &diagram
						base.Blocks[1].EdgeAnchors = append(base.Blocks[1].EdgeAnchors, types.DiagramEdgeAnchor{
							FromNode: ids[0], ToNode: "Other", FromIdentity: names[0], ToIdentity: names[2], RelationKind: types.DiagramRelCall, VisibleLabel: "inspect",
						})
						raw, _ := json.Marshal(base)
						accepted, acceptErr := (&EmitAnswerDocument{}).Execute(bus, raw)
						if acceptErr != nil || !accepted.Success {
							t.Fatalf("explicit original selection must publish: %v %+v", acceptErr, accepted)
						}
						raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "replace_blocks": []types.AnswerBlock{doc.Blocks[1]}})
						result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
					}
					if err != nil || !result.Success {
						t.Fatalf("valid model edge must not be rebound by unrelated topology: err=%v summary=%s", err, result.Summary)
					}
					got := bus.Mutable.AnswerDocumentV2()
					if got == nil {
						t.Fatal("public normalization did not publish a document")
					}
					gotJSON, _ := json.Marshal(got.Blocks[1])
					wantJSON, _ := json.Marshal(before)
					if string(gotJSON) != string(wantJSON) {
						t.Fatalf("public normalization changed the selected model relation: got=%s want=%s", gotJSON, wantJSON)
					}
					if issues := DiagramCallEdgeEvidenceMismatches(got, view, bus.EvidenceItems); len(issues) != 0 {
						t.Fatalf("published relation must retain downstream validity: %+v", issues)
					}
				})
			}
		}
	}
}

func b1687SelectedTriangle() (*types.BusContext, *types.AnswerDocumentV2) {
	bus, doc := b1687TopologyFixture("Order.send", "Transport.write", "Buffer.bytes", "Session.open", "Registry.create", "A", "B")
	bus.EvidenceItems = []types.EvidenceItem{
		diagramEvidenceTestCall("Order.send", "Transport.write"),
		diagramEvidenceTestCall("Transport.write", "Buffer.bytes"),
		diagramEvidenceTestCall("Order.send", "Buffer.bytes"),
	}
	recipes := []types.DiagramEdgeAnchor{
		{FromNode: "n1", ToNode: "n2", FromIdentity: "Order.send", ToIdentity: "Transport.write", RelationKind: types.DiagramRelCall},
		{FromNode: "n2", ToNode: "n3", FromIdentity: "Transport.write", ToIdentity: "Buffer.bytes", RelationKind: types.DiagramRelCall},
		{FromNode: "n1", ToNode: "n3", FromIdentity: "Order.send", ToIdentity: "Buffer.bytes", RelationKind: types.DiagramRelCall},
	}
	bus.Mutable.SetFinalizerTypedRelationRecipeAnchors(recipes)
	doc.Blocks[1].Diagram.Body = "sequenceDiagram\n participant A as \"送出订单\"\n participant B as \"写入传输层\"\n participant C as \"读取字节\"\n A->>B: send\n B->>C: encode\n A->>C: inspect"
	doc.Blocks[1].EdgeAnchors = append([]types.DiagramEdgeAnchor(nil), recipes...)
	for i, nodes := range [][2]string{{"A", "B"}, {"B", "C"}, {"A", "C"}} {
		doc.Blocks[1].EdgeAnchors[i].FromNode, doc.Blocks[1].EdgeAnchors[i].ToNode = nodes[0], nodes[1]
		doc.Blocks[1].EdgeAnchors[i].VisibleLabel = []string{"send", "encode", "inspect"}[i]
		doc.Blocks[1].EdgeAnchors[i].ClaimForm = types.ClaimCallEdge
	}
	doc.Blocks[1].EdgeAnchors[2].FromIdentity, doc.Blocks[1].EdgeAnchors[2].ToIdentity = "", ""
	return bus, doc
}

func TestB1687PublicSelectedEndpointsStillRecoverMissingPair(t *testing.T) {
	for _, lane := range []string{"emit", "patch"} {
		t.Run(lane, func(t *testing.T) {
			bus, doc := b1687SelectedTriangle()
			before := *doc.Blocks[1].Diagram
			var result types.ToolResult
			var err error
			if lane == "emit" {
				raw, _ := json.Marshal(doc)
				result, err = (&EmitAnswerDocument{}).Execute(bus, raw)
			} else {
				base := *doc
				base.Blocks = []types.AnswerBlock{doc.Blocks[0]}
				raw, _ := json.Marshal(base)
				accepted, acceptErr := (&EmitAnswerDocument{}).Execute(bus, raw)
				if acceptErr != nil || !accepted.Success {
					t.Fatalf("base emit failed: %v %+v", acceptErr, accepted)
				}
				raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "add_blocks": []types.AnswerBlock{doc.Blocks[1]}})
				result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
			}
			if err != nil || !result.Success {
				t.Fatalf("existing same-diagram endpoint selections must still recover: err=%v %s", err, result.Summary)
			}
			got := bus.Mutable.AnswerDocumentV2().Blocks[1]
			if !reflect.DeepEqual(*got.Diagram, before) || len(got.EdgeAnchors) != 3 {
				t.Fatal("identity recovery changed the visible graph or edge count")
			}
			want := append([]types.DiagramEdgeAnchor(nil), doc.Blocks[1].EdgeAnchors...)
			want[2].FromIdentity, want[2].ToIdentity = "Order.send", "Buffer.bytes"
			if !reflect.DeepEqual(got.EdgeAnchors, want) {
				t.Fatalf("recovery moved other metadata: got=%+v want=%+v", got.EdgeAnchors, want)
			}
			if issues := DiagramCallEdgeEvidenceMismatches(bus.Mutable.AnswerDocumentV2(), types.BuildAnswerSemanticViewForBusContext(bus), bus.EvidenceItems); len(issues) != 0 {
				t.Fatalf("recovered selected relation must pass the ordinary evidence validator: %+v", issues)
			}
		})
	}
}

func TestB1687SelectedNodeBindingsStayWithinExactOwnership(t *testing.T) {
	for _, mutation := range []string{"same_block", "other_block", "unproved_pair", "partial_pair", "contradictory_pair", "contradictory_partial", "missing_visible_edge", "relation_mismatch", "direction_mismatch"} {
		t.Run(mutation, func(t *testing.T) {
			bus, doc := b1687SelectedTriangle()
			recipes := bus.Mutable.FinalizerTypedRelationRecipeAnchors()
			switch mutation {
			case "other_block":
				other := doc.Blocks[1]
				other.ID = "other"
				other.EdgeAnchors = append([]types.DiagramEdgeAnchor(nil), other.EdgeAnchors[:2]...)
				doc.Blocks = append(doc.Blocks, other)
				doc.Blocks[1].EdgeAnchors[0].FromIdentity, doc.Blocks[1].EdgeAnchors[0].ToIdentity = "", ""
				doc.Blocks[1].EdgeAnchors[1].FromIdentity, doc.Blocks[1].EdgeAnchors[1].ToIdentity = "", ""
			case "unproved_pair":
				doc.Blocks[1].EdgeAnchors[1].ToIdentity = "Unproved.bytes"
			case "partial_pair":
				doc.Blocks[1].EdgeAnchors[1].ToIdentity = ""
				// Keep a real ambiguity so the existing safe one-sided repair
				// cannot resolve this omission before the topology pass.
				recipes = append(recipes, types.DiagramEdgeAnchor{FromNode: "n2", ToNode: "n4", FromIdentity: "Transport.write", ToIdentity: "Other.bytes", RelationKind: types.DiagramRelCall})
			case "contradictory_pair", "contradictory_partial":
				conflict := doc.Blocks[1].EdgeAnchors[0]
				conflict.FromIdentity = "Other.send"
				if mutation == "contradictory_partial" {
					conflict.ToIdentity = ""
				}
				doc.Blocks[1].EdgeAnchors = append(doc.Blocks[1].EdgeAnchors, conflict)
			case "missing_visible_edge":
				doc.Blocks[1].Diagram.Body = strings.Replace(doc.Blocks[1].Diagram.Body, " B->>C: encode\n", "", 1)
			case "relation_mismatch":
				doc.Blocks[1].EdgeAnchors[1].RelationKind = types.DiagramRelReturn
			case "direction_mismatch":
				doc.Blocks[1].EdgeAnchors[1].FromIdentity, doc.Blocks[1].EdgeAnchors[1].ToIdentity = "Buffer.bytes", "Transport.write"
			}
			beforeBody := doc.Blocks[1].Diagram.Body
			normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes)
			got := doc.Blocks[1].EdgeAnchors[2]
			if (mutation == "same_block") != got.HasEndpointIdentityPair() {
				t.Fatalf("ownership boundary changed: anchor=%+v", got)
			}
			if doc.Blocks[1].Diagram.Body != beforeBody {
				t.Fatal("metadata recovery changed model-visible content")
			}
		})
	}
}

func TestB1687PublicExplicitUnsupportedIdentityStillFailsEvidenceGate(t *testing.T) {
	for _, lane := range []string{"emit", "patch"} {
		t.Run(lane, func(t *testing.T) {
			bus, doc := b1687TopologyFixture("Caller", "Callee", "Other", "Factory", "Registry", "A", "B")
			doc.Blocks[1].Diagram.Body = "sequenceDiagram\n participant A as \"调用者\"\n participant B as \"处理者\"\n A->>B: output"
			doc.Blocks[1].EdgeAnchors[0].FromIdentity = "Caller"
			doc.Blocks[1].EdgeAnchors[0].ToIdentity = "UnprovenMethod"
			// Even a published recipe does not replace executable call proof.
			bus.Mutable.SetFinalizerTypedRelationRecipeAnchors([]types.DiagramEdgeAnchor{{
				FromNode: "n1", ToNode: "n2", FromIdentity: "Caller", ToIdentity: "UnprovenMethod", RelationKind: types.DiagramRelCall,
			}})
			var result types.ToolResult
			var err error
			if lane == "emit" {
				raw, _ := json.Marshal(doc)
				result, err = (&EmitAnswerDocument{}).Execute(bus, raw)
			} else {
				base := *doc
				base.Blocks = []types.AnswerBlock{doc.Blocks[0]}
				raw, _ := json.Marshal(base)
				accepted, acceptErr := (&EmitAnswerDocument{}).Execute(bus, raw)
				if acceptErr != nil || !accepted.Success {
					t.Fatalf("baseline must be accepted: %v %+v", acceptErr, accepted)
				}
				raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "add_blocks": []types.AnswerBlock{doc.Blocks[1]}})
				result, err = (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
			}
			if err != nil || result.Success || !strings.Contains(result.Summary, "call_edge_unproven") {
				t.Fatalf("explicit unsupported model identity must still fail call proof: err=%v %s", err, result.Summary)
			}
			if got := bus.Mutable.AnswerDocumentV2(); got != nil {
				for _, block := range got.Blocks {
					if block.ID == "diag" {
						t.Fatal("unsupported relation reached the accepted answer")
					}
				}
			}
		})
	}
}

// The defect is not confined to a partial source component. Even a unique
// whole-graph match cannot identify model-authored business aliases that never
// selected either endpoint. Topology is not a semantic selection receipt.
func TestB1687UniqueTopologyWithoutSelectedEndpointsRemainsUnchanged(t *testing.T) {
	for _, kind := range []types.DiagramKind{types.DiagramSequence, types.DiagramFlow, types.DiagramArchitecture, types.DiagramCallDAG} {
		t.Run(string(kind), func(t *testing.T) {
			bus, doc := b1687TopologyFixture("Order.send", "Transport.write", "Buffer.bytes", "Session.open", "Registry.create", "businessSend", "businessOutput")
			doc.Blocks[1].Diagram.Kind = kind
			if kind != types.DiagramSequence {
				doc.Blocks[1].Diagram.Body = "flowchart LR\n businessSend[\"送出订单\"] --> businessOutput[\"写入传输层\"]\n"
			} else {
				doc.Blocks[1].Diagram.Body = "sequenceDiagram\n participant businessSend as 送出订单\n participant businessOutput as 写入传输层\n businessSend->>businessOutput: output\n"
			}
			recipes := bus.Mutable.FinalizerTypedRelationRecipeAnchors()[2:]
			before, _ := json.Marshal(doc)
			if n := normalizeDiagramEdgeAnchorIdentitiesFromTypedRecipes(doc, recipes); n != 0 {
				t.Fatalf("shape alone must not mint any endpoint identity: fixed=%d anchors=%+v", n, doc.Blocks[1].EdgeAnchors)
			}
			after, _ := json.Marshal(doc)
			if string(before) != string(after) || strings.Contains(string(after), "Session.open") {
				t.Fatal("unselected endpoint identity or visible content was changed")
			}
		})
	}
}

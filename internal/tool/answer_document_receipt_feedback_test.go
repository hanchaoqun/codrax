package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func receiptFeedbackView() *types.AnswerSemanticView {
	return &types.AnswerSemanticView{
		RuntimeWorkRelationContract: &types.RuntimeWorkRelationContract{Rows: []types.RuntimeWorkRelationRow{{
			ObservationID: "work-1", WorkLabel: "observed work", MeasuredDurationMS: 2,
			AllowedConclusions: []types.RuntimeWorkRelationConclusion{types.RuntimeWorkRelationConclusionRelationUnproven},
		}}},
		ConceptualTerminalResolutionContract: &types.ConceptualTerminalResolutionContract{Rows: []types.ConceptualTerminalResolutionRow{{
			EvidenceID: "operation-1", TerminalCallable: "Store.save", ExactOperation: "Buffer.write", Source: "store.go:12",
			AllowedConclusions: []types.ConceptualTerminalResolutionConclusion{types.ConceptualTerminalResolutionDestinationUnproven},
		}}},
	}
}

func receiptFeedbackHints(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView) []emitFixHint {
	var receipts []emitFixHint
	for _, hint := range runPreEmitChecks(doc, view, nil) {
		if strings.HasSuffix(hint.Field, ".runtime_work_relation") || strings.HasSuffix(hint.Field, ".conceptual_terminal_resolution") {
			receipts = append(receipts, hint)
		}
	}
	return receipts
}

func TestReceiptFeedbackCollectsBothBindingsWithoutChangingModelFields(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "runtime", Kind: types.BlockSummary, Text: "model runtime conclusion",
			RuntimeWorkRelation: &types.AnswerRuntimeWorkRelationReceipt{ObservationID: "unknown", Conclusion: types.RuntimeWorkRelationConclusionRelationUnproven}},
		{ID: "destination", Kind: types.BlockSection, Text: "model destination conclusion",
			ConceptualTerminalResolution: &types.AnswerConceptualTerminalResolutionReceipt{EvidenceID: "unknown", Conclusion: types.ConceptualTerminalResolutionDestinationUnproven}},
	}}
	before, _ := json.Marshal(doc)
	hints := receiptFeedbackHints(doc, receiptFeedbackView())
	if len(hints) != 2 {
		t.Fatalf("both exact binding errors must be reported before persist, got %+v", hints)
	}
	for i, field := range []string{"blocks[id=runtime].runtime_work_relation", "blocks[id=destination].conceptual_terminal_resolution"} {
		if hints[i].Field != field || !preEmitHintHardByDefault(hints[i]) || !strings.Contains(hints[i].ExpectedShape, "block_receipt_edits_v1") {
			t.Fatalf("receipt feedback must name stable target and existing repair channel: %+v", hints[i])
		}
	}
	after, _ := json.Marshal(doc)
	if string(before) != string(after) {
		t.Fatal("preflight changed model-authored fields")
	}
}

func TestReceiptFeedbackMatchesExistingBinders(t *testing.T) {
	for _, variant := range []string{"active", "empty", "inactive"} {
		t.Run(variant, func(t *testing.T) {
			view := receiptFeedbackView()
			switch variant {
			case "empty":
				view.RuntimeWorkRelationContract = &types.RuntimeWorkRelationContract{}
				view.ConceptualTerminalResolutionContract = &types.ConceptualTerminalResolutionContract{}
			case "inactive":
				view.RuntimeWorkRelationContract = nil
				view.ConceptualTerminalResolutionContract = nil
			}
			for _, id := range []string{"", "work-1", "operation-1", "invented"} {
				for _, conclusion := range []string{"", "relation_unproven", "destination_unproven", "requested_destination_supported", "invalid"} {
					runtime := types.AnswerRuntimeWorkRelationReceipt{ObservationID: id, Conclusion: types.RuntimeWorkRelationConclusion(conclusion)}
					terminal := types.AnswerConceptualTerminalResolutionReceipt{EvidenceID: id, Conclusion: types.ConceptualTerminalResolutionConclusion(conclusion)}
					doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: "unchanged", RuntimeWorkRelation: &runtime, ConceptualTerminalResolution: &terminal}}}
					runtimeCopy, terminalCopy := runtime, terminal
					want := 0
					if !types.BindRuntimeWorkRelationReceipt(&runtimeCopy, view.RuntimeWorkRelationContract) {
						want++
					}
					if !types.BindConceptualTerminalResolutionReceipt(&terminalCopy, view.ConceptualTerminalResolutionContract) {
						want++
					}
					if got := receiptFeedbackHints(doc, view); len(got) != want {
						t.Fatalf("id=%q conclusion=%q: preflight=%+v existing binders reject=%d", id, conclusion, got, want)
					}
					if runtime.IsBound() || terminal.IsBound() {
						t.Fatal("preflight must bind copies, not the model document")
					}
				}
			}
			if got := receiptFeedbackHints(&types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "plain", Kind: types.BlockSummary, Text: "ordinary prose"}}}, view); len(got) != 0 {
				t.Fatalf("absent receipt must not create a new obligation: %+v", got)
			}
		})
	}
}

func TestReceiptFeedbackFullAndPatchReportMixedErrorsThenAtomicRepairPersists(t *testing.T) {
	for _, lane := range []string{"full", "patch"} {
		t.Run(lane, func(t *testing.T) {
			bus := &types.BusContext{Mutable: types.NewMutableState("exact receipt feedback"),
				AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
					Intent: types.IntentTrace, PredicateAxis: types.AxisCall,
					AnalyzerHints:            types.AnalyzerHints{Kind: string(types.ReqCallChain)},
					CallChainEndpointProfile: &types.CallChainEndpointProfile{Source: "Store.save", SinkMode: types.CallChainSinkResolutionDiscoverTerminal},
				}},
				EvidenceItems: []types.EvidenceItem{{ID: "operation-1", Kind: types.EvidenceRelationship,
					Subject: "Store.save", Predicate: "calls", Object: "Buffer.write", Source: "store.go", LineStart: 12,
					Scope: types.ScopeLine, AnchorKind: types.AnchorCall, GroundingStatus: types.GroundingGrounded,
					Producer: types.EvidenceProducerRepoMapSelectedCallableBodyCall}},
			}
			blocks := []types.AnswerBlock{
				{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "Model-owned assessment.",
					ConceptualTerminalResolution: &types.AnswerConceptualTerminalResolutionReceipt{EvidenceID: "stale", Conclusion: types.ConceptualTerminalResolutionDestinationUnproven}},
				{ID: "diagram", Kind: types.BlockDiagram,
					Diagram:     &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n A[Store.save] --> B[Buffer.write]\n"},
					EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B", FromIdentity: "Store.save", ToIdentity: "Invented.write", RelationKind: types.DiagramRelCall}}},
			}
			var res types.ToolResult
			var err error
			if lane == "full" {
				raw, _ := json.Marshal(map[string]any{"blocks": blocks})
				res, err = (&EmitAnswerDocument{}).Execute(bus, raw)
			} else {
				bus.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, Text: "accepted base"}}})
				raw, _ := json.Marshal(map[string]any{"replace_blocks": blocks[:1], "add_blocks": blocks[1:]})
				res, err = (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
			}
			if err != nil || res.Success || res.Repair == nil {
				t.Fatalf("mixed invalid draft must return structured feedback: %+v %v", res, err)
			}
			for _, want := range []string{"blocks[id=summary].conceptual_terminal_resolution", "block_receipt_edits_v1", "edge_anchors"} {
				if !strings.Contains(res.Summary, want) {
					t.Fatalf("same-round feedback missing %q: %s", want, res.Summary)
				}
			}
			// Simulate the next dispatch using the exact model-authored rejected
			// base. The model selects both the legal receipt and the relation fix.
			bus.Mutable.SetPendingAnswerDocumentPatchBase(&types.AnswerDocumentV2{DocumentModel: "v2", Blocks: blocks})
			fixedDiagram := blocks[1]
			fixedDiagram.EdgeAnchors = append([]types.DiagramEdgeAnchor(nil), fixedDiagram.EdgeAnchors...)
			fixedDiagram.EdgeAnchors[0].ToIdentity = "Buffer.write"
			raw, _ := json.Marshal(map[string]any{
				"replace_blocks":         []types.AnswerBlock{fixedDiagram},
				"block_receipt_edits_v1": []any{map[string]any{"block_id": "summary", "field": "conceptual_terminal_resolution", "value": map[string]any{"evidence_id": "operation-1", "conclusion": "destination_unproven"}}},
			})
			res, err = (&EmitAnswerDocumentPatch{}).Execute(bus, raw)
			if err != nil || !res.Success {
				t.Fatalf("existing atomic receipt plus independent relation repair must persist: %+v %v", res, err)
			}
			got := bus.Mutable.AnswerDocumentV2()
			if got == nil || got.Blocks[0].Text != blocks[0].Text || !got.Blocks[0].ConceptualTerminalResolution.IsBound() ||
				got.Blocks[0].ConceptualTerminalResolution.Conclusion != types.ConceptualTerminalResolutionDestinationUnproven ||
				!reflect.DeepEqual(got.Blocks[1].Diagram, blocks[1].Diagram) {
				t.Fatalf("model prose/graph/conclusion lost through exact repair: %+v", got)
			}
			// Direct callers cannot bypass the original last-line binding guard.
			got.Blocks[0].ConceptualTerminalResolution.EvidenceID = "stale-again"
			res, err = persistMergedFinalAnswerArtifactsWithAttachmentPolicy(bus, "test", types.MutationPartial, "guard", got, nil, time.Now(), false)
			if err != nil || res.Success {
				t.Fatalf("persist binding guard was removed: %+v %v", res, err)
			}
		})
	}
}

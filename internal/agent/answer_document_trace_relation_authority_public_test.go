package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Keep the actual analyzer shape: an attached trace with IntentTrace resolves
// to root_cause_trace even when the precise request is relation_analysis.
// Replacing that legacy intent with explain would miss the production bypass.
func traceRelationDiagramContext(t *testing.T, result types.ToolResult) *types.AgentContext {
	t.Helper()
	ctx := transactionRequiredDiagramContext(t, types.DiagramSequence, 1, 1.05, []types.ToolResult{result})
	ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
	ctx.AnalysisIR.RequestModel.PerfTrace = &types.PerfBundle{}
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope: types.RuntimeQuestionScopeRelationAnalysis, FrameCausalityRequested: false,
	}
	return ctx
}

func TestTraceRelationDiagramPublicNativeAuthority(t *testing.T) {
	result := queryTransactionDiagramSupport(t, 1, 1.05)
	for _, name := range []string{"native", "zero_edges", "actor_only", "unproved_sibling", "reversed_identity", "wrong_relation"} {
		t.Run(name, func(t *testing.T) {
			ctx := traceRelationDiagramContext(t, result)
			bus := types.ToolBusContext(ctx, types.AgentFinalizer)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			view := types.BuildAnswerSemanticViewForBusContext(bus)
			if view.Family != types.QFRootCauseTrace || view.DiagramPlan == nil || !view.DiagramPlan.Required {
				t.Fatalf("fixture must retain the production family and required diagram: %+v", view)
			}
			if !strings.Contains(prompt, "Available runtime diagram anchors") || !strings.Contains(prompt, "edge_anchor=") {
				t.Fatal("actual finalizer must receive native relation identities")
			}
			rows := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
			if len(rows) != 3 {
				t.Fatalf("want three exact scoped native relations: %+v", rows)
			}
			doc := transactionSupportedDiagram(rows, types.DiagramSequence)
			block := &doc.Blocks[1]
			block.FacetIDs = []string{string(types.FacetDiagramSpine), string(types.FacetObservedArtifactFact)}
			switch name {
			case "zero_edges":
				block.Diagram.Body = "sequenceDiagram\n participant App as 应用\n participant Service as 渲染服务\n"
				block.EdgeAnchors = nil
			case "actor_only":
				block.Diagram.Body = "sequenceDiagram\n participant App as 应用\n participant Service as 渲染服务\n App-->>Service: 提交\n Service-->>App: 消费\n"
				block.EdgeAnchors = []types.DiagramEdgeAnchor{
					{FromNode: "App", ToNode: "Service", RelationKind: types.DiagramRelObserve},
					{FromNode: "Service", ToNode: "App", RelationKind: types.DiagramRelObserve},
				}
			case "unproved_sibling":
				block.Diagram.Body += " App->>Service: 待确认的交接\n"
				block.EdgeAnchors = append(block.EdgeAnchors, types.DiagramEdgeAnchor{
					FromNode: "App", ToNode: "Service", FromIdentity: "unproved-a", ToIdentity: "unproved-b", RelationKind: types.DiagramRelObserve,
				})
			case "reversed_identity":
				a := &block.EdgeAnchors[0]
				a.FromIdentity, a.ToIdentity = a.ToIdentity, a.FromIdentity
			case "wrong_relation":
				block.EdgeAnchors[0].RelationKind = types.DiagramRelWakeup
			}
			wantMissing := name == "zero_edges" || name == "actor_only"
			if got := tool.RequiredRuntimeDiagramRelationMissing(bus, doc, view); got != wantMissing {
				t.Errorf("minimum native relation missing=%t want=%t", got, wantMissing)
			}
			mismatches := tool.DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(bus, doc, view, nil)
			wantMismatch := name != "native" && name != "zero_edges"
			if (len(mismatches) > 0) != wantMismatch {
				t.Errorf("exact relation authority mismatches=%+v want failure=%t", mismatches, wantMismatch)
			}
			raw, _ := json.Marshal(doc)
			emitted, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
			if err != nil || emitted.Success != (name == "native") {
				t.Fatalf("public emit success=%t want=%t err=%v result=%+v", emitted.Success, name == "native", err, emitted)
			}
			if name == "native" {
				accepted := ctx.Mutable.AnswerDocumentV2()
				if !strings.Contains(accepted.Blocks[1].Diagram.Body, "窗外关联背景") || len(accepted.Blocks[1].EdgeAnchors) != 3 {
					t.Fatal("accepted graph lost explicit window boundaries or native branches")
				}
				shared := false
				for i, a := range block.EdgeAnchors {
					for _, b := range block.EdgeAnchors[i+1:] {
						shared = shared || (a.ToIdentity == b.ToIdentity && a.FromIdentity != b.FromIdentity)
					}
				}
				if !shared {
					t.Fatal("fixture lost the two submissions sharing one physical consumption")
				}
			}
		})
	}
}

func TestTraceRelationDiagramPublicCausalAuthorityRemainsSeparate(t *testing.T) {
	result := queryTransactionDiagramSupport(t, 1, 1.05)
	for _, name := range []string{"causal_diagnosis", "frame_relation", "causal_work_relation", "unspecified", "missing_profile"} {
		t.Run(name, func(t *testing.T) {
			ctx := traceRelationDiagramContext(t, result)
			p := ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile
			switch name {
			case "causal_diagnosis", "causal_work_relation":
				p.Scope = types.RuntimeQuestionScopeCausalDiagnosis
				p.RuntimeWorkRelationRequested = name == "causal_work_relation"
			case "frame_relation":
				p.FrameCausalityRequested = true
			case "unspecified":
				p.Scope = types.RuntimeQuestionScopeUnspecified
			case "missing_profile":
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = nil
			}
			bus := types.ToolBusContext(ctx, types.AgentFinalizer)
			_ = (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			view := types.BuildAnswerSemanticViewForBusContext(bus)
			if view.Family != types.QFRootCauseTrace || (view.DiagramPlan != nil && view.DiagramPlan.RequireStructuralEdge) {
				t.Fatalf("causal/legacy diagrams must keep their separate authority: %+v", view)
			}
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "graph", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Body: "sequenceDiagram\n participant A\n participant B\n A->>B: observation"},
			}}}
			if tool.RequiredRuntimeDiagramRelationMissing(bus, doc, view) || len(tool.DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(bus, doc, view, nil)) != 0 {
				t.Fatal("native relation gate must not replace genuine causal/frame projection qualification")
			}
		})
	}
}

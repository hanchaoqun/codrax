package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc241PairedAttach(t *testing.T) (*BaseAgent, *types.AgentContext, json.RawMessage) {
	t.Helper()
	mut := types.NewMutableState("attach a supported relation without changing the body")
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "preserved"},
		{ID: "flow", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Language: "mermaid", Body: "flowchart LR\n A -->|input| B\n"}},
	}}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	lease := types.NewAnswerDiagramRelationRepairLease(doc, []types.AnswerDiagramRelationRepairFailure{{
		BlockID: "flow", Issue: "missing_relation_anchor", FromNode: "A", ToNode: "B", FromIdentity: "pkg.A.run", ToIdentity: "pkg.B.accept", BodyOccurrence: 1,
	}}, []types.AnswerDiagramRelationRepairCandidate{{BlockID: "flow", RelationKind: types.DiagramRelArgumentFlow,
		FromIdentity: "pkg.A.run", ToIdentity: "pkg.B.accept", Source: "source.go:10"}})
	if lease == nil || len(lease.Failures) != 1 || !lease.Failures[0].AllowsAction("attach") {
		t.Fatalf("missing actual paired attach lease: %+v", lease)
	}
	mut.SetAnswerDiagramRelationRepairLease(lease)
	ctx := &types.AgentContext{AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: mut}
	reg := tool.NewRegistry()
	reg.Register(&tool.EmitAnswerDocumentPatch{})
	base := NewBaseAgent(types.AgentFinalizer, &Dependencies{Tools: reg,
		ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentFinalizer: {Mode: types.ToolParamCompatRepair}}}, nil)
	raw := json.RawMessage(fmt.Sprintf(`{"unchanged_block_ids":["summary"],"diagram_edge_edits":[{"action":"attach","failure_ref":%q,"addition_ref":%q,"edge":{"from_node":"A","to_node":"B","visible_label":"input"}}]}`, lease.Failures[0].FailureRef, lease.AllowedAdditions[0].AdditionRef))
	if err := toolparam.Validate(raw, (&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx)); err != nil {
		t.Fatalf("the production dispatch must publish this exact valid branch: %v", err)
	}
	return base, ctx, raw
}

func TestHMC241ExecuteToolUsesCurrentDispatchSchemaForRepair(t *testing.T) {
	for _, normalizedFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(normalizedFirst), func(t *testing.T) {
			base, ctx, raw := hmc241PairedAttach(t)
			var obj map[string]json.RawMessage
			_ = json.Unmarshal(raw, &obj)
			obj["diagram_edge_edits"], _ = json.Marshal(string(obj["diagram_edge_edits"]))
			wrapped, _ := json.Marshal(obj)
			call := llm.ToolCall{ID: "hmc241-attach", Name: "emit_answer_document_patch", Params: wrapped}
			if normalizedFirst {
				actual := base.buildToolSchemas(&skill.Config{ToolSuggestions: []string{call.Name}}, ctx)
				call = base.normalizeToolCallParamsWithContext(ctx, []llm.ToolCall{call}, actual)[0]
				if call.ParamSchemaValidationError != "" {
					t.Fatalf("actual dispatch rejected its own current branch: %+v", call)
				}
			}
			result, _ := base.executeTool(ctx, call)
			if result == nil || !result.Success {
				t.Fatalf("schema-valid live attach was rejected by registry fallback: %+v", result)
			}
			doc := ctx.Mutable.AnswerDocumentV2()
			if doc == nil || doc.Blocks[0].Text != "preserved" || len(doc.Blocks[1].EdgeAnchors) != 1 || strings.Count(doc.Blocks[1].Diagram.Body, "A -->") != 1 {
				t.Fatalf("attach lost the supported relation or changed unrelated content: %+v", doc)
			}
		})
	}
}

func TestHMC241DispatchSchemaDoesNotGrantStaleRelationAuthority(t *testing.T) {
	for _, negative := range []string{"unknown_ref", "missing_lease", "wrong_action"} {
		t.Run(negative, func(t *testing.T) {
			base, ctx, raw := hmc241PairedAttach(t)
			var obj map[string]any
			_ = json.Unmarshal(raw, &obj)
			edit := obj["diagram_edge_edits"].([]any)[0].(map[string]any)
			switch negative {
			case "unknown_ref":
				edit["addition_ref"] = "ra1-not-current"
			case "missing_lease":
				ctx.Mutable.SetAnswerDiagramRelationRepairLease(nil)
			case "wrong_action":
				edit["action"] = "invented"
			}
			before, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
			params, _ := json.Marshal(obj)
			// A schema fingerprint is a normalization optimization, not a
			// lease, relation proof, or permission to mutate the answer.
			result, _ := base.executeTool(ctx, llm.ToolCall{ID: "hmc241-negative", Name: "emit_answer_document_patch", Params: params,
				ParamSchemaFingerprint: toolParamSchemaFingerprint((&tool.EmitAnswerDocumentPatch{}).ParametersFor(ctx))})
			if result == nil || result.Success {
				t.Fatalf("invalid relation operation gained authority: %+v", result)
			}
			after, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
			if string(before) != string(after) {
				t.Fatal("rejected operation mutated the answer")
			}
		})
	}
}

func TestHMC241NativeProducerAttachThroughAgentRepair(t *testing.T) {
	ctx := traceRelationDiagramContext(t, queryTransactionDiagramSupport(t, 1, 1.05))
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	_ = (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	rows := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
	if len(rows) == 0 {
		t.Fatal("real query lost native relation")
	}
	doc := transactionSupportedDiagram(rows[:1], types.DiagramSequence)
	doc.Blocks[1].FacetIDs = []string{string(types.FacetDiagramSpine), string(types.FacetObservedArtifactFact)}
	doc.Blocks[1].EdgeAnchors = nil
	raw, _ := json.Marshal(doc)
	rejected, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || rejected.Success || !installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &rejected, false) {
		t.Fatalf("actual producer must publish a repair: %v %+v", err, rejected)
	}
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	var selected map[string]any
	for _, failure := range lease.Failures {
		for _, candidate := range lease.AllowedAdditions {
			if failure.AllowsAction("attach") && types.AnswerDiagramRelationRepairFailureCanAttachCandidate(failure, candidate) {
				selected = map[string]any{"action": "attach", "failure_ref": failure.FailureRef, "addition_ref": candidate.AdditionRef,
					"edge": map[string]string{"from_node": failure.FromNode, "to_node": failure.ToNode, "visible_label": "协议记录对应"}}
			}
		}
	}
	if selected == nil {
		t.Fatalf("actual query/emit repair did not offer paired attach: %+v", lease)
	}
	items, _ := json.Marshal([]any{selected})
	params, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": string(items)})
	reg := tool.NewRegistry()
	reg.Register(&tool.EmitAnswerDocumentPatch{})
	base := NewBaseAgent(types.AgentFinalizer, &Dependencies{Tools: reg,
		ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentFinalizer: {Mode: types.ToolParamCompatRepair}}}, nil)
	result, _ := base.executeTool(ctx, llm.ToolCall{ID: "hmc241-native", Name: "emit_answer_document_patch", Params: params})
	if result == nil || !result.Success {
		t.Fatalf("actual native relation failed current-schema fallback repair: %+v", result)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil || len(got.Blocks[1].EdgeAnchors) != 1 || got.Blocks[1].EdgeAnchors[0].RelationKind != rows[0].Kind ||
		len(tool.DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(bus, got, types.BuildAnswerSemanticViewForBusContext(bus), nil)) != 0 {
		t.Fatalf("native relation lost its original authority: %+v", got)
	}
}

func TestHMC241RegistryMatchesProjectedToolSchemas(t *testing.T) {
	targets := []tool.Tool{&tool.EmitAnswerDocument{}, &tool.EmitAnswerDocumentPatch{}, &tool.EmitWriteWorkflowDecision{}, &tool.EmitChangePlan{}, &tool.EmitPlanSkeleton{}, &tool.RunTests{}, &tool.ReadFile{}}
	reg := tool.NewRegistry()
	for _, target := range targets {
		reg.Register(target)
	}
	base := NewBaseAgent(types.AgentFinalizer, &Dependencies{Tools: reg}, nil)
	for _, ctx := range []*types.AgentContext{nil,
		{Stage: types.StageFinalize, Mutable: types.NewMutableState("finalize")},
		{Stage: types.StagePlan, Mode: types.ModePlan, Mutable: types.NewMutableState("plan")},
		{Stage: types.StageVerify, Mode: types.ModeApply, Mutable: types.NewMutableState("verify")},
	} {
		for _, target := range targets {
			want := target.Parameters()
			if projected, ok := target.(interface {
				ParametersFor(*types.AgentContext) json.RawMessage
			}); ok {
				want = projected.ParametersFor(ctx)
			}
			if _, ok := target.(*tool.RunTests); ok && ctx != nil {
				if ctx.Stage == types.StagePlan {
					want = writePlannerRunTestsParameters(want)
				}
				if ctx.Stage == types.StageVerify {
					want = writeVerifierRunTestsParameters(want)
				}
			}
			got, ok := base.toolParamSchemaFromRegistry(ctx, target.Name())
			if !ok || string(got) != string(want) {
				t.Fatalf("%s registry schema diverged from its public current projection", target.Name())
			}
		}
	}
}

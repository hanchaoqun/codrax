package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the actual IR compiler and finalizer, not a hand-authored participant
// contract. The accepted request quote is intentionally unlike the native IDs.
func requestedThreadAgentContextHMC242(t *testing.T, identities ...string) *types.AgentContext {
	t.Helper()
	const question = "看看附件中 1.000 到 1.020 秒 target 线程（tid 42）被谁唤醒了。逐次列出时间、双方线程和能确认的进程信息，再画一张唤醒时序图。"
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_event_participants/wakeups.systrace")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{Objective: question, RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentAnalyzer, Stage: types.StageAnalyze,
		AttachedHitrace: string(data), AttachedHitraceSource: path, Mutable: types.NewMutableState(question)}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: path, Signals: []string{"sched_switch", "sched_wakeup"}}})
	raw := json.RawMessage(`{
		"intent":"trace","scenario":"generic","complexity":"moderate","language":"zh",
		"keywords":["wakeup"],"entities":["target 线程（tid 42）"],
		"question_kind":"call_chain","predicate_axis":"call",
		"intent_confidence":0.95,"complexity_confidence":0.95,"kind_confidence":0.95,
		"predicates":{"is_scalar_answer":false,"is_role_locate_lookup":false,"is_count_question":false,"is_cross_component":false,"is_relational_lookup":true,"is_category_enumeration":false,"is_history_lookup":false,"is_diagnostic_question":false,"has_per_member_table":true},
		"diagnostic_profile":{"is_diagnostic":false,"current_risk":false,"historical_regression":false,"current_version_check":false,"confidence":0.95},
		"answer_role_profile":{"is_role_binding_requested":false,"confidence":0.95},
		"error_granularity_profile":{"is_granularity_question":false,"confidence":0.95},
		"runtime_artifact_scope_profile":{"requested_scope":"explicit_time_window","time_start":1,"time_end":1.02,"source_quote":"1.000 到 1.020 秒","confidence":0.95},
		"runtime_question_profile":{"scope":"relation_analysis","runtime_work_relation_requested":false,"frame_causality_requested":false,"confidence":0.95},
		"runtime_target_profile":{"declaration":"named_target","source_quote":"target 线程（tid 42）","confidence":0.95},
		"runtime_targets":[{"kind":"thread","pid":42,"thread":"target-42","source":"user_explicit","confidence":0.95}],
		"history_selection_profile":{"mode":"not_applicable","item_kind":"not_applicable","confidence":0.95},
		"completeness_obligation":{"required":false,"source_quote":""},
		"call_chain_endpoints":{"source":"","sink":"","sink_mode":"exact","runtime_selection_required":false,"runtime_selection_source_quote":""},
		"runtime_selection_profile":{"is_selection_question":false,"source_quote":"","confidence":0.95},
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.95,"dimensions":[{"index":1,"label":"唤醒时序图","role":"diagram","source_quote":"画一张唤醒时序图","required":true}]},
		"diagram_hint":{"required":true,"kind":"sequence","relation_scope_quote":"target 线程（tid 42）被谁唤醒了","participants":[{"identity":"target 线程（tid 42）","source_quote":"target 线程（tid 42）","role":"incident_required"}]}
	}`)
	if len(identities) > 0 {
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		payload["entities"] = []any{identities[0]}
		payload["diagram_hint"].(map[string]any)["participants"].([]any)[0].(map[string]any)["identity"] = identities[0]
		raw, err = json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	res, err := (&tool.EmitAnalysis{}).Execute(types.ToolBusContext(ctx, types.AgentAnalyzer), raw)
	if err != nil || !res.Success {
		t.Fatalf("public analyzer: %v %s", err, res.Summary)
	}
	ctx.AnalysisIR, err = buildAnalysisIR(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ctx.AgentName, ctx.Stage = types.AgentFinalizer, types.StageFinalize
	query := json.RawMessage(`{"source":"attached_trace","view":"wakeup_chain","time_start":1,"time_end":1.02}`)
	res, err = (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), query)
	if err != nil || !res.Success {
		t.Fatalf("public native query: %v %s", err, res.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{res}})
	return ctx
}

func TestRequestedThreadShortIdentityActualFinalizerHMC242(t *testing.T) {
	ctx := requestedThreadAgentContextHMC242(t, "target")
	if got := ctx.AnalysisIR.RequestModel.DiagramHint.Participants[0]; got.Identity != "target" || got.SourceQuote != "target 线程（tid 42）" {
		t.Fatalf("accepted analyzer shape changed: %+v", got)
	}
	rows := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
	if len(rows) != 3 {
		t.Fatalf("want three native events: %+v", rows)
	}
	doc := transactionSupportedDiagram(rows, types.DiagramSequence)
	doc.Blocks[0].Text = "记录三次唤醒，不表示唤醒者造成了全部等待。"
	doc.Blocks[0].SurfaceRole = types.SurfacePrincipal
	doc.Blocks[0].TraceCausalClaimCaliber = types.TraceCausalClaimNoConclusion
	doc.Blocks[0].FacetIDs = []string{string(types.FacetObservedArtifactFact)}
	doc.Blocks[1].FacetIDs = []string{string(types.FacetDiagramSpine), string(types.FacetObservedArtifactFact)}
	raw, _ := json.Marshal(doc)
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	result, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || !result.Success {
		t.Fatalf("accepted short identity cannot reuse selected native thread: %v %s", err, result.Summary)
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, `"participant":"target"`) || !strings.Contains(prompt, "requested_participant_binding") {
		t.Fatal("actual finalizer did not receive short identity's current binding")
	}
	if got := tool.DiagramParticipantCoverageMismatchesWithRuntimeContext(bus, ctx.Mutable.AnswerDocumentV2(), types.BuildAnswerSemanticViewForBusContext(bus), nil); len(got) > 0 {
		t.Fatalf("finalizer coverage disagrees: %+v", got)
	}
}

func TestRequestedThreadActualFinalizerRepairHMC242(t *testing.T) {
	ctx := requestedThreadAgentContextHMC242(t)
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	view := types.BuildAnswerSemanticViewForBusContext(bus)
	rows := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
	if view.DiagramPlan == nil || !view.DiagramPlan.Required || len(view.DiagramParticipantObligations) != 1 {
		t.Fatalf("actual IR did not retain the requested participant: hint=%+v contract=%+v profile=%+v targets=%+v rows=%d view=%+v", ctx.AnalysisIR.RequestModel.DiagramHint, ctx.AnalysisIR.AnswerContract.Diagram, ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile, ctx.AnalysisIR.RequestModel.RuntimeTargets, len(rows), view)
	}
	if !strings.Contains(prompt, "requested_participant_binding") || !strings.Contains(prompt, "target 线程（tid 42）") {
		t.Fatal("finalizer did not receive the precise native participant binding")
	}
	if len(rows) != 3 {
		t.Fatalf("want three original native events, got %d", len(rows))
	}
	doc := transactionSupportedDiagram(rows, types.DiagramSequence)
	doc.Blocks[0].Text = "记录三次唤醒，不表示唤醒者造成了全部等待。"
	doc.Blocks[0].SurfaceRole = types.SurfacePrincipal
	doc.Blocks[0].TraceCausalClaimCaliber = types.TraceCausalClaimNoConclusion
	doc.Blocks[0].FacetIDs = []string{string(types.FacetObservedArtifactFact)}
	doc.Blocks[1].FacetIDs = []string{string(types.FacetDiagramSpine), string(types.FacetObservedArtifactFact)}
	// Remove the unique worker event anchor. Two loader occurrences share a
	// display pair; removing both would correctly require explicit placement,
	// rather than licensing an ambiguous automatic attach.
	var retained []types.DiagramEdgeAnchor
	for i, row := range rows {
		if row.FromLabel != "worker-13" {
			retained = append(retained, doc.Blocks[1].EdgeAnchors[i])
		}
	}
	if len(retained) != 2 {
		t.Fatal("fixture lost its unique worker event")
	}
	doc.Blocks[1].EdgeAnchors = retained
	raw, _ := json.Marshal(doc)
	rejected, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
	if err != nil || rejected.Success || !installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &rejected, false) {
		t.Fatalf("unanchored native edges must yield a real repair lease: %v %s", err, rejected.Summary)
	}
	lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
	var edits []any
	for _, failure := range lease.Failures {
		for _, candidate := range lease.AllowedAdditions {
			if failure.AllowsAction("attach") && types.AnswerDiagramRelationRepairFailureCanAttachCandidate(failure, candidate) {
				edits = append(edits, map[string]any{"action": "attach", "failure_ref": failure.FailureRef, "addition_ref": candidate.AdditionRef,
					"edge": map[string]string{"from_node": failure.FromNode, "to_node": failure.ToNode, "visible_label": "协议记录对应"}})
				break
			}
		}
	}
	if len(edits) != 1 {
		t.Fatalf("expected one local anchor edit preserving both other events, got %d; lease=%+v", len(edits), lease)
	}
	items, _ := json.Marshal(edits)
	params, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": string(items)})
	reg := tool.NewRegistry()
	reg.Register(&tool.EmitAnswerDocumentPatch{})
	base := NewBaseAgent(types.AgentFinalizer, &Dependencies{Tools: reg,
		ToolParamCompatByAgent: map[types.AgentName]types.ToolParamCompatConfig{types.AgentFinalizer: {Mode: types.ToolParamCompatRepair}}}, nil)
	call := llm.ToolCall{ID: "hmc242-native-participant", Name: "emit_answer_document_patch", Params: params}
	schemas := base.buildToolSchemas(&skill.Config{ToolSuggestions: []string{call.Name}}, ctx)
	call = base.normalizeToolCallParamsWithContext(ctx, []llm.ToolCall{call}, schemas)[0]
	if call.ParamSchemaValidationError != "" {
		t.Fatalf("current schema rejected its local repair branch: %+v", call)
	}
	result, _ := base.executeTool(ctx, call)
	if result == nil || !result.Success {
		t.Fatalf("real agent could not repair accepted native participant: %+v", result)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil || got.Blocks[0].Text != doc.Blocks[0].Text || len(got.Blocks[1].EdgeAnchors) != 3 {
		t.Fatalf("local repair lost native events or changed unrelated prose: %+v", got)
	}
	if mismatches := tool.DiagramParticipantCoverageMismatchesWithRuntimeContext(bus, got, types.BuildAnswerSemanticViewForBusContext(bus), nil); len(mismatches) != 0 {
		t.Fatalf("post-finalizer disagrees with exact local binding: %+v", mismatches)
	}
}

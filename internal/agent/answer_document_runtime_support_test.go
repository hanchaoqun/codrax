package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeRequiredDiagramContext(t *testing.T, kind types.DiagramKind, results []types.ToolResult) *types.AgentContext {
	t.Helper()
	start, end := 5.0, 5.041
	rm := types.RequestModel{Language: "zh", Intent: types.IntentTrace, PredicateAxis: types.AxisFlow,
		RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit"}, {Kind: types.RuntimeTargetKindThread, PID: 200, Source: "user_explicit"}},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState, types.RuntimeQuestionFactRecordedReason}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "5.000–5.041 秒"},
		DiagramHint:                 &types.DiagramHint{Kind: kind, Required: true}}
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("请画出这些线程的唤醒关系"), AnalysisIR: &types.AnalysisIR{RequestModel: rm,
			AnswerContract: types.AnswerContract{Diagram: &types.DiagramContract{Required: true, Minimum: 1, RequiredKind: kind, PreferredKinds: []types.DiagramKind{kind}}}}}
	for _, result := range results {
		ctx.Mutable.AppendDispatchToolResult(result)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	return ctx
}

func TestRuntimeRequiredDiagramPublicSupport(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_sleep_dependencies/events.systrace")
	queryCtx := runtimeRequiredDiagramContext(t, types.DiagramSequence, nil)
	args, _ := json.Marshal(map[string]any{"path": path, "view": "event_search", "event_types": []string{"sched_wakeup"}, "time_start": 5, "time_end": 5.041})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(queryCtx, types.AgentExplorer), args)
	if err != nil || !result.Success {
		t.Fatalf("public query: %v %+v", err, result)
	}
	for _, kind := range []types.DiagramKind{types.DiagramSequence, types.DiagramFlow, types.DiagramCallDAG} {
		t.Run(string(kind), func(t *testing.T) {
			ctx := runtimeRequiredDiagramContext(t, kind, []types.ToolResult{result})
			baseline := types.BuildAnswerSurfacePlan(ctx.AnalysisIR, ctx.Mutable, nil, nil, nil, nil)
			if baseline.Diagram != nil && baseline.Diagram.Required {
				t.Fatal("regression fixture unexpectedly has source-backed diagram support")
			}
			bus := types.ToolBusContext(ctx, types.AgentFinalizer)
			for _, plan := range []*types.AnswerSurfacePlan{types.BuildAnswerSurfacePlanForAgentContext(ctx), types.BuildAnswerSurfacePlanForBusContext(bus)} {
				if plan.Diagram == nil || !plan.Diagram.Required || plan.Diagram.RequiredKind != kind || plan.DiagramHardRequirementDropped {
					t.Fatalf("verified runtime relation silently lost required %s: %+v", kind, plan.Diagram)
				}
				if plan.CompiledDiagramFence != "" {
					t.Fatal("runtime support must not author a replacement diagram")
				}
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if !strings.Contains(prompt, "## Diagram Contract") || strings.Contains(prompt, "## Diagram Preference") || !strings.Contains(prompt, "edge_anchor=") {
				t.Fatal("actual finalizer lost the required contract or exact event recipes")
			}
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal,
				TraceCausalClaimCaliber: "no_causal_conclusion", Text: "已记录四次唤醒，未据此判断等待根因。"}}}
			raw, _ := json.Marshal(doc)
			missing, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
			if err != nil || missing.Success {
				t.Fatalf("required graph could be silently deleted: %v %+v", err, missing)
			}
			rows := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
			if len(rows) != 4 {
				t.Fatalf("want four physical events, got %d", len(rows))
			}
			var body strings.Builder
			if kind == types.DiagramSequence {
				body.WriteString("sequenceDiagram\n")
			} else {
				body.WriteString("flowchart TD\n")
			}
			seen := map[string]bool{}
			var anchors []types.DiagramEdgeAnchor
			for _, row := range rows {
				for _, node := range []struct{ id, label string }{{row.FromNode, row.FromLabel}, {row.ToNode, row.ToLabel}} {
					if seen[node.id] {
						continue
					}
					seen[node.id] = true
					if kind == types.DiagramSequence {
						fmt.Fprintf(&body, " participant %s as %s\n", node.id, node.label)
					} else {
						fmt.Fprintf(&body, " %s[\"%s\"]\n", node.id, node.label)
					}
				}
				if kind == types.DiagramSequence {
					fmt.Fprintf(&body, " %s->>%s: 唤醒\n", row.FromNode, row.ToNode)
				} else {
					fmt.Fprintf(&body, " %s -->|唤醒| %s\n", row.FromNode, row.ToNode)
				}
				anchors = append(anchors, types.DiagramEdgeAnchor{FromNode: row.FromNode, ToNode: row.ToNode, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity, RelationKind: row.Kind})
			}
			doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "diagram", Kind: types.BlockDiagram,
				Diagram: &types.AnswerDiagramBlock{Kind: kind, Language: "mermaid", Body: body.String()}, EdgeAnchors: anchors})
			raw, _ = json.Marshal(doc)
			accepted, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
			if err != nil || !accepted.Success {
				t.Fatalf("producer-backed graph: %v %+v", err, accepted)
			}
		})
	}

	for _, tc := range []struct {
		name   string
		mutate func(*types.ToolResult, *types.RequestModel)
	}{
		{"no_observations", func(r *types.ToolResult, _ *types.RequestModel) { r.Observations = nil }},
		{"summary_only", func(r *types.ToolResult, _ *types.RequestModel) {
			for i := range r.Observations {
				r.Observations[i].RichNotes = nil
			}
		}},
		{"model_producer", func(r *types.ToolResult, _ *types.RequestModel) {
			for i := range r.Observations {
				r.Observations[i].Producer = "model"
			}
		}},
		{"missing_query_receipt", func(r *types.ToolResult, _ *types.RequestModel) {
			r.RawRef = ""
			for i := range r.Observations {
				r.Observations[i].SourceRef.PayloadRef, r.Observations[i].SourceRef.RawRef = "", ""
			}
		}},
		{"outside_requested_window", func(_ *types.ToolResult, rm *types.RequestModel) {
			s, e := 8.0, 8.1
			rm.RuntimeArtifactScopeProfile.TimeStart, rm.RuntimeArtifactScopeProfile.TimeEnd = &s, &e
		}},
		{"other_target", func(_ *types.ToolResult, rm *types.RequestModel) {
			rm.RuntimeTargets = []types.RuntimeTarget{{PID: 999, Source: "user_explicit"}}
		}},
		{"conflicting_event", func(r *types.ToolResult, _ *types.RequestModel) {
			var conflicts []types.ObservationRecord
			for _, row := range r.Observations {
				event, valid := types.DecodeRuntimeWakeupEvent(row)
				if !valid {
					continue
				}
				row.ID += ":conflict"
				event.Timestamp += .0000001
				row.Span.StartTs, row.Span.EndTs = event.Timestamp, event.Timestamp
				body, _ := json.Marshal(event)
				row.RichNotes = []string{types.TraceNoteKeyWakeupEventInstance + "=" + string(body)}
				conflicts = append(conflicts, row)
			}
			r.Observations = append(r.Observations, conflicts...)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(result)
			var copyResult types.ToolResult
			if err := json.Unmarshal(raw, &copyResult); err != nil {
				t.Fatal(err)
			}
			ctx := runtimeRequiredDiagramContext(t, types.DiagramSequence, nil)
			tc.mutate(&copyResult, &ctx.AnalysisIR.RequestModel)
			ctx.Mutable.AppendDispatchToolResult(copyResult)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{copyResult}})
			plan := types.BuildAnswerSurfacePlanForAgentContext(ctx)
			if plan.Diagram != nil && plan.Diagram.Required {
				t.Fatalf("unproved/foreign runtime row required a diagram: %+v", plan.Diagram)
			}
			if len(tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)) != 0 {
				t.Fatal("shape and edge authority diverged")
			}
		})
	}
}

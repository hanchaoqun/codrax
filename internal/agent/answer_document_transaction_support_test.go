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

func transactionRequiredDiagramContext(t *testing.T, kind types.DiagramKind, start, end float64, results []types.ToolResult) *types.AgentContext {
	t.Helper()
	ctx := runtimeRequiredDiagramContext(t, kind, results)
	rm := &ctx.AnalysisIR.RequestModel
	rm.Intent = types.IntentExplain
	rm.RuntimeTargets = nil
	rm.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue}
	rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "事务交接时间窗"}
	return ctx
}

func queryTransactionDiagramSupport(t *testing.T, start, end float64) types.ToolResult {
	t.Helper()
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_transaction_handoffs/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx := transactionRequiredDiagramContext(t, types.DiagramFlow, start, end, nil)
	args, _ := json.Marshal(map[string]any{"path": path, "view": "transaction_handoffs", "time_start": start, "time_end": end})
	r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !r.Success {
		t.Fatalf("real transaction query: %v %+v", err, r)
	}
	return r
}

func transactionSupportedDiagram(rows []tool.RuntimeDiagramRelation, kind types.DiagramKind) *types.AnswerDocumentV2 {
	var body strings.Builder
	if kind == types.DiagramSequence {
		body.WriteString("sequenceDiagram\n")
	} else {
		body.WriteString("flowchart TD\n")
	}
	seen := map[string]bool{}
	var anchors []types.DiagramEdgeAnchor
	for _, row := range rows {
		for _, n := range []struct{ id, label string }{{row.FromNode, row.FromLabel}, {row.ToNode, row.ToLabel}} {
			if seen[n.id] {
				continue
			}
			seen[n.id] = true
			if kind == types.DiagramSequence {
				fmt.Fprintf(&body, " participant %s as %s\n", n.id, n.label)
			} else {
				fmt.Fprintf(&body, " %s[%q]\n", n.id, n.label)
			}
		}
		if kind == types.DiagramSequence {
			fmt.Fprintf(&body, " %s->>%s: 协议记录对应\n", row.FromNode, row.ToNode)
		} else {
			fmt.Fprintf(&body, " %s -->|协议记录对应| %s\n", row.FromNode, row.ToNode)
		}
		anchors = append(anchors, types.DiagramEdgeAnchor{FromNode: row.FromNode, ToNode: row.ToNode, FromIdentity: row.FromIdentity, ToIdentity: row.ToIdentity, RelationKind: row.Kind, VisibleLabel: "协议记录对应"})
	}
	return &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "已发布记录内可对应的提交与消费，不表示调用、线程等待或整帧根因。"},
		{ID: "handoffs", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: kind, Language: "mermaid", Body: body.String()}, EdgeAnchors: anchors},
	}}
}

func TestTransactionRequiredDiagramPublicPlanEmitPost(t *testing.T) {
	r := queryTransactionDiagramSupport(t, 1, 1.05)
	for _, kind := range []types.DiagramKind{types.DiagramSequence, types.DiagramFlow, types.DiagramCallDAG} {
		t.Run(string(kind), func(t *testing.T) {
			ctx := transactionRequiredDiagramContext(t, kind, 1, 1.05, []types.ToolResult{r})
			bus := types.ToolBusContext(ctx, types.AgentFinalizer)
			if base := types.BuildAnswerSurfacePlan(ctx.AnalysisIR, ctx.Mutable, nil, nil, nil, nil); base.Diagram != nil && base.Diagram.Required {
				t.Fatal("fixture unexpectedly has source-backed support")
			}
			for _, build := range []func() *types.AnswerSurfacePlan{func() *types.AnswerSurfacePlan { return types.BuildAnswerSurfacePlanForAgentContext(ctx) }, func() *types.AnswerSurfacePlan { return types.BuildAnswerSurfacePlanForBusContext(bus) }} {
				for i := 0; i < 2; i++ {
					plan := build()
					if plan.Diagram == nil || !plan.Diagram.Required || plan.Diagram.RequiredKind != kind || plan.DiagramHardRequirementDropped || plan.CompiledDiagramFence != "" {
						t.Fatalf("verified observe relations lost required diagram: %+v", plan.Diagram)
					}
					plan.Diagram.Required = false // returned clones must not corrupt either cache
				}
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if !strings.Contains(prompt, "## Diagram Contract") || strings.Contains(prompt, "## Diagram Preference") || !strings.Contains(prompt, "edge_anchor=") {
				t.Fatal("finalizer does not see required contract and exact relation recipes")
			}
			rows := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
			if len(rows) != 3 {
				t.Fatalf("want 3 exact observed matches, got %+v", rows)
			}
			doc := transactionSupportedDiagram(rows, kind)
			missing := *doc
			missing.Blocks = doc.Blocks[:1]
			raw, _ := json.Marshal(missing)
			res, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
			if err != nil || res.Success {
				t.Fatalf("required transaction diagram silently deleted: %v %+v", err, res)
			}
			raw, _ = json.Marshal(doc)
			res, err = (&tool.EmitAnswerDocument{}).Execute(bus, raw)
			if err != nil || !res.Success {
				t.Fatalf("grounded observe graph rejected: %v %+v", err, res)
			}
			accepted := ctx.Mutable.AnswerDocumentV2()
			if mismatches := tool.DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(bus, accepted, types.BuildAnswerSemanticViewForBusContext(bus), nil); len(mismatches) != 0 {
				t.Fatalf("post validator disagrees: %+v", mismatches)
			}
			if !strings.Contains(accepted.Blocks[1].Diagram.Body, "窗外关联背景") {
				t.Fatal("out-of-window peers lost their visible boundary")
			}
		})
	}
}

func TestTransactionRequiredDiagramUnsupportedMatrix(t *testing.T) {
	for _, tc := range []struct {
		name       string
		start, end float64
		kind       types.DiagramKind
		mutate     func(*types.ToolResult, *types.AgentContext)
	}{
		{"empty", 2, 2.1, types.DiagramFlow, nil},
		{"missing_consumption", 1.009, 1.011, types.DiagramFlow, nil},
		{"ambiguous_and_missing_submission", 1.024, 1.032, types.DiagramFlow, nil},
		{"identity_unverified", 1.034, 1.036, types.DiagramFlow, nil},
		{"incompatible_kind", 1, 1.05, types.DiagramArchitecture, nil},
		{"not_requested", 1, 1.05, types.DiagramFlow, func(_ *types.ToolResult, ctx *types.AgentContext) {
			ctx.AnalysisIR.AnswerContract.Diagram.Required = false
		}},
		{"source_question_incidental_trace", 1, 1.05, types.DiagramCallDAG, func(_ *types.ToolResult, ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = nil
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeNotApplicable}
			ctx.AnalysisIR.RequestModel.PerfTrace = &types.PerfBundle{}
		}},
		{"no_runtime_request_carrier", 1, 1.05, types.DiagramCallDAG, func(_ *types.ToolResult, ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = nil
			ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = nil
		}},
		{"precise_mixed_source_obligation", 1, 1.05, types.DiagramCallDAG, func(_ *types.ToolResult, ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true, SourceQuotes: []string{"internal/tracequery/parse.go"}, Modes: []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationTraceCurrentFlow}}
		}},
		{"no_observations", 1, 1.05, types.DiagramFlow, func(r *types.ToolResult, _ *types.AgentContext) { r.Observations = nil }},
		{"wrong_window", 1, 1.05, types.DiagramFlow, func(_ *types.ToolResult, ctx *types.AgentContext) {
			s, e := 4.0, 4.1
			ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart, ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &s, &e
		}},
		{"wrong_target", 1, 1.05, types.DiagramFlow, func(_ *types.ToolResult, ctx *types.AgentContext) {
			ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 888, Source: "user_explicit"}}
		}},
		{"model_producer", 1, 1.05, types.DiagramFlow, func(r *types.ToolResult, _ *types.AgentContext) {
			for i := range r.Observations {
				r.Observations[i].Producer = "model"
			}
		}},
		{"conflicting_payload", 1, 1.05, types.DiagramFlow, func(r *types.ToolResult, _ *types.AgentContext) {
			for _, rec := range r.Observations {
				if p, ok := tool.DecodeTraceTransactionHandoffs(rec); ok {
					rec.ID += ":conflict"
					p.Caveats = append(p.Caveats, "different retained payload")
					data, _ := json.Marshal(p)
					rec.RichNotes = []string{types.TraceNoteKeyTransactionHandoffs + "=" + string(data)}
					r.Observations = append(r.Observations, rec)
				}
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := queryTransactionDiagramSupport(t, tc.start, tc.end)
			ctx := transactionRequiredDiagramContext(t, tc.kind, tc.start, tc.end, nil)
			if tc.mutate != nil {
				tc.mutate(&r, ctx)
			}
			ctx.Mutable.AppendDispatchToolResult(r)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
			bus := types.ToolBusContext(ctx, types.AgentFinalizer)
			for _, plan := range []*types.AnswerSurfacePlan{types.BuildAnswerSurfacePlanForAgentContext(ctx), types.BuildAnswerSurfacePlanForBusContext(bus)} {
				if plan.Diagram != nil && plan.Diagram.Required {
					t.Fatalf("unsupported relation required a diagram: %+v", plan.Diagram)
				}
			}
		})
	}
}

func TestTransactionRequiredDiagramSoftMixedAndBusRevision(t *testing.T) {
	ctx := transactionRequiredDiagramContext(t, types.DiagramFlow, 1, 1.05, nil)
	ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true, SourceQuotes: []string{"解释交接机制"}, Modes: []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism}}
	ctx.TurnRouteHint = types.TurnRouteHint{Source: "mixed", NeedsRepoAccess: true, RequiredOutcomes: types.TurnOutcomeSourceExplanation | types.TurnOutcomeExternalArtifact}
	if precision := types.RuntimeSourceRequestCurrentSourceRequirementPrecisionForContract(&ctx.AnalysisIR.RequestModel, ctx.TurnRouteHint, &ctx.AnalysisIR.AnswerContract); precision != types.RuntimeSourceRequirementSoft {
		t.Fatalf("fixture must retain soft, not precise, source advice: %s", precision)
	}
	bus := types.ToolBusContext(ctx, types.AgentFinalizer)
	before := types.BuildAnswerSurfacePlanForBusContext(bus)
	if before.Diagram != nil && before.Diagram.Required {
		t.Fatal("empty runtime request fabricated support")
	}
	r := queryTransactionDiagramSupport(t, 1, 1.05)
	ctx.Mutable.AppendDispatchToolResult(r)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	for _, plan := range []*types.AnswerSurfacePlan{types.BuildAnswerSurfacePlanForAgentContext(ctx), types.BuildAnswerSurfacePlanForBusContext(bus)} {
		if plan.Diagram == nil || !plan.Diagram.Required || plan.DiagramHardRequirementDropped {
			t.Fatal("soft mixed source advice or stale bus cache hid real runtime support")
		}
	}
}

func TestTransactionRequiredDiagramPublicRepair(t *testing.T) {
	r := queryTransactionDiagramSupport(t, 1, 1.05)
	for _, remove := range []bool{true, false} {
		t.Run(fmt.Sprintf("remove=%t", remove), func(t *testing.T) {
			ctx := transactionRequiredDiagramContext(t, types.DiagramFlow, 1, 1.05, []types.ToolResult{r})
			bus := types.ToolBusContext(ctx, types.AgentFinalizer)
			rows := tool.RuntimeDiagramRelations(answerDocObservationLedger(ctx), &ctx.AnalysisIR.RequestModel)
			doc := transactionSupportedDiagram(rows[:1], types.DiagramFlow)
			doc.Blocks[1].EdgeAnchors = nil
			raw, _ := json.Marshal(doc)
			res, err := (&tool.EmitAnswerDocument{}).Execute(bus, raw)
			if err != nil || res.Success || !installAnswerDocDiagramRelationRepairLease(ctx, ctx.Mutable, &res, false) {
				t.Fatalf("expected real missing-anchor repair lease: %v %+v", err, res)
			}
			if remove {
				res, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{"unchanged_block_ids":["summary"],"remove_block_ids":["handoffs"]}`))
				if err != nil || res.Success {
					t.Fatalf("repair deleted user-required supported diagram: %v %+v", err, res)
				}
				return
			}
			lease := ctx.Mutable.AnswerDiagramRelationRepairLease()
			var edits []map[string]any
			for _, failure := range lease.Failures {
				for _, candidate := range lease.AllowedAdditions {
					if types.AnswerDiagramRelationRepairFailureCanAttachCandidate(failure, candidate) {
						edits = append(edits, map[string]any{"action": "attach", "failure_ref": failure.FailureRef, "addition_ref": candidate.AdditionRef, "edge": map[string]any{"from_node": failure.FromNode, "to_node": failure.ToNode, "visible_label": "协议记录对应"}})
					}
				}
			}
			if len(edits) != 1 {
				t.Fatalf("repair did not publish one exact observed relation: %+v", lease)
			}
			raw, _ = json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "diagram_edge_edits": edits})
			res, err = (&tool.EmitAnswerDocumentPatch{}).Execute(bus, raw)
			if err != nil || !res.Success {
				t.Fatalf("exact protocol-anchor repair rejected: %v %+v", err, res)
			}
			got := ctx.Mutable.AnswerDocumentV2()
			if got == nil || len(got.Blocks) != 2 || got.Blocks[0].Text != doc.Blocks[0].Text {
				t.Fatal("repair dropped diagram or rewrote sibling prose")
			}
			if failures := tool.DiagramCallEdgeEvidenceMismatchesWithRuntimeContext(bus, got, types.BuildAnswerSemanticViewForBusContext(bus), nil); len(failures) != 0 {
				t.Fatalf("repaired graph failed post-validation: %+v", failures)
			}
		})
	}
}

package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc298FrameFixture(t *testing.T, currentFrame bool) (*types.BusContext, func(string, float64, float64) types.ToolResult) {
	t.Helper()
	bus, query := hmc296CausalGuidanceFixture(t)
	path := filepath.Join(bus.RepoRoot, "chain.ftrace")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	trace := string(raw)
	if currentFrame {
		trace = "app-100 (100) [001] .... 2.000000: print: B|100|Choreographer#doFrame frame=77\n" + trace
		lines := strings.Split(trace, "\n")
		for i, line := range lines {
			if strings.Contains(line, "2.020000: sched_wakeup") {
				lines[i] = "app-100 (100) [001] .... 2.019500: print: E|100\n" + line
			}
		}
		trace = strings.Join(lines, "\n")
	}
	trace += "\napp-100 (100) [001] .... 9.000000: print: B|100|Choreographer#doFrame frame=88\napp-100 (100) [001] .... 9.010000: print: E|100\nrender-200 (200) [002] .... 9.011000: print: B|200|H:RenderFrame frame=88\nrender-200 (200) [002] .... 9.020000: print: E|200\ngpu-300 (300) [003] .... 9.021000: print: B|300|GPU completion frame=88\ngpu-300 (300) [003] .... 9.030000: print: E|300\n"
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	bus.AttachedHitrace = trace
	bus.AnalysisIR.RequestModel.RuntimeQuestionProfile.FrameCausalityRequested = true
	bus.Mutable.SetRequestModel(bus.AnalysisIR.RequestModel)
	return bus, query
}

func TestHMC298FrameScopeActualFinalizerAndPersistence(t *testing.T) {
	for _, name := range []string{"selected_only", "historical_empty", "historical_edges", "current_absent_with_unrelated_present"} {
		t.Run(name, func(t *testing.T) {
			currentFrame := name != "current_absent_with_unrelated_present"
			bus, query := hmc298FrameFixture(t, currentFrame)
			var results []types.ToolResult
			if name == "historical_empty" {
				probe := query("frame_root_cause_bundle", 8, 8.020)
				if probe.TraceEvidenceAuthority.FrameEvidenceStatus != "absent" {
					t.Fatalf("fixture empty probe: %+v", probe.TraceEvidenceAuthority)
				}
				results = append(results, probe)
			}
			if name == "historical_edges" || !currentFrame {
				probe := query("frame_flow", 9, 9.040)
				if probe.TraceEvidenceAuthority.FrameEvidenceStatus != "present" || probe.TraceEvidenceAuthority.FrameFlowEdgeCount < 2 {
					t.Fatalf("fixture historical edges: %+v", probe.TraceEvidenceAuthority)
				}
				results = append(results, probe)
			}
			selected := query("frame_root_cause_bundle", 2, 2.020)
			wantStatus := "present"
			if !currentFrame {
				wantStatus = "absent"
			}
			if selected.TraceEvidenceAuthority.FrameEvidenceStatus != wantStatus || currentFrame && selected.TraceEvidenceAuthority.FrameItemCount != 1 {
				t.Fatalf("fixture current authority: %+v", selected.TraceEvidenceAuthority)
			}
			results = append(results, selected)
			for i, result := range results {
				a := result.TraceEvidenceAuthority
				t.Logf("producer[%d]: view=%s frame=%s frame_items=%d flow_edges=%d typed_causal_rows=%d causal=%s", i, a.View, a.FrameEvidenceStatus, a.FrameItemCount, a.FrameFlowEdgeCount, a.TypedCausalRowCount, a.CausalConclusion)
			}
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
			view := answerDocRuntimeTraceGuidanceView(ctx)
			if view.FrameEvidenceStatus != wantStatus {
				t.Errorf("current frame status=%q want %q", view.FrameEvidenceStatus, wantStatus)
			}
			selectedScopes := 0
			for _, scope := range view.FrameScopes {
				if len(scope.SelectedProjections) == 0 {
					continue
				}
				selectedScopes++
				if scope.Source.QueryWindowStartTs != 2 || scope.Source.QueryWindowEndTs != 2.020 || scope.Authority.FrameFlowEdgeCount != selected.TraceEvidenceAuthority.FrameFlowEdgeCount || scope.Authority.FrameFlowRelationAuthority != selected.TraceEvidenceAuthority.FrameFlowRelationAuthority {
					t.Errorf("historical census/relation borrowed: %+v", scope)
				}
			}
			if selectedScopes != 1 {
				t.Fatalf("exact current result scope count=%d", selectedScopes)
			}
			before := hmcCausalHandoffSnapshot(t, results)
			contract := types.BuildAnswerSemanticViewForAgentContext(ctx).TraceCausalClaimContract
			if contract == nil || !contract.Active() {
				t.Fatal("real producer did not establish a current projection contract")
			}
			reg := tool.NewRegistry()
			reg.Register(&tool.EmitAnswerDocument{})
			reg.Register(&tool.EmitAnswerDocumentPatch{})
			capture := &traceTeachingCaptureLLM{stop: errors.New("captured HMC298 frame scope")}
			_, err := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}).Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
			if !errors.Is(err, capture.stop) || capture.calls != 1 {
				t.Fatalf("actual capture: %v", err)
			}
			var messages strings.Builder
			for _, message := range capture.messages {
				if message.Role == "system" || message.Role == "user" {
					messages.WriteString(message.Content)
					messages.WriteByte('\n')
				}
			}
			prompt := messages.String()
			if currentFrame {
				for _, bad := range []string{"the selected evidence produced no frame or deadline observation bound to the target", "No target-bound frame boundary, completion, or deadline is provided", "no target-bound frame boundary, completion, or deadline is provided"} {
					if strings.Contains(prompt, bad) {
						t.Errorf("historical result overwrote current present frame: %q", bad)
					}
				}
			} else if !strings.Contains(prompt, "the selected evidence produced no frame or deadline observation bound to the target") {
				t.Error("unrelated present erased actual current absence")
			}
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model_summary", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "Keep this bounded model explanation."}}}
			out, err := tool.ApplyAndPersistMutation(bus, "hmc298-public", types.NewReplaceAllMutation(doc), nil, time.Now())
			if err != nil || !out.Success {
				t.Fatalf("public persistence: %v %+v", err, out)
			}
			wire, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
			if name != "selected_only" {
				foundCoverage := false
				for _, block := range bus.Mutable.AnswerDocumentV2().Blocks {
					if strings.HasSuffix(block.ID, "_coverage") && strings.TrimSpace(block.Text) != "" {
						foundCoverage = true
					}
				}
				if !foundCoverage {
					t.Fatal("public persistence produced no nonempty coverage block")
				}
				if !strings.Contains(string(wire), "query_window=") {
					t.Error("persisted frame boundary lacks original query range")
				}
				if !strings.Contains(string(wire), "query-local") && !strings.Contains(string(wire), "局部边界") {
					t.Error("persisted historical frame boundary was not explicitly query-local")
				}
			}
			if !currentFrame && !strings.Contains(string(wire), "no target-bound frame or deadline evidence was found") && !strings.Contains(string(wire), "未找到可绑定到目标的帧或截止期证据") {
				t.Error("persisted current absence erased by historical present")
			}
			if currentFrame && (name == "historical_empty" || name == "historical_edges") && (strings.Contains(string(wire), "唤醒/阻塞链只支持所选窗口内") || strings.Contains(string(wire), "chains support only selected-window")) {
				t.Error("historical absence lowered current frame/chain claim in persisted coverage")
			}
			if after := hmcCausalHandoffSnapshot(t, results); before != after {
				t.Fatal("consumer modified original producer records")
			}
			t.Logf("current=%s items=%d edges=%d ceiling=%s", wantStatus, selected.TraceEvidenceAuthority.FrameItemCount, selected.TraceEvidenceAuthority.FrameFlowEdgeCount, contract.Ceiling)
		})
	}
}

func TestHMC298NonCausalFrameMeasurementsRemainQueryLocal(t *testing.T) {
	bus, query := hmc298FrameFixture(t, true)
	result := query("frame_timeline", 2, 2.020)
	bus.AnalysisIR.RequestModel.RuntimeQuestionProfile.FrameCausalityRequested = false
	bus.Mutable.SetRequestModel(bus.AnalysisIR.RequestModel)
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	view := answerDocRuntimeTraceGuidanceView(ctx)
	if view.FrameEvidenceStatus != "" || len(view.FrameScopes) != 0 || view.FrameFlowUnproven {
		t.Fatalf("nonframe task received selected frame authority: %+v", view)
	}
	if len(view.FrameMeasurements) != 1 || view.FrameMeasurements[0].ItemCount != 1 || !strings.Contains(view.FrameMeasurements[0].Scope, "query-local") || !strings.Contains(view.FrameMeasurements[0].Scope, "2.000000000..2.020000000") {
		t.Fatalf("legitimate local measurement lost: %+v", view.FrameMeasurements)
	}
}

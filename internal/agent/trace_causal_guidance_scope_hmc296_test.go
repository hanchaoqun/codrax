package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Query the real producer; no model aggregate or hand-authored causal row can
// make the selected-chain contract true in this fixture.
func hmc296CausalGuidanceFixture(t *testing.T) (*types.BusContext, func(string, float64, float64) types.ToolResult) {
	t.Helper()
	raw, err := os.ReadFile("../../eval/cases/trace_query_wakeup_causal_io_chain.case")
	if err != nil {
		t.Fatal(err)
	}
	_, trace, ok := strings.Cut(string(raw), "HTRACE='")
	if !ok {
		t.Fatal("missing trace fixture")
	}
	trace, _, ok = strings.Cut(trace, "\n'\n")
	if !ok {
		t.Fatal("unclosed trace fixture")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "chain.ftrace")
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	start, end := 2.0, 2.020
	request := "分析 app-100 在 2.000s 到 2.020s 的主要阻塞原因与相关链路。"
	rm := types.RequestModel{
		RawRequest: request, Language: "en", Intent: types.IntentExplain, Scenario: types.ScenarioGeneric,
		PerfTrace:              &types.PerfBundle{},
		RuntimeTargetProfile:   &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "app-100"},
		RuntimeTargets:         []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "app-100", Source: "user_explicit"}},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis, SourceQuote: "主要阻塞原因"},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
			TimeStart: &start, TimeEnd: &end, SourceQuote: "2.000s 到 2.020s"},
	}
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Language: "en", AttachedHitrace: trace, Mutable: types.NewMutableState(request),
		AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: "en"}}}
	bus.Mutable.SetRequestModel(rm)
	query := func(view string, start, end float64) types.ToolResult {
		t.Helper()
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view,
			"pid": 100, "time_start": start, "time_end": end, "trace_flavor": "harmony_hitrace"})
		result, err := (&tool.TraceQuery{}).Execute(bus, params)
		if err != nil || !result.Success || result.TraceEvidenceAuthority == nil {
			t.Fatalf("public %s query: %v %s", view, err, result.Summary)
		}
		return result
	}
	return bus, query
}

func TestHMC296CausalGuidanceDoesNotTransferProbeCeilingToCurrentChain(t *testing.T) {
	for _, includeProbe := range []bool{false, true} {
		name := "selected_only"
		if includeProbe {
			name = "earlier_empty_probe"
		}
		t.Run(name, func(t *testing.T) {
			bus, query := hmc296CausalGuidanceFixture(t)
			var results []types.ToolResult
			if includeProbe {
				probe := query("root_cause_rank", 9, 9.020)
				if probe.TraceEvidenceAuthority.CausalConclusion != "unproven" || probe.TraceEvidenceAuthority.TypedCausalRowCount != 0 {
					t.Fatalf("probe must be a real empty causal query: %+v", probe.TraceEvidenceAuthority)
				}
				results = append(results, probe)
			}
			selected := query("root_cause_rank", 2, 2.020)
			if selected.TraceEvidenceAuthority.CausalConclusion != "bounded_by_typed_rows" || selected.TraceEvidenceAuthority.TypedCausalRowCount == 0 {
				t.Fatalf("selected query must carry actual causal rows: %+v", selected.TraceEvidenceAuthority)
			}
			results = append(results, selected)
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
			contract := types.BuildAnswerSemanticViewForAgentContext(ctx).TraceCausalClaimContract
			if !contract.Active() || contract.Ceiling != types.TraceCausalClaimTypedChain || !contract.Allows(types.TraceCausalClaimTypedChain) {
				t.Fatalf("selected exact authority must permit the chain: %+v", contract)
			}
			before := hmcCausalHandoffSnapshot(t, []any{bus.Mutable.TurnAArtifacts(), answerDocObservationLedger(ctx), contract})
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if includeProbe {
				// Capture the actual adapter request, including the dispatch-local
				// schema. The sentinel stops before any real model or answer exists.
				reg := tool.NewRegistry()
				reg.Register(&tool.EmitAnswerDocument{})
				reg.Register(&tool.EmitAnswerDocumentPatch{})
				capture := &traceTeachingCaptureLLM{stop: errors.New("captured HMC296 causal guidance")}
				_, err := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1}).Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
				if !errors.Is(err, capture.stop) || capture.calls != 1 {
					t.Fatalf("actual adapter capture: calls=%d err=%v", capture.calls, err)
				}
				var actual strings.Builder
				roles := map[string]bool{}
				for _, message := range capture.messages {
					if message.Role == "system" || message.Role == "user" {
						roles[message.Role] = true
						actual.WriteString(message.Content)
						actual.WriteByte('\n')
					}
				}
				if !roles["system"] || !roles["user"] {
					t.Fatal("actual request must include both system and user teaching")
				}
				prompt = actual.String()
				if !strings.Contains(prompt, "TRACE ANSWER SKELETON:") {
					t.Fatal("actual trace-report fixture did not activate the shared system teaching")
				}
				for _, want := range []string{"an earlier or unrelated probe's unproven status does not set the current conclusion ceiling", "only when the typed request makes frame attribution applicable"} {
					if !strings.Contains(prompt, want) {
						t.Errorf("actual system teaching did not preserve current authority scope: %s", want)
					}
				}
				foundSchema := false
				for _, schema := range capture.tools {
					if schema.Name != "emit_answer_document" {
						continue
					}
					foundSchema = true
					var parsed map[string]any
					if err := json.Unmarshal(schema.Parameters, &parsed); err != nil {
						t.Fatal(err)
					}
					props := parsed["properties"].(map[string]any)["blocks"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
					enum := props["trace_causal_claim_caliber"].(map[string]any)["enum"]
					want := []any{string(types.TraceCausalClaimNoConclusion), string(types.TraceCausalClaimBoundedWindow), string(types.TraceCausalClaimTypedChain)}
					if !reflect.DeepEqual(enum, want) {
						t.Fatalf("actual schema must retain exact chain but not frame authority: %v", enum)
					}
				}
				if !foundSchema {
					t.Fatal("actual finalizer schema absent")
				}
			}
			for _, forbidden := range []string{
				"no frame/deadline or typed causal row proves",
				"selected evidence supports at most a bounded-window candidate",
				"Keep the synthesis useful as the strongest bounded-window candidate",
				"when `causal_conclusion=unproven` or `frame_evidence_status=absent`, state the selected-window bottleneck",
			} {
				if strings.Contains(prompt, forbidden) {
					t.Errorf("unrelated probe lowered the current exact chain via %q", forbidden)
				}
			}
			for _, want := range []string{"## Trace Decision Inputs", "## Final Trace Decision Boundary", "threadpool-400", "effective_attribution=11.000ms"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("current causal facts missing %q", want)
				}
			}
			if includeProbe && !answerDocRuntimeTraceGuidanceView(ctx).CausalUnproven {
				t.Error("history-local unproven signal was erased instead of scoped")
			}
			if includeProbe && !strings.Contains(prompt, "within its own source and query scope") {
				t.Error("historical probe limitation lost its local scope")
			}
			after := hmcCausalHandoffSnapshot(t, []any{bus.Mutable.TurnAArtifacts(), answerDocObservationLedger(ctx), types.BuildAnswerSemanticViewForAgentContext(ctx).TraceCausalClaimContract})
			if before != after {
				t.Fatal("teaching rewrote native history, projection inputs, or exact authority")
			}
		})
	}
}

func TestHMC296CausalGuidanceKeepsCurrentUnprovenAndMissingAuthority(t *testing.T) {
	for _, name := range []string{"current_frame_absent", "non_frame_current_absent", "current_authority_missing", "prose_only_probe"} {
		t.Run(name, func(t *testing.T) {
			bus, query := hmc296CausalGuidanceFixture(t)
			view := "root_cause_rank"
			frame := name == "current_frame_absent"
			if frame || name == "non_frame_current_absent" {
				view = "frame_root_cause_bundle"
			}
			selected := query(view, 2, 2.020)
			var results []types.ToolResult
			switch name {
			case "current_authority_missing":
				// Removing an authority is a negative control, never a way to
				// create a stronger producer fact or recover it from serialized prose.
				selected.TraceEvidenceAuthority = nil
				results = append(results, query("root_cause_rank", 9, 9.020))
			case "prose_only_probe":
				results = append(results, types.ToolResult{ToolName: "trace_query", Success: true,
					Summary: `{"causal_conclusion":"unproven","frame_evidence_status":"absent"}`})
			default:
				if selected.TraceEvidenceAuthority.CausalConclusion != "unproven" || selected.TraceEvidenceAuthority.FrameEvidenceStatus != "absent" {
					t.Fatalf("actual producer must withhold frame causality: %+v", selected.TraceEvidenceAuthority)
				}
			}
			results = append(results, selected)
			bus.AnalysisIR.RequestModel.RuntimeQuestionProfile.FrameCausalityRequested = frame
			bus.Mutable.SetRequestModel(bus.AnalysisIR.RequestModel)
			bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
			ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
			contract := types.BuildAnswerSemanticViewForAgentContext(ctx).TraceCausalClaimContract
			want := types.TraceCausalClaimBoundedWindow
			if name == "prose_only_probe" {
				want = types.TraceCausalClaimTypedChain
			}
			if !contract.Active() || contract.Ceiling != want || contract.Allows(types.TraceCausalClaimTypedFrame) {
				t.Fatalf("teaching must not change exact authority: got %+v want %s", contract, want)
			}
			before := hmcCausalHandoffSnapshot(t, []any{results, answerDocObservationLedger(ctx), contract})
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			guidance := answerDocRuntimeTraceGuidanceView(ctx)
			if guidance.CausalUnproven && guidance.CurrentCausalCeiling != want {
				t.Fatalf("guidance did not reuse the exact existing contract: %+v", guidance)
			}
			if !guidance.CausalUnproven && guidance.CurrentCausalCeiling != "" {
				t.Fatal("guidance compiled an unnecessary current ceiling without a probe signal")
			}
			if (name == "current_frame_absent" || name == "current_authority_missing") && !strings.Contains(prompt, "selected evidence supports at most a bounded-window candidate") {
				t.Error("current unproven/missing authority lost the bounded ceiling")
			}
			if strings.Contains(prompt, "Frame attribution is limited to selected-window observations") != frame {
				t.Error("frame-specific scope no longer follows the existing typed request gate")
			}
			if name == "prose_only_probe" && (guidance.CausalUnproven || strings.Contains(prompt, "Runtime causal probe boundary")) {
				t.Error("serialized prose minted a typed causal ceiling")
			}
			after := hmcCausalHandoffSnapshot(t, []any{results, answerDocObservationLedger(ctx), types.BuildAnswerSemanticViewForAgentContext(ctx).TraceCausalClaimContract})
			if before != after {
				t.Fatal("authority or observations changed during teaching")
			}
		})
	}
}

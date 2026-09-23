package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Capture the production agent's first adapter request, not an isolated
// renderer. The sentinel ends the local run before any tool/model side effect.
type traceTeachingCaptureLLM struct {
	messages []llm.Message
	tools    []llm.ToolSchema
	calls    int
	stop     error
}

func (l *traceTeachingCaptureLLM) Chat(_ context.Context, messages []llm.Message, tools []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	l.calls++
	l.messages = append([]llm.Message(nil), messages...)
	l.tools = append([]llm.ToolSchema(nil), tools...)
	return llm.Response{}, l.stop
}

func (*traceTeachingCaptureLLM) ModelID() string               { return "trace-teaching-capture" }
func (*traceTeachingCaptureLLM) MaxContextTokens() int         { return 128000 }
func (*traceTeachingCaptureLLM) MaxOutputTokens() int          { return 4096 }
func (*traceTeachingCaptureLLM) RequestTimeout() time.Duration { return 0 }
func (*traceTeachingCaptureLLM) RetryMaxAttempts() int         { return 0 }

func TestTraceQueryTeachingActualExplorerMessages(t *testing.T) {
	for _, scope := range []types.RuntimeQuestionScope{"", types.RuntimeQuestionScopeCausalDiagnosis} {
		t.Run(string(scope), func(t *testing.T) {
			ctx := traceTeachingRuntimeContext(types.StageExplore)
			if scope != "" {
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: scope}
			}
			reg := toolpkg.NewRegistry()
			reg.Register(&toolpkg.TraceQuery{})
			capture := &traceTeachingCaptureLLM{stop: errors.New("captured initial model request")}
			agent := NewExplorerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1})
			_, err := agent.Execute(ctx, traceTeachingSkill(t, "explore-skill"))
			if !errors.Is(err, capture.stop) || capture.calls != 1 {
				t.Fatalf("did not capture exactly the initial model request: calls=%d err=%v", capture.calls, err)
			}

			surfaces := traceTeachingToolSurfaces(t)
			for _, message := range capture.messages {
				assertNoRetiredTraceRankingTeaching(t, message.Content)
				if message.Role == "system" {
					surfaces["model_system"] += message.Content
				}
				if message.Role == "user" && strings.Contains(message.Content, "Explicit Runtime Trace Path Start") {
					surfaces["model_dynamic_user"] += message.Content
				}
			}
			// Shared view selection is owned by the rendered system workflow;
			// the dynamic supplement points back to it instead of copying it.
			// Verify the actual adapter request, so neither omission nor a
			// second complete copy can pass by satisfying isolated renderers.
			matrix := skill.RenderTraceQueryViewMatrix()
			if strings.Count(surfaces["model_system"], matrix) != 1 || strings.Contains(surfaces["model_dynamic_user"], matrix) {
				t.Fatal("actual model request must carry exactly one complete view matrix, owned by the system workflow")
			}
			if !strings.Contains(surfaces["model_dynamic_user"], "Select the view from the full view matrix in the system Workflow's TRACE QUERY rule; it remains available on subsequent tool rounds.") {
				t.Fatal("dynamic supplement lost its explicit locator for the retained complete view matrix")
			}
			for _, name := range []string{"model_system", "model_dynamic_user"} {
				if strings.Contains(surfaces[name], "Root-cause participation uses this authoritative closed typed effective-impact matrix") {
					t.Errorf("%s duplicated the tool's long closed-matrix contract", name)
				}
			}
			foundTool := false
			for _, offered := range capture.tools {
				if !json.Valid(offered.Parameters) {
					t.Errorf("actual offered %s parameters are not valid JSON", offered.Name)
				}
				if offered.Name == "trace_query" {
					foundTool = true
					surfaces["model_tool_description"] = offered.Description
					surfaces["model_tool_parameters"] = string(offered.Parameters)
				}
			}
			if !foundTool {
				t.Fatal("actual model request did not expose trace_query")
			}
			for _, want := range []string{"fragment count, max/p95 segment", "follow the rendered `next_step`", "raw occupancy", "unpriced business/semantic work"} {
				if !strings.Contains(surfaces["model_dynamic_user"], want) {
					t.Errorf("explorer discarded legitimate measurement/navigation teaching %q", want)
				}
			}
			if !strings.Contains(surfaces["model_system"], "positive CAP/compute-supply deficit and otherwise remains context_only") {
				t.Error("self-running teaching lost its positive CAP-deficit participation boundary")
			}
			// The model consumes these two messages together. Preserve every
			// existing rank/frame-flow assertion on their complete instruction
			// contract, while independent tool surfaces keep their own copies.
			surfaces["model_instructions"] = surfaces["model_system"] + "\n" + surfaces["model_dynamic_user"]
			for name, surface := range surfaces {
				if name == "model_system" || name == "model_dynamic_user" {
					continue
				}
				t.Run(name, func(t *testing.T) {
					assertTraceTeachingRankContract(t, surface)
					if strings.Count(surface, skill.TraceFrameFlowEvidenceTeaching) != 1 {
						t.Error("shared frame-flow teaching must appear exactly once on each surface")
					}
					for _, want := range []string{
						"frame_flow", "UI/RS/GPU stage spans", "causal_conclusion=unproven", "explicit typed connector",
						"latency_ms is a clamped nonnegative gap", "overlap may report zero",
						"not measured communication latency", "Stage labels do not prove the owning thread's role",
					} {
						if !strings.Contains(surface, want) {
							t.Errorf("frame-flow capability/evidence ceiling missing %q", want)
						}
					}
				})
			}
		})
	}
}

func TestTraceQueryTeachingActualFinalizerMessages(t *testing.T) {
	ctx := traceTeachingRuntimeContext(types.StageFinalize)
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis}
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{
		Meta:         types.PerfMeta{Source: "hitrace"},
		Observations: []types.PerfObservation{{Kind: "root_cause_rank"}},
	})
	reg := toolpkg.NewRegistry()
	reg.Register(&toolpkg.EmitAnswerDocument{})
	capture := &traceTeachingCaptureLLM{stop: errors.New("captured initial finalizer request")}
	agent := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1})
	_, err := agent.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
	if !errors.Is(err, capture.stop) || capture.calls != 1 {
		t.Fatalf("did not capture exactly the initial finalizer request: calls=%d err=%v", capture.calls, err)
	}
	var dynamic string
	for _, message := range capture.messages {
		assertNoRetiredTraceRankingTeaching(t, message.Content)
		if message.Role == "user" && strings.Contains(message.Content, "Runtime trace handoff hint") {
			dynamic += message.Content
		}
	}
	if dynamic == "" {
		t.Fatal("fixture did not render runtime trace handoff into the actual finalizer request")
	}
	assertTraceTeachingRankContract(t, dynamic)
	for _, want := range []string{
		"Runtime root-cause layering hint", "preserve each layer's emitted tier and effective attribution",
		"separately labelled chain cumulative account",
		"Never rename a chain cumulative account as the dominant state's measured duration",
		"occurrence windows", "representative repeated windows", "fragment count and max/p95 segment",
		"any published next-step guidance", "prefer the bounded `trace_query` facts",
		"significant raw occupancy and unpriced business/semantic work",
	} {
		if !strings.Contains(dynamic, want) {
			t.Errorf("finalizer discarded legitimate measurement/handoff teaching %q", want)
		}
	}
	if strings.Contains(dynamic, "Root-cause participation uses this authoritative closed typed effective-impact matrix") {
		t.Fatal("finalizer duplicated the tool's long closed-matrix contract")
	}
}

func traceTeachingRuntimeContext(stage types.PipelineStage) *types.AgentContext {
	objective := "Explain the attached trace's selected frame delay and repeated dependency windows."
	return &types.AgentContext{
		Stage: stage, Objective: objective, Mutable: types.NewMutableState(objective),
		AttachedHitrace: "app-42 (42) [000] .... 1.000000: sched_switch: prev_comm=app prev_pid=42 prev_state=S ==> next_comm=idle next_pid=0\n",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:    types.IntentRootCause,
			PerfTrace: &types.PerfBundle{Frames: []types.PerfFrame{{FrameNo: 1, DurationMs: 33.3, Janky: true}}},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
				SourceQuotes:      []string{"attached trace only"}, Confidence: 1,
			},
		}},
	}
}

func traceTeachingSkill(t *testing.T, name string) *skill.Config {
	t.Helper()
	reg := skill.NewRegistry()
	skill.RegisterDefaults(reg)
	sk, err := reg.Get(name)
	if err != nil {
		t.Fatal(err)
	}
	return sk
}

func traceTeachingToolSurfaces(t *testing.T) map[string]string {
	t.Helper()
	query := &toolpkg.TraceQuery{}
	if !json.Valid(query.Parameters()) {
		t.Fatal("public trace_query parameters are not valid JSON")
	}
	return map[string]string{"public_description": query.Description(), "public_parameters": string(query.Parameters())}
}

func assertTraceTeachingRankContract(t *testing.T, surface string) {
	t.Helper()
	if strings.Count(surface, skill.TraceRootCauseRankOrderTeaching) != 1 {
		t.Error("shared short ranking contract must appear exactly once on each surface")
	}
	assertNoRetiredTraceRankingTeaching(t, surface)
	plain := strings.ReplaceAll(surface, "`", "")
	for _, want := range []string{
		"emitted chain_relevance, channel, and tier", "effective_impact_ms before score",
		"Only rank #1 is primary on the elected causal ladder", "an adjacent channel's rank=1 is not a primary cause",
		"later positive contenders are secondary/tertiary", "Raw occupancy does not authorize re-ranking",
		"context_only/background", "cumulative_impact_ms", "state_churn", "occurrence_windows",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("ranking/retained measurement teaching missing %q", want)
		}
	}
}

func assertNoRetiredTraceRankingTeaching(t *testing.T, surface string) {
	t.Helper()
	plain := strings.ReplaceAll(surface, "`", "")
	for _, banned := range []string{
		"co-primary", "same-chain cumulative_impact_ms",
		"compare same-chain rows by cumulative_impact_ms before score",
		"compare same-chain primary rows by their typed cumulative account before score",
		"keep all tier=primary layers", "report every tier=primary layer",
		"Treat typed on-chain rows as primary candidates",
	} {
		if strings.Contains(plain, banned) {
			t.Errorf("retired ranking instruction reached model/tool surface: %q", banned)
		}
	}
}

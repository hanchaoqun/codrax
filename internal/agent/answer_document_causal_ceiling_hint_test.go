package agent

// NW-05 软臂 (P3, 2026-07-24) — the composition prompt gains ONE causal-ceiling
// directive when a typed evidence authority published causal_conclusion=
// unproven. Precise trigger (typed authority field), soft effect (prompt hint
// only); absence of the authority keeps the prompt byte-identical.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func answerDocCausalCeilingTestContext(withUnproven bool) *types.AgentContext {
	mut := types.NewMutableState("分析鸿蒙 trace 丢帧")
	mut.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace"},
		Observations: []types.PerfObservation{{
			Kind:    "priority_semantics",
			Subject: "HarmonyOS priority semantics",
			Summary: "Harmony priority semantics: prio=120/ohos_rt observed in attached trace",
			Tags:    []string{"harmony_priority", "prio=120/ohos_rt"},
		}},
	})
	if withUnproven {
		mut.AppendDispatchToolResult(types.ToolResult{
			ToolName: "trace_query",
			Success:  true,
			TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
				View:                "frame_root_cause_bundle",
				FrameEvidenceStatus: "absent",
				CausalConclusion:    "unproven",
			},
		})
	}
	return &types.AgentContext{
		Objective:             "分析鸿蒙 trace 丢帧",
		AttachedHitraceSource: "harmony_hitrace",
		Mutable:               mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
			},
			AnswerContract: types.AnswerContract{},
		},
	}
}

func TestAnswerDocumentEvaluatorRendersCausalCeilingHintOnUnprovenAuthority(t *testing.T) {
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(answerDocCausalCeilingTestContext(true), nil)
	for _, want := range []string{
		"Runtime causal ceiling hint",
		"`causal_conclusion=unproven`",
		"bounded window facts and candidates",
		"`导致丢帧`/`caused the dropped frame`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluatorOmitsCausalCeilingHintWithoutUnprovenAuthority(t *testing.T) {
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(answerDocCausalCeilingTestContext(false), nil)
	if strings.Contains(prompt, "Runtime causal ceiling hint") {
		t.Fatalf("hint must stay absent without the typed unproven authority:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluatorRendersTemporalFrameEdgeAuthorityHint(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(false)
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View:                       "frame_flow",
			FrameFlowEdgeCount:         3,
			FrameFlowRelationAuthority: "temporal_sequence",
			FrameFlowCausalConclusion:  "unproven",
			CausalConclusion:           "unproven",
		},
	})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"Runtime frame-edge authority hint",
		"`frame_flow_causality=unproven`",
		"`relation=temporal_sequence`",
		"`edges=3`",
		"relation_kind=temporal",
		"temporal adjacency (unproven)",
		"unless a separate typed causal row proves that exact relation",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("frame-edge prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestTypedTraceAuthoritySelectsCompactExactGuidance(t *testing.T) {
	ctx := answerDocCausalCeilingTestContext(true)
	got := renderAnswerDocRuntimeTraceAnswerGuidance(ctx)
	for _, want := range []string{
		"Typed trace context precedence",
		"on-chain/adjacent/background population",
		"Thread and span semantic authority",
		"span/marker label proves its label, measured interval",
		"does not by itself prove the internal work",
		"not the owning thread",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compact typed trace guidance missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"Runtime Binder direction hint",
		"Runtime IO/supply hint",
		"Runtime perf support hint",
		"Runtime direct-blocking hint",
		"Runtime root-cause layering hint",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("typed trace guidance retained unrelated generic recipe %q:\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, "Runtime causal ceiling hint") {
		t.Fatalf("context dedupe removed exact causal ceiling:\n%s", got)
	}
}

func TestRequestedDimensionTokensSupportHanRuns(t *testing.T) {
	// NG-4 (§13.4): 中文维度标签此前 token 化为空,只有 ASCII 名维度能上
	// 指标摘录面。Han 连续段现自成 token,两侧仍精确等值匹配。
	tokens := requestedDimensionIdentifierTokens("丢帧阶段 CPU调度分析 vsync")
	want := map[string]bool{"丢帧阶段": false, "cpu": false, "调度分析": false, "vsync": false}
	for _, token := range tokens {
		if _, ok := want[token]; ok {
			want[token] = true
		}
	}
	for token, seen := range want {
		if !seen {
			t.Fatalf("token %q missing from %v", token, tokens)
		}
	}
}

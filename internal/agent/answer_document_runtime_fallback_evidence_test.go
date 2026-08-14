package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeFallbackEvidenceContext() *types.AgentContext {
	mut := types.NewMutableState("trace-only fallback")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		RawRef:   "[trace_query params: view=event_search source=path path=/tmp/donghu.systrace origin=runtime_artifact artifact_kind=trace]",
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{
			View: "event_search",
		},
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:event_search:doFrame",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{
				Kind:         types.ObservationSourceRuntimeArtifact,
				Path:         "/tmp/donghu.systrace",
				ArtifactID:   "donghu-trace",
				ArtifactKind: "trace",
			},
			Span: types.ObservationSpan{
				StartTs: 32136.468701,
				EndTs:   32136.468702,
			},
			Subject:   "Choreographer#doFrame 8002384",
			Predicate: "trace_mark_found",
			Object:    "B|21690|Choreographer#doFrame 8002384",
			ClaimKey:  "trace_mark_found:Choreographer#doFrame 8002384",
			Value:     "32136.468701s",
			Summary:   "exact trace marker found in the attached artifact",
		}, {
			ID:              "trace_query:root_cause_rank:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{
				Kind:         types.ObservationSourceRuntimeArtifact,
				Path:         "/tmp/donghu.systrace",
				ArtifactID:   "donghu-trace",
				ArtifactKind: "trace",
			},
			Span:      types.ObservationSpan{LineStart: 40, LineEnd: 44},
			Subject:   "RenderThread-1",
			Predicate: "root_cause_primary",
			Object:    "runnable",
			ClaimKey:  "root_cause_primary",
			Value:     "5.000",
			Unit:      "ms",
			RichNotes: []string{
				"rank=1",
				"tier=primary",
				"impact_ms=5.000",
				"cumulative_impact_ms=5.000",
				"effective_impact_ms=5.000",
				"chain_relevance=on_chain",
				"causality=on_wakeup_chain",
				"chain_depth=1",
				"dominant_state=runnable",
			},
			Confidence: 0.9,
		}},
	}}})
	mut.SetPerfTrace(&types.PerfBundle{
		Meta: types.PerfMeta{Source: "hitrace"},
		Observations: []types.PerfObservation{{
			Kind:    "time_semantics",
			Summary: "trace clock is boottime",
		}},
	})
	return &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		Language:  "zh",
		Objective: "只分析 trace 中 Choreographer#doFrame 8002384",
		Mutable:   mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Scenario: types.ScenarioPerformanceBottleneck,
			Intent:   types.IntentRootCause,
		}},
	}
}

func TestIsolatedFinalizerProseFallbackPromptCarriesRuntimeLedgerFacts(t *testing.T) {
	prompt := isolatedFinalizerProseFallbackPrompt(runtimeFallbackEvidenceContext(), "zh")
	for _, want := range []string{
		"## 已验证事实",
		"origin=runtime_artifact",
		"producer=trace_query",
		"donghu.systrace",
		"32136.468701",
		"Choreographer#doFrame 8002384",
		"## Trace 解释边界",
		"数字片段是不可拆解的选择器标识，不是时长",
		"只有 B/S 起点而没有可配对 E/F 终点",
		"不同的宽窗、邻近行和 background 行只能支持补充排查",
		"priority_inversion_candidate` 本身也不证明持锁/资源所有权",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("runtime fallback prompt lost %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "当前没有可列出的代码证据") {
		t.Fatalf("runtime fallback prompt must not claim code-evidence absence when typed trace facts exist:\n%s", prompt)
	}
}

func TestAnswerDocumentEmptyModelFallbackCarriesRuntimeFactsWithoutConclusion(t *testing.T) {
	out, err := (&answerDocumentEvaluator{language: "zh"}).ParseOutput(
		runtimeFallbackEvidenceContext(), []llm.Message{{Role: "assistant"}}, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	for _, want := range []string{
		"已收集到的可验证事实",
		"origin=runtime_artifact",
		"Choreographer#doFrame 8002384",
		"系统只展示已落地证据，不补写结论",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("empty runtime fallback lost %q:\n%s", want, out.FinalAnswer)
		}
	}
}

func TestAnswerDocumentRawModelFallbackPreservesProseAndAppendsTypedRuntimeFacts(t *testing.T) {
	const prose = "模型原始判断：当前证据仍不足以确认完整调用链。"
	out, err := (&answerDocumentEvaluator{language: "zh"}).ParseOutput(
		runtimeFallbackEvidenceContext(), []llm.Message{{Role: "assistant", Content: prose}}, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	for _, want := range []string{
		prose,
		"系统确定性证据附录（不替代模型结论）",
		"Trace 因果投影",
		"Trace 关键事实",
		"origin=runtime_artifact",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("raw runtime fallback lost %q:\n%s", want, out.FinalAnswer)
		}
	}
	if strings.Count(out.FinalAnswer, prose) != 1 {
		t.Fatalf("model prose must be preserved exactly once:\n%s", out.FinalAnswer)
	}
	if strings.Index(out.FinalAnswer, prose) > strings.Index(out.FinalAnswer, "系统确定性证据附录") {
		t.Fatalf("model prose must remain before the system appendix:\n%s", out.FinalAnswer)
	}
	if strings.Contains(out.FinalAnswer, answerDocumentRawFallbackAnchorBlockID) {
		t.Fatalf("temporary materializer anchor leaked to the user surface:\n%s", out.FinalAnswer)
	}
}

func TestAnswerDocumentRuntimeFallbackEvidenceEnglishParity(t *testing.T) {
	ctx := runtimeFallbackEvidenceContext()
	ctx.Language = "en"
	out, err := (&answerDocumentEvaluator{language: "en"}).ParseOutput(
		ctx, []llm.Message{{Role: "assistant", Content: "The model-authored degraded answer."}}, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	for _, want := range []string{
		"Deterministic system evidence appendix (does not replace the model conclusion)",
		"trace clock is boottime",
		"origin=runtime_artifact",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("English runtime fallback lost %q:\n%s", want, out.FinalAnswer)
		}
	}
}

func TestAnswerDocumentEmptyRuntimeFallbackPrefersDeterministicSectionsOverShortFactList(t *testing.T) {
	out, err := (&answerDocumentEvaluator{language: "zh"}).ParseOutput(
		runtimeFallbackEvidenceContext(), []llm.Message{{Role: "assistant"}}, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	for _, want := range []string{
		"系统确定性证据附录（不替代模型结论）",
		"Trace 因果投影",
		"Trace 关键事实",
		"Choreographer#doFrame 8002384",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("empty runtime fallback lost deterministic section %q:\n%s", want, out.FinalAnswer)
		}
	}
	if strings.Count(out.FinalAnswer, "Choreographer#doFrame 8002384") != 1 {
		t.Fatalf("accepted fact must remain visible exactly once beside the deterministic appendix:\n%s", out.FinalAnswer)
	}
}

func TestAnswerDocumentRawFallbackWithoutRuntimeFactsKeepsCompactFactLane(t *testing.T) {
	ctx := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		Language:  "zh",
		Mutable:   types.NewMutableState("ordinary fallback"),
	}
	const prose = "模型原始正文。"
	out, err := (&answerDocumentEvaluator{language: "zh"}).ParseOutput(
		ctx, []llm.Message{{Role: "assistant", Content: prose}}, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if !strings.Contains(out.FinalAnswer, prose) {
		t.Fatalf("ordinary fallback lost model prose:\n%s", out.FinalAnswer)
	}
	if strings.Contains(out.FinalAnswer, "系统确定性证据附录") {
		t.Fatalf("non-runtime fallback must not invent a deterministic runtime appendix:\n%s", out.FinalAnswer)
	}
}

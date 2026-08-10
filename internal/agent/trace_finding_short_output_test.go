package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderTraceFindingShortRootCauseIsStructuredJSON(t *testing.T) {
	finding := &types.TraceFindingV1{PrimaryCause: &types.TraceCauseDecision{
		Token:       types.TraceCausalTokenSnapshot{Token: "scheduler_latency", Lane: "scheduling_demand"},
		SubjectName: "RenderThread",
	}}
	got := renderTraceFindingShortRootCauseValue(finding, "zh-CN")
	if !strings.Contains(got, "## 简短根因") {
		t.Fatalf("missing short root-cause heading:\n%s", got)
	}
	jsonText := strings.TrimSuffix(strings.TrimPrefix(strings.SplitN(got, "```json\n", 2)[1], ""), "\n```")
	var payload traceShortRootCauseJSON
	if err := json.Unmarshal([]byte(jsonText), &payload); err != nil {
		t.Fatalf("short root cause is not valid JSON: %v\n%s", err, got)
	}
	if payload.ThreadName != "RenderThread" || payload.RootCause != "RenderThread线程CPU调度延迟" {
		t.Fatalf("unexpected JSON payload: %+v", payload)
	}
}

func TestTraceShortRootCauseUsesClosedVocabularyFormats(t *testing.T) {
	tests := []struct {
		name, token, lane, want string
	}{
		{"io", "io_wait", "io_blocking", "Worker线程IO阻塞"},
		{"lock", "blocking_span", "lock_contention", "ClassLinker classes lock锁竞争"},
		{"binder", "binder_wait", "wakeup_chain", "Worker线程同步binder"},
		{"priority", "priority_inversion_candidate", "scheduling_demand", "Worker线程优先级反转"},
		{"gc", "gc_pause", "cpu_work", "GC耗时长"},
		{"scheduler", "scheduler_latency", "scheduling_demand", "Worker线程CPU调度延迟"},
		{"phase", "running", "cpu_work", "DrawFrame阶段高负载"},
		{"jit", "jit_compile", "cpu_work", "Worker线程JIT编译耗时"},
		{"shader", "shader_compile", "cpu_work", "Worker线程Shader编译"},
		{"sleep", "sleep_wait", "wakeup_chain", "Worker线程阻塞"},
		{"supply", "compute_supply", "compute_delivery", "供给不足"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := traceShortRootCauseFromDecision(types.TraceCauseDecision{
				Token:       types.TraceCausalTokenSnapshot{Token: tt.token, Lane: tt.lane},
				SubjectName: "Worker", ResourceName: "Lock contention on ClassLinker classes lock (owner tid: 42)",
				PhaseName: "DrawFrame",
			})
			if got.RootCause != tt.want {
				t.Fatalf("got %q, want %q", got.RootCause, tt.want)
			}
		})
	}
}

func TestShortRootCausePromptGate(t *testing.T) {
	for _, request := range []string{"需要简短根因", "请显示简短根因", "分析并输出简短根因"} {
		ctx := &types.AgentContext{Objective: request}
		if !traceShortRootCauseRequested(ctx) {
			t.Fatalf("request %q should enable short output", request)
		}
	}
	for _, request := range []string{"分析 Trace 根因", "不需要简短根因，只要完整分析", "不要简短根因"} {
		ctx := &types.AgentContext{Objective: request}
		if traceShortRootCauseRequested(ctx) {
			t.Fatalf("request %q must not enable short output", request)
		}
	}
}

func TestRenderTraceFindingShortRootCauseRequiresPromptOptIn(t *testing.T) {
	mutable := types.NewMutableState("分析 Trace 根因")
	mutable.SetTraceFinding(&types.TraceFindingV1{PrimaryCause: &types.TraceCauseDecision{
		Token: types.TraceCausalTokenSnapshot{Token: "io_wait", Lane: "io_blocking"}, SubjectName: "Worker",
	}})
	ctx := &types.AgentContext{Objective: "分析 Trace 根因", Mutable: mutable}
	if got := renderTraceFindingShortRootCause(ctx, "zh"); got != "" {
		t.Fatalf("ordinary request must not show short output: %s", got)
	}
	ctx.Objective = "分析 Trace，且需要简短根因"
	if got := renderTraceFindingShortRootCause(ctx, "zh"); !strings.Contains(got, "Worker线程IO阻塞") {
		t.Fatalf("explicit opt-in did not show short output: %s", got)
	}
}

func TestMergeTraceFindingWithDetailedAnswerPreservesOriginalBytes(t *testing.T) {
	detail := "# 原始完整结论\n\n这里是原来的 Trace 证据、分析过程和建议。\n"
	if got := mergeTraceFindingWithDetailedAnswer(detail, "", "zh-CN"); got != detail {
		t.Fatalf("no short output must be byte-identical: %q != %q", got, detail)
	}
	short := "## 简短根因\n\n```json\n{\"thread_name\":\"UI\",\"root_cause\":\"UI线程阻塞\"}\n```"
	got := mergeTraceFindingWithDetailedAnswer(detail, short, "zh-CN")
	if !strings.HasPrefix(got, short) || !strings.HasSuffix(got, detail) || !strings.Contains(got, "## 完整分析") {
		t.Fatalf("short summary must prefix an unchanged long answer:\n%s", got)
	}
}

func TestPrepareTraceFindingContractDoesNotChangeFinalizerSchema(t *testing.T) {
	ctx := &types.AgentContext{
		Objective: "分析 Trace，而且需要简短根因",
		Mutable:   types.NewMutableState("分析 Trace，而且需要简短根因"),
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "capture.systrace", Carrier: "request_path"}},
		}),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}},
	}
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatalf("prepareTraceFindingContract: %v", err)
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || contract.Required {
		t.Fatalf("deterministic snapshot must exist with Required=false: %+v", contract)
	}
	if got := renderAnswerDocTraceDecisionHandoff(ctx); strings.Contains(got, "Trace Finding Contract") {
		t.Fatalf("finalizer prompt must not receive the finding contract:\n%s", got)
	}
	finalizeDeterministicTraceFinding(ctx)
	if finding := ctx.Mutable.TraceFinding(); finding == nil || finding.Unresolved == nil {
		t.Fatalf("empty trace evidence must produce a deterministic unresolved sidecar: %+v", finding)
	}
}

func TestPrepareTraceFindingContractOrdinaryTraceRequestStaysInactive(t *testing.T) {
	ctx := &types.AgentContext{
		Objective: "分析 Trace 根因",
		Mutable:   types.NewMutableState("分析 Trace 根因"),
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "capture.systrace"}},
		}),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}},
	}
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatal(err)
	}
	if contract := ctx.Mutable.TraceFindingContract(); contract != nil {
		t.Fatalf("ordinary long-form trace answer must remain untouched: %+v", contract)
	}
}

func TestPrepareTraceFindingContractSidecarFlagWorksWithoutPromptOptIn(t *testing.T) {
	ctx := &types.AgentContext{
		Objective: "分析 Trace 根因", TraceFindingRequired: true,
		Mutable: types.NewMutableState("分析 Trace 根因"),
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "capture.systrace"}},
		}),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentRootCause}},
	}
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatal(err)
	}
	if contract := ctx.Mutable.TraceFindingContract(); contract == nil || contract.Required {
		t.Fatalf("sidecar request must compile a non-schema contract: %+v", contract)
	}
}

func TestPrepareTraceFindingContractNarrowTraceFactStaysInactive(t *testing.T) {
	ctx := &types.AgentContext{
		Objective: "列出线程状态，需要简短根因", Mutable: types.NewMutableState("列出线程状态，需要简短根因"),
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "capture.systrace"}},
		}),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace,
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
				FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactTargetSchedulerState}},
		}},
	}
	if err := prepareTraceFindingContract(ctx); err != nil {
		t.Fatal(err)
	}
	if contract := ctx.Mutable.TraceFindingContract(); contract != nil {
		t.Fatalf("narrow fact request must not be widened: %+v", contract)
	}
}

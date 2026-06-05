package repl

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pterm/pterm"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceFromLLMResponseCapturesReasoningAndToolCall(t *testing.T) {
	trace := traceFromLLMResponse("turn_policy_classifier", llm.Response{
		ReasoningContent: "需要先判断是不是数据任务。",
		ToolCalls: []llm.ToolCall{{
			Name:   "emit_turn_policy",
			Params: json.RawMessage(`{"route":"data"}`),
		}},
	})
	if trace.Scope != "turn_policy_classifier" {
		t.Fatalf("Scope=%q", trace.Scope)
	}
	if trace.Reasoning != "需要先判断是不是数据任务。" {
		t.Fatalf("Reasoning=%q", trace.Reasoning)
	}
	if trace.ToolName != "emit_turn_policy" {
		t.Fatalf("ToolName=%q", trace.ToolName)
	}
	if string(trace.ToolParams) != `{"route":"data"}` {
		t.Fatalf("ToolParams=%s", trace.ToolParams)
	}
}

type staticReplTraceProvider struct {
	trace replLLMCallTrace
}

func (p staticReplTraceProvider) LastReplLLMTrace() replLLMCallTrace { return p.trace }

func TestEmitReplLLMTraceRendersReasoningForDirectDataAndOperationCalls(t *testing.T) {
	var out bytes.Buffer
	r := &REPL{
		renderer: render.New(&out, true),
		language: "zh",
	}
	r.emitReplLLMTrace(staticReplTraceProvider{trace: replLLMCallTrace{
		Scope:      "data_task_planner",
		Reasoning:  "需要先读取表格并计算总额。",
		ToolName:   "emit_data_task_plan",
		ToolParams: json.RawMessage(`{"status":"ready"}`),
	}}, "data_task_planner", types.AgentName("data_planner"), types.PipelineStage("data"))

	got := stripANSIOnly(out.String())
	for _, want := range []string{"需要先读取表格并计算总额", "调用工具", "emit_data_task_plan"} {
		if !strings.Contains(got, want) {
			t.Fatalf("direct LLM trace output missing %q:\n%s", want, got)
		}
	}
}

type directTraceStubAdapter struct {
	response llm.Response
}

func (a directTraceStubAdapter) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	if opts.OnReasoningDelta != nil {
		opts.OnReasoningDelta("reasoning ")
	}
	if opts.OnContentDelta != nil {
		opts.OnContentDelta("content")
	}
	return a.response, nil
}

func (a directTraceStubAdapter) ModelID() string { return "stub-direct" }

func (a directTraceStubAdapter) MaxContextTokens() int { return 200000 }

func (a directTraceStubAdapter) MaxOutputTokens() int { return 0 }

func (a directTraceStubAdapter) RequestTimeout() time.Duration { return time.Minute }

func (a directTraceStubAdapter) RetryMaxAttempts() int { return 1 }

func TestDirectLLMTraceAdapterKeepsCallbacksPassive(t *testing.T) {
	var out bytes.Buffer
	wrapped := NewDirectLLMTraceAdapter(directTraceStubAdapter{response: llm.Response{
		Content:          "final content",
		ReasoningContent: "final reasoning",
	}}, render.New(&out, true), types.AgentName("data_planner"), types.PipelineStage("data"))

	var reasoningDeltas []string
	var contentDeltas []string
	resp, err := wrapped.Chat(context.Background(), nil, nil, llm.ChatOptions{
		OnReasoningDelta: func(delta string) { reasoningDeltas = append(reasoningDeltas, delta) },
		OnContentDelta:   func(delta string) { contentDeltas = append(contentDeltas, delta) },
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "final content" || resp.ReasoningContent != "final reasoning" {
		t.Fatalf("response was modified: %+v", resp)
	}
	if strings.Join(reasoningDeltas, "") != "reasoning " {
		t.Fatalf("reasoning deltas = %+v", reasoningDeltas)
	}
	if strings.Join(contentDeltas, "") != "content" {
		t.Fatalf("content deltas = %+v", contentDeltas)
	}
	if got := stripANSIOnly(out.String()); !strings.Contains(got, "final reasoning") {
		t.Fatalf("direct reasoning should enter permanent scrollback, got %q", got)
	}
	if got := stripANSIOnly(out.String()); strings.Contains(got, "final content") {
		t.Fatalf("ordinary content must not be duplicated as thinking scrollback, got %q", got)
	}
}

func TestDirectLLMTraceAdapterPersistsVisibleThinkBlocksOnly(t *testing.T) {
	var out bytes.Buffer
	wrapped := NewDirectLLMTraceAdapter(directTraceStubAdapter{response: llm.Response{
		Content: "<think>visible model thinking</think>\n\nordinary response",
	}}, render.New(&out, true), types.AgentName("operation_planner"), types.PipelineStage("operation"))

	resp, err := wrapped.Chat(context.Background(), nil, nil, llm.ChatOptions{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content == "" {
		t.Fatal("response content unexpectedly empty")
	}
	got := stripANSIOnly(out.String())
	if !strings.Contains(got, "visible model thinking") {
		t.Fatalf("think block should enter permanent scrollback, got %q", got)
	}
	if !strings.Contains(got, "<think>") || !strings.Contains(got, "</think>") {
		t.Fatalf("explicit think block boundary should stay visible in process scrollback, got %q", got)
	}
	if strings.Contains(got, "ordinary response") {
		t.Fatalf("ordinary response must not be duplicated as thinking scrollback, got %q", got)
	}
}

func TestDirectLLMTraceAdapterPersistsOrdinaryContentWhenToolCalling(t *testing.T) {
	var out bytes.Buffer
	wrapped := NewDirectLLMTraceAdapter(directTraceStubAdapter{response: llm.Response{
		Content: "I will inspect the data plan before calling the tool.",
		ToolCalls: []llm.ToolCall{{
			Name:   "emit_data_task_plan",
			Params: []byte(`{"status":"blocked"}`),
		}},
	}}, render.New(&out, true), types.AgentName("data_planner"), types.PipelineStage("data"))

	_, err := wrapped.Chat(context.Background(), nil, nil, llm.ChatOptions{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	got := stripANSIOnly(out.String())
	if !strings.Contains(got, "I will inspect the data plan before calling the tool.") {
		t.Fatalf("tool-calling prose should enter permanent scrollback, got %q", got)
	}
}

func TestTurnPolicyAuditSummaryShowsTypedRouteDecision(t *testing.T) {
	label, segs := turnPolicyAuditSummary(TurnPolicy{
		Route:      RouteOperation,
		Confidence: 0.82,
		Operation:  "computer_operation",
	}, "zh")
	if label != "路由判定" {
		t.Fatalf("label=%q", label)
	}
	joined := strings.Join(segs, " | ")
	for _, want := range []string{"操作", "置信 82%", "未读源码", "意图 computer_operation"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("summary %q missing %q", joined, want)
		}
	}
}

func TestDataTaskPlanAuditSummaryShowsOutputContract(t *testing.T) {
	label, segs := dataTaskPlanAuditSummary(dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv", "invoices.csv"},
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputCSVLine,
			ExplanationAllowed: false,
		},
		Script: "rows = csv_rows('orders.csv')\nemit({'answer':'1'})",
	}, "zh")
	if label != "数据计划" {
		t.Fatalf("label=%q", label)
	}
	joined := strings.Join(segs, " | ")
	for _, want := range []string{"就绪", "输入 2", "输出 CSV 行", "脚本 2 行", "纯输出"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("summary %q missing %q", joined, want)
		}
	}
}

func TestEmitDataTaskRunnerCallUsesToolCallUX(t *testing.T) {
	var out bytes.Buffer
	r := &REPL{
		renderer: render.New(&out, true),
		language: "zh",
	}
	r.emitDataTaskRunnerCall(dataquery.TaskPlan{
		InputPaths: []string{"orders.csv", "rules.md"},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials:       []dataquery.CoverageMaterial{{Path: "orders.csv", Required: true}},
			DecisionRecordsRequired: true,
		},
		Script: "rows = csv_rows('orders.csv')\nemit({'answer':'1'})",
	}, 2)

	got := stripANSIOnly(out.String())
	for _, want := range []string{"⇢ 数据 · 第 2 轮", "调用工具 data_runner", "输入=2", "脚本=2行", "必需材料=1", "需决策记录"} {
		if !strings.Contains(got, want) {
			t.Fatalf("data runner tool-call output missing %q:\n%s", want, got)
		}
	}
}

func TestEmitDataTaskWorkflowAuditKeepsDeterministicSegmentsFirst(t *testing.T) {
	var out bytes.Buffer
	r := &REPL{
		renderer: render.New(&out, true),
		language: "zh",
	}
	r.emitDataTaskWorkflowAudit("repair", 1, "上次失败 execute data task")

	got := stripANSIOnly(out.String())
	want := "数据工作流 · 修复第 1 次 · 未读源码 · 上次失败 execute data task"
	if !strings.Contains(got, want) {
		t.Fatalf("workflow audit should keep fixed lane status before dynamic details; want %q in:\n%s", want, got)
	}
}

func TestAuditDataTaskPlanWritesFullArtifactsAndRendersShortPreview(t *testing.T) {
	var out bytes.Buffer
	script := strings.Repeat("rows = csv_rows('orders.csv')\n", 80) + "emit({'answer': 'ok'})\n"
	anchor := t.TempDir()
	r := &REPL{
		renderer:      render.New(&out, true),
		language:      "zh",
		runtimeAnchor: anchor,
		repoRoot:      t.TempDir(),
		out:           &out,
	}

	artifact := r.auditDataTaskPlan("initial", 0, dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"orders.csv"},
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputPlainSingleLine,
			ExplanationAllowed: false,
		},
		Script: script,
	})

	if artifact.PlanPath == "" || artifact.ScriptPath == "" {
		t.Fatalf("expected plan and script audit paths, got %#v", artifact)
	}
	if filepath.Dir(artifact.ScriptPath) != filepath.Join(anchor, "data-audit") {
		t.Fatalf("script path should live under data-audit: %s", artifact.ScriptPath)
	}
	rawScript, err := os.ReadFile(artifact.ScriptPath)
	if err != nil {
		t.Fatalf("read script artifact: %v", err)
	}
	if string(rawScript) != script {
		t.Fatalf("script artifact mismatch")
	}
	rawPlan, err := os.ReadFile(artifact.PlanPath)
	if err != nil {
		t.Fatalf("read plan artifact: %v", err)
	}
	if !strings.Contains(string(rawPlan), `"script"`) || !strings.Contains(string(rawPlan), "orders.csv") {
		t.Fatalf("plan artifact missing expected content:\n%s", string(rawPlan))
	}
	got := stripANSIOnly(out.String())
	for _, want := range []string{"数据脚本", "脚本预览", "完整脚本", "完整计划", "面板预览已截断"} {
		if !strings.Contains(got, want) {
			t.Fatalf("audit preview missing %q:\n%s", want, got)
		}
	}
}

func TestDataTaskMutedPreviewKeepsTextAuditable(t *testing.T) {
	var out bytes.Buffer
	r := &REPL{out: &out, language: "zh", colorMode: render.ColorNever}
	preview := r.dataTaskMutedPreview("print('hello')\nprint('world')")
	if strings.TrimSpace(preview) == "" {
		t.Fatal("muted preview should not be empty")
	}
	if strings.Contains(preview, "\x1b[") {
		t.Fatalf("non-rendered preview must not contain ANSI escapes: %q", preview)
	}
	plain := stripANSIOnly(preview)
	for _, want := range []string{"print('hello')", "print('world')"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("muted preview lost %q: raw=%q plain=%q", want, preview, plain)
		}
	}
}

func TestDataTaskAuditPanelUsesMutedColorWhenEnabled(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	var out bytes.Buffer
	r := &REPL{out: &out, language: "zh", colorMode: render.ColorAlways}
	r.renderBorderedMutedCompact("脚本预览：\nprint('hello')\n\n完整脚本：`/tmp/script.py`")
	got := out.String()
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("ColorAlways should color muted data audit panel: %q", got)
	}
	wantQuiet := pterm.NewStyle(pterm.FgWhite, pterm.Fuzzy).Sprint("print('hello')")
	if !strings.Contains(got, wantQuiet) {
		t.Fatalf("muted data audit panel should use quiet-white preview style; want %q in %q", wantQuiet, got)
	}
	plain := stripANSIOnly(got)
	for _, want := range []string{"│ 脚本预览", "│ print('hello')", "│ 完整脚本"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("muted data audit panel lost %q: raw=%q plain=%q", want, got, plain)
		}
	}

	out.Reset()
	r.colorMode = render.ColorNever
	r.renderBorderedMutedCompact("脚本预览：\nprint('hello')")
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("ColorNever must keep data audit panels plain: %q", out.String())
	}
}

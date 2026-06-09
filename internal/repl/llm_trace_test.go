package repl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pterm/pterm"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/dataworkflow"
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

type directTraceToolDeltaStubAdapter struct {
	response llm.Response
	deltas   []string
	name     string
}

func (a directTraceToolDeltaStubAdapter) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	for _, delta := range a.deltas {
		if opts.OnToolCallDelta != nil {
			opts.OnToolCallDelta(0, a.name, delta)
		}
	}
	return a.response, nil
}

func (a directTraceToolDeltaStubAdapter) ModelID() string { return "stub-direct-tool-delta" }

func (a directTraceToolDeltaStubAdapter) MaxContextTokens() int { return 200000 }

func (a directTraceToolDeltaStubAdapter) MaxOutputTokens() int { return 0 }

func (a directTraceToolDeltaStubAdapter) RequestTimeout() time.Duration { return time.Minute }

func (a directTraceToolDeltaStubAdapter) RetryMaxAttempts() int { return 1 }

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

func TestDirectLLMTraceAdapterSurfacesStreamingToolArguments(t *testing.T) {
	var events []render.Event
	wrapped := &directLLMTraceAdapter{
		inner: directTraceToolDeltaStubAdapter{
			name:   "emit_data_task_plan",
			deltas: []string{`{"status":"ready",`, `"script":"` + strings.Repeat("x", 2048)},
			response: llm.Response{ToolCalls: []llm.ToolCall{{
				Name:   "emit_data_task_plan",
				Params: []byte(`{"status":"ready","script":"ok"}`),
			}}},
		},
		emit: func(ev render.Event) {
			events = append(events, ev)
		},
		agent: types.AgentName("data_planner"),
		stage: types.PipelineStage("data"),
	}

	var observed []string
	resp, err := wrapped.Chat(context.Background(), nil, nil, llm.ChatOptions{
		OnToolCallDelta: func(index int, name string, argsChunk string) {
			observed = append(observed, name+":"+argsChunk)
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "emit_data_task_plan" {
		t.Fatalf("response tool call was modified: %+v", resp.ToolCalls)
	}
	if len(observed) != 2 {
		t.Fatalf("caller OnToolCallDelta was not preserved: %+v", observed)
	}
	var got strings.Builder
	for _, ev := range events {
		if ev.Kind == render.EventAgentContent {
			got.WriteString(ev.Reasoning)
			got.WriteByte('\n')
		}
	}
	gotText := got.String()
	for _, want := range []string{"tool_call", "emit_data_task_plan"} {
		if !strings.Contains(gotText, want) {
			t.Fatalf("streaming tool arguments should update direct planner status, missing %q:\n%s", want, gotText)
		}
	}
	if strings.Contains(gotText, strings.Repeat("x", 256)) {
		t.Fatalf("tool argument stream preview should not dump raw JSON payload:\n%s", gotText)
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

func TestTraceFromLLMResponseKeepsContentAndFullToolParams(t *testing.T) {
	rawParams := []byte(`{"status":"ready","script":"` + strings.Repeat("x", 4096) + `"}`)
	trace := traceFromLLMResponse("data_task_planner", llm.Response{
		Content: "<think>visible audit thought</think>\nI will call the tool.",
		ToolCalls: []llm.ToolCall{{
			Name:   "emit_data_task_plan",
			Params: rawParams,
		}},
	})
	if !strings.Contains(trace.Content, "<think>visible audit thought</think>") {
		t.Fatalf("content should preserve visible think block for audit, got %q", trace.Content)
	}
	if string(trace.ToolParams) != string(rawParams) {
		t.Fatalf("tool params should be preserved byte-for-byte for audit")
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
	event := dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
		Kind:   "repair",
		Round:  1,
		Status: "failed",
		Reason: "上次失败 execute data task",
		Plan: dataquery.TaskPlan{
			Goal: "计算业务指标",
		},
	})
	r.emitDataTaskWorkflowEvent(event, dataTaskWorkflowEventRenderOptions{IncludeGoal: true, IncludeFailure: true})

	got := stripANSIOnly(out.String())
	want := "数据工作流 · 修复第 1 次 · 未读源码"
	if !strings.Contains(got, want) {
		t.Fatalf("workflow audit should keep fixed lane status before dynamic details; want %q in:\n%s", want, got)
	}
	if strings.Contains(got, "数据工作流 · 修复第 1 次 · 目标：") {
		t.Fatalf("business detail should not be promoted into the permanent title line:\n%s", got)
	}
	if !strings.Contains(got, "目标：计算业务指标") {
		t.Fatalf("workflow audit should render business-facing detail below the summary line:\n%s", got)
	}
	if !strings.Contains(got, "原因：上次失败 execute data task") {
		t.Fatalf("workflow audit should render failure detail below the summary line:\n%s", got)
	}
}

func TestDataTaskWorkflowGuardContextDetailsUseTypedGuard(t *testing.T) {
	guard := dataworkflow.NewGuardResult("field_contract", "error", dataworkflow.RepairNeedsTypedAction, "missing field amount", dataworkflow.WorkflowViolation{
		Code:          "field_contract",
		MissingFields: []string{"amount"},
	})
	plan := dataquery.TaskPlan{
		WhyThisBatch: "校验字段契约",
		NextBatch:    "补齐缺失字段后继续",
	}
	event := dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
		Kind:         "repair",
		Round:        1,
		Plan:         plan,
		AuditDetails: []string{guard.Code},
		Guard:        &guard,
	})
	got := strings.Join(dataTaskWorkflowEventDetailLines(event, "zh", dataTaskWorkflowEventRenderOptions{IncludeBatch: true, IncludeNext: true, IncludeFailure: true, IncludeAudit: true}), "\n")
	for _, want := range []string{"本批：校验字段契约", "下一步：补齐缺失字段后继续", "原因：missing field amount", "审计：field_contract"} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed guard detail missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "原因：原因：") {
		t.Fatalf("guard reason should not be double-prefixed:\n%s", got)
	}
}

func TestEmitDataTaskWorkflowAuditKeepsAuditCountsOutOfTitle(t *testing.T) {
	var out bytes.Buffer
	r := &REPL{
		renderer: render.New(&out, true),
		language: "zh",
	}
	event := dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
		Kind:         "result",
		Round:        3,
		Plan:         dataquery.TaskPlan{Goal: "计算业务指标"},
		AuditDetails: []string{"消费材料 12 · 规则覆盖 9"},
	})
	r.emitDataTaskWorkflowEvent(event, dataTaskWorkflowEventRenderOptions{IncludeGoal: true, IncludeAudit: true})

	got := stripANSIOnly(out.String())
	if strings.Contains(got, "数据工作流 · 结果第 3 批 · 消费材料") {
		t.Fatalf("audit counters should not be promoted into the permanent title line:\n%s", got)
	}
	if strings.Contains(got, "目标：计算业务指标") {
		t.Fatalf("result event should not repeat plan business details already shown during planning/execution:\n%s", got)
	}
	for _, want := range []string{"数据工作流 · 结果第 3 批 · 未读源码", "结果：本批完成", "审计：消费材料 12 · 规则覆盖 9"} {
		if !strings.Contains(got, want) {
			t.Fatalf("workflow audit missing %q:\n%s", want, got)
		}
	}
}

func TestEmitDataTaskWorkflowAuditDoesNotRepeatPlanDetailsOnEvaluate(t *testing.T) {
	var out bytes.Buffer
	r := &REPL{
		renderer: render.New(&out, true),
		language: "zh",
	}
	event := dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
		Kind:  "evaluate",
		Round: 4,
		Plan: dataquery.TaskPlan{
			Goal:         "计算业务指标",
			WhyThisBatch: "抽取字段",
			NextBatch:    "汇总输出",
		},
	})
	r.emitDataTaskWorkflowEvent(event, dataTaskWorkflowEventRenderOptions{IncludeGoal: true, IncludeBatch: true, IncludeNext: true})

	got := stripANSIOnly(out.String())
	if strings.Contains(got, "目标：计算业务指标") || strings.Contains(got, "本批：抽取字段") || strings.Contains(got, "下一步：汇总输出") {
		t.Fatalf("evaluate event should not repeat plan details:\n%s", got)
	}
	for _, want := range []string{"数据工作流 · 评估第 4 批 · 未读源码", "评估：根据目标"} {
		if !strings.Contains(got, want) {
			t.Fatalf("workflow audit missing %q:\n%s", want, got)
		}
	}
}

func TestEmitDataTaskWorkflowEventRendersTypedResultAudit(t *testing.T) {
	var out bytes.Buffer
	r := &REPL{
		renderer: render.New(&out, true),
		language: "zh",
	}
	event := dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{
		Kind:  "result",
		Round: 2,
		Plan: dataquery.TaskPlan{
			Goal:         "计算业务指标",
			WhyThisBatch: "汇总记录",
		},
		Result: &dataquery.Result{
			ConsumedPaths: []string{"orders.csv", "rules.md"},
			Contributions: []dataquery.ContributionRecord{{
				ItemID: "r1",
			}},
		},
	})
	r.emitDataTaskWorkflowEvent(event, dataTaskWorkflowEventRenderOptions{IncludeAudit: true})

	got := stripANSIOnly(out.String())
	if strings.Contains(got, "目标：计算业务指标") || strings.Contains(got, "本批：汇总记录") {
		t.Fatalf("typed result event should not repeat business context:\n%s", got)
	}
	for _, want := range []string{"数据工作流 · 结果第 2 批 · 未读源码", "结果：本批完成", "审计：consumed_materials=2", "审计：contributions=1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed result event missing %q:\n%s", want, got)
		}
	}
}

func TestDataTaskPlanAuditSplitsTypedIntentIntoDetails(t *testing.T) {
	label, segs := dataTaskPlanAuditSummary(dataquery.TaskPlan{
		Status:        "ready",
		InputPaths:    []string{"orders.csv"},
		Goal:          "计算用户要求的聚合结果",
		WhyThisBatch:  "抽取基础记录并保留后续计算所需字段",
		NextBatch:     "根据抽取结果继续归一和汇总",
		ContinueAfter: true,
		Actions: []dataquery.DataAction{
			{ID: "extract", Kind: dataquery.DataActionExtractRecords},
			{ID: "derive", Kind: dataquery.DataActionDeriveFields},
		},
	}, "zh")
	got := label + " · " + strings.Join(segs, " · ")
	if strings.Contains(got, "目标 ") || strings.Contains(got, "本批 ") || strings.Contains(got, "下一步 ") {
		t.Fatalf("plan summary should not inline long typed intent details:\n%s", got)
	}
	details := strings.Join(dataTaskPlanAuditDetailLines(dataquery.TaskPlan{
		Status:        "ready",
		InputPaths:    []string{"orders.csv"},
		Goal:          "计算用户要求的聚合结果",
		WhyThisBatch:  "抽取基础记录并保留后续计算所需字段",
		NextBatch:     "根据抽取结果继续归一和汇总",
		ContinueAfter: true,
		Actions: []dataquery.DataAction{
			{ID: "extract", Kind: dataquery.DataActionExtractRecords},
			{ID: "derive", Kind: dataquery.DataActionDeriveFields},
		},
	}, "zh"), "\n")
	for _, want := range []string{"目标：计算用户要求的聚合结果", "本批：抽取基础记录并保留后续计算所需字段", "下一步：根据抽取结果继续归一和汇总", "动作：extract_records -> derive_fields"} {
		if !strings.Contains(got, want) {
			if !strings.Contains(details, want) {
				t.Fatalf("plan details missing %q:\n%s", want, details)
			}
		}
	}
}

func TestDataTaskWorkflowDetailsUseTypedEvents(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:        "ready",
		Goal:          "计算用户要求的聚合结果",
		WhyThisBatch:  "抽取基础记录并保留后续计算所需字段",
		NextBatch:     "根据抽取结果继续归一和汇总",
		ContinueAfter: true,
		Actions: []dataquery.DataAction{{
			ID:      "extract",
			Kind:    dataquery.DataActionExtractRecords,
			Purpose: "抽取基础记录",
		}},
	}
	joinedPlan := strings.Join(dataTaskPlanAuditDetailLines(plan, "zh"), "\n")
	if !strings.Contains(joinedPlan, "动作：抽取基础记录") {
		t.Fatalf("display details should show action purpose:\n%s", joinedPlan)
	}

	executeEvent := dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{Kind: "execute", Round: 1, Plan: plan})
	executeLines := strings.Join(dataTaskWorkflowEventDetailLines(executeEvent, "zh", dataTaskWorkflowEventRenderOptions{IncludeBatch: true, IncludeNext: true, IncludeActions: true}), "\n")
	if strings.Contains(executeLines, "目标：") || !strings.Contains(executeLines, "本批：抽取基础记录") || !strings.Contains(executeLines, "下一步：根据抽取结果继续归一和汇总") {
		t.Fatalf("execute workflow lines should show batch context without repeating goal:\n%s", executeLines)
	}

	resultEvent := dataworkflow.BuildWorkflowProcessEvent(dataworkflow.WorkflowProcessEventInput{Kind: "result", Round: 1, Plan: plan, AuditDetails: []string{"消费材料 2"}})
	resultLines := strings.Join(dataTaskWorkflowEventDetailLines(resultEvent, "zh", dataTaskWorkflowEventRenderOptions{IncludeBatch: true, IncludeActions: true, IncludeAudit: true}), "\n")
	if strings.Contains(resultLines, "目标：") || strings.Contains(resultLines, "本批：") || !strings.Contains(resultLines, "审计：消费材料 2") {
		t.Fatalf("result workflow lines should keep audit low-noise without repeated business intent:\n%s", resultLines)
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

func TestAuditDataTaskPlanWritesActionGraphSnapshot(t *testing.T) {
	var out bytes.Buffer
	anchor := t.TempDir()
	r := &REPL{
		renderer:      render.New(&out, true),
		language:      "zh",
		runtimeAnchor: anchor,
		repoRoot:      t.TempDir(),
		out:           &out,
	}

	artifact := r.auditDataTaskPlan("continue", 2, dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:   "normalize",
			Kind: dataquery.DataActionNormalizeEntities,
			Params: map[string]string{
				"source_path":    "records.json",
				"reference_path": "reference.json",
			},
		}},
	})

	if artifact.ActionGraphPath == "" {
		t.Fatalf("expected action graph audit path, got %#v", artifact)
	}
	raw, err := os.ReadFile(artifact.ActionGraphPath)
	if err != nil {
		t.Fatalf("read action graph artifact: %v", err)
	}
	text := string(raw)
	for _, want := range []string{`"original_action"`, `"normalized_action"`, `"action_node"`, `"records.json"`, `"reference.json"`, `"idempotency_key"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("action graph audit missing %q:\n%s", want, text)
		}
	}
	got := stripANSIOnly(out.String())
	if !strings.Contains(got, "完整动作图") {
		t.Fatalf("audit preview missing action graph path:\n%s", got)
	}
}

func TestAuditDataTaskPlanMarksDeferredActions(t *testing.T) {
	r := &REPL{runtimeAnchor: t.TempDir(), repoRoot: t.TempDir()}
	artifact := r.auditDataTaskPlan("deferred", 4, dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:             "join_later",
			Kind:           dataquery.DataActionJoinRecords,
			InputPaths:     []string{"left.json", "right.json"},
			OutputArtifact: "joined.json",
		}},
	})
	if artifact.ActionGraphPath == "" {
		t.Fatalf("expected deferred action graph path")
	}
	raw, err := os.ReadFile(artifact.ActionGraphPath)
	if err != nil {
		t.Fatalf("read deferred action graph: %v", err)
	}
	if !strings.Contains(string(raw), `"status": "deferred"`) {
		t.Fatalf("deferred action graph did not mark node deferred:\n%s", string(raw))
	}
}

func TestAuditDataTaskResultWritesArtifactGraphSnapshot(t *testing.T) {
	var out bytes.Buffer
	anchor := t.TempDir()
	r := &REPL{
		renderer:      render.New(&out, true),
		language:      "zh",
		runtimeAnchor: anchor,
		repoRoot:      t.TempDir(),
		out:           &out,
	}

	artifact := r.auditDataTaskResult(3, dataquery.Result{
		Answer: "ok",
		Artifacts: []dataquery.DataArtifact{{
			ID:          "records",
			Kind:        string(dataquery.DataActionExtractRecords),
			Headers:     []string{"id", "amount"},
			SourcePaths: []string{"records.csv"},
			Fields: map[string]string{
				"artifact_aliases": "records.json",
				"json_shape":       "array(len=2,item=object(keys=id,amount))",
			},
		}},
	})

	if artifact.ResultPath == "" || artifact.ArtifactGraphPath == "" {
		t.Fatalf("expected result and artifact graph audit paths, got %#v", artifact)
	}
	raw, err := os.ReadFile(artifact.ArtifactGraphPath)
	if err != nil {
		t.Fatalf("read artifact graph: %v", err)
	}
	text := string(raw)
	for _, want := range []string{`"node_class"`, `"record"`, `"records.json"`, `"id"`, `"amount"`, `"records.csv"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("artifact graph missing %q:\n%s", want, text)
		}
	}
	got := stripANSIOnly(out.String())
	if !strings.Contains(got, "完整产物图") {
		t.Fatalf("result preview missing artifact graph path:\n%s", got)
	}
}

func TestDataTaskTerminalAuditWritesGraphSnapshot(t *testing.T) {
	anchor := t.TempDir()
	r := &REPL{
		language:      "zh",
		runtimeAnchor: anchor,
		repoRoot:      t.TempDir(),
	}
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:             "extract",
			Kind:           dataquery.DataActionExtractRecords,
			InputPaths:     []string{"records.csv"},
			OutputArtifact: "records.json",
		}}},
		Err: `data planning incomplete: action 1 (filter_eligible) references field(s) [status] that are not present on input records fields [id, amount]. Use an existing artifact from workflow_state_json.artifact_availability, or first materialize the missing field(s) with derive_fields, extract_fields, group_records, enrich_records, join_records, or a valid prior typed action before consuming them.`,
		Result: &dataquery.Result{Artifacts: []dataquery.DataArtifact{{
			ID:      "records",
			Kind:    string(dataquery.DataActionExtractRecords),
			Headers: []string{"id", "amount"},
			Fields:  map[string]string{"artifact_aliases": "records", "json_shape": "array(len=2,item=object(keys=id,amount))"},
		}}},
	}}
	path := r.writeDataTaskTerminalArtifact(
		dataTaskTerminalAudit{Status: "failed", Reason: "field gap", DataRounds: 1, Records: records},
		"failed",
		"field gap",
		records[0].Err,
		"answer_len=0",
	)
	if path == "" {
		t.Fatalf("terminal audit path empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read terminal audit: %v", err)
	}
	text := string(raw)
	for _, want := range []string{`"action_events"`, `"action_graph"`, `"artifact_graph"`, `"progress"`, `"workflow_violations"`, `"process_events"`, `"action_batch"`, `"blocked"`, `"records"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("terminal audit missing %q:\n%s", want, text)
		}
	}
}

func TestDataTaskTerminalAuditWithRuntimeUsesRuntimeSnapshot(t *testing.T) {
	anchor := t.TempDir()
	runtimePlan := dataquery.TaskPlan{
		Goal: "runtime terminal goal",
		Actions: []dataquery.DataAction{{
			ID:             "runtime_terminal_extract",
			Kind:           dataquery.DataActionExtractRecords,
			InputPaths:     []string{"runtime.csv"},
			OutputArtifact: "runtime.json",
		}},
	}
	rt := dataworkflow.NewWorkflowRuntime(dataquery.TaskPlan{})
	rt.SwitchCurrentPlan(4, "continue", runtimePlan, "runtime terminal plan")
	rt.AppendRecord(dataTaskWorkflowRecord{
		Plan: runtimePlan,
		Result: &dataquery.Result{
			Answer:        "runtime-terminal-answer",
			ConsumedPaths: []string{"runtime.csv"},
		},
	})
	rt.SetRounds(4, 1)

	path := writeDataTaskTerminalArtifactFileWithRuntime(
		anchor,
		t.TempDir(),
		rt,
		dataTaskTerminalAudit{
			Status:       "failed",
			Reason:       "terminal runtime",
			DataRounds:   99,
			RepairRounds: 99,
			Records: []dataTaskWorkflowRecord{{
				Plan: dataquery.TaskPlan{Goal: "preview terminal goal"},
				Err:  "preview terminal error",
			}},
		},
		"failed",
		"terminal runtime",
		"preview terminal error",
		"preview terminal summary",
		"test",
	)
	if path == "" {
		t.Fatal("terminal audit path empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read terminal audit: %v", err)
	}
	text := string(raw)
	for _, want := range []string{`"data_rounds": 4`, `"repair_rounds": 1`, "runtime terminal goal", "runtime-terminal-answer"} {
		if !strings.Contains(text, want) {
			t.Fatalf("runtime terminal audit missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"preview terminal goal", "preview terminal error", "preview terminal summary"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("runtime terminal audit leaked preview value %q:\n%s", forbidden, text)
		}
	}
}

func TestDataTaskTerminalAuditDetailedStatusUsesJournalSnapshot(t *testing.T) {
	rt := dataworkflow.NewWorkflowRuntime(dataquery.TaskPlan{
		Status: "ready",
		CoverageContract: dataquery.CoverageContract{
			ContributionLedgerRequired: true,
		},
	})
	rt.SetRounds(1, 0)

	terminal := writeDataTaskTerminalArtifactFileWithRuntimeDetailed(
		t.TempDir(),
		t.TempDir(),
		rt,
		dataTaskTerminalAudit{Status: "complete", DataRounds: 1},
		"complete",
		"",
		"",
		"",
		"test",
	)
	if terminal.Path == "" {
		t.Fatal("terminal audit path empty")
	}
	if strings.TrimSpace(terminal.Snapshot.Status) == "" || strings.EqualFold(terminal.Snapshot.Status, "complete") {
		t.Fatalf("Snapshot.Status=%q, want runtime-derived non-complete status", terminal.Snapshot.Status)
	}
	raw, err := os.ReadFile(terminal.Path)
	if err != nil {
		t.Fatalf("read terminal audit: %v", err)
	}
	if !strings.Contains(string(raw), fmt.Sprintf(`"status": "%s"`, terminal.Snapshot.Status)) {
		t.Fatalf("terminal JSON does not contain detailed snapshot status %q:\n%s", terminal.Snapshot.Status, string(raw))
	}
}

func TestDataTaskWorkflowCheckpointUsesJournalSchema(t *testing.T) {
	anchor := t.TempDir()
	records := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{
			Goal:         "compute requested total",
			WhyThisBatch: "materialize source rows",
			NextBatch:    "derive fields then aggregate",
			Actions: []dataquery.DataAction{{
				ID:             "extract",
				Kind:           dataquery.DataActionExtractRecords,
				InputPaths:     []string{"records.csv"},
				OutputArtifact: "records.json",
			}},
		},
		Result: &dataquery.Result{
			Answer:        "3",
			ConsumedPaths: []string{"records.csv"},
			Artifacts: []dataquery.DataArtifact{{
				ID:      "records",
				Kind:    string(dataquery.DataActionExtractRecords),
				Headers: []string{"id", "value"},
				Fields:  map[string]string{"artifact_aliases": "records"},
			}},
		},
	}}
	guard := dataworkflow.NewGuardResult("missing_executable_body", "error", dataworkflow.RepairNeedsTypedAction, "guard stopped a non-executable batch")
	path := writeDataTaskWorkflowCheckpointFile(anchor, t.TempDir(), records, dataquery.TaskPlan{}, dataquery.TaskPlan{}, 1, 0, "batch result completed", "test", guard)
	if path == "" {
		t.Fatal("checkpoint path empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	text := string(raw)
	for _, want := range []string{`"status": "checkpoint"`, `"action_events"`, `"action_graph"`, `"artifact_graph"`, `"progress"`, `"process_events"`, `"resume"`, `"records"`, `"current_plan"`, `"deferred_plan"`, `"compute requested total"`, `"batch_purpose"`, `"next_step"`, `"action_summary"`, `"audit_details"`, `"materialize source rows"`, `"guard"`, `"missing_executable_body"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("checkpoint missing %q:\n%s", want, text)
		}
	}
}

func TestDataTaskWorkflowCheckpointWithRuntimeUsesRuntimeSnapshot(t *testing.T) {
	anchor := t.TempDir()
	runtimePlan := dataquery.TaskPlan{
		Goal: "runtime goal",
		Actions: []dataquery.DataAction{{
			ID:             "runtime_extract",
			Kind:           dataquery.DataActionExtractRecords,
			InputPaths:     []string{"runtime.csv"},
			OutputArtifact: "runtime.json",
		}},
	}
	rt := dataworkflow.NewWorkflowRuntime(dataquery.TaskPlan{})
	rt.SwitchCurrentPlan(3, "continue", runtimePlan, "runtime plan")
	rt.AppendRecord(dataTaskWorkflowRecord{
		Plan: runtimePlan,
		Result: &dataquery.Result{
			Answer:        "runtime-answer",
			ConsumedPaths: []string{"runtime.csv"},
		},
	})
	rt.EnqueueDeferred(3, dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:   "runtime_deferred",
		Kind: dataquery.DataActionComputeContribs,
	}}}, "runtime deferred")
	rt.SetRounds(3, 2)

	previewRecords := []dataTaskWorkflowRecord{{
		Plan: dataquery.TaskPlan{Goal: "preview goal"},
		Err:  "preview should not enter runtime journal",
	}}
	path := writeDataTaskWorkflowCheckpointFileWithDeferredQueue(
		anchor,
		t.TempDir(),
		rt,
		previewRecords,
		dataquery.TaskPlan{Goal: "preview current"},
		dataworkflow.NewDeferredQueue(dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:   "preview_deferred",
			Kind: dataquery.DataActionFilterRecords,
		}}}),
		99,
		99,
		"runtime checkpoint",
		"test",
	)
	if path == "" {
		t.Fatal("checkpoint path empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	text := string(raw)
	for _, want := range []string{`"data_rounds": 3`, `"repair_rounds": 2`, "runtime goal", "runtime-answer", "runtime_deferred"} {
		if !strings.Contains(text, want) {
			t.Fatalf("runtime checkpoint missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"preview goal", "preview current", "preview_deferred", "preview should not enter runtime journal"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("runtime checkpoint leaked preview value %q:\n%s", forbidden, text)
		}
	}
}

func TestDataTaskTerminalAuditPathRendersLowNoiseSummary(t *testing.T) {
	var out bytes.Buffer
	r := &REPL{
		renderer: render.New(&out, true),
		language: "zh",
	}

	r.emitDataTaskTerminalAuditPath("failed", "/tmp/data-audit/terminal.json")

	got := stripANSIOnly(out.String())
	for _, want := range []string{"数据审计", "failed", "完整终态 /tmp/data-audit/terminal.json"} {
		if !strings.Contains(got, want) {
			t.Fatalf("terminal audit summary missing %q:\n%s", want, got)
		}
	}
}

func TestDataTaskCLITerminalAuditPathRendersFailureReason(t *testing.T) {
	var out bytes.Buffer

	emitDataTaskCLITerminalAuditPath(&out, "zh", "failed", "/tmp/data-audit/terminal.json", "data validation incomplete")

	got := stripANSIOnly(out.String())
	for _, want := range []string{"数据审计", "failed", "完整终态 /tmp/data-audit/terminal.json", "原因：data validation incomplete"} {
		if !strings.Contains(got, want) {
			t.Fatalf("terminal audit summary missing %q:\n%s", want, got)
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

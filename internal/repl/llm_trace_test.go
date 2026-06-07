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
	r.emitDataTaskWorkflowAudit("repair", 1, "上次失败 execute data task")

	got := stripANSIOnly(out.String())
	want := "数据工作流 · 修复第 1 次 · 未读源码"
	if !strings.Contains(got, want) {
		t.Fatalf("workflow audit should keep fixed lane status before dynamic details; want %q in:\n%s", want, got)
	}
	if !strings.Contains(got, "细节：上次失败 execute data task") {
		t.Fatalf("workflow audit should render dynamic detail below the summary line:\n%s", got)
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
	details := strings.Join(dataTaskPlanAuditDetails(dataquery.TaskPlan{
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
	for _, want := range []string{"目标：计算用户要求的聚合结果", "本批：抽取基础记录并保留后续计算所需字段", "下一步：根据抽取结果继续归一和汇总", "步骤 extract:extract_records → derive:derive_fields"} {
		if !strings.Contains(got, want) {
			if !strings.Contains(details, want) {
				t.Fatalf("plan details missing %q:\n%s", want, details)
			}
		}
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
	for _, want := range []string{`"action_graph"`, `"artifact_graph"`, `"workflow_violations"`, `"blocked"`, `"records"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("terminal audit missing %q:\n%s", want, text)
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

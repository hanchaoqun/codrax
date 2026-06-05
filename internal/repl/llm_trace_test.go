package repl

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

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

	got := out.String()
	for _, want := range []string{"需要先读取表格并计算总额", "调用工具", "emit_data_task_plan"} {
		if !strings.Contains(got, want) {
			t.Fatalf("direct LLM trace output missing %q:\n%s", want, got)
		}
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

	got := out.String()
	for _, want := range []string{"⇢ 数据 · 第 2 轮", "调用工具 data_runner", "输入=2", "脚本=2行", "必需材料=1", "需决策记录"} {
		if !strings.Contains(got, want) {
			t.Fatalf("data runner tool-call output missing %q:\n%s", want, got)
		}
	}
}

package repl

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/llm"
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
	}, "zh")
	if label != "数据计划" {
		t.Fatalf("label=%q", label)
	}
	joined := strings.Join(segs, " | ")
	for _, want := range []string{"就绪", "输入 2", "输出 CSV 行", "纯输出"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("summary %q missing %q", joined, want)
		}
	}
}

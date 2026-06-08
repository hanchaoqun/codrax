package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestBuildWorkflowProcessDisplayBuildsStableTitleSegments(t *testing.T) {
	display := BuildWorkflowProcessDisplay(WorkflowJournalEvent{Kind: "continue", Round: 2}, "zh")
	if display.Label != "数据工作流" {
		t.Fatalf("label=%q", display.Label)
	}
	got := strings.Join(display.Segments, " · ")
	for _, want := range []string{"继续第 3 批", "未读源码"} {
		if !strings.Contains(got, want) {
			t.Fatalf("segments=%q, want %q", got, want)
		}
	}
}

func TestBuildWorkflowProcessDisplayUsesTypedPlanIntent(t *testing.T) {
	event := BuildWorkflowProcessEvent(WorkflowProcessEventInput{
		Kind: "execute",
		Plan: dataquery.TaskPlan{
			Goal:         "计算业务指标",
			WhyThisBatch: "抽取基础记录",
			NextBatch:    "根据抽取结果继续归一和汇总",
			Actions: []dataquery.DataAction{{
				Kind:    dataquery.DataActionExtractRecords,
				Purpose: "抽取基础记录",
			}},
		},
	})
	display := BuildWorkflowProcessDisplay(event, "zh")
	got := processDisplayDetailText(display)
	for _, want := range []string{"目标：计算业务指标", "本批：抽取基础记录", "下一步：根据抽取结果继续归一和汇总", "动作：抽取基础记录"} {
		if !strings.Contains(got, want) {
			t.Fatalf("details=%q, want %q", got, want)
		}
	}
}

func TestBuildWorkflowProcessDisplayProvidesDefaultProcessDetail(t *testing.T) {
	display := BuildWorkflowProcessDisplay(WorkflowJournalEvent{Kind: "evaluate", Round: 4}, "zh")
	got := processDisplayDetailText(display)
	if !strings.Contains(got, "评估：根据目标") {
		t.Fatalf("details=%q, want evaluate fallback", got)
	}
}

func TestBuildWorkflowProcessDisplayNamesCompletionGate(t *testing.T) {
	display := BuildWorkflowProcessDisplay(WorkflowJournalEvent{Kind: "completion_gate", Round: 0}, "zh")
	if got := strings.Join(display.Segments, " · "); !strings.Contains(got, "终态校验第 1 批") {
		t.Fatalf("segments=%q, want completion gate title", got)
	}
	if got := processDisplayDetailText(display); !strings.Contains(got, "终态校验：检查最终答案") {
		t.Fatalf("details=%q, want completion gate fallback", got)
	}
}

func TestBuildWorkflowProcessDisplayRendersTypedBlockerSummary(t *testing.T) {
	guard := NewGuardResult("value_contract_violation", "error", RepairNeedsTypedAction, "amount is not numeric", WorkflowViolation{
		Code:              "value_contract_violation",
		ActionID:          "compute",
		ActionKind:        string(dataquery.DataActionComputeContribs),
		InputAlias:        "records.json",
		Field:             "amount",
		Operation:         "numeric_parse",
		RepairActionHints: []string{string(dataquery.DataActionDeriveFields)},
		Reason:            "amount is not numeric",
	})
	event := BuildWorkflowProcessEvent(WorkflowProcessEventInput{
		Kind:   "action_batch",
		Round:  3,
		Status: "failed",
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:   "compute",
			Kind: dataquery.DataActionComputeContribs,
		}}},
		Guard: &guard,
	})
	display := BuildWorkflowProcessDisplay(event, "zh")
	got := processDisplayDetailText(display)
	for _, want := range []string{"当前阻塞：字段值不满足本步操作需要的值类型或形状", "输入 records.json", "字段 amount", "操作 numeric_parse", "可继续：优先生成下一批结构化动作：补齐或派生字段", "原因：amount is not numeric"} {
		if !strings.Contains(got, want) {
			t.Fatalf("details=%q, want %q", got, want)
		}
	}
}

func processDisplayDetailText(display WorkflowProcessDisplay) string {
	var lines []string
	for _, detail := range display.Details {
		lines = append(lines, detail.Text)
	}
	return strings.Join(lines, "\n")
}

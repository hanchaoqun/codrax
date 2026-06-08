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

func TestBuildWorkflowProcessDisplayRendersActionLimitBlocker(t *testing.T) {
	guard := NewGuardResult("action_limit_violation", "error", RepairNeedsTypedAction, "too many records", WorkflowViolation{
		Code:              "action_limit_violation",
		ActionID:          "filter_limit",
		ActionKind:        string(dataquery.DataActionFilterRecords),
		Param:             "max_output_records",
		Limit:             1,
		Observed:          2,
		RepairActionHints: []string{string(dataquery.DataActionFilterRecords)},
		Reason:            "too many records",
	})
	event := BuildWorkflowProcessEvent(WorkflowProcessEventInput{
		Kind:  "action_batch",
		Round: 2,
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:   "filter_limit",
			Kind: dataquery.DataActionFilterRecords,
		}}},
		Guard: &guard,
	})
	display := BuildWorkflowProcessDisplay(event, "zh")
	got := processDisplayDetailText(display)
	for _, want := range []string{"当前阻塞：本步输出超过当前安全上限，需要拆成更小批次", "当前 2 / 上限 1", "参数 max_output_records", "可继续：优先生成下一批结构化动作：筛选记录"} {
		if !strings.Contains(got, want) {
			t.Fatalf("details=%q, want %q", got, want)
		}
	}
}

func TestBuildWorkflowProcessDisplayRendersMaterialContractBlocker(t *testing.T) {
	guard := NewGuardResult("material_contract_violation", "error", RepairNeedsTypedAction, "materials is a directory", WorkflowViolation{
		Code:              "material_contract_violation",
		ActionID:          "profile_inputs",
		ActionKind:        string(dataquery.DataActionCustomTransform),
		InputAlias:        "materials",
		Operation:         "script_consumed",
		RepairActionHints: []string{string(dataquery.DataActionExtractRecords)},
		Reason:            "materials is a directory",
	})
	event := BuildWorkflowProcessEvent(WorkflowProcessEventInput{
		Kind:  "action_batch",
		Round: 2,
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:   "profile_inputs",
			Kind: dataquery.DataActionCustomTransform,
		}}},
		Guard: &guard,
	})
	display := BuildWorkflowProcessDisplay(event, "zh")
	got := processDisplayDetailText(display)
	for _, want := range []string{"当前阻塞：本步选择的材料形态还不能被该动作可靠消费", "输入 materials", "操作 script_consumed", "可继续：优先生成下一批结构化动作：抽取记录"} {
		if !strings.Contains(got, want) {
			t.Fatalf("details=%q, want %q", got, want)
		}
	}
}

func TestBuildWorkflowProcessDisplayRendersActionDependencyBlocker(t *testing.T) {
	guard := NewGuardResult("action_dependency_violation", "error", RepairNeedsTypedAction, "reconcile requires contributions", WorkflowViolation{
		Code:              "action_dependency_violation",
		ActionID:          "reconcile_now",
		ActionKind:        string(dataquery.DataActionReconcile),
		Role:              "contributions",
		Operation:         "reconcile",
		RepairActionHints: []string{string(dataquery.DataActionComputeContribs)},
		Reason:            "reconcile requires contributions",
	})
	event := BuildWorkflowProcessEvent(WorkflowProcessEventInput{
		Kind:  "action_batch",
		Round: 2,
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:   "reconcile_now",
			Kind: dataquery.DataActionReconcile,
		}}},
		Guard: &guard,
	})
	display := BuildWorkflowProcessDisplay(event, "zh")
	got := processDisplayDetailText(display)
	for _, want := range []string{"当前阻塞：本步需要的上游结构化产物还没有就绪", "操作 reconcile", "可继续：优先生成下一批结构化动作：计算贡献"} {
		if !strings.Contains(got, want) {
			t.Fatalf("details=%q, want %q", got, want)
		}
	}
}

func TestBuildWorkflowProcessDisplayRendersOutputContractBlocker(t *testing.T) {
	guard := NewGuardResult("output_contract_violation", "error", RepairNeedsTypedAction, "output contract plain_single_line requires a single line", WorkflowViolation{
		Code:     "output_contract_violation",
		ActionID: "assemble",
		Reason:   "output contract plain_single_line requires a single line",
	})
	event := BuildWorkflowProcessEvent(WorkflowProcessEventInput{
		Kind:  "completion_gate",
		Round: 1,
		Guard: &guard,
	})
	display := BuildWorkflowProcessDisplay(event, "zh")
	got := processDisplayDetailText(display)
	if !strings.Contains(got, "当前阻塞：最终答案还没有满足用户要求的输出格式") {
		t.Fatalf("details=%q, want output-contract blocker", got)
	}
}

func TestBuildWorkflowProcessDisplayRendersArtifactMaterializationBlocker(t *testing.T) {
	guard := NewGuardResult("artifact_materialization_violation", "error", RepairNeedsTypedAction, "artifact payload could not be materialized", WorkflowViolation{
		Code:       "artifact_materialization_violation",
		ActionID:   "extract",
		ActionKind: string(dataquery.DataActionExtractRecords),
		InputAlias: "records",
		Reason:     "artifact payload could not be materialized",
	})
	event := BuildWorkflowProcessEvent(WorkflowProcessEventInput{
		Kind:  "action_batch",
		Round: 1,
		Guard: &guard,
	})
	display := BuildWorkflowProcessDisplay(event, "zh")
	got := processDisplayDetailText(display)
	if !strings.Contains(got, "当前阻塞：本步生成的中间产物还不能被系统可靠保存和复用") ||
		!strings.Contains(got, "输入 records") {
		t.Fatalf("details=%q, want materialization blocker", got)
	}
}

func TestBuildWorkflowProcessDisplayRendersRoleSelectionBlocker(t *testing.T) {
	guard := NewGuardResult("action_role_selection_violation", "error", RepairNeedsTypedAction, "source and reference paths overlap", WorkflowViolation{
		Code:         "action_role_selection_violation",
		ActionID:     "candidate",
		ActionKind:   string(dataquery.DataActionMappingCandidate),
		Role:         "source/reference",
		InputAliases: []string{"source.csv", "reference.csv"},
		Reason:       "source and reference paths overlap",
	})
	event := BuildWorkflowProcessEvent(WorkflowProcessEventInput{
		Kind:  "action_batch",
		Round: 1,
		Guard: &guard,
	})
	display := BuildWorkflowProcessDisplay(event, "zh")
	got := processDisplayDetailText(display)
	if !strings.Contains(got, "当前阻塞：本步还不能明确区分输入材料在动作中的角色") {
		t.Fatalf("details=%q, want role-selection blocker", got)
	}
}

func processDisplayDetailText(display WorkflowProcessDisplay) string {
	var lines []string
	for _, detail := range display.Details {
		lines = append(lines, detail.Text)
	}
	return strings.Join(lines, "\n")
}

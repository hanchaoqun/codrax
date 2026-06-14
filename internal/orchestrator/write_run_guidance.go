package orchestrator

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// Terminal-state recovery guidance for the write controller. Every
// blocked / pending exit carries the same composed section so the user
// always sees WHAT SURVIVED (durable reports, attempt diffs, the
// applied ref pinned in the main repo) and WHAT TO DO NEXT (verbs and
// the yaml knob owning the exhausted budget). All inputs are typed:
// attempt records, plan ids, and a static reason-code→knob table —
// no prose is interpreted.

// blockedReasonKnobHint maps a terminal reason code to the bilingual
// "which knob / which verb" hint. Static enum table; codes are the
// same strings the progress ledger records.
func blockedReasonKnobHint(reasonCode string, zh bool) string {
	type hint struct{ zh, en string }
	table := map[string]hint{
		"budget_exhausted": {
			zh: "预算旋钮:codrax.yaml 的 pipeline_max_steps(当前 Run 步数上限);也可缩小请求范围后重试。",
			en: "Budget knob: pipeline_max_steps in codrax.yaml (per-run step ceiling); narrowing the request also helps.",
		},
		"controller_turn_budget_exhausted": {
			zh: "控制器轮次已达上限:建议缩小请求范围,或提高 codrax.yaml 的 pipeline_max_steps 后重试。",
			en: "Controller turns hit the ceiling: narrow the request, or raise pipeline_max_steps in codrax.yaml and retry.",
		},
		"verify_retry_budget_exhausted": {
			zh: "验证重试预算旋钮:codrax.yaml 的 pipeline_write_retry_budget;已应用的变更仍可经下方 ref 取回。",
			en: "Verify-retry knob: pipeline_write_retry_budget in codrax.yaml; the applied change remains recoverable via the ref below.",
		},
		"max_batches_reached": {
			zh: "批次数已达上限:请把请求拆分为更小的目标分多次执行。",
			en: "Batch cap reached: split the request into smaller goals across runs.",
		},
		"approval_denied": {
			zh: "策略拒绝了该 plan(critical 级风险)。请调整请求避开被拒路径,或修改 plan 后重新生成。",
			en: "Policy denied this plan (critical risk). Adjust the request to avoid the denied paths, or regenerate the plan.",
		},
		"pending_approval": {
			zh: "等待人工批准:当前 batch 已暂停,状态卡会展示风险、fingerprint 和 diff；批准或拒绝当前 batch 后继续。",
			en: "Awaiting manual approval: the current batch is paused; the status card shows risk, fingerprint, and diff. Approve or reject this batch to continue.",
		},
		"apply_not_allowed_in_plan_mode": {
			zh: "当前为显式 plan 模式:本次只落盘计划。要自动执行新目标,直接使用写模式 Auto Pilot；已保存 plan 可用 --write-phase=apply 作为高级入口。",
			en: "Explicit plan mode only writes the plan artifact. For a new goal, use write Auto Pilot directly; saved plans can still use --write-phase=apply as an advanced entry.",
		},
		"verify_not_allowed_in_plan_mode": {
			zh: "当前为显式 plan 模式:本次只落盘计划。验证既有 plan 时使用 --write-phase=verify 高级入口。",
			en: "Explicit plan mode only writes the plan artifact. Use the --write-phase=verify advanced entry to verify an existing plan.",
		},
	}
	h, ok := table[reasonCode]
	if !ok {
		return ""
	}
	if zh {
		return h.zh
	}
	return h.en
}

// publishBlockedRunGuidance composes the recovery section for a
// terminal write-workflow state and appends it to Mutable.Result so
// both the single-shot CLI (which prints Result) and the REPL surface
// it. Idempotent per call site; empty guidance appends nothing.
func (o *Orchestrator) publishBlockedRunGuidance(run *types.WriteWorkflowRun, reasonCode string) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || run == nil {
		return
	}
	zh := isChineseLang(o.busCtx.Language)
	var b strings.Builder
	if zh {
		b.WriteString("## 工作流已停止 — 恢复指引\n\n")
	} else {
		b.WriteString("## Workflow stopped — recovery guide\n\n")
	}
	if hint := blockedReasonKnobHint(reasonCode, zh); hint != "" {
		b.WriteString("- " + hint + "\n")
	}
	reportDir := strings.TrimSpace(o.reportDir)
	for _, batch := range run.Batches {
		st := writeflow.DeriveBatchAttemptState(batch)
		applied := false
		for _, a := range batch.Attempts {
			if a.Kind == "apply" && a.Status == "applied" {
				applied = true
			}
		}
		if st.PlanID == "" && st.ReportID == "" && !applied {
			continue
		}
		if zh {
			fmt.Fprintf(&b, "- 批次 %s:", batch.ID)
		} else {
			fmt.Fprintf(&b, "- batch %s:", batch.ID)
		}
		parts := []string{}
		if st.PlanID != "" {
			parts = append(parts, map[bool]string{true: "plan `" + st.PlanID + "`(/plan show 可审阅)", false: "plan `" + st.PlanID + "` (review via /plan show)"}[zh])
		}
		if st.ReportID != "" {
			loc := st.ReportID
			if reportDir != "" {
				loc = filepath.Join(reportDir, st.ReportID)
			}
			parts = append(parts, map[bool]string{true: "验证报告 `" + loc + "`", false: "verify report `" + loc + "`"}[zh])
		}
		if st.LatestVerifyArtifactRef != "" {
			loc := st.LatestVerifyArtifactRef
			if reportDir != "" {
				loc = filepath.Join(reportDir, st.LatestVerifyArtifactRef)
			}
			parts = append(parts, map[bool]string{true: "尝试补丁 `" + loc + "`", false: "attempt diff `" + loc + "`"}[zh])
		}
		if applied && st.PlanID != "" {
			ref := "refs/codrax/applied/" + st.PlanID
			parts = append(parts, map[bool]string{
				true:  "已应用提交固定在主仓 `" + ref + "`,可执行 `git cherry-pick " + ref + "` 取回",
				false: "applied commit pinned at `" + ref + "`; recover with `git cherry-pick " + ref + "`",
			}[zh])
		}
		b.WriteString(strings.Join(parts, ";"))
		b.WriteString("\n")
	}
	if run.RunID != "" {
		if zh {
			fmt.Fprintf(&b, "- 工作流详情:REPL 中 `/workflow show %s`\n", run.RunID)
		} else {
			fmt.Fprintf(&b, "- workflow details: `/workflow show %s` in the REPL\n", run.RunID)
		}
	}
	guidance := strings.TrimRight(b.String(), "\n")
	existing := strings.TrimSpace(o.busCtx.Mutable.Result())
	if existing != "" {
		o.busCtx.Mutable.SetResultPlain(existing + "\n\n" + guidance)
		return
	}
	o.busCtx.Mutable.SetResultPlain(guidance)
}

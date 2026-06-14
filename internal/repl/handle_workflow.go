package repl

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

type writeWorkflowPlanBinding struct {
	RunID   string
	BatchID string
	PlanID  string
	Path    string
}

func (r *REPL) renderWriteWorkflowShow(rest string) bool {
	if r.writeWorkflowRunStore == nil {
		return false
	}
	runID := workflowShowID(rest)
	var (
		run *types.WriteWorkflowRun
		err error
	)
	if runID != "" {
		run, err = r.writeWorkflowRunStore.Load(runID)
		if err != nil {
			r.errorf("workflow show: %v\n", err)
			return true
		}
	} else {
		run, err = r.writeWorkflowRunStore.FindActiveRun()
		if err != nil {
			r.errorf("workflow show: %v\n", err)
			return true
		}
		if run == nil {
			return false
		}
	}
	r.renderBordered(writeWorkflowRunMarkdown(r.language, *run, r.writeWorkflowActivePlan(run)))
	return true
}

func workflowShowID(rest string) string {
	fields := strings.Fields(rest)
	if len(fields) >= 2 && fields[0] == "show" {
		return fields[1]
	}
	return ""
}

func workflowCommandID(rest, command string) string {
	fields := strings.Fields(rest)
	if len(fields) >= 2 && fields[0] == command {
		return fields[1]
	}
	return ""
}

func (r *REPL) handleWriteWorkflowList() bool {
	if r.writeWorkflowRunStore == nil {
		return false
	}
	infos, err := r.writeWorkflowRunStore.List()
	if err != nil {
		r.errorf("workflow list: %v\n", err)
		return true
	}
	if len(infos) == 0 {
		r.info(writeWorkflowNoRunsMsg(r.language))
		return true
	}
	r.renderBordered(writeWorkflowListMarkdown(r.language, infos))
	return true
}

func (r *REPL) handleWriteWorkflowResume(rest string) bool {
	if r.writeWorkflowRunStore == nil {
		return false
	}
	runID := workflowCommandID(rest, "resume")
	var (
		run *types.WriteWorkflowRun
		err error
	)
	if runID != "" {
		run, err = r.writeWorkflowRunStore.Load(runID)
	} else {
		run, err = r.writeWorkflowRunStore.FindActiveRun()
	}
	if err != nil {
		r.errorf("workflow resume: %v\n", err)
		return true
	}
	if run == nil {
		r.info(workflowNoActiveMsg(r.language))
		return true
	}
	if run.Status == types.WriteWorkflowRunComplete || run.Status == types.WriteWorkflowRunBlocked {
		r.info(writeWorkflowCannotResumeMsg(r.language, run.RunID, string(run.Status)))
		return true
	}
	batchID := resumableWriteWorkflowBatchID(*run)
	if batchID == "" {
		r.info(writeWorkflowCannotResumeMsg(r.language, run.RunID, "no resumable batch"))
		return true
	}
	now := time.Now()
	run.Status = types.WriteWorkflowRunInProgress
	run.ActiveBatchID = batchID
	run.UpdatedAt = now
	run.ProgressLedger = append(run.ProgressLedger, types.WriteWorkflowProgress{
		BatchID:    batchID,
		Stage:      "repl",
		Status:     string(types.WriteWorkflowRunInProgress),
		ReasonCode: "resumed",
		Message:    "workflow selected for next write-mode continuation",
		At:         now,
	})
	if _, err := r.writeWorkflowRunStore.Save(run); err != nil {
		r.errorf("workflow resume: %v\n", err)
		return true
	}
	r.info(writeWorkflowResumedMsg(r.language, run.RunID, batchID))
	return true
}

func resumableWriteWorkflowBatchID(run types.WriteWorkflowRun) string {
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID != "" {
		for _, batch := range run.Batches {
			if batch.ID == activeID && batch.Status != types.WriteWorkflowBatchComplete && batch.Status != types.WriteWorkflowBatchBlocked {
				return batch.ID
			}
		}
	}
	for _, batch := range run.Batches {
		if strings.TrimSpace(batch.ID) == "" {
			continue
		}
		if batch.Status == types.WriteWorkflowBatchComplete || batch.Status == types.WriteWorkflowBatchBlocked {
			continue
		}
		return batch.ID
	}
	return ""
}

func (r *REPL) handleWriteWorkflowClear(rest string) bool {
	if r.writeWorkflowRunStore == nil {
		return false
	}
	runID := workflowCommandID(rest, "clear")
	if runID == "" {
		run, err := r.writeWorkflowRunStore.FindActiveRun()
		if err != nil {
			r.errorf("workflow clear: %v\n", err)
			return true
		}
		if run == nil {
			r.info(workflowNoActiveMsg(r.language))
			return true
		}
		runID = run.RunID
	}
	if err := r.writeWorkflowRunStore.Clear(runID); err != nil {
		r.errorf("workflow clear: %v\n", err)
		return true
	}
	r.info(writeWorkflowClearedMsg(r.language, runID))
	return true
}

func (r *REPL) activeWriteWorkflowPlanBinding() (writeWorkflowPlanBinding, bool) {
	if r.writeWorkflowRunStore == nil || r.planStore == nil {
		return writeWorkflowPlanBinding{}, false
	}
	run, err := r.writeWorkflowRunStore.FindActiveRun()
	if err != nil {
		logging.Warning("[repl/workflow] find active write workflow failed: %v", err)
		return writeWorkflowPlanBinding{}, false
	}
	if run == nil {
		return writeWorkflowPlanBinding{}, false
	}
	batch, ok := activeWriteWorkflowBatch(*run)
	if !ok || !writeWorkflowBatchAwaitsPlanDecision(batch) || strings.TrimSpace(batch.PlanID) == "" {
		return writeWorkflowPlanBinding{}, false
	}
	planID := strings.TrimSpace(batch.PlanID)
	if _, err := r.planStore.Load(planID); err != nil {
		logging.Warning("[repl/workflow] active workflow plan %s missing: %v", planID, err)
		return writeWorkflowPlanBinding{}, false
	}
	return writeWorkflowPlanBinding{
		RunID:   run.RunID,
		BatchID: batch.ID,
		PlanID:  planID,
		Path:    filepath.Join(r.planStore.PlanDir(), planID+".json"),
	}, true
}

func writeWorkflowBatchAwaitsPlanDecision(batch types.WriteWorkflowBatch) bool {
	switch batch.Status {
	case types.WriteWorkflowBatchPendingApproval, types.WriteWorkflowBatchPlanned:
		return true
	default:
		return false
	}
}

func (r *REPL) writeWorkflowActivePlan(run *types.WriteWorkflowRun) *types.ChangePlan {
	if r.planStore == nil || run == nil {
		return nil
	}
	batch, ok := activeWriteWorkflowBatch(*run)
	if !ok || strings.TrimSpace(batch.PlanID) == "" {
		return nil
	}
	plan, err := r.planStore.Load(batch.PlanID)
	if err != nil {
		return nil
	}
	return plan
}

func activeWriteWorkflowBatch(run types.WriteWorkflowRun) (types.WriteWorkflowBatch, bool) {
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" && len(run.Batches) > 0 {
		return run.Batches[0], true
	}
	for _, batch := range run.Batches {
		if batch.ID == activeID {
			return batch, true
		}
	}
	return types.WriteWorkflowBatch{}, false
}

func (r *REPL) bindActiveWriteWorkflowPlanIfIdle(command string) bool {
	if strings.TrimSpace(r.pendingPlanPath) != "" {
		return false
	}
	binding, ok := r.activeWriteWorkflowPlanBinding()
	if !ok {
		return false
	}
	r.pendingPlanPath = binding.Path
	r.info(writeWorkflowBoundPlanMsg(r.language, command, binding.RunID, binding.BatchID, binding.PlanID))
	return true
}

func (r *REPL) markWriteWorkflowBatchAfterApprove(planID string, cleanSuccess bool, lastErr string) {
	if !cleanSuccess {
		if strings.TrimSpace(lastErr) != "" {
			r.updateActiveWriteWorkflowBatchForPlan(planID, types.WriteWorkflowBatchReadyToPlan, "approve_failed", lastErr)
		}
		return
	}
	r.updateActiveWriteWorkflowBatchForPlan(planID, types.WriteWorkflowBatchComplete, "approved_batch_complete", "approved plan applied and verified")
}

func (r *REPL) markWriteWorkflowBatchRejected(planID, reason string) {
	msg := strings.TrimSpace(reason)
	if msg == "" {
		msg = "operator rejected active batch plan"
	}
	r.updateActiveWriteWorkflowBatchForPlan(planID, types.WriteWorkflowBatchBlocked, "rejected", msg)
}

func (r *REPL) updateActiveWriteWorkflowBatchForPlan(planID string, status types.WriteWorkflowBatchStatus, reasonCode, message string) {
	if r.writeWorkflowRunStore == nil || strings.TrimSpace(planID) == "" {
		return
	}
	run, err := r.writeWorkflowRunStore.FindActiveRun()
	if err != nil || run == nil {
		if err != nil {
			logging.Warning("[repl/workflow] update active write workflow failed: %v", err)
		}
		return
	}
	activeID := strings.TrimSpace(run.ActiveBatchID)
	if activeID == "" {
		return
	}
	updated := false
	for i := range run.Batches {
		batch := &run.Batches[i]
		if batch.ID != activeID || strings.TrimSpace(batch.PlanID) != strings.TrimSpace(planID) {
			continue
		}
		batch.Status = status
		updated = true
		break
	}
	if !updated {
		return
	}
	if run.Status != types.WriteWorkflowRunComplete {
		run.Status = types.WriteWorkflowRunInProgress
	}
	run.ProgressLedger = append(run.ProgressLedger, types.WriteWorkflowProgress{
		BatchID:    activeID,
		Stage:      "repl",
		Status:     string(status),
		ReasonCode: strings.TrimSpace(reasonCode),
		Message:    strings.TrimSpace(message),
	})
	if _, err := r.writeWorkflowRunStore.Save(run); err != nil {
		logging.Warning("[repl/workflow] save active write workflow failed: %v", err)
	}
}

func writeWorkflowRunMarkdown(lang string, run types.WriteWorkflowRun, plan *types.ChangePlan) string {
	run = types.NormalizeWriteWorkflowRun(run)
	batch, hasBatch := activeWriteWorkflowBatch(run)
	var b strings.Builder
	if isZh(lang) {
		fmt.Fprintf(&b, "Write workflow `%s`\n\n", run.RunID)
		fmt.Fprintf(&b, "- 状态: `%s`\n", firstNonEmptyString(string(run.Status), "unknown"))
		fmt.Fprintf(&b, "- active batch: `%s`\n", firstNonEmptyString(run.ActiveBatchID, "none"))
		fmt.Fprintf(&b, "- 目标: %s\n", firstNonEmptyString(run.Goal, "未记录"))
		writeWorkflowBudgetLine(&b, lang, run.Budget)
		writeWorkflowBatchLines(&b, lang, run, batch, hasBatch)
		writeWorkflowApprovalLines(&b, lang, batch, hasBatch, plan)
		writeWorkflowContextLines(&b, lang, run, batch, hasBatch)
		writeWorkflowProgressLines(&b, lang, run)
		for _, line := range writeWorkflowNextActionLines(lang, batch, hasBatch) {
			b.WriteString("\n" + line)
		}
		return strings.TrimSpace(b.String())
	}
	fmt.Fprintf(&b, "Write workflow `%s`\n\n", run.RunID)
	fmt.Fprintf(&b, "- Status: `%s`\n", firstNonEmptyString(string(run.Status), "unknown"))
	fmt.Fprintf(&b, "- Active batch: `%s`\n", firstNonEmptyString(run.ActiveBatchID, "none"))
	fmt.Fprintf(&b, "- Goal: %s\n", firstNonEmptyString(run.Goal, "not recorded"))
	writeWorkflowBudgetLine(&b, lang, run.Budget)
	writeWorkflowBatchLines(&b, lang, run, batch, hasBatch)
	writeWorkflowApprovalLines(&b, lang, batch, hasBatch, plan)
	writeWorkflowContextLines(&b, lang, run, batch, hasBatch)
	writeWorkflowProgressLines(&b, lang, run)
	for _, line := range writeWorkflowNextActionLines(lang, batch, hasBatch) {
		b.WriteString("\n" + line)
	}
	return strings.TrimSpace(b.String())
}

func writeWorkflowNextActionLines(lang string, batch types.WriteWorkflowBatch, hasBatch bool) []string {
	advanced := "/workflow list"
	if hasBatch {
		switch batch.Status {
		case types.WriteWorkflowBatchPendingApproval:
			return writeNextActionCardLines(lang, writeActionNeedsApproval, "/approve · /reject <reason> · /workflow list")
		case types.WriteWorkflowBatchComplete:
			return writeNextActionCardLines(lang, writeActionApplied, "/merge · /verify · /workflow list")
		case types.WriteWorkflowBatchBlocked:
			return writeNextActionCardLines(lang, writeActionVerifyFailed, "/workflow show · /reject <reason> · /workflow list")
		case types.WriteWorkflowBatchPlanned, types.WriteWorkflowBatchReadyToPlan:
			return writeNextActionCardLines(lang, writeActionPlanReady, "/approve · /reject <reason> · /workflow list")
		case types.WriteWorkflowBatchApplying, types.WriteWorkflowBatchVerifying, types.WriteWorkflowBatchNeedsExploration:
			if isZh(lang) {
				return []string{
					"  状态：workflow 正在推进；暂不需要用户操作。",
					"  下一步：等待当前 batch 完成、暂停审批或写入失败证据。",
					"  高级入口：" + advanced,
				}
			}
			return []string{
				"  Status: workflow is running; no user action is needed yet.",
				"  Next: wait for the current batch to finish, pause for approval, or record failure evidence.",
				"  Advanced: " + advanced,
			}
		}
	}
	if isZh(lang) {
		return []string{
			"  状态：没有 active batch。",
			"  下一步：查看 workflow 列表或重新发起写目标。",
			"  高级入口：" + advanced,
		}
	}
	return []string{
		"  Status: no active batch.",
		"  Next: inspect saved workflows or start a new write goal.",
		"  Advanced: " + advanced,
	}
}

func writeWorkflowBudgetLine(b *strings.Builder, lang string, budget types.WriteWorkflowBudget) {
	maxBatches := budget.MaxBatches
	remaining := 0
	if maxBatches > 0 && budget.BatchesUsed < maxBatches {
		remaining = maxBatches - budget.BatchesUsed
	}
	if isZh(lang) {
		fmt.Fprintf(b, "- 预算: batches %d/%d, 剩余 %d; exploration %d/%d\n",
			budget.BatchesUsed, maxBatches, remaining, budget.ExplorationRoundsUsed, budget.MaxExplorationRounds)
		return
	}
	fmt.Fprintf(b, "- Budget: batches %d/%d, remaining %d; exploration %d/%d\n",
		budget.BatchesUsed, maxBatches, remaining, budget.ExplorationRoundsUsed, budget.MaxExplorationRounds)
}

func writeWorkflowBatchLines(b *strings.Builder, lang string, run types.WriteWorkflowRun, active types.WriteWorkflowBatch, hasActive bool) {
	if len(run.Batches) == 0 {
		return
	}
	if isZh(lang) {
		b.WriteString("\nBatches:\n")
	} else {
		b.WriteString("\nBatches:\n")
	}
	for _, batch := range run.Batches {
		marker := " "
		if hasActive && batch.ID == active.ID {
			marker = "*"
		}
		planPart := ""
		if strings.TrimSpace(batch.PlanID) != "" {
			planPart = fmt.Sprintf(" plan `%s`", batch.PlanID)
		}
		refPart := ""
		if strings.TrimSpace(batch.ApplyRef) != "" {
			refPart += fmt.Sprintf(" apply `%s`", batch.ApplyRef)
		}
		if strings.TrimSpace(batch.VerifyRef) != "" {
			refPart += fmt.Sprintf(" verify `%s`", batch.VerifyRef)
		}
		goal := strings.TrimSpace(batch.Goal)
		if goal != "" {
			goal = " - " + goal
		}
		fmt.Fprintf(b, "%s `%s` `%s`%s%s%s\n", marker, batch.ID, batch.Status, planPart, refPart, goal)
	}
}

func writeWorkflowApprovalLines(b *strings.Builder, lang string, batch types.WriteWorkflowBatch, hasBatch bool, plan *types.ChangePlan) {
	if !hasBatch || strings.TrimSpace(batch.PlanID) == "" {
		return
	}
	if isZh(lang) {
		fmt.Fprintf(b, "\n审批:\n- active plan: `%s`\n", batch.PlanID)
	} else {
		fmt.Fprintf(b, "\nApproval:\n- Active plan: `%s`\n", batch.PlanID)
	}
	if plan == nil {
		if isZh(lang) {
			b.WriteString("- plan 文件暂不可读\n")
		} else {
			b.WriteString("- Plan file is not readable yet\n")
		}
		return
	}
	if plan.Approval == nil {
		if isZh(lang) {
			fmt.Fprintf(b, "- plan status: `%s`; approval record: none\n", firstNonEmptyString(plan.Status, "unknown"))
		} else {
			fmt.Fprintf(b, "- Plan status: `%s`; approval record: none\n", firstNonEmptyString(plan.Status, "unknown"))
		}
		return
	}
	fmt.Fprintf(b, "- policy=`%s` risk=`%s` action=`%s` user=`%s` reason=`%s` fingerprint=`%s`\n",
		firstNonEmptyString(plan.Approval.Policy, "unknown"),
		firstNonEmptyString(plan.Approval.RiskLevel, "unknown"),
		firstNonEmptyString(plan.Approval.Action, "unknown"),
		firstNonEmptyString(plan.Approval.UserDecision, "pending"),
		firstNonEmptyString(plan.Approval.ReasonCode, "none"),
		firstNonEmptyString(plan.Approval.PlanFingerprint, "none"))
}

func writeWorkflowContextLines(b *strings.Builder, lang string, run types.WriteWorkflowRun, batch types.WriteWorkflowBatch, hasBatch bool) {
	if !hasBatch {
		return
	}
	packs := writeWorkflowPacksForBatch(run, batch.ID)
	if len(packs) == 0 {
		return
	}
	counts := map[types.WriteContextPriority]int{}
	var top []types.WriteContextItem
	for _, pack := range packs {
		view := pack.View(types.WriteConsumerController, 0)
		for _, item := range view.Items {
			counts[item.Priority]++
			if len(top) < 5 {
				top = append(top, item)
			}
		}
	}
	if isZh(lang) {
		fmt.Fprintf(b, "\nHandoff context: %d pack(s), P0=%d P1=%d P2=%d P3=%d\n",
			len(packs), counts[types.WriteContextP0], counts[types.WriteContextP1], counts[types.WriteContextP2], counts[types.WriteContextP3])
	} else {
		fmt.Fprintf(b, "\nHandoff context: %d pack(s), P0=%d P1=%d P2=%d P3=%d\n",
			len(packs), counts[types.WriteContextP0], counts[types.WriteContextP1], counts[types.WriteContextP2], counts[types.WriteContextP3])
	}
	for _, item := range top {
		text := strings.TrimSpace(item.Text)
		if text == "" && item.EvidenceRef != nil {
			text = fmt.Sprintf("%s:%d", item.EvidenceRef.Source, item.EvidenceRef.LineStart)
		}
		fmt.Fprintf(b, "- `%s` `%s`: %s\n", item.Priority, item.Kind, text)
	}
}

func writeWorkflowPacksForBatch(run types.WriteWorkflowRun, batchID string) []types.WriteContextPack {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return nil
	}
	batch, _ := activeWriteWorkflowBatch(run)
	ids := map[string]struct{}{batchID: {}}
	if strings.TrimSpace(batch.PlanID) != "" {
		ids[strings.TrimSpace(batch.PlanID)] = struct{}{}
	}
	for _, id := range batch.ContextPackIDs {
		if strings.TrimSpace(id) != "" {
			ids[strings.TrimSpace(id)] = struct{}{}
		}
	}
	out := make([]types.WriteContextPack, 0, len(run.ContextPacks))
	for _, pack := range run.ContextPacks {
		if pack.BatchID == "" {
			out = append(out, pack)
			continue
		}
		if _, ok := ids[pack.BatchID]; ok {
			out = append(out, pack)
			continue
		}
		if _, ok := ids[pack.PackID]; ok {
			out = append(out, pack)
		}
	}
	return out
}

func writeWorkflowProgressLines(b *strings.Builder, lang string, run types.WriteWorkflowRun) {
	if len(run.ProgressLedger) == 0 {
		return
	}
	start := len(run.ProgressLedger) - 4
	if start < 0 {
		start = 0
	}
	if isZh(lang) {
		b.WriteString("\n最近进展:\n")
	} else {
		b.WriteString("\nRecent progress:\n")
	}
	for _, item := range run.ProgressLedger[start:] {
		msg := strings.TrimSpace(item.Message)
		if msg != "" {
			msg = " - " + msg
		}
		fmt.Fprintf(b, "- `%s` `%s` `%s`%s\n",
			firstNonEmptyString(item.BatchID, "-"),
			firstNonEmptyString(item.Stage, "-"),
			firstNonEmptyString(item.ReasonCode, firstNonEmptyString(item.Status, "-")),
			msg)
	}
}

func writeWorkflowListMarkdown(lang string, infos []WorkflowRunInfo) string {
	var b strings.Builder
	if isZh(lang) {
		b.WriteString("Write workflows\n\n")
	} else {
		b.WriteString("Write workflows\n\n")
	}
	limit := len(infos)
	if limit > 10 {
		limit = 10
	}
	for i := 0; i < limit; i++ {
		info := infos[i]
		fmt.Fprintf(&b, "- `%s` status=`%s` active=`%s` batches=%d packs=%d\n",
			info.ID,
			firstNonEmptyString(info.Status, "unknown"),
			firstNonEmptyString(info.ActiveBatchID, "none"),
			info.Batches,
			info.ContextPacks)
	}
	return strings.TrimSpace(b.String())
}

func writeWorkflowNoRunsMsg(lang string) string {
	if isZh(lang) {
		return "当前没有已保存的 write workflow。"
	}
	return "No saved write workflows."
}

func writeWorkflowResumedMsg(lang, runID, batchID string) string {
	if isZh(lang) {
		return fmt.Sprintf("write workflow `%s` 已恢复为 active，当前 batch `%s`。下一次写模式会继续它。", runID, batchID)
	}
	return fmt.Sprintf("Write workflow `%s` resumed as active at batch `%s`. The next write-mode run will continue it.", runID, batchID)
}

func writeWorkflowClearedMsg(lang, runID string) string {
	if isZh(lang) {
		return fmt.Sprintf("write workflow `%s` 已清理。", runID)
	}
	return fmt.Sprintf("Write workflow `%s` cleared.", runID)
}

func writeWorkflowCannotResumeMsg(lang, runID, status string) string {
	if isZh(lang) {
		return fmt.Sprintf("write workflow `%s` 不能恢复：%s。", runID, status)
	}
	return fmt.Sprintf("Write workflow `%s` cannot be resumed: %s.", runID, status)
}

func writeWorkflowBoundPlanMsg(lang, command, runID, batchID, planID string) string {
	if isZh(lang) {
		return fmt.Sprintf("%s 已绑定 active write workflow `%s` batch `%s` 的 plan `%s`。", command, runID, batchID, planID)
	}
	return fmt.Sprintf("%s bound to active write workflow `%s` batch `%s` plan `%s`.", command, runID, batchID, planID)
}

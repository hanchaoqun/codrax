package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// writeControllerEvaluator drives the optional outer write workflow controller.
// Its only authoritative output is emit_write_workflow_decision; prose is
// ignored and never used for routing.
type writeControllerEvaluator struct {
	emitSeen bool
}

func (e *writeControllerEvaluator) BuildInitialInstruction(ctx *types.AgentContext, _ *skill.Config) string {
	e.emitSeen = false
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	ctx.Mutable.ResetWriteWorkflowDecisionJSON()
	var sections []string
	if task := renderWriteControllerTaskSection(ctx); task != "" {
		sections = append(sections, task)
	}
	if run := renderWriteControllerRunSection(ctx); run != "" {
		sections = append(sections, run)
	}
	if artifacts := renderWriteControllerArtifactSection(ctx); artifacts != "" {
		sections = append(sections, artifacts)
	}
	if pack := buildWriteContextPackPromptSection(ctx, types.WriteConsumerController, "Priority write context pack", 16); pack != "" {
		sections = append(sections, pack)
	}
	if len(sections) == 0 {
		return "## Write workflow controller\n\nNo typed write artifacts are available yet. Emit a conservative block decision with reason_code=no_typed_context."
	}
	sections = append(sections, "## Controller contract\n\nEmit exactly one emit_write_workflow_decision call. The system routes only by the typed action enum and validated payload; explanatory prose is trace-only.")
	return "\n\n" + strings.Join(sections, "\n\n")
}

func (e *writeControllerEvaluator) ShouldStop(_ llm.Response, _ int) bool { return e.emitSeen }

func (e *writeControllerEvaluator) Observe(_ *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.Phase != PhaseMidLoop || obs.LastToolResult == nil || !obs.LastToolResult.Success {
		return LoopSignal{}
	}
	if obs.LastToolResult.ToolName == "emit_write_workflow_decision" {
		e.emitSeen = true
		return LoopSignal{StopRequested: true, StopReason: "emit_write_workflow_decision emitted"}
	}
	return LoopSignal{}
}

func (e *writeControllerEvaluator) ParseOutput(ctx *types.AgentContext, _ []llm.Message, toolResults []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {
	if ctx == nil || ctx.Mutable == nil {
		return &StageOutput{Error: "write_controller requires a writable context; the caller did not provide one"}, nil
	}
	raw := ctx.Mutable.WriteWorkflowDecisionJSON()
	if len(raw) > 0 {
		var decision writeflow.WriteWorkflowDecision
		if err := json.Unmarshal(raw, &decision); err != nil {
			return &StageOutput{Error: "write_controller stored invalid decision JSON: " + err.Error()}, nil
		}
		decision = writeflow.NormalizeWriteWorkflowDecision(decision)
		if errs := writeflow.ValidateWriteWorkflowDecision(decision); len(errs) > 0 {
			return &StageOutput{Error: "write_controller stored invalid decision: " + strings.Join(errs, "; ")}, nil
		}
		return &StageOutput{
			Data:        append(json.RawMessage(nil), raw...),
			StageReport: renderWriteWorkflowDecisionStageReport(decision),
		}, nil
	}
	hasEmitAttempt := false
	var rejections []string
	for _, r := range toolResults {
		if r.ToolName != "emit_write_workflow_decision" {
			continue
		}
		hasEmitAttempt = true
		if !r.Success {
			rejections = append(rejections, r.Summary)
		}
	}
	if !hasEmitAttempt {
		return &StageOutput{Error: "write_controller did not call emit_write_workflow_decision within the ReAct loop"}, nil
	}
	return &StageOutput{Error: "write_controller emit rejected: " + strings.Join(rejections, "; ")}, nil
}

func (e *writeControllerEvaluator) DetermineMissingPiece(_ *types.AgentContext, _ *StageOutput) types.MissingPiece {
	return types.MissingFacts
}

type writeController struct {
	base *BaseAgent
}

func (a *writeController) Name() types.AgentName { return types.AgentWriteController }

func (a *writeController) Execute(ctx *types.AgentContext, sk *skill.Config) (*StageOutput, error) {
	return a.base.Execute(ctx, sk)
}

func NewWriteControllerAgent(deps *Dependencies) Agent {
	d := *deps
	if d.MaxIterations == 0 {
		d.MaxIterations = 4
	}
	eval := &writeControllerEvaluator{}
	base := NewBaseAgent(types.AgentWriteController, &d, eval)
	return &writeController{base: base}
}

func renderWriteControllerTaskSection(ctx *types.AgentContext) string {
	ir := ctx.Mutable.WriteAnalysisIR()
	if ir == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Typed write task\n\n")
	fmt.Fprintf(&b, "- summary: %s\n", ir.Request.Task.Summary)
	fmt.Fprintf(&b, "- kind: %s\n", ir.Request.Task.Kind)
	fmt.Fprintf(&b, "- scope: %s\n", ir.Request.Task.Scope)
	fmt.Fprintf(&b, "- risk: %s\n", ir.Request.Risk.Overall)
	if len(ir.Request.ScopeAnchors) > 0 {
		fmt.Fprintf(&b, "- scope_anchors: %s\n", strings.Join(ir.Request.ScopeAnchors, ", "))
	}
	if len(ir.Request.ExpectedOutcomes) > 0 {
		fmt.Fprintf(&b, "- expected_outcomes: %s\n", strings.Join(ir.Request.ExpectedOutcomes, " | "))
	}
	return strings.TrimSpace(b.String())
}

func renderWriteControllerRunSection(ctx *types.AgentContext) string {
	run := ctx.Mutable.WriteWorkflowRun()
	if run == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Workflow run state\n\n")
	fmt.Fprintf(&b, "- run_id: %s\n", run.RunID)
	fmt.Fprintf(&b, "- status: %s\n", run.Status)
	if run.ActiveBatchID != "" {
		fmt.Fprintf(&b, "- active_batch: %s\n", run.ActiveBatchID)
	}
	if run.Budget.MaxBatches > 0 || run.Budget.MaxExplorationRounds > 0 {
		fmt.Fprintf(&b, "- budget: batches %d/%d, exploration rounds %d/%d per batch\n",
			run.Budget.BatchesUsed, run.Budget.MaxBatches,
			run.Budget.ExplorationRoundsUsed, run.Budget.MaxExplorationRounds)
	}
	if len(run.Batches) > 0 {
		b.WriteString("- batches:\n")
		for i, batch := range run.Batches {
			if i >= 8 {
				fmt.Fprintf(&b, "  - ... +%d more batch(es)\n", len(run.Batches)-i)
				break
			}
			fmt.Fprintf(&b, "  - %s status=%s goal=%s plan=%s\n", batch.ID, batch.Status, limitWriteControllerText(batch.Goal, 120), batch.PlanID)
		}
	}
	if len(run.ProgressLedger) > 0 {
		last := run.ProgressLedger[len(run.ProgressLedger)-1]
		fmt.Fprintf(&b, "- last_progress: batch=%s status=%s reason=%s\n", last.BatchID, last.Status, last.ReasonCode)
	}
	return strings.TrimSpace(b.String())
}

func renderWriteControllerArtifactSection(ctx *types.AgentContext) string {
	var b strings.Builder
	b.WriteString("## Typed write artifacts\n\n")
	wrote := false
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		wrote = true
		fmt.Fprintf(&b, "- change_plan: id=%s status=%s targets=%d\n", plan.ID, plan.Status, len(plan.TargetPaths))
		if plan.Approval != nil {
			fmt.Fprintf(&b, "- approval: action=%s risk=%s decision=%s reason=%s\n",
				plan.Approval.Action, plan.Approval.RiskLevel, plan.Approval.UserDecision, plan.Approval.ReasonCode)
		}
	}
	if report := ctx.Mutable.ChangeReport(); report != nil {
		wrote = true
		fmt.Fprintf(&b, "- change_report: plan_id=%s passed=%t build_failed=%t\n", report.PlanID, report.Passed, report.BuildFailed)
		if strings.TrimSpace(report.FailureSummary) != "" {
			fmt.Fprintf(&b, "- verify_failure_summary: %s\n", limitWriteControllerText(report.FailureSummary, 220))
		}
	}
	if !wrote {
		return ""
	}
	return strings.TrimSpace(b.String())
}

func renderWriteWorkflowDecisionStageReport(decision writeflow.WriteWorkflowDecision) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Action: %s\n", decision.Action)
	if decision.ReasonCode != "" {
		fmt.Fprintf(&b, "Reason code: %s\n", decision.ReasonCode)
	}
	if decision.Batch != nil {
		fmt.Fprintf(&b, "Batch: %s (%s)\n", decision.Batch.ID, decision.Batch.Goal)
	}
	if decision.ExplorationRequest != nil {
		fmt.Fprintf(&b, "Explore: %s (%d question(s), %d candidate path(s))\n",
			decision.ExplorationRequest.BatchID,
			len(decision.ExplorationRequest.ExplorationQuestions),
			len(decision.ExplorationRequest.CandidatePaths))
	}
	return strings.TrimSpace(b.String())
}

func limitWriteControllerText(raw string, max int) string {
	raw = strings.Join(strings.Fields(raw), " ")
	if max <= 0 || len([]rune(raw)) <= max {
		return raw
	}
	runes := []rune(raw)
	return string(runes[:max]) + "..."
}

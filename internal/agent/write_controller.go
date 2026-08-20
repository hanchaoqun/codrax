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
	sections = append(sections, renderWriteControllerActionContract(ctx))
	sections = append(sections, "## Controller contract\n\nEmit exactly one emit_write_workflow_decision call. The system routes only by the typed action enum and validated payload; explanatory prose is trace-only.")
	return "\n\n" + strings.Join(sections, "\n\n")
}

// renderWriteControllerActionContract keeps the prose view of the controller
// action surface on the same typed authority as the projected tool schema.
// A static superset here is actively misleading: ModePlan deliberately hides
// apply/verify from the JSON schema, and presenting those names in prose makes
// a structurally impossible action look available to the model.
func renderWriteControllerActionContract(ctx *types.AgentContext) string {
	mode := types.ModeRead
	if ctx != nil {
		mode = ctx.Mode.Normalize()
	}
	var run *types.WriteWorkflowRun
	if ctx != nil && ctx.Mutable != nil {
		run = ctx.Mutable.WriteWorkflowRun()
	}
	actions := writeflow.WorkflowActionsForRun(mode, run)
	names := make([]string, 0, len(actions))
	for _, action := range actions {
		names = append(names, string(action))
	}
	return fmt.Sprintf("## Available controller actions\n\n- mode: %s\n- action enum: %s\n- Choose exactly one action from this projected enum. Actions absent here are unavailable in this dispatch.", mode, strings.Join(names, ", "))
}

func (e *writeControllerEvaluator) ShouldStop(_ llm.Response, _ int) bool { return e.emitSeen }

func (e *writeControllerEvaluator) Observe(_ *types.AgentContext, obs LoopObservation) LoopSignal {
	if obs.Phase == PhaseSoftStop && len(obs.Response.ToolCalls) == 0 {
		key := "write-controller.required-tool.no-tool"
		boundary := "The completed response did not use the required function-calling channel."
		if obs.Response.StopReason == "length" {
			key = "write-controller.required-tool.length-no-tool"
			boundary = "The completed response reached the output-token limit before producing the required tool call."
		}
		return LoopSignal{
			HintRequested:  true,
			HintKey:        key,
			Hint:           boundary + " Use only the current typed workflow state and the available action enum. Call emit_write_workflow_decision exactly once with one valid action; do not repeat analysis, reconstruct the discarded draft, or emit explanatory prose.",
			Progress:       true,
			BypassThrottle: true,
			BypassBudget:   true,
		}
	}
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
	expectedOutcomes := ir.Request.ExpectedOutcomes
	behaviorContracts := ir.Request.BehaviorContracts
	if plan := ctx.Mutable.ChangePlan(); plan != nil && plan.BehaviorContractGeneration == types.WriteBehaviorContractGenerationPlanAcceptanceRebase {
		expectedOutcomes = plan.AcceptanceTests
		behaviorContracts = types.ChangePlanVerificationBehaviorContracts(plan)
		fmt.Fprintf(&b, "- behavior_contract_generation: %s\n", plan.BehaviorContractGeneration)
	}
	if len(expectedOutcomes) > 0 {
		fmt.Fprintf(&b, "- analyzer_proposed_outcomes (planning_only; request/preserve constraints/source/tests take precedence): %s\n", strings.Join(expectedOutcomes, " | "))
	}
	if len(behaviorContracts) > 0 {
		b.WriteString("- behavior_contracts:\n")
		for _, c := range behaviorContracts {
			fmt.Fprintf(&b, "  - id=%s kind=%s operator=%s expected=%s", c.ID, c.Kind, c.Operator, c.Expected)
			if c.Polarity != "" {
				fmt.Fprintf(&b, " polarity=%s", c.Polarity)
			}
			if c.Subject != "" {
				fmt.Fprintf(&b, " subject=%s", c.Subject)
			}
			if c.Placement != nil {
				if c.Placement.Surface != "" {
					fmt.Fprintf(&b, " placement_surface=%s", c.Placement.Surface)
				}
				if c.Placement.Anchor != "" {
					fmt.Fprintf(&b, " placement_anchor=%s", c.Placement.Anchor)
				}
				if c.Placement.Expected != "" {
					fmt.Fprintf(&b, " placement_expected=%s", c.Placement.Expected)
				}
				if c.Placement.Relation != "" {
					fmt.Fprintf(&b, " placement_relation=%s", c.Placement.Relation)
				}
				if c.Placement.Delimiter != "" {
					fmt.Fprintf(&b, " placement_delimiter=%s", c.Placement.Delimiter)
				}
			}
			if types.IsPlanningOnlyWriteBehaviorContract(c) {
				b.WriteString(" planning_only=true")
			} else if c.Required {
				if types.IsHardRequiredWriteBehaviorContract(c) {
					b.WriteString(" hard_required=true")
				} else {
					b.WriteString(" soft_required=true")
				}
			}
			b.WriteByte('\n')
		}
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
			// Canonical single-phase view: state and the typed cause of
			// that state come from one derivation, so a batch can never
			// read as two contradictory phases at once.
			st := writeflow.DeriveBatchAttemptState(batch)
			line := fmt.Sprintf("  - %s state=%s", batch.ID, st.Phase)
			if st.Cause != "" {
				line += " cause=" + st.Cause
			}
			if st.FailedVerifyAttempts > 0 {
				line += fmt.Sprintf(" failed_verify_attempts=%d", st.FailedVerifyAttempts)
			}
			if batch.ExecutionMode != "" {
				line += " execution_mode=" + string(batch.ExecutionMode)
			}
			line += fmt.Sprintf(" goal=%s plan=%s", limitWriteControllerText(batch.Goal, 120), batch.PlanID)
			b.WriteString(line + "\n")
		}
	}
	if len(run.ProgressLedger) > 0 {
		last := run.ProgressLedger[len(run.ProgressLedger)-1]
		fmt.Fprintf(&b, "- last_event: batch=%s event=%s\n", last.BatchID, last.ReasonCode)
		if msg := strings.TrimSpace(last.Message); msg != "" {
			fmt.Fprintf(&b, "- last_event_detail: %s\n", limitWriteControllerText(msg, 220))
		}
	}
	return strings.TrimSpace(b.String())
}

func renderWriteControllerArtifactSection(ctx *types.AgentContext) string {
	var b strings.Builder
	b.WriteString("## Typed write artifacts\n\n")
	wrote := false
	if handoff := ctx.Mutable.WriteExplorationHandoff(); handoff != nil {
		normalized := types.NormalizeWriteExplorationHandoff(*handoff)
		wrote = true
		fmt.Fprintf(&b, "- exploration_handoff: status=present batch_id=%s target_files=%d symbols=%d evidence_refs=%d unknowns=%d confidence=%s\n",
			normalized.BatchID,
			len(normalized.TargetFiles),
			len(normalized.RelevantSymbols),
			len(normalized.EvidenceRefs),
			len(normalized.Unknowns),
			normalized.Confidence,
		)
		for i, path := range normalized.TargetFiles {
			if i >= 4 {
				fmt.Fprintf(&b, "- exploration_target_file: ... +%d more\n", len(normalized.TargetFiles)-i)
				break
			}
			fmt.Fprintf(&b, "- exploration_target_file: %s\n", path)
		}
		b.WriteString("- exploration_handoff_boundary: status=present is authoritative for handoff availability; context-pack compaction does not mean exploration is absent, and the controller must still choose the next typed action itself\n")
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		wrote = true
		fmt.Fprintf(&b, "- change_plan: id=%s status=%s targets=%d\n", plan.ID, plan.Status, len(plan.TargetPaths))
		if plan.Approval != nil {
			fmt.Fprintf(&b, "- approval: action=%s risk=%s decision=%s reason=%s\n",
				plan.Approval.Action, plan.Approval.RiskLevel, plan.Approval.UserDecision, plan.Approval.ReasonCode)
		}
	}
	if report := authoritativeWriteControllerReport(ctx); report != nil {
		wrote = true
		fmt.Fprintf(&b, "- change_report: plan_id=%s channel=%s passed=%t build_failed=%t\n",
			report.PlanID, report.Channel, report.Passed, report.BuildFailed)
		passedResults, failedResults := writeControllerVerificationResultCounts(report)
		fmt.Fprintf(&b, "- verification_evidence: status=%s passed_results=%d failed_results=%d total_results=%d\n",
			report.NormalizeVerificationStatus(), passedResults, failedResults, len(report.TestResults))
		if plan := ctx.Mutable.ChangePlan(); plan != nil {
			hard, covered, planningOnly := writeControllerBehaviorContractCoverage(plan, report)
			fmt.Fprintf(&b, "- verification_completion_scope: required_typed_contracts=%d covered_required_typed_contracts=%d planning_only_contracts=%d natural_language_acceptance_items=%d all_verified_applies_to=required_typed_obligations acceptance_items_authority=planning_guidance_only\n",
				hard, covered, planningOnly, len(plan.AcceptanceTests))
		}
		for i, cmd := range report.ExecutedCommands {
			if i >= 8 {
				fmt.Fprintf(&b, "- verification_command: ... +%d more\n", len(report.ExecutedCommands)-i)
				break
			}
			fmt.Fprintf(&b, "- verification_command: runner=%s cwd=%s suite=%s outcome=%s exit_code=%d source=%s command=%q\n",
				strings.TrimSpace(cmd.Runner),
				strings.TrimSpace(cmd.WorkingDir),
				strings.TrimSpace(cmd.Suite),
				strings.TrimSpace(cmd.Outcome),
				cmd.ExitCode,
				strings.TrimSpace(cmd.Source),
				limitWriteControllerText(cmd.Command, 240),
			)
		}
		if len(report.ExecutedCommands) > 0 {
			b.WriteString("- verification_result_accounting_boundary: total_results counts top-level runner results, not nested Make/Gradle/npm subcommands; retain the exact verification_command evidence and do not infer that a declared sub-check was skipped solely because total_results is smaller than the number of acceptance criteria\n")
		}
		for _, row := range writeControllerChangedPathCoverageRows(report.ChangedPathCoverage, 8) {
			fmt.Fprintf(&b, "- changed_path_verification: path=%s status=%s caliber=%s capability=%s\n",
				row.Path, row.Status, row.Caliber, row.Capability)
		}
		if audit := report.WorktreeAudit; audit != nil {
			fmt.Fprintf(&b, "- verification_worktree_audit: status=%s tracked_effects=%d untracked_effects=%d reason_code=%s\n",
				audit.Status, audit.TrackedEffectCount, audit.UntrackedEffectCount, audit.ReasonCode)
			for i, effect := range audit.Effects {
				if i >= 8 {
					fmt.Fprintf(&b, "- verification_worktree_effect: ... +%d more\n", len(audit.Effects)-i)
					break
				}
				fmt.Fprintf(&b, "- verification_worktree_effect: path=%s kind=%s ownership=%s action=%s\n",
					effect.Path, effect.Kind, effect.Ownership, effect.Action)
			}
		}
		if writeControllerHasProductionSourceStaticOnlyCoverage(report.ChangedPathCoverage) {
			b.WriteString("- changed_path_verification_boundary: source_static/syntax_only coverage proves source shape only; it is not target execution or target behavior, so do not select all_verified from report passed status alone\n")
		}
		if passedResults > 0 && report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
			b.WriteString("- verification_evidence_boundary: passed_results are retained partial evidence; the non-passed verification status means required verification remains incomplete and must not be described as zero checks or as fully verified\n")
		}
		if strings.TrimSpace(report.FailureSummary) != "" {
			fmt.Fprintf(&b, "- verify_failure_summary: %s\n", limitWriteControllerText(report.FailureSummary, 220))
		}
	}
	if !wrote {
		return ""
	}
	return strings.TrimSpace(b.String())
}

// writeControllerBehaviorContractCoverage summarizes only structured plan and
// verifier records. It deliberately does not compare acceptance prose with
// commands, tests, or model output. The proof ledger remains the completion
// authority; this compact row keeps the controller from overstating what the
// all_verified enum means.
func writeControllerBehaviorContractCoverage(plan *types.ChangePlan, report *types.ChangeReport) (hard, covered, planningOnly int) {
	if plan == nil {
		return 0, 0, 0
	}
	contracts := types.ChangePlanVerificationBehaviorContracts(plan)
	hardIDs := types.HardRequiredWriteBehaviorContractIDs(contracts)
	hard = len(hardIDs)
	for _, contract := range contracts {
		if types.IsPlanningOnlyWriteBehaviorContract(contract) {
			planningOnly++
		}
	}
	if report == nil || hard == 0 {
		return hard, 0, planningOnly
	}
	seen := map[string]bool{}
	for _, record := range report.VerificationConfidence {
		if strings.TrimSpace(record.Status) != "satisfied" {
			continue
		}
		switch strings.TrimSpace(record.Category) {
		case "probe_contract_refs", "project_test_contract_refs":
		default:
			continue
		}
		for _, raw := range record.ContractRefs {
			ref := strings.TrimSpace(raw)
			if _, required := hardIDs[ref]; required {
				seen[ref] = true
			}
		}
	}
	return hard, len(seen), planningOnly
}

func writeControllerChangedPathCoverageRows(rows []types.ChangedPathVerificationCoverage, limit int) []types.ChangedPathVerificationCoverage {
	out := make([]types.ChangedPathVerificationCoverage, 0, len(rows))
	seen := map[string]bool{}
	for _, row := range rows {
		path := strings.TrimSpace(row.Path)
		if path == "" {
			continue
		}
		key := strings.Join([]string{path, string(row.Status), string(row.Caliber), string(row.Capability)}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		row.Path = path
		out = append(out, row)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func writeControllerHasProductionSourceStaticOnlyCoverage(rows []types.ChangedPathVerificationCoverage) bool {
	for _, row := range rows {
		path := strings.TrimSpace(row.Path)
		if path == "" || types.SourcePathRoleIsAuxiliary(types.ClassifySourcePathRole(path)) ||
			row.Status != types.ChangedPathVerificationCovered {
			continue
		}
		switch row.Capability {
		case types.VerificationCapabilitySourceStatic, types.VerificationCapabilitySyntaxOnly, types.VerificationCapabilityUnknown:
			return true
		}
	}
	return false
}

func writeControllerVerificationResultCounts(report *types.ChangeReport) (passed, failed int) {
	if report == nil {
		return 0, 0
	}
	for _, result := range report.TestResults {
		if result.Passed {
			passed++
		} else {
			failed++
		}
	}
	return passed, failed
}

func authoritativeWriteControllerReport(ctx *types.AgentContext) *types.ChangeReport {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	report := ctx.Mutable.ChangeReport()
	if report == nil {
		return nil
	}
	if report.Channel != "" && report.Channel != types.ChangeReportChannelPostApplyVerify {
		return nil
	}
	currentPlanID := ""
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		currentPlanID = strings.TrimSpace(plan.ID)
	}
	if currentPlanID != "" && strings.TrimSpace(report.PlanID) != "" && strings.TrimSpace(report.PlanID) != currentPlanID {
		return nil
	}
	if currentPlanID == "" {
		if run := ctx.Mutable.WriteWorkflowRun(); run != nil {
			for _, batch := range run.Batches {
				if batch.ID == run.ActiveBatchID {
					currentPlanID = strings.TrimSpace(batch.PlanID)
					break
				}
			}
		}
	}
	if currentPlanID != "" && strings.TrimSpace(report.PlanID) != "" && strings.TrimSpace(report.PlanID) != currentPlanID {
		return nil
	}
	return report
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

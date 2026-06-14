package orchestrator

import (
	"fmt"
	"hash/fnv"
	"os"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/worktree"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// stage_hooks.go implements per-stage pre/post hooks the write-mode
// scheduler runs around each dispatchStage call. The hooks own the
// "stage-edge" work that isn't part of the agent's ReAct loop:
// worktree provisioning, baseline capture, plan-status persistence,
// disk side-effects on the report.
//
// Read-mode stages have no hooks because the read scheduler keeps
// its existing inline logic (Tier1Floor, contract retry, etc.).
// Hooks are write-mode only — read mode looks them up in
// writeStageHooks and gets the empty Hooks zero value, which
// short-circuits to no-op.

// stageHookFunc is the per-hook signature. Pre hooks return an error
// that terminates the stage dispatch (e.g. worktree provision
// failure). Post hooks are best-effort: errors log a warning but do
// not fail the stage — the hook's job is to record state, not gate
// the dispatch.
type stageHookFunc func(o *Orchestrator) error

// stageHooks bundles a stage's pre + post hooks. Either may be nil.
type stageHooks struct {
	Pre  stageHookFunc
	Post stageHookPostFunc
}

// stageHookPostFunc gets the StageOutput so the hook can branch on
// success vs. error (e.g. apply post hook persists PlanStatusApplyFailed
// when StageOutput.Error is set; success leaves status untouched
// because verify will flip it).
type stageHookPostFunc func(o *Orchestrator, out *agent.StageOutput) error

// writeStageHooks is the static lookup table the write scheduler
// consults before/after each dispatchStage call. Entries without a
// hook (e.g. StagePlan post-hook is just a Result render) keep the
// nil pointer; the scheduler skips nil hooks.
var writeStageHooks = map[types.PipelineStage]stageHooks{
	types.StagePlan: {
		Pre:  planPreHook,
		Post: planPostHook,
	},
	types.StageApply: {
		Pre:  applyPreHook,
		Post: applyPostHook,
	},
	types.StageVerify: {
		Pre:  verifyPreHook,
		Post: verifyPostHook,
	},
}

// runStageHooks fires the pre hook before the dispatch + the post
// hook after, with sensible nil-skipping. Returns the pre-hook
// error verbatim so the dispatch loop can surface it as a stage
// failure (the post hook never runs in that case).
func runStagePreHook(o *Orchestrator, stage types.PipelineStage) error {
	h := writeStageHooks[stage].Pre
	if h == nil {
		return nil
	}
	return h(o)
}

func runStagePostHook(o *Orchestrator, stage types.PipelineStage, out *agent.StageOutput) {
	h := writeStageHooks[stage].Post
	if h == nil {
		return
	}
	if err := h(o, out); err != nil {
		logging.Warning("[orchestrator] %s post-hook: %v", stage, err)
	}
}

// planPostHook fires after the planner returns. The plan is on
// Mutable.ChangePlan via emit_change_plan; this hook reads it back
// and renders the human-visible Result summary that ModePlan Runs
// terminate with.
//
// Idempotent PendingApplies refresh: emit_change_plan.Execute
// normally populates WriteClosure.PendingApplies as part of the tool
// call, but a test stub or alternative emission path may install the
// ChangePlan without touching the closure. To make the plan node's
// CritPlanReady SuccessCriteria robust to that, the hook seeds
// PendingApplies from plan.Changes when the queue is empty. Real
// LLM-driven runs see the queue already populated so the loop is a
// no-op.
func planPostHook(o *Orchestrator, out *agent.StageOutput) error {
	if o == nil || o.busCtx == nil {
		return nil
	}
	plan := o.busCtx.Mutable.ChangePlan()
	if plan == nil {
		// Planner ran but no plan emitted — translate the internal
		// "did not call emit_change_plan" diagnostic into a
		// user-actionable explanation. The historical message
		// leaked the tool name and stage internals into the answer
		// surface; users who asked a "how do I install pygame"
		// question while sticky in plan mode received the literal
		// "planner did not call emit_change_plan" — confusing and
		// unprofessional.
		//
		// SetResultPlain still bypasses glamour because the message
		// contains identifier-shaped tokens (`/mode auto` etc.)
		// that chroma's tokenizer would otherwise fragment.
		msg := plannerProseFallbackMessage(o.busCtx)
		o.busCtx.Mutable.SetResultPlain(msg)
		return fmt.Errorf("%s", msg)
	}
	// Multi-repo write contract gate (P4.G design §4.5.5). Fail-loud
	// when the ChangePlan touches more than one sub-repo — multi-
	// repo write is contractually banned because the worktree
	// cleanup defer cannot reliably undo cross-sub-repo dirtying.
	// Single-repo posture trivially passes (every path resolves to
	// the same SubRepoSnapshot).
	if v := ValidateChangePlanScope(o.busCtx, plan); v != nil {
		o.busCtx.Mutable.SetResultPlain(fmt.Sprintf("write blocked: %s\n\n%s", v.Detail, v.Repair))
		// Mirror the violation into the EvidenceClosure ledger so
		// retry-loop consumers and telemetry see it. Severity High +
		// non-promotable + LocusTerminal makes it terminal — no
		// auto-recovery.
		if cl := o.busCtx.Mutable.EvidenceClosure(); cl != nil {
			cl.AppendViolation(*v)
		}
		return fmt.Errorf("write blocked (multi-repo cross-sub-repo plan): %s", v.Detail)
	}

	wc := o.busCtx.Mutable.WriteClosure()
	if len(wc.PendingApplies()) == 0 {
		for _, c := range plan.Changes {
			wc.EnqueuePendingApply(types.PendingApply{
				Path:      c.Path,
				Rationale: c.Rationale,
				Origin:    "plan_post_hook",
			})
		}
	}
	// Pin the WriteAnalysisIR snapshot to the plan struct so that
	// a subsequent /approve or /approve --retry against this plan
	// reuses the same IR instead of re-dispatching write_analyzer
	// (commit 9 #3). Without this, plan_critic on apply-time
	// might reason about a slightly different IR than the one
	// the planner used to generate the plan; cost-wise it also
	// avoids a second LLM call.
	if ir := o.busCtx.Mutable.WriteAnalysisIR(); ir != nil {
		plan.WriteAnalysisIR = ir
	}
	o.stampWorkflowPlanForActiveBatch(plan, nil)

	o.busCtx.Mutable.SetResult(renderChangePlanSummary(plan, o.busCtx.Language))
	logging.Info("[orchestrator] plan stage: id=%s changes=%d", plan.ID, len(plan.Changes))

	if err := enforceWriteApprovalBeforeApply(o, plan, "plan_post_hook"); err != nil {
		return err
	}

	// Optional pre-apply review (commit 4 P1-F). When the operator
	// has wired plan_critic and the yaml gate is on, dispatch a
	// single-Chat review of the plan. Output is informational only —
	// stored on Mutable.PlanCritique for the same Run AND folded
	// into the persisted ChangePlan.PlanCritique so /plan show
	// surfaces the critique across REPL sessions. Failures degrade
	// silently to "no critique"; apply is never blocked by review
	// plumbing.
	if o.planCritic != nil {
		reviewCtx := o.CancelContext()
		verdict, err := o.planCritic.Review(reviewCtx, buildPlanCriticInput(o.busCtx))
		if err != nil {
			logging.Warning("[orchestrator] plan_critic degraded: %v (plan continues without critique)", err)
		} else if !verdict.IsEmpty() {
			critique := AssemblePlanCritiqueProse(verdict)
			if critique != "" {
				o.busCtx.Mutable.SetPlanCritique(critique)
				plan.PlanCritique = critique
				// Persist back to the plan file so REPL restart still
				// shows the critique. PlanStore-resident plans also
				// flow through this path because plan_post_hook re-
				// renders against the live Mutable plan.
				if o.busCtx.PlanPath != "" {
					if writeErr := types.WritePlanToFile(plan, o.busCtx.PlanPath); writeErr != nil {
						logging.Warning("[orchestrator] plan persist (with critique) failed: %v", writeErr)
					}
				}
			}
			// Block 1 (architecture overhaul 2026-05-02) — also fold
			// each risk into the EvidenceClosure ledger so the
			// stage-wise health snapshot, Block 3's fallback policy,
			// and the end-of-Run summary line see plan_critic's
			// findings the same way they see every other gate's
			// findings. Each Risk becomes one ViolPlanCritic at
			// Stage="plan"; Confidence comes from the verdict's
			// label via PlanCriticConfidenceFloat. SOFT-by-default
			// (cmd/root.go default strict-kinds excludes ViolPlanCritic),
			// preserving the historical "informational only" behaviour
			// — apply is never blocked even if dozens of risks land.
			confidence := PlanCriticConfidenceFloat(verdict.Confidence)
			closure := o.busCtx.Mutable.EvidenceClosure()
			riskCount := 0
			for i, risk := range verdict.Risks {
				risk = strings.TrimSpace(risk)
				if risk == "" {
					continue
				}
				riskCount++
				closure.AppendViolation(types.Violation{
					Kind:       types.ViolPlanCritic,
					ClusterKey: types.RootClusterKey("plan_critic_risk"),
					Detail:     fmt.Sprintf("plan_critic risk %d/%d: %s", i+1, len(verdict.Risks), risk),
					Repair:     "review the plan against this risk before /approve; the critic is observational, not a hard reject. Address the risk by editing the plan or accept it as a known concern.",
					Stage:      string(types.StagePlan),
					SuspectedRoot: types.SuspectedRoot{
						IRField:    "plan_critic_risk",
						Reason:     "independent reviewer LLM flagged a concern",
						Confidence: confidence,
					},
				})
			}
			// Audit followup (2026-05-02): surface the review event to
			// the dock so the user knows the reviewer ran and how many
			// risks it flagged — without this, the critic's full output
			// only appears via /plan show, which the user might not
			// check before /approve. The full critique still persists
			// on plan.PlanCritique; the dock line is a discoverability
			// hint, not a replacement.
			o.emit(render.Event{
				Kind:       render.EventOrchestratorNotice,
				Timestamp:  time.Now(),
				Agent:      "orchestrator",
				NoticeKind: render.NoticePlanReview,
				Reasoning:  softPlanCriticReviewMessage(o.busCtx.Language, riskCount),
			})
		}
	}
	mergeWritePlanContextPack(o, plan)
	return nil
}

func enforceWriteApprovalBeforeApply(o *Orchestrator, plan *types.ChangePlan, source string) error {
	if o == nil || o.busCtx == nil || plan == nil {
		return nil
	}
	if o.busCtx.Mode != types.ModeApply {
		return nil
	}
	assessment := writeflow.AssessWriteRisk(o.writeRiskAssessmentInput(plan))
	policy := o.writeApprovalPolicy
	if policy == "" {
		policy = writeflow.ApprovalPolicyAutoSafe
	}
	decision := writeflow.DecideWriteApproval(policy, assessment)
	mergeWriteRiskContextPack(o, plan, assessment, decision)
	fingerprint := types.PlanFingerprint(plan)
	switch decision.Action {
	case writeflow.ApprovalActionAutoExecute:
		stampWriteApprovalRecord(o, plan, assessment, decision, source, "auto", fingerprint)
		persistWriteApprovalRecord(o, plan)
		mergeWritePlanContextPack(o, plan)
		return nil
	case writeflow.ApprovalActionDeny:
		stampWriteApprovalRecord(o, plan, assessment, decision, source, "denied", fingerprint)
		plan.Status = types.PlanStatusBlocked
		msg := writeApprovalGateMessage(o.busCtx.Language, plan.ID, assessment, decision)
		o.busCtx.Mutable.SetResultPlain(msg)
		if o.busCtx.PlanPath != "" {
			persistWriteApprovalRecord(o, plan)
		}
		mergeWritePlanContextPack(o, plan)
		return fmt.Errorf("write approval denied for plan %s: %s", plan.ID, decision.ReasonCode)
	case writeflow.ApprovalActionManual:
		if !plan.ApprovalRecordIntegrityOK() {
			// The persisted record fails its own tamper-evidence hash:
			// whatever user_decision it claims cannot be trusted. Fall
			// through to the manual-approval-required lane with a typed
			// reason instead of honoring the record.
			decision.ReasonCode = "approval_record_integrity_failed"
			decision.Reason = "the persisted approval record does not match its own fingerprint; re-approve the plan"
		} else if writeApprovalRecordAllowsManualApply(plan, fingerprint) {
			mergeWritePlanContextPack(o, plan)
			return nil
		}
		userDecision := "required"
		if plan.Approval != nil && plan.Approval.UserDecision == "approved" && plan.Approval.PlanFingerprint != fingerprint {
			decision.ReasonCode = "stale_approval_fingerprint"
			decision.Reason = "previous manual approval was for a different plan fingerprint"
			userDecision = "stale"
		}
		stampWriteApprovalRecord(o, plan, assessment, decision, source, userDecision, fingerprint)
		plan.Status = types.PlanStatusPending
		msg := writeApprovalGateMessage(o.busCtx.Language, plan.ID, assessment, decision)
		o.busCtx.Mutable.SetResultPlain(msg)
		if o.busCtx.PlanPath != "" {
			persistWriteApprovalRecord(o, plan)
		}
		mergeWritePlanContextPack(o, plan)
		return fmt.Errorf("write approval required for plan %s: %s", plan.ID, decision.ReasonCode)
	default:
		return nil
	}
}

func mergeWritePlanContextPack(o *Orchestrator, plan *types.ChangePlan) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || plan == nil {
		return
	}
	pack := types.WriteContextPackFromChangePlan(plan)
	if len(pack.Items) == 0 {
		return
	}
	o.busCtx.Mutable.MergeWriteContextPack(pack)
}

func mergeWriteRiskContextPack(o *Orchestrator, plan *types.ChangePlan, assessment writeflow.RiskAssessment, decision writeflow.ApprovalDecision) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || plan == nil {
		return
	}
	pack := writeflow.ContextPackFromRiskAssessment(plan.ID, plan.Summary, assessment, decision)
	if len(pack.Items) == 0 {
		return
	}
	o.busCtx.Mutable.MergeWriteContextPack(pack)
}

func stampWriteApprovalRecord(o *Orchestrator, plan *types.ChangePlan, assessment writeflow.RiskAssessment, decision writeflow.ApprovalDecision, source, userDecision, fingerprint string) {
	if plan == nil {
		return
	}
	operator := ""
	if o != nil && o.busCtx != nil {
		root := o.busCtx.MainRepoRoot
		if strings.TrimSpace(root) == "" {
			root = o.busCtx.RepoRoot
		}
		operator = worktree.OperatorIdentity(root)
	}
	plan.Approval = writeflow.NewApprovalRecord(assessment, decision, source, userDecision, fingerprint, operator)
}

func writeApprovalRecordAllowsManualApply(plan *types.ChangePlan, fingerprint string) bool {
	if plan == nil || plan.Approval == nil || fingerprint == "" {
		return false
	}
	return plan.Approval.PlanFingerprint == fingerprint &&
		plan.Approval.Action == string(writeflow.ApprovalActionManual) &&
		plan.Approval.UserDecision == "approved"
}

func persistWriteApprovalRecord(o *Orchestrator, plan *types.ChangePlan) {
	if o == nil || o.busCtx == nil || plan == nil || strings.TrimSpace(o.busCtx.PlanPath) == "" {
		return
	}
	if err := types.WritePlanToFile(plan, o.busCtx.PlanPath); err != nil {
		logging.Warning("[orchestrator] persist write approval record failed: %v", err)
	}
}

func writeApprovalGateMessage(lang, planID string, assessment writeflow.RiskAssessment, decision writeflow.ApprovalDecision) string {
	zh := !strings.EqualFold(strings.TrimSpace(lang), "en")
	reasons := assessment.TopReasons(5)
	var b strings.Builder
	if zh {
		fmt.Fprintf(&b, "写入计划 %s 暂停执行。\n\n", planID)
		fmt.Fprintf(&b, "- 审批动作: %s\n- 策略: %s\n- 风险: %s\n- 原因: %s\n",
			decision.Action, decision.Policy, assessment.Level, decision.ReasonCode)
		if len(reasons) > 0 {
			b.WriteString("\n主要风险:\n")
			for _, reason := range reasons {
				if reason.Path != "" {
					fmt.Fprintf(&b, "- %s/%s: %s (%s)\n", reason.Level, reason.Code, reason.Detail, reason.Path)
				} else {
					fmt.Fprintf(&b, "- %s/%s: %s\n", reason.Level, reason.Code, reason.Detail)
				}
			}
		}
		if decision.Action == writeflow.ApprovalActionManual {
			b.WriteString("\n请审阅风险和 diff；准备好后批准或拒绝当前 batch。")
		}
		return b.String()
	}
	fmt.Fprintf(&b, "Write plan %s paused before apply.\n\n", planID)
	fmt.Fprintf(&b, "- approval_action: %s\n- policy: %s\n- risk: %s\n- reason: %s\n",
		decision.Action, decision.Policy, assessment.Level, decision.ReasonCode)
	if len(reasons) > 0 {
		b.WriteString("\nTop risk reasons:\n")
		for _, reason := range reasons {
			if reason.Path != "" {
				fmt.Fprintf(&b, "- %s/%s: %s (%s)\n", reason.Level, reason.Code, reason.Detail, reason.Path)
			} else {
				fmt.Fprintf(&b, "- %s/%s: %s\n", reason.Level, reason.Code, reason.Detail)
			}
		}
	}
	if decision.Action == writeflow.ApprovalActionManual {
		b.WriteString("\nReview risk and diff; then approve or reject the current batch when ready.")
	}
	return b.String()
}

// planPreHook is the structural authorization gate for the plan
// stage. Three independent decision points, evaluated in order so
// each tier surfaces its own actionable error before the next is
// considered:
//
//  1. Repo state probe (worktree.DetectRepoState). Ready repos pass
//     unconditionally — that is the byte-identical pre-2026-04-29
//     happy path.
//  2. Init authorization. State NeedsInit + !autoInitRepo →
//     fail-fast with bareDirAuthorizationMessage. The user must
//     explicitly opt into git-init mutation of their target dir.
//  3. Scaffold authorization. State NeedsInit + autoInitRepo + the
//     dir is structurally empty (no source files outside .git /
//     .codrax) + !scaffoldEnabled → fail-fast with
//     scaffoldAuthorizationMessage. Init authorization is NOT a
//     proxy for "may invent files for an empty dir" — those are
//     two distinct user-granted permissions, gated separately so
//     the user is not surprised by codrax fabricating an entire
//     project from a single --auto-init-repo flag.
//
// All three checks are STRUCTURAL — they read git state and
// directory contents, they DO NOT classify the user's request text.
// The same call that fails fast for "请用 python 写一个游戏" against
// an empty dir would equally fail fast for any other request,
// because the gate's job is to refuse a setup the planner cannot
// run productively, not to interpret intent.
//
// On the scaffold-authorized path we seed PlanningHint with the
// SCAFFOLD DIRECTIVE so the planner uses the streaming-safe
// multi-round emission path on its first dispatch. The planner has
// no source files to read on a from-scratch run, so the LLM is
// going to assemble the entire plan from the request text + general
// knowledge — exactly the case where a single emit_change_plan
// call is most likely to overflow the stream. Pre-arming the hint
// turns the second-attempt recovery into a first-attempt preference.
//
// The hook does NOT provision a worktree — plan stage runs against
// the main repo's working tree (read-only ops). applyPreHook owns
// the git init + worktree path when the apply stage actually needs
// it.
func planPreHook(o *Orchestrator) error {
	if o == nil || o.busCtx == nil {
		return fmt.Errorf("plan pre-hook: nil orchestrator/bus")
	}
	root := o.busCtx.MainRepoRoot
	if root == "" {
		root = o.busCtx.RepoRoot
	}
	state, err := worktree.DetectRepoState(root)
	if err != nil {
		msg := fmt.Sprintf("plan stage: repo state probe failed: %v", err)
		o.busCtx.Mutable.SetResultPlain(msg)
		return fmt.Errorf("%s", msg)
	}
	if !state.NeedsInit() {
		return nil
	}

	// Tier 1: init authorization.
	if !o.autoInitRepo {
		msg := bareDirAuthorizationMessage(o.busCtx, state, "plan")
		o.busCtx.Mutable.SetResultPlain(msg)
		return fmt.Errorf("%s", msg)
	}

	// Tier 2: scaffold authorization, gated on dir emptiness.
	// Non-empty bare dirs (existing source, no git) bypass this
	// check — the planner has real code to read and only needs
	// the upstream init authorization. dirIsEffectivelyEmpty walks
	// the dir capped at 256 entries so a deeply nested non-empty
	// tree returns false quickly.
	empty := dirIsEffectivelyEmpty(root)
	if empty && !o.scaffoldEnabled {
		msg := scaffoldAuthorizationMessage(o.busCtx)
		o.busCtx.Mutable.SetResultPlain(msg)
		return fmt.Errorf("%s", msg)
	}

	// Authorized. Defer git init to applyPreHook (plan stage does
	// not need a git index). On the empty-dir path additionally
	// seed PlanningHint with the SCAFFOLD DIRECTIVE so the planner
	// picks the streaming-safe multi-round emission path on its
	// first dispatch. Existing PlanningHint (from a prior verify→
	// plan SC retry) is preserved.
	if empty {
		logging.Info("[orchestrator] plan pre-hook: empty bare repo at %s; scaffold authorized (init deferred to apply stage)", root)
		if o.busCtx.Mutable != nil && o.busCtx.Mutable.PlanningHint() == "" {
			o.busCtx.Mutable.SetPlanningHint(plannerScaffoldHint())
		}
	} else {
		logging.Info("[orchestrator] plan pre-hook: non-empty bare repo at %s; init deferred to apply stage (existing source detected)", root)
	}
	return nil
}

// applyPreHook prepares the worktree before the coder dispatches.
// Three sub-steps:
//
//  1. Load ChangePlan from disk when PlanPath is set and Mutable
//     hasn't already received one (the post-plan-stage path leaves
//     Mutable.ChangePlan populated; the --plan-file path doesn't).
//  2. Provision a fresh worktree and swap busCtx.RepoRoot to it.
//     Idempotent: if WorktreePath is already set (mid-retry second
//     apply round), reuses the existing worktree's path.
//  3. Optionally capture the pre-apply baseline test snapshot.
func applyPreHook(o *Orchestrator) error {
	if o == nil || o.busCtx == nil {
		return fmt.Errorf("apply pre-hook: nil orchestrator/bus")
	}
	// 1. Plan loaded?
	if o.busCtx.Mutable.ChangePlan() == nil {
		if o.busCtx.PlanPath == "" {
			msg := "apply stage: Mutable.ChangePlan is nil and no --plan-file was supplied; cannot apply without a plan"
			o.busCtx.Mutable.SetResult(msg)
			return fmt.Errorf("%s", msg)
		}
		plan, err := types.LoadChangePlanFromFile(o.busCtx.PlanPath)
		if err != nil {
			msg := fmt.Sprintf("apply stage: load plan file failed: %v", err)
			o.busCtx.Mutable.SetResult(msg)
			return fmt.Errorf("%s", msg)
		}
		o.busCtx.Mutable.SetChangePlan(plan)
		// Seed PendingApplies so CritPlanReady passes on the first
		// EntryConditions evaluation. Same shape the legacy
		// the apply stage hook used.
		wc := o.busCtx.Mutable.WriteClosure()
		for _, c := range plan.Changes {
			wc.EnqueuePendingApply(types.PendingApply{
				Path:      c.Path,
				Rationale: c.Rationale,
				Origin:    "load_from_file",
			})
		}
		// Restore the pinned WriteAnalysisIR snapshot when the plan
		// carries one (commit 9 #3). Lets plan_critic and any
		// downstream IR consumer in this Run see the SAME task-shape
		// classification the planner used at emit time, instead of
		// a fresh (potentially drifted) IR from a re-run write_analyzer.
		// Falls through to nil when the plan was emitted before commit
		// 9 — runWriteAnalyzePhase will then dispatch fresh.
		if plan.WriteAnalysisIR != nil {
			o.busCtx.Mutable.SetWriteAnalysisIR(plan.WriteAnalysisIR)
			logging.Info("[orchestrator] apply pre-hook: restored pinned WriteAnalysisIR from plan (kind=%s scope=%s)",
				plan.WriteAnalysisIR.Request.Task.Kind, plan.WriteAnalysisIR.Request.Task.Scope)
		}
		logging.Info("[orchestrator] apply pre-hook: loaded plan %s from %s (%d changes)",
			plan.ID, o.busCtx.PlanPath, len(plan.Changes))
	}
	plan := o.busCtx.Mutable.ChangePlan()
	if err := enforceWriteApprovalBeforeApply(o, plan, "apply_pre_hook"); err != nil {
		return err
	}
	// 2. Worktree provisioning.
	if o.worktreeBase == "" {
		msg := "apply stage: worktree base directory not configured (orchestrator.SetWorktreeBase was not called)"
		o.busCtx.Mutable.SetResult(msg)
		return fmt.Errorf("%s", msg)
	}
	if o.busCtx.WorktreePath == "" {
		// Bare-directory scaffolding gate. DetectRepoState classifies
		// the main repo as ready / not-initialized / no-commits.
		// `git worktree add --detach HEAD` (inside worktree.Create)
		// fails on the latter two, so transition them to ready first
		// — but ONLY when the operator has authorized it via the
		// three-tier surface (CLI --auto-init-repo, yaml
		// write_auto_init_repo, or REPL consent that pre-toggles
		// the orchestrator setter for this Run). Without authorization
		// fail-loud with a hint that names every surface, so the
		// operator picks the path that fits their workflow.
		if state, err := worktree.DetectRepoState(o.busCtx.MainRepoRoot); err == nil && state.NeedsInit() {
			if !o.autoInitRepo {
				// Build the user-facing error message via the
				// env_recommend Renderer when EnvFacts is available
				// (env_recommend_enabled=true). Falls back to the
				// legacy hardcoded prose when env_recommend is
				// disabled (R6 byte-identical retention guarantee).
				msg := bareDirAuthorizationMessage(o.busCtx, state, "apply")
				o.busCtx.Mutable.SetResultPlain(msg)
				return fmt.Errorf("%s", msg)
			}
			// Apply-tier scaffold gate. Single-shot `--mode=write --write-phase=apply
			// --plan-file=X` against an empty bare dir must NOT
			// silently scaffold just because init was authorized;
			// the same two-permission split planPreHook enforces
			// applies here so a CLI user who skipped the plan
			// stage cannot get scaffold behaviour with only
			// --auto-init-repo. Non-empty bare dirs bypass.
			if dirIsEffectivelyEmpty(o.busCtx.MainRepoRoot) && !o.scaffoldEnabled {
				msg := scaffoldAuthorizationMessage(o.busCtx)
				o.busCtx.Mutable.SetResultPlain(msg)
				return fmt.Errorf("%s", msg)
			}
			commitMsg := "codrax: initial commit"
			if plan := o.busCtx.Mutable.ChangePlan(); plan != nil && plan.ID != "" {
				commitMsg = "codrax: initial commit for " + plan.ID
			}
			if err := worktree.EnsureInitialCommit(o.busCtx.MainRepoRoot, commitMsg); err != nil {
				msg := fmt.Sprintf("apply stage: auto-init repo failed: %v", err)
				o.busCtx.Mutable.SetResult(msg)
				return fmt.Errorf("%s", msg)
			}
			logging.Info("[orchestrator] apply pre-hook: auto-initialized bare repo at %s", o.busCtx.MainRepoRoot)
			// Refresh EnvFacts.GitRepoState in place so downstream
			// surfaces (stallPlateauMessage, env_recommend
			// detectGitState, REPL /env show) read the new ground
			// truth — without this, a Run that init'd the repo
			// post-probe leaves "not_initialized" cached, and any
			// later failure rendering recommends `git init` /
			// `--auto-init-repo` for a dir that IS now a real repo.
			if o.busCtx.EnvFacts != nil {
				o.busCtx.EnvFacts.GitRepoState = "ready"
			}
		} else if err != nil {
			msg := fmt.Sprintf("apply stage: repo state probe failed: %v", err)
			o.busCtx.Mutable.SetResult(msg)
			return fmt.Errorf("%s", msg)
		}
		if o.emit != nil {
			o.emit(render.Event{
				Kind:      render.EventWorktreePreparingStart,
				Timestamp: time.Now(),
				Agent:     "orchestrator",
			})
		}
		sess, err := worktree.Create(o.worktreeBase, o.busCtx.MainRepoRoot, o.busCtx.TraceID)
		if o.emit != nil {
			o.emit(render.Event{
				Kind:      render.EventWorktreePreparingEnd,
				Timestamp: time.Now(),
				Agent:     "orchestrator",
			})
		}
		if err != nil {
			msg := fmt.Sprintf("apply stage: worktree provisioning failed: %v", err)
			o.busCtx.Mutable.SetResult(msg)
			return fmt.Errorf("%s", msg)
		}
		o.busCtx.WorktreePath = sess.Path()
		o.busCtx.RepoRoot = sess.Path()
		o.busCtx.Mutable.SetRepoRoot(sess.Path())
		logging.Info("[orchestrator] apply pre-hook: worktree at %s", sess.Path())
	}
	// 3. Baseline capture. The cache always tries first when it is
	// enabled — a hit gives us a free baseline regardless of the
	// per-Run capture flag, because re-running the test suite for
	// the same main-repo HEAD would produce identical results. On
	// miss, fall through to the captureBaseline path when the
	// per-Run capture flag is set.
	if o.busCtx.Mutable.BaselineReport() == nil {
		o.tryBaselineFromCacheThenCapture()
	}
	return nil
}

// tryBaselineFromCacheThenCapture is the cache-first wrapper around
// captureBaseline. Order:
//  1. Cache lookup keyed by the main-repo HEAD SHA. Hit → install
//     the cached report on Mutable, skip the test re-run.
//  2. Cache miss + baselineCaptureEnabled=true → captureBaseline()
//     runs the test suite, installs Mutable.BaselineReport, and
//     writes the result to the cache for future Runs.
//  3. Cache miss + baselineCaptureEnabled=false → no-op (legacy
//     behaviour preserved).
//
// Errors at every stage are logged and swallowed; baseline data is
// advisory for CritNoRegression and never blocks apply.
func (o *Orchestrator) tryBaselineFromCacheThenCapture() {
	cache := o.baselineCache
	sha := ""
	if cache.Enabled() && o.busCtx.MainRepoRoot != "" {
		if got, err := gitHeadSHA(o.busCtx.MainRepoRoot); err == nil {
			sha = got
		}
	}
	if sha != "" {
		if cached := cache.Lookup(sha); cached != nil {
			o.busCtx.Mutable.SetBaselineReport(cached)
			logging.Info("[orchestrator] baseline reused from cache (sha=%s tests=%d)",
				sha[:8], len(cached.TestResults))
			return
		}
	}
	if !o.baselineCaptureEnabled {
		return
	}
	// Cache miss + capture enabled: signal the dock so row 1
	// reads "抓取基准" / "capturing baseline" while the
	// (potentially 30s+) test suite runs. Cache hits skip this
	// pair — the lookup-only path returns above before the emit.
	if o.emit != nil {
		o.emit(render.Event{
			Kind:      render.EventBaselineCapturingStart,
			Timestamp: time.Now(),
			Agent:     "orchestrator",
		})
	}
	o.captureBaseline()
	if o.emit != nil {
		o.emit(render.Event{
			Kind:      render.EventBaselineCapturingEnd,
			Timestamp: time.Now(),
			Agent:     "orchestrator",
		})
	}
	// After capture, write to the cache for future Runs. captureBaseline
	// already populated Mutable.BaselineReport — read back and store.
	if sha != "" && cache.Enabled() {
		if report := o.busCtx.Mutable.BaselineReport(); report != nil {
			cache.Store(sha, report)
		}
	}
}

// classifyApplyFailureStatus picks the right status enum value for
// an apply-stage failure. Differentiates two recovery shapes:
//
//   - applied_failed: apply produced ZERO successful units; the
//     worktree is structurally untouched. The operator can /reject
//     and start over without needing to clean anything up.
//   - partially_applied: apply succeeded on some units before
//     hitting a rejection on a later one. The worktree carries a
//     partial diff. /merge would land incoherent code; /reject
//     discards everything; /approve --retry can re-plan from the
//     current AppliedSet.
//
// Differentiator: WriteClosure.AppliedSet has at least one entry
// that's also in plan.TargetPaths, AND AppliedSet ∩ TargetPaths is
// a strict subset of TargetPaths. When AppliedSet covers EVERY
// TargetPath but err is set, the failure happened post-apply (e.g.
// commit-checkpoint failure) and applied_failed is the conservative
// fallback — the bytes are on disk in the worktree but the
// operator should investigate before /merge.
func classifyApplyFailureStatus(busCtx *types.BusContext) string {
	if busCtx == nil || busCtx.Mutable == nil {
		return types.PlanStatusApplyFailed
	}
	plan := busCtx.Mutable.ChangePlan()
	if plan == nil || len(plan.TargetPaths) == 0 {
		return types.PlanStatusApplyFailed
	}
	applied := busCtx.Mutable.WriteClosure().AppliedSet()
	appliedTargets := 0
	for _, p := range plan.TargetPaths {
		if applied[p] {
			appliedTargets++
		}
	}
	if appliedTargets > 0 && appliedTargets < len(plan.TargetPaths) {
		logging.Info("[orchestrator] apply post-hook: partial state — %d of %d target paths landed before failure",
			appliedTargets, len(plan.TargetPaths))
		return types.PlanStatusPartiallyApplied
	}
	return types.PlanStatusApplyFailed
}

// applyPostHook runs after the coder returns. On dispatch error,
// persists PlanStatusApplyFailed (or PlanStatusPartiallyApplied
// when some units succeeded) and surfaces the error so the
// scheduler stops the cycle. On success, renders an apply-summary
// Result. Status persistence on success is deferred to verify's
// post-hook (verify_failed vs applied) so the disk file isn't
// double-written on every Run.
func applyPostHook(o *Orchestrator, out *agent.StageOutput) error {
	if o == nil || o.busCtx == nil {
		return nil
	}
	if out != nil && out.Error != "" {
		status := classifyApplyFailureStatus(o.busCtx)
		o.busCtx.Mutable.SetResult(out.Error)
		// Commit 42 P0: persist AppliedPaths subset on
		// partially_applied so /plan show can render which
		// files landed vs which didn't. Empty / fully-failed
		// applies leave the field untouched.
		var appliedPaths []string
		if status == types.PlanStatusPartiallyApplied {
			appliedPaths = o.collectAppliedTargetPaths()
		}
		o.persistPlanStatusWithApplied(status, nil, appliedPaths)
		return nil
	}
	plan := o.busCtx.Mutable.ChangePlan()
	if plan == nil {
		return nil
	}
	applied := o.busCtx.Mutable.WriteClosure().AppliedSet()
	o.busCtx.Mutable.SetResult(renderApplySummary(plan, applied, o.busCtx.WorktreePath,
		worktree.AppliedRef(plan.ID), o.keepWorktreeOnSuccess || o.skipVerify, o.busCtx.Language))
	logging.Info("[orchestrator] apply stage: completed, %d/%d changes applied",
		len(applied), len(plan.TargetPaths))
	o.persistCurrentChangePlanSnapshot()
	// Warm-worktree retry checkpoint: commit the applied content as a
	// git commit inside the worktree, capture the HEAD SHA, and stash
	// it on the orchestrator. If this iteration turns out to be the
	// best (clearForReplan promotes it), the SHA becomes the rewind
	// target for subsequent retries. Failures here log a warning and
	// degrade gracefully — the fallback is the original "discard +
	// reset to main" path in clearForReplan, which still works
	// correctly without the SHA, just less effectively.
	if o.busCtx.WorktreePath != "" && plan.ID != "" {
		sha, err := worktree.CommitChanges(o.busCtx.WorktreePath, applyCommitMessage(plan))
		if err != nil {
			logging.Warning("[orchestrator] apply post-hook: git commit (warm-retry checkpoint) failed: %v", err)
		} else {
			o.currentIterCommitSHA = sha
			logging.Debug("[orchestrator] apply post-hook: checkpoint committed at %s", sha)
			// Pin the apply commit in the main repo's ref namespace
			// at refs/codrax/applied/<plan-id>. The worktree's HEAD
			// is detached; without a ref pointing to this commit,
			// `git gc` can prune it after worktree.Discard. The ref
			// is the keep_on_success=false recovery surface — /merge
			// falls back to it when no preserved worktree exists,
			// and the operator can manually `git cherry-pick
			// refs/codrax/applied/<id>` from the main repo even
			// after the worktree directory is destroyed.
			if o.busCtx.MainRepoRoot != "" {
				if ref, terr := worktree.TagAppliedCommit(o.busCtx.MainRepoRoot, plan.ID, sha); terr != nil {
					logging.Warning("[orchestrator] apply post-hook: tag recovery ref failed: %v", terr)
				} else {
					logging.Info("[orchestrator] apply post-hook: tagged %s = %s (recovery ref for /merge)",
						ref, sha)
				}
			}
			// Persist SHA on the plan JSON so /merge can read it
			// without scanning git refs. PlanPath is empty in plan-
			// generation mode; in /approve dispatch it points at the
			// reviewed plan file. Best-effort: log on failure, the
			// ref above is still recoverable.
			if o.busCtx.PlanPath != "" {
				plan.AppliedCommitSHA = sha
				o.busCtx.Mutable.SetChangePlan(plan)
				o.persistCurrentChangePlanSnapshot()
			}
		}
	}
	return nil
}

// verifyPreHook prepares the verify stage. Two responsibilities:
//
//  1. Load the ChangePlan from disk when ModeVerify runs standalone
//     (`--mode=write --write-phase=verify --plan-file=X` or REPL `/verify <id>`). In
//     ModeApply the plan is already on Mutable from the apply pre-
//     hook or the planner stage; only standalone verify needs to
//     hydrate it. Skipped when ChangePlan is already set.
//
//  2. Swap busCtx.RepoRoot to a preserved worktree path when
//     orchestrator.SetReuseWorktreePath was called (REPL `/verify
//     <plan-id>` against a worktree preserved by
//     pipeline_keep_worktree_on_success). Without the swap, run_tests
//     would execute against the main repo HEAD instead of the
//     applied-bytes worktree — defeating the re-verify intent.
//
// The reused path is NOT written to busCtx.WorktreePath so the outer
// Run() cleanup defer leaves the preserved tree alone.
func verifyPreHook(o *Orchestrator) error {
	if o == nil || o.busCtx == nil {
		return nil
	}
	if o.busCtx.Mutable.ChangePlan() == nil && o.busCtx.PlanPath != "" {
		plan, err := types.LoadChangePlanFromFile(o.busCtx.PlanPath)
		if err != nil {
			msg := fmt.Sprintf("verify stage: load plan file failed: %v", err)
			o.busCtx.Mutable.SetResult(msg)
			return fmt.Errorf("%s", msg)
		}
		o.busCtx.Mutable.SetChangePlan(plan)
		logging.Info("[orchestrator] verify pre-hook: loaded plan %s from %s", plan.ID, o.busCtx.PlanPath)
		// CLI auto-pickup: when the loaded plan has a WorktreePath
		// recorded (preserved via pipeline_keep_worktree_on_success
		// during the original apply), and the orchestrator wasn't
		// explicitly told to reuse a different path, use the plan's
		// own recorded worktree. This makes `--mode=write --write-phase=verify
		// --plan-file=X` work the same way as REPL `/verify <id>`
		// without forcing the CLI user to discover the worktree
		// path manually.
		if o.reuseWorktreePath == "" && plan.WorktreePath != "" {
			o.reuseWorktreePath = plan.WorktreePath
			logging.Info("[orchestrator] verify pre-hook: auto-reusing plan's recorded worktree %s", plan.WorktreePath)
		}
	}
	// Reuse-worktree swap. Stat the path to fail-loud when the user
	// supplies a stale plan whose preserved worktree was already
	// cleaned up by the next-startup reap.
	if o.reuseWorktreePath != "" {
		info, err := os.Stat(o.reuseWorktreePath)
		if err != nil || !info.IsDir() {
			msg := fmt.Sprintf("verify stage: preserved worktree %q missing or not a directory (was it discarded?)", o.reuseWorktreePath)
			o.busCtx.Mutable.SetResult(msg)
			return fmt.Errorf("%s", msg)
		}
		o.busCtx.RepoRoot = o.reuseWorktreePath
		o.busCtx.Mutable.SetRepoRoot(o.reuseWorktreePath)
		logging.Info("[orchestrator] verify pre-hook: swapped RepoRoot to preserved worktree %s", o.reuseWorktreePath)
	}
	return nil
}

// verifyPostHook persists the ChangeReport to disk + flips the plan
// status. On verify pass: PlanStatusApplied + AppliedAt timestamp.
// On verify fail: PlanStatusVerifyFailed (status field only). Result
// rendering uses renderVerifySuccess (appends to apply summary) for
// pass and renderVerifyFailure for fail.
func verifyPostHook(o *Orchestrator, out *agent.StageOutput) error {
	if o == nil || o.busCtx == nil {
		return nil
	}
	report := o.busCtx.Mutable.ChangeReport()
	if report != nil {
		o.saveChangeReport(report)
		pack := types.WriteContextPackFromChangeReport(report)
		if len(pack.Items) > 0 {
			o.busCtx.Mutable.MergeWriteContextPack(pack)
		}
		// Append a fingerprint per verify round so the write-scheduler
		// retry decision can spot "no change in any signal" stalls.
		// Adjacent identical fingerprints across (AppliedCount,
		// VerifyPassed, VerifyFailed, FailureSummaryHash) mean the
		// current iteration produced the exact same outcome as the
		// previous one — further retry burns LLM budget on a problem
		// the planner can't structurally fix.
		appendVerifyFingerprint(o.busCtx, report)
	}
	if out != nil && out.Error != "" {
		o.busCtx.Mutable.SetResult(renderVerifyFailure(report, out.Error, o.busCtx.Language))
		o.persistPlanStatus(types.PlanStatusVerifyFailed, nil)
		return nil
	}
	// Verify ran cleanly. Decide between PlanStatusApplied (tests
	// actually verified the change) and PlanStatusUnverified (runner
	// reported zero tests for the changed code AND the plan
	// modified non-test files). The decision reads typed structured
	// signals only:
	//   - report.NoTestsRunners — set by the run_tests parser when
	//     a runner exited successfully but discovered zero tests.
	//   - WriteAnalysisIR.Request.Task.Kind — set by write_analyzer
	//     to characterise the user's intent. WriteTaskTest /
	//     WriteTaskDocs / WriteTaskConfig are benign no-test
	//     scenarios (the user wanted to add docs / tests / config —
	//     "0 tests for that" is not a problem); fall back to
	//     plan.Changes inspection when WriteAnalysisIR is absent.
	existing := o.busCtx.Mutable.Result()
	if report != nil && len(report.NoTestsRunners) > 0 && planTouchesNonTestCode(o.busCtx) {
		o.busCtx.Mutable.SetResult(existing + renderVerifyUnverified(report, o.busCtx.Language))
		now := time.Now()
		o.persistPlanStatus(types.PlanStatusUnverified, &now)
		return nil
	}
	o.busCtx.Mutable.SetResult(existing + renderVerifySuccess(report, o.busCtx.Language))
	now := time.Now()
	o.persistPlanStatus(types.PlanStatusApplied, &now)
	return nil
}

// planTouchesNonTestCode reports whether the active ChangePlan
// modifies at least one file that is not a test or test-only asset.
// Drives the unverified decision in verifyPostHook: a doc / config /
// test-only plan that produces zero tests is benign (nothing was
// supposed to be tested), but a feature / bugfix that produces zero
// tests means the change shipped without verification.
//
// Decision precedence:
//
//  1. If WriteAnalysisIR is on Mutable and Task.Kind is one of the
//     "no tests expected" categories (test / docs / config), return
//     false — the plan itself was about non-runtime code.
//  2. Otherwise scan plan.Changes for paths that look like
//     production source. Test-file recognition uses well-known
//     ESTABLISHED testing conventions across languages — these are
//     not keyword heuristics, they are the canonical filename
//     patterns the test runners themselves use to discover tests.
func planTouchesNonTestCode(busCtx *types.BusContext) bool {
	if busCtx == nil || busCtx.Mutable == nil {
		return false
	}
	if ir := busCtx.Mutable.WriteAnalysisIR(); ir != nil {
		switch ir.Request.Task.Kind {
		case types.WriteTaskTest, types.WriteTaskDocs, types.WriteTaskConfig:
			return false
		}
	}
	plan := busCtx.Mutable.ChangePlan()
	if plan == nil {
		// No plan visible — defensive default: treat as production.
		// verifyPostHook is only called in write mode where a plan
		// would normally exist; this branch only fires under bizarre
		// test fixtures.
		return true
	}
	for _, c := range plan.Changes {
		if c.Kind == "delete" {
			continue
		}
		if !isTestPath(c.Path) {
			return true
		}
	}
	return false
}

// isTestPath returns true when path matches one of the canonical
// test-file conventions test runners themselves use for discovery.
// Sourced from the run_tests.go runner manifest commands —
// each runner's discovery pattern is reflected here exactly:
//
//   - Go: *_test.go (go test)
//   - Python: test_*.py, *_test.py, *_tests.py (pytest)
//   - Java/Kotlin: *Test.java / *Tests.java / *Test.kt / *Spec.kt
//   - Ruby: *_spec.rb (rspec)
//   - JS/TS: *.test.{js,ts,jsx,tsx,mjs} / *.spec.{js,ts,...}
//   - Rust: tests/*.rs (cargo test integration tests)
//   - Swift: *Tests.swift, *Test.swift
//   - C/C++: tests/*, test/* (CMake/Meson convention)
//
// Files inside top-level "tests/", "test/", "spec/", "__tests__/"
// directories are also test-shaped regardless of extension.
func isTestPath(path string) bool {
	p := strings.ToLower(strings.ReplaceAll(path, "\\", "/"))
	if p == "" {
		return false
	}
	for _, dir := range []string{"tests/", "test/", "spec/", "__tests__/"} {
		if strings.HasPrefix(p, dir) || strings.Contains(p, "/"+dir) {
			return true
		}
	}
	base := p
	if i := strings.LastIndex(p, "/"); i >= 0 {
		base = p[i+1:]
	}
	switch {
	case strings.HasSuffix(base, "_test.go"),
		strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"),
		strings.HasSuffix(base, "_test.py"),
		strings.HasSuffix(base, "_tests.py"),
		strings.HasSuffix(base, "test.java"),
		strings.HasSuffix(base, "tests.java"),
		strings.HasSuffix(base, "test.kt"),
		strings.HasSuffix(base, "tests.kt"),
		strings.HasSuffix(base, "spec.kt"),
		strings.HasSuffix(base, "_spec.rb"),
		strings.HasSuffix(base, ".test.js"),
		strings.HasSuffix(base, ".test.ts"),
		strings.HasSuffix(base, ".test.jsx"),
		strings.HasSuffix(base, ".test.tsx"),
		strings.HasSuffix(base, ".test.mjs"),
		strings.HasSuffix(base, ".spec.js"),
		strings.HasSuffix(base, ".spec.ts"),
		strings.HasSuffix(base, ".spec.jsx"),
		strings.HasSuffix(base, ".spec.tsx"),
		strings.HasSuffix(base, ".spec.mjs"),
		strings.HasSuffix(base, "tests.swift"),
		strings.HasSuffix(base, "test.swift"):
		return true
	}
	return false
}

// appendVerifyFingerprint snapshots the current verify-round signal
// onto WriteClosure.fingerprints. The retry detector reads these
// to compare adjacent rounds; a no-progress signal triggers
// retry-suppression in the write scheduler. Hash is FNV-32 over
// the trimmed FailureSummary so the in-memory fingerprint stays
// small regardless of stderr length.
func appendVerifyFingerprint(busCtx *types.BusContext, report *types.ChangeReport) {
	if busCtx == nil || busCtx.Mutable == nil || report == nil {
		return
	}
	wc := busCtx.Mutable.WriteClosure()
	if wc == nil {
		return
	}
	passed, failed := 0, 0
	for _, r := range report.TestResults {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(strings.TrimSpace(report.FailureSummary)))
	applied := len(busCtx.Mutable.WriteClosure().AppliedSet())
	wc.AppendFingerprint(types.ApplyVerifyFingerprint{
		AppliedCount:       applied,
		VerifyPassed:       passed,
		VerifyFailed:       failed,
		FailureSummaryHash: fmt.Sprintf("%x", hash.Sum32()),
		Timestamp:          time.Now(),
	})
}

// clearForReplan resets write-mode Mutable state between verify→plan
// retry iterations and seeds Mutable.PlanningHint with the previous
// failure narrative so the planner's next dispatch incorporates the
// retry rationale.
//
// Called by the write scheduler when verify SuccessCriteria fails
// AND retry budget remains. The flow:
//
//  1. Capture prevReport + prevPlan BEFORE any reset wipes them —
//     they're needed to build the planning hint and to drive the
//     best-known-good latch decision.
//  2. Append a verbatim row to Mutable.IterationLedger so the
//     planner's next dispatch sees the full retry history.
//  3. Update the best-known-good (plan, report) latch when this
//     iteration improved the score — bestAppliedCommitSHA is also
//     pinned to the running-best commit so a subsequent regression
//     can warm-rewind back to it instead of starting from stub.
//  4. Build the heuristic hint from the failure summary + suspect
//     paths + best-vs-current delta. Optionally enrich with a
//     reflector-LLM critique paragraph (silent fall-back to
//     heuristic-only when the reflector is absent or errors).
//  5. Worktree handling — preferred: warm rewind via
//     `git reset --hard <bestSHA>` so the LLM iterates on top of
//     the running best. Fall back: cold discard + restore RepoRoot
//     to MainRepoRoot when no best SHA is available.
//  6. Reset ChangePlan + ChangeReport. WriteClosure reset has two
//     flavours: warm rewind keeps appliedSet aligned with disk via
//     ResetExceptApplied + reseed from bestPlan.TargetPaths; cold
//     discard wipes the closure entirely.
//  7. Clear o.planPath / busCtx.PlanPath so a user-supplied
//     --plan-file no longer short-circuits the next plan dispatch.
//  8. Install the assembled hint on Mutable.PlanningHint so the
//     planner's BuildInitialInstruction picks it up via consume-once.
//
// Note: the per-node transient-stall bookkeeping (transientStallSignatures
// + transientNoEmitStreak) is wiped by the caller in
// write_scheduler.go just before invoking this function, not here.
func clearForReplan(o *Orchestrator, attempt int) {
	if o == nil || o.busCtx == nil {
		return
	}
	prevReport := o.busCtx.Mutable.ChangeReport()
	prevPlan := o.busCtx.Mutable.ChangePlan()

	// Module C: append a row to the iteration ledger BEFORE any of
	// the resets below clear our source data. The planner's next
	// dispatch reads the full ledger via Mutable.IterationLedger()
	// and decides what to try based on the running history — no
	// system pre-classification. Verbatim PlanSummary +
	// FailureSummary (no truncation); blob ref propagated when the
	// runner blobbed the full stderr.
	if o.busCtx.Mutable != nil {
		o.busCtx.Mutable.AppendIteration(buildIterationRecord(attempt, prevPlan, prevReport))
	}

	// Best-known-good latch: track the highest-passing (plan, report)
	// pair across retry iterations so a regression in iteration N+1
	// (LLM "fixed" the code but introduced a new failure) does not
	// permanently lose ground that iteration N had won.
	//
	// Bug provenance: eval Batch K forth-py — three retry iterations
	// scored 49/54 → 46/54 → 46/54. Each iteration unconditionally
	// overwrote the previous plan, so the final result locked in the
	// regression even though iteration 1 had a strictly-better plan
	// in hand.
	//
	// On every retry decision we (a) update the best slot if this
	// iteration improved on it, and (b) seed PlanningHint with a
	// "current vs best" delta so the planner's next dispatch sees
	// what it just lost. The actual restoration of the best plan
	// happens at end-of-budget (selectFinalReport, called from Run)
	// rather than mid-loop, so the LLM gets a real chance to fix the
	// regression before we fall back. Mid-loop reset would defeat
	// the retry budget.
	bestPlan, bestReport := o.busCtx.Mutable.BestPlanReport()
	if prevReport.IsBetterThan(bestReport) {
		o.busCtx.Mutable.SetBestPlanReport(prevPlan, prevReport)
		bestPlan, bestReport = prevPlan, prevReport
		// Pin the warm-worktree rewind target to this iteration's
		// commit SHA. Subsequent retries that regress will be reset
		// back to this SHA before the planner re-dispatches, so the
		// LLM iterates from the running best, not the original stub.
		// Empty SHA (apply commit failed for some reason) leaves the
		// previous best SHA intact — a stale-but-still-best target
		// is better than no target at all.
		if o.currentIterCommitSHA != "" {
			o.bestAppliedCommitSHA = o.currentIterCommitSHA
		}
		// Persist the new best to disk so a process crash mid-retry
		// does not lose the high-water mark. <plan-dir>/<plan-id>.best.json.
		// Failure is non-fatal — the in-memory latch is still
		// authoritative for the rest of the Run; the disk copy is
		// strictly for crash recovery on the next Run.
		if o.busCtx.PlanPath != "" && bestPlan != nil && bestReport != nil {
			if err := types.WriteBestPlanReportPair(bestPlan, bestReport, o.busCtx.PlanPath); err != nil {
				logging.Warning("[orchestrator] best-plan disk persist failed: %v", err)
			}
		}
	}

	heuristicHint := buildRetryHintWithBest(prevReport, prevPlan, bestReport, bestPlan, attempt)

	// Reflexion-pattern critic (optional). When configured, dispatch
	// one side LLM call to interpret the failure as a critique
	// paragraph; planner sees critique + heuristic facts together.
	// Any reflector failure path silently degrades to heuristic-only
	// — retry MUST NOT be blocked by reflection plumbing.
	hint := heuristicHint
	if o.reflector != nil {
		input := buildReflectorInput(o.busCtx, prevReport, prevPlan, attempt)
		out, err := o.reflector.ReflectFull(o.busCtx.Context(), input)
		if err != nil {
			// Commit 59 Batch E.1 (audit HIGH #12): record + log so
			// the Run-end summary surfaces broken learning.
			if mu := o.busCtx.Mutable; mu != nil {
				mu.AppendLearningFailure("reflector", err.Error())
			}
			logging.Warning("[orchestrator] reflector failed (degrading to heuristic hint): %v", err)
		} else if out != nil {
			critique := out.Observation
			if strings.TrimSpace(critique) != "" {
				// When the critic emitted typed preservation_clauses
				// naming aspects of the previous plan that should be
				// kept, the heuristic's "the regression is in the
				// edits to these files" framing directly contradicts
				// it. Soften the heuristic suspect list to a neutral
				// "files modified" so the planner does not receive
				// contradictory orders in the same prompt.
				//
				// Keyword-gate audit HIGH-2 (2026-05-17): the gate
				// reads the typed slice len(out.PreservationClauses)
				// instead of grepping the LLM-prose Observation for
				// the substring "Preserve:" — a CLAUDE.md "precise
				// signals for hard gates" red-line requirement.
				heuristic := softenSuspectListForPreservation(heuristicHint, len(out.PreservationClauses) > 0)
				hint = critique + "\n\n" + heuristic
				// Block 1 (architecture overhaul 2026-05-02) — also fold
				// the per-iteration Observation into the EvidenceClosure
				// ledger so Block 1's stage-wise summary, Block 3's
				// fallback policy, and the end-of-Run health snapshot
				// see reflector findings the same way they see every
				// other gate's findings. The Observation also flows
				// into the planner via `hint` (PlanningHint above) —
				// this is the LLM-facing channel; the closure write is
				// the system-facing channel. Two-track design preserves
				// the byte-identical Reflexion behaviour while making
				// the signal observable to non-LLM consumers.
				appendReflectorObservationToClosure(o.busCtx.Mutable, critique, attempt)
			}
			// Stage 3: persist the reusable abstract pitfall
			// when the reviewer distilled one. Append handles
			// dedup + decay + sweep; failure of the persist is
			// non-fatal (the in-memory critique still drives
			// THIS retry, only the cross-Run learning is
			// affected).
			if out.Pattern != nil && o.failureTaxonomyStore != nil {
				if saved, perr := o.failureTaxonomyStore.Append(out.Pattern); perr != nil {
					logging.Warning("[orchestrator] failure_taxonomy persist failed: %v", perr)
				} else if saved != nil {
					logging.Info("[orchestrator] failure_taxonomy: persisted pattern %s (hits=%d)",
						saved.ID, saved.HitCount)
				}
			}
		}
	}
	// Worktree handling — two paths:
	//
	//  Warm rewind (preferred): when bestAppliedCommitSHA is set, the
	//  existing worktree gets `git reset --hard <bestSHA>` so its
	//  contents match the best iteration's applied state. RepoRoot
	//  stays pointed at the worktree so the planner's read_file calls
	//  surface the BEST code as the planner's working baseline. The
	//  next iteration's coder applies its plan ON TOP, converting the
	//  retry budget from "N independent from-stub attempts" into "N
	//  iterative refinements on running best".
	//
	//  Cold discard (fallback): when no best SHA is available (apply
	//  never committed, or commit-checkpoint failed), or when the
	//  rewind itself errors, we fall back to the historical behavior:
	//  discard the worktree, reset RepoRoot to MainRepoRoot, fresh
	//  worktree on next apply pre-hook. Still correct, just less
	//  effective for retry iteration.
	rewound := false
	if o.busCtx.WorktreePath != "" && o.bestAppliedCommitSHA != "" {
		if err := worktree.ResetHard(o.busCtx.WorktreePath, o.bestAppliedCommitSHA); err != nil {
			logging.Warning("[orchestrator] warm-retry rewind failed (falling back to cold discard): %v", err)
		} else {
			rewound = true
			logging.Info("[orchestrator] warm-retry rewind: worktree reset to best SHA %s", o.bestAppliedCommitSHA)
		}
	}
	if !rewound {
		if o.busCtx.WorktreePath != "" {
			if err := worktree.DiscardByPath(o.busCtx.WorktreePath, o.busCtx.MainRepoRoot); err != nil {
				logging.Warning("[orchestrator] retry worktree discard failed: %v", err)
			}
			o.busCtx.WorktreePath = ""
		}
		if o.busCtx.MainRepoRoot != "" {
			o.busCtx.RepoRoot = o.busCtx.MainRepoRoot
			o.busCtx.Mutable.SetRepoRoot(o.busCtx.MainRepoRoot)
		}
	}
	o.busCtx.Mutable.ResetChangePlan()
	o.busCtx.Mutable.ResetChangeReport()
	// WriteClosure reset has two flavours, picked by the rewind path:
	//  - Warm rewind succeeded: the worktree disk state carries the
	//    files from the best plan's apply. AppliedSet MUST stay in sync
	//    with that disk state so apply_patch's idempotent path (which
	//    reads AppliedSet) engages on a re-emit and short-circuits the
	//    "file already exists in worktree" rejection. Reseed
	//    AppliedSet from the rewound plan's TargetPaths because, by
	//    construction, that's exactly what the rewound commit holds.
	//  - Cold discard: the worktree was thrown away; AppliedSet must
	//    revert to empty so the next plan starts from a clean disk.
	if rewound && bestPlan != nil && len(bestPlan.TargetPaths) > 0 {
		o.busCtx.Mutable.WriteClosure().ResetExceptApplied()
		// Reseed AppliedSet from the rewound plan's TargetPaths. The
		// warm rewind landed at bestSHA, which was committed by
		// applyPostHook AFTER bestPlan's apply succeeded — so every
		// path in bestPlan.TargetPaths is on disk in the worktree right
		// now and apply_patch must treat them as already-applied to
		// keep idempotency aligned with disk.
		closure := o.busCtx.Mutable.WriteClosure()
		for _, p := range bestPlan.TargetPaths {
			closure.MarkApplied(p)
		}
	} else {
		o.busCtx.Mutable.WriteClosure().Reset()
	}
	o.planPath = ""
	o.busCtx.PlanPath = ""
	// Multi-phase carry-through: when a phase is in flight, the
	// orchestrator pinned a "## Phase X of Y: <goal>" header onto
	// o.phaseContextPrefix at phase entry. PlanningHint itself is
	// consume-once and was already drained by the first planner
	// dispatch, so without this prepend the retry's planner would
	// see only the failure critique and lose the phase boundary
	// it should still be working within. Single-phase Runs leave
	// phaseContextPrefix empty, so this is a no-op there.
	if o.phaseContextPrefix != "" {
		hint = o.phaseContextPrefix + hint
	}
	o.busCtx.Mutable.SetPlanningHint(hint)
	logging.Info("[orchestrator] verify→plan retry attempt %d: hint=%q", attempt, hint)
}

// restoreBestIfRegressed swaps Mutable.ChangePlan + Mutable.ChangeReport
// to the best-known-good pair captured across retry iterations IFF
// the current pair scored strictly worse than the best. Called from
// the verify→plan retry loop's terminal-failure path so the user sees
// the highest-passing plan on disk + in the result, not the last-
// (worse) one a regressed retry produced.
//
// On success path (current is best, current passed all tests, etc.)
// this is a no-op — bestReport.IsBetterThan(curReport) returns false
// and we leave Mutable untouched. Re-persists the report file when
// swap actually occurs so .codrax/plans/<id>.report.json reflects the
// restored pair.
//
// Bug provenance: eval Batch K forth-py — see clearForReplan
// commentary above.
func restoreBestIfRegressed(o *Orchestrator) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	curReport := o.busCtx.Mutable.ChangeReport()
	bestPlan, bestReport := o.busCtx.Mutable.BestPlanReport()
	if bestReport == nil || !bestReport.IsBetterThan(curReport) {
		return
	}
	bp, bt := bestReport.Score()
	cp, ct := -1, -1
	if curReport != nil {
		cp, ct = curReport.Score()
	}
	logging.Warning("[orchestrator] retry-budget exhausted: restoring best-known-good plan (best=%d/%d > current=%d/%d)",
		bp, bt, cp, ct)
	o.busCtx.Mutable.SetChangePlan(bestPlan)
	o.busCtx.Mutable.SetChangeReport(bestReport)
	// Re-render Mutable.Result so the user-facing summary reflects
	// the restored report, not the last-iteration verifyPostHook
	// rendering. Otherwise the user sees "Verify FAILED 0/54" while
	// the in-memory + on-disk state actually carries 51/54 — pure
	// stale-UI confusion that defeats Fix 1's value.
	//
	// Render shape mirrors verifyPostHook: when the restored report
	// passed, render a success banner; otherwise render the failure
	// summary so the user can see the (better but still failing)
	// state of the best iteration.
	if bestReport.Passed {
		o.busCtx.Mutable.SetResult(renderVerifySuccess(bestReport, o.busCtx.Language))
		now := time.Now()
		o.persistPlanStatus(types.PlanStatusApplied, &now)
	} else {
		o.busCtx.Mutable.SetResult(renderVerifyFailure(bestReport, "", o.busCtx.Language))
		o.persistPlanStatus(types.PlanStatusVerifyFailed, nil)
	}
	// Re-persist the report so the on-disk artifact reflects what is
	// now in Mutable. saveChangeReport now honors o.reportDir as a
	// fallback when busCtx.PlanPath is empty (see saveChangeReport
	// commentary).
	o.saveChangeReport(bestReport)
	// Sync the worktree contents to match the restored best plan so
	// the on-disk state the user inspects (or that
	// pipeline_keep_worktree_on_success preserves) matches the
	// in-memory + disk-report data. Without this, the worktree could
	// still hold the regressed last-iteration's content while the
	// rest of the plan/report state had been rolled back. Best-effort
	// — failures log a warning.
	if o.busCtx.WorktreePath != "" && o.bestAppliedCommitSHA != "" {
		if err := worktree.ResetHard(o.busCtx.WorktreePath, o.bestAppliedCommitSHA); err != nil {
			logging.Warning("[orchestrator] restoreBestIfRegressed: worktree rewind to best SHA failed: %v", err)
		} else {
			logging.Info("[orchestrator] restoreBestIfRegressed: worktree reset to best SHA %s", o.bestAppliedCommitSHA)
		}
	}
}

// buildReflectorInput translates the failed iteration's structured
// state into a ReflectorInput. Module G: every failing test's
// FailureDetail is passed VERBATIM (no ExtractFailureSignal call,
// no 600-char cap) — the reviewer is a model and reads what the
// runner emitted. The IterationLedger is also threaded through so
// the reviewer can observe patterns across attempts; the system
// never pre-classifies the patterns.
func buildReflectorInput(busCtx *types.BusContext, report *types.ChangeReport, plan *types.ChangePlan, attempt int) ReflectorInput {
	in := ReflectorInput{Attempt: attempt}
	if busCtx != nil && busCtx.Mutable != nil {
		in.OriginalRequest = strings.TrimSpace(busCtx.Mutable.Objective())
		in.BaselineAvailable = busCtx.Mutable.BaselineReport() != nil
		in.IterationLedger = busCtx.Mutable.IterationLedger()
		// Commit 7 P1-F gap-fix: feed the reviewer the user's
		// task-shape framing so observations can call out
		// outcome-vs-test alignment instead of just stderr-vs-plan.
		if ir := busCtx.Mutable.WriteAnalysisIR(); ir != nil {
			in.TaskSummary = ir.Request.Task.Summary
			in.ExpectedOutcomes = append(in.ExpectedOutcomes, ir.Request.ExpectedOutcomes...)
		}
	}
	if plan != nil {
		in.PlanSummary = plan.Summary
		in.TargetPaths = append([]string(nil), plan.TargetPaths...)
		in.AcceptanceTests = append([]string(nil), plan.AcceptanceTests...)
	}
	if report != nil {
		in.FailureSummary = report.FailureSummary
		in.BuildFailed = report.BuildFailed
		for _, tr := range report.TestResults {
			if tr.Passed {
				continue
			}
			in.FailingTests = append(in.FailingTests, ReflectorFailedTest{
				Suite:       tr.Suite,
				AssertionID: tr.AssertionID,
				Detail:      tr.FailureDetail, // verbatim
			})
		}
	}
	return in
}

// softenSuspectListForPreservation rewrites the heuristic suspect-
// list framing when the reflector emitted typed preservation
// clauses. The pre-audit (2026-05-17) implementation grep'd the
// reflector's free-form Observation prose for the substring
// "Preserve:" — a CLAUDE.md "precise signals for hard gates"
// red-line violation (HIGH-2 in docs/design/keyword_gate_audit_
// 2026_05_17.md): a wording polish or a Chinese critique that
// said "保留" silently bypassed the rewrite, and a critique that
// quoted "Preserve:" inside an unrelated code example falsely
// triggered it.
//
// The replacement gate reads the typed
// ReflectorOutput.PreservationClauses slice in the caller and
// hands the boolean "any preservation clause emitted" to this
// helper. The slice's content is informational (planner reads the
// observation directly); the gate cares only about emit-vs-absent.
//
// Pure function — no orchestrator state — so it can be unit-tested
// without mocking the reflector LLM call path.
func softenSuspectListForPreservation(heuristic string, hasPreservationClause bool) string {
	if !hasPreservationClause {
		return heuristic
	}
	return strings.ReplaceAll(heuristic,
		"Files modified by the previous plan (suspect list — the regression is in the edits to these files):",
		"Files modified by the previous plan (review for compatibility; the critic above identified what to preserve):",
	)
}

// appendReflectorObservationToClosure is the dual-track helper for
// the reflector LLM. The reflector's per-iteration Observation
// already flows into Mutable.PlanningHint (the LLM-facing channel)
// via the caller's `hint` composition; this helper adds the
// system-facing channel — write the same Observation into the
// EvidenceClosure ledger as one ViolReflectorObservation at
// Stage="verify" so:
//
//   - Block 1's StageHealthSnapshot sees the verify-stage activity
//     count grow when reflector fires
//   - Block 3's selective fallback policy can choose BackToExplore /
//     FailLoud based on accumulated reflector observations across the
//     write_retry_budget
//   - end-of-Run summary surfaces "verify retried N times, reflector
//     said X" without re-parsing the PlanningHint string
//
// Soft-by-default classification (cmd/root.go default strict-kinds
// excludes ViolReflectorObservation) preserves the existing apply
// path: even N pessimistic observations cannot block re-plan.
func appendReflectorObservationToClosure(mut *types.MutableState, observation string, attempt int) {
	if mut == nil {
		return
	}
	observation = strings.TrimSpace(observation)
	if observation == "" {
		return
	}
	closure := mut.EvidenceClosure()
	if closure == nil {
		return
	}
	closure.AppendViolation(types.Violation{
		Kind:       types.ViolReflectorObservation,
		ClusterKey: types.RootClusterKey("reflector_observation"),
		Detail:     fmt.Sprintf("reflector observation (verify retry attempt %d): %s", attempt, observation),
		Repair:     "this is an observational note from an independent reviewer LLM; the planner already received it on its retry. No direct action expected.",
		Stage:      string(types.StageVerify),
		SuspectedRoot: types.SuspectedRoot{
			IRField:    "reflector_observation",
			Reason:     "Reflexion-pattern critic distilled per-iteration verify failure",
			Confidence: 0.6,
		},
	})
}

// writeRiskAssessmentInput assembles the typed inputs for write risk
// assessment. The declaration-span source rides on the repository graph when
// one is loaded for this Run; without it the assessor degrades to the softer
// medium-grade analyzer axes (nil-safe by contract).
func (o *Orchestrator) writeRiskAssessmentInput(plan *types.ChangePlan) writeflow.AssessmentInput {
	input := writeflow.AssessmentInput{Plan: plan}
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return input
	}
	if g, ok := o.busCtx.Mutable.SearchGraph().(*repotypes.Graph); ok && g != nil {
		input.Decls = repomap.NewDeclSpanSource(g)
	}
	return input
}

// applyCommitMessage builds the checkpoint commit message. The commit
// is what `git cherry-pick refs/codrax/applied/<id>` lands on the
// user's branch, so its subject is the plan's own summary (first
// line, bounded) instead of machine bookkeeping; the plan id rides as
// a trailer for provenance. An empty summary falls back to the
// legacy machine form.
func applyCommitMessage(plan *types.ChangePlan) string {
	if plan == nil {
		return "codrax apply iter"
	}
	summary := strings.TrimSpace(plan.Summary)
	if idx := strings.IndexByte(summary, '\n'); idx >= 0 {
		summary = strings.TrimSpace(summary[:idx])
	}
	const subjectCap = 72
	if summary != "" {
		runes := []rune(summary)
		if len(runes) > subjectCap {
			summary = string(runes[:subjectCap-1]) + "…"
		}
		return summary + "\n\nplan: " + plan.ID
	}
	return "codrax apply iter (plan=" + plan.ID + ")"
}

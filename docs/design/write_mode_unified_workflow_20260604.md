# Write Mode Unified Workflow

Date: 2026-06-04

## Problem Ledger

Recent audits of write mode show three structural gaps:

1. Approval semantics are still mostly binary. `write_enabled` gates whether write
   mode may exist, while `--auto-apply` and `/approve` gate execution. There is
   no write-lane equivalent of the operation lane's typed auto/manual/deny
   decision.
2. The write scheduler still centers on a single `ChangePlan` flowing through
   `plan -> apply -> verify`. Verify failure can requeue `plan`, but the loop is
   reactive after a failed batch, not goal-driven before each batch.
3. Multi-phase write mode exists, but it depends on an up-front phase proposal
   and each phase still runs the same single-plan chain. It does not progressively
   expand the next batch based on fresh exploration, verification, risk, and
   acceptance signals.

These gaps are architecture-level. They should not be patched with keyword
checks, prose parsing, or more `/approve` conditionals.

## Current Code Audit

### Fixed write graph

`BuildWriteTaskGraph` builds a deterministic plan/apply/verify graph. The
comments explicitly describe it as a fixed 3-node chain with no risk matrix:

- plan emits `CritPlanReady`
- apply consumes `CritPlanReady` and must satisfy `CritPatchApplies`
- verify consumes `CritPatchApplies` and must satisfy `CritTestsPass` and
  `CritNoRegression`
- verify can requeue plan/apply through validation feedback

This is useful and should remain as the inner batch executor.

### Reactive retry

`runWriteSchedulerLoop` already handles verify failure by clearing mutable
state, preserving iteration history, and requeueing plan/apply while the retry
budget remains. This is valuable retry infrastructure, but it is still
post-failure recovery. It does not decide whether the next batch should explore,
wait for approval, narrow scope, or stop before applying.

### Planner shape

The planner installs one `ChangePlan` on `Mutable`. It can use the newer
`emit_plan_skeleton` + `emit_plan_change` path for large payloads, but the end
state remains one batch of file changes. The coder is intentionally a mechanical
marshaller that applies the installed plan, which is good; it means richer
workflow control belongs above the coder, not inside it.

### Existing multi-phase group

`PlanGroup` can run sequential phases. This is the closest existing asset to the
desired workflow. However, a group is currently created from a static phase
proposal and each phase is still implemented by building a fresh single-phase
write task graph. It should be evolved into a workflow controller rather than
discarded.

### Operation-lane precedent

The operation lane now has two patterns write mode should copy conceptually:

- a single typed approval decision (`auto_execute`, `manual`, `deny`)
- a single typed evaluator verdict (`complete`, `continue_*`,
  `needs_approval`, `needs_clarification`, `blocked`, `budget_exhausted`,
  `partial_answer_possible`)

Write mode must not import operation semantics directly. Code edits have a
different safety surface, but the architecture principle is the same: one source
of truth for approval and one source of truth for "what happens next".

## Red Lines

1. `write_enabled` remains the hard write capability gate. It is not an approval
   policy.
2. Read mode must remain byte-preserved. No changes to read-mode task graph,
   source evidence gates, trace/log analysis, MCP readonly lanes, or repo_map
   behavior.
3. Hard gates may consume only typed fields or deterministic repository facts:
   risk enums, file paths, change kinds, phase status, plan status, test
   verdicts, repo/worktree state, and explicit approval policy.
4. Do not hard-gate on keyword matching user text, model prose, plan summaries,
   rationales, or final-answer text.
5. Dangerous writes must fail before apply. The system should not rely on tests
   to detect safety violations such as outside-repo paths, large deletes,
   disabling tests, or secret writes.
6. Existing worktree blast-radius guarantees remain: apply happens in a
   worktree, main repo bytes are not changed automatically, and cleanup rules
   remain intact.
7. JSON exposed to models must use the existing structured compatibility layer
   or a matching flexible decoder. Invalid-but-repairable model output should be
   normalized; unrecoverable output should produce targeted retry hints.

## Target Architecture

Add a write-lane controller above the current plan/apply/verify inner graph:

```go
type WriteWorkflowPlan struct {
    ID string
    Goal string
    Constraints []WriteConstraint
    AcceptanceCriteria []string
    Tasks []WriteWorkflowTask
    ActiveTaskID string
    Status WriteWorkflowStatus
}

type WriteBatchPlan struct {
    ID string
    WorkflowID string
    TaskID string
    Purpose string
    ExplorationNeeds []string
    TargetPaths []string
    AcceptanceCriteria []string
    Risk WriteRiskAssessment
}

type WriteRiskAssessment struct {
    Level WriteRiskLevel // none|low|medium|high|critical
    Reasons []WriteRiskReason
    RequiresApproval bool
    Deny bool
}

type WriteApprovalDecision struct {
    Action WriteApprovalAction // auto_execute|manual|deny|none
    ReasonCode string
    Reason string
}

type WriteEvaluation struct {
    Status WriteEvaluationStatus
    Reason string
    NextTaskID string
    NextBatchHint string
}
```

The controller drives a goal loop:

1. Build or load `WriteWorkflowPlan`.
2. Select the next task.
3. Run targeted read-only exploration for that task when needed.
4. Build a small `WriteBatchPlan` / `ChangePlan`.
5. Run `DecideWriteApproval`.
6. If auto, run the existing plan/apply/verify graph for this batch.
7. Evaluate the batch result.
8. Continue, ask for approval, ask for user-owned clarification, deny, or finish.

The existing single `ChangePlan` remains the concrete apply unit. The new layer
controls how many ChangePlans are generated and when.

## Approval Semantics

`write_enabled` controls whether write mode can run at all.

`write_approval_policy` controls whether an approved write-mode run advances
automatically:

- `manual`: every batch needs user approval.
- `auto_safe`: low and medium safe batches can auto-run; high requires approval;
  critical is denied.
- `auto_low_only`: only low safe batches auto-run; medium/high require approval;
  critical is denied.

Recommended default after `write_enabled=true`: `auto_safe`, matching the
operation-lane product expectation while still keeping the write capability
explicitly opt-in.

Deterministic risk sources:

- change kind: create / modify / patch / delete
- path class: source, test, docs, config, build, dependency, generated, secret,
  outside repo, ignored file
- scope: single file, package, cross-package, project-wide
- write analysis booleans: affects public API, persistence, build system
- plan shape: number of files, delete count, full-file overwrites, dependency
  manifest changes
- verification surface: no tests, runner missing, baseline failures

Risk rules:

- low: docs, tests, small localized bugfix, small config with no runtime impact.
- medium: multi-file but local change, normal feature work, non-destructive
  config, generated code checked by tests.
- high: public API, persistence, auth/security, build system, dependency
  manifest, large refactor, broad delete, migration, cross-package behavior.
- critical: path outside repo/worktree, destructive delete without recovery,
  secret writes, disabling tests/guards, modifying `.git`, unbounded generated
  rewrite, or any plan that violates worktree boundaries.

## Prompt / Tool Teaching

Planner prompts must teach:

- Do not plan the whole project when the task can be batched.
- Emit the smallest useful next batch.
- Explore before modifying when the target file or API surface is uncertain.
- State acceptance criteria per batch.
- Respect user constraints and existing style.
- Never widen scope silently; if the next batch needs more files, explain the
  scope expansion as typed batch metadata.

Evaluator prompts must teach:

- Passing tests is not always goal completion.
- A failed command/test is not always task failure; it may require another
  targeted batch.
- Ask the user only for user-owned missing inputs.
- If safe source exploration can answer the missing detail, continue exploring.
- Keep final reports clean; raw execution details belong to stage panels.

## JSON Compatibility

Every new model-visible object must use one of these paths:

1. Existing tool structured compatibility (`applyStructuredPayloadCompat`) when
   exposed as a tool.
2. Flexible decoder with string/list/bool/number coercion, matching
   command-operation planner behavior, when used as direct model JSON.

Required tests:

- string booleans
- numeric strings
- stringified arrays
- extra trailing text
- enum aliases normalized to canonical values
- unrecoverable schema failure returns targeted retry hint

## Delivery Tasks

### Batch 1: Design Ledger

- [x] Audit current write graph, scheduler, planner, phase group, and operation
      approval/evaluator precedent.
- [x] Add this design and task ledger.

### Batch 2: Risk And Approval Model

- [x] Add write-specific risk and approval types.
- [x] Implement deterministic `AssessWriteRisk` over `ChangePlan`,
      `WriteAnalysisIR`, and path facts.
- [x] Implement `DecideWriteApproval`.
- [x] Add unit tests for low/medium/high/critical decisions.
- [x] Surface risk/approval summary in plan display only; do not change apply
      behavior yet.

### Batch 3: Workflow And Batch Schema

- [x] Add `WriteWorkflowPlan` and `WriteBatchPlan` types.
- [x] Add schema compatibility coverage with the shared `toolparam` normalizer.
- [x] Add a builder that converts current `WriteAnalysisIR.PhaseProposal` into a
      workflow seed without requiring all future batches to be known.

### Batch 4: Deterministic Write Unified Evaluator

- [x] Evaluate task completion from typed batch status, tests, acceptance checks,
      and risk status.
- [x] Return `continue_explore`, `continue_plan`, `needs_approval`, `complete`,
      `blocked`, `danger_denied`, `rollback_required`, or `budget_exhausted`.
- [x] Add tests for verify failure, no tests, high-risk expansion, critical
      denial, and complete status.

### Batch 5: REPL / CLI Approval Integration

- [x] Add `write_approval_policy` config.
- [x] Keep `write_enabled` as the capability gate.
- [x] Change plan approval UX to show low/medium auto, high manual, critical
      denied.
- [x] Keep `/approve` for manual high-risk and explicit user override paths.
- [x] Preserve existing `--auto-apply` compatibility until the new policy is
      fully wired.

### Batch 6: Batch Workflow Scheduler

- [x] Wrap the existing plan/apply/verify graph as the inner batch executor.
- [x] Add a pre-apply write approval guard for freshly generated retry /
      multi-phase batch plans.
- [ ] Drive multiple batches with `WriteUnifiedEvaluator`.
- [x] Reuse `PlanGroup` persistence where possible.
- [x] Ensure newly generated batches cannot silently apply high/critical risk
      changes without policy approval.
- [x] Ensure each batch can do fresh targeted exploration before planning.
      The planner now receives a typed `## Rolling write workflow` seed derived
      from `WriteAnalysisIR`, and the change-plan skill explicitly scopes each
      dispatch to the current bounded batch before emitting a `ChangePlan`.
- [ ] Ensure failed batches replan narrowly rather than regenerating the whole
      workflow.

### Batch 7: Tests And Eval

- [x] Unit tests for risk, approval, workflow schema, evaluator, JSON repair,
      and planner/skill workflow prompt wiring.
- [ ] E2E tests for one-file bugfix, multi-batch feature, high-risk approval,
      critical denial, verify failure replan, and no-test unverified.
- [ ] Regression tests that read mode, log/trace analysis, source evidence gates,
      and operation mode are unaffected.

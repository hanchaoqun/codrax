# Operation Unified Approval Decision

Date: 2026-06-04

## Problem

Operation approval has accumulated several local decision points:

- command step evaluation in `internal/operation/command.go`;
- plan-level `ApprovalMode` assignment in `BuildCommandOperationPlan`;
- REPL replan auto-continue checks in `internal/repl/repl.go`;
- continuation auto-continue checks;
- renderer copy that says either "will run automatically" or "waiting for approval".

This made similar bugs repeat: a step could be `eligible` while the plan was
`manual`, or an initial plan could auto-run while a safe revised plan stopped at
`/approve`. The latest customer case is a low-risk revised `curl` download/read
plan that still rendered "waiting for approval".

## Root Cause

The operation lane lacks a single source of truth for approval. The current code
mixes three different concepts:

1. **Step safety**: whether each command step is eligible, manual, or denied.
2. **Plan approval**: whether all steps permit auto execution.
3. **Context guard**: whether a replan/continuation may auto-continue after a
   prior round.

`BuildCommandOperationPlan` evaluates (1) and (2), while the REPL replan path
re-evaluates parts of (2) and (3). The replan path also treated any
`side_effects` value as a manual approval trigger, which is too broad for
observation-only effects such as `network_read`, `local_file_read`,
`environment_read`, or `process_read`.

## Red Lines

- Do not affect source-code analysis, log/trace analysis, MCP observation, or
  write-mode ChangePlan pipelines.
- Do not route by user-prose keywords.
- Hard gates must use typed fields: policy flags, plan status, risk enum, step
  approval enum, side-effect enum, workdir equality, and risk rank.
- Keep catastrophic/destructive commands hard-denied.
- Keep higher-risk repair plans and changed-workdir repair plans manual.
- Do not make `write_enabled` relevant to operation approval.

## Design

Add a deterministic approval decision layer in `internal/operation`:

```go
type CommandApprovalPhase string

const (
    CommandApprovalInitial      CommandApprovalPhase = "initial"
    CommandApprovalReplan       CommandApprovalPhase = "replan"
    CommandApprovalContinuation CommandApprovalPhase = "continuation"
)

type CommandApprovalAction string

const (
    CommandApprovalNone        CommandApprovalAction = "none"
    CommandApprovalAutoExecute CommandApprovalAction = "auto_execute"
    CommandApprovalManual      CommandApprovalAction = "manual"
    CommandApprovalDeny        CommandApprovalAction = "deny"
)

type CommandApprovalOptions struct {
    Phase        CommandApprovalPhase
    PreviousPlan *CommandOperationPlan
}

type CommandApprovalDecision struct {
    Action       CommandApprovalAction
    ApprovalMode string
    ReasonCode   string
    Reason       string
}
```

`DecideCommandPlanApproval(policy, plan, opts)` consumes only structured fields:

- non-ready plans return `none`;
- blocked or denied plans return `deny`;
- any denied step returns `deny`;
- any non-eligible step returns `manual`;
- initial/continuation plans auto-run when plan steps are eligible and
  `AutoApprove || AutoLowRisk`;
- replan plans additionally require:
  - same normalized work directory as the previous plan;
  - no risk escalation versus the previous plan;
  - no manual replan side effects.

Manual replan side effects are write/submit/destructive/remote/install classes:

- `local_file_write`, `directory_write`, overwrite/delete variants;
- package install/uninstall;
- `network_submit`, `network_write`, upload;
- `external_system_write`, `remote_exec`, `destructive`.

Observation-only side effects remain auto-eligible when the step itself is
eligible:

- `network_read`;
- `local_file_read` / `file_read`;
- `environment_read`;
- `process_read`;
- `metadata_read`.

This preserves the product rule:

- ordinary structured read/discovery operations continue automatically;
- high-risk or destructive operations are reviewed or denied;
- a repair plan may not silently widen scope or introduce write/submit effects.

## Integration Points

1. `BuildCommandOperationPlan`
   - Build steps as before.
   - Apply `DecideCommandPlanApproval(..., phase=initial)`.

2. `REPL.executeCommandOperationPlanAttempt`
   - Replace `commandReplanCanAutoExecute` local logic with the operation
     decision layer for `phase=replan`.
   - Manualize replan plans using the decision's `ApprovalMode`, not local
     side-effect heuristics.

3. `REPL.maybeReplanCommandOperation`
   - Same decision layer as the main loop to avoid legacy divergence.

4. Continuation plans
   - Use `phase=continuation` instead of raw `ApprovalMode` checks.

5. UX
   - Keep existing localized renderers.
   - The plan body can still show `ApprovalMode`; the decision decides whether
     the plan is rendered as auto-run, manual, or blocked.

## Task Checklist

### Batch 1: Design Ledger

- [x] Audit current approval split.
- [x] Document unified decision design.
- [x] Document red lines and task checklist.

### Batch 2: Deterministic Decision Layer

- [ ] Add `CommandApprovalPhase`, `CommandApprovalAction`,
      `CommandApprovalOptions`, and `CommandApprovalDecision`.
- [ ] Implement `DecideCommandPlanApproval`.
- [ ] Add side-effect class helpers for observation-only versus manual replan
      side effects.
- [ ] Add unit matrix tests for initial/replan/continuation, auto flags,
      side-effect classes, risk escalation, and workdir changes.

### Batch 3: REPL Integration

- [ ] Apply the decision in `BuildCommandOperationPlan`.
- [ ] Replace REPL local replan auto/manual logic with the unified decision.
- [ ] Replace continuation raw `ApprovalMode` checks with the unified decision.
- [ ] Keep pending-operation persistence behavior unchanged.

### Batch 4: Regression Coverage

- [ ] Add E2E case: failed command -> revised `curl`/`network_read` plan auto
      continues when `AutoApprove=true`.
- [ ] Preserve E2E case: changed-workdir revised plan waits for approval.
- [ ] Preserve E2E case: risk-escalating repair waits for approval.
- [ ] Preserve E2E case: destructive plan is blocked/denied.
- [ ] Run focused `internal/operation` and `internal/repl` tests.
- [ ] Run `go test ./...`.

## Non-goals

- No new operation routing logic.
- No keyword matching.
- No changes to source evidence gates, trace/query tools, write-mode approval,
  or MCP observation lanes.

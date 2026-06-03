# Command Operation Capability Snapshot and Replan

## Problem

The current command-operation path is safe and usable for explicit requests
such as `go version`, but it is still under-informed and one-shot:

- `internal/repl/command_operation_planner.go` only sends `repo_root`,
  `route_policy`, and the raw user request to the model.
- `/env` already exposes `types.EnvFacts`, but the command-operation planner
  does not consume it.
- `internal/operation/executor.go` executes the approved plan in order and
  stops at the first failed step. It does not ask the planner to adjust the
  remaining commands based on stderr/stdout.

This makes the model less able to choose the best command path for the local
machine, and a small command typo or missing tool requires the user to start a
new turn.

## Red Lines

- Do not affect source-code analysis, trace/log analysis, MCP observations, or
  write-mode plan/apply/verify.
- Do not infer user intent with keyword matching. The operation path remains
  driven by typed `TurnPolicy.Route == operation` and
  `operation_kind == computer_operation`.
- Do not auto-expand risk. A replan that introduces new side effects, shell
  form, destructive actions, unknown programs, or higher risk must return to
  manual approval.
- Do not execute discovery commands before approval. Capability discovery must
  be read-only and bounded.
- Keep hard gates deterministic and typed. Model text can guide planning, but
  execution policy remains in `internal/operation`.

## Current Reusable Pieces

- `internal/env/probe` builds `types.EnvFacts`: OS/arch/shell, package
  managers, Python/Node/Rust/Java/Ruby toolchains, network, git repo state,
  and project files.
- `internal/operation.CommandPolicy` already carries timeout, output preview,
  hard-deny, low-risk auto approval, and mkdir policy.
- `internal/repl/command_operation_planner.go` already has a tool-only planner
  call and JSON repair via `repairTurnPolicyParamsJSON`.
- `internal/operation.CommandExecutor` already captures stdout/stderr, writes
  large outputs to `.codrax/operation/`, and returns per-step status/error.

## Design

### 1. OperationCapabilitySnapshot

Add a compact, prompt-safe snapshot for command operation planning. It should be
built only when the command-operation route runs.

Inputs:

- `repoRoot`
- cached `REPL.envFacts` when available
- otherwise `env.Probe` with a conservative profile

Content:

- OS/arch/shell/container/git state
- network/proxy status
- project marker files relevant to command choices
- package managers and paths
- language runtimes with versions
- common command availability via `exec.LookPath`
- command policy summary: approval mode, auto-low-risk, timeout, output preview

Output format:

- A small markdown/text section named `## capability_snapshot`
- Keep it bounded; do not include full PATH or environment variables.
- Mark it advisory: the planner can use it to choose commands, but policy still
  decides approval and risk.

Why not feed raw `EnvFacts` JSON:

- It can be verbose and uneven across systems.
- A compact section is easier for small models and avoids leaking environment
  details that do not affect planning.

### 2. Planner Integration

Extend the planner prompt with:

- Use the capability snapshot when choosing tools.
- Prefer available commands shown in the snapshot.
- If a requested capability is missing, ask for clarification or propose a
  safe install/check plan, depending on user intent.
- Do not invent installed tools not present in the snapshot unless the user
  explicitly requested an install or custom command.

Keep the public interface minimal:

- Add a `PlanCommandOperationWithContext`-style internal option or extend the
  existing planner implementation to build/accept the snapshot.
- Preserve tests and fallback behavior when no snapshot is available.

### 3. Failure Replan Loop

Add a controlled replan path after execution failure.

Trigger:

- The approved plan fails at a step.
- The failed step is not cancelled or timed out by user cancellation.
- Replan attempts remain below a small cap, initially 1.

Planner input:

- original user request
- original route policy
- capability snapshot
- original approved plan summary
- executed step results
- failed step id, exit code, error, bounded output preview

Planner output:

- same `CommandOperationRequest` schema as initial planning.

Policy handling:

- Build a fresh `CommandOperationPlan` with deterministic policy.
- If the new plan is blocked or needs clarification, render that state.
- If the new plan is ready and all remaining commands stay low risk and within
  the prior manual approval envelope, the REPL may execute it as a continuation.
- If risk increases, shell form appears, side effects expand, workdir changes
  materially, or unknown programs are introduced, store it as a new pending
  plan and ask for `/approve`.

V1 conservative default:

- Attempt one replan.
- Auto-execute a replan only when it is read-only, same or lower risk, no shell,
  no new side effects, and deterministic policy marks it low-risk eligible.
- Otherwise render the revised plan for manual approval.

### 4. UX

Initial planning:

- Continue rendering the current operation plan block.
- The capability snapshot stays in model context, not in the user-facing plan
  by default.

Failed execution:

- Show failed step output as today.
- If a replan is generated, add a compact section:
  `已根据失败输出生成修订计划，等待批准。`
- If replan auto-continues under strict low-risk rules, show:
  `根据失败输出自动改用低风险只读命令继续。`

### 5. JSON Compatibility

No new LLM-facing tool schema is required for V1. The same
`emit_command_operation_plan` tool is reused, so existing compatibility remains:

- string/list coercion for `args`, `env`, `side_effects`, `suggestions`
- string/bool coercion for `requires_confirmation`
- string/int coercion for `timeout_ms`
- structural repair through `repairTurnPolicyParamsJSON`

If later adding explicit fields such as `replan_reason`, they must use the same
flexible decoding and repair path.

## Task Checklist

### Batch 1: Design

- [x] Audit operation planner, executor, env probe, command policy.
- [x] Document current gaps, red lines, design, and rollout tasks.

### Batch 2: Capability Snapshot

- [x] Add `internal/operation` capability snapshot type and renderer.
- [x] Reuse `types.EnvFacts` and bounded `exec.LookPath` probing.
- [x] Feed snapshot into command-operation planner prompt.
- [x] Add tests for snapshot content and planner request payload.

### Batch 3: Replan Loop

- [ ] Add planner method for failed-plan replanning using the same schema.
- [ ] Add deterministic prior-approval envelope checks.
- [ ] Wire REPL execution to attempt at most one replan after failure.
- [ ] Add tests for low-risk replan, risk-escalating replan requiring approval,
      and cancellation/timeout not replanning.

### Batch 4: Validation

- [ ] Run `go test ./internal/operation ./internal/repl`.
- [ ] Run focused command-operation E2E tests.
- [ ] Run a real-model REPL smoke test with a simple command.
- [ ] Push each batch.

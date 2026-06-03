# Operation Goal Loop Convergence

## Problem

Recent real-operation runs show that the command-operation lane is still too
batch-centric. It can route typed computer-operation requests correctly and can
auto-run low-risk command batches, but it may stop before the user's goal is
actually satisfied.

Observed failure pattern:

- The model emits a command batch.
- The executor rejects or fails a command, for example a stdin-consuming shell
  command such as bare `grep`, `cat`, or `head` without a file, redirection, or
  upstream producer.
- Codrax performs at most one replan.
- If the revised batch is still invalid or incomplete, Codrax falls through to
  final answer synthesis and the user receives a partial or failed answer.

This is not a missing tool list problem. Command names and user operation
scenarios are unbounded: system inspection, software install/uninstall, SSH
work, file movement, reading manuals, extracting large files, building
artifacts, and custom enterprise tools all need the same framework. The system
must therefore control convergence, safety, and goal completion while allowing
the model to choose commands.

## Red Lines

- Do not route by raw user keywords. Operation entry remains driven by typed
  classifier signals such as `route=operation`,
  `operation_kind=computer_operation`, and `needs_operation_access`.
- Do not affect source-code analysis, trace/log analysis, MCP external
  observation analysis, or write-mode plan/apply/verify.
- Operation approval is independent from write-mode `write_enabled`.
- Keep high-risk approvals and hard-deny destructive patterns deterministic.
- Ask users only for user-owned information: credentials, remote host names,
  destructive scope, destination paths, and business choices that cannot be
  safely discovered. Do not ask the user for facts that safe local commands can
  discover.
- Keep model prose as soft guidance. Hard decisions use typed status, risk,
  side-effect enums, lint codes, failure classes, budget counters, and command
  policy.

## Root Cause

Current code splits responsibility across three linear hooks:

- `operationDispatch` builds a single `CommandOperationPlan`.
- `executeCommandOperationPlanAttempt` executes that plan and records one
  result.
- `maybeReplanCommandOperation` can ask the planner for one revised plan, then
  stops after `replanAttempts >= 1`.
- `maybeContinueCommandOperation` only runs after a successful batch with
  `continue_after=true`.

That means the system tracks "did a command batch run" better than "did the
user's goal converge". Invalid plans are also detected inside the executor,
after the REPL has already displayed an execution panel. The model can choose
commands, but the system lacks a first-class loop around:

- goal state
- known constraints
- missing observations
- success criteria
- next command batch
- why this batch is needed
- pre-execution lint
- repair budget
- completion evaluation independent of command exit status

## Design

### Goal State

Extend command-operation planning with optional typed goal fields:

- `goal`: concise user objective.
- `known_constraints`: constraints already known from the route, user request,
  policy, or prior operation observations.
- `missing_observations`: facts still needed to decide the answer.
- `success_criteria`: checks that indicate the user goal is satisfied.
- `next_batch`: short explanation of the current command batch's purpose.
- `why_this_batch`: why these commands are the right next step.

These fields are advisory to the planner and final answer handoff. They do not
create hard gates by themselves, but they make the operation loop and final
answer reason about goal completion rather than raw command success.

The first command batch should be deliberately small for non-trivial goals:
collect the minimum observations needed to choose the next step, set
`continue_after=true`, and defer later irreversible or speculative actions until
real command output confirms they are needed. This avoids brittle all-at-once
plans for tasks that naturally require discovery, tool learning, large-file
navigation, remote inspection, or environment-dependent command choices.

All fields must use the same JSON compatibility path as existing planner
fields: flexible string/list decoding plus structural repair through
`repairTurnPolicyParamsJSON`.

### Pre-Execution Plan Lint

Move command-shape validation before display and execution. The same
validation still runs in the executor as a final backstop.

Lint codes should be typed and recoverable:

- `empty_plan`
- `empty_command_step`
- `stdin_without_input`
- `invalid_shell_shape`
- `repeated_failed_command`
- `risk_policy_blocked`

When lint fails:

- Do not show "will run automatically".
- Do not execute the batch.
- Feed a synthetic failed result to the repair loop with
  `failure_class=invalid_plan`.
- Let the planner repair with a file operand, redirection, upstream producer,
  different tool, or a bounded discovery command.

### Goal Loop

Replace the one-shot attempt/replan recursion with a bounded goal loop.

Loop inputs:

- original user request
- current plan
- accumulated command result records
- capability snapshot
- operation memory/handoff
- loop budget

Per round:

1. Lint the plan.
2. If valid and auto-approved, execute it.
3. If manual approval is required, persist pending plan and stop.
4. If blocked, stop with a blocked final report.
5. Record the typed result.
6. Decide whether to continue, repair, clarify, or answer.

Budgets:

- `max_command_rounds`: initially 5 total command batches.
- `max_repair_rounds`: initially 3 invalid/failed repair rounds.
- Existing workflow/provider DAG budgets remain separate.

Invalid plan repairs should not count as true task failure, but they do consume
repair budget. Timeout/cancel stops immediately. High-risk expansion pauses for
approval.

### Operation Evaluator

Add a goal evaluator step separate from command execution.

Evaluator statuses:

- `complete`: observations satisfy the user's success criteria.
- `continue`: safe next observations can still be collected.
- `needs_clarification`: missing user-owned input prevents safe progress.
- `blocked`: policy or capability prevents progress.
- `budget_exhausted`: loop budget ended before full completion.
- `partial_answer_possible`: enough observations exist for a useful partial
  answer, but some requested details remain missing.

V1 can reuse the existing continuation planner call shape, but the prompt and
typed fields must make this explicit. Deterministic fallbacks:

- failed + repair budget available + not timeout/cancel => repair
- executed + `continue_after=true` => continue
- executed + no requested missing observations apparent => final answer
- budget exhausted => partial final answer with clear reason

### Handoff and UX

- Keep REPL/CLI progress concise.
- Show lint/repair as progress, not as final failure.
- Show commands before execution when useful, but avoid empty "即将执行" blocks.
- Preserve raw outputs as execution details and payload refs.
- Final report must answer the original user request first; raw details remain
  secondary.
- Keep operation observations in the external execution lane, not source-code
  citations.

## Task List

### Batch 1: Design Ledger

- [x] Record the gap, root cause, red lines, and phased design.

### Batch 2: Goal Fields and JSON Compatibility

- [ ] Add goal-state fields to `operation.CommandOperationRequest` and
      `operation.CommandOperationPlan`.
- [ ] Extend `emit_command_operation_plan` schema and planner prompt.
- [ ] Decode new fields with flexible string/list types.
- [ ] Include goal-state fields in replan/continuation context and result
      handoff.
- [ ] Add JSON repair/compatibility tests for string, array, and malformed
      trailing JSON.

### Batch 3: Pre-Execution Lint

- [ ] Add typed plan lint in `internal/operation`.
- [ ] Reuse existing shell stdin validation without duplicating policy logic.
- [ ] Keep executor validation as a safety backstop.
- [ ] Run lint before auto-run rendering and before manual pending storage.
- [ ] Add tests for bare `grep`, `cat`, `head`, empty command, and repeated
      failed command.

### Batch 4: Goal Loop

- [ ] Replace one-shot command execution recursion with a bounded loop.
- [ ] Allow multiple repair rounds within budget.
- [ ] Keep manual approval/hard-deny stop conditions unchanged.
- [ ] Keep timeout/cancel behavior unchanged.
- [ ] Ensure invalid plan repair does not show as final task failure unless the
      repair budget is exhausted.
- [ ] Add tests for repeated invalid plan repair, command-not-found fallback,
      nonzero-exit repair, safe continuation, manual approval pause, and budget
      exhaustion.

### Batch 5: Evaluator and Final Answer

- [ ] Teach continuation/evaluator prompt the statuses:
      `complete`, `continue`, `needs_clarification`, `blocked`,
      `budget_exhausted`, `partial_answer_possible`.
- [ ] Keep asking users only for user-owned inputs.
- [ ] Ensure final answer receives accumulated goal state and prioritized
      observations.
- [ ] Add E2E tests for system-info query, software-running/version query,
      large-file extraction, unfamiliar tool help-to-command workflow, and
      ambiguous destructive target clarification.

### Batch 6: Regression Protection

- [ ] Run focused operation tests.
- [ ] Run `go test ./...`.
- [ ] Add route regression tests showing code, trace/log, external observation,
      and write-mode requests are not pulled into operation by keyword-like
      text.
- [ ] Push each batch separately.

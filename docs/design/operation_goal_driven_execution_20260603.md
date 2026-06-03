# Operation Goal-Driven Execution and Answer Handoff

## Problem

The current operation route is too plan-centric:

- Low-risk command operations often stop for `/approve`, even when the
  deterministic policy can prove every step is read-only or otherwise
  auto-eligible.
- MCP/operation-skill providers default to a gate, so even configured
  providers pause unless the user manually approves.
- After execution, the REPL primarily shows raw plan/result markdown. That is
  useful as an execution detail, but it is not the final answer to the user's
  task.

The desired product flow is goal-driven:

1. The user states an operation task and target.
2. The model may call tools, lazy operation skills, MCP providers, local
   command execution, or a bounded operation workflow/DAG to reach the target.
3. REPL/CLI show concise progress without excessive noise.
4. Only typed risky actions require approval, and catastrophic actions are
   denied deterministically.
5. Final output is a model-synthesized report that answers the user's original
   request. Raw outputs remain available as execution details or payload refs.

## Red Lines

- Do not route code, trace, log, or write-mode requests through operation just
  because text mentions files or tools. Operation routing remains typed
  classifier driven.
- Do not use user prose or keyword matching as a hard safety gate.
- Approval/denial must use precise signals: risk enum, side-effect enum,
  provider gate, and deterministic command policy.
- Keep source citation lanes separate from operation outputs. Operation results
  are external execution observations.
- Keep dangerous commands blocked even when auto-low-risk execution is enabled.

## Root Cause

- `DefaultCommandPolicy.AutoLowRisk` was false, so safe reads such as `pwd`,
  `ls`, and `go version` waited for approval unless a config explicitly opted
  in.
- `BuildCommandOperationPlan` treated the model's
  `requires_confirmation=true` as a hard override, even when every step was
  deterministically low-risk.
- MCP and local operation-skill provider descriptors defaulted to
  `RequiresGate=true`.
- Provider operation plans did not auto-execute when `CanExecute=true`.
- Command results were rendered directly through
  `commandOperationResultMarkdown` instead of being handed back to the LLM for
  a user-facing answer.

## Design

### Durable Pending Operation State

Operation approvals are part of the operation lane, not write-mode ChangePlan
state. They must therefore persist in their own small store:

- Store path: `<runtime-anchor>/operations/pending.json`.
- Stored state: exactly one unresolved command plan, command clarification, or
  provider/workflow action.
- Not stored: large command/provider output, execution history, source
  evidence, trace evidence, or final-answer context.
- REPL startup restores the pending state before rendering the banner.
- The banner shows a concise pending-operation line with `/operation show`,
  `/approve`, `/reject`, and `/cancel`.

This fixes the restart gap where a manual-approval operation existed only in
process memory, so the next REPL session could not tell the user that a
decision was still waiting.

### Goal-Driven Operation Graph

The operation route should behave like a bounded executor for the user's
goal, not like a raw command-plan printer.

Execution nodes may be:

- local command plans
- MCP operation-provider calls
- local operation-skill calls
- workflow `next_actions`
- workflow `return_action` handoffs

The existing `WorkflowInstance` and queued `WorkflowAction` structures are the
right base for DAG support. This batch keeps execution sequential and bounded,
but treats the queue as a goal graph: run executable nodes automatically until
one of these precise stop conditions is reached:

- a node is risky and needs approval
- a node is denied by policy
- a node fails and no safe replan exists
- max depth/action budget is reached
- the workflow completes

The model-facing final report must summarize the whole graph result, not just
one node's raw output.

### Approval Policy

Default behavior:

- `operation_command_auto_approve=true` is the operation-lane default and is
  independent from write-mode `write_enabled`. In this mode, structured
  command steps auto-run unless precise high-risk or hard-deny signals match.
- Keep manual approval for shell commands, high-risk steps, installs/uninstalls,
  network submission, destructive/overwrite/delete, and explicit provider gates.
- Keep hard deny for catastrophic destructive patterns.
- Do not grow an unbounded OS/tool whitelist to chase individual examples.
  Long-tail structured commands may still be planned and auto-run under the
  operation auto-approval policy unless a precise high-risk signal is present.
  Setting `operation_command_auto_approve=false` restores conservative
  low-risk-only auto approval.

The model's `requires_confirmation` remains useful planner metadata, but it is
not allowed to block deterministic low-risk steps by itself.

Provider behavior:

- Operation providers default to no manual gate.
- A provider can explicitly set `operation_requires_confirmation: true`.
- High-risk side effects still require manual approval regardless of provider
  defaults.

### Execution UX

- Auto-run low-risk command and provider plans without asking the user to
  approve every intermediate step.
- Render concise progress and execution details, with large outputs kept behind
  payload refs.
- If execution fails, perform one bounded replan using the failure output.
- If the revised plan remains low-risk and same-directory, continue
  automatically; otherwise pause for approval.

### Final Answer Handoff

After execution completes and no further replan is needed:

- Build a compact operation result context.
- Ask the model to answer the user's original request using only those
  operation observations.
- Render the synthesized answer first.
- Render raw execution details below it so users can still inspect commands,
  outputs, verification, and payload refs.

Fallback:

- If the answer synthesis LLM call fails, show the execution details directly.

## Task List

- [x] Update deterministic command defaults to auto-run proven low-risk steps.
- [x] Keep dangerous command hard-deny and manual approval for non-low-risk
      steps.
- [x] Make provider gates opt-in instead of default-on.
- [x] Auto-execute executable provider plans and continue executable workflow
      next-actions until a manual gate, failure, or workflow end.
- [x] Add LLM result synthesis for command-operation results.
- [x] Add LLM result synthesis for provider/skill/MCP operation results.
- [x] Keep raw execution details visible but secondary.
- [x] Add concise operation progress lines before command/provider execution.
- [x] Update config docs and user guide.
- [x] Add tests for low-risk auto, dangerous block, provider auto, result
      synthesis fallback, and no source/trace/log route regressions.
- [x] Persist pending operation approvals outside write-mode PlanStore.
- [x] Restore pending operations on REPL startup and surface them in the
      startup banner.
- [x] Clear the pending-operation store on approve/reject/cancel/execution.
- [x] Avoid solving operation auto-execution by enumerating every platform
      command; use structured high-risk / hard-deny signals plus the operation
      auto-approval policy.

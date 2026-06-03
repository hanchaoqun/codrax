# Operation Unified Evaluator Design

Date: 2026-06-04

## Problem Ledger

Recent operation-mode traces show the same class of failure in several forms:

1. Command batches can execute successfully while the user's goal is still not satisfied.
2. Provider/MCP/Skill workflows can return payload refs, artifact refs, or next actions, but the flow may synthesize a final answer before the relevant material is read, searched, or extracted.
3. Command continuation and provider workflow scheduling are both useful, but they make completion decisions in separate local loops.
4. Operation result details must stay in per-round UI panels, while the final answer should be a clean user-facing report.
5. The fix must not affect code analysis, trace/log analysis, write mode, source evidence gates, or MCP readonly evidence lanes.

The missing architecture is a single operation-lane evaluator that consumes all operation observations and emits a typed verdict:

- complete
- continue_command
- continue_provider
- needs_approval
- needs_clarification
- blocked
- budget_exhausted
- partial_answer_possible

This evaluator is not a new executor. It is a decision layer above the existing command executor and provider workflow executor.

## Current Code Audit

### Command Operation Loop

`internal/repl/repl.go::executeCommandOperationPlanAttempt` already owns a bounded command goal loop:

- deterministic lint via `operation.LintCommandOperationPlan`
- command execution via `operation.CommandExecutor`
- failure repair via `CommandOperationReplanner`
- bounded continuation via `CommandOperationContinuationPlanner`
- per-round result rendering via `renderCommandOperationRoundResult`
- final synthesis via `commandOperationFinalMessage`

The command planner schema already includes goal-oriented fields:

- `goal`
- `known_constraints`
- `missing_observations`
- `success_criteria`
- `next_batch`
- `why_this_batch`
- `continue_after`

This is valuable and should remain the command-side evaluator substrate.

### Provider / Skill / MCP Workflow Loop

`internal/repl/repl.go::executeProviderOperationFlow` already owns a serial provider workflow DAG:

- provider execution through MCP or local operation Skill
- typed `next_actions`
- typed `return_action`
- `workflow_state`
- workflow depth/action budgets
- provider result rendering
- provider final answer synthesis

This is also valuable. The gap is that when the provider DAG is empty, the flow currently proceeds directly to final answer synthesis. It does not ask whether the provider observations are enough to answer the user's actual goal or whether local command extraction is needed.

### Operation Materials

`internal/operation/material.go` already normalizes command/provider/memory references:

- payload refs
- artifact refs
- next actions
- return actions
- workflow state
- preview-only observations

These references are prompt-safe external operation materials. They are not source-code evidence and must never enter current-source citation gates.

### Existing JSON Repair

Command-operation tool params already use flexible field types plus `repairTurnPolicyParamsJSON` in `unmarshalCommandOperationPlan`. Any new model-visible evaluator tool must use the same flexible decoding and repair path.

## Root Cause

The operation lane currently has two local completion concepts:

1. Command path: continuation planner says complete after command records.
2. Provider path: provider workflow queue empties, then answer.

Neither concept sees the full cross-surface operation state:

- command records
- provider records
- operation materials
- workflow queue/state
- budgets
- current pending approval/clarification
- original user success criteria

That creates premature finalization, especially when a provider returns "full content saved at payload_ref" or "next action suggested" and the final answer treats the compact summary as the full material.

## Red Lines

1. No keyword routing. Operation entry remains driven by the typed route/classifier already in the system.
2. No source evidence side effects. Operation observations stay in the operation/external lane.
3. No changes to repo_map, grep/read_file evidence gates, trace_query, perf_triage, or write-mode worktree rules.
4. No hard gates based on noisy summaries. The evaluator verdict may drive operation flow, but it must consume typed status, budgets, explicit pending actions, and model-emitted typed enums.
5. Provider/MCP/Skill output does not grant execution permission. Every next step still passes deterministic policy and approval.
6. Final answer remains clean; execution records stay in per-round operation panels.

## Target Architecture

Add an operation-lane `Unified Evaluator`.

```go
type OperationEvaluationStatus string

const (
    EvalComplete              OperationEvaluationStatus = "complete"
    EvalContinueCommand       OperationEvaluationStatus = "continue_command"
    EvalContinueProvider      OperationEvaluationStatus = "continue_provider"
    EvalNeedsApproval         OperationEvaluationStatus = "needs_approval"
    EvalNeedsClarification    OperationEvaluationStatus = "needs_clarification"
    EvalBlocked               OperationEvaluationStatus = "blocked"
    EvalBudgetExhausted       OperationEvaluationStatus = "budget_exhausted"
    EvalPartialAnswerPossible OperationEvaluationStatus = "partial_answer_possible"
)

type OperationEvaluation struct {
    Status        OperationEvaluationStatus
    Reason        string
    Confidence    string
    MissingInputs []string
    Materials     []OperationMaterial
}
```

The REPL builds an evaluation context from existing records:

- original user request
- typed route policy
- command operation rounds
- provider operation rounds
- operation materials
- provider workflow state
- pending approval/clarification
- budgets and counters

The evaluator is layered:

1. Deterministic pre-evaluation:
   - pending approval -> `needs_approval`
   - pending clarification -> `needs_clarification`
   - provider workflow queue non-empty -> `continue_provider`
   - command/provider budget exhausted -> `budget_exhausted`
   - explicit failure/block status -> `blocked` or `partial_answer_possible`

2. Model-assisted typed evaluation:
   - only inside operation route
   - returns a strict typed enum
   - uses flexible JSON decoding and repair
   - can decide `complete`, `continue_command`, `continue_provider`, or `partial_answer_possible`
   - cannot execute; it only requests the next bounded step through existing planners/executors

3. Existing execution:
   - `continue_command` calls the existing command planner/continuation path and then `operation.CommandExecutor`
   - `continue_provider` uses existing provider workflow planning and execution
   - final answer synthesis runs only after evaluator returns `complete`, `partial_answer_possible`, `blocked`, or `budget_exhausted`

## Why This Is General

This supports all operation surfaces without adding per-case command recipes:

- system/environment inspection
- software install/uninstall/version checks
- SSH/remote environment workflows
- file moves/copies/conversions
- reading large files, web pages, manuals, logs, and saved payloads
- MCP and local operation Skill workflows
- document/PPT/table/browser/desktop providers when added

The evaluator does not need to know every command. It only decides whether the current observations satisfy the goal and which already-existing executor should continue.

## JSON Compatibility Requirements

Any new evaluator tool must:

- use flexible string/bool/list decoding where model output may vary
- call `repairTurnPolicyParamsJSON` on unmarshal failure
- tolerate stringified lists and booleans
- default unknown/empty status to `partial_answer_possible` or fail softly, not crash the REPL
- include tests for malformed JSON, string booleans, string lists, and extra fields

## Prompt / Teaching Requirements

Evaluator prompt must teach:

- provider summaries are previews, not proof that full payload/artifact content was inspected
- payload/artifact/material refs are consumable materials for the next bounded command/provider action
- ask the user only for user-owned missing inputs
- if safe local discovery can answer the missing fact, emit `continue_command`
- if a configured provider next action is needed, emit `continue_provider`
- do not mention source-code evidence unless the operation route explicitly generated source-analysis observations, which this design does not do
- final report should be clean and not include raw execution detail blocks

## Implementation Batches

### Batch 1: Design Ledger

- [x] Audit command loop, provider workflow loop, material ledger, and JSON repair.
- [x] Add this design document.
- [x] Commit and push the design.

### Batch 2: Typed Evaluation Model

- [x] Add operation evaluator status/types in `internal/operation`.
- [x] Keep context collection in REPL-side render helpers so `operation` does not import REPL internals.
- [x] Add tests for status normalization and terminal classification.

### Batch 3: Model Evaluator Tool

- [x] Add `emit_operation_evaluation` schema to `internal/repl/command_operation_planner.go`.
- [x] Add `ProviderOperationEvaluator` interface.
- [x] Implement flexible JSON unmarshal + `repairTurnPolicyParamsJSON`.
- [x] Add prompt teaching for material refs, provider summaries, and bounded continuation.
- [x] Add unit tests for JSON repair and evaluator prompt payload.

### Batch 4: Command Path Adapter

- [x] Keep existing command continuation path as the command-side evaluator substrate.
- [x] Keep existing behavior byte-close for command-only tasks.
- [x] Ensure command round result remains in operation result panels and final answer remains clean.
- [x] Re-run command continuation, repair, and clean-final-answer tests.

### Batch 5: Provider Path Adapter

- [x] Evaluate provider records before final provider answer.
- [x] If evaluator returns `complete` or `partial_answer_possible`, synthesize final provider answer through the existing path.
- [x] If evaluator returns `continue_command`, call existing command planner with provider/material handoff and execute through normal command policy.
- [x] Keep typed provider `next_actions` handled by the existing provider workflow queue before evaluator finalization.
- [x] Preserve pending approval and manual approval UX.
- [x] Add tests for provider payload -> command extraction and rerun provider workflow tests.

### Batch 6: Handoff and UX

- [x] Keep per-round operation result panels as the raw execution-detail location.
- [x] Make provider-to-command handoff include evaluator status and operation materials.
- [x] Keep REPL/CLI style consistent by reusing existing operation route summaries.
- [x] Keep large outputs as payload refs with short UI preview through existing output capture.

### Batch 7: Regression Coverage

- [x] Code-only question does not expose operation evaluator. Covered by `TestTurnPolicyDispatch_RepoRouteEntersPipeline`.
- [x] Trace/log question still uses trace/log pipeline and evidence lanes. Covered by `TestTurnPolicyDispatch_ExternalObservationAnalysisDoesNotCallOperationEvaluator`.
- [x] Mixed external observation + source question remains source-aware. Covered by `TestTurnPolicyDispatch_HybridCarriesDirectiveIntoPipeline`.
- [x] Write modes remain gated by `write_enabled`. Covered by existing `cmd/writemode_resolve_test.go` and full `go test ./...`.
- [x] MCP readonly evidence path remains separate from operation provider path. Covered by external-observation dispatch regression plus operation-provider tests.
- [x] Operation Skill/MCP provider route can continue through command extraction only when evaluator emits typed `continue_command`. Covered by `TestOperationProviderEvaluatorContinuesWithCommandExtraction`.

## Rollout Scope

The first implementation should be conservative:

- Command-only path keeps the current command continuation loop, only exposed through typed evaluator wrappers.
- Provider path gains one evaluator checkpoint before final synthesis.
- Provider-to-command continuation is bounded and policy-reviewed.
- No automatic source-code analysis is introduced from operation materials.
- No automatic browser/desktop provider is introduced here; those remain future providers.

## Success Criteria

1. A provider returning only a payload ref no longer causes final answer to imply the payload was read.
2. If a safe command extraction can satisfy the user's request, evaluator returns `continue_command` and the command executor runs normally.
3. If the user goal is already satisfied, evaluator returns `complete` and the final answer is clean.
4. Manual approval, high-risk denial, and pending provider actions still work.
5. Code/trace/log/write tests remain green.

# Operation Workflow Orchestrator

**Status:** design and task ledger before implementation.
**Date:** 2026-06-03
**Scope:** upgrade the operation provider path from one pending provider action
to a bounded workflow instance. The workflow is operation-lane only: it must not
affect source analysis, trace/log analysis, MCP read-mode observations, or
write-mode code changes.

## 1. Code Audit

Current state:

- `internal/repl/repl.go` stores one `pendingProviderOperation`.
- `/approve` first checks command operations, then provider operations.
- `/reject` and `/cancel` clear that single provider pending action.
- Provider execution converges through `providerOperationResult`.
- Local operation skills can return `next_actions[]` and `workflow_state`.
- `queueProviderNextAction` currently picks the first valid next action and
  stores it as the new `pendingProviderOperation`.
- Handoff and memory already preserve provider result summaries, refs,
  `next_actions`, and `workflow_state`.

This means the safe implementation point is still the REPL operation lane. We
do not need to modify analyzer/explorer prompts, trace tools, current-source
evidence gates, or write-mode task graph.

## 2. Gap

The current single-pending model cannot represent:

- multiple pending `next_actions`;
- a workflow queue where actions are executed one by one;
- a provider's `return_action` back to an earlier skill/step;
- visible workflow status and cancellation;
- a stable workflow handoff record that says which step is current, queued,
  completed, skipped, failed, or cancelled.

The user-facing problem is that Codrax can do `skill A -> skill B`, but cannot
yet manage `A -> [B, C] -> return to A` in a controlled, resumable way.

## 3. Red Lines

- No keyword-based routing. Workflows start only after the typed operation
  route selected and executed an operation provider.
- Provider-authored actions remain suggestions. Codrax still matches configured
  provider descriptors and enforces approval and risk policy.
- Default approval remains manual. `/approve` advances one pending workflow
  action.
- V1 is serial. Multiple `next_actions` are queued in order, not run in
  parallel.
- Workflow actions never become source citations or trace evidence.
- No fallback from invalid workflow actions to source analysis.
- Workflow state is compact operation state; large output must stay in payload
  refs/artifact refs.

## 4. DAG / Plan Graph Data Model

Add operation-owned types:

```go
type WorkflowEdgeKind string

const (
    WorkflowEdgeNext WorkflowEdgeKind = "next"
    WorkflowEdgeReturn WorkflowEdgeKind = "return"
    WorkflowEdgeFallback WorkflowEdgeKind = "fallback"
)

type WorkflowActionStatus string

const (
    WorkflowActionQueued WorkflowActionStatus = "queued"
    WorkflowActionPending WorkflowActionStatus = "pending"
    WorkflowActionExecuted WorkflowActionStatus = "executed"
    WorkflowActionFailed WorkflowActionStatus = "failed"
    WorkflowActionSkipped WorkflowActionStatus = "skipped"
    WorkflowActionCancelled WorkflowActionStatus = "cancelled"
)

type WorkflowAction struct {
    ID string
    ParentID string
    SourceProvider string
    NextAction WorkflowNextAction
    ReturnAction WorkflowNextAction
    Plan operation.Plan
    Request operation.Request
    Input map[string]any
    WorkflowState WorkflowState
    Depth int
    Status WorkflowActionStatus
    Diagnostics []string
}

type WorkflowEdge struct {
    From string
    To string
    Kind WorkflowEdgeKind
}

type WorkflowInstance struct {
    ID string
    RootRequest string
    Actions []WorkflowAction
    Edges []WorkflowEdge
    CurrentID string
    Queue []string
    Cancelled bool
    MaxDepth int
    MaxActions int
}
```

The graph is the source of truth; `Queue` is only the serial scheduling view for
V1. This gives us DAG/Plan Graph support from the start:

- multiple `next_actions` become sibling nodes under the executed provider;
- `return_action` becomes a `return` edge to the requested provider/step;
- future parallel fanout can schedule independent queued nodes without changing
  provider result contracts;
- cycle and depth checks can operate on graph edges.

The REPL may keep runtime execution handles privately, but the graph type should
live in `internal/operation` so tests, handoff, and future providers share one
contract.

## 5. Provider Result Protocol Extension

Keep existing fields and add optional `return_action`:

```json
{
  "success": true,
  "summary": "Built the deck.",
  "artifact_refs": ["out/deck.pptx"],
  "return_action": {
    "provider": "skill:manual_reader",
    "operation_kind": "artifact_generation",
    "target_surface": "local_file",
    "request": "Compose the final workflow report.",
    "input": {
      "deck_path": "out/deck.pptx"
    }
  },
  "workflow_state": {
    "workflow_id": "manual-to-deck-001",
    "step": "deck_created",
    "return_to": "skill:manual_reader"
  }
}
```

Aliases follow `next_action`: `return_to_action`, `return`, and
`callback_action` are accepted as `return_action` if they contain an object.

## 6. Runtime Behavior

1. Initial operation provider plan creates a `WorkflowInstance` with one current
   action.
2. `/approve` executes the current workflow action.
3. Codrax appends all valid `next_actions` as DAG nodes and queues them in
   provider order.
4. If `return_action` is valid, Codrax appends it after the returned
   `next_actions`, records a `return` edge, and queues it after the child
   actions so the workflow can hand control back to the requested skill.
5. Codrax advances to the next queued action and renders a concise workflow
   status. It does not execute until the user runs `/approve`.
6. `/reject` rejects only the current pending action and advances/cancels the
   workflow according to queue state.
7. `/cancel` cancels the active workflow instance.
8. `/workflow show` renders current, queued, completed, failed, and diagnostics.
9. `/workflow cancel` cancels the active workflow instance.

## 7. UX

Daily flow remains simple:

- `/approve` advances one step.
- `/reject [reason]` rejects the current step.
- `/cancel` cancels the active workflow.

Advanced inspection:

- `/workflow show`
- `/workflow cancel`

The rendered status should say:

- workflow id;
- current provider/action;
- queued count;
- completed count;
- failed count;
- next user command.

## 8. Delivery Tasks

### Batch A: Design Ledger

- [x] Audit current provider execution, queueing, approval, rejection,
      cancellation, handoff, and memory.
- [x] Record data model, provider protocol, red lines, runtime behavior, UX,
      and tasks.

### Batch B: Types and Parsing

- [ ] Add `operation.WorkflowAction`, `operation.WorkflowInstance`, and compact
      render helpers.
- [ ] Add `operation.WorkflowEdge` / edge kinds so the workflow is a DAG/Plan
      Graph even while V1 scheduling remains serial.
- [ ] Parse `return_action` from local skill JSON with aliases.
- [ ] Preserve `ReturnAction` in `providerOperationResult`, handoff, and memory
      where useful.
- [ ] Add unit tests for `return_action` parsing and workflow queue helpers.

### Batch C: Serial Workflow Queue

- [ ] Replace single `pendingProviderOperation` behavior with a workflow
      instance that can hold current + queued actions.
- [ ] Queue all valid `next_actions` in order.
- [ ] Keep `/approve` semantics as "execute current workflow action".
- [ ] Add E2E tests for `A -> B -> C` serial chaining and invalid action
      diagnostics.

### Batch D: Return Action and UX

- [ ] Append valid `return_action` after next actions.
- [ ] Add `/workflow show` and `/workflow cancel`.
- [ ] Update `/operation show`, `/reject`, and `/cancel` behavior to reflect
      active workflow state.
- [ ] Add E2E tests for `A -> B -> return A`, `/workflow show`, and cancellation.

### Batch E: Docs and Regression

- [ ] Update user guide MD/HTML.
- [ ] Run focused REPL/operation/cmd tests.
- [ ] Push each batch.

## 9. Non-goals

- No automatic parallel fanout.
- No auto-running chains without approval.
- No source/trace/log route changes.
- No provider-authored system instruction execution.

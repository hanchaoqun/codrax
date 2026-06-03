# Operation Skill Workflow Chaining

**Status:** implemented and pushed in batches.
**Date:** 2026-06-03
**Scope:** let local/MCP operation providers return typed follow-up actions and
workflow state so Codrax can queue a safe next operation after approval. This is
operation-lane only and must not affect source analysis, trace/log analysis,
MCP read-mode observations, or write-mode code changes.

## 1. Code Audit

Current operation provider flow is already isolated:

- `internal/repl/repl.go` stores one `pendingProviderOperation`.
- `/approve` calls `executeProviderOperation`, then appends the result through
  `appendProviderOperationResult`.
- MCP-backed providers and local `operation_skills[]` providers both converge
  into the private `providerOperationResult` shape:
  `status/provider/tool/summary/payload_ref/artifact_refs/verification/error`.
- `renderCommandOperationHandoff` passes the last provider results to the
  command-operation planner as operation observations, not source evidence.
- `operation.BuildProviderMemoryEntry` persists compact provider lessons and
  refs into operation memory.
- Provider execution is already approval-gated and does not enter the source
  pipeline.

This gives us a safe seam: extend provider results with typed
`next_actions[]` and `workflow_state`, then queue a follow-up provider plan only
inside the same operation lane.

## 2. Gap

Local/MCP operation skills can finish one subtask, but cannot explicitly say:

- "call another provider next";
- "pass this payload/artifact ref as input";
- "return to me after that provider finishes";
- "keep this workflow state for the next approval".

The current handoff/memory can let the model infer a next step later, but there
is no deterministic typed chain. That makes multi-tool workflows such as
"read manual -> call generator -> verify artifact -> summarize" less stable.

## 3. Red Lines

- No keyword-based routing. Chaining starts only after the typed operation route
  selected and executed a provider.
- Follow-up actions are **suggestions**, not authority. Codrax still matches the
  provider descriptor and applies approval policy.
- Default approval remains manual. The first implementation queues the next
  provider and asks the user to `/approve`; it does not auto-run chains.
- Chain depth and action count are bounded.
- Invalid or unmatched next actions are surfaced as caveats in operation output;
  they do not fall back to source analysis.
- Workflow data is operation artifact state, not current-source evidence.

## 4. Provider Result Protocol

Providers may return:

```json
{
  "success": true,
  "summary": "Read the manual and extracted the command template.",
  "artifact_refs": ["out/manual-notes.md"],
  "next_actions": [
    {
      "provider": "skill:ppt_builder",
      "operation": "presentation_generation",
      "operation_kind": "presentation_generation",
      "target_surface": "slides",
      "risk_level": "medium",
      "side_effects": ["local_file_write"],
      "requires_confirmation": true,
      "request": "Create slides from the extracted notes.",
      "input": {
        "source_payload_ref": "out/manual-notes.md",
        "output_path": "out/deck.pptx"
      }
    }
  ],
  "workflow_state": {
    "workflow_id": "manual-to-deck-001",
    "step": "manual_extracted",
    "return_to": "skill:manual_reader",
    "data": {
      "source_payload_ref": "out/manual-notes.md"
    }
  }
}
```

Accepted aliases for resilience:

- `next_action` as a single object is normalized to `next_actions[0]`.
- `provider_name` aliases `provider`.
- `kind` aliases `operation_kind`.
- `surface` aliases `target_surface`.
- `requires_approval` aliases `requires_confirmation`.

Unknown fields are ignored; unknown provider names are not executed.

## 5. Runtime Behavior

1. Provider executes after `/approve`.
2. Codrax parses `next_actions[]` and `workflow_state`.
3. If at least one next action is valid and budget remains:
   - build a new `operation.Request`;
   - match it against configured providers with `operation.BuildPlan`;
   - store it as `pendingProviderOperation`;
   - render a clear message that the next workflow step is queued and requires
     `/approve`.
4. If no action is valid, record invalid action diagnostics in result output and
   stop the chain.
5. Each chained execution appends to handoff and operation memory.

V1 queues only the first valid next action. Additional actions are listed in the
result and can be picked in a later iteration after the single pending action is
handled. This avoids uncontrolled fanout.

## 6. Data Carriers

Add operation-owned typed carriers:

```go
type WorkflowNextAction struct {
    Provider string
    Tool string
    Operation string
    OperationKind string
    TargetSurface string
    RiskLevel string
    SideEffects []string
    RequiresConfirmation bool
    Request string
    Input map[string]any
}

type WorkflowState struct {
    WorkflowID string
    Step string
    ReturnTo string
    Data map[string]any
}
```

`providerOperationResult` then gains:

- `NextActions []operation.WorkflowNextAction`
- `WorkflowState operation.WorkflowState`
- `WorkflowDiagnostics []string`

The JSON envelope passed to local skills and MCP operation providers should also
include the previous workflow state when present.

## 7. Delivery Tasks

### Batch A: Design Ledger

- [x] Audit provider result, handoff, memory, REPL pending state, and planner
      descriptor paths.
- [x] Record protocol, red lines, runtime behavior, and delivery tasks.

### Batch B: Typed Result Parsing and Handoff

- [x] Add `operation.WorkflowNextAction` and `operation.WorkflowState`.
- [x] Parse `next_actions` / `next_action` and `workflow_state` from local skill
      JSON results with alias tolerance.
- [x] Preserve next actions and workflow state in provider result, handoff, and
      operation memory.
- [x] Add tests for parsing aliases, invalid fields, and compact handoff output.

### Batch C: REPL Queueing and Approval

- [x] Extend pending provider operation with workflow depth/state.
- [x] After provider execution, queue the first valid matched next action.
- [x] Render the queued step with `/approve` / `/reject` guidance.
- [x] Enforce bounded depth/action count and surface diagnostics for invalid
      actions.
- [x] Add E2E tests for skill A -> skill B handoff plus existing unmatched
      provider/manual approval coverage.

### Batch D: Docs and Regression

- [x] Update `codrax.yaml.example` comments if needed; no YAML field change was
      needed for this batch.
- [x] Update user guide MD/HTML with `next_actions` and `workflow_state`.
- [x] Run focused tests for operation, REPL, cmd.
- [x] Push each batch.

## 8. Non-goals

- No auto-running multi-skill chains in V1.
- No source/trace/log route changes.
- No workflow fanout execution.
- No provider-authored prompt/system instruction execution.

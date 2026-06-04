# Write Exploration Handoff Workflow

Date: 2026-06-04

## Problem Ledger

Write mode has become safer through worktree isolation, write-risk approval,
rolling batch seeds, and `WriteUnifiedEvaluator`. The remaining commercial gap
is that read-mode exploration is still not a first-class input to write-mode
planning.

Observed architecture shape:

- `write_analyzer` emits typed task framing and rough phase/batch targets.
- `planner` receives task framing, likely files, test surface, retry history,
  and active pitfalls.
- `explorer` already produces rich `TurnAArtifacts` in read mode.
- `planner` can run read-only tools itself, but the system does not preserve
  a typed "what exploration already proved" handoff for write planning.

This makes hard changes riskier than they need to be:

- Planner may re-discover code patterns instead of using a prior read-only
  investigation.
- Multi-batch write workflows cannot deterministically ask for "explore first,
  then plan a small change" without relying on prompt prose.
- Verify failures can replan without enough structured information about what
  should be re-explored.

## Red Lines

- Read mode must stay byte-preserved.
- `write_enabled` remains false by default for commercial release.
- Write tools must still never call `ground.*`.
- Do not route by user keywords or parse model prose for hard decisions.
- Hard decisions consume typed enums, typed status, risk levels, approval
  actions, and budget counters.
- Explorer output is advisory evidence for planning; it must not become source
  citation evidence in final answers unless the existing evidence pipeline
  creates citations through its normal read-mode contracts.

## Current Code Audit

### Existing Write Control

- `internal/writeflow/workflow.go` defines `WriteWorkflowPlan` and
  `WriteBatchPlan`, including `BatchNeedsExploration` and
  `NeedsCodeExploration`.
- `internal/writeflow/evaluator.go` returns typed verdicts such as
  `continue_explore`, `continue_plan`, `needs_approval`, and `complete`.
- `internal/orchestrator/phase_scheduler.go` records phase-level
  `WriteWorkflowEvaluationSnapshot`.
- `internal/orchestrator/write_scheduler.go` still executes a fixed
  `plan -> apply -> verify` graph.

### Existing Planner Context

`internal/agent/planner.go::BuildInitialInstruction` already renders:

- task framing from `WriteAnalysisIR`
- rolling workflow seed
- likely relevant files
- test surface
- probe history
- iteration history
- active pitfalls
- retry planning context

There is no section for "prior code exploration handoff".

### Existing Read Exploration Handoff

`internal/types.TurnAArtifacts` carries:

- read files
- tool results
- MCP responses
- evidence items
- flow findings
- accepted aggregate facts
- accepted closure reason/result kind
- source inventory observations

It is intentionally read-mode oriented and too broad/noisy to inject directly
into planner prompts. Write mode needs a smaller projection.

## Target Design

### WriteExplorationRequest

Typed request from write workflow controller to the read-only explorer lane:

- `batch_id`
- `goal`
- `exploration_questions`
- `candidate_paths`
- `constraints`
- `max_rounds`
- `evidence_requirements`

This is a control artifact. It does not execute code, apply patches, or make
source-citation claims.

### WriteExplorationHandoff

Typed read-only result projected from `TurnAArtifacts`:

- `batch_id`
- `goal`
- `target_files`
- `relevant_symbols`
- `existing_patterns`
- `invariants`
- `test_surface`
- `risk_notes`
- `unknowns`
- `evidence_refs`
- `confidence`

The handoff is a planning substrate. It says what the planner should consider
when emitting a small `ChangePlan`; it does not replace `ChangePlan`.

### Workflow Controller

The eventual write workflow loop should be:

1. `write_analyzer` emits typed task framing.
2. A workflow decision chooses one typed next action:
   - `explore_code`
   - `plan_change`
   - `ask_user`
   - `block`
3. If `explore_code`, the system dispatches the existing explorer in read-only
   mode with a `WriteExplorationRequest`.
4. Explorer output is projected into `WriteExplorationHandoff`.
5. Planner consumes the handoff and emits one bounded `ChangePlan`.
6. Apply/verify stay on the existing worktree-safe chain.
7. `WriteUnifiedEvaluator` decides whether to finish, re-explore, replan, wait
   for approval, or stop.

### Prompt / Teaching

Planner prompt should include a neutral section:

`## Prior code exploration handoff`

The section must teach:

- treat the handoff as read-only planning context
- prefer existing patterns and invariants from the handoff
- if `unknowns` are material, use read-only tools before emitting a plan
- do not unfold the whole project when a smaller batch can be changed and
  verified

This is prompt guidance only; no hard gate reads planner prose.

### JSON Compatibility

Any model-visible new fields must be covered by schema-driven repair:

- string-to-array splitting for list fields
- string-to-int coercion for `max_rounds`
- string-to-bool if future booleans are added
- unknown-field pruning through existing `toolparam.Normalize`

If a payload cannot be repaired, tools should return a clear structured retry
hint rather than failing silently.

## Implementation Batches

### Batch 1: Design Ledger

- [x] Record the gap, root cause, red lines, target architecture, and task list.

### Batch 2: Typed Handoff Model

- [ ] Add `WriteExplorationRequest`, `WriteExplorationHandoff`, and
      `WriteExplorationEvidenceRef` in `internal/types`.
- [ ] Add defensive-copy storage on `MutableState`.
- [ ] Add projection from `TurnAArtifacts` to `WriteExplorationHandoff`.
- [ ] Add tests for defensive copy and projection.

### Batch 3: Planner Consumption

- [ ] Render `## Prior code exploration handoff` in planner prompts before
      likely-file ranking.
- [ ] Teach bounded planning from handoff without forcing source exploration.
- [ ] Add prompt tests proving read mode is unaffected and planner sees the
      handoff.

### Batch 4: Workflow Decision Schema

- [ ] Add typed `WriteWorkflowDecision` / next-action schema.
- [ ] Support JSON compatibility repair for new fields.
- [ ] Add tests for enum normalization and string/list repair.

### Batch 5: Read-Only Explorer Adapter

- [ ] Add an orchestrator helper that can dispatch the existing explorer as a
      write-lane read-only subflow.
- [ ] Build its objective from `WriteExplorationRequest`.
- [ ] Project the resulting `TurnAArtifacts` into `WriteExplorationHandoff`.
- [ ] Ensure no write tools are exposed during this subflow.

### Batch 6: Evaluator Front-Loading

- [ ] Evaluate before each batch whether to explore, plan, ask, block, or stop.
- [ ] Record evaluation snapshots at every batch boundary.
- [ ] Preserve current approval semantics: low/medium auto under
      `auto_safe`, high manual, critical deny.

### Batch 7: Review / Security Lanes

- [ ] Add independent diff reviewer hook that consumes diff + success criteria.
- [ ] Add security scan lane for secrets, dependency/workflow changes,
      permission/path risks.
- [ ] Feed typed results into write risk and final workflow evaluation.

### Batch 8: Regression Coverage

- [ ] Read-mode byte-preservation snapshot.
- [ ] Planner handoff prompt snapshot.
- [ ] Verify-failure re-explore decision fixture.
- [ ] Approval policy regression fixtures.
- [ ] End-to-end write eval with exploration handoff, small batch change,
      tests, and final report.

## Commercial Release Guidance

Commercially safe default remains:

```yaml
write_enabled: false
write_approval_policy: auto_safe
```

When customers explicitly enable write mode:

- low/medium risk may auto-progress under `auto_safe`
- high risk requires approval
- critical risk is denied
- worktree, plan, report, and evaluation snapshots remain auditable
- no automatic merge into the main branch

Do not market write mode as fully autonomous until exploration handoff,
front-loaded evaluation, independent review, and security scan lanes have
real tests and eval coverage.

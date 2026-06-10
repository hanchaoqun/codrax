# Write Mode Dynamic Commercial Delivery

Date: 2026-06-10
Status: Batch 3 ready to commit
Branch: codex/write-mode-commercial-workflow

## 1. Goal

Upgrade write mode from a mostly linear `plan -> apply -> verify` lane into a
commercial-grade, dynamically converging workflow:

- low and medium risk batches progress without extra approval under
  `auto_safe`;
- high risk batches require explicit user approval;
- critical risk batches are rejected before any apply-side mutation;
- complex code changes converge through typed exploration, bounded batches,
  apply, verify, and typed re-evaluation;
- read mode, operation mode, simple write mode, and worktree cleanup remain
  stable.

This is a system-level design. Do not patch around one prompt, one user request,
or one model phrasing. Hard routing and safety gates must consume typed
artifacts, repository facts, parsed structured files, and deterministic policy
results only.

## 2. Current Code Audit

### Existing strengths

- `BuildWriteTaskGraph` already provides a deterministic inner executor:
  `plan -> apply -> verify`, with verify feedback back to plan/apply.
- `writeflow` already has typed workflow, decision, risk, approval, and
  evaluator primitives.
- `runPhaseGroup` can run sequential phases and can dispatch a read-only write
  exploration subflow before a phase.
- REPL `/approve` already computes write risk before applying a plan.
- The operation lane has a mature typed approval precedent:
  `auto_execute / manual / deny` with replan/continuation context.

### Commercial gaps

1. **Final apply boundary is not the single approval source of truth.**
   Freshly generated plans are checked in `planPostHook`, but plan-file apply
   skips the first plan dispatch. The final gate must run after plan load and
   before worktree creation in `applyPreHook`.

2. **Approval state is not persisted as an approval record.**
   `PlanStatus` describes execution lifecycle, while approval policy, risk,
   decision source, and plan fingerprint need their own audit record.

3. **Dynamic task decomposition is still mostly static.**
   `PlanGroup.Phases` is intentionally fixed after creation. A commercial
   controller needs a separate append/split/follow-up run object rather than
   mutating the static phase group contract.

4. **Write exploration handoff is useful but not prioritized.**
   Current handoff preserves bounded read-only findings, but consumers need a
   prioritized context pack that carries constraints, evidence, risk, unknowns,
   and verification signals without flooding prompts.

5. **Safety policy must avoid prose keyword logic.**
   The existing risk assessor is already deterministic, but content checks need
   to move toward structured parsers and exact protocol signatures rather than
   interpreting user/model prose.

6. **Design/documentation completion marks drifted ahead of code reality.**
   Prior design docs contain checked boxes for controller-shaped work, while the
   code still lacks a controller loop. Every batch below must update this ledger
   with concrete files, tests, commit, and push status.

## 3. Red Lines

- `write_enabled: false` remains the default hard capability gate.
- Do not change read scheduler byte identity or read-mode contracts.
- Do not expose write tools to read-only exploration subflows.
- Do not route by user text keywords, model summary/rationale prose, or prompt
  wording.
- Prompt text may teach behavior only; hard gates read typed artifacts.
- No broad dependency additions. Keep the repo's dependency posture intact.
- Apply happens only in a git worktree. Main repo bytes are never changed by
  write apply.
- Existing stable single-batch write tasks must remain on the legacy path until
  the controller is explicitly enabled.

## 4. Target Architecture

Keep the existing write task graph as the inner batch executor. Add a new outer
workflow object and controller:

```text
WriteAnalysisIR seed
  -> write_controller
  -> read-only exploration runner
  -> priority WriteContextPack
  -> planner emits one bounded ChangePlan
  -> apply-pre risk/approval gate
  -> apply in worktree
  -> verify
  -> typed evaluator
  -> next/split/re-explore/finish/block
```

### New persistence object

`types.WriteWorkflowRun` is separate from `PlanGroup`:

- `RunID`, `Goal`, `Status`, `ActiveBatchID`
- `Batches`, `Edges`
- `ContextPacks`
- `Budget`
- `ProgressLedger`

Batch status:

- `needs_exploration`
- `ready_to_plan`
- `planned`
- `pending_approval`
- `applying`
- `verifying`
- `complete`
- `blocked`

Edge kind:

- `seed`
- `explore`
- `plan`
- `apply`
- `verify`
- `split`
- `followup`
- `blocked`

### Approval record

`types.WriteApprovalRecord` belongs on `ChangePlan`, not in `PlanStatus`:

- `policy`
- `risk_level`
- `action`
- `user_decision`
- `reason_code`
- `reasons`
- `source`
- `plan_fingerprint`
- `decided_at`

The final decision is recomputed at apply time. A stale fingerprint invalidates
any previous approval.

### Priority context pack

`WriteContextPack` carries cross-stage context with priority:

- P0: user constraints, safety boundary, risk, approval, scope boundary
- P1: target files, symbols, invariants, line-backed evidence refs
- P2: tests, verification failures, unknowns
- P3: style and implementation pattern hints

Consumers render only their relevant Top-N view; full context is persisted.

## 5. Delivery Tasks

### Batch 0: Design Ledger

- [x] Create branch `codex/write-mode-commercial-workflow`.
- [x] Add this design and task ledger.
- [x] Commit `docs: record write-mode commercial workflow delivery plan`.
- [x] Push branch.

### Batch 1: Apply Boundary Approval

- [x] Add `PlanStatusBlocked`.
- [x] Add `WriteApprovalRecord` and deterministic plan fingerprint.
- [x] Persist approval record on `ChangePlan`.
- [x] Move final approval enforcement into `applyPreHook` after plan load and
      before worktree provisioning.
- [x] Keep `planPostHook` as a preview/compatibility check without relying on
      it as the final gate.
- [x] Update `/approve` to record explicit user approval with the current
      fingerprint.
- [x] Add plan-file apply tests for auto, manual, deny, and stale approval.
- [x] Commit, push, and update this ledger.

### Batch 2: Structured Risk Policy

- [x] Introduce a small shared safety package for structured write policy.
- [x] Preserve deterministic path classification.
- [x] Parse `package.json` with `encoding/json` for lifecycle scripts.
- [x] Parse workflow YAML with `yaml.v3` for privilege escalation.
- [x] Parse Android manifests with `encoding/xml` for sensitive permissions.
- [x] Detect PEM/private key material through exact boundary signatures.
- [x] Reuse operation-lane command approval ideas for write-mode
      `exec_command` policy without widening read-mode `exec_command`.
- [x] Reuse the existing agent-level `LoopPolicy` doom-loop guard for repeated
      identical tool calls, repeated structural error classes, and hint floods.
- [x] Add unit matrix tests for parser-backed high/critical signals.
- [x] Commit, push, and update this ledger.

### Batch 3: Priority Handoff

- [x] Add `WriteContextPack`, item priorities, sources, and consumer masks.
- [x] Project from `WriteAnalysisIR`, `TurnAArtifacts`, risk assessment,
      approval record, plan critique, and verify report.
- [x] Persist context packs on `WriteWorkflowRun` schema; the atomic store lands
      in Batch 4.
- [x] Render planner/verifier views from priority packs and expose controller
      views through typed `WriteContextPack.View`.
- [x] Add tests that high-priority constraints and evidence are preserved and
      low-priority noise is bounded.
- [ ] Commit, push, and update this ledger.

### Batch 4: Controller MVP

- [ ] Add `write_controller` agent/stage behind configuration.
- [ ] Add `emit_write_workflow_decision` tool using the existing typed
      decision schema.
- [ ] Add `WriteWorkflowRunStore` with atomic save/load/list.
- [ ] Support `explore_code`, `plan_change_batch`, `finish`, and `block`.
- [ ] Default `write_workflow_engine` to `legacy`.
- [ ] Add hygiene tests proving controller routing is typed, not prose-based.
- [ ] Commit, push, and update this ledger.

### Batch 5: Dynamic Batch Loop

- [ ] Add outer controller scheduler for enabled runs.
- [ ] Keep the existing `plan/apply/verify` graph as inner executor.
- [ ] Support append, split, replan, and follow-up batches.
- [ ] Enforce default budgets: 5 batches, 2 exploration rounds per batch.
- [ ] Wrap current write exploration subflow as `ReadExplorationRunner`.
- [ ] Add E2E dynamic two-batch test and verify-failure re-explore test.
- [ ] Commit, push, and update this ledger.

### Batch 6: CLI/REPL UX And Docs

- [ ] Add `/workflow show`.
- [ ] Route workflow-local approve/reject without discarding the whole run.
- [ ] Update `docs/user_guide.md`, `docs/architecture.md`, and
      `codrax.yaml.example`.
- [ ] Add REPL tests for workflow visibility and approval UX.
- [ ] Commit, push, and update this ledger.

### Batch 7: Commercial Hardening

- [ ] Run focused packages after each batch.
- [ ] Run `go test ./...`.
- [ ] Run `make test`.
- [ ] Confirm read-mode red lines, worktree cleanup, operation lane, and simple
      write flows remain stable.
- [ ] Consider switching the default engine only after the full matrix passes;
      otherwise retain `legacy` default for one release.
- [ ] Commit final ledger update and push.

## 6. Progress Ledger

| Batch | Status | Commit | Push | Tests |
| --- | --- | --- | --- | --- |
| 0 | complete | 81b7ebef | pushed | not run |
| 1 | complete | b4ab2eb7 | pushed | `go test ./internal/types -run 'TestPlanStatus\|Test.*ChangePlan\|Test.*Approval\|Test.*Fingerprint'`; `go test ./internal/writeflow`; focused `./internal/orchestrator`; focused `./internal/repl` |
| 2 | complete | 5dd96bb3 | pushed | `go test ./internal/safety`; `go test ./internal/writeflow`; `go test ./internal/tool -run 'TestExecCommand_ReadModeShellWriteGate\|TestWritePolicy\|Test.*Risk\|Test.*PromptHygiene'`; `go test ./internal/agent -run 'TestDefaultLoopPolicy_HasHistoricalValues\|TestLoopPolicy_IdenticalAfterSuccess_StopsImmediately\|TestLoopPolicy_IdenticalAfterFailure_AllowsTwoRetries\|TestLoopPolicy_IdenticalErrorStreak_ForcesStop\|TestLoopPolicy_MaxPerKey'`; focused `./internal/orchestrator` |
| 3 | ready_to_commit | pending | pending | `go test ./internal/types -run 'TestWriteContextPack\|TestWriteWorkflowRun\|TestWriteExploration\|TestMutableStateWriteExploration'`; `go test ./internal/writeflow -run 'TestContextPack\|TestAssessWriteRisk\|TestDecideWriteApproval'`; focused `./internal/agent`; focused `./internal/orchestrator` |
| 4 | pending | pending | pending | pending |
| 5 | pending | pending | pending | pending |
| 6 | pending | pending | pending | pending |
| 7 | pending | pending | pending | pending |

## 7. Acceptance Criteria

- All apply paths, including plan-file apply, pass through apply-pre approval.
- Critical writes are rejected before worktree creation.
- Low/medium risk remains low-friction under `auto_safe`.
- Dynamic controller work is disabled by default until the commercial matrix
  passes.
- Handoff context survives across controller, planner, verifier, and approval
  views with priority preserved.
- No hard logic depends on user/model prose keyword matching.

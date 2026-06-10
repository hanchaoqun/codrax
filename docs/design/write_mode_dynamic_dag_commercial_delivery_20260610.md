# Write Mode Dynamic DAG Commercial Delivery

Date: 2026-06-10
Status: Batch 7 implemented
Branch: codex/write-mode-commercial-workflow

## 1. Delivery Goal

Promote write mode from a legacy `plan -> apply -> verify` lane with an
optional controller wrapper into a controller-first dynamic DAG engine.

The old write-mode path has no customer compatibility requirement. This delivery
therefore removes the public legacy scheduler choice and makes the write
controller the canonical scheduler for `ModePlan`, `ModeApply`, and
`ModeVerify`, while preserving non-write modes:

- read mode byte identity and read-mode red lines;
- log, trace, data, operation, and computer-operation lanes;
- worktree-only mutation semantics for writes;
- `write_enabled: false` as the default capability gate.

This is a system-level implementation. It must not route by user-text keywords,
model prose, rationale summaries, or prompt wording. Hard gates may consume only
typed artifacts, repository facts, parsed structured files, and deterministic
policy output. Prompt text remains advisory.

## 2. Current Gap Ledger

The repository already has strong foundations:

- `write_controller` agent and `emit_write_workflow_decision` typed tool;
- `WriteWorkflowRun`, batches, edges, budget, and progress ledger;
- `WriteContextPack` with priority and consumer views;
- plan fingerprint and `WriteApprovalRecord`;
- structured write risk checks under `internal/safety`;
- final apply-pre approval gate;
- REPL workflow display and active-batch approval binding.

Remaining commercial gaps:

1. **Controller is still optional.** `write_workflow_engine` exposes
   `legacy|controller`, defaults legacy, and `Run()` still branches to legacy
   single-phase and static multi-phase schedulers.
2. **Controller delegates too much to the old fixed graph.** `plan_change_batch`
   still invokes `BuildWriteTaskGraph(ModeApply, ...)`; direct apply/verify
   actions are declared but not schedulable.
3. **Workflow persistence is snapshot-shaped, not resume-shaped.** Runs lack
   batch attempts, result refs, approval refs, timestamps, and restart
   consumption semantics.
4. **Mode semantics are inconsistent.** `ModePlan` and `ModeVerify` still map
   primarily to legacy stage behavior, while the controller only accepts
   `ModeApply` without a plan file.
5. **Permission policy is split.** Operation approval and write risk share
   concepts but not one typed allow/ask/deny engine; write agent command
   permissions are not expressed as per-agent policy.
6. **Handoff is rich but not durable enough.** Context packs are embedded in run
   snapshots; full evidence refs and consumer-specific projections need a
   durable store and stable references.
7. **Prompts still carry too much workflow procedure.** The controller must own
   routing through typed decisions; planner/verifier prompts should only provide
   batch-local soft guidance.

## 3. Target Architecture

`WriteAnalysisIR` seeds a durable `WriteWorkflowRun`. The controller repeatedly
emits one typed action, and the scheduler executes that action through narrow
executors:

```text
WriteAnalysisIR seed
  -> write_controller typed decision
  -> explore_code       -> read-only exploration runner -> WriteContextPack
  -> plan_batch         -> planner emits one bounded ChangePlan
  -> apply_plan         -> apply-pre permission/risk gate -> coder in worktree
  -> verify_batch       -> verifier typed result
  -> replan/split/append/explore/finish/block
```

Canonical mode behavior:

- `ModePlan`: controller may explore and plan bounded batches, then stop before
  apply. No worktree mutation.
- `ModeApply`: controller runs end-to-end. Low/medium risk auto-executes under
  policy, high risk pauses for approval, critical risk is denied.
- `ModeVerify`: controller verifies a workflow run, active batch, or imported
  plan seed through the same durable run object.
- `--plan-file`: imports an existing `ChangePlan` as a single batch seed. It
  never bypasses the controller or final apply-pre gate.

Public compatibility choice:

- Remove the public `write_workflow_engine` switch from docs/examples/config
  behavior.
- Controller is the only write-mode scheduler. Any retained legacy functions
  are internal helpers or temporary test fixtures, not user-facing behavior.

## 4. Permission And Approval Model

Use a shared typed permission model for write and operation lanes:

- action: `allow`, `ask`, `deny`;
- deny wins before allow;
- low/medium risk writes auto-run by default;
- high risk writes require explicit approval;
- critical risk writes are blocked before mutation;
- stale plan fingerprint invalidates a prior approval;
- external-directory, `.git`, secret-like path, workflow/config escalation,
  destructive command, and repeated-loop signals are policy inputs.

Hard gates must read only:

- repo-relative resolver output and worktree boundary checks;
- JSON/YAML/XML parser output;
- exact PEM/private-key signatures;
- typed command classifications;
- typed agent permission records;
- typed plan/risk/approval/workflow artifacts.

## 5. Priority Handoff Model

Persist full `WriteContextPack` artifacts and attach references to batches and
events. Consumers render bounded typed views:

- P0: user constraints, safety boundary, approval/risk, scope boundary;
- P1: target files, symbols, invariants, line-backed evidence;
- P2: test surface, verify failure, unknowns;
- P3: style and local pattern hints.

The controller uses P0/P1/P2 for routing. The planner uses P0/P1/P3 for bounded
implementation. The verifier uses P0/P2 and plan/apply refs. Raw tool output is
not stuffed into prompts directly.

## 6. Delivery Batches

### Batch 0: Design Ledger

- [x] Add this document.
- [x] Commit `docs: record dynamic write dag delivery plan`.
- [x] Push branch.

### Batch 1: Canonical Controller Engine

- [x] Route all write modes through the controller scheduler.
- [x] Redefine `ModePlan`, `ModeApply`, and `ModeVerify` behavior as controller
      modes.
- [x] Import `--plan-file` into a workflow seed rather than a legacy skip path.
- [x] Remove public `write_workflow_engine` config/docs/defaults.
- [x] Prove read/log/trace/data/operation dispatch remains untouched.

### Batch 2: Durable DAG Store And Resume

- [x] Extend `WriteWorkflowRun` with attempts, refs, timestamps, and replan
      metadata.
- [x] Add atomic load-active-or-create/resume behavior.
- [x] Preserve pending approval, verify failure, active batch, context refs, and
      budget state across process restart.
- [x] Add tests for resume, stale approval, and run normalization.

### Batch 3: Action Executors

- [x] Implement native `explore_code`, `plan_batch`, `apply_plan`,
      `verify_batch`, `append_batch`, `split_batch`, and `replan_batch`
      scheduling.
- [x] Stop using `BuildWriteTaskGraph` as the controller's main execution path.
- [x] Keep planner/coder/verifier agents but call them through action-specific
      executors.
- [x] Support verify-failure convergence through re-explore/replan/split.

### Batch 4: Unified Permission Engine

- [x] Extract shared allow/ask/deny permission primitives.
- [x] Reuse operation approval concepts without coupling write mode to operation
      workflow internals.
- [x] Add per-agent write command permissions.
- [x] Enforce external-directory and doom-loop policy through typed signals.

### Batch 5: Durable Priority Handoff Store

- [x] Persist context packs as artifacts and attach refs to runs/batches/events.
- [x] Project context from analysis, exploration, risk, plan, apply, and verify.
- [x] Render consumer-specific Top-N views from refs.
- [x] Add evidence-retention tests.

### Batch 6: Prompt Simplification And Hygiene

- [x] Reduce planner/verifier prompts to batch-local soft guidance.
- [x] Keep controller prompt focused on typed actions only.
- [x] Add prompt hygiene tests for keyword routing, prose hard gates, and
      unsupported controller actions.

### Batch 7: CLI/REPL And Docs

- [x] Add `/workflow show/list/resume/clear`.
- [x] Make `/approve` and `/reject` operate on the active batch.
- [x] Update `docs/architecture.md`, `docs/user_guide.md`, and
      `codrax.yaml.example`.
- [x] Update this progress ledger after each pushed batch.

### Batch 8: Commercial Hardening

- [ ] Run `go test ./...`.
- [ ] Run `make test`.
- [ ] E2E cover low-risk auto apply, high-risk approval, critical deny,
      imported plan gate, dynamic split/append, verify-failure replan, resume,
      and handoff evidence retention.
- [ ] Confirm non-write modes and worktree cleanup red lines.

## 7. Acceptance Criteria

- Controller is the only public write scheduler.
- Low/medium risk writes do not request unnecessary approval.
- High risk writes pause with auditable approval records.
- Critical risk writes fail before any apply mutation.
- Imported plans and generated plans use the same final gate.
- Dynamic DAG actions are scheduled from typed decisions only.
- Handoff evidence survives across controller turns and process restarts.
- No hard gate consumes user/model prose or keyword intent matching.
- Read mode, log/trace/data, operation mode, and worktree cleanup do not
  regress.

## 8. Progress Ledger

| Batch | Status | Commit | Push | Notes |
| --- | --- | --- | --- | --- |
| 0 | complete | `d4dc7840` | pushed | New controller-first ledger created. |
| 1 | complete | `24db3c62` | pushed | Write modes route through controller-first scheduler; canonical action executors handle plan/apply/verify; legacy engine config is compatibility-only. Tests: `go test ./internal/types ./internal/config ./internal/skill ./internal/writeflow ./internal/orchestrator`. |
| 2 | complete | `8dde5687` | pushed | Workflow run schema now carries timestamps, refs, and attempts; scheduler resumes active runs when no plan-file seed is supplied. Tests: `go test ./internal/types ./internal/writeflow ./internal/repl ./internal/orchestrator`. |
| 3 | complete | `24db3c62` | pushed | Action-level controller executors were delivered with the controller canonicalization batch: plan/apply/verify are directly schedulable, verify failure returns to controller under retry budget, and `BuildWriteTaskGraph` is no longer the controller main execution path. |
| 4 | complete | `181833a0` | pushed | Shared `allow/ask/deny` permission primitive added; write approval maps to shared permission decisions; write-mode `exec_command` uses typed permission output before running observation commands. Tests: `go test ./internal/safety ./internal/tool ./internal/writeflow ./internal/operation`. |
| 5 | complete | `e15d884d` | pushed | `WriteContextPack` artifacts are persisted under workflow context dirs and batch refs are attached to durable runs. Tests: `go test ./internal/types ./internal/repl ./internal/orchestrator ./internal/agent`. |
| 6 | complete | `f363be76` | pushed | Planner prompt now scopes to the active workflow batch and typed context pack; controller prompt hygiene tests pin canonical actions and forbid prose-routing/unsupported action drift. Tests: `go test ./internal/skill`. |
| 7 | implemented | pending | pending | `/workflow resume` and `/workflow clear` added for durable write runs; clear removes context artifacts; user and architecture docs now describe controller-first write mode. Tests: `go test ./internal/repl`. |

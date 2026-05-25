# Sub-agent Default-on Optimization Plan (2026-05-25)

## Decision

Keep `propose_sub_agents` enabled by default. The feature is useful as a
model-driven scoped investigation worker, but it must not compete with the
main explorer as a second unbounded breadth engine.

The optimized contract is:

- sub-agents may help the model investigate independent scopes in parallel;
- sub-agents receive a read-only context and never mutate the parent
  `MutableState`;
- sub-agents may use navigation tools to decide where to look, but navigation
  output is not evidence;
- rich sub-agent prose, JSON, tables, diagrams, logs, and trace summaries are
  preserved as advisory handoff, not promoted to citations unless existing
  grounding paths verify them;
- the system must not use user-question keywords or model prose to decide
  control flow.

## Current Code Findings

### Tool visibility

`internal/agent/sub_explorer.go` currently creates a scoped skill with:

```go
ToolSuggestions: []string{"read_file", "list_files", "grep"}
```

This makes `sub_explorer` depend on broad listing and grep even when the parent
run already has a reusable repo graph. It is weaker than the main explorer for
large or cross-language scopes because it cannot call `repo_map` or
`source_inventory`.

### Read-only graph handoff

`BuildSubAgentContext` already copies a read-only graph handle into
`AgentContext.SearchGraph`. That is the right boundary: sub-agents should be
able to reuse the graph without receiving `Mutable`.

However, tool dispatch narrows `AgentContext` back to `BusContext` through
`types.ToolBusContext`, and that function does not currently copy
`AgentContext.SearchGraph` into `BusContext.SearchGraph`. `repo_map` also
looks mainly at `BusContext.Mutable.SearchGraph()` in
`GraphFromBusContextOrLoad`. Therefore simply exposing `repo_map` to
`sub_explorer` would risk rebuilding scoped indexes instead of reusing the
parent graph.

### Concurrency safety

The critical race has already been fixed:

- `SubExplorer.Run` creates a fresh evaluator/base per request.
- `BuildSubAgentContext` intentionally drops `Mutable`.
- `SubAgentReducer` is the only merge point.

Remaining risk: `TypedDenials` in `SubAgentContext` points at
`bus.TypedDenials`. Multiple parallel sub-agent tool calls can append denials.
Today most sub-agent tools only read denials; nevertheless, future write paths
must not add unsynchronized writes through this shared pointer.

### Advisory handoff

Recent changes already carry bounded `InvestigationNotes` through
`SubAgentResult`, `SubAgentReducer`, and `orchestrator.applyStageOutput` into
`TurnAArtifacts.InvestigationNotes`.

Remaining risk: large rich artifacts are truncated before downstream stages see
them. That is acceptable for prompt budget only if the full original can be
persisted as a blob/ref or the truncation is explicit. It must never silently
turn a rich scoped finding into a missing finding.

### Evidence quality

`subExplorerEvaluator.Observe` counts only legacy markdown bullets beginning
with `- [DIRECT]` or `- [REGISTRATION]`. Rich JSON/prose can be informative but
will still look weak to soft-stop. This should not make advisory artifacts
disappear or trigger broad refetch loops.

## Commercial-grade Design

### P0: Navigation parity without mutation

Expose `repo_map` to `sub_explorer`, but only after the read-only graph reuse
path is complete.

Implementation requirements:

- Copy `AgentContext.SearchGraph` into `BusContext.SearchGraph` in
  `types.ToolBusContext`.
- Teach `repo_map.GraphFromBusContextOrLoad` to reuse `BusContext.SearchGraph`
  before falling back to `Mutable.SearchGraph()` or rebuilding.
- Add `repo_map` to the `sub_explorer` skill only after the two changes above.
- Update `sub_explorer` prompt text so it teaches the same progressive lens
  pattern as the main explorer:
  broad `repo_map` summary -> narrower `source_inventory` by model-chosen
  scope/roles -> `read_file`/targeted `grep` verification.
- Keep unavailable tools out of the prompt. Do not mention `emit_*`,
  `exec_command`, or nested `propose_sub_agents` as available actions.

Validation:

- Unit test: `ToolBusContext` preserves `SearchGraph`.
- Unit test: `GraphFromBusContextOrLoad` reuses a read-only
  `BusContext.SearchGraph` when `Mutable` is nil.
- Unit test: sub-explorer schemas include `repo_map` and exclude `emit_*`,
  `exec_command`, and `propose_sub_agents`.
- Prompt test: sub-explorer instruction mentions `repo_map(view="source_inventory")`
  and still says navigation rows must be verified with `read_file` or `grep`.

### P1: Rich advisory artifact preservation

Keep the existing `TurnAArtifacts.InvestigationNotes` wheel, but make large
sub-agent advisory artifacts durable.

Implementation requirements:

- When a sub-agent advisory note exceeds the inline advisory budget and
  `WorkDir` is available, persist the full text via the existing blob artifact
  mechanism.
- Keep prompt-facing text bounded and explicit: show a preview plus a stable
  artifact path/ref.
- Do not promote advisory text to `EvidenceItem` unless the existing parser and
  grounding path verify it.
- Preserve diagrams/tables/log summaries as text; do not wrap or reformat them
  in the handoff layer.

Validation:

- Unit test: long advisory note keeps a visible truncation notice and a blob
  reference when `WorkDir` is set.
- Unit test: no `WorkDir` degrades to explicit preview-only truncation.
- Unit test: advisory notes do not satisfy evidence gates by themselves.

### P2: Scope and overlap governance

Keep the default-on behavior, but prevent obviously unsafe or wasteful
sub-agent proposals.

Implementation requirements:

- Normalize each `SubTask.Scope` against the active repo/sub-repo boundary.
- Reject parent escape and absolute-path escape using existing path/active-set
  helpers where possible.
- Deduplicate exact duplicate scopes.
- Treat parent/child overlap as advisory by default, not a hard reject, unless
  it is machine-obvious that every proposed scope is the same target.
- Surface overlap telemetry so eval can decide whether future default-on
  behavior should be narrowed.

Validation:

- Unit tests for parent escape, absolute escape, duplicate scopes, and
  parent/child overlap.
- No test may depend on a specific project path or Go-only symbol shape.

### P3: Observability and eval

Add metrics/debug counters, not answer-time decisions:

- sub-agent proposal count;
- sub-agent branch count;
- effective parallelism;
- repo_map graph reused vs rebuilt;
- duplicate read/grep ratio across sub-agent branches;
- advisory bytes kept inline vs persisted;
- structured evidence items accepted from sub-agent branches.

Eval:

- one broad source inventory case;
- one mechanism/call-chain case;
- one multi-language scope case;
- one attached log/trace + source correlation case;
- one overlapping proposal fixture.

## Red Lines

- Do not use user-question keywords or model prose to trigger sub-agent
  control flow.
- Do not hard-reject model output unless the violation is machine-verifiable
  and local to the sub-agent proposal structure or active repository boundary.
- Do not convert advisory notes into citations.
- Do not expose tools in prompts that are not in the active schema.
- Do not introduce a second advisory ledger when `TurnAArtifacts` already
  exists.

## Task Checklist

- [x] P0-A: preserve read-only `SearchGraph` through `ToolBusContext`.
- [x] P0-B: reuse `BusContext.SearchGraph` in `repo_map`.
- [x] P0-C: expose `repo_map` to `sub_explorer`.
- [x] P0-D: update sub-explorer prompt and schema tests.
- [x] P1-A: persist long advisory artifacts through existing blob helpers.
- [x] P1-B: add advisory preview/ref tests.
- [x] P2-A: normalize and validate sub-task scopes.
- [x] P2-B: add dedupe/overlap governance and tests.
- [x] P3-A: add sub-agent observability counters.
- [ ] P3-B: run targeted tests and eval samples.

## P3 Validation Notes

- 2026-05-25 targeted unit tests and `go test ./...` passed after P3-A.
- `eval/cases/qf_relation_subagent_registry.case` passed 1/1 with no finalizer
  rejects and no tool-history pruning.
- While running `eval/cases/read_combo_pipeline_sequence_table.case`, the logs
  exposed a generic prompt/tool-surface drift: an evidence-repair hint described
  the *current* emit-only surface while the next filtered surface allowed a
  surgical `read_file`. This is not a stage/pipeline-specific issue. The fix is
  to render repair guidance from the next effective repair surface after the
  evaluator updates repair state, so model-facing context cannot contradict the
  tools it will actually see next.
- The same interrupted eval also reconfirmed a separate long-tail gap outside
  the sub-agent default-on work: after an upstream LLM stream stall, the read
  scheduler can restart broad exploration even though the previous attempt had
  already accepted grounded evidence. This must be handled by the transient
  retry / evidence-checkpoint workstream, not by weakening sub-agent behavior or
  adding answer-specific gates. The run was stopped after the diagnostic signal
  was captured because it used a pre-fix binary and no longer validated this
  batch.

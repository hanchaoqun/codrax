# Eval Gap Systemic Remediation Plan — 2026-06-04

## Goal

Turn the latest representative eval findings into systemic fixes. The scope is
not to patch individual cases. Each fix must consume typed system signals,
preserve existing code/log/trace/write-mode behavior, and avoid hard gates based
on user prose keywords or model free-form answer text.

Source issue ledger:

- `docs/design/eval_representative_run_20260604_203331.md`

## Red Lines

- No keyword matching on user intent or model prose for hard routing/gating.
- No conversion of external observations into current-source citations unless a
  typed current-source lane is required or exact current-source support exists.
- No disruption to existing write mode, code read mode, trace/log analysis, MCP,
  or multi-repo default behavior.
- Keep precise typed signals for hard gates; prompt text remains soft guidance.

## Existing Building Blocks

The codebase already has reusable structures:

- Runtime frame drift detection:
  - `internal/analysis/logtriage.DetectDriftForFrame`
  - `internal/types.FrameDriftStatus`
  - prompt rendering in `internal/context/builder.go`
- External observation lanes:
  - `CurrentSourceLaneDecision`
  - `ExternalObservationPolicy`
  - `ExternalObservationSufficiency`
  - `ObservationLedger`
- Aggregate fact origin projection:
  - `AnswerAggregateFactEvidenceOrigins`
  - `CompileAnswerClaimBindingsFromAggregateFacts`
- Operation execution:
  - command plan lint/replan/continuation
  - operation materials and payload refs
  - `OperationEvaluationStatus`
- Multi-repo disclosure:
  - `BuildInactiveScopeDisclosureObligationFromBus`
  - `answerDocumentPrincipalSlateIsEmpty`

The remediation should wire these structures together more completely rather
than introduce parallel subsystems.

## Batch A — Runtime Drift Authority Boundary

### Problem

Runtime stack-frame drift is currently rendered as prompt guidance. The model
can notice the drift during exploration and still write a current-source causal
claim later.

### Design

Add a typed drift-boundary carrier derived from log/perf frames and current
symbol locator results. The carrier should list drifted runtime frames and their
authority ceiling. Finalizer/evaluator inputs consume that carrier as a
structural boundary:

- Drifted frames remain valid runtime observations.
- Drifted frame file:line must not satisfy current-source causal proof.
- Current-source explanation is allowed only with separate grounded current
  source evidence.

### Tasks

- Add typed helper to compile drifted runtime frame bindings from RequestModel
  log/perf bundles.
- Render the typed boundary in finalizer dynamic instructions.
- Add a pre-emit/evaluator guard that rejects or repairs current-source hard
  claim pressure from drifted runtime frames without separate current support.
- Add tests around `logtri_go` shape: line drift must not become a current
  source root cause.

## Batch B — Operation Unified Evaluator Gate

### Problem

The command operation loop can discover the next safe material/action but still
finalize early with a partial answer. This is not specific to HTML: it applies
to local files, command payload refs, MCP/Skill provider payloads, and downloaded
documents.

### Design

Use a unified operation evaluator before final answer generation. It consumes
typed operation records, materials, command outcomes, known next actions,
success criteria, and budget state.

Evaluator statuses:

- `complete`
- `continue_command`
- `continue_provider`
- `needs_clarification`
- `blocked`
- `budget_exhausted`
- `partial_answer_possible`

The command CLI/REPL loop should not emit a final answer when evaluator returns
`continue_command` and budget remains. If budget is exhausted, the final answer
must say why and list the safe next action.

### Tasks

- Add deterministic material coverage evaluator for command records.
- Integrate evaluator into CLI and REPL command operation finalization.
- Keep existing planner continuation as the way to produce the next batch; the
  evaluator decides whether finalization is allowed.
- Add YAML-configurable command operation max rounds if current defaults are too
  tight.
- Add tests for material refs discovered but not consumed, and for budget
  exhaustion.

## Batch C — External-Origin Aggregate Fact Defaulting

### Problem

Originless `aggregate_facts` default to `current_source`. In MCP/runtime-only
answers this creates current-source claim bindings even though the typed ledger
already has sufficient external observations.

### Design

Make aggregate-fact origin fallback lane-aware:

- Explicit origin dimensions/provenance/support refs still win.
- When the current-source lane is required, preserve current-source defaults.
- When typed external observation sufficiency exists and current source is not
  required, originless aggregate facts inherit the dominant external origin
  (MCP/runtime/web/command/etc.) or become origin-specific supporting facts.

### Tasks

- Extend `AnswerAggregateFactEvidenceOrigins` to inspect structured support refs
  as origin tokens.
- Add a typed external-origin fallback helper for RequestModel/external
  sufficiency contexts.
- Update claim binding tests so MCP/runtime aggregate facts do not synthesize
  current-source bindings.

## Batch D — Multi-Repo Principal Slate Detection

### Problem

Inactive-scope disclosure treats a correct scalar/summary exact answer as an
empty principal slate because it only recognizes list/table principal blocks.

### Design

Broaden principal-slate detection using typed answer document structure:

- Principal `BlockScalar` with non-empty text is a principal answer.
- Principal `BlockSummary` with non-empty text can satisfy locate/scalar
  answers when exact resolution is not absent.
- Exact-resolution exact match with a concrete anchor/value is populated.
- List/table behavior remains unchanged.

### Tasks

- Update `answerDocumentPrincipalSlateIsEmpty`.
- Add tests for principal scalar, exact-match summary/scalar, and true absence.
- Re-run multi-repo focused eval.

## Batch E — Trace/Perf Provenance Wording

### Problem

Answers can say `trace_query` ran even when the actual evidence came from
perf-triage or attached-trace structured observations.

### Design

Expose typed trace evidence producer to finalizer:

- `trace_query` tool result
- attached trace / perf triage bundle
- model-authored aggregate closure

Finalizer prompt should phrase provenance neutrally unless a `trace_query` tool
result is present. This is a presentation/provenance correctness fix, not a
semantic root-cause gate.

### Tasks

- Add finalizer dynamic instruction or claim binding field for trace evidence
  producer.
- Add a test where perf-triage facts exist and no `trace_query` tool result
  exists; answer guidance must not claim trace_query execution.

## Batch F — Eval Semantics

### Problem

PASS alone missed answer-quality failures.

### Design

Add semantic expectations for representative cases. This is not a runtime gate;
it protects regressions.

### Tasks

- Strengthen `logtri_go` expectations for drift disclosure.
- Strengthen operation web/material cases to require actual target material
  coverage or explicit budget exhaustion.
- Add operation metrics: command rounds, material refs discovered/consumed,
  terminal evaluator status.


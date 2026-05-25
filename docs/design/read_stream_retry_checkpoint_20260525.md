# Read-Mode Stream Retry Evidence Checkpoint

Date: 2026-05-25

## Problem

A read-mode exploration dispatch can make durable progress, then fail on a
stream-level transport error before `dispatchStage` returns cleanly. Existing
protection only covered the narrow case where `emit_investigation_complete` had
already produced an accepted closure. When the model had accepted evidence but
not yet closed the investigation, the scheduler requeued the whole explorer
window with the ordinary DAG hint. The next prompt looked like a fresh
investigation, so the model could restart broad `repo_map` / `grep` / `read_file`
work and lose the user's high-value progress in practice.

This is not a tool-call budget issue. It is a retry-continuation contract issue.

## Constraints

- Do not mark exploration complete merely because evidence exists. That would
  let the system decide sufficiency for the model.
- Do not inspect user prose or model narrative for control flow.
- Do not hard-block legitimate follow-up search.
- Preserve the model's ability to decide whether to continue reading or call
  `emit_investigation_complete`.
- Reuse existing state: `MutableState`, `TurnAArtifacts`, DAG window hints, and
  pre-prune checkpoint behavior.

## Design

Add a transient retry checkpoint for explore-stage stream errors that happen
after durable progress but before accepted closure.

The checkpoint is advisory and model-facing:

1. Detect durable progress from structured state only:
   - accepted / mutable evidence,
   - bus evidence, flow findings, answer chains, symbols,
   - accepted aggregate facts / closure state,
   - already captured read/tool results from the same dispatch.
2. Store a one-shot retry directive on the scheduler state.
3. On the next explorer dispatch, prepend that directive to the normal DAG
   window hint.
4. The directive says this is a continuation from a preserved checkpoint, not a
   fresh investigation. It asks the model to reuse accepted evidence and avoid
   repeating broad repo-wide scans unless the checkpoint is clearly missing the
   needed target. It may continue with narrow follow-up reads or close with
   `emit_investigation_complete`.

The accepted-closure fast path remains separate: if the model already emitted a
valid closure before the stream failure, the scheduler may safely advance. The
new path never advances by itself; it only improves the retry prompt.

## Remaining Gap Audit

- Pre-prune checkpoint: already implemented in `agent.go` and covered by unit
  tests. It preserves accepted structured state before raw tool history is
  elided.
- Analyze stage: usable IR after a transient failure already advances rather
  than reanalyzing.
- Extract stage: standalone transient retry is already separated from content
  retry budget.
- Finalize stage: accepted / recoverable finalizer output has its own transient
  preservation path.
- Open follow-up: measure how often explore transient retries install this
  checkpoint, and whether any later broad scan still occurs without new scoped
  rationale.

## Task List

- [x] Document root cause and safe boundary.
- [x] Add one-shot explore transient checkpoint hint to `graphState`.
- [x] Install the hint only for stream-level explore retry with structured
      progress and no accepted closure.
- [x] Prepend the hint to the next window hint without replacing normal DAG
      objectives.
- [x] Add regression tests for evidence-before-stream-stall retry continuation.
- [x] Run focused tests and full Go test suite.

## Eval Verification: `read_combo_pipeline_sequence_table`

Run:

```bash
source eval/runner_lib.sh && \
eval_run_with_timeout 1800 env \
  EVAL_RESULTS_ROOT=.codrax/eval-stream-checkpoint-20260525 \
  bash eval/run.sh eval/cases/read_combo_pipeline_sequence_table.case 1
```

Result directory:

`.codrax/eval-stream-checkpoint-20260525/read_combo_pipeline_sequence_table-20260525-225308`

Result: PASS.

Key metrics:

- `tool_read_file=17`
- `tool_repo_map=1`
- `tool_history_prunes=5`
- `midloop_inject=6`
- `max_context_tokens_est=71954`
- `explorer_iters=17`
- `finalizer_iters=1`
- `finalizer_rejects=0`
- `finalizer_rewrites=0`
- `semantic_quality_concerns=0`

Positive signals:

- No finalizer JSON/prose recovery was needed. The finalizer emitted
  `emit_answer_document` once, with the requested Mermaid `sequenceDiagram` and
  a stage input/output/state-carrier table.
- The pre-prune checkpoint path fired as designed: `TOOL HISTORY PRUNED`
  appeared 5 times and injected structured checkpoints before older raw tool
  results were elided.
- The exploration handoff preserved accepted closure text, read files, and
  deterministic evidence into extractor/finalizer context. The final answer did
  not collapse to a skinny response.
- No stream-level retry happened in this concrete run. The new partial-evidence
  stream retry path remains covered by unit tests, while runtime eval coverage
  requires a deterministic fault-injection provider or harness.

Deep findings from the same run:

1. **Explorer remains too expensive for pipeline / lifecycle explanation
   questions.**

   The run passed, but explorer spent 17 iterations, read 17 files, injected 6
   mid-loop hints, and reached ~72k estimated context tokens. The model built
   the answer by reading large windows of `orchestrator.go`, `scheduler.go`,
   `context.go`, agent evaluators, and topology files. The deeper root cause is
   that Codrax has repo-map navigation and evidence checkpoints, but lacks a
   low-cost, generic "lifecycle / pipeline / handoff lens" that can expose
   already-known stage, component, input/output artifact, and state-carrier
   relations as advisory navigation. Without that lens, both small and large
   models fall back to repeated large `read_file` windows to reconstruct the
   lifecycle manually.

   Safe follow-up direction: add a generic relation/lifecycle view on top of
   existing graph and typed evidence surfaces. It must be advisory-only, typed by
   structural relations rather than user keywords, and must still require
   `read_file`/`emit_evidence` for cited final claims.

2. **Read-without-emit hints work, but fire late and do not cap context growth.**

   The current code intentionally waits for at least 2 iterations and 2
   successful `read_file` calls before `postReadWithoutEmitSignal` fires. In this
   eval it repeatedly allowed 2-3 large read windows to accumulate before
   pushing `emit_evidence`, then tool-history pruning had to clean up. That is
   safe, but not efficient.

   Safe follow-up direction: introduce a structured "read window checkpoint"
   policy that summarizes or materializes high-value read windows earlier, while
   preserving model autonomy. Do not hard-close; only constrain continued broad
   reads after the model has already been asked to materialize evidence.

3. **Pre-prune checkpoint protects correctness but can still grow large.**

   Checkpoint injection reached about 10.6k bytes. This is far better than
   losing evidence, but it means repeated prune cycles still carry a sizable
   checkpoint plus live read windows.

   Safe follow-up direction: compact checkpoint rows by request facets and
   salience, while keeping accepted aggregate facts and closure reason intact.
   Never drop accepted principal evidence silently; omit only lower-salience or
   duplicate support rows with an explicit count.

4. **Eval `source_inventory_lens` metric is noisy.**

   The summary reported `source_inventory_lens=11`, but the log has only one
   actual `repo_map` tool call and it used `view="task_map"`. The metric pattern
   currently counts prompt text and advisory hints mentioning
   `source_inventory`, not just actual tool calls or tool results.

   Safe follow-up direction: update eval telemetry to count actual
   `tool=repo_map params=...view":"source_inventory"` calls separately from
   prompt/hint exposure. Otherwise Repo Lens adoption metrics can produce false
   confidence.

5. **A soft lane/block-kind violation was recorded but did not hurt output.**

   `answer_contract_check` reported one `lane_block_kind` violation, then
   semantic quality passed with confidence 1.00 and no rewrite. This is the
   intended conservative behavior for a soft support-lane mismatch, but the
   summary only exposes `semantic_quality_concerns=0`, not soft contract
   violations.

   Safe follow-up direction: add eval metrics for soft contract violations by
   kind. Keep them non-blocking unless the same soft violation correlates with
   visible answer loss in future runs.

Open tasks after eval:

- [ ] Add deterministic stream-stall fault-injection eval that interrupts after
      accepted `emit_evidence` but before `emit_investigation_complete`, so the
      new continuation checkpoint is exercised outside unit tests.
- [ ] Design generic lifecycle / relation handoff lens for pipeline, route,
      config, dependency, handler, state-machine, and cross-language component
      questions.
- [ ] Tune read-window checkpointing so large read bursts are materialized or
      summarized earlier without forcing closure.
- [ ] Separate actual `source_inventory` tool usage from prompt/hint exposure in
      eval telemetry.
- [ ] Track soft contract violations in eval metrics.

## Generalized Remediation Plan

The remaining gaps are not specific to the pipeline eval or this repository.
They fall into three reusable classes: telemetry contamination, expensive
navigation before evidence materialization, and soft structural warnings that
are invisible to eval summaries.

### Red Lines

- Do not use user-prose keywords or model narrative text to decide control flow.
- Do not convert advisory navigation into answer facts.
- Do not hard-reject unless the conflict is machine-checkable against structured
  evidence, the requested answer axis is clear, and a local repair path exists.
- Preserve user intent and model-authored conclusions. System-added material
  must remain visibly supplemental and localized.

### Batch 1: Observability Contracts

Problem: `source_inventory_lens` currently counts prompt text and model-visible
guidance mentioning `source_inventory`, even when the model never called
`repo_map(view="source_inventory")`. Soft answer-contract mismatches are also
logged but not summarized in eval metrics.

Design:

- Count actual Source Inventory adoption only from timestamped control-plane
  tool-call logs where `tool=repo_map` and the JSON arguments include
  `view:"source_inventory"`.
- Keep prompt/hint exposure separate from tool adoption. Do not mix the two in
  one metric.
- Sum `answer_contract_check section=... violations=N` control logs into eval
  metrics, including a section-specific count for `lane_block_kind`.
- Add shell tests that quote control-looking lines inside model content and
  verify the metrics stay zero.

### Batch 2: Generic Relation / Lifecycle Navigation

Problem: lifecycle, route/handler, config, dependency, dataflow, state-machine,
and cross-repo questions often force models to reconstruct relation tables by
reading many large files. Codrax has repo-map graph data and typed evidence, but
does not yet expose a low-cost relation/lifecycle navigation surface.

Design:

- Build on existing repo-map / source-inventory / typed-relation concepts
  instead of creating a parallel evidence system.
- Treat relation views as advisory navigation only. They can suggest files,
  symbols, relationship candidates, and ambiguity, but final claims still need
  `read_file` / `grep` / typed evidence.
- Let the model drive scope and role expansion through tool parameters. The
  system may present compact summaries and next-call suggestions, but must not
  infer a final answer axis from user prose.
- Cover common relation families generically:
  lifecycle / stage handoff, route to handler, config key to consumer, type to
  method, interface to implementation, import/dependency, caller/callee,
  producer/consumer, state transition, artifact/log section to source symbol,
  and cross-repo service/module links.
- If a relation family is not recognized, fall back to source-inventory style
  candidates and explicit uncertainty, not hard rejection.

Implementation status:

- [x] Added `repo_map(view="relation_map")` as a model-driven advisory lens.
      Parameters are structural (`sources`, `scope`/`scopes`,
      `relation_kinds`, `query`, `top_n`) and do not read user prose or model
      narrative for control flow.
- [x] The view currently exposes graph-backed call, import, inheritance,
      implements, reference, and type-usage rows across the languages already
      parsed by repo_map. It also lists concrete verification files so the model
      can choose focused `read_file` / `grep` follow-up.
- [x] Broad navigation discovery hints now offer two generic next paths:
      `source_inventory` for member/attribute checklists and `relation_map` for
      structural edge inspection. Both are advisory-only and require source
      verification before citation.
- [ ] Future extension: feed accepted typed evidence / external observations
      into a unified relation lens once those carriers have a low-noise query
      shape. Keep graph-only rows advisory until exact evidence exists.

### Batch 3: Read-Window Materialization

Problem: when a model reads several large windows before emitting evidence, the
system may prune raw tool history later. Pre-prune checkpoints protect accepted
evidence, but large unmaterialized read windows still inflate context and make
retry prompts fragile.

Design:

- Use structured tool results and current schema only. Do not parse model prose
  for intent.
- Earlier advisory checkpoint: after repeated successful reads without evidence,
  ask for a compact evidence batch or explicit skip reason before more broad
  navigation.
- Never force closure. The model may continue investigating, but should do so
  from materialized evidence rather than raw history.
- Keep checkpoint compaction conservative: accepted principal evidence,
  aggregate facts, closure reason, and model-authored summaries must not be
  silently dropped.

Implementation status:

- [x] Existing two-read `read_file` without `emit_evidence` nudge remains the
      default materialization guard.
- [x] Added an earlier large-window guard based only on structured
      `read_file` banners/result size. A single large unrecorded read window can
      now prompt the model to emit a compact evidence batch before more broad
      navigation or later tool-history pruning.
- [x] The guard is advisory-only. It does not close the investigation, does not
      infer user intent, and does not restrict tools unless the model ignores
      the existing escalation path.

### Batch 4: Deterministic Runtime Regression

Problem: the stream retry fix is covered by unit tests, but runtime eval needs a
fault-injection provider or harness path to prove retry continuation under real
orchestrator flow.

Design:

- Add a deterministic eval or harness that fails the stream after accepted
  `emit_evidence` and before `emit_investigation_complete`.
- Verify the next explorer turn receives the continuation checkpoint and does
  not restart broad navigation unless the model chooses a scoped reason.

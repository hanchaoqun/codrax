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

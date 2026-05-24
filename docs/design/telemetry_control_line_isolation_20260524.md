# Telemetry Control-Line Isolation

Date: 2026-05-24

Status: T16 complete.

## Goal

Eval and telemetry metrics must describe system control flow, not words that
appear inside answers, source code, logs, traces, command output, or customer
snippets. This is especially important for retry/reject metrics: if a user asks
about code that contains `finalizer_rewrites`, `repair_plan`, or a quoted
customer log with `成文校验未通过`, the eval harness must not treat those words as
fresh system failures.

This batch is telemetry-only. It must not change product prompts, model
decisions, finalizer gates, answer rendering, or runtime retry policy.

## Red-Line Contract

- Count retry/reject/rewrite/repair events only from structured control lines.
- Do not infer a retry from answer text, source snippets, model free-form prose,
  or tool payloads.
- Shared helper functions are the only allowed path for finalizer rejects,
  finalizer rewrites, answer-document patch calls, semantic/self-consistency
  reviewer counts, mid-loop injects, and per-agent iteration/dispatch counters.
- If a metric cannot be scoped to an unambiguous control line, keep it as a
  content/search metric and do not use it to flag finalizer instability.

## Code Audit

| Surface | Current state before T16 | Risk |
| --- | --- | --- |
| `eval/runner_lib.sh` | Already has `eval_count_finalizer_rejects` and `eval_count_finalizer_rewrites`. | Patterns were not timestamp-anchored, so a quoted full log line inside `ASSISTANT content` could still match. |
| `eval/run.sh` | Uses finalizer helpers, but still uses broad `count_pattern` for `midloop_inject`, per-agent iterations/dispatches, semantic reviewer counts, and patch calls in sweep scripts. | Source snippets can contain strings such as `repair_plan`, `self_consistency_reviewer`, `MIDLOOP inject`, or `diag finalizer`. |
| `eval/parallel_all.sh` / `parallel_priority.sh` | Use shared finalizer reject helper, but patch/semantic/self counters still use broad grep. | Sweep summaries can report phantom patch/reviewer activity. |
| `eval/telemetry/main.go` | Finalizer reject/rewrite logic is partially scoped, but render-stage, analyzer retry, repair-plan, and consistency counters can still read payload text. | Telemetry reports can rank the wrong logs as hot retry sources. |

## Design

1. Add shared bash helpers in `eval/runner_lib.sh`:
   - `eval_count_agent_iterations(file, agent)`
   - `eval_count_agent_dispatches(file, agent)`
   - `eval_count_midloop_injects(file)`
   - `eval_count_answer_document_patch_calls(file)`
   - `eval_count_semantic_quality_dispatches(file)`
   - `eval_count_semantic_quality_concerns(file)`
   - `eval_count_self_consistency_concerns(file)`

   These helpers match timestamped Codrax control lines only.

2. Tighten existing finalizer helpers so they require timestamped
   `[diag finalizer]` / `[render]` control lines.

3. Replace direct grep usage in eval runners with the shared helpers where the
   metric is control-plane telemetry.

4. Tighten `eval/telemetry/main.go` to:
   - parse render stages and first-draft previews only from `INFO [render]`;
   - parse repair plans only from `INFO [orchestrator] repair_plan:`;
   - parse self-consistency activity only from render control lines or
     `INFO [self_consistency_reviewer]`;
   - treat `模型响应出错` as an LLM error only on render control lines.

5. Add regression tests with payload/source lines that quote full control-like
   strings. Those payload lines must not increment control counters.

## Task List

| Task | Status | Validation |
| --- | --- | --- |
| T16.1 Audit existing eval metric paths and document control/content split. | Done | This document |
| T16.2 Add/anchor shared bash control-line helpers. | Done | `bash eval/runner_lib_test.sh` |
| T16.3 Route eval runners through shared helpers. | Done | `bash eval/runner_lib_test.sh`; focused `eval/run.sh` metrics smoke |
| T16.4 Tighten Go telemetry collector against payload contamination. | Done | `go test ./eval/telemetry` |
| T16.5 Recompute focused mixed-VCS metrics after implementation. | Done | `CASES='eval/cases/read_combo_git_two_diffs_current_code.case' PARALLEL=1 RUNS=1 TIMEOUT=1500 bash eval/convergence_audit.sh` |

## Completion Notes

- Bash eval helpers now require timestamped Codrax control lines for
  finalizer rejects, finalizer rewrites, answer-document patch calls, mid-loop
  injects, semantic/self reviewer counters, and per-agent iteration/dispatch
  counts.
- `eval/run.sh`, `parallel_all.sh`, and `parallel_priority.sh` consume the
  shared helpers rather than local broad grep for these control metrics.
- Go telemetry now parses render-stage, first-draft, analyzer retry,
  finalizer rewrite/reject, repair-plan, and self-consistency activity only
  from structured control lines. Quoted customer/source/model text is ignored.
- Focused mixed VCS/current-source replay
  `read_combo_git_two_diffs_current_code-20260524-191054` passed with
  `finalizer_iters=1`, `finalizer_reject=0`, `finalizer_rewrite=0`,
  `midloop=2`, and no flags.

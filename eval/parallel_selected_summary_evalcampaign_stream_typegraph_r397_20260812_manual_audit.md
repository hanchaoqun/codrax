# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T16:28:02Z
- sweep_start_ts: 20260812-092801
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cond_resolve_stall_timeout | PASS | eval/results/cond_resolve_stall_timeout-20260812-092802 | answer_regex | none | 112s | 24 | read=3,repo_map=0,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Correctly distinguishes the mid-stream byte-silence watchdog (`streamStallTimeout`, default 120s / `llm_stream_stall_timeout_seconds`) from the pre-progress watchdog (`streamFirstByteTimeout`, default 180s / `stream_first_byte_timeout_seconds`). One wording nuance compresses “first usable progress” and raw byte activity, but it does not change the selected field or configuration answer. No finalizer retry. |
| 2 | qf_type_relation_loop_controller | FAIL | eval/results/qf_type_relation_loop_controller-20260812-092802 | answer_regex,answer_contains | none | 132s | 24 | read=13,repo_map=3,list=0,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | The textual roster and file locations cover the 12 production implementations, but the explicitly requested relation diagram disappears. The first model draft contained all 12 relations with reversed `LoopController -> implementer` arrows; the strict exact typed gate correctly rejected them. The repair context then falsely said no typed carrier existed and steered deletion, despite the same final prompt already carrying 12 exact `implementer -> LoopController` rows. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit conclusions

### Stream liveness and the “active for four minutes” question

The stream case is a real production PASS. Cold code review of `internal/llm/openai.go`
and its watchdog tests confirms that Codrax does not terminate or degrade an active
stream merely because its cumulative age exceeds four minutes (and the literal `4ms`
has no authority either). Hidden reasoning, tool-call argument chunks, visible content,
and SSE keep-alive traffic all preserve byte liveness. The only authoritative exits are:

- caller cancellation or caller deadline;
- no upstream bytes before first usable model progress, bounded by
  `streamFirstByteTimeout`;
- no further upstream SSE bytes after activity, bounded by `streamStallTimeout`;
- precise transport/decode failure.

Compatibility error types for old no-visible-output/total-duration policies remain in
the type surface, but the production OpenAI SSE adapter does not mint them. Therefore a
live stream with continued bytes but no completed answer must continue waiting; it must
not be replaced by an age-based fallback answer.

### B659 — exact relation provider split between finalizer and validator

This is deterministic system behavior, not model fluctuation. The graph case exposes
two consumers of the same source truth:

1. The strict pre-emit validator calls the exact typed relation candidate provider,
   sees all `implements` rows, accepts `implementer -> LoopController`, and correctly
   rejects the reverse direction.
2. Finalizer relationship authority reads only the EvidenceItem enrichment pool. The
   12 relations existed in typed relation projection/role handoff but not as structured
   EvidenceItems, so it emitted `explicit_typed_directed_relations=0` and the retry hint
   stated “No copy-ready typed relation carrier is available.”

That asymmetric contract made the first answer fail and then taught the model to delete
the requested diagram. The fix exports one request-scoped projection of
coverage-gate-eligible candidates into citable type-relation EvidenceItems. Finalizer
and validator now consume the same exact provider and direction. Exact evidence is only
an authoring carrier: it neither requires a diagram nor authors/replaces the diagram or
the model conclusion. Name-only and heuristic candidates remain fail-closed.

The regression matrix covers Go, Java, Kotlin, ArkTS, Cangjie, C++, and Rust paths. The
change is relation-kind/provider based rather than parser-language or `LoopController`
specific. Targeted and core-package tests pass; production replay remains required in a
later exact-two batch.

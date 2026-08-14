# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T15:43:18Z
- sweep_start_ts: 20260814-084317
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_perf_quality_raw_fallback | PASS | eval/results/trace_query_perf_quality_raw_fallback-20260814-084318 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 115s | 30 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Statistical caliber remained correct, but the finalizer misread ftrace task identity `(20)` as CPU 20 even though `[005]`, explicit sample cpu=5, and three deterministic target_cpu_running rows all say CPU 5. It invented a CPU20→CPU5 migration; the prose-triggered fact appendix then emitted an irrelevant CPU20 no-frequency row. B810 adds a typed target-CPU identity boundary at the final decision tail. |
| 2 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260814-084318 | log_attachment,answer_contains | log_triage | 116s | 24 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | B809 reached the explorer and changed its closure to independent peer facts with cross_error_relation unproven. The final answer still added a qualified cross-language propagation inference. With typed contexts now internally consistent and no causal dimension requested, further prose-specific hardening would overfit; retain as a model-behavior watch item while improving general analyzer/finalizer scope teaching only through typed signals. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Cross-case findings

- No finalizer validation retry, malformed JSON recovery, missing answer, or Mermaid repair occurred. The mixed-log explorer did require one completion repair because its first aggregate-facts carrier was malformed; this is JSON/shape-teaching churn, not answer-document churn.
- The Trace failure is deterministic context salience, not missing engine data: target_cpu_running was complete and assigned entirely to CPU 5. A task PID/TGID must never become CPU authority.
- The peer-error answer improved from a definite propagation assertion to an explicitly unproven inference, but it still exceeds the bounded fact request. No final-prose scan or system rewrite will be added.
- No fixed short-age/4ms active-stream degradation was observed.

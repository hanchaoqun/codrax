# Selected Eval Manual Audit Scaffold

- date: 2026-07-01T01:39:32Z
- sweep_start_ts: 20260701-093932
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | arkts_repomap | FAIL | eval/results/arkts_repomap-20260701-093932 | typed_inventory_rowset,answer_contains | none | 97s | 17 | read=0,repo_map=2,list=1,trace=0,source_lens=2 | midloop=1,inv=2/1,fin_reject=0,unavail=0,prune=0 | fail | The answer found the four @Entry corpus rows and two @Builder rows, but reframed the principal answer as production-only zero even though the request was repo-wide. This exposes D1-G209: source-class universe is not load-bearing enough; git-tracked auxiliary/corpus rows should remain principal unless typed exclusion excludes them. |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260701-093932 | typed_inventory_rowset,dimension_substring,answer_contains | none | 161s | 23 | read=11,repo_map=3,list=2,trace=0,source_lens=3 | midloop=6,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass_with_efficiency_debt | Final answer satisfies declared inventory oracles, but the path still used 11 read_file calls and 6 midloop injections. Keep as non-P0 efficiency follow-up after source-class correctness is fixed. |
| 3 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260701-094110 | answer_regex,answer_contains | none | 80s | 23 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Answer and tool path are acceptable for relation/localization coverage. |
| 5 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260701-094231 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 122s | 23 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Good trace-only baseline: trace_query-only path, no source/blob fallback, one completion call. |
| 4 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260701-094214 | trace_attachment,answer_regex | perf_triage+trace_query | 266s | 36 | read=7,repo_map=0,list=0,trace=3,source_lens=0 | midloop=6,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass_with_latency_debt | Mixed runtime/current-source answer is acceptable, but latency and midloop injections remain high; audit after D1-G208 to ensure trace-only suppression does not weaken true mixed-source paths. |
| 6 | trace_query_donghu_real_frame_multicausal | TIMEOUT | eval/results/trace_query_donghu_real_frame_multicausal-20260701-094434 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 1200s | 47 | read=41,repo_map=3,list=0,trace=101,source_lens=0 | midloop=4,inv=17/1,fin_reject=0,unavail=0,prune=4 | fail_timeout | First trace branch had answer-grade trace_query observations and successful completion, but source/blob guidance plus sibling dispatches dragged the run back into read_file/repo_map/trace_query loops. This is D1-G208 and is higher priority than additional eval runs. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

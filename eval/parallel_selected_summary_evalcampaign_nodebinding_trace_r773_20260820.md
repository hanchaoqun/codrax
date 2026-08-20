# Selected parallel eval sweep

- date: 2026-08-20T12:53:43Z
- sweep_start_ts: 20260820-055342
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_type_relation_loop_controller | PASS | - | 256s | 1 | 1 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260820-055344 |
| 2 | trace_query_wakeup_causal_io_chain | FAIL | trace_final_projection_blocks:0_want_1 missing:iowait no_regex_match:threadpool-400.*(主因|primary|root cause|根因|r | 262s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260820-055344 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

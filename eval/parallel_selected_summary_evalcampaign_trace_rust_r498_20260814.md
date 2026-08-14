# Selected parallel eval sweep

- date: 2026-08-14T16:25:29Z
- sweep_start_ts: 20260814-092527
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_rust_cross_module_chain | PASS | - | 121s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260814-092529 |
| 1 | trace_query_wakeup_causal_runnable | FAIL | no_regex_match:(主因|primary|rank|root cause|根因).*(worker-200|worker)|(worker-200|worker).*(主因|primary|rank|ro | 175s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260814-092529 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

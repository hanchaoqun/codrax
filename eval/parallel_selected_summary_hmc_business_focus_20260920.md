# Selected parallel eval sweep

- date: 2026-09-20T14:57:11Z
- sweep_start_ts: 20260920-075709
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_business_focus_20260920

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | - | 187s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_business_focus_20260920/trace_query_wakeup_causal_io_chain-20260920-075711 |
| 1 | trace_query_business_marker_io_chain | FAIL | no_primary_regex_match:(^|[^0-9])35([.]0+)?[[:space:]*]*(ms|毫秒) | 367s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_business_focus_20260920/trace_query_business_marker_io_chain-20260920-075711 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

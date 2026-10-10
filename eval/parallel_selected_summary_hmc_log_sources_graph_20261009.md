# Selected parallel eval sweep

- date: 2026-10-10T01:55:01Z
- sweep_start_ts: 20261009-185458
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_log_sources_graph_20261009

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | log_shared_sources | PASS | - | 152s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage+log_query | eval/results/hmc_log_sources_graph_20261009/log_shared_sources-20261009-185501 |
| 2 | trace_transaction_handoffs | PASS | - | 176s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_log_sources_graph_20261009/trace_transaction_handoffs-20261009-185501 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

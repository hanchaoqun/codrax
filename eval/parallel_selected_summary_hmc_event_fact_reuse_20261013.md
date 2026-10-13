# Selected parallel eval sweep

- date: 2026-10-13T11:38:57Z
- sweep_start_ts: 20261013-043855
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_event_fact_reuse_20261013

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_gzip_sqlite_jank_multiapp_inventory | PASS | - | 82s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_event_fact_reuse_20261013/trace_gzip_sqlite_jank_multiapp_inventory-20261013-043857 |
| 2 | trace_lossless_sampling_tree | PASS | - | 152s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_event_fact_reuse_20261013/trace_lossless_sampling_tree-20261013-043857 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

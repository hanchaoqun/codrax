# Selected parallel eval sweep

- date: 2026-10-08T01:12:58Z
- sweep_start_ts: 20261007-181257
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results/hmc_restart_graph_20261007

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_sleep_dependencies | PASS | - | 150s | 1 | 1 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_restart_graph_20261007/trace_query_sleep_dependencies-20261007-181258 |

**Pass: 1 / 1 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

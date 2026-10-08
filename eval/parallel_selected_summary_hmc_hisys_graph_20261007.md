# Selected parallel eval sweep

- date: 2026-10-08T02:25:26Z
- sweep_start_ts: 20261007-192524
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_hisys_graph_20261007

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_existing_sqlite_hisys_row_identity | FAIL | no_primary_regex_match:9007199254740993 no_primary_regex_match:9223372036854775807 | 121s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_hisys_graph_20261007/trace_existing_sqlite_hisys_row_identity-20261007-192526 |
| 2 | trace_query_sleep_dependencies | PASS | - | 216s | 1 | 2 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_hisys_graph_20261007/trace_query_sleep_dependencies-20261007-192526 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

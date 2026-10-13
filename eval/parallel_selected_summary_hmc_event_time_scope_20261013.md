# Selected parallel eval sweep

- date: 2026-10-13T13:26:14Z
- sweep_start_ts: 20261013-062612
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_event_time_scope_20261013

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_lossless_sampling_tree | PASS | - | 224s | 1 | 0 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_event_time_scope_20261013/trace_lossless_sampling_tree-20261013-062614 |
| 1 | trace_gzip_sqlite_late_jank_reports | PASS | - | 297s | 1 | 2 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_event_time_scope_20261013/trace_gzip_sqlite_late_jank_reports-20261013-062614 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

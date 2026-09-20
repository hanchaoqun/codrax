# Selected parallel eval sweep

- date: 2026-09-20T09:22:00Z
- sweep_start_ts: 20260920-022158
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_io_group_scope_replay_20260920

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_business_marker_io_chain | PASS | - | 269s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_io_group_scope_replay_20260920/trace_query_business_marker_io_chain-20260920-022200 |
| 1 | trace_query_io_request_latency_distribution | PASS | - | 329s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_io_group_scope_replay_20260920/trace_query_io_request_latency_distribution-20260920-022200 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

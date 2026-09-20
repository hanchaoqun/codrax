# Selected parallel eval sweep

- date: 2026-09-20T06:14:03Z
- sweep_start_ts: 20260919-231403
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_parallel_handoff_replay_20260920

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_business_marker_io_chain | FAIL | trace_final_projection_blocks:0_want_1 no_primary_text_regex_match:((backup|备份|(^|[^0-9])900([^0-9]|$)).{0,180}(背� | 332s | 1 | 2 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_parallel_handoff_replay_20260920/trace_query_business_marker_io_chain-20260919-231403 |
| 1 | trace_query_io_request_latency_distribution | PASS | - | 367s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_parallel_handoff_replay_20260920/trace_query_io_request_latency_distribution-20260919-231403 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

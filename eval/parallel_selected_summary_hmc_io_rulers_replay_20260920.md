# Selected parallel eval sweep

- date: 2026-09-20T10:03:59Z
- sweep_start_ts: 20260920-030359
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_io_rulers_replay_20260920

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_business_marker_io_chain | FAIL | trace_final_projection_blocks:0_want_1 no_primary_regex_match:(^|[^0-9])35([.]0+)?[[:space:]*]*(ms|毫秒) no_primary_te | 328s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_io_rulers_replay_20260920/trace_query_business_marker_io_chain-20260920-030359 |
| 2 | trace_query_io_request_latency_distribution | FAIL | missing_primary:P90 missing_primary:P95 | 671s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_io_rulers_replay_20260920/trace_query_io_request_latency_distribution-20260920-030359 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**

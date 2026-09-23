# Selected parallel eval sweep

- date: 2026-09-23T16:11:08Z
- sweep_start_ts: 20260923-091107
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_scheduler_concurrency | FAIL | no_primary_regex_match:(未闭合|未完成|缺.{0,10}(终点|闭合|调度)|没有.{0,10}(终点|闭合|调度)) | 468s | 1 | 1 | 0 | 1 | 0 | 2 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_scheduler_concurrency-20260923-091108 |
| 2 | trace_query_io_inflight | FAIL | no_primary_regex_match:(^|[^0-9])1[.]40*([^0-9]|$) no_primary_regex_match:(^|[^0-9])0[.]60*([^0-9]|$) no_primary_regex_m | 929s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_io_inflight-20260923-091108 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**

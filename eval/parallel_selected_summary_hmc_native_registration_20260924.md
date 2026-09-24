# Selected parallel eval sweep

- date: 2026-09-24T08:08:45Z
- sweep_start_ts: 20260924-010842
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | empty_python_module_apply | PASS | - | 234s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/empty_python_module_apply-20260924-010845 |
| 2 | trace_query_io_activity | FAIL | no_primary_regex_match:(^|[^0-9])28([.]0+)?([^0-9]|$) no_primary_regex_match:(37,?888|148([.]0+)?|151,?552) | 461s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_io_activity-20260924-010845 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

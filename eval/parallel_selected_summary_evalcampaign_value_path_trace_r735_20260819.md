# Selected parallel eval sweep

- date: 2026-08-19T16:03:33Z
- sweep_start_ts: 20260819-090331
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h3_iofam_one_seat | FAIL | missing:41.329 no_regex_match:41\.329.*(非墙钟|不可相加|non.wall.clock|non.additive) | 380s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h3_iofam_one_seat-20260819-090333 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 636s | 1 | 2 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260819-090333 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

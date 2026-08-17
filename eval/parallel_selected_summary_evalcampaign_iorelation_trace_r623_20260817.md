# Selected parallel eval sweep

- date: 2026-08-17T14:43:29Z
- sweep_start_ts: 20260817-074328
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h3_iofam_one_seat | FAIL | no_regex_match:1\.347.*(墙钟|墙上|wall.clock) no_regex_match:1\.337.*(墙钟|墙上|wall.clock) | 164s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h3_iofam_one_seat-20260817-074329 |
| 2 | trace_query_wakeup_causal_runnable | PASS | - | 194s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260817-074329 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

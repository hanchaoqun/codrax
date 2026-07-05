# Selected parallel eval sweep

- date: 2026-07-05T13:20:06Z
- sweep_start_ts: 20260705-212006
- total cases: 1
- parallel: 1
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | real_trace_e1_dual_window_normalized | FAIL | no_text_regex_match:(窗口 ?A|短窗|2\.992|第一个窗口).{0,200}((^|[^0-9.])0(\.0+)? *(ms|毫秒)|没有获得|未� | 159s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_e1_dual_window_normalized-20260705-212006 |

**Pass: 0 / 1 — Fail/Timeout/LaunchFail: 1**

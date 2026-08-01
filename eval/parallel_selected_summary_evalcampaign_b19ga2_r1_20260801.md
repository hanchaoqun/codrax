# Selected parallel eval sweep

- date: 2026-08-01T08:54:26Z
- sweep_start_ts: 20260801-015425
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_c2_dstate_iowait | FAIL | no_regex_match:(^|[^0-9])(3|三) ?[次条].*(iowait|io_?wait|IO|blocked_reason|D ?状态|D-state)|(iowait|io_?wait|IO|bl | 141s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260801-015426 |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 216s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260801-015426 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

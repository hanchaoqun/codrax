# Selected parallel eval sweep

- date: 2026-08-01T05:36:39Z
- sweep_start_ts: 20260731-223637
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 153s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260731-223639 |
| 2 | read_combo_git_two_diffs_current_code | PASS | - | 193s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_git_two_diffs_current_code-20260731-223639 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

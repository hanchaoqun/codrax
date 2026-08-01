# Selected parallel eval sweep

- date: 2026-08-01T05:25:35Z
- sweep_start_ts: 20260731-222533
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | LAUNCH_FAIL | no fresh result dir produced | 0s | - | - | - | - | - | 0 | 0 | 0 | 0 | 0 | - |  |
| 2 | read_combo_git_two_diffs_current_code | FAIL | banned:系统按已验证证据补充 | 120s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_git_two_diffs_current_code-20260731-222535 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**

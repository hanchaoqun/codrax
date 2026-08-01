# Selected parallel eval sweep

- date: 2026-08-01T14:51:13Z
- sweep_start_ts: 20260801-075112
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_git_diff_hunk_current_code | PASS | - | 177s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_git_diff_hunk_current_code-20260801-075113 |
| 1 | read_combo_git_current_source_explanation | PASS | - | 212s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_git_current_source_explanation-20260801-075113 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

# Selected parallel eval sweep

- date: 2026-08-01T13:29:43Z
- sweep_start_ts: 20260801-062941
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_git_current_source_explanation | FAIL | no_regex_match:(commit|提交|diff|合入).*(当前|源码|实现|internal/|\.go:)|(当前|源码|实现|internal/|\.go: | 177s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_git_current_source_explanation-20260801-062943 |
| 2 | read_combo_git_diff_hunk_current_code | PASS | - | 191s | 1 | 2 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | none | eval/results/read_combo_git_diff_hunk_current_code-20260801-062943 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

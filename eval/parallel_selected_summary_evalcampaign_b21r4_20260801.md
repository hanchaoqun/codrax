# Selected parallel eval sweep

- date: 2026-08-01T13:58:01Z
- sweep_start_ts: 20260801-065800
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_git_diff_hunk_current_code | FAIL | no_regex_match:(作用|影响|链路|依据|边界).*(diff|hunk|当前源码|源码)|(diff|hunk|当前源码|源码).*(� | 159s | 1 | 1 | 0 | 1 | 0 | 2 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_git_diff_hunk_current_code-20260801-065801 |
| 1 | read_combo_git_current_source_explanation | FAIL | no_regex_match:(commit|提交|diff|合入).*(当前|源码|实现|internal/|\.go:)|(当前|源码|实现|internal/|\.go: | 200s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_git_current_source_explanation-20260801-065801 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**

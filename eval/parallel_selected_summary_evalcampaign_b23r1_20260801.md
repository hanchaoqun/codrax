# Selected parallel eval sweep

- date: 2026-08-01T17:41:21Z
- sweep_start_ts: 20260801-104119
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | patch_python_typo | PASS | - | 64s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_python_typo-20260801-104121 |
| 2 | read_combo_git_current_source_explanation | FAIL | no_regex_match:(commit|提交|diff|合入).*(当前|源码|实现|internal/|\.go:)|(当前|源码|实现|internal/|\.go: | 127s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_git_current_source_explanation-20260801-104121 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

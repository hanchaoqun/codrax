# Selected parallel eval sweep

- date: 2026-08-28T16:05:45Z
- sweep_start_ts: 20260828-090545
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_c_typo | PASS | - | 86s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_c_typo-20260828-090545 |
| 1 | log_path_question_multi_runtime_files | PASS | - | 104s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | none | eval/results/log_path_question_multi_runtime_files-20260828-090545 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

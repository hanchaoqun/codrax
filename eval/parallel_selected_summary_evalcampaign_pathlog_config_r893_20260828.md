# Selected parallel eval sweep

- date: 2026-08-28T16:34:16Z
- sweep_start_ts: 20260828-093416
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | log_path_question_multi_runtime_files | PASS | - | 157s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/log_path_question_multi_runtime_files-20260828-093416 |
| 2 | qf_config_precedence | PASS | - | 192s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_config_precedence-20260828-093416 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

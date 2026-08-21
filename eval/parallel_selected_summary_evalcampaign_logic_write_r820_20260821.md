# Selected parallel eval sweep

- date: 2026-08-21T17:40:47Z
- sweep_start_ts: 20260821-104046
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_python_typo | PASS | - | 71s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_python_typo-20260821-104047 |
| 1 | qf_logic_view_read_pipeline | FAIL | degraded_answer_checks_skipped:1 | 871s | 1 | 2 | 0 | 1 | 0 | 20 | 19 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260821-104047 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

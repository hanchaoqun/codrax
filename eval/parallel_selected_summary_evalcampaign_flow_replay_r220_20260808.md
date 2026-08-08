# Selected parallel eval sweep

- date: 2026-08-08T17:25:21Z
- sweep_start_ts: 20260808-102518
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_diagram_pipeline | PASS | - | 483s | 1 | 1 | 0 | 1 | 0 | 6 | 5 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260808-102521 |
| 1 | qf_logic_view_read_pipeline | FAIL | degraded_answer_checks_skipped:1 missing:finalizer | 665s | 1 | 1 | 0 | 1 | 0 | 9 | 8 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260808-102521 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

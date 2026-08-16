# Selected parallel eval sweep

- date: 2026-08-16T03:45:15Z
- sweep_start_ts: 20260815-204513
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_loose_multi_question_units | FAIL | degraded_answer_checks_skipped:1 | 155s | 1 | 1 | 0 | 1 | 0 | 6 | 5 | 0 | 0 | 0 | none | eval/results/read_combo_loose_multi_question_units-20260815-204515 |
| 1 | mr_poly_binding_chain | PASS | - | 168s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260815-204515 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

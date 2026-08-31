# Selected parallel eval sweep

- date: 2026-08-31T01:34:57Z
- sweep_start_ts: 20260830-183455
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_json_strict_ids | PASS | - | 335s | 0 | 0 | 0 | 0 | 3 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260830-183457 |
| 1 | qf_sequence_analyzer_gate | FAIL | degraded_answer_checks_skipped:1 | 495s | 1 | 2 | 0 | 1 | 0 | 8 | 7 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260830-183457 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

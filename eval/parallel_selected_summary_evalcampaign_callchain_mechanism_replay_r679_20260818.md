# Selected parallel eval sweep

- date: 2026-08-18T10:18:13Z
- sweep_start_ts: 20260818-031812
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | read_combo_loose_multi_question_units | PASS | - | 197s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_loose_multi_question_units-20260818-031813 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 253s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260818-031813 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

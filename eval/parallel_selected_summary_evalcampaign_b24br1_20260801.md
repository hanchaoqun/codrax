# Selected parallel eval sweep

- date: 2026-08-01T18:46:50Z
- sweep_start_ts: 20260801-114648
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | patch_c_typo | PASS | - | 119s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_c_typo-20260801-114650 |
| 2 | qf_sequence_analyzer_gate | PASS | - | 295s | 1 | 1 | 0 | 1 | 0 | 6 | 3 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260801-114650 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

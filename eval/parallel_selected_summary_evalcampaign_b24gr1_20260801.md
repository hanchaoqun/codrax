# Selected parallel eval sweep

- date: 2026-08-01T20:25:15Z
- sweep_start_ts: 20260801-132514
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | patch_c_typo | PASS | - | 129s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_c_typo-20260801-132515 |
| 2 | qf_sequence_analyzer_gate | PASS | - | 293s | 1 | 1 | 0 | 1 | 0 | 8 | 4 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260801-132515 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

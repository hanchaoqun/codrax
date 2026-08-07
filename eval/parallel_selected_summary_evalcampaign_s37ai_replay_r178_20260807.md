# Selected parallel eval sweep

- date: 2026-08-07T17:49:08Z
- sweep_start_ts: 20260807-104907
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_python_typo | PASS | - | 102s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_python_typo-20260807-104908 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 430s | 1 | 1 | 0 | 2 | 0 | 2 | 1 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260807-104908 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

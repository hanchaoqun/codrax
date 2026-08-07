# Selected parallel eval sweep

- date: 2026-08-07T17:34:02Z
- sweep_start_ts: 20260807-103401
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_python_typo | PASS | - | 160s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_python_typo-20260807-103403 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 234s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260807-103403 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

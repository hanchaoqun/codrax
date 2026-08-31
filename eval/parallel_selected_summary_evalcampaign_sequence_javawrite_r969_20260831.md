# Selected parallel eval sweep

- date: 2026-08-31T11:36:53Z
- sweep_start_ts: 20260831-043652
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_java_typo | PASS | - | 56s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_java_typo-20260831-043653 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 357s | 1 | 1 | 0 | 1 | 0 | 3 | 4 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260831-043653 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

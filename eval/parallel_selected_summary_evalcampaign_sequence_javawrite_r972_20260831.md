# Selected parallel eval sweep

- date: 2026-08-31T13:03:45Z
- sweep_start_ts: 20260831-060344
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_java_typo | PASS | - | 49s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_java_typo-20260831-060345 |
| 1 | qf_sequence_analyzer_gate | PASS | - | 457s | 1 | 1 | 0 | 1 | 0 | 10 | 10 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260831-060345 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

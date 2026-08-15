# Selected parallel eval sweep

- date: 2026-08-15T18:30:01Z
- sweep_start_ts: 20260815-113000
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_python_typo | PASS | - | 59s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_python_typo-20260815-113001 |
| 1 | qf_relation_subagent_registry | PASS | - | 100s | 1 | 1 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | none | eval/results/qf_relation_subagent_registry-20260815-113001 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

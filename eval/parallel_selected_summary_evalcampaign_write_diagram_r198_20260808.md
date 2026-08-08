# Selected parallel eval sweep

- date: 2026-08-08T08:00:25Z
- sweep_start_ts: 20260808-010024
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | patch_java_typo | PASS | - | 86s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_java_typo-20260808-010026 |
| 2 | qf_diagram_pipeline | PASS | - | 579s | 1 | 2 | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260808-010026 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

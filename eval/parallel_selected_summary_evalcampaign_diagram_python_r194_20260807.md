# Selected parallel eval sweep

- date: 2026-08-08T06:51:33Z
- sweep_start_ts: 20260807-235132
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_python_typo | PASS | - | 92s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_python_typo-20260807-235133 |
| 1 | qf_diagram_pipeline | PASS | - | 130s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260807-235133 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

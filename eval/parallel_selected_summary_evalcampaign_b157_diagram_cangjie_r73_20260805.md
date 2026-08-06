# Selected parallel eval sweep

- date: 2026-08-06T02:59:47Z
- sweep_start_ts: 20260805-195945
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | qf_diagram_pipeline | PASS | - | 107s | 1 | 1 | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260805-195947 |
| 2 | cangjie_repomap | PASS | - | 142s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260805-195947 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

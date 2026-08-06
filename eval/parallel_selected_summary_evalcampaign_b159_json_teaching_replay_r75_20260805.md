# Selected parallel eval sweep

- date: 2026-08-06T03:37:37Z
- sweep_start_ts: 20260805-203735
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | operation_system_inventory | PASS | - | 37s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_system_inventory-20260805-203737 |
| 1 | qf_diagram_pipeline | PASS | - | 94s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_diagram_pipeline-20260805-203737 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

# Selected parallel eval sweep

- date: 2026-08-05T11:36:35Z
- sweep_start_ts: 20260805-043633
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | operation_web_manual_summary | PASS | - | 135s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_web_manual_summary-20260805-043635 |
| 2 | mr_poly_binding_chain | PASS | - | 230s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260805-043635 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

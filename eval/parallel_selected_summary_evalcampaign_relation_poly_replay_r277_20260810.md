# Selected parallel eval sweep

- date: 2026-08-10T21:48:56Z
- sweep_start_ts: 20260810-144855
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | mr_poly_binding_chain | PASS | - | 150s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260810-144856 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 703s | 1 | 2 | 0 | 1 | 0 | 6 | 6 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-144856 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

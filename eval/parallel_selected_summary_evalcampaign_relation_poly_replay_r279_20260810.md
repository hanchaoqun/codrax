# Selected parallel eval sweep

- date: 2026-08-10T22:38:06Z
- sweep_start_ts: 20260810-153804
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | mr_poly_binding_chain | PASS | - | 121s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260810-153806 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 712s | 1 | 1 | 0 | 1 | 0 | 12 | 12 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-153806 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

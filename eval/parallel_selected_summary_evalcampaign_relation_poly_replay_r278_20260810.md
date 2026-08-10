# Selected parallel eval sweep

- date: 2026-08-10T22:12:10Z
- sweep_start_ts: 20260810-151209
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | mr_poly_binding_chain | PASS | - | 243s | 2 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260810-151210 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 342s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-151210 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

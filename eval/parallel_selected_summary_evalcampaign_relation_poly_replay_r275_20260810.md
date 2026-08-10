# Selected parallel eval sweep

- date: 2026-08-10T20:02:21Z
- sweep_start_ts: 20260810-130220
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | mr_poly_binding_chain | PASS | - | 201s | 1 | 1 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260810-130222 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 487s | 1 | 2 | 0 | 1 | 0 | 2 | 2 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-130222 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

# Selected parallel eval sweep

- date: 2026-08-11T00:09:14Z
- sweep_start_ts: 20260810-170913
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | mr_poly_binding_chain | PASS | - | 112s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260810-170914 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 524s | 1 | 5 | 0 | 2 | 1 | 5 | 6 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-170914 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

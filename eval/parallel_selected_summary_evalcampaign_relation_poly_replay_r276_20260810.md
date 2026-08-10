# Selected parallel eval sweep

- date: 2026-08-10T21:00:39Z
- sweep_start_ts: 20260810-140038
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | mr_poly_binding_chain | PASS | - | 187s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260810-140039 |
| 1 | qf_logic_view_read_pipeline | FAIL | mermaid_edges:0<1 | 864s | 1 | 5 | 0 | 2 | 1 | 9 | 10 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-140039 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

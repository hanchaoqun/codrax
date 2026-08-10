# Selected parallel eval sweep

- date: 2026-08-10T19:26:47Z
- sweep_start_ts: 20260810-122646
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | mr_poly_binding_chain | PASS | - | 213s | 1 | 1 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260810-122647 |
| 1 | qf_logic_view_read_pipeline | FAIL | degraded_answer_checks_skipped:1 mermaid_edges:0<1 | 776s | 1 | 1 | 0 | 2 | 1 | 13 | 13 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260810-122647 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

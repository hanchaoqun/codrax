# Selected parallel eval sweep

- date: 2026-08-05T12:29:56Z
- sweep_start_ts: 20260805-052955
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | mr_poly_binding_chain | PASS | - | 191s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260805-052956 |
| 2 | sr_java_call_chain | PASS | - | 411s | 1 | 1 | 0 | 1 | 0 | 7 | 7 | 0 | 0 | 0 | none | eval/results/sr_java_call_chain-20260805-052956 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

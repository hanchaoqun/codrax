# Selected parallel eval sweep

- date: 2026-08-05T11:54:53Z
- sweep_start_ts: 20260805-045451
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_ts_workspace_chain | PASS | - | 141s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_chain-20260805-045453 |
| 1 | mr_poly_binding_chain | FAIL | degraded_answer_checks_skipped:1 | 481s | 1 | 1 | 0 | 1 | 0 | 6 | 5 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260805-045453 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

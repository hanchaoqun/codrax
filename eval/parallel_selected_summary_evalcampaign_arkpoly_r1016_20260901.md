# Selected parallel eval sweep

- date: 2026-09-02T02:47:10Z
- sweep_start_ts: 20260901-194708
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | arkts_repomap | PASS | - | 162s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260901-194710 |
| 2 | mr_poly_binding_chain | PASS | - | 485s | 1 | 2 | 0 | 1 | 0 | 13 | 14 | 0 | 0 | 0 | none | eval/results/mr_poly_binding_chain-20260901-194710 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

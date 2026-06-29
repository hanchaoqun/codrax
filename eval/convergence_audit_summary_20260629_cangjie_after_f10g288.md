# Selected parallel eval sweep

- date: 2026-06-29T09:13:56Z
- sweep_start_ts: 20260629-171356
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | cangjie_repomap | FAIL | missing_dimension:package:demo.greeter missing_inventory_row:public_class:App_main.cj_demo.app missing_inventory_row:pub | 257s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260629-171356 |

**Pass: 0 / 1 — Fail/Timeout/LaunchFail: 1**

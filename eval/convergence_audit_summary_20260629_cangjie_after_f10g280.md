# Selected parallel eval sweep

- date: 2026-06-29T08:21:35Z
- sweep_start_ts: 20260629-162135
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | cangjie_repomap | FAIL | missing_dimension:package:demo.greeter missing_inventory_row:public_class:Greeter_02_class_init_methods.cj_demo.greeter  | 266s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260629-162135 |

**Pass: 0 / 1 — Fail/Timeout/LaunchFail: 1**

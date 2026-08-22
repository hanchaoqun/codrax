# Selected parallel eval sweep

- date: 2026-08-22T05:26:27Z
- sweep_start_ts: 20260821-222626
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | patch_c_typo | PASS | - | 89s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_c_typo-20260821-222627 |
| 2 | sr_py_registry_dispatch | PASS | - | 285s | 1 | 2 | 0 | 1 | 0 | 6 | 6 | 0 | 0 | 0 | none | eval/results/sr_py_registry_dispatch-20260821-222627 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

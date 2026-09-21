# Selected parallel eval sweep

- date: 2026-09-21T06:35:33Z
- sweep_start_ts: 20260920-233533
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results/hmc_scope_xerr_read_write_20260920

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_java_typo | PASS | - | 99s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/hmc_scope_xerr_read_write_20260920/patch_java_typo-20260920-233533 |
| 1 | trace_query_jank_field_inventory | PASS | - | 157s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/hmc_scope_xerr_read_write_20260920/trace_query_jank_field_inventory-20260920-233533 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

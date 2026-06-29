# Selected parallel eval sweep

- date: 2026-06-29T13:32:07Z
- sweep_start_ts: 20260629-213207
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | cangjie_repomap | FAIL | missing_inventory_row:extend:extend_Cart_Cart.cj_demo.cart inventory_count_mismatch:extend:got1:want2 | 142s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260629-213207 |
| 2 | read_combo_trace_current_source_explanation | PASS | - | 172s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage | eval/results/read_combo_trace_current_source_explanation-20260629-213207 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

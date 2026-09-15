# Selected parallel eval sweep

- date: 2026-09-15T02:13:46Z
- sweep_start_ts: 20260914-191345
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_rust_cross_module_chain | PASS | - | 124s | 1 | 1 | 0 | 1 | 0 | 2 | 3 | 0 | 0 | 0 | none | eval/results/sr_rust_cross_module_chain-20260914-191346 |
| 1 | patch_go_typo | FAIL | no_plan_regex:"kind":[[:space:]]*"patch" | 156s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260914-191346 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

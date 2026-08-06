# Selected parallel eval sweep

- date: 2026-08-06T07:43:40Z
- sweep_start_ts: 20260806-004338
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_dayjs_duration_nan | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 159s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_dayjs_duration_nan-20260806-004340 |
| 2 | cangjie_repomap | FAIL | missing_inventory_row:public_class:App_main.cj_demo.app missing_inventory_row:public_class:Animal_08_modifiers_combos.cj | 268s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260806-004340 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**

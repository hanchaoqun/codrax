# Selected parallel eval sweep

- date: 2026-09-09T09:06:17Z
- sweep_start_ts: 20260909-020617
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_go_typo | FAIL | write_final_verdict:unverified:verification_proof_incomplete no_plan_regex:"kind":[[:space:]]*"patch" | 198s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260909-020617 |
| 1 | sr_ts_workspace_chain | PASS | - | 246s | 1 | 2 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_chain-20260909-020617 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

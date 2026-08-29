# Selected parallel eval sweep

- date: 2026-08-29T08:30:58Z
- sweep_start_ts: 20260829-013057
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_ts_workspace_chain | PASS | - | 194s | 1 | 1 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_chain-20260829-013058 |
| 2 | patch_go_typo | FAIL | write_final_run_status:blocked write_final_verdict:missing:missing | 532s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260829-013058 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

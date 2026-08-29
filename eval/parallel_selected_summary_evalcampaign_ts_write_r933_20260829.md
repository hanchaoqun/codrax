# Selected parallel eval sweep

- date: 2026-08-29T09:12:59Z
- sweep_start_ts: 20260829-021259
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_go_typo | PASS | - | 97s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260829-021259 |
| 1 | sr_ts_workspace_chain | PASS | - | 257s | 1 | 2 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_chain-20260829-021259 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

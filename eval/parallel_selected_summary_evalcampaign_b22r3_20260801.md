# Selected parallel eval sweep

- date: 2026-08-01T15:56:40Z
- sweep_start_ts: 20260801-085639
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_config_precedence | PASS | - | 123s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_config_precedence-20260801-085640 |
| 1 | patch_go_typo | PASS | - | 170s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260801-085640 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

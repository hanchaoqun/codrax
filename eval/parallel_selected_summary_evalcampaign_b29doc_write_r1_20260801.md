# Selected parallel eval sweep

- date: 2026-08-02T00:14:40Z
- sweep_start_ts: 20260801-171439
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_go_typo | PASS | - | 102s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260801-171440 |
| 1 | read_combo_analyze_retry_anchor | PASS | - | 153s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_analyze_retry_anchor-20260801-171440 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

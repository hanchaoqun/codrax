# Selected parallel eval sweep

- date: 2026-08-02T04:13:39Z
- sweep_start_ts: 20260801-211337
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | logtri_goroutine_dump | PASS | - | 73s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/logtri_goroutine_dump-20260801-211339 |
| 1 | patch_go_typo | PASS | - | 85s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_go_typo-20260801-211339 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

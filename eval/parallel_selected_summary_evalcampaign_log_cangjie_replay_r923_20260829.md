# Selected parallel eval sweep

- date: 2026-08-29T05:06:47Z
- sweep_start_ts: 20260828-220646
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | logtri_oversized | PASS | - | 128s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/logtri_oversized-20260828-220647 |
| 2 | cangjie_repomap | PASS | - | 321s | 1 | 3 | 0 | 1 | 0 | 1 | 2 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260828-220647 |

**Pass: 2 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 0**

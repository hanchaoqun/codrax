# Selected parallel eval sweep

- date: 2026-07-30T14:38:02Z
- sweep_start_ts: 20260730-073802
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_architecture | PASS | - | 94s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_architecture-20260730-073802 |
| 1 | logtri_go | PASS | - | 129s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/logtri_go-20260730-073802 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

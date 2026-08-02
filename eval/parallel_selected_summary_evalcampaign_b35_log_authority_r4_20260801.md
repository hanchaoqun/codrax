# Selected parallel eval sweep

- date: 2026-08-02T05:04:48Z
- sweep_start_ts: 20260801-220447
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | logtri_java | PASS | - | 91s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/logtri_java-20260801-220448 |
| 1 | logtri_goroutine_dump | PASS | - | 115s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/logtri_goroutine_dump-20260801-220448 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

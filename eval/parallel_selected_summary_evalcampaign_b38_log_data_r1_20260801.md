# Selected parallel eval sweep

- date: 2026-08-02T09:52:18Z
- sweep_start_ts: 20260802-025217
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_jsonl_filter_count | PASS | - | 63s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_jsonl_filter_count-20260802-025218 |
| 1 | logtri_goroutine_dump | PASS | - | 114s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/logtri_goroutine_dump-20260802-025218 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

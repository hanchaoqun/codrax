# Selected parallel eval sweep

- date: 2026-08-02T11:49:10Z
- sweep_start_ts: 20260802-044910
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_text_filter_count | FAIL | data_terminal_missing no_log_regex:route=data no_log_regex:\[.*data\] data task result | 68s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_text_filter_count-20260802-044910 |
| 2 | trace_query_binder_ipc_peer | PASS | - | 113s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_binder_ipc_peer-20260802-044910 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

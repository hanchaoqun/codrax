# Selected parallel eval sweep

- date: 2026-08-17T09:35:06Z
- sweep_start_ts: 20260817-023505
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_multifile_reference_projection | FAIL | read_exit:1 data_terminal_missing no_log_regex:\[cli/data\] data task result.*contributions=[1-9][0-9]*.*reconcile=pass  | 71s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260817-023506 |
| 2 | trace_query_wakeup_causal_runnable | PASS | - | 160s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260817-023506 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

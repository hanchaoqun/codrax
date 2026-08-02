# Selected parallel eval sweep

- date: 2026-08-02T10:36:29Z
- sweep_start_ts: 20260802-033628
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | data_text_filter_count | PASS | - | 41s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_text_filter_count-20260802-033629 |
| 1 | trace_query_binder_ipc_peer | PASS | - | 78s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_binder_ipc_peer-20260802-033629 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

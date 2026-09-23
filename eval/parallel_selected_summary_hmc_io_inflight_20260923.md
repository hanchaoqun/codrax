# Selected parallel eval sweep

- date: 2026-09-23T13:31:41Z
- sweep_start_ts: 20260923-063141
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | empty_python_module_apply | PASS | - | 144s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/empty_python_module_apply-20260923-063141 |
| 1 | trace_query_io_inflight | FAIL | answer_surface_receipt_missing read_exit:2 missing_primary:RQ missing_primary:BIO no_primary_regex_match:(^|[^0-9])1[.]4 | 726s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_io_inflight-20260923-063141 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

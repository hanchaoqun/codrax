# Selected parallel eval sweep

- date: 2026-09-24T04:47:41Z
- sweep_start_ts: 20260923-214741
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | empty_python_module_apply | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 236s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/empty_python_module_apply-20260923-214741 |
| 1 | trace_query_io_inflight | PASS | - | 662s | 1 | 2 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_io_inflight-20260923-214741 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

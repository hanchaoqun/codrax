# Selected parallel eval sweep

- date: 2026-08-03T01:43:01Z
- sweep_start_ts: 20260802-184300
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | operation_web_manual_summary | PASS | - | 93s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/operation_web_manual_summary-20260802-184301 |
| 2 | real_trace_h5_smr_multirow_disposition | FAIL | no_regex_match:等待对象 dma_fence_default_w | 260s | 1 | 1 | 0 | 1 | 0 | 2 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h5_smr_multirow_disposition-20260802-184301 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

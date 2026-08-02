# Selected parallel eval sweep

- date: 2026-08-02T19:19:16Z
- sweep_start_ts: 20260802-121915
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_memoclaw_text_search_multirepo_py | FAIL | write_report_failed write_final_verdict:unverified:verification_incomplete | 178s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260802-121916 |
| 1 | real_trace_h5_smr_multirow_disposition | FAIL | no_regex_match:等待对象 dma_fence_default_w | 287s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h5_smr_multirow_disposition-20260802-121916 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**

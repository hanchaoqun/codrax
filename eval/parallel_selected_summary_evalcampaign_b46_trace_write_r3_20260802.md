# Selected parallel eval sweep

- date: 2026-08-02T19:41:40Z
- sweep_start_ts: 20260802-124138
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_memoclaw_text_search_multirepo_py | PASS | - | 198s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260802-124140 |
| 1 | real_trace_h5_smr_multirow_disposition | FAIL | no_regex_match:等待对象 dma_fence_default_w | 284s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h5_smr_multirow_disposition-20260802-124140 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

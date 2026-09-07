# Selected parallel eval sweep

- date: 2026-09-07T01:32:28Z
- sweep_start_ts: 20260906-183218
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_memoclaw_text_search_multirepo_py | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 245s | 1 | 2 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260906-183228 |
| 1 | real_trace_h11_cross_direction_overlap | PASS | - | 291s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h11_cross_direction_overlap-20260906-183228 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

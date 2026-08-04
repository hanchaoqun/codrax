# Selected parallel eval sweep

- date: 2026-08-04T04:40:48Z
- sweep_start_ts: 20260803-214045
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | trace_query_wakeup_causal_runnable | PASS | - | 151s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260803-214048 |
| 1 | github_issue_memoclaw_text_search_multirepo_ts | FAIL | write_final_verdict:unverified:patch_review_semantic_uncovered:behavior_contract_without_verify_coverage | 235s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_memoclaw_text_search_multirepo_ts-20260803-214048 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

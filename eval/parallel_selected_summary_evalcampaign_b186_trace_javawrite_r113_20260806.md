# Selected parallel eval sweep

- date: 2026-08-06T17:38:50Z
- sweep_start_ts: 20260806-103849
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 202s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260806-103850 |
| 2 | github_issue_commons_lang_random_ascii | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 268s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_commons_lang_random_ascii-20260806-103850 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

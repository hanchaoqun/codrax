# Selected parallel eval sweep

- date: 2026-08-06T03:53:58Z
- sweep_start_ts: 20260805-205356
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 166s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260805-205358 |
| 2 | github_issue_commons_lang_random_ascii_symptom | PASS | - | 305s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_commons_lang_random_ascii_symptom-20260805-205358 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

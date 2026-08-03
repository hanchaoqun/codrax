# Selected parallel eval sweep

- date: 2026-08-03T12:05:42Z
- sweep_start_ts: 20260803-050540
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_fmt_tm_year_overflow_symptom | PASS | - | 310s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260803-050542 |
| 1 | trace_query_wakeup_causal_runnable | FAIL | missing:worker-200 missing:app-100 missing:runnable no_regex_match:(worker-200|worker).*runnable|runnable.*(worker-200|w | 539s | 1 | 1 | 0 | 1 | 0 | 40 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260803-050542 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

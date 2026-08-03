# Selected parallel eval sweep

- date: 2026-08-03T12:26:14Z
- sweep_start_ts: 20260803-052613
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_wakeup_causal_runnable | PASS | - | 167s | 1 | 1 | 0 | 1 | 0 | 2 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260803-052614 |
| 2 | github_issue_fmt_tm_year_overflow_symptom | PASS | - | 203s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_fmt_tm_year_overflow_symptom-20260803-052614 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

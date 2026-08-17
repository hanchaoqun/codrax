# Selected parallel eval sweep

- date: 2026-08-17T19:48:57Z
- sweep_start_ts: 20260817-124855
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_libgit2_foreach_worktree | FAIL | no_regex_match:(return[[:space:]]+lookup_result[[:space:]]*;|[(]error[[:space:]]*=[[:space:]]*lookup_result[)][[:space:] | 111s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_libgit2_foreach_worktree-20260817-124857 |
| 1 | trace_query_wakeup_causal_runnable | PASS | - | 196s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_runnable-20260817-124857 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

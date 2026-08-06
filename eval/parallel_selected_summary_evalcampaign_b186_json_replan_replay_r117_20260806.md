# Selected parallel eval sweep

- date: 2026-08-06T19:03:44Z
- sweep_start_ts: 20260806-120342
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | - | 162s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260806-120344 |
| 2 | github_issue_libgit2_foreach_worktree | FAIL | no_regex_match:(error[[:space:]]*=[[:space:]]*cb_result[[:space:]]*;|[(]error[[:space:]]*=[[:space:]]*cb_result[)][[:spa | 249s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_libgit2_foreach_worktree-20260806-120344 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

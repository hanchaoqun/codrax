# Selected parallel eval sweep

- date: 2026-09-14T01:32:37Z
- sweep_start_ts: 20260913-183237
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_h11_cross_direction_overlap | FAIL | no_regex_match:12\.658ms.*IO阻塞.*共47段 | 164s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h11_cross_direction_overlap-20260913-183237 |
| 2 | github_issue_libgit2_foreach_worktree_symptom | PASS | - | 222s | 1 | 2 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_libgit2_foreach_worktree_symptom-20260913-183237 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

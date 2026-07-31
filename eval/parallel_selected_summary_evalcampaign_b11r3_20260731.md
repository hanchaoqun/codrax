# Selected parallel eval sweep

- date: 2026-07-31T21:30:25Z
- sweep_start_ts: 20260731-143024
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | real_trace_h1_binder_true_false_attribution | PASS | - | 228s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h1_binder_true_false_attribution-20260731-143025 |
| 1 | github_issue_libgit2_foreach_worktree_symptom | PASS | - | 327s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_libgit2_foreach_worktree_symptom-20260731-143025 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

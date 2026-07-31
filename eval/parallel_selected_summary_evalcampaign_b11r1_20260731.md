# Selected parallel eval sweep

- date: 2026-07-31T19:55:56Z
- sweep_start_ts: 20260731-125555
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_libgit2_foreach_worktree_symptom | FAIL | write_report_failed | 217s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_libgit2_foreach_worktree_symptom-20260731-125556 |
| 2 | real_trace_h1_binder_true_false_attribution | PASS | - | 242s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h1_binder_true_false_attribution-20260731-125556 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

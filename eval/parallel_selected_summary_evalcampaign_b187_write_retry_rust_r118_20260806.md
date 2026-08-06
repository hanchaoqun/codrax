# Selected parallel eval sweep

- date: 2026-08-06T19:17:23Z
- sweep_start_ts: 20260806-121722
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | sr_rust_trait_impls | PASS | - | 86s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/sr_rust_trait_impls-20260806-121724 |
| 1 | github_issue_libgit2_foreach_worktree | PASS | - | 126s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_libgit2_foreach_worktree-20260806-121724 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

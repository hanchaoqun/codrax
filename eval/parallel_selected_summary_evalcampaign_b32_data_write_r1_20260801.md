# Selected parallel eval sweep

- date: 2026-08-02T01:30:52Z
- sweep_start_ts: 20260801-183050
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | data_json_strict_ids | PASS | - | 44s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260801-183052 |
| 2 | github_issue_libgit2_foreach_worktree | PASS | - | 163s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_libgit2_foreach_worktree-20260801-183052 |

**Pass: 2 / 2 — Fail/Timeout/LaunchFail: 0**

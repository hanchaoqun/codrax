# Selected parallel eval sweep

- date: 2026-09-16T10:01:34Z
- sweep_start_ts: 20260916-030134
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_libgit2_foreach_worktree_symptom | FAIL | plan_not_written apply_not_run no_regex_match:(error[[:space:]]*=[[:space:]]*visit_worktree[(]callback_status[)][)][[:sp | 232s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_libgit2_foreach_worktree_symptom-20260916-030134 |
| 1 | cangjie_repomap | PASS | - | 255s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260916-030134 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

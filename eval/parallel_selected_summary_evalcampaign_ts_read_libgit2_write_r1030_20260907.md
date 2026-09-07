# Selected parallel eval sweep

- date: 2026-09-07T08:24:23Z
- sweep_start_ts: 20260907-012419
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | sr_ts_workspace_chain | PASS | - | 263s | 1 | 2 | 0 | 1 | 0 | 3 | 3 | 0 | 0 | 0 | none | eval/results/sr_ts_workspace_chain-20260907-012423 |
| 2 | github_issue_libgit2_foreach_worktree_symptom | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 331s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_libgit2_foreach_worktree_symptom-20260907-012423 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

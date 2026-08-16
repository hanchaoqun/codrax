# Selected parallel eval sweep

- date: 2026-08-16T11:49:28Z
- sweep_start_ts: 20260816-044927
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_libgit2_foreach_worktree | FAIL | write_final_verdict:unverified:unverified no_regex_match:(return[[:space:]]+lookup_result[[:space:]]*;|[(]error[[:space: | 369s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_libgit2_foreach_worktree-20260816-044928 |
| 1 | read_combo_answer_document_tools | PASS | - | 400s | 1 | 1 | 0 | 1 | 0 | 4 | 4 | 0 | 0 | 0 | none | eval/results/read_combo_answer_document_tools-20260816-044928 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

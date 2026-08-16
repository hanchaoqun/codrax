# Selected parallel eval sweep

- date: 2026-08-16T12:38:15Z
- sweep_start_ts: 20260816-053814
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_libgit2_foreach_worktree | FAIL | no_regex_match:(error[[:space:]]*=[[:space:]]*cb_result[[:space:]]*;|[(]error[[:space:]]*=[[:space:]]*cb_result[)][[:spa | 160s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_libgit2_foreach_worktree-20260816-053815 |
| 1 | read_combo_answer_document_tools | FAIL | degraded_answer_checks_skipped:1 | 1225s | 1 | 3 | 0 | 2 | 1 | 25 | 24 | 0 | 0 | 0 | none | eval/results/read_combo_answer_document_tools-20260816-053815 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**

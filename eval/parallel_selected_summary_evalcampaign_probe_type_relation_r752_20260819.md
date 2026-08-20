# Selected parallel eval sweep

- date: 2026-08-20T01:54:34Z
- sweep_start_ts: 20260819-185433
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | qf_type_relation_loop_controller | PASS | - | 316s | 1 | 1 | 0 | 1 | 0 | 4 | 5 | 0 | 0 | 0 | none | eval/results/qf_type_relation_loop_controller-20260819-185434 |
| 1 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 377s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-185434 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

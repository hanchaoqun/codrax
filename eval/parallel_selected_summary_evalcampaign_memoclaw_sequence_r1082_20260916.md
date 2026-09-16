# Selected parallel eval sweep

- date: 2026-09-16T07:36:48Z
- sweep_start_ts: 20260916-003647
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_memoclaw_text_search_multirepo_py | FAIL | write_final_verdict:unverified:verification_proof_incomplete | 281s | 1 | 2 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_memoclaw_text_search_multirepo_py-20260916-003649 |
| 2 | qf_sequence_analyzer_gate | PASS | - | 468s | 1 | 1 | 0 | 1 | 0 | 10 | 9 | 0 | 0 | 0 | none | eval/results/qf_sequence_analyzer_gate-20260916-003649 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

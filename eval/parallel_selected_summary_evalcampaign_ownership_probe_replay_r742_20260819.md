# Selected parallel eval sweep

- date: 2026-08-19T20:51:40Z
- sweep_start_ts: 20260819-135139
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | write_final_verdict:unverified:behavior_contract_observation_missing | 668s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-135140 |
| 1 | qf_logic_view_read_pipeline | PASS | - | 1194s | 1 | 1 | 0 | 2 | 1 | 6 | 7 | 0 | 0 | 0 | none | eval/results/qf_logic_view_read_pipeline-20260819-135140 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

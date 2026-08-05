# Selected parallel eval sweep

- date: 2026-08-05T12:50:29Z
- sweep_start_ts: 20260805-055027
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | github_issue_memoclaw_text_search_multirepo_ts | FAIL | write_final_verdict:unverified:proof_weak | 205s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_memoclaw_text_search_multirepo_ts-20260805-055029 |
| 2 | data_multifile_reference_projection | FAIL | read_exit:1 data_terminal_status:failed no_log_regex:route=data no_regex_match:^[[:space:]]*17[[:space:]]*,[[:space:]]*0 | 411s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260805-055029 |

**Pass: 0 / 2 — Fail/Timeout/LaunchFail: 2**

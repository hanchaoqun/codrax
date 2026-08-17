# Selected parallel eval sweep

- date: 2026-08-17T22:46:25Z
- sweep_start_ts: 20260817-154624
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_zod_prefault | FAIL | write_final_verdict:unverified:production_verification_source_static_only | 129s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault-20260817-154625 |
| 1 | read_combo_config_two_knobs_precedence | FAIL | no_regex_match:(代码默认值[^0-9]{0,24}3([^0-9]|$)|code default[^0-9]{0,24}3([^0-9]|$)|MaxRetriesPerStage.{0,48}(初 | 204s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/read_combo_config_two_knobs_precedence-20260817-154625 |

**Pass: 0 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 2**

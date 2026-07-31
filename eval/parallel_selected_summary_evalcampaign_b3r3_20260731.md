# Selected parallel eval sweep

- date: 2026-07-31T10:06:02Z
- sweep_start_ts: 20260731-030602
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_log_current_code_boundary | FAIL | no_regex_match:internal/(orchestrator|agent|llm|render)/[^[:space:]]+\.go:[0-9]+ | 87s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_code_boundary-20260731-030602 |
| 2 | logtri_oversized | PASS | - | 296s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/logtri_oversized-20260731-030602 |

**Pass: 1 / 2 — Fail/Timeout/LaunchFail: 1**

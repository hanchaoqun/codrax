# Selected parallel eval sweep

- date: 2026-07-30T15:01:52Z
- sweep_start_ts: 20260730-080152
- total cases: 1
- parallel: 1
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | read_combo_log_current_code_boundary | FAIL | no_regex_match:internal/(orchestrator|agent|llm|render)/[^[:space:]]+\.go:[0-9]+ | 103s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_code_boundary-20260730-080152 |

**Pass: 0 / 1 — Fail/Timeout/LaunchFail: 1**

# Selected parallel eval sweep

- date: 2026-09-15T08:10:14Z
- sweep_start_ts: 20260915-011013
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | patch_cpp_typo | PASS | - | 63s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/patch_cpp_typo-20260915-011014 |
| 1 | read_combo_trace_current_code_dimensions | FAIL | no_regex_match:internal/(analysis|tool|agent|orchestrator|types)/[^[:space:]]+\.go:[0-9]+ | 274s | 1 | 5 | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_code_dimensions-20260915-011014 |

**Pass: 1 / 2 — Skip/Unavailable: 0 — Fail/Timeout/LaunchFail: 1**

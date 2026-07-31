# Selected parallel eval sweep

- date: 2026-07-30T14:51:49Z
- sweep_start_ts: 20260730-075149
- total cases: 6
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 2 | github_issue_gson_lazy_number | PASS | - | 183s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_gson_lazy_number-20260730-075149 |
| 1 | github_issue_zod_prefault | PASS | - | 299s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault-20260730-075149 |
| 3 | trace_query_state_churn_root_cause_rank | PASS | - | 131s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_state_churn_root_cause_rank-20260730-075453 |
| 5 | read_combo_log_current_code_boundary | FAIL | no_regex_match:internal/(orchestrator|agent|llm|render)/[^[:space:]]+\.go:[0-9]+ | 104s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_code_boundary-20260730-075705 |
| 4 | read_combo_trace_current_source_explanation | PASS | - | 196s | 1 | 1 | 0 | 1 | 0 | 2 | 0 | 0 | 0 | 0 | perf_triage | eval/results/read_combo_trace_current_source_explanation-20260730-075649 |
| 6 | qf_config_precedence | PASS | - | 107s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_config_precedence-20260730-075850 |

**Pass: 5 / 6 — Fail/Timeout/LaunchFail: 1**

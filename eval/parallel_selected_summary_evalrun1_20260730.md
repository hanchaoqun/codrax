# Selected parallel eval sweep

- date: 2026-07-30T11:03:18Z
- sweep_start_ts: 20260730-040318
- total cases: 12
- parallel: 2
- timeout: 1500s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | trace_query_blocked_reason_chain | PASS | - | 148s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_blocked_reason_chain-20260730-040319 |
| 3 | logtri_go | PASS | - | 126s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/logtri_go-20260730-040546 |
| 2 | trace_query_state_churn_root_cause_rank | PASS | - | 309s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_state_churn_root_cause_rank-20260730-040319 |
| 4 | logtri_java | PASS | - | 98s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/logtri_java-20260730-040752 |
| 6 | read_combo_log_current_code_boundary | PASS | - | 121s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_code_boundary-20260730-040930 |
| 5 | read_combo_trace_current_source_explanation | PASS | - | 186s | 1 | 1 | 0 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_source_explanation-20260730-040828 |
| 7 | qf_architecture | PASS | - | 120s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_architecture-20260730-041132 |
| 8 | qf_config_precedence | PASS | - | 128s | 2 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_config_precedence-20260730-041135 |
| 10 | github_issue_gson_lazy_number | PASS | - | 140s | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_gson_lazy_number-20260730-041344 |
| 9 | github_issue_zod_prefault | PASS | - | 252s | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/github_issue_zod_prefault-20260730-041333 |
| 11 | data_json_strict_ids | FAIL | read_exit:1 data_terminal_status:failed no_regex_match:"ids" no_regex_match:"u1" no_regex_match:"u3" | 230s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260730-041605 |
| 12 | data_multifile_reference_projection | PASS | - | 134s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_multifile_reference_projection-20260730-041745 |

**Pass: 11 / 12 — Fail/Timeout/LaunchFail: 1**

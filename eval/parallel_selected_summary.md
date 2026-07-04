# Selected parallel eval sweep

- date: 2026-07-04T19:02:01Z
- sweep_start_ts: 20260705-030201
- total cases: 9
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | qf_relation_subagent_registry | PASS | - | 114s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_relation_subagent_registry-20260705-030201 |
| 2 | arkts_repomap | PASS | - | 129s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260705-030201 |
| 3 | cangjie_repomap | PASS | - | 227s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260705-030355 |
| 4 | trace_query_openharmony_bytrace_thread | PASS | - | 285s | 2 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_openharmony_bytrace_thread-20260705-030410 |
| 5 | read_combo_log_current_source_explanation | PASS | - | 125s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/read_combo_log_current_source_explanation-20260705-030742 |
| 7 | data_basic_sum_with_rules | FAIL | data_terminal_missing no_log_regex:route=data no_log_regex:\[repl/data\] data task result | 69s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_basic_sum_with_rules-20260705-030948 |
| 8 | data_json_strict_ids | PASS | - | 52s | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_json_strict_ids-20260705-031058 |
| 9 | data_text_filter_count | FAIL | data_terminal_missing no_log_regex:route=data no_log_regex:\[.*data\] data task result no_regex_match:^2[[:space:]]*$ | 47s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/data_text_filter_count-20260705-031151 |
| 6 | read_combo_trace_current_source_explanation | PASS | - | 317s | 4 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_source_explanation-20260705-030856 |

**Pass: 7 / 9 — Fail/Timeout/LaunchFail: 2**

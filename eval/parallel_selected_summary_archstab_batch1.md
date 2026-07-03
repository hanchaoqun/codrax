# Selected parallel eval sweep

- date: 2026-07-03T01:37:11Z
- sweep_start_ts: 20260703-093711
- total cases: 6
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 1 | trace_query_donghu_real_frame_multicausal | LAUNCH_FAIL | no fresh result dir produced | 0s | - | - | - | - | - | 0 | 0 | 0 | 0 | - |  |
| 2 | trace_query_state_churn_root_cause_rank | PASS | - | 131s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_state_churn_root_cause_rank-20260703-093711 |
| 3 | read_combo_trace_current_code_boundary | PASS | - | 211s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_code_boundary-20260703-093712 |
| 4 | logtri_go | PASS | - | 169s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | log_triage | eval/results/logtri_go-20260703-093923 |
| 6 | cangjie_repomap | PASS | - | 173s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260703-094213 |
| 5 | qf_multi_member_set_count_caveat | PASS | - | 738s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_multi_member_set_count_caveat-20260703-094044 |

**Pass: 5 / 6 — Fail/Timeout/LaunchFail: 1**

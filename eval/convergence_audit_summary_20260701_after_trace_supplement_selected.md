# Selected parallel eval sweep

- date: 2026-07-01T01:39:32Z
- sweep_start_ts: 20260701-093932
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | arkts_repomap | FAIL | missing:@Component | 97s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260701-093932 |
| 1 | cangjie_repomap | PASS | - | 161s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260701-093932 |
| 3 | qf_relation_subagent_registry | PASS | - | 80s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_relation_subagent_registry-20260701-094110 |
| 5 | trace_query_wakeup_causal_io_chain | PASS | - | 122s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260701-094231 |
| 4 | read_combo_trace_current_source_explanation | PASS | - | 266s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_source_explanation-20260701-094214 |
| 6 | trace_query_donghu_real_frame_multicausal | TIMEOUT | exceeded 1200s wall-time | 1200s | 1 | 6 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260701-094434 |

**Pass: 4 / 6 — Fail/Timeout/LaunchFail: 2**

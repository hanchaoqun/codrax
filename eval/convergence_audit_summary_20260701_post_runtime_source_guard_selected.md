# Selected parallel eval sweep

- date: 2026-07-01T04:33:08Z
- sweep_start_ts: 20260701-123308
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|---------|------------|
| 2 | arkts_repomap | PASS | - | 97s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/arkts_repomap-20260701-123308 |
| 3 | qf_relation_subagent_registry | PASS | - | 116s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/qf_relation_subagent_registry-20260701-123445 |
| 1 | cangjie_repomap | PASS | - | 213s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | none | eval/results/cangjie_repomap-20260701-123308 |
| 5 | trace_query_wakeup_causal_io_chain | PASS | - | 107s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_wakeup_causal_io_chain-20260701-123641 |
| 4 | read_combo_trace_current_source_explanation | PASS | - | 128s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/read_combo_trace_current_source_explanation-20260701-123641 |
| 6 | trace_query_donghu_real_frame_multicausal | PASS | - | 300s | 1 | 2 | 0 | 1 | 0 | 2 | 1 | 0 | 0 | perf_triage+trace_query | eval/results/trace_query_donghu_real_frame_multicausal-20260701-123828 |

**Pass: 6 / 6 — Fail/Timeout/LaunchFail: 0**

# Selected parallel eval sweep

- date: 2026-07-30T11:28:35Z
- sweep_start_ts: 20260730-042835
- total cases: 10
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

| # | case | verdict | reason | sec | ana | exp | ext | fin | repair | rejects | patch | sem | self | style | runtime | result_dir |
|--:|------|---------|--------|----:|----:|----:|----:|----:|-------:|--------:|------:|----:|-----:|------:|---------|------------|
| 1 | real_trace_a3_whole_trace_overview | PASS | - | 125s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 1 | perf_triage+trace_query | eval/results/real_trace_a3_whole_trace_overview-20260730-042835 |
| 2 | real_trace_b2_tid_only_waker | PASS | - | 151s | 1 | 3 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_b2_tid_only_waker-20260730-042835 |
| 4 | real_trace_c3_vsync_periodic | PASS | - | 81s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c3_vsync_periodic-20260730-043106 |
| 3 | real_trace_c2_dstate_iowait | FAIL | no_regex_match:(^|[^0-9])(3|三) ?[次条].*(iowait|io_?wait|IO|blocked_reason|D ?状态|D-state)|(iowait|io_?wait|IO|bl | 148s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_c2_dstate_iowait-20260730-043041 |
| 5 | real_trace_d4_demand_vs_supply | PASS | - | 133s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_d4_demand_vs_supply-20260730-043227 |
| 6 | real_trace_e2_cross_trace_asymmetry | FAIL | no_text_regex_match:(excerpt|摘录|短|第二[份个]|donghu_short).{0,120}(没有|无|缺少|未采样|不含|采不� | 148s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | none | eval/results/real_trace_e2_cross_trace_asymmetry-20260730-043310 |
| 7 | real_trace_f1_exclude_no_code | PASS | - | 105s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_f1_exclude_no_code-20260730-043441 |
| 8 | real_trace_g1_english_dstate | PASS | - | 78s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_g1_english_dstate-20260730-043539 |
| 9 | real_trace_h2_dstate_dma_fence_triform | PASS | - | 178s | 1 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h2_dstate_dma_fence_triform-20260730-043627 |
| 10 | real_trace_h4_supply_thermal_witness | PASS | - | 338s | 1 | 2 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | perf_triage+trace_query | eval/results/real_trace_h4_supply_thermal_witness-20260730-043658 |

**Pass: 8 / 10 — Fail/Timeout/LaunchFail: 2**

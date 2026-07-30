# Selected Eval Manual Audit Scaffold

- date: 2026-07-30T11:28:35Z
- sweep_start_ts: 20260730-042835
- total cases: 10
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_a3_whole_trace_overview | PASS | eval/results/real_trace_a3_whole_trace_overview-20260730-042835 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 125s | 32 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 2 | real_trace_b2_tid_only_waker | PASS | eval/results/real_trace_b2_tid_only_waker-20260730-042835 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 151s | 32 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=5/2,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 4 | real_trace_c3_vsync_periodic | PASS | eval/results/real_trace_c3_vsync_periodic-20260730-043106 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 81s | 30 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 3 | real_trace_c2_dstate_iowait | FAIL | eval/results/real_trace_c2_dstate_iowait-20260730-043041 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 148s | 36 | read=1,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 5 | real_trace_d4_demand_vs_supply | PASS | eval/results/real_trace_d4_demand_vs_supply-20260730-043227 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 133s | 33 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 6 | real_trace_e2_cross_trace_asymmetry | FAIL | eval/results/real_trace_e2_cross_trace_asymmetry-20260730-043310 | log_regex,answer_regex,answer_contains | none | 148s | 37 | read=2,repo_map=0,list=0,trace=6,source_lens=0 | midloop=2,inv=3/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 7 | real_trace_f1_exclude_no_code | PASS | eval/results/real_trace_f1_exclude_no_code-20260730-043441 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 105s | 34 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 8 | real_trace_g1_english_dstate | PASS | eval/results/real_trace_g1_english_dstate-20260730-043539 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 78s | 29 | read=1,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 9 | real_trace_h2_dstate_dma_fence_triform | PASS | eval/results/real_trace_h2_dstate_dma_fence_triform-20260730-043627 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 178s | 35 | read=2,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 10 | real_trace_h4_supply_thermal_witness | PASS | eval/results/real_trace_h4_supply_thermal_witness-20260730-043658 | log_regex,trace_attachment,answer_contains | perf_triage+trace_query | 338s | 36 | read=2,repo_map=0,list=0,trace=19,source_lens=0 | midloop=2,inv=3/0,fin_reject=0,unavail=1,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

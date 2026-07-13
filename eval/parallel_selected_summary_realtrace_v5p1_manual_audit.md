# Selected Eval Manual Audit Scaffold

- date: 2026-07-13T05:33:06Z
- sweep_start_ts: 20260712-223306
- total cases: 6
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | eval/results/real_trace_h2_dstate_dma_fence_triform-20260712-223306 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 192s | 42 | read=2,repo_map=0,list=0,trace=6,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=1,prune=0 | TODO | TODO |
| 1 | real_trace_h1_binder_true_false_attribution | FAIL | eval/results/real_trace_h1_binder_true_false_attribution-20260712-223306 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 212s | 41 | read=1,repo_map=0,list=0,trace=7,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 4 | real_trace_h4_supply_thermal_witness | PASS | eval/results/real_trace_h4_supply_thermal_witness-20260712-223639 | log_regex,trace_attachment,answer_contains | perf_triage+trace_query | 144s | 36 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 3 | real_trace_h3_iofam_one_seat | PASS | eval/results/real_trace_h3_iofam_one_seat-20260712-223619 | log_regex,trace_attachment,answer_contains | perf_triage+trace_query | 207s | 41 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 5 | real_trace_h5_smr_multirow_disposition | PASS | eval/results/real_trace_h5_smr_multirow_disposition-20260712-223904 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 226s | 40 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 6 | real_trace_h6_channel_mixed_display | PASS | eval/results/real_trace_h6_channel_mixed_display-20260712-223947 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 193s | 35 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

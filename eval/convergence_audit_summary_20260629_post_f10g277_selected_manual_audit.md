# Selected Eval Manual Audit Scaffold

- date: 2026-06-29T06:43:36Z
- sweep_start_ts: 20260629-144336
- total cases: 6
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | arkts_repomap | PASS | eval/results/arkts_repomap-20260629-144336 | typed_inventory_rowset,answer_contains | none | 111s | 18 | read=6,repo_map=1,list=1,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 3 | qf_relation_subagent_registry | PASS | eval/results/qf_relation_subagent_registry-20260629-144527 | write_patch_oracle,answer_contains | none | 96s | 25 | read=4,repo_map=2,list=0,trace=0,source_lens=1 | midloop=4,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 1 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260629-144336 | typed_inventory_rowset,dimension_substring,answer_contains | none | 250s | 26 | read=11,repo_map=4,list=0,trace=0,source_lens=4 | midloop=6,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Machine PASS hid visible answer defects: final answer mixed scoped source-inventory rows with global presentation, claimed broad `foreign func` / `public class` counts, included wrong-category rows such as `ohSum` / `runOnMainThread`, and appended source-inventory supplement noise. Root class recorded as D1-G153/D1-G156 authority split, not a Cangjie-only case. |
| 4 | read_combo_trace_current_source_explanation | PASS | eval/results/read_combo_trace_current_source_explanation-20260629-144704 | trace_attachment,write_patch_oracle | perf_triage | 194s | 41 | read=9,repo_map=0,list=4,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=1 | TODO | TODO |
| 5 | read_combo_log_current_source_explanation | PASS | eval/results/read_combo_log_current_source_explanation-20260629-144747 | log_attachment,write_patch_oracle | log_triage | 154s | 31 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |
| 6 | trace_query_openharmony_bytrace_thread | PASS | eval/results/trace_query_openharmony_bytrace_thread-20260629-145018 | trace_attachment,write_patch_oracle,answer_contains | perf_triage+trace_query | 156s | 18 | read=1,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | TODO | TODO |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

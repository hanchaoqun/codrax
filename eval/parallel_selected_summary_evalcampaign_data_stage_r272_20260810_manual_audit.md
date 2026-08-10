# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T17:23:18Z
- sweep_start_ts: 20260810-102317
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_multifile_reference_projection | PASS | eval/results/data_multifile_reference_projection-20260810-102318 | log_regex,answer_regex | none | 430s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | First result consumed all four explicit materials; terminal answer is exactly `17,0,5`, reference_complete=true with 3/3 keys, all required ledgers are satisfied, and no terminal workflow violation remains. Three planner repairs and 12 data rounds are an efficiency observation, not a correctness failure. |
| 2 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260810-102318 | answer_regex,answer_contains,mermaid_edge_count | none | 571s | 42 | read=14,repo_map=2,list=0,trace=0,source_lens=0 | midloop=14,inv=5/0,fin_reject=10,unavail=0,prune=1 | fail | B467 preserved the field, but Finalizer nested model-authored `participant_boundaries` under `diagram`; the generic hoister listed this field but its shape switch omitted it, so quarantine erased the rows and the same missing-boundary contract repeated four times. Degraded output retained only three low-level call edges and did not show the requested stage/state data flow. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

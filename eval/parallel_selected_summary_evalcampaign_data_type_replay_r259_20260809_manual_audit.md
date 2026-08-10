# Selected Eval Manual Audit Scaffold

- date: 2026-08-10T02:48:57Z
- sweep_start_ts: 20260809-194856
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_json_strict_ids | FAIL | eval/results/data_json_strict_ids-20260809-194857 | log_regex,answer_regex | none | 83s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | The initial material-scheduling guard correctly rejected a script that declared both users.json and instructions.md but consumed only users.json. The normal and compact repair replies then both emitted schema-invalid emit_data_task_plan parameters, so the workflow terminated before any data round. This run did not reach the B445 late-rule generation shape and therefore is not a B445 production regression or closure witness. |
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260809-194857 | answer_regex,answer_contains | none | 273s | 28 | read=9,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=1/0,fin_reject=4,unavail=0,prune=0 | fail | B446's metadata escape is closed: deleting edge_anchors now triggers missing_relation_anchor. The final answer nevertheless proves implements edges for only 5 of 12 listed evaluator types; seven types are disconnected because exact graph roster authority reached completion but their source rows were never read/promoted to citable repomap_implementer_relation evidence for Finalizer. Four rejects and the answer's own partial-support note confirm an authority-handoff gap, not a language-specific label problem. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

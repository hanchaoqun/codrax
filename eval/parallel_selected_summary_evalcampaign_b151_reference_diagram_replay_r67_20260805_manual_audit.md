# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T01:15:09Z
- sweep_start_ts: 20260805-181508
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260805-181509 | log_regex,answer_regex | none | 465s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | The new soft candidate state worked: the evaluator explicitly declared `complete_reference=true`, `targets.csv#records`, and `canonical_label`, and grounding correctly required `17,0,5`. The repair action then used `order_by=reference`, but the executor did not recognise that enum and silently fell through to group-key sort, repeatedly emitting `17,5,0`. The terminal failure was honest and preserved the exact slot-2/slot-3 mismatch. Root gap: a documented typed order value had no executor semantics and unknown values defaulted silently. |
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260805-181509 | answer_regex,answer_contains | none | 568s | 39 | read=3,repo_map=42,list=0,trace=0,source_lens=41 | midloop=26,inv=14/0,fin_reject=0,unavail=0,prune=2 | pass | Final answer correctly shows the four-stage Mermaid flow and gives grounded responsibilities for Analyze/Explore/Extract/Finalize. The JSON predicate fix has a production witness: the first emitted analysis contains all nine booleans only under `predicates`, including typed `has_per_member_table=false`, with no schema retry. Efficiency remains poor because the analyzer also declared a repo-wide `source_inventory_profile(type)` for a bounded conceptual mechanism/member-set question. That unrelated lane forced 42 repo-map calls, 41 source-inventory lenses, 14 completion attempts, and repeated `role_type_incomplete_count` debt before the already-sufficient four-stage evidence could close. Treat as a soft classification/context gap, not an answer failure and not a reason to weaken real source-inventory completeness. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

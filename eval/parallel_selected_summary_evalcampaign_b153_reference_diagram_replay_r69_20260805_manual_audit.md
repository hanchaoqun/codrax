# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T01:53:07Z
- sweep_start_ts: 20260805-185306
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260805-185307 | answer_regex,answer_contains | none | 103s | 24 | read=2,repo_map=3,list=0,trace=0,source_lens=3 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Production witness for node/edge authority split: first-pass support seed contains four node declarations and no invented edge; the model uses collected StageBinding/AllMainStages evidence to draw only Analyze→Explore→Extract→Finalize. Nine Analyzer predicates are correctly nested, source_inventory=false, and there is no schema retry. Citations for per-stage bullets lean on enum lines although direct responsibility evidence was available in the same round; content remains correct, so this is P2 citation-selection variance rather than a hard-gate target. |
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260805-185307 | log_regex,answer_regex | none | 330s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | fail | Final 17,0,9 is wrong; expected 17,0,5. normalize_entities emitted the correct typed Beta→GroupB and Gamma alt→GroupC rows, but filtered_observations retained original _source_index values 1,2,4,5,6 while entity-resolution item_id used compacted local ordinals 1..5. apply_entity_resolutions explicitly joined base _source_index to item_id, so Beta(_source_index=4) received the fourth compacted mapping Gamma alt→GroupC. Contributions and reconcile then self-consistently certified the system-created error. This is a typed identity-domain bug, not model arithmetic or reference projection regression. No malformed-JSON/schema retry occurred; later projection churn is downstream of the poisoned materialized row. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

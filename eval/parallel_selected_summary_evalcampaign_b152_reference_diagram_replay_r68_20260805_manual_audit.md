# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T01:35:21Z
- sweep_start_ts: 20260805-183520
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260805-183521 | answer_regex,answer_contains | none | 79s | 21 | read=1,repo_map=1,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Source-inventory routing is production-fixed: Analyzer emitted `source_inventory=false`, all nine JSON predicates were correctly nested, and the case fell from 568s / 42 repo-map / 41 source lenses / 14 completion calls to 79s / 1 / 1 / 1. The final answer still has a semantic graph defect despite runner PASS: after the requested four-stage chain it appends `StageFinalize --> ReadModeMainStageBindings --> builtinStageBindings --> MutableState.WriteExplorationRequest`, presenting support/implementation symbols as post-finalize pipeline steps. Stage responsibilities are also broader than their enum-line citations. Root gap: the system's first-pass flow seed linearly connects an unordered set of diagram-eligible support nodes; node grounding does not prove edge direction or adjacency. |
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260805-183521 | log_regex,answer_regex | none | 215s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | The execution result is already correct: four source contributions, business reconciliation pass, final `17,0,5`, and an assemble receipt with `reference_projected=true`, `reference_path=targets.csv`, `reference_key_field=canonical_label`, `order_by=input`, and `order_preserved=true`. Terminal completion nevertheless failed. The output-graph input retained an earlier candidate `ReferenceGap.Present`, then overwrote its candidate/declaration with the successful typed target grounding without clearing Present, so the graph simultaneously encoded slot-by-slot grounding success and `incomplete_reference`. This is a typed state-merge conflict, not model variance or a failure of the new reference-order executor. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

# Selected Eval Manual Audit Scaffold

- date: 2026-09-01T03:50:46Z
- sweep_start_ts: 20260831-205044
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260831-205046 | answer_regex,answer_contains,mermaid_edge_count | none | 111s | 28 | read=2,repo_map=2,list=0,trace=0,source_lens=1 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | B1526 production positive: the analyzer accepted an explicit empty participant slate with `is_cross_component=false` in two iterations instead of manufacturing nodes from broad discovery entities. The model-authored answer accurately presents Analyze → Explore → Extract → Finalize, reader-facing transitions, responsibilities, and a valid Mermaid graph. |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260831-205046 | answer_regex,answer_contains | none | 339s | 47 | read=11,repo_map=3,list=0,trace=0,source_lens=0 | midloop=10,inv=3/1,fin_reject=1,unavail=0,prune=2 | fail | Core four-stage order and the stage/state tables are substantially correct, and the relation gate correctly removed nine unsupported state-flow arrows. However, analyzer normalization silently dropped eight repository-inferred participant rows and the later empty-slate error drove the model to flip `is_cross_component`; the final answer exposes internal `analyze\\x00explore`-style keys, leaves disconnected BusContext/Mutable participants, and attaches conditional pre-stage guards to Analyze/Explore rather than placing those steps before Analyze. B1527 adds a typed normalization receipt; B1528 tightens only the typed finalizer guidance and does not rewrite the answer or relax relation truth. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings

- `B1526-DIAGRAMDISCOVERYENTITYHARDGATE1/P0`: production-positive and core-closed by case 1.
- `B1527-DIAGRAMPARTICIPANTNORMALIZATIONRECEIPT1/P1`: when raw participant rows are all removed by exact current-request provenance normalization, the later typed cross-field conflict reported only an empty normalized slate. The repair model could not see why its rows disappeared and changed the independent cross-component fact to bypass the check.
- `B1528-READMODEPRESENTATIONBOUNDARY1/P1`: checkout evidence exposed internal separator-bearing lookup literals and field containment, but did not clearly tell the answer author that those literals are not reader-facing relations, that field ownership does not require a disconnected sequence participant, and that conditional pre-stages precede Analyze rather than annotate a main stage.
- Both remediations are model guidance/diagnostics from typed carriers. They do not scan request, reasoning, final prose, Markdown, or Mermaid text; do not synthesize actors or relations; and do not modify model-authored conclusions or answers.

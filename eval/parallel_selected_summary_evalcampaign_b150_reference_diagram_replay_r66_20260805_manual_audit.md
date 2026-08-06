# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T00:58:06Z
- sweep_start_ts: 20260805-175804
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260805-175806 | log_regex,answer_regex | none | 64s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Full computation was correct (GroupA=17, GroupB=4, GroupC=5), but the final typed action omitted complete_reference/reference_path/reference_key_field and emitted 17,4,5 instead of targets.csv order 17,0,5. The state graph misleadingly reported reference_complete=true even though an undeclared targets.csv.canonical_label candidate was only unjudged, while fallback prose promised full reference projection without typed authority. |
| 2 | qf_diagram_pipeline | PASS | eval/results/qf_diagram_pipeline-20260805-175806 | answer_regex,answer_contains | none | 112s | 29 | read=7,repo_map=2,list=0,trace=0,source_lens=1 | midloop=6,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Correct four-stage Mermaid flow and concise stage descriptions. Process debt: Analyzer first placed boolean has_per_member_table at root rather than predicates (one strict retry); Finalizer encoded blocks[] as a string, but the existing lossless flat-mode parser recovered all blocks and deterministic Mermaid/citation metadata repair completed with zero finalizer retry. Some stage-role descriptions cite enum/carrier lines more directly than their implementation body; advisory grounding-richness item only. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

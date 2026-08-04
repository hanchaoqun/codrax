# Selected Eval Manual Audit Scaffold

- date: 2026-08-04T13:46:20Z
- sweep_start_ts: 20260804-064619
- total cases: 2
- parallel: 2
- timeout: 900s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | data_join_entity_reconcile | PASS | eval/results/data_join_entity_reconcile-20260804-064620 | log_regex,answer_regex | none | 127s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Final `30`; two Alpha contributions 20+10, one reconcile group, and answer comparison all agree. Mapping/join parameter-contract batch introduced no semantic regression. Two failed edges were recoverable DAG-stage scheduling, not value corruption. |
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260804-064620 | log_regex,answer_regex | none | 159s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | Final `17,4,5`, expected `17,0,5`. Filtering, join, and contributions are correct; the last typed assemble action names `targets` as an input and `canonical_label` as its reference key field, but omits complete_reference/reference_path. The completion authority fails to resolve that source-derived alias back to `targets.csv`, so internally consistent present-group projection is signed complete and GroupB usurps GroupX's zero slot. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

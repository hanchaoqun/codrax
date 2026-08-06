# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T02:08:38Z
- sweep_start_ts: 20260805-190837
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_go_typo | PASS | eval/results/patch_go_typo-20260805-190838 | write_apply,write_patch_oracle,answer_contains | none | 144s | 21 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | One-line structured patch is exact; only main.go line 25 changes, the applied tree removes retrun, and verify executes go test -json ./... with changed_path_coverage(main.go)=project_runner/target_behavior/pass. No malformed JSON or empty verification signature. Efficiency gaps: Analyzer wrongly tried optional field_value_profile twice for before/after edit tokens (source_quote then owner-qualified-target rejects); Planner then proposed a source-token/external-go-build wrapper probe, hit executable-failure and production-coupling rejects, and finally omitted the unnecessary probe. Typed validators were correct, but prompt choice order imposed four avoidable retries. |
| 1 | data_multifile_reference_projection | FAIL | eval/results/data_multifile_reference_projection-20260805-190838 | log_regex,answer_regex | none | 210s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | Final answer is the exact required 17,0,5; entity application no longer cross-attaches Beta to GroupC, and reconcile/output projection pass. Three contributions are also semantically complete for the chosen valid DAG: r1=10+r2=7 for target T1 and r5=5 for target T3; inactive r3, non-target GroupB/r4, and unresolved r6 are excluded before contribution calculation. Runner FAIL is an over-hard eval oracle requiring exactly contributions=4, which assumes a different but not uniquely required DAG that computes non-target GroupB before projection. The oracle should require a non-empty contribution ledger plus reconcile=pass and the exact answer, not one internal cardinality. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

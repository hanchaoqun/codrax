# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T19:06:14Z
- sweep_start_ts: 20260811-120613
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_gson_lazy_number_symptom | FAIL | eval/results/github_issue_gson_lazy_number_symptom-20260811-120614 | write_apply,write_patch_oracle | none | 124s | 22 | read=4,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B561 production positive: the patch is correct, one source-level check passed, and the final card now preserves that partial evidence while keeping the Java behavior verdict unverified because the runtime is unavailable. The runner FAIL is the expected full-verification oracle, not a false product success. |
| 2 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260811-120614 | answer_regex,answer_contains,mermaid_edge_count | none | 743s | 53 | read=14,repo_map=4,list=0,trace=0,source_lens=0 | midloop=14,inv=5/1,fin_reject=19,unavail=0,prune=3 | fail | The recovered model draft contains accurate component prose and grounded relation fragments, but the requested participant topology is disconnected after 19 pre-emit rejects. This is not a fixed-four-minute timeout: model progress remained active through 743s. The hard contract is satisfiable, but its repair payload conflates relation endpoint authority, participant display identity, and boundary state; it does not provide a copyable exact anchor/display action, so the model repeatedly retargets exact endpoints to broad component names or toggles between an edge and an unproven boundary. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

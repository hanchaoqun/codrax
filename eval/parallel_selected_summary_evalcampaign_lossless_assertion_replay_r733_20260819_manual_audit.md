# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T14:23:27Z
- sweep_start_ts: 20260819-072326
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_nlohmann_long_double_symptom | FAIL | eval/results/github_issue_nlohmann_long_double_symptom-20260819-072327 | write_apply,answer_regex | none | 206s | 26 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B1168 production positive: the malformed selected `changes` wrapper recovered `changes + acceptance_tests + project_test_observations` together and the persisted plan retained the exact test/suite/assertion/contract refs. The source patch is the correct `%.*Lg` fix and `make check` passed. B1165 also gets its exact negative join: the only execution receipt remained aggregate (`suite=check`, `assertion_id=make-test`), so it did not impersonate the declared `tests/long_double_format.cpp / long_double_format_cpp / snprintf_float_produces_nonempty_output` observation; final verdict stayed honestly unverified. |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-072328 | answer_regex,answer_contains,mermaid_edge_count | none | 753s | 52 | read=19,repo_map=3,list=0,trace=0,source_lens=2 | midloop=17,inv=12/0,fin_reject=5,unavail=0,prune=0 | partial | Final answer is present and the Mermaid is legal: it preserves stage precedence plus model-authored reader labels and three independently grounded carrier edges. It still cost 49 explorer iterations, 12 completion attempts, 5 finalizer rejects and 103,070 context tokens. One parser-owned argument repair (`types.AgentExtractor -> ctxbuilder.BuildAgentContext`) was returned byte-exact on six completion attempts because the model repeatedly omitted `subject`; the pending validation latch has no bounded unproven exit and eventually reopened broad exploration. Finalizer then chased `anchor_without_visible_edge -> participant_identity_not_visible -> component_not_connected` with repeated ~28KB relation payloads. This is system convergence/payload debt, not mere model variance. The final component chain is useful but remains an abstraction over exact technical endpoints, so relation semantics are partial rather than a proof of one end-to-end shared-state path. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

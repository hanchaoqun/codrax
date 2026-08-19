# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T22:27:49Z
- sweep_start_ts: 20260819-152747
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-152749 | log_regex,write_apply,answer_regex,answer_contains | none | 364s | 26 | read=8,repo_map=3,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | B1190 positive: the 24-step apply lane reached a fourth repair, removed the stale ids reset, and both the bounded probe plus Make check passed. Human delivery still fails because exact project_test_observations were never run through an assertion-scoped candidate; the aggregate Make pass cannot mint that receipt, yet the controller repeatedly reran the same probe/Make pair and ended missing_terminal_verify_verdict. B1193 confirmed. |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260819-152749 | answer_regex,answer_contains,mermaid_edge_count | none | 648s | 49 | read=8,repo_map=4,list=0,trace=0,source_lens=0 | midloop=6,inv=7/0,fin_reject=6,unavail=0,prune=0 | partial | B1189 positive: analyzer emitted separate Mutable and BusContext obligations. The final recovered answer contains a useful four-stage precedence graph and three Mutable interactions, but six finalizer rejections caused degraded output. Exact precedence endpoints were valid; separate local call edges reused stage participant nodes for method identities. The retarget validator named only participants, not the unique offending body-edge/anchor side, while publishing unrelated precedence candidates. The model repeatedly edited boundaries/stage edges instead of the offending local endpoint. B1192 confirmed; active semantic output was never cut off at 4m. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

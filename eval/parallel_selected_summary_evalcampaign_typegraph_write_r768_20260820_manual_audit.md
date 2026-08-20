# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T11:23:11Z
- sweep_start_ts: 20260820-042310
- total cases: 2
- parallel: 2
- timeout: 2400s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_nlohmann_long_double | PASS | eval/results/github_issue_nlohmann_long_double-20260820-042311 | write_apply,answer_regex | none | 118s | 26 | read=6,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | B1232 production positive. The applied tree changes only the two requested production headers from `%.*lg` to `%.*Lg`; `tests/long_double_format.cpp` is unchanged. `make check` exited 0, both changed paths are covered, the worktree audit is clean, and final status is verified. The six planning-only contracts remain planning guidance and no longer mint an uncloseable `project_test_assertion_not_observed` debt. The final handoff explicitly bounds `all verified` to required structured obligations and does not claim independent execution evidence for every natural-language acceptance item. |
| 1 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260820-042311 | answer_regex,answer_contains | none | 254s | 34 | read=14,repo_map=1,list=0,trace=0,source_lens=0 | midloop=8,inv=4/2,fin_reject=2,unavail=0,prune=0 | fail | B1233 production positive on its visible result: all 12 exact implementer-to-interface edges remain, every anchor/body pair keeps model-authored `implements`, and neither `type_relation` nor another raw relation enum leaks to readers. The two rejects came from citation identity adoption and participant coverage, not label deletion. New deterministic B1234: analyzer represents the requested set role as participant `主要实现类型`; participant coverage then requires unproven boundaries for both that abstract set role and `LoopController` even though the exact provider proves 12 concrete implementer→LoopController relations. The accepted answer therefore simultaneously draws all proved edges and says those same requested relations are unproven. Adding a subgraph only makes the abstract participant visible; it cannot resolve the semantic contradiction. Runner is a false positive. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

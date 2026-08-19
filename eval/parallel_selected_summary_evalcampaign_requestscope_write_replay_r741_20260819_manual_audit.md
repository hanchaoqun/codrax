# Selected Eval Manual Audit Scaffold

- date: 2026-08-19T20:08:34Z
- sweep_start_ts: 20260819-130833
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260819-130834 | answer_regex,answer_contains,mermaid_edge_count | none | 260s | 37 | read=7,repo_map=3,list=0,trace=0,source_lens=0 | midloop=5,inv=3/0,fin_reject=2,unavail=0,prune=0 | partial | B1182 production positive: the old repo/parser false spine is absent and invented arrows were rejected. B1183 confirmed: a no-arrow subgraph made `Mutable` the owner of `BusContext`, reversing the typed containment recipe while passing because grouping structure is not validated. Final prose still overstates full shared-state flow despite the diagram boundary; keep as model/context guidance debt, not a prose hard gate. |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260819-130834 | log_regex,write_apply,answer_regex,answer_contains | none | 574s | 26 | read=7,repo_map=2,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=1,prune=0 | fail | Patch is correct; `newline-collapse-probe` and `make check` both passed. B1184 confirmed: admission already proved the Python probe imports the sole changed module, but the proof ledger discarded all contract authority because the planner did not redundantly emit `changed_symbol_refs`; it then created an unnecessary proof-followup and ended `missing_terminal_verify_verdict`. B1179 verify-failure branch was not exercised because no verification failed. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusions

- B1182 is production-positive. Requested relation scope no longer consumes repo/parser expansions; `renderExplorerToolBudgetPlan -> append -> BusContext` did not reappear.
- B1183 is a generalized diagram-semantics gap, not a Mermaid syntax issue. A subgraph is a visible ownership/containment assertion even when it has no arrow. Validation currently checks node and edge authority but not owner/member direction in group nesting.
- B1184 is a typed-ledger contradiction. The same structured Python import signal was hard enough to admit the probe as coupled to the changed module, but was not carried into `changed_symbol_refs`; post-apply proof therefore treated a passed exact probe as unbound. The fix must reuse the existing language parser only for a unique changed-file match, never infer from request/plan/answer prose and never treat aggregate project-test rows as assertion-scoped proof.
- Active response bytes continued well beyond 4ms in both cases. No fixed-age degradation or empty answer occurred. Legitimate caller cancellation/deadline and byte-stall handling remain unchanged.
- This replay did not modify or exercise Trace. Explicit windows, causal projection, auto-supplement, typed on-chain root causes, occupancy/business clues, and rule-priced eliminable impact remain protected.

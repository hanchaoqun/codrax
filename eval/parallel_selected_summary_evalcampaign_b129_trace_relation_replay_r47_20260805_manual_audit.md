# Selected Eval Manual Audit Scaffold

- date: 2026-08-05T20:34:12Z
- sweep_start_ts: 20260805-133410
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260805-133412 | answer_regex,answer_contains | none | 124s | 20 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=2,unavail=0,prune=0 | pass | The final answer correctly selects `JsonPlugin` and explains `run_pipeline -> resolve -> REGISTRY["json"] -> cls()` plus import-time decorator registration. It honestly removes the optional diagram after two unsupported-edge rejects. Minor locator drift remains: it says the class is defined at line 17, while line 17 is the decorator and the class statement is line 18. Process audit found the parser had all three bases but unified evidence merge collapsed same-line sibling relations to one; this did not invalidate the asked registration answer but is a generic context-loss gap. |
| 2 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260805-133412 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 167s | 29 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | Explicit 5.000–5.007s window, deterministic supplement, occupancy axis, existing-rule eliminable axis, root ranking, wakeup chain and Trace causal projection all survived with zero retries. However the model first calls `class_verification` the dropped-frame root cause while also stating `causal_conclusion=unproven` / `frame_evidence_status=absent`, then invents the counterfactual “fix #1 and #2 naturally disappears.” No typed joint counterfactual or cross-direction overlap row proves that dependency. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T04:09:48Z
- sweep_start_ts: 20260811-210945
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_java_call_chain | PASS | eval/results/sr_java_call_chain-20260811-210948 | primary_answer | none | 117s | 24 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=2,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Ordinary direct-call regression remained clean: four load-bearing hops, the capacity guard and its max source, repository insert, and AuditLog.record are all identified with source lines. The answer also corrects the question's “audit persistence” premise: AuditLog.record currently writes stdout, not a database. No callback-pair debt, finalizer reject, malformed recovery, system answer, or elapsed fallback. The optional graph was omitted, but the user did not explicitly request one and the hop list preserves the relationship surface. |
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260811-210948 | answer_regex,answer_contains | none | 140s | 23 | read=2,repo_map=1,list=0,trace=0,source_lens=1 | midloop=6,inv=2/0,fin_reject=1,unavail=0,prune=0 | pass | B601 production positive: callback normalization accepted loop.run_in_executor->handle, action-required callback_receiver_call_pair asked the model to emit the independent run_pipeline->loop.run_in_executor call, and the model did so. Final context reports grounded_callsite_facts=2 / explicit_typed_directed_relations=4 and one connected verified component spanning resolve, JsonPlugin instance, run_pipeline, executor and handle. Core class, lookup/instantiation, decorator registration and cooperative MRO answer is correct. The first optional diagram ignored the exact recipe and invented unsupported messages, received one precise reject, then the model removed the optional block; record as B602 soft recipe-consumption/churn watch, not a required-diagram hard gate. No malformed recovery, system answer, or elapsed fallback. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

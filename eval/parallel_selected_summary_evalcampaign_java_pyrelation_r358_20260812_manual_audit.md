# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T03:54:39Z
- sweep_start_ts: 20260811-205436
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_java_annotation_route | PASS | eval/results/sr_java_annotation_route-20260811-205439 | answer_regex,answer_contains | none | 103s | 23 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | uncertain | B599 production positive: 2 exact call rows survived, final context reports grounded_callsite_facts=2 / explicit_typed_directed_relations=2, and the old false claim that StatsHandler lacked direct @Route disappeared. Core /stats binding, register/getAnnotation, injectFields/field.set and shared AuditLog conclusions are correct. The lead nevertheless says Router “scans all Handler implementations”; source Main explicitly constructs and registers handlers, so this is one model-authored over-generalization contradicted by its own later register(handler) explanation. No finalizer retry, malformed recovery, system answer, or fixed-age fallback. Keep as model-variance watch; do not add a prose hard gate. |
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260811-205439 | answer_regex,answer_contains | none | 108s | 23 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | fail | Prose correctly identifies JsonPlugin, import-time @register("json"), resolve lookup, executor callback handoff, and cooperative MRO. The first diagram was correctly rejected for unproved resolve->REGISTRY and wrong callback endpoints, but the accepted patch became two disconnected components: run_pipeline->resolve and loop.run_in_executor->handle. Exact line 17 independently proves run_pipeline->loop.run_in_executor plus loop.run_in_executor->plugin.handle; callback normalization retained only the latter. This is B601 relation-composition loss, not model variance or Mermaid syntax. No malformed recovery, system answer, or elapsed-time fallback. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

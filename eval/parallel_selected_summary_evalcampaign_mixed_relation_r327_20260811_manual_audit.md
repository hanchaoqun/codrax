# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T18:22:54Z
- sweep_start_ts: 20260811-112253
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260811-112255 | answer_regex,answer_contains | none | 149s | 23 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | The model kept a rich flow first, then attempted a smaller but still unsupported graph, and finally removed it. AxisFlow intentionally withheld the whole-diagram skeleton, so B555's repair promise had no copy-ready payload; exact per-edge recipes existed elsewhere but were not repeated in the local optional repair. The exact B550 caveat remained local. |
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260811-112254 | answer_regex,answer_contains | none | 319s | 29 | read=3,repo_map=1,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=5,unavail=0,prune=0 | fail | Typed context carried two unary guards and the return relation, but the required call_dag skeleton retained only three call edges. It falsely reported `visual_annotation_relation_count=0` while naming omitted return as preserved Notes. Five diagram/endpoint repair rounds converged only after the exact call skeleton was repeated. Prose stayed useful, but the graph omitted the runtime selection condition/result. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- Runner: 2/2; human: 0/2.
- `B554` is production-positive only for typed acquisition, not visual closure: `typed_unary_annotations` carried exact C++ and Python guards, but flow/call_dag capability remained closed and the final visuals did not carry them. Extend flow-family skeletons with standalone, unanchored fact nodes; never synthesize a guard arrow/self-loop.
- `B555` did not close Python: `AxisFlow` correctly withholds a system-authored whole-flow skeleton, but the optional repair did not fall back to the existing exact typed relation-boundary recipes. New `B556-OPTIONALFLOWBOUNDARYREPAIR1/P1-high`: repeat bounded exact recipes locally and prefer a faithful subset; deletion remains a model-owned last option.
- New `B557-ANNOTATIONRECEIPT1/P1`: call_dag authority emitted `visual_annotation_relation_count=0; annotation_relation_kinds=return` followed by text claiming the relation was preserved as a Note, although no Note/fact node existed. Counts, kinds and carrier claim must come from the actually rendered visual subset; omitted facts need an explicit not-rendered disclosure.
- C++ spent five Finalizer rejects on mixed block/diagram identity repairs before the exact required skeleton lane became applicable. Record `B558-REPAIRCONVERGENCEWATCH/P2` and re-evaluate after B554/B556/B557; do not create a broad auto-rewrite or relax hard evidence gates from one model trajectory.
- The 319-second C++ run remained active and delivered the model's own answer. No fixed four-minute fallback or system-written answer occurred; this is a production positive for the active-stream red line.

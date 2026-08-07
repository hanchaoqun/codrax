# Selected Eval Manual Audit Scaffold

- date: 2026-08-07T00:52:26Z
- sweep_start_ts: 20260806-175225
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260806-175227 | answer_regex,answer_contains | none | 106s | 21 | read=4,repo_map=4,list=0,trace=0,source_lens=1 | midloop=4,inv=2/0,fin_reject=2,unavail=0,prune=0 | pass-with-gap | S36d parser fix is production-confirmed: no synthetic `write<br/>` edge and no degraded recovery. The final text correctly covers `Logger.log -> sink_->write -> ConsoleSink.write -> std::fputs(stderr)`, error-level flush, factory branch, constructor injection and type inheritance. It still spends two rejects trying unsupported dynamic-dispatch bridge arrows before honestly deleting the optional diagram; typed relation authoring remains too reconstruction-heavy. |
| 2 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260806-175226 | answer_regex,answer_contains | none | 108s | 20 | read=2,repo_map=1,list=0,trace=0,source_lens=1 | midloop=6,inv=2/0,fin_reject=2,unavail=0,prune=0 | fail | The class selection, import-time registration, `resolve`, and callback handoff text are correct. The answer only lists the base roster and omits the actual cooperative execution order `TimestampMixin.handle -> ValidationMixin.handle -> BasePlugin.handle`. Its first diagram also recasts dictionary lookup and callback handoff as ordinary calls; after two rejects the model removes the diagram. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `4c8fda49a` closes the Mermaid-label false-edge regression: the former C++ seven-reject / 476s degraded path is now a normal 106s structured answer. This is a parser-level production witness, not merely a unit-test green.
- Both cases still incur exactly two finalizer rejects for the same cross-language reason. The hard relation gate is semantically correct, but the prompt gives typed facts and a generic JSON contract on separate surfaces; the model must reconstruct node IDs, map evidence shapes to relation kinds, and keep Mermaid plus `edge_anchors[]` synchronized.
- Register `EVAL-B208-POLYCOMPOSE1`: publish a bounded, citable, typed relation authoring capsule using the real `DiagramEdgeAnchor` JSON carrier. It may expose call/callback/register/assignment/return/type-relation facts and reusable node aliases, but must neither require a diagram nor synthesize a dynamic bridge or final conclusion.
- Keep `EVAL-B209-COOPPATH1` open: declared base order and independent `super()` facts do not yet produce a typed cooperative execution-path handoff. Solve this through parser/graph relations across supported languages, not Python fixture words or final-answer scanning.
- JSON decoding was clean in r139: strict remap/carrier/element and both string-recovery counters are all zero. These rejects are semantic relation-authoring failures, not malformed JSON. Schema remains the sole field/type/required authority.

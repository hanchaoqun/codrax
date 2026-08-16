# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T03:28:18Z
- sweep_start_ts: 20260815-202816
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260815-202818 | answer_regex | none | 133s | 25 | read=2,repo_map=1,list=1,trace=0,source_lens=1 | midloop=5,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B874 production pass: no disconnected whole-diagram skeleton, no finalizer rejection/patch, and the model retains the complete Python → `_fastlex` → pyo3 wrapper → Rust core chain plus the mutually-exclusive Python fallback as an ordered list. New B875: analyzer hard-rejects the explicitly requested fallback subtopic twice because conceptual entities do not resolve as repo symbols. |
| 2 | qf_type_relation_loop_controller | PASS | eval/results/qf_type_relation_loop_controller-20260815-202818 | answer_regex,answer_contains | none | 156s | 27 | read=14,repo_map=2,list=0,trace=0,source_lens=0 | midloop=4,inv=2/0,fin_reject=0,unavail=0,prune=0 | fail | All 12 production implementers and type-relation arrows are correct, Mermaid is valid, and there is no finalizer repair. However the lead falsely says every Go struct embeds a `LoopController` field; the structs merely satisfy the interface method set. B876 confirms that typed `implements` vs embedding/extends semantics need clearer language-neutral guidance. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### mr_poly_binding_chain

- Compared with r546, runtime falls from 163s to 133s and finalizer rejects/patches fall from 1/1 to 0/0. The old system-provided disconnected sequence skeleton is absent.
- The model chooses no optional diagram and answers with a six-stage ordered path/fallback explanation. This is not a fixed-age or missing-answer degradation: every requested chain identity, native module name, registration boundary and fallback branch remains visible.
- Minor prose precision debt remains around the Rust wrapper/core signature, but the requested path and branch conclusion are correct enough for human pass.
- Analyzer quality gate rejects twice before exploration. In both rejects, the fallback subtopic is directly required by the question, yet `fallback`, `py_ext`, `Optional` or equivalent conceptual entities receive zero repo-symbol hits while the sibling chain subtopic resolves. R1.5 treats that asymmetric resolver signal as deterministic hallucination and forces a retry. This is B875 and violates the noisy-signal/hard-gate red line.

### qf_type_relation_loop_controller

- The final table and Mermaid graph enumerate the 12 production implementations and direct every `type_relation` edge implementer → `LoopController`; no test-only implementer leaks into the principal set.
- The graph is valid and uses implements rather than call arrows. No system patch or answer replacement occurs.
- The lead nevertheless states that each struct embeds the `LoopController` interface in a field. Source inspection confirms `analyzerEvaluator`, `answerDocumentEvaluator`, and peers do not embed such a field; Go conformance is implicit through the `Observe` method set. The typed relation predicate was `implements`, so this is a semantic overreach, not missing source evidence.
- B876 should remain soft-guidance/typed-vocabulary work: preserve each exact relation subtype (`implements`, `extends`, `embeds`, conformance) and explain its language semantics without scanning or rewriting model/final prose.

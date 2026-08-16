# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T03:10:45Z
- sweep_start_ts: 20260815-201044
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260815-201045 | answer_regex | none | 163s | 25 | read=3,repo_map=5,list=1,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | Prose names the complete Python → `_fastlex` → pyo3 wrapper → Rust core path and fallback, but the system-provided repair skeleton replaces the rejected diagram with three disconnected call pairs plus Notes. The graph does not connect `_fastlex.tokenize_bytes` to the pyo3 wrapper/module registration, so it does not visually express the requested cross-language chain. This is B874, not model variance. |
| 1 | real_trace_h11_cross_direction_overlap | PASS | eval/results/real_trace_h11_cross_direction_overlap-20260815-201045 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 178s | 38 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | B873 production pass. The answer keeps exact 12.115ms only for the typed mutually-exclusive #2/#3 pair, leaves keva and cross-direction relations unresolved/non-additive, and does not turn absence of a pair row into independence. Explicit window, causal projection, auto-supplement, on-chain root ranking, actual occupancy/business spans, and priced eliminable directions are all retained. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### real_trace_h11_cross_direction_overlap

- The model-authored lead preserves both requested dimensions: actual state/occupancy (`running=157.248ms`, business-span families) and existing-rule recoverable space (`58.320ms` supply fold, `12.115ms` exact lock/priority subtotal, scheduling and IO leaders).
- `12.115ms` is derived only from CompThread_0 `7.405ms` plus JankManager `4.710ms`, whose typed relation is mutually exclusive. Extra same-direction seats and all cross-direction combinations remain unresolved and non-additive.
- The final answer uses “relationship unresolved/not established” for missing pair authority. No physical-independent conclusion is minted from separate accounting vocabulary. `Trace 因果投影` and deterministic supplement are present; adjacent/background rows stay support-only.
- Minor presentation debt remains (`mutually_exclusive`, `Recovery`, and several audit enums leak into Chinese prose), but it does not overturn the B873 semantic result and is not a reason to fail this case.

### mr_poly_binding_chain

- Investigation emits enough typed evidence for the complete mechanism: two Python calls, the pyo3 wrapper-to-core call, module import/availability facts, and `m.add_function(wrap_pyfunction!(tokenize_bytes, m)?)?` registration evidence.
- The first model diagram overstates several relations as calls and is correctly rejected. The retry hint then provides a copy-ready skeleton containing only `n4→n5`, `n4→n6`, and `n9→n10`; module and registration facts become unanchored Notes.
- The model copies that skeleton. The resulting diagram is valid Mermaid and evidence-safe, but it is not a cross-language call-chain visualization: its components are disconnected and the module-registration bridge is absent as a directed typed relation. Runner PASS only checks answer text, so human correctness is fail.
- Generalized gap B874: the optional repair-skeleton selector optimizes per-edge validity but has no request-scoped topology/coverage criterion. A disconnected subset can therefore be promoted as the repair for a question whose principal requested value is the relationship path. The fix must operate on typed relation carriers and requested participant/path coverage, not on user/model/final prose keywords and not by system-authoring a diagram.

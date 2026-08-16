# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T21:02:31Z
- sweep_start_ts: 20260816-140230
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260816-140231 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 235s | 46 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B934 production-positive: final prompt carries incomplete scopes and the answer now discloses root-rank 12/65 plus critical-blocking 20/33 instead of claiming no omissions. B933 wiring is present, but the final answer still turns the kernel call-site symbol `dma_fence_default_w` into a GPU-fence wait object/cause and an optimization directive, then later admits object/holder/subsystem are unknown. The coarse `fix_direction=io_dependency` token means IO/kernel/dependency, while this seat has io_wait=0; that raw internal token remains a misleading second signal. Supply and inversion arithmetic are also imprecisely paraphrased. |
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260816-140231 | answer_regex | none | 284s | 32 | read=4,repo_map=3,list=1,trace=0,source_lens=1 | midloop=6,inv=1/0,fin_reject=5,unavail=0,prune=0 | partial | B935 production-positive: the compact handoff preserves the exact `py.tokenize_bytes -> tokenize_bytes` wrapper/core edge and the final diagram keeps it. Five finalizer rejects remain. Two are caused by copying the display label `tokenize_bytes (core)` into exact edge identity even though a unique typed recipe names `tokenize_bytes`; later patches correct it. More importantly, full-block patch replacements omit the original `facet_ids` and `surface_role`, so the accepted final block loses `principal_path_edge` and the renderer appends a false-looking “main-path relation incomplete” caveat despite four verified list anchors and a valid diagram. Several item citations are also misbound. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings and next batches

1. `B934-RUNTIMEENUMERATIONFINALAUTHORITY1` is closed in production. Its final-tail authority is visible before synthesis and the model consumes the incomplete row counts. Exact separately complete target-wait rows remain separately usable.
2. `B933-BLOCKEDREASONCALLSITEFINALCALIBER1` is only partial. The call-site/object/holder boundary is present, but the raw coarse repair-family token `io_dependency` still competes with it. The generalized follow-up must expose `fix_direction_caliber=coarse_validation_family` and state that identifier spelling plus the repair-family token do not prove a waited object, holder, subsystem, or IO mechanism. This is typed prompt guidance only; no answer-text scan, rejection, or rewrite.
3. `B935-REGISTEREDCALLABLEQUALIFIEDIDENTITY1` is closed in production: the true wrapper/core edge survives compact selection and appears in the diagram.
4. New `B936-STRUCTUREDPATCHCARRIERPRESERVATION1` (P1): a local full-block replacement can silently drop stable carrier metadata. Safely inherit omitted `facet_ids` and `surface_role` only when the replacement names one exact previous block, preserves its kind, and retains at least one exact non-empty item ID. Explicit fields (including explicit empty `facet_ids`) remain model-owned; ambiguity or wholesale content replacement does not inherit.
5. New `B937-DISPLAYQUALIFIERIDENTITYRECEIPT1` (P1): reader-facing suffixes such as `name (role)` can leak into `from_identity/to_identity`. Repair only when one dispatch-scoped typed recipe uniquely matches both endpoints after removing one whitespace-separated trailing display qualifier. The repair changes identity metadata only; it never changes visible labels, prose, edge direction, relation kind, or creates a relation. Ambiguity remains fail-closed. Apply uniformly to standalone structured relation carriers and diagrams, independent of source language.
6. The remaining citation misbinding is retained as a separate observation; it must be addressed through typed row/citation identity rather than label keyword fitting.
7. No active-stream age degradation, malformed-JSON recovery, empty answer, or system-authored conclusion occurred. Explicit-window Trace causal projection, auto-supplement, on-chain-only primary causes, background support-only, and the actual-occupancy versus rule-priced eliminable axes remain intact.

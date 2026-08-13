# Selected Eval Manual Audit Scaffold

- date: 2026-08-13T09:14:11Z
- sweep_start_ts: 20260813-021409
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260813-021411 | answer_regex,answer_contains,mermaid_edge_count | none | 229s | 38 | read=10,repo_map=3,list=1,trace=0,source_lens=0 | midloop=5,inv=3/0,fin_reject=1,unavail=0,prune=1 | fail | B715: route classifier and AgentContext both carried `PresentationDiagramRequired=true`, but the AgentContext-to-tool `BusContext` projection dropped that typed authority. `emit_analysis` therefore normalized the required diagram to optional. The first draft contained a diagram whose unsupported relations were correctly rejected; the repair prompt then falsely called the requested diagram optional and explicitly allowed deletion, so the model removed it and the no-diagram answer passed. This is a deterministic projection gap, not model variance or a reason to relax relation evidence. |
| 1 | real_trace_h7_self_seat_full_spectrum | FAIL | eval/results/real_trace_h7_self_seat_full_spectrum-20260813-021411 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 248s | 46 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=1,prune=0 | fail | B716: the explicit-window typed projection remained correct: it selected the 233.190ms user window, crowned the on-chain 39.734ms supply-fold seat, retained D/IO, inversion, deterministic JIT and business-span evidence, and kept adjacent/background rows out of the root-cause board. The model prose instead added supply deficits from three nested, overlapping query windows into 129.805ms and promoted two adjacent runnable rows by addition into a 69.113ms root-cause item. It also treated the recorded blocked-reason caller as a GPU-fence wait object despite the typed caveat. The runner's fixed 65.912/49.623 oracle is stale relative to the current selected-window projection, but changing that regex would not cure the product defect. Finalizer context must foreground one canonical requested-window projection and typed cross-window non-additivity before drafting, while preserving other windows as explicitly non-additive exploration support. No prose keyword gate and no system rewrite. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

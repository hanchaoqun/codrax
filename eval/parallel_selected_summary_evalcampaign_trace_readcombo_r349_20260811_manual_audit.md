# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T01:06:59Z
- sweep_start_ts: 20260811-180657
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_frame_semantic_span_optimization | PASS | eval/results/trace_query_frame_semantic_span_optimization-20260811-180659 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 165s | 28 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B587 production positive: analyzer no longer retained pre-triage scalar/summary authority and explicit-window trace rows remained complete. B585 residual: the exact current-source quote was still copied into a typed answer-member exclusion. Final prose also placed runnable delay after VerifyClass completion although the intervals overlap, and called target running time a direct frame-incompletion manifestation despite frame evidence being absent. The typed trace context itself remained accurate; no system rewrite is authorized. |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-180659 | answer_regex,answer_contains | none | 332s | 39 | read=19,repo_map=3,list=0,trace=0,source_lens=0 | midloop=10,inv=2/0,fin_reject=3,unavail=0,prune=1 | fail | B588 production positive: checkout-verified stage binding/topology authority was read and all four stages appeared in prose/table. Human graph failure: the final diagram replaced the required Analyzer→Explorer→Extractor→Finalizer precedence spine with supporting Orchestrator/helper calls, despite the prompt carrying the exact typed spine. Soft selection is insufficient; a precise provider-driven principal-spine completeness gate is required. The 332s active model stream completed normally without elapsed-time degradation. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

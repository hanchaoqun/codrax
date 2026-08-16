# Selected Eval Manual Audit Scaffold

- date: 2026-08-16T16:41:12Z
- sweep_start_ts: 20260816-094110
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260816-094112 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 240s | 41 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=1,inv=1/0,fin_reject=1,unavail=0,prune=0 | partial | Four-state and exact frequency facts are correct, but a bounded “limit row → target effect” question was analyzed as full causal diagnosis and received a full Trace causal projection. The principal answer says CPU12 has no limit record and policy constraint cannot be confirmed, but the narrow regex expects a different binding/impact phrase. Product GAP is typed analysis-role ambiguity, not a reason to relax causal projection for genuine diagnostic windows or to tune the regex alone. |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260816-094112 | answer_regex,answer_contains,mermaid_edge_count | none | 879s | 41 | read=21,repo_map=4,list=0,trace=0,source_lens=0 | midloop=8,inv=5/0,fin_reject=3,unavail=0,prune=0 | partial | Final Mermaid is valid and preserves stage precedence plus several typed operations, but Mutable and BusContext remain disconnected while prose calls them the shared backbone. Earlier drafts invented unsupported edges and were correctly rejected. Three completion calls each spent about 90s in aggregate_normalization; code audit found repeated whole-repository symbol/relation scans in SOFT repair navigation. B915 replaces them with one graph-derived exact index and adds sub-phase timings; production speed requires replay. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Additional runtime witness: REPL logs

- `/Users/han/opt/customlogs/repl_log.txt` and `/Users/han/opt/customlogs/repl_log2.txt` show an active large-Trace stream continuing for minutes; they do not show a 4ms or fixed-total-age degradation. Byte activity correctly kept the request alive.
- The historical run eventually exhausted structured finalization and rendered the last model prose, which incorrectly claimed that no systrace content had been provided despite extensive typed Trace evidence. Current main already has structured-document and model-surface recovery lanes, but raw last prose can still outrank an earlier useful model-authored surface when no valid document exists. This remains a typed carrier-order audit item; do not select drafts by prose keywords and do not synthesize a system conclusion.
- The run's long exploration and two no-tool finalizer rounds are separate from the B915 completion-scan hotspot. Neither issue authorizes a 4ms/fixed-age fallback while bytes remain active.

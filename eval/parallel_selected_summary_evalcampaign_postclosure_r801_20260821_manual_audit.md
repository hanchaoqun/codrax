# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T07:11:32Z
- sweep_start_ts: 20260821-001131
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-001132 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 196s | 32 | read=0,repo_map=0,list=0,trace=2,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | System/typed surface passed: explicit 2.000..2.020s window, two target-filtered trace queries, four-hop wakeup chain, 11.000ms on-chain IO first seat, three independent 1.000ms runnable/priority candidates, elapsed-vs-eliminable ledgers, background isolation, and one complete Trace causal projection all survived. Post-completion source enrichment was 0ms with no source fallback. Model prose still over-expanded the `fscache_page_wait_on_page_bit` call-site and wakeup ordering into a concrete page-cache mechanism / chain-control claim beyond the published wait-completion authority; keep under the existing soft-guidance audit, with no prose hard gate or system rewrite. |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260821-001132 | answer_regex,answer_contains | none | 498s | 44 | read=20,repo_map=3,list=0,trace=0,source_lens=0 | midloop=9,inv=3/1,fin_reject=3,unavail=0,prune=5 | partial | B1281 production-positive: the accepted completion dispatch spent 3ms in structured evidence and 3ms in concrete values, with no broad Dataflow rerun; wall time fell from r800 634s to 498s and the final table remained complete. The final Mermaid is legal and retains analyze→explore→extract→finalize, but three finalizer rejects exposed B1282: the first reject already carried participant and 18 relation failures, while retry routing disclosed only the participant delta; relation failure refs appeared only after two more patches, so the model ultimately removed all 18 unsupported orchestration/state arrows and shipped a much thinner graph. Final prose also misstates the `runReadSchedulerLoop` definition as line 4200 despite citing its actual line 4528, so runner PASS is not a full human pass. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

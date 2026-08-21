# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T07:33:37Z
- sweep_start_ts: 20260821-003335
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-003337 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 224s | 35 | read=0,repo_map=0,list=0,trace=13,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit 2.000..2.020s window, four-hop typed wakeup chain, 11.000ms on-chain IO first seat, three independent 1.000ms priority candidates, occupancy/eliminable ledgers, background isolation, and full Trace causal projection all survived with zero finalize retry. Active stream was not degraded by a fixed duration. Presentation quality remains partial: the model copied `target_direct_blocking_authority=not_provided` and `resolved_files=0` into visible prose despite explicit soft teaching, and suggested cross-CPU affinity as a conditional drill-down without direct latency evidence. Do not prose-scan or rewrite; retain as model-facing-language observation. |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260821-003337 | answer_regex,answer_contains | none | 655s | 53 | read=35,repo_map=3,list=0,trace=0,source_lens=0 | midloop=22,inv=10/0,fin_reject=12,unavail=1,prune=1 | pass | Final answer is concise and materially correct: four-stage precedence diagram, complete five-column stage table, Orchestrator mechanism, BusContext/Mutable carriers, and grounded citations. B1282 received production positive evidence on the first and third rejects: the same retry exposed participant and relation deltas together. System quality is not closed: 12 finalize rejects/13 patch calls exposed B1283, where a live `prior_anchor` ref with body_occurrence=0 is told both to set body_occurrence because three visible TASK->DISP edges are ambiguous and to omit it because the ref selected zero. This is a deterministic contradictory contract, not model fluctuation; the model eventually escaped through a whole-block replacement, then removed stale participant boundaries. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

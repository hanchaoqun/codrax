# Selected Eval Manual Audit Scaffold

- date: 2026-08-11T14:36:07Z
- sweep_start_ts: 20260811-073606
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260811-073607 | answer_regex,answer_contains,mermaid_edge_count | none | 330s | 40 | read=8,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=4/0,fin_reject=2,unavail=0,prune=0 | fail | B537 production-positive but incomplete: after exact `ctx.Mutable.<operation>` rows, completion shrank the uncovered slate from `[BusContext Mutable]` to `[BusContext]`, proving exact operation-owner alignment works without minting an edge. The model then found BusContext initialization/`BuildAgentContext(o.busCtx,...)`, but did not materialize a value-bearing BusContext relation. Finalizer correctly rejected conceptual component arrows; the accepted diagram is four-stage precedence plus detached exact call/data-flow fragments and a disconnected BusContext. Prose still overstates that all agents receive Mutable/BusContext and that the shown graph is their data flow. Runner edge/token oracles are false-green. Active model work completed at 330s with no system-authored timeout answer. |
| 2 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260811-073607 | answer_regex,answer_contains | none | 904s | 47 | read=35,repo_map=3,list=0,trace=0,source_lens=0 | midloop=13,inv=10/0,fin_reject=3,unavail=1,prune=10 | fail | Stable multi-surface scope failure. The request asked for a stage sequence diagram plus a separate stage I/O/state-carrier table. Analyzer ignored the existing teaching and emitted table-only examples `BusContext`, `AnalysisIR`, and `AnswerDocument` as incident-required diagram participants; unanchored invented operation participants were dropped, but these three exact request identities survived. Explorer spent a second 32-round window chasing unrelated setters and aggregate-row repairs. Finalizer needed four drafts and ultimately produced the valid stage precedence spine plus many disconnected implementation fragments and unproven table-carrier nodes, not the requested concise sequence. Several table/prose claims are inaccurate (`runReadSchedulerLoop` does not run analyze; AnalysisIR/EvidenceItems are BusContext fields rather than Mutable-owned handles). The 904s active run still returned the model-authored answer; no four-minute/system-degraded answer was emitted. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

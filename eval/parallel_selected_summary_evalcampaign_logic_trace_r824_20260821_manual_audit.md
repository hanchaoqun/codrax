# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T19:58:49Z
- sweep_start_ts: 20260821-125847
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260821-125849 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 301s | 41 | read=0,repo_map=0,list=0,trace=8,source_lens=0 | midloop=1,inv=3/1,fin_reject=0,unavail=0,prune=0 | pass | Explicit 2.000..2.020s window, target-filtered typed queries, threadpool-400→network-300→cookie-200→app-100 wake chain, 11.000ms on-chain IO top seat, three separate 1.000ms runnable/priority candidates, CPU identities, actual-occupancy/eliminable dual ledgers, background isolation, automatic supplement, and full Trace causal projection all survived. No fixed-time degradation. The model's wording about the 11ms interval and direct blocking remains a soft precision issue; typed timing/authority stayed correct and the system did not rewrite it. |
| 1 | qf_logic_view_read_pipeline | FAIL | eval/results/qf_logic_view_read_pipeline-20260821-125849 | answer_regex,answer_contains,mermaid_edge_count | none | 1784s | 78 | read=61,repo_map=5,list=0,trace=0,source_lens=0 | midloop=44,inv=26/0,fin_reject=20,unavail=0,prune=1 | fail | System/process failure, not a runner-oracle or model-only fluctuation. Exploration expanded to 117 iterations, 61 reads, 480 evidence rows and repeatedly navigated Mutable to extract_work.go although the exact BusContext.Mutable→AgentContext.Mutable assignment is in internal/context/builder.go. Finalization then exposed a broad legacy patch surface beside a live ref lease; the model alternated stale candidate names, legacy match coordinates, whole-block replacement, and hidden identity rewrites, all rejected by the same typed lease. After 20 rejects the system shipped a previous draft that still contained relations the validators had explicitly rejected. B1307's exact optional-orphan shape did not occur, so it remains tests-positive/pending-production. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

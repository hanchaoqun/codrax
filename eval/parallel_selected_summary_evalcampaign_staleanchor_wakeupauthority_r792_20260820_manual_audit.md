# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T00:58:53Z
- sweep_start_ts: 20260820-175853
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | read_combo_pipeline_sequence_table | INCOMPLETE | eval/results/read_combo_pipeline_sequence_table-20260820-175853 | answer_regex,answer_contains | none | 317s | 43 | read=20,repo_map=3,list=0,trace=0,source_lens=0 | midloop=3,inv=4/0,fin_reject=0,unavail=0,prune=2 | inconclusive | The host execution session ended while explorer round 13 had a fresh model request in flight. The product had emitted 56 evidence rows but had not reached Extract/Finalize, no answer document existed, and B1268 was therefore not exercised. Earlier 529 retries increased latency but were not exhausted at termination. This is neither a 4ms/4m active-stream degradation nor a product verdict; rerun the exact pair with the host session actively polled. |
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-175853 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 212s | 40 | read=0,repo_map=0,list=0,trace=6,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | partial | Explicit 2.000..2.020s scope, typed trace queries, deterministic auto-supplement, Trace causal projection, and the proved threadpool-400 -> network-300 -> cookie-200 -> app-100 wake chain all ship. The 11.000ms on-chain IO-wait seat remains #1 and three independent 1.000ms scheduler-supply/priority candidates remain separate; sleep is state context and adjacent/background rows do not become roots. The r791 prose reversal and cross-CPU mechanism overclaim did not repeat, so B1269 remains an observed model fluctuation rather than a hard-gate target. One softer evidence-boundary drift remains: the model expanded the recorded kernel call-site name into an fscache subsystem description despite the typed call-site boundary; observe across heterogeneous traces before changing guidance. |

## Human Audit Checklist

- The incomplete read row has no product verdict and must not be counted as a regression or a B1268 production witness.
- Both streams were active while running; no elapsed 4ms, 4m, first-byte, stall, or cumulative-age policy emitted a degraded answer.
- Trace roots remain chain-only. Adjacent and aggregate background evidence remains support-only.

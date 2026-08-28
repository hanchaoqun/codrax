# Selected Eval Manual Audit Scaffold

- date: 2026-08-28T19:04:54Z
- sweep_start_ts: 20260828-120452
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260828-120454 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 217s | 39 | read=6,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Exact 34579.472865..34579.587805 window, ThreadPoolForeg→NetworkService→CookieMonsterCl→target chain, on-chain priority/scheduler/supply/D/IO candidates, VerifyClass business clue, actual-vs-eliminable dual ledger, background separation, and full Trace causal projection all survived. Frame causality remained honestly unproven. One mixed-language word (`strongest`) is model wording fluctuation, not authority drift. |
| 1 | qf_config_precedence | PASS | eval/results/qf_config_precedence-20260828-120454 | answer_regex,answer_contains | none | 251s | 27 | read=24,repo_map=1,list=0,trace=0,source_lens=1 | midloop=17,inv=8/2,fin_reject=1,unavail=0,prune=0 | partial | Core precedence answer and citations are correct. B1395 removed the false uncertainty-boundary and stale richness footer. B1394 did not prevent the first ownership completion downgrade: the actionable advisory sat after a 4.1KB evidence audit and the model repaired only after completion. Last-mile rendering also appended a fact-free requested-dimension source-quote block and converted resolved pre-completion telemetry into a false generic consistency caveat. B1396 remains: finalizer received 79 evidence rows, including 49+ unrelated shared same-file rows. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Confirmed gaps and disposition

- `B1397-POSTEMITADVISORYTAIL1/P1`: actionable typed advisory was behind the long per-item audit. Fixed by placing it before the audit while leaving evidence accepted and model-owned.
- `B1398-REQUESTDIMENSIONSOURCEQUOTEBACKFILL1/P1`: a visible-text coverage heuristic appended the user's own wording as a system block without supplying missing facts. The last-mile source-quote backfill is retired; precise typed structural retries remain.
- `B1399-RESOLVEDPREFLIGHTCONSISTENCYCAVEAT1/P1`: accepted runs materialized `ViolPreCompleteDowngrade`/`ViolSelfRefLiteral` history as final-answer inconsistency. These stay in operator telemetry and no longer publish on an accepted surface; genuine paired self-contradictions remain visible.
- `B1396-SAMEFILEFINALIZERCONTEXT1/P2`: still open. The visible supplement is fixed, but same-file finalizer context remains materially over-broad and should be solved by typed row selection, not keyword filtering.

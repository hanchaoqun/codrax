# Selected Eval Manual Audit Scaffold

- date: 2026-08-20T23:05:14Z
- sweep_start_ts: 20260820-160512
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-160514 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 175s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | B1260 production-positive: explicit 2.000..2.020s window, auto-supplement, four-hop wake chain, 11ms IO-wait primary seat, and three independent 1ms priority/scheduling seats all survive. B1264 display gap: cookie/network primary sleep hop rows still inherit candidate wording even though the separate ranked candidate correctly owns only 1ms. |
| 1 | read_combo_pipeline_sequence_table | PASS | eval/results/read_combo_pipeline_sequence_table-20260820-160514 | answer_regex,answer_contains | none | 767s | 51 | read=20,repo_map=2,list=0,trace=0,source_lens=0 | midloop=20,inv=6/0,fin_reject=12,unavail=0,prune=0 | fail | A usable document eventually ships, but the required table retains generic headers, some stage carriers are inaccurate, and the sequence diagram contains disconnected orchestration nodes. Twelve finalizer rejects expose B1265: model-selected, explicitly allowed precedence additions are lease-rejected before their canonical typed identities are mechanically restored; whole-block replacement passes only after the later recipe normalizer. B1262's old identity-only failure did not directly reproduce, so this run is not its production closure. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. `B1260-PRIORITYDIMENSIONDOMINANCE1` has positive production evidence. The Trace answer preserves the real blocking ruler and the independently eliminable scheduling ruler: `threadpool-400 / iowait = 11.000ms` remains rank #1, while cookie/network/threadpool each carry a separate `priority_inversion_candidate = 1.000ms`. The model also keeps the direct-blocker caveat and does not promote background IO pressure into the chain root.
2. `B1264-PRIMARYIMPACTROLELEAK1/P1` is confirmed. The raw `wakeup_causal_impact` observation for cookie/network still publishes candidate notes and summary after B1260 split the actual candidate into a separate root-rank seat. Projection therefore labels the 17ms/14ms sleep carrier as “优先级反转候选” while a sibling row correctly publishes the qualified 1ms candidate. This is a producer-side typed carrier-role leak, not model variance.
3. `B1265-ALLOWEDRELATIONIDENTITYORDER1/P0` is confirmed. The finalizer explicitly authored three visible precedence relations from the validator's allowed candidate map. Atomic additions omitted invisible canonical `from_identity/to_identity`, and the local repair lease rejected them as `unlisted_relation_added` before the typed-recipe normalizer could restore those exact fields. The same relations passed only through a later whole-block replacement where identity restoration ran before structural validation. The system neither needs nor may choose a relation; it may fill identity metadata only after an unambiguous model-authored edge matches one allowed typed recipe.
4. `B1262-IDENTITYONLYLEASE1` is not contradicted but does not receive direct production closure from this run: the old `typed_anchor_without_visible_edge`/empty-node failure did not recur. Keep its implementation/full-suite status and obtain a dedicated witness later.
5. `B1263-STAGEROLECONTEXTLEAK1` remains open. The read run collected 363 evidence records, used two explorer dispatches and 20 file reads, yet the final table still says `项目/列2/列3/列4/列5`, assigns `BusContext.Mutable.answerDocumentV2` to analyze, and describes extract with weakly grounded carriers. The final graph also leaves `dispatchStage`, `runReadSchedulerLoop`, `executeStageRequest`, and `AgentFinalizer` disconnected. This is evidence/context precision and relation-repair usability debt, not a reason for the system to rewrite the model answer.
6. Both streams remained active through completion. No 4ms, 4m, first-byte, stall, or cumulative-age fallback replaced either answer. Trace causal projection and deterministic supplementation remained present.

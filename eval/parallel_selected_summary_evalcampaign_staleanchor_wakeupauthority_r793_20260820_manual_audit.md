# Selected Eval Manual Audit Scaffold

- date: 2026-08-21T01:07:45Z
- sweep_start_ts: 20260820-180744
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | trace_query_wakeup_causal_io_chain | PASS | eval/results/trace_query_wakeup_causal_io_chain-20260820-180745 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 212s | 33 | read=0,repo_map=0,list=0,trace=1,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | Explicit 2.000..2.020s scope, typed query, deterministic supplement, Trace causal projection, and the proved threadpool-400 -> network-300 -> cookie-200 -> app-100 wake direction all ship. The 11.000ms on-chain IO wait remains #1; three independent 1.000ms scheduler-supply/priority candidates stay separate; sleep, adjacent, and aggregate background evidence do not become roots. However, for the second consecutive replay the model expands the recorded call-site name fscache_page_wait_on_page_bit into a specific cache subsystem/IO-completion mechanism and proposed file/cache remedies although resource identity and subsystem mechanism are explicitly not provided. The answer also leaks the typed status token complete in Chinese prose. The call-site boundary drift is now confirmed P1; fix context placement/reader-language facts, never scan or rewrite final prose. |
| 1 | read_combo_pipeline_sequence_table | FAIL | eval/results/read_combo_pipeline_sequence_table-20260820-180745 | answer_regex,answer_contains | none | 748s | 65 | read=11,repo_map=3,list=0,trace=0,source_lens=0 | midloop=19,inv=4/0,fin_reject=20,unavail=0,prune=0 | fail | The original B1268 contradiction is absent: removing a lease-listed typed_anchor_without_visible_edge no longer requires a body edge. The run instead exposes B1270: local relation repair makes the model repeatedly re-encode visible node ids, canonical identities, relation kind, and occurrence; a wrong selector receives only match did not select occurrence, with no stable failure reference or bounded exact prior-anchor roster. Nineteen patches/20 rejects then degrade to the old draft. The recovery table has semantic headers, but the answer still falsely says Orchestrator.dispatchStage uniformly dispatches all four stages (Explore uses its window dispatch path), and the sequence diagram presents those unproved calls/replies. B1266 transactional rollback remains correct; rejected patches do not advance the base. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Audit Notes

- B1268's exact executor behavior is no longer contradictory, but its production usability is only partial; B1270 should add a stable lease-owned failure reference that the model selects without retyping four coordinate systems.
- The r792 read child later produced an orphaned answer after its parent host session ended. It had one full-document rejection and one accepted table-only patch, but did not exercise B1268; keep the r792 runner row INCOMPLETE and treat the orphan answer as manual auxiliary evidence only.
- No stream was degraded because 4ms, 4m, first-byte, stall, or cumulative age elapsed while activity continued.

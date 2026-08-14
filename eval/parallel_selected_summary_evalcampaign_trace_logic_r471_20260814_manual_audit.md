# Selected Eval Manual Audit Scaffold

- date: 2026-08-14T06:51:09Z
- sweep_start_ts: 20260813-235108
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h4_supply_thermal_witness | FAIL | eval/results/real_trace_h4_supply_thermal_witness-20260813-235109 | log_regex,trace_attachment,principal_answer | perf_triage+trace_query | 143s | 32 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | CPU roster/running total exact and frequency target-binding verdict correct; four-state table merges Sleep with D-state and derives 70.338ms by subtraction because the finite finalizer context carries only the target_window_states ref, not its exact state-account values. |
| 2 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260813-235109 | answer_regex,answer_contains,mermaid_edge_count | none | 488s | 39 | read=17,repo_map=3,list=0,trace=0,source_lens=0 | midloop=13,inv=3/0,fin_reject=6,unavail=0,prune=1 | partial | B769 exact o.busCtx argument_flow reached evidence and the final diagram. Business-level BusContext/Mutable remain disconnected while prose claims shared cross-stage flow; six finalizer rejects oscillated over technical endpoints versus unproven participant boundaries. Mermaid is valid after one mechanical source repair. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human Findings

### 1. H4: exact trace fact existed, but the finite answer context did not materialize it

- Typed trace output contains one complete selected-window account:
  `running=157.248ms`, `runnable=5.604ms`, `sleep=70.338ms`, `d_state=0`,
  `io_wait=0`, `total=233.190ms`, plus the complete 8-CPU/90-segment running roster.
- B768's finite caliber hint did reach the Finalizer, so this is not a missing-teaching
  problem. The Finalizer's `Typed Repair And Evidence Handoff` exposed only the
  `target_window_states` record reference. `Trace Target-State Scope Authority` was absent:
  its old compiler required a causal-projection anchor, while a correctly bounded finite
  question deliberately has no root-cause board/anchor.
- The model therefore retained only running/runnable from the compact ledger and computed
  the rest as `window-running-runnable`. It wrote one combined `Sleep/D-state` row instead
  of the requested separate `Sleep=70.338ms, D=0ms`, and asserted the full remainder was
  non-CPU sleep. This is a deterministic context gap, not model fluctuation.
- Frequency reasoning is materially correct: CPU0/CPU4 policy ceilings are present and
  target-slice binding remains unproven. The runner's second frequency regex does not
  accept the semantically equivalent direct sentence; do not loosen that oracle until the
  substantive four-state gap is replayed.

### 2. QF: B769 production-positive, but participant-level relation closure is still partial

- After the Explorer emitted the grounded direct call at
  `internal/orchestrator/orchestrator.go:8026`, B769 caused the follow-up exact operation
  to be authored and accepted:
  `anchor_kind=argument, subject=o.busCtx, object=ctxbuilder.BuildAgentContext`.
  The final diagram preserved the same-direction `argument_flow` edge. This is production
  proof that the generic complete-argument discovery/obligation path works.
- The requested business graph is nevertheless incomplete. `BusContext` and `Mutable`
  remain disconnected `(unproven)` nodes, while prose says stages exchange state through
  them. The exact technical edge proves one call argument only; it does not by itself
  connect all six requested participants into one stage/data-flow graph.
- Finalization rejected six times. The first broad draft invented many unsupported arrows
  and was correctly rejected. Later patch rounds alternated between
  `missing_unproven_boundary` and `unproven_boundary_has_visible_incident_edge` for
  `Mutable`: the model repeatedly used the broad participant name as the endpoint of a
  local technical assignment, then also marked that participant unproven. The eventual
  accepted shape removes that edge and keeps the honest boundary, but costs excessive
  retries and leaves a prose/diagram gap.
- Register `B771-PARTICIPANTRELATIONAUTHORITYPARITY1/P1`: Explorer completion, typed
  candidate guidance, and Finalizer participant coverage need one shared distinction
  between (a) exact technical operation incidence and (b) a requested cross-participant
  graph. Repair guidance should expose the precise technical endpoint/group mapping once,
  not make the model infer it through alternating boundary errors. This must remain typed
  source evidence; no participant-name keyword gate and no system-drawn bridge.

### 3. JSON, Mermaid, active stream and authorship

- Neither case used malformed-JSON string recovery or degraded answer recovery;
  `answer_document_blocks_string_recovery_events=0` and
  `degraded_read_answer_check_skips=0`.
- QF records `mermaid_source_repair_applied=1` and rendered a legal final Mermaid graph.
  This is syntax self-heal only; it did not create semantic edges.
- Both runs remained active for 143s/488s and produced complete answers. No 4ms cumulative
  age degradation occurred or is authorized.
- System-generated audit notes did not replace the model answer or conclusion. H4 had a
  typed value cross-check supplement; QF had a requested-dimension reminder. Both remain
  display debt because they expose system wording, but neither changed the principal text.

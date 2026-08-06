# Selected Eval Manual Audit Scaffold

- date: 2026-08-06T17:54:44Z
- sweep_start_ts: 20260806-105443
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | qf_architecture | PASS | eval/results/qf_architecture-20260806-105444 | answer_regex,answer_contains | none | 117s | 28 | read=2,repo_map=3,list=0,trace=0,source_lens=1 | midloop=3,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Correctly enumerates all seven read-mode stages, distinguishes four unconditional core stages from three conditional pre-stages, explains agent/outputs and preserves citations. No Trace contract leakage, malformed JSON, or finalizer retry. |
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260806-105444 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 209s | 38 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=4,inv=1/0,fin_reject=3,unavail=0,prune=0 | fail | S15 relation JSON separation worked: closure is one-shot, no string recovery, and the final answer no longer promotes the upstream IRQ/IO row to the target's direct blocker. The answer still binds trace line 15620 at 34579.595130 (outside requested end 34579.587805) to this selected frame despite frame_evidence_status=absent, infers a Vsync cadence/completion story, sums two typed non-additive same-direction seats as about 43ms, and labels pressure density 5.26 as medium/not serious without absolute_level calibration. Finalizer also rejects three times because `causal_conclusion=unproven` and the separate `trace_causal_claim_caliber` enum are shown without an explicit mapping. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch finding

- `EVAL-B186-RELJSON1=resolved/r114`: all five Trace calls retained the exact requested window/PID, closure emitted once with zero rejection and zero aggregate string recovery. The canonical relation copy/diagnostic split removed the S15 JSON contradiction.
- `EVAL-B186-DIRECTREL1=resolved/r114`: the answer now says no typed direct blocker was established for the target and keeps the upstream IO lane outside that relationship. The direct-blocking typed boundary changed guidance only; it did not author the diagnosis.
- `EVAL-B186-CALIBERMAP1=P1/confirmed`: the first final emit copied the nearby evidence status `unproven` into the JSON caliber and was rejected; the next emit omitted the required field; the first patch chose `no_causal_conclusion` despite a candidate-ranking lead and was rejected; only the fourth round chose `bounded_window_candidate`. The dynamic tool schema, final typed boundary, and retry hint need one dispatch-local mapping and must explicitly say that evidence-status values are not JSON enum values.
- `EVAL-B186-WINDOWBLEED1=P1/confirmed`: an attached-artifact preview row beyond the typed selected-window end was treated as this frame's Vsync boundary and as proof of completion/cadence even though typed frame evidence was absent. General fix: mark all out-of-window preview/triage rows navigation-only for the selected-window conclusion unless a separate typed relation binds them into the projection.
- `EVAL-B186-PRESSCAL1=P1/confirmed`: `pressure_density=5.26` was measured, but no `absolute_level` was present. The answer nevertheless assigned “medium / not serious”. General fix: publish a compact negative authority when aggregate value/density lacks typed absolute-level calibration; retain the number as background evidence without forcing a qualitative conclusion.
- `EVAL-B186-RELSYNTH1=P1/repeated-observe`: the model still summed 23.994ms and 19.041ms despite the exact overlapping-members authority and final cross-row no-add rule. No prose hard gate will be added. Re-evaluate after the JSON/window context is simplified; if repeated, carry the exact relation decision at the final typed tail rather than adding output keyword scans.
- Preservation check: explicit-window Trace projection, deterministic supplement, target state partition, root ranking, wakeup path, actual occupancy, existing-rule eliminable axis and model-owned conclusion all remain present. Architecture control remains clean.

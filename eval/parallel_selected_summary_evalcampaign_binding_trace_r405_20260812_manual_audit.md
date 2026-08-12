# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T21:22:47Z
- sweep_start_ts: 20260812-142245
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260812-142247 | answer_regex | none | 148s | 23 | read=3,repo_map=2,list=0,trace=0,source_lens=1 | midloop=4,inv=2/0,fin_reject=1,unavail=0,prune=0 | partial | B669 schema-invalid binding teaching is production-positive: the corrected `registration/call` row at `core-rs/src/lib.rs:47` survives into the typed relation capsule, and the final sequence diagram carries the export binding as a non-call Note. B667 wrapper-to-core call and the Python native/fallback split also survive. One relation-completeness gap remains: the principal member_set calls itself a complete five-member chain and includes `best_merge`, but no typed `tokenize_bytes -> best_merge` call row was emitted even though the already-read body proves it at line 13. Completion currently lets node coverage stand in for edge coverage; the final diagram is therefore legal but disconnected before `best_merge`, and prose cites the helper definition line rather than the callsite. Treat as B672, not model-only fluctuation. |
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-142247 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 170s | 39 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | Explicit 13762.791708..13763.024898 window is preserved. The model separately explains actual occupancy/business spans and rule-eliminable on-chain roots: self running/compute supply, D-state `dma_fence_default_w`, priority inversion, scheduling supply and IO. Adjacent/background rows are expressly non-principal. Trace causal projection and deterministic supplement remain present; finalizer has zero rejects. Streaming requests remain under first-byte/byte-stall watchdog ownership and do not degrade on literal 4ms or cumulative age. B671 is production-closed. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch conclusion

- Runner: `2/2 PASS`; human: `1 pass / 1 partial`.
- `B669-REGISTEREDBRIDGEDIAGRAMCONTINUITY1=production-positive` through the
  schema-invalid repair lane. The newer B670 post-grounding repair lane remains
  covered by focused cross-language tests but was not the lane exercised here.
- `B671-TABLERETRYCANONICALSHAPE1=production-closed`: H7 produced its table in
  one Finalizer turn with no repeated contradictory convention teaching.
- `B672-PRINCIPALMEMBERSETEDGECOVERAGE1=open/P1`: a principal node set may
  currently close the call-chain interior while a parser-known, already-read
  relation between listed members is absent from typed evidence. The fix must
  use exact parser callsite tuples and read closure; member order, labels,
  notes, raw request text, model prose and final-answer prose have no direction
  authority.
- `B664-ACTIVESTREAMUPPERBUDGET1=production-reconfirmed-r405`: analyzer logs
  explicitly assign ownership to `stream_first_byte_and_byte_stall_watchdogs`.
  An active byte-producing stream must never fall back merely because 4ms,
  4s, 4min, or another cumulative-age threshold elapsed. Explicit caller
  cancellation/deadline, no-first-byte, byte stall, transport failure and
  decode failure retain termination authority.
- No system-authored answer or conclusion was observed. Trace explicit-window,
  causal projection, auto-supplement and typed on-chain-only root authority are
  unchanged.

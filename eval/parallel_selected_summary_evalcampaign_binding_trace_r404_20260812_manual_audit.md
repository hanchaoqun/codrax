# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T20:34:07Z
- sweep_start_ts: 20260812-133405
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260812-133407 | answer_regex | none | 192s | 24 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=6,inv=2/0,fin_reject=3,unavail=0,prune=0 | fail | The textual chain is mostly correct, but the optional diagram disappeared. Explorer had already read the exact Rust binding at `core-rs/src/lib.rs:47`; it submitted the nearby module definition at line 46 as a registration. Grounding truthfully downgraded it, then incorrectly printed `Current actionable repair targets: none`, so the load-bearing registration debt vanished. Finalizer later proposed the right Python and Rust call edges but could not prove the export binding and deleted the graph after three strict rejects. This is B670, not model-only fluctuation. |
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260812-133407 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 400s | 42 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=2,inv=1/0,fin_reject=2,unavail=0,prune=0 | pass | The explicit 233.190ms window, typed on-chain principal causes, separate adjacent/background lane, Trace causal projection, and automatic supplement all survive. The answer preserves both dimensions: actual occupancy/business spans and rule-based eliminable supply/D-state/priority/scheduling candidates. Two avoidable finalizer rejects came from table JSON teaching: after malformed cell strings were repaired, the model followed the advertised legacy label/text form and again produced seven visible values for six columns. This is B671. Active streams stayed under first-byte/byte-stall ownership and were never degraded by a fixed 4ms/age threshold. |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Batch verdict

- Runner: `2/2 PASS`; human: `1/2 PASS`.
- The Trace lane is a positive red-line witness: explicit-window causal
  projection and automatic supplementation remained present, principal causes
  came only from typed chain seats, and adjacent/background facts remained
  supporting context. The live 400-second run used streaming first-byte and
  byte-stall watchdogs; no 4ms or fixed-age fallback existed.
- The polyglot relation failure is producer-side. A strict call-edge gate was
  right to reject a registration as an invocation, but the evidence producer
  failed to turn its own exact nearby source observation into durable repair
  debt. Weakening diagram validation would hide the missing relation.

## B670 — post-grounding registration binding repair (`P1`, implemented)

Root cause:

1. B669 kept schema-invalid registrations actionable, but this production row
   was schema-valid and failed later in endpoint grounding. Its grounding note
   contained the generic `do not repair this item` marker, so the summary
   explicitly called it non-actionable even though line 47 had already been
   read and held the unique binding expression.
2. The downgrade carried no durable syntax obligation. A later unrelated
   successful `emit_evidence` could therefore let completion proceed while the
   requested cross-component relationship remained disconnected.

General repair:

- for a typed cross-component call-chain/registration request, a submitted
  registration whose endpoint fails grounding now searches only already-read
  source lines in the same enclosing callable and a bounded nearby window;
- a language-neutral balanced-call parser extracts the unique structural form
  `receiver.bindingCall(argumentContaining(endpoint))`. It recognizes nested
  wrappers/macros but does not know PyO3, JNI, plugin, ArkTS, Cangjie, or C++
  framework keywords;
- a unique match publishes a copy-ready action-required recipe containing exact
  source, line, `registration/call`, receiver, binding call, and complete bound
  argument. The system leaves the rejected row ungrounded and does not mint an
  edge; only a corrected model emit can satisfy the debt;
- the obligation persists across unrelated successful emits and is satisfied
  only by a citable `registration_edge` with the exact expression. Ambiguous or
  missing source shapes fail open without invention;
- focused tests cover Rust macro wrappers, ArkTS, Cangjie, C++, ambiguity-safe
  behavior through uniqueness, debt persistence, and successful model-owned
  re-emission.

Status: `B670-POSTGROUNDREGISTRATIONBINDINGREPAIR1=implemented/targeted+internal-tool-pass/pending-production-replay`.

## B671 — single-shape table retry teaching (`P1`, implemented)

The old validator accepted three historical table row conventions and repeated
all three in every failure message. In this run the model repaired malformed
cell strings, then interpreted the advertised legacy alternative as permission
to retain a full six-cell row plus `label` and `text`; the same 7-vs-6 failure
repeated.

The parser remains backward-compatible with all valid historical carriers, but
failure guidance now recommends one shape only: `label/text` empty and exactly
one `cells[]` value per declared column. When `cells[]` is already complete and
both auxiliary fields are present, the error identifies that exact shape and
instructs the model to keep cells unchanged, move prose to a non-table block,
and avoid rebuilding sibling rows. This is structural JSON guidance only; no
cell, conclusion, or answer prose is rewritten by the system.

Status: `B671-TABLERETRYCANONICALSHAPE1=implemented/targeted+internal-tool-pass/pending-production-replay`.

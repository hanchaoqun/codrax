# r396 manual audit — Trace full-spectrum + C write/apply

- Revision: `main@8746dc27b3d7`
- Runner: `2/2 PASS`
- Human: `1/2 PASS`
- Cases ran concurrently with `PARALLEL=2` from one immutable binary snapshot.

| Case | Runner | Human | Time | Manual verdict |
|---|---|---|---:|---|
| `patch_c_typo` | PASS | PASS | 117s | One-file/one-line `retrun -> return` patch, isolated worktree, project `make test`, 3/3 proof obligations and final strong proof all agree. No empty cumulative verification domain signed green. |
| `real_trace_h7_self_seat_full_spectrum` | PASS | FAIL | 184s | Explicit 233.190ms window, four typed trace queries, on-chain-only crown, wakeup chain, actual-occupancy/eliminable dual axes, compute supply, D/non-IO, business spans, deterministic JIT clue, adjacent/background separation and full causal projection all survived. The model nevertheless conflated independent observation domains: 11 scheduler D-state segments / 36.757ms, four rank-row merged members, and 12 blocked_reason records / Σ39.157ms became an invented “8 confirmed, 4 explicit, 4 omitted” account. It also overstates a policy ceiling as a thermal track and a lower-priority same-CPU candidate as proven preemption. |

## New generalized gap: B658-BLOCKEDREASONSTATEJOINLOSS1

`trace_query` already emitted the correct early relation authority:
`blocked_reason_census_relation ... value_caliber=kernel_record_count ...
state_relation_authority=census_alone_not_sufficient ...
typed_interval_join_required=true`. The Finalizer's compressed wait handoff retained
the census count/caller/Σdelay but dropped that relation boundary. The model therefore
received both exact rulers without the typed statement that they were unjoined.
The post-render system caveat could correct the report only after model composition;
it could not help the model form a coherent conclusion.

The fix copies the same typed relationship into the final decision boundary only when
the selected target-state account and blocked_reason census match on capture, selected
window, and subject. It states that scheduler state is sched_switch interval wall clock,
blocked_reason is kernel record count plus vendor-reported delay sum, the two domains are
unjoined, and count/duration differences cannot be interpreted without a separate typed
interval join. Cross-capture, cross-window, cross-subject, malformed count, and missing
caller census shapes fail closed.

This is soft model context. It does not inspect request/model/final prose, reject an
answer, rewrite model blocks, bind records to state occurrences, alter the Trace causal
projection, or change any root-cause value.

## Other observations

- Analyzer reasoning recognized `CompThread_0-2955` but emitted
  `runtime_target_profile=no_named_target`. The query phase still established the exact
  typed target and every final projection was correctly anchored. B658 deliberately
  consumes that selected typed target account instead of adding a raw-request identity
  hard gate. One sample remains model classification fluctuation, not authority for a
  prose scanner.
- The same-CPU/lower-priority and policy-ceiling evidence ceilings were present in the
  final prompt. The model's “preempted” and “thermal track” upgrades are repeated
  compliance residuals; do not solve them by scanning final wording or system-authored
  replacement conclusions.
- The report is large (about 130 KiB), but this case explicitly requests the full
  spectrum including small contributors. It is not evidence for globally suppressing
  projection detail.
- Active streams still have no fixed-age degradation path. Neither four minutes nor a
  literal `4ms` grants fallback authority; only caller cancellation/deadline,
  first-byte or byte-silence timeout, transport failure, or decode failure may end a
  stream.

Status: `B658=implemented/targeted-pass/pending-production-replay`;
`Trace explicit-window/causal projection/auto-supplement=unchanged`;
`principal-root-population=typed-on-chain-only`;
`adjacent/background=support-only`;
`system-answer-rewrite=none`; `raw-prose-hard-gate=none`.

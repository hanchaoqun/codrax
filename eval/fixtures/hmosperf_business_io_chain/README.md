# Business marker to response-impact IO chain

This is an authored synthetic trace, not a customer capture. It reuses the
request grammar and S-state completion-closed witness from
`internal/tracequery/block_io_wakeup_chain_test.go`. Only `events.systrace` is
attached; case/oracle documentation is outside the `stub_repo` analysis root.

Independent truth:

- `OpenDocument` belongs to header TID 100 and runs from 1.000 to 1.050 s:
  a 50 ms business response window, not proof of a 50 ms CPU execution.
- Main TID 100 sleeps at 1.001 and worker TID 200 wakes it at 1.045.
- Worker 200 issues sector 923339752, device 12,80, length 64 at 1.005.
  IRQ header TID 80 completes it at 1.040 and wakes that same worker at 1.040010.
  TGID 2 must not replace the IRQ's header TID 80.
- Issue-to-complete request residence is **35 ms**. The worker's S switch-out
  at 1.009010 to completion-closed wake at 1.040010 is **31 ms** of response-impact IO
  waiting. These are overlapping measurements and must not be added.
- Worker 200 is runnable from 1.040010 to 1.041010 (**1 ms**). Main 100 is runnable
  from 1.045 to 1.046 (**1 ms**). Do not merge either interval into IO wait.
- `LoadDocumentIndex` is a business investigation clue on the worker, not an
  independently proved code-level optimization or a required implementation.
- Background backup-900 issues sector 800000 at 1.002 and completes at 1.049
  (**47 ms**). There is no wakeup/blocked closure to the target; being longer
  does not make it this response's root cause.

The user question deliberately names no view or query order. Event discovery,
span window lookup, state/IO summaries and causal recipes/bundles can be
combined in different valid ways. Judge the grounded answer and scope, not a
prescribed tool count or fixed decomposition. This exercises multiple views
of the existing trace tool; it does not claim coverage of cross-tool source
inspection or of raw binary admission.

Human audit: inspect discovery-to-window continuity, source/TID identity,
on-chain IO membership, separate rulers, surviving business clue and final
Trace causal projection. Inspect the model context and first-answer retries;
do not mark PASS from numeric substrings alone. If a diagram is produced,
audit directions and relation evidence, not just Mermaid parsing.

# Independent oracle: document response IO sequence

This file is not model input. Only the existing synthetic `hmosperf_business_io_chain/events.systrace` is attached. The question asks an ordinary response diagnosis and sequence diagram: it does not name internal tools, forbid known mistakes, demand a rejection, or force the recovery lane. The source fixture's README is the independent evidence inventory.

## Observed sequence and distinct measurements

| Event or interval | Exact source truth |
|---|---|
| OpenDocument | TID 100; [1.000, 1.050) seconds, 50 ms response wall time |
| Main switches to sleep | TID 100 at 1.001 seconds |
| Document work marker | LoadDocumentIndex, header TID 200; [1.0045, 1.0445), 40 ms wall time |
| Read request issued | document-worker TID 200 at 1.005; device 12,80; sector 923339752; 64 sectors |
| That request completed | storage-irq header TID 80 at 1.040; same device/sector/request shape |
| Worker awakened | TID 80 → TID 200 at 1.040010 |
| Worker scheduled | TID 200 at 1.041010, 1 ms after wakeup |
| Main awakened | TID 200 → TID 100 at 1.045 |
| Main scheduled | TID 100 at 1.046, 1 ms after wakeup |
| Response marker ends | TID 100 at 1.050 |

The read request resides for 35 ms from issue to completion. Worker TID 200's completion-closed sleeping wait runs from 1.009010 to 1.040010: 31 ms of response-impact IO waiting. These overlap; neither their sum nor the business-marker wall time is measured CPU execution. The main thread's [1.001, 1.045) sleep is its dependency wait, not an independently identified CPU or storage cause. TID 80 is the storage completion/waker identity; its header TGID 2 must not replace it.

The causal chain is storage completion/wakeup → document-worker 200 → app-main 100. Actual request issue/completion pairing is distinct from thread wakeup edges; temporal order alone must not turn the marker, a stage, or an arbitrary background request into a communication or dependency edge. The graph may include separate chronological annotations for measured steps. It must preserve the two real wake directions and show actual scheduling times separately from wakeup times.

## Competing background evidence and interpretation

Backup TID 900 issues a different write request at 1.002 and its IRQ TID 81 completes it at 1.049, lasting 47 ms. It has no target-linked wakeup or blocked-interval closure. Its greater duration is background, not evidence that it caused OpenDocument's delay, and it must not enter the causal diagram as a dependency to 100 or 200.

LoadDocumentIndex is an on-chain business clue for where to investigate document-index loading and its IO wait. Its name does not prove a particular source implementation, cache miss, lock, disk fault, optimization or hardware bottleneck. Any upstream scheduling/priority/capacity diagnosis must retain the tool's actual chain and state evidence rather than infer it from names. The most defensible priority is to investigate the measured document-worker IO wait and its business operation, with the two small runnable delays reported under their own measure.

## Graph and recovery audit

Audit actual query parameters/results, the finalizer's source-bound event/timeline facts, the final accepted document, and rendered diagrams. Do not accept a participant-only diagram or reverse 100 → 200 / 200 → 80 edges merely because the picture parses. Permit any valid sequence representation that clearly expresses the observed relations and distinguishes request residence, sleeping wait, wake-to-run and running points. The existing deterministic Trace causal projection must survive.

Normal successful emission is a valid positive result: no recovery is required. If the run naturally reaches recovery, inspect the shipped recovered document and all attachment routes. A relation graph not successfully checked against the current evidence must not reappear as a normal authoritative diagram via a retry-state, text-recovery or attachment copy. A truthful boundary notice plus surviving native facts is safer recovery, but absence of the requested valid diagram remains an unmet user requirement and the case is not a complete capability PASS. Conversely, unrelated rejection must not quarantine an independently verified good diagram.

Preserve all rejected drafts and diagnostics. Machine successful-tool and projection checks are only smoke checks, not a semantic graph verdict. Manual review is mandatory; prior failed evals do not close just because this new ordinary task succeeds.

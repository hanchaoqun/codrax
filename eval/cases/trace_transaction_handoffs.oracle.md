# Independent oracle: rendering transaction handoffs

Not supplied to the model; the natural question does not teach implementation safeguards.

- Requested window [1,1.05) seconds. The protocol is MarshRSTransactionData transactionFlag:[tid,seq] followed by ProcessCommandUni key lists, not neighboring spans or inferred frame IDs.
- Application recorded process 100 contains observed emitter TIDs101/102; service recorded process200 emits on TID201. Marker payload PID is not proof of emitter identity. CPU execution, waits and frame completion are not requested or established.
- Key[101,7] has submission0.999 outside window and consumption1.020 inside. Key[102,8] has submission1.003 and the SAME consumption1.020. Preserve two independent submission branches into one consumption; don't serialize the submissions or invent a call from101 to102. In-window counts for these two keys differ; the service event counts once, not twice.
- Key[101,9] has submissions1.004 and1.060, plus consumption1.025. The duplicate outside the query window is in the same published-source uniqueness universe: ambiguous, do not pick the first or nearest submission.
- Key[101,10] has submission1.010 but no observed consumption. Key[103,11] has consumption1.030 but no observed submission. Missing observations are not proof of dropped transaction or thread blocking.
- Key[101,12] has submission1.040 and consumption exactly1.050. The latter is outside right-open window and may be shown only as association context, not counted as a window consumption.
- Key[999,13] is emitted by TID101 at1.012, not999, and consumed1.035. Keep both observed protocol rows but mark identity unverified; no sender999 node as a proven emitting thread.
- Total selected keys7; in-window submission events5, consumption events4. Observed-unique protocol matches3 ([101,7],[102,8],[101,12]); ambiguous1; missing consumption1; missing submission1; identity-unverified1. Native result totals are derived before display trimming. A count of key-consumptions is not the count of physical consumption events.
- Output can be concise. Diagram should show only proven protocol-key matches as solid submission→consumption, branch correctly, label outside-window endpoints. Unmatched/ambiguous rows may be a separate table or clearly non-causal annotations. No invented completion, wakeup, frame root cause, GPU or timing thresholds.
- Text and default SQLite tests verify the native view. Runtime answer acceptance requires inspecting actual calls, output, actual finalizer handoff and diagram. Machine PASS alone is not acceptance. The word unique is scoped to published observations, not every original database row or OS thread incarnation.

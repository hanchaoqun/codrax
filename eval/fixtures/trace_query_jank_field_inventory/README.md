# Synthetic jank marker field inventory

This is authored synthetic data, not a modified customer trace. The eval
attaches only `events.systrace`; its repository is the separate `stub_repo`.
This README and the `.case` oracle are not analysis inputs.

## Independent truth

The exact marker grammar is `B|201|jank_event_sync: ...`. Header TID 101,
marker PID 201 and payload appid 620/621 deliberately differ. Native payload
timestamps are integer nanoseconds above 2^53. No header/native clock mapping
or application-to-scheduler-identity mapping is supplied.

For appid 620 and jank_frames >= 2, the valid numeric match set has **3 rows**.
The descending jank_frames order is:

| Trace line | Header time (s) | jank_frames | Native start (ns) | Native end (ns) | Reported end-start |
|---:|---:|---:|---:|---:|---:|
| 9 | 5.040000 | 7 | 9007199255740993 | 9007199325740993 | 70000000 ns = 70 ms |
| 11 | 5.050000 | 4 | 9007199256740993 | 9007199296740993 | 40000000 ns = 40 ms |
| 3 | 5.010000 | 2 | 9007199254740993 | 9007199274740993 | 20000000 ns = 20 ms |

Line 5 is below threshold; line 7 has another appid; line 13 belongs to another
marker despite containing the same text; line 15 has invalid numeric metadata
and must not acquire a fabricated zero. The accepted match count is therefore
three valid matching records, not a claim that every malformed record's
underlying real-world condition has been disproved.

Every B row closes with an E row 10 microseconds later. Those header-clock
marker lifetimes are intentionally different from the producer-reported
payload durations. Neither may be silently substituted for the other.

## Human audit beyond machine checks

- Check exact selected membership, order, total and row-associated durations.
- Inspect tool inputs/results and finalizer context: native integers remain
  exact; malformed/unknown metadata stays disclosed instead of becoming zero.
- Do not require one particular query decomposition or force API parameters
  through the user's natural-language question. Observe use of numeric
  predicates, but judge the answer on grounded facts as well.
- Payload durations are reported marker metadata, not measured scheduler
  waits, frame interval proof, CPU demand or independently proved root causes.
- Header time is occurrence time; native start/end cannot directly select
  scheduler windows without a proven clock map. appid is not a proven TID.
- No scheduler or wakeup evidence is present. Do not mint a causal chain or
  crown a frame root cause from these records alone.
- Internal enum repetition is not success. Boundary explanations should use
  ordinary user-facing language; machine output-text checks are only a floor.

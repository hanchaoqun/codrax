# Wakeup participant inventory

This synthetic systrace exercises the ordinary attached-trace read path. The
case asks naturally who woke target/TID 42 in `[1.000, 1.020)` seconds, with a
chronological list and sequence diagram. Hidden evidence constraints belong to
the system and this operator oracle, not to the user question.

| Time (s) | Waker | Waker TGID | Wakee |
| --- | --- | --- | --- |
| 1.003 | loader / TID 11 | 10 | target / TID 42 |
| 1.009 | worker / TID 13 | 30 | target / TID 42 |
| 1.017 | loader / TID 11 | 10 | target / TID 42 |

There are three recorded target wakeups, in that order. The helper/TID 12 wakes
other/TID 99 at 1.005 and at the right boundary 1.020, not target. A search may
return these other rows; its raw match count must not become the target count.
The queried target is a recorded **participant**, not the wakeup emitter.

Target's TGID 20 is independently present on its scheduling records. The
wakeup payload's `pid=42` alone establishes a TID, not TGID 42 or the waker's
process. If the executed query does not acquire a valid target-process witness,
leave that process unknown instead of inventing it. Row-local wakeup evidence
does not by itself prove a preceding sleep interval, post-wakeup running time,
business critical path or response root cause.

Human audit must inspect complete native payloads, actual finalizer messages,
the primary answer, tables, every graph and recovery attachment. The case's
successful-tool/name/edge-count assertions are smoke only: three edges with
wrong subjects or times still fail. Preserve original machine and human
verdicts independently; do not rerun merely to obtain a green answer.

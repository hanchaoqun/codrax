# Independent oracle: source-owned windows and wakeup timelines

This oracle is not model input. The question contains only two source/window selections, TID 41, CPU state/frequency, and a request for a sequence diagram. The two small systraces are synthetic and redistributable; they are not evidence of raw-binary parity or a real-device performance regression.

## Required source-local facts

| Fact | Baseline: baseline.systrace [1.000,1.010) s | Current: current.systrace [4.000,4.020) s |
|---|---|---|
| Target | target, TID 41 | target, TID 41 |
| Wake event | wakerA, TID 201 → 41 at 1.002 s | wakerB, TID 301 → 41 at 4.004 s |
| Wake-to-run | 1.002 → 1.006 s = 4 ms | 4.004 → 4.012 s = 8 ms |
| Running in window | [1,1.001), [1.006,1.009): 4 ms | [4,4.002), [4.012,4.018): 8 ms |
| Runnable in window | [1.002,1.006): 4 ms | [4.004,4.012): 8 ms |
| Sleeping contribution in window | [1.001,1.002), [1.009,1.010): 2 ms | [4.002,4.004), [4.018,4.020): 4 ms |
| Window wall time | 10 ms | 20 ms |
| CPU | CPU 0 only | CPU 0 only |
| CPU frequency transitions | 1 GHz at window entry; 2 GHz at 1.005 s | 1.5 GHz at entry; 750 MHz at 4.010 s |
| CPU idle transitions | active → idle state 0 at 1.001; active at 1.006; idle state 0 at 1.009 | active → idle state 0 at 4.002; active at 4.012; idle state 0 at 4.018 |

Wake-to-run latency is not sleep-before-wakeup: those preceding sleeps are respectively 1 ms and 2 ms. The first running segments begin at 0.999 s and 3.999 s in their source files; only their overlap belongs in these window totals. All queried wake-to-run and running intervals have real closing scheduling events. The last sleep contributions are window-accounted states, not observed sleep-ending timestamps or proof that either capture is complete.

The second wait is 4 ms longer (twice the first) as a direct comparison of two measured durations. Both windows have 40% running, 40% runnable and 20% sleeping contribution for this target. That normalization does not establish same workload, hardware or response-time comparability. A CPU-frequency change is an observed timeline fact, not proof that it caused the wait difference. The waker threads lack their own scheduler histories here; their names and wake edges alone do not establish an upstream compute/priority/IO cause.

## Independent CPU state × frequency arithmetic

Intersect the transition intervals in each physical file with that file's requested window. Each side has one observed CPU and complete known joint coverage within this selected observation window.

| State × frequency | Baseline duration / window share | Current duration / window share |
|---|---|---|
| active × initial frequency | 1 ms / 10% at 1 GHz | 2 ms / 10% at 1.5 GHz |
| idle state 0 × initial frequency | 4 ms / 40% at 1 GHz | 8 ms / 40% at 1.5 GHz |
| idle state 0 × later frequency | 2 ms / 20% at 2 GHz | 4 ms / 20% at 750 MHz |
| active × later frequency | 3 ms / 30% at 2 GHz | 6 ms / 30% at 750 MHz |

Known joint CPU-time is 10 and 20 CPU-ms; unknown joint CPU-time is 0 for these selected intervals. `cpu_idle state=4294967295` is the ftrace exit-idle/active sentinel; state 0 is the recorded idle state, not an invented device C-state name. Raw frequencies are kHz (1000000, 2000000, 1500000, 750000). CPU active residency and TID running happen to match in this authored fixture; neither metric is generally interchangeable with the other.

## Opposite-window interference and diagram audit

Both files also contain real, parseable activity at the other source's requested numeric window, with the **same target TID 41**:

- Baseline queried incorrectly at [4,4.020) has `otherA-701 → 41` at 4.002 s, wake-to-run 1 ms, running 18 ms and CPU 3 GHz throughout (90% active, 10% idle).
- Current queried incorrectly at [1,1.010) has `otherB-801 → 41` at 1.002 s, wake-to-run 1 ms, running 8 ms and CPU 4 GHz throughout (80% active, 20% idle).

These are neither unparseable noise nor malformed rows. Their correct status for the natural question is outside that source's requested window. They must not enter requested native tables, causal projections, auto-completed diagram edges or prose merely because their times fit the pooled list of both windows. Valid out-of-window closing witnesses for an in-window state remain permissible; do not delete them by raw timestamp and break Trace projection.

The accepted graph must show two source-local sequences (two diagrams or clearly separated source panels), with genuine directed wake edges 201 → 41 and 301 → 41, followed by the corresponding run times. Equal TID/CPU numbers across captures do not merge their actors, align their clocks or authorize cross-source arrows. A timeline with only participant declarations and notes, no observed wake edges, does not satisfy this question's requested interaction. Check diagram syntax and rendered layout, including multiline notes, separately from factual correctness; do not demand one particular valid diagram grammar.

## Deterministic confirmation and live acceptance

Before live evaluation, a temporary Go fixture probe validated every request against the published `TraceQuery.Parameters()` and called the real `TraceQuery.Execute` for all four source/window combinations with `cpu_state_frequency`, `window_stats`, `thread_timeline` and `wakeup_chain`. These returned the values above, including all four distinct wakers and both opposite-window interference populations. `scheduler_concurrency` is an internal view/output name; the schema-valid public query used here is `window_stats`.

For live acceptance, audit the accepted analyzer's per-source binding, actual query parameters/results, handoff context, final accepted structured answer and rendered diagrams. Both native CPU and scheduler facts must remain attached to their original source/window; generic successful calls or an accurate table beside a false diagram are insufficient. Do not close old manually failed cases solely because this fixture probe passes. No source-file mutation is permitted.

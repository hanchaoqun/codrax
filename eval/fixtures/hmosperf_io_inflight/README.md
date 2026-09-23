# Complete-pair IO in-flight fixture

Authored text fixture for a selected half-open window `[1.000000, 1.010000)` seconds. It does not exercise binary conversion. `expected.json` records the arithmetic for manual row-to-group verification; smoke oracles do not replace that audit.

The RQ / device `8,0` / read group has four unambiguous complete intervals: `[0.998, 1.004)`, `[1.002, 1.006)`, `[1.006, 1.010)`, and `[1.008, 1.012)`. Clipping occupancy to the selected window gives peak 2 requests, time-weighted mean 1.4 requests, busy time 10 ms, and depth-time area 14 request·ms. The completion and new issue at 1.006 must not create a transient depth of 2. Carry-in and carry-out remain part of their original complete requests; they do not change the selected window.

The same group also contains two overlapping starts with the same pairing key, followed by two completions, plus one unfinished request. Those three requests cannot be assigned complete lifetimes by guessing. The six in-window issue events are arrival inventory, not peak or mean depth. Pairing coverage must disclose the unfinished and ambiguous records; qualified-pair arithmetic does not prove complete capture.

Three separate populations exercise write operations, another device, and the BIO endpoint family. There is no cross-layer mapping. No scheduler or wakeup evidence is present, so these measurements cannot establish issuer blocking, a user-response dependency, hardware queue occupancy, or a root cause.

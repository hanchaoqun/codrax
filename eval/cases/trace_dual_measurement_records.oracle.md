# Independent oracle: two capture measurement records

This file is not an attachment or model input. The natural question is in the adjacent case. It asks for two independent raw measurement inventories, not HMC-10.2 numeric deltas or causal comparison.

## Inputs and scope

- `baseline.data` is generated from `eval/fixtures/hmosperf_measurements/capture.sql`; its bytes match that existing closed synthetic fixture. Requested window: [1, 2) seconds. Capture range: [0, 3) seconds.
- `current.data` is generated from `eval/fixtures/hmosperf_dual_measurements/current.sql`. Requested window: [4, 4.5) seconds. Capture range: [3, 6) seconds.
- Both source files must remain unchanged. Keep each source identity and generation distinct, including filters with matching IDs and names. Different paths with identical baseline bytes do not authorize treating the second capture as the first or importing the other window.
- Device, workload, units, hardware resource identities, state encoding and shared clock alignment are not established. They must not be inferred from filenames, sequence names, matching filter IDs or matching source reference integers. Unknown metadata does not prevent listing the recorded facts.
- Requested windows are independently 1 second and 0.5 seconds. Neither is the full capture range, and neither establishes continuous measurement coverage. No rate or difference is requested.

## Baseline [1, 2)

The complete row-level oracle is `eval/fixtures/hmosperf_measurements/README.md`: 15 input rows, 13 selected rows, 7 groups (5 uniquely resolved filters and 2 unresolved records). Row 5 begins at the excluded right edge; row 14 has no timestamp and cannot be assigned to the window. All 13 selected original values, storage types and intervals must remain visible. Carry-in row 1 retains its original [0.9, 1.2) interval and selected [1, 1.2) portion. NULL and negative durations do not extend rows 4 or 15 to the window end. The first value is exactly 334200000.5; equivalent scientific notation is not a precision failure.

## Current [4, 4.5)

10 input rows, 8 selected rows, 4 distinct series. Row 8 starts at the excluded right edge; row 9 has no timestamp and is unpositioned. Source file order need not be chronological.

| Physical row | Filter / sequence | Raw value / storage | Original interval (seconds) | Selected interval |
|---|---|---|---|---|
| 1 | 10 / gpufreq | 0 / INTEGER | [3.9, 4.1) | [4, 4.1) |
| 2 | 10 / gpufreq | NULL | [4.1, 4.2) | unchanged; unknown value |
| 3 | 10 / gpufreq | 250000000.25 / REAL | starts 4.2; duration NULL | timestamped observation; end unknown |
| 4 | 11 / gpu_state | "0" / TEXT | [4.25, 4.3) | unchanged; not numeric zero |
| 5 | 11 / gpu_state | 1 / INTEGER | [4.05, 4.15) | unchanged; state semantics unknown |
| 6 | 20 / gpufreq | 750000000 / INTEGER | [4.1, 4.3) | unchanged; separate same-name series |
| 7 | 30 / vendor.sample | 9007199254740995 / INTEGER | point at 4.4; duration 0 | exact integer and valid zero duration |
| 10 | 10 / gpufreq | 500000000 / INTEGER | starts 4.45; duration -1 ns | timestamped observation; invalid duration, end unknown |

The two gpufreq filters are separate within each capture, and all same-name series remain separate across captures. Numeric zero, NULL, TEXT "0", the exact large integer and unknown endpoints must not collapse. GPU-like names are source labels, not verified units, active-frequency semantics or hardware bindings. Independent source-local ordering is allowed; a shared timeline or performance improvement conclusion is not established.

## Acceptance paths

Audit the actual query parameters and per-side published facts, the finalizer context, accepted structured answer and final rendered text. The current case log matcher proves only a tool-call attempt; even a pre-execution rejection can match. A machine PASS therefore does not prove a successful query or complete capability. Verify successful execution and per-side native publications from the actual results. Facts shown by a system-owned native table and model-owned explanations are audited separately; explanations cannot contradict the facts or add unverified identity/units.

Single-side failure, both-side failure, cancellation, source-generation replacement, missing-vs-zero, stale selectors and independent unit/coverage metadata are deterministic public-path boundary tests. They are not additional live cases and must not be packed into the user question. HMC-10.1 closes only when its complete original exit matrix is verified; even a successful pair alone cannot close it.

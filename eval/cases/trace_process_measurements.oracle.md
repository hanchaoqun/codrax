# Independent oracle: native process measurements

This file is not an attachment or model input. Natural question is in the case.

- Input: closed, synthetic SQLite generated from the adjacent fixture SQL; source must remain unchanged. No source-code work is requested.
- Requested range is [1, 2) seconds, not the attachment extent. `process_measurements` must be queried; a C-counter-only inventory cannot answer the interval question.
- Owners app.alpha/PID100 and app.beta/PID200 are distinct even when metric names match. No observation establishes an emitting TID, CPU0, execution state, dependency or cause.
- Alpha Resident: raw intervals [0.9,1.1) value1000 (clipped to [1,1.1)); [1.1,1.3)1400; [1.3,1.5)NULL; [1.7,1.9)1800. [1.6,1.7) contains TEXT `unavailable`, not a known numeric value. No interpolation/fill of the gaps; the 9900 row at2 is outside.
- Beta Resident: [1,1.25)2000, [1.25,1.5)1900, [1.5,1.75)2100, [1.75,2)2200. These source values may be described over time, but source protocols do not establish units or stock/delta semantics; do not claim a byte growth comparison or memory leak.
- Alpha Page faults has a real zero at1.05 with dur0. Allocation sample at1.2 has exact integer9007199254740993 and NULL duration: timestamped observation, not a sustained interval. NULL is never a measured zero.
- Twelve rows have positionable starts, one starts at the excluded right edge; a thirteenth row has NULL timestamp and cannot be assigned to the requested interval. The query exposes eleven selected rows and one unpositioned row separately. Record counts are not capture completeness or unique-process counts.
- Answer need not recite all guardrails or every internal field. It should convey actual owner-specific changes, limitations and useful follow-up without fabricated units, causal ranking, continuous coverage or extrapolation. Verify typed tables, source/window and exact large integers in the actual finalizer context and final answer, not only tool exit status.

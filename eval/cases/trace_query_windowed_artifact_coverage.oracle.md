# Independent oracle: two-window scheduler source coverage

This file is operator-only, not model input. The question asks a normal table comparison of two named threads and two time ranges. It names no tool, internal field, required call order, expected number, tail correction or prescribed conclusion. This new variant does not rewrite or replace the original OpenDocument causal-diagram case or its failed records.

Source: existing authored exported-text `eval/fixtures/hmosperf_business_io_chain/events.systrace`, 2312 bytes, SHA256 `62a95ccfba79994e192e6328f23b492f7f0510f6916b20f6170028c9f81cce70`. It is not a new device capture. Complete physical source timestamp extent is 0.999000–1.051000 seconds (lines 3 and 20), regardless of which query first opens or caches the source. A filtered event preview is not the complete source extent. The oracle uses the original text, not product-query output.

## Independently witnessed intervals

| Thread | State | Source interval, seconds | Evidence |
|---|---|---|---|
| app-main-100 | running | [0.999000,1.001000) | lines 3–5 |
| app-main-100 | sleep | [1.001000,1.045000) | lines 5–15 |
| app-main-100 | runnable | [1.045000,1.046000) | lines 15–17 |
| app-main-100 | running | [1.046000,1.051000) | lines 17–20 |
| document-worker-200 | running | [1.004000,1.009010) | lines 7–10 |
| document-worker-200 | sleep | [1.009010,1.040010) | lines 10–12; wakeup closes the interval |
| document-worker-200 | runnable | [1.040010,1.041010) | lines 12–13 |
| document-worker-200 | running | [1.041010,1.045500) | lines 13–16 |
| document-worker-200 | open sleep | [1.045500,1.051000) as source-bounded observation | line 16 starts sleep; source ends at line 20, not a worker closure |

The worker has no governing state before 1.004000. The main has no governing state before 0.999000. Neither thread has measured positive-duration coverage after the physical source end 1.051000. A main switch at that endpoint is an observed state-change point, not permission to extend a measured sleep to 1.055000. Unknown coverage is not a measured idle/sleep/running interval.

## Requested-window totals

All totals are milliseconds. Intersect witnessed intervals with each query separately; do not add the two overlapping query populations.

| Query | Thread | Running | Runnable | Sleep | Known state total | Unmeasured part of requested window |
|---|---|---:|---:|---:|---:|---:|
| [0.990,1.055), 65ms | app-main-100 | 7 | 1 | 44 | 52 | 13 (9 before observed head, 4 after source end) |
| [0.990,1.055), 65ms | document-worker-200 | 9.5 | 1 | 36.5 | 47 | 18 (14 before first governing state, 4 after source end) |
| [1.030,1.050), 20ms | app-main-100 | 4 | 1 | 15 | 20 | 0 within this window |
| [1.030,1.050), 20ms | document-worker-200 | 4.49 | 1 | 14.51 | 20 | 0 within this window |

Worker wide-window sleep comprises 31ms closure-witnessed sleep plus 5.5ms source-bounded open tail. Narrow-window sleep comprises 10.01ms contribution from the same closure-witnessed interval plus 4.5ms contribution from the open tail. A display may preserve the full 31ms interval as an independently labeled physical measure; it must not replace the narrow contribution with 31ms. Narrow running is [1.041010,1.045500), 4.49ms. Main narrow sleep is [1.030,1.045), 15ms, followed by 1ms runnable and 4ms running.

## Audit boundaries

Inspect actual tool arguments, complete producer payloads, their per-source/selected-window scopes, finalizer INIT, accepted answer and table/HTML source. Distinguish complete-source extent from selected-event bounds, state closure from a window/source observation endpoint, and preview from full populations. Completion at 1.040000, worker wakeup at 1.040010 and worker dispatch at 1.041010 are different observations if mentioned. OpenDocument business time is 50ms; it is not either requested query duration.

Any valid query view/sequence is allowed; no specific tool order, root-cause rank, diagram, forced retry, warmed-cache order, explicit metadata field or extra binary path is an exit condition. If a cold/warm or source-window branch does not occur naturally, record NOT_TRIGGERED rather than manufacturing coverage. Positive known-window accounting does not prove complete device capture, source implementation, all-machine CPU idleness/saturation, hardware causality or a synchronously blocking business call. Machine checks only confirm minimal tool smoke; manual audit owns identity, numerical and scope correctness without imposing a particular thread-name/TID spelling. Do not add oracle teaching to the user question or model fixture.

# Independent oracle: unbounded comparison with an unavailable side

This document is not an attachment or model input. The adjacent question only names two sources and asks for their original measurement records. It does not supply operational guardrails, a tool recipe, or a fabricated time window.

## Physical inputs and acquisition

- Reuse the unchanged `hmosperf_dual_measurements/empty.htrace` and `current.data`. The former is zero bytes and must have an actual input/preparation failure, not an executed empty measurement population. The latter is the TraceStreamer-shaped SQLite fixture defined by `current.sql`.
- No time window is requested. The system must recognize two independent named inputs, attempt them independently, and retain the healthy side despite the empty baseline. Neither source discovery nor a comparison-wrapper success flag proves that the healthy source was queried.
- Do not inherit the historical case's [1,2) or [4,4.5) windows, require the user to invent windows, or interpret baseline failure as zero. A native pair call and independently admitted single calls are both acceptable if source roles and actual statuses survive handoff.
- Match each current private preparation/query receipt to the original input and generation. Prepared paths, SQL source files, parent directories and nearby captures are not additional requested inputs. Input bytes remain unchanged.

## Source facts, independently checkable with SQLite

The `measure` table contains 10 physical rows in 4 distinct series: filter 10 / gpufreq (6 rows, including the untimed record), filter 11 / gpu_state (2), filter 20 / gpufreq (1), filter 30 / vendor.sample (1). Matching names do not merge filters 10 and 20. The corresponding series in the timed measurement table have 5, 2, 1 and 1 rows respectively.

| Row | Filter | Raw value / storage | Original start (s) | Original duration |
|---|---|---|---|---|
| 1 | 10 | 0 / INTEGER | 3.9 | 200 ms; ends 4.1 |
| 2 | 10 | NULL | 4.1 | 100 ms; ends 4.2 |
| 3 | 10 | 250000000.25 / REAL | 4.2 | NULL; end unknown |
| 4 | 11 | "0" / TEXT | 4.25 | 50 ms; ends 4.3 |
| 5 | 11 | 1 / INTEGER | 4.05 | 100 ms; ends 4.15 |
| 6 | 20 | 750000000 / INTEGER | 4.1 | 200 ms; ends 4.3 |
| 7 | 30 | 9007199254740995 / INTEGER | 4.4 | 0 ns; point observation |
| 8 | 10 | 999000000 / INTEGER | 4.5 | 50 ms; ends 4.55 |
| 9 | 10 | 55 / INTEGER | unknown: SQL NULL | 50 ms; no placeable endpoint |
| 10 | 10 | 500000000 / INTEGER | 4.45 | -1 ns; invalid duration, end unknown |

The table is obtained directly from `SELECT rowid, filter_id, typeof(value), quote(value), typeof(ts), quote(ts), typeof(dur), quote(dur) FROM measure ORDER BY rowid` using `sqlite3 -readonly` on the existing fixture. No producer code is needed to derive these original facts.

## Actual producer behavior and answer audit

The deterministic public-tool probe on 2026-10-11 validated the published JSON schema and called the real default preparer and `trace_query`:

- `view=measurements` with no time selectors returns 9 navigable, timed rows, `omitted=0`, plus one explicitly unpositioned record. It does **not** put row 9 into its timed table. All original values and durations of those 9 rows remain available.
- The producer's default sorting-coordinate bounds are 0 through 4.5 seconds, with the endpoint included for the final observation. They are not a user window, a verified capture-completeness claim, or the stored `trace_range` [3,6) metadata. Row 9's internal sorting coordinate 0 is not its timestamp.
- Row 8 is retained despite being absent from the older explicit [4,4.5) case. Its original 50 ms duration remains 50 ms; the default selection's clipped [4.5,4.5] representation does not make the original duration zero. The answer may report original intervals without displaying that internal clipped representation.
- `event_search` for `measure_interval`, without a time filter, exposes all 10 raw records; exactly one has `SourceTimeKnown=false`. This provides an existing route to retrieve row 9's original value 55 and unknown timestamp. The user does not need to request this tool.

A complete answer preserves the 9 timed records and distinguishes the untimed record (including its original value if claiming to list all original records). Merely stating that one record cannot be positioned is honest partial output, not full satisfaction of “列出原始记录”. Do not counterfeit a 10-row timed native table to satisfy the count.

Audit actual tool results, accepted analyzer scope, per-side preparation state, finalizer context, final accepted answer and rendered native tables. A `trace_query` success header is only an execution smoke check. No unit, device, workload, shared clock, hardware owner, state meaning, performance delta or root cause is established by this fixture. Keep historical manual FAIL evidence; this case does not replace it.

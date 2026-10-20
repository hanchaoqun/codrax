# Named real gzip ftrace: independent acceptance oracle

The model-facing question contains only the input, target, window and requested facts. This oracle must not be appended to the question or capture.

## Original material

Reference fixture: `HarmonyOS_PerfMcpServer-main/tests/fixtures/logandtrace/record_trace_20260821095539@6YF0126116-5955.sys.gz`. Its manifest describes a real six-second HarmonyOS capture. The acceptance input is an exact-byte read-only copy named `capture.data`, not a new recording or newly encoded gzip.

- Compressed: 3,602,086 bytes, SHA256 `4d66880019663f81462fe5df1615da58b368b6c5225e211dae88600cfc5396e9`.
- Full decoded: 54,122,686 bytes, SHA256 `f9d1e3a77441e38eaf659272d7430f5227cb1434ef4251c2425b213d3069b572`.
- Independent full scan: 429,766 physical lines / 429,756 events, no unmatched non-comment data rows; first/last timestamps 170435355043000 / 170441384953000 ns.
- More than the 250,000-event index bound: use existing full streaming/window paths, do not raise the memory bound or count a prefix as complete.

## Target window

SaInit0, tid3051 / recorded TGID1182; half-open `[170438000000000,170438100000000)` ns.
Independent raw sched_switch/wakeup integration, including preceding boundary state:

| Observed state | Window duration |
| --- | ---: |
| S sleep | 96.780 ms |
| Runnable | 1.088 ms |
| Running | 2.132 ms |
| Total window | 100.000 ms |

Eight target wake events, from these exact writer identities:

| Writer TID | Recorded TGID | Recorded name | Count | Decoded physical lines |
| ---: | ---: | --- | ---: | --- |
| 51299 | 3762 | OS_FFRT_2_1041 | 2 | 154702,154822 |
| 1021 | 908 | OS_FFRT_2_0 | 3 | 156371,156526,157293 |
| 20975 | 908 | OS_FFRT_2_15 | 3 | 157374,157518,158057 |

The full independent oracle retains target rows, pre/post-window anchors, intervals and exact decimal timestamps. Raw writer identity is not a physical hardware role or a broader business role. Sleep→wake and wake→switch-in are distinct intervals. Independent native-query results must be checked against the raw rows; disagreement must be investigated rather than editing the oracle to match the tool.

## Acceptance and non-claims

Natural ordinary name → safe candidate discovery → full shared preparation → actual native query, without `--htrace` or pre-preparation; preserve any system supplement's exact source and requested window. Candidate recognition alone is not ready, CRC validation, event completeness or causal authority. Decoded line numbers must not be attributed to compressed bytes. Original compressed bytes and strong identity must remain unchanged.

Audit final prose, tables, structured primary/principal and any diagram against the same source/window. No adjacent IO or background thread may be promoted to target root cause. No arbitrary name, sample weight, or parser label can establish execution/hardware semantics. A fact-only question does not require a root-cause election.

Track bad-source/CRC/cancellation/selector-retry and diagram branches as NOT_TRIGGERED unless this actual run exercises them; deterministic regressions are separate. Exactly one real run of this case, parallel with the fixed second case. Machine success cannot replace independent human semantic audit; original failures remain retained.

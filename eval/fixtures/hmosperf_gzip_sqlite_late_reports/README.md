# HMC237 late-report and grouped native-fact oracle

This is a new, synthetic, redistributable SQLite fixture; HMC236 inputs remain
unchanged. Only `capture.transport` is attached. This README, `oracle.json`, SQL,
and unpacked database are independent audit material, not model context. The
source repository is `stub_repo`, not this fixture directory.

The natural question selects reports emitted in **[1.0, 1.5) seconds** with at
least 3 reported jank frames, asks for each record and counts by application,
and asks for a record overview diagram. It does not prescribe a tool, native
selector, result count, safety caveat, or unique query sequence.

| Report time (s) | appid | Frames | Native start (ns) | Native end (ns) | Duration (ms) |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1.000 | 27599 | 3 | 920000000 | 980000000 | 60 |
| 1.081 | 41002 | 4 | 980000000 | 1080000000 | 100 |
| 1.161 | 27599 | 5 | 1100000000 | 1160000000 | 60 |
| 1.421 | 41002 | 7 | 1320000000 | 1420000000 | 100 |
| 1.499 | 41002 | 6 | 1430000000 | 1470000000 | 40 |

The exact result is **5 records: appid 27599 has 2; appid 41002 has 3**. The
first record's entire payload interval precedes the report window; the second
crosses its lower boundary. Both remain members because the question selects
report emissions, not payload starts or overlap. The raw payload timestamps
already share the trace clock; no timezone or domain conversion is needed.

Four independent negative controls are present: a 9-frame report at 0.999s,
a 2-frame report at 1.301s, an 8-frame report exactly at 1.500s, and a 10-frame
report at 1.501s. The latter two payload intervals are inside the requested
window but their reports are outside. Thus marker total 9, report-window total
6, threshold-selected total 5, two applications and counts 2/3 are distinct.
Adding an unrequested `payload.start_ts >= 1000000000` condition silently
reduces the valid result to 3; this is a diagnostic counterexample, not an
alternative oracle or evidence of a repaired product contract.

The low-level event lookup retains its existing closed time-selector semantics:
an executed lookup from 1.0 through 1.5 yields 6 threshold-matching records,
including the right-boundary report. The request-owned half-open display then
retains 5, preserving the original query coverage of 6 and reporting one scope
exclusion separately from budget omissions. Likewise, the extra payload-start
filter yields 4 native records but only 3 in the requested half-open view.
Do not change the global lookup contract to make this fixture pass.

Human acceptance verifies preparation and actual tool arguments, complete
native members, selected columns/rows and group counts, final prose/table/graph
tuples, and every output side channel. Report time must not replace native
start/end time. A graph is a record overview, not causal evidence or a fabricated
wakeup sequence. Other source facts must not be introduced as roots. Native
summary/table/diagram selection can be credited only when actually exercised;
correct freeform rendering is not proof of native selector use. A valid
partitioned query is allowed if its full application population is established.

Case regexes are intentionally only smoke checks. `oracle.json` and full human
tuple/coverage audit remain authoritative; the case does not assert that regex
presence proves grouping, membership, or chart correctness. Existing failed
answers remain unchanged, and deterministic checks do not substitute for a new
natural failed-query/retry sequence.

Regenerate absent binary outputs with `bash generate.sh`. It refuses overwrite,
creates a closed database from `capture.sql`, checks SQLite integrity, produces
one deterministic gzip member (`gzip -n`) and checks decompressed byte identity.
The transport suffix and gzip header provide no source or clock hint. Record
case/SQL/JSON/database/transport SHA-256 before the live run. This covers
synthetic gzip/SQLite intake, not device BRBE/SPE or every binary format.

# Filtered hotspot oracle — audit-only, not attached to the model

The natural question selects TID 11, [1, 2) seconds, the recorded `work`
hotspot in `/app/A.so`. The input fixture is unchanged. This is a flat recorded
hotspot selection; it does not establish an exact sampled prefix path for every
matching sample.

| Producer / event | Selected samples | Selected weight | Full-window cohort samples / weight |
| --- | ---: | ---: | ---: |
| raw_perfdata_fallback / cycles | 2 | 15 | 3 / 35 |
| raw_perfdata_fallback / instructions | 1 | 100 | 1 / 100 |
| simpleperf_report_sample / cycles | 1 | 7 | 1 / 7 |

The selected raw cycles samples have weights 10 and 5. The weight-5 sample
records this symbol/library but has no verified frame boundaries. The exact
`sys;root` → A path has weight 10, not 15; the literal `sys;root` symbol must not
be split on its semicolon. `/app/B.so` weight 20 is not a selected member, and
the 3-second A sample with weight 900 is outside the requested window.

When reporting the filtered-population share, A is 15/15 = 100% of raw cycles;
when separately comparing with the full-window raw-cycles cohort, it is 15/35
= 42.857143%. Both are meaningful only with their actual denominator and query
scope stated. The other two selected cohorts have 100% shares in their own
populations. Never combine the independent producers/events into a denominator
of 122 or 142, nor turn counts/weights into CPU duration or a causal conclusion.

Audit actual tool arguments, full native payloads and the finalizer context as
well as prose, tables, figures and sidecars. A successful unfiltered query plus
model-side arithmetic may answer the question, but does not naturally exercise
the new filtered-member handoff; record that branch as not triggered rather
than treating a machine PASS as implementation coverage. Likewise a companion
tree's unsupported-filter state is not an empty flat result, and a valid flat
receipt cannot certify a complete unfiltered tree.

This case is a real-model run over a constructed text fixture, not a fresh
device capture or the full sample-only binary input. It does not re-sign the
original lossless-tree or binary cases. Preserve failed runs and independently
record machine and human verdicts.

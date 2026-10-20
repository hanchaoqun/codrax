# Independent oracle: ordinary named gzip sampling input

This oracle is never attached to the model. The question names an input,
thread, time window and requested facts; it does not teach tool routing,
missing-data rules, source identity, units or causal constraints.

Use an operator-created gzip transport of the unmodified reference capture:
`HarmonyOS_PerfMcpServer-main/tests/fixtures/hiperf_brbe_001/test_brbe.data`.
The original is 30,062,585 bytes, SHA256
`da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`.
Pin the actual gzip bytes and physical identity before and after the run.
This is one real PERFILE2 recording in a synthetic transport, not a new device
capture or independent second recording. Do not preattach it via `--htrace`.

Independently parse the original with the reference binary parser before the
run. Full population: 12,000 samples; [486489.4,486489.6) seconds: 1,393;
tid1609: 164 samples, period/weight 259. CPU0 and CPU1 have 82 samples each,
weights 85 and 174. Their recorded timestamp envelopes in nanoseconds are
[486489464088122,486489465612497] and
[486489588095622,486489589609372]. These are observation envelopes, not
continuous execution durations or CPU residency proof.

Audit the actual natural classifier, bounded input navigation, ordinary
shared preparation, complete native query, successful material handoff and
automatic supplement. A candidate only identifies an existing reader to try;
it is not CRC validation, readiness or a successful query. Inspect both the
original compressed source and the complete prepared material provenance.
Native `perf_stats` may legitimately include the sampling tree without a
separate `perf_sampling_tree` call. Do not require a particular call order.

Record sample kind and frame provenance exactly. A recorded IP/single frame
does not prove a fully unwound leaf, interrupt/callback context or execution
state. Counts and weights are not elapsed time; hotspots are not causal-chain
roots. Preserve the previous manual sampling interpretation failure unless
the actual complete answer passes this audit. Correct caveats elsewhere do
not cancel an unsupported affirmative claim in the narrative/table/diagram.

Audit same-source reuse with the actual query receipts, field coverage,
preview completeness and context block sizes. If natural calls do not
exercise equivalent selectors or a budget branch, record NOT_TRIGGERED;
public deterministic cases cover that branch, not a third model run.
Malformed/truncated/CRC/over-budget/canceled/changed source, unknown or nested
content and independent sources remain separate negative-control tests.

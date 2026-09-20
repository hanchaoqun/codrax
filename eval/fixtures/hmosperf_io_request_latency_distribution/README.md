# Complete IO request latency populations

This is an authored synthetic text trace, not a customer capture. Only
`events.systrace` is attached. This README and `expected.json` stay outside the
`stub_repo` analysis root; they are evaluator truth, not model context.

The question fixes the window to **1.000–14.000 seconds** and requests separate
RQ/BIO, device and read/write groups. All in-window valid pairs are wholly
inside that window. Durations are full request start-to-completion elapsed
times, not CPU time or response-impact blocking time.

## Independent numerical oracle

Every group has source `events.systrace`, device `12,80`, and layer `block`.
Quantiles use sorted samples and linear interpolation at `(n-1) * p`.

| Endpoint family / operation | Samples (ms) | n | Min | Max | Mean | P50 | P90 | P95 | P99 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| RQ / read | 1,2,3,4,5,6,7,8,9,10,11 | 11 | 1 | 11 | 6 | 6 | 10 | 10.5 | 10.9 |
| BIO / read | 2,4,6 | 3 | 2 | 6 | 4 | 4 | 5.6 | 5.8 | 5.96 |
| RQ / write | 5,15,25 | 3 | 5 | 25 | 15 | 15 | 23 | 24 | 24.8 |

There are **17 complete valid pairs across three distinct groups**. Summing
their sample counts is an inventory count only: there is no combined latency
distribution, no RQ/BIO end-to-end join, and no response-delay sum. RQ measures
issue-to-complete; BIO measures queue-to-complete. The endpoint families must
remain distinct even though the device and operation overlap.

The three lowest RQ-read samples cannot appear in the public eight-longest
request preview. Computing from that preview loses valid samples and yields
the wrong distribution. Different tool decompositions are acceptable, but
the answer must consume or independently recover the complete exact-pair
population, not reinterpret Top-N as the population.

Exclusions and quality:

- The 499 ms RQ-read pair at 0.500–0.999 s and the 1000 ms pair at 15–16 s
  do not intersect the requested window and must not affect any statistic.
- Sector `999000` has two concurrent starts at 13.500/13.510 and two completions
  at 13.520/13.530. Its entire coarse-key cohort is ambiguous: one ambiguous
  cohort, two suppressed requests, zero latency samples. Do not FIFO-guess.
- Sector `999008` starts at 13.900 with no completion anywhere in the trace:
  one unpaired start, no invented duration, not a zero-duration observation.
- Other groups have no unmatched or ambiguous endpoints. No summary groups
  overflow the display cap. `expected.json` records exact machine values.
- There are no scheduler switch, wakeup, business span or response target
  records. High P99 is descriptive IO activity, not proof that any thread
  blocked a user response. No causal root or priority diagnosis may be minted.

## Manual acceptance

1. Verify window continuity, all three group identities, their sample counts
   and every requested statistic against the table (rounding to at least two
   decimal places where needed; added trailing zeros are harmless).
2. Inspect tool logs/model context: the full typed distribution must be
   available, and Top-N events must not be the sole basis for aggregation.
3. Confirm the ambiguity and incomplete tail are disclosed and excluded,
   rather than treated as successful pairs, zero durations or causal absence.
4. Require explicit limits on response/root-cause conclusions and on merging
   distinct endpoint layers. Business-friendly labels are preferred; internal
   method/policy enums need not appear in the answer.
5. The case does not demand a diagram. If one is volunteered, audit its
   relation meaning and syntax; IO durations alone cannot become wakeup edges.
6. Review first-answer rejections, repair attempts and available context.
   Regex smoke PASS alone is not semantic acceptance: group-to-number binding
   and unsupported claims require human audit.

## Paired replay (exactly two parallel cases, one run each)

For a zero-model deterministic fixture check, use the included `collect.yaml`:

```sh
./codrax --tracediag eval/fixtures/hmosperf_io_request_latency_distribution/collect.yaml \
  --trace eval/fixtures/hmosperf_io_request_latency_distribution/events.systrace \
  --out /tmp/hmc081-io-distribution-evidence.txt
```

This produces the three typed distribution rows and pairing-quality counts;
compare them with `expected.json` before interpreting a model replay.

Commit the intended Go changes and rebuild first. Then run from repository root:

```sh
PARALLEL=2 TIMEOUT=1800 \
EVAL_RESULTS_ROOT=eval/results/hmc081_io_distribution_20260920 \
EVAL_SELECTED_SUMMARY=eval/parallel_selected_summary_hmc081_io_distribution_20260920.md \
bash eval/parallel_selected.sh \
  eval/cases/trace_query_io_request_latency_distribution.case \
  eval/cases/trace_query_business_marker_io_chain.case
```

`parallel_selected.sh` invokes `eval/run.sh <case> 1` for each of the two paths.
The companion existing case preserves business discovery, S-state IO closure,
separate request/blocking/runnable rulers, background exclusion and final
Trace causal projection. This fixture addition does not itself constitute a
live replay result.

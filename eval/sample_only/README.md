# Sample-only original binary: HMC 18.3 / batch 239

One natural question uses the ordinary `eval/run.sh` single-shot CLI with
`--htrace` pointing to the **complete original Linux perf binary**. No
preconverted `.perftrace`/bundle/oracle is supplied as the attachment. The source
is the reference repository's `tests/fixtures/hiperf_brbe_001/test_brbe.data`;
its name does **not** mean Codrax interprets BRBE branch records.

The 30,062,585-byte source is intentionally not committed or binary-cropped.
Its SHA-256 is
`da713af8c490d00fffa49b7c4274b93037b9171a4a73e05500d64d071232c632`.
Keep one unchanged copy in the ignored batch evidence archive, with this hash.
The reference test's older size/sample-count comment is stale; actual bytes and
independent `HiperfBinaryParser.parse_samples()` report 12,000 samples over
486489.326161559–486490.116057028 seconds.

## Offline oracle and actual CLI startup (zero model requests)

From the repository root, choose a fresh output directory whose parent exists:

```sh
CODRAX_SAMPLE_ONLY_INPUT=/Users/han/opt/hmosperf/HarmonyOS_PerfMcpServer-main/tests/fixtures/hiperf_brbe_001/test_brbe.data \
CODRAX_SAMPLE_ONLY_ORACLE_DIR=/tmp/codrax-hmc239-sample-only-oracle-new \
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk \
go test ./eval/sample_only -run '^TestReferenceSampleOnlyBinaryOracleAndCLI239$' -count=1 -v
```

The test pins the entire source hash, byte count, original file identity,
mtime/mode and adjacent directory entries. It prepares the complete input with
the production parser under an operator-only directory, retaining preparation
receipt, full/window/target native JSON and `oracle.json`. It builds the actual
CLI once and runs the same ordinary read-case flags/attachment with `--request`
omitted and only `/exit` on stdin. This checks real CLI attachment preparation
and exit, not model analysis. A local refusing provider must receive zero
requests. The child must independently retain a source-bound sample-only
preparation and all 12,000 samples. Its preparation is separate from the
operator oracle. `cli-preflight.json` explicitly marks model query and answer
unexercised. Tests skip by default unless the local input is explicitly supplied;
they never call a real model and do not read provider credentials.

## Frozen natural question and oracle

> 请看这份采集中 486489.4 到 486489.6 秒 poll_report 线程（tid 1609）的采样热点，主要落在哪些函数？这些结果能解释它为什么停顿吗？

The input contains no scheduler events. Whole-window source-wide inventory has
1,393 samples across 11 precisely identified threads. The selected thread has
164 samples in one `hw-cpu-cycles` / `cycles` cohort, total event weight 259:

| Recorded hotspot | Samples | Event weight | Within-target weight share |
| --- | ---: | ---: | ---: |
| Unresolved kernel IP `0xffffff800b829d24` (`[kernel.kallsyms]`) | 82 | 174 | 67.1815% |
| `OHOS::HDI::Sensor::V3_0::SensorCallbackProxy::OnDataEvent_(...)` (`/vendor/lib64/libsensor_stub_3.0.z.so`) | 82 | 85 | 32.8185% |

The reference binary parser independently confirms the time-window sample
count, TID count, and per-TID event-weight totals. Function/IP names come from
Codrax's source-bound native result and remain subject to its symbolization
quality flags. Exact frame boundaries do not imply complete symbols/unwinding.
The parser discloses skipped `branch_stack` and `unknown:0x80000000` payloads,
unknown sample kind and assumed clock alignment. CPU identifiers are known;
that is not scheduler running/runnable/blocked-time evidence. The question asks
whether the evidence explains a pause; no actual pause duration/cause is part
of this fixture oracle.

## One authorized real run

After the main agent freezes/builds the same binary used for both batch cases:

```sh
CODRAX_SAMPLE_ONLY_INPUT=/absolute/path/to/the/unchanged/test_brbe.data \
EVAL_RESULTS_ROOT=eval/results/hmc_sampling_identity_20261013 \
CODRAX_BIN=/absolute/path/to/frozen/codrax \
bash eval/run.sh eval/cases/trace_binary_sample_only.case 1
```

Use the already approved provider routing for `run.sh`; never copy credentials
into the case/oracle/archive. Missing input is an explicit error, not a fallback
to a different fixture. The case uses an isolated stub source repository and
the existing 15-step read runner budget; there is no new controller/harness or
automatic rerun.

`EXPECT_SUCCESSFUL_TOOL_RESULTS=trace_query` plus `OnDataEvent_` on the
digest-bound primary answer is only machine smoke, not acceptance. Human audit
must read the entire final answer and actual preparation/query payloads: verify
original SHA → child source/derived receipt → target window/TID query → answer;
the 164-sample / 259-weight target is not the 1,393-sample source population;
unresolved IP stays unresolved; percentages are event weights, never wall-time,
CPU occupancy, or expected sample density. No scheduler or BRBE/SPE/IPC metrics
may be fabricated, and no hotspot alone proves a pause root cause. Read every
produced table/diagram/attachment for the same boundaries. Missing diagrams or
natural retries are unexercised, not passed. Retain any failure unchanged.

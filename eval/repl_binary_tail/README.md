# Actual CLI REPL binary-tail evaluation (HMC 18.3 / batch 238)

This is one explicitly opted-in real-model case. It starts the ordinary built
`codrax` executable with `--mode=code` and without `--request`, sends `/htrace <original>`, one natural
question, then `/exit`. It does not install a runner, controller, model adapter,
converter stub, or precomputed answer in the child process. Ordinary Go tests
exercise deterministic fixture, transport, and public REPL lifecycle behavior;
they do not call a model.

The input is a **constructed complete production-format RMQ binary**, not a new
device capture. Its wire layout matches the existing CLI/REPL binary-tail
fixtures. The real production parser prepares all 12 `sched_wakeup` events.
The attachment preview budget is fixed at 1024 bytes: the final event
`1.011 s / tail-target / 424242` must be absent from that preview. The question is:

> 这份采集在 1.0105 到 1.012 秒唤醒了哪个线程？请列出时间、线程名称和线程编号。

The expected interval contains one event. `head-worker / 42` belongs to earlier
events and must not replace the tail. No fixture source, oracle, converted file,
or expected answer is supplied in the child repository. The operator-only
preflight is stored separately and cannot satisfy child-evidence checks.

## Run one case

Build the project first. The output parent must exist; the selected output
directory must not exist. Use the approved provider file (its contents are
never copied by this harness). For example, from the repository root:

```sh
go run ./eval/repl_binary_tail -live \
  -binary /Users/han/opt/codrax/codrax \
  -providers /absolute/path/to/providers.yaml \
  -out /Users/han/opt/codrax/eval/results/hmc_repl_terminal_20261013/repl_rmq_binary_tail \
  -timeout 20m
```

An optional `-settings /absolute/path/to/non-secret-runtime.yaml` preserves the
approved baseline settings, with recorded overrides for read-only operation,
1024-byte previews, answer dumps, no auto-resume, and no preview server. The
normal 50-step pipeline budget is retained. No automatic rerun is performed.
On macOS, use the repository's configured `SDKROOT` when running Go commands.

Fixture-only validation (no CLI or model run):

```sh
go run ./eval/repl_binary_tail -prepare-only -out /tmp/new-rmq-oracle-directory
go test ./eval/repl_binary_tail -count=1
```

## Evidence and acceptance boundary

`evaluation.json`, `stdin.txt`, `runtime.yaml`, `run.out`, `oracle.json`, child
logs, original input, preparation receipts, query payloads, REPL snapshots, and
answer dumps are retained, including on failure. `receipt.json` indexes child
artifacts by SHA-256. Credentials remain in the external provider file; normal
logs should still be treated as local evaluation evidence, not public data.

Machine checks prove original-byte identity, original→child preparation→query
path binding, the actual REPL snapshot's bounded tail-free preview, presence of
the exact tail tuple in a child-native query payload, model/tool telemetry, and
an answer sidecar bound to its Markdown digest. Answer tuple checks are only a
smoke oracle. Telemetry proves model and tool activity in this child, not a
per-call association with that query; presence of a tuple does not validate
query-window selection, exclusivity, or all answer surfaces.

Even success is `machine_checks_pass_human_pending`. Read the **entire** final
answer and process logs to verify original attachment→real preparation→native
query, the requested time window, the one complete tuple, table/prose/diagram
agreement, no preview substitution, no invented causal/depth/wait claims, and
any actual failure→repair→retry sequence. No observed retry means retry is
unexercised, not passed. An unrequested/unproduced diagram is also unexercised.

The deterministic slow-runner test uses the real public REPL loop, blocks the
answer until the input reader has consumed `/exit` and physical EOF, then
releases it. It checks the answer is rendered before exit, without a timing
sleep; this lifecycle stub is never used by the live entry. The transport
negative uses a helper test process that exits 37: a correct-looking stdout
tuple cannot produce successful native evidence or live acceptance.

## CLI startup regression and original failed attempt

The first batch-238 attempt failed before any model call: the harness supplied
`--mode=read`, confusing the internal pipeline phase with the public CLI task
mode. Its `machine_checks_failed` receipt, 0.915989334-second execution, and
`unknown mode "read"` output remain unchanged in the original result directory.
The earlier argument-string and public-Loop tests did not exercise CLI parsing;
their passing status did not prove the actual executable could enter the REPL.
The new actual-CLI regression then exposed a separate production seam: piped
stdin was not passed into the REPL, so it incorrectly attempted to open a TTY.
That deterministic failure is retained separately from the initial live-attempt
failure; it also occurred before any model call.

The minimal correction is `--mode=code`, with the existing `write_enabled: false`
kill switch unchanged. A non-short deterministic test builds the actual CLI
once and runs the exact harness argv with only `/exit` on stdin. It requires
normal exit and the REPL's goodbye output. An otherwise identical invalid
`--mode=read` negative must fail explicitly. A local rejecting provider endpoint
must receive zero requests, and child logs must contain no model/tool/answer
execution. These startup checks are not real-model acceptance. Any separately
authorized real attempt must use a fresh result directory; it cannot overwrite
or relabel the original failure.

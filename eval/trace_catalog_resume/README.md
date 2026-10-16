# Fresh-process capture checklist evaluation

This is one real CLI question after explicitly bounded, zero-model operator
preparation. It is not a naturally interrupted model run. The two SQLite
captures are the existing constructed `hmosperf_catalog_objects` fixture, not
new device recordings. No provider configuration is read by this helper.

The preparation process copies the two full originals into a fresh workspace,
checks an independent read-only SQLite oracle, invokes public `trace_catalog`
to declare four queries, and invokes public `trace_query` for exactly one of
them. It saves the checkpoint, original tool result and source hashes. The
checkpoint is inside the current repository; oracle/results remain outside.
The preparation process then exits. Do not copy this workspace to a new path:
the catalog retains exact physical source paths and generations.

## Prepare, then run exactly once

From the repository root, with the output parent already present:

```sh
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk \
  go run ./eval/trace_catalog_resume \
  -out /absolute/new-result-directory
```

Only after the production binary and inputs are frozen, the operator starts
the ordinary CLI in a fresh process (this command invokes real models):

```sh
cd /absolute/new-result-directory/workspace
/Users/han/opt/codrax/codrax \
  --repo /absolute/new-result-directory/workspace --branch main \
  --providers /absolute/existing/providers.yaml \
  --pipeline-max-steps 50 --log-level debug \
  --log-dir /absolute/new-result-directory/logs --color never \
  --request "$(cat /absolute/new-result-directory/question.txt)" \
  > /absolute/new-result-directory/run.out 2>&1
```

The question asks to continue the prior checklist, check each capture for
threads 101 and 202 during 10.000–10.050 seconds, and give allocations, frees,
call sites and completion by capture/thread. It does not disclose expected
answers, tool names, schema fields, or safety constraints. Keep the CLI exit
code, frozen binary hash, whole logs, blobs and final answer. There is no
automatic retry or answer correction. Do not use `eval/run.sh`'s fixture copy
path for this entry; its new paths would break the checkpoint identity.

## Oracle and exit conditions

| Capture | TID | Events | Source frame rows |
| --- | ---: | ---: | ---: |
| captures/capture.data | 101 | 3 | 8 |
| captures/capture.data | 202 | 1 | 3 |
| captures/nested/capture.data | 101 | 1 | 3 |
| captures/nested/capture.data | 202 | 0 | 0 |

The operator checkpoint has one actual success (first row) and three
unexecuted entries. Cross-process resume must produce four current pending
entries: old success is historical navigation, not a reusable current result.
All four current members must be actually queried in the new CLI. The empty
member is successful empty, not unexecuted. Root-capture event at 10.050 is
outside the window; captures keep independent clocks and identities. Missing
symbols/depths and duplicate depths remain distinct from display truncation.
There are no scheduler witnesses: do not infer CPU time, leak or response
causality from these resource events.

Public deterministic tests cover fresh-state resume, all four real native
queries, old ID non-authority, foreign-root rejection and query-ref overrides.
They use no LLM. The real evaluation must separately show the model selecting
resume and the new query references, normal source preparation, new payloads,
and complete final answer. Rediscovery with a correct answer does not prove
the new recovery path was exercised. An unused recovery path is NOT_TRIGGERED,
not PASS. Changed/failed/canceled members remain visible; never fill a missing
result from the operator oracle or old payload.

The exact `saved_index` in the successful current `trace_catalog` result may
be supplied explicitly for a post-run structured smoke audit:

```sh
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk \
  go run ./eval/trace_catalog_resume \
  -out /absolute/new-result-directory \
  -audit-index /absolute/exact/current/catalog.json
```

This does not discover the newest file or scan prompts for keywords. It checks
unchanged source/checkpoint bytes, expected current member selectors/windows,
terminal statuses and absence of old result refs, then writes
`current-index-audit.json` with `manifest_smoke_pass_human_pending`. It does not
authenticate serialized evidence or replace original-payload, real-execution
and all-answer-surfaces human audit. Output files are never overwritten.
The current attempt's executed arguments must also equal the explicit frozen
plan; checking only its planned window would not establish what was executed.

Before the paid run, the operator can use the frozen binary with the same
repository/provider/log/color/pipeline arguments, omit `--request`, and send
`/exit` on stdin. This tests real CLI startup with no model request, not the
natural request route or resume behavior. Preserve that preflight separately;
do not use the existing-query Go test as a substitute for CLI startup.

Run only zero-model tests during development:

```sh
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk \
  go test ./eval/trace_catalog_resume -count=1 -v
```

Requirements: existing fixture originals, Go toolchain, Git and SQLite CLI.
The live child alone uses the configured provider; do not archive credentials.

## Evidence export variant

Pass `-export-target capture-review` during preparation to append only the
natural deliverable request “并将本次检查的证据包导出为 capture-review。” to the
same question. The operator checkpoint, four expected queries and zero-model
preparation are unchanged; preparation must not export anything. Run this in
a separate fresh directory from the unchanged recovery case, once each.

Audit the precise package path returned by the actual successful export call,
not a newest-directory guess. Keep its manifest, report and original member
files. Check manifest hashes and every current query/attempt against the real
tool outputs and source windows. Physical export success, registered execution
coverage and material completeness are separate axes; an export of pending,
failed or missing material is not a completed analysis. Historical results
remain navigation and must not become current evidence. A model that never
invokes export leaves this entrance NOT_TRIGGERED, even if another output
file happens to exist. No automatic replay is allowed.

## Explicit final-report replacement variant

Use the same natural export question in a separate fresh workspace, and pass
the existing host CLI option `--report-md /absolute/selected-report.md`. Seed
that exact file with a known prior report and record its SHA before startup.
The CLI flag, not a model path or hidden question constraint, authorizes the
replacement. An `/exit` startup preflight omits single-shot report flags and
must leave the prior report intact; separate public flag tests reject them in
REPL mode. This preflight checks startup, not the report publication path.
Run the real question once; independently compare the published Markdown with
the normal final-answer dump and verify its logged byte count/SHA. Preserve
the prior bytes, source hashes, package receipts, final answer, and process
receipt outside the question. No automatic retries or simulated disk faults
are part of this live lane; public deterministic tests cover failed writes,
external changes and the per-file transaction boundary. Successful physical
report publication and complete answer correctness receive separate verdicts.

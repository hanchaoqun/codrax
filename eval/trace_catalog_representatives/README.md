# Native representative comparison: one real CLI question

This entry prepares four constructed SQLite inputs, then leaves discovery,
query planning, representative selection and every actual query to a normal
CLI/model process. Preparation uses no catalog or trace tools and no model.
The four inputs are two full copies of each existing
`hmosperf_catalog_objects` original, at distinct physical paths. They are not
four new device recordings. Equal bytes do not make their source identities
interchangeable.

## Prepare and run once

From the repository root, choose a new absolute output directory whose parent
already exists. No existing preparation or audit receipt is overwritten.

```sh
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk \
  go run ./eval/trace_catalog_representatives \
  -out /absolute/new-result-directory
```

The helper copies inputs read-only, checks an independent SQLite event/frame
oracle, and initializes a Git repository. Oracle files and preparation metadata
remain outside the model's workspace. No checkpoint, query result, chosen
representatives or canned answer is seeded. Do not copy this prepared workspace
to another path after freezing the inputs.

After freezing the standard production binary and all input hashes, the
operator starts exactly one ordinary CLI request (real models):

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

The natural question asks to group the checks by thread/window, select two
representatives per group, compare allocation/free/call locations during
10.000–10.050 seconds for TIDs101/202, and describe progress. It contains no
internal API names, expected values or implementation guardrails. Do not
silently correct the model or rerun to turn a failure green.

Before that paid run, the frozen binary may be started with the same flags but
without `--request`, feeding `/exit` on stdin. This validates real CLI startup
without a model call; it does not validate routing, selection or query behavior.
Preserve it separately. The helper's public-tool tests are not a substitute.

## Independent oracle and acceptance

| Workspace input | Original fixture | TID101 events/frames | TID202 events/frames |
|---|---|---:|---:|
| captures/session-a/capture.data | root original | 3 / 8 | 1 / 3 |
| captures/session-b/capture.data | nested original | 1 / 3 | 0 / 0 |
| captures/session-c/capture.data | root original, second copy | 3 / 8 | 1 / 3 |
| captures/session-d/capture.data | nested original, second copy | 1 / 3 | 0 / 0 |

The required task population is eight exact query plans: four captures ×
two thread selectors, with the same explicit window. Two same-specification
groups each select two representatives: four selected, four not selected.
Selection is a soft navigation priority, not a severity or statistical claim.
The operator does not force particular query IDs or files to be selected.
Values are audited against the originals actually selected, not against the
best-looking answer. If two selected copies have equal values, equality says
nothing about unselected inputs.

Every selected representative must be queried through the normal preparation
and native query path. A selected empty result is executed empty, not pending
or proof the capture is healthy. Members outside the selected four remain
explicitly unselected, even when ordinary exploration actually queries them.
Their execution state must come from their own recorded attempts: not executed,
success, empty or failure are different states. Neither directory coverage nor
a chosen member proves another member's runtime behavior.
Root-original event at10.050 is outside this request. Duplicate frame depths,
missing symbols and depth gaps remain source facts, not a fabricated complete
call path. Resource events alone do not prove CPU time, leaks or stall causes.

The real acceptance must separately inspect:

1. Model-origin discovery/planning/selection before execution of the selected
   task queries, complete page traversal if needed, and unchanged selected IDs
   across status refreshes. Tool availability or operator tests do not prove
   natural adoption.
2. Original source identity/hash, actual query parameters and full payloads;
   the native facts must match the table above for the chosen four members.
3. Final prose/tables/diagrams, per-group differences, source/window/target
   attribution, current progress, all four unselected members and lack of
   unsupported generalization. No diagram is forced by this question.

## Structured post-run smoke audit

Use the exact `saved_index`, `selection_snapshot`, and
`selection_snapshot_sha256` returned together by the final successful
`trace_catalog(action=representative_status)` result. The selection snapshot is
separate from the ordinary catalog; it is navigation-only and cannot restore
execution authority. Do not choose files by timestamp or manufacture a mirror.

```sh
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk \
  go run ./eval/trace_catalog_representatives \
  -out /absolute/new-result-directory \
  -audit-index /absolute/exact/current/catalog.json \
  -selection-snapshot /absolute/exact/current/selection.json \
  -selection-sha256 EXACT_SHA256_FROM_THAT_RESULT
```

This checks unchanged source bytes and Git HEAD; all eight source/object/window
plans; the paired two-group selection; four selected members whose latest actual
attempts are successful/empty and complete, with matching parameters and source
revisions; four unselected members with their true current execution states;
and exact coverage totals. Genuine failure-then-success retries are retained;
success-then-failure does not borrow the earlier success to pass. It writes
`representatives_smoke_pass_human_pending` in `current-index-audit.json` only
on success, exclusively. It does not authenticate serialized payload references
or verify model prose. A serialized success never substitutes for raw execution
and all-surface human audit. Legitimate auxiliary queries are independently
retained: queries excluded at selection and queries added afterward must be
known, mutually disjoint, outside the core selection, and together account for
every extra current record. Their complete attempt histories are preserved in
the audit receipt. Extra exploration does not enlarge or replace the required
8-member / 2-group / 4-selected / 4-unselected task matrix. A selection polluted
to 16 members / 4 groups / 8 selected / 8 unselected still fails. These helper
rules do not retroactively pass earlier failed natural evaluations.

Development uses deterministic zero-model tests only:

```sh
SDKROOT=/Library/Developer/CommandLineTools/SDKs/MacOSX26.5.sdk \
  go test ./eval/trace_catalog_representatives -count=1 -v
```

Requirements: Go, Git, SQLite CLI and the existing complete fixture originals.
Only the separate live CLI uses the configured provider; never archive its
credential file. Tests prepare temporary directories, never the live output.

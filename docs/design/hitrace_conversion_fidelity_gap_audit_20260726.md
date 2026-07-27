# HiTrace Conversion Fidelity Gap Audit and Remediation Plan (2026-07-26)

## Scope and baseline

Baseline: `main@23c0c58ef`.

Customer symptoms:

- a pure Harmony/OpenHarmony `.sys` capture contains thread names but the
  generated systrace contains many `unknown` comm values;
- the generated systrace loses a large share of trace-marker spans;
- Donghu captures can carry a host TID/TGID in the ftrace header and a
  namespace PID in the `B|pid|...` / `C|pid|...` marker payload.

Reference witnesses:

- `/Users/han/opt/donghu/donghu.ftrace`;
- `/Users/han/opt/donghu/xxx_all.systrace`;
- hmtrace snapshot `7fb4eabae01f310beccecf339403aca4e9660131`;
- OpenHarmony SmartPerf `trace_streamer`;
- OpenHarmony Hiview trace collection sources.

The two text traces are different captures. They prove the host/namespace PID
shape and rich event-family expectations, but they are not a binary conversion
A/B pair and cannot quantify one capture's conversion loss.

## Confirmed failure chains

### Thread names

1. `sched_switch_lite` carries no `prev_comm` / `next_comm`.
2. The reviewed trace_streamer source can select the lite scheduler family and
   stop consuming later full scheduler rows that carry names.
3. Saved cmdlines populate an event comm but do not name lite prev/next
   identities.
4. Codrax SQL export reads display names only from `thread.name`.
5. Empty `thread.name` becomes `unknown` in registration, scheduler, wakeup and
   callstack output. There is no conversion-quality downgrade.

### Donghu namespace PID and span suppression

1. A ftrace row carries the host TID/TGID.
2. Its trace-marker payload carries a namespace PID.
3. The reviewed trace_streamer print/slice path binds the public TID to the
   marker payload PID, creating or selecting a different internal thread
   identity from the scheduler identity.
4. Scheduler Running intervals remain attached to the host internal identity.
5. Codrax looks up start/end CPU by the callstack internal identity.
6. `unknown_start_cpu` / `unknown_end_cpu` rejects the complete span.

Codrax's refusal to invent CPU 0 is correct. The gap is the absence of a typed
host/namespace identity relation and the coupling of span existence to CPU
availability.

## Remediation batches

### Batch A — conversion-quality authority and customer diagnostics

Status: delivered as `main@f2e69d4ad` on 2026-07-26.

- Add exact `TraceDBCoverage.metrics`.
- Count unnamed accepted threads.
- Count public TIDs represented by multiple ITIDs and owner IPIDs.
- Count scheduler boundaries rendered with at least one unknown comm.
- Count callstack pre-pairing, CPU, identity and shared sync-span suppression.
- Insert a bounded `conversion_quality/__semantic_quality__` coverage row near
  the front of CLI/REPL disclosure.
- Emit a soft `semantic quality is degraded` caveat. It never blocks conversion
  and never changes source admission.

### Batch B — name fidelity

Status: delivered as `main@ce08a5e92` on 2026-07-26.

- Preserve a typed, precomputed display-name roster per canonical ITID.
- Keep the canonical `thread.name` as the first display source.
- Use `process.name` only for a proven main thread.
- For an unnamed ITID, reuse another internal identity's name only when every
  nonempty name attached to the same exact public TID agrees.
- Use the display roster in registration, scheduler, wakeup and callstack
  output without mutating canonical thread metadata.
- Expose recovered-main, recovered-public-TID and still-unresolved counts.
- Keep ambiguity fail-closed for display recovery and keep names completely
  outside identity, lifecycle, owner and CPU authority.

Remaining upstream work: a DB cannot reconstruct full scheduler comm bytes that
trace_streamer discarded before SQLite materialization. The representative
binary fixture in Batch E must decide whether the embedded parser needs a
source-level full/lite fix or a bounded raw name-inventory companion.

### Batch C — host/namespace dual identity

Status: delivered as `main@9c814d012` on 2026-07-26.

- Preserve host TID/TGID/ITID separately from marker subject PID/ITID.
- Treat marker PID as span subject, never as host owner authority.
- Reconcile a marker ITID to a host scheduler ITID only when a distinct process
  with the same exact public TID has typed Running CPU at every required
  endpoint and the complete lifecycle authority leaves exactly one candidate.
- Emit the host header identity while retaining the namespace PID in the marker
  payload.
- Preserve the marker IPID as async/sync owner authority; the alias supplies
  only the header TGID and endpoint CPU lane.
- Keep ambiguous candidates fail-closed as
  `ambiguous_same_public_tid_scheduler_alias`.
- Persist the separate marker PID through both the in-memory and bounded SQLite
  sync-span stages.

### Batch D — span existence versus CPU availability

Status: implemented and verified on 2026-07-26; delivery commit is the Batch D
commit following `main@9c814d012`.

- Use a recovered exact host scheduler identity for endpoint CPU placement.
- Keep `span_present` distinct from `cpu_status`.
- Do not copy hmtrace's unconditional CPU-0 default.
- When identity, lifecycle and the interval are proven but CPU placement is
  unknown, tainted, lifecycle-rejected or alias-ambiguous, preserve the marker
  as an inline versioned comment:
  `# codrax_trace_mark_cpu_unavailable/v1 ...`.
- The comment is inside the generated systrace/tracebundle rather than a new
  repository or sibling directory. Generic ftrace readers safely ignore it;
  Codrax parses it as `trace_mark` evidence with
  `trace_marker_cpu_status=unavailable` and an exact closed-enum reason.
- Preserve sync B/E and async S/F pairing, marker PID, host TID/TGID, comm,
  name/cookie, timestamp and duration. Never attach those rows to CPU 0 and
  never use them for per-CPU/core attribution.
- Persist CPU placement separately through the in-memory and bounded SQLite
  shared sync-span stages; both backends produce byte-identical output.
- Add `source_rows_preserved_cpu_unavailable` and a distinct customer caveat.
  This count is not mixed into span-suppression or name/span-completeness loss.
- Keep actual malformed identity, lifecycle and pairing rows fail-closed;
  CPU placement failure alone no longer poisons a sync lane or async key.

Verified arms:

- strict versioned wire format, canonical base64 and closed action/reason enums;
- trace timestamp scan, full parse, span reconstruction and query caveat;
- unknown start/end, tainted Running, lifecycle-rejected Running and ambiguous
  host/namespace scheduler alias;
- laminar same-thread span preservation and exact-key async pairing;
- memory/SQLite staging parity and namespace marker-PID coexistence;
- public Event JSON surface and sparse side-table promotion guards.

Compatibility boundary: classic systrace has no representation for “trace
marker exists but physical CPU is unavailable”. Therefore external viewers
that only understand physical ftrace envelopes will ignore the versioned
comment. Codrax retains and analyzes it without inventing CPU evidence.

### Batch E — coverage closure

Status: E1 and the bounded E2 customer-collection lane are implemented and
verified on 2026-07-26; the redistributable binary parity fixture remains open.

- Enumerate all non-internal DB tables after the existing exporters complete.
  Exact coverage table names classify exported/resolver/unsupported schema
  surfaces; unmatched tables are checked with a bounded nonempty witness.
- Publish a `conversion_inventory/__table_inventory__` summary with classified,
  unclassified-empty, unclassified-nonempty, uninspectable and truncation
  counts.
- Publish each unmatched nonempty table (bounded to 128 names) as
  `unsupported_input`, and add a customer caveat saying its rows were not
  converted. This is advisory only: it does not reject positive rows or turn a
  future/noisy table family into a hard conversion gate.
- Table enumeration is capped at 1024 names, excludes SQLite internals, quotes
  identifiers safely and refuses control-character/oversized display names.
  Cap or name-integrity loss gets a separate retained-DB-review caveat.
- Expand typed raw-family coverage needed by trace_query.
- Make unhandled nonempty tables visible.
- Commit a redistributable representative no-perf `.sys` fixture with SQL and
  explicit builtin parity assertions.

### Batch E2a — bounded customer evidence

Status: implemented and verified on 2026-07-26; delivery is the diagnostics
commit following `main@4a955ea40`.

Customer feedback is constrained to one report below 1000 lines. Conversion
now accepts:

```text
codrax trace convert --input <capture.sys> --diagnostic-report <codrax-trace-diag.txt>
```

The customer returns only `codrax-trace-diag.txt`; the raw `.sys`, retained
SQLite DB, generated systrace and full console transcript are not required.
The report:

- is produced on both conversion success and conversion failure;
- has profile `codrax_trace_conversion_diagnostic/v1`;
- has a hard limit of 900 physical lines, including its final receipt;
- keeps the first 64 and last 64 progress events and explicitly counts the
  omitted middle;
- carries typed conversion-input, trace-provider, fallback, builtin-decoder
  and archive error fields when present;
- carries all result-side provider decisions, artifacts, conversion caveats,
  trace coverage and trace-DB coverage until the global hard cap;
- therefore includes the exact name-recovery, unresolved-name,
  scheduler-unknown, span-preserved/suppressed, host/namespace alias and
  unhandled-nonempty-table metrics already published by Batches A-E1;
- converts CR/LF/tab/NUL inside values to visible escapes and bounds every
  physical line to 8192 bytes, so one driver/tool error cannot inflate the
  customer response;
- refuses to overwrite an existing report and refuses report paths aliasing
  the input, systrace output or explicit retained DB.

The console can still be verbose; it is not the requested evidence artifact.
The 900-line report is the only file the customer needs to return.

### Donghu text-sample replay (E1 evidence)

The two user-supplied text traces were indexed through the production
`tracequery.BuildIndex` path:

- `donghu.ftrace`: 27,845 physical lines, 27,843 parsed events, 27,843 known,
  zero unparsed rows and zero parser panics; all 14 observed event families
  were recognized.
- `xxx_all.systrace`: 15,623 physical lines/events, 15,623 known, zero
  unparsed rows and zero parser panics; all 21 observed event families were
  recognized.

This narrows the current loss locus: for those text families, Codrax
trace-query parsing is not dropping events. Customer loss is in the binary
parser/SQLite materialization/export path or in families absent from these two
different-capture text references.

The E2 parity arm remains intentionally open: these two text files are not the
conversion output and oracle for one identical `.sys` capture. The bounded E2a
report can localize the failed provider/table/name/span stage without a large
customer upload, but it cannot prove which source rows were absent before
SQLite materialization. A redistributable pure trace binary (including
namespace PID and names/spans) is still required to measure
binary-to-DB-to-systrace row parity and decide any embedded trace_streamer
full/lite parser change without guessing.

## Customer diagnostic replay and Batch F closure (2026-07-27)

Evidence:

- `/Users/han/opt/customlogs/cmd_result.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag.txt`;
- customer converter `0.1.20260727`, bundled trace_streamer
  `7fb4eabae01f310beccecf339403aca4e9660131`;
- input size 27,022,926 bytes, trace interval
  `69326.012181718..69328.343094061`;
- SQL-normalized output 189,739 query-ready events and 33,326,385 bytes.

The successful receipt proves transport, SQLite export, row sorting,
publication and tracequery reparse. It does **not** prove semantic
completeness: the same typed report proves that large source families were
withheld before publication.

### Exact failure attribution from the returned report

1. The `callstack` table had 96,221 physical rows and emitted zero spans before
   the shared pairing stage:

   - `invalid_name=80209`;
   - `invalid_flag=15911`;
   - `invalid_duration=101`;
   - `source_rows_suppressed_pre_pairing=96221`.

   This is the dominant explanation for the customer's sparse span output.
   The first two counters are decoder-contract gaps, not absence in the
   binary trace.

2. The thread registry accepted all 1,294 rows but 139 names remained empty;
   12,985 scheduler boundaries were consequently rendered with an `unknown`
   comm. This is a display-metadata loss chain. It does not authorize
   cross-ITID identity merging.

3. The generic `measure_filter` path read 14,873 clock-related samples but
   emitted only 2,455. It withheld 12,418 samples after 89 duplicate semantic
   filter names collapsed distinct lanes. The official specialized
   `clock_event_filter(id,type,name,cpu)` table was nonempty but unread, so the
   lost CPU dimension was exactly the missing disambiguator.

4. The high-level `raw` table had 66,856 rows but no argset column. Withholding
   it is correct: name and timestamp alone cannot reconstruct event payloads.
   This is not evidence that the immutable binary lacks those payloads, nor
   authority to synthesize them.

5. The eight-table unclassified roster mixed true output gaps with dependency
   and metadata tables:

   - `args`, `cpu_measure_filter`, `process_measure_filter` are consumed
     dependencies whose lineage was not represented;
   - `clock_event_filter` is an authoritative clock-event registry that was
     not consumed;
   - `data_type`, `device_info`, and `meta` are dictionary/device/parser
     metadata, not standalone ftrace events;
   - `frame_maps(src_row,dst_row)` is a real cross-frame relation and remains
     semantically unpreserved.

### Delivered Batch F repairs

Each item was committed and pushed to `main` independently:

| Batch | Commit | Closed behavior |
|---|---|---|
| F1 | `c587d5506` | Accept canonical SQL NULL callstack flags instead of classifying absence as an invalid flag. |
| F2 | `2968320d4` | Scope rejected callstack lanes to their producer; one producer cannot poison an unrelated producer's valid lane. |
| F3 | `f3d2722a5` | Localize name-only callstack rejection; an unusable display name does not erase independently valid producer rows. |
| F4 | `f8f80cb8b` | Preserve proven wakeup dependencies with unavailable emitter CPU in a versioned typed row; never substitute CPU 0. |
| F5 | `7c785245a` | Recover empty SQL thread display names from the immutable source cmdline segment under exact same-TID and ambiguity guards. |
| F6 | `5fb69fe56` | Add typed `SourceTables` lineage so consumed dependency tables are not falsely reported as wholly unhandled. |
| F7 | `efa31001a` | Consume authoritative `clock_event_filter`; carry exact CPU owner; cross-check equal generic IDs; poison conflicts per ID; forbid damaged-specialized fallback. |
| F8 | `33a70fa22` | Audit the physical `data_type(typeId,desc)` registry; prove supported argument kinds per exact type ID; localize malformed or unsupported kinds to their exact arg key. |
| F9 | `311f2fc2f` | Preserve admitted `frame_maps` rows as versioned typed relations with exact frame-row references and timestamps, without fabricating CPU, thread, duration, or B/E nesting. |
| F10 | `7c77a9265` | Preserve bounded `device_info` and `meta` values as display-only coverage metadata; keep parser/device metadata outside ftrace rows and all source-admission or hard-gate decisions. |

The fixes deliberately do not promise an exact recovered row count for this
customer capture: the returned report was produced by the pre-fix binary and
the raw `.sys` is unavailable locally. A new bounded diagnostic replay is the
only honest way to measure the post-fix counts.

### Follow-up batch closure and remaining verification

#### F8 — typed `data_type` dependency audit (P2)

Status: closed in `33a70fa22`.

Before F8, `args.datatype` was decoded through a closed hard-coded `0=int`,
`1=string` contract. The official registry also carries `2=double` and
`3=boolean`. Codrax must inspect the physical `data_type(typeId,desc)` registry,
prove the closed IDs it consumes, expose it as dependency lineage, and keep
unsupported value kinds local to their exact arg key. It must not mark the
table handled without actually reading and validating it.

The implementation now distinguishes an absent legacy table from a present
malformed table. Only exact official `(typeId,desc)` pairs acquire registry
authority; malformed, duplicate, conflicting, or unsupported IDs never trigger
legacy fallback and poison only argument keys that cite the affected ID.

#### F9 — `frame_maps` relation preservation (P2)

Status: closed in `311f2fc2f`.

Official `frame_maps(id,src_row,dst_row)` rows relate source and destination
`frame_slice` rows. Before F9, Codrax exported the individual frame intervals
but dropped this relation. The selected target is a typed bounded relation in the
tracebundle/tracequery evidence model, with strict row-ID referential checks.
It is not a synthetic duration and must not be rendered as a fake B/E span.

The implementation emits a versioned comment record that generic ftrace readers
ignore and `tracequery-v34` restores as `EventFrameMap`. Both endpoints must
reference frame rows already admitted by frame identity, lifecycle, and endpoint
CPU checks. Duplicate relation IDs, duplicate semantic edges, self-edges,
unsupported profiles, and unavailable endpoints fail closed. The relation
retains source and destination timestamps but makes no duration or causal
direction claim beyond the producer's `src_row`/`dst_row` labels.

#### F10 — device/parser metadata preservation (P3)

Status: closed in `7c77a9265`.

`device_info(physical_width,physical_height,physical_frame_rate)` and
`meta(name,value)` require bounded typed metadata coverage. They must
not become ftrace events. `physical_frame_rate` may later qualify frame
deadline interpretation only when its exact source and units are proven;
arbitrary `meta` keys remain display diagnostics, never hard-gate authority.

The implementation requires the exact official columns. `device_info` must be a
singleton and preserves only positive exact SQLite integers. `meta` is capped at
64 rows, 128 UTF-8 bytes per name, 1024 UTF-8 bytes per value, and 16 KiB total;
duplicate names are monotonically omitted. Values are published only in the
typed coverage `metadata` field with role `diagnostic_metadata`. Neither table
adds a systrace event. The raw frame-rate integer is intentionally not converted
to a frame deadline because this table alone does not prove its timing unit.

#### E2 — identical-capture parity fixture (still open)

Status: external verification remains open; it cannot be closed from the
pre-fix diagnostic report alone.

The post-fix customer replay should show:

- a material reduction in `callstack_source_rows_suppressed_pre_pairing`;
- nonzero emitted/preserved callstack rows unless the remaining 101 duration
  rows are the only rows present in a lane;
- reduced `scheduler_boundaries_with_unknown_comm` and
  `unresolved_thread_names` when the source cmdline roster is usable;
- `clock_event_filter` classified with exact CPU provenance and no duplicate
  generic publication;
- the dependency tables removed from the unclassified roster.

Only an identical binary input and output oracle can prove total event parity.
Counts from the two different Donghu text captures cannot substitute for that
test.

## Customer retry: mixed-precision output clock regression (2026-07-27)

Evidence:

- `/Users/han/opt/customlogs/cmd_result_002.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-retry.txt`;
- customer converter `0.1.20260727`, Windows amd64;
- input snapshot 27,022,926 bytes;
- embedded trace_streamer DB export completed in about 2 seconds;
- DB normalization ran for about 7.8 seconds and then failed with the exact
  postvalidation reason `tracequery_postvalidation_clock_regression`;
- no public systrace was published.

This retry excludes input-path, PATH installation, trace_streamer discovery,
binary decoding and DB-export failures. The failure is inside Codrax's
DB-to-systrace physical ordering contract.

### Confirmed mechanism

Codrax has two timestamp representations in one generated systrace:

1. standard ftrace envelope rows are rendered with six fractional digits;
   their source nanoseconds are rounded to microseconds on the wire;
2. versioned typed comment rows such as CPU-unavailable markers/wakeups and
   `frame_maps` relations carry an exact integer `ts_ns`.

Before `a1a801e3d`, the global sorter used the pre-render source nanoseconds for
both row classes. That key was not the timestamp the parser would later
recover from a standard row. Around a half-microsecond boundary this can invert
the parsed order in either direction:

- source standard `1499ns` renders as `1000ns`, while an exact typed `1200ns`
  row was sorted before it: parsed order `1200 -> 1000`;
- source standard `1501ns` renders as `2000ns`, while an exact typed `1800ns`
  row was sorted after it: parsed order `2000 -> 1800`.

The strict postvalidator correctly rejected this physically regressed output.
The gap was the sort key, not the validator. Removing or weakening
`tracequery_postvalidation_clock_regression` would publish a corrupt temporal
order and is explicitly forbidden.

The prior customer inventory contained nonempty `frame_maps`, and F9 added an
exact-nanosecond typed relation, so that row family is a plausible trigger for
this retry. The pre-fix report did not carry the first regressing row pair,
therefore it does not prove that `frame_maps` was the exact witness. The
systemic defect applied to every mixture of rounded standard rows and exact
typed rows and was fixed at their shared boundary.

### Batch G1 — wire-equivalent sorter authority

Status: closed and pushed as `a1a801e3d`.

- Standard ftrace rows now enter the global sorter with the exact nanosecond
  value represented by their six-decimal wire timestamp.
- Exact typed `ts_ns` rows retain nanosecond precision; no typed evidence is
  rounded away.
- Frozen raw-pairing rows use the same wire-equivalent key because their
  already-rendered line is a standard ftrace envelope.
- Equal rounded timestamps retain deterministic sequence/ingest ordering.
- Regression tests cover both half-microsecond inversion directions and
  reparse the final physical output through the production streaming parser.
- The full `internal/hitraceconv` suite passed.

Given the customer's exact failure code, this repair removes the identified
normalization blocker and should allow this capture to proceed to publication.
A post-fix customer replay remains required to prove that no later,
independent invariant fails and to obtain the new semantic-quality counts.

### Batch G2 — bounded first-regression witness

Status: closed and pushed as `0844f3b42`.

The original 15-line diagnostic report named the hard failure but did not
identify the first regressing row pair. The validator now records, from the
same O(1) parser pass, only:

- previous/current physical line number;
- previous/current parsed timestamp in seconds;
- previous/current typed event family.

The witness contains no event payload and no private staging path. It is
attached as `TraceClockRegressionWitnessError` through the existing error
graph and appears in the bounded diagnostic report as
`typed_error_clock_regression`. The report remains capped at 900 physical
lines. The successful path and the hard validation decision are unchanged.
Full `internal/hitraceconv` and `cmd` suites passed.

### Second customer replay and build-provenance gap

Evidence:

- `/Users/han/opt/customlogs/c.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-fixed.txt`.

The file named `converted-fixed.systrace` was not published. The run again
completed the 27,022,926-byte snapshot and trace_streamer DB export, then
failed the same `tracequery_postvalidation_clock_regression` gate.

This replay is **not** evidence that G1 failed. A build containing G2 must add
`typed_error_clock_regression` for this exact failure, because the witness is
attached at the same validator branch that increments the nonzero regression
count and survives the production provider error graph. The returned report
does not contain that typed record. Its only build identity is the day-level
`0.1.20260727`, which cannot distinguish multiple same-day commits. Therefore
the report proves that the customer binary does not contain the full
`main@0844f3b42` closure; it cannot prove whether the binary contains G1
`a1a801e3d` or predates both repairs.

The SQL production audit found no third standard-row publication path:

- validated standard envelopes use `traceDBStandardWireTimestampNS`;
- frozen raw-pairing rows use the same wire-equivalent key;
- the three versioned typed comment families retain exact `ts_ns`.

Profiler-container and built-in decoder sorters have separate direct
`renderedRow` sites, but they do not execute in this trace_streamer
DB-normalization failure path.

#### Batch G3 — exact build and capability provenance

Status: closed and pushed as `d759935de`.

- `make` injects the 12-character git revision, with an explicit dirty suffix;
  source archives without `.git` report `unknown` rather than inventing a
  commit.
- `codrax version` reports version, build time and revision.
- the bounded conversion diagnostic reports `build_time` and
  `build_revision`.
- it also publishes closed compile-time capabilities:
  `sql_mixed_precision_wire_sort_v1` and
  `clock_regression_first_witness_v1`.

The next customer report can now resolve the branch deterministically:

- missing `sql_mixed_precision_wire_sort_v1` means the runtime package does
  not contain G1;
- present sort capability plus a regression includes the first typed witness,
  proving a separate remaining sorter source;
- a successful run reaches semantic-quality coverage and can finally measure
  the name/span recovery batches.

### Customer collection encoding finding

`cmd_result_002.txt` is readable because the customer ran the conversion
directly and copied the PowerShell terminal transcript. Earlier mojibake was
introduced by Windows PowerShell 5.1 native-output capture (`2>&1`) decoding
Codrax's UTF-8 console bytes through the active legacy code page before
`Set-Content -Encoding UTF8` wrote the already-corrupted characters. Changing
only the final file encoding cannot reverse that damage.

The durable collection contract is therefore:

- run `codrax trace convert ... --diagnostic-report ...` directly;
- return the ASCII/JSON diagnostic report;
- if console context is needed, copy it directly from the terminal rather than
  piping native output through Windows PowerShell 5.1.

## Third customer replay: conversion succeeds, pipe-bearing callstack names remain a dominant loss (2026-07-27)

Evidence:

- `/Users/han/opt/customlogs/d.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-fixed2.txt`;
- the same 27,022,926-byte customer input;
- capabilities `sql_mixed_precision_wire_sort_v1` and
  `clock_regression_first_witness_v1`;
- build revision `unknown`, which is expected for a source-archive build
  without `.git`; the closed capability roster is the authoritative feature
  proof.

The conversion now succeeds end to end:

- trace_streamer DB export completed in about 2.1 seconds;
- normalization completed in about 8.3 seconds;
- 227,471/227,471 output events are known, authoritative and query-ready;
- strict tracequery cross-validation passed;
- the 38,670,872-byte systrace and the tracebundle were both published;
- no clock regression remains.

Compared with the earlier successful pre-fidelity replay (189,739 rows), the
same input now retains 37,732 additional rows, about 19.9%. This closes the
customer's conversion-blocking G1 issue and proves that the new build was used.

### H1 — callstack names containing `|` are still withheld (P1)

Status: confirmed, highest-leverage remaining fidelity batch.

The report reads 96,221 `callstack` rows but admits only 15,676 before pairing.
Of the 80,545 pre-pairing suppressions:

- 80,209 are `invalid_name_pipe`;
- 235 are `sync_with_async_identity`;
- 101 have invalid duration.

Thus 99.58% of the current pre-pairing loss is caused by one presentation-wire
restriction, not by missing identity, time, lifecycle or CPU evidence.

The current validator is locally correct for the legacy physical grammar:
placing an arbitrary DB name directly in
`B|pid|name` or `S|pid|name|cookie` makes the delimiter ambiguous. The system
gap is that Codrax treats "not representable by this legacy text grammar" as
"source evidence invalid" and has no exact CPU-known alternative. The
CPU-unavailable lane already proves the appropriate compatibility pattern:
a versioned comment with exact timestamp and base64 fields is ignored by
generic ftrace readers and reconstructed by Codrax as typed trace-mark
evidence.

Frozen repair:

1. retain strict `|` rejection for generic physical marker tokens and cookies;
2. admit `|` in a strictly bounded, nonblank, edge-trimmed, valid UTF-8
   `callstack.name`;
3. add a closed versioned exact trace-mark record carrying `ts_ns`, CPU,
   header TID/TGID, marker PID, action, comm, name and value;
4. use that record only for callstack B/E/S/F endpoints whose name cannot be
   represented by the legacy delimiter grammar;
5. keep both endpoints of one B/E span on the exact lane, so no generic reader
   sees an orphan physical end;
6. preserve namespace marker PID separately from host header TGID;
7. parse the exact record before the generic ftrace envelope at every timestamp
   and event-admission entry point;
8. expose `source_rows_preserved_exact_name` in conversion coverage;
9. bump the parser cache generation and pin sync/async, known/unavailable CPU,
   sorter and round-trip fixtures.

Forbidden shortcuts:

- do not replace or escape `|` in the source name;
- do not truncate names;
- do not reinterpret the final field by guessing;
- do not fabricate CPU 0;
- do not relax marker-token validation for other DB producers.

### H2 — unresolved thread names cannot use source cmdlines for this envelope (P2)

Status: confirmed diagnostic/format gap; implementation is intentionally
deferred until the envelope schema is proven.

The DB contains 1,294 thread rows, of which 139 remain unnamed. Scheduler output
therefore contains 12,985 boundaries with unknown comm. The source companion
reports:

`unsupported envelope magic=0xdf49 version=1 file_type=1`

The existing immutable-input cmdline scanner recognizes a different proven
envelope. Merely adding `0xdf49` to an allowlist would assign an unproven binary
schema to customer bytes and could fabricate TID/name mappings. The next batch
must first establish the producer envelope or add a bounded, non-payload
diagnostic inventory sufficient to prove its segment grammar. Recovered names
remain display-only and must never become identity, namespace, lifecycle or CPU
authority.

### H3 — upstream parser self-audit is degraded (P2 qualification)

Status: confirmed as an absence-confidence gap, not a conversion blocker.

trace_streamer reports 428,293 received records, zero reported drops, 19,850
`not_match` and two `invalid_data` records:

- `sched_blocked_reason:not_match=19,771`;
- `trace_vsync:not_match=79`;
- `tracing_mark_write:invalid_data=2`.

Positive parsed evidence remains usable. Absence-based conclusions for these
families are not complete and must stay qualified. The bounded report already
surfaces the exact typed counts; a later schema audit must determine whether
the records require a new parser/export lane or are intentionally unsupported
producer variants.

### Batch order after the successful replay

1. H1: exact pipe-bearing callstack-name preservation; this can recover up to
   80,209 source rows in this customer trace and is independent of H2.
2. H2: prove the `0xdf49` source envelope, then recover immutable cmdline names
   as display-only metadata.
3. H3: audit the three trace_streamer parser issue families using bounded
   producer-form witnesses; do not weaken positive-evidence admission.

## Invariants

- Never fabricate CPU, PID, TGID, comm, timestamp or lifecycle evidence.
- Namespace PID must not replace host ownership.
- Display-name recovery must not become identity authority.
- Quality ratios and counts are advisory unless a future hard gate reads a
  precise typed invariant.
- Production emits one selected trace body; diagnostic comparison must not
  merge two independently converted bodies.

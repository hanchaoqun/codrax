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
customer's conversion-blocking G1 issue and proves that a G1/G3-capable build
was used. It does **not** verify H1 or H2: the report capability roster contains
neither `callstack_exact_name_v1` nor
`source_cmdline_official_rawtrace_v1`. Therefore the 80,209
`invalid_name_pipe` suppressions and the `unsupported envelope
magic=0xdf49` message below are measurements from the intermediate G build,
not regressions of the already-pushed H1/H2 implementations.

### H1 — callstack names containing `|` are still withheld (P1)

Status: closed and pushed as `926c2700e`; identical-input customer
verification remains pending.

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
8. expose `source_rows_admitted_exact_name_pre_pairing` in conversion coverage;
9. bump the parser cache generation and pin sync/async, known/unavailable CPU,
   sorter and round-trip fixtures.

Implementation result:

- `# codrax_trace_mark_exact/v1` is a closed canonical wire with exact
  nanosecond timestamp, CPU, host TID/TGID, marker PID and base64 text fields;
- only callstack names containing `|` select this lane; all other producer
  marker-token contracts remain unchanged;
- CPU-unavailable pipe-bearing names continue through the existing typed
  unavailable lane without acquiring CPU 0;
- sync B/E publication stays under the single two-pass laminar authority and
  keeps both endpoints on the same exact lane;
- async S/F keeps its strict cookie and owner-generation audit;
- parser cache generation for that batch was `tracequery-v35`; H4d below
  advances the current generation to `tracequery-v36`;
- diagnostics expose capability `callstack_exact_name_v1` and the truthful
  pre-pairing metric `source_rows_admitted_exact_name_pre_pairing`;
- namespace PID, known/unavailable CPU, sync/async, in-memory/SQLite stage,
  exact timestamp sort and tracequery round-trip fixtures pass;
- `go vet ./internal/tracequery ./internal/hitraceconv ./cmd` and
  `go test ./... -count=1` pass.

For the same customer input, acceptance requires
`invalid_name_pipe` to disappear, the exact-name admission metric to become
nonzero, and strict cross-validation to remain green. Later laminar/lifecycle
suppression remains separately authoritative and must not be counted as
successful final emission merely because a name passed pre-pairing admission.

Forbidden shortcuts:

- do not replace or escape `|` in the source name;
- do not truncate names;
- do not reinterpret the final field by guessing;
- do not fabricate CPU 0;
- do not relax marker-token validation for other DB producers.

### H2 — unresolved thread names cannot use source cmdlines for this envelope (P2)

Status: closed and pushed as `6c2a7a2eb`; identical-input customer
verification remains pending.

The DB contains 1,294 thread rows, of which 139 remain unnamed. Scheduler output
therefore contains 12,985 boundaries with unknown comm. The source companion
reports:

`unsupported envelope magic=0xdf49 version=1 file_type=1`

The existing immutable-input cmdline scanner recognizes a different proven
envelope.

The schema is now proven directly from upstream
`openharmony/developtools_smartperf_host@260b028b`:

- `trace_streamer_selector.cpp` defines
  `RAW_TRACE_MAGIC_NUMBER = 57161`, exactly `0xdf49`;
- `common_types.h` defines the aligned `RawTraceFileHeader` as
  `uint16 magicNumber`, `uint8 fileType`, aligned `uint16 versionNumber` and
  `uint32 reserved`, which is the same 12-byte common header layout;
- the same file defines `CONTENT_TYPE_CMDLINES = 2`;
- `rawtrace_parser.cpp` consumes each following segment as `uint32 type`,
  `uint32 len`, then exactly `len` payload bytes;
- `FtraceProcessor::HandleCmdlines` consumes each payload row as
  `pid + space + taskName`.

This is not an allowlist guess. The customer header exactly matches the current
official TraceStreamer raw-trace envelope and V1 cmdline grammar.

The compatibility repair admits `0xdf49`, version 1, file type 0 or 1 only in
the bounded immutable-input cmdline companion scanner. The built-in
`0x0ace` Harmony RMQ page decoder remains closed and explicitly rejects
`0xdf49`, because the two file types still have different CPU page/event
layouts. Recovered names remain display-only and never become identity,
namespace, lifecycle or CPU authority.

Diagnostics expose capability `source_cmdline_official_rawtrace_v1` and metric
`source_envelope_official_rawtrace_v1=1`. The identical-input customer replay
must show the prior `unsupported envelope magic=0xdf49` line gone. Recovery is
then measured by `thread_names_recovered_source_cmdline`,
`unresolved_thread_names`, and
`scheduler_boundaries_with_unknown_comm`; absence of a cmdline row or an
ambiguous same-public-TID namespace roster still fails closed locally.

### H3 — decoded events fail upstream association and are not retained (P2 qualification)

Status: H3a typed semantics are implemented in this batch; H3b evidence
recovery remains open. This is an absence-confidence gap, not a conversion
blocker.

trace_streamer reports 428,293 received records, zero reported drops, 19,850
`not_match` and two `invalid_data` records:

- `sched_blocked_reason:not_match=19,771`;
- `trace_vsync:not_match=79`;
- `tracing_mark_write:invalid_data=2`.

The original “parser self-audit degraded” label is too broad for these rows.
Direct audit of upstream
`openharmony/developtools_smartperf_host@260b028b` establishes:

- raw and text sched-blocked-reason paths decode `pid`, caller and `io_wait`
  before calling the shared blocked-reason insertion path
  (`CpuDetailParser::SchedBlockReasonEvent` /
  `BytraceEventParser::BlockedReason`);
- `sched_blocked_reason:not_match` is incremented when
  `InsertBlockedReasonEvent` cannot attach that decoded event to a matching
  thread-state interval;
- on this failure, TraceStreamer does not append an independent raw or blocked
  DB row, so the 19,771 source records cannot be reconstructed from the
  exported SQLite DB;
- `trace_vsync:not_match` is likewise produced when
  `PrintEventParser::HandleFrameSliceEndEvent` cannot match a frame-end event
  to VSync state;
- `tracing_mark_write:invalid_data` is a union of trace-marker parse rejection
  and owner/process admission rejection; the current aggregate cannot
  distinguish the two.

H3a adds an optional typed `semantics` field on every surfaced capture issue
and capability `capture_issue_semantics_v1`. The exact customer cohorts become:

- `sched_blocked_reason/not_match`:
  `decoded_event_not_attached_to_thread_state; standalone_db_row_unavailable`;
- `trace_vsync/not_match`:
  `decoded_frame_end_not_matched_to_vsync_state`;
- `tracing_mark_write/invalid_data`:
  `trace_marker_parse_or_owner_admission_rejected`.

Generic `not_match` is explicitly described as downstream
association/pairing failure after event decoding, not raw binary parser
mismatch. Positive parsed evidence remains usable; absence-based conclusions
for these families remain qualified.

H3b cannot be honestly implemented against the current exported DB. Recovery
requires one of two new authorities:

1. an upstream TraceStreamer change that retains unmatched decoded
   blocked-reason/frame rows in a typed table; or
2. a bounded official `0xdf49` raw-page/event decoder driven by authoritative
   event-format metadata.

Counts alone must never be expanded into fabricated events. H3b stays open
until one of those authorities exists and has customer-format fixtures.

### Batch order after the successful replay

1. H1: closed in `926c2700e`; customer replay pending.
2. H2: closed in `6c2a7a2eb`; customer replay pending.
3. H3a: typed issue semantics implemented; customer replay must expose
   `capture_issue_semantics_v1`.
4. H3b: preserve/recover unmatched decoded evidence using an authoritative
   upstream-retention or official raw-page lane; do not infer rows from
   aggregate counts and do not weaken positive-evidence admission.

## Fourth customer replay: H1/H2/H3a verified, callstack lane quarantine is now the dominant loss (2026-07-27)

Evidence:

- `/Users/han/opt/customlogs/e.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-fixed_e.txt`;
- the same 27,022,926-byte customer input;
- all five capabilities are present:
  `sql_mixed_precision_wire_sort_v1`,
  `clock_regression_first_witness_v1`,
  `callstack_exact_name_v1`,
  `source_cmdline_official_rawtrace_v1`, and
  `capture_issue_semantics_v1`.

The conversion succeeds and strict tracequery cross-validation remains green:

- 287,151 authoritative/query-ready rows;
- 49,356,305-byte systrace;
- zero unknown, advisory or header-only rows;
- no output-clock regression;
- report length remains 85 physical lines, below the 900-line contract.

Relative to the preceding G-only replay, output increases from 227,471 to
287,151 rows: +59,680 rows (+26.2%). Relative to the earlier 189,739-row
pre-fidelity replay, the gain is 97,412 rows (+51.3%).

### H1 verification — exact-name admission works, but most newly admitted spans are quarantined later

H1 passes its direct acceptance checks:

- `invalid_name_pipe` disappears;
- `callstack_source_rows_admitted_exact_name_pre_pairing=80,209`;
- pre-pairing suppression falls from 80,545 to 336;
- emitted callstack endpoints rise from 29,026 to 88,706.

However, the shared sync authority now receives 95,885 callstack spans and
suppresses 51,532 of them. Only 44,353 spans publish. Before H1 it received
15,676 and suppressed 1,163. Thus H1 makes the hidden rows visible to the next
gate and exposes a separate amplification gap: 50,369 additional spans are
lost after successful source admission.

The direct witness is internally consistent:

- only 336 callstack rows fail before pairing:
  `invalid_duration=101` and `sync_with_async_identity=235`;
- those rows issue 336 callstack lane-poison declarations;
- 47 physical TID lanes become poisoned;
- producer-scoped lane poison then suppresses 51,532 otherwise admitted
  callstack spans and 103,064 endpoints.

This is not H1 failure and not a sorter/query-parser loss. It is a later
quarantine blast-radius failure.

### H2 verification — official RawTrace cmdlines recover almost all names

H2 passes:

- official envelope witness `source_envelope_official_rawtrace_v1=1`;
- one cmdline segment contains 10,513 rows;
- 10,372 unique rows are admitted, 103 same-name duplicates are compacted,
  and 38 malformed rows are locally rejected;
- 138 of 139 unnamed DB threads recover a display name;
- unresolved names fall from 139 to 1;
- scheduler boundaries with unknown comm fall from 12,985 to 816, a 93.7%
  reduction;
- no conflicting or namespace-ambiguous public-TID cohort is admitted.

The one unresolved display name and 816 boundaries remain disclosed. They do
not authorize identity inference.

### H3a verification — issue semantics are present and exact

H3a passes:

- `capture_issue_semantics_v1` is present;
- generic `not_match` provenance explicitly says association/pairing failure
  after event decoding, not raw binary parser mismatch;
- all three customer cohorts carry the expected event-specific typed
  semantics.

H3b remains open exactly as recorded above; this replay supplies no independent
rows from which Codrax could recover the 19,771 blocked-reason associations.

### H4-01 — Codrax interprets official `callstack` async/distributed fields incorrectly (P1)

Status: classification and quarantine-scope portion implemented as H4a and
verified by the fifth replay. H4d typed interval publication is now
implemented; its customer count is pending the next replay.

Current Codrax rules say:

- `flag=S/C` denotes separate async endpoints;
- nonzero `cookie` or `chainId` on an otherwise synchronous row is
  `sync_with_async_identity`;
- the rejected row poisons the complete callstack producer lane.

The official table contract says something different:

- a `callstack` row is an already-associated interval (`ts`, `dur`);
- non-NULL `cookie` identifies an async slice;
- `chainId`, `spanId`, `parentSpanId` and `flag=C/S` are distributed-call
  metadata attached to a slice;
- `AppendInternalAsyncSlice` stores the cookie on the row, while
  `FinishAsyncSlice` completes that same row's duration;
- `SetDistributeInfo` independently stores distributed metadata on a normal
  slice.

Therefore `chainId` is not a cookie and `flag=C/S` is not an S/F endpoint
action. The current `sync_with_async_identity=235` cohort includes legitimate
official shapes unless a narrower typed contradiction is proven.

Frozen repair boundary:

1. select row kind from strict cookie presence, not `chainId` or distributed
   role;
2. retain `chainId` and `flag` as bounded metadata only; they must not become
   endpoint pairing keys;
3. continue to use the row's exact interval for completed synchronous slices;
4. do not fabricate an async finish emitter/CPU: the high-level row preserves
   the completed interval and cookie but not necessarily both original
   physical endpoint emitters;
5. give completed async rows a typed interval authority before publishing S/F
   semantics;
6. split every rejection counter by official row kind so a future customer
   report cannot collapse distributed metadata and async evidence again.

H4a implementation result:

- strict non-NULL SQLite INTEGER `cookie` is now the only official async-row
  selector; integer zero is preserved as a valid cookie;
- `chainId` no longer substitutes for cookie or participates in endpoint
  pairing;
- `flag=C/S` with no cookie is admitted as distributed metadata on the
  completed synchronous interval, not interpreted as S/F;
- official `(ts,dur,cookie)` async intervals are withheld locally as
  `official_async_interval_endpoint_authority_unavailable`; they do not issue
  synchronous lane poison and do not enter legacy cross-row pairing;
- the pre-existing zero-duration `flag=S/C + cookie` compatibility shape
  remains isolated as a legacy endpoint lane; it is not used to classify
  official rows;
- coverage exposes
  `source_rows_official_async_shaped`,
  `source_rows_withheld_official_async_interval` and
  `source_rows_rejected_official_async_shape`, plus
  `source_rows_with_distributed_metadata`; all propagate to the semantic
  quality summary;
- diagnostics expose capability
  `callstack_official_field_semantics_v1`;
- fixtures pin distributed role, chain metadata, cookie zero, local async
  withholding, absence of sync-lane poison, legacy compatibility, lifecycle,
  namespace/identity and exact-name behavior.

The H4a implementation at this historical checkpoint intentionally did not
claim that withheld async intervals were emitted. H4d below supersedes that
boundary with a dedicated typed interval representation. H4a's immediate
customer value was removal of the false `sync_with_async_identity`
classification and its cross-span synchronous quarantine amplification.

### H4-02 — localizable bad callstack rows poison an entire TID history (P1)

Status: implemented and verified by the fifth customer replay.

The current stage stores callstack poison as one bit per physical header TID.
It has no timestamp or interval. One bad row therefore erases valid spans
before and after that row across the full capture.

The safe target is monotonic, typed localization:

- a row with exact lane and exact timestamp but unknown end creates a
  producer-scoped suffix fence from that timestamp, preserving earlier clean
  history;
- a row with an exact interval creates an overlap fence, not whole-history
  poison;
- an unlocalizable row retains the existing fail-closed lane/global behavior;
- other producers on the same physical B/E lane remain subject to the shared
  laminar audit and cannot be erased merely by callstack metadata noise;
- pass 1 still freezes all candidates and fences before pass 2 publishes any
  endpoint.

H4b implementation result:

- partial row parsing now carries two precise facts independently:
  `TimestampKnown` and `IntervalKnown`; a duration/type/overflow failure no
  longer erases an already-proven timestamp;
- exact positive intervals create a producer-scoped half-open overlap fence;
  candidates ending exactly at the fence start or starting exactly at its end
  remain eligible;
- timestamp-only rows, including invalid duration/overflow and rejected
  zero-duration points without a provable extent, create a suffix fence;
- rows without a trusted timestamp retain the old exact-lane poison. The
  invalid-timestamp fixture proves that both earlier and later same-lane
  callstack spans remain suppressed while another lane survives;
- fences are frozen beside candidates in the bounded stage. The memory backend
  sorts typed fence records in place; the SQLite backend uses a private
  `fence` table and pinned `fence_lane_idx` query plan;
- pass 1 merges overlapping intervals and suffix coverage under the existing
  active-depth/active-byte budgets, audits only surviving candidates and
  records bad lanes. Pass 2 independently re-reads the same sealed fence
  stream and applies the identical predicate before publishing;
- localization is producer scoped: a callstack fence cannot erase a syscall
  span on the same physical B/E lane. Surviving cross-producer geometry still
  passes the shared laminar/identity/depth audit and can therefore fail closed
  for an independent real contradiction;
- coverage now exposes `localized_fence_lanes`,
  `localized_fence_declarations` and
  `sync_spans_suppressed_by_local_fence`; diagnostics expose capability
  `callstack_time_local_fence_v1`;
- memory/SQLite parity fixtures pin prefix retention, interval-only overlap
  suppression, suffix suppression, other-producer retention and exact output
  parity. Exporter fixtures additionally pin dual-identity interval fences and
  the unlocalizable-time whole-lane fallback.

The fifth replay below closes the customer measurement. It does not authorize
removing the remaining local fences: surviving rows may still contain genuine
crossing, duplicate, identity or depth contradictions and must remain
fail-closed.

### H4-03 — CPU-unavailable exact spans remain Codrax-private in generic systrace consumers (P2)

Status: H4c-1 lossless standard subset implemented; CPU-unavailable and
non-microsecond/private-wire cohorts remain open.

`source_rows_preserved_cpu_unavailable` rises from 2,870 to 31,928 because H1
now admits the previously hidden cohorts. These spans are retained for Codrax
through typed exact comments and do not fabricate CPU 0, but a generic
systrace consumer does not understand Codrax's comment protocol.

For CPU-known synchronous B/E rows, the official Harmony begin grammar consumes
the entire tail as the name and explicitly parses pipe-separated trace
metadata. A blanket claim that every pipe-bearing B name is physically
ambiguous is therefore too broad. Standard-consumer parity needs a separate
compatibility lane, while exact nanosecond reconstruction and async trailing
cookie ambiguity remain protected.

CPU-unavailable generic placement cannot be recovered from this DB. It belongs
with the authoritative official raw-page lane, not a CPU-0 fallback.

H4c-1 adds the safe standard-consumer subset:

- an untagged Harmony synchronous B payload now consumes its complete tail as
  the opaque span name; S/F keeps the existing strict right-edge cookie rule;
- a CPU-known pipe-bearing synchronous span uses standard B/E only when both
  source timestamps are exact integer microseconds and the B payload
  round-trips to the identical name through the production parser;
- a final component that is an exact Harmony metadata token remains on the
  typed exact lane because the standard grammar would separate it from the
  name;
- any nanosecond remainder retains the typed exact lane, so generic
  compatibility never lowers Codrax timestamp authority;
- namespace PID remains the marker payload PID while the ftrace envelope keeps
  the proven host TID/TGID;
- coverage publishes `standard_sync_pipe_spans_emitted` per producer and on
  the shared authority; diagnostics advertise
  `standard_sync_pipe_compat_v1`.

This is deliberately not a dual-publication scheme: each source endpoint has
exactly one wire representation, so Codrax never sees a duplicate standard and
typed marker.

### H4-04 — completed official async intervals need a non-fabricating duration authority (P1)

Status: H4d implemented; customer replay pending.

The official `callstack(ts,dur,cookie)` row proves a completed logical async
interval but does not preserve a physical finish emitter or finish CPU.
Recreating two ordinary S/F rows would therefore invent endpoint provenance.

H4d adds `# codrax_trace_async_interval/v1`, one physical comment row per
official interval:

- exact start/end nanoseconds, stable source row, start emitter TID/host TGID,
  marker namespace PID, name and cookie are retained;
- start CPU is either one exact physical CPU or typed unavailable with the
  existing closed reason set;
- `finish_emitter_status=unavailable` and
  `finish_cpu_status=unavailable` are explicit typed fields; no S/F or B/E
  endpoint is synthesized;
- lifecycle admission is a point gate on the proven start emitter. The
  producer-completed logical interval does not claim that this emitter
  remained alive or running until the async finish;
- a unique same-public-TID scheduler alias may supply the host header/start
  CPU while the marker payload keeps the namespace PID; ambiguity remains
  typed CPU-unavailable;
- tracequery v36 reconstructs `EventTraceAsyncInterval`, projects it directly
  into `TraceSpanSummary{kind=async}`, clips it to query windows, exposes it in
  event search/span windows, and admits it to trace-mark carry discovery as
  one completed interval—not as a two-endpoint pair;
- full-index-derived and cold window gates keep an interval whose start is
  before the requested time when its exact end overlaps the window;
- sparse warm-parse anchors carry a running maximum completed-interval end,
  so a large-file seek cannot jump past a single-row carry-in interval;
- positive-duration `flag=S/C + cookie` rows are also completed typed
  intervals (the flag remains distributed-role metadata); only the frozen
  zero-duration S/C shape enters the legacy endpoint-pair lane;
- coverage publishes `source_rows_emitted_official_async_interval`;
  diagnostics advertise `callstack_completed_async_interval_v1`.

The existing zero-duration `flag=S/C + cookie` compatibility lane remains a
real endpoint-pair lane and is unchanged. Invalid duration, identity, name,
cookie, lifecycle or source-row shapes remain locally rejected and cannot
taint the synchronous B/E authority.

## Fifth customer replay: H4a/H4b verified and remaining work frozen (2026-07-27)

Evidence:

- `/Users/han/opt/customlogs/g.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-fixed_g.txt`;
- the same 27,022,926-byte customer input;
- converter capability set includes
  `callstack_official_field_semantics_v1` and
  `callstack_time_local_fence_v1`.

H4a passes:

- 264 rows have the official completed-async shape;
- 253 valid completed intervals are locally withheld pending H4d;
- 11 malformed official async shapes remain locally rejected;
- the former `sync_with_async_identity` false classification is absent;
- the synchronous authority is no longer globally contaminated by these
  official async rows.

H4b passes:

- 90 rejected callstack declarations become timestamp-local fences across 41
  physical lanes;
- synchronous span suppression falls from 51,532 to 1,262, a 97.55% reduction;
- 95,867 callstack span candidates are reconciled to 94,605 published spans
  and exactly 189,210 endpoints; the authority row reports 191,654 total
  endpoints when the 2,444 thread-registration endpoints are included;
- the final artifact contains 387,655 known rows, zero advisory/unknown rows,
  and strict tracequery cross-validation reads all 387,655 without an
  unparsed row or clock regression;
- exact-name admission remains 80,209 and CPU-unavailable preservation remains
  typed at 31,912 rows.

The authoritative reconciliation identities are:
`suppressed_endpoints=2,524`, `suppressed_spans=1,262`,
`rows_emitted=191,654`, and the producer-specific submitted/emitted counters
in the diagnostic report. No rounded headline is allowed to replace those
typed counters.

The remaining 1,262 spans are not an automatic recovery target. They overlap
one of the 90 exact rejected-row fences or fail another frozen lane invariant;
emitting them without better source evidence would reopen a correctness bug.

### Frozen task list and batch boundaries

| Batch | Priority | Scope | Implementable now | Closure condition |
| --- | --- | --- | --- | --- |
| H4c-1 | P1 compatibility | Publish a standard B/E representation for CPU-known synchronous pipe-bearing names only when its printed timestamp is exactly equal to the source nanosecond timestamp; retain the private exact lane otherwise | implemented | generic reader sees the eligible spans; Codrax parses the full pipe-bearing B name; byte/exact-time and strict cross-validation fixtures pass |
| H4d | P1 evidence | Add a versioned typed completed-async interval carrying start, duration/end, source emitter identity, owner/namespace PID, name and cookie without inventing a finish emitter or CPU | implemented | next customer replay reports the valid cohort under `source_rows_emitted_official_async_interval`; invalid rows stay rejected; no synthetic S/F endpoint is emitted |
| E2a | P1 verification | Build an identical-input deterministic conversion receipt/fixture using the checked-in conversion pipeline and pin row-family accounting | implemented for synthetic integration authority | the fixture proves immutable child-input SHA parity, deterministic output/receipt SHA, all DB-family accounting and tracequery event counts; customer binary parity remains external until a redistributable real `.sys` oracle is available |
| H3b-0/H4e-0 | P2 diagnostics | Inventory the official `0xdf49` segment/page/event-format authority and surface unsupported raw cohorts with typed reasons and bounded witnesses | implemented | diagnostics distinguish absent/empty/nonempty format and raw payloads, disclose exact decoder authority, and never synthesize events from aggregate counts |
| H3b/H4e | P1 fidelity | Decode unmatched blocked-reason/VSync records and recover physical CPU placement from official raw pages | blocked on authority | requires authoritative event-format metadata plus a customer-format fixture or an upstream TraceStreamer retention table |
| P3 cleanup | P3 | One unresolved name, 816 unknown-comm boundaries, two invalid markers, build revision and residual raw/DMA coverage | evidence-dependent | each item either gains a typed source authority or remains explicitly unavailable |

The batches are intentionally independent and must be committed and pushed
separately. H4c-1 is a safe subset, not a claim of full generic parity:
non-microsecond exact timestamps, CPU-unavailable rows, and S/F rows with a
trailing cookie remain on typed private wires until a lossless standard
representation exists.

### E2a result — identical-input accounting fixture

`TestSameInputTraceStreamerAccountingReceiptIsDeterministic` now runs the
complete checked-in SQL conversion pipeline twice over one fixed synthetic
`.sys` generation and one fixed TraceStreamer DB authority. The fake external
tool copies the exact private input argument it consumed, allowing the test to
prove:

- source and child-consumed snapshot are both 8,442 bytes with SHA-256
  `6294cbbff9509cc1458771f83f0c44d49a224eeead56b4a2e49aa8c64b0271ab`;
- both runs produce the same 2,819-byte systrace SHA, 22 receipt-validated
  known events, the same sorted DB-family read/emitted/skipped/metric ledger,
  and the same tracequery event-type census;
- sorter `rows_read=rows_emitted=22`, artifact
  `rows=known=22`, and cross-validation `rows_emitted=22`;
- the canonical path/time-independent accounting receipt is itself pinned by
  SHA, so a family counter, rejection reason, output wire or event census
  change must be reviewed explicitly.

This is deterministic pipeline/accounting evidence, not real-format parity.
The existing representative-fixture gate still refuses a synthetic capture as
retirement evidence. A redistributable real customer/vendor `.sys` plus its
approved hash remains necessary to close the external parity portion.

### H3b-0/H4e-0 result — bounded source raw-authority inventory

The immutable-input companion scan now emits
`source_rawtrace_authority/__source_segments__` and diagnostics advertise
`source_rawtrace_authority_inventory_v1`. It reads only the common V1
container envelope; it does not decode an official raw page.

The coverage row reports:

- exact admitted envelope profile (`official_rawtrace_v1` for magic `0xdf49`,
  or the legacy RMQ profile), version, file type and CPU-count hint;
- a range-checked segment census and byte totals for event-format, cmdline,
  TGID, raw-trace, header-page, printk, kallsyms and unknown families;
- independent `event_format_state` and `raw_payload_state` values:
  `absent`, `present_empty`, or `present_nonempty_unvalidated`;
- `decode_authority` values that distinguish no payload, empty payload,
  missing/empty event-format authority, incomplete segment inventory, legacy
  strict RMQ availability, and
  `unavailable_official_page_decoder_not_implemented`;
- for a nonempty official raw cohort, the explicit recovery requirement
  `requires_official_page_decoder_or_upstream_retained_rows`;
- a maximum of 4,096 segment records and 16 distinct unknown-type witnesses;
  excess types/records are counted and the inventory fails closed.

`rows_read/rows_emitted` on this face count audited segment records, not pages
or events. Incomplete inventory publishes zero emitted segment records and one
typed reason. Most importantly, the coverage explicitly says that aggregate
`stat.not_match` counts cannot be mapped back into source records. Thus the
19,771 `sched_blocked_reason` and 79 `trace_vsync` failures remain correctly
classified as upstream post-decode association failures; this batch tells the
next replay whether recoverable raw bytes plus format material are physically
present, but it does not fabricate those lost DB associations.

### Execution order

1. commit this replay/task ledger;
2. H4c-1 standard synchronous compatibility;
3. H4d completed-async typed interval;
4. E2a same-input accounting fixture;
5. H3b-0/H4e-0 bounded raw-authority diagnostics;
6. run the full converter/tracequery suite, refresh this ledger, then request
   one customer replay only after every implementable batch is on `main`.

## Pre-replay closure audit: implementable residuals completed (2026-07-27)

This audit re-read the fifth customer diagnostic against current `main`. It
separates a recoverable implementation omission from source evidence that is
already absent after TraceStreamer export. “Can write code” is not sufficient
authority to publish a trace event: an implementation is classified as
implementable only when the current immutable input or retained DB proves all
fields required by that event family.

### Completed batches

| Batch | Commit | Result |
| --- | --- | --- |
| H4c-1 | `a5abc3cb3` | lossless standard B/E compatibility for the exact CPU-known, integer-microsecond synchronous subset |
| H4d | `0ddbc5444` | typed completed async interval with no fabricated finish emitter/CPU |
| E2a | `7a6f57757` | deterministic same-input child snapshot, output, family-accounting and tracequery receipt |
| H3b-0/H4e-0 | `5aa05af5a` | bounded official raw-envelope/segment/format authority inventory |
| P3a | `10d1ce2c5`, `f665d0005` | closed source-cmdline rejection reasons plus bounded unresolved canonical identity and unknown-comm subject witnesses |
| P3b | `05eaf37ce` | independent build revision provenance and exact running-executable SHA-256 fingerprint |

P3a does not change name selection. It adds:

- one closed reason counter for every rejected cmdline row
  (`invalid_length`, `missing_tid_name_separator`, `invalid_tid`,
  `empty_name`, `placeholder_name`, or `invalid_display_name`);
- at most 16 sorted ambiguous source TIDs;
- at most 16 sorted unresolved canonical
  `itid/tid/ipid/switch_count/source_cmdline_state` witnesses;
- at most 16 sorted unique `itid/tid/tgid` subjects participating in an
  unknown-comm scheduler boundary, plus exact first/last boundary timestamps
  and total unique-subject count;
- explicit capability `unresolved_trace_identity_witnesses_v1`.

All values are diagnostic metadata. They do not become identity, namespace,
lifecycle, CPU, event, or name authority. In particular, a namespace/public
TID collision remains fail-closed.

P3b does not invent a commit for an archive build. Revision selection is:

1. exact linker-provided revision;
2. exact Go `vcs.revision` build setting, with its dirty bit;
3. typed `unavailable`.

Independently, the diagnostic hashes the running executable and publishes its
full SHA-256 with `executable_hash_status`. Therefore a replay can prove that
the customer changed binaries even when the source package did not contain
`.git`. Capability: `executable_build_fingerprint_v1`.

### Residuals that are not recoverable from the retained DB

| Residual in fifth replay | Current exact evidence | Why no safe implementation exists yet | Required authority |
| --- | --- | --- | --- |
| `sched_blocked_reason:not_match=19771` | producer decoded pid/caller/io_wait, then failed to attach the event to a thread-state row; no standalone retained row | the aggregate count cannot reconstruct timestamp, subject, state interval, or CPU | official raw page decoder using the capture's format metadata, or an upstream retained decoded row |
| `trace_vsync:not_match=79` | producer decoded a frame-end record but failed VSync-state association | count cannot reconstruct the missing frame/VSync endpoint or its identity | same raw-page/upstream-retention authority |
| `tracing_mark_write:invalid_data=2` | only an aggregate parser/owner-admission rejection survives | neither marker bytes, timestamp, owner nor marker grammar branch is retained | raw-page record plus format, or upstream rejected-row retention |
| `raw.rows=66856`, missing `argset/argsetid` | exact timestamp/name/cpu/itid columns exist, but event arguments are absent | publishing standard raw ftrace would claim an incomplete payload; duplicating already-derived high-level rows as argument-less typed events would distort event accounting without restoring spans | an argument-bearing raw table or official raw-page decode |
| high-level `dma_fence.rows=1787` | timestamp, category, driver/timeline/context/seqno and predecessor delta; no emitter/CPU | `dur` is not wait duration and cannot form a span; CPU/TID cannot be defaulted | strict raw DMA start/end rows with args, emitter identity and CPU |
| 1,262 locally fenced synchronous spans | exact rejected-row time fences overlap these candidates | this is deliberate fail-closed suppression after H4b, not an unimplemented bulk-recovery path | corrected producer rows or independent raw authority proving the rejected declarations |
| CPU-unavailable spans | exact interval and identity are retained on Codrax typed wires | generic systrace cannot place them on a physical CPU without invention | official raw CPU/envelope authority; Codrax query fidelity is already retained |

The safe action for these rows is typed absence plus a recovery requirement,
not a guessed event. H3b/H4e therefore remains blocked on evidence, not on an
unwritten local patch.

### Replay gate

All implementation work justified by the fifth replay is now on `main`.
The next customer replay is authorized only with a binary containing all
capabilities below:

- `standard_sync_pipe_compat_v1`;
- `callstack_completed_async_interval_v1`;
- `source_rawtrace_authority_inventory_v1`;
- `executable_build_fingerprint_v1`;
- `unresolved_trace_identity_witnesses_v1`.

The replay must verify:

1. `build_identity.executable_sha256` is present and differs from the fifth
   replay's old binary;
2. valid official async intervals move from
   `source_rows_withheld_official_async_interval` to
   `source_rows_emitted_official_async_interval`;
3. eligible standard synchronous pipe spans report
   `standard_sync_pipe_spans_emitted`;
4. `source_rawtrace_authority/__source_segments__` states whether both
   event-format and raw payload segments are physically present;
5. the remaining unresolved thread and the 816 affected scheduler boundaries
   are reduced to bounded exact TID/ITID witnesses, so the next decision can
   target one authority failure instead of requesting another broad log.

No further customer data is needed before this replay. If the raw-authority
row reports nonempty official raw payload plus nonempty event-format material,
the next engineering batch is an official page decoder against that exact
profile. If either is absent, recovery must be requested from TraceStreamer
upstream rather than attempted from the normalized DB.

## Sixth customer replay and RPD-0 raw-page profile probe (2026-07-27)

The sixth replay used the expected new executable
(`sha256=9e73484500d74e307efcdfe6c4fd1594cc5cadbc653fa58dbf7dd89bd6057afa`)
and proves that H4d worked without an accounting leak:

- `253` official completed async intervals were emitted;
- total events increased from `387655` to `387908`, exactly `+253`;
- callstack rows suppressed before pairing fell from `354` to `101`;
- the standard synchronous compatibility metric was absent because this input
  contains no row satisfying the exact CPU-known plus integer-microsecond
  compatibility subset, not because the wire was absent;
- all `816` scheduler boundaries with unknown comm belong to one canonical
  subject, `itid=398/tid=29352/tgid=68`; the source cmdline is absent, so this
  witness does not prove a namespace collision or authorize a guessed name.

The same immutable input also proves that recovery material exists before the
normalized DB:

| Source fact | Sixth-replay value |
| --- | --- |
| envelope | `magic=0xdf49`, `version=1`, `file_type=1`, `cpu_count=12` |
| event-format material | one segment, `30,993` bytes |
| raw payload | one segment, `26,066,944` bytes = `6,364 × 4,096` |
| other segment | type `33`, `90` bytes |
| DB association losses | `sched_blocked_reason:not_match=19,771`; `trace_vsync:not_match=79` |
| marker admission losses | `tracing_mark_write:invalid_data=2` |

The aggregate `not_match` values remain post-decode association failures, not
binary parse failures. They cannot be reconstructed from counts. The raw
payload may contain the missing record-level authority, but publishing it
requires proving the page/record profile first.

### RPD-0 implementation

RPD-0 adds capability `official_raw_page_profile_probe_v1` and one
`source_rawtrace_profile/__raw_page_probe__` diagnostic row. It is deliberately
non-publishing: `RowsEmitted` is always zero and no observed value can become
event, timestamp, CPU, identity, lifecycle, namespace, display-name or causal
authority.

The probe is closed and bounded:

- only official `0xdf49`, version `1`, file type `0|1` envelopes are eligible;
- event-format input is capped at `128` segments and `16 MiB`, parsed by the
  existing strict tracefs descriptor parser;
- raw inspection is capped at `131,072` pages;
- every page must satisfy the candidate qword timestamp/qword logical-length/
  byte-CPU geometry and every record must satisfy checked header, alignment,
  bounds and timestamp arithmetic;
- exact event-ID matches are counted against the admitted format catalog;
- at most `32` target format witnesses are surfaced for scheduler,
  blocked-reason, VSync, marker and DMA-fence families;
- at most `4` unknown segments of at most `4,096` bytes receive an exact
  SHA-256 witness; safe UTF-8 text is shown only when at most `256` bytes;
- cap, read, parse, layout, CPU, bounds and arithmetic failures become typed
  diagnostic states and never degrade the existing TraceStreamer conversion.

`decoder_readiness=structural_candidate_requires_fixture_parity` is emitted
only when all probed pages match the candidate geometry, at least one record is
present, and at least one record ID matches a strictly admitted event format.
This still does not authorize publication: it only selects the RPD-1 decoder
profile for parity testing. Any inconsistent page yields
`page_layout_state=candidate_rejected` and
`decoder_readiness=requires_different_page_layout`.

### Frozen next batches

The state column below records the RPD-0 commit-time gate; the seventh-replay
section immediately following it is the current superseding state.

| Batch | State | Entry condition | Scope |
| --- | --- | --- | --- |
| RPD-0 | implemented | sixth replay proves raw plus format material | bounded non-publishing profile and type-33 witness |
| RPD-1 | waiting for one replay | all-page candidate plus record/format matches | independent typed raw decode, fixture parity and family accounting; still no bulk publication |
| RPD-1-alt | waiting for one replay | candidate rejected | implement the exact observed page profile; do not loosen the candidate parser |
| RPD-2 | blocked by RPD-1 evidence | per-family identity, CPU, timestamp and argument parity proven | publish only independently proven missing families with duplicate suppression |

The next replay needs only the new capability and the
`source_rawtrace_profile` coverage row. It does not need another broad customer
log. RPD-1 must not start by assuming that the official producer reused the
legacy page geometry.

## Seventh customer replay and RPD-1 typed decode ledger (2026-07-28)

The customer replay used the RPD-0 binary
(`sha256=0c72c304c2c41b4b10516f9b25322226cf271441f13da00f37e11159748d4fdf`)
and satisfies the RPD-1 flagship entry condition:

| RPD-0 proof | Exact result |
| --- | --- |
| page geometry | `6,364/6,364` pages match; zero invalid page |
| CPU roster | exact `0..11` |
| event-format catalog | `43` admitted; all `43` have exact common type |
| record/format accounting | `491,411/491,411` physical records match an admitted format |
| target physical records | blocked reason `21,566`; marker `7,518`; DMA fence `2,085`; wakeup-new `120` |
| type 33 | kernel `HongMeng Kernel 1.12.0`, Unix/boot clock anchors, exact SHA-256 witness |
| publication effect | zero; systrace remains `387,908` events and `69,744,973` bytes |

The bounded target roster contains only ten exact format names and did not hit
its cap. Therefore exact `sched_switch`, `trace_vsync`, `sched_wakeup` and
`sched_waking` formats are absent from this raw-ftrace catalog. This is
especially important for `trace_vsync:not_match=79`: those losses cannot be
assigned to the raw-ftrace decoder merely because another raw family is
recoverable.

The strongest cross-layer witness is already exact:

```text
raw sched_blocked_reason physical records = 21,566
TraceStreamer DB rows emitted             =  1,795
TraceStreamer not_match                   = 19,771
                                               ------
                                               21,566
```

This proves that the `19,771` count is the missing association subset of one
physically complete raw event family. It still does not authorize blindly
adding every raw row: RPD-2 must prove strict body admission and suppress the
`1,795` DB-derived duplicates using an exact event key.

### RPD-1 implementation

Capability `official_raw_record_decode_ledger_v1` adds two non-publishing
coverage faces:

1. `source_rawtrace_decode/__raw_record_decode__`
   (`diagnostic_ledger`);
2. `source_rawtrace_reconciliation/__raw_vs_trace_streamer__`
   (`diagnostic_reconciliation`).

The decode ledger reuses the already proven immutable page traversal but gains
no publication authority from it:

- all physical records retain exact format-ID counts and a sorted format
  roster;
- any poisoned format ID, unmatched physical record, or record-census mismatch
  withdraws ledger completion;
- only the closed target registry receives body decoding;
- every decoded target requires the exact common PID/flags/preempt envelope;
- scheduler-core, exact scheduler-switch, marker and DMA-wait decoders use
  their existing strict typed admission; non-wait DMA legacy rendering does
  not gain authority;
- body admitted/rejected/unsupported counters are separated per exact target;
- target body decoding is capped at `1,000,000` rows and the format roster at
  `64`; either cap withdraws completion rather than publishing prefix-biased
  conclusions;
- unsafe or oversized event-format names are represented only by exact
  SHA-256/byte-length witnesses;
- raw and target timestamp bounds are diagnostic only;
- `RowsEmitted=0` is invariant.

The TraceStreamer stat reader now retains selected event statuses as one exact
five-bit roster plus nonzero counts. Thus a reported zero is distinguishable
from an absent status without adding dozens of `=0` values to the customer
console. Reconciliation computes, per exact family:

- raw physical count;
- TraceStreamer `received/data_lost/not_match/not_supported/invalid_data`;
- a provisional raw-versus-stat closure;
- for blocked reason only, a provisional DB-emitted-versus-received closure;
- explicit non-equivalence for high-level DMA activity rows;
- explicit `raw_format_absent` recovery state for VSync.

No equality changes conversion admission. The result is a typed RPD-2 task
selector, not a second trace provider.

### RPD-2 decision gate

After one replay containing `official_raw_record_decode_ledger_v1`:

| Family | May enter RPD-2 only when |
| --- | --- |
| blocked reason | all raw bodies admitted; the exact event-specific counter equations close; an exact raw/DB duplicate key is unique |
| tracing marker | the physical `print` plus `tracing_mark_write` group closes against `received`; admitted/rejected bodies, marker ownership and duplicate suppression are proven |
| DMA wait | start/end strict bodies plus pair-key reconciliation close; high-level DB rows are never subtracted as if equivalent |
| VSync | a different retained source segment or upstream decoded row is identified; raw-ftrace absence remains fail-closed |
| scheduler | an exact format/profile exists; no alias is inferred from DB scheduler rows |

RPD-1 itself is complete without another customer capture. The next customer
run is a short decision replay after the new binary is pushed; only the
diagnostic report is required.

## RPD-1 customer replay correction and RPD-1b (2026-07-28)

The RPD-1 customer replay succeeded and remained deliberately non-publishing:
the output is still `387,908` events, `69,744,973` bytes, with SHA-256
`c03eac1dd980cc4c34a63ceab0f2f676a79ca121d138e4e37d50a64eb17273b2`.
The raw ledger scanned all `491,411` physical records, hit no page, format,
record or witness cap, and emitted zero rows.

The replay invalidated one RPD-1 assumption: TraceStreamer stat statuses are
not one globally disjoint partition and must never be summed generically.
Inspection of the matching upstream parser implementation confirms that each
event handler decides where counters are incremented:

- rawtrace `sched_blocked_reason` increments `received` once before
  association, increments it again after a successful thread-state
  attachment, and increments `not_match` after a failed attachment;
- `PrintEventParser` increments `tracing_mark_write:received` once for every
  `print` or `tracing_mark_write` input, then may additionally increment
  `invalid_data`;
- rawtrace `sched_wakeup_new` and the four simple DMA events increment
  `received` once per physical event.

Therefore the old five-status `stat_sum` and
`DB emitted == received` comparisons were invalid diagnostic logic, not
conversion loss. The customer numbers instead close under the exact
event-specific equations:

```text
blocked raw physical                    21,566
blocked DB rows emitted                  1,795
blocked not_match                       19,771
DB emitted + not_match                  21,566  = raw
raw + DB emitted                        23,361  = received

raw print                              175,165
raw tracing_mark_write                   7,518
print + tracing_mark_write             182,683  = marker received
marker invalid_data                          2  (overlapping subset, not additive)
```

Other exact replay findings:

- all `21,566` blocked-reason bodies pass Codrax's strict envelope and body
  admission;
- all `149` DMA wait-start and `149` wait-end bodies pass strict admission;
- all `120` `sched_wakeup_new` records pass the envelope but fail the old
  four-byte-only priority gate; upstream has an explicit signed 16-bit/32-bit
  priority profile, so this is a Codrax decoder compatibility gap;
- exact `sched_switch`, `sched_wakeup` and `sched_waking` formats are absent,
  while the admitted catalog contains `sched_switch_lite=117,226` and
  `sched_wakeup_lite=66,736`; this is a separate closed-format compatibility
  batch, not authority to alias those rows blindly;
- exact `trace_vsync` remains absent from raw ftrace. Its `received=389` and
  `not_match=79` belong to another retained/decoded source and remain outside
  raw-ftrace recovery;
- non-wait DMA raw bodies remain unsupported by the strict Codrax renderer,
  and the `1,787` high-level DB activity rows remain non-equivalent.

RPD-1b implements the following non-publishing correction:

1. capability `official_raw_record_reconciliation_v2` identifies corrected
   reports;
2. stat statuses are surfaced individually and are never generically summed;
3. direct one-per-event relations are restricted to a closed family roster;
4. blocked reason uses the two exact equations above and explicitly reports
   that duplicate-key proof is still missing;
5. `print` joins the closed raw target roster so marker physical/body
   accounting covers both upstream input names;
6. bounded target descriptor geometry exposes exact field
   name/offset/size/signed witnesses without exposing print format text;
7. the exact wakeup family accepts only the upstream signed 16-bit or signed
   32-bit priority profiles; other signed fields keep their existing width
   gates.

RPD-1b still emits no recovered source row. In particular, count closure does
not prove a one-to-one blocked duplicate key, marker stack ownership, DMA pair
identity, namespace ownership, or VSync provenance.

### Frozen post-RPD-1b batches

| Batch | Priority | State | Scope and exit condition |
| --- | --- | --- | --- |
| RPD-1b | P0 | implemented | correct counter semantics, include `print`, expose bounded geometry, accept exact 16/32-bit wake priority; zero recovered rows |
| RPD-2A | P0 | diagnostic ledger implemented | construct bounded raw/DB blocked-reason content-key ledgers and prove the safe raw-only subset; no row is published |
| RPD-2A-PUB | P0 | implemented | publish only content cohorts absent from DB after exact raw coordinates plus target/header lifecycle and namespace-safe identity proof |
| RPD-2B | P1 | implemented for synchronous B/E | retain exact marker endpoints and poison witnesses, reconstruct clean emitter-local LIFO pairs, suppress exact existing DB candidates and submit only DB-disjoint pairs to the shared laminar authority; async/counter/instant marker lanes remain separate follow-up scope |
| RPD-2C | P1 | implemented | retain exact raw DMA wait endpoints and publish only complete, lifecycle-admitted, wire-representable clean pair lanes; never subtract high-level DMA DB activity as if it were a raw endpoint |
| RPD-CAP1 | P0 | implemented | raise the bounded strict-decode census from 250,000 to 1,000,000 after the complete RPD-1 closed target roster proved 390,416 rows |
| RPD-LITE-D | P1 | implemented | strict non-publishing decode and bounded retention for exact `sched_switch_lite` and `sched_wakeup_lite` profiles |
| RPD-LITE-JS | P1 | implemented | enrich only a one-raw/one-DB `sched_switch_lite` boundary with exact switch-out priority, header flags and full next-info receipt; emit no second event |
| RPD-LITE-JW | P1 | implemented | enrich only a one-raw/one-DB `sched_wakeup_lite` edge with exact positive wake priority, source/target CPU and header envelope; never use name-based aliasing |
| RPD-VSYNC | P1 | blocked by source evidence | identify the non-raw source segment or an upstream retained row for the 79 unmatched frame ends |

No further customer capture is needed before implementing the deterministic
RPD-2A key ledger and the closed LITE decoder profiles. The next customer
replay should wait until those locally implementable batches have landed, and
then needs only the diagnostic report plus the produced systrace receipt.

### RPD-2A exact content-cohort ledger

Cold comparison of the source record and DB exporter proves that a
one-to-one duplicate key cannot be reconstructed from TraceStreamer's current
DB:

- raw retains the blocked event's exact timestamp, CPU, common PID, target
  TID, iowait, caller and optional delay;
- DB retains target TID, iowait and caller, but its timestamp is the
  `thread_state.ts` projection, its CPU is the preceding sched-slice
  projection, its header thread is the blocked subject projection, and the
  raw parser discards the optional delay before DB insertion.

Consequently, subtracting one DB count from one matching raw count would be a
fabricated one-to-one association. RPD-2A instead uses an exact comparable
content cohort:

```text
(target_tid, iowait, canonical caller symbol-or-raw-address)
```

The ledger retains every admitted raw blocked record internally under the
existing `1,000,000` target-row cap and compares the full raw/DB multisets:

- every DB key must exist in raw;
- each DB key count must not exceed its raw count;
- any mismatch withdraws the ledger;
- a raw cohort which appears at least once in DB is withheld in full;
- only raw cohorts absent from DB are eligible for later publication;
- each eligible row must resolve both its target TID and exact common PID to
  one canonical host thread/process at the raw timestamp and pass the shared
  lifecycle point gate;
- missing, rejected or multiple lifecycle-valid candidates fail closed and
  are never rewritten as a host/namespace PID.

Capability `official_raw_blocked_key_ledger_v1` and coverage family
`source_rawtrace_blocked_key/__raw_vs_db_blocked_key__` expose only bounded
counts and deterministic SHA-256 multiset receipts. Caller values and raw
record identities are not printed. `RowsEmitted=0` and
delegated `publication_authority` remain invariant in this diagnostic family;
only the separate RPD-2A-PUB family may emit rows.

This design intentionally trades recall for duplicate safety: a content cohort
may contain one DB-backed row and many genuinely missing raw rows, but the
entire cohort remains withheld because the producer erased the original DB
coordinates. RPD-2A-PUB may publish only the disjoint cohorts which the ledger
proves have zero DB representation.

### RPD-2A-PUB exact raw-only publication

RPD-2A-PUB consumes the RPD-2A ledger directly; it does not repeat or weaken
the cohort decision. Capability `official_raw_blocked_recovery_v1` and
coverage family
`source_rawtrace_blocked_recovery/__raw_only_blocked_reason__` identify this
publication path.

A recovered row is published only when all of the following precise
conditions hold:

1. the raw decode ledger is complete and the DB multiset is an exact subset
   of the raw content multiset;
2. the row belongs to a raw content cohort with zero DB representation;
3. the retained row count equals the identity-admitted ledger census;
4. the exact raw timestamp and CPU are valid;
5. the payload target TID resolves to exactly one canonical thread/process at
   that timestamp and passes the shared lifecycle point gate;
6. the raw `common_pid` independently resolves to exactly one canonical
   header thread/process at that timestamp and passes the same gate;
7. the canonical blocked body and ftrace envelope pass the existing strict
   single-line and numeric bounds.

The emitted line preserves the exact raw timestamp, CPU, flags,
preempt-count, `common_pid` header identity, payload target TID, iowait,
caller profile, optional cnode index and optional delay. It carries
`source=official_rawtrace_rpd2a raw_db_content_cohort=absent` so downstream
audits can distinguish it from TraceStreamer DB projections. A canonical
kernel thread with no positive process PID keeps the exact header TID and
prints an unavailable TGID (`-----`); it is never rewritten as `TGID=TID`.

Publication remains deliberately incomplete:

- a cohort with even one DB representation is withheld in full;
- a namespace-shaped PID without a unique canonical host mapping is
  withheld, never rewritten;
- lifecycle absence, rejection or ambiguity withholds the individual row;
- a ledger mismatch, retained/admitted census mismatch or cancellation
  publishes no recovered prefix to the final artifact;
- these rows do not alter the existing DB scheduler coverage counters; their
  own coverage reports `RowsRead`, `RowsEmitted` and a typed
  `publication_state`.

Tests pin the exact timestamp/CPU/flags/header/body output, overlap
suppression, unresolved namespace rejection, census mismatch, cancellation,
zero-eligible publication and end-to-end coverage attachment. This closes the
safe blocked-reason subset; it does not claim recovery of all `19,771`
unmatched upstream associations.

### RPD-LITE-D strict decode and retention

Cold comparison against the upstream `FtraceEventProcessor` and
`CpuDetailParser` freezes two byte-exact lite scheduler profiles:

```text
sched_switch_lite:
  prev_pid:s32, prev_prio:s16, prev_state:u64,
  next_pid:s32, next_prio:s16, next_info:u64

sched_wakeup_lite:
  pid:s32, prio:s16, target_cpu:s32
```

Each format must also carry the exact common envelope
`common_type:u16@0`, `common_flags:u8@2`,
`common_preempt_count:u8@3`, `common_pid:s32@4`. Missing, duplicated,
overlapping, renamed, width-drifted or appended payload fields fail closed.

Capability `official_raw_scheduler_lite_decode_v1` adds both families to the
bounded target geometry roster and strict record ledger. Exact timestamp,
source CPU, common PID, flags, preempt count and payload fields are retained
internally under the bounded target-record cap. The observed RPD-1 capture has
`117,226 + 66,736` lite records. With marker, blocked, wakeup-new and the
closed DMA roster included, its complete target census is `390,416`; this
exposed that the former `250,000` cap would withdraw every join on replay.
RPD-CAP1 raises the bound to `1,000,000`. A future capture which exceeds the
new cap still withdraws completion rather than publishing a prefix.

The packed `next_info` value remains the authority. The existing character
field lane losslessly preserves the kernel's prefix-stable five/six/seven/
eight-plus textual versions. The lite lane, however, exposes only one packed
`u64`: Codrax knows the current bits through cgroup ID (bits 0..52), retains
the full raw word, and counts nonzero high tail bits without guessing their
future field boundaries or meaning. RPD-LITE-D does not infer cpuset from a
name, descriptor field count, missing suffix or undocumented high bits.

This sub-batch remains diagnostic: `RowsEmitted=0` and
`publication_authority=withheld_rpd1_diagnostic_only`. RPD-LITE-JS/JW now
consume its retained rows only after a separate unique raw-to-DB
boundary/edge proof; they enrich a DB-derived scheduler row, but never add a
second scheduler event or replace canonical ITID/lifecycle authority.

### RPD-LITE-JS exact switch-boundary enrichment

The switch half of RPD-LITE-J is implemented as capability
`official_raw_scheduler_lite_join_v1` and coverage family
`source_rawtrace_scheduler_lite_join/__raw_vs_db_sched_switch__`. It changes
one already-authorized DB scheduler boundary in place; it never appends a
second `sched_switch`.

The join follows the upstream implementation at OpenHarmony SmartPerf commit
`260b028b289befa8dc2f85b98687d323c7d20fa0`. That implementation stores a
`sched_switch_lite` event as follows:

- the boundary timestamp is the next `sched_slice.ts`, equal to the preceding
  slice's exact `ts+dur`;
- CPU and canonical next thread come from the new slice;
- the preceding slice's `end_state` is the raw `prev_state` rendered through
  the fixed TraceStreamer `EndState` table;
- the next slice priority is the raw event's `next_prio`;
- the preceding slice priority was captured when that thread switched **in**,
  so it is not the event's switch-out `prev_prio`.

Consequently the exact join key is:

```text
(timestamp_ns, cpu, prev_canonical_public_tid, mapped_prev_state,
 next_canonical_public_tid, next_priority)
```

The raw record is admitted only when `common_pid == prev_pid`; both public
PIDs must already equal the canonical DB public TIDs. There is no comm-based
join and no host/namespace-PID rewrite. A raw key and DB key must each occur
exactly once across the complete audited input. Raw duplicates, DB
duplicates, lifecycle-suppressed CPU lanes, an incomplete raw ledger, a
retained/admitted census mismatch, unmapped state, invalid envelope scalar or
missing DB census all fail closed.

On a unique match, the existing DB row receives:

- raw `prev_prio`, which is the exact switch-out snapshot;
- exact raw `common_flags` and `common_preempt_count`;
- `next_info=<known-prefix>` for current trace-query consumers;
- `codrax_next_info_raw=0x...` as the lossless authoritative packed receipt;
- `codrax_next_info_source=official_raw_sched_switch_lite`.

The semantic `next_info` renderer decodes only documented bits through
cgroup ID. Bits 53..63 remain preserved in the raw hex receipt and never gain
invented future field boundaries. This is compatible with the user's
prefix-stable five/six/seven/eight-plus textual `next_info` rule: the textual
character-field lane continues preserving every appended decimal field,
while the packed-lite lane preserves the complete word and decodes only the
known prefix.

Tests pin the complete upstream state table, unique enrichment, exact
switch-out priority, trace-query consumption, future-tail receipt, namespace
header mismatch, state/priority mismatch, duplicate raw key suppression,
zero duplicate physical events, typed coverage attachment, and the
missing-DB-census arm. RPD-LITE-JW remains separate because a wakeup edge uses
different DB tables and producer shapes; switch-boundary proof must not be
reused as wakeup authority.

### RPD-LITE-JW exact wakeup-edge enrichment

The wakeup half is implemented as capability
`official_raw_scheduler_lite_wakeup_join_v1` and coverage family
`source_rawtrace_scheduler_lite_join/__raw_vs_db_sched_wakeup__`. It consumes
the existing DB `instant`/`raw` unique bipartite pairing and enriches that
already-authorized edge; it never adds a second wakeup event.

The same pinned upstream SmartPerf implementation writes a rawtrace
`sched_wakeup_lite` event into:

- `instant(ts, name=sched_wakeup, ref=wakee_itid,
  wakeup_from=waker_itid)`;
- `raw(ts, name=sched_wakeup, cpu=target_cpu, itid=wakee_itid)`.

This is intentionally narrower than Codrax's general wakeup compatibility
path. The latter also supports bytrace rows whose `raw.itid` is the waker and
`sched_wakeup_new` rows normalized for pairing. Neither alternate producer
shape gains lite authority.

After the DB instant/raw pairing is itself unique, the lite join key is:

```text
(timestamp_ns, waker_canonical_public_tid,
 wakee_canonical_public_tid, target_cpu)
```

The exact source record must have `common_pid == waker TID`,
`payload.pid == wakee TID`, and the same target CPU. Both endpoints must pass
the shared point-lifecycle gate. No comm alias, host/namespace rewrite,
nearest-time relation or inferred TGID participates in the key. Raw and DB
key multiplicity must each equal one.

TraceStreamer aliases exact `sched_wakeup` and `sched_wakeup_lite` into the
same DB name. Therefore the entire lite join is withdrawn whenever the
complete raw ledger contains even one exact `sched_wakeup` source record;
the DB could not prove which format produced a same-shaped edge.
`sched_wakeup_new` remains a separate, ineligible instant name.

On an exact join, the existing row receives:

- the source raw page CPU as the exact physical emitter CPU, replacing
  `thread_state.Running` lookup for this event only;
- the raw target CPU, cross-checked against DB `raw.cpu`;
- exact raw `common_flags` and `common_preempt_count`;
- the exact positive signed-16 raw priority;
- `codrax_wakeup_source=official_raw_sched_wakeup_lite` as an opaque receipt
  tail.

The exact priority deliberately carries no `codrax_prio_source` token:
tracequery defines a native present priority with no token as exact, accepts
only the two degradation tokens `inferred_next_sched_slice` and `unknown`,
and correctly treats every other value as untrusted. Nonpositive raw lite
priorities therefore remain diagnostic and use the pre-existing fallback
rather than weakening that parser invariant.

If exact lite matching fails, Codrax retains the old behavior byte-for-byte:
it uses lifecycle-gated Running CPU when known, emits the typed CPU-free
wakeup form when CPU is unavailable, and labels next-sched priority as
inferred. Tests pin exact CPU/priority round-trip, no duplicate event,
namespace/header mismatch, wakee and target-CPU mismatch, raw-key
multiplicity, exact-source co-presence withdrawal, bytrace DB-shape
rejection, nonpositive priority, fallback preservation, missing DB census,
typed coverage attachment and diagnostic capability publication.

### RPD-2C exact raw DMA-wait recovery

RPD-2C is implemented as capability
`official_raw_dma_wait_recovery_v1` and coverage family
`source_rawtrace_dma_wait/__raw_dma_wait__`. The RPD-1 source contains 149
strict `dma_fence_wait_start` and 149 strict `dma_fence_wait_end` records.
TraceStreamer's normalized `raw` table has no argset column on this producer,
so its raw exporter cannot render them; the `dma_fence` high-level table is
also intentionally non-equivalent because its `dur` is a previous-event
delta and it carries no emitter or CPU.

The raw ledger now retains, for these two exact descriptors only:

- nanosecond timestamp and raw-page CPU;
- `common_pid`, flags and preempt count;
- driver, timeline, uint32 context and uint32 sequence number;
- exact start/end event name.

Publication requires the complete decode ledger and equality between the
physical, body-admitted and retained endpoint censuses. If the DB raw-ftrace
DMA class emitted even one row, source publication withdraws whole because
the current DB coverage does not retain a cross-source duplicate key. The
high-level activity table is never counted as an endpoint and never
subtracted.

The sole key authority remains
`tracequery.FingerprintPairingEndpoint`:

```text
(common_pid, driver, timeline, context, seqno, dma_fence_wait)
```

`common_pid` must resolve to exactly one canonical public host TID at the
exact timestamp and pass the shared lifecycle point gate. It must remain
byte-for-byte equal to that canonical public TID; namespace-shaped values are
not rewritten through a name, process or neighboring scheduler row. Exact
raw CPU, flags and preempt count are preserved in the emitted ftrace
envelope. A kernel thread may retain an unavailable TGID, but Codrax never
sets `TGID=TID`.

Within each exact key lane, timestamps are sorted and the topology must
alternate one start followed by one end. Repeated/nested starts, orphan ends,
same-timestamp ambiguity, open tails, timestamp regression and intervals
which cannot remain positive after the standard six-decimal systrace
round-trip poison the whole lane. Clean sibling lanes remain publishable.
This is the anti-rescue rule: removing a bad endpoint must never let its
neighbors form a fabricated wait.

The emitted body remains the canonical four-field DMA wire without an
unrecognized provenance suffix, and a typed-to-wire parity check runs before
the first sink insertion. Rows are prepared completely before publication,
so cancellation or an invariant error emits no prefix. Tests pin strict raw
retention, exact CPU/flags/header/body, tracequery 3ms pairing, namespace
rejection, DB-raw overlap withdrawal, sub-microsecond suppression, retained
census mismatch, cancellation, poisoned-lane isolation and clean-sibling
survival.

### RPD-2B1 raw marker endpoint ledger

The customer RPD-1 replay was built before the recovery batches and therefore
cannot validate their publication. Its independent raw census nevertheless
adds the decisive marker witness:

```text
print=175,165
tracing_mark_write=7,518
physical marker carriers=182,683
TraceStreamer tracing_mark_write:received=182,683
```

The old diagnostic ledger decoded only the smaller
`tracing_mark_write` carrier. RPD-1b added `print` to the physical/body census;
RPD-2B1 now retains the exact synchronous endpoint evidence from both governed
carriers without publishing a row.

`tracequery.DecodeTraceMarkEndpointPayload` is the single exported verdict
over the existing complete-payload `parseTraceMarkValidated` authority.
Conversion does not carry another B/E grammar. For every exact B/E it retains:

- physical raw-record ordinal, nanosecond timestamp and page CPU;
- exact `common_pid`, flags and preempt count;
- original complete marker buffer;
- typed action, payload PID and begin name;
- admitted/rejected reason from the same query grammar.

The payload PID remains marker namespace data and is never rewritten to the
host `common_pid`, TGID, ITID or process PID. A governed carrier which passes
the common envelope but fails its strict descriptor/body decoder is retained
as a localized rejection at its exact emitter and timestamp. It does not
disappear and later allow adjacent endpoints to bridge the evidence hole.
An envelope failure is unlocalizable and withdraws the complete retained
marker ledger.

The return gate proves all of these equalities before exposing retained rows:

```text
physical carriers = envelope-admitted carriers
envelope admitted = body admitted + body rejected
body admitted = retained B/E + non-sync marker payloads
body rejected = retained localized carrier rejections
retained slice = retained B/E + retained carrier rejections
```

RPD-2B1 itself remains non-publishing. RPD-2B2 supplies the separately audited
publication path described below.

### RPD-2B2 strict raw synchronous-marker recovery

RPD-2B2 is implemented as capability
`official_raw_marker_sync_recovery_v1` and coverage family
`source_rawtrace_marker_sync/__raw_marker_sync__`. It reconstructs only
synchronous B/E pairs; S/F, G/H, C and instant rows remain outside this batch.

The physical stack lane is exactly `common_pid`, matching tracequery's
source-plus-emitter synchronous stack. Records are sorted by nanosecond
timestamp and immutable physical ordinal, then consumed as a LIFO stack.
Nested spans are supported. Any of the following withholds every source-raw
candidate from that emitter while leaving clean sibling emitters available:

- a rejected B/E schema, malformed S/F reset witness, or strict carrier
  rejection;
- invalid/repeated physical order;
- orphan E or open B;
- zero/negative or six-decimal-wire-unrepresentable interval;
- invalid CPU/flags/preempt scalar;
- endpoint identity absence, ambiguity, lifecycle rejection or drift.

For every closed pair, `common_pid` is independently resolved at B and E to
one canonical host TID/process. The canonical ITID/IPID and positive host TGID
must remain equal across the interval, and the shared closed lifecycle gate
must admit the full span. The B payload PID is retained verbatim as
`MarkerPID`; it is never resolved or rewritten, so Donghu namespace PID syntax
coexists with the host ftrace envelope. A namespace-shaped `common_pid` which
does not independently resolve is rejected rather than redirected through the
payload PID or comm.

Cross-source duplicate detection stays inside the already bounded sync-span
stage. On first lookup, its memory candidates are promoted to the same private
SQLite authority; no second candidate map or unbounded collection is allowed.
The exact lookup key is:

```text
(host_header_tid, host_header_tgid, effective_marker_pid,
 canonical_itid, owner_ipid, start_ns, end_ns, exact_name)
```

CPU, flags and task display do not split this logical key. If an earlier DB
candidate has the exact key, the raw pair is counted as corroboration and is
not submitted. A name difference, interval difference or owner difference is
not a duplicate. If the bounded stage has already failed closed, duplicate
completeness is unavailable and the raw publication batch withdraws.

Only DB-disjoint pairs enter the existing single sync-span authority as a new
closed producer. Therefore all cross-producer containment, crossing,
identity-conflict, duplicate-stable-ID, poison, budget and two-pass publication
rules still apply. The raw producer preserves exact B/E timestamp, CPU,
flags, preempt count and the complete original marker buffers. This retains
Harmony metadata suffixes and pipe-bearing names without regenerating,
truncating or reinterpreting the payload.

Tests pin DB-disjoint publication, exact CPU/flags/body, exact existing-DB
suppression, namespace payload preservation, namespace header rejection,
malformed async reset retention, poisoned-emitter isolation, strict producer
provenance, bounded-stage-only duplicate lookup, and the structural rule that
no producer publishes before the shared finalize pass.

### RPD-2B3 — complete endpoint action census; async publication remains frozen

The RPD1 replay is an older build whose capability roster ends at
`official_raw_record_decode_ledger_v1`. It nevertheless supplies one decisive
same-input closure: raw `print=175165` plus raw
`tracing_mark_write=7518` equals trace_streamer
`tracing_mark_write:received=182683`. Therefore `print` is the dominant
physical carrier in this capture, and a converter which decodes only the
event named `tracing_mark_write` necessarily misses most marker payloads.
RPD-2B1/B2 already govern both carriers.

Capability `official_raw_marker_action_census_v1` now adds exact metrics for
every governed endpoint action accepted or rejected by the sole complete
payload parser:

```text
target_marker_endpoint_{b,e,s,f,g,h,n,i}_{admitted|rejected}
```

This census is diagnostic only. Clean S/F, G/H and N/I rows are not added to
the synchronous publication ledger, and counter/ordinary print payloads are
reported separately as `target_marker_non_endpoint_payloads`. The existing
`target_marker_non_sync_payloads` closure is unchanged.

Raw S/F publication is intentionally frozen until two authorities coexist in
one bounded stage:

1. an exact `(source,payload_pid)` lifecycle generation, because S and F may
   be emitted by different host threads and numeric namespace PIDs can be
   reused; and
2. an exact cross-source comparison against trace_streamer's already completed
   `(payload_pid,name,cookie,start,end)` async interval, so recovery cannot
   double-count the 253 typed intervals already emitted in RPD1.

Resolving payload PID through host `thread/process` tables would violate the
Donghu namespace invariant and is forbidden. Pairing only by
`pid+name+cookie` without lifecycle generation is also forbidden.

G/H has the same lifecycle requirement and additionally needs its independent
`pid+track+cookie` ambiguity state machine because H carries no span name.
N/I and C are point/counter evidence, but direct publication remains frozen
until an exact DB duplicate authority exists. VSync recovery remains frozen:
this source event-format catalog explicitly lacks `trace_vsync`, while the
official DB exposes only aggregate `received=389/not_match=79`; aggregate
counts cannot reconstruct timestamps, CPU, identities or frame pairing.

### RPD-2B4 — RPD2 print legacy profile correction

RPD2 contains every B1-B3 capability but proves that marker recovery still
published zero endpoints:

- `print=175165` was rejected whole as
  `mixed_or_invalid_marker_profile`;
- the only admitted raw endpoints were
  `B=3759,E=2276`, while `tracing_mark_write` rejected exactly 1483 rows;
- `543/543` raw emitter lanes were poisoned and all 182683 physical marker
  records were withheld;
- the output grew by exactly 204 rows versus RPD1, equal to the 102 clean DMA
  wait pairs, so no marker row contributed to the delta.

Two closed producer facts explain both failures. OpenHarmony TraceStreamer
routes the event names `print` and `tracing_mark_write` through the same
`pid,name,start` record profile when the event body is the expanded form.
Codrax nevertheless prohibited that exact profile solely when the name was
`print`. This made the dominant carrier fail before the canonical payload
grammar. Separately, the 1483 rejected tracing rows exactly close the begin/end
imbalance: `3759 - 2276 = 1483`. The legacy E action has no logical name, but
Codrax previously required its nonsemantic fixed name storage to contain a
valid NUL-terminated string.

Capability `official_raw_marker_print_legacy_v1` removes the event-name veto
only after the mutually-exclusive exact `pid+name+start` declaration set has
been proven. Buffer and legacy profiles still cannot mix; IP is forbidden on
the legacy form; start remains exact `0|1`; PID remains signed int32; B still
requires the exact bounded, NUL-terminated, single-line name. E audits the
declared name field geometry and record bounds but does not read or publish
its unobservable storage bytes.

The raw marker coverage now also carries a concise
`marker_format_geometry_witnesses` field copied from the immutable event-format
catalog. This avoids RPD2's diagnostic truncation, where the 8 KiB raw-decode
line ended exactly inside the `print` field list.

## Invariants

- Never fabricate CPU, PID, TGID, comm, timestamp or lifecycle evidence.
- Namespace PID must not replace host ownership.
- Display-name recovery must not become identity authority.
- Quality ratios and counts are advisory unless a future hard gate reads a
  precise typed invariant.
- Production emits one selected trace body; diagnostic comparison must not
  merge two independently converted bodies.

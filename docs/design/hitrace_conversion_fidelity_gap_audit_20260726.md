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

Both payload public PIDs must already equal the canonical DB public TIDs.
`common_pid` is an independently counted envelope observation, not switch
identity; it is never rendered or used as a host/namespace mapping. There is
no comm-based join and no host/namespace-PID rewrite. A raw key and DB key
must each occur exactly once across the complete audited input. Raw
duplicates, DB duplicates, lifecycle-suppressed CPU lanes, an incomplete raw
ledger, a retained/admitted census mismatch, unmapped state, invalid envelope
scalar or missing DB census all fail closed.

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
switch-out priority, trace-query consumption, future-tail receipt,
`common_pid` nonidentity, state/priority mismatch, duplicate raw key
suppression, zero duplicate physical events, typed coverage attachment, and
the missing-DB-census arm. RPD-LITE-JW remains separate because a wakeup edge
uses different DB tables and producer shapes; switch-boundary proof must not
be reused as wakeup authority.

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

### RPD-2B5 — scheduler-lite and blocked-reason decision diagnostics

RPD2 also proves that two recovery lanes are still withheld:

- all `117226` `sched_switch_lite` records fail at the descriptor-layout
  gate;
- all `66736` `sched_wakeup_lite` records pass that layout gate but fail the
  `target_cpu` field/value gate;
- all `397` emitted DB blocked-reason content cohorts are absent from the
  `19629` raw content cohorts under the exact
  `(target_tid,iowait,caller)` key.

The existing raw-decode coverage contains the field geometry needed to
distinguish a legitimate kernel-version profile from a malformed near
profile. In RPD2 that JSON physical line is truncated at 8 KiB before the
scheduler descriptors. Loosening either decoder from aggregate rejection
counts would turn a noisy symptom into a hard admission signal and is
forbidden.

Capability `official_raw_scheduler_lite_geometry_v1` therefore copies a
bounded, dedicated `scheduler_lite_format_geometry_witnesses` value into each
small scheduler-lite join coverage row. It includes exact field name, offset,
size, signed bit and normalized descriptor type. The same row carries a
sorted `source_decoder_census`, so the next replay directly binds geometry to
the typed rejection reason without requiring the customer to extract the
binary format segment. This remains diagnostic-only and cannot publish,
rewrite or join an event.

RPD2 also rejects all `120` raw `sched_wakeup_new` records at the same
`target_cpu` field/value gate. Capability
`official_raw_scheduler_wakeup_new_geometry_v1` copies a second short witness
covering both `sched_wakeup_lite` and `sched_wakeup_new` into the wakeup join
coverage. It remains non-publishing; the existing DB lifecycle creation rows
continue to be authoritative.

### REP-1A — OpenHarmony high-ID compact print profile

REP1 contains the expected post-RPD2 capabilities but still emits the exact
same 388112 rows. The new geometry proves why: all 175165 `print` records use
the compact high-ID profile `print#32886[buffer@8:4]`, not the expanded
`pid+name+start` profile repaired by RPD-2B4. The 7518 expanded
`tracing_mark_write` records now close exactly (`B=3759,E=3759`), proving the
E-name correction itself worked, but every emitter lane remains poisoned by
the rejected compact print carriers.

Capability `official_raw_marker_print_compact_v1` admits only the exact
OpenHarmony profile selected by all of:

- event name `print`;
- event ID at or above the OpenHarmony `0x8000` offset;
- exactly one `buffer` carrier and no `buf`, legacy or IP fields;
- the existing strict data-loc/fixed/C-string carrier and payload grammar.

The traditional low-ID `print` profile remains exactly `ip+buf`; a low-ID
`buffer`, mixed `buf+buffer`, or high-ID compact profile with IP fails closed.
Signedness on a byte-string data-loc descriptor remains nonsemantic, matching
the pinned producer parser.

### REP-1B — observed Harmony compact scheduler profiles

REP1's dedicated geometry closes the scheduler ambiguity. The capture uses:

```text
sched_switch_lite:
  prev_tid:int32, pprio:s16, pstate:u16,
  next_tid:int32, nprio:s16, ninfo:unsigned-char[8]
sched_wakeup_lite:
  pid:int32, prio:s16, target_cpu:s8
sched_wakeup_new:
  pname:char[16], pid:int32, prio:s16, target_cpu:s8
```

These names and widths match the pinned producer's positional decoding into
the same protobuf semantics; they are not corrupt near profiles. Capability
`official_raw_scheduler_compact_profile_v1` adds this complete field set as a
second mutually-exclusive profile. Mixed canonical/compact names fail the
closed layout gate. `pstate` is admitted only as unsigned 16-bit, `ninfo` only
as the exact eight-byte unsigned-char scalar/array receipt, and target CPU only
as signed `s8` or the existing signed int32 profile. CPU range validation,
namespace fail-closed identity, exact DB joins and full future `next_info`
receipt remain unchanged.

### REP-1C — blocked caller signed-char descriptor semantics

REP1 proves that all `21,566` raw `sched_blocked_reason` rows were admitted as
hard `(target_tid, iowait, caller_raw)` observations, but every caller label
fell back to opaque. The observed descriptors are fixed arrays:

```text
func_name: char[20], signed=1
mod_name:  char[12], signed=1
```

The DB retained `1,795` symbolized caller rows. Exact caller-key comparison
therefore failed, while the reduced `(target_tid, iowait)` multiset proved that
all DB subjects were present in raw. The producer's `HandleStrField` consumes
these arrays as strings and does not give the descriptor signed bit numeric
meaning. Codrax incorrectly rejected the labels solely because `signed=1`.

Capability `official_raw_signed_char_array_string_v1` now treats signedness as
nonsemantic only for an exact fixed `char[...]` string carrier. It does not
admit scalar chars or different types, and still requires one declared field,
an exact byte extent, NUL termination, no embedded/line/control injection and
the caller token grammar. Blocked publication continues to use the full
caller-bearing exact reconciliation key; the reduced subject key remains
diagnostic evidence only and cannot authorize recovered rows. Namespace and
incarnation gates are unchanged.

The blocked mismatch also cannot safely be called “raw record loss”: the
upstream parser retains only its chosen caller string in the DB, while Codrax
independently reconstructs the raw caller profile. Capability
`official_raw_blocked_subject_census_v1` adds:

- raw/DB caller-profile row counts (`symbolized` versus `opaque`);
- a second diagnostic multiset over only `(target_tid,iowait)`;
- the typed relation `db_target_tid_iowait_multiset_is_raw_subset` or its
  negative form;
- a concise typed `sched_blocked_reason` field-geometry witness.

The original exact caller-bearing key remains the only publication gate.
The reduced key is diagnostic evidence only: it may prove that the mismatch
is isolated to caller representation, but it can never deduplicate or publish
rows. Namespace TIDs remain byte-preserved and unresolved identities remain
fail-closed.

### REP-2A — raw marker recovery must be monotonic over the DB baseline

REP2 proves all three REP1 decoders are active, but also exposes a new P1
composition failure:

- raw marker decoding admits all `175,165` compact `print` carriers;
- `5,326` raw marker pairs enter the shared sync-span authority;
- all `5,326` are suppressed;
- all `305` otherwise-clean raw emitter lanes become
  `unproven_identical_lanes`;
- the authority suppresses `14,674` spans (`29,348` endpoints), so the output
  loses previously accepted DB spans even though raw recovery is additive.

The exact mechanism is an interval collision whose host/payload/canonical
identity and nanosecond endpoints equal an earlier DB candidate while its name
does not. The full semantic deduplication key includes name, so it misses this
case; the later all-lane auditor then correctly refuses to order two
same-interval, different-name candidates, but incorrectly lets an optional
recovery candidate invalidate the already-admitted baseline.

Capability `official_raw_marker_name_drift_fence_v1` restores monotonicity.
Before a raw pair is submitted, a second exact bounded index checks the same
host TID/TGID, marker PID, canonical ITID/owner and start/end interval without
discarding any identity dimension. If the full name-bearing key matched, the
existing duplicate rule applies. If only the exact identity+interval matched,
the raw pair is locally withheld as
`raw_pairs_withheld_exact_interval_name_drift`; it never reaches the shared
lane audit and therefore cannot suppress the DB baseline. This rule does not
choose either name, merge identities, rewrite namespace PID, or authorize a
raw row. DB-disjoint raw pairs remain eligible under the existing laminar
authority.

### REP-2B — `common_pid` is not `sched_switch_lite` payload identity

REP2 retains all `117,226` compact scheduler-switch records but admits only
`1,299` into the exact DB-boundary join; `115,927` are rejected before the DB
census. The decisive Codrax-only condition was
`common_pid == prev_tid`. The pinned upstream implementation does not impose
that relation: `FtraceEventProcessor::SchedSwitchLite` decodes only the six
payload fields, and `CpuDetailParser::SchedSwitchLiteEvent` constructs the
switch from payload `prev_pid`, `next_pid`, priorities, state and
`next_info`. The common event PID is not passed to `InsertSwitchEvent`.

Capability `official_raw_scheduler_lite_common_pid_nonidentity_v1` removes
that extra equality from the publication key. This does not weaken the
namespace or identity fence:

- publication still requires a unique exact raw/DB match on timestamp, CPU,
  payload prev/next public TID, mapped state and next priority;
- DB canonical public TIDs remain the output identities;
- `common_pid` is never rewritten into a payload TID, TGID or namespace PID,
  never rendered, and never used for name or ownership;
- raw flags, preempt count, switch-out priority and `next_info` can enrich only
  that already-existing DB boundary, so no second scheduler event is emitted.

The typed metric `raw_records_common_pid_differs_from_prev_tid` preserves the
observed envelope/body difference for audit. Any records rejected by the
remaining exact payload/state/range gates stay fail-closed.

### REP-2C — scheduler-lite decision diagnostics without semantic guessing

REP2 reports only one aggregate `raw_records_key_rejected=115927`, so after
removing the incorrect `common_pid` equality it could not distinguish an
unsupported `pstate` from an invalid scalar if any rejection remained.
Capability `official_raw_scheduler_lite_decision_diagnostics_v1` preserves
the aggregate and adds one closed typed counter for the first exact rejection
gate (`unsupported_prev_state`, timestamp/CPU/TID/envelope/priority range).
These counters are observational and do not relax any gate.

The same capability adds
`scheduler_lite_next_info_unknown_tail_or` and
`scheduler_lite_next_info_unknown_tail_and` to the raw-decode ledger whenever
bits 53..63 are present. REP2 has such bits in all `117,226` records, but the
available pinned producer and public reference converter do not define their
field boundaries or meaning. Codrax therefore continues to:

- render only the proven prefix through cgroup ID;
- preserve the complete authoritative packed word in
  `codrax_next_info_raw`;
- expose aggregate high-bit geometry for the next replay;
- make no seventh/eighth-field, cpuset, namespace or policy claim from those
  undocumented bits.

### REP-2D — recover names carried only by raw `sched_wakeup_new`

REP2 has one unresolved canonical thread (`itid=398`, `tid=29352`,
`tgid=68`) that accounts for all `816` scheduler boundaries rendered with
`comm=unknown`. The capture also contains `120` compact
`sched_wakeup_new` records whose exact `pname[16]` bytes are decoded by
Codrax. The pinned upstream `CpuDetailParser::SchedWakeupNewEvent`, however,
calls `UpdateOrCreateThread` rather than `UpdateOrCreateThreadWithName`, so
that payload name is not retained in the DB thread table. Codrax previously
discarded it after diagnostic body validation as well.

Capability `official_raw_scheduler_wakeup_new_name_v1` retains a display-only
receipt `(timestamp,payload_tid,name)` from an otherwise fully admitted
official raw `sched_wakeup_new` record. It fills an unresolved canonical
display name only when:

- the complete raw decode census is available;
- the payload public TID resolves to exactly one DB thread/process
  incarnation at the exact raw timestamp through the shared lifecycle
  authority;
- every admitted raw name for that canonical ITID is byte-identical;
- no direct, cmdline, main-process or existing unique-public-TID display name
  is already present.

Conflict, lifecycle absence, rejected identity, namespace ambiguity and an
existing name all fail closed. The result updates only the precomputed
display-name map and the effective unresolved-name counter. It does not
change public/canonical IDs, TGID/owner, namespace mapping, lifecycle cuts,
CPU, event count, wakeup pairing or causal authority. The next replay will
show whether the single REP2 unknown subject is among those 120 target TIDs;
the implementation makes no such claim in advance.

## REP3 replay audit and remaining span blast radius (2026-07-28)

REP3 used a binary carrying all four REP2 capabilities. The conversion
completed with `407,864` query-ready rows and an `87,523,430`-byte systrace.
The scheduler-lite identity correction is fully effective:

- all `117,226` raw `sched_switch_lite` records pass the payload key;
- `117,214` exact DB boundaries are enriched and only the `12` DB open tails
  remain unmatched;
- all `66,736` lite wakeups are enriched;
- `115,927` records explicitly report
  `common_pid != payload.prev_tid`, confirming that treating the common
  envelope PID as switch identity was the earlier false rejection.

The wakeup-new display-name experiment also reaches a closed result. All
`120` names are retained and lifecycle-audited, but `15` already equal an
existing canonical display name and `105` differ from an existing name.
None targets the sole unresolved subject
`itid=398/tid=29352/tgid=68`. Therefore the persistent `816` unknown
scheduler boundaries are source-name absence for this capture, not a
recovery-gate rejection. No other thread may donate its name.

### REP3-S0 — exact callstack accounting

The final output is not missing the whole callstack table:

| Stage | Exact count |
| --- | ---: |
| physical DB callstack rows | 96,221 |
| admitted before pairing | 96,120 |
| rejected locally as invalid duration | 101 |
| official completed async intervals emitted | 253 |
| synchronous spans submitted | 95,867 |
| synchronous spans suppressed by 90 local fences on 41 TID lanes | 1,262 |
| synchronous spans emitted | 94,605 |
| total logical callstack intervals emitted | 94,858 |
| physical callstack output rows | 189,463 |

Thus the proven DB loss is `101` rejected source rows plus `1,262` collateral
sync spans. The latter is the existing H4b timestamp-suffix fail-closed rule:
an invalid duration proves a start but not an end, so later callstack
candidates on that producer/TID are fenced. It reduced the former
whole-history suppression from `51,532` to `1,262`, but remains visible
customer loss. Removing it without another endpoint authority would turn an
unknown open extent into an assumed local defect and is not authorized.

### REP3-S1 — raw marker local defects incorrectly poison whole lanes (P1)

Status: implemented and pushed as `09bc1b571`.

The independent raw marker census is complete:

- `168,117` exact synchronous B/E endpoints;
- `517` physical emitter lanes;
- only `305` lanes classified clean;
- `212` lanes classified poisoned, withholding `157,463` endpoints from the
  supplemental path.

The last number is not a proven count of final missing spans because many raw
pairs already have DB candidates. The current implementation discards the
lane before performing that DB closure, so the diagnostic cannot distinguish
duplicates from recoverable raw-only pairs.

The poison reasons expose an avoidable blast radius:

- `169` lanes contain a pair shorter than one microsecond or whose endpoints
  round to the same systrace timestamp;
- `33` lanes contain an orphan end;
- `9` lanes finish with an open begin;
- `1` lane contains one locally invalid candidate.

Each of these facts has exact physical coordinates. A sub-microsecond pair
cannot be rendered faithfully and must remain withheld, but it does not make
earlier or later balanced pairs ambiguous. An orphan end cannot alter a
later empty-stack pair, and a trailing open begin cannot invalidate already
closed pairs. Likewise, failure of the exact identity/CPU validation for one
already-paired interval is local to that pair. Whole-lane poison remains
necessary only when physical ordering or an unclassified/rejected carrier
makes the B/E stack itself unknowable.

Batch REP3-S1 therefore freezes the following policy:

1. pair structurally by exact emitter, timestamp and physical ordinal;
2. count and locally withhold unrepresentable closed pairs, orphan ends,
   trailing open begins and individually rejected exact pairs;
3. submit every remaining pair through the unchanged lifecycle, namespace,
   semantic-deduplication and shared laminar authority;
4. retain whole-lane poison for invalid physical ordering, rejected carriers
   and invalid/unclassified actions;
5. report partial/salvaged lanes separately from fully clean and poisoned
   lanes. No locally rejected endpoint or pair is rendered.

### REP3-S2 — pre-final deduplication can hide a later-fenced DB candidate

The raw path reports `5,326` exact identity+interval collisions whose name
differs from a DB candidate, plus one full semantic duplicate. Withholding
these before submission protects the DB baseline from the REP2 identical-span
regression. However, DB candidate lookup occurs before the shared authority
applies the 90 producer-scoped callstack fences. The current report cannot
prove whether any raw pair was withheld in favor of a DB candidate that was
subsequently suppressed.

This is a separate composition gap. It must not be fixed by blindly submitting
all collisions, which would reintroduce REP2 lane suppression. The next batch
must add an exact typed answer to “does the colliding DB candidate survive its
producer fence?” and admit the raw alternative only when the DB candidate is
provably fence-suppressed. Final cross-producer laminar and identity audits
remain mandatory.

Capability `official_raw_marker_post_fence_dedup_v1` implements that answer
inside the same bounded sync-span stage. The full semantic and
identity+interval indexes now have a second exact query which excludes a DB
candidate only when:

- an exact producer/physical-header-TID interval or suffix fence affects that
  candidate under the same half-open predicates and lane scope used by
  finalization; or
- the candidate is callstack-produced and its exact physical TID has a typed
  callstack producer poison.

Raw recovery first proves that at least one DB collision exists. It keeps the
old withholding behavior if any matching DB candidate remains locally
admitted. Only when all exact matches are locally suppressed may the raw
alternative enter the shared authority. The raw candidate still passes final
cross-producer nesting, identity, duplicate, budget and wire checks; this
change cannot rescue a genuinely conflicting lane. Typed counters distinguish
an exact semantic DB candidate from an identity+interval name-drift candidate
that was locally suppressed. REP3 cannot predict either count because its
diagnostic predates this capability; the next replay supplies the measurement.

## REP4 replay audit and remaining exact span gaps (2026-07-28)

REP4 carries both REP3 recovery capabilities and completes successfully with
`410,036` query-ready rows and an `87,777,760`-byte systrace. Relative to
REP3, the artifact gains exactly `2,172` physical rows. The raw recovery lane
submits `1,086` spans, emits exactly `2,172` endpoints, and reports no
raw-producer suppression. Therefore the delta closes exactly: all `1,086`
raw alternatives whose colliding DB callstack candidate was locally fenced
were published. This is measured recovery, not an estimate.

The remaining span gaps have three distinct authorities and must not be
combined:

| Gap | Exact REP4 count | Current authority |
| --- | ---: | --- |
| locally fenced DB callstack spans without a published raw replacement | 176 | `1,262 - 1,086`; exact only because the raw submitted/emitted/suppressed census closes |
| balanced raw pairs not representable at systrace microsecond precision | 18,033 | withheld; emitting equal timestamps or inflating duration would fabricate wire timing |
| balanced raw pairs rejected by the post-pair endpoint validator | 3,151 | currently typed only as the over-broad `invalid_endpoint`; recoverability is not yet known |
| orphan raw ends | 82 | locally withheld |
| trailing open raw begins | 85 | locally withheld |

The `61,699` exact interval/name-drift pairs and `6` exact semantic DB
candidates are not missing spans: their corresponding DB candidate survived
the local producer fence, so the raw duplicate is correctly withheld. The
`816` scheduler boundaries with `comm=unknown` also remain a proven
source-name absence for this capture, not a rejected wakeup-new recovery.

### REP4-S3 — gross DB suppression is incorrectly presented as net loss

Status: implemented as capability `raw_marker_replacement_closure_v1`.

The semantic-quality row and user caveat still publish only
`callstack_sync_spans_suppressed=1262`. They omit the exact `1,086` raw
replacements, making a successful recovery look like no recovery. The repair
is advisory and does not change trace rows or any hard gate:

1. copy the reconciled callstack local-fence count and raw producer census;
2. close replacement arithmetic only when every raw submitted span emitted
   two endpoints and the raw producer suppressed none;
3. publish gross suppression, recovered replacement spans, total residual,
   and local-fence residual as separate typed metrics;
4. use the residual rather than the gross count in the degradation caveat;
5. retain gross-only wording whenever the exact closure cannot be proven.

For REP4 the expected typed result is `recovered=1086`, `residual=176`.

### REP4-S4 — `invalid_endpoint` hides the next recovery decision

Status: implemented as capability `raw_marker_pair_diagnostics_v1`.

`traceDBRawMarkerSyncCandidate` currently collapses all endpoint-envelope,
payload round-trip, name, timestamp, CPU, flags and preempt-count failures
into one `invalid_endpoint` reason. REP4 places all `3,151` locally rejected
pairs in that bucket, so another customer replay cannot identify whether the
loss is an invalid name, CPU range, payload mismatch, timestamp overflow, or
another exact field. This batch splits the same validation predicate into
closed first-failure typed reasons. It does not relax admission or emit a
previously rejected span. Any later recovery requires a reason-specific
authority and a separate review.

The capability performs that split without changing the predicate or its
order. It also publishes a bounded longest-pair roster plus exact counts for
zero start, durations at least 100 ms / 1 s, and pairs covering at least half
of the raw-marker window. The DB callstack lane independently publishes its
accepted zero-start/long-duration counts, one bounded longest accepted row,
and an advisory comparison with the complete raw target's first timestamp.
Names longer than 128 bytes or unsafe for one physical diagnostic line are
represented only by SHA-256 and byte count.

### REP4-S5 — customer-visible `start=0` / full-window span (P0 audit)

Status: customer text witness audited; no physical zero timestamp reproduced.
Two generic-viewer compatibility gaps confirmed; one quality disclosure
implemented, data-plane recovery remains open.

The REP4 artifact receipt proves:

- all `410,036` physical output rows were parsed as known authoritative rows
  by cross-validation;
- the first parsed output timestamp is `69326.012182s`, not zero;
- every converter-generated synchronous B/E producer routes through one
  laminar authority, raw orphan ends/open begins are withheld, and direct
  completed async intervals are atomic typed rows.

Therefore the current evidence does **not** prove that Codrax emitted a
physical trace row at timestamp zero. The customer-visible zero can still be:

1. a business payload such as `start_ts=0`;
2. viewer-relative time zero for a real long pair beginning at capture start;
3. a viewer boundary fill for an incomplete pair in another lane;
4. a high-level DB callstack row with `ts=0` accepted because this capture's
   `trace_range.start_ts=0` does not provide a nonzero floor.

The fourth item is a real latent gap even though REP4's first-timestamp receipt
rules out a published zero-time row in this particular artifact. Codrax treats
SQLite integer zero as a potentially valid timestamp and cannot reinterpret it
as a sentinel without source evidence. The new diagnostic compares accepted
callstack rows against the independent complete raw-record first timestamp but
keeps that comparison advisory. A future data-plane rejection is authorized
only if the offending output line proves a zero-start row and the independent
raw/source ledger proves the same family cannot validly begin there. Viewer
relative-zero and business `start_ts=0` must not trigger that rejection.

The follow-up Linux receipts `collect01.txt` and `collect02.txt` add the
following exact evidence:

- the bounded grep returned `200` `codrax_trace_async_interval/v1` rows because
  the command itself used `head -200`;
- none of the returned rows contains a physical `0.000000:`, `ts_ns=0`,
  `end_ns=0`, `start_ts=0` or `start=0`;
- every typed interval uses absolute timestamps within
  `69326.012182..69328.343094s`;
- the longest returned completed intervals are real notification/animation
  intervals: `NtfDataPlanAnimation[0]` is `1.047822500s`,
  `NtfHeaderAnimation[0]` is `0.762287968s`, and
  `StatusBarNTFKeepMoveDownAnimation[0]` is `0.654742656s`;
- the first standard B/E rows are instead thread-registration compatibility
  pairs generated at the capture-start timestamp. Each sampled B is followed
  immediately by E at the same timestamp; they are zero-duration metadata
  points, not evidence that the source span began at timestamp zero.

This closes “the converter printed a physical zero timestamp in REP4” as the
current cause. It confirms two separate hazards:

1. **generic-viewer omission**: completed async rows are encoded as Codrax
   versioned comments. Codrax `trace_query` reconstructs them, but a generic
   systrace viewer may ignore the comments or render an absent start as its
   own relative zero. Capability
   `completed_async_generic_viewer_caveat_v1` now publishes this limitation
   whenever such rows remain;
2. **registration point compatibility**: the converter published redundant
   capture-start zero-duration B/E points in addition to `task_rename`.
   Although the physical pairs were closed, a viewer with a different B/E
   stack interpretation could extend one to its display boundary. Capability
   `thread_registration_metadata_only_v1` removes those non-business spans:
   each admitted thread now emits exactly one standard `task_rename` metadata
   row and reports `legacy_zero_duration_sync_spans_withheld`; no registration
   B/E candidate enters the shared span authority.

The same receipt also shows why a plain `grep unknown` overstates the remaining
name gap. Many hits are `task_rename oldcomm=unknown newcomm=<resolved>` or an
`unknown` historical field while the physical row header already contains the
resolved name; for example PID `32788` is headed by `ss.hm.ugc.aweme`.
Diagnostics must distinguish historical placeholder input from unresolved
output identity instead of asking customers to count raw substring matches.
The same registration batch also prevents an exact source-cmdline/display
recovery from being overwritten by a main-process placeholder `unknown`;
the recovered display name becomes both the physical header comm and
`task_rename` newcomm/oldcomm snapshot value. Truly unresolved names remain
`unknown` and continue to be counted by the existing typed resolver metrics.

### REP4-S5B — completed async generic-viewer recovery plan

Status: implemented as capability `official_raw_marker_async_recovery_v1`;
customer replay pending.

REP4 already reports `253` official completed async rows. The immutable raw
ledger independently counted admitted physical marker endpoints
(`S=264`, `F=10,322`) but deliberately retained only B/E sync endpoints;
admitted S/F endpoints were census-only and then discarded. That is the
highest-leverage remaining recovery gap.

The frozen implementation sequence is:

1. retain admitted raw S/F records with exact payload PID, name, cookie,
   timestamp, physical ordinal, common PID, CPU, flags and preempt count;
2. pair only an exact alternating S/F key
   `(payload_pid,name,cookie)`, withholding orphan F, open S, duplicate-open
   and non-representable intervals by typed reason;
3. independently resolve start and finish common-PID emitters at their exact
   lifecycle points; cross-thread/process completion is valid and must not be
   rewritten to one emitter;
4. replace one high-level typed interval with standard physical S/F only when
   payload identity, name, cookie, exact start/end and start emitter envelope
   match one unique raw pair;
5. leave unmatched high-level rows typed and publish the generic-viewer caveat;
   do not publish unmatched raw pairs in the first batch, avoiding duplicate
   legacy DB endpoint lanes.

The implemented ledger retains admitted S/F rows without placing them in the
B/E stack. It performs a deterministic per-key alternating audit, counts
orphan finish, trailing open start, duplicate-open poison, invalid physical
order, unrepresentable wire interval and endpoint/lifecycle validation
failures separately. A matchable pair preserves independent start/finish
common PID, host process, CPU, flags and preempt count.

Callstack publication now has two disjoint outcomes:

- `source_rows_emitted_official_async_raw_pair`: one high-level interval was
  replaced by two standard physical S/F rows from one unique exact raw pair;
- `source_rows_emitted_official_async_interval`: no unique exact raw pair was
  available, so the existing single typed interval remains.

The replacement does not infer namespace ownership from common PID: the raw
payload PID is compared to the high-level marker PID verbatim, while the raw
start header must independently match the high-level start emitter envelope.
Finish emitter/process/CPU come only from the raw F record. Cross-thread and
cross-process finish is therefore retained without rewriting either header.

### REP4-S5A — TaskPool allocation-only rows mint open async spans

Status: implemented as capability `task_pool_complete_pair_v1`.

The all-producer audit found one independent generic-viewer hazard:
`exportTraceDBTaskPool` always emitted an `S|` row after resolving allocation,
but emitted `F|` only when the linked execute row existed. A capture-ending
allocation therefore became an open async span which a viewer may extend to
the trace boundary. The same path used different payload owner TGIDs when the
allocation and execution emitters belonged to different processes, which
cannot form one S/F key even though cross-process execution itself is valid.

The repair publishes neither endpoint unless the task has:

- both linked allocation and execute timestamps;
- non-negative allocation/execute timestamps and execute duration;
- a checked execute end not preceding allocation;
- strict resolved allocation/execute ITIDs and independently retained physical
  emitter headers;
- one exact positive allocation-owner TGID reused as the logical S/F payload
  owner at both endpoints, without rewriting the execute emitter header;
- lifecycle-filtered Running CPU witnesses at both endpoints, with CPU 0
  accepted only when it is actually proven.

Incomplete, overflowing and reversed tasks are counted by closed typed
reasons. Cross-process execution remains representable because header identity
and logical payload owner stay separate. Missing/tainted/lifecycle-rejected
endpoint CPU evidence is withheld instead of being printed as CPU 0. REP4's
TaskPool table emitted no rows, so this was not the direct source of REP4's
visible long span; it is nevertheless the same systemic open-span failure
class and is closed independently.

## REP5 replay audit and next recovery batches (2026-07-28)

REP5 used a build carrying
`official_raw_marker_async_recovery_v1` and
`thread_registration_metadata_only_v1`. Conversion completed with `407,647`
query-ready rows and an `87,559,179`-byte systrace. Cross-validation accepted
all `407,647` selected rows as known and reported zero advisory, intentional
unknown or header-only rows.

The REP4-to-REP5 row delta closes exactly:

| Change | Exact row delta |
| --- | ---: |
| retire `1,222` thread-registration B/E compatibility pairs | `-2,444` |
| replace `55` one-row typed async intervals with two physical S/F rows | `+55` |
| total | `-2,389` |

`410,036 - 2,389 = 407,647`, exactly the REP5 output count. Therefore no
unexplained row loss was introduced by the two batches. The registration
exporter now reads `1,294` thread records, emits `1,222` admitted
`task_rename` metadata rows and reports
`legacy_zero_duration_sync_spans_withheld=1222`; it emits no business B/E
span. The previous viewer-sensitive zero-duration registration lane is
closed.

Name recovery is also quantitatively closed for this capture. All `1,294`
thread rows passed identity admission; `139` DB names were initially empty,
`138` were recovered from the immutable source cmdline segment, and exactly
one canonical thread remains unresolved
(`itid=398/tid=29352/ipid=1/switch_count=1`). A raw grep for the word
`unknown` still cannot be used as an unresolved-name count because historical
metadata and scheduler source absence are distinct fields.

### REP5-A1 — 198 complete raw async pairs remain unclaimed

Status: diagnostic batch implemented as capability
`official_raw_marker_async_join_diagnostics_v1`; data-plane relaxation
withheld pending the typed replay.

The raw ledger retained `10,586` S/F records, grouped them into `8,858` keys
and constructed `253` lifecycle-valid, wire-representable complete pairs.
The DB exporter also had `253` valid completed official async intervals:
`55` claimed one unique raw pair and became physical S/F, while `198`
remained one-row typed intervals. The old metric
`official_intervals_without_exact_raw_pair=198` cannot distinguish:

- payload PID/name/cookie identity-key mismatch;
- start timestamp mismatch;
- end timestamp mismatch after the start matched;
- start emitter TID or host TGID mismatch;
- exact start CPU mismatch when DB CPU placement is known;
- an already-claimed exact pair or an ambiguous exact match.

This is a real observability GAP. The equality `253 raw pairs == 253 official
intervals` is strong correlation but is not permission to pair by ordinal or
count. Namespace payload PID, host emitter identity and both timestamps remain
independent authorities.

The diagnostic batch preserves the exact publication predicate and adds one
closed first-failure counter for every unclaimed official interval. No output
row, hard gate or matching tolerance changes. The next replay can therefore
select a reason-specific repair without guessing. In particular, timestamp
tolerance is forbidden until a typed mismatch distribution proves which
clock/rounding relation exists.

### REP5-S1 — 32,042 typed synchronous spans still lack physical CPU

Status: exact overlap census and candidate-level recovery implemented as
capabilities `raw_marker_cpu_unavailable_collision_census_v1` and
`raw_marker_cpu_unavailable_replacement_v1`; customer replay must measure the
residual.

REP5 preserves `32,042` accepted callstack rows in the CPU-unavailable typed
lane. Codrax trace_query can consume these rows, but a generic systrace viewer
may omit the typed comments and every such row lacks CPU/core attribution.
This is now the largest generic-viewer completeness surface.

The raw B/E ledger simultaneously found `61,699` physical pairs sharing exact
host/payload/canonical identity and interval with a locally admitted DB
candidate but carrying a different name. Those are not presently missing:
the DB candidate survives and the raw duplicate is withheld. However the
current diagnostic does not disclose how many of the surviving DB candidates
are among the `32,042` CPU-unavailable rows. It is therefore impossible to
tell from REP5 whether an exact raw page CPU can replace a typed unavailable
candidate.

The implemented diagnostic batch adds an exact, advisory-only collision
census:

1. count locally admitted interval collisions by producer and CPU-placement
   state;
2. separate one unique CPU-unavailable callstack candidate from known-CPU,
   other-producer and ambiguous collision sets;
3. do not change names, CPU placement or publication;
4. authorize a later raw replacement only for one unique exact
   host/payload/canonical interval whose raw B/E endpoints independently pass
   identity, lifecycle and envelope validation.

The census reuses the same SQLite interval-identity key and the same
producer-local fence/poison predicates as final publication. For each withheld
raw pair it distinguishes:

- one unique known-CPU callstack candidate;
- one unique CPU-unavailable callstack candidate;
- one unique non-callstack candidate;
- multiple locally admitted candidates;
- an incomplete census caused by the existing bounded stage budget.

Candidate-row totals are also exposed separately from raw-pair totals. The
semantic-quality summary copies the unique CPU-unavailable pair count and
distinguishes it from known-CPU and ambiguous collisions. Query errors fail
loud; a bounded-stage incompleteness is typed and does not change the
pre-existing withholding decision. This census batch alone leaves the
systrace data plane byte-for-byte unchanged.

The following recovery batch consumes only the census's strongest arm:

1. the raw pair must collide with exactly one locally admitted DB candidate
   under the full host TID/TGID, namespace marker PID, canonical ITID, owner
   IPID and exact start/end identity;
2. the same full interval identity must have exactly one validated raw pair;
   nested or duplicate raw pairs with the same interval but different names
   are ambiguous and keep the DB fallback;
3. that sole DB candidate must be callstack-produced with a typed
   CPU-unavailable placement;
4. the candidate is marked `superseded` in the bounded SQLite stage by its
   exact identity; no interval fence is used, so nested or overlapping sibling
   spans are not affected;
5. the validated raw candidate then enters the unchanged shared laminar
   authority with source page CPU, flags, preempt count, exact raw name and
   marker bodies;
6. final accounting keeps the DB candidate as submitted-but-superseded,
   requires exactly one updated candidate, and requires the raw span to emit
   two endpoints before replacement closure is claimed.

Known-CPU DB candidates, multiple-candidate collisions, other producers and
incomplete bounded-stage censuses keep the previous withholding behavior.
Namespace payload PID is compared verbatim and never rewritten to the host
TGID. The semantic-quality summary subtracts only successfully closed raw
replacements from the CPU-unavailable residual, so the old gross `32,042`
count is not presented as the final typed loss after recovery.

### REP5 remaining typed gaps and priority

| Priority | Gap | REP5 evidence | Next action |
| --- | --- | --- | --- |
| P0 | generic-viewer sync span omission | `32,042` pre-replacement CPU-unavailable typed rows | replay the implemented unique-evidence replacement and use its exact residual |
| P1 | generic-viewer async omission | `198` typed intervals despite `253` complete raw pairs | consume the new join-reason counters before relaxing any field |
| P2 | local-fence residual | `176` unrecovered callstack spans | keep the exact replacement-closure disclosure; no new source authority in REP5 |
| P2 | scheduler name absence | `816` boundaries with unknown comm | source has no admitted name for those boundaries; do not infer from neighboring tasks |
| P3 | one unresolved canonical thread | `1` exact witness | retain typed witness; no namespace/comm guess |

No customer recapture is required for the source data. A replay with the next
Codrax diagnostic build is required only to expose the newly added typed join
reasons. Diagnostic output remains bounded below the existing 900-line cap.

## Official SmartPerf Host SQL/lane cross-audit (2026-07-28)

Status: source audit complete; implementation batches frozen. The audit pins
OpenHarmony `developtools_smartperf_host` at
`5c5afb0c479b070148d8a6e336120638a1a03930` (2025-09-12) instead of treating a
moving `master` checkout as evidence. The primary references are:

- `trace_streamer/src/base/pbreader_file_header.h`;
- `trace_streamer/src/parser/pbreader_parser/pbreader_parser.cpp`;
- `trace_streamer/src/parser/hiperf_parser/perf_data_parser.cpp`;
- `trace_streamer/src/filter/slice_filter.cpp`;
- `trace_streamer/src/table/ftrace/callstack_table.cpp`;
- `trace_streamer/src/trace_data/trace_data_cache.cpp`;
- `trace_streamer/doc/des_tables.md`;
- `ide/src/trace/component/SpSystemTrace.init.ts` and the SQL/receiver code
  beneath `ide/src/trace/database`.

The official implementation is a strong producer-schema and relationship
authority. Its UI conveniences are not automatically conversion authority.
In particular, Codrax must not copy display-only name fallbacks, fuzzy thread
matching, sample-to-next-sample synthetic durations, or trace-end extension of
unfinished intervals into precise trace evidence.

### Corrected architecture conclusion

SmartPerf Host does **not** normalize every SQL table into standard ftrace
text before drawing all lanes. TraceStreamer parses the binary into one
relational SQLite model; the SmartPerf UI then queries interval, relation,
counter and resolver tables directly. Examples include:

- `callstack(ts,dur,callid,parent_id,depth,cookie,child_callid,...)`;
- `frame_slice(...,callstack_id,...)`;
- `frame_maps(src_row,dst_row)`;
- `gpu_slice(frame_row,dur)`;
- `perf_sample(timestamp_trace,...)` joined to `perf_callchain`,
  `perf_thread`, `perf_files` and `perf_report`;
- `perf_napi_async(...)`, which joins native-hook async work to perf samples.

Therefore “complete systrace” needs two coexisting text lanes:

1. rows that have an exact standard ftrace representation remain standard
   `sched_*`, `tracing_mark_write`, IRQ, counter or log rows;
2. rows or fields that cannot be represented losslessly as standard ftrace
   use a versioned Codrax typed text record in the same `.systrace`.

The typed lane is necessary, not optional polish. One per-thread B/E stack
cannot encode arbitrary crossing SQL intervals. A relation row has no honest
CPU, emitter thread or duration. An unfinished `dur=-1` interval has no
observed end. Fabricating those fields merely to satisfy a generic viewer
would corrupt the evidence. Generic ftrace viewers may ignore the comment
records; Codrax must parse them and expose their typed lanes.

### Ruling for the current REP5 customer capture

The returned REP5 DB contains exactly the same 89-table roster registered by
the pinned official TraceStreamer build:

| Measure | REP5 |
| --- | ---: |
| tables in inventory | 89 |
| classified by a Codrax exporter/resolver | 41 |
| unclassified | 48 |
| unclassified but empty | 48 |
| unclassified and non-empty | 0 |

This disproves the broad theory that the current customer lost most spans
because the 48 unsupported official families contained data. They were all
empty in this capture. They remain forward-coverage gaps, but are not the
direct cause of the REP5 span deficit.

The direct current evidence remains:

- `96,221` `callstack` rows read;
- `96,120` admitted before pairing;
- `101` rejected as `invalid_duration`;
- `95,867` synchronous intervals submitted;
- `1,262` suppressed by localized fences, with most already recovered by raw
  marker replacement;
- `253` official completed async intervals and `253` complete matchable raw
  async pairs, but only `55` exact replacements and `198` typed-only
  intervals;
- `778` `frame_slice` rows and `120` `frame_maps` rows;
- `gpu_slice` and every other unclassified table empty.

No source recapture is required before the following implementation batches.
The retained customer input is sufficient to replay the direct fixes.

### OSP-01 (P0) — official async start emitter is `child_callid`

Status: implemented in O1; customer replay intentionally waits for O2/O3.
This is the highest-confidence direct explanation for the `198` unclaimed
async pairs.

Official `SliceFilter::StartAsyncSlice(timeStamp, pid, threadGroupId, ...)`
constructs:

- `callid = UpdateOrCreateThread(timeStamp, threadGroupId)`: the logical
  async owner/process lane from the marker payload PID;
- `child_callid = UpdateOrCreateThread(timeStamp, pid)`: the physical thread
  that emitted the start marker.

`PrintEventParser::ParseStartEvent` passes the raw event's common `pid` as the
first identity and the parsed marker `tgid_` as the second identity. The
official table documentation states the same rule: for a non-NULL cookie,
`callid` is the process-unique identity and `child_callid` is the child/start
thread identity.

Codrax currently selects only `itid` and `callid`, does not inspect
`child_callid`, and resolves `callid` as `EmitterITID` for every row shape.
That is valid for synchronous rows but wrong for this official async schema.
The raw exact-pair join then compares a logical owner lane to a physical
emitter lane. Async starts emitted by the owner thread can match
coincidentally; cross-thread starts do not.

Frozen repair:

1. inspect `child_callid` as an optional column;
2. only for a proven official completed async shape (`cookie` non-NULL), keep
   `callid` as the logical owner and require a strict `child_callid` for the
   physical start emitter when the column is present;
3. resolve both through the same canonical thread/process/lifecycle authority;
4. preserve namespace marker PID separately from host emitter TGID;
5. use `child_callid` only for the start emitter envelope and start CPU;
6. continue requiring the raw `F` endpoint to prove the physical finish
   emitter and CPU; never infer either from `child_callid`;
7. if the optional column is absent or invalid, retain a typed interval with a
   reason-specific unavailable status rather than falling back across
   identities;
8. add an official-schema fixture where owner and emitter differ, plus
   namespace-PID and same-thread control arms.

No timestamp tolerance, ordinal pairing or count-based pairing is authorized.

O1 implementation selects the producer schema once from the presence of
`child_callid`. For a completed cookie-bearing row it resolves `callid` and
`child_callid` independently, uses the latter for the physical start envelope,
and requires an optional `itid` to converge with the child emitter. It
preserves logical owner IPID/PID separately from emitter TID/TGID, checks both
lifecycles at the start, and exposes
`source_rows_official_async_child_emitter_resolved`. A missing or invalid
child fails closed with an `async_child_*` reason; it will be retained as an
uninterpreted source row by O2 rather than reassigned to the owner thread.

Tests cover different owner/emitter threads, the same exact raw S/F
replacement path, namespace owner PID distinct from host emitter TGID, a
missing-child fail-closed arm, and the legacy schema without
`child_callid`.

### OSP-02 (P0) — lossless SQL-to-text residual carrier

Standard systrace output is a compatibility projection, not a lossless carrier
for the official relational model. Today the temporary TraceStreamer DB is
deleted unless `--keep-trace-db` or `--trace-db-output` is selected. The
tracebundle therefore cannot recover interval relations or unsupported
non-empty families after conversion.

The next foundational batch must add a versioned textual residual carrier to
the generated `.systrace`:

- every official non-empty table is classified as standard-event,
  typed-interval, typed-relation, typed-counter, resolver-metadata, or
  diagnostic-only;
- every source row gets a deterministic disposition:
  `standard_exact`, `typed_exact`, `resolver_consumed`, `duplicate_exact`, or
  `rejected(reason)`;
- fields omitted by a standard row remain available in a typed residual
  record, keyed to the same stable source identity;
- SQLite storage class is preserved for every typed cell:
  `NULL`, signed INTEGER, REAL bit pattern, exact TEXT bytes (including NUL or
  non-UTF-8 bytes), or BLOB bytes;
- column order, table name, source row identity, chunk order and content hash
  are explicit; large TEXT/BLOB values use deterministic chunks rather than
  truncation;
- no row uses SQL `COALESCE`, fuzzy aliases or zero-value repair;
- a final receipt contains schema hash, rows read, rows represented, rows
  rejected and typed-record hash for every non-empty table;
- conversion fails loud if a row disappears between the source census and
  the final receipt.

The carrier is not permission to expose arbitrary SQLite internals as causal
facts. Resolver-only tables remain typed metadata. Query consumers must opt
into a versioned table family and its semantic adapter before using a row as a
span, CPU lane or causal edge.

#### O2 implementation result

Status: implemented and verified locally; this batch is committed/pushed
independently before O3 starts.

The generated `.systrace` now contains two deliberately separate lanes:

1. the existing standard ftrace/systrace projection for exact semantic events;
2. `# codrax_trace_db_record/v1` comment records for lossless SQLite storage.

The typed lane enumerates every non-internal table in sorted
`sqlite_master.name` order, including empty tables. For each table it emits:

- one schema record containing exact table name bytes, creation SQL and
  `PRAGMA table_xinfo` column order/metadata;
- one row record for every physical source row, including exact `rowid` for
  ordinary tables; `WITHOUT ROWID` tables use declared primary-key order;
- one receipt containing source row count, row-chunk count, schema SHA-256 and
  ordered row-payload SHA-256.

Each cell is tagged with its actual SQLite storage class. INTEGER uses
canonical signed decimal, REAL uses the exact IEEE-754 64-bit pattern, and
TEXT/BLOB use unpadded base64url over the exact returned bytes. A source
payload larger than 32 KiB is split into deterministic chunks; every physical
chunk has its own SHA-256 and every logical record repeats its full-record
SHA-256. No source value is clipped to the one-line display limit.

The parser treats this wire as known, strictly validated syntax. It verifies
canonical numbers/base64, chunk geometry and hashes, then counts and discards
the row before relation pruning and `MaxEvents` admission. Consequently:

- preservation rows do not occupy the in-memory event index;
- preservation rows cannot become scheduler/span/causal evidence;
- SQL artifact capability reports them as `advisory_rows`;
- `authoritative_known = known - advisory_rows`;
- publication still fails if the physical typed-record count differs from the
  exporter receipt.

The all-storage fixture covers `NULL`, minimum-side signed INTEGER, exact REAL
bits, TEXT containing NUL and invalid UTF-8, BLOB, a 70 KiB multi-chunk value,
ordinary tables, `WITHOUT ROWID`, every-table schema/receipt parity, row-count
conservation, cold query-index isolation and artifact authority accounting.
The deterministic same-input fixture now pins standard authority separately
from typed preservation rows.

This closes “the temporary DB is deleted and unsupported information
disappears.” It does not claim that every table has query semantics. O3/O4/O5
still promote official relations and family-specific lanes without inventing
generic B/E spans.

### OSP-03 (P1) — `dur=-1` is unfinished, not generic malformed duration

Status: O1 diagnostic split implemented; typed source-row preservation is an
O2 carrier obligation.

Official SQL and receiver code consistently distinguish `dur=-1`/NULL as an
unfinished interval and set `nofinish`. Codrax currently reports every
negative `callstack.dur` as `invalid_duration`; REP5 contains `101` such rows.

The immediate repair is diagnostic and preservation only:

- split `unfinished_duration_sentinel` from malformed negative duration,
  invalid storage class and overflow;
- preserve start, identity, name, nesting and `completion=unobserved` in the
  typed lane;
- do not emit a naked B row;
- do not set `end=trace_end` or turn the display width into measured duration;
- exclude unfinished duration from wall-time/root-cause arithmetic unless a
  query explicitly requests open-interval overlap with the required caveat.

SmartPerf's “draw until trace end” behavior is useful for a canvas, but is not
an observed finish timestamp and must not enter hard evidence.

### OSP-04 (P1) — exact frame/callstack/GPU relations are discarded

Status: O3 implemented and verified locally; awaiting the independent O3
commit/push recorded below.

The official schema supplies stronger causal input than name/time heuristics:

- `frame_slice.callstack_id` references the exact `callstack` row associated
  with the frame;
- `frame_maps` links an app frame row to a RenderService frame row;
- `gpu_slice.frame_row,dur` attaches GPU render duration to a frame.

Codrax exports `frame_slice` and `frame_maps`, but ignores
`frame_slice.callstack_id` and has no `gpu_slice` exporter. `gpu_slice` is
empty in REP5, so it is not this customer's current loss. The uninspected
`callstack_id` relation can still affect the customer's causal projection and
needs an exact presence/value census in the next replay.

Frozen repair:

1. decode frame and callstack stable identities with their selected producer
   profiles;
2. retain an exact typed frame-to-callstack relation;
3. add `gpu_slice` as a frame resource-duration relation with no fabricated
   CPU/thread header;
4. publish missing/ambiguous/dangling endpoint counts;
5. let causal projection consume only admitted exact relations and label the
   producer relation separately from inferred causal direction.

#### OSP-04 implementation result

The converter now builds an exact frame relation roster from only
`frame_slice.id,ts,callstack_id`. This roster is intentionally independent of
the S/F frame-span gates: lifecycle, CPU, owner identity, duration, kind and
flag rejection can suppress a physical frame span without deleting an exact
SQL foreign-key relation.

Three frame-family surfaces now use that roster:

- existing `frame_maps` no longer requires both frames to have survived span
  admission;
- `# codrax_frame_callstack/v1` preserves an exact unique
  `frame_slice.callstack_id -> callstack.id` edge;
- `# codrax_frame_gpu/v1` preserves `gpu_slice.id,frame_row,dur`.

The GPU wire uses referenced `frame_slice.ts` only as a sorting/query anchor.
The typed payload says resource duration explicitly; no CPU, thread or GPU
start timestamp is invented and no B/E pair is produced. Invalid, duplicate
and dangling identities are withheld separately in coverage instead of being
collapsed into generic parser loss. A fixture pins that `frame_maps` survives
even when the corresponding frame span is rejected.

### OSP-05 (P1) — mixed trace+perf is supported, `perf_napi_async` is missing

Status: O3 implemented and verified locally; awaiting the independent O3
commit/push recorded below.

Mixed trace+perf in one OpenHarmony binary is confirmed. The 1024-byte
`ProfilerTraceFileHeader` distinguishes protobuf trace data (`dataType=0`),
Hiperf data (`dataType=1`) and standalone plugin data (`dataType=1000`).
`PbreaderParser::ParseDataRecursively` consumes consecutive segment headers
and dispatches embedded Hiperf data to `PerfDataParser`. Each `perf_sample`
retains its original sample time and calibrated `timestamp_trace`; when other
trace data already defines `trace_range`, perf completion deliberately keeps
that primary range.

Codrax already handles the main official perf tables:
`perf_sample`, `perf_thread`, `perf_report`, `perf_files` and
`perf_callchain`. It also avoids producing a duplicate standalone perftrace
when the TraceStreamer DB already contains query-ready perf samples.
Therefore “embedded perf is wholly unsupported” is false.

The missing official relation is `perf_napi_async`. Official code builds it
after native-hook and Hiperf parsing to join a native `napi:<traceid>` async
work item, callstack slice, perf callchain and perf sample. Codrax has no
production reference to the table. The repair must retain its exact
`traceid`, CPU, thread/process, caller/callee callchains, perf sample, count and
event-type identities as a typed relation. It must not turn sample-to-sample
display width into span duration.

#### OSP-05 implementation result

`# codrax_perf_napi_async/v1` now carries the official row's exact timestamp,
row ID, CPU, public TID/PID, native caller callchain, Hiperf callee callchain,
perf-sample row, event count, event type and trace ID. Trace ID TEXT bytes use
canonical unpadded base64url on the wire so spaces, separators and non-ASCII
text cannot reopen token parsing.

Publication is fail-closed at exact endpoints:

- `perf_sample_id` must resolve to one unique canonical `perf_sample.id`;
- `timestamp_trace`, CPU, TID, callee callchain, event count and event type
  must converge with that sample row;
- `perf_thread` must provide one unique matching `thread_id -> process_id`.

The caller callchain is retained as native-hook identity and is deliberately
not misclassified as a `perf_callchain` endpoint. The result is a typed point,
not a span. Parser, timestamp extraction, event search, public JSON surface
and side-table memory accounting are pinned.

### OSP-06 (P2) — official table-family coverage matrix

The pinned official build registers 89 tables. Codrax currently has a useful
non-empty-unclassified inventory, so unsupported data is not silently ignored,
but “query-ready systrace” proves parseability of selected lanes rather than
semantic completeness of every captured family.

| Official family | Principal tables | Current Codrax state | Required action |
| --- | --- | --- | --- |
| core scheduling/identity | `process`, `thread`, `sched_slice`, `thread_state`, `raw` | covered with typed integrity gates | retain |
| ftrace slices | `callstack`, `task_pool`, `app_startup`, `static_initalize`, `syscall`, `dma_fence` | covered/partially covered | add residual-field receipt |
| frame/render | `frame_slice`, `frame_maps`, `gpu_slice`, `animation`, `dynamic_frame` | first three exact core relations covered; animation/dynamic-frame preserved but uninterpreted | typed adapters for remaining tables |
| IRQ/instant/log | `irq`, `instant`, `log`, `hisys_all_event` | covered | add residual-field receipt |
| counters | `process_measure`, `live_process`, `xpower_measure` | covered | retain |
| embedded Hiperf | five `perf_*` catalogs/sample tables | covered | retain |
| Hiperf/native async relation | `perf_napi_async` | exact typed point relation covered | retain |
| native allocation | `native_hook` | covered | retain |
| native resolver/statistics | `native_hook_frame`, `native_hook_statistic` | absent | typed resource adapters |
| eBPF I/O/VM | `file_system_sample`, `paged_memory_sample`, `bio_latency_sample`, `ebpf_callstack` | O4a official typed intervals and qualified callchains covered | retain; add downstream aggregate consumers separately |
| ArkTS | `js_cpu_profiler_*`, `js_heap_*`, `js_config` | absent | typed profiler/heap artifact adapters |
| memory/GPU snapshot | `memory_*`, `smaps`, `sys_mem_measure`, `sys_event_filter` | mostly absent | typed snapshot/counter adapters |
| device/Hisysevent | `device_state`, `hisys_event_measure`, `app_name`, `device_info` | partial | typed metadata/counter adapters |
| XPower detail | `xpower_app_*`, `xpower_component_top` | absent | typed counter/detail adapters |
| resolver/config | `args`, `data_dict`, `symbols`, `clock_snapshot`, `datasource_clockid`, `trace_config`, `meta`, filters | partially consumed | residual metadata receipt |

Implementation order after OSP-01/02 is evidence-driven:

1. exact frame/render and perf/native relations;
2. eBPF file/VM/BIO and native-hook resolver lanes;
3. animation/dynamic-frame and memory/GPU counters;
4. ArkTS heap/CPU profiler and XPower detail.

#### O4a implementation result — official eBPF intervals

Status: implemented and verified locally; this is the next independent
commit/push batch after O3.

The official eBPF interval families are now query-visible through
`# codrax_ebpf_interval/v1`:

- `file_system_sample`: exact open/close/read/write summary type, interval,
  internal identity, callchain and nullable return/error/fd/file/size/argument
  fields;
- `paged_memory_sample`: exact interval, type, size and address;
- `bio_latency_sample`: exact latency interval, type, tier, size, block,
  path and duration-per-4K fields.

Admission requires strict INTEGER storage for governed scalars and exact
`end_ts-start_ts=dur` (or `latency_dur`). Official unsigned values projected
through SQLite signed integers are carried bit-exactly. The one official
schema/implementation mismatch, `bio_latency_sample.path_id` declared TEXT but
returned through an integer setter, accepts only INTEGER or canonical decimal
TEXT and is isolated to that field.

`ebpf_callstack` becomes available only when its source row identities are
unique and each callchain has one unique contiguous zero-based depth set.
An interval with a missing/poisoned callchain is still real interval evidence:
it is emitted with `callchain_status=unavailable`; only the callchain edge is
withheld. The official `-1` sentinel is emitted as
`callchain_status=absent`.

No eBPF table supplies a CPU, so the typed event uses `CPU=-1` and never
fabricates an ftrace header. Internal `ipid/itid` always survive. Public
PID/TID appear only when the shared identity roster, exact IPID match and
lifecycle interval gate all agree; otherwise an explicit
`unavailable|mismatch|lifecycle_rejected` status is published. Family details
are canonical, schema-checked JSON inside the versioned base64url wire.
Invalid UTF-8 or an overlong typed row is withheld from semantic publication
with an exact coverage reason while O2 still retains its source bytes.

An official table being empty in a capture is `supported_absent`, not a
conversion failure. A non-empty table without an adapter is
`typed_preserved_uninterpreted`, not “query-ready causal evidence.”

### OSP-07 (P1) — family completeness authority

The tracebundle needs a per-family capability vector in addition to the
current global `trace_query_ready` bit:

- `source_rows`;
- `standard_rows`;
- `typed_rows`;
- `resolver_rows`;
- `rejected_rows` by exact reason;
- `relation_endpoints_missing`;
- `semantic_adapter=ready|preserved_uninterpreted|unsupported`;
- schema/content hashes.

Positive evidence from a ready family remains queryable when another family is
unsupported. Absence claims and cross-family causal projections must disclose
the relevant family state. No noisy coverage ratio becomes a hard gate; exact
row conservation and exact relation endpoint checks may fail loud.

### Frozen delivery batches before customer replay

| Batch | Priority | Deliverable | Replay gate |
| --- | --- | --- | --- |
| O1 | P0 | `child_callid` official async emitter semantics, `dur=-1` split, exact fixtures | existing REP5 should reduce or explain all `198` unmatched intervals |
| O2 | P0 | lossless SQL typed residual text carrier and per-table conservation receipt | every non-empty official table/row has a disposition; byte/chunk hashes close |
| O3 | P1 | `frame_slice.callstack_id`, `gpu_slice`, `perf_napi_async` typed relations | exact endpoint/dangling census and query parser fixtures pass |
| O4 | P2 | eBPF/native/frame-family typed adapters | non-empty fixture rows appear as typed lanes without invented headers |
| O5 | P2 | memory/ArkTS/XPower family adapters and capability vector closure | full 89-table schema fixture has no silent non-empty family |

Each batch is committed and pushed independently. Customer replay starts only
after O1-O3 and the all-table conservation fixture are green. Diagnostic
reports remain bounded; the generated systrace itself is not truncated merely
to satisfy the diagnostic line cap.

Current progress:

- O1: implemented, verified and pushed in `c0dd22bea`;
- O2: implemented, verified and pushed in `147d40504`;
- O3: implemented and locally verified; commit/push is the current batch;
- O4a: official eBPF intervals implemented and locally verified; commit/push
  is the current batch;
- O4b-O5: pending;
- customer replay: deliberately not requested yet.

## REP6 replay audit and repair batches (2026-07-28)

Replay input:

- `/Users/han/opt/customlogs/rep6.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-rep6.txt`;
- customer build executable SHA-256
  `de6a31d6c3cf7559e3441cc686436b2249810815c23effab7c064e60351735b7`.

### R6-01 (P0) — query-visible pre-capture relation timestamps expand the trace by 13.5 hours

Confirmed. The source capture is the same approximately 2.3-second trace
around `69326..69328`, but REP6 reports
`first_timestamp_sec=20857.394782085` and
`last_timestamp_sec=69328.343094`. The new first timestamp is not a converted
span endpoint. It comes from a pre-capture `frame_slice` row admitted by
`loadTraceDBFrameRelationRoster`, then published as a query-visible
`frame_slice_callstack`/GPU relation timestamp. The semantic frame exporter
already rejects the same row as `before_capture_start`.

This is a correctness bug even when a particular external viewer ignores
Codrax typed comments: Codrax and any typed-aware consumer sees a synthetic
approximately 13.5-hour envelope, so valid 2.3-second spans appear visually
collapsed.

Repair:

- keep every pre-capture SQL row in the O2 exact fidelity stream;
- exclude its timestamp from the query-ready frame relation roster using the
  precise `trace_range.start_ts` authority;
- report `before_capture_start=N` in relation-roster coverage;
- pin that in-capture relations remain independent of frame span
  CPU/lifecycle admission.

### R6-02 (P0) — “preserved” is not “visible in the official viewer”

Confirmed. REP6 has three distinct classes and they must not be merged:

| Class | REP6 count | Current disposition | Official/generic viewer |
| --- | ---: | --- | --- |
| truly unpublished closed sync spans | 176 | local fence, no raw replacement | missing |
| CPU-unavailable callstack source rows after raw replacement | 9,521 | Codrax typed comment lane | invisible |
| completed async intervals without proven physical finish emitter | 62 | Codrax typed comment lane | invisible |

The first class is actual conversion loss. The latter two are lossless for
Codrax but still a customer-visible compatibility gap. O2 SQL fidelity rows
also do not make these spans visible in the official viewer.

Frozen acceptance rule:

- a span counts as official-viewer-published only when emitted through a
  standard syntax accepted by the pinned OpenHarmony SmartPerf parser;
- typed comments remain a conservation/audit layer and never count as
  official-viewer span success;
- CPU, thread, owner, namespace PID or finish emitter must never be invented
  merely to make a viewer draw a span;
- viewer-visible, Codrax-typed-visible and source-preserved counts are
  reported separately.

R6-D implements that receipt at the final reconciled authority:

- `official_viewer_standard_spans_emitted` counts standard callstack B/E,
  source-raw B/E and raw-proven S/F intervals only;
- `codrax_typed_only_sync_spans_emitted` and
  `codrax_typed_only_async_intervals_emitted` count versioned comment lanes
  which the official viewer does not display;
- `callstack_closed_sync_spans_unpublished` reports net loss after exact raw
  replacement closure;
- `official_viewer_span_visibility` is a typed
  `complete|degraded_*|not_evaluated_*` state.

The customer caveat now says “official SmartPerf/generic systrace viewer does
not display” instead of the weaker “may omit”, and explicitly states that
typed-only conservation is not official-viewer conversion success.

### R6-03 (P0) — 18,033 exact raw spans are rejected only by Codrax's microsecond wire profile

Confirmed by the raw-marker ledger. REP6 retains 83,975 structurally closed
raw pairs but withholds 18,033 because the current six-decimal timestamp wire
rounds the endpoints to the same microsecond. This is not missing source data
and not an OpenHarmony parser failure. It is a Codrax output-profile
limitation.

Official parser evidence is now pinned to
`developtools_smartperf_host@5c5afb0c479b070148d8a6e336120638a1a03930`:

- `ptreader_parser.h:133-134` matches timestamps with `(\d+\.\d+)`, so the
  fractional part has no six-digit ceiling;
- `ptreader_parser.cpp:298-315` parses that value and multiplies it by `1e9`
  for `BytraceLine.ts`;
- `BytraceEventParser::TracingMarkWriteOrPrintEvent` passes standard
  `tracing_mark_write` B/E/S/F rows through the official print-event parser.

R6-B is therefore implemented without a private viewer protocol:

- microsecond-aligned rows keep the conventional six digits;
- rows requiring more precision use nine digits;
- the sorter key remains the exact timestamp printed on the wire;
- every strictly increasing nanosecond interval is representable;
- raw-marker sync, async, frame and raw DMA wait consumers share the same
  exact gate.

The former 18,033 REP6 rejections can now enter the normal standard B/E
publication pipeline. Final publication remains subject to the unchanged
identity, lifecycle, name-drift, duplicate and laminar-span gates, so a future
replay must report the exact restored count instead of assuming all 18,033
survive later independent checks. Restored raw spans may also replace part of
the 9,521 CPU-unavailable typed-only population because the raw endpoints
already carry authoritative CPU/emitter/name evidence.

### R6-04 (P1) — local-fence closure is still broader than the actual bad rows

The 176 remaining sync spans are all locally fenced. O2 proves their source
rows survive, but does not make them viewable. Audit the 101 pre-pairing
rejections against each fence's exact `(itid, depth, start, end, row id)`
provenance. Narrow only where one precise bad row currently suppresses an
unrelated valid pair. Ambiguous nesting remains fail-closed.

REP6 exposed only aggregate fence counts, which is insufficient to repair a
future residual without asking the customer for another replay. The
diagnostic arm now emits, on the existing callstack coverage row, at most
eight exact declarations:

`tid / canonical itid / interval-or-suffix / start_ns / end_ns / closed reason`.

It also emits typed `localized_fence_witnesses_emitted` and
`localized_fence_witnesses_omitted` counts. These witnesses are diagnostic
geometry only and never become alternate CPU, identity or span publication
authority. The cap keeps customer diagnostics bounded to one existing JSON
line rather than adding an unbounded event roster.

### R6-05 (P1) — async standard publication lacks an official-viewer-safe fallback

The 62 typed completed async intervals include 58 raw-async identity-key
mismatches and four start mismatches. Reconcile namespace PID, host emitter
TID, `child_callid` and raw marker owner against the official parser's async
key construction. Emit standard `S/F` only when both physical endpoint
emitters and CPUs are proven. Otherwise retain the typed interval and report it
as viewer-invisible; never synthesize an endpoint.

The 58-row identity class is now repaired. The previous join incorrectly used
the DB logical-owner process PID as if it were the raw `S/F` payload PID.
Those identities may legitimately differ in Donghu/PID-namespace captures.
The fallback join now requires one unique raw pair with exact
`name+cookie+start+end`, then independently converges the physical start TID,
host TGID and known CPU. Publication keeps the raw payload PID and both raw
physical endpoint envelopes byte-semantically intact. Two different raw
payload-PID candidates with the same shared coordinates remain ambiguous and
fail closed.

The four start mismatches are not covered by that correction: a timestamp may
not be shifted or fuzzily joined merely to increase viewer coverage. They
remain typed-only until a precise producer clock/association rule is proven.

### R6-06 (P1) — O2 fidelity records dominate conversion cost

REP6 makes the performance bottleneck conclusive:

- trace_streamer DB export: 2.4 seconds;
- DB normalization: 73.2 seconds;
- output: 1,237,948 rows / 774,310,090 bytes;
- O2 advisory fidelity rows: 829,327;
- global sorter: 39.096 seconds, 17 spills and 1,775,695,414 temporary bytes;
- post-validation: 7.074 seconds.

The exact SQL fidelity lane is over half the output rows and is passed through
the semantic global sorter even though its own schema/row/receipt order is
already authenticated. The performance repair keeps all rows and hashes:

1. add separate elapsed/byte/row timings for SQL scan, encoding, spill, merge,
   fidelity append and post-validation;
2. cache the immutable DB schema/table/row-count inventory and remove repeated
   metadata scans;
3. append one authenticated O2 tail after sorted semantic events in strict
   `schema -> row chunks -> receipt` order, then validate its row counts and
   SHA-256 receipts;
4. optimize the validation parser without removing full validation;
5. use a versioned binary private spill record only if the preceding changes
   leave the semantic sorter dominant.

Disallowed shortcuts remain: disabling O2, skipping raw recovery or full
validation, deleting ordering, fabricating CPU/identity, or merely increasing
the memory ceiling.

R6-E implements the highest-leverage safe arm:

- semantic viewer rows alone enter the global timestamp sorter;
- O2 is written once to a bounded private tail in its already-canonical
  `table -> schema -> row/chunk -> receipt` order;
- sealing binds exact bytes, row count, anchor timestamp and SHA-256;
- publication reopens and re-hashes every tail byte while appending it after
  the sorted semantic rows; size, truncation or hash drift fails closed before
  the buffered tail is flushed;
- the unchanged final tracequery postvalidator still parses every O2 chunk and
  verifies sequence, chunk hash, record hash, table row count and receipt hash;
- sorter coverage splits `semantic_rows_sorted`,
  `authenticated_tail_rows` and `authenticated_tail_bytes`, while
  `sql_text_fidelity.elapsed_us` now measures the previously invisible SQL
  scan/encoding/tail-spool phase.

This removes the REP6-shaped O2 rows from JSON spill encoding, 17-way global
sorting and merge without deleting one output row or weakening one receipt.
The physical systrace body remains deterministic: all O2 rows already share
the maximum semantic timestamp, and their canonical sequence is now explicit
instead of depending on sorter sequence ties.

R6-F then removes avoidable per-record allocation without changing the wire:

- SQLite scan destinations are allocated once per table and reused after each
  row has been fully typed, hashed and emitted;
- the tracequery O2 parser uses exact ordered field cuts instead of
  `strings.Split`;
- strict raw-URL base64 decoding replaces decode-then-re-encode canonicality
  checks;
- canonical SHA-256 text is decoded/compared without temporary hex buffers;
- multi-chunk final-record SHA comparison reuses one fixed digest scratch
  instead of allocating `Sum(nil)` plus `hex.EncodeToString` per logical row.

The parser microbenchmark on darwin/arm64 is pinned at 277 ns/op, 128 B/op and
2 allocs/op for a representative O2 row. The remaining allocations are
confined to decoding and returning the exact payload required by the full
logical-record SHA validator; it is not removed by weakening validation.

### REP6 frozen delivery order

| Batch | Priority | Work | Independent push gate |
| --- | --- | --- | --- |
| R6-A | P0 | pre-capture relation fence and trace-envelope regression | exact roster/coverage fixtures green |
| R6-B | P0 | official SmartPerf timestamp grammar audit; exact-nanosecond standard raw spans when proven | official-parser evidence plus sub-microsecond B/E round-trip fixture |
| R6-C | P1 | narrow local-fence loss; reconcile raw async identities | every newly visible span has exact source CPU/emitter authority |
| R6-D | P1 | split viewer-visible / typed-visible / fidelity-only receipts and caveats | no typed comment counted as official-viewer success |
| R6-E | P1 | P0 timings and authenticated O2 tail fast path | row/cell/hash conservation unchanged; semantic output unchanged |
| R6-F | P2 | metadata cache, scan-buffer reuse and validator allocation reduction | benchmark improves without weakening full verification |

Customer replay is needed after R6-B through R6-F are pushed. Existing REP6 is
sufficient for implementation and regression design; asking the customer to
replay before these code-side repairs would only reproduce known failures.

Progress:

- R6-A: implemented, verified and pushed in `d3aaf7563`;
- R6-B: implemented, verified and pushed in `b9c33281f`;
- R6-C async namespace-PID arm: implemented, verified and pushed in
  `bb218290e`;
- R6-D: implemented, verified and pushed in `d7dde30e6`, including removal of
  the obsolete microsecond-alignment gate for standard pipe-name spans;
- R6-E: implemented, full-suite verified and pushed in `99aced87c`;
- R6-F: implemented, both owning packages fully verified and pushed in
  `3cfb04a79`;
- R6-C local-fence arm: replay-gated. R6-B can now admit the formerly
  microsecond-collapsed raw pairs and R6-C already repairs 58 async
  namespace-PID joins, so the old REP6 count of 176 is no longer a valid
  residual roster. Narrowing fences before a new exact residual witness would
  risk publishing ambiguous nesting and is therefore prohibited. The bounded
  exact-fence diagnostic needed by that replay is implemented, fully verified
  and pushed in `f481fa02a`.

### REP7 replay gate

The next customer replay is now useful and should be the first replay after
the full R6-A through R6-F sequence. Acceptance is not “conversion exited 0”:

1. `trace_cross_validation.first_ts` must be inside the captured
   `trace_range`, not the former pre-capture 20,857-second relation timestamp;
2. sorter coverage must contain nonzero `semantic_rows_sorted`,
   `authenticated_tail_rows` and `authenticated_tail_bytes`, with total
   rows still equal to postvalidation `expected_rows=parsed_known`;
3. `official_viewer_standard_spans_emitted` must increase from exact
   nanosecond raw recovery and the namespace-PID async join;
4. `callstack_closed_sync_spans_unpublished`,
   `codrax_typed_only_sync_spans_emitted` and
   `codrax_typed_only_async_intervals_emitted` are the only authoritative
   residual work rosters; when the first is nonzero,
   `localized_fence_witnesses` provides the bounded exact next-step geometry;
5. `official_viewer_span_visibility=complete_for_governed_callstack` is the
   only state that permits a full official-viewer compatibility claim.

Any `degraded_*` state is a successful lossless Codrax conversion but not a
successful official-viewer span conversion. The residual counts and their
typed fence/join reasons must drive the next code batch; they may not be
hidden behind O2 preservation or converted into guessed standard events.

## REP7 customer replay audit (2026-07-28)

Inputs:

- `/Users/han/opt/customlogs/rep7.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-rep7.txt`;
- customer executable SHA-256
  `04c003ea0d05a9d642cbac9552cd0ea4774583aae38810caeb86659c59817946`.

The executable reports `build_revision=unknown`, but its capability roster
contains all R6-A through R6-F production capabilities, including
`clock_regression_first_witness_v1`,
`callstack_local_fence_witness_v1`,
`sql_mixed_precision_wire_sort_v1` and `sql_text_fidelity_v1`. The replay is
therefore admissible as a post-R6 result; the missing revision remains a
release-identification hygiene gap, not a reason to discard the exact
coverage counters.

### REP7-01 — fixes proven effective

| Gate | REP6 | REP7 | Verdict |
| --- | ---: | ---: | --- |
| first output timestamp | 20857.394782085 s | 69326.012181718 s | pre-capture pseudo-time fixed |
| DB normalization | 73.225 s | 44.102 s | 39.8% faster |
| semantic sorter | 39.096 s / 17 spills / 1.776 GB temp | 7.962 s / 3 spills / 0.241 GB temp | authenticated O2 tail effective |
| O2 fidelity | 829327 rows, mixed into sorter | 829327 authenticated tail rows / 691010668 bytes | all rows retained outside semantic sort |
| postvalidation | 7.074 s | 8.037 s | full validation retained |
| raw standard B/E recovery | 23479 spans | 29889 spans | +6410 exact source spans |
| CPU-unavailable residual after raw replacement | 9521 rows | 3209 rows | -6312 |
| closed sync spans unpublished after replacement | 176 | 78 | -98 |

The artifact is query-ready and exact SQL preservation is complete, but the
viewer gate remains failed:

- `official_viewer_standard_spans_emitted=42809`;
- `codrax_typed_only_sync_spans_emitted=53171`;
- `codrax_typed_only_async_intervals_emitted=62`;
- `callstack_closed_sync_spans_unpublished=78`;
- `official_viewer_span_visibility=degraded_typed_only_and_unpublished`.

Thus REP7 is not a complete official/generic viewer conversion. The
1,237,789 total output rows and 829,327 O2 rows are conservation evidence,
not viewer-visible span evidence.

### REP7-02 (P1) — typed-only sync census is not actionable

The authority reports 53,171 emitted sync spans on the Codrax-only lane, but
does not split them by the closed publication decisions:

- unknown start CPU;
- unknown end CPU;
- tainted scheduler witness;
- lifecycle-rejected scheduler witness;
- ambiguous same-public-TID scheduler alias;
- CPU-known name not losslessly representable in standard trace-mark syntax.

The gross pre-pairing `source_rows_preserved_cpu_unavailable=31914` cannot be
used as this split because later raw replacement, laminar suppression and
publication change the population. Add the reason census at the final shared
sync authority, after all suppression and supersession decisions. Its sum
must equal `codrax_typed_only_sync_spans_emitted`; no reason may grant CPU or
viewer authority.

### REP7-03 (P1) — local-fence witness lacks the rejected source fact

REP7 proves:

- 101 pre-pairing rows rejected as `invalid_duration`;
- 90 localized fence declarations;
- 1,262 otherwise admitted sync spans suppressed by those fences;
- 78 remain unpublished after exact raw replacement.

The eight bounded witnesses are all suffix fences, for example
`tid=38326/itid=38/start_ns=69326013638593`, but every witness has only the
generic reason `rejected_callstack_candidate`. It omits the rejected row ID
and the exact SQLite duration storage/value class. That is insufficient to
distinguish a producer sentinel, a REAL/TEXT schema drift, corrupt storage or
an over-broad admission rule. Preserve a bounded rejected-row witness on the
existing callstack coverage line with:

`row_id / tid / itid / ts / reason / exact bounded dur scalar`.

This evidence is diagnostic only. The suffix fence must not be narrowed until
the precise rejected value proves a safe producer rule.

### REP7-04 (P1) — the R6 async namespace repair did not close the customer cohort

REP7 still has exactly 253 matchable raw async pairs, of which 191 are
claimed and 62 remain unclaimed. The residual classification is unchanged:

- 58 `raw_async_official_intervals_identity_key_mismatch`;
- 4 `raw_async_official_intervals_start_mismatch`.

The implemented cross-payload-PID join is present, but did not cover these
rows. The current `identity_key_mismatch` bucket conflates payload PID, name
and cookie and therefore cannot prove another join rule. Split the mismatch
against exact raw candidates by dimension (`cookie`, `name`, `start`, `end`,
physical begin TID/TGID/CPU, ambiguity) and emit at most eight bounded
high-level/raw witnesses. Only an exact one-to-one official-parser rule may
publish S/F; timestamp fuzzing and PID rewriting remain prohibited.

### REP7-05 (P2) — remaining performance work is no longer the first blocker

The safe fast path removed 29.1 seconds from normalization. The current
44.1-second normalization consists materially of:

- exact SQL scan/encoding/tail spool: 16.058 seconds;
- semantic sort/merge: 7.962 seconds;
- full output postvalidation: 8.037 seconds;
- remaining semantic export and publication overhead: about 12 seconds.

Further work may cache immutable schema metadata and use a versioned binary
private semantic spill record, but it must not delay the viewer-visibility
work or weaken O2/full-output verification.

### REP7 frozen delivery order

| Batch | Priority | Work | Push gate |
| --- | --- | --- | --- |
| R7-A | P0 audit | record replay verdict and freeze exact residuals | document contains all REP7 hard-gate counters |
| R7-B | P1 diagnosis | final-publication typed-only sync reason census | reason sum equals final typed-only sync count |
| R7-C | P1 diagnosis | rejected duration/fence witness with bounded exact scalar | no extra diagnostic line; no fence/admission change |
| R7-D | P1 diagnosis | dimensioned async mismatch census and bounded witnesses | no fuzzy join, no PID rewrite, no synthetic endpoint |
| R7-E | P0 repair | apply only official-parser-backed fence/async/viewer fixes proven by R7-C/D | every added B/E or S/F has source CPU/emitter/time authority |
| R7-F | P2 performance | remaining metadata/spill optimization after visibility closure | exact row/cell/hash and postvalidation invariants unchanged |

Customer replay is not needed before R7-B through R7-D: REP7 already proves
the diagnostic gaps and their exact affected cohorts. A replay is needed
after those batches to obtain the newly dimensioned evidence for R7-E.

Progress:

- R7-A: audit frozen and pushed in `0075e0eb9`;
- R7-B: implemented and pushed in `772932bab`. The final shared sync authority now classifies every
  emitted typed-only span into one of the five closed CPU-placement reasons or
  the lossless-name representability reason. The producer coverage and
  semantic-quality summary carry the same counters, and a typed
  `official_viewer_typed_only_sync_reason_census=complete` receipt requires
  their sum to equal the final typed-only sync count. A mismatch changes the
  viewer state to `not_evaluated_typed_only_reason_census_mismatch`; it cannot
  silently claim coverage.
- R7-C: implemented and pushed in `3c0bb5995`. The existing callstack coverage row now carries at most
  eight `rejected_callstack_fence_witnesses`, each binding rejected row ID,
  resolved physical TID/canonical ITID, exact timestamp, closed rejection
  reason and a bounded exact SQLite duration scalar. INTEGER and REAL retain
  their exact value/bits; short TEXT/BLOB retains base64url bytes; larger
  values retain byte length plus SHA-256. Emitted/omitted counters are typed.
  The witness does not change the existing fence or publication decision and
  does not add another diagnostic line.
- R7-D: implemented and pushed in `7595f586c`. Raw async misses are now classified independently as
  exact cookie, name, start, end, whole-interval, split-endpoint, physical
  begin TID/TGID/CPU, already-claimed or ambiguity mismatches. The existing
  callstack coverage row carries at most eight
  `raw_async_mismatch_witnesses`, binding the DB interval and the first exact
  comparison candidate while emitted/omitted counters preserve the bounded
  roster. The indexes exclude payload PID only for diagnosis and the existing
  exact namespace join; they do not create a fuzzy join or publish an
  endpoint.

## REP8 customer replay audit (2026-07-28)

Inputs:

- `/Users/han/opt/customlogs/rep8.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-rep8.txt`;
- customer executable SHA-256
  `b9b76caa232ae74f64fd8abc6a0241257481f52e5015515d0fd7d865c6ac71d8`.

The customer binary still reports `build_revision=unknown`, but its capability
roster contains all three R7 diagnostic capabilities:

- `official_viewer_typed_only_reason_census_v1`;
- `callstack_rejected_scalar_witness_v1`;
- `official_raw_marker_async_mismatch_witness_v1`.

REP8 is therefore an admissible R7-E repair witness. The baseline audited here
is `main@07937aae3`.

### REP8-01 — R7 diagnosis is complete and the viewer failure is unchanged

The final typed-only sync equation is exact:

```text
53171 =
  50043 name_unrepresentable
+  2638 cpu_unknown_start
+   488 cpu_source_tainted
+     2 cpu_unknown_end
```

No lifecycle-rejected or alias-ambiguous final span is hidden in this cohort.
The complete viewer roster remains:

- 42,809 standard-visible spans;
- 53,233 Codrax typed-only spans, of which 53,171 are sync and 62 are async;
- 78 closed sync spans unpublished after raw replacement.

The 1,237,789 output-row receipt and 829,327 authenticated O2 rows remain
losslessness evidence only. They do not change the
`degraded_typed_only_and_unpublished` viewer verdict.

### REP8-02 (P0) — 50,043 sync spans have a safe physical-source replacement

Three independently computed final/producer counters converge on the same
cohort:

```text
official_viewer_typed_only_sync_spans_name_unrepresentable = 50043
raw_pairs_withheld_exact_interval_name_drift               = 50043
raw_pairs_name_drift_unique_cpu_known_callstack_candidate = 50043
```

Every raw candidate already passed the immutable-source decoder, lifecycle,
namespace-PID, physical header TID/TGID/CPU, exact nanosecond interval and
one-to-one collision census. The current code nevertheless withholds it
because replacement is restricted to CPU-unavailable DB candidates. It then
keeps the CPU-known DB candidate on the Codrax-only exact-name lane.

The safe repair is narrower than “allow name drift”:

1. require one and only one locally admitted callstack candidate on the exact
   host/payload/canonical identity and interval;
2. require one and only one raw interval;
3. require that the DB candidate is CPU-known but its complete name is
   demonstrably not losslessly representable by the standard trace-mark
   grammar;
4. supersede only that candidate and publish the original raw B/E pair with
   its original name and independent physical endpoint envelopes.

A CPU-known DB candidate whose name is already standard-representable remains
authoritative and must still withhold a different-name raw collision. The
repair must split CPU-unavailable and name-unrepresentable replacement
counters; the existing `superseded_by_raw_cpu` wording may not absorb both.

### REP8-03 (P0) — all 62 async residuals are exact name-only joins

The R7-D census reports:

```text
raw_async_official_intervals_name_mismatch = 62
raw_async_official_intervals_without_exact_raw_pair = 62
raw_async_pairs_unclaimed = 62
```

Every bounded witness has exactly one raw candidate with:

- equal cookie;
- equal start and end nanoseconds;
- equal physical begin TID, host TGID and CPU;
- a different name only.

For example, the DB interval name
`H:[SCB]traverseSessionTree` is paired with the original raw name
`H:[a92ab5d29d3be69,b1008,33bbc5d]#[SCB]traverseSessionTree`.
OpenHarmony TraceStreamer treats distributed marker metadata as structured
callstack information, while the raw S/F endpoint remains the physical wire
authority. The repair therefore joins only one unique equal-cookie,
equal-interval, equal-begin-envelope raw pair and publishes that pair's
original S/F payloads. It does not rewrite payload PID, compare fuzzy
timestamps, synthesize a finish endpoint or require the DB-normalized display
name to equal the raw wire name.

### REP8-04 (P1) — the duration witness exists but is lost by diagnostic framing

R7-C did collect eight `rejected_callstack_fence_witnesses`, and the metrics
prove 82 more were deliberately omitted. However, the callstack coverage JSON
line also carries about 12 KiB of async mismatch witnesses. The diagnostic
writer caps one physical line at 8 KiB, so the later-sorted duration metadata
is cut out of the returned report. The customer cannot inspect the duration
scalar even though conversion collected it.

This is a diagnostic publication bug, not permission to relax duration
admission. The diagnostic report must emit selected bounded witness values on
their own JSON lines before the full coverage row. The full coverage row may
remain bounded. This keeps the report below 1,000 lines while making the
existing exact scalar reachable.

Until a subsequent replay exposes those values, the 90 suffix/interval fences
and 78 unrecovered closed spans remain fail-closed. In particular, TEXT/REAL
duration coercion, sentinel reinterpretation and suffix-fence narrowing are
not yet authorized.

### REP8-05 — performance is stable and subordinate to visibility

Normalization improved from 44.102 seconds in REP7 to 42.168 seconds in REP8.
The major components remain:

- SQL fidelity scan/encoding/tail spool: 14.676 seconds;
- semantic sorter: 7.039 seconds, three spills and 240,967,642 temporary bytes;
- complete tracequery cross-validation: 8.189 seconds.

No performance regression blocks R8 repair. Schema caching and private binary
spill remain safe later work, but they must not weaken authenticated O2
publication or full postvalidation and must not precede the two exact
viewer-recovery cohorts.

### REP8 frozen delivery order

| Batch | Priority | Work | Independent push gate |
| --- | --- | --- | --- |
| R8-A | P0 audit | record REP8 equations and freeze exact repair predicates | all REP8 hard counters and invariants present |
| R8-B | P0 repair | replace unique CPU-known/name-unrepresentable DB collision with exact raw B/E | standard-representable drift remains withheld; replacement counters split |
| R8-C | P0 repair | claim unique equal-cookie/equal-interval/equal-begin-envelope raw async pair despite DB/raw name drift | ambiguity and every non-name mismatch remain typed |
| R8-D | P1 diagnosis | publish selected bounded coverage witnesses on independent diagnostic lines | duration scalar survives an oversized neighboring metadata value; report stays under 900 lines |
| R8-E | P1 repair | decide duration/fence rule only from the R8-D customer scalar witness | no SQLite coercion or fence narrowing without an exact producer rule |
| R8-F | P2 performance | schema/cache/private-spill work after viewer repair | row/cell/hash and full postvalidation unchanged |

R8-B through R8-D do not need another customer replay: REP8 proves their exact
inputs. The next replay should occur after all three are pushed. Its primary
acceptance equations are:

```text
name_unrepresentable typed-only sync: 50043 -> 0
raw async name mismatch:               62 -> 0
standard-visible spans:             42809 -> at least 92914
rejected duration scalar witness:   independently present in diagnostic
```

The remaining 3,128 CPU-authority sync rows and 78 fenced rows are not promised
by this batch. They require separate physical CPU or exact duration evidence.

Progress:

- R8-A: REP8 audit and repair predicates frozen and pushed in `93b754f6a`;
- R8-B: implemented, package-verified and pushed in `fce2c34ad`. One unique
  CPU-known callstack interval is superseded only after the DB candidate name
  fails the same standard trace-mark round-trip predicate used by final viewer
  classification. The exact raw B/E pair retains its original name and both
  physical envelopes. Standard-representable name drift and ambiguous raw
  intervals remain withheld. CPU-unavailable and name-unrepresentable
  supersession counters are independent, and replacement closure accounts for
  both without relabelling name repair as CPU repair;
- R8-C: implemented, package-verified and pushed in `75aec4e1b`. An official
  completed async interval may now claim one unique raw pair when cookie,
  nanosecond start/end and physical begin TID/TGID/CPU are exact even if the DB
  display name differs. Publication writes the original raw S/F payloads.
  Cookie/time/envelope mismatch and ambiguity remain typed fail-closed;
- R8-D: implemented, command-package verified and pushed in `df4029e22`.
  `rejected_callstack_fence_witnesses` and
  `raw_async_mismatch_witnesses` are emitted as independent bounded diagnostic
  sideband lines before their full coverage row. An oversized neighboring
  metadata value can no longer erase the exact duration scalar. Capability
  `coverage_witness_sideband_v1` identifies the fixed report shape;
- R8-E: replay-gated. No duration coercion or fence narrowing has been made.
  The next customer replay must provide the now-reachable exact duration
  witness first;
- R8-F: deferred behind viewer replay acceptance; no validation has been
  weakened for performance.

Final verification after R8-B through R8-D: `go test ./...` passed.

## REP9 customer replay audit (2026-07-28)

Inputs:

- `/Users/han/opt/customlogs/rep9.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-rep9.txt`;
- customer executable SHA-256
  `6660473ce3f36882ea38cb5656969eff4f658778a4fe92e54a5fc8d7aa538295`.

The binary still reports `build_revision=unknown`, but the capability roster
contains `coverage_witness_sideband_v1` together with the R8-B/C capabilities.
REP9 is therefore an admissible R8 acceptance and R8-E planning witness. The
code baseline audited here is `main@1d0a3398e`.

### REP9-01 — R8-B/C/D all passed their exact acceptance equations

The output has 1,237,851 postvalidated events, including 829,327 authenticated
O2 records. Expected, parsed and callback counts all equal 1,237,851. The
standard-viewer result is:

```text
standard-visible spans = 92914
typed-only spans       =  3128
unpublished sync spans =    78
```

The 50,043 sync name-only collisions were all replaced:

```text
raw_marker_name_unrepresentable_replacement_spans             = 50043
raw_pairs_name_drift_name_unrepresentable_callstack_replaced   = 50043
official_viewer_typed_only_sync_spans_name_unrepresentable     = 0
raw_pairs_withheld_exact_interval_name_drift                   = 0
```

The 62 async name-drift intervals were also all recovered:

```text
raw_async_official_intervals_exact_name_drift_joined = 62
raw_async_pairs_claimed                              = 253
raw_async_pairs_matchable                            = 253
typed-only async spans                               = 0
```

The independent witness sideband is present and exposes all eight retained
duration samples. R8-B, R8-C and R8-D are closed by this replay.

### REP9-02 — the remaining 3,128 typed-only sync spans are a closed CPU-authority roster

The final equation is exact:

```text
3128 =
  2638 cpu_unknown_start
+  488 cpu_source_tainted
+    2 cpu_unknown_end
```

No name, lifecycle or alias reason is hidden in this cohort. These spans retain
identity and duration, but a standard B/E envelope requires physical CPU at
both endpoints. CPU 0, a nearest scheduler row or a process-level CPU cannot
be substituted. The next repair must correlate each final typed candidate to
an exact raw B/E disposition or another physical endpoint authority. Until
then, typed preservation is not standard-viewer success.

### REP9-03 (P1) — `dur=NULL` exposes a potentially over-wide suffix-fence gap

All eight independently published rejected-duration samples are SQLite NULL:

```text
row_id=66  ... reason=invalid_duration/dur=null
row_id=117 ... reason=invalid_duration/dur=null
...
row_id=284 ... reason=invalid_duration/dur=null
```

The producer emits 8 bounded witnesses and reports 82 omitted fence witnesses,
so 90 exact lane fences exist. The shared authority then suppresses 1,262
callstack spans under those fences; 1,184 obtain exact raw replacements and 78
remain unpublished.

The current rule maps any timestamp-known, interval-unknown rejection to a
callstack-producer suffix fence `[ts,+infinity)`. This is safe but can be much
wider than the upstream meaning of a NULL callstack duration: the DB row proves
an unfinished/open slice at its start, not that every later independent slice
on that physical lane is invalid forever.

This replay does **not** authorize deleting or blindly shortening the suffix:

- only the bounded eight scalars are proven NULL; the omitted 82 need a typed
  storage-class census;
- a raw E at a later time is not sufficient by itself; a repair needs one
  unique raw B/E pair with the same physical emitter, canonical owner,
  namespace payload PID, exact start and name;
- ambiguity, name mismatch, invalid raw payload, open raw begin and poisoned
  emitter lane must retain the suffix fail-closed rule.

The optimal next batch therefore records an exact NULL-duration fence to raw
pair correlation census first. Only a unique full-key raw closure may replace
that open DB row and narrow its producer fence to the proven interval. This
uses the raw end as duration authority; it never coerces SQL NULL to zero or
guesses an end.

### REP9-04 (P2) — performance regression has two independent components

On the same customer file:

| component | REP8 | REP9 | delta |
| --- | ---: | ---: | ---: |
| total DB normalization | 42.168s | 46.919s | +4.751s |
| raw sync recovery | 3.437s | 5.296s | +1.859s |
| SQL fidelity `__all_tables__` | 14.677s | 17.564s | +2.888s |
| semantic sorter | 7.039s | 6.250s | -0.789s |
| full tracequery validation | 8.189s | 8.861s | +0.672s |

The raw-sync increase has a code-level hot path: each of the 50,043 new
CPU-known/name-unrepresentable replacements performs a candidate SELECT,
Go-side name representability check and then an UPDATE. Representability is
already an exact immutable candidate property. It can be stored once in the
private staging row and included in the guarded UPDATE predicate, eliminating
the second read without changing collision cardinality, raw authority,
supersession or final validation.

The SQL-fidelity row count remains exactly 829,327 and its authenticated
content contract did not change. The roughly 20% scan/encode increase is not
explained by the R8 repair and may be host/run variance. It must be measured
with phase timing before any format change. O2, row/cell hashes and complete
postvalidation remain mandatory.

### REP9-05 — additional open conversion gaps

1. Raw sync pairing closes 83,970 structural pairs, but 4,038 are withheld by
   local validation: 3,759 `invalid_begin_payload_pid` and 279
   `invalid_span_name`. This is a real residual cohort. It needs bounded
   physical/raw payload witnesses and comparison with the official parser
   grammar before any compatibility extension.
2. Scheduler output still has 816 unknown-comm boundaries, all currently
   summarized under one canonical subject
   `itid=398/tid=29352/tgid=68`; final unresolved thread-name count is one.
   This is narrow but remains an honest name-completeness gap.
3. Upstream `trace_vsync:not_match=79` still means decoded frame ends were not
   associated with a VSync state. O2 preserves the SQL evidence, but no code
   may claim a complete standard frame lane from those rows.
4. `build_revision=unknown` remains a release-attribution gap. Capability and
   executable SHA make this replay auditable, but build revision should be
   embedded by the distribution pipeline.

### REP9 frozen delivery order

| Batch | Priority | Work | Independent push gate |
| --- | --- | --- | --- |
| R9-A | P0 audit | record REP9 acceptance, residual equations and immutable predicates | every count above traceable to REP9 |
| R9-B | P2 performance | persist exact viewer disposition in private candidate staging and remove the 50,043 per-collision SELECTs | output semantics byte-equivalent in fixtures; standard-representable drift still withheld |
| R9-C | P1 diagnosis | dimension NULL/non-NULL invalid durations and correlate exact NULL fence starts with unique full-key raw pairs; add bounded raw local-validation witnesses | diagnostic only; no fence/admission change; report remains below 900 lines |
| R9-D | P1 repair | resolve/narrow only a uniquely proven NULL-duration fence from an exact raw B/E closure | SQL NULL never becomes a duration; ambiguity remains suffix-fenced |
| R9-E | P1 diagnosis/repair | correlate the 3,128 final CPU-unavailable candidates with exact raw pair disposition, then repair only authoritative cohorts | CPU 0/nearest-row/process inference forbidden |
| R9-F | P2 performance | add independent O2 phase timing and optimize only measured scan/encode/postvalidate allocations | authenticated O2 and full tracequery validation unchanged |

R9-B and the diagnostic portion of R9-C can be implemented without another
customer replay. R9-D and R9-E are replay-gated on the new exact correlation
censuses. The next replay should be requested only after R9-B/C are pushed, so
one customer run answers both correctness and performance questions.

Progress:

- R9-A: REP9 audit, acceptance equations and residual task order frozen and
  pushed in `e932743e0`;
- R9-B: implemented. The private sync-span staging row now stores the exact
  final viewer disposition computed from the immutable candidate. The
  name-unrepresentable supersession UPDATE includes that typed disposition in
  the same exact interval/identity/fence/poison predicate, removing the
  per-collision SELECT and Go rescan. The caller's unique collision census and
  the UPDATE affected-row `==1` invariant remain independent cardinality
  guards. Standard-representable name drift still affects zero rows and stays
  withheld. `go test ./internal/hitraceconv -count=1` passed.
- R9-C: implemented. Callstack coverage now splits every `invalid_duration`
  storage class and retains a bounded exact hint only for a NULL-duration,
  timestamp-known, single-emitter, valid-name sync row. Raw recovery compares
  those hints with independently decoded, lifecycle-admitted B/E candidates
  and publishes a closed census for unique full-key closure, ambiguity,
  namespace payload-PID mismatch, name mismatch and absence. This is
  diagnostic-only: it does not supply an end, change a suffix fence, submit a
  span or supersede a candidate. The 4,038 raw local-validation rejects now
  expose at most eight exact physical pair/first-failure witnesses through an
  independent sideband line, with emitted/omitted counters and the existing
  900-line report cap. Capabilities
  `null_duration_raw_closure_census_v1` and
  `raw_marker_local_validation_witness_v1` identify the new evidence. Full
  `internal/hitraceconv` and `cmd` package tests passed.

Final verification after R9-B and R9-C: `go test ./... -count=1` passed on
`main@ad373561a`. The worktree and `origin/main` were synchronized and clean.

## REP-A customer replay audit (2026-07-29)

Inputs:

- `/Users/han/opt/customlogs/repA.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-repA.txt`;
- customer executable SHA-256
  `611f574ba982203f412126f63b2c7b2008303ece36a7cbfe20e358476d6cc67a`.

The executable still reports `build_revision=unknown`, but carries both
`null_duration_raw_closure_census_v1` and
`raw_marker_local_validation_witness_v1`. REP-A is therefore an admissible
R9-C replay. The audited code baseline is `main@e14b391df`.

### REP-A-01 — output closure and viewer counts remain exact

Full postvalidation again proves:

```text
expected rows = parsed rows = callback rows = 1237851
authenticated O2 rows                         =  829327
standard-visible spans                        =   92914
typed-only sync spans                         =    3128
unpublished closed sync spans                 =      78
```

The typed-only roster is unchanged and still closes exactly as:

```text
3128 = 2638 cpu_unknown_start
     +  488 cpu_source_tainted
     +    2 cpu_unknown_end
```

There is no evidence of a regression in the R8 name or async repairs:
79,932 raw sync spans were submitted, including 28,705 CPU-unavailable and
50,043 standard-name-unrepresentable callstack replacements.

### REP-A-02 — R9-D suffix-fence narrowing is not authorized

All 101 rejected callstack duration scalars are SQL NULL. Ninety are
synchronous rows for which the exact start/identity/name hint was retained;
the other eleven are separately accounted official-async shape rejects.

The independent raw-closure census closes:

```text
NULL-duration hints total/retained        = 90 / 90
unique exact valid raw B/E closures       =  0
hints without a valid raw closure         = 90
```

Therefore none of the 90 suffix fences may be narrowed by R9-D. In
particular, a NULL duration must not be coerced to zero, and an unrelated raw
end must not be attached to it. The 78 still-unpublished closed callstack
spans remain fail-closed. A future diagnostic may classify the 90 starts
against structurally closed-but-locally-rejected pairs and trailing open
begins, but that classification is not duration authority.

### REP-A-03 (P0) — official structured marker `pid=0` is a confirmed compatibility gap

The raw local-validation cohort closes:

```text
raw structural B/E pairs                         = 83975
raw valid/submitted pairs                        = 79932
raw pairs withheld by local validation           =  4038
  invalid begin payload PID                      =  3759
  invalid span name                              =   279
```

All eight bounded first-cohort witnesses are the same exact shape:

```text
physical event profile = tracing_mark_write pid/name/start
header common_pid      = 118
begin/end payload PID  = 0
span name              = iofast_alloc
```

This is not corrupt marker data. The official OpenHarmony SmartPerf
implementation was audited at commit
`5c5afb0c479b070148d8a6e336120638a1a03930`:

- `FtraceEventProcessor::TracingMarkWriteOrPrintFormat` converts the
  producer's exact `pid/name/start` fields to `B|pid|name` or `E|pid|`,
  including `pid=0`;
- `PrintEventParser::GetThreadGroupId` returns zero for that payload;
- `SliceFilter::BeginSlice` and `EndSlice` explicitly treat zero as “no TGID
  override” and attach the slice to the physical row-header PID.

Source fingerprints used for this ruling:

```text
ftrace_event_processor.cpp sha256 =
  1f72412c4adc5ea2ae7a75b0b9c4d920e9fc3d1311af8aa84a010dfd5793059f
print_event_parser.cpp sha256 =
  f138f39618ccbf2fc5f01ade2834ce565b72a53261ee918258b326cce4375620
```

Codrax currently admits the structured body but then rejects the closed pair
because raw sync recovery requires `PayloadPID > 0`. This is stricter than the
official parser and loses an exact, closed cohort.

The repair predicate is frozen narrowly:

1. only the already strict OpenHarmony `print|tracing_mark_write`
   `pid/name/start` producer profile qualifies;
2. both B and E must carry source PID zero and the same positive physical
   emitter;
3. both endpoints must independently resolve to the same canonical host
   thread/process and pass the existing lifecycle interval gate;
4. the public standard marker PID is that proven host TGID, while the
   candidate records the source marker PID as absent/zero provenance;
5. every nonzero payload PID remains verbatim namespace data; generic compact
   or textual `B|0` is not admitted by this exception;
6. collision cardinality, laminar authority, CPU/flags/preempt envelopes and
   full postvalidation remain unchanged.

This profile-specific normalization matches the official viewer's ownership
semantics while producing a positive standard marker PID for generic
systrace/Codrax consumers. It does not equate a nonzero namespace PID with a
host PID.

The 279 `invalid_span_name` rows are not covered by this ruling and remain
withheld pending exact name witnesses.

### REP-A-04 — performance regression is not reproduced

On the same customer file:

| component | REP9 | REP-A | delta |
| --- | ---: | ---: | ---: |
| total DB normalization | 46.919s | 42.867s | -4.052s |
| raw sync recovery | 5.296s | 4.405s | -0.891s |
| SQL fidelity `__all_tables__` | 17.564s | 13.950s | -3.614s |
| semantic sorter | 6.250s | 7.526s | +1.276s |
| full tracequery validation | 8.861s | 8.953s | +0.092s |

R9-B removed about 16.8% from the raw-sync phase. Total normalization is again
within roughly 1.7% of REP8, so the prior SQL-fidelity increase was run
variance rather than a proven format hot path. No O2, hash, immutable snapshot
or full-postvalidation weakening is justified.

### REP-A frozen delivery order

| Batch | Priority | Work | Independent push gate |
| --- | --- | --- | --- |
| RA-A | P0 audit | record REP-A equations and the official `pid=0` ruling | official source commit/hash plus exact repair predicate present |
| RA-B | P0 repair | normalize only exact structured zero-PID B/E pairs to proven header identity | positive standard PID; source-zero provenance; compact/text zero and nonzero namespace controls remain withheld/verbatim |
| RA-C | P1 diagnosis | correlate NULL hints with rejected closed pairs and raw open begins | diagnostic only; no duration/fence change; report remains below 900 lines |
| RA-D | P1 diagnosis | publish bounded `invalid_span_name` witnesses by exact reason class | no name repair without a closed official grammar |
| RA-E | P1 diagnosis/repair | finish the 3,128 CPU-disposition correlation from exact raw pair ledgers | no CPU inference |

RA-B is authorized without another customer replay. RA-C and RA-D should be
shipped before the next replay so one run can decide the remaining 78 fenced
rows and 279 name rejects. RA-E remains a separate CPU-authority batch.

Progress:

- RA-A: REP-A equations, official source fingerprints, the zero-PID ruling and
  the frozen narrow repair predicate were pushed in `6cfb03da9`;
- RA-B: implemented and pushed in `7c5d3a9b8`. The raw ledger types
  zero-as-header-identity only when the strict OpenHarmony
  `print|tracing_mark_write` `pid/name/start` producer selected the body.
  Recovery requires B/E agreement, one physical emitter, equal canonical
  thread/process generations and the existing lifecycle gate. It then records
  the source marker PID as absent/zero provenance and renders the proven host
  TGID into the standard B/E payload. Compact/text `B|0`, one-sided zero and
  mismatched endpoint PIDs fail closed. Nonzero namespace PIDs remain
  verbatim. Capability
  `official_raw_marker_zero_pid_header_identity_v1` identifies this repair;
- RA-C/RA-D: implemented and pushed in `82870aeca`. Each exact NULL-duration
  hint is now independently correlated with a valid raw closure, a locally
  rejected closed pair including its first-failure reason, and a trailing raw
  open begin. The result is diagnostic-only and cannot change a duration or
  fence. Raw local-validation witnesses are retained per sorted reason class
  rather than by one global first-eight race, so an `invalid_span_name` cohort
  cannot be hidden behind a larger PID/CPU cohort. Reason and witness caps are
  typed. Capabilities `null_duration_raw_disposition_census_v1` and
  `raw_marker_local_validation_reason_witness_v1` identify the new evidence.

Package verification after RA-B and after RA-C/RA-D:
`go test ./internal/hitraceconv ./cmd -count=1` passed.

The next customer replay should carry all four new REP-A capabilities. Its
acceptance checks are:

```text
raw_pairs_withheld_invalid_begin_payload_pid                  = 0
raw_pairs_official_zero_pid_header_identity_normalized        = 3759
raw local-validation witnesses include invalid_span_name
90 NULL hints split across valid/rejected-closed/open/absent dispositions
expected rows = parsed rows = callback rows
```

`standard-visible spans` should not decrease. Its increase is deliberately not
predeclared as exactly 3,759: an admitted raw pair may be DB-disjoint and add a
span, may replace one CPU-unavailable DB candidate, or may prove an already
standard-visible exact DB duplicate. Collision cardinality decides each case.

## REP-B customer replay audit (2026-07-29)

Inputs:

- `/Users/han/opt/customlogs/repB.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-repB.txt`;
- customer executable SHA-256
  `d24de16a0271381c2a6966bb894ac0dc8373591131bcb6484d4374ea0965e6b0`.

The capability roster contains all four REP-A acceptance capabilities:

```text
official_raw_marker_zero_pid_header_identity_v1
null_duration_raw_disposition_census_v1
raw_marker_local_validation_reason_witness_v1
raw_marker_local_validation_witness_v1
```

`build_revision` remains unavailable, but capability plus executable hash
makes REP-B an admissible acceptance replay. The audited code baseline is
`main@eeb566c6f`.

### REP-B-01 — RA-B passed; the 3,759 pairs were existing standard DB spans

The zero-PID equation closes:

```text
official structured zero-PID pairs normalized       = 3759
raw pairs withheld: invalid begin payload PID       =    0
raw exact-semantic CPU-known DB candidates           = 3764
  previous non-zero baseline                         =    5
  newly admitted zero-PID exact duplicates           = 3759
```

All 3,759 repaired raw pairs resolve to one already locally admitted,
CPU-known, exact-semantic DB candidate. The shared authority therefore keeps
the existing standard DB span and suppresses the duplicate raw pair. This is
why the output remains:

```text
standard-visible spans = 92914
typed-only sync spans  =  3128
unpublished sync spans =    78
```

The unchanged visible count is successful deduplication, not a failed repair
and not another 3,759-span loss. Publishing both representations would corrupt
the B/E stack.

### REP-B-02 (P1) — all surfaced residual name rejects use official trailing-space semantics

After zero-PID repair, local validation has only one reason:

```text
raw pairs withheld by local validation = 279
invalid_span_name                       = 279
```

The per-reason witness lane now exposes four independent examples. Every
example is the exact structured `tracing_mark_write` profile and ends with one
ASCII space, for example:

```text
H:RSFilterDrawable::CreateDrawFunc node[23669564790680]␠
```

The official `PrintEventParser::GetPointNameForBegin` removes ASCII spaces from
the right edge before creating or matching a slice. Codrax instead retains the
raw fixed-array text and then rejects it through the generic callstack
edge-whitespace predicate. This is another producer/viewer compatibility gap,
not an invalid binary record.

The repair predicate is frozen narrowly:

1. only a body selected by the exact OpenHarmony
   `print|tracing_mark_write` `pid/name/start` producer qualifies;
2. remove only one-or-more trailing ASCII U+0020 bytes, matching the official
   viewer; leading space, tabs, other whitespace, controls and invalid UTF-8
   remain rejected;
3. the trimmed name must independently pass the existing complete span-name
   predicate and be nonempty;
4. PID, namespace, header identity, timestamps, CPU, flags, preempt count,
   lifecycle and B/E pairing remain unchanged;
5. the source raw name remains retained in the raw ledger; only the public
   standard viewer body uses the official normalized name;
6. exact DB collision cardinality still decides duplicate/replacement/new
   publication. No count of 279 new visible spans is promised.

The four witnesses prove this cohort exists; code must continue counting any
other invalid-name shape separately rather than assuming all 279 are spaces.

### REP-B-03 — all 90 NULL-duration hints lack any exact raw-start disposition

The valid raw closure count remains zero. Neither the new
`rejected-closed-pair` nor `open-begin` metrics is present, so under the complete
disposition capability the equation is:

```text
NULL hints total                                      = 90
valid exact raw closure                               =  0
exact raw closed pair rejected locally                =  0
exact raw open begin                                  =  0
no exact raw start disposition                        = 90
```

These DB callstack rows are not recoverable from the current raw marker ledger.
They may be an upstream derived/open-slice lane rather than physical
print-marker records. The 78 later callstack spans must remain suffix-fenced:
there is still no exact end or narrower invalid interval. The diagnostic
should emit the final `no exact raw start disposition=90` counter explicitly
instead of requiring subtraction; that is a presentation/accounting gap, not
span authority.

### REP-B-04 — the remaining viewer gap is exactly 3,128 CPU-authority spans

The roster remains:

```text
3128 = 2638 cpu_unknown_start
     +  488 cpu_source_tainted
     +    2 cpu_unknown_end
```

Zero-PID and trailing-space repairs do not provide physical CPU evidence for
this cohort. The next diagnostic should retain bounded final-candidate
witnesses per reason (TID/TGID, canonical owner, start/end and name) after
shared laminar/fence suppression. Pre-final callstack rows are not an adequate
proxy because 3,209 CPU-unavailable source rows reduce to 3,128 final emitted
typed spans.

### REP-B-05 — total runtime rose, but only raw collision work is code-correlated

| component | REP-A | REP-B | delta |
| --- | ---: | ---: | ---: |
| total DB normalization | 42.867s | 47.5s | about +4.6s |
| raw sync recovery | 4.405s | 5.464s | +1.059s |
| SQL fidelity `__all_tables__` | 13.950s | 16.317s | +2.367s |
| semantic sorter | 7.526s | 7.729s | +0.203s |
| full tracequery validation | 8.953s | 8.655s | -0.298s |

The raw-sync increase is consistent with 3,759 newly admitted pairs performing
exact collision checks. The SQL-fidelity increase again has identical
829,327-row closure and is run variance. Across REP-A/B, all 89 per-table row
hashes are equal except official table `meta`; that table deliberately carries
conversion-dependent `runtime`, source/output filename and related metadata.
Exact O2 preservation therefore makes the whole artifact hash vary while
semantic table hashes remain stable. This is not evidence loss.

No O2/hash/postvalidation weakening is authorized. A future performance batch
may combine the exact duplicate collision lookup, but only with byte/semantic
parity fixtures and an affected-row cardinality guard.

### REP-B frozen delivery order

| Batch | Priority | Work | Independent push gate |
| --- | --- | --- | --- |
| RB-A | P0 audit | record REP-B equations, official trailing-space ruling and performance split | all counts above traceable to REP-B |
| RB-B | P1 repair | normalize trailing ASCII space only for the exact official structured producer | compact/text and every other invalid name stay withheld; collision authority unchanged |
| RB-C | P2 diagnosis | emit explicit NULL disposition closure including exact-absent count | equation equals all retained hints; no fence change |
| RB-D | P1 diagnosis | retain final typed-only CPU witnesses per exact reason after shared authority | counts equal 3,128; no CPU inference |
| RB-E | P2 performance | optimize exact duplicate collision lookup only after RB-B/D | output semantics and full postvalidation unchanged |

RB-B/C/D can be implemented before the next customer replay. The next replay
should be requested after all three, so one run validates the 279-name cohort,
the explicit 90-hint equation and the final 3,128 CPU witness roster.

### REP-B implementation closure

The frozen batches were delivered independently to `main`:

- **RB-A** — audit equations and official-source ruling:
  `43cb0fbdf` (`docs: audit REP-B conversion replay`);
- **RB-B** — exact official structured-marker trailing ASCII-space
  normalization:
  `7d16a78bd` (`fix: match official trailing-space marker names`).
  The source raw name remains unchanged in the ledger. Compact/text markers,
  leading spaces, tabs and other invalid names remain withheld;
- **RB-C** — closed NULL-duration raw disposition accounting:
  `26b14038c` (`diag: close null-duration raw dispositions`). Every retained
  hint is now counted exactly once as valid closure, rejected closed pair,
  open begin, no exact raw-start disposition, or conflicting disposition
  kinds. It does not infer an end or alter a fence;
- **RB-D** — bounded witnesses for final emitted typed-only sync spans:
  `d044f4b79` (`diag: witness final typed-only spans`). Witnesses are sampled
  only after shared laminar, fence, poison and supersession decisions. Each
  exact reason retains at most four rows and separately counts omitted rows.
  TID/TGID, marker PID provenance, canonical ITID/IPID, interval, CPU values
  and CPU provenance are explicit. Capability
  `official_viewer_typed_only_final_witness_v1` identifies the lane;
- **RB-E** — combined raw-marker collision census:
  `0f65bbf16` (`perf: combine raw marker collision census`). One prepared
  query now returns global exact-name and interval cardinalities, local
  admission cardinality, and the closed CPU-known/CPU-unavailable split.
  This replaces the sequential exact/interval/local/census query chain while
  preserving the original decision tree. All inequalities are checked
  fail-closed. Capability `raw_marker_combined_collision_census_v1` and metric
  `raw_collision_combined_census_requests` identify the optimized path.

Package gates passed after RB-D and RB-E:

```text
go test ./internal/hitraceconv ./cmd -count=1
```

The next customer replay is now warranted. Acceptance is:

```text
raw_pairs_withheld_invalid_span_name = 0
raw_pairs_official_trailing_space_name_normalized = 279

null_duration_hints_exact_raw_disposition_accounted = 90
null_duration_hints_no_exact_raw_start_disposition = 90
  (or an explicit, summing split if new raw evidence appears)

official viewer typed-only reason totals sum to the final typed-only sync total
each nonzero reason carries 1..4 final witness rows plus an omitted count

raw_collision_combined_census_requests equals valid raw sync candidates examined
expected rows = parsed rows = callback rows
```

The RB-E performance gate is the raw-sync phase on the same file and machine,
not total elapsed time alone. SQL-fidelity runtime varied by more than two
seconds between REP-A and REP-B without any semantic-table hash drift. No
fixed speedup is predeclared.

Two evidence limits deliberately remain open:

1. the 78 suffix-fenced callstack spans cannot be recovered unless a later
   replay exposes an exact valid/rejected/open raw disposition; the current
   REP-B evidence says all 90 retained NULL hints have no exact raw start;
2. the 3,128 typed-only spans cannot become standard viewer B/E rows without
   exact physical CPU placement. RB-D makes their final identities
   inspectable; it does not infer CPU 0 or copy CPU from a nearby row.

## REP-C customer replay audit (2026-07-29)

Inputs:

- `/Users/han/opt/customlogs/repC.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-repC.txt`;
- customer executable SHA-256
  `520295f716a087b43ed410fdab5ef0c3354d1cc43596eedea41799ed98cf5a51`.

The diagnostic contains every RB-B through RB-E capability, including:

```text
official_raw_marker_trailing_space_name_v1
null_duration_raw_disposition_census_v1
official_viewer_typed_only_final_witness_v1
raw_marker_combined_collision_census_v1
```

REP-C is therefore an admissible acceptance replay of
`main@6d0f5a3a6`. It is not an old customer binary.

### REP-C-01 — RB-C passed: the 90 NULL-duration dispositions close exactly

```text
null_duration_fence_hints_total                    = 90
null_duration_fence_hints_retained                 = 90
null_duration_hints_exact_raw_disposition_accounted= 90
null_duration_hints_no_exact_raw_start_disposition = 90
```

There is no valid raw closure, rejected closed pair or open begin for any
retained hint. The 78 suffix-fenced callstack spans therefore remain
unrecoverable from this source generation. No end timestamp or narrower
invalid interval can be inferred.

### REP-C-02 — RB-D passed, but its CPU witness text has a zero-value ambiguity

The final emitted typed-only census still closes:

```text
3128 = 2638 cpu_unknown_start
     +  488 cpu_source_tainted
     +    2 cpu_unknown_end
```

The bounded final witness sidebands are present after shared suppression:

```text
cpu_unknown_start  emitted=4 omitted=2634
cpu_source_tainted emitted=4 omitted=484
cpu_unknown_end    emitted=2 omitted=0
```

They expose exact row ID, TID/TGID, namespace marker PID, canonical ITID/IPID,
interval and name. However, unavailable endpoints currently render
`start_cpu=0/end_cpu=0` next to
`start_cpu_source=callstack_unavailable`. The provenance is honest, but the
numeric zero is only a Go placeholder and can be misread as measured CPU 0.
This is the same zero-value presentation class previously found in per-CPU
busy reporting. The witness must render each unavailable endpoint as
`cpu=unavailable`; a known CPU 0 remains numeric `0`.

### REP-C-03 (P0) — RB-B production predicate excluded the actual `print.buf` cohort

The RB-B capability is present, but acceptance failed unchanged:

```text
raw_pairs_official_trailing_space_name_normalized =   0
raw_pairs_withheld_invalid_span_name              = 279
raw_pairs_withheld_local_validation                = 279
```

All four retained examples are still exact trailing-ASCII-space names. The
reason is now proven by the source-record census:

```text
target_print_records                               = 175165
target_tracing_mark_write_records                  =   7518
target_marker_sync_zero_pid_header_identity_endpoints = 7518
```

Every structured `tracing_mark_write(pid,name,start)` record in this file is
the already repaired zero-PID cohort. The residual nonzero-PID names arrive
through the admitted official `print.buf` string carrier.

The official `CpuDetailParser` passes `print_format().buf()` to the same
`PrintEventParser::ParsePrintEvent` path, and
`GetPointNameForBegin` removes trailing ASCII spaces for every B payload.
Codrax instead stored only `OpenHarmonyStructuredProfile` and required it on
both B/E records before trimming. The ruling was correct but the carrier
predicate was narrower than the official parser.

The corrected predicate is frozen:

1. retain a distinct `OpenHarmonyPrintParserProfile` fact only when the strict
   governed decoder admitted an exact `print` or `tracing_mark_write` body;
2. require that fact on both physical B/E endpoints;
3. trim trailing ASCII U+0020 only from the B name/body, then rerun the complete
   span-name predicate;
4. keep `OpenHarmonyStructuredProfile` separate and narrow; only it may
   authorize zero-PID-as-header-identity;
5. `tracing_mark_write_xacct`, arbitrary text rows, leading space, tabs,
   controls, invalid UTF-8 and empty-after-trim remain withheld;
6. collision, identity, namespace, timestamp, CPU, flags, lifecycle and
   laminar decisions remain unchanged.

The output remained byte-count and row-count identical to REP-B:

```text
rows  = 1,237,851
bytes = 767,905,606
standard-visible spans = 92,914
```

Thus RB-B did not alter production semantics in REP-C. The differing artifact
SHA is expected from the official `meta` table's runtime/path data.

### REP-C-04 — RB-E is active and total runtime improved, but phase timing is hidden

The combined-census equation is exact:

```text
raw pairs structurally closed                  = 83,975
invalid names withheld                         =    279
combined collision census requests             = 83,696

83,696 = 79,932 submitted + 3,764 exact existing DB candidates
```

`raw_pairs_interval_collision_locally_suppressed=1184` is a subset of the
submitted alternative path, not another terminal bucket.

Observed same-machine timing:

| component | REP-B | REP-C | delta |
| --- | ---: | ---: | ---: |
| total DB normalization | about 47.5s | 41.833s | about -5.7s |
| trace_streamer DB export | about 2.5s | 2.011s | about -0.5s |
| semantic sorter | 7.729s | 7.352s | -0.377s |
| full tracequery validation | 8.655s | 6.930s | -1.725s |

The result is materially faster, but it does not isolate RB-E's raw-sync
contribution: the oversized raw-marker coverage JSON is truncated before
`elapsed_us`. Existing witness sidebands preserve selected metadata, not the
coverage receipt. A compact, valid JSON coverage-summary sideband is required
whenever the full coverage line exceeds the 8 KiB line cap. It must include
family/table, rows, elapsed time, skipped state and metric/metadata cardinality
without duplicating arbitrary large values.

### REP-C-05 — source inventory carries a stale decoder contradiction

`source_rawtrace_authority/__source_segments__` still reports:

```text
decode_authority=unavailable_official_page_decoder_not_implemented
recovery_authority=requires_official_page_decoder_or_upstream_retained_rows
```

The same report then proves a complete strict raw-page profile and admits
390,416 target envelopes, including marker, scheduler-lite, blocked-reason,
wakeup-new and DMA-wait decoders. The generic all-event decoder remains
incomplete, but the closed target decoders are real. The inventory wording
must say `partial_closed_target_decoders`; it must not claim that no official
page decoder exists.

### REP-C-06 — 1,787 exact DMA lifecycle records remain typed-only/unpublished

The raw ledger has exact envelopes but no body adapters for:

```text
dma_fence_destroy       493
dma_fence_enable_signal 305
dma_fence_init          494
dma_fence_signaled      495
total                  1787
```

The high-level `dma_fence` table also has 1,787 rows but lacks emitter
identity/CPU and exposes predecessor delta rather than wait duration, so its
withholding is correct. These are not evidence that 1,787 thread spans were
lost. They are nevertheless an official-viewer event/lane completeness gap:
the exact source envelopes and descriptor values should be researched against
the official renderer and published only as their actual point/counter
semantics, never fabricated as B/E waits. The already complete 149
`dma_fence_wait_start/end` pairs are separate and remain correct.

### REP-C delivery order

| Batch | Priority | Work | Independent push gate |
| --- | --- | --- | --- |
| RC-A | P0 audit | record REP-C equations and official print-parser scope | capability/hash plus source-record census present |
| RC-B | P0 repair | extend trailing-space authority to exact admitted official print-parser carriers | structured zero-PID authority unchanged; all negative name profiles pinned |
| RC-C | P1 diagnosis | render unavailable witness CPU as typed unavailable, not numeric zero | measured CPU 0 control remains numeric |
| RC-D | P2 diagnosis | add compact sideband for oversized coverage receipts | valid JSON, bounded line count, elapsed/rows survive |
| RC-E | P2 diagnosis | correct stale partial-decoder inventory wording | no claim of generic all-event completeness |
| RC-F | P2 design | map four DMA lifecycle families to official point/counter semantics | no B/E publication without an official interval contract |

RC-B through RC-E can be implemented without another customer capture. RC-F
requires an official-source semantic ruling and dedicated fixtures before
publication. The next customer replay should occur after RC-B through RC-E so
one run validates both the name repair and the phase timing.

### REP-C implementation closure

All REP-C batches are now implemented and independently pushed:

| Batch | Commit | Closure |
| --- | --- | --- |
| RC-A | `29d702f68` | replay equations, carrier ruling and residual inventory recorded |
| RC-B | `e5d47cda3` | exact admitted `print` and `tracing_mark_write` carriers share the official trailing-ASCII-space display normalization; structured zero-PID authority remains separate |
| RC-C | `b20f0ed2f` | unavailable typed-only witness CPU renders as `unavailable`; exact CPU 0 remains numeric |
| RC-D | `b5e87e66d` | every oversized coverage row gets a bounded valid-JSON receipt retaining rows, elapsed time and error/skipped presence |
| RC-E | `2cabe21dc` | source authority reconciles the strict raw profile/ledger before declaring supported-family decoder availability |
| RC-F | `1f956a047` | four official DMA lifecycle families publish exact point-event wires after closed census and overlap checks |

RC-E also corrects the raw-ledger publication wording. The ledger's own
`RowsEmitted` remains zero, but retained typed records are no longer described
as permanently non-publishable: a dedicated family gate may publish them only
after its own census, deduplication, coordinate and wire checks. This is the
already established architecture for blocked-reason, DMA wait and marker
recovery, not a new generic decoder.

RC-F is grounded in the official SmartPerf source at
`5c5afb0c479b`:

- `rawtrace_parser/ftrace_event_processor.cpp` decodes
  `driver`, `timeline`, `context`, `seqno` for each lifecycle record;
- `rawtrace_parser/cpu_detail_parser.cpp` appends each
  init/destroy/enable-signal/signaled record as one `DmaFenceRow`;
- `filter/slice_filter.cpp::DmaFence` computes the table `dur` as the delta
  from the previous arbitrary event on the same timeline;
- `trace_data/trace_stdtype/ftrace/render_service_stdtype.h` confirms that
  the row itself contains timestamp, event name and the four payload fields,
  but no thread-wait begin/end contract.

Therefore Codrax publishes only these exact standard ftrace point wires:

```text
dma_fence_init: driver=... timeline=... context=... seqno=...
dma_fence_destroy: driver=... timeline=... context=... seqno=...
dma_fence_enable_signal: driver=... timeline=... context=... seqno=...
dma_fence_signaled: driver=... timeline=... context=... seqno=...
```

It does not publish `B/E`, `dur`, wait time or a synthetic interval. Source
`common_pid`, CPU, timestamp, flags and preempt count are copied exactly. If
comm/TGID cannot be proven, the standard header uses `unknown-<common_pid>`
and `(-----)`; a Donghu namespace-shaped PID such as `32788` remains `32788`
and is never rewritten to a guessed host PID or TGID. The point remains
visible to the official parser because its DMA row semantics use the event
name and four payload fields, not a manufactured process identity.

RC-F publication is all-or-nothing under these precise gates:

1. raw profile and target ledger are complete;
2. for each of the four names,
   `physical records == body admitted`, and the sum equals retained rows;
3. no retained-envelope or body capture failure exists;
4. the normalized SQLite raw-ftrace DMA lane emitted no row;
5. every retained timestamp, CPU, common PID, flags, preempt count and payload
   remains wire-representable.

The high-level SQLite `dma_fence` table does not suppress RC-F because it is a
non-equivalent official activity table without emitter identity/CPU and with
predecessor-delta `dur`. Its exact cells remain preserved by SQL text
fidelity.

Package verification after RC-F:

```text
go test ./internal/hitraceconv ./cmd -count=1
ok github.com/hanchaoqun/codrax/internal/hitraceconv
ok github.com/hanchaoqun/codrax/cmd
```

The end-to-end fixture invokes the actual trace-streamer normalization path;
deleting the RC-F wiring loses the expected point and fails the test. Separate
fixtures pin strict descriptor/body retention, exact point wire, namespace PID
non-rewrite, raw-DB overlap withdrawal, atomic invalid-row withdrawal and the
absence of interval wording.

### Next replay acceptance after REP-C

One customer replay on the same binary input is now required to validate the
production artifact, not to discover the implementation:

```text
diagnostic_capabilities contains:
  official_raw_marker_print_parser_trailing_space_v2
  official_viewer_typed_only_final_witness_v2
  coverage_receipt_sideband_v1
  source_rawtrace_partial_decoder_authority_v1
  official_raw_dma_lifecycle_point_recovery_v1

raw_pairs_official_trailing_space_name_normalized = 279
raw_pairs_withheld_invalid_span_name              = 0

typed-only final witnesses use cpu=unavailable where provenance is unavailable
known CPU 0 witnesses remain cpu=0

source_rawtrace_authority.decode_authority
  = available_closed_target_decoders_for_supported_families
source_rawtrace_decode.target_body_unsupported    = 0

493 + 305 + 494 + 495 = 1787 retained lifecycle points
source_rawtrace_dma_lifecycle.rows_read           = 1787
source_rawtrace_dma_lifecycle.rows_emitted        = 1787
publication_state = published_exact_official_point_events

oversized trace_db_coverage entries have a matching
trace_db_coverage_receipt with rows and elapsed_us

expected rows = parsed rows = callback rows
```

The 78 suffix-fenced callstack rows and 3,128 typed-only CPU-tainted spans
remain evidence-limited exactly as recorded above. Their typed accounting is
not a license to invent end timestamps or physical CPU placement. They are
separate from the 1,787 DMA lifecycle point events now restored.

## REP-D replay audit (2026-07-29)

REP-D used a new customer executable, not a stale pre-fix build:

```text
codrax_version = 0.1.20260729
build_time     = 2026-07-29T11:01:40Z
executable_sha = b4ea6a169b8793598b3a2706379b7a9305e833cf192f85113354b101f9a072cc

official_raw_marker_print_parser_trailing_space_v2 = present
official_viewer_typed_only_final_witness_v2        = present
coverage_receipt_sideband_v1                       = present
source_rawtrace_partial_decoder_authority_v1       = present
official_raw_dma_lifecycle_point_recovery_v1       = present
```

The immutable input remained 27,022,926 bytes. Conversion succeeded with
1,237,851 exact parsed/callback rows, zero unknown or unparsed rows, and
artifact SHA-256
`5640569fb773831efb810caaa6fcd3d1ea4a40bab9eedf8a6c261890e30562cf`.
Normalization took 45.703 seconds, including 7.455 seconds in the sorter and
6.973 seconds in full tracequery postvalidation. This replay does not prove a
new performance regression: those two independently timed phases remain in
the established range, while the end-to-end normalization delta is still
dominated by the full SQLite scan/O2 preservation path.

REP-C diagnostic repairs did take effect:

- typed-only witnesses without CPU provenance now say `cpu=unavailable`;
- oversized coverage entries have bounded receipts;
- raw decoder authority says closed target decoders are available;
- the four DMA lifecycle families are recognized and publication fails closed
  when their exact census is incomplete.

Two implementation gaps remain. Both are deterministic from the replay plus
the current code and do not require another customer capture before repair.

### REP-D-01 — admitted official `print` name normalization is lost in raw-body revalidation

The carrier gate now recognizes all 279 official-parser trailing-space
candidates, but none reaches shared sync authority:

```text
raw_pairs_official_trailing_space_name_normalized = 0
raw_pairs_withheld_candidate_validation_failed    = 279
raw_pairs_withheld_local_validation                = 279
```

The witnesses are exact production names such as:

```text
H:RSFilterDrawable::CreateDrawFunc node[23669564790680]<ASCII SPACE>
```

The failure has moved from the pre-RC-B name admission gate to
`validateTraceDBSyncSpanCandidate`, so RC-B widened the correct carrier but
did not preserve the normalized name through the second parser. In
`traceDBRawMarkerSyncCandidate`, `candidate.Name` is trimmed, while
`StartMarkerBody` is changed with `strings.TrimRight(body, " ")`. A production
official `print` body can carry an admitted suffix after the name, so the
space is inside the body rather than at its end. Revalidation decodes the
unchanged name from `StartMarkerBody`, observes that it differs from
`candidate.Name`, and rejects the candidate.

The repair must rewrite only the exact admitted `B|pid|name` field:

1. bind the body prefix to the already admitted action and payload PID;
2. require the decoded original name at that exact location;
3. permit only an empty suffix or the already decoded structured suffix
   beginning with `|`;
4. replace only the admitted trailing ASCII spaces in the name;
5. decode the reconstructed body again and prove action, PID, normalized name
   and all remaining fields;
6. compose this rewrite with zero-PID header normalization without restoring
   the original untrimmed body.

This is display-name normalization only. It must not grant PID, TGID, comm,
namespace, CPU, timestamp or lifecycle authority. A production-shaped suffix
fixture and a combined suffix plus zero-PID fixture are mandatory; the
existing no-suffix fixture is insufficient.

### REP-D-02 — lifecycle `dma_fence_init` incorrectly rejects official empty-driver rows

The raw record census remains exact:

```text
destroy       493 / 493 admitted
enable_signal 305 / 305 admitted
init          339 / 494 admitted; 155 rejected
signaled      495 / 495 admitted
retained      1632 / 1787
```

The publisher then correctly reports:

```text
publication_state = withheld_raw_point_census_incomplete
rows_read          = 1632
rows_emitted       = 0
```

The missing 155 are not absent source records and are not intervals. Codrax
currently sends lifecycle records through the shared strict DMA wait decoder.
That decoder rejects an empty `driver` scalar. Official SmartPerf lifecycle
parsing rejects an empty `timeline`, but interns and retains `driver` even
when its exact C string is empty. Applying the wait-pair hard key to lifecycle
points is therefore an over-strict local rule.

The repair must split the semantic profiles:

- DMA wait start/end keeps the existing non-empty driver and non-empty
  timeline hard keys;
- DMA lifecycle accepts an exact empty NUL-terminated driver field, keeps the
  timeline non-empty, and keeps the descriptor roster, offsets, non-overlap,
  integer fields and wire safety strict;
- publication stays a point event and renders the exact official
  `driver= timeline=... context=... seqno=...` wire;
- an empty timeline, malformed data-loc range, missing NUL, descriptor drift
  or census mismatch still withdraws the complete family atomically.

The success equation is:

```text
493 + 305 + 494 + 495 = 1787 body-admitted
source_rawtrace_dma_lifecycle.rows_read    = 1787
source_rawtrace_dma_lifecycle.rows_emitted = 1787
publication_state                         = published_exact_official_point_events
```

### REP-D delivery order

| Batch | Priority | Work | Independent push gate |
| --- | --- | --- | --- |
| RD-A | P0 audit | freeze REP-D build, equations, causes and invariants | this section committed before code |
| RD-B | P0 span repair | field-local official `print` name rewrite plus composed zero-PID normalization | all 279-shaped fixtures submit; negative carrier/name profiles remain closed |
| RD-C | P0 event repair | lifecycle-only exact empty-driver admission | 1,787-point census publishes; wait empty-driver remains rejected |
| RD-D | P1 closure | capability/replay contract and full repository verification | package plus `go test ./... -count=1` pass |

No customer replay is needed between RD-A, RD-B and RD-C. One replay after
RD-D is required to validate the production census and output artifact on the
same Windows machine.

### REP-D implementation closure

| Batch | Commit | Closure |
| --- | --- | --- |
| RD-A | `fca9fb1c1` | froze the new-build proof, replay equations, two residual causes and fail-closed acceptance contract |
| RD-B | `444850faf` | rewrites only the admitted official B-name field, preserves a structured suffix byte-for-byte, reparses the result, and composes with zero-PID header normalization |
| RD-C | `fef69bf0a` | gives lifecycle points their official empty-driver/nonempty-timeline profile while leaving DMA wait hard keys unchanged |

RD-B adds
`official_raw_marker_print_parser_trailing_space_v3`. Its production-shaped
fixture uses a metadata suffix and the actual customer witness name. A second
fixture combines that suffix, trailing-space repair and structured zero-PID
header identity. Both must reach the one shared sync-span authority. Ordinary
carriers, leading spaces and tabs remain rejected. The structural B/E
whitelist names the one normalization function explicitly, so a second
source-side publisher cannot be added unnoticed.

RD-C adds `official_raw_dma_lifecycle_point_recovery_v2`. It is grounded in
all four official SmartPerf lifecycle handlers, not only the customer-visible
init symptom: each handler rejects an empty timeline but stores `driver`
without a non-empty check. The Codrax lifecycle decoder therefore admits only
the exact one-byte NUL empty-driver carrier, retains a strict non-empty
timeline, and reruns the existing descriptor/range/overlap/integer gates. The
point renderer has the same profile. The wait decoder and wait renderer still
reject an empty driver, which is pinned by a negative test.

Package verification after each repair:

```text
go test ./internal/hitraceconv ./cmd -count=1
ok github.com/hanchaoqun/codrax/internal/hitraceconv
ok github.com/hanchaoqun/codrax/cmd
```

### Next replay acceptance after REP-D

The customer should update once after RD-D and replay the same 27,022,926-byte
input. The diagnostic report must contain:

```text
official_raw_marker_print_parser_trailing_space_v3
official_raw_dma_lifecycle_point_recovery_v2

raw_pairs_official_trailing_space_name_normalized = 279
raw_pairs_withheld_candidate_validation_failed    = 0
raw_pairs_withheld_local_validation                = 0

target_dma_fence_init_records                       = 494
target_dma_fence_init_body_admitted                 = 494
target_dma_fence_init_body_rejected                 = 0
target_dma_fence_init_reject_missing_or_invalid_dma_fence_payload = 0

source_rawtrace_dma_lifecycle.rows_read              = 1787
source_rawtrace_dma_lifecycle.rows_emitted           = 1787
source_rawtrace_dma_lifecycle.publication_state
  = published_exact_official_point_events
```

The output must contain exact lifecycle point rows including the previously
rejected shape:

```text
dma_fence_init: driver= timeline=<nonempty> context=<uint32> seqno=<uint32>
```

It must not contain a manufactured lifecycle B/E pair or duration. DMA wait
rows with empty driver must remain absent. Marker endpoints that publish must
retain their original metadata suffix with only the admitted trailing ASCII
spaces removed from the name. Finally:

```text
expected rows = parsed known rows = callback rows
unknown rows = unparsed rows = 0
```

The exact final artifact row count and SHA are deliberately not predicted:
restored marker candidates still pass through shared laminar arbitration, and
authenticated ordering changes the artifact hash. The typed family equations
above are the stable acceptance authority.

## REP-E customer acceptance replay (2026-07-30)

Inputs:

- `/Users/han/opt/customlogs/repE.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-repE.txt`;
- customer executable SHA-256
  `349cdbf049452631b932cb99aff3ceac535ff384c3fed075c950724171bb4491`.

The executable is a new build:

```text
codrax_version = 0.1.20260730
build_time     = 2026-07-30T00:49:20Z

official_raw_marker_print_parser_trailing_space_v3 = present
official_raw_dma_lifecycle_point_recovery_v2       = present
```

The immutable input remains 27,022,926 bytes with the same
69,326.012181718 through 69,328.343094061 timestamp range. REP-E is therefore
an admissible same-input acceptance replay of RD-B and RD-C, not a stale
customer run.

### REP-E-01 — both REP-D repair equations pass

The official marker-name repair closes exactly:

```text
raw_pairs_official_trailing_space_name_normalized = 279
raw_pairs_withheld_candidate_validation_failed    = 0 (key absent)
raw_pairs_withheld_local_validation                = 0 (key absent)
raw_pairs_submitted                                = 80211
raw sync endpoints emitted                         = 160422
```

The 279 repaired spans preserve the full source suffix and reach the shared
laminar authority. Zero-valued metrics are omitted by the compact metric map;
the absence of both rejection keys plus the absence of
`local_validation_pairs` from the typed skipped receipt is the exact zero
verdict.

The DMA lifecycle equation also closes:

```text
493 destroy + 305 enable_signal + 494 init + 495 signaled = 1787

source_rawtrace_dma_lifecycle.rows_read    = 1787
source_rawtrace_dma_lifecycle.rows_emitted = 1787
publication_state                         = published_exact_official_point_events
duration                                  = not_constructed
```

There is no remaining lifecycle census rejection. Empty-driver init rows are
published as exact official point events, not converted into waits or
intervals.

### REP-E-02 — the output-row delta is fully reconciled

REP-D emitted 1,237,851 rows; REP-E emits 1,239,640, a net increase of 1,789.
That is not 556 missing repair rows. The exact producer/replacement equation
is:

```text
new raw marker endpoints              +558  (279 spans × B/E)
superseded callstack endpoints        -556  (278 spans × B/E)
new DMA lifecycle point events       +1787
                                      -----
net artifact rows                    +1789
```

The span population also conserves exactly:

```text
REP-D: 92914 standard + 3128 typed-only + 78 unpublished = 96120
REP-E: 93193 standard + 2850 typed-only + 77 unpublished = 96120

standard-visible delta   = +279
typed-only delta         = -278
unpublished delta        =   -1
```

Thus all 279 repaired spans became official-viewer-visible: 278 replaced an
already conserved typed representation and one recovered a previously
unpublished fenced span. No span was duplicated or dropped by the new
publication path.

The final artifact remains internally exact:

```text
expected_rows    = 1239640
parsed_known     = 1239640
callback_count   = 1239640
unknown/unparsed = 0
typed O2 rows    = 829327
```

Artifact SHA-256 is
`5cf9c59acc6b429166d284763f506ebf6246604011104cd9c82587ac7d9fe886`.

### REP-E-03 — performance improved without weakening validation

| Phase | REP-D | REP-E | Delta |
| --- | ---: | ---: | ---: |
| trace_streamer DB export | 2.312 s | 2.292 s | stable |
| complete DB normalization | 45.703 s | 37.910 s | -17.1% |
| SQL typed-exact preservation | 17.536 s | 13.259 s | -24.4% |
| raw marker recovery | 4.596 s | 3.627 s | -21.1% |
| shared sync-span authority | 3.244 s | 2.273 s | -29.9% |
| semantic sorter | 7.455 s | 6.097 s | -18.2% |
| full tracequery postvalidation | 6.973 s | 7.017 s | +0.6% |

All 829,327 authenticated O2 rows and full postvalidation remain enabled.
REP-E therefore proves no conversion-performance regression from either
repair. It does not justify weakening validation for a larger headline
speedup.

The SQL aggregate `tables_sha256` changed, but the per-table audit localizes
the change entirely to trace_streamer's nine-row `meta` table. All other 88
source table row hashes, all schemas, all row counts and all typed record
counts are identical. This is runtime metadata variability, not semantic DB
or preservation drift.

### REP-E-04 — residual evidence limits, not new repair regressions

The official-viewer gate remains deliberately degraded:

```text
2850 typed-only =
  2483 cpu_unknown_start
  +365 cpu_source_tainted
    +2 cpu_unknown_end

77 closed sync spans unpublished after exact raw replacement
```

The 2,850 spans retain exact identity and interval in the Codrax typed lane,
but this source generation has no exact physical CPU envelope for them.
Standard ftrace B/E headers require a CPU. Copying a neighbor's CPU or using
CPU 0 would fabricate placement, so no safe code batch can make these rows
generic-viewer-visible from REP-E evidence.

The remaining 77 spans are protected by the existing localized stack fence.
All 90 retained `dur=NULL` hints still report
`no_exact_raw_start_disposition`; no source end or narrower invalid interval
exists. RD-B's new exact raw evidence safely recovered one former residual,
but the remaining 77 cannot be unfenced without guessing stack balance.

The 816 scheduler boundaries with unknown comm all refer to one subject:

```text
itid=398 tid=29352 tgid=68
source_cmdline=absent
```

This is one missing capture-time display name repeated across boundaries, not
816 lost thread-name records. Identity, timestamps and scheduling state remain
published. Neither the DB thread table nor the immutable source cmdline
segment contains the missing name, so a host/namespace/display alias must not
be guessed.

Finally, the raw marker ledger retains 85 open begins and 82 orphan ends.
They are unpaired endpoints, not closed spans. Publishing them as standard
B/E would reintroduce the customer-reported whole-trace phantom-span failure;
continuing to withhold them is the correct fail-closed behavior.

### REP-E disposition

REP-E closes RD-B and RD-C. It exposes no new deterministic conversion-code
gap and does not authorize another repair batch. The remaining work requires
new physical evidence, not a softer gate:

- a future capture containing exact raw endpoints/CPU can promote more
  typed-only spans;
- a source cmdline or another exact thread-name record can recover TID 29352;
- a complete begin/end disposition can safely narrow the remaining stack
  fences.

Another same-file replay is not needed. If the customer still observes a
specific missing viewer span, the next useful evidence is that span's exact
name/TID/time range or a viewer screenshot plus a bounded systrace excerpt,
not another identical diagnostic conversion.

## CVT-ERR large-trace conversion failure (2026-07-30)

Customer evidence:

- `/Users/han/opt/customlogs/cvt_err.txt`;
- immutable input size `151107309` bytes;
- trace_streamer DB export succeeded in `9.7s`;
- DB normalization failed after `2m29.8s`;
- exact primary reason:
  `trace_db_text_fidelity_tail_size_limit`;
- the RMQ `invalid_magic=0xdf49` message is only the built-in provider fallback
  after normalization failed and is not the source failure.

### CVT-P0 — a 4 GiB working-file ceiling rejected a valid requested output

The O2 typed-exact lane used one additional private text tail before copying
the same bytes into the private final output. Its hard ceiling reused
`defaultTraceDBActiveTempBytes = 4 GiB`. This customer input reached that
working-file ceiling after approximately 150 seconds even though:

- the official parser had already produced a valid SQLite DB;
- the final systrace format and postvalidator had no 4 GiB output limit;
- the output path had not been published;
- retrying the same input on the same build could only repeat the failure.

This was a conversion-availability bug, not a corrupt input and not an
appropriate reason to invoke the RMQ decoder. Raising the constant to 8 GiB
would merely move the deterministic failure and double the required working
disk, so it is explicitly rejected.

### CVT-B1 implementation — direct authenticated final-generation suffix

The semantic row set is now completely prepared, spilled, merged and
pre-authenticated first. Codrax then opens its existing private final staging
generation and writes:

1. the fixed header plus sorted semantic rows;
2. the canonical O2 `schema -> row/chunks -> receipt` suffix directly from
   the still-sealed read-only DB.

There is no `text-fidelity-tail-*.systrace`, no fixed 4 GiB O2 working-file
ceiling and no second full O2 copy. The private staging file is not adopted as
a sealed generation until:

- the O2 buffer has flushed;
- typed row/byte accounting closes;
- the SQLite DB and sealed VFS close successfully;
- the output file handle closes successfully.

Any earlier error deletes the private staging directory and cannot create the
public output. The writer now hashes the complete header + semantic + O2 byte
stream while generating it. Full tracequery postvalidation must match that
expected wire digest, after which the existing publication-time full-file
measurement still verifies the public generation. Chunk hashes, logical
record hashes, table sequence, row ordinals, exact SQLite storage cells and
all postvalidation gates remain enabled.

The former-limit regression advances the logical direct suffix beyond 4 GiB
without allocating or writing 4 GiB. Structural pins require DB/VFS close and
writer close to dominate `AdoptRegularChild`, and require the expected wire
digest to dominate publication.

### Remaining performance batches before replay

| Batch | Priority | Scope | Acceptance |
| --- | --- | --- | --- |
| CVT-B2 | P1 | unchanged-v1 allocation reduction: row/cell reuse, append-based wire construction, immutable DB metadata cache, sorter single-encode | byte-identical O2 records and receipts; lower benchmark allocations/time |
| CVT-B3 | P1 | bounded ordered parallel row encoding if B2 timing still shows encoder CPU dominance | deterministic table/ordinal order, bounded bytes/jobs, cancellation and first-error fail closed |
| CVT-B4 | P1 | backward-compatible compact O2 v2 embedded-comment carrier | every SQLite cell and receipt conserved; v1 remains readable; official/generic viewer semantic rows unchanged |

#### CVT-B2 implementation

The unchanged-v1 optimization is implemented:

- each table now allocates one typed row and one cell slice, reusing them
  after the synchronous marshal/hash/write transaction completes;
- the optional rowid cell uses one stable storage slot instead of one escaping
  pointer allocation per source row;
- REAL bit-pattern formatting uses one fixed 16-byte lowercase-hex buffer;
- the O2 physical line is assembled once with append-based canonical decimal,
  base64url and lowercase hex encoders;
- the writer accepts the completed byte slice directly, removing
  `fmt.Sprintf`, payload `EncodeToString`, two digest strings and the final
  string-to-byte copy;
- immutable `tableExists`, `columnNames` and `rowCount` results are cached on
  the sealed trace DB authority. Cache hits still poll Context first and
  column results are cloned so a caller cannot mutate the authority.

The representative v1 physical-record benchmark on darwin/arm64 is pinned at
approximately `355-368 ns/op`, `576 B/op`, `1 alloc/op` and `345-358 MB/s`.
The all-storage fixture and deterministic same-input fixture prove the v1
wire, schema, cell, chunk and receipt bytes remain unchanged.

#### CVT-B3 disposition — do not parallelize the live SQLite cursor

The proposed ordered parallel row encoder is not implemented. B2's physical
record benchmark shows the canonical v1 envelope itself at approximately
`355-368 ns/op`; the serial authority remains `database/sql.Rows`, whose
values are valid only until the next scan. Parallelizing after that boundary
would require copying every TEXT/BLOB cell into a second bounded job queue,
increase peak memory for the 4+ GiB customer case and add a second ordering
protocol before the authenticated writer. The local evidence does not show
that risk is justified.

The next higher-leverage safe operation is to reduce private preservation
wire and postvalidation work while leaving the live SQLite scan, table order
and canonical record construction serial.

#### CVT-B4 implementation — compact authenticated O2 v2 carrier

New conversions no longer write one physical comment line per canonical v1
chunk. They accumulate at most 64 KiB of complete canonical v1 lines,
including each LF delimiter, and write one deterministic raw-DEFLATE v2
carrier:

```text
# codrax_trace_db_block/v2 block=<n> records=<n> raw_bytes=<n> ts_ns=<n> codec=deflate payload=<base64url> payload_sha256=<sha256> raw_sha256=<sha256>
```

This is transport compaction, not a new source-data representation. Inflating
the payload recovers the exact v1 lines; those lines still carry the exact
SQLite schema/cell JSON, chunk SHA-256, logical-record SHA-256, table receipt,
row ordinal and table ID. A block never crosses a table boundary. The output
therefore preserves the existing canonical
`schema -> row/chunks -> receipt` topology byte for byte inside the carrier.

The reader remains backward compatible with physical v1 lines. For v2 it
fails closed unless all of the following hold:

- every decimal and base64url field is canonical and the codec is exactly
  `deflate`;
- declared raw bytes are positive and at most 64 KiB, record count is bounded,
  and decompression produces exactly the declared bytes plus EOF;
- no compressed trailing bytes, CR, missing final LF or nested non-v1 line is
  present;
- compressed-payload SHA-256 and raw-v1 SHA-256 both match;
- every recovered v1 chunk independently passes its existing chunk/hash and
  timestamp checks;
- carrier block IDs are contiguous, v1 and v2 are not mixed, full logical
  record hashes close, and table/row topology remains canonical;
- the physical carrier-row count, total artifact row count and whole generated
  wire SHA-256 all match the pre-publication receipts.

The tracequery index now distinguishes physical
`TraceDBTextCarrierRows` from recovered logical `TraceDBTextRecords`.
Both remain advisory-only and are synchronously discarded before MaxEvents or
causal admission. Unique record hashes and per-block summary strings are no
longer interned into the retained index; only the two fixed carrier event
names can remain in the string interner. This closes a previously hidden
large-trace memory/GC amplification in both v1 and v2 reads.

Official and generic viewers receive the same sorted semantic ftrace rows as
before. The compact data is still a comment-only Codrax preservation suffix;
no scheduler, span, frame, PID/TGID, CPU, comm or timestamp claim is changed,
and a viewer does not need a v2 adapter to display the semantic lanes.

Local acceptance evidence:

```text
deterministic same-input physical O2 rows: 66 -> 17
deterministic same-input artifact bytes:   58005 -> 37140 (-36.0%)
recovered logical O2 records:              66 -> 66
authoritative semantic rows:               18 -> 18
unknown / unparsed rows:                   0 / 0
```

On darwin/arm64, the v2 writer benchmark including periodic compression is
approximately `684 ns/source-record`, `587 B/op`, `1 alloc/op` and
`185 MB/s` of source payload. A 100-record block parser is approximately
`124 us/block`; pooling the bounded inflater reduced the first implementation
from about `186 us` and `203 KiB/op` to `124 us` and `116 KiB/op`. These are
local microbenchmarks, not a claim about the customer's end-to-end speed.
The exact customer ratio remains a required same-input replay result.

Regression coverage includes v1 compatibility, v2 exact recovery and logical
counting, corruption/noncanonical fields, compressed trailing garbage,
decompression beyond the 64 KiB bound, zero-retention indexing, all SQLite
storage classes including a 70,000-byte BLOB, deterministic output SHA, the
former 4 GiB crossing, and full tracequery postvalidation.

#### CVT-B5 implementation — customer-verifiable capability stamp

The bounded diagnostic capability roster now carries both:

```text
sql_text_fidelity_v1
sql_text_fidelity_v2
```

The v1 stamp is retained because query and postvalidation remain backward
compatible with existing physical v1 artifacts. The v2 stamp proves that the
running executable contains the compact writer/reader contract. A customer
report that lacks v2 is from an older build and cannot test CVT-B4.

The code-side batches are committed and pushed:

```text
4eb9e23b2  fix: stream large SQL fidelity suffix directly
fb1e02450  perf: reduce SQL fidelity v1 encoding allocations
e5a8b2618  perf: compact exact SQL fidelity records
52898264c  fix: advertise compact SQL fidelity diagnostics
```

### Local replay disposition

The final tree at `main@52898264c` passes `go test ./...` with zero failed
packages and a release-tagged `make` build. The built executable reports:

```text
version       = 0.1.20260730
revision      = 52898264c235
darwin/arm64 executable sha256
              = 6221294343b2178a24d1722e8bd74cea02b49c5b5d4b6f176aacfbc9dab351ec
```

The deterministic same-input SQLite replay passes twice with:

```text
semantic authority rows = 18
physical O2 carriers     = 17
logical O2 v1 records    = 66
artifact rows            = 35
unknown/unparsed         = 0/0
artifact bytes           = 37140
artifact sha256          = 427d8b8664897dba6641f271fb01ec29a3870c18b9417c26019da8ccb8388752
```

The all-storage SQLite replay also passes exact recovery of every table,
NULL/INTEGER/REAL/TEXT/BLOB storage, embedded NUL/non-UTF8 TEXT, a 70,000-byte
BLOB, schemas, row IDs and receipts, followed by full tracequery
postvalidation.

The repository's `/Users/han/opt/donghu/donghu.ftrace` cannot serve as the
SQL replay input on this host: it is already CRLF ASCII ftrace text, while
`trace convert` admits binary trace input, and the darwin/arm64 build has no
bundled trace_streamer. An explicit built-in attempt therefore correctly
fails its RMQ binary magic gate before conversion. This result neither
validates nor invalidates the customer's Windows trace_streamer path and must
not be reported as a customer-file failure.

### Customer same-file replay is now required

The remaining acceptance evidence can only come from the customer's immutable
151,107,309-byte `.sys` on the machine that has trace_streamer. The new run
must establish:

1. the diagnostic roster contains both fidelity v1 and v2;
2. DB export and SQLite-to-text normalization both complete;
3. `trace_db_text_fidelity_tail_size_limit` is absent;
4. `carrier_rows` is the physical compact count while
   `typed_record_lines` remains the recovered logical count;
5. expected rows, parsed-known rows and callback rows agree, with zero
   unknown/unparsed owned rows;
6. the output is published only after whole-wire and tracequery validation;
7. elapsed time and output bytes are compared with the failed 149.8-second
   pre-v2 normalization attempt.

Retrying with any build whose report lacks `sql_text_fidelity_v2` is not
useful.

## LARG-A large-trace replay and construction batches (2026-07-30)

Inputs:

- `/Users/han/opt/customlogs/largA.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-largA.txt`;
- immutable input bytes `151107309`;
- customer executable SHA-256
  `658fc98bc595d32df93232bb83300604d88c0bd0e21a31357728566295758301`.

The compact SQL fidelity repair is accepted. Normalization completed in
`158.3599699` seconds and published a `1,188,946,453` byte artifact with
`2,310,004` owned rows. Tracequery postvalidation matched expected, parsed and
callback rows with zero unknown and zero unparsed rows. The exact O2 lane
preserved `5,467,934` logical v1 records in `71,087` physical v2 carriers, a
`98.70%` physical-line reduction. The former 4 GiB tail failure is absent.

That success is not official-viewer span completeness. Of `713,774` governed
callstack spans, only `55,377` are standard-viewer rows; `654,147` are Codrax
typed-only and `4,250` remain unpublished. The largest typed-only reason is
`name_unrepresentable=497087`.

### LARG-A root cause

The immutable raw segment contains `2,760,444` closed target records, but the
RPD-CAP1 ordinal ceiling decoded only the first `1,000,000`. Its global
`incomplete_target_decode_cap` state withdrew every raw marker, scheduler-lite,
blocked-reason and DMA recovery consumer. The partial prefix decoded only
approximately 35-40% of each large family. This is the first production
witness anticipated by the RPD-CAP1 fail-closed clause.

Raising the row constant is rejected. The retained raw slices were also
deep-copied at the normalization ownership boundary, so an unbounded row
increase could double hundreds of MiB of fixed record storage.

### Frozen batches

| Batch | Priority | Scope | Acceptance |
| --- | --- | --- | --- |
| LARG-A1 | P0 | replace ordinal decode cap with conservative retained-byte authority; remove duplicate raw-slice copy | all target rows decoded; retained bytes bounded; budget overflow typed and fail closed |
| LARG-A2 | P0 | split retention and recovery authority by independent family | one incomplete family cannot withdraw unrelated complete families |
| LARG-A3 | P0/P1 | rerun raw marker/scheduler/blocked/DMA consumers and correct semantic closure | incomplete raw evidence can never render `complete_no_replacement_candidate`; no CPU/PID/namespace fabrication |
| LARG-A4 | P1 | raw scan/decode/retention timing and safe sorter/O2/postvalidation work | full SQL/O2 and whole-wire validation unchanged |

### LARG-A1 implementation

The fixed one-million target ordinal gate is removed. Every structurally
scanned target now reaches its strict decoder and exact census. Only the typed
records needed by later recovery consumers are retained, under a conservative
`768 MiB` accounting budget which over-counts struct storage and string
payloads. Crossing that budget produces
`incomplete_target_retention_budget`; it never publishes a retained prefix.
Coverage exposes `target_retained_bytes` and the exact family which exceeded
the budget.

The normalization boundary still clones mutable name maps and coverage maps,
but takes ownership of the already immutable raw slices instead of copying
their complete fixed storage a second time. Capability
`official_raw_record_decode_retained_bytes_v3` distinguishes this contract
from RPD-CAP1 builds.

### LARG-A2a semantic-closure correction

The final conversion-quality reducer no longer treats an empty metric set from
a withheld raw-marker consumer as proof that no replacement candidate exists.
Any typed `publication_state=withheld_*` now projects to
`raw_marker_replacement_closure=not_evaluated_withheld_*`. Only a completed
raw-marker consumer may produce `complete_no_replacement_candidate`.

### LARG-A2b independent family authority

The retained-byte budget is partitioned across marker, blocked-reason,
scheduler switch-lite, scheduler wakeup-lite, wakeup-name, DMA-wait and DMA
lifecycle families. The sum remains `768 MiB`; no family can consume another
family's reservation. Exact target decoding and the physical per-format census
always continue to completion.

Coverage carries `retention_<family>_state` plus exact retained-byte metrics.
Marker, blocked, scheduler and DMA consumers now read only their own typed
family authority. A budget or capture-census failure nils that family's
records and leaves every independent complete family usable. Diagnostic
raw/TraceStreamer count reconciliation and the raw target timestamp envelope
read census completeness rather than recovery-storage completeness.

### LARG-A3 recovery activation

No relaxed viewer syntax or inferred envelope is introduced. Once the large
trace completes a family's retained census, the existing audited consumers
run unchanged:

- exact raw B/E endpoints may replace a unique CPU-unavailable or
  standard-name-unrepresentable callstack candidate;
- exact raw S/F endpoints retain namespace payload PID and require both
  physical endpoint emitters;
- scheduler-lite enriches only a unique existing DB edge/boundary;
- blocked-reason and DMA publish only their existing exact raw-only cohorts.

The family-isolation pin proves that a complete marker family remains
authorized when scheduler retention is withdrawn, and the converse. This is
the code-side closure needed for LARG-A's `497087` name-unrepresentable roster;
the exact recovered count remains replay evidence and is never predicted.

### LARG-A4 timing authority

`source_rawtrace_decode.elapsed_us` now measures the complete immutable-source
event-format profile, raw-page structural scan, strict target decode and
retained-family census. Its `elapsed_scope` is emitted verbatim, and capability
`official_raw_record_decode_elapsed_v1` identifies the build. This closes the
zero-duration observability gap before optimizing the newly admitted
1.76-million-record suffix. Sorter, O2 and full postvalidation timings remain
separate and none of their verification gates is weakened.

### Delivery and next replay gate

Delivered batches:

```text
c82459fc5  LARG-A1 complete-target decode plus retained-byte bound
44c49f24a  LARG-A2a withheld semantic-closure correction
add10041c  LARG-A2b independent retained-family authority
719956575  LARG-A3 recovery-activation pin and capability
845d50daa  LARG-A4 raw decode elapsed authority
```

The next same-file replay is accepted only when:

1. capabilities include `official_raw_record_decode_retained_bytes_v3`,
   `official_raw_record_family_authority_v1` and
   `official_raw_record_decode_elapsed_v1`;
2. `target_decode_rows=target_records=2760444`;
3. marker retention is complete and raw-marker publication is not withheld;
4. no `target_retention_budget_exceeded` exists, or any withdrawn family is
   isolated while other complete families still publish;
5. standard-visible, typed-only and unpublished span counts close the governed
   callstack census, with the prior `55377 / 654147 / 4250` retained only as
   the before baseline;
6. raw decode, sorter, O2 and postvalidation elapsed values are reported
   separately, with zero unknown/unparsed final rows.

## LARG-B large-trace replay and follow-up construction (2026-07-30)

Inputs:

- `/Users/han/opt/customlogs/largB.txt`;
- `/Users/han/opt/customlogs/codrax-trace-diag-largB.txt`;
- the same immutable `151107309` byte customer input;
- customer output SHA-256
  `bc92a2bc0836cf1f4983a701065e57c5716b020e339f10f071040b912c2d015a`.

LARG-A1 through LARG-A4 are accepted. The strict raw ledger closed
`target_decode_rows=target_records=2760444`, retained all governed families
under the independent byte budgets, and published a query-ready
`1153505931` byte artifact with `2433390` known rows. Tracequery
postvalidation matched the complete expected/callback census with zero
unknown and zero unparsed rows.

Raw marker recovery submitted `596649` exact standard sync spans, including
`129272` CPU-unavailable replacements and `464740` standard-name-
unrepresentable replacements. The final official-viewer census improved from
LARG-A's `55377 / 654147 / 4250` standard-visible / typed-only / unpublished
counts to `655409 / 56752 / 1613`. This is an approximately `11.84x`
standard-visible increase; it does not close the remaining lanes.

### LARG-B1 — scheduler whole-CPU-lane collateral suppression

The DB scheduler authority read `790734` `sched_slice` rows but emitted only
`219784`. Six CPU lanes were suppressed in full:

```text
cpu 0: 126960 rows / 25 lifecycle_half_open_rejected witnesses
cpu 1: 174518 rows / 10 witnesses
cpu 2: 128929 rows / 16 witnesses
cpu 3: 111499 rows /  4 witnesses
cpu 6:  15552 rows /  1 witness
cpu10:  13486 rows /  1 witness
```

Only `57` lifecycle failures therefore suppressed `570944` DB rows. The
independent exact raw `sched_switch_lite` ledger retained `790734` records;
the old DB-only enrichment contract enriched `219784` and left `570950`
unique raw records unmatched. Weakening the complete-per-CPU DB audit is
rejected because it would let one malformed interval contaminate scheduler
continuity.

Batch `b503d29b5` adds a separate raw-unmatched publication arm without
changing that DB red line. One raw record may publish only when:

1. its closed decoder key and exact timestamp/CPU coordinate are unique;
2. no audited DB scheduler event was emitted at that coordinate;
3. both raw prev and next public TIDs resolve at that exact point to one
   canonical thread/process generation;
4. public TID zero passes the single canonical idle authority;
5. raw state, priorities, flags, preempt count and the complete `next_info`
   receipt remain exact source fields.

An existing DB coordinate always wins, so a raw record cannot duplicate a DB
boundary. Ambiguous raw keys/coordinates, pre-capture timestamps, absent,
rejected or multi-incarnation public TIDs and lifecycle-rejected subjects stay
withheld with typed metrics. No namespace/host PID rewrite or comm inference
is added. Capability
`official_raw_scheduler_lite_unmatched_publication_v1` identifies this
contract.

### LARG-B2 — rejected marker carrier local segment fence

LARG-B exposed one poisoned marker emitter lane:

```text
raw_emitter_lanes_poisoned=1
raw_endpoints_withheld_poisoned_lane=64841
reason=rejected_endpoint_or_carrier
```

The source had only `64` rejected marker carriers in total. The former parser
stopped at the first rejected row and discarded every otherwise independent
pair in that emitter lane.

Batch `5e3f01fad` narrows only classified rejected endpoint/carrier rows to an
ordered local segment fence:

- already closed pairs before the fence remain candidates;
- the rejected physical row is withheld;
- every open begin at the fence is recorded and the LIFO stack is cleared;
- an end after the fence cannot close a begin before it and is treated as an
  orphan;
- a later independent B/E pair starts a new segment and may publish normally;
- invalid physical ordering and an unclassified admitted action still poison
  the complete emitter lane.

Coverage reports exact fence counts, open begins withheld at fences, partial
lane salvage and at most eight
`emitter/physical-ordinal/timestamp/reason` witnesses. These witnesses are
diagnostic only. Capability
`official_raw_marker_local_segment_fence_v1` identifies this contract.

### Delivery and replay gate

Delivered follow-up batches:

```text
b503d29b5  LARG-B1 lifecycle-proven raw-unmatched scheduler publication
5e3f01fad  LARG-B2 rejected marker carrier local segment fences
```

The next same-file replay must contain both new capabilities and establish:

1. `raw_unmatched_published` plus DB scheduler rows increase standard
   `sched_switch` visibility without any duplicate timestamp/CPU boundary;
2. every withheld raw scheduler row closes under a typed prev/next identity,
   lifecycle, ambiguity or coordinate reason;
3. rejected marker carriers produce `raw_marker_local_fences` rather than
   `raw_emitter_lanes_poisoned_rejected_endpoint_or_carrier`;
4. fence witnesses are bounded, cross-fence pairs remain impossible, and
   `raw_emitter_lanes_partially_salvaged` accounts for recovered lanes;
5. standard-visible / typed-only / unpublished spans and the governed total
   close again after shared laminar suppression;
6. expected, parsed and callback rows still agree with zero unknown/unparsed
   rows.

Only after that replay should residual `32347` name-unrepresentable,
`24073` CPU-unknown-start, `322` source-tainted, `8` CPU-unknown-end and
`1613` locally fenced spans be reclassified. Performance work remains a
separate batch: LARG-B normalization was `200.6422781` seconds, with
crossvalidation `41.988821`, sorter `35.216446`, raw-marker recovery
`31.849983` and strict raw decode `12.159032` seconds. No correctness or
whole-wire verification gate may be removed to improve those values.

## Invariants

- Never fabricate CPU, PID, TGID, comm, timestamp or lifecycle evidence.
- Namespace PID must not replace host ownership.
- Display-name recovery must not become identity authority.
- Quality ratios and counts are advisory unless a future hard gate reads a
  precise typed invariant.
- Production emits one selected trace body; diagnostic comparison must not
  merge two independently converted bodies.

## LARG-C replay audit and follow-up batches (2026-07-30)

Customer replay `largC.txt` /
`codrax-trace-diag-largC.txt` validates both LARG-B repairs:

- the scheduler ledger closes exactly as
  `219784 DB + 570854 raw-published + 57 prev-withheld + 39 next-withheld
  = 790734 retained raw records`;
- output rows increased by exactly `570854`, with zero duplicate scheduler
  events reported by the join;
- `64` rejected marker carriers became `64` local fences rather than one
  emitter-lane poison;
- standard viewer spans increased by exactly `32352`, from `655409` to
  `687761`;
- the governed span total still closes as
  `687761 standard + 24400 typed-only + 1613 unpublished = 713774`.

The replay also establishes that scheduler publication and scheduler CPU
placement were still separate consumers. The converter published the
`570854` recovered raw switch boundaries, but callstack/frame CPU lookup
continued to read only `thread_state` Running rows. Eight malformed potential
Running rows tainted ITID lanes and rejected `29448` Running witnesses. The
remaining typed-only span census therefore contained `24067`
unknown-start-CPU, `322` source-tainted and `8` unknown-end-CPU rows, while
`1454` frame rows were withheld for the same placement class.

### LARG-C1 — raw scheduler CPU fallback authority

This batch adds capability
`official_raw_scheduler_cpu_fallback_v1`. The authority is enabled only when:

1. the immutable `sched_switch_lite` decoder family is complete;
2. physical record count, body-admitted count and retained-record count are
   exactly equal;
3. body rejects and record-capture failures are both zero;
4. each endpoint has one unique timestamp/CPU coordinate and passes the
   existing closed scheduler-lite key validation;
5. `current.next_tid == next.prev_tid`;
6. the current public next TID resolves at interval start to exactly one
   canonical thread/process generation, and the complete half-open interval
   passes the shared lifecycle authority.

An invalid or duplicate boundary fences both adjacent intervals. Public TIDs
are never rewritten into guessed host/namespace identities. DB Running
remains primary: raw supplies a CPU only for DB-unknown or DB-source-tainted
points; an exact DB/raw disagreement becomes typed source-tainted
unavailability; a lifecycle-rejected DB lane can never be bypassed.

The first consumer scope is deliberately limited to callstack and frame.
Scheduler publication, wakeup, perf, raw-ftrace, syscall, native-hook and
task-pool behavior is unchanged. Coverage family
`source_rawtrace_scheduler_cpu / __raw_sched_switch_running_intervals__`
reports the complete interval and rejection census.

### Frozen follow-up task order

1. **LARG-C1 / P0:** raw scheduler CPU fallback for callstack/frame
   (implemented in this batch; same-file replay required for recovered-count
   measurement).
2. **LARG-C2 / P1:** select a complete raw `sched_blocked_reason` family as
   the sole physical-event publication authority, suppressing the lossy DB
   timestamp projection and eliminating whole-content-cohort withholding
   (implemented in the following batch).
3. **LARG-C3 / P2:** publish one reconciled scheduler summary containing DB
   boundaries, raw enrichments, raw-unmatched events and typed withheld
   reasons, so the base DB `rows_suppressed` value cannot be mistaken for
   final output loss (implemented in the following batch).
4. **PACKAGING / P2:** keep executable SHA authority and make release
   `build_revision` non-unknown; local development builds may remain
   explicitly identified as such (diagnostic authority consistency fixed in
   the following batch; Makefile release builds already inject revision).

External-evidence holds remain unchanged: `trace_vsync:not_match=632`,
`1613` local callstack fences without raw closure, and two completed async
intervals without a physical finish emitter/CPU must remain fail-closed.

### LARG-C2 — complete raw blocked-reason family authority

LARG-C retained `109770` strict raw `sched_blocked_reason` records. The former
RPD-2A policy emitted `103130` DB-disjoint raw events, retained `6557` DB
projection rows and wholly withheld `6639` raw rows in their overlapping
content cohorts. The overlap therefore contained `82` additional physical raw
events which could not be subtracted safely because TraceStreamer DB rows lack
their original timestamp, CPU, header and delay.

Capability `official_raw_blocked_family_authority_v1` replaces that mixed
publication mode only when all of the following are true:

1. the immutable raw blocked family retention ledger is complete;
2. target physical-record count equals body-admitted count and retained row
   count;
3. body-rejected and key-capture-failed counts are both zero;
4. every DB publication candidate has an exact comparable content key and the
   complete DB key multiset is a subset of raw;
5. each published raw row independently resolves both payload target TID and
   raw `common_pid` header TID at its exact timestamp through the shared
   lifecycle authority.

When selected, raw is the sole physical-event authority: every identity-
admitted raw row is published with its exact timestamp/CPU/flags/header/body,
and all lossy DB projection rows are suppressed before either family writes.
An unresolved namespace-shaped target/header TID remains individually
withheld and is never replaced by a guessed host identity. This is not
count-subtraction and cannot duplicate DB events.

Coverage pins:

- key ledger state `exact_raw_family_authority`;
- `raw_family_identity_admitted` plus typed
  `raw_family_identity_rejected_*`;
- `db_rows_suppressed_by_raw_family`;
- recovery publication state
  `published_complete_raw_family_identity_admitted`;
- wire marker
  `source=official_rawtrace_rpd3 raw_db_family_authority=raw`.

If any completeness or subset proof fails, the converter retains the earlier
RPD-2A behavior; it never partially selects raw-family authority.

### LARG-C3 — scheduler final-publication reconciliation

Capability `scheduler_publication_reconciliation_v1` adds the coverage family
`scheduler_publication_reconciliation /
__sched_switch_publication__`. It is computed after the DB scheduler exporter
and raw scheduler join have both finalized, and before semantic-quality
publication.

The summary distinguishes:

- DB source rows read and suppressed by strict complete-per-CPU auditing;
- DB `sched_switch` boundaries actually published;
- published DB boundaries enriched by an equal raw record (not a second
  event);
- independently published raw-unmatched `sched_switch` events;
- the final standard `sched_switch` total
  (`DB published + raw-unmatched published`);
- typed raw-unmatched withholding reasons and any unclassified residual.

For LARG-C the expected equation is:

```text
raw closure:
  219784 DB-enriched
+ 570854 raw-unmatched-published
+     96 typed-withheld
= 790734 retained raw records

final standard sched_switch:
  219784 DB-published
+ 570854 raw-unmatched-published
= 790638 events
```

The human caveat explicitly states that
`db_source_rows_suppressed=570944` is a DB-lane audit count, not final
systrace loss after exact raw recovery. A production-wiring pin requires this
summary to be appended before semantic-quality construction.

The diagnostic header now uses the same resolved revision authority as the
JSON `build_identity` line and also publishes `build_revision_source`.
Makefile builds continue to use ldflags; plain Go builds may use VCS build
information. If neither is available, both fields consistently report
unknown/unavailable while executable SHA-256 remains the replay identity.

### LARG-C4 — raw-family-aware upstream blocked counter reconciliation

Selecting raw as the sole blocked-event publication authority intentionally
sets the DB exporter `RowsEmitted` value to zero. The pre-existing diagnostic
equations, however, describe TraceStreamer attachment behavior rather than
Codrax final publication:

```text
raw physical records = TraceStreamer DB-attached rows + not_match
TraceStreamer received = raw physical records + DB-attached rows
```

Therefore the reconciliation now reads
`db_rows_suppressed_by_raw_family` as its DB-attached counter when key-ledger
state is `exact_raw_family_authority`. It continues to report final DB
published rows as zero and separately reports raw rows published. This avoids
misclassifying deliberate DB suppression as an upstream parser mismatch and
does not give suppressed DB rows any publication authority.

## LARG-D replay audit and follow-up construction (2026-07-30)

Customer replay `largD.txt` /
`codrax-trace-diag-largD.txt` completed successfully:

- output receipt: `3,007,234` rows and `1,350,041,982` bytes;
- cross-validation expected/parsed/callback counts all equal `3,007,234`,
  with zero unknown, unparsed or header-only rows;
- output SHA-256:
  `b4727d89e2dc9e555ce8b81ef83d5e5b63280d931cf9ec2c5a9b90b39f91a41a`;
- all `790,734` raw `sched_switch_lite` records were admitted; `790,665`
  exact half-open CPU intervals were built, while the remaining adjacent
  candidates close as `15` lifecycle-withheld plus `42`
  start-identity/lifecycle-withheld;
- all `1,454` formerly tainted eligible frame intervals recovered. The final
  frame equation is
  `8,221 source - 1,707 before-capture - 2 invalid-duration - 4,806 erased
  = 1,706 standard frame intervals` (`3,412` S/F endpoints);
- standard-visible spans increased from `687,761` to `688,103`; exactly
  `342` former typed-only spans moved to the standard lane. The former
  `322 source-tainted + 8 unknown-end + 12 unknown-start` residual vanished;
- governed spans still close as
  `688,103 standard + 24,058 typed-only + 1,613 unpublished = 713,774`.

The replay proves that LARG-C1 repaired every eligible frame and the intended
bounded callstack subset. It does not prove that the remaining `24,055`
`cpu_unknown_start` spans can be assigned a CPU. LARG-D predates the consumer
lookup census, so it cannot distinguish an absent canonical ITID lane from a
point before/after the retained raw interval range, a true interval gap, an
overlap conflict or a lifecycle rejection. Extending an interval or guessing a
namespace mapping from this report would fabricate CPU placement.

### LARG-D1 — scheduler reconciliation double count

The LARG-C3 implementation scanned every metric named
`raw_unmatched_withheld_*`. In the production join,
`raw_unmatched_withheld_db_coordinate_present=219784` is an observational
overlap with the already-counted `db_boundaries_enriched=219784`; it is not a
second withheld cohort. The old summary therefore reported the impossible
values:

```text
raw_typed_withheld=219880
raw_records_accounted=1010518
raw_records_retained=790734
```

The physical publication was correct. The exact closure is:

```text
219784 DB-enriched
+ 570854 raw-unmatched-published
+     96 raw_unique_records_unmatched
= 790734 retained raw records
```

Commit `3fdf72d13` makes the producer's exact
`raw_unique_records_unmatched` residual the typed-withheld authority and
accounts key-rejected and ambiguous raw records separately. A production-shape
fixture includes the `219784` DB-coordinate metric explicitly, preventing it
from re-entering the sum. Event admission and wire output are unchanged.

### LARG-D2 — blocked upstream attachment count

LARG-C4's use of `db_rows_suppressed_by_raw_family` as the upstream DB-attached
counter is superseded by LARG-D. The customer has:

```text
raw physical records                         = 109770
TraceStreamer not_match                      = 103212
DB source rows read before Codrax gates      =   6558
DB publication candidates suppressed by raw =   6557
raw rows identity-admitted and published     = 109769
```

The one-row difference is the DB row rejected by Codrax's lifecycle gate and
the independently rejected raw identity row. It remains relevant to the
upstream attachment counter even though it has no final publication
authority. The exact upstream equations close only with exporter `RowsRead`:

```text
109770 = 103212 + 6558
116328 = 109770 + 6558
```

Commit `3fdf72d13` therefore uses audited DB source `RowsRead` for the two
upstream equations while retaining emitted, raw-authority-suppressed and
raw-published values as separate publication metrics. The LARG-D numbers are
literal regression-test inputs.

### LARG-D3 — raw CPU fallback consumer census

Commit `60c7837ac` adds capability
`official_raw_scheduler_cpu_lookup_census_v1`. The shared fallback index keeps
one zero-allocation typed census across its only authorized consumers, with
callstack and frame counted separately. Every lookup is classified by:

- DB status: `known`, `unknown`, `source_tainted` or
  `lifecycle_rejected`;
- raw result: known, known agreement, known conflict, lane absent, before the
  first interval, after the last interval, between intervals, overlapping CPU
  conflict, invalid CPU, or deliberately not consulted;
- a bounded maximum of four exact
  `(canonical itid, public tid, timestamp, known DB/raw CPU)` witnesses per
  classification.

The final coverage is sealed only after callstack and frame exporters finish.
Counts are lookup attempts, not unique spans, and are diagnostic only. The
raw fallback remains unavailable to scheduler publication, wakeup, perf,
raw-ftrace, syscall, native-hook and task-pool consumers.

The next recovery decision is frozen as follows:

| Census result | Allowed follow-up |
|---|---|
| DB unknown/source-tainted + raw known | already recovered; no new authority needed |
| raw lane absent | inspect exact canonical/public TID and namespace evidence; never guess or alias by comm |
| before first / after last interval | disclose capture-boundary absence; never extend a scheduler interval past a physical switch boundary |
| between intervals | inspect discontinuity, duplicate-coordinate and lifecycle fences; repair only if a missing physical boundary becomes independently available |
| raw overlap conflict or DB/raw known conflict | remain typed unavailable and fail closed |
| lifecycle rejected / raw not consulted | never bypass the shared lifecycle authority |

No further standard-span recovery is authorized until a same-file replay
publishes this census.

### LARG-D4 — safe raw CPU construction performance

LARG-D normalized the DB in `239.5808842s`, versus `230.9558782s` in LARG-C
(`+8.625006s`, `+3.73%`). Sorter time increased from `44.388889s` to
`48.936944s`, while whole-output cross-validation improved from
`53.380815s` to `52.538972s`. The former raw CPU coverage had no independent
elapsed value, so the remaining delta could not be attributed safely.

Commit `092e28c96` adds capabilities
`official_raw_scheduler_cpu_build_elapsed_v1` and
`official_raw_scheduler_cpu_ordered_fast_path_v1`:

- raw CPU construction now has its own `elapsed_us`;
- each CPU lane is first audited for nondecreasing timestamps;
- an already ordered lane skips `sort.SliceStable`;
- a lane containing any strict timestamp regression retains the original
  stable sort;
- equal-timestamp source order, duplicate-coordinate fencing, interval
  admission and final wire output are unchanged;
- the lookup census steady-state hot path is pinned to zero allocations.

### Delivered batches and replay gate

```text
3fdf72d13  LARG-D1/D2 exact scheduler and blocked diagnostic closure
60c7837ac  LARG-D3 raw CPU consumer lookup census
092e28c96  LARG-D4 independent timing plus ordered-lane fast path
```

The next customer replay must contain capabilities
`official_raw_scheduler_cpu_lookup_census_v1`,
`official_raw_scheduler_cpu_build_elapsed_v1` and
`official_raw_scheduler_cpu_ordered_fast_path_v1`. Acceptance requires:

1. conversion and cross-validation remain exact;
2. scheduler typed-withheld reports `96`, not `219880`;
3. both blocked upstream equations report exact closure;
4. frame remains `1,706` standard intervals;
5. raw CPU lookup metrics partition every callstack/frame lookup;
6. raw CPU build `elapsed_us` and ordered/reordered lane census are present;
7. only after reading the miss partition may a new span-recovery batch be
   designed.

`build_revision=unknown` remains a release-packaging provenance gap when the
customer executable is produced outside the Makefile/VCS-aware build lane.
Executable SHA-256 remains authoritative for identifying that binary, but the
release pipeline should still inject a revision.

## LARG-E replay audit and capture-head construction (2026-07-30)

Customer artifacts:

- `/Users/han/opt/customlogs/largE.txt`
- `/Users/han/opt/customlogs/codrax-trace-diag-largE.txt`

LARG-E is a successful same-file replay carrying all three LARG-D
capabilities. The output receipt and whole-file cross-validation close exactly:

```text
input bytes                         = 151,107,309
output rows                         = 3,007,234
output bytes                        = 1,350,041,999
expected / parsed / callback rows   = 3,007,234 / 3,007,234 / 3,007,234
unknown / header-only rows          = 0 / 0
normalization elapsed               = 238.5257967s
sorter elapsed                      = 47.991843s
cross-validation elapsed            = 51.829347s
raw scheduler CPU build elapsed     = 0.620885s
already ordered / reordered lanes   = 12 / 0
```

The ordered-lane fast path therefore ran on every CPU lane and reduced sorter
time by `0.945101s` relative to LARG-D. The raw CPU builder itself is only
`0.621s`; it is not the cause of the approximately four-minute normalization.
No additional scheduler-sort shortcut is justified by this replay.

The LARG-D diagnostic corrections also close on production data:

```text
scheduler:
  219,784 DB-enriched
  + 570,854 raw-unmatched-published
  +      96 typed-withheld
  = 790,734 retained raw records

blocked reason:
  109,770 raw = 103,212 not_match + 6,558 DB source rows
  116,328 received = 109,770 raw + 6,558 DB source rows
```

Frame CPU placement has no residual miss: `504` DB/raw agreements plus `2,908`
source-tainted DB points recovered from raw equals all `3,412` frame endpoint
lookups, preserving `1,706` standard frame intervals.

### LARG-E1 — exact lookup partition

The callstack consumer made `1,368,734` CPU point lookups:

| DB/raw disposition | Lookup attempts |
|---|---:|
| DB known and raw agrees | 1,116,625 |
| DB source-tainted, raw known | 196,224 |
| DB unknown, raw known | 450 |
| lifecycle rejected, raw deliberately not consulted | 124 |
| raw miss before first interval | 354 |
| raw miss after last interval | 20,838 |
| raw miss between intervals | 11,569 |
| raw lane absent | 22,550 |
| **total** | **1,368,734** |

Together with the `3,412` frame lookups this equals the reported
`lookup_calls_total=1,372,146`. There are no raw overlap conflicts, DB/raw CPU
conflicts or invalid-CPU classifications.

These are point lookup attempts, not unique final spans. They must not be
subtracted directly from the final `24,055 cpu_unknown_start` span count.
Nevertheless the geometry decides what can be repaired:

- `lane_absent` has no physical CPU interval for that canonical ITID. A CPU
  number cannot be inferred from comm, TGID, namespace PID or another thread.
- `between_intervals` means the point is outside every proved half-open running
  interval. Filling the gap would assert that a non-running thread was running.
- `after_last` is relative to that ITID's last proved interval, not necessarily
  the trace's last switch. The current SQLite authority has no strict
  capture-end singleton, so extending a final interval would be unbounded.
- lifecycle rejection remains a mandatory fail-closed fence.
- `before_first` contains one narrower, previously unused proof: on each CPU,
  the first unique `sched_switch_lite` says that its exact `prev_tid` was
  running immediately before that switch. When and only when the strict
  singleton `trace_range.start_ts` exists, this proves
  `[capture start, first switch)` for that `prev_tid`.

The last rule can recover only a subset of the `354` before-first lookup
attempts. It does not authorize head recovery for any other TID and does not
change the other three miss categories.

### LARG-E2 — capture-head CPU authority

The next batch adds capability
`official_raw_scheduler_cpu_capture_head_v1` and constructs at most one extra
interval per CPU:

```text
[trace_range.start_ts, first unique sched_switch_lite timestamp)
owner = first sched_switch_lite.prev_tid
cpu   = exact raw page CPU
```

Admission remains conjunctive:

1. the immutable `sched_switch_lite` family is complete with zero body reject;
2. `trace_range.start_ts` is an exact non-negative singleton;
3. the first per-CPU boundary has a valid unique timestamp/CPU coordinate;
4. capture start is strictly before that boundary;
5. `prev_tid` resolves at capture start to exactly one canonical host identity
   without PID-namespace rewriting;
6. the complete half-open interval passes the shared thread/process generation
   and global lifecycle authority.

Any failed condition withholds only the head interval and emits a typed metric.
The existing adjacent-switch intervals are unchanged. No capture-tail interval
is constructed because no exact capture-end authority exists.

Regression tests pin exact-start recovery, missing-start rejection, half-open
boundary behavior, ordered and stable-sort parity, lifecycle call-site
inventory, and the diagnostic capability list.

### Remaining fidelity boundary after LARG-E2

The final viewer census remains:

```text
688,103 standard-visible spans
 24,058 Codrax typed-only spans
  1,613 unpublished locally-fenced closed sync spans
```

LARG-E2 may reduce the first-start subset after the next replay, but it cannot
turn the remaining physical-evidence absences into standard ftrace B/E rows.
Assigning CPU `0`, extending a thread across a switch gap, borrowing another
namespace identity, or emitting a synthetic capture tail would make a generic
viewer look fuller by fabricating evidence. Codrax therefore continues to
preserve those spans in its typed lane and discloses that the official/generic
systrace viewer does not display that lane.

The customer binary still reports `build_revision=unknown`; executable SHA-256
identifies the build, but release revision injection remains independently
open.

## LARG-F replay audit and capture-head withdrawal (2026-07-30)

Customer artifacts:

- `/Users/han/opt/customlogs/largF.txt`
- `/Users/han/opt/customlogs/codrax-trace-diag-largF.txt`

LARG-F carries `official_raw_scheduler_cpu_capture_head_v1` and converts the
same immutable input successfully:

```text
expected / parsed / callback rows = 3,007,234 / 3,007,234 / 3,007,234
unknown / header-only rows        = 0 / 0
capture-head intervals emitted    = 12
raw CPU intervals                 = 790,677 (LARG-E: 790,665)
raw CPU builder elapsed           = 0.611069s
```

The final visibility delta is small:

```text
                                 LARG-E    LARG-F    delta
standard-visible spans           688,103   688,104   +1
Codrax typed-only spans            24,058    24,057   -1
unpublished locally fenced spans    1,613     1,613    0
```

However, LARG-F disproves the capture-head authority itself. The raw CPU lookup
census gained a classification which was exactly zero in LARG-E:

```text
lookup_callstack_db_known_raw_miss_overlap_cpu_conflict = 54
witness itid=40/tid=1988:
  ts_ns=69291530138593 db_cpu=1
  ts_ns=69291530149061 db_cpu=1
  ts_ns=69291531067811 db_cpu=1
  ts_ns=69291531086405 db_cpu=1
```

The newly constructed raw head placed the same canonical ITID on another CPU
over timestamps where DB physical running evidence already says CPU 1. This is
not a harmless diagnostic difference: it proves that the premise recorded in
LARG-E2 was insufficient.

### LARG-F1 — root-cause correction

`trace_range.start_ts` is an exact global DB lower bound. It is not an exact
per-CPU raw ftrace retention start. The immutable raw decoder proves that every
record still present in the supplied per-CPU pages was decoded, but it does not
prove that each ring buffer retained every earlier record back to the global DB
start. Per-CPU ring-buffer overwrite can therefore remove one or more earlier
switches before the first retained switch on that CPU.

Consequently:

```text
global trace start + first retained switch.prev_tid
!= proof that prev_tid ran continuously from global start
```

Identity and lifecycle gates cannot repair the missing physical retention
boundary. They can reject a known conflict, but absence of a conflict is not
positive continuity evidence. Dropping only the 54 observed conflicts would
still allow unsupported head intervals for lanes where no independent witness
happened to expose the error.

The `+1` standard-visible span in LARG-F is therefore not accepted as a valid
recovery result.

### LARG-F2 — mandatory withdrawal

The capture-head numeric authority is withdrawn in full:

- no interval is emitted from the global `trace_range.start_ts`;
- each otherwise-shaped first-boundary candidate is counted as
  `capture_head_candidates_global_start_only`;
- each candidate is typed-withheld as
  `capture_head_withheld_per_cpu_retention_start_unavailable`;
- the diagnostic capability becomes
  `official_raw_scheduler_cpu_capture_head_withdrawn_v2`;
- adjacent unique switch-to-switch intervals remain unchanged;
- the existing DB-primary, DB/raw agreement and lifecycle precedence remain
  unchanged.

A production-shape regression fixture carries two CPU lanes where the old
CPU-4 head would overlap the same ITID's exact CPU-5 physical interval. The
fixture requires the head to be absent and the CPU-5 lookup to remain uniquely
known. A second fixture pins that even an exact global trace start cannot mint
a per-CPU head.

Capture-head recovery may be reconsidered only if the immutable source exposes
an exact per-CPU retention-start authority or a separately complete earlier
scheduler segment. The current global DB start, first raw timestamp, comm,
TGID, namespace PID and absence of an observed conflict are all insufficient.

### LARG-F performance disposition

Normalization increased from `238.5257967s` to `246.4085559s`
(`+7.8827592s`). The capture-head builder itself decreased from `0.620885s` to
`0.611069s`, so the new interval construction is not the timing cause.
Same-file component variation accounts for most of the increase:

```text
raw marker sync       35.920479s -> 37.465904s
callstack export      15.093724s -> 15.671432s
sync span authority   21.318666s -> 22.469221s
sorter                47.991843s -> 49.339832s
cross-validation      51.829347s -> 53.674622s
raw decode            15.785656s -> 14.825322s
```

This is consistent with machine/runtime variance across the same row counts
and byte-scale work, not a capture-head regression. The approximately
four-minute conversion remains a real performance limitation, but weakening
sort, authenticated SQL preservation or whole-output validation is still not
authorized.

`build_revision=unknown` also remains open and independent of this correctness
withdrawal.

## LARG-G withdrawal replay closure (2026-07-30)

Customer artifacts:

- `/Users/han/opt/customlogs/largG.txt`
- `/Users/han/opt/customlogs/codrax-trace-diag-largG.txt`

LARG-G carries
`official_raw_scheduler_cpu_capture_head_withdrawn_v2`. It closes every
acceptance condition from LARG-F:

```text
capture-head global-start-only candidates     = 12
capture-head candidates typed-withheld        = 12
capture-head intervals emitted                = 0
raw scheduler CPU intervals                   = 790,665
raw overlap CPU conflicts                     = 0
authorized CPU lookup attempts                = 1,372,146
```

The lookup partition, raw interval count and semantic publication counts are
byte-for-byte numerically equal to the pre-head LARG-E baseline. In particular:

```text
standard-visible spans                        = 688,103
Codrax typed-only spans                       = 24,058
  sync CPU-unknown-start                      = 24,055
  sync name-unrepresentable                   = 1
  completed async interval                    = 2
unpublished locally fenced closed sync spans  = 1,613
```

The unsupported `+1` standard span and all `54` cross-CPU conflicts observed
in LARG-F are gone. Frame placement remains completely resolved:
`504` DB/raw agreements plus `2,908` raw recoveries equals all `3,412`
frame endpoint lookups.

Whole-output closure is also exact:

```text
expected / parsed / callback rows = 3,007,234 / 3,007,234 / 3,007,234
unknown / header-only rows        = 0 / 0
output bytes                      = 1,350,041,991
```

Scheduler and blocked-reason reconciliation retain the previously proven
equations, including `scheduler_raw_typed_withheld=96` and both exact blocked
counter equations.

### LARG-G1 — same-input artifact SHA disposition

LARG-G's artifact size/SHA does not equal LARG-E even though its semantic
counts do. A per-table SQL fidelity comparison resolves the difference:

- all semantic table schemas, row counts and `rows_sha256` values are equal;
- the only differing row hash is official TraceStreamer table `meta`;
- the authenticated SQL tail differs by eight bytes;
- `meta` deliberately contains conversion-dependent runtime, source/output
  filename and related producer metadata, as already ruled in REP-B-05.

Exact SQL preservation therefore causes the whole output SHA to vary across
conversions while the evidence-bearing table hashes remain stable. This is not
event loss, ordering drift or a new deterministic-conversion defect. Removing
or normalizing the official `meta` rows would violate the exact SQL fidelity
contract and is not authorized.

### LARG-G2 — performance

This replay is materially faster than both LARG-E and LARG-F:

```text
normalization       215.6410017s  (E 238.5257967s, F 246.4085559s)
raw decode           14.000824s
callstack export     15.019345s
raw marker sync      35.884744s
sync span authority  19.900681s
sorter               42.259193s
cross-validation     42.193685s
raw CPU builder       0.530620s
```

The unchanged input/row counts and large E/F/G wall-time spread confirm that
the capture-head change was not a performance driver and that machine/runtime
variance remains significant. The approximately `3m35.6s` normalization is
still a product performance limitation; sorter plus mandatory full
cross-validation account for about `84.45s`. This replay does not authorize
weakening either integrity stage.

### Closure and remaining boundary

The LARG-F P0 correctness incident is closed. No further customer replay is
needed for capture-head withdrawal.

The remaining viewer fidelity gap is now stable and typed:

- `24,055` sync spans lack a proved physical start CPU;
- `1` sync name cannot round-trip through the standard trace-mark grammar;
- `2` completed async intervals lack a proved standard S/F emitter/CPU;
- `1,613` locally fenced closed sync spans lack a valid raw replacement;
- `22` thread names remain unresolved and `426` scheduler boundaries therefore
  use unknown display comm;
- release packaging still reports `build_revision=unknown`.

None of these may be repaired by assigning CPU zero, extending across a raw
gap, reusing a comm/TGID/namespace PID, or pairing across a rejected marker
segment. Further fidelity recovery requires new physical source authority,
such as an exact per-CPU retention-start/end ledger, an earlier complete raw
scheduler segment, or independently retained marker endpoints.

## LARG-H repeat stability replay (2026-07-30)

Customer artifacts:

- `/Users/han/opt/customlogs/largH.txt`
- `/Users/han/opt/customlogs/codrax-trace-diag-largH.txt`

LARG-H is a second independent replay of the withdrawn-v2 build family. It
does not expose a new correctness gap:

```text
output rows / parsed / callback      = 3,007,234 / 3,007,234 / 3,007,234
unknown / header-only                = 0 / 0
capture-head candidates / withheld   = 12 / 12
capture-head intervals emitted       = 0
raw CPU intervals                    = 790,665
raw overlap CPU conflicts            = 0
authorized CPU lookup attempts       = 1,372,146
standard / typed-only / unpublished  = 688,103 / 24,058 / 1,613
```

Scheduler, blocked-reason, frame, marker replacement and final viewer
visibility counters are exactly equal to LARG-G. The output contains the same
`3,007,234` rows and all evidence-bearing SQL table schema/row hashes are
equal. A direct G/H comparison again finds only official `meta.rows_sha256`
different; its nine conversion-dependent metadata rows account for the
whole-artifact size/SHA change.

### LARG-H performance variance

Normalization rose from `215.6410017s` in LARG-G to `258.4720479s` in LARG-H:

```text
component             LARG-G        LARG-H        delta
raw decode             14.001s       14.028s       +0.027s
callstack export       15.019s       16.838s       +1.819s
raw marker sync        35.885s       41.224s       +5.340s
SQL fidelity total     28.492s       36.049s       +7.557s
sync span authority    19.901s       23.670s       +3.770s
sorter                 42.259s       52.614s      +10.355s
cross-validation       42.194s       52.523s      +10.329s
raw CPU builder         0.531s        0.624s       +0.094s
```

The same rows and hashes flow through all stages while several independent
CPU/I/O-heavy stages slow down together. No new event family, spill pass,
sort run, validation mode or semantic branch is activated. This supports
machine load/cache/storage variance rather than a localized code regression.

The range now observed for this same 151 MB input is approximately
`215.6s..258.5s` normalization (`~19.9%` relative to the fastest replay).
That variability is operationally relevant even though correctness is stable.
A future performance campaign should benchmark on a controlled machine and
report stage throughput plus host CPU/storage telemetry; weakening exact SQL
preservation, sorter integrity or whole-output cross-validation remains
forbidden.

No code change or further correctness replay is required from LARG-H.

## CVT-I customer long-name / Windows cleanup / WSL staging incident (2026-08-16)

Customer evidence:

- `/Users/han/opt/customlogs/trace_cvt_err.txt`
- input size `211,307,693` bytes;
- Windows working directory `D:\temp\微信启动`;
- exact input basename length `189` characters.

This incident contains three independent layers. Quoting the input is not the
cause: both quoted and unquoted Windows attempts reach the same provider and
fail in about `300ms`.

### CVT-I1 — official child path exceeded the legacy Windows budget

The immutable snapshot correctly preserved the exact customer basename, but
the former private leaf was `codrax-trace-streamer-` plus a 128-bit random
suffix. For this exact customer shape the child input argv was `265`
characters:

```text
D:\temp\微信启动\.codrax\codrax-trace-streamer-<32 hex>\<189-char basename>
```

Codrax's own held-handle APIs use extended Windows paths, but the official
child still receives a normal pathname. A legacy-path implementation in that
child can therefore fail before parsing, which matches the immediate
`exit status 1` and empty child output. This is a code-backed, high-confidence
root-cause inference; the customer log cannot expose the child's internal
file-open error because the external child emitted no diagnostic bytes.

The fix changes only the fixed private namespace to `ts-<32 hex>`. The random
suffix remains the full 128 bits, the exact input basename and extension remain
unchanged, and all held-parent / DACL / generation / no-replace contracts stay
in force. The same customer-shaped argv is now `246` characters. A regression
pins both the customer shape under the legacy boundary and the unchanged
32-hex random suffix. This is not a general promise that every arbitrary
255-character basename under every arbitrarily deep directory can fit a
legacy child; such inputs must still fail honestly rather than rename the
basename behind the official parser.

### CVT-I2 — delete-pending cleanup race masked the provider result

After the child failed, Windows cleanup enumerated the snapshot but its
relative reopen returned `ERROR_DELETE_PENDING` / `STATUS_DELETE_PENDING`:

```text
A non close operation has been requested of a file object with a delete pending.
```

That state means deletion was already authorized and the name was waiting for
the final kernel handle close. Treating it as an identity or security failure
made an otherwise provider-classifiable child failure hard and prevented the
normal auto fallback. Cleanup now recognizes only those two exact typed status
codes, waits in `10ms` steps for at most `2s`, and re-enumerates through the
same held parent. Every other open error remains fail-loud; timeout remains an
error, so a leaked/maliciously-held child cannot be silently abandoned. A
Windows-native contract test holds a delete-pending child briefly and requires
cleanup to converge only after the last handle closes.

### CVT-I3 — WSL DrvFS cannot satisfy the private 0700 contract

The WSL replay failed before the child started because `/mnt/d` surfaced the
new private directory as `drwxrwxrwx` even after chmod. Relaxing the `0700`
gate would expose the immutable trace snapshot and is prohibited.

The trace-convert CLI now supplies `$HOME/.codrax` as a fallback only when both
of these precise conditions hold:

1. the host is typed as WSL by kernel release or WSL environment; and
2. an actual private-directory probe on the primary `<CWD>/.codrax` fails with
   `errPrivateConversionDirSecurityInvalid`.

The fallback is itself probed with the identical ownership/mode contract. A
permission, identity, cleanup or general I/O failure never activates it, and
an insecure fallback also fails. Direct library callers retain their existing
single-anchor behavior. Thus repository-local runtime remains the default,
while a WSL interop mount uses the existing Linux-home `.codrax` namespace
instead of creating an unrelated directory or weakening security.

### Verification and replay status

Completed locally:

- focused `internal/hitraceconv` tests for security-only fallback, exact
  basename, shorter private path and end-to-end transient DB cleanup;
- focused `cmd` WSL signal/fallback test;
- `gofmt` and `git diff --check`.

The local macOS host has no MinGW cross compiler; a `CGO_ENABLED=0` Windows
compile is not a valid substitute because repository tree-sitter dependencies
require CGO. The Windows-only delete-pending test is therefore compile/run
evidence for the release Windows lane, not falsely reported as locally run.

Customer replay is still required after a Windows build containing this batch:
success proves CVT-I1 for the real official binary; if the child still fails,
auto mode must now preserve the provider failure/fallback decision without the
delete-pending masking error. The WSL route should no longer reject `/mnt/d`
mode `0777`; its private staging must be under Linux-home `.codrax`.

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

The fixes deliberately do not promise an exact recovered row count for this
customer capture: the returned report was produced by the pre-fix binary and
the raw `.sys` is unavailable locally. A new bounded diagnostic replay is the
only honest way to measure the post-fix counts.

### Remaining conversion gaps and order

#### F8 — typed `data_type` dependency audit (P2)

`args.datatype` is currently decoded through a closed hard-coded `0=int`,
`1=string` contract. The official registry also carries `2=double` and
`3=boolean`. Codrax must inspect the physical `data_type(typeId,desc)` registry,
prove the closed IDs it consumes, expose it as dependency lineage, and keep
unsupported value kinds local to their exact arg key. It must not mark the
table handled without actually reading and validating it.

#### F9 — `frame_maps` relation preservation (P2)

Official `frame_maps(id,src_row,dst_row)` rows relate source and destination
`frame_slice` rows. Codrax exports the individual frame intervals but drops
this relation. The optimal target is a typed bounded relation in the
tracebundle/tracequery evidence model, with strict row-ID referential checks.
It is not a synthetic duration and must not be rendered as a fake B/E span.

#### F10 — device/parser metadata preservation (P3)

`device_info(physical_width,physical_height,physical_frame_rate)` and
`meta(name,value)` should receive bounded typed metadata coverage. They must
not become ftrace events. `physical_frame_rate` may later qualify frame
deadline interpretation only when its exact source and units are proven;
arbitrary `meta` keys remain display diagnostics, never hard-gate authority.

#### E2 — identical-capture parity fixture (still open)

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

## Invariants

- Never fabricate CPU, PID, TGID, comm, timestamp or lifecycle evidence.
- Namespace PID must not replace host ownership.
- Display-name recovery must not become identity authority.
- Quality ratios and counts are advisory unless a future hard gate reads a
  precise typed invariant.
- Production emits one selected trace body; diagnostic comparison must not
  merge two independently converted bodies.

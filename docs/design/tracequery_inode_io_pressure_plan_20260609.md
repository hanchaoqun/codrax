# trace_query inode IO pressure design (2026-06-09)

## Goal

Add first-class trace_query support for fine-grained file IO signals in a selected
time window. The model should be able to consume deterministic summaries such as:

- hot inode/file read-write activity by bytes, count, latency, and thread;
- page-cache add/delete churn by inode and offset;
- storage-layer latency by block, MMC, SCSI, and BIO families;
- IO pressure/load context that connects file IO, page-cache churn, block latency,
  and scheduler iowait/D-state symptoms.

This must follow the same end-to-end contract as other trace_query fields:
parser fields -> WindowStats JSON -> compact tool rendering -> root-cause/evidence
flow -> ObservationLedger projection -> Explorer/final-answer teaching. Model
input fields must also be covered by the structured JSON compatibility layer.

## Current gaps

- `f2fs_*`, `android_fs_*`, and `scsi_dispatch_cmd_*` are not classified into
  storage/filesystem lanes.
- `mm_filemap_*` is recognized as memory/page_cache, but not aggregated as
  file/page-cache IO.
- File identity fields (`dev`, `ino`, `entry_name`, `offset`, `ofs`, `pos`,
  `rw`, `ret`) are not carried on Event.
- Key/value parsing only supports `key=value`, while common ftrace rows use
  `key value` or `key = value`.
- `block_bio_remap` only preserves the target block identity and loses the source
  filesystem device/sector.
- `window_stats` lacks file-IO by inode, page-cache by inode, storage-layer
  pairing, and IO pressure summaries.
- Tool teaching mentions block IO and resource rows but not inode-level file IO.
- ObservationLedger only projects root cause, root evidence, critical blocking,
  and state_churn trace_query rows.

## Data model

### Event fields

Add typed fields to `tracequery.Event`:

- `fs_dev`, `inode`, `parent_inode`, `entry_name`
- `file_offset`, `file_len`, `file_rw`, `file_ret`, `file_size`
- `block_src_dev`, `block_src_sector`

These fields are trace output fields, not model input parameters.

### WindowStats output fields

Add:

- `file_io_by_inode []FileIOSummary`
- `page_cache_by_inode []PageCacheSummary`
- `storage_latency_by_layer []StorageLatencySummary`
- `io_pressure_summary *IOPressureSummary`

Compact text output lines:

- `- file_io inode=... dev=... name=... op=... count=... bytes=... total_latency=... max_latency=... threads=... lines=... — ...`
- `- page_cache inode=... dev=... adds=... deletes=... churn=... bytes=... offsets=... lines=... — ...`
- `- storage_latency layer=... event=... dev=... op=... count=... paired=... max_latency=... avg_latency=... bytes=... lines=... — ...`
- `- io_pressure signal=... score=... block_max=... file_bytes=... page_cache_churn=... iowait_blocked=... d_state=... top_inode=... — ...`

## Parser changes

- Extend key/value parsing to accept:
  - `key=value`
  - `key = value`
  - selected space-separated keys such as `dev 260:136`, `ino 0x478e5`,
    `ofs=12288`, `offset 0`, `bytes 4096`.
- Recognize:
  - `f2fs_*` as filesystem;
  - `android_fs_*` as filesystem;
  - `scsi_dispatch_cmd_*` as storage;
  - `mm_filemap_*` as memory/page_cache and page-cache IO source.
- Parse file identity and IO fields from Android FS, F2FS, EXT4, and filemap rows.
- Parse `block_bio_remap` source device/sector for cross-layer breadcrumbs.

## Query/statistics changes

- `file_io_by_inode`: aggregate filesystem file IO rows by dev + inode + op,
  preserving entry name, bytes, latency, ret, top threads, first/last lines.
- `page_cache_by_inode`: aggregate `mm_filemap_add_to_page_cache` and
  `mm_filemap_delete_from_page_cache` by dev + inode, preserving adds/deletes,
  offset span, and churn count.
- `storage_latency_by_layer`: pair start/done style rows for block, SCSI, MMC,
  and F2FS direct/sync rows when identity is available; keep unpaired counts as
  caveats rather than hard failures.
- `io_pressure_summary`: compute a soft advisory pressure signal from block max
  latency, file bytes/events, page-cache churn, iowait blocked reasons, and
  D-state totals. This is soft guidance for root-cause ranking, not a hard gate.
- `root_cause_rank`: add candidates for hot inode/file IO, page-cache churn, and
  IO pressure when their effective impact dominates other IO candidates.
- Evidence pack: include the top file IO, page-cache, storage latency, and IO
  pressure facts.

## Model-facing contract

### Tool schema and JSON repair

Input parameters stay minimal. The new model-relevant input alias is only for
event filtering:

- `event_types` examples and repair layer must include `file_io`, `page_cache`,
  `android_fs`, `f2fs`, `scsi`, `mmc`, `storage_latency`, and `io_pressure`.

No output-only field (`file_io_by_inode`, `page_cache_by_inode`,
`storage_latency_by_layer`, `io_pressure_summary`) should be added as a tool-call
parameter. They are consumed from `window_stats`, `root_cause_rank`, `recipe`, or
`evidence_pack`.

Because `trace_query` already routes parameters through
`applyStructuredPayloadCompat`, schema descriptions and tests are enough for
new input aliases that use existing fields.

### Prompt teaching

- Explorer runtime-trace start guidance should say:
  - use `window_stats` for file IO hot inode, page-cache churn, storage layer
    latency, and IO pressure;
  - use `event_search` with event_types `file_io/page_cache/f2fs/android_fs`
    and literal inode/name patterns to inspect raw rows;
  - treat inode summaries as runtime-artifact observations; inode-to-path mapping
    requires trace `entry_name` or external filesystem mapping.
- Final answer guidance should preserve IO-specific facts: inode/dev/op/bytes,
  latency, page-cache add/delete churn, and iowait/D-state caveats.
- `entry_name` is a trace file-name label, not an absolute path. The model must
  not turn `entry_name=foo.db` into `/foo.db`, `/data/...`, or any
  directory-qualified path unless the trace row or an external mapping provides
  that exact path.

### ObservationLedger

Project compact `file_io`, `page_cache`, `storage_latency`, and `io_pressure`
lines into runtime-artifact ObservationRecords so answer synthesis can consume
them like `state_churn`.

## Task list

- [x] Add Event fields and parser helpers.
- [x] Add filesystem/storage classification for F2FS, Android FS, SCSI, and
  filemap aliases.
- [x] Add file IO/page-cache/storage latency/IO pressure summary structs.
- [x] Implement aggregators and root-cause/evidence integration.
- [x] Render compact lines and event_search details.
- [x] Update trace_query schema description and event_types teaching.
- [x] Update Explorer/default skill/final-answer guidance.
- [x] Update ObservationLedger parsing/projection.
- [x] Add parser/query/tool/compat/ledger tests.
- [x] Run targeted tests.

## Verification

- `go test ./internal/tracequery ./internal/tool ./internal/types ./internal/agent ./internal/skill`

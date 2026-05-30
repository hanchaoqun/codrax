# Runtime trace causality query engine plan — 2026-05-30

## Problem

Customer trace/root-cause investigations currently rely on the model composing
`grep`, `awk`, and `read_file` loops over large runtime artifacts. That path is
fragile: regex field order can miss real rows, broad searches waste context, and
multi-dispatch exploration can lose the current wakeup-chain frontier unless the
model has already emitted a structured fact.

Existing infrastructure already has useful carriers:

- attached log / trace plumbing and blob storage;
- `perf_triage` for high-level jank / stall / startup summaries;
- `grep` line-window and large-artifact advisories;
- `ObservationLedger` runtime-artifact origin lanes.

What is missing is a deterministic scheduler/resource query layer that can parse
ftrace-compatible text, compute thread state and same-window resource context,
and return compact line-backed facts to the model.

## V1 scope

- Add a read-only `trace_query` tool, exposed lazily only when the run has an
  attached trace/log runtime artifact or the user names an explicit trace/log
  artifact path.
- Support attached trace blobs and repo/workspace-relative trace paths.
- Parse ftrace/systrace/hitrace-style text. Perfetto proto/JSON is deferred.
- Treat ftrace/systrace/hitrace timestamps as seconds end-to-end on the trace
  clock. `928.081774` means `928s + 0.081774s`; when six fractional digits are
  printed, the fractional part is microsecond precision (`81774us`), not a
  separate millisecond field. Durations derived from intervals may be rendered
  in milliseconds, but input window parameters stay in seconds.
- Accept customer/model timestamp variants at the `trace_query` boundary:
  bare numbers, `s`/`秒` suffixes, and explicit `ms`/`毫秒` or `us`/`微秒`
  units are normalized to seconds. Shortened fractional timestamps receive only
  a tiny bounded tolerance so near-boundary trace rows are not missed.
- Preserve HarmonyOS/hitrace priority semantics as metadata: for user-space
  priorities, larger numeric value means higher priority; `1-40` is CFS,
  `41-139` is RT, and values outside that range remain raw/system-kernel
  priorities.
- Provide deterministic views:
  - `event_search`
  - `thread_timeline`
  - `window_stats`
  - `ipc_graph`
  - `wakeup_chain`
  - `evidence_pack`
- Preserve line-backed source references in runtime-artifact lanes. Do not turn
  runtime trace rows into current-source citations.

## Red lines

- Do not change `repo_map`, source evidence grounding, or finalizer citation
  gates for current-source answers.
- Do not automatically parse traces for ordinary code/config questions.
- Do not make noisy trace statistics into hard gates. They may guide the model
  and final answer, but hard rejects must continue to consume precise typed
  signals only.
- Do not treat trace symbols, thread names, or span labels as current checkout
  source symbols unless separately grounded by source evidence.
- Keep output bounded: compact inline summaries plus blob refs for structured
  payloads, never unbounded trace dumps.

## Task checklist

1. Parser and index
   - Stream ftrace-compatible text with line numbers.
   - Normalize scheduler, CPU, IO (`block_rq_*`, `block_bio_remap`), binder
     send/receive rows, irq, tracing span/counter, memory-ish, and unknown rows.
   - Preserve `sched_blocked_reason` details, including `iowait=0/1` and
     caller text, so D-state evidence can distinguish IO-like waits from other
     uninterruptible blocking candidates.
   - Cache parsed indexes by path, size, mtime, and parser version.
2. Timeline and stats
   - Build per-thread running / runnable / S sleep / D sleep / IO / unknown
     intervals.
   - Compute same-window CPU busy/idle/frequency, IO, binder send/receive,
     irq, blocked-reason, and thread top-N summaries.
   - Treat `cpu_frequency: state=<kHz> cpu_id=<N>` as a CPU frequency state
     transition for CPU `N`: the frequency persists until the next frequency
     transition for that CPU. Same-timestamp events are ordered per CPU and do
     not create artificial duration for unrelated rows at the same trace time.
3. Causality solver
   - Build wakeup DAGs from sleeping/off-CPU intervals and wakeup rows.
   - Build binder IPC edges from `binder_transaction` and
     `binder_transaction_received` rows when transaction ids are visible.
     Treat these as runtime-artifact causal candidates with line-backed
     evidence, not source-code call edges.
   - Let wakeup-chain results carry IPC edges for the selected window so a
     sleeping thread can be explained by synchronous IPC waits when the trace
     shows a transaction handoff.
   - Stop on running, runnable, D/IO, missing rows, cycles, max depth/branch, or
     low-duration branches.
4. Tool integration
   - Register `trace_query`.
   - Expose it only for runtime artifact contexts.
   - Teach explorer/sub-explorer to prefer it for scheduler/time-window
     causality while keeping grep/read_file/exec fallback available.
5. Downstream propagation
   - Mark `trace_query` results as runtime-artifact observations in
     `ObservationLedger`.
   - Include payload refs, row refs, and artifact-local line spans where
     available.
6. Tests
   - Unit coverage for parser, timeline, stats, causality, tool schema/output,
     lazy exposure, and ledger projection.

## IPC causality V1

Binder IPC rows are deterministic runtime observations. V1 should parse
transaction ids and endpoint hints, then build bounded IPC graphs:

- `binder_transaction` sender: current row `comm/pid/tgid`.
- Destination hints: `dest_proc`, `dest_thread`, `reply`, `flags`, and `code`
  when present.
- `binder_transaction_received` receiver: current row `comm/pid/tgid`, matched
  by `transaction`.
- Confidence is high when send and receive rows share a transaction id; lower
  when only `dest_thread` / `dest_proc` hints are present.
- `flags` are carried through as raw metadata. `0x1` is treated as a likely
  oneway/asynchronous hint, but this remains advisory because vendor traces can
  vary.

Non-goals for V1:

- Do not infer Java/native source call graphs from binder rows.
- Do not claim a blocking IPC root cause from a binder send row alone. It must
  be combined with a thread sleep/runnable/D-state interval or explicit trace
  evidence.
- Do not require binder rows for wakeup-chain success; scheduler-only and
  resource-only traces remain valid.

## P0/P1 enhancement batch — 2026-05-30

### Gaps observed

The first V1 trace engine can parse and count scheduler/resource rows, but some
customer root-cause questions still need stronger deterministic joins:

- IO evidence is counted by `block_rq_issue` / `block_rq_complete`, but not
  paired into latency intervals. D-state / IO-wait answers therefore lack the
  exact request duration that made IO suspicious.
- Runnable evidence is aggregated per thread, but not tied back to same-CPU
  pressure, competing higher-priority running work, or the CPU frequency seen
  during the wait.
- Binder IPC edges are available as graph rows, but the wakeup solver does not
  yet combine “thread sent binder transaction, then slept” into an explicit
  `binder_wait` root evidence candidate.
- CPU frequency residency is computed per CPU, but thread running/runnable
  summaries do not yet state the frequency active on the involved CPU/interval.
- Trace `B/E/C` rows are parsed, but span durations and counters are not
  summarized. This weakens Choreographer / RenderFrame / GC / business span
  analysis.
- IRQ rows are counted, but bursts are not clustered by CPU/IRQ name.
- Memory-ish rows are counted, but not split into page-cache, reclaim, fault,
  GC, and generic memory categories.
- Long traces can contain PID reuse or thread-name drift. The current resolver
  uses PID/name matches, but does not surface confidence caveats when one PID
  has multiple names or TGIDs in the selected window.

### Root cause

The missing pieces are not parser availability problems; they are join and
aggregation problems. The engine already has line-backed typed events, selected
time windows, IPC edges, and window stats. The shortfall is that important rows
remain independent counters instead of being correlated into bounded,
explainable evidence objects.

### Design

Keep the same trace-query contract and add bounded summaries:

- Pair block IO by `(device, op, sector, length)` with FIFO matching and report
  top latencies. Unmatched rows remain caveats, not proof.
- Build per-CPU pressure from scheduler intervals: busy/idle, runnable wait on
  that CPU, top runnable threads, top running competitors, and high-priority
  running time. This is advisory context for runnable roots.
- Enrich thread duration rows with `cpu`, `frequency`, and line spans so
  running/runnable/D summaries can explain where the interval occurred.
- Add `binder_wait` candidates only when a synchronous-looking binder edge is
  close to a sleep interval for the sender. A binder send row alone must never
  become a hard root cause.
- Build trace spans with per-PID stacks for `B/E`, plus counter snapshots for
  `C`, then report bounded top durations/counters.
- Cluster IRQ/softirq rows by CPU/name into bursts with first/last lines.
- Classify memory rows into stable coarse categories while preserving raw text.
- Detect PID/TGID/name drift inside the selected window and emit caveats.

All additions stay inside `internal/tracequery` and the `trace_query` tool
summary. Normal code/config analysis, `repo_map`, current-source evidence
grounding, and answer gates are unchanged.

### Task checklist

- [x] P0: parse block identity fields and compute IO latency pairs.
- [x] P0: compute per-CPU runnable/running pressure and interval frequency
      correlation.
- [x] P0: derive safe binder-wait candidates in wakeup-chain results.
- [x] P0: update summary/evidence output and focused tests.
- [x] P1: build trace span/counter summaries.
- [x] P1: cluster IRQ bursts.
- [x] P1: classify memory rows.
- [x] P1: detect PID/TGID/name drift and surface confidence caveats.
- [x] P1: update summary/evidence output and focused tests.

## Follow-up: Runtime Trace Query Recovery Hardening — 2026-05-30

### Gaps observed

Customer `trade_q.log` and `trace_repl.log` exposed recovery gaps after the
first `trace_query` rollout:

- `trace_query(event_search)` did not match common ftrace/hitrace customer
  thread labels such as `com.tencent.mm-36379`. The parser normalizes that row
  to `Comm=com.tencent.mm` and `PID=36379`, while the query path matched the
  free-form `thread` string only against parsed comm fields. The same issue can
  appear as `com.tencent.mm 36379`, `com.tencent.mm [36379]`,
  `com.tencent.mm (36379)`, `pid=36379`, `[GT]ColdPool#5-36624`, or
  `binder:486_1-10803`.
- Empty `trace_query` results proved the index parsed successfully, but did not
  tell the model `matched_events=0`, did not show the normalized thread/pid, and
  did not give a concrete next view such as `thread_timeline` or
  `wakeup_chain` with `pid=36379`.
- Grep line-window parsing treated numeric `-` segments inside blob/path names
  as the line-number separator, producing invalid `read_file path=...` hints.
- `fixed_string=true` with regex-looking patterns such as `.*` or `\d` returns
  true zero matches, but the tool did not explain that fixed-string mode treats
  those metacharacters literally.
- Runtime trace `read_file` follow-up hints still used source-evidence wording
  in pure runtime/log/trace read batches. That is noisy for artifact-only trace
  analysis, but the normal wording must remain for code reads and mixed
  trace+code comparison tasks.

### Root cause

These are model-recovery contract gaps rather than parser coverage gaps:

- Thread identity is structured in the trace rows, but model/customer inputs are
  naturally written as ftrace task labels or pid-bearing prose.
- Empty result summaries did not expose enough typed diagnostics for the model
  to repair the next tool call.
- Grep's line-window parser was too eager: it accepted the first
  `separator + digits + separator` pattern in the whole output line instead of
  rejecting path-continuation fragments.
- Runtime artifact evidence and current-source evidence share the same explorer
  read-without-emit nudge unless the read is recognized as a broad header page.

### Design

- Normalize thread selectors inside `internal/tracequery`, not in prompts.
  Extract pid candidates from safe forms (`pid=36379`, `36379`,
  `name-36379`, `name 36379`, `name [36379]`, `name (36379)`) and preserve the
  remaining name as an advisory comm fragment. If a pid is available, matching
  uses pid as the precise selector and does not require the comm string to match
  every scheduler role row.
- Add empty-result diagnostics to `trace_query` summaries. They must be
  advisory only: report `matched_events=0`, the normalized selector, and concrete
  next-call shapes. They must not silently rewrite or rerun the query.
- Harden grep line-window extraction so path fragments containing numeric
  hyphen groups are not mistaken for line numbers. The fix must stay generic for
  code, config, log, trace, blob, and Windows-style paths.
- Add a no-match advisory when `fixed_string=true` is combined with common regex
  syntax. The tool result remains a true no-match; the system only teaches the
  recovery path.
- Split runtime-only read hints from mixed/code read hints. If the current read
  backlog contains only runtime/log/trace artifact reads, steer the model to
  `trace_query`, targeted grep, line-window `read_file`, and
  `emit_investigation_complete.reason` / `aggregate_facts`. If any current-code
  file was also read, keep the existing source-evidence materialization wording.

### Task checklist

- [x] T22. Add flexible trace thread selector parsing and pid-first matching.
- [x] T23. Add `trace_query` empty-result diagnostics and concrete recovery
      examples.
- [x] T24. Harden grep line-window parsing for hyphenated/numeric path names.
- [x] T25. Add `fixed_string=true` + regex-looking-pattern no-match advisory.
- [x] T26. Add runtime-only read-without-emit hint while preserving normal
      behavior for code and mixed trace+code reads.
- [x] T27. Add focused tests for T22-T26, then run focused and full Go tests.

2026-05-30 recovery-hardening delivery:

- `trace_query` now accepts pid-bearing thread selectors such as
  `com.tencent.mm-36379`, `com.tencent.mm 36379`, `com.tencent.mm [36379]`,
  `com.tencent.mm (36379)`, `pid=36379`, and bare `36379`. The normalized pid
  is used for matching scheduler role fields, while the remaining name stays as
  advisory identity context.
- `event_search` summaries now print `matched_events=N`. Empty event searches
  include advisory recovery hints, including normalized thread selector details
  and concrete `thread_timeline` / `wakeup_chain` next-call shapes when a pid is
  available.
- Grep line-window parsing now rejects path-continuation fragments such as
  numeric hyphen groups inside blob directory names before choosing a
  `path:line:` location.
- Grep no-match output now warns when `fixed_string=true` is combined with
  common regex syntax (`.*`, `\d`, `\s`, escaped dots, and similar), without
  rewriting or rerunning the search.
- Explorer read-without-emit hints now have a runtime-only branch for pure
  log/trace artifact reads. Mixed trace+code reads keep the existing
  current-source evidence materialization wording, preserving code-analysis and
  trace/source comparison behavior.
- Focused validation passed: `go test ./internal/tracequery ./internal/tool
  ./internal/agent`.

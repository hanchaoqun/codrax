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

## Follow-up: Explicit Trace Path Routing — 2026-05-30

### Gap observed

In a pure REPL trace question that named
`record_trace_20260526174055@2907-917050782_cut_2939727458580.systrace`,
thread `com.tencent.mm-36379`, and a seconds-based sleep window
`2942.124416..2942.260210`, the analyzer still spent several rounds on
`repo_map`, `list_files`, and `grep` before classification. The explorer then
used broad grep for four rounds and only called `trace_query(wakeup_chain)` in
round 5.

### Root cause

The attached-artifact shortcut covered structured `--log` / `--htrace` /
`--atrace` inputs with external triage, but an explicit workspace trace path was
still treated like an ordinary repository file. The explorer skill mentioned
`trace_query`, but the generic source-code breadth-scan instruction appeared
before the trace-specific workflow, so models tended to start with repo_map or
broad grep.

### Design

- Add a conservative prompt-only detector for explicit runtime trace artifact
  paths. It fires only for trace-like extensions and stands down when the
  current request explicitly asks for current-source / current-checkout /
  repository-code verification.
- For analyzer prompts, classify explicit trace-only requests immediately and
  avoid repo pre-scan. This is advisory text, not a tool gate.
- For explorer prompts, start pure explicit trace-path investigations in a
  trace-query-first depth mode. `trace_query` remains lazy-exposed by the
  existing tool availability check; grep/read_file/exec remain fallbacks.
- Keep mixed trace+source questions separate: `trace_query` owns runtime facts,
  while normal source tools own current-code proof.

### Task checklist

- [x] T28. Add explicit trace-path-only request detector with conservative
      current-source cue suppression.
- [x] T29. Add analyzer shortcut for explicit trace-only paths.
- [x] T30. Add explorer trace-query-first start prompt for explicit trace-only
      paths.
- [x] T31. Move trace-query-first skill teaching ahead of generic source-code
      breadth scan.
- [x] T32. Add focused tests proving pure trace is accelerated while mixed
      trace+source still uses the normal source lane.

## Follow-up: Trace Flavor and Priority Semantics — 2026-05-31

### Gap observed

`trace_query` can parse ftrace-compatible HarmonyOS HiTrace, Android atrace,
and systrace text rows, but the current deterministic engine does not carry a
platform/flavor dimension. Both supported entry paths lose or underuse flavor:

- Attachment path: `--atrace` and `/atrace` are aliases for the same
  `AttachedHitrace` payload used by `--htrace` / `/htrace`.
- Explicit path path: `.systrace`, `.htrace`, `.atrace`, and `.trace` names
  only control lazy tool exposure; they do not choose parser semantics.

The scheduler row structure is mostly shared, so wakeup-chain and timeline
parsing still work. Priority interpretation is not shared: HarmonyOS customer
traces use the documented user-space rule "larger numeric priority is higher;
1-40=CFS, 41-139=RT", while Android/Linux ftrace priority values must not be
silently interpreted through that HarmonyOS mapping.

### Root cause

`trace_query` stores only a runtime artifact source (`path` or
`attached_trace`) and the parsed row index. `Index`, `Query`, and `Result` have
no trace flavor field. The parser assigns OHOS priority classes directly during
row parsing, and the query layer always renders HarmonyOS priority semantics.
CPU-pressure statistics then use those OHOS classes to decide whether a running
thread counts as high-priority pressure.

### Design

- Add a first-class trace flavor dimension:
  `auto`, `harmony_hitrace`, `android_atrace`, and `generic_ftrace`.
- Preserve attachment spelling as a soft hint:
  `--htrace` / `/htrace` -> Harmony hint, `--atrace` / `/atrace` -> Android
  hint. The raw text still uses the shared attached-trace channel.
- Detect explicit path flavor from low-cost, bounded signals:
  extension/header hints plus content markers such as OHOS/FFRT/Harmony render
  rows or Android/atrace/system-server rows. `.systrace` remains ambiguous
  unless content is strong.
- Resolve effective flavor conservatively:
  explicit tool `trace_flavor` wins; source spelling hints are advisory; low
  confidence or conflicting weak signals fall back to `generic_ftrace`.
- Move priority mapping out of parse-time mutation and into query-time
  flavor-aware helpers. HarmonyOS gets the documented CFS/RT classes; Android
  and generic ftrace expose raw scheduler priority with a caveat instead of
  applying Harmony ranges.
- Surface `trace_flavor`, confidence, priority semantics, and detection signals
  in tool output and JSON payload so models and final answers can explain the
  assumption.

### Red lines

- Do not change `repo_map`, source-code evidence gates, or source citation
  rules.
- Do not auto-parse traces for code-only questions; keep the existing lazy
  runtime-artifact tool exposure.
- Do not make noisy flavor detection a hard answer gate. Low confidence must
  degrade to generic semantics and a caveat.
- Do not reinterpret trace thread names as current-source symbols.

### Task checklist

- [x] T33. Add trace flavor schema, detection, confidence, and priority
      semantics helpers.
- [x] T34. Preserve CLI/REPL attached trace spelling as a soft flavor hint and
      propagate it to tool execution.
- [x] T35. Apply flavor-aware priority classes in `event_search` and
      `window_stats`, including CPU-pressure high-priority accounting.
- [x] T36. Update `trace_query` schema, summaries, skill teaching, and runtime
      observation payload metadata.
- [x] T37. Add focused tests for Harmony detection, Android/generic fallback,
      explicit override, attachment hints, and code-only non-exposure.

2026-05-31 trace-flavor delivery:

- `trace_query` now reports `trace_flavor`, `trace_flavor_confidence`,
  `trace_flavor_signals`, and flavor-specific `priority_semantics` in both the
  JSON payload and the compact tool summary.
- The deterministic parser keeps scheduler priorities raw at parse time and
  applies priority classes at query time according to the resolved flavor.
  HarmonyOS/hitrace gets the customer-documented CFS/RT mapping; Android and
  generic ftrace keep raw scheduler priority and do not contribute
  Harmony-style high-priority CPU pressure.
- CLI and REPL attachment spelling is preserved as an advisory hint:
  `--atrace` / `/atrace` -> Android, `--htrace` / `/htrace` -> Harmony. Stronger
  content signals can still override weak source hints; explicit
  `trace_flavor` / `platform` tool parameters win for the current call.
- Explicit path analysis uses bounded extension/header/content signals. Plain
  `.systrace` with weak or mixed signals degrades to `generic_ftrace` rather
  than silently claiming Harmony priority semantics.
- Focused validation passed:
  `go test ./internal/tracequery ./internal/tool ./internal/agent ./internal/repl ./cmd`.

2026-05-31 trace-flavor audit follow-up:

- Explicit `trace_query.trace_flavor` / `platform` remains authoritative for
  the current call. When it conflicts with strong content detection, the result
  now keeps the explicit setting but emits a caveat naming both the explicit
  flavor and the content-detected flavor so users can audit the assumption.
- REPL tool-call rendering now surfaces explicit `trace_flavor` / `platform`
  parameters in the one-line `trace_query` call detail. Auto-detected flavor is
  still reported in the tool result summary and JSON payload after parsing.
- `trace_query` continues to enter through the shared structured-parameter
  compatibility path; focused coverage now includes string-wrapped JSON,
  string numeric fields, and the `platform` alias.

2026-05-31 root-cause ranking gap and design:

Customer trace questions now regularly ask for more than a raw wakeup chain:
they name a trace span as the time-window anchor, ask for primary/secondary
causes, and ask for cross-thread interaction counts. The current engine can
already compute line-backed timelines, recursive wakeup chains, IPC edges, CPU
frequency residency, CPU pressure, IO latency, IRQ, memory, and trace spans, but
three gaps remain:

- Span anchors are only listed under `window_stats.trace_spans`; there is no
  first-class `span_window` view and no way for `wakeup_chain` /
  `root_cause_rank` to derive `time_start/time_end` from a unique span name.
- Root evidence is a flat list. It is line-backed, but it is not
  deterministically ranked into primary/secondary/tertiary causes using impact
  duration, confidence, and evidence type.
- Cross-thread/cross-process interaction counts require manual event searches
  or IPC graph reads. There is no single `interaction_stats` view that reports
  bidirectional wakeup and binder interaction Top-N for a target thread.

Design constraints:

- Keep all new capabilities inside `trace_query`; do not affect repo_map,
  current-source evidence gates, or code-only questions.
- `span_window` must be line-backed and conservative. If a span name matches
  multiple windows, return candidates and a caveat; do not silently choose one
  unless it is unique inside the selected filters.
- `root_cause_rank` must be advisory evidence, not a final-answer gate. It can
  rank candidates by deterministic impact score, but final prose still belongs
  to the model and must cite the emitted trace facts.
- `interaction_stats` must be format-generic: scheduler wakeup edges and binder
  IPC edges are normalized trace events, not Harmony-specific logic.
- If the user explicitly states a platform (`harmony`, `harmonyos`, `鸿蒙`,
  `东湖`, `ohos`, `android`, `安卓`, etc.) and the model passes it through
  `trace_flavor` / `platform`, that explicit intent wins for the current call.
  Content detection is retained only as audit signal/caveat.

Task checklist:

- [x] T38. Add `span_name`, `interaction_direction`, and new result schemas for
      `span_window`, `root_cause_rank`, and `interaction_stats`.
- [x] T39. Implement span search/window resolution, including unique-span
      auto-window derivation for downstream views.
- [x] T40. Implement deterministic root-cause ranking from wakeup-chain root
      evidence plus window stats, sorted by impact score and tiered as
      primary/secondary/tertiary.
- [x] T41. Implement interaction statistics for bidirectional/incoming/outgoing
      sched_wakeup and binder IPC interactions.
- [x] T42. Extend explicit platform aliases for Chinese/customer spellings and
      update tool descriptions/skills/REPL summaries.
- [x] T43. Add focused tests for span windows, root-cause ranking,
      interaction stats, explicit platform aliases, and code-scenario
      isolation.

2026-05-31 root-cause ranking delivery:

- `trace_query(view="span_window", span_name=...)` now returns line-backed B/E
  trace span windows. When a downstream trace view receives a unique
  `span_name` and no explicit time window, the span becomes the selected
  `time_start/time_end`; multiple matches are returned as candidates with a
  caveat instead of silently choosing.
- `trace_query(view="root_cause_rank")` now produces deterministic
  primary/secondary/tertiary root-cause candidates from wakeup-chain terminal
  evidence and same-window stats such as CPU pressure, IO latency, D-state,
  runnable wait, trace spans, and IRQ bursts. Ranking is advisory and uses
  impact duration, confidence, and evidence-type weights.
- `trace_query(view="interaction_stats")` now reports Top-N peers interacting
  with a target thread through `sched_wakeup` / `sched_waking` and binder IPC,
  with `interaction_direction=both|incoming|outgoing`.
- Explicit platform aliases now cover customer spellings including `鸿蒙`,
  `东湖`, `OHOS`, `Open Harmony`, `Android`, and `安卓`; these aliases normalize
  into `harmony_hitrace` or `android_atrace` when passed through
  `trace_flavor` / `platform`.

2026-05-31 parameter teaching/compat audit:

- `interaction_direction` is now surfaced in the REPL tool-call detail and
  the `trace_query` result banner, matching the existing visibility for
  `span_name`, platform/flavor, thread, pid, and time/line windows.
- Focused compatibility coverage now verifies model-emitted camelCase
  aliases such as `spanName`, `interactionDirection`, `timeStart`, and
  `timeEnd` are repaired through the shared structured tool payload
  compatibility layer before strict `trace_query` decoding.

## Perfetto/Ftrace practice absorption batch — 2026-05-31

### Gaps observed

External trace-analysis practice from Perfetto / Android / ftrace reinforces a
few product gaps that remain even after the first trace-query batches:

- Runnable latency is present in `window_stats`, but there is no dedicated
  `scheduler_latency_stats` view that reports per-wait p95/p99/max, target CPU,
  same-CPU competitors, other-CPU idle time, frequency seen during the wait,
  and line-backed evidence.
- CPU frequency and idle residency are computed, but the engine does not yet
  emit an explicit compute-supply judgement that joins target
  running/runnable intervals with frequency/idle/CPU pressure.
- `root_cause_rank` ranks several evidence types, but runnable latency,
  same-window CPU saturation, low-frequency/limited compute supply, and trace
  completeness gaps should be surfaced as first-class ranked evidence.
- Trace completeness caveats are scattered. Missing `sched_wakeup` /
  `sched_waking`, missing initial `cpu_frequency`, missing block completions,
  and PID/name drift should be disclosed consistently for root-cause views.
- Frame and render pipeline triage is still implicit through generic
  `span_window` / trace spans. Jank-style questions need a format-generic
  `frame_window` / `render_pipeline` view that works for Harmony HiTrace,
  Android atrace, and generic ftrace text rows.
- Blocking rows such as futex/lock/sync/binder/IO are split across trace spans,
  blocked reasons, binder waits, and IO summaries. Models need one
  `critical_blocking_calls` view that lists bounded blocking candidates.
- Models still need to choose many low-level views. A `recipe` layer can return
  a compact standard evidence pack for common questions without hard-coding
  platform-specific assumptions.

### Root cause

The current engine already parses scheduler/resource rows and computes many
building blocks. The remaining gap is product shape: the useful facts are
available, but not grouped into the diagnostic questions customers naturally
ask: "was this runnable delay CPU pressure?", "was the CPU supply too low?",
"which frame/span defines the window?", "what were the top blocking calls?",
and "give me the standard root-cause pack for this trace window".

### Design

Keep the parser and existing views generic. Add deterministic views and bounded
summaries:

- `scheduler_latency_stats`: build line-backed runnable wait intervals from
  `sched_switch`, with p95/p99/max/mean, same-CPU top running competitors,
  high-priority running time, same-CPU busy/idle, other-CPU idle, and frequency
  active at the wait start.
- `window_stats.compute_supply`: summarize running/runnable supply candidates
  with CPU, frequency, idle, runnable wait, high-priority pressure, and a
  conservative advisory verdict (`cpu_pressure`, `low_frequency`,
  `mixed_pressure`, or `insufficient_signal`).
- `root_cause_rank`: fold scheduler latency and compute-supply summaries into
  ranked evidence. These remain advisory evidence, not hard gates.
- Unified completeness caveats: disclose missing wakeup rows for sleep
  intervals, missing initial frequency before selected windows, unpaired IO
  rows, and thread identity drift.
- `frame_window` / `render_pipeline`: reuse trace `B/E` spans and a
  platform-neutral keyword set (frame, vsync, choreographer, render, draw,
  traversal, measure, layout, present, gpu, surface) to derive frame/pipeline
  windows and phase summaries.
- `critical_blocking_calls`: combine blocked reasons, IO latency, binder waits,
  lock/futex/sync-like trace spans/counters/raw markers, D-state intervals, and
  memory/reclaim/GC signals into a bounded candidate list.
- `recipe`: a thin selector over existing deterministic views. `recipe_name`
  supports `auto`, `jank`, `sleep_root_cause`, `runnable_delay`,
  `binder_wait`, `io_wait`, and `cpu_supply`; all outputs still preserve the
  underlying view payload and line references.

Harmony/鸿蒙/东湖 support is not a special fork: the same views run for all
flavors, while priority interpretation continues to use the existing
flavor-aware mapper. Explicit user-provided platform/flavor still wins over
auto-detection and is surfaced in results.

### Task checklist

- [x] P0: add `scheduler_latency_stats` schema, view, summary, evidence, and
      tests for runnable wait percentiles and CPU competition.
- [x] P0: add compute-supply summaries to `window_stats` and root-cause
      ranking, with Harmony-aware priority semantics and generic caveats.
- [x] P0: centralize trace completeness caveats for missing wakeups, missing
      initial CPU frequency, IO pairing gaps, and PID/name drift.
- [x] P1: add `frame_window` / `render_pipeline` views backed by B/E spans and
      frame/render keyword heuristics.
- [x] P1: add `critical_blocking_calls` view for futex/lock/sync/binder/IO/D
      state/memory blocking candidates.
- [x] P1: add `recipe` view plus `recipe_name` parameter for standard evidence
      packs without removing low-level views.
- [x] P1: update tool schema, explorer prompt teaching, REPL/tool summaries,
      docs, and JSON compatibility tests for all new parameters.

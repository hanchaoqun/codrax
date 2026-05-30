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
- Treat ftrace/systrace/hitrace timestamps as seconds on the trace clock
  (`928.081774` means seconds). Durations derived from intervals may be rendered
  in milliseconds, but input window parameters stay in seconds.
- Preserve HarmonyOS/hitrace priority semantics as metadata: for user-space
  priorities, larger numeric value means higher priority; `1-40` is CFS,
  `41-139` is RT, and values outside that range remain raw/system-kernel
  priorities.
- Provide deterministic views:
  - `event_search`
  - `thread_timeline`
  - `window_stats`
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
   - Normalize scheduler, CPU, IO (`block_rq_*`, `block_bio_remap`), binder,
     irq, tracing span, memory-ish, and unknown rows.
   - Cache parsed indexes by path, size, mtime, and parser version.
2. Timeline and stats
   - Build per-thread running / runnable / S sleep / D sleep / IO / unknown
     intervals.
   - Compute same-window CPU busy/idle/frequency, IO, binder, irq, and thread
     top-N summaries.
3. Causality solver
   - Build wakeup DAGs from sleeping/off-CPU intervals and wakeup rows.
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

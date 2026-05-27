# Large-repo scan resilience

Status: implemented; 2026-05-27 short-term parser liveness follow-up completed.

2026-05-27 same-run cache reuse follow-up: in a weak-host Linux-tree run,
the analyzer finished a full `repo_map` build and published the graph to
`Mutable.SearchGraph`, but the explorer's keyword-search prelude then
entered a second `BuildOrLoadGraph` path and showed another "counting repo
index files" line. The root cause is not repo_map capability size; it is a
handoff gap. `keyword_search` passed only `MultiGraph` into `repoMapRank`.
On single-repo runs with no discovered child repos, `MultiGraph` is nil, so
the already-built in-memory graph is invisible and cache validation touches
the 90k-file tree again. This must be fixed by reusing the run-local
`SearchGraph` before disk/cache work, not by trimming repo_map views.

The same customer trace also shows why cache-difference validation needs
explicit progress. Even a hot cache still hashes the current file set to
detect changed bytes; on WSL `/mnt/d` this can be slow. That phase is
correctness-preserving and must remain, but the UI should say "checking cache
differences" with `done/total` progress instead of looking stalled.

## Problem

A repomap full scan of a very large repository (the Linux kernel tree:
93 461 files, 63 896 parsed source files) stresses a small host on two
axes — memory and CPU — and either can take the process or the operator
down.

**Memory.** The scan was OOM-killed mid-parse on a 3.7 GiB host:

```
dmesg: Out of memory: Killed process (codrax) anon-rss:2618512kB
```

Two consecutive runs died at the identical point — the parse phase of
`repomap: full scan (... no cache)` — and each produced no usable cache,
so every retry repeated the full cost from zero.

**CPU.** With the OOM addressed, a later run instead saturated every CPU
core for the duration of the parse phase, starving `sshd` enough to drop
the operator's remote SSH session.

Four independent causes, four independent fixes:

| # | Cause | Fix |
|---|-------|-----|
| 1 | Go heap grows unbounded; GC is not pressured to run before RSS crosses the host OOM threshold. | Install a soft heap limit (`GOMEMLIMIT`) derived from host RAM at startup. |
| 2 | The parse phase produces a large volume of transient garbage (file bytes, tree-sitter ASTs) that is never returned to the OS before the graph-build phase allocates on top of it. | Force a GC + `debug.FreeOSMemory()` after the parse phase. |
| 3 | An interrupted scan installs no manifest, so its already-parsed chunks are unreachable; the next run re-parses everything. | Detect orphan chunk directories and resume from them (hash-verified). |
| 4 | The parse phase saturates every CPU core, starving interactive processes (sshd) and dropping remote sessions. | Cap `GOMAXPROCS` during a scan so the whole Go runtime leaves cores free for the host. |

2026-05-27 follow-up finding: the earlier fixes keep large scans alive, but
the parser collector still had two avoidable peak-memory costs on constrained
hosts:

| # | Cause | Short-term fix |
|---|-------|----------------|
| 5 | `ParseFilesWithProgressSinkAndActive` used `len(entries)`-sized job/result buffers. On huge repos this lets parsed `FileInfo` pointers accumulate in channel storage before the collector drains them, increasing peak heap without adding precision. | Use small worker-proportional buffers and run the job feeder concurrently with the collector, so parser workers, sink writes, and progress collection stay streaming. |
| 6 | Parser worker count was CPU-only. `GOMEMLIMIT` can pressure GC, but too many simultaneous tree-sitter parses still create unnecessary transient heap on low-memory machines. | Derive parser worker count from both `scanCPUBudget()` and the active Go memory limit; cap to roughly one parser worker per 512MiB soft heap budget, never below one. |
| 7 | A single pathological generated/test source file can keep one tree-sitter parse in C for many minutes. The UI appears stuck on that filename, and cooperative cancellation cannot land until the parse returns. | Add a per-file tree-sitter parse safety valve: default 120s, enabled by default, configurable/disableable. A timed-out file becomes path-only with a logged fallback reason instead of blocking the full scan. |
| 8 | REPL double Ctrl+C promised force-exit, but the force-exit path ran worktree cleanup synchronously before `os.Exit`. If cleanup or the host filesystem was stalled, "force" could still wait. | Bound cleanup wait to 500ms on the second Ctrl+C, then exit with code 130. Cleanup remains best-effort; force-exit remains forceful. |
| 9 | Analyzer prewarm graph was not passed into explorer keyword_search in single-repo/no-MultiGraph posture, so a second stage in the same Run repeated inventory/cache validation. | Carry the run-local SearchGraph into keyword_search/repoMapRank and let the existing `GraphFromBusContextOrLoad` reuse path clone+rerank it before disk/cache work. |
| 10 | Cache-difference validation was correct but easy to misread as a stalled scan on very large trees. | Keep the hash check, surface the existing `change_scan` phase as "checking cache differences" with bounded `checked/total` progress. |

## 2026-05-27 Task List — same-run graph reuse and cache validation progress

- [x] Reproduce and document the customer root cause from logs: analyzer
  prewarm publishes `Mutable.SearchGraph`; explorer keyword_search loses that
  handle and starts a second graph load.
- [x] Thread a read-only `SearchGraph` handle through `keywordSearchOptions`
  into `repoMapRank`.
- [x] Reuse the existing `repomap.GraphFromBusContextOrLoad` facade rather
  than adding a parallel cache path.
- [x] Add a single-repo/no-MultiGraph regression test proving keyword_search
  can rank from an in-memory graph when the source file is no longer on disk
  (which would fail if it scanned).
- [x] Preserve multi-repo behavior: active MultiGraph aggregation still wins
  for true multi-repo workspaces, and pending sub-repo filtering is unchanged.
- [x] Keep cache-difference progress visible and localized as a `change_scan`
  phase with bounded `checked/total` updates.
- [x] Run targeted tests and full `make test`.
- [x] Commit and push the batch (`e8935572`).

Accuracy note: the SearchGraph reuse fix does not reduce answer precision. It
reuses the complete graph already built earlier in the same Run and reranks a
clone for the current query. If no matching run-local graph exists, the old
cache/load/scan path remains intact. Cache-difference progress is UI telemetry
only and is not read by model prompts or hard gates.

Fixes 1–3 are synergistic: #1 caps RSS, #2 lowers the graph-build
baseline, #3 shrinks the parse working set on every retry. #4 is
orthogonal — it addresses host responsiveness, not process survival.

## Part 1 — soft heap limit (`internal/memlimit`)

`GOMEMLIMIT` is a *soft* limit: it bounds the GC's heap target, not a
hard ceiling. When the heap approaches the limit the GC runs more
aggressively, trading scan latency for survival. It reins in transient
garbage effectively; it cannot save a process whose *live* working set
genuinely exceeds the limit (the GC then runs continuously without
reclaiming — a death spiral — but that is still strictly better than an
unannounced kill, and a large scan's footprint is dominated by garbage,
not live data).

`memlimit.Apply` runs once at startup, before any tool executes:

1. Disabled by config → no-op.
2. `GOMEMLIMIT` already set in the environment → defer to the operator,
   no-op (Go has already applied it).
3. Explicit `memory_soft_limit_bytes` → use it verbatim.
4. Otherwise read host RAM and apply `total × memory_soft_limit_fraction`.
5. Clamp to a 512 MiB floor so a misconfigured tiny value cannot
   strangle the process.

Host-RAM detection is per-platform (`systemTotalMemory`, build-tagged):
Linux reads `/proc/meminfo` `MemTotal`; macOS reads the `hw.memsize`
sysctl; Windows calls `GlobalMemoryStatusEx`. On any other platform the
auto path is skipped with a logged note and an operator pins
`memory_soft_limit_bytes` explicitly. All detectors return a 64-bit
byte count, so they are correct on 64-bit hosts of any size.

Config knobs (`codrax.yaml`, all optional, pointer-typed):

- `memory_soft_limit_enabled` — master switch (default `true`).
- `memory_soft_limit_fraction` — fraction of host RAM (default `0.8`,
  clamped to `(0, 1]`).
- `memory_soft_limit_bytes` — explicit override; `0` → auto.

## Part 2 — post-parse memory reclaim

`fullScan` calls `runtime.GC()` + `debug.FreeOSMemory()` between the
parse phase and `BuildGraph`, gated on `len(parseable) >=
forceReclaimMinParseableFiles` (2 000) so small repos and REPL turns pay
nothing. At that point the file bytes and tree-sitter ASTs from parsing
are all dead; returning their pages to the OS lowers the RSS baseline
that the graph-build and ranking phases then allocate on top of.

## Part 3 — resumable interrupted scan

### Existing cache shape

A full scan streams parsed `FileInfo` records to
`FileInfoCacheWriter`, which flushes them in 1 024-record JSON chunks
into a per-scan directory `fileinfos.<gen>.d/`. Only `Close()` — reached
solely on success — installs `fileinfos.manifest.json`, which is the
sole reference to a chunk directory. A scan killed mid-parse therefore
leaves a complete set of `chunk-*.json` files that nothing points at;
`pruneOldFileInfoChunkDirs` (also success-only) never runs, so they
survive on disk, unused.

### Resume mechanism

1. **Sentinel.** `NewFileInfoCacheWriter` writes `chunkdir.meta.json`
   into the chunk directory at creation time, recording the cache
   schema version and per-language extractor versions. A resumer
   validates this before trusting any chunk — chunks written by an
   incompatible binary are ignored.

2. **Atomic chunks.** `flush()` writes each chunk to a temp file and
   renames it into place, so a chunk file is either absent or complete.
   A resumer never sees a truncated chunk.

3. **Discovery.** `LoadResumableFileInfos` enumerates `fileinfos.*.d`
   directories, *excludes* the one referenced by a currently valid
   installed manifest (that is the live cache, not an orphan), validates
   each remaining directory's sentinel, and loads every parseable chunk
   into a `relpath → *FileInfo` map. A chunk that fails to parse is
   skipped (its ≤1 024 files simply get re-parsed).

4. **Hash verification.** Before reuse, every candidate's stored content
   hash is re-checked against the file's current bytes in parallel.
   Only files whose bytes (and detected language) still match are
   reused; everything else is re-parsed. This keeps resume correct even
   if the tree changed between the killed run and the retry.

5. **Integration.** `fullScan` partitions the file set into reused vs.
   re-parsed, parses only the latter, and streams *both* into the new
   scan's cache writer — so a retry that is itself interrupted leaves an
   even more complete chunk set for the run after it. Progress converges
   across repeated interruptions instead of restarting from zero.

Orphan directories are pruned by `pruneOldFileInfoChunkDirs` on the
first scan that runs to completion, so they do not accumulate
indefinitely.

Config knob: `repomap_resume_interrupted_scan` (default `true`).

### Safety

- Resume never changes scan *output*: a reused `FileInfo` is
  byte-identical to what a fresh parse of the same bytes would produce
  (verified by content hash), so the resulting graph is identical.
- Resume is bypassed entirely when the knob is off, when no orphan
  directory exists, or when the sentinel is incompatible — i.e. it
  degrades to today's behaviour, never worse.
- No red line is touched: this is repomap cache plumbing, not an
  emit-time gate, contract check, or LLM-facing prompt.

### Language coverage

Resume is language-agnostic. A reused record is keyed by repo-relative
path and gated by content hash + detected language; the chunk
directory's sentinel validates the cache schema and *every* per-language
extractor version (`cacheManifestVersionValid` over the full
`extractorVersions` map — Go, Java, Python, JavaScript, TypeScript,
ArkTS, Cangjie, Kotlin, Ruby, Swift, Lua, Proto, Rust, C, C++). A bump
to any one extractor invalidates resume for the whole directory, so a
reused record can never have been produced by a stale extractor of any
language. `TestResumableFileInfos_ReusesAcrossAllLanguages` exercises
the reuse path against every versioned language.

## Part 4 — CPU headroom for interactive processes

The parse phase runs one tree-sitter worker per CPU core, each pegged at
100 % for the minutes a huge-repo scan takes. With every core saturated,
a small host has no room left to schedule interactive processes; on a
remote box `sshd` misses enough keepalives to drop the operator's
session.

### Why scheduling niceness is not enough

The obvious fix — raise the process / worker-thread `nice` value so the
kernel preempts scan work — was tried and found insufficient. Niceness
is a *per-OS-thread* attribute, and codrax can only reliably renice the
threads it explicitly creates and pins. The threads that *also* burn CPU
are outside that set:

- the Go runtime's **GC worker threads** — and `GOMEMLIMIT` (Part 1)
  deliberately keeps the GC busy;
- the goroutines that run `BuildGraph` and `RankGraph` after parsing,
  scheduled by the runtime onto arbitrary Ms;
- the scheduler / sysmon threads.

That residual normal-priority CPU still saturated the host. Niceness
addressed the parse workers but not the runtime itself, so it was
removed rather than kept as misleading half-coverage.

### The fix — cap `GOMAXPROCS` for the scan

`runtime.GOMAXPROCS` bounds how many OS threads execute Go code
simultaneously across the *entire* runtime — parse workers, GC workers,
graph build and rank alike. `ApplyScanGOMAXPROCS` lowers it to
`baseGOMAXPROCS − repomap_scan_reserve_cpus` for the duration of
`buildOrLoadGraph` and restores it on return. With the budget set to
`cores − 1`, at most `cores − 1` threads ever run Go code at once, so one
core is genuinely free for the OS and `sshd` no matter what the Go side
is doing.

`baseGOMAXPROCS` is captured once at package init so repeated calls do
not compound the subtraction; the parse worker pool size
(`scanCPUBudget`) derives from the same base, so workers and `GOMAXPROCS`
agree.

This is a *hard* guarantee and does cost scan throughput — reserving 1 of
4 cores is a ~25 % slower parse. Because that cost applies to every
scan, it defaults to `0` (off): codrax does not silently slow scans for
everyone. An operator who hits dropped SSH sessions while scanning a
huge repo on a small remote host sets it to `1`. Pure Go,
cross-platform, no platform-specific code.

Config knob:

- `repomap_scan_reserve_cpus` — cores held free for the host during a
  scan (default `0`, i.e. use every core; set `1` on a small remote
  host where a scan starves sshd).

## Part 5 — bounded parser queues and memory-aware workers

Parser scheduling remains language-neutral and precision-preserving: every
file that was parseable before is still parsed with the same extractor. The
change only alters queueing and worker count.

1. **Small queues.** Parser job/result channels are bounded to
   `min(total_files, workers × 2)` instead of `total_files`. The job feeder
   runs in its own goroutine so the collector can drain results while jobs are
   still being submitted. This avoids the classic deadlock that would happen if
   a small result buffer were added while the main goroutine was still trying to
   enqueue every job before reading results.

2. **Memory-aware worker cap.** `parseWorkerBudget` starts from
   `scanCPUBudget()` and then reads the active Go memory limit with
   `debug.SetMemoryLimit(-1)`. When a finite soft heap limit is installed, the
   parser worker count is capped to approximately one worker per 512MiB. This
   is intentionally conservative: it reduces simultaneous file bytes,
   tree-sitter ASTs, and extractor temporaries on small hosts, while high-memory
   hosts still use their CPU budget.

3. **No hard memory gate.** This is not a correctness gate and does not skip,
   truncate, or downgrade any language. If the live graph itself exceeds RAM,
   the long-term answer is a two-stage/partitioned graph design; the short-term
   cap only reduces transient parse pressure.

## Part 6 — per-file parse liveness safety valve

Tree-sitter parsing is normally fast enough that full-scan latency is dominated
by the number of source files, not by one file. Large generated/test fixtures
break that assumption: a single C/C++/JavaScript/etc. source can keep the parser
inside a native grammar for minutes. While that call is in progress, the parser
worker cannot report progress, and Ctrl+C can only be observed by the broader
pipeline after the tool call reaches a cooperative checkpoint.

The short-term commercial fix is a per-file parse timeout:

- `repomap_parse_timeout_enabled` defaults to `true`.
- `repomap_parse_timeout_seconds` defaults to `120`.
- `repomap_parse_timeout_enabled: false` or `repomap_parse_timeout_seconds: 0`
  disables the safety valve.

When a file times out, codrax records a Tier-4/path-only `FileInfo` with
`FallbackReason="tree-sitter parse timed out after ..."`. That preserves the
file in the index and cache, avoids pretending the file was fully understood,
and prevents one generated fixture from blocking the whole repository. This is
language-neutral because it wraps the shared tree-sitter parse helper used by
all grammar-backed languages; non-tree-sitter paths such as Cangjie native
scanning are unaffected.

## Short-term task list

| ID | Status | Task | Validation |
|----|--------|------|------------|
| LRM-T1 | Done | Install soft heap limit from `memory_soft_limit_*` / `GOMEMLIMIT` | `internal/memlimit` tests |
| LRM-T2 | Done | Resume interrupted full scans from hash-verified FileInfo chunks | `TestResumableFileInfos_*` |
| LRM-T3 | Done | Parse large files first and surface active-file progress | `TestParseJobOrderLargeFilesFirst`, heartbeat tests |
| LRM-T4 | Done | Stream FileInfo chunks and derived sidecars instead of writing giant end-of-scan JSON/markdown buffers | cache progress tests |
| LRM-T5 | Done | Bound parser job/result queues and collect concurrently to reduce huge-repo peak heap | parser queue tests |
| LRM-T6 | Done | Cap parser workers by active Go soft heap limit as well as CPU budget | parser worker budget tests |
| LRM-T7 | Done | Add default-on configurable per-file tree-sitter parse timeout with path-only fallback | parser timeout tests |
| LRM-T8 | Done | Make second Ctrl+C force-exit wait only briefly for cleanup before exiting | REPL signal-path code audit |

## Concurrency safety

The repomap cache directory for a repo is a shared path: two codrax
processes pointed at the same repo, or two scans within one process,
can touch it concurrently. The resume path is designed to be safe under
that without a lock.

1. **Atomic writes.** Every chunk file and the chunk-directory sentinel
   are written via a temp file + rename (`writeFileAtomic`). A reader —
   resume logic or a parallel process — sees each file as either absent
   or fully written, never torn. A scan killed between the temp write
   and the rename leaves only a `.tmp-` file, which resume skips by
   name.

2. **In-process verification is race-free.** `verifyResumableFileInfos`
   fans hashing out across workers; results land in one map guarded by
   a mutex. The `FileInfoCacheWriter` is single-writer by construction —
   `fullScan` streams reused records into it *before* the parser's
   collector goroutine starts streaming parsed records, so `Append` is
   never called concurrently.

3. **Reads tolerate concurrent deletion.** `loadOrphanChunkFileInfos`,
   `loadChunkDirInto` and `liveManifestChunkDir` treat every filesystem
   error — a directory or chunk removed mid-read by another process's
   pruning — as "skip and re-parse", never as a failure. A chunk that
   fails to unmarshal is skipped the same way.

4. **Prune does not delete a live scan's directory.**
   `pruneOldFileInfoChunkDirs` (run only on a completed scan) skips any
   chunk directory modified within `chunkDirPruneGraceWindow`
   (10 minutes). A concurrent scan still streaming chunks keeps its
   directory's mtime fresh and is therefore shielded; a genuine orphan
   stops being modified, ages past the window, and is reclaimed by a
   later completed scan. Resume consumes orphans in the meantime, so the
   delayed cleanup costs nothing.

5. **Correctness firewall.** Even if resume picks up chunks from another
   scan that is still running, every reused record is content-hash
   verified against the file's current bytes before use. A mismatched
   or changed file is re-parsed. The resulting graph is therefore
   correct regardless of what concurrent activity produced the orphan
   chunks — concurrency can only affect *how much* is reused, never
   *whether the output is right*.

## Platform support

| Capability | Linux | macOS | Windows | other |
|------------|-------|-------|---------|-------|
| Soft heap limit — explicit `memory_soft_limit_bytes` | ✓ | ✓ | ✓ | ✓ |
| Soft heap limit — auto host-RAM detection | ✓ `/proc/meminfo` | ✓ `hw.memsize` | ✓ `GlobalMemoryStatusEx` | ✗ → use explicit bytes |
| Post-parse `FreeOSMemory` reclaim | ✓ | ✓ | ✓ | ✓ |
| Resumable interrupted scan | ✓ | ✓ | ✓ | ✓ |
| Scan CPU headroom (`repomap_scan_reserve_cpus`) | ✓ | ✓ | ✓ | ✓ |
| Per-file parse timeout (`repomap_parse_timeout_*`) | ✓ | ✓ | ✓ | ✓ |

Every capability either works natively or degrades to a logged no-op —
nothing crashes or fails to build on any platform. The host-RAM
detectors are isolated behind build tags (`memlimit_linux.go` /
`_darwin.go` / `_windows.go` / `_other.go`).

`internal/memlimit` is **pure Go**: it reaches platform APIs through the
standard `syscall` package (`syscall.Sysctl`, `syscall.NewLazyDLL`),
with no `import "C"`. It therefore builds with `CGO_ENABLED` either on or
off and adds no C-toolchain requirement — a Windows build via MinGW
(needed only for the tree-sitter cgo elsewhere in codrax) compiles it
with no extra setup. All host-RAM detectors return a 64-bit byte count,
so the code is correct on 64-bit hosts (amd64, arm64) as well as 32-bit.
The `GOMAXPROCS` cap is plain `runtime` API — platform-independent.

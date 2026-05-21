# Large-repo scan resilience

Status: implemented.

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
| 4 | The parse phase runs one CPU-bound worker per core at normal priority, starving interactive processes (sshd) and dropping remote sessions. | Raise the process / worker-thread scheduling nice value; optional core reservation. |

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
4. Otherwise read host RAM (`/proc/meminfo` `MemTotal`) and apply
   `total × memory_soft_limit_fraction`.
5. Clamp to a 512 MiB floor so a misconfigured tiny value cannot
   strangle the process.

Host-RAM detection is Linux-only (`/proc/meminfo`). On platforms where
it is unavailable the auto path is skipped with a logged note; an
operator can still pin `memory_soft_limit_bytes` explicitly.

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

## Part 4 — CPU politeness (`internal/cpulimit`)

The parse phase runs one tree-sitter worker per CPU core
(`scanCPUBudget`), each pegged at 100 % for the minutes a huge-repo scan
takes. At normal scheduling priority that leaves the kernel little room
to run interactive processes; on a small host `sshd` misses enough
keepalives to drop the operator's session.

The fix is scheduling niceness, not fewer workers. `cpulimit.Apply`
raises the process nice value at startup, and every scan worker
goroutine additionally pins niceness on its own OS thread —
`runtime.LockOSThread` then `cpulimit.NiceCurrentThread` — so the
CPU-bound thread is polite regardless of when the Go runtime created the
underlying M. The kernel then lets normal-priority processes preempt
scan work whenever they are runnable.

The key property: **niceness costs no scan throughput when the host is
otherwise idle.** A niced worker still gets every spare cycle; it only
yields under contention, and `sshd` needs only a sliver. So the scan
runs at full speed in the common case and merely shares fairly when
something interactive wakes up — exactly the desired trade-off, with no
fixed efficiency tax.

Niceness is raised, never lowered, so it never needs privilege. It is
Linux/Unix-only (`setpriority`); on other platforms `Apply` degrades to
a logged no-op and the operator can fall back to core reservation.

### Optional core reservation

`repomap_scan_reserve_cpus` (default `0`) caps every scan worker pool at
`GOMAXPROCS` minus the reserved count, leaving that many cores entirely
free. Unlike niceness this is a *hard* headroom guarantee, but it does
cost scan throughput even on an idle host (e.g. reserving 1 of 4 cores
is a ~25 % slower parse). It is left off by default precisely because
niceness already keeps the host responsive at no throughput cost; the
knob exists for platforms without `setpriority` or for operators who
want a guaranteed-free core regardless of scheduler behaviour.

Config knobs:

- `cpu_politeness_enabled` — master switch (default `true`).
- `cpu_politeness_nice` — target nice value, clamped to `[0, 19]`
  (default `10`).
- `repomap_scan_reserve_cpus` — cores left free (default `0`).

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

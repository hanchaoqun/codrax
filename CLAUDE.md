# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A 5-layer multi-agent AI system in Go that decomposes software engineering tasks through a YAML-driven state machine pipeline. The layers are: Orchestration → Execution (Agents) → Strategy (Skills) → Capability (Tools/MCP) → Intelligence (LLM).

## Build & Run Commands

```bash
# Build (requires CGO for tree-sitter; outputs ./codrax)
make

# Static build (Linux only, musl)
make static

# Single-shot run (explicit flags; any omitted flag falls back to
# config/codrax.yaml then to the code default)
./codrax -config config/orchestrator.yaml -repo . -branch main -request "task" -pipeline-max-steps 50

# Interactive REPL (no -request → enters multi-turn mode with
# /exit /clear /history /compact /help slash commands, memory
# persisted under memory/)
./codrax

# Debug mode: full ReAct trace written to logs/ and mirrored to stdout
./codrax -log-level debug -log-stdout -request "task"

# Run all tests
make test

# Run a single test
go test ./internal/orchestrator/ -run TestOrchestratorRun

# Run tests with verbose output
make test-v
```

## Architecture

### Pipeline Flow

User request → Orchestrator (Layer 1) dispatches specialized Agents (Layer 2) through stages defined in `config/orchestrator.yaml`. Agents use Skills (Layer 3) for strategy, Tools/MCP (Layer 4) for capabilities, and LLM adapters (Layer 5) for intelligence.

**Critical rule:** All tool invocation and LLM calls pass through Layer 2 (Agent). The Orchestrator never directly calls Tools, MCP, or LLM.

### Pipeline Stages

`analyze → explore → plan → design_review → implement → code_review → verify → finalize`

Not all stages run for every task. Task policies in the YAML config control which stages are allowed:
- **analysis**: analyze → explore → finalize
- **implementation**: analyze → explore → plan → implement → verify → finalize
- **high_risk_implementation**: full pipeline including design_review & code_review

Transitions between stages are priority-weighted and filtered by task policy, pipeline settings, and runtime signals (e.g., `HasPlan`, `HasPatch`, `DesignReviewPassed`).

### Key Data Structures (`internal/types/`)

- **BusContext**: Shared execution state flowing through the entire pipeline — tasks, signals, facts, tool results, policy context.
- **AgentContext** (`internal/context/builder.go`): Narrowed view of BusContext built for a specific agent.
- **StageOutput**: What agents return — data, signal updates, new facts, errors.

### Agent System (`internal/agent/`)

All 8 agent types embed `BaseAgent` which provides the ReAct loop (Reason → Act → Observe). Each agent implements the `Evaluator` interface: `BuildInitialPrompt`, `ShouldStop`, `ParseOutput`, `DetermineMissingPiece`.

### Evidence Chain System (`internal/agent/explorer.go`)

The explorer agent includes a programmatic evidence extraction pipeline that supplements LLM investigation with deterministic, source-code-derived facts. This addresses a systemic LLM weakness: the ability to read code files and extract individual facts, but the inability to chain those facts across files to reach specific conclusions (multi-hop reasoning).

**Three-phase investigation model:**

1. **Phase 0 — Breadth Scan.** Keyword search (`keywordSearch` in `keyword_search.go`) combines repo_map structural ranking with grep IDF scoring. Produces a ranked file list with symbol tables. Uses ripgrep when available (auto-detected at startup via `tool.SearchCommand()`), falling back to GNU grep.

2. **Phase 1 — Evidence Collection.** LLM reads files and extracts structured evidence entries tagged `[DIRECT]`, `[CONDITIONAL]`, `[REGISTRATION]`, `[MECHANISM]`, `[RELATIONSHIP]`. The evaluator tracks investigation notes, cross-references, and file coverage with escalating read prompts (3-level: gentle → forceful → final).

3. **Synthesis.** `SynthesisPrompt` assembles a prompt with five programmatic layers, each independent — when upper layers produce no output, lower layers still function:

| Layer | Source | Deterministic? | What it provides |
|-------|--------|:-:|---|
| Concrete Values table | `buildConcreteValuesSection` | Yes | Return values, registrations, config entries from source code |
| Resolution Chains | same function | Yes | `RegisterX binds NewFoo → Foo.Name() returns "bar"` |
| Type Hierarchy Chains | graph `embedding`+`inheritance` relations | Yes | `ExecCommand embeds ReadOnly → IsWrite() returns false` |
| Cross-reference map | `buildCrossReferenceMap` | Yes | Symbols spanning 2+ evidence sets |
| Unresolved Conditions | notes scan | Semi | `[CONDITIONAL]` entries with no matching concrete value |
| Evidence Catalog | LLM investigation notes | No | Structured facts the LLM extracted from files |

**Concrete value extraction (`extractConcreteValues`)** scans source code for patterns that establish ground-truth facts. Recognized patterns, cross-language:

| Pattern | Languages | Example |
|---------|-----------|---------|
| `return "literal"` / `return 'literal'` | All | `Name() → "explorer"` |
| `return true/false/nil/null` | All | `IsWrite() → false` |
| Inline return: `func() { return X }` | Go, Java | single-line methods |
| Arrow function: `() => "value"` | JS/TS | `getName = () => "explorer"` |
| Implicit return (last expression) | Rust, Ruby | `"explorer"` as bare string |
| Constructor-passing call: `verb(NewFoo())` | Go, Java, Python, JS | `Register(NewHandler())` |
| `new Xxx(...)` | Java, JS | `add(new UserController())` |
| Capitalized class instantiation | Python, JS | `register(UserHandler)` |
| Map/dict literal entries: `key: value,` | Go, Python, JS | `AgentExplorer: NewExplorerAgent` |
| Decorator/annotation: `@route("/path")` | Python, Java | `@app.get("/api") → handler` |
| YAML/JSON config leaf values | Config files | `default_agent = explorer` |

**File scanning scope:** all keyword-search scored files + all LLM-read files + files defining symbols mentioned in investigation notes. Short methods (≤3 lines) are fully extracted; longer functions (≤30 lines) are scanned only for registrations/mappings when their name contains `Register`, `Defaults`, `Routes`, `Handlers`, `Config`, `Map`, or `Init`. Config files (YAML/JSON/TOML) are parsed with `yaml.v3`/`encoding/json` and flattened to dotted key paths.

**Resolution chain tracing** is multi-pass (up to 5 iterations): when a concrete value mentions a type name T, all of T's concrete values are pulled into the relevant set and linked. This supports chains of arbitrary depth: `RegisterX binds NewFoo → Foo returns NewBar → Bar.Name returns "baz"`.

**Search backend (`internal/tool/search.go`).** At first use, `SearchCommand()` probes for `rg` via `exec.LookPath` and caches the result. When ripgrep is available: `.gitignore` auto-exclusion (logs, memory, build artifacts), binary skip, smart-case, and batch keyword search via `rg --json` with multi-pattern (`-e kw1 -e kw2 ...`). JSON match output is parsed with lightweight string scanning to build per-keyword file lists for IDF scoring — no file reads, no truncation, large-repo safe. Falls back to GNU grep with manual `--exclude-dir` flags from the shared `tool.ExcludeDirs` list.

**Shared exclude list (`tool.ExcludeDirs`).** Single authoritative directory exclusion list used by GrepTool, keyword search, and `isNoisePath`: `.git`, `.hg`, `.svn`, `node_modules`, `vendor`, `__pycache__`, `.tox`, `logs`, `memory`, `target`, `dist`, `build`.

### Configuration

Three YAML files under `config/`, strictly non-overlapping:

- **`orchestrator.yaml`** — pipeline topology only: stages, transitions (priority-weighted), task policies, agent/skill bindings. Loaded by `internal/config/loader.go` into `ResolvedConfig`.
- **`providers.yaml`** — LLM provider credentials and per-agent model routing. Loaded by `internal/config/providers.go`. Secrets, never committed.
- **`codrax.yaml`** — per-process runtime knobs. Three flat groups by prefix: bare keys (`log_dir`, `memory_dir`, `lang`, `repo`, `branch`, `orchestrator_config`, `providers_config`); `blob_*` keys for tool output sizing (`blob_max_inline_bytes`, `blob_preview_head_bytes`, `blob_preview_tail_bytes`); `pipeline_*` keys for orchestrator behavior and budgets (`pipeline_max_steps`, `pipeline_enable_verify`, `pipeline_require_review`, `pipeline_max_retries_per_stage`, `pipeline_max_stage_visits`, `pipeline_allow_skip_plan_for_small_change`). Loaded by `internal/config/runtime.go`. All fields are pointer-typed so the merge in `main.go` can distinguish "absent" from "explicit zero value".

**Precedence** for the bare keys (lowest wins last): code default → `config/codrax.yaml` → command-line flag.

**Precedence** for the `pipeline_*` keys (lowest wins last): code default → `codrax.yaml` `pipeline_*` keys → command-line flags (`-pipeline-max-steps`, `-pipeline-max-retries`, and `-pipeline-max-stage-visits` only — the three boolean toggles have no CLI override).

**Precedence** for the `blob_*` keys (lowest wins last): code default in `internal/tool/blob.go` → `codrax.yaml` `blob_*` keys. No CLI overrides.

`main.go` applies the runtime overrides into `cfg.PipelineSettings` (from `codrax.yaml`) and via `tool.SetBlobLimits` immediately after `LoadAndResolve`, before any agent or tool runs. Override the entry file with `CODRAX_SETTINGS=path/to/codrax.yaml` to bootstrap an entire environment (since `orchestrator_config` / `providers_config` paths can also live in `codrax.yaml`).

**Path anchoring.** `main.go` searches for `codrax.yaml` in three places: `$CODRAX_SETTINGS` → `<CWD>/config/codrax.yaml` → `<exeDir>/config/codrax.yaml` → `<exeDir>/../config/codrax.yaml` (the `bin/` install layout). The directory containing the found file's `config/` becomes the *anchor*, and any relative default path (`log_dir`, `memory_dir`, `orchestrator_config`, `providers_config`) is rewritten to `anchor/<value>` *before* flag registration so `-h` shows the resolved path. User-supplied flag values are passed through verbatim and remain CWD-relative. `-repo` is excluded from anchoring — its default `.` always means the current working directory because it names a target, not tool state. When no settings file is found anywhere, the anchor falls back to CWD, preserving the historical behavior.

**Per-target-repo namespacing.** Default `log_dir` and `memory_dir` get an extra `<basename>-<fnv32>` suffix derived from the absolute, symlink-resolved `-repo` path, so multiple target repos sharing one codrax install keep their logs and conversation memory in disjoint subtrees (`<anchor>/logs/foo-a3f9c2b1/`, `<anchor>/memory/foo-a3f9c2b1/`). The slug is baked into the flag default so `-h` shows the final path; if the user overrides `-repo` on the command line and leaves `-log-dir`/`-memory-dir` defaulted, `main.go` re-slugs after `flag.Parse`. Explicit `-log-dir`/`-memory-dir` always wins — opting out is one flag away. The slug uses the resolved absolute path so reaching the same repo via different CWDs or symlinks lands in the same memory bucket.

**Multi-instance concurrency model.** Once two codrax processes can land in the same repo slug, the logging and memory subsystems both have to tolerate concurrent writers. The contract is enforced at two layers, both pure stdlib (no `golang.org/x/sys`):

- *Logging.* Filenames carry the writer's PID: `codrax-<timestamp>-<pid>.log`. Every `NewFromFlags` call always opens a fresh file (the legacy "resume into the newest existing file" path was removed because the PID suffix already isolates each process). The retention sweeper in `pruneOldFiles` parses the PID out of each filename and skips deletion if `isPidAlive` reports the owning process is still running, so instance A's rotation never tears instance B's active log file out from under it. PID liveness is `syscall.Kill(pid, 0)` on Unix and `OpenProcess` + `GetExitCodeProcess` on Windows, both behind `//go:build` tags in `internal/logging/pid_{unix,windows}.go`.

- *Memory.* Two file locks govern the directory: `MEMORY.md.lock` is a per-operation flock — shared for `loadIndex`/`BuildContext`, exclusive for `appendIndexEntry`/`Clear`/`compactOldest`. Every operation reloads `s.index` from disk after acquiring the lock so the in-process cache never lags behind a peer's writes. `.instance.lock` is a *lifetime* shared lock used purely for presence detection: at `NewStore` time we try a non-blocking exclusive on it; success means we are the only Store on this directory and may safely run `loadOrphanRecent` to recover the tail of a crashed previous session. Failure means peers exist, the un-compacted turn files in `turns/` belong to their recent buffers, and orphan recovery is skipped to avoid double-compaction. After recovery the alone-Store atomically downgrades to shared (Linux: `flock(LOCK_SH)` in place; Windows: unlock+re-lock with a window that is harmless because no `Append` has happened yet). Turn IDs include the PID (`turn-<unix-nano>-<pid>`) so two processes never collide on a turn filename. The Windows file-lock primitives (`LockFileEx`/`UnlockFileEx`) are not exported by stdlib `syscall` in Go 1.22, so `internal/memory/lock_windows.go` calls them through `syscall.NewLazyDLL("kernel32.dll")` to keep the dependency footprint at zero.

### Runtime subsystems (`internal/logging`, `internal/memory`, `internal/repl`)

- **`internal/logging`** — leveled logger (`error/warning/info/debug`) writing to `logs/codrax-YYYYMMDD-HHMMSS-mmm-<pid>.log` with 4 MB rotation and a 7-file retention cap. Each process always opens a fresh PID-stamped file; `pruneOldFiles` skips files whose embedded PID is still a live process. Package-level `Error/Warning/Info/Debug` free functions delegate to `logging.Default`, initialized from `main()`. A debug-gated `[diag ...]` trace in `BaseAgent.Execute` dumps the full ReAct loop (initial prompt, assistant turns, tool results, stop reason) when `-log-level debug` is set.
- **`internal/memory`** — multi-turn REPL store. Recent turns live in-memory + verbatim on disk under `memory/turns/<id>.md` where `<id>` = `turn-<unix-nano>-<pid>`. Once the recent buffer exceeds 6 turns or 20 KB, the oldest is LLM-summarized into a `{topic, keywords, summary, full_ref}` entry appended to `memory/MEMORY.md`. Cross-process safety: a per-operation flock on `MEMORY.md.lock` serializes mutations and reloads the in-process index after each acquire so peers' writes are immediately visible; a lifetime shared lock on `.instance.lock` lets `NewStore` decide via a non-blocking exclusive try whether it's alone in the directory and may safely run orphan recovery — when peers are present, recovery is skipped to prevent double compaction. On a *true* fresh start (alone, previous session crashed or exited), `NewStore` parses `MEMORY.md` *and* resurrects orphan turns into the recent buffer so the tail of the last session survives crash/Ctrl-C. `BuildContext(request)` assembles a prompt block with recent turns plus keyword-matching compacted entries (index entry + inlined full turn).
- **`internal/repl`** — interactive loop. Reads one line at a time, injects prior conversation via `Store.BuildContext` prepended as `## Prior conversation\n...\n\n## Current request\n...` (zero changes to BusContext or any agent — history flows as part of the request string). Slash commands: `/exit /quit /clear /history /compact /help`.

### Response language switch

`main.go` passes a `-lang` (default `zh`) through `orchestrator.SetLanguage`. Inside `Run()`, a language directive is appended to `BusContext.Preferences`, which `context/builder.go` now renders as a "User Preferences" system section on every agent prompt. The directive always includes a fallback clause so a question asked in another language is answered in that language. Set `-lang=off` (or `none`) to revert to the pre-feature behavior.

## Dependencies

Minimal: only `gopkg.in/yaml.v3`. Go 1.22.5. No external build tools, linters, or CI configuration.

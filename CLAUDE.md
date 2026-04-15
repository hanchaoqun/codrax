# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Codrax is a read-only code analysis tool in Go. It takes a natural-language question about a repository, drives a deterministic 4-stage LLM pipeline (analyze → explore → extract → finalize), and emits a grounded structured answer. It does not modify source files.

## Build & Run Commands

```bash
# Build (requires CGO for tree-sitter; outputs ./codrax)
make

# Static build (Linux only, musl)
make static

# Single-shot run (explicit flags; any omitted flag falls back to
# config/codrax.yaml then to the code default)
./codrax -repo . -branch main -request "task" -pipeline-max-steps 50

# Interactive REPL (no -request → enters multi-turn mode with
# /exit /clear /history /compact /help slash commands, memory
# persisted under memory/)
./codrax

# Debug mode: full ReAct trace written to logs/ and mirrored to stdout
./codrax -log-level debug -log-stdout -request "task"

# Run all tests
make test

# Run a single test
go test ./internal/orchestrator/ -run TestRunTaskGraph_HappyPath

# Run tests with verbose output
make test-v
```

## Architecture

### Pipeline Flow

User request → Orchestrator dispatches agents through 4 hardcoded stages. All tool invocation and LLM calls pass through an agent — the orchestrator never calls tools, MCP, or LLM directly.

### Pipeline Stages

`analyze → explore → extract → finalize`

Hardcoded in `internal/orchestrator/topology.go`. Every stage maps 1:1 to one agent and one default skill:

| Stage | Agent | Default Skill | Purpose |
|---|---|---|---|
| analyze | analyzer | analysis-skill | Classify the request, emit `AnalysisIR` via `emit_analysis` (one LLM call) |
| explore | explorer | repo-explore-skill | Turn A investigation: read_file / grep / repo_map, emit evidence via `emit_evidence` |
| extract | extractor | extract-skill | Turn B structuring: no file IO; drain Turn A's transcript into `emit_answer_symbol` / `emit_hypothesis_verdict` batches |
| finalize | finalizer | final-answer-skill → answer-document-skill | Emit one `emit_answer_document` tool call; deterministic renderer turns the struct into prose |

At dispatch time `Orchestrator.dispatchStage` routes the finalize stage to `answer-document-skill` when it is registered, overriding the default. There is no priority-weighted transition graph, no task policy system, no per-task stage allowlist — the pipeline is a deterministic DAG walk.

### Per-task DAG scheduler

After analyze produces `AnalysisIR.TaskGraph`, `runTaskGraph` walks the graph with a **merged-window scheduler**:

1. Collect every ready non-finalize node (probe / evidence / validate / reconcile). Dispatch them as ONE `StageExplore` call per round. Mark the window done on success.
2. Dispatch `StageExtract` once after a successful explore window (Turn B drains the transcript).
3. If a finalize node is ready, dispatch `StageFinalize`, then run the AnswerContract checker over the final answer.
   - Pass: mark finalize done, record the answer, return.
   - Fail with retry budget remaining: requeue the finalize and every window node, record one cross-window retry, inject the violation diagnostic into the next window's retry hint, loop.
   - Fail with budget exhausted: prepend a fail-loud warning to the answer and return the original body beneath it.
4. Loop until all nodes done or the per-task step budget is exhausted. On a scheduler stall the driver forces one finalize dispatch so the task always terminates with a result.

### Key Data Structures (`internal/types/`)

- **BusContext** — shared execution state (tasks, signals, facts, tool results, analysis IR). The `Mutable` region is the only part tools may write to during the ReAct loop.
- **AgentContext** (`internal/context/builder.go`) — narrowed BusContext view built per agent.
- **StageOutput** — what agents return: data, signal updates, new facts, errors, analysis IR, final answer.
- **AnalysisIR** (`internal/types/analysis_ir.go`) — the analyze stage's sole structured output: `RequestModel`, `TaskGraph`, `EvidencePlan`, `AnswerContract`, `HypothesisSet`, `QualityGate`. Analyzer is the sole writer; downstream stages mutate hypothesis status via the dedicated `MarkHypothesis` API and never rewrite structural fields.
- **ExecutionSignals** — a single-field struct (`HasEnoughFacts bool`). All write-pipeline signals were deleted with the write pipeline.
- **PipelineSettings** — budget knobs only (`MaxRetriesPerStage`, `MaxStageVisits`). Loaded from `codrax.yaml`.

### Agent System (`internal/agent/`)

All 4 agent types embed `BaseAgent` which provides the ReAct loop (Reason → Act → Observe). Each agent implements the `Evaluator` interface: `BuildInitialInstruction`, `ShouldStop`, `ParseOutput`, `DetermineMissingPiece`.

### Analyzer post-processing pipeline (`internal/agent/analyzer.go:buildAnalysisIR`)

The analyzer makes ONE LLM call that emits a `RequestModel` via `emit_analysis`. `ParseOutput` then runs a deterministic chain:

1. `normalizer.Normalize` — canonical TermGraph from the raw request
2. `compiler.InferScenario` + `compiler.Compile` — scenario template → TaskGraph + EvidencePlan + AnswerContract
3. `risk.Evaluate` — 6-dimension risk matrix (Security / DataIntegrity / Compatibility / Performance / Ops / Compliance)
4. `hdp.Plan` + `hdp.Bind` — hypotheses (including risk-driven ones on Security≥3 / DataIntegrity≥3) bound to TaskGraph nodes
5. `counterfactual.Expand` — optional branch expansion for complex+ambiguous explain/root_cause
6. `gate.Run` — deterministic quality gate (coverage, DAG closure, budget sanity, contract completeness, hypothesis coverage)

Gate failures are retryable via analyze re-entry; the analyzer retries once on a retryable reject and fails loud otherwise.

### Evidence Chain System (`internal/agent/explorer.go`)

The explorer agent supplements LLM investigation with deterministic, source-code-derived facts. This addresses a systemic LLM weakness: the ability to read code files and extract individual facts, but the inability to chain those facts across files to reach specific conclusions (multi-hop reasoning).

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

Two YAML files under `config/`, strictly non-overlapping:

- **`providers.yaml`** — LLM provider credentials and per-agent model routing. Loaded by `internal/config/providers.go`. Secrets, never committed.
- **`codrax.yaml`** — per-process runtime knobs. Four flat groups by prefix: bare keys (`log_dir`, `memory_dir`, `lang`, `repo`, `branch`, `providers_config`); `blob_*` keys for tool output sizing (`blob_max_inline_bytes`, `blob_preview_head_bytes`, `blob_preview_tail_bytes`); `pipeline_*` keys for per-run budget limits (`pipeline_max_steps`, `pipeline_max_retries_per_stage`, `pipeline_max_stage_visits`); `analysis_*` keys for emit_analysis runtime validation (`analysis_warn_below_keywords`, `analysis_reject_below_keywords`, `analysis_generic_entity_blocklist`, `analysis_reject_multiple_emit`). Loaded by `internal/config/runtime.go`. All fields are pointer-typed so the merge in `cmd/root.go` can distinguish "absent" from "explicit zero value".

The pipeline topology (stages + agents + skills) is hardcoded in `internal/orchestrator/topology.go` and has no YAML counterpart.

**Precedence** for the bare keys (lowest wins last): code default → `config/codrax.yaml` → command-line flag.

**Precedence** for the `pipeline_*` keys (lowest wins last): code default → `codrax.yaml` `pipeline_*` keys → command-line flags (`-pipeline-max-steps`, `-pipeline-max-retries`, `-pipeline-max-stage-visits`).

**Precedence** for the `blob_*` keys (lowest wins last): code default in `internal/tool/blob.go` → `codrax.yaml` `blob_*` keys. No CLI overrides.

**Precedence** for the `analysis_*` keys (lowest wins last): code default in `internal/tool/analysis_limits.go` (`DefaultAnalysisLimits`) → `codrax.yaml` `analysis_*` keys. No CLI overrides. `analysis_warn_below_keywords` (default 8) tags `emit_analysis` summaries with a soft warning when the LLM emits fewer keywords; `analysis_reject_below_keywords` (default 0 = never reject) fails the tool call when the keyword count is below a hard floor; `analysis_generic_entity_blocklist` (default: `count`, `function`, `thing`, `agent`, `handler`, `module`, and a handful of nearby generic nouns) is the lowercase word list the validator drops from `emit_analysis.entities` so ERM ranking is not poisoned by domain-neutral tokens. Set the blocklist to `[]` to disable the filter. `analysis_reject_multiple_emit` (default false) drives the analyzer's call-count gate in `analyzer.ParseOutput`: when the LLM invokes `emit_analysis` more than once in a single analyze dispatch, false logs a warning and keeps the last write, true additionally sets `StageOutput.Error` so eval harnesses see a loud signal. The 0-call branch is never gated by this knob — it always warns and falls back to `readOrSynthesizeRequestModel`, surfacing `analysis_fallback_used=true` in `StageOutput.Data` alongside the `analysis_emit_calls` counter.

`cmd/root.go` applies the runtime overrides into a local `types.PipelineSettings` and via `tool.SetBlobLimits` + `tool.SetAnalysisLimits` immediately after `LoadRuntimeSettings`, before any agent or tool runs. Override the entry file with `CODRAX_SETTINGS=path/to/codrax.yaml` to bootstrap an entire environment (since `providers_config` paths can live in `codrax.yaml`).

**Path anchoring.** `cmd/root.go` searches for `codrax.yaml` in three places: `$CODRAX_SETTINGS` → `<CWD>/config/codrax.yaml` → `<exeDir>/config/codrax.yaml` → `<exeDir>/../config/codrax.yaml` (the `bin/` install layout). The directory containing the found file's `config/` becomes the *anchor*, and any relative default path (`log_dir`, `memory_dir`, `providers_config`) is rewritten to `anchor/<value>` *before* flag registration so `-h` shows the resolved path. User-supplied flag values are passed through verbatim and remain CWD-relative. `-repo` is excluded from anchoring — its default `.` always means the current working directory because it names a target, not tool state. When no settings file is found anywhere, the anchor falls back to CWD, preserving the historical behavior.

**Per-target-repo namespacing.** Default `log_dir` and `memory_dir` get an extra `<basename>-<fnv32>` suffix derived from the absolute, symlink-resolved `-repo` path, so multiple target repos sharing one codrax install keep their logs and conversation memory in disjoint subtrees (`<anchor>/logs/foo-a3f9c2b1/`, `<anchor>/memory/foo-a3f9c2b1/`). The slug is baked into the flag default so `-h` shows the final path; if the user overrides `-repo` on the command line and leaves `-log-dir`/`-memory-dir` defaulted, `cmd/root.go` re-slugs after flag parse. Explicit `-log-dir`/`-memory-dir` always wins — opting out is one flag away. The slug uses the resolved absolute path so reaching the same repo via different CWDs or symlinks lands in the same memory bucket.

**Multi-instance concurrency model.** Once two codrax processes can land in the same repo slug, the logging and memory subsystems both have to tolerate concurrent writers. The contract is enforced at two layers, both pure stdlib (no `golang.org/x/sys`):

- *Logging.* Filenames carry the writer's PID: `codrax-<timestamp>-<pid>.log`. Every `NewFromFlags` call always opens a fresh file (the legacy "resume into the newest existing file" path was removed because the PID suffix already isolates each process). The retention sweeper in `pruneOldFiles` parses the PID out of each filename and skips deletion if `isPidAlive` reports the owning process is still running, so instance A's rotation never tears instance B's active log file out from under it. PID liveness is `syscall.Kill(pid, 0)` on Unix and `OpenProcess` + `GetExitCodeProcess` on Windows, both behind `//go:build` tags in `internal/logging/pid_{unix,windows}.go`.

- *Memory.* Two file locks govern the directory: `MEMORY.md.lock` is a per-operation flock — shared for `loadIndex`/`BuildContext`, exclusive for `appendIndexEntry`/`Clear`/`compactOldest`. Every operation reloads `s.index` from disk after acquiring the lock so the in-process cache never lags behind a peer's writes. `.instance.lock` is a *lifetime* shared lock used purely for presence detection: at `NewStore` time we try a non-blocking exclusive on it; success means we are the only Store on this directory and may safely run `loadOrphanRecent` to recover the tail of a crashed previous session. Failure means peers exist, the un-compacted turn files in `turns/` belong to their recent buffers, and orphan recovery is skipped to avoid double-compaction. After recovery the alone-Store atomically downgrades to shared (Linux: `flock(LOCK_SH)` in place; Windows: unlock+re-lock with a window that is harmless because no `Append` has happened yet). Turn IDs include the PID (`turn-<unix-nano>-<pid>`) so two processes never collide on a turn filename. The Windows file-lock primitives (`LockFileEx`/`UnlockFileEx`) are not exported by stdlib `syscall` in Go 1.22, so `internal/memory/lock_windows.go` calls them through `syscall.NewLazyDLL("kernel32.dll")` to keep the dependency footprint at zero.

### Runtime subsystems (`internal/logging`, `internal/memory`, `internal/repl`)

- **`internal/logging`** — leveled logger (`error/warning/info/debug`) writing to `logs/codrax-YYYYMMDD-HHMMSS-mmm-<pid>.log` with 4 MB rotation and a 7-file retention cap. Each process always opens a fresh PID-stamped file; `pruneOldFiles` skips files whose embedded PID is still a live process. Package-level `Error/Warning/Info/Debug` free functions delegate to `logging.Default`, initialized from `cmd/root.go`. A debug-gated `[diag ...]` trace in `BaseAgent.Execute` dumps the full ReAct loop (initial prompt, assistant turns, tool results, stop reason) when `-log-level debug` is set.
- **`internal/memory`** — multi-turn REPL store. Recent turns live in-memory + verbatim on disk under `memory/turns/<id>.md` where `<id>` = `turn-<unix-nano>-<pid>`. Once the recent buffer exceeds 6 turns or 20 KB, the oldest is LLM-summarized into a `{topic, keywords, summary, full_ref}` entry appended to `memory/MEMORY.md`. Cross-process safety: a per-operation flock on `MEMORY.md.lock` serializes mutations and reloads the in-process index after each acquire so peers' writes are immediately visible; a lifetime shared lock on `.instance.lock` lets `NewStore` decide via a non-blocking exclusive try whether it's alone in the directory and may safely run orphan recovery — when peers are present, recovery is skipped to prevent double compaction. On a *true* fresh start (alone, previous session crashed or exited), `NewStore` parses `MEMORY.md` *and* resurrects orphan turns into the recent buffer so the tail of the last session survives crash/Ctrl-C. `BuildContext(request)` assembles a prompt block with recent turns plus keyword-matching compacted entries (index entry + inlined full turn).
- **`internal/repl`** — interactive loop. Reads one line at a time, injects prior conversation via `Store.BuildContext` prepended as `## Prior conversation\n...\n\n## Current request\n...` (zero changes to BusContext or any agent — history flows as part of the request string). Slash commands: `/exit /quit /clear /history /compact /help`.

### Response language switch

`cmd/root.go` passes a `-lang` (default `zh`) through `orchestrator.SetLanguage`. Inside `Run()`, a language directive is appended to `BusContext.Preferences`, which `context/builder.go` renders as a "User Preferences" system section on every agent prompt. The directive always includes a fallback clause so a question asked in another language is answered in that language. Set `-lang=off` (or `none`) to revert to the pre-feature behavior.

## Dependencies

Minimal: only `gopkg.in/yaml.v3`. Go 1.22.5. No external build tools, linters, or CI configuration.

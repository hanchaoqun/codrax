# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A 5-layer multi-agent AI system in Go that decomposes software engineering tasks through a YAML-driven state machine pipeline. The layers are: Orchestration → Execution (Agents) → Strategy (Skills) → Capability (Tools/MCP) → Intelligence (LLM).

## Build & Run Commands

```bash
# Build
go build .

# Single-shot run (explicit flags; any omitted flag falls back to
# config/codrax.yaml then to the code default)
go run . -config config/orchestrator.yaml -repo . -branch main -request "task" -max-steps 50

# Interactive REPL (no -request → enters multi-turn mode with
# /exit /clear /history /compact /help slash commands, memory
# persisted under memory/)
go run .

# Debug mode: full ReAct trace written to logs/ and mirrored to stdout
go run . -log-level debug -log-stdout -request "task"

# Run all tests
go test ./...

# Run a single test
go test ./internal/orchestrator/ -run TestOrchestratorRun

# Run tests with verbose output
go test -v ./internal/...
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

### Configuration

Three YAML files under `config/`, strictly non-overlapping:

- **`orchestrator.yaml`** — pipeline topology: stages, transitions (priority-weighted), task policies, agent/skill bindings, `pipeline_settings` (per-stage limits like `max_retries_per_stage` and `max_stage_visits`). Loaded by `internal/config/loader.go` into `ResolvedConfig`.
- **`providers.yaml`** — LLM provider credentials and per-agent model routing. Loaded by `internal/config/providers.go`. Secrets, never committed.
- **`codrax.yaml`** — per-process runtime knobs: log sink, memory directory, default response language, global `Run()` step budget, target repo/branch, and pointers to the two files above. Loaded by `internal/config/runtime.go`. All fields are pointer-typed so the merge in `main.go` can distinguish "absent" from "explicit zero value".

**Precedence** (lowest wins last): code default → `config/codrax.yaml` → command-line flag. Override the entry file with `CODRAX_SETTINGS=path/to/codrax.yaml` to bootstrap an entire environment (since `orchestrator_config` / `providers_config` paths can live in `codrax.yaml` too).

### Runtime subsystems (`internal/logging`, `internal/memory`, `internal/repl`)

- **`internal/logging`** — leveled logger (`error/warning/info/debug`) writing to `logs/codrax-YYYYMMDD-HHMMSS-mmm.log` with 4 MB rotation and a 7-file retention cap. Restart resumes into the newest under-cap file. Package-level `Error/Warning/Info/Debug` free functions delegate to `logging.Default`, initialized from `main()`. A debug-gated `[diag ...]` trace in `BaseAgent.Execute` dumps the full ReAct loop (initial prompt, assistant turns, tool results, stop reason) when `-log-level debug` is set.
- **`internal/memory`** — multi-turn REPL store. Recent turns live in-memory + verbatim on disk under `memory/turns/<id>.md`. Once the recent buffer exceeds 6 turns or 20 KB, the oldest is LLM-summarized into a `{topic, keywords, summary, full_ref}` entry appended to `memory/MEMORY.md`. On restart, `NewStore` parses `MEMORY.md` *and* resurrects "orphan" turns (files under `turns/` not referenced by the index) into the recent buffer so the tail of the last session survives crash/Ctrl-C. `BuildContext(request)` assembles a prompt block with recent turns plus keyword-matching compacted entries (index entry + inlined full turn).
- **`internal/repl`** — interactive loop. Reads one line at a time, injects prior conversation via `Store.BuildContext` prepended as `## Prior conversation\n...\n\n## Current request\n...` (zero changes to BusContext or any agent — history flows as part of the request string). Slash commands: `/exit /quit /clear /history /compact /help`.

### Response language switch

`main.go` passes a `-lang` (default `zh`) through `orchestrator.SetLanguage`. Inside `Run()`, a language directive is appended to `BusContext.Preferences`, which `context/builder.go` now renders as a "User Preferences" system section on every agent prompt. The directive always includes a fallback clause so a question asked in another language is answered in that language. Set `-lang=off` (or `none`) to revert to the pre-feature behavior.

## Dependencies

Minimal: only `gopkg.in/yaml.v3`. Go 1.22.5. No external build tools, linters, or CI configuration.

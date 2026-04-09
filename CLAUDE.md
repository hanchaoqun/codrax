# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A 5-layer multi-agent AI system in Go that decomposes software engineering tasks through a YAML-driven state machine pipeline. The layers are: Orchestration → Execution (Agents) → Strategy (Skills) → Capability (Tools/MCP) → Intelligence (LLM).

## Build & Run Commands

```bash
# Build
go build .

# Run
go run . -config config/orchestrator.yaml -repo . -branch main -request "task" -max-steps 50

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

`config/orchestrator.yaml` defines the entire system: stages, transitions (with priority weights), task policies, pipeline settings, agent bindings, and skill definitions. The config is loaded and resolved by `internal/config/loader.go`.

## Dependencies

Minimal: only `gopkg.in/yaml.v3`. Go 1.22.5. No external build tools, linters, or CI configuration.

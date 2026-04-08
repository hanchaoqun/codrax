# AI Agent System Architecture

A 5-layer AI Agent system built around a YAML-driven Orchestrator state machine. The Orchestrator routes tasks through up to 7 pipeline stages, dispatching specialized Agents equipped with Skills, Tools, and MCP integrations, all powered by LLM reasoning.

> **Orchestrator** decides *who does what* | **Agent** *executes* | **Skill** defines *how* | **Tool/MCP** provides *capabilities* | **LLM** is the *brain*

## Quick Reference

| Layer | Name | Components | Responsibility |
|-------|------|------------|----------------|
| 1 | Orchestration | Orchestrator | Agent selection, pipeline control, state management, termination |
| 2 | Execution | Agent (6 types) | Receive prompt, call LLM, use tools, produce output |
| 3 | Strategy | Skill (9 skills) | Workflow steps, tool suggestions, output format, constraints |
| 4a | Capability | Tool | Local operations (file, exec, grep, test) |
| 4b | Capability | MCP | External system integration (GitHub, DB, Notion, etc.) |
| 5 | Intelligence | LLM | Reasoning, decision-making, text generation |

## Documentation

- **[Architecture Design Document](docs/architecture.md)** — Full system specification including component details, data structures, state machine, and lifecycle
- **[Orchestrator Configuration](config/orchestrator.yaml)** — Reference YAML configuration with stage definitions, transitions, task policies, and feature flags

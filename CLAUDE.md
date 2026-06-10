# CLAUDE.md

Guidance for Claude Code working in this repository. Detail lives in `docs/architecture.md`; this file holds only the parts that affect day-to-day work and the invariants you must not violate.

## Project Overview

Codrax is a Go code-analysis + change-proposal tool. **Read mode** (default): natural-language question → 4-stage LLM pipeline (analyze → explore → extract → finalize) → grounded structured answer; no source files touched. **Write mode** (opt-in via `codrax.yaml :: write_enabled: true` + `--mode=write --write-phase=plan|apply|verify`): adds plan → apply → verify inside a git worktree; main repo HEAD bytes never change automatically.

The analyzer makes one LLM call to classify the request; TaskGraph / EvidencePlan / hypotheses / quality gate are built deterministically by 14 sub-packages under `internal/analysis/`. Fail-loud: missing `emit_analysis` → stage errors and retries.

## Build & Run

```bash
make                          # CGO build → ./codrax
make static                   # Linux musl static
make test
go test ./internal/orchestrator/ -run TestRunTaskGraph_HappyPath
```

```bash
./codrax --repo . --branch main --request "task" --pipeline-max-steps 50
./codrax                      # REPL: /exit /clear /history /compact /log /paste /chat /help
./codrax --log-level debug --log-stdout --request "task"

# Log/perf-triage: attach a runtime artifact so analyzer extracts anchors and seeds RequiredFiles
./codrax --repo . --request "panic source" --log /tmp/panic.txt
kubectl logs pod/foo | ./codrax --repo . --request "analyse crash" --log -
# REPL: /log <path>  |  /log (paste, end /end)  |  /log clear  |  /log show
# Same shape: --htrace / --atrace and /htrace / /atrace.

# Write mode (requires write_enabled: true):
./codrax --mode=write --write-phase=plan --request "add X" --plan-out /tmp/p.json
./codrax --mode=write --write-phase=apply --plan-file=/tmp/p.json --auto-apply
./codrax --mode=write --write-phase=verify --plan-file=/tmp/p.json
# REPL: /mode [read|plan|apply|verify]  /plan [show|clear|list]  /approve  /reject [reason]
```

## Pipeline at a glance

`[log_triage?] [perf_triage?] → analyze → explore → extract → finalize`, hardcoded in `internal/orchestrator/topology.go`. Pre-stages are conditional on `BusContext.AttachedLog` / `AttachedHitrace`. Orchestrator never calls tools/MCP/LLM directly — everything flows through an agent. Write mode runs the read analyzer as classifier (preserves L1 byte-identity), then `write_analyzer` emits `WriteAnalysisIR`, then `BuildWriteTaskGraph` substitutes plan→apply→verify.

For everything else — stage table, agent contracts, retry layering, write-mode hooks, ChangePlan validation, multi-phase groups, Failure Taxonomy, baseline cache, log/perf-triage layers, repomap language matrix + fallback tiers, analyzer post-processing chain, configuration knobs (codrax.yaml prefix groups, providers.yaml schema, lookup precedence, path anchors, multi-instance safety) — read `docs/architecture.md`.

## Red lines (enforced by structural tests)

- **L1**: read mode byte-preserved — `runReadSchedulerLoop` is byte-identical to pre-T4 `runTaskGraph` body.
- **L2**: `write_enabled: false` by default; write modes refuse without yaml gate.
- **L3**: write tools MUST NOT call `ground.BuildContext` / `ground.GroundItem`.
- **L5**: worktree cleanup unconditional — outer defer in `Run()` calls `worktree.DiscardByPath` on any exit.
- **L6**: write skills keep `exec_command` in `ToolSuggestions` (worktree contains blast radius).
- **L7**: `render/mermaid` failure paths MUST rewrite the fence to ` ```text` and inject a `# ⚠ <reason>` leader.
- **L8**: `render/mermaid` library-subset gaps absorbed by L1+L2 shims; MUST NOT propagate into LLM-facing prompts.

Repomap red lines: `extToLang[".ts"] → LangArkTS` only when `IsArkTSProject` finds `oh-package.json5` in any ancestor; `.cjo` denied at scanner; Cangjie `FileInfo.Package` MUST come from `package_clause` (path inference forbidden); all parse fallbacks log `repomap: <file> X→Y (tier N→M): <reason>` at WARN.

Evidence-lite runtime gate: `BaseAgent.executeTool` → `validateAnalyzerPrescanToolCall` rejects `grep` in `StageAnalyze` when `files_only` is not true.

## Architectural principle (red line)

**Precise signals for hard gates, noisy signals for soft guidance.** Hard structural gates (emit-time hard rejects, contract.Check soft fails) MUST read PRECISE signals — single boolean flags, single integer comparisons, verbatim string substring matches, schema-validated typed enums. Noisy signals (ranker scores, grep candidate counts, similarity heuristics, frequency-weighted lists) MUST drive only SOFT guidance — skill-prompt directives, advisory log lines, retry-hint suggestions. The reverse (hard gates on noisy signals) fires on signal-side noise and produces user-visible failures for STRUCTURALLY fine questions. When in doubt, default to soft guidance via skill prompt + answer-side hard gate.

## Configuration files (where they live)

- `providers.yaml` — LLM credentials + per-agent model routing (secret). Per-agent fields use non-zero-overrides inheritance; optional `<name>_fallback` slot for any agent.
- `codrax.yaml` — runtime knobs (`internal/config/runtime.go`); all fields pointer-typed. Prefix groups documented in `docs/architecture.md` §10.
- Lookup: `$CODRAX_SETTINGS` → `<exeDir>/codrax.yaml` → `<exeDir>/codrax/codrax.yaml` → 3 legacy `config/` paths. First hit wins.
- Precedence (lowest→highest): code default → `codrax.yaml` → CLI flag. Only `bare`, `pipeline_*`, log/perf attach, `--chitchat-classifier`, `--color` have CLI overrides.
- Path anchors (`cmd/root.go`): `configAnchor = exeDir` (providers_config); `runtimeAnchor = <CWD>/.codrax/` (log_dir, memory_dir, cache_dir, blob session root); `-repo` is not anchored.

Pipeline topology is code-only; no YAML counterpart.

## Dependencies

`gopkg.in/yaml.v3` only. Go 1.22.5. No linters, no CI config.

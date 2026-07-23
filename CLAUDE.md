# CLAUDE.md

Guidance for Claude Code working in this repository. Detail lives in `docs/architecture.md`; this file holds only the parts that affect day-to-day work and the invariants you must not violate.

## Project Overview

Codrax is a Go code-analysis + change-proposal tool. **Read mode** (default): natural-language question → 4-stage LLM pipeline (analyze → explore → extract → finalize) → grounded structured answer; no source files touched. **Write mode** (CLI opt-in via `--mode=write`, defaulting to Auto Pilot apply; `--write-phase=plan|verify` are advanced lanes; REPL opt-in via `/mode write` / `/write`, or auto mode structured `route=write`): runs controller-first Auto Pilot inside an isolated git worktree. The controller may explore, split, plan, apply, verify, and replan; low/medium deterministic risk proceeds automatically, high risk pauses for approval, critical risk denies, and main repo HEAD/merge never changes automatically (two explicitly-authorized exceptions — fast-forward `/merge` and authorized bare-repo auto-init — are documented in `docs/architecture.md` §8.12 / §8.13). `codrax.yaml :: write_enabled: false` is the organization-level kill switch, absent defaults to enabled.

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
./codrax                      # REPL: /help (common paths), /help all (full table), /exit /clear /history /compact /log /paste /chat
./codrax --log-level debug --log-stdout --request "task"

# Log/perf-triage: attach a runtime artifact so analyzer extracts anchors and seeds RequiredFiles
./codrax --repo . --request "panic source" --log /tmp/panic.txt
kubectl logs pod/foo | ./codrax --repo . --request "analyse crash" --log -
# REPL: /log <path>  |  /log (paste, end /end)  |  /log clear  |  /log show
# Same shape: --htrace / --atrace and /htrace / /atrace.

# Zero-LLM deterministic trace collection (--tracediag mode; pure read, bypasses the LLM pipeline entirely):
./codrax --tracediag examples/tracediag/collect_open_gap_witness.yaml --trace t.systrace --trace-window 10.0..10.2 --trace-tid 1234 --out report.txt

# Write mode (CLI explicit opt-in; REPL auto route enters Auto Pilot; refused when write_enabled: false):
./codrax --mode=write --request "add X"
./codrax --mode=write --write-phase=plan --request "add X" --plan-out /tmp/p.json  # advanced plan-only
./codrax --mode=write --write-phase=apply --plan-file=/tmp/p.json                  # advanced saved-plan apply
./codrax --mode=write --write-phase=verify --plan-file=/tmp/p.json
# REPL: type a clear code-change request. /write <request> forces one write turn; /mode write keeps later code-change goals in Auto Pilot.
# Routine Auto Pilot should not need /workflow or /plan commands; safe active runs auto-resume and typed status cards say when user action is required.
# Advanced recovery/audit only: /workflow show|list|resume|clear  /plan show|list  /approve  /reject  /verify  /merge
```

## Pipeline at a glance

`[log_triage?] [perf_triage?] → analyze → explore → extract → finalize`, hardcoded in `internal/orchestrator/topology.go`. Pre-stages are conditional on `BusContext.AttachedLog` / `AttachedHitrace`. Orchestrator never calls tools/MCP/LLM directly — everything flows through an agent. Write mode runs the read analyzer as classifier (preserves L1 byte-identity), then `write_analyzer` emits `WriteAnalysisIR`, then the write controller drives a durable dynamic DAG of explore/plan/apply/verify/replan/split/append/finish/block actions.

For everything else — stage table, agent contracts, retry layering, write-mode hooks, ChangePlan validation, multi-phase groups, Failure Taxonomy, baseline cache, log/perf-triage layers, repomap language matrix + fallback tiers, analyzer post-processing chain, configuration knobs (codrax.yaml prefix groups, providers.yaml schema, lookup precedence, path anchors, multi-instance safety) — read `docs/architecture.md`.

## Red lines (enforced by structural tests)

- **L1**: read-mode `Run` behavior is write-machinery-independent — `Mode=""` and `Mode=ModeRead` produce equivalent `BusContext` output, pinned by `TestRunMode_ReadByteIdentical` (`internal/orchestrator/mode_dispatch_test.go`) plus `read_e2e_regression_test.go`. `runReadSchedulerLoop` does evolve for read-mode features; the invariant is behavioral equivalence with write machinery absent, not a frozen byte copy.
- **L2**: write Auto Pilot can be entered explicitly (`/mode write` / `/write` / CLI `--mode=write`) or by REPL structured TurnPolicy `route=write`; classifier auto-route may enter `ModeApply` but cannot skip deterministic risk/approval/fingerprint/worktree gates or merge to main. Low-confidence write routes demote to repo analysis, unsettled plans/workflows block conflicting new writes, and explicit `write_enabled: false` refuses all write modes.
- **L3**: write tools MUST NOT call `ground.BuildContext` / `ground.GroundItem`.
- **L5**: worktree cleanup defer sits at the top of `Run()` — every failure path unconditionally calls `worktree.DiscardByPath`; `pipeline_keep_worktree_on_success` / skip-verify is a success-path-only opt-out.
- **L6**: write execution skills (code-write-skill / test-execute-skill) keep `exec_command` in `ToolSuggestions` (worktree contains blast radius). The planner (change-plan-skill) deliberately does NOT expose generic `exec_command` — it uses typed dry-run probes; both directions are pinned in `internal/skill/defaults_test.go`. The `exec_command` read-only surface is an LLM-mistake guardrail (not an adversarial security sandbox); program-body semantic validation (awk/sed) plus in-repo path-operand validation is what makes the worktree blast-radius premise hold (§29.213).
- **L7**: `render/mermaid` failure paths MUST rewrite the fence to ` ```text` and inject a `# · <reason>` leader (soft `·` replaced `⚠` on user-facing surfaces, deliberate — commit 9ec4bf01).
- **L8**: `render/mermaid` library-subset gaps absorbed by L1+L2 shims; MUST NOT propagate into LLM-facing prompts.

Repomap red lines: `extToLang[".ts"] → LangArkTS` only when `IsArkTSProject` finds `oh-package.json5` in any ancestor; `.cjo` denied at scanner; Cangjie `FileInfo.Package` MUST come from `package_clause` (path inference forbidden); all parse fallbacks log `repomap: <lang> <file> tier N→M: <reason>` at WARN (format pinned by TestFallbackWarnLogFormatPinned).

Trace causal-token red line: `internal/tracequery/causal_token_registry.go` is the single source of truth for causal token semantic lanes (demand/supply split, additivity, subject kind); before moving any token or wording lane read `docs/architecture.md` §7.2.1 and ledger §7.4/§7.5 (`docs/design/customer_dead_session_audit_20260703.md`).

Trace correctness sub-systems (status ledger: `docs/design/trace_analysis_open_gap_ledger_20260710.md`; rulings: campaign ledger `real_trace_campaign_20260705.md` §29.24+): bundle provenance hard gate (`internal/tracequery/provenance.go` — only same-domain or calibrated-affine clock mappings enter the shared causal timeline), thread/scheduler identity generations (`thread_incarnation_guard.go`), structured `C|` counter parsing (`trace_counter.go`), and the zero-LLM `--tracediag` collection mode (`internal/tracediag/` + `examples/tracediag/`, see `docs/architecture.md` §13.7).

Proof-lane red line (§29.21, user ruling 2026-07-10): pure model assertions never mint `current_source`-level observations — `CurrentSourceSatisfied` accepts only deterministic tool witnesses (read_file coverage / grep / trace_query typed observations); model claims stay on the advisory lane (`internal/types/observation_ledger.go` / `answer_evidence_origin.go`).

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

Go 1.25.0 (`go.mod`). Direct deps: `gopkg.in/yaml.v3` (config), cobra (CLI), bubbletea/bubbles/huh + lipgloss + glamour + goldmark + pterm + termenv + go-runewidth (REPL/TUI + markdown), go-tree-sitter (repomap), mermaid-ascii (diagram preview), go-udiff (diffs), golang.org/x/{sys,term}. No linters, no CI config.

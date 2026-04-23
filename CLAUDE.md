# CLAUDE.md

Guidance for Claude Code working in this repository.

## Project Overview

Codrax is a read-only code analysis tool in Go. It takes a natural-language question about a repository, drives a deterministic 4-stage LLM pipeline (analyze → explore → extract → finalize), and emits a grounded structured answer. It never modifies source files.

The analyzer makes one LLM call that classifies the request; everything else — TaskGraph, EvidencePlan, hypotheses, quality gate — is built deterministically by 14 sub-packages under `internal/analysis/`. Every `AnalysisIR` field is either consumed by a typed evaluator (the `criterion` package handles `EntryConditions` / `SuccessCriteria` / `StopConditions` / `RequiredEvidence` / `FalsificationCondition` / `AcceptanceTests`) or carries an explicit `pending(artifact-exchange)` marker. Fail-loud contract: if the LLM fails to call `emit_analysis`, the stage errors and retries — no silent fallback to a zero-value IR.

## Build & Run

```bash
make                          # CGO build → ./codrax
make static                   # Linux musl static
make test                     # Run all tests
go test ./internal/orchestrator/ -run TestRunTaskGraph_HappyPath
```

```bash
./codrax --repo . --branch main --request "task" --pipeline-max-steps 50
./codrax                      # interactive REPL (/exit /clear /history /compact /log /paste /chat /help)
./codrax --log-level debug --log-stdout --request "task"

# Log-triage (session 19): attach a runtime log so analyzer extracts
# stack-frame anchors and seeds EvidencePlan.RequiredFiles.
./codrax --repo . --request "这个 panic 哪来的" --log /tmp/panic.txt
kubectl logs pod/foo | ./codrax --repo . --request "analyse this crash" --log -
./codrax --repo . --request "trace this ASAN report" --log-source-prefix /build/src/ --log /tmp/asan.out
# REPL sticky /log (survives across turns): /log <path>  |  /log (paste mode, end with /end)  |  /log clear  |  /log show
# REPL auto-route (one-shot, cleared after dispatch): paste a log body inside a request, splitPastedLog detects and routes it to AttachedLog for this turn only
# Paste fallback (SSH/tmux that strips bracketed paste): /paste → lines → /end; next prompt seeds a [Pasted text #0] token
```

## Architecture

### Pipeline

`[log_triage] → analyze → explore → extract → finalize`, hardcoded in `internal/orchestrator/topology.go`. `[log_triage]` fires only when `BusContext.AttachedLog` is non-empty (Guard in `preStages` slice); the other four stages always run. The orchestrator never calls tools, MCP, or LLM directly — everything flows through an agent.

| Stage | Agent | Default Skill | Purpose |
|---|---|---|---|
| log_triage (conditional) | log_triager | log-triage-skill | LLM-driven extraction: read `AttachedLog`, emit structured `LogBundle` (Meta/Errors/Residue) via `emit_log_triage`. System validates paths + derives Layer 4 (ResolvedFiles/Entities/IntentHint/Coverage). Two-step fallback (`log-segmentation-skill`) fires on coverage < threshold OR size > threshold. Failure is advisory — main pipeline continues. |
| analyze | analyzer | analysis-skill | 1–2 pre-scan rounds (`repo_map` / `grep files_only=true` / `list_files`) then `emit_analysis`. Reads `bus.Mutable.LogTriage()` for Entities / Intent / RequiredFiles seeding. Forbidden: `read_file` / `exec_command`. |
| explore | explorer | explore-skill | Turn A investigation; emit via `emit_evidence` + `emit_investigation_complete`. |
| extract | extractor | extract-skill | Turn B structuring, dispatched **once** before finalize; drains Turn A into `emit_answer_symbol` / `emit_hypothesis_verdict`. |
| finalize | finalizer | answer-document-skill | Single `emit_answer_document` call; deterministic renderer prints prose. |

Phase 1 dispatches analyze with fail-loud retry (`MaxRetriesPerStage`). Phase 2 walks `AnalysisIR.TaskGraph` via a **criterion-aware window scheduler** (`runTaskGraph`):

1. `stopcond.ShouldStop` evaluates `EvidencePlan.StopConditions` (OR-ed) → force-close to finalize.
2. `readyExplorerWindow` collects nodes with satisfied `EntryConditions`; dispatched as one `StageExplore` call. Blocked nodes surface in the retry hint. `ExploreBudget` on `MutableState` enforces per-tool caps from `EvidencePlan.NodeBudgetHints`.
3. After dispatch, `markSuccessCriteriaFailed` requeues failing nodes; a failed `validate` node uses `EdgeValidationFeedback` to requeue only its upstream evidence nodes (fine-grained backtrack). `runAutoVerdicts` sets hypothesis verdicts without an LLM call.
4. When finalize is ready (`firstFinalizeReadyMerged`), dispatch `StageExtract` once, then `StageFinalize`, then AnswerContract check. On fail-with-budget: requeue finalize + upstream, inject violation into next retry hint. Scheduler stall forces one finalize dispatch so the task always terminates.

### Key Data Structures (`internal/types/`)

- **BusContext** — shared execution state. `Mutable` is the only write surface during the ReAct loop.
- **AgentContext** (`internal/context/builder.go`) — per-agent narrowed view. Turn B prompts assembled from structured channels (Prior Stage Findings, Structured Evidence, Hypothesis Verdicts, Answer Symbols). Measurement-scalar questions additionally get a **Raw Tool Outputs from Turn A** section — the only channel that surfaces command-level scalars (`wc -l`, `grep -c`) because `emit_evidence`'s schema requires `source + line_start + anchor_kind`.
- **StageOutput** — agent return: data, signals, facts, errors, analysis IR, final answer.
- **AnalysisIR** (`analysis_ir.go`) — `RequestModel`, `TaskGraph`, `EvidencePlan`, `AnswerContract`, `HypothesisSet`, `QualityGate`. `TaskNode.EntryConditions` and `AnswerContract.AcceptanceTests` are `[]Criterion`. Analyzer is the sole writer; downstream uses `MarkHypothesis` and never rewrites structural fields.
- **ExecutionSignals** — single field `HasEnoughFacts bool`.
- **PipelineSettings** — budget knobs + `GateThresholds` + `Explore` cap.

### Agent system (`internal/agent/`)

All 4 agents embed `BaseAgent` (ReAct loop). Evaluator contract: `BuildInitialInstruction`, `ShouldStop`, `ParseOutput`, `DetermineMissingPiece`. Loop control (continue / stop / inject hint) flows through the optional `LoopController` interface. Throttling, dedup, budget rules live in `LoopPolicy` (`loop_policy.go`). See `docs/architecture.md` §3.2.

### Analyzer post-processing (`analyzer.go:buildAnalysisIR`)

One LLM call emits `RequestModel`, then a deterministic chain:

1. `normalizer.Normalize` with repomap-backed `SymbolResolver` + LLM `Entities` (session 17) — canonical TermGraph. `kindEnWord` surfaces promote to `TermSymbol` only under a **dual gate**: `NormalizeCodeKey(surface) ∈ AnalyzerHints.Entities` ∧ resolver returns ≥1 hit. Confidence scales via `1/(1+ln N)` rarity so generic words that match many definitions sink. Empty entities disables gate A (preserves pre-v3 behavior). `kindCamel`/`kindSnake` surfaces bypass gate A but still get Confidence=1.0 + Domain from resolver hits. Graph handle resolved via `analyzerGraphForNormalize` which first reads `Mutable.SearchGraph()` (writeback from `buildAnalyzerRepoOverview`), else falls back to `BuildOrLoadGraph`.
1a. `analyzer_complexity.reconcileComplexity` + `analyzer_intent.reconcileIntent` — structural overrides on the LLM's classification. Log prefix `[analyzer] * reconciled:`.
2. `compiler.InferScenario` + `compiler.Compile` — scenario template → TaskGraph + EvidencePlan + AnswerContract. Budget is multi-dimensional (`internal/analysis/budget`). `sourcemix.FromTemplateMix` → `NodeBudgetHints`. **Measurement-scalar carve-out** (signal: `isMeasurementScalarRequest`) rewrites `RequiredAnswerShape = ShapeValue` and strips `CritCitationCountGE` from all three citation-gate surfaces (`CitationReq` + `AcceptanceTests` + every `TaskNode.SuccessCriteria`). Grep-able invariant: `CitationReq.Required = false` has **one** producer in production.
3. `risk.Evaluate` — 6-dimension matrix.
4. `hdp.Plan` → `priority.Score` (IntentMatch 0.35 / RiskElevation 0.30 / TermCardinality 0.20 / AmbiguityResolution 0.15). Bindings via `binder.BindByRelevance`.
5. `counterfactual.Expand` — optional branch expansion for complex+ambiguous explain/root_cause; triggers a second `BindByRelevance` pass.
6. `gate.Run` — 7 checks in order: coverage (weighted 1.0/0.7/0.4), dag_closure, budget_sanity, contract_complete, hypothesis_coverage, criterion_resolvable, pending_fields_wellformed (soft; the rest hard). Retryable rejects re-enter analyze once.

### Analysis sub-packages (`internal/analysis/`)

| Package | Entry point | Purpose |
|---|---|---|
| `normalizer` | `Normalize` | Canonical TermGraph |
| `compiler` | `Compile` / `InferScenario` | Template → TaskGraph + EvidencePlan + AnswerContract |
| `budget` | `Compute` | Multi-dim EvidenceBudget |
| `sourcemix` | `FromTemplateMix` / `BudgetForTool` | Per-tool caps |
| `risk` | `Evaluate` | 6-dim risk matrix |
| `hdp` | `Plan` / `Validate` | Hypothesis gen + coverage |
| `priority` | `Score` | 4-dim hypothesis scoring |
| `binder` | `BindByRelevance` | Hypothesis ↔ node binding |
| `counterfactual` | `ShouldExpand` / `Expand` | Branch expansion |
| `gate` | `Run` | 7-check quality gate |
| `stopcond` | `ShouldStop` | StopConditions evaluation |
| `criterion` | `Eval` / `EvalAll` | 19-Kind contract evaluator |
| `contract` | `Check` | AnswerContract validation |
| `dataflow` | `Analyze` | Source→sink chains |
| `logtriage` | `ValidateBundle` / `StripBuildPathPrefix` / `ResolveJavaFile` / `MergeBundles` | System-side validation + Layer-4 derivation for the LLM-emitted `LogBundle`. Replaced the deleted `logparse` regex package (session 20). |

### Log-triage (`AttachedLog` + `log_triage` stage — session 20 LLM-driven)

Session 20 refactor. Extraction is LLM-driven; the system validates and consumes. The `log_triage` pre-stage fires before analyze when `BusContext.AttachedLog` is non-empty. User attaches a runtime log via `--log <file|->` / `--log-text <inline>` / REPL `/log`; the payload lives on `BusContext.AttachedLog` — **never injected into the request string** so `normalizer.Normalize` stays clean.

**Responsibility boundary** (load-bearing):

- **LLM (log_triager agent)**: reads `AttachedLog`, emits a `LogBundle` via `emit_log_triage`. Fills exactly three layers:
  - Layer 1 **Meta**: `lang` (enum), `signals[]` (10-value enum: panic/crash/oom/timeout/permission/db/network/validation/logic/other), optional `summary`
  - Layer 2 **Errors**: recursive tree — top-level slice is parallel snapshots (e.g. Go goroutine dumps); each error carries optional `cause` pointer for chronological chains (Java Caused-by / Rust `#[source]` / Python `__cause__`)
  - Layer 3 **Residue**: `unknown_chunks[]` for text the LLM could not structure (zero information loss)
- **System (`internal/analysis/logtriage/ValidateBundle`)**: validates every path (`StripBuildPathPrefix` → `IsRuntimeInternalFile` filter → Java basename glob → `os.Stat` inside repo → reject "../" escapes) and derives Layer 4 from Layer 1-3: `ResolvedFiles`, `Entities` (cap 32), `IntentHint`, `Coverage`.

Downstream consumers (analyzer only, today):

1. **Entity augmentation** — `logtriage.MergeEntities(rm.AnalyzerHints.Entities, bundle.Entities)` unions the derived tokens into the analyzer's entity list. Normalizer's dual-gate then promotes them to `TermSymbol`.
2. **Intent override** — `reconcileIntent(intent, preds, bundle)` forces `IntentRootCause` when `bundle.IntentHint == IntentRootCause` (derived from `any(frame.line>0)` OR `Signals ∩ {panic, crash, oom}`).
3. **RequiredFiles merge** — `analyzerRequiredFiles` prepends `bundle.ResolvedFiles` ahead of the structural ranker (cap 10) via `logtriage.MergeResolvedFiles`.

**Two-step fallback**: when a single `emit_log_triage` call returns `coverage < log_triage_two_step_coverage` (default 0.3) or the input size exceeds `log_triage_two_step_bytes` (default 32 KB conservative), the log_triager escalates: dispatches the `log-segmentation-skill` which calls `emit_log_segmentation` to split the log into byte-addressed `stack/caused_by/header/context/trace/noise` regions; then re-dispatches `emit_log_triage` per stack-shaped segment. `logtriage.MergeBundles` combines the partial bundles (union of signals; concatenation of Errors[]; re-derived Layer 4). Total LLM calls capped at `log_triage_max_llm_calls` (default 8).

**Feature gate**: `codrax.yaml :: log_triage_enabled` (default true). CLI `--log-source-prefix` or YAML `log_triage_source_prefix` supplies the CI build root. Other knobs: `log_triage_min_bytes` / `log_triage_max_retries` / `log_triage_two_step_enabled` / `log_triage_two_step_bytes` / `log_triage_two_step_coverage` / `log_triage_max_llm_calls`.

**Session 20 red lines held**: LLM is the extractor, system is the validator; no new `Criterion Kind` / `Hypothesis family` / `AnchorKind` / `Scenario`; analyzer carries zero log-triage bolt-on code (the 6 helpers session 19 added under `analyzer.go:1147-1373` are gone); `reconcileIntent` signature is `(intent, preds, *LogBundle)` — no tag-along booleans.

### Explorer evidence chain (`explorer.go`)

Supplements LLM investigation with deterministic source-derived facts. Three phases:

1. **Breadth scan** — `keywordSearch` combines `repo_map` structural ranking with grep IDF scoring. Entity boost 1.3×–1.6×. **Domain boost 1.15×** (session 17): files whose `FileInfo.Package` matches any TermSymbol `Domain` from the analyzer's TermGraph get a sibling-level lift. Strictly < entity boost by construction. Consumed via `irDomainHints(ctx)` → `keywordSearchOptions.DomainHints`. Complexity-aware top-N cap (simple=15, moderate=20, complex=30). Uses ripgrep when available (`tool.SearchCommand()`), falls back to GNU grep.
2. **Evidence collection** — LLM reads files and emits tagged evidence (`[DIRECT]`, `[CONDITIONAL]`, `[REGISTRATION]`, `[MECHANISM]`, `[RELATIONSHIP]`). Mid-loop hint `detectCrossFileSymbolGaps` pushes on symbol references in notes whose defining files aren't in ReadSet.
3. **Synthesis** — `SynthesisPrompt` layers five programmatic sections: Concrete Values table, Resolution Chains, Type Hierarchy Chains, Cross-reference map, Unresolved Conditions, Evidence Catalog. `extractConcreteValues` scans source for return-literal / registration / map-entry / decorator / config-leaf patterns across Go/Java/Python/JS/TS/Rust/Ruby. `HasEnoughFacts` = toolDiversity ∧ fileCoverage ∧ evidenceQuality; `emit_investigation_complete` overrides all heuristics.

Complexity-aware ERM thresholds (`checkRequirementSatisfaction` via `thresholdForKind`) raise per-kind floors at `ComplexityComplex`. `EvidencePlan.RequiredFiles` is rendered into the explorer's initial prompt (as a path list — no content injection) and merged into Phase1Ranking so the CGEC pre-complete gate counts it as first-class coverage.

### Configuration

Two YAML files live flat next to the binary:

- **`providers.yaml`** — LLM credentials + per-agent model routing (`internal/config/providers.go`). Secret, never committed.
- **`codrax.yaml`** — runtime knobs (`internal/config/runtime.go`). All fields pointer-typed so the merge can distinguish "absent" from "explicit zero". Key groups by prefix:
  - bare: `log_dir`, `memory_dir`, `lang`, `repo`, `branch`, `providers_config`
  - `log_*` — retention
  - `blob_*` — tool-output sizing and session retention
  - `pipeline_*` — per-run budget (`max_steps`, `max_retries_per_stage`, `max_stage_visits`)
  - `analysis_*` — `emit_analysis` runtime validation (keyword floors, entity blocklist, multi-emit, `max_prescan_rounds`, hit-ratio floors)
  - `gate_*` — quality-gate thresholds (loaded via `gate.SetGlobalThresholds`)
  - `explore_*` — explorer mid-loop / soft-stop heuristics (`types.DefaultExploreHeuristics`)
  - `agent_*` — per-agent loop limits and sub-topic scaling (`types.DefaultAgentSettings`)
  - `memory_*` — REPL memory-store buffers (`types.DefaultMemorySettings`)
  - `summary_cap_*` — master switch + per-shape Summary length ceilings (`types.SummaryCapConfig`, default disabled)
  - `citation_quote_max_chars` — single knob; Citation.Quote preview ceiling (`types.DefaultCitationMaxQuoteChars = 500`). Oversize Quotes truncate on UTF-8 boundary; file:line always preserved; prose defense via grounder token-match is orthogonal
  - `chitchat_enabled` — REPL `/chat <msg>` slash command (default true). Single LLM Chat call bypasses pipeline for casual conversation. Provider routed via `providers.yaml :: agents.chitchat_responder` (falls back to default). REPL-only; single-shot `--request` never touches it
  - `chitchat_classifier_enabled` — auto-classifier gate before normal dispatch (default false). One LLM tool-call per turn emits `{chitchat, repo_question}`; chitchat reroutes to responder, anything else (incl. errors) falls through to pipeline. Skipped when attached log present. Provider routed via `providers.yaml :: agents.chitchat_classifier`
  - **No codrax.yaml knob for memory summarizer** — presence of `providers.yaml :: agents.memory_summarizer` is itself the opt-in; absent, reuses `llm.default`. Summarizer uses local `emit_memory_summary` tool schema (session 31); every failure path (no tool call / malformed JSON / chat error) falls back to heuristic IndexEntry so compaction cannot break the REPL
  - `cgec_*` — Citation-Grounded Evidence Closure tunables

Pipeline topology (stages/agents/skills) is code-only; no YAML counterpart.

**`codrax.yaml` lookup**: `$CODRAX_SETTINGS` → `<exeDir>/codrax.yaml` → `<exeDir>/codrax/codrax.yaml` → three legacy `config/` paths (deprecation warning). Stops at first hit.

**Precedence** (lowest wins last): code default → `codrax.yaml` → CLI flag (only `bare` and `pipeline_*` groups have CLI overrides).

**Path anchors** (`cmd/root.go`, resolved to absolute paths before flag registration):

- `configAnchor = exeDir` — `providers_config`.
- `runtimeAnchor = <CWD>/.codrax/` — `log_dir`, `memory_dir`, `cache_dir`, blob session root.
- `-repo` is not anchored (its default `.` means CWD).

**Per-repo namespacing**: default `log_dir` / `memory_dir` get a `<basename>-<fnv32>` suffix from the resolved absolute `-repo` path, so multi-repo use keeps logs and memory disjoint. Blob sessions are not per-repo — they're content-addressed (`<tool>-<sha8>.txt`).

**Multi-instance safety** (pure stdlib, no `golang.org/x/sys`): log filenames carry PID; retention skips live-PID files. Memory dir uses `MEMORY.md.lock` (per-op flock, reload-after-acquire) + `.instance.lock` (lifetime shared, non-blocking-exclusive probe to gate orphan recovery). Turn IDs include PID. Windows file locks via `syscall.NewLazyDLL("kernel32.dll")`.

**Evidence-lite runtime gate**: `BaseAgent.executeTool` → `validateAnalyzerPrescanToolCall` rejects `grep` in `StageAnalyze` when `files_only` is not true. Hard constraint: line-level matches overflow analyze's context budget. Other stages unaffected.

### Runtime subsystems

- **`internal/logging`** — leveled logger, 4 MB rotation, default 7-file retention (`log_max_files`). Files named `codrax-YYYYMMDD-HHMMSS-mmm-<pid>.log`. `IsPidAlive` + `FileTimeLayout` reused by `internal/tool/blob`.
- **`internal/memory`** — multi-turn REPL store. Recent turns on disk under `turns/turn-<unix-nano>-<pid>.md`; oldest LLM-summarized into `MEMORY.md` when recent buffer exceeds `MaxRecentTurns` / `MaxRecentBytes`. Compaction runs in a background goroutine (single-flight via `compactInFlight` + `compactWG`); `Close`/`Clear`/`Compact` wait on the WG. REPL persists the paste-folded `display` text; `Turn.RequestForSummary` carries the expanded paste to the summarizer so `IndexEntry.Summary/Keywords` reflect real content, not a `[Pasted text #N]` placeholder. `BuildContext(request)` prepends recent-turn previews + keyword-matched compacted entries (Topic + Summary **only**; full turn text stays on disk) as `## Prior conversation\n...\n\n## Current request\n...`.
- **`internal/repl`** — line-by-line interactive loop. History flows as part of the request string; no BusContext changes.
- **`internal/tool/blob`** — per-process blob storage. Session dir `<CWD>/.codrax/blob/<timestamp>-<pid>/`, assigned to `BusContext.WorkDir`. `PruneBlobSessions` honors live-PID.

### Response language

`-lang` (default `zh`) → `orchestrator.SetLanguage` → appended to `BusContext.Preferences` → rendered as a "User Preferences" system section. Always includes a fallback clause so a question in another language is answered in that language. `-lang=off` / `none` reverts.

## Dependencies

`gopkg.in/yaml.v3` only. Go 1.22.5. No linters, no CI config.

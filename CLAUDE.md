# CLAUDE.md

Guidance for Claude Code working in this repository.

## Project Overview

Codrax is a code analysis + change-proposal tool in Go. In read mode (default), it takes a natural-language question about a repository, drives a deterministic 4-stage LLM pipeline (analyze → explore → extract → finalize), and emits a grounded structured answer without touching source files. In write mode (opt-in via `codrax.yaml :: write_enabled: true` + `--mode=plan|apply|verify`), it additionally drives a plan → apply → verify cycle inside a git worktree, so source-file changes stay sandboxed until the user explicitly approves them; main repo HEAD bytes never change automatically.

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

# Write mode (opt-in; requires codrax.yaml :: write_enabled: true):
./codrax --mode=plan --request "add X feature" --plan-out /tmp/p.json  # single-shot: emit plan JSON, no edits
./codrax --mode=apply --plan-file=/tmp/p.json --auto-apply              # apply + verify; worktree sandboxed
./codrax --mode=verify --plan-file=/tmp/p.json                          # rerun verify against existing plan
# REPL write mode: /mode [read|plan|apply|verify] (sticky); /plan [show|clear|list]; /approve; /reject [reason]
# pipeline_write_retry_budget (yaml, default 0, cap 5): enables verify→plan retry loop with PlanningHint injection
```

## Architecture

### Pipeline

`[log_triage] [perf_triage] → analyze → explore → extract → finalize`, hardcoded in `internal/orchestrator/topology.go`. The two pre-stages are independent: `[log_triage]` fires when `BusContext.AttachedLog` is non-empty; `[perf_triage]` fires when `BusContext.AttachedHitrace` is non-empty. Either, both, or neither may run. The other four stages always run. The orchestrator never calls tools, MCP, or LLM directly — everything flows through an agent.

| Stage | Agent | Default Skill | Purpose |
|---|---|---|---|
| log_triage (conditional) | log_triager | log-triage-skill | LLM-driven extraction: read `AttachedLog`, emit structured `LogBundle` (Meta/Errors/Residue) via `emit_log_triage`. System validates paths + derives Layer 4 (ResolvedFiles/Entities/IntentHint/Coverage). Two-step fallback (`log-segmentation-skill`) fires on coverage < threshold OR size > threshold. Failure is advisory — main pipeline continues. |
| perf_triage (conditional) | perf_triager | perf-triage-skill | LLM-driven extraction: read `AttachedHitrace` (HiTrace / atrace / systrace / perfetto), emit structured `PerfBundle` (Meta + Frames/Janks/Stalls/Startup + Residue) via `emit_perf_trace`. System derives Layer 4 (IntentHint=performance / Entities / ResolvedFiles / signals from threshold checks). Two-step fallback (`perf-segmentation-skill` + `emit_perf_segmentation`) fires on coverage < threshold OR size > threshold. Failure is advisory. |
| analyze | analyzer | analysis-skill | 1–2 pre-scan rounds (`repo_map` / `grep files_only=true` / `list_files`) then `emit_analysis`. Reads `bus.Mutable.LogTriage()` for Entities / Intent / RequiredFiles seeding. Forbidden: `read_file` / `exec_command`. |
| explore | explorer | explore-skill | Turn A investigation; emit via `emit_evidence` + `emit_investigation_complete`. |
| extract | extractor | extract-skill | Turn B structuring, dispatched **once** before finalize; drains Turn A into `emit_answer_symbol` / `emit_hypothesis_verdict`. |
| finalize | finalizer | answer-document-skill | Single `emit_answer_document` call; deterministic renderer prints prose. |

Phase 1 dispatches analyze with fail-loud retry (`MaxRetriesPerStage`). Phase 2 walks `AnalysisIR.TaskGraph` via a **criterion-aware window scheduler** (`runTaskGraph`):

1. `stopcond.ShouldStop` evaluates `EvidencePlan.StopConditions` (OR-ed) → force-close to finalize.
2. `readyExplorerWindow` collects nodes with satisfied `EntryConditions`; dispatched as one `StageExplore` call. Blocked nodes surface in the retry hint. `ExploreBudget` on `MutableState` enforces per-tool caps from `EvidencePlan.NodeBudgetHints`.
3. After dispatch, `markSuccessCriteriaFailed` requeues failing nodes; a failed `validate` node uses `EdgeValidationFeedback` to requeue only its upstream evidence nodes (fine-grained backtrack). `runAutoVerdicts` sets hypothesis verdicts without an LLM call.
4. When finalize is ready (`firstFinalizeReadyMerged`), dispatch `StageExtract` once, then `StageFinalize`, then AnswerContract check. On fail-with-budget: requeue finalize + upstream, inject violation into next retry hint. Scheduler stall forces one finalize dispatch so the task always terminates.

### Write mode (plan → apply → verify, opt-in)

When `BusContext.Mode` is anything but `ModeRead`, the analyzer still runs as a classifier; then `Run()` substitutes the analyzer's read TaskGraph with a fixed plan→apply→verify graph from `BuildWriteTaskGraph` (`internal/orchestrator/write_graph.go`). The same `runTaskGraph` entry point dispatches to `runWriteSchedulerLoop` when `IsWriteGraph` is true (T4 fold-in — write nodes share the criterion env / EntryConditions / SuccessCriteria machinery with read nodes).

| Mode | TaskGraph shape | Exit |
|---|---|---|
| `ModePlan` | single plan node, `OneShot=true`, SC=`CritPlanReady` | Plan emitted on Mutable.ChangePlan; `cmd/root.go` writes JSON to `--plan-out` or `.codrax/plans/<id>.json`; REPL auto-saves via `PlanStore`. |
| `ModeApply` | plan → apply → verify (linear), with `EdgeValidationFeedback` from verify back to plan for the retry cycle. plan node skipped when `PlanPath` is set (R8a: do not regenerate a reviewed plan). | Worktree discarded on exit; `ChangePlan.Status` flipped on disk (applied / applied_failed / verify_failed). |
| `ModeVerify` | single verify node | Standalone re-verify against an existing plan. |

**Stage hooks** (`stage_hooks.go`): the write scheduler runs per-stage Pre/Post hooks around each `dispatchStage` call. `applyPreHook` provisions the worktree, swaps `RepoRoot`, and runs baseline capture. `applyPostHook` persists `applied_failed` on dispatch errors; `verifyPostHook` saves `ChangeReport` to disk + flips status to `applied` or `verify_failed`. `clearForReplan` runs on verify SuccessCriteria failure to reset state and seed `Mutable.PlanningHint` before the verify→plan requeue.

**Agents**: planner (emit_change_plan), coder (per-unit apply_patch with W1/W1b), verifier (run_tests dispatch). All three embed `BaseAgent` like the read agents.

**Test runners**: `internal/tool/run_tests.go` detects the project's manifest and routes to one of these runners (priority order from `detectRunner`):

| Manifest | Runner tag | Command shape | Output parser |
|---|---|---|---|
| `go.mod` | go | `go test -json ./...` | go-test-json events |
| `oh-package.json5` / `build-profile.json5` / `hvigorfile.ts` | hvigor | `hvigorw test` (or `hvigor` from PATH) | JUnit XML (reuses Java path) |
| `cjpm.toml` | cjpm | `cjpm test` | cargo-style text footer |
| `package.json` | node | `npm test -- --json --silent` | jest/vitest JSON |
| `pyproject.toml` / `pytest.ini` / `setup.py` | python | `pytest --json-report` | pytest-json-report file |
| `Cargo.toml` | rust | `cargo test` | cargo text |
| `pom.xml` / `build.gradle[.kts]` | java | `mvn test` / `./gradlew test` | JUnit XML directory walk |
| `Gemfile` | ruby | `bundle exec rspec --format json` | RSpec JSON |
| `CMakeLists.txt` | cmake | `ctest --output-junit <tmp>` | JUnit XML single file |
| `meson.build` | meson | `meson test --xunit-file <tmp>` | JUnit XML single file |
| `Makefile` | make | `make check` (or grepped target) | exit-code synthesis |

**ChangePlan schema** (`internal/types/change_plan.go`): `FileChange.Kind` is one of `create` / `modify` / `delete` / `patch`. `patch` pipes `FileChange.Patch` (unified diff) to `git apply -` inside the worktree. `FileChange.DependsOn []string` is a path-based DAG (B1 Q1); `emit_change_plan` validates dup / unknown / cycle via DFS.

**Write-closure invariants** (W1/W1b enforced at `apply_patch.Execute`): path must appear in `plan.TargetPaths`; every `unit.DependsOn` must already be in `WriteClosure.AppliedSet`. Idempotent: a second apply on the same path is a no-op success.

**verify→plan retry loop** (T4): when `pipeline_write_retry_budget > 0` (yaml knob, default 0, hard-capped at 5), a verify SuccessCriteria failure (TestsPass / NoRegression) requeues the plan node via `EdgeValidationFeedback`. Before the requeue, `clearForReplan` seeds `Mutable.PlanningHint()` from `ChangeReport.FailureSummary` + first 3 failing test names + the prior plan's TargetPaths, and clears `ChangePlan` / `ChangeReport` / `WriteClosure.AppliedSet` / `PlanPath`. `plannerEvaluator.BuildInitialInstruction` consume-once reads the hint and prepends a "## Retry feedback" section to the dispatch.

**Write-mode Criterion evaluators** (`internal/analysis/criterion/eval.go`): `CritPlanReady` reads `WriteClosure.PendingApplies`; `CritPatchApplies` intersects `AppliedSet ∩ TargetPaths`; `CritTestsPass` reads `ChangeReport.TestResults`; `CritNoRegression` diffs `BaselineReport` vs current + checks `MetricDelta.Threshold`. Read-mode shortcut: nil env slots → Satisfied=true under L3 byte-identity.

**Approval flow**:
- Single-shot: `--mode=apply --plan-file=<path>` loads + applies + verifies in one Run. `--auto-apply` required for safety.
- REPL: `/mode plan` generates plan and auto-saves to PlanStore; `/plan show | clear | list` inspects; `/approve` triggers a second Run with `Mode=ModeApply` + `SetPlanPath`; `/reject [reason]` deletes the plan and records a `memory.KindPlan` turn.

**PlanStatus lifecycle** (`types/change_plan.go`): `pending_approval` → `applied` / `applied_failed` / `verify_failed`, persisted via `UpdatePlanStatusOnDisk` called from `orchestrator.persistPlanStatus` at each decision point. `PlanInfo.Status` surfaces in `/plan list`; `/history` pairs each plan with its sibling `.report.json`.

**Red lines** (enforced by structural tests):
- L1: read mode behaviour is preserved — the read scheduler loop (`runReadSchedulerLoop`) is byte-identical to the pre-T4 `runTaskGraph` body, only renamed.
- L2: `write_enabled: false` by default; write modes refuse to dispatch without the yaml gate.
- L3: write tools MUST NOT call `ground.BuildContext` / `ground.GroundItem` (enforced by `internal/tool/write_mode_red_lines_test.go` via go/ast scan).
- L5: worktree cleanup is unconditional — outer defer in `Run()` calls `worktree.DiscardByPath` on any exit path. Stage hooks may CREATE worktrees but never destroy them.
- L6: write skills keep `exec_command` in `ToolSuggestions` (Q2 red line — worktree sandbox contains blast radius).

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
| `logtriage` | `ValidateBundle` / `StripBuildPathPrefix` / `ResolveJavaFile` / `ResolveArkTSFile` / `ResolveCangjieFile` / `ResolveKotlinFile` / `MergeBundles` / `MergeEntities` / `MergeResolvedFiles` | System-side validation + Layer-4 derivation for the LLM-emitted `LogBundle`. Per-language basename resolvers handle stack frames whose path was minified to a bare basename. |
| `perftriage` | `MergePerfBundles` | Two-step merge for the LLM-emitted `PerfBundle`: frame/jank/stall dedup by signature, startup picks largest app_launch_ms, signals/residue union, Layer 4 re-derived. |

### Repomap language coverage

Tree-sitter and Go-native extractors live under `internal/tool/repomap/index/`. `types/lang.go` registers the canonical lang ids and ext map.

| Language | Lang id | Extensions | Extractor | Resolver |
|---|---|---|---|---|
| Go | `go` | `.go` | `extract_go.go` (tree-sitter) | `resolver_go.go` |
| Python | `python` | `.py` `.pyi` | `extract_python.go` | `resolver_python.go` |
| JavaScript | `javascript` | `.js` `.jsx` `.mjs` | `extract_javascript.go` | `resolver_javascript.go` (tsconfig-aware) |
| TypeScript | `typescript` | `.ts` `.tsx` (when no `oh-package.json5` present) | `extract_javascript.go` | shared with JS |
| Java | `java` | `.java` | `extract_java.go` | `resolver_java.go` |
| Kotlin | `kotlin` | `.kt` `.kts` | `extract_kotlin.go` (tree-sitter-kotlin) | `resolver_kotlin.go` (package-decl aware) |
| Rust | `rust` | `.rs` | `extract_rust.go` | `resolver_rust.go` |
| C / C++ | `c` / `cpp` | `.c` `.h` `.cc` `.cpp` `.cxx` `.hpp` `.hh` | `extract_c.go` | `resolver_cpp.go` |
| ArkTS | `arkts` | `.ets` (and `.ts` when `oh-package.json5` lives in any ancestor dir) | `extract_arkts.go` (TS grammar + post-pass for `struct` / 21-decorator whitelist) | `resolver_arkts.go` (oh-package.json5 bundle map via `json5_parser.go`; `@ohos.*` / `@kit.*` / `@hms.*` / `@arkui.*` / `@system.*` builtin black-hole) |
| Cangjie | `cangjie` | `.cj` (`.cjo` denied at scan) | `cangjie_lexer.go` + `cangjie_parser.go` (recursive-descent Go-native, tracks enclosing-type stack) | `resolver_cangjie.go` (cjpm.toml deps via `toml_parser.go`; `std.*` / `core.*` / `runtime.*` / `ohos.*` builtin black-hole) |

**Fallback chain** (`parse_fallback.go`): every parse records a `FileInfo.ParseTier` (1 = primary grammar, 2 = secondary salvage, 3 = regex-only, 4 = path-only). `retrieve.rank.go::parseTierDiscount` multiplies the rank score by 1.0/0.85/0.6/0.3 so degraded parses cannot outrank Tier 1 siblings. Per-language Tier 2+ ratios above `fallbackBannerThreshold` (ArkTS 0.4 / Cangjie 0.5) emit a one-shot WARN banner suggesting an extractor / grammar update.

**Red lines**:
- `extToLang[".ts"] → LangArkTS` only when `IsArkTSProject` finds an `oh-package.json5` in any ancestor dir. Pure TypeScript projects keep `LangTypeScript`.
- ArkTS Tier 1 grammar reject is explicit; downgrade to Tier 2 fires a WARN — never silent.
- `.cjo` Cangjie compiled artefact is denied at scanner time.
- Cangjie `FileInfo.Package` MUST come from the source's `package_clause`; path inference is forbidden.
- All fallbacks must log `repomap: <file> X→Y (tier N→M): <reason>` at WARN level; the banner ratio gates an aggregate signal on top.

### Log-triage (`AttachedLog` + `log_triage` stage)

Extraction is LLM-driven; the system validates and consumes. The `log_triage` pre-stage fires before analyze when `BusContext.AttachedLog` is non-empty. User attaches a runtime log via `--log <file|->` / `--log-text <inline>` / REPL `/log`; the payload lives on `BusContext.AttachedLog` — **never injected into the request string** so `normalizer.Normalize` stays clean.

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

**Two-step fallback**: when a single `emit_log_triage` call returns `coverage < log_triage_two_step_coverage` (default 0.3) or the input size exceeds `log_triage_two_step_bytes` (default 32 KB conservative), the log_triager escalates: dispatches the `log-segmentation-skill` which calls `emit_log_segmentation` to split the log into byte-addressed `stack/caused_by/header/context/trace/noise` regions; then re-dispatches `emit_log_triage` per stack-shaped segment. `logtriage.MergeBundles` combines the partial bundles (union of signals; concatenation of Errors[]; re-derived Layer 4). Total LLM calls capped at `log_triage_max_llm_calls` (default 12 — sized so 1 single-shot + 1 segmentation + 10 per-segment covers the segmenter's 10-segment cap).

**Feature gate**: `codrax.yaml :: log_triage_enabled` (default true). CLI `--log-source-prefix` or YAML `log_triage_source_prefix` supplies the CI build root. Other knobs: `log_triage_min_bytes` / `log_triage_max_retries` / `log_triage_two_step_enabled` / `log_triage_two_step_bytes` / `log_triage_two_step_coverage` / `log_triage_max_llm_calls`.

**Attach cap** (distinct family, `log_attach_*`): `log_attach_max_bytes` (default `50 * 1024 * 1024` = 50 MB) hard-caps every attach surface BEFORE triage sees the payload — applies INDEPENDENTLY to log channel (`--log` / `--log-text` / REPL `/log`) and to perf channel (`--htrace` / `--atrace` / REPL `/htrace` / `/atrace`); a single Run can carry up to 50 MB of each. stdin uses `io.LimitReader(N+1)` so multi-GB pipes never swell process memory. Oversize → tail-truncate + `WARN [cmd] attached log truncated`. Hard ceiling 1 GiB enforced (`maxAttachedLogHardCeiling`) — operators raising `log_attach_max_bytes` past 1 GiB get clamped with a WARN. Fires even when `log_triage_enabled: false` (it's about memory safety, not triage quality). Non-positive values fall back to the default. cmd writes the resolved cap to `maxAttachedLogBytes`; REPL receives it via `Config.AttachedLogMaxBytes`.

### Perf-triage stage (`perf_triage`)

Conditional pre-stage that fires before analyze when `BusContext.AttachedHitrace` is non-empty. The `perf_triager` agent reads the attached HiTrace / atrace / systrace / perfetto text (CLI `--htrace` / `--atrace` / `--htrace-text` / `--atrace-text`; REPL `/htrace` / `/atrace`) and produces a validated `PerfBundle` on `Mutable.PerfTrace()`.

**Layers**:

- Layer 1 **Meta**: `source` (enum: hitrace / atrace / systrace / perfetto / unknown), `duration_ms`, `app_pid`, `signals[]` (jank / cold-start-slow / main-thread-stall / io-block / gc-pause / render-miss), `summary`.
- Layer 2 **Events**: `Frames[]` (FrameNo / TsMs / DurationMs / Phase / Janky), `Janks[]` (start_ts_ms / duration_ms / trigger_span / reason / tags[]), `Stalls[]` (symbol / file / line), `Startup` (mode / app_launch_ms / ability_init_ms / first_frame_ms).
- Layer 3 **Residue**: `residue[]` for unstructurable chunks.
- Layer 4 **Derivation** (system, `internal/tool/emit_perf_trace.go::derivePerfLayer4`): `IntentHint=performance` when any jank/stall/slow-cold-start is observed; `Entities` (cap 32) from trigger spans + tags + stall symbols + startup mode; `ResolvedFiles` (cap 10) from stall files; signals auto-augmented against thresholds `PerfFrameBudget60HzMs` (16.67 ms), `PerfStartupSlowColdMs` (1.2 s), `PerfMainThreadStallMs` (100 ms).

**Two-step fallback**: when single-shot returns `coverage < perf_triage_two_step_coverage` (default 0.3) OR `len(AttachedHitrace) >= perf_triage_two_step_bytes` (default 64 KB), `perf-segmentation-skill` runs first via `emit_perf_segmentation` to split the trace into `frame_window/jank_region/startup/thread_run/context/noise` byte ranges; `emit_perf_trace` then runs once per actionable segment; `internal/analysis/perftriage.MergePerfBundles` unions the partials (frame/jank/stall dedup by signature, startup picks largest app_launch_ms, signals/residue union, Layer 4 re-derived). Total LLM calls capped at `perf_triage_max_llm_calls` (default 12 = 1 single-shot + 1 segmentation + 10 per-segment).

**Downstream consumers** (analyzer):

1. `logtriage.MergeEntities(rm.AnalyzerHints.Entities, perfBundle.Entities)` unions trigger spans + stall symbols into the analyzer's entity list.
2. `analyzerRequiredFiles` unions `perfBundle.ResolvedFiles` with log-derived `ResolvedFiles` before the structural ranker (cap 10 total).

**Feature gate**: `codrax.yaml :: perf_triage_enabled` (default true). Tuning knobs mirror `log_triage_*`: `perf_triage_min_bytes` (200) / `perf_triage_max_retries` (1) / `perf_triage_two_step_enabled` / `perf_triage_two_step_bytes` (64 KB) / `perf_triage_two_step_coverage` (0.3) / `perf_triage_max_llm_calls` (12). Failure of perf_triage is non-fatal — main pipeline continues with `bus.PerfTrace()==nil`.

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
  - `log_*` — retention (own log files)
  - `log_attach_*` — input caps for user-attached logs (applies before log_triage; covers CLI `--log` + REPL `/log` + auto-route)
  - `blob_*` — tool-output sizing and session retention
  - `pipeline_*` — per-run budget (`max_steps`, `max_retries_per_stage`, `max_stage_visits`, `max_verify_retries` for the B2.3 verify→plan loop)
  - `write_enabled` — top-level gate for write mode; must be explicitly `true` before `--mode=plan|apply|verify` will dispatch
  - `analysis_*` — `emit_analysis` runtime validation (keyword floors, entity blocklist, multi-emit, `max_prescan_rounds`, hit-ratio floors)
  - `gate_*` — quality-gate thresholds (loaded via `gate.SetGlobalThresholds`)
  - `explore_*` — explorer mid-loop / soft-stop heuristics (`types.DefaultExploreHeuristics`)
  - `agent_*` — per-agent loop limits and sub-topic scaling (`types.DefaultAgentSettings`). Includes the per-evaluator two-stage iteration caps: `planner_soft_iter_cap` / `planner_hard_iter_cap` (default 6 / 9), `verifier_soft_iter_cap` / `verifier_hard_iter_cap` (default 5 / 8), `extractor_soft_iter_cap` / `extractor_hard_iter_cap` (default 3 / 5), and `coder_soft_iter_slack` / `coder_hard_iter_recovery` (defaults 3 / 3; coder soft cap = `len(plan.TargetPaths) + slack`, hard = soft + recovery). Soft-cap hits + LLM idle → stop; soft→hard window allows one streaming-truncation retry of the agent's structured emit tool (`emit_change_plan` / `apply_patch` / `emit_test_results` / `emit_answer_symbol` + `emit_hypothesis_verdict`); hard cap stops unconditionally. `ResolvedAgentSettings` clamps `hard <= soft` back to defaults. Shared helper: `internal/agent/iteration_cap.go::iterationCapShouldStop`.
  - `memory_*` — REPL memory-store buffers (`types.DefaultMemorySettings`)
  - `summary_cap_*` — master switch + per-shape Summary length ceilings (`types.SummaryCapConfig`, default disabled)
  - `citation_quote_max_chars` — single knob; Citation.Quote preview ceiling (`types.DefaultCitationMaxQuoteChars = 500`). Oversize Quotes truncate on UTF-8 boundary; file:line always preserved; prose defense via grounder token-match is orthogonal
  - `chitchat_enabled` — REPL `/chat <msg>` slash command (default true). Single LLM Chat call bypasses pipeline for casual conversation. Provider routed via `providers.yaml :: agents.chitchat_responder` (falls back to default). REPL-only; single-shot `--request` never touches it
  - `chitchat_classifier_enabled` — auto-classifier gate before normal dispatch (default **true**; one LLM tool-call per REPL turn, fail-safe falls back to pipeline on any error). Emits `{chitchat, repo_question}`; chitchat reroutes to responder, anything else (incl. errors) falls through to pipeline. Skipped when attached log present. Provider routed via `providers.yaml :: agents.chitchat_classifier` — route to a cheap model to cap cost. **CLI override**: `--chitchat-classifier[=true|false]` flag takes precedence over yaml for one run (only classifier has a flag; `chitchat_enabled` is yaml-only — it's a deploy-time decision). Startup emits `[chitchat] auto-classifier: ON (model=...)` at INFO level so the state is never silent
  - **No codrax.yaml knob for memory summarizer** — presence of `providers.yaml :: agents.memory_summarizer` is itself the opt-in; absent, reuses `llm.default`. Summarizer uses local `emit_memory_summary` tool schema (session 31); every failure path (no tool call / malformed JSON / chat error) falls back to heuristic IndexEntry so compaction cannot break the REPL
  - `cgec_*` — Citation-Grounded Evidence Closure tunables

Pipeline topology (stages/agents/skills) is code-only; no YAML counterpart.

**`codrax.yaml` lookup**: `$CODRAX_SETTINGS` → `<exeDir>/codrax.yaml` → `<exeDir>/codrax/codrax.yaml` → three legacy `config/` paths (deprecation warning). Stops at first hit.

**Precedence** (lowest wins last): code default → `codrax.yaml` → CLI flag. Only `bare`, `pipeline_*`, log-triage attach (`--log` / `--log-text` / `--log-source-prefix`), and `--chitchat-classifier` have CLI overrides; every other yaml key is yaml-only.

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

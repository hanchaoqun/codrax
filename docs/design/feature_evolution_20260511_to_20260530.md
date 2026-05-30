# Feature Evolution Digest — 2026-05-11 → 2026-05-30 (953 commits)

> Reconstructed 2026-05-30 from git history (4 parallel digest passes, each confirmed
> against the tree). file:line / SHA accurate as of HEAD `2d8f25aa`; verify before
> relying on a specific line. This is a capability map, not a per-commit changelog.

## Headline new capabilities (SHIPPED in `main`)

### 1. Explorer parallel fan-out (Phase 2A/2B + E' DAG dispatch)
Read-mode exploration moved from serial (`evidence_t0 → t1 → t2`) to orchestrator-owned
capped concurrency. The scheduler detects a DAG "ready window" with ≥2 `NodeEvidence`
siblings, splits it into declaration-ordered sub-windows, and dispatches them concurrently
through a bounded worker pool; outputs and forked mutable deltas merge back in **DAG
declaration order** (stable evidence/fact/ToolResult ordering, not goroutine completion
order).

- **Trigger reads only precise typed signals** — `TaskNode.Type==NodeEvidence`, DAG
  ready-window membership, `PipelineSettings.MaxParallelism`. No question-text parsing, no
  model-prose branching. Honors the "precise signals for hard gates" architecture red line.
- **State isolation** — the stateful `explorerEvaluator` is wrapped in an `ExplorerAgent`;
  each dispatch runs with a **forked `MutableState`** (own tool buffers, budget counters,
  investigation-complete latch, EvidenceClosure read-set). Workers never write the parent
  `busCtx.Mutable`; `RetryHint` is not a shared mailbox.
- **Config** — `pipeline_max_parallelism` (yaml → `PipelineSettings.MaxParallelism`):
  default **2**, `1` = strict serial, `0` = unlimited per surface, hard ceiling **16**. One
  shared cap covers explorer siblings, post-emit reviewer fan-out, and sub-agent fan-out.
- **Regression guards** — single-sub_topic behavior unchanged; `pipeline_max_parallelism: 1`
  forces the legacy serial path; merge order is DAG-stable.
- Files: `internal/orchestrator/parallelism.go`, `internal/orchestrator/contract_check.go`,
  `internal/agent/agent.go`. SHAs `c216aac1` / `86d8c84d` / `e62aabfb` / `850c66d8`.
- An earlier "system-driven fan-out on sub-topic decomposition" pair (`209eaf58` / `29b95bb5`)
  was **reverted same day** (`37298341` / `d6ca6352`) and replaced by the typed DAG approach.

### 2. Runtime trace query engine
A brand-new read-only package `internal/tracequery/` (~3200 LOC: `parse.go`, `query.go`,
`ipc.go`, `thread_selector.go`, `types.go`) plus tool `internal/tool/trace_query.go`.
Deterministic ftrace / systrace / hitrace parser + scheduler / IPC / wakeup-chain query
layer that replaces fragile model-driven `grep` / `awk` / `read_file` loops over large
traces with compact line-backed facts.

- Views: `event_search` / `thread_timeline` / `window_stats` / `ipc_graph` / `wakeup_chain`
  / `evidence_pack`.
- HarmonyOS hitrace priority + blocked-reason (`sched_blocked_reason`, `iowait`), binder IPC
  trace graph, CPU-frequency residency, wakeup causality DAG, latency/pressure joins,
  timestamp normalization (s/ms/us, `秒`/`毫秒`).
- **Lazily exposed** only when the run has an attached trace/log artifact or the user names
  an explicit trace path. Trace symbols/threads are a runtime-artifact lane — never current-
  source `file:line` citations; noisy trace stats are soft guidance only.
- Plan doc: `docs/migration/runtime_trace_query_engine_20260530.md` (the implementation
  already coexists with it). SHAs `94bb9b94` (tool), `3dff35a0` (binder IPC), `9da02556`
  (CPU freq), `7cb20060` (thread_selector). Routing: `6b1e77ed` sends explicit trace paths
  to the engine (`internal/agent/runtime_artifact_request.go`); `bdad6a66` adds a runtime/
  trace explore continuation checkpoint.

### 3. Local-model (OpenAI-compatible) compatibility
Hardening for a local `Qwen3.5-9B`-class model at `127.0.0.1:8000`.

- Shared tool-boundary repair `applyStructuredPayloadCompat`
  (`internal/tool/structured_payload_compat.go`) reused by ~14 emit tools. **Structural
  carrier repair only** — never authors, deletes, or reorders content. Handles JSON-string
  `items`, camelCase keys, single-object→one-element-array wrapping, `_str` field variants,
  truncated-JSON typed failure (re-emit smaller, do not invent rows). A regression guard
  ensures no emit tool can bypass it.
- Provider fields: `recover_text_tool_calls`, `tool_param_compat.mode: repair`,
  `split_string_arrays`.
- **Runtime tool-surface enforcement** (`adc45c0d`) — the execution boundary shares the
  terminal emit-only predicate, so a local model can't hand-write a prescan tool call after
  the stage moved to terminal `emit_analysis`.
- Docs: `local_model_eval_compat_20260523.md`, `json_payload_cognitive_load_gap_20260523.md`.

### 4. Large-repo resilience (validated on the Linux kernel, ~93k files)
`internal/memlimit/` (GOMEMLIMIT soft cap) + `FreeOSMemory` after parse + parse-liveness
timeout + memory-pressure bounding + hash-based cache invalidation + scan-time GOMAXPROCS
limit. Config `repomap_scan_reserve_cpus` (default 0, opt-in). Native macOS/Windows mem/cpu
limits. Linux repomap navigation batch (`linux_repomap_navigation_gap_batch_20260527.md`)
shipped 40/40 tasks.

## Typed contract overhaul (structures landed + wired; some producer eval still in-flight)

- **EvidenceSalience** lane (`internal/types/evidence_salience.go`): values
  `load_bearing` / `exhaust_listed` / `supporting` / `context`; only relaxes the display
  cap, never narrows it (anti-silent-drop).
- **Unified Evidence/Answer Contract** (`unified_evidence_answer_contract_20260520.md`,
  in-flight): separates three conflated axes — evidence origin / requested surface /
  grounding rule. `AnswerEvidenceOrigin` enum + `AnswerClaimBinding` + `ClaimGroundingPolicy`
  (`internal/types/answer_intent_contract.go`, `answer_claim_binding.go`). Retries/reviewers
  route through claim bindings.
- **Observation Ledger** (`observation_ledger_contract_20260521.md`, in-flight): one ledger
  indexing all non-code facts by origin/source-ref/span/claim-role; `EvidenceItem`,
  `aggregate_facts`, `LogBundle`, `PerfBundle`, `ToolResult` become producer adapters
  (`internal/types/observation_ledger.go`). Red line: `emit_evidence` stays current-checkout
  source-line-only; VCS/runtime/command facts never enter repo `citations[]`.
- **Typed relation evidence hardening** (deepest thread, 05-24→26): one common contract
  `BuildTypedRelationQuery(rm, purpose, maxMembers)` (`internal/types/typed_relation_hint.go`)
  generalized across implementer / caller / import / inheritance. A hard gate is allowed only
  when every signal is precise; graph-only / name-only candidates stay soft. Relation
  authority handoff ships with **zero built-in repo-specific providers** (framework only).
  Relation Dossier advisory prompt section. `repo_map` gains a **`relation_map`** view as the
  typed second-stage lens. External-observation → source-anchor candidates
  (`ObservationRelationCandidateSource`).
- Other typed lanes: `ChangeImpactProfile`, `EntityProvenance`/`EntityOrigin`
  (telemetry-only; consumer deferred to a later batch), `CurrentSourceExplanationProfile`,
  `RequestedAnswerDimensionProfile`, `EvidenceRevisionKey` (same-anchor amendments via
  `emit_evidence`), `SourceInventoryAdvisory`, `RequiresSourceOperationSiteMemberSetHandoff`
  (`request_traits.go`, `2613cb98`).

## Other shipped lines

- **Multi-question investigation units** (`075c43dd`): `InvestigationPlan` /
  `InvestigationUnit` typed view separates decomposition (`SubTopics`) / answer partition
  (`Buckets`, outranks) / progress display. REPL renders "调查单元 / 用户分区" instead of a
  misleading "关注点 2/2" completion counter.
- **Subagent default-on** (`bb42cce5`): `propose_sub_agents` default true but bounded;
  `sub_explorer` surface widened to `{repo_map, read_file, list_files, grep, trace_query}`;
  read-only advisory handoff (not promoted to citations unless normal grounding verifies);
  `BusContext.SearchGraph` projection reuse.
- **Explorer completion convergence contract**
  (`explorer_completion_convergence_contract_20260527.md`, Batch 5 done, Batch 6 open):
  generic ranker/pre-scan forced-read debt (`phase1_unread`, `pre_complete.*`) **degrades to
  advisory** when a typed model-owned boundary holds (≥1 `aggregate_facts role=principal_answer`
  + `member_set` with ≥2 grounded members). Fixes a Linux io_uring 3× completion downgrade.
- **grep runtime-artifact contract** (`grep_runtime_artifact_contract_20260529.md`):
  literal / `fixed_string` mode, line-window params (`line_start`/`line_end`), blob streaming
  for huge single-file logs, portable-regex hardening.
- **Post-finalize reviewers made opt-in** (`6db40b20`): self-consistency + semantic-quality
  reviewers now default **off**; operators opt in via runtime.go knobs.
- **REPL / render UX**: structured tool summaries (`internal/render/structured_tool_summary.go`),
  local markdown preview server (`internal/preview/`, embedded mermaid.min.js), final output
  path echo, transient-frame ghost-row fix (`22d31e06`), output-artifact request expansion
  (`567ae052`), Mermaid label normalization unified into `internal/mermaidcompat`
  (`NormalizeSourceForMarkdown`). `git_history_search` deterministic tool.
- **answer_document_v2 fragment recovery** (`22afdd30`): prune stream-corrupted scalar
  `items[]` fragments when a valid item object + visible carrier remain.

## Eval status timeline (the case set grew across sweeps)

| Date | Result | Notes |
|---|---|---|
| 2026-05-20 | 80 / 82 | fail `u7b`, `arkts_repomap` |
| 2026-05-22 | 94 / 103 | source-inventory enumeration cluster (`u8b`/`s11b`/`u7d`/`u7e`/`s3d`/…) |
| 2026-05-24 (local-small campaign) | 4 / 6 | fail VCS-diff dimension drop, `s5b` timeout — seeded RequestedAnswerDimensions + perf fixes |
| 2026-05-26 (full sweep, 118 cases) | 76 / 90 ≈ 84% | **stopped at batch 9/12 by operator**; recurring fails = VCS/current-source visible "changed files/impact" surface (`u7e`–`u7i`), config absent/present mix, member-set scope, `s5b` timeout |

(The 2026-05-30 `eval/parallel_priority.log` 53/58 is a separate, smaller priority sweep.)

## In-flight / open at window close (next-session candidates)

- Unified Evidence/Answer Contract + Observation Ledger: typed structures wired; executable
  future-producer eval coverage pending.
- Explorer completion convergence Batch 6.
- `analyzer_typed_resilience.md` is still "Design (not yet implemented)" at the top, but its
  sub-products (EntityProvenance telemetry, error-granularity lane) partly landed;
  EntityProvenance `UseForSearch`/`UseForShape` consumer deferred to a later batch.
- Source-inventory enumeration / VCS-visible-surface eval fails (the 05-22 / 05-26 recurring
  cluster) — partly addressed by RequestedAnswerDimensions, not fully closed.
- MCP integration remains a stub (`internal/mcp/stdio.go`); the dispatch path is wired but
  the JSON-RPC protocol layer is unimplemented.

Architecture red lines held throughout: precise-signals-only hard gates, external
observations never faked as `file:line`, model-authored topology/answer kept primary, and
single-repo / serial regression guards on every fan-out path.

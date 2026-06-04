# Representative Eval Gap Delivery Plan

Date: 2026-06-04

Source ledger:

- `docs/design/eval_representative_gap_plan_20260604.md`

This document is the implementation ledger for the latest representative eval
sweep. It deliberately solves classes of problems through typed contracts,
origin-aware routing, and deterministic evaluators. It must not introduce hard
routes based on user prose keywords or model-authored prose.

## Red Lines

- Hard decisions consume typed request / route / origin / policy fields only.
- No Go-side keyword matching of user intent.
- No hard gates on model prose.
- Source-code, log/trace, MCP/external observation, operation, and write-mode
  flows keep separate evidence origins.
- Fixes here must preserve the default mixed external-observation + source
  analysis posture unless a typed user policy explicitly excludes source.

## Problem Clusters

### C1. External Observation Policy Overloads Two Decisions

Findings:

- G9 showed that "do not cite log lines as current source" can be collapsed into
  `external_observation_policy.current_source_mode=exclude`, suppressing a
  separately requested current-source lane.

Root cause:

- `ExternalObservationPolicy` can only express current-source inclusion/exclusion.
  It cannot independently express artifact citation semantics.
- `CurrentSourceLaneDecision()` currently checks the exclusion before checking
  positive current-source anchors.

System solution:

- Add `ArtifactCitationMode` to `ExternalObservationPolicy`.
- Preserve JSON compatibility through the existing flexible structured payload
  path; string-wrapped `external_observation_policy` remains accepted.
- Update analyzer schema and skill teaching so models can express "artifact line
  refs are external-only" without suppressing source.
- Make positive typed current-source anchors win over accidental exclusion:
  `current_source_explanation_profile`, resolved current files, required source
  file hints, exact current-source anchors, and `current_key_code` dimensions
  require current-source evidence.

Tasks:

- [x] Extend `internal/types/external_observation_source_policy.go`.
- [x] Extend `emit_analysis` wire schema, parse path, and summary.
- [x] Update analyzer skill text.
- [x] Add tests for artifact-citation policy and source-lane precedence.

### C2. MCP Observation Can Be Misrouted To Command Operation

Findings:

- G10 showed a CLI run where MCP typed-line observation was routed into command
  operation and produced clarification instead of MCP evidence.

Root cause:

- Turn-policy teaching already distinguishes MCP observation from operation, but
  CLI single-shot operation dispatch still depends on the final
  route/needs-operation pair.
- Operation route should require a concrete operation surface, not just a stray
  boolean.

System solution:

- Introduce a typed `IsConcreteOperationPolicy` guard in the turn-policy package.
- CLI and REPL operation entry points should use it before starting operation
  planning.
- MCP / external_tool + `operation=investigate` remains repo/external-observation
  pipeline unless a configured operation provider / skill surface is explicitly
  selected.

Tasks:

- [x] Add concrete operation guard.
- [x] Wire CLI gate to the guard.
- [x] Add direct tests for the exact drift tuple observed in eval.
- [ ] Add hidden eval assertion that MCP cases must produce an MCP observation
  or MCP call, not command clarification.

Follow-up found during Batch 1 validation:

- The concrete-operation guard correctly demotes read-only MCP requests away
  from command operation.
- A second layer remains: the demoted turn-policy hint is not propagated into
  analyzer pre-scan, so MCP-only requests can still receive repo_map/grep source
  navigation before the analyzer emits `emit_analysis`.
- This is not a command-operation bug. It is an origin-hint propagation gap
  between turn classification and analyzer pre-scan.

### C2b. Analyzer Pre-Scan Does Not Consume Turn Origin Hints

Findings:

- `mcp_typed_line` after the C2 route fix no longer enters operation
  clarification, but one run still spent analyzer/explorer effort reading
  repository source files and test fixtures instead of using the MCP resource
  as the primary external observation.

Root cause:

- CLI/REPL turn-policy classification produces structured fields such as
  `source=external_tool`, `route=repo`, and no concrete operation surface.
  These fields are consumed only by local/operation dispatch.
- When the request falls through to the read pipeline, the analyzer starts from
  the raw user request and its generic repo pre-scan rules; it does not see the
  already-computed origin hint.

System solution:

- Add a typed current-turn route/origin hint on `BusContext` or `MutableState`
  and mirror it into `AgentContext`.
- Render that hint in analyzer prompt as source-lane metadata, not as user prose.
- Analyzer pre-scan should treat `source=external_tool|artifact` + no concrete
  operation surface as an external-observation-first request. It may still use
  source later when a typed current-source bridge is emitted, but it must not
  auto-promote external artifact line coordinates into current-source targets.

Tasks:

- [x] Add `TurnRouteHint` type in `internal/types` with route/source/operation
  origin, concrete-operation flag, and confidence.
- [x] Add orchestrator setters for CLI/REPL to pass the guarded turn hint.
- [x] Render analyzer prompt guidance from the typed hint before pre-scan.
- [x] Add tests: MCP-only does not start with repo source scan; MCP+source still
  allows current-source bridge; code/log/trace default flows unchanged.

Validation:

- `go test ./internal/types ./internal/context ./internal/agent ./internal/repl ./cmd` passed.
- `mcp_typed_line` focused eval passed with analyzer pre-scan guidance present,
  `tool_repo_map=0`, and one MCP typed-line observation call.

Follow-up found during Batch 2 validation:

- `mcp_typed_line` no longer starts with analyzer repo pre-scan, but explorer can
  still over-read infrastructure source after using the MCP observation. This is
  an efficiency / planning-teaching gap, not an evidence-origin correctness gap.
- `read_combo_log_current_code_dimensions` showed that exploration and extract
  can preserve rich typed dimensions while the finalizer omits them from the
  final visible answer. This is a handoff-to-answer coverage gap tracked as C6.

### C3. Trace Priority Facts Need Role-Qualified Output

Findings:

- G11 showed trace answers can classify `prev_prio` on an idle row as a generic
  process priority claim.

Root cause:

- Trace events contain role-specific priority fields, but compact output and
  prompts can let the model flatten them into unqualified priority facts.

System solution:

- Preserve and render priority role labels (`prev_task`, `next_task`, `wakee`).
- Treat role-qualified facts as the only source for priority classification in
  trace final answers.

Tasks:

- [ ] Audit trace_query result structs and compact rendering.
- [ ] Add role-qualified priority summary fields.
- [ ] Add tests with idle `prev_prio` / target `next_prio` examples.

### C4. Operation Material Coverage Is Not A First-Class Evaluator

Findings:

- G12 showed an operation answer could be useful but still miss the requested
  manual page / requested material target.

Root cause:

- Operation has payload refs and excerpts, but no typed coverage model over
  requested materials, discovered resources, fetched payloads, and extracted
  user-relevant content.

System solution:

- Introduce a generic material coverage evaluator that works across command
  output, files, web pages, logs, traces, MCP/provider payloads, and skill
  artifacts.
- Do not special-case HTML. Extraction adapters can produce normalized snippets,
  but the coverage state is content-type agnostic.

Tasks:

- [ ] Add `operation.MaterialCoverage` typed state.
- [ ] Feed command/provider material refs and payload excerpts into evaluator.
- [ ] Use evaluator verdict in operation continuation/final-answer decisions.
- [ ] Add tests for complete text payload, missing discovered link, payload ref
  not consumed, and binary/non-text material.

### C5. Tool Schema / Eval Noise

Findings:

- G13: stage-specific tool availability and examples still drift.
- G14: write-mode eval summary overcounts changes.
- G15: scalar/config subtopic gate can false-reject file-backed dimensions.
- G16: deterministic supplements can be noisy when the answer is already
  complete.

System solutions:

- Generate or validate prompt examples from backend schemas.
- Separate eval metrics for `ChangePlan.changes[]` from write-analysis task
  metadata.
- Add typed anchor kinds for config/scalar dimensions.
- Render deterministic supplements only when a typed completeness gap remains.

Tasks:

- [ ] Add unavailable-tool and JSON-retry metrics to eval summaries.
- [ ] Fix write eval summary metric labels/counts.
- [ ] Extend subtopic coherence by anchor kind.
- [ ] Gate supplement rendering on typed completeness gaps.

### C6. Requested Answer Dimensions Are Prompt-Only At Finalization

Findings:

- `read_combo_log_current_code_dimensions` exploration and extraction preserved
  the requested dimensions (`日志线索`, `当前关键代码`, `异常类型区分`, `影响`, `边界`),
  and the finalizer prompt received them, but the final answer collapsed the
  result into summary / chain / caveat blocks and omitted visible sections for
  several required dimensions.

Root cause:

- `requested_answer_dimensions` currently acts as prompt guidance plus a narrow
  last-mile source quote supplement.
- There is no post-emit evaluator that compares typed requested dimensions with
  the emitted answer document and asks the finalizer to repair missing visible
  dimensions before stopping.

System solution:

- Add a finalizer answer-document coverage evaluator for typed requested answer
  dimensions.
- The evaluator must consume only structured analysis dimensions and structured
  answer-document blocks; it must not infer user intent from keywords or apply a
  hard gate to model-authored prose.
- Missing dimensions should trigger a bounded repair hint that asks the model to
  add visible headings, table rows, or concise bullets while preserving existing
  grounded content.

Tasks:

- [ ] Add requested-dimension visible-coverage detection for AnswerDocumentV2.
- [ ] Wire coverage into finalizer mid-loop before the normal answer-document
  stop signal.
- [ ] Add unit tests for missing and satisfied requested dimensions.
- [ ] Rerun `read_combo_log_current_code_dimensions` and review the final answer
  manually.

## Delivery Batches

1. **Batch 1: Typed lane and route boundary**
   - C1 and C2 implementation.
   - Focused unit tests and `mcp_typed_line` rerun.
   - Status: implemented in code; focused tests passed.
   - Validation:
     - `go test ./internal/types ./internal/tool ./internal/repl ./cmd` passed.
     - `read_combo_log_current_code_dimensions` passed 3/3, confirming external
       artifact citation policy does not suppress explicitly requested current
       source analysis.
     - `mcp_typed_line` improved from 1/3 to route-correct runs; latest focused
       run showed `raw_route=operation route=repo ... source=external_tool`,
       but also exposed C2b analyzer pre-scan origin-hint gap. Full 3/3 is
       deferred until C2b.

2. **Batch 2: Trace role facts and eval metric cleanup**
   - C2b implementation.
   - Status: implemented in code; focused tests passed; `mcp_typed_line` passed
     with analyzer repo pre-scan removed.

3. **Batch 3: Requested dimension finalizer coverage**
   - C6 implementation and focused eval rerun.

4. **Batch 4: Trace role facts and eval metric cleanup**
   - C3 and G14 implementation.
   - Focused tracequery and write-mode eval checks.

5. **Batch 5: Operation material coverage**
   - C4 architecture and tests.
   - Rerun operation material cases.

6. **Batch 6: Schema/noise cleanup**
   - C5 implementation.
   - Representative eval subset rerun.

## Validation

- Focused Go tests for touched packages.
- Focused evals:
  - `read_combo_log_current_code_dimensions`
  - `mcp_typed_line`
  - `trace_query_donghu_mixed_platform`
  - `operation_web_manual_summary`
  - `patch_go_typo`
  - `qf_config_precedence`
- Manual review of final answers, not just PASS status.

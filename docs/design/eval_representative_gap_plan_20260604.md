# Representative Eval Gap Root Cause and Delivery Plan

Date: 2026-06-04

Source run:

- `docs/design/eval_representative_run_20260604_152306.md`
- `eval/results/representative-p2-20260604-154448`

This document turns the representative eval findings into code-grounded root
cause analysis and a generic delivery plan. It intentionally separates eval
assertion gaps from product gaps, because treating a correct answer as a product
bug is as dangerous as treating a green regex as a correct answer.

## Red Lines

- Do not hard-route by keyword matching user prose.
- Do not hard-gate on model-authored prose.
- Use typed classifier / request model / answer contract / observation origin
  fields for hard decisions.
- Keep read-mode source analysis, log/trace analysis, mixed external+source
  analysis, write mode, and REPL operation behavior stable.
- Existing REPL operation behavior is not the implementation model for CLI by
  copy-paste. Shared operation execution needs a reusable controller.

## Findings and Root Causes

### G1. Provider 402 turned the whole matrix into no-signal work

Observed issue:

- Earlier representative sweeps all hit
  `LLM API error (status 402): insufficient_balance_error`.
- `eval/run.sh` can classify each case as
  `BLOCKED_PROVIDER insufficient_balance`, but the full matrix still launches
  and spends time on repo setup / dispatch retries before each case is blocked.

Code status:

- `eval/runner_lib.sh::eval_detect_provider_blocked` already detects
  timestamped control-plane 402 / `insufficient_balance_error` lines.
- The detector ignores `DEBUG [diag ...] ASSISTANT content`, so customer logs
  that quote a 402 string do not contaminate product verdicts.
- `eval/run.sh` writes `BLOCKED_PROVIDER ...` after the case has already run.

Root cause:

- Provider health classification exists only at per-case tail time.
- There is no sweep-level provider preflight.

Generic solution:

- Add a matrix preflight that runs before parallel case workers.
- Use the same binary, provider config, OAuth/TLS settings, and model route as
  the sweep.
- Perform one minimal LLM adapter health request in a scratch/empty repo.
- If the provider blocker is deterministic (config/auth/balance/model list),
  stop the matrix and write a matrix-level `BLOCKED_PROVIDER` report.
- Keep per-case detection as a fallback for outages that start mid-sweep.

### G2. Correct answers failed because eval assertions were too narrow

Affected cases:

- `read_combo_config_absent_present_mix`
- `read_combo_trace_current_code_boundary`

Observed issue:

- Config case answer correctly said
  `explore_per_tool_default_cap` is independent / different from the absent
  `explore_xyz_phantom_unique_budget`, but the regex only accepted narrower
  wording.
- Trace+source case correctly used current-source citations under
  `internal/types/...`, but the regex accepted only
  `internal/(analysis|tool|agent|orchestrator)/...`.

Root cause:

- The case assertions encode an implementation-path expectation and a small
  synonym list rather than the typed behavior the case actually wants.

Generic solution:

- Keep eval assertions behavior-based:
  - the absent key must be tied to an absence/no-value statement;
  - the present key must be tied to its example/source anchor;
  - the answer must explicitly keep the two keys separate;
  - trace line/time evidence must be separated from current-source evidence;
  - any current-source package should satisfy the source-lane citation check
    unless the case intentionally targets a package.
- Treat path/package expectations as separate optional diagnostics, not pass
  criteria, unless the user/case explicitly asks for that package.

### G3. Read pipeline stage answer collapsed requested stages

Affected case:

- `read_combo_pipeline_sequence_table`

Observed issue:

- User asked for read-mode timing from `analyze` to `finalizer`, Mermaid
  `sequenceDiagram`, and a table listing every stage's input/output/carriers.
- Final answer only rendered `analyze` and `finalize`.
- It omitted `explore` and `extract`.

Log/code evidence:

- The finalizer log shows the model itself believed "read mode pipeline has 2
  stages: analyze and finalize".
- Existing last-mile supplement
  `renderVerifiedStageBindingSupplement` in
  `internal/agent/answer_document_evaluator.go` can append a verified
  `StageAnalyze` / `StageExplore` / `StageExtract` / `StageFinalize` table.
- That supplement currently only fires when `internal/types/stage_binding.go`
  was cited or landed as evidence (`answerDocumentHasStageBindingSource`).
- In this eval, exploration did not land `stage_binding.go`; it cited
  orchestrator/context/AnalysisIR/AnswerDocument evidence instead.

Root cause:

- Requested presentation completeness ("every stage") is not represented as a
  typed stage-binding contract.
- The deterministic stage-binding supplement is source-evidence gated, so it
  cannot recover when the model follows nearby orchestrator prose and compresses
  conceptual phases.

Generic solution:

- Add a typed presentation-completeness contract for structural stage/workflow
  enumerations:
  - analyzer emits requested visible dimensions / role bindings for stage
    sequence when the current request asks for a stage/workflow table;
  - explorer may satisfy it with actual stage-binding source evidence, but the
    finalizer must preserve all requested structural members already present in
    typed contracts / verified constants;
  - the last-mile supplement may use deterministic stage-binding constants only
    when a precise typed stage/workflow requirement is present.
- Do not infer this from arbitrary prose in the finalizer. The hard signal must
  be analyzer/request-model derived, not a regex over the answer text.

### G4. CLI operation cases fell through into repository analysis

Affected cases:

- `operation_system_inventory`
- `operation_web_manual_summary`

Observed issue:

- Both cases were marked PASS by weak regex, but manual inspection showed bad
  answers.
- `operation_system_inventory` entered normal read pipeline, cited code such as
  `internal/memlimit/memlimit_darwin.go`, and said it could not query the real
  system.
- `operation_web_manual_summary` did not fetch `http://codrax.net/`; it searched
  local repo files and summarized local docs.

Code evidence:

- Single-shot CLI resolves `--request` and calls
  `runSingleShot`, which directly executes `app.orch.Run(...)`.
- REPL has typed operation dispatch in `internal/repl/repl.go`:
  `RouteOperation` -> `operationDispatch` -> command/provider operation plan /
  execution / final operation answer.
- The command operation planner and execution loop live behind REPL methods
  (`operationDispatch`, `executeCommandOperationPlan`,
  `maybeDispatchCommandOperationFollowup`).

Root cause:

- Operation is implemented as a REPL route, not as a shared operation
  controller.
- CLI single-shot has no equivalent typed operation dispatcher before the read
  pipeline.
- Eval can therefore pass operation words through a source-analysis pipeline
  and produce plausible but wrong answers.

Generic solution:

- Extract a reusable `operation.Controller` / `operation.Runner` boundary that
  both REPL and CLI can call.
- Shared flow:
  1. typed turn policy identifies `RouteOperation` or operation-required route;
  2. shared controller plans, lints, applies policy, executes or blocks;
  3. shared evaluator decides continue / complete / clarify / blocked;
  4. shared answerer writes the final operation report;
  5. REPL and CLI differ only in rendering and approval UX.
- Do not route by keywords. The single-shot CLI must use the same typed
  classifier output that REPL uses.
- Preserve read-mode default behavior when the typed route is repo/log/trace or
  mixed source analysis.

### G5. MCP external observation answers still experience current-source citation pressure

Affected case:

- `mcp_typed_line`

Observed issue:

- Final answer was factually correct.
- The run read the same MCP resource more than once and then read three
  current-source files to satisfy citation pressure.
- The answer carried system supplement/caveat noise even though the user only
  asked what MCP line 7 and line 12 mean.

Log/code evidence:

- `emit_evidence` correctly kept MCP URI rows out of current-source evidence:
  external observations are not source citations.
- Observation Ledger renders MCP records in
  `renderAnswerDocObservationLedger`.
- The answer contract / closure repair path still led the model to read current
  source after a completion attempt because the current-source citation floor
  was not considered satisfied by MCP row references.
- Runtime artifact has an explicit citation-floor carve-out via
  `RequestModel.HasRuntimeArtifactWithoutRequiredCurrentSource`, but MCP-only
  observation requests do not have the same complete hard-decision path.

Root cause:

- Origin-specific evidence lanes are visible to the finalizer, but the hard
  contract still treats cite-eligible current-source anchors as the main way to
  satisfy coverage.
- MCP row/line references need an origin-specific citation-equivalent path for
  MCP-only questions.

Generic solution:

- Extend the existing Observation Ledger / AnswerEvidenceOrigin mechanism into
  an explicit `external_observation_coverage` contract:
  - current-source citations satisfy current-source claims;
  - runtime/log/trace row refs satisfy runtime artifact claims;
  - MCP resource URI + line/span refs satisfy MCP claims;
  - connector/web/document refs satisfy their own origin claims.
- Hard citation floors should be waived or replaced only when the typed request
  and accepted observations prove a non-current-source lane is sufficient.
- Mixed external+source questions must keep current-source requirements when
  `CurrentSourceLaneDecision` is `required`.

## Delivery Batches

### Batch 0: Eval assertion repair and documentation

- Broaden the two brittle case regexes without changing product behavior.
- Keep path/package expectations out of pass criteria unless case scope requires
  them.
- Record root-cause notes and downstream tasks.

Validation:

- `bash eval/runner_lib_test.sh`
- rerun the two repaired cases when provider is available.

### Batch 1: Provider preflight for eval sweeps

- Add a sweep-level provider health helper.
- Integrate it into broad parallel sweep scripts with an opt-out env flag.
- Emit matrix-level blocked summaries instead of spawning every case when
  deterministic provider blockers are detected.

Validation:

- runner-lib unit tests for 402/config/auth/model-list parse/no-contamination.
- dry-run fixture with a fake blocked log.

### Batch 2: Shared operation controller for CLI and REPL

- Extract the command/provider operation loop out of REPL-only methods into a
  reusable package boundary.
- REPL keeps rendering, spinner, `/approve`, pending state, and history UX.
- CLI gets the same typed operation route before `orch.Run(...)`.
- CLI operation output must be a final operation report, not source-analysis
  fallback.

Validation:

- unit tests for typed route -> operation controller and typed route -> normal
  read pipeline.
- eval cases:
  - `operation_system_inventory`
  - `operation_web_manual_summary`
  - source-only architecture case must still use read pipeline.

### Batch 3: Origin-specific external-observation coverage

- Add a typed coverage result that can satisfy final answer evidence for MCP
  rows without current-source reads.
- Keep mixed source+MCP requests in mixed mode.
- Reduce repeated MCP reads by preserving accepted MCP facts through Turn A /
  Observation Ledger handoff.

Validation:

- MCP-only typed-line test: no current-source forced reads.
- MCP+source mixed test: source reads still allowed/required when typed profile
  asks for source explanation.

### Batch 4: Structural stage/workflow presentation completeness

- Add or reuse typed request fields for requested stage/workflow tables.
- Teach explorer/finalizer to preserve all typed structural members.
- Allow deterministic stage-binding supplement only under precise typed
  stage/workflow request signal.

Validation:

- `read_combo_pipeline_sequence_table` includes analyze/explore/extract/finalize.
- generic architecture questions are not cluttered with stage tables unless
  requested.

### Batch 5: Representative eval quality guard

- Update representative eval runner/report to separate:
  - automated PASS/FAIL;
  - provider-blocked;
  - manual-review quality flags.
- Add hidden-failure checks for operation cases, e.g. requiring operation
  execution telemetry for operation evals rather than matching only answer
  words.

Validation:

- representative matrix run with manual inspection log updated.


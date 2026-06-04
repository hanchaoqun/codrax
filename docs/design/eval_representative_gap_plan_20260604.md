# Representative Eval Gap Root Cause and Delivery Plan

Date: 2026-06-04

Source run:

- `docs/design/eval_representative_run_20260604_152306.md`
- `eval/results/representative-p2-20260604-154448`

Additional randomized representative sweep:

- Ran in batches of two on 2026-06-04 with manual answer review.
- Covered code analysis, runtime log/trace observations, mixed
  external+source analysis, multi-repo, operation, and write-mode cases.
- Sampled cases:
  - `qf_config_precedence`
  - `mr_cross_repo_compare`
  - `logtri_java`
  - `trace_query_donghu_mixed_platform`
  - `read_combo_log_current_code_dimensions`
  - `read_combo_trace_current_code_boundary`
  - `operation_web_manual_summary`
  - `patch_python_typo`
  - `mcp_typed_line`
  - `patch_go_typo`
  - `qf_architecture`
  - `trace_query_frame_timeline_flow`

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

### G6. Multi-repo focus selection needs precise pre-scan before model recommendation

Affected case:

- `mr_focus_single`

Observed issue:

- A representative run previously selected the wrong sub-repo for a precise
  symbol/constant lookup and missed the actual value `42`.
- Rerun after the focus pre-scan fix passed:
  `eval/results/mr_focus_single-20260604-163613`.

Code/log evidence:

- The passing run logged:
  `multi-repo: exact focus pre-scan selected 1 sub-repo(s): repo-stub-rust`.
- The final answer cited `repo-stub-rust/src/lib.rs:8` and returned `42`.

Root cause:

- Model-only focus choice from repository names and topology is a noisy
  heuristic. It is fine for soft guidance, but too weak to decide hard active
  sub-repo selection for precise entity queries.

Generic solution:

- Keep the multi-repo focus pipeline typed:
  - user-explicit focus remains authoritative;
  - no-focus requests first run a bounded deterministic exact pre-scan for
    precise request entities / symbols / paths across topology members;
  - only accept exact-pre-scan focus when the hit set is non-empty and within
    `multi_repo_max_active`;
  - otherwise use the model focus selector, and only then fallback preview.
- Do not infer focus from arbitrary user prose by keyword matching. The exact
  pre-scan is based on structured request entities and file-system hits, not a
  hard route from noisy natural language.

### G7. Operation material retrieval can look green while content coverage is partial

Affected case:

- `operation_web_manual_summary`

Observed issue:

- Earlier run `eval/results/operation_web_manual_summary-20260604-163933`
  reached the command-round budget and the final answer still claimed the task
  was complete.
- Follow-up run
  `eval/results/operation_web_manual_summary-20260604-164426` correctly used the
  operation lane, fetched the target pages, and preserved command output in
  payload refs; manual inspection still showed weak HTML text extraction and a
  partial answer.

Root cause:

- Terminal operation state was not a first-class final-answer constraint, so a
  model could present a budget-exhausted / failed / partial command sequence as
  a completed user goal.
- Long material extraction remains too planner-dependent. The operation loop
  knows about payload refs and large-output truncation, but it does not yet have
  a deterministic material coverage/evaluation layer that can prove the relevant
  content was actually read/extracted before answer synthesis.

Generic solution:

- Add `terminal_operation_state` to the command-operation answer prompt and
  deterministically prefix non-success final reports with typed status
  (`failed`, `budget_exhausted`, `partial_answer_possible`, etc.).
- Add a conservative material-coverage caveat when saved payload refs remain
  unconsumed by later operation steps, so a final answer cannot quietly present
  an uninspected full payload as complete evidence.
- Include a bounded material excerpt for saved payload refs in the operation
  answer prompt. This is a generic text-first fallback:
  - plain text payloads are compacted and bounded;
  - HTML payloads use a small adapter that strips script/style/tag noise and
    exposes readable text;
  - binary-ish payloads are skipped with kind/caveat rather than forced into
    context.
  The goal is to make real material available to synthesis without dumping
  whole artifacts or fitting one website.
- Add eval hidden log assertions for operation cases so PASS requires the typed
  operation subsystem to run, not merely answer text that looks plausible.
- Future architecture work: introduce a material coverage layer for large
  payloads/files/web pages/provider artifacts:
  - identify saved payloads and content types;
  - extract/search/page relevant sections with bounded commands or provider
    actions;
  - track coverage against the user's requested content;
  - only allow `complete` when the coverage evaluator says the requested
    material has been inspected.
  This should be generic across files, web pages, manuals, logs, traces, MCP
  results, operation skill artifacts, and non-text adapters such as PDF,
  spreadsheet, slide, image/OCR, or archive payloads.

### G8. Operation command fields can contain structured JSON after model JSON drift

Affected case:

- `operation_web_manual_summary`

Observed issue:

- In `eval/results/operation_web_manual_summary-20260604-164918`, one
  continuation plan produced a command whose `shell` field was a JSON array of
  step objects. The executor attempted to run it and failed with
  `sh: [{id:: command not found` before repair recovered.

Root cause:

- The flexible command-operation decoder tolerates many LLM JSON slips, but
  the command-level lint did not reject a `program` or `shell` string that is
  itself structured JSON.
- This is not a natural-language intent problem; it is a typed schema-shape
  problem inside the operation lane.

Generic solution:

- Extend command-plan lint and execute-time validation to reject structured JSON
  objects/arrays inside executable command fields before any process is spawned.
- Feed the typed `invalid_plan` failure back to the existing repair loop with a
  precise instruction to move JSON members into typed step fields.
- Keep this scoped to operation command plans. It must not affect source
  analysis, trace/log analysis, write mode, or eval assertion matching.

### G9. Mixed log+current-source questions can lose the current-source lane

Observed in sweep:

- `eval/results/read_combo_log_current_code_dimensions-20260604-174648`
- Automated verdict: `FAIL`
- Manual finding: the final answer explained the log-only facts, but did not
  cite current source code even though the user explicitly requested "当前关键代码".

Root cause:

- The request contained two separate constraints:
  - do not treat attached log line numbers as current-source citations;
  - also explain the current key code.
- The analyzer/model collapsed the first constraint into
  `external_observation_policy.current_source_mode=exclude`, and the explorer
  then followed the runtime-artifact-only lane instead of reading current code.
- This is not a keyword problem. It is a typed-policy conflict problem:
  "do not re-anchor external observation lines to source" is different from
  "do not inspect current source".

Code evidence:

- `internal/tool/emit_analysis.go` exposes only
  `external_observation_policy.current_source_mode` for this surface. Its schema
  says `exclude` suppresses current-source exploration, and
  `parseExternalObservationPolicy` returns only `CurrentSourceMode`,
  `SourceQuotes`, `Confidence`, and `Rationale`.
- `internal/skill/analysis_contract.go` correctly teaches that external
  observations default to mixed external + current-source analysis, but there is
  no separate field for "external artifact line numbers are not current-source
  citations".
- The current type in `internal/types/external_observation_source_policy.go`
  therefore cannot express the observed request without overloading
  `current_source_mode`.

Generic solution:

- Split the external-observation policy into two orthogonal typed fields:
  - `artifact_citation_policy`: whether external artifact line numbers may be
    rendered as source citations;
  - `current_source_lane`: required / optional / excluded.
- A user instruction like "不要把日志行当成当前源码引用" should set only the
  artifact-citation policy. It should not exclude current source when
  `current_source_explanation_profile.Active()` or requested answer dimensions
  include current code.
- Hard gates should consume these two fields separately:
  - artifact line refs satisfy external-observation claims;
  - current-source claims still require actual current-source evidence.
- Add regression cases for:
  - external log only, no current-source request;
  - external log + current-source explanation;
  - external log + explicit "不要分析源码".

### G10. Explicit MCP fixture requests can be misrouted into command operation

Observed in sweep:

- `eval/results/mcp_typed_line-20260604-175428`
- Automated verdict: `FAIL`
- Manual finding: the CLI turn policy logged `raw_route=repo route=operation`
  and the command-operation planner returned `needs_clarification` because it
  could not find a shell command for the MCP fixture. No MCP tool call happened.

Root cause:

- The CLI operation route now correctly catches computer-operation requests,
  but it also caught an explicit external-tool/MCP observation request.
- `needs_operation=true` was treated as enough to enter command operation even
  when the request named an MCP resource/tool surface.
- MCP/external observation is a data lane, not necessarily a computer-operation
  lane. The default "external observation + source if useful" policy should not
  be preempted by command execution unless the typed route explicitly asks for
  computer operation / artifact generation / shell execution.

Code evidence:

- `internal/repl/turn_policy.go` already teaches the right distinction:
  `needs_operation_access` must not be set for ordinary source/log/trace/MCP
  external-observation investigation, and the examples route MCP observation to
  `repo` with `source=external_tool`.
- The normalizer has a typed repair path in `isAnalysisOnlyPolicy`, but a
  contradictory classifier output can still survive if it carries an operation
  signal outside the analysis-only tuple.
- `cmd/root.go::maybeRunSingleShotOperation` starts the CLI operation pipeline
  whenever the post-guard policy is `route=operation` and
  `needs_operation_access=true`. It does not additionally verify that the
  operation kind/surface is a concrete command/browser/artifact/skill surface.
- `internal/repl/repl.go` also documents that REPL `MCPServers` are only for
  explicitly configured operation providers; explorer/read-mode MCP exposure
  lives in the agent layer. This reinforces that MCP observation and MCP-backed
  operation provider are separate lanes.

Generic solution:

- Add typed route precedence:
  - `external_tool_observation` / MCP resources first enter the MCP/external
    observation pipeline;
  - command operation is selected only when the typed operation surface is
    `command_line`, `browser`, `desktop`, `artifact_generation`, or a configured
    operation skill provider.
- Keep mixed MCP+source questions in mixed mode unless the typed user policy
  excludes source.
- Add a hidden eval assertion for MCP cases requiring at least one MCP call or
  accepted MCP observation, so they cannot pass or fail through command
  clarification.

### G11. Trace questions can pass without exercising trace_query, and small-trace answers can overclaim

Observed in sweep:

- `trace_query_donghu_mixed_platform-20260604-174358` passed, but
  `tool_trace_query=0`; the answer relied on perf-triage plus tool/source
  descriptions.
- The answer classified `idle` rows with `prev_prio=120` as RT high-priority,
  which is a risky overclaim because `prev_prio` / `next_prio` are event-role
  fields, not a generic process-priority label.
- `trace_query_frame_timeline_flow-20260604-175536` correctly used
  `trace_query` twice and produced a good frame-flow answer.

Root cause:

- For small attached traces, the model can answer from perf-triage/read_file
  without invoking `trace_query`, even when a case is meant to validate the
  deterministic trace engine.
- Trace facts are not always rendered as role-qualified facts. A generic
  sentence may turn `prev_prio` on an idle scheduling row into a process
  priority claim.

Generic solution:

- Case design: when the product capability under test is `trace_query`, add
  hidden log assertions requiring `tool_trace_query>0` or a trace-query
  observation, while allowing small traces to use read_file in ordinary user
  scenarios.
- Product output: trace_query should emit role-qualified priority facts such as
  `prev_prio=.../role=prev_task` and `next_prio=.../role=next_task`, and final
  prompts should teach that priority classification applies to the named task
  role, not to arbitrary labels like idle without that role.
- Keep time and platform semantics in deterministic trace metadata, not in
  prose-only summaries.

### G12. Operation material retrieval still needs a coverage evaluator

Observed in sweep:

- `operation_web_manual_summary-20260604-175216` passed, but manual review
  showed the answer still reported partial completion and failed to locate the
  dedicated user-guide page in that run.
- The bounded material excerpt improved synthesis when content was available,
  but the system still depends on the planner to discover and consume relevant
  material paths.

Root cause:

- The operation loop has payload refs and bounded excerpts, but it does not yet
  maintain a typed coverage model over the user's requested materials.
- "The answer includes useful content" and "the requested material was actually
  found/read" are different checks.

Generic solution:

- Implement the Batch 7 material coverage evaluator:
  - track requested material targets, discovered links/files/resources,
    fetched payloads, extraction attempts, and covered sections;
  - return `complete` only when requested targets are covered or a typed
    impossibility reason is established;
  - support text, HTML, logs, traces, MCP/provider payloads, operation skill
    artifacts, and future binary adapters through a common material interface.

### G13. Finalizer/tool-schema ergonomics still create recoverable noise

Observed in sweep:

- `read_combo_trace_current_code_boundary-20260604-174648` passed, but the
  first `emit_answer_document` call failed with truncated JSON and had to be
  re-emitted.
- The same run hit a mismatch between documented artifact citation examples
  (`trace:5-6`) and what `emit_hypothesis_verdict` validation accepted.
- `trace_query_frame_timeline_flow-20260604-175536` tried to call
  `emit_evidence` in a tool set where it was unavailable, then recovered.

Root cause:

- Some tool descriptions and stage-specific tool availability still drift from
  the actual schema/validator.
- Large answer-document payloads make JSON truncation likely even when the
  semantic answer is straightforward.

Generic solution:

- Add schema-aware prompt generation for each stage/tool set: examples must be
  generated from the same validators the backend uses.
- Add a compact-answer-document retry hint that points to accepted evidence
  references instead of serializing large row sets.
- Add eval hidden metrics for unavailable tool attempts and truncated JSON
  retries so PASS cases with noisy recovery stay visible.

### G14. Write-mode eval metrics can overcount changes

Observed in sweep:

- `patch_python_typo-20260604-175216` and
  `patch_go_typo-20260604-175428` both produced correct one-file, one-line
  patch plans.
- The summary table reported `changes=2` and `kinds=patch,bugfix`, even though
  the actual `ChangePlan.changes` array contained one patch. The second kind is
  from `write_analysis_ir.request.task.kind`.

Root cause:

- Eval summary aggregation appears to combine top-level ChangePlan changes with
  write-analysis task metadata.

Generic solution:

- Separate eval artifacts:
  - `change_plan_changes` / `change_plan_kinds` from `changes[]`;
  - `write_task_kind` from `write_analysis_ir.request.task.kind`.
- Keep current pass/fail logic based on post-apply file checks and plan JSON,
  but fix the summary table so manual reviewers are not misled.

### G15. Config/scalar subtopic gates can false-reject file-backed dimensions

Observed in sweep:

- `qf_config_precedence-20260604-174017` passed but took 22 explorer
  iterations and initially failed analyzer quality:
  `subtopic_coherence` rejected the default-value subtopic because
  `codrax.yaml.example` did not resolve as a repo symbol.

Root cause:

- The subtopic resolver treats an unresolvable file/config/example anchor like
  a hallucinated code symbol.
- Config/scalar questions often have dimensions that are backed by files,
  constants, or values rather than resolver-visible symbols.

Generic solution:

- Extend subtopic coherence with typed anchor kinds:
  - code symbols continue to require resolver symmetry;
  - file-backed config examples, scalar defaults, and CLI flags should be
    checked by path/key/flag existence instead of symbol resolution;
  - if a subtopic is a scalar dimension with no independent symbol, merge it
    softly into the parent topic instead of hard rejecting the analysis call.

### G16. Verified supplements and caveats can add answer noise

Observed in sweep:

- `mr_cross_repo_compare-20260604-174017` answered correctly, then appended
  deterministic "系统按已验证证据补充缺失成员" tables and a caveat saying some
  anchors were not fully verified.
- `qf_architecture-20260604-175536` answered correctly and also appended a
  deterministic stage-binding supplement.

Root cause:

- Deterministic supplements are useful for correcting missing members, but they
  are not ranked against whether the model already answered the requested
  shape well.
- The caveat is conservative but can read like distrust even when the answer is
  fully grounded enough for the user's question.

Generic solution:

- Make supplement rendering conditional on a typed completeness gap:
  - append full supplement only when a requested member/dimension is missing or
    the answer is structurally incomplete;
  - otherwise keep deterministic verification as hidden support or a compact
    footnote.
- Separate "anchor failed to publish as citation" from "fact not verified" in
  user-facing caveats.

## Delivery Batches

### Batch 0: Eval assertion repair and documentation

- Broaden the two brittle case regexes without changing product behavior.
- Keep path/package expectations out of pass criteria unless case scope requires
  them.
- Record root-cause notes and downstream tasks.

Status:

- Implemented and pushed in `bcb4f092 relax representative eval assertions`.

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

Status:

- Implemented and pushed in
  `c0b0cac5 preflight provider availability in eval sweeps`.

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

Status:

- CLI single-shot now uses the typed operation route and shared command-operation
  runner before entering the read pipeline.
- Implemented and pushed in
  `194b3684 route CLI operation requests through operation runner`.

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

Status:

- Implemented via typed origin-specific completion coverage. The
  pre-complete forced-read / anchor-backbone / citation-floor gates now accept
  accepted `aggregate_facts` whose evidence origins are non-current-source
  origin-specific lanes.
- `RequiresCurrentSourceForExternalObservation` keeps explicit current-source
  profiles and source-oriented contracts blocking, so MCP+source and
  runtime+source mixed questions remain protected.
- Explorer handoff marks accepted origin-specific external-observation
  completion so downstream criterion checks can count MCP typed rows without
  converting them into current-source evidence.
- Implemented and pushed in
  `53a590d9 honor external observation completion coverage`.

### Batch 4: Structural stage/workflow presentation completeness

- Add or reuse typed request fields for requested stage/workflow tables.
- Teach explorer/finalizer to preserve all typed structural members.
- Allow deterministic stage-binding supplement only under precise typed
  stage/workflow request signal.

Validation:

- `read_combo_pipeline_sequence_table` includes analyze/explore/extract/finalize.
- generic architecture questions are not cluttered with stage tables unless
  requested.

Status:

- Implemented via typed `stage_workflow` answer dimension and deterministic
  stage-binding supplement under precise typed request signal.
- Implemented and pushed in
  `dfd47fbc preserve stage workflow answer dimensions`.

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

Status:

- `eval/run.sh` now supports `EXPECT_LOG_MATCHES_REGEX` and
  `EXPECT_LOG_NOT_MATCHES_REGEX` over per-run control-plane logs.
- Operation representative cases require command-operation planning/execution
  telemetry so they cannot pass by accidentally falling into source analysis.

### Batch 6: Operation terminal-state preservation

- Add typed terminal-state constraints to command-operation final answer
  synthesis.
- Deterministically prefix non-success final operation reports with their typed
  status so the model cannot present budget-exhausted / partial / failed runs as
  fully complete.

Validation:

- Unit tests for CLI operation final answer status preservation.
- Unit tests that the answer prompt includes `terminal_operation_state`.
- Re-run `operation_system_inventory` and `operation_web_manual_summary` with
  hidden operation-log assertions.

Status:

- Implemented in this batch, including a conservative material-coverage caveat
  for unconsumed payload refs and bounded generic text material excerpts in the
  operation answer prompt. HTML handling is only a content adapter on top of the
  text fallback, not a website-specific path.

### Batch 7: Material coverage evaluator (follow-up architecture)

- Design and implement a generic material coverage layer for long files, saved
  command payloads, web pages, manuals, logs/traces, MCP/provider payloads, and
  external Skill artifacts.
- It should not be a website-specific extractor. It should track payload refs,
  content type, extraction attempts, matched sections, and coverage against the
  typed user goal.
- Operation evaluator should use the material coverage result before returning
  `complete`.

Validation:

- Web/manual operation case must not report full completion from metadata-only
  extraction.
- Large local file extraction, MCP payload extraction, and operation Skill
  artifact extraction cases should share the same evaluator.

Status:

- Recorded as a product gap from manual eval inspection. The current batch adds
  bounded text excerpts, an HTML text adapter, and conservative caveats; the
  full coverage evaluator remains follow-up architecture work.

### Batch 8: Structured command-field lint

- Reject JSON objects/arrays in `program` or `shell` command fields at plan-lint
  time.
- Keep execute-time validation as a defensive fallback.
- Teach the operation replan prompt how to repair the typed `invalid_plan`
  failure.

Validation:

- Unit tests for command lint on JSON-in-command fields.
- Existing command-operation prompt tests cover the replan hint.

Status:

- Implemented in this batch.

### Batch 9: Operation command-round budget configuration (follow-up)

- Audit whether the default command-operation round budgets are sufficient for
  real multi-step operation tasks.
- Add YAML configuration if needed for command rounds and repair rounds, with
  sane defaults and hard clamps.
- Update `codrax.yaml.example`, user guide Markdown/HTML, and tests.
- Keep budgets scoped to the operation lane; do not affect read-mode source
  analysis, trace/log analysis, or write mode.

Validation:

- Unit tests for default values, YAML overrides, and clamping.
- Operation eval cases that require multiple discovery/extraction rounds.

Status:

- Recorded from 2026-06-04 eval follow-up. Not implemented in this batch.

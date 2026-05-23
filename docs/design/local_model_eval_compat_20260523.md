# Local Small-Model Eval Compatibility Notes - 2026-05-23

## Scope

This document tracks real eval observations from the local provider in `providers.yaml`
and records system-side fallback ideas that do not require changing prompt code.

Current local provider observed in `providers.yaml`:

- `base_url`: `http://127.0.0.1:8000/v1`
- `model`: `Qwen3.5-9B-OptiQ-4bit`
- `context_window`: `200000`
- `recover_text_tool_calls`: `true`
- `tool_param_compat.mode`: `repair`
- `tool_param_compat.split_string_arrays`: `true`

Eval run root:

- `eval/results/local-small-20260523`

## Implementation Status

- Batch 1 landed in `b43154dc`: analyze-stage disallowed deep-read calls now
  capture safe routing intent, append safe paths/patterns to the prescan
  summary, and force the next analyzer turn to `emit_analysis` without reading
  source content in the analyze stage.
- Batch 2 landed in `c9d1fe22`: explorer evidence-backlog and evidence-repair states now
  have schema-level action-space gating, so repeated ignored hints expose only
  structured materialization tools instead of relying on prose nudges. The
  repair hint now allows a stale or wrong row to be omitted/replaced when the
  just-read source window proves it is not grounded.
- Batch 3 landed in `029e661c`: bounded trace / call-chain requests now suppress
  stale enumeration file-coverage floors and broad ranker-coverage pushbacks.
  The legacy overview-window check no longer parses assistant prose keywords;
  it is driven by typed RequestModel / QuestionFamily signals only.
- Batch 4 landed in `4abba21a`: evidence ranking now has a baseline structural ordering
  even when no question entities are extracted, so grounded/read/citable
  definition and call anchors are not left behind broad concrete-value noise by
  input order alone.
- Batch 5 started from the real `qf_sequence_analyzer_gate` eval: analyzer
  schema filtering alone was not enough because a local model can still emit a
  handwritten prescan tool call after the runtime has moved the stage into
  terminal `emit_analysis` mode. The runtime execution boundary now shares the
  same terminal emit-only predicate, so hidden or text-recovered calls cannot
  bypass the analyzer/explorer contract.
- Batch 6 extends the same schema/runtime parity to explorer repair states:
  read-without-emit escalation now rejects hidden navigation calls at execution
  time, while evidence-repair keeps `read_file` available because the structured
  hint asks for exact re-reads before re-emitting grounded evidence. The shared
  tool-parameter compatibility layer also handles explicit `_str` field-name
  variants such as `offset_str` / `limit_str` before schema scalar repair.
- Batch 7 fixes a typed-contract leak in bounded trace requests: an analyzer may
  split one trace request into multiple operational sub-topics ("call order" and
  "key intermediate functions") without turning the user request into an
  exhaustive file enumeration. `IntentTrace` + call/conditional/register
  requirement now remains in the bounded-trace lane unless an explicit typed
  member-set/count/relation/category obligation is present.

## Operating Principles

- Do not change prompt code for these compatibility fixes unless there is no safe
  runtime-side alternative.
- Prefer deterministic repair at tool boundaries, structured payload normalization,
  validator-side repair, and retry-loop policy changes.
- Keep repairs semantic-preserving: fix shape, types, citations, and provably
  equivalent representations; do not infer answer content from user/model prose.
- Keep big-model quality stable by applying repairs only when they are lossless,
  schema-directed, or validated by already accepted structured evidence.
- User intent is the highest-priority contract. Runtime guards may preserve,
  route, annotate, or validate model output, but must not replace the user's
  requested answer shape or silently substitute a different system-preferred
  task.
- Prefer the LLM's structured suggestions when they are schema-valid and
  grounded. If the system adds caveats or repaired structure, the addition must
  be clearly system-originated and localized in the active UI language.
- Finalizer rewrite is the last resort. When a finalizer retry is necessary,
  trace the defect upstream first: analyzer classification, explorer evidence
  handoff, extractor slate, support-lane construction, and answer-document
  validation should carry enough structure that finalizer is not asked to solve
  preventable upstream omissions.

## Deep Root Causes

### RC1 - Analyze Blocks Deep Reads But Does Not Absorb The Intent

The analyze stage already enforces the correct hard boundary: tools such as
`read_file` are not available because analyze is classification-only. However,
the rejected call still carries useful information: the model is naming files it
believes are relevant. Today the system mostly returns a rejection and asks the
model to try again. That is safe, but it wastes rounds and context.

Systemic fix: make disallowed analyze-stage deep-read calls an intent sink.
Record only non-content metadata (`tool`, `path`, `pattern`, `query`) as
candidate routing hints, keep the content-read blocked, and move the next
request toward `emit_analysis`. This preserves the user's intent and the model's
suggested targets without letting analyze become investigation.

### RC2 - Explorer Backlog Hints Are Advisory Only

The explorer can detect "read several files but emitted no structured evidence"
and it already emits escalating hints. Real logs show small models can ignore
the hint for many rounds and keep calling navigation tools. This is not a prompt
problem; it is an action-space problem.

Systemic fix: once the runtime has observed repeated navigation after an
evidence-backlog hint, temporarily filter the tool schema to materialization
tools only (`emit_evidence`, and optionally `emit_investigation_complete` when
completion is valid). Release the filter after a successful evidence emit or
after the backlog is cleared. This is loop-state driven, not user-text or
model-prose keyword matching.

### RC3 - Evidence Repair Targets Lack A Stale/Invalidated Lifecycle

The evidence repair loop currently treats "recovered/ungrounded row at file:line"
as an active repair target until a successful replacement `emit_evidence` arrives.
Logs show a failure mode where the model re-reads the target window and correctly
finds that the requested anchor is not there, but the next hint still asks it to
repair the same exact location. The system unintentionally traps the model.

Systemic fix: repair targets need a lifecycle: `open`, `rechecked`,
`repaired`, `dropped`, `invalidated`. At minimum, the hint and action space must
allow "drop or replace the bad row with a newly grounded row" after re-check,
instead of demanding byte-for-byte repair at the stale location.

### RC4 - Bounded Path Questions Can Be Misrouted As Exhaustive Enumeration

The `qf_sequence_analyzer_gate` log asked for a sequence diagram from
`buildAnalysisIR` to `gate.Run` plus key intermediate functions. This is a
bounded path contract. The runtime nevertheless produced a broad enumeration
coverage hint ("read 0 of 23 discovered files"), which pulled exploration away
from the user's requested path.

Systemic fix: typed answer shape must distinguish `bounded_path` from
`member_set`. For trace / call-chain / sequence-style contracts, prioritize
endpoint definitions, call edges, and path hops; do not use exhaustive file
coverage percentage unless the typed RequestModel declares an exhaustive member
set or completeness obligation.

### RC5 - Principal Evidence Ordering Must Preserve Grounded, Scoped Anchors

For final answer quality, the principal evidence pool must prioritize facts that
are:

1. grounded / accepted,
2. from already-read current source or authoritative external artifact,
3. definition or direct call/edge anchors,
4. aligned with the user's typed scope / facet / answer contract,
5. rich enough to carry the model's useful summary downstream.

Bulk concrete values, nearby helper facts, search hits, tests, and broad
supporting context must not outrank these anchors. The rich summaries attached
to accepted evidence must remain available to downstream support lanes, output
documents, preview surfaces, and external-observation ledgers; truncation is
only acceptable for UI display, not for the backend handoff contract.

### RC6 - External Observations Need The Same Contract

The same preservation rules apply to non-source observations: git history/diff,
logs, traces, runtime artifacts, command outputs, cross-repo index facts,
external documents, web/MCP/connector observations, and browser previews. These
facts must flow through typed observation ledgers or aggregate facts instead of
being converted into fake source citations. Current source can verify or bound
them, but must not erase the original external observation unless it is proven
wrong.

### RC7 - Legacy Prose-Keyword Flow Control Violates The Intent Boundary

A horizontal scan found an older mid-loop guard that inferred a structural
overview intent by scanning assistant prose for phrases such as "overall
structure" / "整体结构". Even when useful, that class of logic is fragile and
violates the product rule that runtime flow control must not keyword-match user
or model text.

Systemic fix: replace this class with typed RequestModel / QuestionFamily /
tool-result metadata checks. The first cleanup is the explorer overview-window
guard: it now fires only when the analyzer's typed output says the answer needs
project or architecture overview coverage, and bounded trace/call-chain
requests are excluded.

### RC8 - Schema Filters Are Advisory Unless Runtime Enforces The Same Boundary

The `qf_sequence_analyzer_gate` eval exposed a deeper stage-policy issue. After
an analyze-stage `read_file` was correctly blocked, `FilterToolSchemas` exposed
only `emit_analysis` on the next model request. The local model nevertheless
emitted handwritten `grep` calls. Because the execution path still treated
prescan tools as generally allowed during analyze, those calls executed and the
analyzer started drifting back toward investigation.

Systemic fix: every schema-level action-space filter needs a matching
pre-execution runtime boundary. The runtime predicate is authoritative; schemas
are only the model-facing affordance. For analyzer terminal emit-only mode, all
tools except `emit_analysis` are rejected before normalization or dispatch, and
the rejection reports only `emit_analysis` as the available action.

The same rule applies horizontally to explorer. A read-without-emit escalation
that exposes only materialization tools must reject hidden `grep` / `read_file`
calls before dispatch. Evidence repair is different: its hint explicitly asks
the model to re-read exact source windows, so the correct restricted surface is
`read_file` plus structured completion tools, not completion tools only.

### RC9 - Schema-Compatible Field Name Variants Can Otherwise Drop Intent

Small models sometimes emit explicit string-typed field variants such as
`offset_str` / `limit_str`. If these are left as unknown JSON fields, Go's
strict struct decoder may either reject the call or silently ignore the model's
intended range, causing `read_file` to return the wrong slice and feeding bad
line-number state downstream.

Systemic fix: handle this in the existing schema-aware `toolparam` normalizer,
not in per-tool ad hoc code. A key ending in `_str` aliases to the canonical
schema field only when that exact base field exists and the alias is
unambiguous; the existing scalar repair then converts `"2310"` to `2310` if
the schema says the field is an integer. No missing fields are invented and no
prose is inspected.

### RC10 - Analyzer Sub-Topics Are Work Decomposition, Not User Intent Override

The local `qf_sequence_analyzer_gate` eval showed a bounded sequence-diagram
request classified correctly as `intent=trace`, `question_kind=call_chain`, and
`diagram_hint=sequence`. The analyzer also split the work into two sub-topics:
one for the call order and one for key intermediate functions. A downstream
single-topic trace helper treated `len(sub_topics)>1` as disqualifying and let
the broad enumeration coverage gate fire, telling explorer to read 23
discovered files. That changed the user's path-shaped request into a system
preferred inventory task.

Systemic fix: distinguish user intent from orchestration decomposition. A typed
trace/call-chain request remains bounded when it has no explicit member-set,
count, relation, or category-enumeration obligation, even if the analyzer split
the work into multiple operational sub-topics. The system may use those
sub-topics to schedule work, but must not use them to impose exhaustive
file-coverage semantics.

## Red-Line Design Boundaries

- No keyword matching against the user question or model prose for intent or
  flow control. Runtime decisions must consume typed RequestModel fields, tool
  schemas, tool-result metadata, loop state, validated evidence, or parsed
  source/artifact structure.
- No case-answer overfitting. Fixes must apply to tool/stage contracts, evidence
  lifecycle, support-lane ordering, and source/artifact typing.
- No silent system substitution of the user's answer shape. If a system caveat,
  fallback, or supplemental block is added, it must be visibly marked and
  localized.
- No prompt-first solution when a runtime/tool/validator contract can solve the
  issue safely.
- Language support must remain repo-map wide: Go, Python, JS/TS/TSX, Java,
  Kotlin, Rust, C/C++, Ruby, Swift, Lua, Proto, ArkTS, Cangjie, YAML/JSON/TOML,
  shell, and mixed-language repositories. Source-structure checks must use
  existing repomap/symbol/line-index helpers where possible instead of
  hand-written language-specific string logic.

## Implementation Tasks

### Batch 1 - Analyze Intent Sink And Terminal Emit Guard

- Add typed storage for analyze-stage blocked deep-read intent metadata.
- When analyze receives a disallowed content-reading tool call, record only
  non-content parameters and keep the tool blocked.
- Feed those candidate paths/patterns into the pre-scan corpus without spending
  a pre-scan round or reading source.
- After a blocked deep-read, expose only `emit_analysis` on the next analyzer
  request.
- Add tests for read_file/path capture, no content read, reset per dispatch, and
  big-model clean path unaffected.

### Batch 2 - Explorer Backlog Action-Space Gating

- Add `explorerEvaluator.FilterToolSchemas`.
- When evidence-backlog escalation or evidence-repair closure-only state is
  active, filter navigation tools out and expose only structured materialization
  tools.
- Release the filter after successful `emit_evidence` / completion progress.
- Update closure-only wording to allow dropping or replacing a bad row when a
  re-read proves the target anchor is stale.
- Add tests that repeated ignored hints remove navigation tools without changing
  normal explorer tool availability.

### Batch 3 - Bounded Path vs Exhaustive Enumeration

- Reuse typed helpers such as `IsSingleTopicStructuralTrace` to keep
  trace/call-chain/sequence contracts out of broad enumeration coverage hints.
- Remove legacy model-prose keyword flow control from explorer mid-loop checks;
  overview/window guards must consume typed RequestModel / QuestionFamily
  signals instead.
- Add tests for a trace/sequence request with key intermediate functions: no
  "read N discovered files" enumeration hint unless typed exhaustive-member
  obligations are present.
- Add eval cases covering bounded path, relation lookup, source inventory,
  external log/trace current-code boundary, git-history + current-code answer,
  and command-output observations.

### Batch 4 - Principal Evidence Contract Hardening

- Audit support-lane ordering so grounded, citable, scope-aligned definition /
  direct-call anchors outrank bulk concrete values, search hits, tests, and
  broad related context across all question families.
- Ensure rich evidence summaries are carried in backend lanes without destructive
  truncation; UI rendering may summarize, but downstream structured payloads must
  retain the original accepted summary.
- Add regression tests for current-source, git, log/trace, command output,
  cross-repo, external document, web/MCP/connector observation preservation.

### Batch 5 - Eval Coverage

- Add or extend evals for:
  - analyzer tries to deep-read during analyze,
  - explorer ignores evidence-backlog hint,
  - stale evidence repair target after re-read,
  - bounded path / sequence diagram,
  - member-set enumeration,
  - relation lookup,
  - git-history plus current-code verification,
  - log/trace/runtime artifact plus current-source boundary,
  - command-output-only answer,
  - cross-repo and multi-language source anchors.
- Track pass/fail plus cost metrics: iterations, midloop injections,
  compatibility repairs, finalizer rewrites, and recovered-but-extra-rounds.

### Batch 6 - Stage Policy Runtime Parity

- Audit every evaluator `FilterToolSchemas` implementation and ensure the same
  condition is enforced before tool execution.
- For analyzer terminal emit-only mode, reject handwritten / recovered prescan
  calls before parameter compatibility repair or actual tool dispatch.
- For explorer read-without-emit escalation, reject hidden navigation calls
  before dispatch; for evidence repair, keep `read_file` available and reject
  broad search/navigation tools.
- Ensure user-facing repair summaries show the current effective tool surface,
  not the broader stage default.
- Add regression tests for schema-filter bypass via handwritten tool calls.
- Extend shared tool-parameter normalization for unambiguous `_str` field-name
  variants so range/counter intent is not silently dropped.

### Batch 7 - Bounded Trace Contract Preservation

- Treat analyzer sub-topics as work decomposition unless a separate typed
  principal-member obligation is present.
- Keep `IntentTrace` + `call_chain` / `conditional` / `registration` requests
  out of broad enumeration coverage gates.
- Add regression coverage for sequence-diagram requests that ask for key
  intermediate functions but do not ask for an exhaustive member set.

## Progress

- 2026-05-23: Synced `main` to `d4547331`, built successfully, ran local small
  model eval samples, and captured root causes above.
- 2026-05-23: Batch 0 documentation prepared. Code implementation starts with
  Batch 1.
- 2026-05-23: Batch 1 implemented. Analyze-stage blocked deep-read calls now
  record safe non-content intent metadata, keep source content blocked, preserve
  safe path/pattern hints in the analysis corpus without spending a pre-scan
  round, and force the next analyzer request to `emit_analysis` only. Added
  regression tests for safe path capture, unsafe path exclusion, and clean
  emit-only schema behavior.
- 2026-05-23: Real `qf_sequence_analyzer_gate` rerun found a schema/runtime
  parity gap: after terminal emit-only schema filtering, handwritten `grep`
  still executed. The runtime analyzer boundary now rejects every non
  `emit_analysis` tool in the same state and reports only the effective allowed
  tool surface.
- 2026-05-23: The same eval exposed explorer parity gaps: read-without-emit
  escalation allowed hidden broad `grep`, and evidence-repair schema filtering
  contradicted its own exact re-read hint. Explorer now enforces restricted
  surfaces at runtime, and evidence repair exposes `read_file` plus emit tools.
  The shared normalizer also repairs `_str` schema-key variants before scalar
  conversion.
- 2026-05-23: The follow-up run showed the trace request still being pulled into
  broad discovered-file coverage because analyzer sub-topics disqualified the
  single-topic trace helper. The bounded-trace predicate now preserves
  `intent=trace` / `question_kind=call_chain` contracts unless a typed
  principal-member obligation explicitly says otherwise.

## Observations

### `qf_relation_subagent_registry`

Result: PASS.

Useful signals:

- `tool_param_compat` repaired analyzer `grep` calls by forcing
  `files_only=true` in analyze stage.
- Analyzer attempted `read_file` in analyze stage; the runtime rejected it and
  mid-loop guidance recovered the flow without prompt changes.
- Explorer emitted one `emit_evidence` row with `anchor_kind="call"` on a
  definition line. The evidence validator rejected only the bad row, gave a
  targeted repair location, and the model corrected it to `definition`.
- Extractor emitted `emit_hypothesis_verdict.items` as a JSON string; compatibility
  normalized it to a native array.
- Finalizer emitted a valid `emit_answer_document`. The answer-document runtime
  normalized principal enumeration blocks from the accepted evidence-rich row
  contract.

Costs / gaps:

- The run still used many mid-loop hints: `midloop_inject=7`.
- Explorer required 15 iterations for a one-member relation question.
- Analyzer's forbidden `read_file` attempt was recoverable, but it consumed a
  round and added noisy history.
- Evidence `anchor_kind` drift was recoverable, but required a re-read and re-emit.

Prompt-free fallback candidates:

- Add a typed auto-repair path for `emit_evidence.anchor_kind` when the cited line
  shape is unambiguous and the validator already knows the intended correction.
  Example: definition-shaped line + `anchor_kind="call"` can be normalized to
  `definition` only when the exact anchor symbol appears on the definition line
  and no call-site semantics are being asserted.
- Add a stage-tool-policy fast repair for analyze-stage `read_file`: when the
  model tries to read a file in analyze, convert the attempt into a lightweight
  allowed action when safe, such as `grep(files_only=true, pattern=<symbol/path>)`
  or a stage-local advisory result that does not append a full tool rejection to
  the conversation. This must be schema/tool-policy driven, not based on the
  question text.
- Track "recovered but extra-round" metrics separately from hard failures so eval
  can distinguish PASS-by-repair from clean PASS.

### `read_combo_answer_document_tools`

Result: PASS.

Metrics: `tool_read_file=27`, `midloop_inject=23`, `analyzer_iters=5`,
`explorer_iters=36`, `extractor_iters=1`, `finalizer_iters=1`.

Useful signals:

- Analyzer correctly identified the intent, mechanism shape, two sub-topics,
  diagram need, and the three likely files.
- Before `emit_analysis`, the local model attempted `read_file` in analyze.
  Runtime policy rejected the calls, so the stage boundary held.
- Finalizer emitted `blocks` as a JSON-encoded string instead of a native array.
  The flat-mode tolerance path re-parsed it successfully.
- The finalizer also produced a diagram; the answer-document runtime promoted a
  recovered diagram attachment into a formal diagram block and accepted the
  document on the first finalizer iteration.
- Explorer lanes duplicated reads of the same core files in parallel branches.
- Explorer sometimes left structured evidence behind even after reading enough
  source, then continued widening scope.
- Evidence repair got stuck around one stale/unrepairable target:
  `BuildToolSchemas @ internal/agent/answer_document_evaluator.go:6228`. The
  model correctly discovered that the symbol was not at that file/line, but the
  repair hint kept asking it to re-emit evidence for the stale location.

Costs / gaps:

- Analyze-stage deep-read attempts were safe but still cost turns and context.
- Evidence-repair closure can become counterproductive when the current repair
  target has already been proven wrong by a follow-up read/grep.
- DAG exploration can amplify local-model drift because separate lanes repeat
  similar reads and similar grounding mistakes.
- PASS required substantial runtime guidance: 23 mid-loop injections and 36
  explorer iterations for a bounded comparison question.

Prompt-free fallback candidates:

- Add an analyzer "intent sink" for disallowed read tools: preserve attempted
  paths as candidate files, compact the rejection, and move the model toward
  `emit_analysis` once classification fields are sufficient.
- Add a stale-repair-target escape hatch: when a repair target's file/line is
  re-read and the requested symbol is provably absent, drop that row from the
  active repair set or convert the instruction from "repair this exact row" to
  "drop or replace this row using a newly grounded anchor." This is based on
  source-window facts, not question content.
- Add cross-lane read sharing to reduce duplicate context and repeated mistakes.

### `qf_sequence_analyzer_gate`

Result: FAIL in this sampled run after manual stop (`read_exit:143`). The stop
was intentional once the repeated compatibility pattern was clear; the run had
not reached extractor/finalizer.

Metrics before stop: `tool_read_file=21`, `midloop_inject=25`,
`analyzer_iters=5`, `explorer_iters=38`, `extractor_iters=0`,
`finalizer_iters=0`, `explorer_dispatches=2`.

Useful signals:

- Analyzer again attempted `read_file` in analyze and later recovered to a valid
  `emit_analysis`.
- The analyzer correctly requested a `sequence` diagram and named the intended
  anchors: `analyzer.go`, `buildAnalysisIR`, and `gate.Run`.
- Exploration initially interpreted "列出关键中间函数" as a broad enumeration
  task and produced a hint to read 23 discovered files. The user actually asked
  for a bounded path between two endpoints, not an exhaustive member set.
- After repeated "read without emit" hints, the model still chose navigation
  tools (`grep`, `read_file`, `list_files`) until it finally emitted evidence.
- Evidence repair then got stuck on `Run @ internal/agent/analyzer.go:1047`.
  Re-reading that window showed the line was an analyzer must-emit hint string,
  not a `gate.Run` call. The active repair target still kept asking for the same
  stale location, so the model continued scanning nearby code.

Prompt-free fallback candidates:

- Distinguish bounded path enumeration from exhaustive set enumeration. A request
  for "关键中间函数" attached to explicit endpoints should create a path-members
  contract, not a "read N discovered files" enumeration hint.
- Add hard evidence-backlog gating after repeated ignored closure-only hints:
  temporarily remove navigation tools until one structured `emit_evidence` call
  succeeds or the backlog is explicitly cleared.
- Add stale repair-target invalidation: when a required repair location is
  re-read and the asserted anchor is absent, mark that target as
  `invalidated_by_source_window`. The next hint should allow "drop this row or
  replace it with a newly grounded anchor" instead of demanding repair at the
  same wrong location.
- Track a metric such as `repair_target_invalidated` so this class of stall is
  visible in eval dashboards.

## Candidate Fallback Backlog

### P0 - Lossless Runtime Normalization

- Continue expanding schema-directed JSON normalization for all emit tools:
  native arrays wrapped as strings, integer fields as strings, object fields as
  strings that parse successfully, and safe bracket/brace completion.
- Apply the same normalization layer before both tool execution and preview
  rendering so user-visible progress does not lose model-generated content.
- Keep an audit event for each normalization with field path and repair kind.

### P0 - Stage Tool Policy Recovery

- For stage-disallowed tools, return a structured lightweight repair result that
  names the allowed substitute class and avoids bloating the model history.
- Safe substitutions must be based on tool contract only. Example: analyze-stage
  `grep` missing `files_only=true` is already repaired; analyze-stage `read_file`
  should not read content, but can be rejected with a compact policy result or
  transformed into a file-discovery query only when the parameters contain a
  concrete path/pattern and the substitute remains within analyze permissions.

### P0 - Prevent Analyzer From Becoming Explorer

Observed in `read_combo_answer_document_tools`: the local model attempted
`read_file` in analyze after discovering likely files. The stage boundary rejected
the calls correctly, but the rejection consumed a model turn and added noisy
history before the analyzer eventually emitted `emit_analysis`.

Preferred runtime-side design:

- Keep the hard boundary: analyze never executes content-reading tools.
- Compact repeated disallowed tool calls in the same analyzer turn into one
  concise policy event instead of one full tool-result per attempted read.
- Treat blocked `read_file(path=...)` arguments as useful file-intent signals:
  record those paths as candidate `suggested_files` / pre-scan leads, but do not
  read them in analyze.
- After a blocked deep-read attempt, bias the loop controller toward immediate
  `emit_analysis` if the analyzer already has a valid intent, question kind,
  entities, and candidate file list. The downstream explorer will read the files.
- Count this as `analyzer_deep_read_blocked` telemetry so eval can measure
  "analysis-as-investigation" pressure without weakening the stage contract.

This avoids prompt churn and preserves big-model behavior: a capable model that
already emits `emit_analysis` is unaffected; a small model that tries to dig too
deep gets its file intent preserved but cannot drag analyze into exploration.

Implementation shape:

- Introduce a small analyzer-side state record, for example
  `BlockedDeepReadIntent`, populated only from stage-disallowed tool calls whose
  schema is known and whose parameters parse successfully.
- Store only non-content metadata: requested paths, patterns, and tool names.
  Never execute the deep-read tool in analyze.
- Feed those paths into the same downstream file-candidate channel that
  `emit_analysis.suggested_files` uses, with a provenance flag like
  `blocked_analyze_read`.
- If the current analyzer attempt has a valid analysis skeleton
  (`intent`, `question_kind`, entities/keywords or suggested files), make the
  next mid-loop hint a short "emit analysis now; exploration will read files"
  nudge and suppress repeated per-file rejection text.
- If the model repeats the same blocked deep-read fingerprint, break early to a
  degraded but valid `emit_analysis` draft instead of spending another full LLM
  round on the same mistake.

Why this is graceful:

- The user's question and the model's prose are not inspected for special cases.
- Big models that follow the schema never enter this path.
- Small models are not punished with noisy failures; their useful search intent
  is preserved and handed to the proper stage.
- The analyze/explore contract stays clean: analyze classifies and routes,
  explorer reads and proves.

### P1 - Evidence Shape Auto-Repair

- Add validator-side repair for unambiguous `anchor_kind` mismatches:
  definition line vs `call`, return statement vs `definition`, assignment line vs
  `call`.
- Repair only when the cited line text and anchor symbol make one canonical kind
  obvious. Otherwise keep the current rejection path.
- Preserve the current targeted repair guidance as the fallback.

Additional observed shapes from `read_combo_answer_document_tools`:

- The model cited doc-comment / type-definition areas with nearby semantic
  summaries. The grounder recovered to the concrete symbol definition line, but
  the repair still cost an extra turn.
- The model emitted line-scoped `mechanism` / `condition` rows without
  `anchor_symbol`; those rows were skipped because line-shaped scopes require an
  anchor symbol.

Safe repair candidates:

- For doc-comment lines immediately attached to a definition, allow a
  comment-to-definition normalization when the cited file/line is in the leading
  comment group and the next declaration's symbol matches the model's
  `anchor_symbol`. This preserves semantics and uses language parser structure
  rather than question-text heuristics.
- For `scope=line` rows missing `anchor_symbol`, do not invent a principal
  symbol from prose. If a parsed source line has exactly one visible code
  identifier or a deterministic enclosing function, normalize the row into a
  supporting evidence item anchored to that identifier/enclosing function and
  mark the repair in telemetry. If ambiguity exists, keep rejecting.
- Batch skipped-row diagnostics by cause so the model receives one compact
  correction rather than several long row-level errors.

### P1 - Retry Cost Control

- Add eval telemetry for "rounds spent on compatibility repair" by class:
  JSON shape, stage-tool policy, evidence grounding, answer-document validation.
- Use this to identify high-ROI runtime repairs without weakening validators.

### P1 - Evidence Backlog Tool Gating

Observed in `read_combo_answer_document_tools`: after reading enough source lines,
the explorer received repeated mid-loop hints to emit `emit_evidence`, but the
local model continued to call `read_file` / `grep` to widen scope. This is safe
but slow, and it grows context before the structured handoff exists.

Preferred runtime-side design:

- Keep the first hint as advisory.
- If the same dispatch ignores the evidence-backlog hint for N consecutive
  model turns while new navigation calls keep arriving, temporarily filter the
  tool schema to completion tools only: `emit_evidence` and, when allowed,
  `emit_investigation_complete`.
- Lift the filter immediately after a successful `emit_evidence` batch or after
  the backlog is otherwise cleared.
- Do not infer this from the user's question or answer content. Trigger only
  from loop-observer state: read files since last evidence emit, available
  grounded line windows, repeated navigation after backlog hint, and current
  stage.

This is analogous to the finalizer's schema-level switch to patch after repeated
full-document failures: it constrains available actions only after the model has
demonstrated it is ignoring a runtime state transition.

### P1 - Bounded Path vs Exhaustive Enumeration

Observed in `qf_sequence_analyzer_gate`: the request asked for a sequence from
one endpoint (`buildAnalysisIR`) to another endpoint (`gate.Run`) plus the key
intermediate functions. The exploration observer treated it like a broad
enumeration and pushed a "read 0 of 23 discovered files" hint.

Preferred runtime-side design:

- Let analyzer output carry a structural answer shape: `bounded_path`,
  `member_set`, `comparison`, `mechanism`, etc. This should come from existing
  RequestModel fields where possible, not from user-text keyword matching.
- For `bounded_path`, hints should prioritize endpoint windows, direct call-site
  searches, and already discovered path hops. They should not require broad file
  coverage percentages.
- Only use "read N discovered files" enumeration hints for true principal member
  sets where the answer contract needs exhaustive coverage.
- If the model asks for a diagram between explicit endpoints, the explorer can
  stop once it has grounded endpoint definitions, the call edge / transition
  evidence, and enough intermediate hops to satisfy the path contract.

This keeps small models from over-expanding and also improves big-model latency
because the system's runtime guidance better matches the answer contract.

### P1 - Stale Repair Target Invalidation

Observed in both `read_combo_answer_document_tools` and
`qf_sequence_analyzer_gate`: repair guidance can keep naming an old file/line
after the model has re-read that location and shown the asserted anchor is not
there.

Preferred runtime-side design:

- Every active repair target should have a lifecycle: `open`, `rechecked`,
  `repaired`, `dropped`, `invalidated`.
- If a source window for the target is read and the target anchor is absent,
  mark it `rechecked_absent`. If a follow-up search also shows the asserted
  anchor belongs elsewhere or is only mentioned in a comment/string, mark the
  original target `invalidated`.
- Invalidated targets must stop generating "repair this exact line" hints. The
  next hint should say: drop the row or replace it with a newly grounded row.
- Do not auto-create the replacement unless there is a single unambiguous parsed
  symbol/call line. Otherwise preserve validator strictness and ask the model to
  emit only grounded rows.

### P1 - Cross-Explorer Read Sharing

Observed in `read_combo_answer_document_tools`: DAG-scheduled exploration lanes
read the same core files (`emit_answer_document.go`,
`emit_answer_document_patch.go`, `finalizer.go`) in separate branches. This is
valid but inefficient for local models.

Preferred runtime-side design:

- Deduplicate identical `read_file(path, offset, limit)` calls across concurrent
  exploration lanes within the same run.
- Share the tool result bytes from the first completed call with later identical
  calls instead of hitting disk and appending duplicate long output.
- For overlapping ranges, prefer exact-result sharing first; range coalescing can
  be a later optimization because it has more citation-boundary risk.
- Preserve per-lane visibility: each lane can still receive the result as if it
  called the tool, but the log should mark it as shared/cache-hit.

### P1 - Test-File Evidence Demotion

Observed in `read_combo_answer_document_tools`: after production files had already
shown the mechanism, the local model read finalizer test files to understand the
switch-to-patch behavior. Tests are useful orientation, but they are derivative
and should not become principal proof for a production-code question.

Preferred runtime-side design:

- Allow test-file reads when the model asks for them, but mark their tool-result
  provenance as `auxiliary_test` unless the user explicitly asked about tests.
- Do not let `auxiliary_test` rows satisfy principal citation requirements when
  production evidence exists.
- If production source already covers the same symbol / mechanism, summarize the
  test read compactly or omit it from high-priority evidence lanes to save
  context.
- If a test-file evidence row resolves via package-symbol recovery to a
  production symbol, prefer the production anchor as repair target and demote the
  test row to illustrative context unless the user asked about test behavior.
- Keep this policy language-agnostic by using file-role classification
  (`*_test.go`, `test/`, `spec`, etc.) already available in repo inventory rather
  than matching the user question.

### P2 - Answer Surface Preservation

- When a rejected or intermediate model response contains diagram/table/prose that
  is not accepted as final structure, keep using existing preview/recovery lanes
  to surface it to the REPL and output artifacts where safe.
- Do not treat recovered preview content as validated evidence or citation-bearing
  answer content unless it passes the normal answer-document validator.

## Open Questions

- Whether `emit_evidence.anchor_kind` auto-repair should run inside
  `emit_evidence` itself or in a shared grounding repair layer. Shared grounding
  is preferable if existing helpers already classify definition/call/return lines.
- Whether analyze-stage `read_file` should be silently compacted into a policy
  notice or surfaced visibly as a recoverable model mistake. The current visible
  behavior is debuggable but costs context and iterations.

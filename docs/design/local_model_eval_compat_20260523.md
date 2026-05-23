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
- Batch 8 landed: the explorer "Closing Lane" treats completion-readiness
  as a fallible structured signal, not an oracle. The first readiness hint stays
  advisory. Only after the model ignores that hint long enough to enter the
  existing completion-ready escalation state does the runtime narrow the action
  space by blocking broad scope-expansion tools while still allowing exact
  `read_file` checks and structured emit tools.
- Batch 9 was scoped from the post-Batch-8 eval.
  Correctness held, but two typed-policy leaks still cost rounds: bounded trace
  can still receive broad enumeration / overview-window hints in at least one
  runtime path, and evidence repair still allows repeated `read_file` calls after
  the targeted source windows have already been read. Both fixes are state- and
  RequestModel-driven; neither may inspect user prose or model prose keywords.
- Batch 9 landed: broad enumeration mid-loop hints now require an explicit typed
  exhaustive obligation rather than a loose category fallback, trace/path-shaped
  requests suppress overview-wide-read hints through a typed guard, and evidence
  repair switches from `read_file + emit` to emit-only after target windows are
  covered or a bounded repair-read quota is exhausted.
- Batch 22 targets a deeper search-width root cause from
  `qf_relation_subagent_registry`: analyzer R1.4/R2.2 auto-corrections mutate
  `RequestModel`, but the already-compiled `TaskGraph`, `EvidencePlan`, and
  `AnswerContract` can stay stale. The fix is to recompile the deterministic IR
  artifacts after a structural auto-correction before the gate is re-run.

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

### RC11 - Completion-Ready Needs A Gradual Runtime Lane, Not A Hard Stop

The explorer already has a `postCompletionReadySignal` that fires when typed
readiness says the current evidence can answer the user request. The log-backed
eval shows this signal is often correct, but it must not become a hard stop:
large repositories, multi-language call chains, attached logs/traces, git
history, command outputs, cross-repo indexes, MCP/connector data, and external
documents can all produce legitimate last-mile checks after the first ready
signal.

The current code therefore has the right first step:
`postCompletionReadySignal` only injects a hint. The systemic gap is the second
step. When the model ignores the hint for multiple turns,
`postCompletionReadyEscalationSignal` and
`postCompletionReadyClosureOnlySignal` still only speak in prose. A local model
can keep widening with `grep`, `list_files`, `repo_map`, or shell commands even
after the system has a typed, citable answer surface. This does not usually
break correctness, but it increases latency, context size, and downstream noise.

Systemic fix: introduce a Closing Lane that activates only after the existing
completion-ready escalation latch is set. The lane is state-driven and
tool-class based:

- The initial completion-ready hint is advisory and leaves the normal explorer
  tool surface unchanged.
- Escalated completion-ready blocks broad scope expansion tools, not final
  answer content. It allows structured progress tools and exact source
  inspection: `read_file`, `emit_evidence`, and `emit_investigation_complete`.
- Evidence repair remains higher priority and may keep exact `read_file`
  available for validator-named repair targets.
- The lane must not inspect user-question keywords or model prose. It is derived
  from existing evaluator fields (`midLoopCompletionReadySent`,
  `midLoopCompletionReadyEscalated`, repair latches, and tool names).
- If a future structured gap requires another narrow tool class, add it through
  an explicit typed repair state rather than by parsing answer text.

This keeps user intent primary: the system does not decide the answer text or
diagram shape, and it does not force a rewrite. It only stops ungrounded
post-ready expansion while preserving a small, language-agnostic escape hatch for
exact file checks across every repomap-supported language.

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

### Batch 8 - Explorer Closing Lane

- Reuse `explorerEvaluator.restrictedToolSurface()` and
  `validateExplorerToolBoundary()`; do not create a separate policy stack.
- Keep the first completion-ready signal advisory: no schema/runtime restriction
  while `midLoopCompletionReadySent=true` and
  `midLoopCompletionReadyEscalated=false`.
- After `midLoopCompletionReadyEscalated=true`, expose only
  `read_file`, `emit_evidence`, and `emit_investigation_complete`.
- Reject hidden / text-recovered broad expansion calls at runtime with the same
  effective tool surface summary shown by schema filtering.
- Preserve higher-priority repair states:
  - evidence repair keeps exact `read_file` plus emit tools;
  - read-without-emit escalation remains materialization-only because no
    structured evidence exists yet.
- Add regression tests for:
  - advisory completion-ready leaves schemas unchanged;
  - escalated completion-ready filters broad tools but keeps `read_file`;
  - runtime execution rejects hidden `grep` / `repo_map` in the escalated lane;
  - `emit_investigation_complete` and `emit_evidence` still pass.

Follow-up batches if eval shows remaining latency:

- Add parameter-aware "known path only" checks for `read_file` after escalation,
  using already-read files and structured repair targets. This is intentionally
  not part of Batch 8 because a tool-name-only lane is lower risk and avoids
  false negatives on multi-language source layouts.
- Extend typed repair states for git/history, command-output-only, MCP,
  connector, and external-document lanes before allowing any additional narrow
  post-ready tool class.

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
- 2026-05-24: Re-ran `qf_sequence_analyzer_gate` with the local
  `Qwen3.5-9B-OptiQ-4bit` provider after Batch 6/7. Result: PASS. Important
  counters: `enumeration_push=0`, `finalizer_iters=1`, `tool_read_file=13`,
  `midloop_inject=17`. The answer document was accepted on the first finalizer
  emit and preserved the requested `sequenceDiagram`.
- 2026-05-24: Designed Batch 8 Closing Lane against the current code. Root
  cause: `postCompletionReadySignal` and its escalation correctly identify a
  typed close-ready state, but the existing restricted tool surface only covered
  evidence repair and read-without-emit escalation. The next implementation
  reuses that policy surface to block broad post-ready expansion after
  escalation while keeping exact `read_file` and structured emit tools.
- 2026-05-24: Batch 8 implemented. `restrictedToolSurface()` now keeps the first
  completion-ready state advisory and activates the Closing Lane only when
  `midLoopCompletionReadyEscalated` is set. The lane exposes `read_file`,
  `emit_evidence`, and `emit_investigation_complete`; hidden broad expansion
  calls are rejected before execution by the existing explorer runtime boundary.
  Added regression tests for advisory schema stability, escalated schema
  narrowing, and runtime rejection of hidden broad calls.
- 2026-05-24: Re-ran `qf_sequence_analyzer_gate` after Batch 8. Result: PASS,
  with `finalizer_iters=1`, `answer_chain_lines=14`, and a valid final answer.
  The cost profile is still not acceptable for customer-scale repos:
  `explorer_iters=40`, `midloop_inject=23`, `tool_read_file=19`, and
  `explorer_dispatches=2`. Raw logs also show `explorer.mid-loop.enumeration`
  injected for a typed `IntentTrace` / `ReqCallChain` / sequence-diagram request,
  even though the eval summary counter reported `enumeration_push=0`. This is a
  logging / metric mismatch plus a real policy leak; Batch 9 is scoped to close
  that gap and tighten evidence-repair reads.
- 2026-05-24: Batch 9 implemented. `shouldRunEnumerationCoverageMidLoop()` now
  refuses trace/path contracts before any broad file-coverage calculation and no
  longer treats a loose category-shaped fallback as sufficient for "read N
  discovered files"; explicit typed exhaustive obligations still trigger the
  enumeration lane. Evidence repair now tracks structured repair targets against
  read-file line windows and narrows to emit-only once the target window is
  covered. Added regression tests for sequence-trace suppression, repair-window
  coverage, runtime rejection after coverage, and the explicit-obligation
  enumeration path.
- 2026-05-24: Post-Batch-9 targeted tests and `make build` passed. A real
  `qf_sequence_analyzer_gate` rerun with a 10 minute cap timed out in finalizer
  after reaching the first finalizer model request. Important observations:
  no `explorer.mid-loop.enumeration` and no `intent-window-mismatch` appeared in
  the raw log; evidence repair did switch to tools=2 after the exact target
  window was read. Remaining cost is now dominated by path-specific issues:
  large-function partial-read nudges pushed pagination through most of
  `buildAnalysisIR`, and read-without-emit escalation narrowed tools to emit-only
  before the sink endpoint was fully covered, causing one local-model
  "I cannot read anymore" absence attempt. These are Batch 10 candidates, not a
  regression in Batch 9.

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

Follow-up rerun after Batch 6/7:

- Result: PASS.
- The analyzer still attempted disallowed read/search tools after terminal
  emit-only state, but every attempt was rejected before dispatch and the next
  successful `emit_analysis` preserved `intent=trace`, `question_kind=call_chain`,
  and `diagram_hint=sequence`.
- The broad enumeration mid-loop hint no longer fired (`enumeration_push=0`);
  analyzer sub-topics no longer changed the user request from bounded sequence
  trace to file inventory.
- Finalizer produced a valid `emit_answer_document` on the first iteration,
  including a Mermaid `sequenceDiagram`.
- Remaining non-blocking cost gap: explorer can still spend several iterations
  on navigation after completion-ready has already fired. This did not affect
  correctness in the rerun, but the same schema/runtime parity pattern can be
  applied to completion-ready escalation in a later batch to reduce latency.

Follow-up rerun after Batch 8:

- Result: PASS.
- Metrics: `tool_read_file=19`, `midloop_inject=23`, `analyzer_iters=5`,
  `explorer_iters=40`, `extractor_iters=1`, `finalizer_iters=1`,
  `explorer_dispatches=2`, `answer_chain_lines=14`.
- Batch 8 did not regress correctness. Finalizer accepted the first
  `emit_answer_document`; no finalizer rewrite storm occurred.
- The Closing Lane did not address the dominant cost in this run because the
  completion-ready escalation state did not become the primary loop controller
  before evidence-repair / refinement lanes took over.
- Raw logs show `explorer.mid-loop.enumeration` injected at explorer iteration 2
  for a typed trace request (`intent=trace`, `question_kind=call_chain`,
  `diagram_hint=sequence`, `is_category_enumeration=false`). That should be
  impossible for bounded trace and must be treated as a P0 policy leak, not as a
  prompt issue.
- Raw logs also show `explorer.mid-loop.intent-window-mismatch` being built for
  the same trace request. It was throttled rather than injected in this run, but
  the "overview needs wide read" lane should not even be considered for bounded
  endpoint/path contracts.
- Evidence-repair worked eventually, but the model spent multiple turns reading
  around the same target windows after the relevant windows were already
  available. The repair surface should transition from `read_file + emit` to
  emit-only once every active repair target has a covering read window or a
  small bounded read quota has been consumed.

Follow-up rerun after Batch 9:

- Result: TIMEOUT at 10 minutes during the first finalizer LLM request. The run
  had already reached finalize; the timeout is a latency / cost failure, not an
  observed answer-quality failure.
- The P0 leaks addressed by Batch 9 stayed closed in the raw log:
  `explorer.mid-loop.enumeration` did not appear, and neither did
  `explorer.mid-loop.intent-window-mismatch`.
- Evidence repair target coverage behaved as designed. After the model read
  `internal/agent/analyzer.go` lines 2321-2340 for a recovered
  `analyzerGraphForNormalize` row, the next request exposed only
  `emit_evidence` and `emit_investigation_complete`, and the model re-emitted
  the corrected row.
- The run surfaced two next root causes. First, the generic partial-read nudge
  treated a bounded path through a very large function as if the entire function
  body had to be read, even after the requested terminal call was covered.
  Second, read-without-emit escalation can be too blunt for bounded traces: it
  may hide `read_file` / `grep` before the sink endpoint or repair target is
  covered, making small models complain that they cannot continue investigating.
- The next batch should narrow these generic loop policies by typed path state,
  not by user text: endpoint coverage, terminal call coverage, active repair
  targets, and current support-lane gaps.

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

### P0 - Bounded Trace Leak Audit

Observed after Batch 8 in `qf_sequence_analyzer_gate`: the analyzer emitted a
typed bounded trace contract (`IntentTrace`, `ReqCallChain`, `diagram_hint`
sequence, and no category enumeration), but explorer still injected a broad
"read N discovered files" enumeration hint. The summary metric did not count it,
which means the metric pipeline can under-report the exact hint class that most
hurts small-model latency.

Preferred runtime-side design:

- Treat bounded trace as an answer-shape contract, not as a natural-language
  heuristic. The guard should depend only on the typed `RequestModel`,
  `AnalyzerHints`, and explicit exhaustive obligations.
- `IntentTrace` plus call-chain / conditional-flow / registration-flow should
  suppress broad enumeration hints unless the RequestModel also carries a typed
  principal member-set, count, relation, category enumeration, or exhaustive
  completeness obligation.
- Apply the same suppression before building overview-window mismatch hints.
  A path/sequence answer needs endpoint and edge coverage; it does not need a
  whole-file structural overview unless the typed request separately asks for
  file architecture.
- Add a regression test that mirrors the raw log shape, including analyzer
  sub-topics and a sequence diagram hint, and asserts no enumeration or
  overview-wide-read hint is injected or built.
- Fix eval metric extraction so an injected `explorer.mid-loop.enumeration`
  cannot be reported as `enumeration_push=0`.

### P0 - Evidence Repair Read-After-Target Gate

Observed after Batch 8: evidence repair correctly exposed `read_file` because
the repair hint can require exact source rechecks. However, after the target
window has been read, allowing unlimited further reads lets small models loop on
nearby offsets instead of making the only safe decision: re-emit grounded rows
or drop/replace invalid ones.

Preferred runtime-side design:

- Keep the first repair turn permissive: `read_file`, `emit_evidence`, and
  `emit_investigation_complete`.
- Track active repair targets structurally from validator output and read-window
  coverage from tool results. Do not parse model prose to decide whether a
  target is fixed.
- Once every active target has a covering read window, or once a small bounded
  quota of repair reads is exhausted, narrow the repair surface to emit-only:
  `emit_evidence` and `emit_investigation_complete`.
- If a later validator error introduces a new target, reopen the exact-read
  allowance for that new target only.
- Never synthesize replacement evidence content. The runtime only narrows the
  action space after the model has enough source to decide; the validator still
  enforces grounding.
- Add telemetry: repair target count, covering read count, repair-read quota
  use, and whether the surface moved to emit-only.

### P0 - Bounded Path Partial-Read Policy

Observed after Batch 9: a sequence/call-chain request over a large function kept
receiving generic partial-read nudges until most of the function body was read.
For a path answer, the runtime should care about endpoint and edge coverage, not
whole-function pagination once the requested sink edge has been grounded.

Preferred runtime-side design:

- Keep generic partial-read hints for mechanism explanations, architecture
  surveys, and cases where the model has only entered the beginning of a
  relevant function and has not found the requested terminal edge yet.
- For typed path requests, suppress or soften partial-read once the current
  grounded chain lane includes the source endpoint, at least one intermediate
  call edge, and the sink/terminal edge or a typed waiver explaining why the
  sink is indirect.
- If a path request still lacks the sink, emit a targeted missing-endpoint hint
  naming the endpoint and nearest already-read range instead of asking the model
  to read the rest of the entire function.
- Use existing `FlowFindings`, `EvidenceRequirement`, and answer-chain lane data;
  do not create a parallel path-ranking system.

### P0 - Read-Without-Emit Escalation Must Respect Uncovered Path Endpoints

Observed after Batch 9: read-without-emit escalation correctly prevents endless
navigation, but in a bounded trace it can activate before the sink endpoint has
been covered. The small model then sees only emit tools and may conclude it
cannot finish the investigation.

Preferred runtime-side design:

- Before switching to emit-only for bounded path requests, check whether the
  source endpoint, sink endpoint, or active repair target lacks a covering
  read-file window.
- If an endpoint is still uncovered, keep a narrow `read_file` allowance for that
  endpoint file/range instead of the full broad navigation surface. `grep` may be
  allowed only when it is path-scoped to an already-known file or endpoint
  symbol; repo-wide expansion remains blocked.
- Once endpoint coverage exists, reuse the existing emit-only escalation.
- The decision must come from typed endpoint/requirement structures and tool
  results, never from model prose such as "I need to read".
- If the model repeats the same blocked deep-read fingerprint, break early to a
  degraded but valid `emit_analysis` draft instead of spending another full LLM
  round on the same mistake.

Batch 10 implementation plan (2026-05-24):

- Reuse the existing call-chain endpoint contract in
  `answer_support_plan_call_chain.go`; export only small endpoint helper wrappers
  instead of rebuilding a second matcher in explorer.
- Treat endpoint coverage as typed runtime state:
  grounded/recovered `emit_evidence` fields, current `AnswerSurfacePlan`
  surface evidence, and `read_file` windows that cover a matching repomap symbol.
  Raw user text and model prose remain out of the decision path.
- When read-without-emit escalation is about to hide navigation tools on a
  bounded trace, first check the terminal endpoint. If it is uncovered, emit one
  targeted endpoint hint and keep the normal tool surface open. Once the terminal
  endpoint is covered, fall back to the existing emit-only escalation.
- For generic partial-read hints on bounded traces, suppress whole-function
  pagination once a typed path carrier and the terminal endpoint are covered.
  If the terminal endpoint is still missing, keep partial-read nudges only for
  the endpoint-bearing symbol; otherwise let the endpoint hint guide a targeted
  read/grep rather than "finish the entire large function".
- Tests:
  - bounded trace with missing terminal endpoint must not latch
    `midLoopNoEmitEscalated`;
  - bounded trace with covered terminal endpoint keeps the existing emit-only
    behavior;
  - bounded trace partial-read hints are suppressed after terminal endpoint plus
    call-chain evidence coverage;
  - unrelated partial-read hints are not used as a surrogate for a missing
    terminal endpoint.

Task status:

- [x] Audit existing loop-control and support-lane code paths.
- [x] Export endpoint helper wrappers from `internal/types`.
- [x] Add explorer endpoint-coverage helpers based on structured evidence,
  answer-surface plan, repomap symbols, and read windows.
- [x] Gate read-without-emit escalation on terminal endpoint coverage.
- [x] Gate bounded-trace partial-read hints on endpoint/path-carrier coverage.
- [x] Add focused regression tests and run targeted test suites.

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

## 2026-05-24 Batch 11 - Evidence Backbone Handoff Fidelity

Problem statement:

- The explorer already builds a ranked, deduplicated evidence backbone in
  `StageOutput.EvidenceItems`.
- The extractor's primary deterministic digest reads
  `TurnAArtifacts.EvidenceItems`.
- For non-mechanism questions with strict answer chains, the current Turn A
  handoff stores only the strict chain terminals. That is correct for the
  cardinality baseline, but it can hide adjacent grounded/read/scope-relevant
  mechanism facts from Turn B. Small models then either shrink the final answer
  or spend retries trying to rediscover details they are no longer allowed to
  read.

Existing code to reuse:

- `rankEvidenceByRelevanceWithSubject` already orders evidence by the request
  model, subject, read-file coverage, graph relation, anchor kind, salience, and
  diversity. Do not add a second scorer.
- `EvidenceSalience` and `SalienceLockedForScoring` already protect
  load-bearing / exhaustively listed evidence.
- `supportLaneScope`, `BuildAnswerSupportPlan`, and typed enrichment lanes
  already constrain finalizer enrichment. Do not introduce a parallel support
  plan.
- `TerminalEvidenceCount` is already the strict answer-cardinality baseline and
  is explicitly not `len(EvidenceItems)`.

Design:

- Split Turn A handoff evidence into two semantic roles:
  - strict terminal evidence, used only to compute and preserve
    `TerminalEvidenceCount`;
  - support evidence, already ranked and grounded by the explorer, used as
    extractor/finalizer context.
- Store a deduped union in `TurnAArtifacts.EvidenceItems`: strict terminals
  first, then top ranked support evidence up to the existing extractor display
  budget.
- Keep `TerminalEvidenceCount` unchanged. Adding support evidence must never
  create a larger required answer-symbol slate.
- Preserve current mechanism behavior: mechanism/step-list paths with no strict
  chains still seed the handoff from ranked evidence.
- Preserve the old conservative boundary for non-seed paths with no strict
  terminal evidence: do not newly expose loose ranked evidence unless the
  request shape already allowed ranked-evidence seeding.
- Prefer existing IDs via `StableEvidenceID`; no prose matching, user-question
  keyword matching, or case-specific answer matching is allowed.
- Do not mutate evidence content or promote context rows into principal members.
  Downstream prompts and support lanes already state that deterministic evidence
  rows are enrichment/context unless a typed support lane or answer contract
  makes them principal.

Commercial safety boundaries:

- Big-model quality is preserved because validators, support lanes, and
  finalizer contracts are unchanged.
- Small-model compatibility improves because useful evidence is visible in the
  no-tool extractor phase.
- The fix is language-agnostic: it works over `EvidenceItem` metadata produced
  by the existing repomap/grounding layer for every supported language.
- The handoff remains bounded by `extractorMaxEvidence`, so large customer repos
  do not get an unbounded transcript blow-up.

Task list:

- [x] Add a shared helper that builds Turn A handoff evidence from strict chains
  plus ranked evidence.
- [x] Replace the inline strict-evidence construction in `explorer.ParseOutput`
  with that helper.
- [x] Add regression tests for:
  - strict terminals remain first and do not block ranked support details;
  - `TerminalEvidenceCount` remains independent from support evidence count;
  - mechanism no-chain behavior stays capped at `extractorMaxEvidence`;
  - duplicate strict/ranked items are deduped by stable evidence ID.
- [x] Run targeted tests and full build.

Next batches after this patch:

- Evaluate whether extractor value-fact rendering should explicitly label rows
  as `principal` vs `support` using existing `ContextRole`/support-lane
  metadata, without changing schemas.
- Audit answer-document validation rejections after this handoff fix to avoid
  fixing downstream symptoms that are caused by upstream evidence narrowing.

### Targeted Eval Observations - 2026-05-24

Model/config:

- Local provider: `Qwen3.5-9B-OptiQ-4bit`.
- `recover_text_tool_calls: true`.
- Binary: current `main` after Batch 11.

Raw artifacts:

- Custom q1:
  `.codrax/eval-targeted-handoff/q1/explorer_subagent_sequence.out`
- Custom q1 log:
  `.codrax/logs/targeted-handoff/q1/codrax-20260524-021239-000-25243.log`
- Eval q2:
  `.codrax/eval-targeted-handoff/qf_relation_subagent_registry-20260524-022554/run-1.out`
- Eval q2 log:
  `.codrax/eval-targeted-handoff/qf_relation_subagent_registry-20260524-022554/run-1.logs/codrax-20260524-022555-000-32917.log`

Confirmed observations:

- Batch 11 handoff behavior is visible in q1. One explorer branch wrote
  `TurnAArtifacts` with `24 evidence` and `termCount=0`, preserving the ranked
  mechanism evidence for downstream stages instead of handing off an empty
  mechanism snapshot.
- The analyzer still frequently acts like an investigator. In q1 it attempted
  `read_file` and then `list_files` after analyze had entered terminal emit
  mode. In q2 it also called `read_file x2` during analyze before `emit_analysis`.
  The boundary rejection works, but it costs multiple LLM rounds before the real
  exploration stage begins.
- Small models confuse evidence semantic kind with evidence location scope. In
  q1 the first `emit_evidence` batch used
  `scope="definition"|"mechanism"|"relationship"` while `evidence_kind` already
  carried the correct semantic value. The validator rejected the whole batch;
  the next retry fixed it to `line` / `line_range` and succeeded. This is a safe
  auto-repair candidate: when `scope` is one of known evidence kinds and
  `source + line_start` are present, move the semantic value to
  `evidence_kind` only if needed and set `scope=line` or `line_range` based on
  whether `line_end` is present. Do not invent evidence content.
- Explorer often writes a complete prose answer before structured closure. In q1
  the model produced a long complete answer with call-chain diagram/code blocks
  inside explore. In q2 it produced the exact answer: one default subagent,
  member `explorer`, plus registration and `Name()` evidence. The UI surfaced
  this text, which is good, but the controller treated it as phase progress and
  kept exploring instead of converging.
- Soft-stop / readiness is not yet monotonic enough for local models. In q2 the
  system later emitted `explorer.mid-loop.completion-ready`, but only after the
  model had already spent many rounds widening scope. After a correct prose
  conclusion, it still searched for other default subagents before attempting
  closure.
- Narrow relation/member-set questions still over-expand. q2 should terminate
  once `RegisterDefaultSubAgents`, `Register`, and `SubExplorer.Name()` are
  grounded and the model-authored member set is `{explorer}`. Instead it read
  unrelated neighboring files and emitted unrelated evidence such as
  `ExecCommand`, showing that principal/support boundaries remain too weak in
  explorer guidance and evidence lanes.
- Closure JSON compatibility still has gaps. In q2 the first
  `emit_investigation_complete` included `aggregate_facts.kind="registration"`,
  which is not accepted. The next attempt omitted aggregate facts and was
  downgraded for missing relation member-set handoff. A later attempt included a
  usable-looking `member_set`, but the downgrade message referenced stale or
  unrelated members from other aggregate facts (`NewSubExplorer`,
  `buildScopedSearchGraph`, `egrepQueryFromObjective`, `NewSubAgentRegistry`).
  This suggests the closure repair target/context may be mixing historical
  aggregate candidates with the current attempted payload.
- After repeated closure downgrades, the q2 model said it needed to "clear the
  evidence cache" and re-emit evidence. This is not an action the model can
  actually perform, and it indicates the repair diagnostics are pushing the
  model toward imaginary state management instead of a concrete current-payload
  fix.
- Stream stalls can multiply exploration cost. In q1 the model generated a rich
  explorer answer, then the LLM stream later stalled and the orchestrator retried
  exploration, causing duplicate work rather than salvaging the already visible
  draft/evidence.

High-ROI follow-up design candidates:

- Analyze-stage action gating:
  once analyze has enough pre-scan signals or has rejected a deep-reading tool,
  constrain the tool surface to `emit_analysis` earlier and stop offering
  navigation tools for that dispatch. Trigger from stage/tool state only, not
  user text.
- `emit_evidence.scope` semantic-value repair:
  implement in the shared text/tool-call normalization or emit-evidence
  parameter normalization layer. The repair is structural and schema-safe when
  file/line anchors exist.
- Explorer convergence guard:
  when the branch has grounded principal evidence plus a model-authored prose
  conclusion that matches already accepted structured evidence, push closure
  rather than a phase transition. Do not use user-question keywords; rely on
  evidence buffer, accepted aggregate facts, and typed answer-shape readiness.
- Relation/member-set principal boundary:
  for typed relation/enumeration requests, rank and hint principal evidence
  separately from generic support/context. Evidence such as tools, registries,
  runtimes, and neighboring helpers may explain why, but must not compete with
  the verified principal member set.
- Closure repair isolation:
  when `emit_investigation_complete` is retried, diagnostics should be scoped to
  the current payload plus accepted evidence buffer. Historical rejected
  aggregate members must not pollute the next error unless they are still present
  in the current payload.
- Prose salvage path:
  preserve explorer-stage complete answers as transparent progress, but mark
  them as non-final. If a later stall/retry happens, downstream should still be
  able to use the already accepted evidence and saved draft for finalizer
  context without forcing a full re-exploration loop.

## 2026-05-24 Batch 12 - Closure Convergence and Evidence-Scope Compatibility

Problem statement:

- The latest local-model evals show the explorer can collect the right facts and
  even write a correct provisional answer, but still burn many rounds because
  completion-ready is advisory for too long and the escalated closing lane still
  exposes `read_file`.
- The first q1 `emit_evidence` batch failed only because the model put semantic
  evidence words (`definition`, `mechanism`, `relationship`) into `scope`. The
  same payload already contained source, line, anchor, and semantic fields, so
  this is a structural compatibility issue, not a factual one.
- q2 closure failures also showed that repair hints must stay scoped to the
  current payload and live accepted evidence. Historical rejected aggregate
  members must never reappear as if they were part of the current repair.

Existing code to reuse:

- `toolparam.Normalize` remains the generic schema-aware JSON repair layer.
  Batch 12 must not add a second generic JSON normalizer.
- `emit_evidence.repairEmitEvidenceItemShape` is already the tool-local place
  for evidence semantic/location confusion such as evidence_kind vs anchor_kind.
- `explorerEvaluator.FilterToolSchemas` already narrows the runtime tool surface
  after structured loop signals. It is the right place to make completion-ready
  convergence monotonic.
- `effectiveCompletionAggregateFacts` already preserves a crucial isolation
  invariant: when the current `emit_investigation_complete` payload has
  aggregate facts, older retained facts are not merged back into it. Future
  changes must preserve this invariant.
- `EvidenceClosure.ActiveRepairs` already filters stale read repairs against
  current pending reads. Do not bypass it with direct repair-ledger reads.

Design:

- P0-A: tighten completion-ready escalation.
  - First `explorer.mid-loop.completion-ready` remains advisory.
  - After the existing escalation latch fires, the schema filter should expose
    only structured progress tools: `emit_evidence` and
    `emit_investigation_complete`.
  - Exact additional reads remain available through the separate
    evidence-repair lane (`midLoopEvidenceRepairSent`) and pending-read repair
    lane. Completion-ready by itself is a "close or materialize current facts"
    state, not a permission to keep browsing.
  - This is typed-state driven only: no user question keyword matching and no
    model prose matching.

- P0-B: repair `emit_evidence.scope` semantic-value mistakes conservatively.
  - If `scope` is not a valid evidence scope but equals a known semantic
    evidence kind, or an anchor-kind alias such as `definition` / `call`, and
    the item already has a repo source plus positive `line_start`, rewrite the
    scope to `line` or `line_range` based only on `line_end`.
  - Preserve an already valid `evidence_kind`.
  - If `evidence_kind` is missing and the misplaced scope can be mapped to a
    valid evidence kind, fill it from the misplaced value using the existing
    `evidenceKindForAnchorShape` mapping for anchor-kind words.
  - Do not repair `negative`, `file`, `crossfile`, or any non-line-shaped item
    without source/line data. Do not invent subject/object/summary/content.

- P0-C: keep closure repair diagnostics isolated.
  - Current-payload aggregate facts must remain authoritative for the current
    closure attempt.
  - Empty-payload retries may still carry the last accepted aggregate handoff,
    because that is the existing narrow repair-window behavior.
  - Tests should pin this boundary so future refactors do not accidentally
    merge historical rejected member sets into a non-empty current payload.

- P1: analyzer phase action gating.
  - Analyze is classification and light routing. Once the analyzer has already
    crossed a terminal-emit boundary or a deep-reading tool has been rejected,
    keep the schema surface to `emit_analysis` for that dispatch.
  - This belongs in `analyzerEvaluator.FilterToolSchemas`, reusing the existing
    analyzer terminal-emit state, not in prompt text.

- P2: eval expansion.
  - Keep q1/q2 as regression probes.
  - Add one scalar/count question, one runtime/log artifact question, one
    cross-language Java/C++ or Kotlin/Java relation question, and one
    diagram-request question to cover the same failure class beyond the CodraX
    Go repo.

Commercial safety boundaries:

- The system must not change user intent or rewrite model conclusions. Batch 12
  only changes available tool surfaces after typed readiness signals and repairs
  schema/location fields that are mechanically verifiable.
- Big-model quality is preserved because normal exploration remains unchanged
  until completion-ready escalation or an already-existing repair state fires.
- The scope repair is language-agnostic: it relies on source path + line anchor
  metadata and existing grounders, not Go syntax or case-specific identifiers.
- If a repair cannot be proven structurally, the validator still rejects with a
  normal tool error.

Task list:

- [x] Update `completionReadyClosingToolNames` and tests so escalated
  completion-ready is emit-only.
- [x] Add conservative `emit_evidence.scope` semantic-value repair in
  `repairEmitEvidenceItemShape`.
- [x] Add regression tests for semantic scope values, anchor-kind scope aliases,
  missing evidence_kind fill, and no repair when source/line are absent.
- [x] Add regression coverage for aggregate-fact current-payload isolation
  (`TestEffectiveCompletionAggregateFacts_CurrentPayloadReplacesStaleRetainedFacts`).
- [x] Run focused tests plus `make build`.
- [x] Commit and push Batch 12.

## 2026-05-24 Batch 13 - Analyzer Action Boundary Completion

Problem statement:

- Latest `main` already contains most analyzer action gating:
  `FilterToolSchemas` narrows analyze to `emit_analysis` plus lightweight
  pre-scan tools, `validateAnalyzerToolBoundary` blocks content-reading tools
  before execution, and blocked deep-read intent forces the next request into
  terminal emit-only mode.
- The remaining small-model gap is the low-level `grep` parameter mistake:
  when compatibility repair is disabled or unavailable, a model may call
  `grep` without `files_only=true`. The runtime rejects it, but only the tool
  result carries the correction. Some small models miss that correction and
  spend another turn as if analyze were an investigation stage.

Existing code to reuse:

- `analyzerEvaluator.Observe` already translates structured tool repairs into
  loop hints for prescan budget and terminal-emit mode.
- `normalizeAnalyzerPrescanGrepCompat` already auto-adds `files_only=true` when
  per-agent tool-param compatibility repair is enabled.
- `validateAnalyzerPrescanToolCall` is the authoritative runtime gate for
  pre-scan tool shape. Do not duplicate its JSON validation elsewhere.

Design:

- Add one Observe branch for `analyzer_grep_files_only_required`.
- The branch should emit a typed loop hint that says the only valid next actions
  are `grep(files_only=true)` for the same lightweight pre-scan purpose, or
  `emit_analysis` with current classification. It must not tell the model to
  read file contents.
- Do not force emit-only solely because of this shape error: the user/model may
  still need one files-only pre-scan query. The hard boundary remains runtime
  validation plus the existing prescan budget.
- This keeps large-model behavior stable: compliant tool calls are unaffected,
  and repair-mode providers already auto-normalize before this branch matters.

Task list:

- [x] Confirm existing analyzer action-gating code and tests on current `main`.
- [x] Add loop hint for rejected analyze-stage `grep` without `files_only=true`.
- [x] Add regression test for the new Observe branch.
- [x] Run analyzer/tool-param focused tests plus `make build`.
- [x] Commit and push Batch 13.

## 2026-05-24 Batch 14 - Targeted Local-Model Eval After Batches 12/13

Problem statement:

- Batches 12 and 13 changed upstream convergence behavior. The next high-ROI
  step is measurement, not another speculative code patch.
- The eval must reuse existing harnesses and cases so results are comparable to
  earlier `eval/results/local-small-*` runs.

Existing assets to reuse:

- `eval/run.sh` already builds the binary, runs a case serially, captures
  stdout, debug logs, verdicts, and mechanism metrics.
- Existing cases cover the three target surfaces:
  - `qf_sequence_analyzer_gate.case`: analyzer-gate + sequence diagram.
  - `qf_relation_subagent_registry.case`: relation/member_set principal answer.
  - `read_combo_pipeline_sequence_table.case`: finalizer-heavy diagram + table.
- Local provider from `providers.yaml` uses
  `Qwen3.5-9B-OptiQ-4bit`, `recover_text_tool_calls=true`, and
  `tool_param_compat` settings. Do not duplicate provider routing in the eval
  command.

Design:

- Run the cases serially with `N=1` into a fresh results root:
  `.codrax/eval-batch14-local-small`.
- Use the existing single-shot read-mode path, not REPL, because the harness
  already records comparable metrics and logs.
- After every case, inspect:
  - verdict and expected-surface failures;
  - analyzer/explorer/extractor/finalizer iteration counts;
  - `midloop_inject`, repair-plan, strict-decode, and answer-document rejection
    signatures;
  - whether provisional diagrams/tables are preserved into final output.
- Only implement code after the logs identify a system-side repair that is
  structural, typed, and safe. Do not react to one-off answer wording.

Task list:

- [x] Confirm current workspace is clean and eval harness/cases exist.
- [x] Run `qf_sequence_analyzer_gate`.
- [x] Run `qf_relation_subagent_registry`.
- [ ] Run `read_combo_pipeline_sequence_table`.
- [ ] Summarize metrics, failures, and remaining gaps in this document.
- [ ] Decide the next code batch from observed evidence.

Observed results so far:

- `qf_sequence_analyzer_gate`: FAIL. The run was healthy from a loop/JSON
  perspective (`analyzer_iters=3`, `explorer_iters=7`, `extractor_iters=1`,
  `finalizer_iters=1`, `midloop_inject=3`, no strict-decode repairs), but the
  final answer omitted the explicitly requested Mermaid sequence diagram.
  The finalizer prompt had `has_diagram=true` and a required `diagram` block,
  and the semantic reviewer correctly reported that the answer lacked Mermaid.
  However, `emit_answer_document` accepted the missing diagram as a soft
  pre-emit advisory (`blocks[].kind=diagram ... currently emitted: 0`) instead
  of forcing an immediate repair. This is a system-side contract enforcement
  gap, not a JSON recovery gap.
- `qf_relation_subagent_registry`: manually stopped after repeated same-cause
  relation handoff downgrades. The model repeatedly produced the correct
  principal facts (`members=["explorer"]`, `value="1"`) and later added
  `support_refs` plus `role="principal_answer"`, but
  `emit_investigation_complete` still reported
  `aggregate_facts[0] role="supporting_coverage" is not principal_answer`.
  The run then re-entered exploration and kept collecting more evidence for an
  already-proven member set. This indicates a system-side aggregate-fact
  normalization/reconciliation bug: a verifiable relation `member_set` can be
  demoted to supporting coverage even when the model marks it principal and
  backs it with a source line.
- The same run also showed a search-width usability issue. Broad symbol greps
  such as `RegisterDefaultSubAgents` with context returned hundreds of
  auxiliary/test/doc matches, increasing context without adding principal
  evidence. This is secondary to the member-set bug, but worth addressing with
  existing active-scope / production-priority tools rather than prompt changes.

Search-width root cause and optimal design:

- Current code already has useful local protections:
  - `grep` separates production vs auxiliary results through
    `annotateGrepOutputByPathRole`.
  - `repo_map`, `grep`, and `list_files` share multi-repo active-set / path
    scope gates, so parent-directory escape is hard-refused before scanning.
  - analyzer pre-scan has a separate `files_only=true` and round-budget
    contract.
- The missing architecture is a shared retrieval-width contract across
  discovery tools. Today each tool faithfully returns the result of the model's
  broad query. The system only annotates or truncates after the fact, so a
  broad query can still consume large context and bias later evidence selection.
- The right fix is a single `retrieval governor` layer used by `grep`,
  `repo_map`, `list_files`, and any future repository/external-observation
  search tool. It should not inspect user prose or model free-form prose to
  decide intent. It should only use typed state already produced by the system:
  pipeline stage, active repository scope, structured analyzer fields
  (`entities`, `exact_targets`, `required_files`, `source_scope_profile`,
  `diagram_hint`, `question_kind`), current read-set/evidence-set, and the
  tool parameters in the current call.
- The governor should produce a structured `RetrievalPlan` before execution:
  `scope_class` (primary_file / required_file / active_dir / repo_root),
  `query_specificity` (exact_symbol / path_like / phrase / generic_regex),
  `mode` (discovery / line_window / count / map), `budget`, and
  `degrade_action`.
- Execution should stay semantic-preserving:
  - never rewrite the user's question or the model's answer;
  - never change a query into a different semantic query;
  - safe parameter normalization is allowed only when it narrows presentation,
    not meaning: e.g. prefer `files_only=true` for broad discovery, clamp
    `context_lines`, cap `top_n`, and partition production/auxiliary results;
  - if a query is too broad for line output, return a successful, structured
    `too_broad` result with top production files and a suggested next shape,
    rather than dumping thousands of lines or forcing a model rewrite.
- Returned results should be tiered, not merely truncated:
  1. exact production hits inside required/primary/read files;
  2. production hits inside active scope;
  3. auxiliary/test/doc hits;
  4. overflow counts and a blob reference for exact raw paging.
  Downstream should see the top tiers inline and keep the full raw output
  available through the existing blob path. This preserves information without
  letting noise dominate context.
- This must be language-agnostic. Source/auxiliary classification should use
  existing `types.ClassifySourcePathRole`, `types.LooksLikeTestFilePath`,
  repomap language metadata, and file-type mappings instead of Go-only suffixes.
- The UI/log should report the governor decision compactly:
  `grep narrowed: broad line query → discovery summary (production=12,
  auxiliary=244, raw=blob://...)`. This helps users understand progress without
  adding prompt noise.

Implementation tasks for the search-width architecture:

- [ ] Add a shared retrieval-governor package or helper beside existing tool
      path/scope helpers; do not duplicate active-set logic.
- [ ] Route `grep` through the governor before execution and through a tiered
      result packer after execution.
- [ ] Route `repo_map` through the same budget model for `top_n`, query length,
      and root-wide task maps.
- [ ] Extend tests with polyglot production/test/doc fixtures so the policy is
      not Go-only.
- [ ] Add telemetry counters for broad-query downgrades, production/auxiliary
      counts, raw blob preservation, and user-visible summary text.

## 2026-05-24 Batch 15 - Retrieval Width Governor, Grep First

Problem statement:

- Local-model eval showed broad repository searches can return hundreds of
  inline matches. The result is technically correct but poor retrieval UX:
  context grows, auxiliary/test/doc hits distract the model, and later stages
  spend turns repairing a path that should have been focused earlier.
- We need an architectural fix that is not prompt-specific, not
  case-specific, and not tied to Go-only paths.

Current code anchors:

- `internal/tool/builtin.go` owns `GrepTool.Execute`, search backend selection,
  params banners, timeout handling, and `StoreBlob` integration.
- `annotateGrepOutputByPathRole` already partitions production vs auxiliary
  matches using `types.ClassifySourcePathRole`.
- `internal/tool/search.go` owns shared directory exclusions.
- `internal/types/source_path.go` and `internal/types/test_path.go` provide
  language-neutral production/test/doc/fixture classification.
- `repo_map` already has hard path-scope guards in
  `internal/tool/repomap/tool.go`; this batch does not duplicate them.

Design:

- Batch 15 implements the first slice in `grep` only:
  - execute the model's exact query unchanged;
  - compute full match counts and full annotated output;
  - if the result is narrow, preserve the old byte-for-byte path through
    `StoreBlob`;
  - if the result is broad, keep the full result in an existing blob artifact
    and return a compact tiered summary inline.
- Broadness is structural, not semantic:
  - `files_only=true` is broad when it returns too many paths;
  - line/context mode is broad when it returns too many match/context lines or
    too many bytes;
  - thresholds are fixed conservative constants and do not depend on the user
    wording or model prose.
- Tiered summary contract:
  - preserve the original `[grep: N matching ...]` prefix and `[grep params]`
    banner for existing parsers;
  - add a `[grep retrieval governor]` section with counts, caps, and raw blob;
  - list production matches first, auxiliary/test/doc matches second, and
    passthrough/context separators last;
  - include omission counts for every tier;
  - never discard full raw bytes when `ctx.WorkDir` is available.
- This keeps large-model behavior stable: narrow compliant searches are
  unchanged, while broad searches get less noisy but more explicit output.

Batch 15 task list:

- [x] Explore existing grep, source-role classification, blob, and tests.
- [x] Add grep retrieval-governor helper.
- [x] Wire helper into native and shell-backed grep result paths.
- [x] Add polyglot tests covering production/test/doc broad result compaction.
- [x] Run focused tool tests and `make build`.
- [x] Run full `go test ./...`.
- [x] Commit and push Batch 15.

## 2026-05-24 Batch 16 - Relation Member-Set Recheck After Upstream Fixes

Problem statement:

- Batch 14 found that `qf_relation_subagent_registry` repeatedly emitted the
  correct member set (`explorer`, count `1`) but
  `emit_investigation_complete` kept reporting the relation handoff as
  `supporting_coverage` rather than `principal_answer`.
- Remote `main` then added relation-focused commits:
  `688ffdba Preserve typed relation implementer sets` and
  `d1c28447 Design typed relation coverage contract`.
- Batch 15 also reduced broad `grep` result noise, which should help the same
  eval avoid irrelevant auxiliary/test/doc matches.

Design:

- Re-run `qf_relation_subagent_registry` once with the current `providers.yaml`
  local model and a fresh result root.
- Inspect whether:
  - `emit_investigation_complete` now accepts the relation `member_set`;
  - the final answer contains count `1`, the member `explorer`, registration
    evidence, and `Name()` return evidence;
  - broad `grep` calls are compacted by the retrieval governor rather than
    flooding the model context;
  - retries are bounded and do not re-enter generic exploration after a valid
    member set has been produced.
- Only patch code if the log shows a remaining structural/system-side gap.

Batch 16 task list:

- [x] Confirm branch is clean and includes upstream relation commits.
- [x] Run `qf_relation_subagent_registry`.
- [x] Analyze result, metrics, and log signatures.
- [x] Decide whether the next batch is relation-handoff repair or `repo_map`
      retrieval-governor expansion.

Batch 16 observations:

- The Batch 15 grep governor did fire and preserved full raw results through
  blob artifacts. The model no longer received hundreds of inline auxiliary
  matches.
- The remaining failure was ordering, not truncation. A broad production grep
  still surfaced an unrelated production comment before the already-read /
  evidence-bearing SubAgent files. The local model followed the inline order,
  read the wrong file, and then spent turns repairing anchors from irrelevant
  code.
- This shows that production-vs-auxiliary classification is necessary but not
  sufficient. The next layer must rank broad inline results by typed relevance:
  explicit tool scope, analyzer-required files, already-read files, accepted
  evidence files, and phase-1 ranked candidates. The raw backend order must be
  preserved within each equal relevance tier so the model's own scoped search
  order is still respected.

## 2026-05-24 Batch 17 - Typed Relevance Tiering for Broad Grep Results

Problem statement:

- Broad grep compaction now protects context size, but broad production results
  can still place generic implementation/comment hits ahead of files that the
  system already knows are more relevant from structured state.
- This is especially harmful for small local models: they often trust the first
  inline item and spend multiple turns repairing anchors from the wrong file.
  Large models also benefit because the finalizer/extractor JSON contracts are
  already cognitively heavy; retrieval order should reduce, not add, load.

Current code anchors:

- `internal/tool/builtin.go`:
  - `GrepTool.Execute` executes the exact model query and builds the params
    banner.
  - `finalizeGrepOutput` decides whether to return narrow output or compact a
    broad result.
  - `compactBroadGrepOutput` currently partitions only by source path role.
  - `annotateGrepOutputByPathRole` must remain stable for narrow output and
    analyzer prescan compatibility.
- `internal/types/context.go` already exposes the needed structured signals:
  - `BusContext.AnalysisIR.EvidencePlan.RequiredFiles`;
  - `MutableState.ExactContextRequiredFiles`;
  - `MutableState.Phase1Ranking`;
  - `MutableState.TurnAArtifacts().ReadFiles`;
  - `MutableState.TurnAArtifacts().ToolResults`;
  - `MutableState.EmittedEvidence`;
  - `BusContext.EvidenceItems`, `AnswerChains`, and `ToolResults`.
- `internal/tool/ground` already contains the canonical read-file banner parser
  and repo-relative path canonicalizer, so this batch should reuse those instead
  of adding another parser.

Design:

- Keep query semantics unchanged:
  - do not rewrite the user's request;
  - do not rewrite the model's grep pattern/path/file type;
  - do not inspect model free-form prose or user-prose keywords to decide
    relevance;
  - continue storing the complete raw result when compaction fires.
- Build a `grepRelevanceIndex` from structured state only:
  1. exact explicit file path from the current tool call;
  2. analyzer-required and exact-context-required files;
  3. already-read files and accepted/emitted evidence source files;
  4. phase-1 ranked files;
  5. explicit directory scope from the current tool call;
  6. ordinary production matches;
  7. auxiliary/test/doc/fixture matches.
- Preserve model/system ordering:
  - the model's current tool parameters define the search scope and are never
    widened;
  - ordered structural lists keep their order as the secondary sort key;
  - raw backend order is stable within the same tier and same structural rank.
- Keep path-role partitioning intact:
  - production matches are still listed before auxiliary/test/doc matches;
  - each section is internally relevance-tiered;
  - auxiliary files that are already read or evidence-bearing are promoted only
    inside the auxiliary section, not mislabeled as production proof.
- Keep narrow output stable:
  - if a result is below broadness thresholds, the existing annotation path is
    unchanged.

Batch 17 task list:

- [x] Explore existing BusContext/MutableState relevance signals and grep
      compaction code.
- [x] Record the typed relevance design in this document.
- [x] Implement structured relevance indexing for broad grep compaction.
- [x] Add regression tests for evidence/required files ranking ahead of generic
      production hits while preserving stable order inside equal tiers.
- [x] Run focused tool tests, affected agent/tool tests, `make build`, and full
      `go test ./...`.
- [x] Commit and push Batch 17.

## 2026-05-24 Batch 18 - Relation Eval Recheck, Remaining Gaps

Eval run:

- Case: `eval/cases/qf_relation_subagent_registry.case`
- Output root:
  `.codrax/eval-batch18/qf_relation_subagent_registry-20260524-035436`
- Model observed in log: `Qwen3.5-9B-OptiQ-4bit`
- The run was stopped after the blocking gaps below were captured. Continuing
  would mostly spend local-model time on the already-observed repair loop.
- The planned second eval (`qf_type_relation_loop_controller`) is still pending
  for this batch. If another user/customer eval is already running, do not
  interrupt it.

Positive signal from Batch 17:

- Broad line-output grep now ranks typed relevance correctly. In the
  `RegisterDefaultSubAgents` grep with 105 matching lines, the inline summary
  reported:
  `decision=broad_result_compacted mode=line_output entries=105 production=14 auxiliary=91`
  and
  `relevance=required=2 read_or_evidence=10 phase1_ranked=1 production_rest=1 auxiliary_rest=91`.
- The first production rows were the correct anchors:
  `internal/agent/subagent.go:62-63`, before ordinary production noise such as
  comments in `internal/tool/builtin.go`.
- This confirms the typed relevance index is useful and should be reused beyond
  only the broad-compaction path.

Remaining gaps discovered:

1. Medium/narrow grep output still uses path-role ordering only.

   - The current implementation applies typed relevance ranking in
     `compactBroadGrepOutput`, but the non-compacted annotation path still goes
     through `annotateGrepOutputByPathRole`.
   - In the same eval, a `files_only` grep and a 21-line grep did not trigger
     broad compaction. Their production sections still surfaced ordinary
     production files such as `internal/tool/builtin.go`,
     `internal/tool/defaults.go`, or `internal/skill/defaults.go` before
     already-read / evidence-bearing `internal/agent/subagent.go`.
   - This can still lead the model into irrelevant reads even after Batch 17.
   - Preferred fix: reuse the same typed relevance partition for every grep
     result that is sectioned by path role, not only broad compacted output.
     Narrow output should keep the existing headers and should not add noisy
     governor metadata unless useful, but the row order should still be
     structured-state-aware.

2. Explorer keeps expanding after sufficient principal evidence exists.

   - By iteration 7, the run had accepted 9 grounded evidence items covering:
     `SubAgentRegistry`, `Register`, `RegisterDefaultSubAgents`,
     `NewSubExplorer`, and `SubExplorer.Name() -> "explorer"`.
   - The model then correctly stated in prose that the default registered
     subagent name is only `"explorer"`, but continued to verify with more grep
     and reads (`cmd/root.go`, `internal/tool/builtin.go`, etc.).
   - Context grew from roughly 53k to 63k estimated tokens and added several
     slow local-model turns.
   - Existing mid-loop hints did eventually force evidence emission, but they
     did not strongly convert "principal enumeration evidence is sufficient"
     into a closure-only path.
   - Preferred fix: add a typed sufficiency gate for enumeration/relation
     questions that uses structured facts only: accepted grounded member
     evidence, required evidence axes, aggregate member count, and absence of
     actionable repair targets. When satisfied, prefer
     `emit_investigation_complete` / repair-only tools over generic
     navigation. Do not infer this from user text or model prose.

3. `emit_investigation_complete` member-set support matching is too strict for
   value-return members.

   - The model emitted:
     `aggregate_facts=[{kind:"member_set", value:"1", members:["explorer"], support_refs:["SubExplorer (internal/agent/sub_explorer.go:31)"]}]`.
   - This was downgraded as:
     `member "explorer" has no typed evidence / member-specific support_ref`.
   - However, grounded evidence already existed at
     `internal/agent/sub_explorer.go:31`, with `anchor_kind=return`,
     `anchor_symbol=Name`, and a snippet / source line returning `"explorer"`.
   - The current validator appears to require the member string to match the
     evidence subject/object/anchor name too literally, and does not treat a
     grounded return line containing the exact quoted member literal as member
     support.
   - Preferred fix: extend member-set support resolution to accept grounded
     evidence when the cited source line or snippet contains the exact member
     literal, especially for `return` / assignment / concrete-value evidence.
     This is a structural source-line check, not a free-form prose heuristic.

4. Duplicate evidence skipping interacts poorly with closure repair.

   - After the first downgrade, the model attempted to add evidence for the
     same `Name @ sub_explorer.go:31` return line. `emit_evidence` correctly
     skipped duplicates, but the closure validator still could not use the
     already-buffered duplicate target as member support.
   - This creates a repair loop: the model is told to add support, the support
     already exists, the duplicate is skipped, and the next completion attempt
     may fail the same way.
   - Preferred fix: make closure repair consume the existing deduplicated
     evidence pool before asking for more evidence, and make the downgrade
     message name the exact accepted support forms only when the pool truly
     lacks them.

5. Closure reason can contain a model-side naming slip that should not override
   structured aggregate facts.

   - The closure reason said the "完整成员名" was `SubExplorer`, while the
     structured aggregate member was correctly `explorer`.
   - Downstream answer construction should prioritize the structured
     `member_set.members` value and the grounded return-literal evidence over
     the explanatory prose reason.
   - This does not require changing the prompt. It is another reason to make the
     structured member-set handoff succeed when its evidence is valid.

Next high-ROI candidates from this eval:

1. Reuse typed relevance ordering for all grep sectioned output, not only broad
   compaction.
2. Fix member-set support matching for exact return/assignment/concrete-value
   literals.
3. Add relation/enumeration sufficiency gating to reduce post-evidence
   over-exploration.

## 2026-05-24 Batch 19 - Relevance Ordering for All Grep Sections

Problem statement:

- Batch 17 ranked broad compacted grep output correctly, but Batch 18 showed the
  same model can still be misled by medium/narrow grep results that do not cross
  compaction thresholds.
- Those results still use `annotateGrepOutputByPathRole`, which separates
  production vs auxiliary but preserves raw backend order inside production.
  Ordinary production files can therefore appear before already-read,
  required, or evidence-bearing files.

Design:

- Use the existing `grepRelevanceIndex` and `partitionGrepOutputByRelevance`
  for every grep output that is rendered with path-role sections.
- Preserve narrow-output surface stability:
  - keep `[grep production matches]` / `[prescan production matches]` headers;
  - do not add `[grep retrieval governor]` metadata unless broad compaction
    actually fires;
  - preserve backend order within equal relevance tiers.
- Keep semantic boundaries:
  - do not change the model's query, path, file type, or context setting;
  - do not inspect model prose or user-prose keywords;
  - do not relabel auxiliary/test/doc files as production even if they are
    evidence-bearing; only rank them earlier inside their own auxiliary section.
- Remove the old `annotateGrepOutputByPathRole` helper instead of leaving a
  tempting downgrade path. The existing `annotateAnalyzerPrescanGrepOutput`
  compatibility shim can call `annotateGrepOutputByRelevance` directly with the
  legacy `filesOnly` bit.
- Add a source-level regression test to keep `finalizeGrepOutput` on the
  relevance-aware path and to keep the deleted legacy wrapper from coming back.

Batch 19 task list:

- [x] Confirm current narrow output path and call sites.
- [x] Route `finalizeGrepOutput` through relevance-aware annotation.
- [x] Add regression coverage for narrow `files_only` and narrow line-output
      ordering.
- [x] Run focused grep tests, affected agent/tool tests, `make build`, and full
      `go test ./...`.
- [x] Commit and push Batch 19.

## 2026-05-24 Batch 20 - Value-Literal Member Support Reconciliation

Problem statement:

- Batch 18 exposed a reusable support-matching gap, not a one-off
  `SubExplorer` issue.
- A model can correctly emit a principal `member_set` whose visible member is a
  value literal, config key, route string, enum string, registered name, or
  other scalar identity, while its `support_refs[]` points at the owner or
  declaration line that produced the value.
- The grounded evidence is already value-bearing (`return`, `assignment`,
  `initializer`, or `string_literal`) and the accepted evidence summary/snippet
  contains the exact value, but the current member support check may only
  compare the member to the evidence anchor symbol / subject / object at the
  exact cited line.
- This incorrectly asks the model to re-emit evidence that the system already
  accepted, causing duplicate-evidence skips and closure-repair loops.

Design:

- Reuse the existing support-ref parser, location index, and
  `AnswerCodeSurfaceAppearsInText` identity-boundary matcher. Do not add a
  separate parser or any user/model prose keyword logic.
- Treat an accepted evidence item as member support when all of these hold:
  - the support_ref resolves to the same file:line location as the evidence;
  - the evidence is not ungrounded;
  - the evidence anchor kind is value-bearing:
    `return`, `assignment`, `initializer`, or `string_literal`;
  - the existing identity-boundary matcher finds the exact member surface in
    the accepted snippet or, for these value-bearing anchors only, in the
    accepted evidence summary.
- Keep the boundary conservative:
  - no semantic migration between different members;
  - no guessing from the closure reason or assistant prose;
  - no acceptance when the requested member value does not appear in the
    accepted value-bearing evidence text.
- This is language-neutral. The anchor kinds already cover Go, Java, Kotlin,
  JavaScript/TypeScript/ArkTS, Python, Rust, C/C++, Cangjie, config-like object
  initializers, and other repomap-supported languages through the shared
  evidence model.

Batch 20 task list:

- [x] Add a regression for owner-labeled support_ref plus returned literal
      member.
- [x] Add a negative regression proving a different member literal is still
      rejected.
- [x] Reuse `aggregateEvidenceTextContainsAnyLabel` with a value-bearing
      evidence text policy instead of adding a new matching path.
- [x] Run focused pre-complete tests, affected tool/types tests, `make build`,
      and full `go test ./...`.
- [x] Commit and push Batch 20.

## 2026-05-24 Batch 21 - Typed Set-Handoff Completion Convergence

Problem statement:

- After Batch 19/20 the local model can surface the right relation/enumeration
  evidence and can hand off value-literal members correctly, but explorer can
  still spend extra rounds widening search before it closes.
- The existing completion-ready lane is already the right mechanism: first emit
  an advisory close-now hint, then escalate through the existing schema/runtime
  restricted tool surface if the model ignores it.
- The observed gap is that the generic completion-ready signal is depth-phase
  only. Typed relation/enumeration questions can collect enough structured
  member evidence during the breadth/focused-search part of a dispatch, then
  keep widening solely because the convergence lane is not eligible yet.

Design:

- Reuse the existing `postCompletionReadySignal`,
  `postCompletionReadyEscalationSignal`, `FilterToolSchemas`, and
  `validateExplorerToolBoundary` lanes. Do not create a parallel hard-stop
  mechanism.
- Keep ordinary breadth exploration unchanged. Early convergence before depth
  is allowed only when the request carries a typed structured set-handoff
  obligation:
  - `RequiresRelationMemberSetHandoff`, or
  - the existing exhaustive-enumeration member-set handoff predicate.
- The same existing readiness gates must still pass:
  - a successful `emit_evidence` has occurred;
  - tool-source diversity, file coverage, and evidence-quality readiness are
    all satisfied;
  - there is a terminal evidence carrier.
- The hint must explicitly preserve the structured handoff contract by telling
  the model that the successful close needs `aggregate_facts.kind=member_set`.
- This is not a semantic answer decision. The system does not infer members
  from user text or assistant prose; it only changes when the already-existing
  close-now lane becomes eligible.

Batch 21 task list:

- [x] Add a regression proving typed relation set evidence can trigger
      completion-ready before depth phase.
- [x] Add a negative regression proving ordinary non-set breadth exploration
      still does not close early.
- [x] Extend `needsStructuredMemberSetHandoff` to reuse the relation handoff
      predicate as well as exhaustive enumeration.
- [x] Gate early completion-ready with typed set-handoff eligibility, not with
      prose keywords.
- [x] Run focused explorer tests, affected agent/tool/types tests, `make build`,
      and full `go test ./...`.
- [x] Commit and push Batch 21.

## 2026-05-24 Batch 22 - Analyzer Auto-Correction Recompile Contract

Problem statement:

- The focused local-model eval
  `.codrax/eval-batch21/qf_relation_subagent_registry-20260524-043606`
  found a search-width failure that survived the previous convergence fixes.
- The analyzer emitted three same-axis enumeration `sub_topics` for one
  logical set lookup. The R1.4 auto-correction correctly identified this as a
  single-axis enumeration, moved the sub-topic terms into analyzer hints, and
  cleared `RequestModel.SubTopics`.
- However, `TaskGraph` had already been compiled before the correction. Its
  stale evidence nodes (`n1_evidence_t0`, `n1_evidence_t1`,
  `n1_evidence_t2`) still represented the old sub-topics, so the scheduler ran
  multiple exploration lanes even though the corrected `RequestModel` no longer
  had multiple lanes.
- This is not a model-output compatibility issue alone. It is a deterministic
  IR consistency bug: after any accepted structural mutation of the
  `RequestModel`, every derived artifact must either be recomputed or proven
  unaffected.

Design:

- Add a shared analyzer-auto-correction rebuild step in the orchestrator.
  Existing R1.4 and R2.2 corrections remain the only callers; no new
  correction condition is introduced.
- Rebuild only deterministic artifacts derived from the corrected
  `RequestModel`:
  - `TaskGraph`
  - `EvidencePlan`
  - `AnswerContract`
  - `HypothesisSet`
- Reuse the same existing analyzer pipeline components rather than hand-editing
  the graph:
  - `compiler.Compile`
  - `hdp.Plan`
  - `compiler.RecomputeBudget`
  - `amplifier.AmplifyPostCompile`
  - `binder.BindByRelevance`
  - `counterfactual.Expand` when the corrected request still qualifies
- Preserve analyzer post-compile data that the orchestrator cannot safely
  regenerate without analyzer-only context:
  - carry forward an already-required diagram contract so user/model diagram
    intent is not lost;
  - preserve citation-free carve-outs when the previous contract had already
    removed the citation floor;
  - merge previously computed required-file hints with newly compiled hints.
- After rebuilding, write the corrected `RequestModel` back into
  `BusContext.Mutable` so downstream tools do not read pre-correction state.
- Keep the safety boundary:
  - no user-text or model-prose keyword matching;
  - no answer-content synthesis;
  - no change to prompt code;
  - no weakening of the existing gate. The gate is still re-run after rebuild,
    and mixed failures remain refused by the existing R1.4/R2.2 guards.

Batch 22 task list:

- [x] Add an orchestrator helper to rebuild analyzer-derived artifacts after a
      structural auto-correction.
- [x] Wire R1.4 and R2.2 auto-correct paths through that helper before
      `rerunAnalyzerGateReport`.
- [x] Update mutable request-model writeback after a successful correction.
- [x] Add regression coverage proving R1.4 collapse also removes stale
      `_t*` evidence lanes from `TaskGraph`.
- [x] Run focused orchestrator tests, `make build`, and full `go test ./...`.
- [x] Commit and push Batch 22.

## Open Questions

- Whether `emit_evidence.anchor_kind` auto-repair should run inside
  `emit_evidence` itself or in a shared grounding repair layer. Shared grounding
  is preferable if existing helpers already classify definition/call/return lines.
- Whether analyze-stage `read_file` should be silently compacted into a policy
  notice or surfaced visibly as a recoverable model mistake. The current visible
  behavior is debuggable but costs context and iterations.

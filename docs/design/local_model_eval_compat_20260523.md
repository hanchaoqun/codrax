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

## Open Questions

- Whether `emit_evidence.anchor_kind` auto-repair should run inside
  `emit_evidence` itself or in a shared grounding repair layer. Shared grounding
  is preferable if existing helpers already classify definition/call/return lines.
- Whether analyze-stage `read_file` should be silently compacted into a policy
  notice or surfaced visibly as a recoverable model mistake. The current visible
  behavior is debuggable but costs context and iterations.

# Read Relation Handoff and REPL Stability Plan

Date: 2026-06-16

## Scope

This plan covers the read-mode gaps exposed by customer feedback and the
adjacent UX issue observed in REPL status rendering:

- live dock row 3 repaints high-churn elapsed text next to the cancel hint;
- relation-qualified member questions can still degrade into a mechanism-only
  answer or a broad support-inventory answer;
- structured evidence found during exploration is not always carried with
  enough priority into finalizer consumption;
- tool-call JSON repair and retry hints need one shared mental model so models
  receive schema-local corrections instead of broad prose rewrites.

Write-mode behavior is not in this batch except where shared REPL/CLI status
rendering improves user experience. Do not touch write-controller semantics in
this delivery while another developer is actively editing write mode.

## Red Lines

- No raw user-keyword routing. Runtime control flow must consume typed analyzer
  fields, structured evidence rows, aggregate facts, answer surface plans, or
  schema-validated tool payloads.
- No parsing model scratch prose or final-answer prose for hard decisions.
- No Codrax-specific hard-coded answer members. The `explorer` customer example
  is a regression seed, not a rule.
- No hard gate from noisy evidence ranker scores, grep counts, or similarity
  heuristics. Those signals can only feed prompt guidance or advisory rows.
- Hard rejection is allowed only when a typed request shape, exact structured
  carrier, grounded same-member evidence, and local repair path are all present.
- Keep existing stable scenes: mechanism explanations remain prose-first unless
  typed fields declare a member set or a structured relation carrier creates an
  advisory boundary.

## Current System Inventory

The repository already has the right primitives. The fix should connect them
rather than build a parallel path.

- Analyzer contract: `internal/skill/analysis_contract.go` and
  `internal/tool/emit_analysis.go` define the typed predicates that later stages
  trust.
- Relation member-set gates: `types.RequiresRelationMemberSetHandoff`,
  `emit_investigation_complete` pre-complete checks, and explorer parse-output
  retry hints already require `aggregate_facts.member_set` for typed
  set-valued relation lookups.
- Exploration enrichment: `consumer_gate`, `bridge_literal`, typed relation
  rows, support refs, and aggregate facts already exist.
- Finalizer handoff: `renderAnswerDocRelationSurfaceHandoff`,
  `renderAnswerDocAggregateFacts`, `renderAnswerDocPrincipalMemberSetContract`,
  and `renderAnswerDocSubmissionChecklist` already feed finalizer.
- Unified answer-document repair: full emit and patch emit already converge on
  `runPreEmitChecksWithContext`, `emitFixHint`, `ToolRepair`, and
  `ApplyAndPersistMutation`.
- REPL dock: live status is centralized in `composeCurrentDockRows`,
  `composeDockRow3`, and the 100ms animation loop.

## Gap Analysis

### G1. Live Status Volatility

Row 3 currently combines elapsed timers with the interactive cancel hint. The
animation goroutine repaints the dock every 100 ms. Even though elapsed strings
are second-truncated, the row is still part of the animated region and the
visible timer changes every second during long read runs.

System gap: stable affordance text and progress timing are mixed in one live
line. Completion rows already carry elapsed totals, so live row 3 does not need
to be a timer when a cancel hint is visible.

### G2. Classifier Shape Ambiguity

Questions that ask "which member satisfies relation Y" can be semantically
set-valued even when phrased as an explanation or mechanism question. If the
analyzer emits only `intent=explain` / `question_kind=mechanism`, downstream
strict member-set gates correctly stay off. That preserves red lines, but it
also leaves finalizer to infer the principal member from support evidence.

System gap: the analysis contract does not strongly teach a language-neutral
"qualifying member by relationship" shape. The solution belongs in the typed
contract and schema descriptions, not in Go keyword matching.

### G3. Advisory Handoff Is Too Weak For Boundary-Rich Relation Rows

`consumer_gate` rows are structured and grounded, but finalizer currently sees
them as generic `flow_or_handoff` rows. That label does not make the critical
boundary visible: a caller-side gate plus registry identity evidence limits the
qualifying member set; global registration or dispatcher/runtime evidence is
supporting context, not a principal member set.

System gap: relation handoff preserves rows, but it does not prioritize
qualifying-member, gate, dispatcher, registry, and helper roles clearly enough
for answer writing.

### G4. Principal Relation Shape Check Only Protects Multi-Member Sets

`preCheckRelationMemberSetAnswerShape` currently enforces row carriers only for
multi-member relation `member_set` facts. Singleton relation answers are common
and high-risk: a one-member answer can disappear into prose while a larger
support inventory becomes the visible list.

System gap: singleton principal relation member sets need visibility checks
through the same typed pre-emit layer, while still avoiding hard gates when no
model-authored member_set exists.

### G5. Hints Still Spread Across Too Many Places

The system has a good pre-emit repair chokepoint, but relation-specific hints
are split across aggregate-fact prose, relation advisory prose, and structural
pre-checks. Models can miss the intended priority order.

System gap: the prompt/hint stack should present one priority ladder:
principal `member_set` > principal enumeration rows/support lanes > relation
role advisory rows > mechanism/background rows. Corrections should use
schema-local fields such as `blocks[].items[]`, not broad natural-language
rewrites.

## Target Design

### D1. Stable Live Dock Policy

When `cancelHint` is present, row 3 renders only the cancel affordance. Elapsed
time remains available in stage-completion and final-summary rows. Non-cancel
contexts may keep timer-only row 3.

This is a UI composition rule, not a scheduler change.

### D2. Typed Relation-Member Contract

The analyzer contract and `emit_analysis` schema describe this semantic shape:

- the answer is a set/count of principal members;
- those members qualify by a relationship to a named target;
- the classifier should set `is_relational_lookup=true`;
- if the answer asks for concrete member names, also set
  `is_category_enumeration=true`;
- if the answer asks for only a number, set `is_count_question=true`;
- `predicate_axis` carries the relation verb such as call/register/configure;
- entities may name the relation target before the actual members are known.

This remains model-authored typed output. Runtime code does not infer it from
request text.

### D3. Relation Role Priority Handoff

Finalizer receives relation rows with explicit role guidance:

1. principal relation `member_set` facts are the answer-member carrier;
2. principal enumeration rows/support lanes provide richer per-member notes and
   citations;
3. gate/consumer rows constrain qualification when grounded;
4. registry/binding rows prove population or identity;
5. dispatcher/runtime/tool-registration rows explain execution after a
   qualifying member emits or reaches the relation;
6. helper/interface/background rows cannot become principal members by
   themselves.

`consumer_gate` keeps advisory status unless a typed member_set exists, but its
role label and prompt text should prevent support inventory substitution.

### D4. Singleton Member-Set Visibility

Pre-emit checks require a direct visible carrier for every principal relation
member_set, including singleton sets, when the request shape requires relation
member handoff or the fact itself is marked principal. The local repair is a
schema fix: add an `ordered_list`, `bullet_list`, or `table` item/row carrying
the member before mechanism prose. No check fires without a structured
principal member_set.

### D5. Unified JSON/Hint Contract

Keep the existing `emitFixHint` / `ToolRepair` path. Add relation-member fixes
there rather than introducing another retry channel. Hints must state:

- exact schema field to change;
- expected carrier shape;
- why it preserves the typed answer contract;
- that the model should preserve existing facts and only adjust missing fields.

## Delivery Batches

### Batch 0 - Plan Artifact

- Add this design document.
- Commit and push independently.

### Batch 1 - REPL Live Row Stabilization

- Update `composeDockRow3` so `cancelHint` suppresses live elapsed segments.
- Keep timer-only behavior for contexts without a cancel hint.
- Update dock unit tests to pin both behaviors.
- Run `go test ./internal/render`.

### Batch 2 - Analyzer Typed Contract

- Update `internal/skill/analysis_contract.go` to describe
  relation-qualified member lookups in semantic terms.
- Mirror the concise schema wording in `internal/tool/emit_analysis.go`.
- Add tests that the rendered contract and schema expose the generic semantic
  rule without a runtime keyword router.
- Run focused analyzer/skill tests.

### Batch 3 - Relation Role Handoff Priority

- Extend `answerDocRelationSurfaceRole` / row scoring for structured
  gate-style evidence such as `Producer=consumer_gate`.
- Strengthen `renderAnswerDocRelationSurfaceHandoff` to explain the role
  priority ladder and the global-support-vs-qualifying-member boundary.
- Keep this section advisory when no principal member_set exists.
- Add tests using generic gate/registry/dispatcher evidence and the customer
  style registry case as a regression seed.
- Run `go test ./internal/agent -run 'RelationSurface|AnswerDocRelation'`.

### Batch 4 - Principal Relation Visibility At Pre-Emit

- Generalize `preCheckRelationMemberSetAnswerShape` from multi-member-only to
  every principal relation member_set that lacks a visible item/table carrier.
- Preserve the existing soft/hard split through `ViolationKind`.
- Ensure singleton prose mentions alone do not satisfy the carrier when the
  typed contract requires a member-set answer.
- Add tests for singleton relation set, multi-member relation set, and
  advisory-only relation rows with no member_set.
- Run focused `internal/tool` tests.

### Batch 5 - Integrated Verification

- Run all changed-package tests:
  - `go test ./internal/render`
  - `go test ./internal/skill ./internal/agent ./internal/tool ./internal/types`
- Run `git diff --check`.
- Audit the diff for raw request keyword routing and prose parsing.

## Acceptance Criteria

- The live REPL cancel row no longer includes changing elapsed text.
- Typed relation member questions have clearer classifier instructions without
  runtime keyword routing.
- Structured gate/registry/dispatcher evidence reaches finalizer with explicit
  role boundaries.
- Principal relation member sets, including singletons, render as visible
  answer-member carriers.
- All relation repairs go through the existing pre-emit chokepoint.
- No write-mode controller semantics are touched in this delivery.

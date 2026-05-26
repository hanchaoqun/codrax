# Repo Map Relation-Flow Navigation Contract (2026-05-26)

## Background

The Linux `io_uring` SEND/RECV call-chain probe exposed a systemic gap in the
repo-map navigation contract:

- The analyzer correctly emitted a relation-flow shape (`intent=trace`,
  `question_kind=call_chain`, `predicate_axis=call`), but also emitted a
  `source_inventory_profile` for functions.
- The explorer used `repo_map(view="task_map")`, then drifted to
  `source_inventory` and broad grep/read loops instead of the intended
  two-stage navigation (`task_map`/`file_map` -> narrow `relation_map`).
- A repair turn later attempted `read_file` when the active tool surface no
  longer exposed file-reading tools.

These are not Linux-specific problems. They are contract mismatches between
typed request shape, repo-map lens selection, tool-surface teaching, and
downstream source-inventory handoff.

## Red-Line Constraints

- Do not inspect raw user prose or model free text to decide control flow.
- Do not turn relation-flow questions into principal source inventories.
- Do not suppress genuine bounded inventory / enumeration questions.
- Relation-map rows are navigation candidates, not semantic citations.
- System supplements may guide or add clearly labeled notes, but must not
  replace model-authored answer content.
- The design must remain language-neutral across all repomap-supported
  languages.

## Root Cause

### G1: Relation-flow and source-inventory lanes were not mutually scoped

`emit_analysis` already drops `source_inventory_profile` for typed relation
member-set questions via `dropSourceInventoryProfileForTypedRelation`, but that
helper only covers relation membership / implementer-list shapes. A
single-topic structural trace (`intent=trace`, `predicate_axis=call`, or
`question_kind=call_chain`) is not a principal member-set question, so the
function inventory lane survived and later became finalizer prompt noise.

### G2: The explorer primer treats relation_map as optional instead of the
typed second-stage lens for relation-flow questions

The static repo-map primer mentions `relation_map`, but it does not explicitly
say that relation-flow/call-chain questions should prefer relation_map after
task/file orientation and before broad grep/read expansion. As a result models
can legally choose a broad inventory or grep path even when the typed request
shape has already selected relation candidates through `BuildTypedRelationQuery`.

### G3: Restricted repair turns can still tempt unavailable read tools

Some repair prompts ask the model to ground or repair already-visible evidence
while the narrowed tool surface excludes `read_file` / `grep` / `repo_map`.
When the previous context says "re-read" but the tool surface no longer exposes
file tools, models may attempt unavailable calls. The correct behavior is to use
only the visible line gutters / already accepted evidence or close with an
honest caveat.

### G4: Finalizer source-inventory handoff should be silent for relation-flow

If G1 leaks a source-inventory profile, finalizer receives a large
`source_inventory_handoff` even though the user asked for a flow. This can make
answers drier and increase contract noise without adding answer value.

## Design

### D1: Typed relation-flow source-inventory suppressor

Add a schema-only helper in `internal/types`:

`SourceInventoryIsPrincipalAnswerShape(rm RequestModel) bool`

It returns true only when source inventory is the principal requested answer
surface: category enumeration, count/scalar inventory, relational lookup where
the members themselves are requested, import/export inventory, or an active
explicit source-inventory profile that is not a single-topic structural trace.

Then extend `dropSourceInventoryProfileForTypedRelation` to also suppress
source inventory when:

- `types.IsSingleTopicStructuralTrace(rm)` is true, or
- `intent=trace` / `question_kind=call_chain` selects relation-flow through
  `BuildTypedRelationQuery`, while the predicates do not declare a principal
  enumeration/count/relational member-set.

This keeps true questions like "list all functions in package X" or "list all
implementers of interface Y" intact, while preventing "trace how A calls B" from
becoming a dry function inventory.

### D2: Relation-map as the typed second-stage navigation lens

Strengthen explorer model-visible guidance:

- For call-chain / relation-flow / structural-edge questions, after overview /
  task_map / file_map identifies candidate files or source symbols, prefer a
  narrow `repo_map(view="relation_map", sources=[...], scopes=[...],
  relation_kinds=[...])`.
- Use `source_inventory` for member/count checklists, not as a substitute for
  relation edges.
- Continue to verify selected semantic claims with `read_file` / targeted grep.

This is guidance, not a hard gate. It preserves model autonomy and avoids
over-constraining cases where relation_map has no index coverage.

### D3: Restricted repair-turn unavailable-tool teaching

In tool-surface error guidance and repair prompts, make the current tool list
the source of truth. If `read_file` is unavailable, the result should explicitly
say to use already-visible evidence, emit a caveat, or close the investigation
instead of retrying unavailable file tools.

### D4: Eval and regression guard

Add focused tests:

- `emit_analysis` drops source inventory for single-topic structural trace.
- `emit_analysis` preserves source inventory for real bounded inventory.
- `emit_analysis` preserves relation member-set behavior for implementer lists.
- Explorer repo-map primer documents relation-map two-stage navigation and keeps
  source_inventory scoped to inventories.

End-to-end validation:

- Re-run the Linux `io_uring` call-chain scenario and confirm:
  - no source-inventory handoff in finalizer prompt;
  - model is guided toward relation_map;
  - finalizer remains one-pass;
  - no user-facing answer richness loss.

## Task List

- [x] T0: Record root cause and design in this document.
- [x] T1: Add typed source-inventory principal-shape / relation-flow suppressor.
- [x] T2: Extend `emit_analysis` normalization and warnings.
- [x] T3: Strengthen explorer/analyzer repo-map relation-map teaching.
- [x] T4: Add tests for trace suppression, inventory preservation, and primer
      wording.
- [x] T5: Run targeted Go tests and full build.
- [ ] T6: Commit and push batch 1.
- [ ] T7: Re-run Linux relation-flow scenario and update this document with
      observed behavior.
- [ ] T8: If repair-turn unavailable-tool attempts remain, implement the
      restricted-tool teaching follow-up as batch 2.

## Progress Log

- 2026-05-26: Created after Linux `io_uring` repo-map experiment. The first
  root cause is confirmed in code: `dropSourceInventoryProfileForTypedRelation`
  only checks `HasTypedRelationMemberSetShape`, while
  `IsSingleTopicStructuralTrace` already models the missing trace/flow boundary.
- 2026-05-26: Implemented `SourceInventoryProfileConflictsWithRelationFlow`,
  extended `emit_analysis` normalization with a trace/relation-flow warning,
  tightened analyzer/explorer repo-map teaching, and added focused tests for
  trace suppression, true inventory preservation, and relation-map primer
  wording. Targeted tests passed:
  `go test ./internal/types -run 'TestSourceInventoryProfileConflictsWithRelationFlow|TestHasTypedRelationMemberSetShape'`;
  `go test ./internal/tool -run 'TestEmitAnalysis_Execute_DropsSourceInventoryFor(RelationFlow|TypedRelation)|TestEmitAnalysis_Execute_PersistsSourceInventoryProfile'`;
  `go test ./internal/agent -run TestExplorer`.
- 2026-05-26: Full validation passed: `go test ./...` and `make`.

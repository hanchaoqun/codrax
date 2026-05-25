# Relation Dossier Advisory Layer

## Goal

Support arbitrary customer-repository relation reasoning without hard-coding a
single product's relation vocabulary. The system should help the model notice,
carry, and verify relation directions, while the model remains responsible for
choosing which semantic relation answers the user's intent.

## Redlines

- Do not infer relation intent from raw user keywords or assistant prose.
- Do not synthesize answer members or rewrite the model's answer.
- Do not turn repo_map/source_inventory/typed graph candidates into final facts.
- Do not add default repository-specific authority providers.
- Do not hard-reject on relation completeness unless an explicit authority
  provider defines an exact source of truth and local repair path.

## Structured Carriers

The first implementation reuses existing wheels:

- `TypedRelationHint`: repository-index relation candidates such as implements,
  extends, called-by, references, imports, exports, configures, routes-to, and
  source-anchor. These are advisory candidates.
- `EvidenceItem`: model/tool-authored relation observations when subject and
  object are both present. Citable rows are verified observations; recovered or
  ungrounded rows remain leads.
- `AnswerAggregateFact` with relation-shaped `member_set` members: model-authored
  candidate or complete relation sets. They preserve count/list identity but
  still need member-level evidence or explicit support refs for user-visible
  claims.
- `SourceInventoryObservation`: model-driven repo-lens observations with
  machine-checkable counts, roles, scoped members, and row-local attributes.
  These are navigation checklists, not answer members.

No raw prompt text, localized wording, final-answer prose, or tool-result prose
is parsed for this layer.

## Prompt Contract

Render a compact `Relation Dossier (advisory)` section when at least one
structured carrier exists. The section must say:

- it is advisory only;
- candidates should guide next verification, not become final claims by
  themselves;
- verified evidence rows can be cited through the normal citation path;
- unknown/partial candidate sets should be caveated rather than completed by the
  system.

## Commercial Design

- Bounded output: cap typed relation groups, evidence observations, aggregate
  relation sets, and per-row examples.
- Stable ordering: typed hints, source-inventory observations, model evidence,
  then model aggregate sets. This exposes navigation first while preserving
  model-authored observations and member sets in the same dossier.
- Cross-repo and cross-language: all file/language/scope data comes from existing
  relation carriers; no Go-only assumptions.
- External observations: source-anchor relations from logs, traces, git, command
  output, web, MCP, and connectors stay advisory until linked to current-source
  evidence or origin-specific answer support.
- Future authority opt-in: a domain can add a `structuredRelationAuthorityProvider`
  only after documenting exact source, trigger carriers, repair path, and
  no-trigger tests.

## Task List

- [x] Document advisory relation dossier design and redlines.
- [x] Add prompt section title and deterministic render order.
- [x] Render dossier from typed relation hints, source-inventory observations,
      relation evidence, and relation aggregate facts.
- [x] Add tests proving raw objective text alone does not render a dossier.
- [x] Add tests proving typed hints are advisory and not hard authority.
- [x] Add tests proving model-authored relation evidence/aggregate facts flow
      into downstream context.
- [x] Run focused and package tests.

## Eval Follow-up: Completion Payload Salvage And Close-Ready Restraint

2026-05-25 focused evals showed two generic gaps that are not specific to
`LoopController` or this repository:

1. Some models put sibling tool fields at the end of a text field, e.g.
   `reason="... confidence: high, result_kind: resolved, aggregate_facts: [...]"`.
   The payload is structurally recoverable because the misplaced fragments are
   exact schema fields, but the existing recovery only handled the inverse shape
   where `aggregate_facts` was a string with sibling fields appended.
2. Close-ready and exact-absence nudges can still out-rank the model's
   structured relation evidence. Once a positive exact target is already backed
   by defining evidence, absence-oriented same-family prompts are noise. Once a
   first soft-stop is already close-ready, generic partial-function-read nudges
   should not override the model's decision to close unless there is a typed,
   load-bearing blocker.
3. Count aggregate facts can carry a valid scalar `value` while the model
   accidentally includes sample `members[]` whose length does not equal the
   value. This is structurally recoverable for count facts because the safe
   repair is to omit the partial members and preserve the model-authored count.
   It is NOT safe for `member_set`, where `members[]` is the exact answer set.
4. A later reconcile-only scheduler window can reopen exploration even after a
   successful `emit_investigation_complete` with grounded evidence and
   model-authored `member_set`. In the 2026-05-25 focused relation eval, the
   first accepted closure carried 8 production members, then reconcile
   dispatched a second explorer because `has_enough_facts` was false; the
   second explorer produced a narrower 7-member set and overwrote the stronger
   handoff. This is not a LoopController-specific issue: any exact relation,
   inventory, route/config, class hierarchy, multi-repo, or external-artifact
   investigation can be harmed if a post-closure advisory reconcile step is
   allowed to replace an accepted structured closure.
5. Some providers serialize a text tool call as a keyed payload wrapper such as
   `{ "emit_analysis_payload": { ...schema fields... } }`. This is a generic
   transport shape, not an answer-specific issue. If the key is derived from a
   currently exposed tool name plus a neutral suffix (`payload`, `params`,
   `arguments`, `input`, etc.), the recovery layer can safely map it back to
   that tool and let the normal schema validator decide whether the payload is
   valid.
6. Some providers also serialize the arguments object itself as the whole
   assistant message, e.g. `{ "entities": [...], "intent": ... }` during the
   analysis lane. Existing schema-aware normalization only runs after the
   content has been classified as a tool call, so the missing piece is transport
   classification, not another JSON repairer.

### Generalized Design

- Extend `emit_investigation_complete` payload compatibility at the tool
  boundary, not in prompts:
  - inspect only schema field names (`confidence`, `result_kind`,
    `aggregate_facts`, `absence_justification`) inside the `reason` suffix;
  - require at least two recoverable sibling fields before treating the suffix
    as a misplaced tool payload;
  - parse successfully before mutating anything;
  - use recovered values only when the top-level field is missing or invalid;
  - trim the recovered field suffix from `reason` so the preserved conclusion
    stays user-visible and does not carry JSON noise.
- Keep this repair structural-only:
  - no answer members are invented;
  - no user question keywords or assistant prose semantics are parsed;
  - invalid or ambiguous fragments fall through to existing validators.
- Restrain explorer close-ready behavior:
  - suppress exact-absence hints when grounded defining proof for the exact
    target already exists;
  - on a first voluntary soft-stop that is already close-ready, skip generic
    partial-read prompts. The model may close with
    `emit_investigation_complete`; deterministic validators still protect
    truly missing structured handoff.
- Normalize partial count members at the shared aggregate-fact boundary:
  - for `total_count`, `unique_count`, `grouped_count`, and `bucket_count`, when
    `value` is numeric and `members[]` is present but incomplete, drop
    `members[]`/`member_notes[]`, preserve `value`, and record provenance;
  - do not apply this count-member repair to `member_set`; exact answer members
    stay on the existing `member_set` canonicalization path. `excluded_count`
    keeps using its existing partial-exclusion normalizer;
  - disclose the normalization in tool summaries so the model and user can see
    that the count was kept while partial samples were not promoted to facts.
- Treat an accepted, structured investigation closure as enough for a
  reconcile-only scheduler node, but only inside strict machine-checkable
  boundaries:
  - the investigation-complete policy is `soft` or `override`;
  - `MutableState` has a current or retained accepted closure;
  - existing evidence context exists (`EvidenceItem`, flow finding, answer
    chain/symbol, accepted aggregate fact, or accepted closure artifact);
  - mixed-origin required lanes, pending validation targets, stage retries,
    load-bearing pending reads, and blocking repairs are all absent;
  - relation-chain enrichment pending reads (`chain_promotion.*`) are advisory
    for reconcile-only auto-complete after accepted closure, because they are
    downstream navigation/synthesis leads rather than proof that the accepted
    closure is false. Primary-anchor, required-file, multi-path, and other
    pre-complete load-bearing origins still block;
  - success criteria are still evaluated, but `has_enough_facts` may be
    satisfied by the accepted closure for this reconcile auto-complete decision
    only.
  This does not synthesize facts, does not inspect raw user/model prose, and
  does not skip real blockers. It only prevents a reconcile-only advisory window
  from replacing a model-authored structured closure that already passed the
  tool gate.
- Extend text tool-call recovery for keyed payload wrappers:
  - generate aliases from exposed tool names only, e.g.
    `emit_analysis_payload`, `emit_analysis_params`, `analysis_payload`;
  - preserve exact tool-name precedence if a real tool with that exact name is
    ever exposed;
  - recover only complete JSON objects and then run the existing schema pruning
    and tool validation path. Invalid payloads still fail normally.
- Extend bare-argument recovery for whole-message structural emit payloads:
  - only when `recover_text_tool_calls` is enabled and no real protocol tool
    call was returned;
  - only for complete JSON objects that uniquely match a schema-rich `emit_*`
    tool by the existing schema scorer;
  - keep ordinary tools such as `grep` and `read_file` explicit in auto mode,
    so user-requested JSON prose is not swallowed as a guessed tool call;
  - after classification, reuse the existing schema pruning, key aliasing, and
    tool validator path.

### Follow-up Task List

- [x] Add reason-suffix schema-field recovery for `emit_investigation_complete`.
- [x] Add no-false-positive tests proving ordinary `confidence:` prose is not
      promoted.
- [x] Suppress exact-absence same-family nudges after positive defining proof.
- [x] Suppress first-soft-stop partial-read prompts when the branch is already
      close-ready.
- [x] Normalize partial members on count aggregate facts without touching exact
      `member_set` payloads.
- [x] Run focused tool/agent tests.
- [x] Re-run the focused relation evals after implementation.
- [x] Auto-complete reconcile-only scheduler nodes from accepted structured
      closure when all blockers are absent.
- [x] Add DAG regression coverage proving accepted closure without an explicit
      `HasEnoughFacts` signal does not reopen exploration.
- [x] Add a monotonicity regression proving chain-promotion enrichment debt is
      advisory for reconcile-only closure but load-bearing anchor debt still
      blocks.
- [x] Recover tool-name keyed payload wrappers such as
      `emit_analysis_payload` without adding prompt-specific logic.
- [x] Recover whole-message bare argument objects for uniquely matched
      structural `emit_*` tools in compatibility mode, reusing the existing
      schema-aware path.

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

# Structured Relation Authority Handoff

## Background

The focused `read_combo_pipeline_sequence_table` run exposed a generic
handoff gap rather than a prompt problem. Exploration had model-authored,
structured aggregate `member_set` facts for the read pipeline stages, agents,
and state carriers, but the stage/agent member sets lacked `support_refs`.
The completion tool then dropped those optional decorated member sets before
finalization. Finalizer produced an otherwise accepted answer, but the visible
answer compressed away the `explorer` and `extractor` actor bindings.

This must not be fixed by matching the eval question or by forcing a finalizer
rewrite. The safe root fix is to make structured relation authority evidence
materialize upstream when the model itself has already emitted a relation-shaped
candidate surface and the system has a machine-known authority source.

## Redline Boundaries

- Do not infer user intent from raw user keywords or model prose.
- Do not synthesize answer members or rewrite the model's answer.
- Do not hard-reject unless all of these are true:
  - the relation surface comes from structured model output or typed evidence;
  - a provider can name an exact authority source;
  - the authority source is machine-verifiable;
  - the repair path is local: read the authority file or emit evidence from an
    already-read authority file.
- If no provider can prove the authority, keep the existing behavior: advisory
  or caveat, not forced rewrite.

## Generalized Design

Add a provider-style pre-complete check called structured relation authority
handoff. A provider consumes only structured carriers:

- `aggregate_facts.member_set.members`
- `EvidenceItem.Subject`
- `EvidenceItem.Object`
- `EvidenceItem.AnchorSymbol`
- existing repo/authority graph data

It does not inspect raw user text or free-form assistant prose.

Provider contract:

1. Detect relation sides from structured carriers.
2. Require enough signal on both sides to avoid single-symbol noise.
3. Return authority files and rationale only when the source is exact.
4. If the authority file is unread, enqueue a `RepairReadFile`.
5. If the file was read but no evidence from that file was emitted, enqueue
   `RepairEmitEvidence`.
6. If authority evidence already exists, do nothing.

The first concrete provider is codrax's built-in stage-to-agent binding because
the repository already has an exact authority table `types.AllStageBindings()`
in `internal/types/stage_binding.go`, and finalizer already has a gated
supplement that only renders after this source is grounded.

The stage-to-agent relation itself is not claimed to be universal across
customer repositories. The reusable part is the provider contract above:
relations may block completion only when an exact authority provider exists.
The same pattern can later cover route-to-handler, config-key-to-reader,
service-to-entrypoint, module-to-public-type, and cross-repo interface binding
when those relations have their own machine-verifiable authority sources.

## Stage-Agent Provider Trigger

Trigger only when structured carriers include both:

- at least two known stage identifiers or values from `types.AllStageBindings`;
- at least two known agent identifiers or values from `types.AllStageBindings`.

The provider accepts surfaces such as `StageExplore`, `explore`,
`AgentExplorer`, `explorer`, and decorated members like
`AgentExplorer ("explorer")`, but only from structured fields. It does not
trigger on ordinary prose.

## Relation Provider Catalog

This audit separates "navigation relation" from "authority relation." A
navigation relation may help the model decide where to look. An authority
relation may block completion only when it has exact provenance and a local
repair path.

| Relation family | Current carrier | Languages / scope | Current use | Authority status |
|---|---|---|---|---|
| built-in stage -> agent -> skill | `types.AllStageBindings()` plus `internal/types/stage_binding.go` | codrax runtime topology only | pre-complete authority handoff, then finalizer supplement after evidence lands | Authority-eligible because exact machine table exists |
| implements / extends | repo_map graph through `TypedRelationCandidateSource` | repo_map supported languages with type graph edges | prompt hint and typed coverage checks | Not a pre-complete authority provider yet; hard checks still require exact carrier + model-authored member_set + grounded evidence |
| called-by / references | repo_map graph relation candidates | repo_map supported call/reference graph languages | prompt hint and change-impact navigation | Navigation-first; do not block completion without an exact request-scope contract |
| imports / exports | repo_map import graph | all languages where repo_map extracts imports | source inventory / dependency navigation | Navigation-first; import inventory may have its own exact universe coverage, not this provider |
| registers | accepted `EvidenceRegistration` rows | language-neutral evidence | typed prompt hint from evidence | Evidence itself is the authority; no extra pre-complete provider needed |
| configures | accepted config/key evidence rows | config files plus source languages | prompt hint for config trace / mapping | Prompt-only until a per-framework canonical config authority exists |
| routes-to | accepted route/handler evidence rows | route frameworks and source languages | prompt hint for route/handler lookup | Prompt-only until route registry authority can be proven per framework |
| source-anchor | observation ledger | logs, traces, git, command output, external docs, MCP/connectors | links external observations to current-source anchors | Prompt/navigation only; external observation is not a current-source citation by itself |
| directory/package/module membership | source_inventory / repo_map lens | multi-repo, cross-language | candidate universe and checklist guidance | Can support exact universe coverage when scope/count/member_set are machine-verifiable, but not relation-authority blocking |

Guardrail: adding a row to `TypedRelationKind` or source_inventory does not make
it authority-eligible. To become a blocking authority provider, a relation must
also define:

- exact source of truth;
- exact trigger carriers;
- minimum signal threshold;
- read/materialize repair path;
- unit tests for no-trigger cases;
- an explicit statement that prompt hints remain advisory.

## Non-Goals

- Do not convert all typed relation hints into completion blockers.
- Do not use the stage-agent provider for customer repository architecture.
- Do not infer route/config/service relations from naming conventions alone.
- Do not turn source_inventory suggestions into a read whitelist.
- Do not append system-authored answer members when the model did not emit or
  ground the relation.

## Model-Guided Relation Follow-Up

Customer repositories contain arbitrary business relations that no static
system table can enumerate: feature flag -> experiment arm, task -> worker,
message topic -> subscriber, state -> transition, protobuf field -> handler,
database table -> DAO, and many others. Trying to hard-code these would violate
the user-intent redline and would always be incomplete.

The durable strategy is:

1. **System covers common relation carriers.** Keep a small set of common,
   language-neutral carriers: typed graph relations, import/export edges,
   evidence-backed registration/config/route rows, source_inventory membership,
   and external-observation source anchors.
2. **Model chooses the semantic relation.** The model decides which candidate
   relation matters for the user's question. The system must not infer this
   from raw keywords or free-form model prose.
3. **System turns the chosen direction into next-action guidance.** When the
   model emits structured evidence, aggregate facts, or a tool call that reveals
   a relation direction, the system may provide an advisory dossier:
   - candidates already seen;
   - source/provenance and precision;
   - known ambiguity;
   - whether the set is complete, partial, or unknown;
   - suggested next tool calls to narrow scope or verify a candidate.
4. **No authority, no blocking.** If the relation cannot be exhaustively proven
   by a provider, the dossier is advisory. It may ask the model to verify,
   caveat, or continue narrowing, but it must not fabricate a member set or
   block completion as if the system knew the complete answer.

This preserves model agency while still lowering model cognitive load: the
system exposes structured navigation and verification affordances, but the
model decides how those affordances apply to the user's actual intent.

### Follow-Up Dossier Task

- [x] Reuse existing `TypedRelationHint` / source_inventory rendering where it
      already carries candidates, precision, provenance, and counts.
- [x] Keep the first implementation as prompt/context guidance, not a new
      completion gate. The existing prompt pool can carry partial/unknown
      relation candidates without introducing a parallel dossier format.
- [x] Trigger relation guidance only from structured carriers:
      tool calls/results, source_inventory observations, typed relation hints,
      evidence rows, or aggregate facts.
- [x] Never trigger it from raw user text or assistant prose.
- [x] Add tests proving that unsupported relation shapes do not become hard
      completion blockers.
- [x] Add prompt-context tests proving typed relation candidates are advisory
      and are not described as authoritative citation grounding.
- [ ] Add a separate lightweight dossier only if future eval data shows the
      unified evidence pool cannot express partial/unknown relation coverage
      without noise.

## Prompt / Context Contract

Structured relation guidance enters the model through the existing unified
knowledge pool and source_inventory lens. The model must be able to use these
rows to decide its own next actions, but the system must not make the relation
semantic choice for it.

The prompt contract is:

- `llm_evidence` rows are model/tool-authored observations and may ground
  claims through the normal citation path.
- `typed_graph`, `typed_observation`, and other `typed_*` rows are navigation
  candidates unless the model verifies them through normal evidence/citation
  flow.
- Typed rows may suggest likely members, files, or relation edges, but they are
  not a user-visible answer and not a reason to rewrite the model's answer by
  themselves.
- A relation provider may become a hard authority only after it documents an
  exact source of truth, minimum trigger signal, and local repair path.

This keeps common relation carriers useful across mechanism, architecture,
enumeration, route/config, cross-language, multi-repo, and external-artifact
questions while still letting the model choose the relation that matters for
the user's intent.

## Task List

- [x] Document the root cause and provider boundary.
- [x] Add the pre-complete structured relation authority handoff provider.
- [x] Queue unread authority files through existing `RepairReadFile` and
      `PendingRead` machinery.
- [x] Queue already-read-but-unemitted authority files through existing
      `RepairEmitEvidence` machinery.
- [x] Preserve current optional `member_set` drop behavior when no authority
      provider can prove the relation.
- [x] Add regression tests for:
      - unread authority file is requested;
      - read authority file without emitted evidence asks for `emit_evidence`;
      - grounded authority evidence allows completion;
      - single-side or unsupported relation member sets do not trigger.
- [x] Re-run focused unit tests.
- [x] Re-run the focused failing eval.

## Expected Effect

The finalizer no longer needs to guess or remember a complex relation from
weak handoff state. If the model has already surfaced a relation-shaped
candidate and a machine authority exists, the authority becomes normal evidence
before completion. If no machine authority exists, the system stays quiet and
trusts the model rather than inventing a stricter contract.

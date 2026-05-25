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
- [ ] Re-run the failing eval.

## Expected Effect

The finalizer no longer needs to guess or remember a complex relation from
weak handoff state. If the model has already surfaced a relation-shaped
candidate and a machine authority exists, the authority becomes normal evidence
before completion. If no machine authority exists, the system stays quiet and
trusts the model rather than inventing a stricter contract.

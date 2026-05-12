# Aggregate Facts and Forced-Read Convergence

## Background

The 2026-05-12 REPL architecture-view run exposed two separate but related
handoff/convergence gaps:

1. `emit_investigation_complete.aggregate_facts` rejected a model-authored
   `member_set` because `value` was omitted, even though the same structured
   payload already contained the complete `members` array.
2. The forced-read pre-complete gate repeatedly demanded unread slivers of
   very large symbols such as `Orchestrator.Run` and `dispatchStage`, even
   after the model had grounded the load-bearing call sites needed for an
   architecture/sequence answer.

Both failures share one architectural lesson: system gates should consume
precise typed handoffs, and should not ask the model to repeat work after a
typed handoff is already sufficient.

## Red Lines

- Do not synthesize user-facing answer content from raw tool output, grep
  output, read-file prose, or closure reasons.
- System canonicalization is allowed only over model-authored structured data.
  For example, deriving `member_set.value = len(member_set.members)` is valid
  because both the set and its members were emitted by the model in the tool
  payload.
- Hard gates must read precise typed signals: enum kind, explicit members,
  file/line ranges, grounding status, claim form, facet/principal lane, and
  exact integer comparisons. No hard gate may use noisy keyword ranking or
  semantic similarity.
- Any broad forced-read escape must preserve commercial grounding quality:
  enough citable principal evidence must already exist, and unresolved exact
  targets / absence / exhaustive enumeration obligations must still be blocked.

## Workstream A: Aggregate Fact Canonicalization

### Problem

`AnswerAggregateFact` currently requires `Value` before normalizing `Members`.
This rejects the most natural `member_set` payload shape:

```json
{
  "kind": "member_set",
  "label": "Pipeline stages",
  "members": ["analyze", "explore", "extract", "finalize"]
}
```

The retry is wasteful and often causes the model to drop lower-priority
aggregate facts while fixing the schema error.

### Design

For `kind=member_set` only:

- Normalize `members` first.
- If `value` is empty and `members` is non-empty, set `value` to
  `strconv.Itoa(len(members))`.
- Keep the existing cardinality validator: if `value` is present and does not
  equal `len(members)`, reject.
- Keep all non-`member_set` count-like facts strict. A `total_count` or
  `unique_count` with omitted `value` remains invalid because its `members`
  may be samples rather than the full set.

### Tests

- `member_set` without `value` is accepted and stored with canonical count.
- `member_set` with mismatched explicit `value` is still rejected.
- `total_count` without `value` remains rejected.

## Workstream B: Principal Anchor Coverage for Forced Reads

### Problem

`multipath.EvaluateAnchor` currently runs symbol coverage before evidence
coverage. For any question-related symbol found in a file, L1 demands coverage
of the whole symbol definition span plus context. This is correct for small
symbols and unresolved exact targets, but it is too strong for large functions,
large structs, and architecture/control-flow explanations:

- A single method may span hundreds of lines.
- The answer may only need the load-bearing call/guard/assignment lines.
- The model can already emit grounded principal/facet evidence at those lines.
- The gate still demands the rest of the symbol body, causing repeated
  pre-complete downgrades and moving completion out of reach.

### Design

Add a precise, typed short-circuit before the L1 symbol-span demand:

1. Compute the question-related symbols as today.
2. If those symbols exist, check whether every demanded symbol is already
   represented by citable grounded/recovered model-authored evidence in the
   same file.
3. Coverage must come from typed evidence surfaces:
   - `EvidenceItem.AnchorSymbol`
   - `EvidenceItem.Subject`
   - `EvidenceItem.Object`
   - `EvidenceItem.SurfaceTerms`
   - `EvidenceItem.OwnerSymbol`
4. The evidence must be answer-grade enough to cite:
   - same file,
   - `LineStart > 0`,
   - grounding status is `grounded` or `recovered`,
   - grounding tier is line/source grade,
   - producer is model-authored or LLM-emittable evidence.
5. If every question-related symbol is covered this way, skip the symbol-span
   forced read with a specific `SignalEvidenceVerified` reason. Otherwise keep
   the existing L1 surgical demand.

This does not bypass ordinary grounding. It only says: once the model has
already emitted citable typed evidence for the symbol surfaces that caused the
forced-read demand, the system should not additionally require reading the
entire enclosing symbol span.

### Non-Goals

- Do not make raw `read_file` or `grep` output satisfy the gate.
- Do not infer missing answer members from surrounding source.
- Do not weaken exact-resolution, absence, count, or exhaustive-member
  contracts. Those have separate typed gates.
- Do not add language-specific symbol-name heuristics. The same surface-match
  logic must work for Go, C/C++, ArkTS, Cangjie, Java/Kotlin, JS/TS, Python,
  Rust, paths, routes, macros, and package/module labels.

### Tests

- A large symbol with grounded model-authored evidence for the demanded symbol
  skips whole-span forced read.
- A large symbol with only raw read coverage but no emitted evidence still
  demands the missing span.
- Partial coverage of one symbol among several still demands the uncovered
  symbols.
- Existing EOF clamp, surgical drain, small-file, and keyword-anchor tests
  remain unchanged.

## Delivery Order

1. Land this design document.
2. Implement Workstream A with focused unit tests and commit/push.
3. Implement Workstream B with `multipath` unit tests and full `go test ./...`,
   then commit/push.
4. Re-run a same-shape architecture/sequence request and audit logs for:
   - no `member_set value is required` retry,
   - no repeated whole-span forced-read loop after principal evidence exists,
   - finalizer receives aggregate facts and citable principal evidence without
     system-authored answer completion.

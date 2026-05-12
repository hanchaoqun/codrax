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
   represented by strict-citable model-authored evidence in the same file.
3. Coverage must come from typed evidence surfaces:
   - `EvidenceItem.AnchorSymbol`
   - `EvidenceItem.Subject`
   - `EvidenceItem.Object`
   - `EvidenceItem.SurfaceTerms`
   - `EvidenceItem.OwnerSymbol`
4. The evidence must be answer-grade enough to cite:
   - same file,
   - `LineStart > 0`,
   - `EvidenceItem.IsCitable()` is true,
   - grounding tier is line/source grade,
   - producer is an `emit_evidence` lane and the evidence kind is
     LLM-emittable.
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

## Workstream C: AnswerSymbol Definition-Line Canonicalization

### Problem

The same-shape architecture run showed a downstream extractor loop after
Workstreams A/B had converged. Turn A emitted useful evidence for symbols such
as `Tool`, `BuildAgentContext`, `TaskState`, and `Message`, but several entries
pointed at the first line of a doc comment rather than the executable/type
definition line. `emit_answer_symbol` then rejected the slate because the cited
line was comment-only, while the extractor had no read tools available to
re-open the file and locate the adjacent definition.

This is not a `Tool`-specific problem. It appears anywhere a model carries a
model-authored structured symbol `{name, file, line, kind}` from a documentation
anchor into an answer-symbol channel whose contract requires the definition
line.

### Design

Extend the shared grounding verifier, not the extractor prompt:

- `ground.VerifyLineAnchor` must prefer a non-comment line that bears the
  anchor within the ordinary ±radius window.
- If the claimed line is a pure comment, the verifier may scan forward through
  the directly-adjacent doc-comment block and return the first non-comment line
  only when that line contains the same typed anchor.
- The caller (`emit_answer_symbol`) must store the matched line returned by the
  verifier, so the persisted `AnswerSymbol.Line` becomes the canonical
  definition line.
- No new answer item may be invented. This is only field canonicalization over a
  model-authored structured item plus already-read source text.
- The lookahead is bounded and adjacency-based. It must not search the whole
  file or use noisy similarity.

### Tests

- `VerifyLineAnchor` maps a Go doc-comment line to the adjacent `type Tool`
  definition line.
- `emit_answer_symbol` accepts a doc-comment line for `Tool` and persists the
  definition line.
- Existing strictness remains: unrelated comments and non-adjacent definitions
  still reject.

## Workstream D: Sub-topic Entity Surface Typing

### Problem

After Workstream C, the same architecture/sequence request still burned two
analyzer attempts. The first attempt marked the request as cross-component
without emitting `sub_topics`. The second attempt repaired that but used file
surfaces (`orchestrator.go`, `context.go`, `explorer.go`) as sub-topic
entities. The subtopic-coherence R1.5 gate treated those as failed repo-symbol
lookups and rejected them, even though they were valid FileIndex members and
were already listed as high-confidence required files.

This is the same principal/context boundary in another layer: answer-bearing
objects can be symbols, files, paths, packages, modules, routes, or config
keys. A hard gate that assumes every entity must be a symbol causes retry
storms and may pressure the model into inventing symbol names.

### Design

- Keep `normalizer.SymbolResolver.LookupSymbol` as the symbol lane.
- Add an optional `LookupFileSurface(surface)` capability on resolver
  implementations. The gate consumes it only through an interface assertion, so
  tests and older adapters remain nil-safe.
- A file surface resolves when:
  - the exact repo-relative path is in FileIndex, or
  - a basename / suffix resolves to exactly one FileIndex entry.
- Ambiguous basenames return no hit. A hard gate must not treat noisy path
  guesses as proof.
- Reuse the central cross-language code/config suffix table, so Go, C/C++,
  Cangjie, ArkTS/ETS, Java/Kotlin, JS/TS, Python, Rust, Swift, Lua, Proto, and
  declarative config files share the same policy.
- Multi-repo resolution works against active sub-repos and returns the
  path-from-parent form, still requiring uniqueness across the active set.

### Tests

- R1.5 accepts mixed symbol-backed and file-backed sub-topics.
- File-surface lookup accepts exact paths and unique basenames across Go,
  ArkTS/ETS, Cangjie, and C++ examples.
- Ambiguous basenames return no file hit and therefore cannot satisfy a hard
  coherence gate.

## Workstream E: Diagram Kind/Body Coherence

### Problem

The final answer emitted a `diagram` block whose Mermaid body started with
`sequenceDiagram`, while the structured `diagram.kind` was `architecture`.
Existing validators allowed this when the family diagram plan was advisory,
because plan-kind mismatch is intentionally relaxed for optional diagrams.
That relaxation is correct for semantic family choice, but the body syntax and
declared semantic family still cannot contradict each other.

### Design

- Validate the diagram block's own structured payload before edge grounding:
  - `sequenceDiagram` body requires `diagram.kind=sequence`.
  - `flowchart` / `graph` body is compatible with `flow`, `architecture`, or
    `call_dag`, but incompatible with `sequence`.
- The check is structural and precise: it reads the Mermaid directive in
  `diagram.body`, not the user request text and not a fuzzy label.
- Keep optional diagram family flexibility: architecture answers may still use
  flowchart syntax; call DAG answers may still use flowchart syntax.
- Emit a finalizer-local repair so the model reuses existing citations and
  fixes only the diagram payload.

### Tests

- `sequenceDiagram` body + `architecture` kind fails.
- `flowchart` body + `architecture` kind passes.
- `graph` body + `sequence` kind fails.

## Workstream F: Precise Semantic Quality Promotion

### Problem

The semantic-quality reviewer correctly observed that the final answer declared
the `current_code_path` facet multiple times but anchored it zero times. The
same reviewer also noticed typed evidence (`PipelineStage`, `Run`,
`runAnalyzePhase`, `runTaskGraph`, `runReadSchedulerLoop`, `IsWriteGraph`,
`stageMapping`, `builtinStageBindings`) was not surfaced in prose. However,
`answer_semantic_underfilled` is soft by default, so the answer shipped with a
thin and partly wrong topology narrative.

Making every richness concern strict would overfit and create false positives.
The hard boundary must use typed runtime state, not subjective reviewer taste.

### Design

- Keep ordinary `richness_gap` as soft.
- Promote only reviewer concerns that align with precise typed gaps:
  - `coverage_gap` / `grounding_gap` becomes a strict
    `principal_prose_underfilled` violation when it targets a promoted facet
    whose `DeclaredCount > 0` and `AnchoredCount == 0`.
  - `diagram_gap` becomes strict `diagram_edge_unsupported` only when the
    typed diagram contract is required and the block is absent/empty or edge
    minimums are not satisfied.
  - `topic_mismatch` remains the existing high-severity
    `answer_topic_mismatch` path.
- Matching concern topics to facets uses the typed facet id / public label
  projection supplied to the reviewer. It does not scan the user's raw text for
  keywords.
- The repair locus remains finalizer-only: the evidence and citations already
  exist; the model must re-render the answer around them.

### Tests

- Shallow promoted facet coverage gap promotes to strict
  `principal_prose_underfilled`.
- `richness_gap` stays soft even when a promoted facet exists.
- Required diagram edge shortfall promotes to strict
  `diagram_edge_unsupported`.

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
5. Implement Workstream C if the same-shape run shows doc-comment-to-definition
   drift in the AnswerSymbol slate, then re-run focused and full tests before
   commit/push.
6. Implement Workstreams D/E/F together because they are all runtime
   classification/finalizer boundary fixes surfaced by the same E2E.
7. Re-run focused tests, full `go test ./...`, then the architecture/sequence
   E2E to confirm analyzer retries and final answer quality improve without
   system-authored answer completion.

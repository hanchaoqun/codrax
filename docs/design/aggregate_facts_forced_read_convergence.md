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
- If `value` is present as a non-negative integer but does not equal
  `len(members)`, canonicalize it to `len(members)`. For `member_set`,
  `members[]` is the model-authored exact set and `value` is a derived
  cardinality, so this is a structural repair rather than answer synthesis.
- Keep non-integer `value` invalid and keep `member_set` without members
  invalid.
- Keep all non-`member_set` count-like facts strict. A `total_count` or
  `unique_count` with omitted `value` remains invalid because its `members`
  may be samples rather than the full set.

### Tests

- `member_set` without `value` is accepted and stored with canonical count.
- `member_set` with mismatched numeric explicit `value` is accepted and stored
  with canonical count from the structured members.
- `total_count` without `value` remains rejected.

### 2026-05-19 extension: source-line decorators

The same post-fix eval also showed a related compatibility gap: the model
emitted exact members such as `foo (L123)`, but omitted `support_refs`. Because
the line decorator is already structured on the member, the tool now fills
`support_refs` from already-read `read_file` gutter lines when all of these
precise checks pass:

- the qualifier is a single source-line marker (`L123`, `line 123`, `第123行`)
  or a file-line marker (`file.go:123`, `path/to/file.go:123`);
- the already-read gutter contains exactly one matching file:line;
- that line is not a comment and contains the member's code identity as a
  bounded code surface.

Non-line decorators such as `(8 checks)` or `(runtime bucket)` still require
explicit support refs or ordinary typed evidence.

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

## Workstream G: Diagram Syntax Profile Registry

### Problem

Workstream E fixed one observed contradiction (`sequenceDiagram` body with
`diagram.kind=architecture`), but a local switch still leaves the same class
of bug open for future diagram families. Users can ask for many visual shapes;
the system cannot safely enumerate those requests in prompts or validators one
case at a time.

### Design

Introduce a `DiagramSyntaxProfile` registry in `internal/types`:

- `DiagramKind` remains the semantic answer family.
- `MermaidSyntaxFamily` is the concrete body directive family.
- Each current semantic kind declares compatible Mermaid families:
  - `flow` -> flowchart / graph syntax,
  - `sequence` -> sequenceDiagram syntax,
  - `call_dag` -> flowchart / graph syntax plus code-endpoint semantics,
  - `architecture` -> flowchart / graph syntax.
- Known Mermaid directives whose edge semantics are not currently parsed
  (`classDiagram`, `stateDiagram`, `erDiagram`, C4, etc.) are classified as
  `unsupported`, not silently accepted.
- Validators consume `DiagramKindAllowsMermaidSyntax` and
  `DiagramKindUsesCodeEndpoints` instead of local kind switches.

This makes support extensible: adding a new diagram type means adding a typed
profile, edge parser semantics, prompt surface, and tests in one place. It does
not pretend the current system can fully support arbitrary Mermaid directives
without typed parser/validator support.

### Tests

- Every current `DiagramKind` has a syntax profile.
- `sequenceDiagram` is accepted only for `sequence`.
- `flowchart` / `graph` is accepted for `flow`, `architecture`, and `call_dag`.
- Known unsupported Mermaid directives are rejected with a rewrite/omit repair
  instead of slipping past edge grounding.

## Workstream H: Evidence Grounding Definition-Line Canonicalization

### Problem

The architecture/sequence E2E still showed `emit_evidence` repeatedly turning
`runReadSchedulerLoop` back into the first doc-comment line. Workstream C fixed
`emit_answer_symbol`, but the earlier evidence grounding path still trusted
repomap's symbol line directly. When a parser reports the doc-comment start as
the symbol line, typed evidence becomes citable yet points at prose instead of
the definition.

### Design

Reuse the shared `ground.VerifyLineAnchor` path inside evidence grounding:

- When graph-based definition recovery returns a symbol line, canonicalize it
  through the already-read source line index.
- If the graph line is a doc comment and the adjacent definition line contains
  the same model-authored anchor, rewrite the evidence line to that definition
  line.
- If the graph line is a doc comment and no adjacent definition with the same
  anchor exists, do not accept the comment as recovered proof.
- When no line index exists, preserve the old graph-only behavior; the system
  must not invent line corrections without source text.
- Snippet attachment for definition-like anchors uses the same verifier so the
  support lane carries the executable/type line, not the doc-comment sentence.

This is a grounding canonicalization over model-authored structured evidence
plus already-read source text. It does not synthesize answer content.

### Tests

- `emit_evidence` at a doc-comment line canonicalizes to the adjacent function
  definition line.
- Same-file graph recovery from an unrelated claimed line also canonicalizes to
  the adjacent definition line when repomap reports the doc-comment line.
- Existing graph-only recovery stays unchanged when no read-file line index is
  available.

## Workstream I: Principal Surface Profile Extension Point

### Problem

`MemberSurface` already separates symbol-like, display-label, and
source-location answer members. That covers today's import/path/config-role
and architecture evidence, but future answer families may introduce other
visible principal surfaces such as routes, macros, spans, protocol messages, or
aggregate buckets.

### Design

Keep the boundary typed and extensible:

- Principal-surface classification must be derived from `ClaimForm`,
  relation orientation, aggregate fact kind, and evidence fields.
- Symbol oracles only guard symbol-like surfaces.
- Display-label/source-location surfaces satisfy answer obligations through
  grounded member labels and citations, not symbol lookup.
- Future surfaces should extend the profile/classification layer first, then
  inherit the existing support-lane and `MissingPrincipalSupportMembers`
  validators.

This is the general answer to "can the system enumerate every user ask": no,
but it can force each new answer-surface family to declare its typed profile
before it can influence hard gates.

## Workstream J: Read-Mode Shell Mutation Gate

### Problem

The architecture/sequence E2E exposed an L1 red-line failure. A reconcile
dispatch was routed through `StageExplore`, where `exec_command` is exposed for
deterministic read-only computations. The model used free-form shell heredocs
to write `docs/ARCHITECTURE.md` and PlantUML files in read mode. Prompt text
already said "do not modify files", but a prompt-only prohibition cannot
enforce byte preservation.

### Design

Make `exec_command` read-mode safe at the tool execution boundary:

- Any non-write pipeline stage must pass a deterministic read-only shell
  validator before execution.
- Write stages (`StagePlan`, `StageApply`, `StageVerify`,
  `StageWriteAnalyze`) preserve their existing escape hatch because they run
  under the write-mode/worktree contract.
- The validator permits common read-only command pipelines used for counts and
  inspection (`find ... | wc -l`, `grep`, `rg`, `git status`, `git diff`,
  `sed -n`, `awk`, `sort`, `uniq`, etc.).
- It rejects precise shell mutation surfaces:
  - output/input redirection and heredocs (`>`, `>>`, `<<`, `2>`, `&>`),
  - command substitution,
  - mutating command names (`mkdir`, `rm`, `mv`, `cp`, `tee`, shells,
    language interpreters, etc. via allowlist exclusion),
  - mutating options on otherwise read-oriented tools (`find -delete`,
    `find -exec`, `sed -i`, unsafe `git` subcommands).
- The gate is lexical and quote-aware, so operators inside quoted grep/printf
  patterns do not false-trigger.

This is a hard gate over the exact shell string the model authored. It is not a
semantic keyword match on the user request, and it does not synthesize answers.

### Tests

- Read mode rejects heredoc writes and does not create the target file.
- Read mode rejects `mkdir`, `tee`, `find -delete`, `git clean`, and `sed -i`.
- Read mode allows read-only pipelines and quoted operator characters.
- Write-stage exec remains available for worktree-contained apply/verify
  commands.

## Workstream K: Cross-Language Assignment / Member Initializer Evidence

### Problem

The latest architecture/sequence E2E showed `AnchorAssignment` still behaves as
if every assignment-like fact were `:=` or `=`. That is too narrow for the
languages Codrax supports:

- Go struct/composite literals: `EntryConditions: critEntry(...)`
- TypeScript / ArkTS object literals: `entryConditions: buildEntry(...)`
- C / C++ designated initializers: `.entry_conditions = build_entry(...)`
- Rust / Swift / Kotlin / Java / Cangjie property or named-field
  initialization surfaces
- config/object formats where the visible key is the principal surface

When these facts cannot be represented as assignment evidence, the explorer
either emits a nearby helper/call anchor or the finalizer loses a useful
source-grounded detail. That is a cross-language evidence-carrier gap, not a
Go struct-literal corner case.

### Design

Make repomap/tree-sitter the primary carrier and keep lexical fallback precise:

- Add typed line features for assignment-like source shapes:
  `assignment` for variable/property assignment statements and
  `member_initializer` for field/object/designated initializer entries.
- Map language-specific AST node names to those features inside repomap
  extraction. ArkTS rides the TypeScript grammar; Cangjie emits the same
  feature through its Go-native parser/scanner path because no tree-sitter
  grammar is authoritative for Cangjie.
- `ground.AnchorAssignment` consumes those features first. When the cited
  non-comment line has a typed assignment/member-initializer feature, that line
  is a valid assignment-shaped anchor; the model-authored fields/snippet still
  carry the user-visible subject/value surface downstream.
- If AST features are absent because the file fell back to a lower parse tier,
  use a structural line-syntax fallback that recognizes assignment operators,
  map/object/struct field entries, and designated initializers. The fallback
  reads only the model-authored `EvidenceItem` plus already-read source text; it
  never scans the user request or fabricates answer content.
- Keep symbol oracles out of this path. A field label, YAML key, object member,
  route label, or designated initializer is a source-surface assignment fact,
  not necessarily a repo symbol definition.

### Tests

- Go composite literal field assignment grounds on both the field label and RHS
  helper call.
- ArkTS / TypeScript object literal member entries ground as assignment facts.
- C / C++ designated initializers ground as assignment facts.
- Cangjie-style named member/property initialization has a covered fallback
  path until the parser emits the typed feature directly.
- Comment-only lines mentioning the same surface do not ground assignment
  evidence.
- Existing `:=` / `=` assignment grounding stays unchanged.

## Workstream L: Alias-Resolved Subtopic Coherence

### Problem

The analyzer still sometimes emits conceptual aliases such as
`read_scheduler_loop` / `write_scheduler_loop` while the repo contains
`runReadSchedulerLoop` / `runWriteSchedulerLoop`. The second attempt usually
recovers, but the first attempt retry is a symptom of a typed resolution gap:
subtopic coherence has exact, flat, file-surface, and role-suffix resolution,
but no bounded action-prefix alias lane.

### Design

Add a resolver lane that is stricter than fuzzy search:

- Normalize the analyzer surface and repo symbols by case/separator.
- Accept only a small set of structural action-prefix variants on repo symbols
  (`run`, `build`, `make`, `new`, `create`, `load`, `parse`, `handle`) when
  the remaining flat form equals the analyzer surface.
- Require a unique match before the hard coherence gate treats the entity as
  resolved.
- Keep this lane inside the resolver/gate boundary only; it is a typed
  existence proof for coherence, not a canonical answer-member rewrite.

### Tests

- `read_scheduler_loop` resolves uniquely to `runReadSchedulerLoop`.
- Ambiguous prefix matches do not resolve.
- Short stems and ordinary prose surfaces do not resolve.

## Workstream M: Diagram Preference Propagation

### Problem

The diagram validator now catches semantic/body mismatches, but the finalizer
still sometimes emits `diagram.kind=architecture` with a `sequenceDiagram`
body on the first try. That means the schema boundary is correct while the
first-pass preference signal is too weak.

### Design

Keep the validator as the hard boundary and improve upstream preference
ordering:

- Preserve explicit `DiagramHint` ordering when the current evidence supports
  the requested kind.
- When the user asks for a sequence diagram and the grounded architecture
  evidence is the only available seed, present that as "sequence requested,
  architecture seed available" instead of encouraging an architecture kind with
  sequence syntax.
- Continue to reject unsupported Mermaid families at runtime through the
  diagram syntax profile registry.

### Tests

- Explicit sequence hint remains the first preferred diagram kind when support
  exists.
- `architecture` + `sequenceDiagram` mismatch still triggers validator repair.
- Unsupported Mermaid directives remain rejected rather than silently rendered.

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
8. Implement Workstream G before adding more diagram prompt variants, so any
   future diagram syntax goes through the same profile/validator/test surface.
9. Implement Workstream H before further E2E tuning; otherwise support-lane
   richness remains vulnerable to doc-comment line drift.
10. Implement Workstream J before running further real E2E evals; otherwise
    read-mode validation can pollute the user's checkout while investigating.
11. Implement Workstream K before more field/config/count evals; otherwise
    cross-language assignment facts continue to fall through to helper anchors.
12. Implement Workstreams L/M as bounded first-pass quality improvements after
    K, keeping all hard gates on typed resolver/profile signals.

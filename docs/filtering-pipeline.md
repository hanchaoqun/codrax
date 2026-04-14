# 过滤流水线快照 / Filtering Pipeline Snapshot

> **目的 / Purpose** — 把 `docs/architecture-root-cause-remediation.md` §4
> 清点出的 11 条后置过滤器 / 验证器 / 清洗器 / 修正器，画成一张明确
> 的执行顺序 DAG，并把它们之间**隐式的 load-bearing 依赖**写下来。
> 这是 P0.3 的产出物，目标根因 **R6**。
>
> Documents the 11 post-hoc filters/validators/scrubbers inventoried in
> `architecture-root-cause-remediation.md` §4 as an explicit execution
> DAG, making the implicit load-bearing dependencies between them
> reviewable. This is the P0.3 artefact; it targets root cause **R6**.
>
> **Status**: descriptive snapshot, **not** a redesign. No code change
> accompanies this doc. When a filter is added, moved, or removed,
> update this doc *in the same commit*.
>
> **HEAD at write**: post-P0.2 ship `41f4b61`. P0.3 is this doc.
>
> **What this is NOT**:
> - NOT a spec — the code in `internal/agent/evidence.go`, `explorer.go`,
>   `finalizer.go`, `agent.go`, and `internal/tool/blob.go` is the
>   authority.
> - NOT a redesign — see P1.2 / P2.3 in the remediation doc for the
>   actual plan to replace this accretion with a typed DAG.
> - NOT a ship gate — "matches this DAG" is not a green-light
>   criterion for new code. Manual inspection rules.

---

## 0. Reading order

1. Skim §1 for the three data channels and the ASCII DAG.
2. Read §2 for the filter inventory table (use as a lookup).
3. Read §3 for the ordering invariants — these are the load-bearing
   edges that a new filter must not break.
4. Read §4 for the fail-open list — every entry here is a place where
   the next fake-green can slip through without triggering any signal.
5. Read §5 before adding a new filter.

---

## 1. Three data channels

The 11 filters do not form one pipeline. They form **three parallel
pipelines**, one per data channel, plus a small number of cross-channel
edges. The three channels are:

- **A. Request channel** — the user question string, threaded
  through REPL memory + analyzer rewrites into `AgentContext.CurrentTask`.
- **B. Tool output channel** — raw tool bytes from `tools.Execute()`,
  stored as `types.ToolResult` and accumulated into the LLM message
  history.
- **C. Evidence → Answer channel** — structured `[]types.EvidenceItem`
  plus the free-text `StageOutput.Data` prose (see R1), feeding the
  explorer synthesis prompt and then the finalizer prompt.

### ASCII DAG

```
 user question                 tool invocation             LLM-authored notes
      │                               │                            │
      ▼                               ▼                            ▼
 ┌──────────┐                 ┌──────────────┐           ┌───────────────────┐
 │  F1      │                 │  F3  StoreBlob│           │  (no filter here — │
 │ stripConv│                 │  F3' HeadOnly │           │   prose → fact     │
 │ (REPL)   │                 │  (blob cap)   │           │   drift slips       │
 └────┬─────┘                 └──────┬────────┘           │   through; see R2) │
      │                              │                    └─────────┬─────────┘
      ▼                              ▼                              │
 explorer task                  ToolResult record                    │
                                      │                              ▼
                                      ▼                      ┌──────────────┐
                                ┌───────────────┐             │  F4          │
                                │  F2           │             │ parseEvidence│
                                │ pruneToolHist │             │   Items       │
                                │ (150KB roll)  │             │ (regex →      │
                                └──────┬────────┘             │  EvidenceItem)│
                                       │                      └──────┬────────┘
                                       └──┐                          │
                                          │                          ▼
                                          │                 ┌─────────────────┐
                                          │                 │  F5              │
                                          │                 │ groundEvidence   │
                                          │                 │  Items           │
                                          │                 │ (line check →    │
                                          │                 │  /ungrounded)    │
                                          │                 └──────┬───────────┘
                                          │                        │
                                          │                        ▼
                                          │                 ┌─────────────────┐
                                          │                 │  F6              │
                                          │                 │ mergeEvidence    │
                                          │                 │  Items           │
                                          │                 │ (StableID dedup) │
                                          │                 └──────┬───────────┘
                                          │                        │
                                          │                        ▼
                                          │                 ┌─────────────────┐
                                          │                 │  F7              │
                                          │                 │ rankEvidenceBy   │
                                          │                 │  Relevance       │
                                          │                 │ (reorder)        │
                                          │                 └──────┬───────────┘
                                          │                        │
                                          │                        ▼
                                          │                 ┌─────────────────┐
                                          │                 │  F8              │
                                          │                 │ filterEvidence   │
                                          │                 │  ByPrimaryFiles  │
                                          │                 │ (mechanism only, │
                                          │                 │  fail-open)      │
                                          │                 └──────┬───────────┘
                                          │                        │
                                          │                        ▼
                                          │                 synthesis prompt
                                          │                        │
                                          │                        ▼
                                          │                 ┌─────────────────┐
                                          │                 │  F9              │
                                          │                 │ scrubSibling     │
                                          │                 │  EvidenceBlocks  │
                                          │                 │ (prose channel,  │
                                          │                 │  mechanism only) │
                                          │                 └──────┬───────────┘
                                          │                        │
                                          └──────┐                 ▼
                                                 │         finalizer prompt
                                                 │                 │
                                                 ▼                 ▼
                                          ┌─────────────────────────────┐
                                          │  F10                         │
                                          │ Prompt soft-constraints      │
                                          │  (explorer Rules, finalizer  │
                                          │   Hard constraints blocks)   │
                                          └──────────────┬───────────────┘
                                                         │
                                                         ▼
                                          ┌─────────────────────────────┐
                                          │  F11  Answer-shape validator │
                                          │   family (S3 layer):         │
                                          │   • outOfListSymbols         │
                                          │   • validateStepList         │
                                          │   • validateValue            │
                                          │   • validateBoolean          │
                                          │   • validateConfigValue      │
                                          │   retry via ContinuationPrompt│
                                          └──────────────────────────────┘
```

Explanatory edges (not drawn above to keep the DAG readable):

- F4 reads `e.investigationNotes` which is **the same slice** that the
  synthesis prompt later scrubs via F9 — so F4 and F9 share an input,
  but F4 runs in `ensureStructuredEvidence` and F9 runs inside
  `SynthesisPrompt` much later. Both read, neither writes back, so
  there is no mutation ordering — but any *new* filter that *mutates*
  `e.investigationNotes` between these two read sites would break both.
- F7 runs **twice**: once in `ParseOutput` at `explorer.go:1563` (for
  stage-output ranking) and once in `SynthesisPrompt` at
  `explorer.go:1846` (for synthesis digest). Both calls are idempotent
  under the current ranker; changing the ranker to be non-idempotent
  would silently corrupt the second call's output.

---

## 2. Filter inventory (post-P0.2)

| # | Channel | Name | File:line | Catches | Action | Data form |
|---|---|---|---|---|---|---|
| F1 | Request | `stripConversationPrefix` | `internal/agent/explorer.go:65` | REPL memory pollution in objective string | Prefix strip | string / regex |
| F2 | Tool output | `pruneToolHistory` | `internal/agent/agent.go:268` | LLM message history > 150KB | Replace old tool messages with stub | struct / byte |
| F3 | Tool output | `StoreBlob` | `internal/tool/blob.go:93` | Per-tool output > inline cap (~32KB) | Spool to disk + inline preview | struct / size |
| F3' | Tool output | `StoreBlobHeadOnly` | `internal/tool/blob.go:128` | Same, head-only variant (for tools that truncate mid-content catastrophically) | Spool + head-only preview | struct / size |
| F4 | Evidence | `parseEvidenceItems` | `internal/agent/evidence.go:59` | (Not a filter in the strict sense; extractor) Markdown → `EvidenceItem` | Regex parse + intra-batch dedup via `mergeEvidenceItems` | regex → struct |
| F5 | Evidence | `groundEvidenceItems` | `internal/agent/evidence.go:320` | LLM line-number hallucination | Clear `LineStart`, tag `Producer` with `/ungrounded` (does NOT drop) | struct (2-tier validator) |
| F6 | Evidence | `mergeEvidenceItems` | `internal/agent/evidence.go:237` | Duplicate evidence across sources (LLM notes / concrete values / mechanism / dataflow) | Dedup on `StableEvidenceID` hash | struct (hash) |
| F7 | Evidence | `rankEvidenceByRelevance` | `internal/agent/evidence.go:847` | Low-relevance items pushed into top-N | Reorder + weighted bonus (does NOT drop) | regex / string heuristics over struct |
| F8 | Evidence | `filterEvidenceByPrimaryFiles` | `internal/agent/explorer.go:614` (called at `:1583`) | Non-primary-file evidence leaking into mechanism-kind finalizer prompt | Drop; **fail-open on 0 survivors** | struct (source-path match) |
| F9 | Prose channel | `scrubSiblingEvidenceBlocks` | `internal/agent/evidence.go:633` (called at `explorer.go:1891`) | Sibling-file `## Evidence from` markdown blocks inside investigation notes reaching synthesis prompt | Drop block | string / regex (markdown headers) |
| F10 | Prompt | Explorer Rules block + Finalizer Hard-constraints block | `internal/agent/explorer.go:1058-1076`, `internal/agent/finalizer.go:88-136` | Format / answer-shape violations expressible as English rules | Prompt soft constraint (no enforcement) | string / prompt |
| F11 | Answer | `outOfListSymbols` + shape-validator family | `internal/agent/finalizer.go:276` (S3 core), `internal/agent/finalizer_validators.go` (P0.2 shape validators), dispatch at `finalizer.go:169` / `:194`, retry via `ContinuationPrompt` at `finalizer.go:190` | Finalizer shape violations: out-of-list symbol names (`list_of_symbols`); insufficient step count vs `[CONDITIONAL]` branches (`step_list`); missing/mis-typed value literal (`value`); missing YES/NO/是/否 prefix (`boolean`); missing key/value/file:line triple (`config_value`) | Inject correction prompt + retry ≤2×; on exhaustion **fail LOUD** via `validationFailed` (P0.2 red line) — `list_of_symbols` legacy path still silent-accepts | struct (allowed set + regex shape predicates) |

**Tallies (post-P0.2)**:

- By channel: **1 Request, 3 Tool, 5 Evidence, 1 Prose, 1 Prompt, 1 Answer** = 12 slots if you count F3' as distinct, 11 if you fold it into F3. The `architecture-root-cause-remediation.md` §4 table counts 11; this doc keeps that number by listing F3/F3' as one row.
- By data form: 6 string/regex/prompt (F1, F4, F7, F9, F10, half of F11) + 5 struct/schema (F2, F3/F3', F5, F6, F8, other half of F11). R2 is the reason the string-form half is where the parser-class bugs keep happening.
- By action: 6 drop (F3, F3', F8, F9, part of F11, parts of F6) + 3 rewrite (F5, F1, parts of F11) + 2 retry (F2, F11) + 1 reorder (F7).
- By layer order in the DAG: F1 → F2/F3/F3' → F4 → F5 → F6 → F7 → F8 → (synthesis prompt inserts prose) → F9 → F10 → F11.

---

## 3. Ordering invariants (the load-bearing edges)

These are the implicit ordering constraints that make R6 load-bearing.
Each line reads "**A must run before B because …**". Breaking any of
these silently corrupts downstream state with no compile-time or
test-time signal.

1. **F1 (stripConversationPrefix) must run before entity extraction.**
   Reason: without it, REPL prior-conversation prefixes get scanned as
   user-authored entities and contaminate the analyzer's rewrite. See
   the REPL-equivalence audit memo.
2. **F4 (parseEvidenceItems) must run before F5 (groundEvidenceItems).**
   Reason: F5's input signature is structured `EvidenceItem` with
   `LineStart/Source/Subject`. If F4 is bypassed, F5's grounding
   operation has no input.
3. **F5 (groundEvidenceItems) must run before F6 (mergeEvidenceItems).**
   Reason: `mergeEvidenceItems` dedups on `StableEvidenceID`, and
   `Producer` (which F5 suffixes with `/ungrounded`) is a hash input.
   Dedup-before-ground would collapse grounded and ungrounded variants
   of the same item into one, losing the tag.
4. **F6 (mergeEvidenceItems) must run before F7 (rankEvidenceByRelevance).**
   Reason: the ranker's stable sort depends on deterministic set
   membership; running rank first and merge second produces a different
   top-N than the reverse, because merge can remove items whose
   rank-order would otherwise appear in top-N.
5. **F7 (rank) must run before the top-18 truncation inside
   `formatEvidenceItems` at `internal/context/builder.go`.**
   Reason: truncation is positional.
6. **F8 (filterEvidenceByPrimaryFiles) is the *structured-channel* half
   of the primary-file scope narrowing. F9 (scrubSiblingEvidenceBlocks)
   is the *prose-channel* half.** They must **both** run, because one
   alone leaks through the other channel. This is the
   `architecture-root-cause-remediation.md` R1 symptom: two filters on
   two channels to solve the same semantic problem.
7. **F9 runs inside `SynthesisPrompt` and must run before the notes are
   written into the synthesis digest.** If F9 runs after digest
   construction (or a new filter is inserted between F9 and the digest
   writer), sibling-file blocks leak into synthesis prose and the t3
   drift regresses.
8. **F11 runs strictly last.** It operates on the finalizer LLM's
   response text, not on evidence — any filter placed *after* F11
   would operate on the answer handed back to the user and would
   (a) require a second LLM round trip, (b) subvert the P0.2
   fail-loud contract.
9. **F10 prompt-level rules must NOT be relied on as a substitute for
   any of F1..F9 or F11.** This is a rule about intent, not about order:
   every time a prompt rule is added where a structural filter would
   have worked, it encodes an R3 regression. (See the P0.2 ship record:
   its entire motivation was converting F10 soft constraints for
   step_list/value/boolean/config_value into F11 struct validators.)

---

## 4. Fail-open behaviour

These are places where a filter silently lets a bug through. Every
entry here is a place where the next fake-green can slip through
without triggering any signal.

| Filter | Failure mode | Why it fail-opens | Downstream effect |
|---|---|---|---|
| F5 `groundEvidenceItems` | LLM cites the wrong line | `LineStart` cleared + `/ungrounded` tag appended to `Producer`; item is NOT dropped | Finalizer honoring the /ungrounded contract is prompt-soft (F10) until P0.1 made it a renderer-visible tag (shipped). Still not a hard reject. |
| F8 `filterEvidenceByPrimaryFiles` | 0 items survive the primary-file filter | Explicit fail-open at `explorer.go:1583-1591` — returns the unfiltered set | Non-primary evidence reaches the finalizer on an "unusual" investigation. Marked as acceptable to unblock small cases; audit when grid variance rises. |
| F11 `outOfListSymbols` | LLM continues to emit banned symbols after 2 retries | Loop exits at `finalizer.go:221-225` after `maxFinalizerCorrectionRetries` with a debug log but **accepts the violation silently** (legacy S3 path) | Banned symbols reach the user. The P0.2 shape validators (`validateStepList`, …) deliberately do NOT silent-accept — they set `validationFailed` and `ParseOutput` appends a ⚠️ honesty note. The `list_of_symbols` path is still legacy until it is lifted into the P0.2 discipline. |
| F9 `scrubSiblingEvidenceBlocks` | Gated by `question_kind == "mechanism"` | `enumeration` / `call_chain` / `dataflow` questions legitimately span multiple files and must not be scrubbed | Any mis-classified mechanism question (analyzer says "enumeration" when it should say "mechanism") silently skips F9 and leaks sibling blocks. |
| F4 `parseEvidenceItems` | LLM writes evidence markdown in a shape the parser does not recognise | Regex match count drops to 0; nothing downstream; the prose still reaches synthesis via F9's input slice | Parser-class bug family. Two instances tracked so far: `c04298f→ba081db` (ERM entity pollution) and `133973d` (markdown-backtick source header). `memory/project_evidence_as_tool_refactor_deferred.md` tracks the counter; threshold = 3. P1.1 (`emit_evidence` tool) is the planned replacement. |
| — (no filter) | Pure prose→fact drift inside investigation notes (the t5 `slice/total` hallucination case) | No filter in any channel matches this shape | F5 catches it only *indirectly* when the prose contains a line number. Pure prose with no cite slips through. Addressed only by P1.2 (`StageOutput.Data` deterministic rendering) + P2.2 (`AnswerDocument` renderer). |

---

## 5. Adding a new filter — checklist

Before adding a filter, ask:

1. **Is this symptom already covered by a filter I can tighten?** Most
   R6-class regressions come from adding a new filter next to an
   existing one instead of fixing the existing one. F8 and F9 are a
   live example: they solve the same problem on two channels and are
   the textbook case of "added rather than unified."
2. **Which channel does it belong to?** A filter that crosses channels
   is a code smell — prefer placing it where its input originates.
3. **What are its predecessors and successors?** Add new ordering rows
   to §3 *in the same commit* that adds the filter. If you cannot name
   both predecessors and successors, you do not yet understand the
   placement.
4. **Is it fail-open or fail-loud?** State this explicitly in the §2
   row and the §4 table. Default to fail-loud unless there is a
   documented reason otherwise. Silent accept is a P0.2 red line and
   the honesty-over-cleverness memo is the governing feedback rule.
5. **Does it depend on LLM-authored prose structure?** If yes, you are
   about to add a parser-class bug to the inventory. Re-read
   `memory/project_evidence_as_tool_refactor_deferred.md` and consider
   whether the new filter should be inside the (eventual) `emit_evidence`
   tool contract instead.
6. **Is it a substitute for P1 / P2 work?** If the filter exists to
   paper over R1 (two channels) or R2 (LLM writer==reader), it is a
   band-aid and should be tagged as such in its source comment, linked
   to the root cause it belongs to.
7. **Overfitting audit (mandatory, `memory/feedback_no_overfitted_solutions.md`).**
   Reverse test, deletion test, class test, no-bait test, no-contamination
   test — before code, not after.

---

## 6. Cross-references

- `docs/architecture-root-cause-remediation.md` — the full 1035-line
  bilingual remediation roadmap. §4 is the source of the 11-filter
  inventory; §5 R6 is the reason this doc exists; §6 P0.3 is the
  specification this doc is fulfilling.
- `docs/architecture.md` — the pipeline-topology reference (the 5
  layers, the 8 stages). This doc assumes that context.
- `memory/project_fake_green_audit_2026_04_14.md` — post-ship grid
  record and the pattern→fix map. Several §3 invariants trace back to
  patterns named there.
- `memory/project_p0_2_shipped.md` — the P0.2 validator-family ship
  record, including the fail-loud contract that F11's P0.2 branch
  implements.
- `memory/project_p0_1_shipped.md` — the P0.1 `/ungrounded` renderer-
  tag ship, which closes the F5 → F10 soft-constraint gap for
  downstream honoring.
- `memory/project_evidence_as_tool_refactor_deferred.md` — parser-class
  bug counter (currently 2/3) and the P1.1 preconditions.
- `memory/feedback_no_overfitted_solutions.md` — mandatory audit gate.
- `memory/feedback_honesty_over_cleverness.md` — the rule behind F11's
  P0.2 fail-loud branch.

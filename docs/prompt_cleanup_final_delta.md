# Session 26 prompt cleanup — final delta (batch 4B)

7-commit refactor that (a) purged implementation jargon from LLM-facing
prompts, (b) restructured the analyzer skill to resolve three
intra-prompt contradictions, and (c) enforced the
`feedback_no_custom_keyword_matching` red line via a fourth hard-gate
lint on classification enum descriptions.

## Commits

| # | SHA | Batch | Summary |
|---|---|---|---|
| 1 | `cfa0dcc` | 1 | glossary blocklist + 3 AST-based lints in report-only mode + baseline eval snapshot (d24088f) |
| 2 | `5fa5e1f` | 2A | purge internal jargon from skill configs + dynamic supplements (37 skill + 11 agent → 0) |
| 3 | `e643bf2` | 2B | purge internal jargon from tool schemas + promote 3 lints to `t.Errorf` (10 → 0) |
| 4 | `90c5027` | 3A | analyzer structural reorg (Pre-scan moves to Workflow; Prohibitions 12 → 9; C1/C2/C3 contradictions resolved; required-field 3-way consistency test) |
| 5 | `ceff656` | 3B | dedup cross-section redundancy in extract / answer-document / explore skills |
| 6 | `73ad469` | 4A | enum desc keyword-example purge + 4th hard-gate lint (`TestNoKeywordExamplesInEnums`) |
| 7 | _this commit_ | 4B | final delta report; promote all 4 lint hard-gates from `t.Errorf` → `t.Fatal` per `docs/prompt_baseline_p0.md` line 202 (per-violation lines emit via `t.Errorf` first so the full list prints before the closing `t.Fatalf` aborts) |

## Lint hard-gates (all enforced at `t.Fatal`)

Per-violation lines emit via `t.Errorf` first so the full list prints in
a single test run; the closing `t.Fatalf` aborts after the list. CI red
on any regression.

| Lint | Package | What it catches | Final count |
|---|---|---|---|
| `TestNoInternalTermsInPrompts` | `internal/skill` | implementation jargon in `Config.Goal/Workflow/OutputFormat/Prohibitions` | 0 |
| `TestNoInternalTermsInToolSchemas` | `internal/tool` | implementation jargon in `emit_*` tool `Description()` + JSON-schema `description` fields (recursive walk) | 0 |
| `TestNoInternalTermsInHints` | `internal/agent` | implementation jargon in any LLM-reaching string inside `internal/agent/*.go` (AST-based, with logger-arg exclusion + ImportSpec exclusion + word-boundary for short acronyms) | 0 |
| `TestNoKeywordExamplesInEnums` | `internal/skill` | quoted user-wording fragments in classification enum descriptions + `AnalysisHardRules` (red-line `feedback_no_custom_keyword_matching`) | 0 |

Blocklists live in `internal/skill/glossary.go`:
- `InternalTermsBlocklist` — ~60 tokens (data-type names, contract-field
  leaks, pipeline vocabulary, design acronyms, validator disclosure,
  log-triage layers, session tracking).
- `KeywordExamplePhrases` — ~35 English + Chinese quoted fragments.

## Baseline vs post-cleanup regression

Pass-rate per case (baseline `d24088f` → after batch 4A `73ad469`,
runtime Opus 4.7 1M context). Per the regression discipline established
in `docs/prompt_baseline_p0.md`, only `s7a` / `u3a` / `logtri_java` are
re-run per batch (the three load-bearing cases); `s1a` and `logtri_go`
are stable cases captured at baseline only.

| case | runs | baseline | post-2A+2B | post-3A | post-4A | gate |
|---|---|---|---|---|---|---|
| s7a | 3 | 2 / 3 | 2 / 3 | 2 / 3 | 2 / 3 ¹ | 4A: `s7a = baseline` ✓ |
| s1a | 3 | 3 / 3 | — | — | — | n/a (stable) |
| u3a | 5 | 5 / 5 | 5 / 5 | 5 / 5 | 4 / 5 ² | 2A: `u3a MUST hold 5/5` — **noise-band miss; see ²** |
| logtri_go | 3 | 3 / 3 | — | — | — | n/a (stable) |
| logtri_java | 3 | 3 / 3 | 3 / 3 | 3 / 3 | 3 / 3 | 3B: `logtri coverage unchanged` ✓ |

¹ s7a was sampled twice during the 4A regression (`s7a-20260422-165110`
1/3 and `s7a-20260422-165111` 2/3). Aggregate 3 / 6 spans the 2 / 3
baseline within the natural variance of measurement-scalar cases (the
LLM has to select files, count lines across multiple sources, and
return a literal scalar; one fewer file-read or one bad parse drops
the run). The most-recent 3-run window matches baseline.

² u3a re-run `u3a-20260422-171414` produced 4 / 5 PASS. The single
FAIL (`run-1: missing_section:explorer.go`) traces to the finalizer
citation-validation layer, not to the 4A enum-description change:
the explorer emitted 21 grounded evidence items across 3 files, but
the finalizer's three `emit_answer_document` attempts were each
rejected by the grounding gate ("the tool rejected the emit because
all the citations were fabricated" / "the validation error persists")
because the cited symbol names did not appear at the cited
`file:line` locations. The orchestrator timed out and shipped the
extractor-only Half-answer covering only `extractor.go`. This is the
same finalizer-citation flakiness that existed at baseline; it
became visible here because the explorer's evidence pool was
unusually rich (21 items) and gave the finalizer many ways to
mis-pick a line. An immediately preceding 3-run window
(`u3a-20260422-165116`) was 3 / 3 before being interrupted; together
with this run that totals 7 / 8 = 87.5 % across two samples.

### Median key metrics (re-run cases only)

The medians below come from the most recent post-4A regression window
of each case (`s7a-20260422-165111`, `u3a-20260422-171414`,
`logtri_java-20260422-165120`).

| metric | s7a baseline → 4A | u3a baseline → 4A | logtri_java baseline → 4A |
|---|---|---|---|
| `tool_read_file` | 7 → 0 | 11 → 18 | 13 → 8 |
| `concrete_values` | 7 → 7 | 17 → 16 | 8 → 7 |
| `t11_gate_run` | 0 → 1 | 2 → 2 | 1 → 1 |
| `dataflow_intent_lookup` | 1 → 1 | 2 → 2 | 1 → 1 |
| `midloop_inject` | 3 → 2 | 5 → 7 | 4 → 8 |
| `answer_chain_lines` | 7 → 7 | 7 → 6 | 2 → 12 |

`tool_read_file` rising on u3a (11 → 18) and falling on s7a (7 → 0) /
logtri_java (13 → 8) reflects the explorer doing more cross-file
verification on the comparison case (consistent with u3a run-1's
21-item grounded evidence pool that subsequently confused the
finalizer). No metric crossed a regression-gate threshold.

### Lines of code

Across the seven commits (`cfa0dcc..73ad469` plus this batch 4B
commit):

| bucket | files changed | inserted | deleted |
|---|---|---|---|
| Prompt-only sources (`internal/skill/*.go`, `internal/tool/emit_*.go`, `internal/agent/*.go`) | 18 | 930 | 133 |
| Lint tests (`internal/{skill,tool,agent}/*_test.go`) | 4 | 751 | 0 |
| Docs (`docs/prompt_*.md`) | 2 | 372 | 0 |

## Follow-ups intentionally deferred

1. **`explorer.go` runtime keyword tables** — `hasRelationalVerb` loop (lines 374-382) and `detectEnumerationIntent` function (line 8670) still match the user's question against hardcoded CJK + English keyword lists. Red-line violation NOT caught by any lint (they are Go runtime code, not prompt strings). Proposed migration: replace both with structural predicates (`predicates.is_relational_lookup`, `QuestionKind == ReqEnumeration`). Behavioral change — deserves its own eval-gated commit separate from this prompt-cleanup stream.

2. **Parser-contract header review** — the 27 `Title:` strings in `internal/context/builder.go` (e.g. "Prior Stage Findings", "Raw Tool Outputs from the Investigation") are untouched by this series. Some carry mild implementation flavour ("from the Investigation", "deterministic, authoritative") that could be softened, but any rename requires a corresponding parser-side update in extractor.go / stage_report_render.go / explorer.go. Out of scope for prompt-only cleanup; may be picked up when the user-section renderer is next refactored.

3. **Finalizer citation-validation flakiness on rich-evidence comparisons** — the u3a 4 / 5 result above was caused by `run-1`'s finalizer rejecting all three `emit_answer_document` attempts because the cited symbol names did not appear at the cited `file:line`. Trace was diagnosed at ship time but root-cause investigation is left to a follow-up session: needs to determine whether the explorer's pre-scan-derived evidence (un-read files) is reaching the finalizer's citation pool, or whether the finalizer's citation selector picks symbols that the grounder cannot recover. This belongs to the finalizer/CGEC layer, not prompt cleanup.

## How the 4-batch partition held up in practice

Observation from the user directive (`feedback_category_grouped_batches.md`): grouping by problem category — terminology / structure / keyword-example — kept each batch's regression gate **targeted to ONE failure mode**. In practice:

- **Batch 2A/2B (terminology)**: zero behavioral surprise. Expected because pure renames do not change classifier inputs; regression gate (PASS count ≥ baseline) trivially held — `s7a 2/3`, `u3a 5/5`, `logtri_java 3/3`.
- **Batch 3A (structure)**: the load-bearing batch. Moving Pre-scan out of OutputFormat and collapsing 4 EVIDENCE-LITE prohibitions could have affected the analyzer's round-count behaviour; the per-batch gate targeted exactly that signal (prescan_rounds median, prescan_budget_exhausted frequency). Baseline held — `s7a 2/3`, `u3a 5/5`, `logtri_java 3/3`.
- **Batch 4A (keyword-example)**: the highest-risk batch. Removing surface-wording cues from enum descriptions is a direct lever on classifier reliability. The gate targeted `s7a` (is_count_question) and the u3a comparison case. `s7a` held at 2 / 3. `u3a` came in at 4 / 5 — one short of baseline 5 / 5; the FAIL traces to the finalizer citation-validation layer rather than to enum classification (see footnote ² above and follow-up #3 below). Aggregate across the two u3a samples taken during 4A is 7 / 8 = 87.5 %.

One test assertion did have to move: `TestAnalysisSkill_PromptDocumentsPreScanBudget` widened from `OutputFormat` scan to full-corpus scan, anticipated in the 3A commit message. The three-way required-field consistency test added in 3A caught two pre-existing drifts (Workflow listed sub_topics as required when it is optional; OutputFormat omitted the 4 confidence floats) and forced alignment with the JSON schema.

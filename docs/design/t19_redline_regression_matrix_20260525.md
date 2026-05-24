# T19 Redline Regression Matrix

Date: 2026-05-25

## Goal

Prevent the recurring accident class where deterministic system structure
overpowers user intent or useful model-authored answer content. This batch is a
test-and-fence batch, not a new answer-generation feature.

Hard rule:

- Model-authored visible content is the primary answer surface.
- System-generated supplements are allowed only when the missing payload is
  precise, typed, non-overlapping, and cannot be locally repaired as metadata.
- A system supplement must be append-only, clearly marked, localized, and must
  not become a competing principal answer.
- Non-current-source observations such as VCS, diff, command, log, trace, web,
  MCP, connector, and external documents must not be rewritten into repository
  `file:line` citations.
- Hard gates must consume typed, precise signals only. No path in this batch
  reads raw user prose or model free-form prose to decide flow.

## Code Paths Audited

| Surface | Current entry point | Risk | Batch action |
| --- | --- | --- | --- |
| Principal enumeration/table supplements | `normalizePrincipalEnumerationRowBlocks` | System table may duplicate or flatten a model table. | Add matrix test through the full `normalizeAnswerDocumentForPreEmit` sequence. |
| Aggregate member carriers | `normalizeAggregateMemberSetCarriers` and scalar-value pre-check | Complete visible member tables could still be forced to show aggregate count/value literals. | Skip scalar-value hard hint for `member_set` when the typed members are already visible and the request is not a count-value request. |
| Current-source citation supplement | `normalizeCurrentSourceCitationSupplement` | A model-authored table with visible `file:line` locations could still receive a second source-anchor appendix. | Treat visible source locations as already covered unless the model emitted broken citation refs that require local metadata repair. |
| Origin-specific absence | `normalizeAggregateNegativeProofSupplement` | VCS/log/trace/web/MCP no-hit facts may be duplicated or re-anchored as source citations. | Add full-normalizer redline matrix coverage. |
| Renderer preserved content | `RenderAnswerDocumentWithAttachments` | Preserved model output could duplicate final answer. | Existing render tests already cover divider/dedupe; keep as T19 display guard. |
| Extractor origin-specific handoff | `extractorEvaluator.BuildInitialInstruction` | External observations could be pushed into `emit_answer_symbol`. | Existing extractor tests cover VCS/MCP narrative; keep as T19 agent guard. |

## Implemented Slice

- Added `internal/tool/answer_document_redline_matrix_test.go`.
- Added a full pre-emit matrix case proving a rich model-authored Markdown table
  remains byte-identical and does not receive a system supplement.
- Added a full pre-emit matrix case proving a VCS negative observation remains
  origin-specific and does not invent source citations.
- Added a full pre-emit matrix case proving genuinely missing members produce
  exactly one marked, append-only supplement and no legacy duplicate carrier.
- Added language-agnostic visible-location coverage for ArkTS (`.ets`) and C++
  (`.cpp`) paths, so the current-source supplement fence is not Go-specific.

## Product Fixes From This Batch

1. `normalizeCurrentSourceCitationSupplement` now suppresses the source-anchor
   appendix when the model-authored visible answer already contains the exact
   source location. It still repairs when citation refs are broken, because that
   is metadata repair rather than competing answer content.
2. `preCheckAggregateScalarValueCoverage` no longer forces the aggregate
   `member_set.value` count into the answer when all members are already visible
   and the typed request is not asking for a count value.

## Remaining T19 Work

- Add eval-level assertions for covered answers: no unwanted
  `系统按已验证证据补充成员` / `System-verified member supplement` when the model
  already rendered the accepted typed answer.
- Add one display-layer assertion that preserved attachments do not surface as a
  second answer when structurally equivalent but not byte-identical.
- Keep expanding this matrix whenever a new deterministic supplement path is
  introduced.

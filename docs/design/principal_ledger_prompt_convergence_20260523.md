# Principal Ledger Prompt Convergence

Date: 2026-05-23
Status: Batch 1 through Batch 5 implemented; targeted eval replay passed

## Problem

The unified evidence contract is mostly in place, but prompt consumers still
have a few split-brain paths:

- finalizer and semantic reviewer both consume `ObservationLedger`, but each
  locally decides how to trim summaries, rich notes, excerpts, source refs, and
  spans;
- prompt dedupe is partly solved at the ledger record level, but rendering
  surfaces can still duplicate summary text as notes or diverge in note budgets;
- mixed-origin questions such as "based on this diff, analyze current code"
  depend on typed ledger ranking staying consistent across finalizer and
  reviewer;
- future patches could accidentally add another raw evidence prompt surface
  instead of reusing the compiled ledger.

These are architecture gaps, not one-case output bugs. The fix must converge
existing paths around the current `types.ObservationLedger` primitives instead
of introducing another evidence stack.

## Red Lines

- Do not parse user prose or model free-form prose to make hard decisions.
- Do not replace, delete, or rewrite model-authored answer text or tables.
- System-generated supplements must stay clearly separate and localized.
- Current-source `file:line` citations remain exclusive to grounded current
  source evidence; git/log/trace/command/MCP/web observations use their own
  typed source refs.
- Rich explorer summaries, member notes, support refs, and payload refs must be
  preserved through downstream prompt budgeting unless a typed safety rule
  proves they are invalid.
- Reuse `ObservationLedger`, `ObservationSourceRef`, `ObservationSpan`,
  `AnswerClaimBinding`, `row_set_ref`, and `payload_ref`; do not create a
  parallel carrier.

## Current Code Map

- Ledger type and compiler:
  `internal/types/observation_ledger.go`
- Agent/bus adapters:
  `internal/types/observation_ledger_context.go`
- Finalizer prompt surface:
  `internal/agent/answer_document_evaluator.go::renderAnswerDocObservationLedger`
- Semantic reviewer projection:
  `internal/orchestrator/semantic_quality_reviewer.go::semanticObservationSummaries`
- Origin/source formatting:
  `internal/types/observation_ledger.go::FormatObservationSourceRef`
- Existing regression coverage:
  `internal/agent/answer_intent_contract_prompt_test.go`,
  `internal/types/observation_ledger_test.go`,
  `internal/orchestrator/semantic_quality_reviewer_test.go`

## Detailed Design

### 1. Shared Prompt Projection

Add a read-only projection helper in `internal/types`:

- input: already-prioritized or raw `[]ObservationRecord`, `RequestModel`,
  `AnswerContract`, prompt limit, and projection options;
- output: compact records containing id, origin, role, grounding policy,
  provenance lane, formatted source, formatted span, claim, value, summary,
  excerpt, rich notes, negative flag, result count, and support-ref count;
- ranking must call `PrioritizeObservationRecords`, preserving the current typed
  relevance rules;
- note selection must dedupe normalized text and must not repeat the visible
  summary as a note;
- current-source records must not render raw excerpts as external support;
- principal and origin-specific support records may get a larger note/excerpt
  budget, but the budget is configured in typed options, not copied between
  consumers.

This moves shared mechanical policy into `types` while leaving finalizer and
reviewer free to render their own section prose.

### 2. Finalizer and Reviewer Consumption

- Finalizer keeps one `## Observation Ledger` section and renders the shared
  projection rows.
- Semantic reviewer builds `SemanticObservationSummary` from the same shared
  projection, avoiding local trimming drift.
- No new prompt section is introduced.
- Existing claim-binding and raw tool-output sections stay unchanged in this
  batch; later audit decides whether any raw surface is redundant.

### 3. Prompt Surface Audit

Audit finalizer/extractor/reviewer prompt builders for duplicate evidence
sections. Record each surface as one of:

- authoritative typed ledger;
- answer-shape/tool schema guidance;
- raw backstop/debug excerpt;
- legacy surface to remove or demote.

The audit result should be added to this document before code removal. No broad
prompt deletion without tests.

### 4. Eval Guardrails

Targeted evals after implementation:

- mixed diff + current-source architecture explanation;
- recent-N git history with purpose and impact;
- large member-set enumeration with rich Chinese summaries;
- log/trace line or window question;
- negative observation mixed with present current-source evidence.

The eval goal is not "zero system supplements"; it is no unintended rewrite,
no model-authored content replacement, rich summaries preserved, and no fake
current-source citations for external observations.

## Task List

- [x] T0: Create this task/design document.
- [x] T1: Add shared `ObservationPromptRecord` projection helper in
  `internal/types` with unit tests for summary-note dedupe, mixed-origin
  ranking, current-source excerpt suppression, and origin-specific rich-note
  budgets.
- [x] T2: Refactor finalizer Observation Ledger prompt to consume the shared
  projection without changing user-visible section semantics.
- [x] T3: Refactor semantic reviewer observation summaries to consume the same
  projection.
- [x] T4: Audit finalizer/extractor/reviewer prompt surfaces and document
  whether each surface is authoritative, support, raw backstop, or removable.
- [x] T5: Run targeted tests and update this document with results.
- [x] T6: Run targeted evals and update
  `docs/design/eval_20260520_full_sweep_gap_tracking.md` with any new gap.
- [x] T7: Generalize the recent-N VCS member-list fix into an
  origin-specific principal-member contract for VCS/diff, runtime/log/trace,
  command output, cross-repo index, external document, web, MCP, and connector
  observations without creating current-source citation pressure.

## Batch Plan

### Batch 1: Projection Helper

Deliver shared projection and types-level tests. No prompt text change except
moving mechanical trimming rules into one helper.

Status: done.

Implemented:

- `internal/types/observation_prompt_projection.go`
- `internal/types/observation_prompt_projection_test.go`

Guardrails:

- notes are normalized and deduped;
- visible summary text is not repeated as a note;
- dry single-token member fallbacks are skipped when the summary/claim already
  carries the same token;
- current-source raw excerpts stay out of compact observation prompts;
- external observations keep bounded excerpts;
- runtime/git/log/trace origin-specific principal rows receive richer note
  budgets;
- span formatting covers current-source, VCS hunk, JSON pointer, row, selector,
  paragraph, text range, and timestamp coordinates.

### Batch 2: Consumer Refactor

Move finalizer and semantic reviewer to the shared projection. Add regression
tests that both consumers preserve rich notes and do not duplicate summary text
as notes.

Status: done.

Implemented:

- `internal/agent/answer_document_evaluator.go` now renders Observation Ledger
  rows from `types.ProjectObservationPromptRecords`.
- `internal/orchestrator/semantic_quality_reviewer.go` now builds reviewer
  observation summaries from the same projection.

Targeted tests:

```bash
go test ./internal/types ./internal/agent ./internal/orchestrator -run 'TestProjectObservationPromptRecords|TestFormatObservationSpan|TestRenderAnswerDocObservationLedger|TestSemanticObservationSummaries|TestRenderSemanticQualityUserMessage'
```

Result: pass.

### Batch 3: Prompt Audit

Document every prompt evidence surface and remove/demote only obviously
duplicated legacy surfaces with tests.

Status: done for code-level audit; no broad prompt deletion in this batch.

Audit table:

| Surface | Producer | Consumers | Role | Decision |
|---|---|---|---|---|
| `Observation Ledger` | `types.CompileObservationLedger` | finalizer, semantic reviewer | authoritative typed compact view for current-source, VCS/diff, command, runtime, MCP/external observations | keep; now rendered through shared projection |
| `Claim Bindings` | aggregate/runtime claim binding compiler | finalizer, semantic reviewer | origin/policy/output boundary, especially citation-pressure policy | keep; complementary to ledger, not duplicate fact prose |
| `Evidence Origin Boundary` | context builder / finalizer evaluator | explorer/extractor upstream, finalizer dynamic copy only | origin policy and mixed-lane plan | keep; existing tests pin no duplicate finalizer copy from `BuildPromptContext` |
| `Knowledge & Evidence Pool` | context builder from `EvidenceItems` | finalizer and non-extractor skills | current-source citation pool | keep; it is the current-source citation authority, not a replacement for external observations |
| extractor `Investigation transcript digest` | extractor evaluator from accepted Turn A snapshot | extractor only | Turn A handoff: accepted closure, read files, ranked evidence, flow findings, cardinality guidance | keep; extractor deliberately skips the generic Structured Evidence section to avoid duplicate evidence surfaces |
| `Accepted exploration closure` | accepted `emit_investigation_complete` payload | extractor, finalizer dynamic sections | advisory set-level model summary plus structured aggregate facts | keep with current caveat: aggregate facts remain authoritative if member/count identity conflicts |
| `Raw Tool Outputs` | context builder from accepted Turn A tool results | extractor/finalizer only for citation-free value/history paths | raw backstop for command/VCS facts that cannot become `emit_evidence` rows | keep gated; not a general evidence prompt and not rendered for ordinary source-code explanations |
| `Principal member set contract` | finalizer evaluator from stable aggregate facts | finalizer | hard visible-member obligation for typed principal member sets | keep; prevents hidden member loss but must not author prose/table content |
| `Typed Exploration Enrichment` / support plan | finalizer evaluator | finalizer | current-source support lanes and rich evidence hints | keep; code already suppresses duplicate external seeds when typed support rendered |
| semantic reviewer `Evidence Anchors` | orchestrator reviewer input | semantic reviewer | identifier cross-reference set for contradiction/fabrication checks | keep; reviewer-only, not an answer-authoring fact surface |

Findings:

- No second authoritative ledger surface remains after Batch 2. Finalizer and
  reviewer share the same `ObservationPromptRecord` projection.
- The extractor already has a deliberate non-duplication rule:
  `BuildPromptContext` skips the generic Structured Evidence section for
  extract-skill because the extractor's own Turn A digest carries the curated
  evidence handoff.
- `Raw Tool Outputs` is still a necessary raw backstop for citation-free
  command/VCS/history questions. It is tightly gated by typed answer shape and
  stage, so removing it now would reintroduce lost git/log/count facts.
- No prompt surface was removed in this batch because the audit did not find a
  clearly redundant authoritative duplicate with zero unique responsibility.

Targeted code references:

- `internal/context/builder.go::shouldRenderRawToolOutputs`
- `internal/context/builder.go::BuildPromptContext`
- `internal/agent/extractor.go::BuildInitialInstruction`
- `internal/agent/answer_document_evaluator.go::renderAnswerDocObservationLedger`
- `internal/orchestrator/semantic_quality_reviewer.go::semanticObservationSummaries`

### Batch 4: Eval Pass

Run targeted evals, record retry/reject data, and decide the next high-ROI gap
from evidence rather than speculation.

Status: done for the first replay tranche.

Targeted replay:

```bash
EVAL_RESULTS_ROOT=eval/results/principal-ledger-u7l-fix-20260523-213328 \
  bash eval/run.sh eval/cases/u7l.case 1
```

Result: `u7l` PASS, with `analyzer_iters=1`, `explorer_iters=5`,
`extractor_iters=1`, `finalizer_iters=1`, `midloop_inject=0`, and no finalizer
rewrite. The answer now carries the model's grouped explanation and an explicit
recent-10 commit member list instead of collapsing to a single summary.

Finding from the replay:

- The prior failure was not only a renderer issue. The accepted
  `emit_investigation_complete.aggregate_facts.member_set` for the recent
  commit list was structurally VCS-backed but had no current-source
  `support_refs`. The older support-ref filter treated decorated member labels
  as current-source code identities, dropped the structured member set, and
  left finalizer with only a narrative summary. That made a missing principal
  list look acceptable.

### Batch 5: Origin-Specific Principal Member Contract

Status: done.

Implemented:

- `types.OriginSpecificMemberSetIsPrincipalList` and
  `types.HasPrincipalOriginSpecificMemberSetForRequest` define the shared
  principal-member contract for non-current-source observations.
- `HistoryMemberSetIsPrincipalList` now reuses the origin-specific path while
  preserving the legacy implicit recent-N VCS behavior.
- `preEmitAggregateMemberSetCoverageHardGate` reads the generalized predicate,
  so an explicit principal MCP/web/log/trace/command member list cannot be
  silently compressed away.
- `decoratedAggregateMemberCanRelyOnOriginSpecificProvenance` keeps decorated
  external-observation members from being rejected for missing current-source
  `support_refs`; current-source decorated members and VCS code identifiers
  still require support refs unless the member is a real commit hash or an
  observation-only runtime/log/trace frame.

Guardrails:

- Unknown-role external member sets stay soft unless the model explicitly marks
  `role=principal_answer`, except for the typed pure-history VCS recent-N list
  compatibility path.
- Support/audit roles remain support/audit and do not become hard visible-member
  gates.
- Current-code verification requests with attached logs/traces still require
  current-source grounding; artifact-local observations do not become checkout
  citations.
- Full commit hashes may be satisfied by standard short-hash renderings (7+
  hex chars) in the model-authored answer. The system must not append a dry
  verified-member table merely because the visible answer used `bfc2054`
  instead of the 40-character hash.

Targeted tests:

```bash
go test ./internal/types ./internal/tool -run 'TestHistoryMemberSetIsPrincipalList|TestOriginSpecificMemberSetIsPrincipalList|TestRunPreEmitChecks_AggregateMemberSetCoverageHardForHistoryList|TestRunPreEmitChecks_AggregateMemberSetCoverageHardForOriginSpecificPrincipalList|TestDropUnsupportedDecoratedMemberSets|TestEmitInvestigationComplete_AllowsDecoratedCommitHashMemberSet|TestEmitInvestigationComplete_DecoratedCodeMemberStillRequiresSupportRefsWithVCSOrigin|TestEmitInvestigationComplete_AllowsDecoratedRuntimeMemberSet|TestEmitInvestigationComplete_DecoratedRuntimeMemberSetRequiresSupportRefsForCurrentVerification|TestEmitInvestigationComplete_AllowsDecoratedExternalOriginMemberSet'
```

Result: pass.

Follow-up replay after the short-hash supplement fix:

```bash
EVAL_RESULTS_ROOT=eval/results/principal-ledger-short-hash-20260523-215117 \
  bash eval/run.sh eval/cases/u7l.case 1
```

Result: PASS, `analyzer_iters=1`, `explorer_iters=3`, `extractor_iters=1`,
`finalizer_iters=1`, `midloop_inject=0`, and no visible system-supplement table.

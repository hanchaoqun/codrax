# Principal Ledger Prompt Convergence

Date: 2026-05-23
Status: design committed, implementation in progress

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
- [ ] T1: Add shared `ObservationPromptRecord` projection helper in
  `internal/types` with unit tests for summary-note dedupe, mixed-origin
  ranking, current-source excerpt suppression, and origin-specific rich-note
  budgets.
- [ ] T2: Refactor finalizer Observation Ledger prompt to consume the shared
  projection without changing user-visible section semantics.
- [ ] T3: Refactor semantic reviewer observation summaries to consume the same
  projection.
- [ ] T4: Audit finalizer/extractor/reviewer prompt surfaces and document
  whether each surface is authoritative, support, raw backstop, or removable.
- [ ] T5: Run targeted tests and update this document with results.
- [ ] T6: Run targeted evals and update
  `docs/design/eval_20260520_full_sweep_gap_tracking.md` with any new gap.

## Batch Plan

### Batch 1: Projection Helper

Deliver shared projection and types-level tests. No prompt text change except
moving mechanical trimming rules into one helper.

### Batch 2: Consumer Refactor

Move finalizer and semantic reviewer to the shared projection. Add regression
tests that both consumers preserve rich notes and do not duplicate summary text
as notes.

### Batch 3: Prompt Audit

Document every prompt evidence surface and remove/demote only obviously
duplicated legacy surfaces with tests.

### Batch 4: Eval Pass

Run targeted evals, record retry/reject data, and decide the next high-ROI gap
from evidence rather than speculation.


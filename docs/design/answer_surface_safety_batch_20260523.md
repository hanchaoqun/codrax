# Answer Surface Safety Batch

Date: 2026-05-23

Status: design tracked; implementation pending.

This document tracks the next high-ROI batch after the principal-ledger and
origin-specific member-list work. The focus is deliberately narrow but
architectural: make system-generated answer supplements and post-emit reviewers
consume the same typed answer surface, without replacing or compressing
model-authored content.

## Problem Statement

Recent evals and customer logs exposed two recurring answer-surface failures:

1. Deterministic row compilers / system supplements can improve coverage, but
   when they add dry tables next to a good model answer they make the answer look
   worse, duplicate information, or introduce table shapes that do not match the
   user's question.
2. Reviewers can judge a projection of the answer that is close to, but not
   exactly, the final rendered panel. That creates avoidable retries, confusing
   telemetry, and in the worst case pressure to rewrite an answer that was
   already acceptable to the user.

The goal is not to disable deterministic repair. The goal is to make it
append-only, visible, localized, provenance-aware, and reviewer-aligned.

## Red Lines

- User intent and model-authored answer text are first-class. The runtime must
  not replace, delete, or rewrite model-authored prose or tables to enforce a
  preferred presentation shape.
- System-generated supplements must be separate visible blocks with localized
  titles that explain they are system-verified additions. They must not look like
  the model's own answer.
- Hard gates may use only precise typed signals: schema fields, validated
  origins, exact counts, exact IDs, and grounded support refs. User/model prose
  and fuzzy keyword matching are advisory only.
- Current-source `file:line` pressure applies only to current-source evidence.
  VCS, diff, logs, traces, command output, MCP, web, and connector observations
  remain valid evidence through their own origin-specific carriers.
- Rich exploration summaries, notes, excerpts, and origin-specific values must
  be preserved through finalizer and reviewer prompts unless a precise typed
  signal proves the record is invalid.
- The solution must cover all languages supported by repomap by staying above
  language-specific syntax where possible. Language-specific logic is allowed
  only behind existing repomap/source-index abstractions.

## Existing Components To Reuse

No new evidence stack should be introduced for this batch. The relevant existing
components are:

- `types.AnswerDocumentV2`, `types.AnswerBlockVisibleSurface`, and
  `types.AnswerBlockItemVisibleSurface` for the visible answer contract.
- `render.RenderAnswerDocumentWithAttachments` for the final user-facing panel.
- `types.CompileEnumerationDisplaySets` and
  `internal/tool/answer_document_principal_enum_compile.go` for accepted
  principal member rows.
- `types.ObservationLedger`, `types.ProjectObservationPromptRecords`, and
  `types.FormatObservationSourceRef` for current-source and external evidence
  projection.
- `internal/tool/answer_document_field_quarantine.go`,
  `emit_answer_document_v2.go`, and `emit_answer_document_patch.go` for shared
  schema-aware repair and quarantine.
- `BuildSemanticQualityInput`, `renderSemanticQualityReviewBodyV2`, and
  `renderConsistencyReviewBodyV2` for reviewer input.
- Existing eval cases under `eval/cases`, especially the principal-ledger,
  multi-repo, history, log, trace, and source-inventory cases.

## Architecture Design

### 1. System Supplement Policy

System supplements are allowed only as deterministic append-only patches for
typed obligations that the model did not visibly cover.

Rules:

- A supplement must never replace or mutate a model-authored Markdown table.
- A supplement may add only rows missing from the final visible surface. Coverage
  must use typed row identity plus safe aliases already compiled by
  `CompileEnumerationDisplaySets`.
- A supplement must be suppressed if the model-visible answer already covers the
  row identity, including origin-specific aliases such as short commit hashes or
  artifact-local coordinates.
- A supplement block must carry a localized title:
  - Chinese: `系统按已验证证据补充...`
  - English: `System-verified ... supplement`
- A supplement block must not render empty columns. If every data field except
  the member label is empty, the runtime should either suppress the supplement
  or use an origin-specific label column rather than generic `符号名称`.
- For non-code origins, column labels should reflect origin semantics where the
  type is known:
  - VCS/diff: commit / change / path / evidence summary
  - runtime artifact/log/trace: artifact line/span / observation / summary
  - command: command result / value / summary
  - MCP/web/connector: resource / observation / summary
- Incompatible model-authored table shapes may receive a separate supplement,
  but only for missing or verified fields. The system must not rebuild the whole
  answer table as a competing primary table.

### 2. Rendered Surface Snapshot

Post-emit reviewers should reason over the same surface the user will see after
deterministic normalization and attachment rendering.

Design:

- Keep `AnswerDocumentV2` as the typed source of truth.
- Add a small shared rendered-surface projection helper rather than duplicating
  renderer behavior in each reviewer.
- The helper should expose:
  - `Summary`: concatenated summary blocks after authority-artifact stripping.
  - `Body`: final visible body projection, including diagrams for semantic
    review and excluding summary blocks.
  - `FullMarkdown`: final rendered panel when needed for telemetry/debug.
  - `Attachments`: display attachments included in the final panel.
- Self-consistency can continue to skip diagrams because it reviews prose
  consistency; semantic quality must include diagrams and system supplements
  because it reviews completeness.
- Reviewer decisions should distinguish:
  - `local_doc_defect`: only for precise missing visible content that can be
    repaired from already supplied typed evidence.
  - `presentation_advisory`: formatting/clarity; never forces rewrite.
  - `evidence_gap` / `analysis_gap`: upstream deficiency; do not ask finalizer
    to invent coverage.

### 3. External Evidence And Mixed Questions

Mixed questions such as "based on this diff/log/trace, analyze current code"
must preserve both external observations and current-source evidence.

Rules:

- Evidence ranking remains intent-aware. High-value current-source rows that are
  read, landed, definition-anchored, and in scope outrank weaker source hints.
- External observations are ranked within the same principal ledger, not in a
  side channel that can be squeezed out of prompt budget.
- Negative observations are bound to target/scope/origin. A precise absence in
  a log, trace, git history, or command result must not erase a separate
  positive current-source or VCS fact.
- Large payloads use existing payload/row-set refs and blob pagination. They
  should not force the model to serialize huge JSON arrays.

### 4. Exact External Detail Preservation

`E20260520-G153` exposed a broader contract gap: external observations may be
accepted as principal evidence, but exact path/date/resource literals inside
the observation can still degrade into ordinary prose. In the replay, `git_log
--stat` returned abbreviated stat paths such as
`.../design/answer_surface_safety_batch_20260523.md`; the finalizer expanded
the abbreviation and changed `20260523` to `20250523`. The answer shape was
good, but an exact literal was no longer trustworthy.

This is not a VCS-only issue. The same class applies to:

- VCS metadata/diff: commit IDs, changed-file paths, hunks, old/new line spans.
- Runtime logs/traces: artifact-local line numbers, event IDs, span names,
  timestamps, thread/process IDs.
- Command output: table rows, counts, JSON fields, shell output line numbers.
- Cross-repo index: repo slug, package path, symbol path, graph node ID.
- External document / web / MCP / connector: URL/resource URI, page/paragraph,
  JSON pointer, row ID, fetched timestamp, connector object ID.

Contract:

- Producers should expose exact literal detail as typed fields, exact detail
  sections, `PayloadRef`, or `RowSetRef`, not only as narrative prose.
- If a producer emits an abbreviated display (for example git stat's `...` path
  compaction), it must also emit an exact companion detail when the exact value
  is cheap and bounded.
- Finalizer/reviewer prompts should tell the model to copy path-like,
  date-like, URL-like, and ID-like literals only from exact typed detail or raw
  payload, and to avoid expanding abbreviated display strings.
- This layer remains presentation/support evidence. It must not become
  current-source `file:line` pressure unless a separate current-source read
  grounds the same claim.
- The mechanism is language-agnostic. It treats paths/IDs/rows as origin-local
  observation details and does not infer semantics from Go, Java, C++, Python,
  TS, ArkTS, Cangjie, Ruby, Shell, or any other repomap language.

## Batch Task List

| Batch | Status | Task | Code Areas | Validation |
| --- | --- | --- | --- | --- |
| T0 | Done in this doc | Record design, red lines, existing components, and implementation plan. | `docs/design/answer_surface_safety_batch_20260523.md` | Doc review |
| T1 | Done | Add a central supplement-policy helper around principal enumeration supplements. It should localize origin-aware labels, suppress all-empty/dry supplements, and keep model-authored tables untouched. | `internal/tool/answer_document_principal_enum_compile.go`, related tests | `go test ./internal/tool -run 'TestNormalizePrincipalEnumerationRowBlocks|TestPrincipalEnumerationPrimaryColumnLabel'` |
| T2 | Partially done | Add regression tests for VCS/history, runtime artifact, command, MCP/web/connector-like rows so non-code supplements use origin-aware labels or are suppressed when dry. | `internal/tool/*_test.go`, `internal/render/*_test.go` | VCS missing-row supplement plus all current origin labels covered; broader eval replay remains under T5. |
| T3 | Done | Introduce a shared reviewer visible-surface helper and route self-consistency / semantic-quality inputs through it. | `internal/orchestrator/contract_check.go`, `semantic_quality_reviewer.go`, self-consistency tests | `go test ./internal/orchestrator -run 'TestBuildReviewerAnswerSurface|TestRenderConsistencyReviewBodyV2|TestRunSemanticQualityReviewWithOutcome|TestRenderSemanticQualityUserMessage'` |
| T4 | Partially done | Add telemetry/tests proving reviewer sees system supplements, model Markdown tables, diagrams, attachments, and external observations exactly as the final panel does. | `internal/orchestrator/*reviewer*_test.go`, `internal/render/answerdoc_test.go` | Shared surface test covers supplements, diagrams, and attachments; external observations already covered by semantic reviewer tests. Targeted eval replay remains under T5. |
| T5 | Partially done | Run targeted evals for principal-ledger/history, source-inventory, multi-repo compare, log/trace artifact, and mixed external+code analysis. | `eval/results/...` | `u7l` and `logtri_artifact_line_anchor` replayed with no finalizer retry; broader mixed/multi-repo tranche remains for the next batch. |
| T6 | Partially done | Refresh gap docs with confirmed residuals and decide next architecture batch. | `docs/design/eval_20260520_full_sweep_gap_tracking.md`, related design docs | VCS path/date literal fidelity residual recorded as `E20260520-G153`. |
| T7 | Pending | Add exact external-detail preservation for VCS tool outputs, starting with `git_log stat=true` exact changed paths and prompt guidance that forbids expanding abbreviated stat paths. | `internal/tool/builtin.go`, `internal/types/observation_ledger.go`, finalizer prompt tests | unit tests + `u7l` replay |
| T8 | Pending | Generalize exact-detail projection tests for non-VCS origins so logs/traces/commands/MCP/connectors keep artifact-local coordinates and resource IDs as origin-local evidence. | `internal/types/observation_*_test.go`, selected eval cases | targeted log/trace/command/MCP-style fixtures |

## Developer Guardrails

- Any new deterministic answer mutation must answer these questions in code
  review:
  1. Does it preserve every model-authored block and attachment?
  2. Is the addition separate and localized when it is system-authored?
  3. Does it consume typed origins/support refs instead of model prose?
  4. Does it work for non-Go languages and non-source evidence?
  5. Is there a regression test proving the supplement does not pollute an
     already sufficient answer?
- Tests should prefer typed fixture construction over prompt text matching.
  Prompt text assertions are acceptable only for public, localized labels or
  stable typed section names.

## Progress Log

- 2026-05-23: Initial code audit found the main row compiler path in
  `internal/tool/answer_document_principal_enum_compile.go` and the reviewer
  surface path in `internal/orchestrator/contract_check.go` /
  `semantic_quality_reviewer.go`. Existing schema repair/quarantine and
  Observation Ledger components are sufficient; this batch should not introduce
  a parallel evidence channel.
- 2026-05-23: Batch T1 implemented origin-aware primary column labels for
  deterministic principal-enumeration system supplements. Current-source
  supplements keep `符号名称` / `Name`; non-code origins now render as
  commit/change/observation/command/resource semantics instead of pretending to
  be code symbols. Regression coverage includes a VCS missing-row supplement and
  every declared `AnswerEvidenceOrigin` family. Existing model-authored table
  preservation behavior is unchanged.
- 2026-05-23: Batch T3 introduced `buildReviewerAnswerSurface`, a shared
  reviewer projection that derives stripped summary, reviewer body, and full
  rendered markdown from the same `AnswerDocumentV2` plus attachments. Both
  self-consistency and semantic-quality reviewers now consume this helper.
  Self-consistency still excludes diagrams by design; semantic-quality includes
  diagrams because requested visual structure is part of completeness. The test
  fixture proves system supplements, diagram bodies, and recovered display
  attachments are visible in the expected projection without adding a second
  prompt surface.
- 2026-05-23: Targeted replay covered
  `eval/results/u7l-20260523-222442` and
  `eval/results/logtri_artifact_line_anchor-20260523-222442`; both passed with
  one finalizer round, no finalizer reject, and no duplicate first-draft panel.
  `u7l` preserved the rich recent-commit list and did not render a dry
  system-generated symbol table. `logtri_artifact_line_anchor` preserved the
  runtime-artifact boundary and answered the line-number / absence mix without
  current-source citations. Residual found: `u7l` still allowed the finalizer to
  miscopy a VCS-mentioned design-doc date in a path (`20260523` became
  `20250523`). This is tracked in the full eval gap ledger as
  `E20260520-G153`; it requires typed VCS path/file-detail preservation rather
  than another supplement-table patch.
- 2026-05-23: Code/log audit for `E20260520-G153` found the deep root cause:
  `git_log stat=true` exposes Git's abbreviated `--stat` display to the model,
  while exact changed paths are not separately carried in the typed observation
  surface. The finalizer prompt had full tool output, but it had to infer exact
  paths from `...` abbreviations. Batch T7 will add bounded exact changed-path
  companions at the VCS producer layer and prompt guidance to copy exact literals
  only from exact detail/raw payload, not from abbreviated display strings.

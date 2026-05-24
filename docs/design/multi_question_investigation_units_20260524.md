# Multi-Question Investigation Units - 2026-05-24

## Problem

`RequestModel.SubTopics` currently carries the analyzer's "I split this request
into N independently-answerable topics" output. The compiler fans those topics
out into separate `NodeEvidence` tasks, and the renderer presents them as
"关注点 / focus areas".

That is useful, but it conflates three different concepts:

- **Investigation decomposition**: how explorer should divide work.
- **User answer partition**: how the final answer should be organized because
  the user explicitly asked for sides, buckets, dimensions, or sections.
- **Progress presentation**: what the REPL should show while those units are
  being investigated.

When the user's question contains multiple loosely-coupled asks, a flat
`SubTopics[]` list is not enough. It can make unrelated asks look like one
coupled topic set, and the UI phrase "关注点 2/2" reads like a completion
counter even when it only means "the second analyzer-created unit is active".

## Existing Code Survey

- `internal/types/analysis_ir.go`
  - `SubTopics[]`: analyzer-authored investigation decomposition.
  - `Buckets[]`: stronger user-authored partition contract.
  - `RequestedAnswerDimensions`: user-visible output dimensions.
  - `CurrentSourceExplanationProfile`: mixed external/current-source routing.
- `internal/analysis/compiler/templates.go`
  - `expandEvidenceNodes` turns each sub-topic into one `NodeEvidence`.
- `internal/types/question_structure.go`
  - `EffectiveQuestionBuckets` preserves user partitions and typed fallback
    comparison buckets without scanning free prose for decision keywords.
- `internal/types/answer_semantic_view_helpers.go`
  - protects against treating all sub-topics as hard code-anchor skeletons.
- `internal/render/status_blocks.go`, `internal/render/renderer_dock.go`,
  `internal/render/status_messages.go`
  - render multi-topic evidence nodes as "关注点 / focus areas".
- `internal/render/structured_tool_summary.go`
  - renders `emit_analysis` summary, but still uses "子问题" wording and only
    reads `sub_topics[].title`; current schema uses `summary`.

## Design Principles

1. **Do not replace model decisions with system preference.**
   Investigation units are typed structure for scheduling and display. They do
   not authorize deterministic answer rewriting.
2. **User partition outranks analyzer decomposition.**
   `Buckets[]` means the user asked for visible sections. `SubTopics[]` means
   the analyzer split investigation work.
3. **External observations are first-class evidence origins.**
   The unit contract reuses `CompileAnswerIntentContract` so VCS, log, trace,
   command, MCP/web/connector, external-doc, and current-source origins can be
   carried consistently.
4. **No raw prose hard decisions.**
   The compiler consumes only typed IR fields and schema-validated profiles.
   Raw request text is used only where existing provenance helpers already
   require exact label validation, such as `QuestionBucket` normalization.
5. **UX should explain semantics, not expose internals.**
   The REPL should say "调查单元" or "用户分区", not "关注点", and should avoid
   `2/2` wording that sounds like completion.

## Contract

Add a derived `InvestigationPlan`:

```go
type InvestigationPlan struct {
    Units          []InvestigationUnit
    Coupling       InvestigationCoupling
    HasUserBuckets bool
}

type InvestigationUnit struct {
    ID              string
    Index           int
    Label           string
    Summary         string
    Entities        []string
    Scopes          []string
    Role            InvestigationRole
    Coupling        InvestigationCoupling
    AnswerPartition InvestigationAnswerPartition
    EvidenceOrigins []AnswerEvidenceOrigin
    Source          InvestigationUnitSource
}
```

Derived from existing fields:

- `Buckets >= 2`: principal user partitions.
- `SubTopics >= 2`: analyzer investigation units.
- Mixed external/current-source origins come from `AnswerIntentContract`.
- Sequential/call-chain and comparison signals come from typed intent/family
  fields, not raw prose.

The contract is deliberately derived for the first batch. A future analyzer
schema field may emit richer per-unit metadata, but only after the derived
contract proves useful and safe.

## UX

### Status Line

Replace:

- `识别到 3 个关注点`
- `关注点 2：`
- `第 2 个关注点，共 3 个`

With:

- `拆分为 3 个调查单元`
- `调查单元 2：`
- `第 2 个调查单元，共 3 个`

When the plan is bucket-backed, persistent scrollback should say:

- `分析保留了 2 个用户分区：`

When it is analyzer decomposition:

- `分析拆分为 3 个调查单元：`

English equivalents:

- `3 investigation units`
- `Unit 2:`
- `unit 2 of 3`
- `Analyzer kept 2 user partitions:`

### Analysis Summary

`emit_analysis` summary should render:

- `调查单元 N 个：...` for sub-topics.
- `用户分区 N 个：...` for buckets.
- existing `答案维度` and `源码关联` lines remain unchanged.

The summary remains a compact overview, not a control surface.

## Task List

| ID | Status | Task | Validation |
|----|--------|------|------------|
| U1 | Done | Record design and task list. | document landed |
| U2 | Done | Add derived `InvestigationPlan` types and compiler from existing typed IR. | `go test ./internal/types -run InvestigationPlan` |
| U3 | Done | Project investigation unit metadata into `EventAnalysisReady` without changing scheduler semantics. | render/orchestrator unit tests |
| U4 | Done | Update REPL status wording from focus areas to investigation units/user partitions. | `go test ./internal/render -run 'Topic|SubTopics|AnalysisTool'` |
| U5 | Done | Update `emit_analysis` compact summary to read `summary` and render units/buckets. | render summary tests |
| U6 | Done | Add focused eval cases for loosely-coupled multi-question requests and bucketed mixed-origin requests. | `eval/cases/read_combo_loose_multi_question_units.case`, `eval/cases/read_combo_log_current_source_bucketed_units.case`; 20260524 focused runs passed |
| U7 | Pending | After eval data, decide whether scheduler should use `InvestigationPlan.Coupling` for dispatch grouping. Hard behavior changes require separate design. | no runtime hard gate before evidence |

## Implementation Notes

- Batch 1 is display/projection only. It intentionally does not alter
  `expandEvidenceNodes`, completion gates, finalizer contracts, or semantic
  reviewers.
- Focused evals added in this batch passed with `finalizer_iters=1` and
  `finalizer_rejects=0`, covering both a loosely-coupled two-question request
  and a mixed log/current-source request.
- The next safe step, if eval proves value, is to let parallel exploration group
  independent units differently from dependent/sequential units. That must use
  typed `InvestigationCoupling`, not raw question/model prose.

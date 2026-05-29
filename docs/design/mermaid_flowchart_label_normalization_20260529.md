# Mermaid Flowchart Label Normalization Contract

Date: 2026-05-29

## Problem

Mermaid diagrams in final answers are model-authored content, but the output
surface has three render paths:

- `output/*.md`
- sibling standalone `output/*.html`
- HTTP preview server

All three already route through `internal/mermaidcompat`, so render failures
caused by model-adjacent Mermaid syntax should be repaired once in that common
layer. Recent failures share one root cause: visible labels carry punctuation
that Mermaid also uses as syntax (`[]`, `{}`, `()`, `|`, `>`), and the repair
logic is distributed across several narrow functions. That makes each new label
shape look like a new bug.

## Red Lines

- Do not alter topology: node ids, edge direction, edge operators, and diagram
  statement order must remain model-authored unless an id must be aliased for
  Mermaid syntax portability.
- Do not use user text or model prose keyword matching to decide answer logic.
  This layer only repairs Mermaid syntax surfaces after the model has already
  produced a diagram.
- Do not gate or retry the model because Mermaid label text is odd. Repair when
  deterministic and meaning-preserving; otherwise preserve raw source/fallback.
- Keep request markdown verbatim. Only final answer Mermaid fences are repaired.

## Existing Code

- Common markdown/output entry:
  - `internal/outputdump/output_dump.go::BuildBody`
  - `internal/preview/markdown.go::normalizeBrowserMermaid`
  - `internal/tool/answer_block_normalize.go::normalizeDiagramPayload`
- Current Mermaid repair root:
  - `internal/mermaidcompat/NormalizeSourceForMarkdown`
- Existing fragmented flowchart label repairs:
  - `NormalizeFlowchartNodeLabels`
  - `NormalizeFlowchartPipeLabels`
  - `NormalizeFlowchartSubgraphTitles`
  - `NormalizeFlowchartMalformedBracketLabels`
  - `NormalizeFlowchartUnsafeNodeIDs`

## Generalized Design

Introduce a single label quoting policy inside `mermaidcompat` and make every
flowchart label repair use it:

1. Recognize label-bearing surfaces by structure, not prose:
   - node shape labels: `A[...]`, `A{...}`, `A((...))`, `A[[...]]`, etc.
   - pipe edge labels: `A -->|...| B`
   - subgraph titles: `subgraph id [title]` or bare title repair output
   - directive refs (`click/style/class`) only follow syntax-safe aliases; they
     do not rewrite visible labels.
2. Normalize visible label text with one helper:
   - preserve leading/trailing whitespace around the label payload;
   - quote only when label is parser-sensitive or the caller has proven the
     label was malformed;
   - escape embedded double quotes as `&quot;`;
   - keep already quoted labels byte-stable.
3. Repair malformed bracket labels through the same helper:
   - `B[[GT]worker>prio=20]` becomes `B["[GT]worker>prio=20"]`;
   - valid subroutine labels such as `S[[valid subroutine]]` stay subroutine
     syntax.
4. Keep all repairs source-level and deterministic so `.md`, `.html`, preview,
   answer document diagram bodies, and terminal/browser renderers stay aligned.

## Task List

- [x] T1 Document the unified label normalization contract.
- [x] T2 Refactor node, edge, subgraph, and malformed-bracket repairs to use one
  shared quote/escape helper.
- [x] T3 Add regression matrix for parser-sensitive labels:
  - trace/thread labels with `[GT]`, `>`, `<br/>`;
  - code/config labels with `()`, `{}`, `[]`, `|`;
  - subgraph titles with brackets/quotes;
  - valid subroutine/decision/database shapes.
- [x] T4 Verify common output paths:
  - `internal/mermaidcompat`
  - `internal/preview`
  - `internal/outputdump`
  - full `make test`
- [x] T5 Refresh this document with implementation status and pushed commit.

## Progress

2026-05-29:

- Added shared `normalizeFlowchartVisibleLabel` / `quoteFlowchartLabel`
  helpers. Node labels, pipe edge labels, malformed bracket labels, merged pipe
  fragments, and repaired subgraph titles now share the same quote/escape
  policy.
- `NormalizeFlowchartPipeLabels` now skips node shapes before scanning pipe
  delimiters, preventing labels such as `A[stage|slot]` from being corrupted by
  the edge-label repair pass.
- Added regression tests for:
  - customer trace labels (`[GT]codraxNode...>prio=...`);
  - unified node/edge/subgraph label quoting;
  - preservation of valid `S[[subroutine]]` syntax;
  - existing output dump and preview Mermaid paths.
- Verification passed:
  - `go test ./internal/mermaidcompat ./internal/preview ./internal/outputdump`
  - `make test`
- Implementation pushed in commit `c95a2fbb` (`Unify Mermaid flowchart label
  quoting`).

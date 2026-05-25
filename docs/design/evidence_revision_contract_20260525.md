# Evidence Revision Contract (2026-05-25)

## Problem

Customer logs showed an explorer correctly noticing that a previously accepted
`emit_evidence` row had the wrong metadata (`anchor_kind=call` where the same
source line should be treated as a definition), but then hesitating to re-emit
the corrected row because the system reports duplicates as no progress.

The current implementation already has the right identity primitive for normal
dedupe: `StableEvidenceID` intentionally excludes mutable grounding metadata
such as `anchor_kind` / `anchor_symbol`. However, compatibility repair can also
change the semantic `EvidenceKind` (for example `direct + call` may be repaired
to `mechanism`), so metadata correction needs one additional typed key:
`EvidenceRevisionKey`, a source/line/durable-token key used only to recognize
same-anchor amendments. Before this batch, `MutableState.AppendEvidence` stored
an append-only list and several downstream merge paths preserved the first
non-empty metadata value. Result: a corrected row could be appended, but the old
metadata might still win later, and full snapshots could show duplicated
evidence rows.

## Contract

`emit_evidence` remains the only model-facing source-code evidence channel.
There is no new correction tool.

The system must interpret evidence rows by two layers:

1. **Stable fact identity**: `StableEvidenceID` identifies the same semantic
   evidence fact across retries, parallel lanes, markdown fallback, and
   deterministic enrichment.
2. **Revision candidate identity**: `EvidenceRevisionKey` identifies a safe
   same-anchor correction candidate by typed source/scope/line/token. It is only
   used when the stable ID does not match but the model points at the same
   concrete anchor.
3. **Mutable evidence metadata**: fields that describe how the same fact is
   anchored or rendered (`anchor_kind`, `anchor_symbol`, owner, snippet,
   grounding details, surface terms, salience, rich summaries) may be amended by
   re-emitting the same stable fact.

Rules:

- Exact duplicate same-ID rows remain a successful no-op and carry
  `evidence_duplicate_noop` so loop controllers do not treat them as progress.
- Same-ID or same-revision-key rows with stronger or corrected metadata are
  accepted as amendments. They must not create duplicated answer-grade evidence
  in full snapshots.
- Mid-loop explorer observers still need to see the correction event, so the
  raw append tail is preserved for `EmittedEvidenceSince`.
- Full downstream snapshots (`EmittedEvidence`, Turn-A merges, parent-fork
  merges, exact-resolution pools, finalizer support plans) must see a compacted
  same-ID merge where latest non-empty anchor metadata can replace stale
  metadata and rich summaries / surface terms are preserved by union.
- This is a typed structural contract. It must not inspect user prose or model
  free-form prose to decide whether a correction is allowed.

## Implementation Plan

- Add one shared typed merge helper in `internal/types` for same-ID /
  same-revision-key evidence amendments instead of duplicating merge policy in
  every consumer.
- Route existing merge paths through that helper:
  - `MutableState.EmittedEvidence()` full snapshot compaction.
  - `mergeEvidenceByStableIDItem` for fork / Turn-A merges.
  - agent-side `mergeEvidenceItems` / incremental no-op detection.
  - exact-resolution surface pool merging.
- Keep `EmittedEvidenceSince` append-tail semantics so mid-loop progress
  remains visible.
- Update `emit_evidence` tool description and duplicate feedback so models know
  they may re-emit the same source/line/fact with corrected fields.
- Add regression tests:
  - exact duplicate remains no-op;
  - same-ID `anchor_kind` correction updates full snapshots without increasing
    answer-grade evidence count;
  - same-ID corrections still appear in delta tails for the current explorer
    loop;
  - downstream merge keeps corrected anchor metadata and merged summaries.

## Status

- 2026-05-25: Implemented `EvidenceRevisionKey`,
  `MergeEvidenceItemByStableID`, compacted full `EmittedEvidence()` snapshots,
  raw delta preservation for `EmittedEvidenceSince`, tool feedback for
  amendments, and focused regression tests in `internal/types`,
  `internal/agent`, and `internal/tool`.

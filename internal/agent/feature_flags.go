package agent

// This file used to host runtime feature flags for the explorer's
// evidence channel, the two-turn explorer topology, and the P2.2
// answer-document finalizer. All three were deleted in the 2026-04-14
// simplification pass:
//
//   - evidence_tool_mode → emit_evidence is unconditionally
//     registered for the explorer; the markdown parser still runs
//     as a secondary merge channel.
//   - two_turn_explorer_mode → Turn A (explorer) + Turn B (extractor)
//     is the only investigation topology. The single-turn legacy
//     path is gone.
//   - answer_document_mode → answerDocumentEvaluator is the only
//     finalizer; the legacy prose finalizer + S3 validators + shape
//     validators are gone.
//
// The file is intentionally left in place (empty package-level
// content) as a landing spot for future runtime flags. Deleting
// it forces an import-graph reshuffle for any future flag that
// needs to live near the agent layer.

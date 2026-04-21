// Package logtriage holds the system-side helpers for the log_triage
// pre-stage. The stage itself is an LLM-driven extractor: an agent
// under internal/agent/log_triager reads the user-attached runtime
// log and emits a structured LogBundle via the emit_log_triage tool.
// This package's job is the complementary half — validate the LLM's
// output and compute the system-derived Layer 4 fields (ResolvedFiles,
// Entities, IntentHint, Coverage) the bundle's downstream consumers
// read.
//
// Responsibility boundary (load-bearing):
//
//   - LLM fills Layer 1 (Meta), Layer 2 (Errors), Layer 3 (Residue).
//     It never fills Layer 4. The emit tool's JSON schema rejects any
//     attempt to populate those fields directly.
//
//   - This package fills Layer 4 exactly once per bundle, inside
//     ValidateBundle. Layer 1-3 values are treated as immutable post-
//     emission; the validator reads them and derives Layer 4 without
//     mutating the LLM's structured output.
//
// Deterministic guardrails ValidateBundle enforces:
//
//  1. Schema cross-field constraints (emit tool catches single-field
//     violations; ValidateBundle catches cross-field ones — e.g. at
//     least one Error OR Residue.UnknownChunks must be non-empty).
//  2. Path safety and normalisation: StripBuildPathPrefix → Java
//     basename resolver → os.Stat inside repo → filepath.Rel rejecting
//     "../" escapes. Frames whose File cannot be verified keep Func/Pkg
//     (they contribute to Entities) but are dropped from ResolvedFiles.
//  3. Dedup and stable ordering: ResolvedFiles by Confidence desc,
//     Entities preserves first-seen order with a 32-cap, Signals are
//     set-deduped.
//  4. Sanity checks: Line > 0 for ResolvedFiles promotion, Func length
//     >= 2 for Entities inclusion, Error.Type length >= 3, Confidence
//     clamped to [0, 1].
//  5. Layer 4 derivation: ResolvedFiles, Entities, IntentHint, Coverage
//     computed from the validated Layer 1-3 values.
//
// The package has one entry point — ValidateBundle — and three
// pure-function helpers (StripBuildPathPrefix, ResolveJavaFile,
// IsRuntimeInternalFile) re-exported from what used to live in the
// deleted internal/analysis/logparse package.
package logtriage

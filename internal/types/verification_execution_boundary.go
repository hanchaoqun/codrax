package types

// VerificationTargetExecutionEvidenceBoundary is shared presentation guidance
// for already-selected weak/unknown changed-path observations. It does not
// classify evidence, select a follow-up, or grant execution/behavior authority.
// In particular, unknown capability is not an observation of static-only work
// or proof that the production target was never executed.
const VerificationTargetExecutionEvidenceBoundary = "source_static/syntax_only coverage proves source shape only; " +
	"unknown capability does not establish what the check exercised. " +
	"None of these observations alone establishes target execution or target behavior, " +
	"so do not select all_verified from report passed status alone"

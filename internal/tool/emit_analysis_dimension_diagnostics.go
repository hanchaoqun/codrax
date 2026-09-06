package tool

import "strings"

// emitAnalysisWithDimensionDiagnostics carries the existing normalizer's
// explanation across a later runtime-contract rejection. It does not classify
// warning text, re-check provenance, alter admission, or repair model fields.
// In particular, an unanchored causal dimension must not be diagnosed only as
// an absent role: the model already submitted that role and needs to repair
// its source quote, not delete unrelated requested presentation dimensions.
func emitAnalysisWithDimensionDiagnostics(summary string, warnings []string) string {
	if len(warnings) == 0 {
		return summary
	}
	return summary + "\nRequested answer-dimension normalization diagnostics:\n- " + strings.Join(warnings, "\n- ") +
		"\nA row reported as unanchored was discarded before this consistency check; this does not mean its submitted role is unsupported. " +
		"For that row, preserve the model-chosen role and required value while copying a short contiguous verbatim current-request phrase into source_quote (or a verbatim label). " +
		"Do not use ellipses, assembled phrases, or paraphrases as quotes. Other requested dimensions may coexist; do not delete or merge them merely to repair this provenance failure. " +
		"Re-emit the complete object with the existing scope/fact_families contract intact. The system has not selected a replacement quote, role, scope, or conclusion."
}

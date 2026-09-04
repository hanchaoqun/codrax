package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestNoInternalTermsInOrchestratorStrings is the orchestrator's
// package-lane marker in the glossarylint renderer roster (§40.52). It
// catches internal-term leaks the reviewer-prompt audit does not scan:
// validator Violation.Detail / Repair / SuspectedRoot.Reason fields,
// retry-hint composer feed strings, repair-cluster Reason fields,
// controller planning hints — all of which the orchestrator funnels
// into the next dispatch's prompt surface.
//
// History: this gate used to run against a hand-mirrored 22-entry
// subset of the glossary (localInternalTerms) justified by an import
// cycle that did not exist. §40.52 switched it to the full vocabulary
// through glossarylint; the first full-list run surfaced 14 leaks
// ("AnswerDocumentV2 block", "(V2 carrier)", "BlockDiagram",
// "FacetCoverage", "SourceCandidate", "diagram spine", "upstream
// stage", "WriteAnalysisIR scope anchors", "ReadSet", "downstream
// stages"), all reworded.
//
// Policy (structural, never a term allow-list):
//   - const values are skipped: typed enum constants such as
//     FallbackTarget = "finalizer_only" are wire identifiers; the
//     package's prose consts (reviewer system prompts) are scanned by
//     the prompt-surface lane in llm_facing_jargon_audit_test.go;
//   - assignments to *Error targets are skipped: TaskState.LastError
//     is a fail-loud operator diagnostic, not prompt input;
//   - write_run_guidance.go renders bilingual config-knob guidance to
//     the operator and is exempted through the typed closed-set lane.
func TestNoInternalTermsInOrchestratorStrings(t *testing.T) {
	glossarylint.RunPackageScan(t, ".", glossarylint.Policy{
		SkipConstRHS:     true,
		SkipErrorTargets: true,
		Exempt: []glossarylint.Exemption{
			{File: "write_run_guidance.go", Reason: glossarylint.ExemptOperatorFacing},
		},
	})
}

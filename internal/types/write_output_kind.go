package types

// WriteOutputKind classifies the type of artifact a write-mode
// dispatch produces. The sole typed home for write-mode artefacts;
// read-mode output is ALWAYS the AnswerDocumentV2 carrier and this
// enum is irrelevant there.
type WriteOutputKind string

const (
	// WriteOutputChangePlan is the planner agent's emit_change_plan
	// output (file changes + dependencies + critique).
	WriteOutputChangePlan WriteOutputKind = "change_plan"

	// WriteOutputChangeReport is the apply+verify combined
	// post-execution report (test results + baseline diff).
	WriteOutputChangeReport WriteOutputKind = "change_report"

	// WriteOutputFinalReport is the terminal delivery audit projection for a
	// write workflow. It summarizes typed plan/report/workflow/handoff state;
	// it is not a scheduler authority and never parses final-answer prose.
	WriteOutputFinalReport WriteOutputKind = "final_report"
)

// AllWriteOutputKinds returns every declared WriteOutputKind in a
// stable enumeration order. Mirrors the AllAnchorKinds /
// AllViolationKinds / AllAnswerBlockKinds pattern. Adding a new
// write-mode artifact kind requires updating this function.
func AllWriteOutputKinds() []WriteOutputKind {
	return []WriteOutputKind{
		WriteOutputChangePlan,
		WriteOutputChangeReport,
		WriteOutputFinalReport,
	}
}

// IsValidWriteOutputKind reports whether k is a declared
// WriteOutputKind. Empty string is invalid.
func IsValidWriteOutputKind(k WriteOutputKind) bool {
	for _, declared := range AllWriteOutputKinds() {
		if k == declared {
			return true
		}
	}
	return false
}

package types

// WriteOutputKind classifies the type of artifact a write-mode
// dispatch produces. Introduced by B8-T0 (block_only_carrier.md
// §5.8) as the destination for the V1 ShapeChangePlan /
// ShapeChangeReport enum values that previously lived inside
// AnswerShape. Splitting them out of AnswerShape is a hard
// prerequisite for B8-T5's "delete AnswerShape entirely" step:
// the read-mode answer carrier is already block-only by B6, so
// once write-mode artifacts have their own typed home, AnswerShape
// has zero remaining consumers.
//
// Read-mode output is ALWAYS the V2 AnswerDocumentV2 carrier; this
// enum is irrelevant there.
type WriteOutputKind string

const (
	// WriteOutputChangePlan is the planner agent's emit_change_plan
	// output (file changes + dependencies + critique).
	WriteOutputChangePlan WriteOutputKind = "change_plan"

	// WriteOutputChangeReport is the apply+verify combined
	// post-execution report (test results + baseline diff).
	WriteOutputChangeReport WriteOutputKind = "change_report"
)

// AllWriteOutputKinds returns every declared WriteOutputKind in a
// stable enumeration order. Mirrors the AllAnchorKinds /
// AllViolationKinds / AllAnswerBlockKinds pattern. Adding a new
// write-mode artifact kind requires updating this function.
func AllWriteOutputKinds() []WriteOutputKind {
	return []WriteOutputKind{
		WriteOutputChangePlan,
		WriteOutputChangeReport,
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

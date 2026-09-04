package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestNoInternalTermsInToolSourceLiterals is internal/tool's static
// marker in the glossarylint renderer roster (§40.52). Tool source
// files carry model-facing text in places the schema gate
// (TestNoInternalTermsInToolSchemas) does not see: ToolResult.Summary
// rejection strings, per-tool emitFixHint Reason fields, mid-loop hint
// builders. Any glossary hit in a non-logger string literal here is the
// same R6 leak as one in a skill Workflow item.
//
// wiring_anchors.go is a repo-path data table matched as data by the
// runtime, never rendered as prose — exempted through the typed
// closed-set lane (a stale row fails the scan).
func TestNoInternalTermsInToolSourceLiterals(t *testing.T) {
	glossarylint.RunPackageScan(t, ".", glossarylint.Policy{
		Exempt: []glossarylint.Exemption{{File: "wiring_anchors.go", Reason: glossarylint.ExemptDataTable}},
	})
}

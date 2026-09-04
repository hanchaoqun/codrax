package ground

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestNoInternalTermsInGroundLiterals is internal/tool/ground's marker
// in the glossarylint renderer roster (§40.52). CitationReport.Reason
// is copied verbatim into ToolResult.Summary by the citing tools, so
// every non-logger literal here is model-facing.
func TestNoInternalTermsInGroundLiterals(t *testing.T) {
	glossarylint.RunPackageScan(t, ".", glossarylint.Policy{})
}

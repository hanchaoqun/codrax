package context

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestNoInternalTermsInPromptContextLiterals is internal/context's
// marker in the glossarylint renderer roster (§40.52). BuildPromptContext
// assembles every system/user section the agents send, so this package
// was the largest unscanned renderer: the first red run listed six
// live leaks ("extraction stage", "the analyzer", "downstream stages",
// "BlockScalar", "literal-grounding gate", "findings_validator").
func TestNoInternalTermsInPromptContextLiterals(t *testing.T) {
	glossarylint.RunPackageScan(t, ".", glossarylint.Policy{})
}

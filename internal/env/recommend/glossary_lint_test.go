package recommend

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestNoInternalTermsInEnvRecommendLiterals is internal/env/recommend's
// marker in the glossarylint renderer roster (§40.52): the environment
// recommendation lane builds its own llm.ToolSchema and system prompt.
func TestNoInternalTermsInEnvRecommendLiterals(t *testing.T) {
	glossarylint.RunPackageScan(t, ".", glossarylint.Policy{})
}

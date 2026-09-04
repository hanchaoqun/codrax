package repomap

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestNoInternalTermsInRepoMapLiterals is internal/tool/repomap's
// marker in the glossarylint renderer roster (§40.52): the repo_map
// tool's Description/Parameters live here, outside internal/tool's
// non-recursive scan.
func TestNoInternalTermsInRepoMapLiterals(t *testing.T) {
	glossarylint.RunPackageScan(t, ".", glossarylint.Policy{})
}

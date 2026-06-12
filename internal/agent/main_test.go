package agent

import (
	"os"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

// TestMain redirects the repomap cache base to a per-run temp dir.
// Several tests in this package build real graphs through production
// paths (BuildInitialInstruction's repo-overview pre-injection,
// keyword_search's grep phase, …) with t.TempDir repos; without the
// redirect every `go test` run minted one orphaned cache dir per such
// test under the developer's real ~/.codrax/cache/repomap — thousands
// accumulated before the startup GC existed, and stopping the leak at
// its source beats relying on the GC to mop up.
func TestMain(m *testing.M) {
	base, err := os.MkdirTemp("", "codrax-agent-test-repomap-*")
	if err == nil {
		repomap.SetCacheDir(base)
	}
	code := m.Run()
	if err == nil {
		_ = os.RemoveAll(base)
	}
	os.Exit(code)
}

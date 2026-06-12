package repomap

import (
	"os"
	"testing"
)

// TestMain redirects the repomap cache base to a per-run temp dir.
// Tests in this package drive Execute/GraphFromBusContextOrLoad over
// t.TempDir repos through the production scan path; without the
// redirect every `go test` run minted orphaned cache dirs under the
// developer's real ~/.codrax/cache/repomap (see the startup GC in
// index.PruneOrphanedCacheDirs — stopping the leak at its source
// beats relying on the GC to mop up).
func TestMain(m *testing.M) {
	base, err := os.MkdirTemp("", "codrax-repomap-test-cache-*")
	if err == nil {
		SetCacheDir(base)
	}
	code := m.Run()
	if err == nil {
		_ = os.RemoveAll(base)
	}
	os.Exit(code)
}

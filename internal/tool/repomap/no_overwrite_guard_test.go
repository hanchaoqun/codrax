package repomap

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/index"
)

// TestRejectedCacheRebuildDirNeverOverwritesNewerSchema pins the
// mixed-version guard: a cache written by a NEWER binary makes the
// fallback full scan run in-memory only (empty cacheDir disables the
// cache writer and the save block), so an old binary can never
// downgrade-overwrite a newer cache — the one-way cut in the
// full-rescan ping-pong (architecture.md §14.7).
func TestRejectedCacheRebuildDirNeverOverwritesNewerSchema(t *testing.T) {
	if got := rejectedCacheRebuildDir("/some/cache/dir", index.CacheRejectSchemaNewer); got != "" {
		t.Fatalf("newer-schema rejection must return empty cacheDir (no overwrite), got %q", got)
	}
	for _, reason := range []index.CacheRejectReason{
		index.CacheRejectSchemaVersion,
		index.CacheRejectExtractorVersion,
		index.CacheRejectCorrupt,
	} {
		if got := rejectedCacheRebuildDir("/some/cache/dir", reason); got != "/some/cache/dir" {
			t.Fatalf("%s must keep the cacheDir for ordinary rebuild, got %q", reason, got)
		}
	}
}

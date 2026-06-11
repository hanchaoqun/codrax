package topology

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTopologyCacheFile(t *testing.T, dir, slug, parentRoot string, age time.Duration) string {
	t.Helper()
	data, _ := json.Marshal(RepoTopology{ParentRoot: parentRoot, ParentSlug: slug})
	p := filepath.Join(dir, slug+".json")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-age)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPruneVanishedTopologyCaches(t *testing.T) {
	dir := t.TempDir()
	liveRoot := t.TempDir()

	keep := writeTopologyCacheFile(t, dir, "just-written", "/does/not/exist-a", time.Hour)
	liveOld := writeTopologyCacheFile(t, dir, "live-root", liveRoot, time.Hour)
	vanishedOld := writeTopologyCacheFile(t, dir, "vanished-old", "/does/not/exist-b", time.Hour)
	vanishedFresh := writeTopologyCacheFile(t, dir, "vanished-fresh", "/does/not/exist-c", time.Minute)
	corruptOld := filepath.Join(dir, "corrupt.json")
	os.WriteFile(corruptOld, []byte("{not json"), 0o644)
	past := time.Now().Add(-time.Hour)
	os.Chtimes(corruptOld, past, past)

	pruneVanishedTopologyCaches(dir, filepath.Base(keep))

	for _, p := range []string{keep, liveOld, vanishedFresh} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s must survive: %v", filepath.Base(p), err)
		}
	}
	for _, p := range []string{vanishedOld, corruptOld} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s must be pruned", filepath.Base(p))
		}
	}
}

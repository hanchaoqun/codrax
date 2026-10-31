package attachment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func TestHMC325MaterialGenerationCacheKeyBindsEveryOriginalVersion(t *testing.T) {
	if (*TraceMaterial)(nil).GenerationCacheKey() != "" {
		t.Fatal("nil material manufactured a generation key")
	}
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "source"), filepath.Join(dir, "query"), filepath.Join(dir, "sidecar")}
	bindings := make(map[string]filegeneration.Identity)
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("unchanged bytes"), 0600); err != nil {
			t.Fatal(err)
		}
		id, err := filegeneration.FromPath(path)
		if err != nil {
			t.Fatal(err)
		}
		bindings[path] = id
	}
	first := &TraceMaterial{sourcePath: paths[0], queryPath: paths[1], bindings: bindings}
	reordered := &TraceMaterial{sourcePath: paths[0], queryPath: paths[1], bindings: map[string]filegeneration.Identity{
		paths[2]: bindings[paths[2]], paths[1]: bindings[paths[1]], paths[0]: bindings[paths[0]],
	}}
	original := first.GenerationCacheKey()
	if original == "" || original != reordered.GenerationCacheKey() {
		t.Fatal("map insertion order changed the same bound material identity")
	}
	// Only a sidecar changes generation, with byte-identical content. The
	// original/query pair alone would fail to distinguish this material.
	replacement := filepath.Join(dir, "replacement")
	if err := os.WriteFile(replacement, []byte("unchanged bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, paths[2]); err != nil {
		t.Fatal(err)
	}
	newVersion, err := filegeneration.FromPath(paths[2])
	if err != nil {
		t.Fatal(err)
	}
	if newVersion.SameVersion(bindings[paths[2]]) {
		t.Fatal("test fixture did not replace a physical generation")
	}
	if first.GenerationCacheKey() != original {
		t.Fatal("cache key inspected current filesystem rather than the immutable held versions")
	}
	changed := *reordered
	changed.bindings = map[string]filegeneration.Identity{paths[0]: bindings[paths[0]], paths[1]: bindings[paths[1]], paths[2]: newVersion}
	if changed.GenerationCacheKey() == original {
		t.Fatal("additional bound file version was omitted from the identity")
	}
	for _, mutate := range []func(*TraceMaterial){
		func(m *TraceMaterial) { m.sourcePath, m.queryPath = m.queryPath, m.sourcePath },
		func(m *TraceMaterial) { m.queryDomain = TraceQueryDomainPacketCapture },
		func(m *TraceMaterial) { m.archiveMember = "selected-member"; m.explicitArchiveSelection = true },
	} {
		copy := *first
		mutate(&copy)
		if copy.GenerationCacheKey() == original {
			t.Fatal("material selection/domain identity was omitted")
		}
	}
}

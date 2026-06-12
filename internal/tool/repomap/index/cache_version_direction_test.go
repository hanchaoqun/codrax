package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// writeManifestWithSchema installs a minimal well-formed manifest+chunk
// whose header carries the given schema version.
func writeManifestWithSchema(t *testing.T, dir string, schema int) {
	t.Helper()
	if err := saveFileInfos(dir, "", []*types.FileInfo{{
		RelPath: "a.go", Language: types.LangGo, Hash: "deadbeef",
		Symbols: []types.Symbol{{Name: "Foo", Kind: "type", File: "a.go", Line: 1, EndLine: 1}},
	}}); err != nil {
		t.Fatal(err)
	}
	mp := filepath.Join(dir, cacheFileInfosManifestFile)
	raw, err := os.ReadFile(mp)
	if err != nil {
		t.Fatal(err)
	}
	var m cacheFileInfosManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m.SchemaVersion = schema
	out, err := json.Marshal(&m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mp, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLoadRejectsWithDirectionalTypedReasons pins the two schema
// directions on the typed loader: OLDER than current → ordinary
// schema_version_mismatch (rebuild + overwrite is the upgrade path);
// NEWER than current → schema_version_newer (the mixed-binary
// no-overwrite guard keys on exactly this distinction).
func TestLoadRejectsWithDirectionalTypedReasons(t *testing.T) {
	older := t.TempDir()
	writeManifestWithSchema(t, older, cacheSchemaVersion-1)
	if got := LoadFileInfosResult(older, nil); got.RejectReason != CacheRejectSchemaVersion || got.Files != nil {
		t.Fatalf("older schema: reason=%q files=%v, want schema_version_mismatch/nil", got.RejectReason, got.Files != nil)
	}

	newer := t.TempDir()
	writeManifestWithSchema(t, newer, cacheSchemaVersion+1)
	if got := LoadFileInfosResult(newer, nil); got.RejectReason != CacheRejectSchemaNewer || got.Files != nil {
		t.Fatalf("newer schema: reason=%q files=%v, want schema_version_newer/nil", got.RejectReason, got.Files != nil)
	}

	// PeekCacheCompatibility (the pre-hash-pass shortcut) must agree.
	if got := PeekCacheCompatibility(newer); got != CacheRejectSchemaNewer {
		t.Fatalf("peek on newer schema: %q, want schema_version_newer", got)
	}
}

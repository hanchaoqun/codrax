package index

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestCallableBodyPresenceCacheRoundTripAndEpoch(t *testing.T) {
	if cacheSchemaVersion < 7 {
		t.Fatal("new parser authority must invalidate the old cache schema")
	}
	prior := map[string]int{types.LangGo: 11, types.LangJava: 10, types.LangPython: 12, types.LangJavaScript: 9, types.LangTypeScript: 12, types.LangArkTS: 13, types.LangCangjie: 10, types.LangKotlin: 10, types.LangRuby: 7, types.LangSwift: 10, types.LangLua: 7, types.LangRust: 12, types.LangC: 11, types.LangCpp: 16}
	for lang, version := range prior {
		if extractorVersions[lang] <= version {
			t.Errorf("%s body semantics changed without extractor version bump", lang)
		}
	}
	dir := t.TempDir()
	files := []*types.FileInfo{{RelPath: "p.go", Language: types.LangGo, Symbols: []types.Symbol{
		{Name: "actual", Kind: "function", File: "p.go", Line: 2, EndLine: 5, BodyPresence: types.CallableBodyPresent, BodyStartLine: 3, BodyEndLine: 5},
		{Name: "declared", Kind: "function", File: "p.go", Line: 6, EndLine: 6, BodyPresence: types.CallableBodyAbsent},
		{Name: "unknown", Kind: "function", File: "p.go", Line: 7, EndLine: 7, BodyPresence: types.CallableBodyUnknown},
	}}}
	if err := saveFileInfos(dir, "", files); err != nil {
		t.Fatal(err)
	}
	loaded := LoadFileInfos(dir)
	if len(loaded) != 1 || len(loaded[0].Symbols) != 3 {
		t.Fatalf("cache roundtrip=%+v", loaded)
	}
	for i, sym := range loaded[0].Symbols {
		if sym.BodyPresence != files[0].Symbols[i].BodyPresence || sym.BodyStartLine != files[0].Symbols[i].BodyStartLine || sym.BodyEndLine != files[0].Symbols[i].BodyEndLine {
			t.Fatalf("cache changed body authority: %+v", sym)
		}
	}
	manifest := readFileInfosManifestForTest(t, dir)
	manifest.SchemaVersion = 6
	writeFileInfosManifestForTest(t, dir, manifest)
	if got := LoadFileInfos(dir); got != nil {
		t.Fatal("pre-body-presence warm cache survived the new schema")
	}
}

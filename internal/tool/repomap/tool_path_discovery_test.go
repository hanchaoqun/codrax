package repomap

import (
	"testing"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	ctypes "github.com/hanchaoqun/codrax/internal/types"
)

func TestRepoMapViewPathDiscoveryUsesStructuredViewDataFiles(t *testing.T) {
	a := &rmtypes.FileInfo{RelPath: "src/api/routes.py"}
	b := &rmtypes.FileInfo{RelPath: "src/web/router.ts"}
	g := &Graph{
		Files: []*rmtypes.FileInfo{a, b},
		FileIndex: map[string]*rmtypes.FileInfo{
			a.RelPath: a,
			b.RelPath: b,
		},
	}
	data := &ViewData{
		Type: "file_map",
		Sections: []ViewSection{{
			Items: []ViewItem{
				{Text: "`ignored` type :1", File: a.RelPath},
				{Text: "`duplicate` type :2", File: a.RelPath},
				{Text: "`directory-not-file`", File: "src/api"},
				{Text: "`display-only-path` src/mobile/routes.kt"},
				{Text: "`unsafe`", File: "../outside.go"},
			},
			Subsections: []ViewSection{{
				Items: []ViewItem{{Text: "`Router` type :3", File: b.RelPath}},
			}},
		}},
	}

	got := repoMapViewPathDiscovery(g, repoMapParams{Path: "src", View: "file_map"}, data)
	if got == nil {
		t.Fatal("repo_map ordinary view should publish typed path discovery")
	}
	if got.Kind != ctypes.ToolPathDiscoveryKindRepoMap || got.Path != "src" || got.ResultCount != 2 {
		t.Fatalf("unexpected repo_map path discovery metadata: %+v", got)
	}
	wantFiles := []string{a.RelPath, b.RelPath}
	if len(got.CandidateFiles) != len(wantFiles) {
		t.Fatalf("candidate files = %+v, want %+v", got.CandidateFiles, wantFiles)
	}
	for i, want := range wantFiles {
		if got.CandidateFiles[i] != want {
			t.Fatalf("candidate files = %+v, want %+v", got.CandidateFiles, wantFiles)
		}
	}
}

func TestRepoMapViewPathDiscoverySkipsSourceInventory(t *testing.T) {
	fi := &rmtypes.FileInfo{RelPath: "src/a.go"}
	g := &Graph{Files: []*rmtypes.FileInfo{fi}, FileIndex: map[string]*rmtypes.FileInfo{fi.RelPath: fi}}
	data := &ViewData{
		Type:     "source_inventory",
		Sections: []ViewSection{{Items: []ViewItem{{Text: "`A` type :1", File: fi.RelPath}}}},
	}

	if got := repoMapViewPathDiscovery(g, repoMapParams{Path: ".", View: "source_inventory"}, data); got != nil {
		t.Fatalf("source_inventory uses SourceInventory carrier, not PathDiscovery: %+v", got)
	}
}

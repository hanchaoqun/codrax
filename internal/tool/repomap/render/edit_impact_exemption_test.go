package render

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestEditImpactExemptFromAuxiliaryDownranking pins the structural
// exemption the budget design relies on: edit_impact enumerates
// dependents exhaustively (test files are HIGH-value dependents
// there), so the auxiliary downranking flag must not change its
// output in any way. call_path and implementers share the rank-free
// structure; edit_impact is the one the design names explicitly.
func TestEditImpactExemptFromAuxiliaryDownranking(t *testing.T) {
	files := []*types.FileInfo{
		{
			RelPath: "pkg/api.go", Language: types.LangGo, Package: "pkg",
			Symbols: []types.Symbol{{Name: "PublicAPI", Kind: "function", File: "pkg/api.go", Line: 1, EndLine: 3, Exported: true}},
		},
		{
			RelPath: "pkg/api_test.go", Language: types.LangGo, Package: "pkg",
			Relations: []types.Relation{{
				Kind:   "call",
				FromEP: types.RelationEndpoint{File: "pkg/api_test.go", Line: 2},
				ToEP:   types.RelationEndpoint{Name: "PublicAPI", File: "pkg/api_test.go", Line: 2},
				File:   "pkg/api_test.go", Line: 2,
				Confidence: types.ConfidenceAST, Provenance: types.ProvenanceTreeSitter, ResolvedBy: "test_fixture",
			}},
		},
	}
	g := &types.Graph{Root: "/repo", Files: files, FileIndex: map[string]*types.FileInfo{}}
	for _, fi := range files {
		g.FileIndex[fi.RelPath] = fi
	}

	base := GenerateView(g, "edit_impact", types.ViewParams{TargetFile: "pkg/api.go"})
	down := GenerateView(g, "edit_impact", types.ViewParams{TargetFile: "pkg/api.go", DeprioritizeAuxiliary: true})
	if base != down {
		t.Fatalf("edit_impact output changed under DeprioritizeAuxiliary:\n--- base ---\n%s\n--- flagged ---\n%s", base, down)
	}
}

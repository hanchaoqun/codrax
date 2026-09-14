package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// Exercise the public navigation-data and Markdown surfaces. These fixtures
// start at the parser-owned symbol/relation boundary; they do not claim to
// exercise every language parser, nor do advisory rows grant call authority.
func TestRelationMapRespectsKnownDeclarationExtent(t *testing.T) {
	for _, lang := range types.SupportedReadLanguages() {
		for _, kind := range []string{"call", "inheritance", "embedding", "reference", "type_usage"} {
			for _, tc := range []struct {
				name           string
				line, end      int
				outer          bool
				wantSource     string
				wantSourceLine int
			}{
				{"inside", 12, 14, false, "Previous", 10},
				{"inclusive_end", 14, 14, false, "Previous", 10},
				{"after_end", 17, 14, false, "", 0},
				{"after_inner_within_outer", 17, 14, true, "Outer", 1},
				{"unknown_end_legacy", 17, 0, false, "Previous", 10},
				{"before_start", 8, 14, false, "", 0},
			} {
				t.Run(lang+"/"+kind+"/"+tc.name, func(t *testing.T) {
					path := "source/" + lang + ".fixture"
					fi := &types.FileInfo{RelPath: path, Language: lang, Symbols: []types.Symbol{
						{Name: "Previous", Kind: "function", File: path, Line: 10, EndLine: tc.end},
					}}
					if tc.outer {
						// Deliberately place the enclosing declaration after the inner
						// one in the slice: ownership must not depend on slice order.
						fi.Symbols = append(fi.Symbols, types.Symbol{Name: "Outer", Kind: "class", File: path, Line: 1, EndLine: 30})
					}
					fi.Relations = []types.Relation{{Kind: kind, File: path, Line: tc.line,
						ToEP:       types.RelationEndpoint{Name: "Target", File: "target.fixture", Line: 5},
						Confidence: types.ConfidenceAST, Provenance: types.ProvenanceTreeSitter}}
					g := index.BuildGraph(t.TempDir(), []*types.FileInfo{fi})
					d := GenerateViewData(g, "relation_map", types.ViewParams{Sources: []string{path}, RelationKinds: []string{kind}, TopN: 10})
					if d == nil || len(d.RelationRows) != 1 {
						t.Fatalf("must preserve the observed relation even without a containing declaration: %+v", d)
					}
					want := tc.wantSource
					if want == "" {
						want = path
					}
					row := d.RelationRows[0]
					if row.Source != want || row.SourceLine != tc.wantSourceLine || row.SourceFile != path {
						t.Fatalf("incorrect owner for observed line %d: got %q:%d want %q:%d", tc.line, row.Source, row.SourceLine, want, tc.wantSourceLine)
					}
					if row.Kind != kind || row.Target != "Target" || row.ObservedFile != path || row.ObservedLine != tc.line || row.Provenance != types.ProvenanceTreeSitter {
						t.Fatalf("boundary handling changed the observation: %+v", row)
					}
					md := RenderMarkdown(d)
					if !strings.Contains(md, "Advisory structural navigation only") || !strings.Contains(md, row.Text) || !strings.Contains(row.Text, fmt.Sprintf("observed @ %s:%d", path, tc.line)) {
						t.Fatalf("Markdown and typed navigation rows must agree without raising authority: %s", md)
					}
				})
			}
		}
	}
}

func TestRelationMapTypeUsageSelectionPreservesCanonicalRelation(t *testing.T) {
	for _, selection := range [][]string{nil, {"type_usage"}, {"type-usage"}, {" TYPE_USAGE ", "type-usage"}, {"reference"}} {
		t.Run(fmt.Sprint(selection), func(t *testing.T) {
			const path = "source/module.fixture"
			g := index.BuildGraph(t.TempDir(), []*types.FileInfo{{RelPath: path, Language: types.LangGo,
				Symbols:   []types.Symbol{{Name: "Owner", Kind: "function", File: path, Line: 1, EndLine: 9}},
				Relations: []types.Relation{{Kind: "type_usage", File: path, Line: 3, ToEP: types.RelationEndpoint{Name: "Value"}}},
			}})
			d := GenerateViewData(g, "relation_map", types.ViewParams{Sources: []string{path}, RelationKinds: selection, TopN: 10})
			if d == nil || len(d.RelationRows) != 1 || d.RelationRows[0].Kind != "type_usage" || d.RelationRows[0].Source != "Owner" {
				t.Fatalf("supported kind spelling must not drop or recast the observation: %+v", d)
			}
			if !strings.Contains(RenderMarkdown(d), d.RelationRows[0].Text) {
				t.Fatal("public Markdown must preserve the same canonical relationship")
			}
		})
	}
}

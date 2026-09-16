package tool

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1694CitationPoolExtentIdentity(t *testing.T) {
	point := types.Citation{File: "src/a.go", Line: 10, Quote: "model point"}
	ranged := types.Citation{File: "src/a.go", Line: 10, LineEnd: 14, Scope: types.ScopeLineRange, Quote: "model range"}
	for _, tc := range []struct {
		name string
		pool []types.Citation
		want types.Citation
		ref  int
	}{
		{"point_does_not_steal_range", []types.Citation{point}, ranged, -1},
		{"range_does_not_steal_point", []types.Citation{ranged}, point, -1},
		{"range_end_is_identity", []types.Citation{ranged}, types.Citation{File: "src/a.go", Line: 10, LineEnd: 15}, -1},
		{"legacy_line_scope_keeps_range", []types.Citation{ranged}, types.Citation{File: "src/a.go", Line: 10, LineEnd: 14, Scope: types.ScopeLine, Quote: "different preview"}, 0},
		{"explicit_single_line_end", []types.Citation{point}, types.Citation{File: "src/a.go", Line: 10, LineEnd: 10, Scope: types.ScopeLineRange}, 0},
		{"point_first_selects_range", []types.Citation{point, ranged}, ranged, 1},
		{"range_first_selects_point", []types.Citation{ranged, point}, point, 1},
		{"legacy_case_normalized_source", []types.Citation{point}, types.Citation{File: "src/A.go", Line: 10}, 0},
		{"unique_suffix", []types.Citation{ranged}, types.Citation{File: "a.go", Line: 10, LineEnd: 14}, 0},
		{"exact_before_suffix", []types.Citation{ranged, {File: "a.go", Line: 10, LineEnd: 14}}, types.Citation{File: "a.go", Line: 10, LineEnd: 14}, 1},
		{"exact_source_different_extent_blocks_suffix", []types.Citation{{File: "a.go", Line: 10}, ranged}, types.Citation{File: "a.go", Line: 10, LineEnd: 14}, -1},
		{"ambiguous_suffix", []types.Citation{ranged, {File: "other/a.go", Line: 10, LineEnd: 14}}, types.Citation{File: "a.go", Line: 10, LineEnd: 14}, -1},
		{"inverted_is_not_point", []types.Citation{point}, types.Citation{File: "src/a.go", Line: 10, LineEnd: 4}, -1},
		{"section_not_line", []types.Citation{ranged}, types.Citation{File: "src/a.go", Line: 10, LineEnd: 14, Scope: types.ScopeSection, SectionPath: "api"}, -1},
		{"line_section_qualifier", []types.Citation{point}, types.Citation{File: "src/a.go", Line: 10, SectionPath: "api"}, -1},
		{"line_crossfile_qualifier", []types.Citation{point}, types.Citation{File: "src/a.go", Line: 10, CrossfileSummary: "contract"}, -1},
		{"line_negative_qualifier", []types.Citation{point}, types.Citation{File: "src/a.go", Line: 10, NegativePattern: "absent"}, -1},
		{"negative_not_line", []types.Citation{point}, types.Citation{File: "src/a.go", Line: 10, Scope: types.ScopeNegative, NegativePattern: "missing"}, -1},
		{"different_file_scope", []types.Citation{{File: "a.go", Scope: types.ScopeFile}}, types.Citation{File: "b.go", Scope: types.ScopeFile}, -1},
		{"same_file_scope", []types.Citation{{File: "a.go", Scope: types.ScopeFile}}, types.Citation{File: "a.go", Scope: types.ScopeFile}, 0},
		{"different_section", []types.Citation{{File: "a.md", Line: 2, Scope: types.ScopeSection, SectionPath: "one"}}, types.Citation{File: "a.md", Line: 2, Scope: types.ScopeSection, SectionPath: "two"}, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]types.Citation(nil), tc.pool...)
			if got := answerDocumentPatchCitationIndex(tc.pool, tc.want); got != tc.ref {
				t.Fatalf("patch ref=%d want=%d", got, tc.ref)
			}
			doc := &types.AnswerDocumentV2{Citations: append([]types.Citation(nil), tc.pool...)}
			wantRef := tc.ref
			if wantRef < 0 {
				wantRef = len(before)
			}
			if got := appendOrReusePreEmitCitation(doc, tc.want); got != wantRef {
				t.Fatalf("full ref=%d want=%d", got, wantRef)
			}
			if !reflect.DeepEqual(doc.Citations[:len(before)], before) || !reflect.DeepEqual(tc.pool, before) {
				t.Fatal("model pool changed")
			}
			if got := appendOrReusePreEmitCitation(doc, tc.want); got != wantRef || len(doc.Citations) != len(before)+b1694AppendCount(tc.ref) {
				t.Fatalf("replay not idempotent: %d %+v", got, doc.Citations)
			}
		})
	}
}

func b1694AppendCount(ref int) int {
	if ref < 0 {
		return 1
	}
	return 0
}

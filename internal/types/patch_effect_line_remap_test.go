package types

import (
	"reflect"
	"testing"
)

// patch_effect_line_remap_test.go — V5-1 (§40.10 item 2): pre-apply line →
// post-apply line binding through hunk tables; every non-mapped outcome is a
// typed status, never a guessed line.
func TestRemapPatchEffectOldLine(t *testing.T) {
	modified := func(hunks ...PatchEffectHunk) *PatchEffectRecord {
		return &PatchEffectRecord{Files: []PatchEffectFile{{Path: "src/main.c", OldPath: "src/main.c", Status: "modified", Hunks: hunks}}}
	}
	insertTop := PatchEffectHunk{OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 3, AddedLines: 3, AddedLineNumbers: []int{1, 2, 3}}
	insertAfter3 := PatchEffectHunk{OldStart: 3, OldLines: 0, NewStart: 4, NewLines: 2, AddedLines: 2, AddedLineNumbers: []int{4, 5}}
	// old 2,3,4 → new 2,3,4,5: old 3 removed, new 3-4 added, old 2/4 kept.
	replaceMid := PatchEffectHunk{OldStart: 2, OldLines: 3, NewStart: 2, NewLines: 4, AddedLines: 2, RemovedLines: 1,
		AddedLineNumbers: []int{3, 4}, RemovedLineTexts: []PatchEffectLine{{Line: 3, Text: "old"}}}
	noTables := PatchEffectHunk{OldStart: 2, OldLines: 3, NewStart: 2, NewLines: 4, AddedLines: 2, RemovedLines: 1}
	removeFive := PatchEffectHunk{OldStart: 5, OldLines: 1, NewStart: 8, NewLines: 0, RemovedLines: 1, RemovedLineTexts: []PatchEffectLine{{Line: 5, Text: "gone"}}}
	cases := []struct {
		name   string
		effect *PatchEffectRecord
		path   string
		line   int
		want   PatchEffectLineRemap
	}{
		{"nil effect", nil, "src/main.c", 3, PatchEffectLineRemap{Status: PatchEffectLineNoPatchEffect}},
		{"invalid line", modified(), "src/main.c", 0, PatchEffectLineRemap{Status: PatchEffectLineInvalid}},
		{"file not in patch", modified(insertTop), "other.c", 3, PatchEffectLineRemap{Path: "other.c", Line: 3, Status: PatchEffectLineFileNotInPatch}},
		{"created file", &PatchEffectRecord{Files: []PatchEffectFile{{Path: "src/new.c", OldPath: "src/new.c", Status: "created"}}}, "src/new.c", 1,
			PatchEffectLineRemap{Path: "src/new.c", Status: PatchEffectLineFileCreated}},
		{"deleted file", &PatchEffectRecord{Files: []PatchEffectFile{{Path: "src/main.c", OldPath: "src/main.c", Status: "deleted"}}}, "src/main.c", 1,
			PatchEffectLineRemap{Status: PatchEffectLineFileDeleted}},
		{"insert above shifts", modified(insertTop), "src/main.c", 3, PatchEffectLineRemap{Path: "src/main.c", Line: 6, Status: PatchEffectLineMapped}},
		{"insertion after the line keeps it", modified(insertAfter3), "src/main.c", 3, PatchEffectLineRemap{Path: "src/main.c", Line: 3, Status: PatchEffectLineMapped}},
		{"line beyond the insertion shifts", modified(insertAfter3), "src/main.c", 4, PatchEffectLineRemap{Path: "src/main.c", Line: 6, Status: PatchEffectLineMapped}},
		{"removed line", modified(replaceMid), "src/main.c", 3, PatchEffectLineRemap{Path: "src/main.c", Status: PatchEffectLineRemovedByPatch}},
		{"context line after the removal", modified(replaceMid), "src/main.c", 4, PatchEffectLineRemap{Path: "src/main.c", Line: 5, Status: PatchEffectLineMapped}},
		{"context line before the removal", modified(replaceMid), "src/main.c", 2, PatchEffectLineRemap{Path: "src/main.c", Line: 2, Status: PatchEffectLineMapped}},
		{"line after the hunk shifts by the delta", modified(replaceMid), "src/main.c", 9, PatchEffectLineRemap{Path: "src/main.c", Line: 10, Status: PatchEffectLineMapped}},
		{"hunk without line tables", modified(noTables), "src/main.c", 4, PatchEffectLineRemap{Path: "src/main.c", Status: PatchEffectLineHunkUnmapped}},
		{"two hunks accumulate", modified(insertTop, removeFive), "src/main.c", 7, PatchEffectLineRemap{Path: "src/main.c", Line: 9, Status: PatchEffectLineMapped}},
		{"rename maps to the new path", &PatchEffectRecord{Files: []PatchEffectFile{{Path: "src/renamed.c", OldPath: "src/main.c", Status: "renamed"}}}, "src/main.c", 2,
			PatchEffectLineRemap{Path: "src/renamed.c", Line: 2, Status: PatchEffectLineMapped}},
		{"dot-slash prefix normalised", modified(insertTop), "./src/main.c", 1, PatchEffectLineRemap{Path: "src/main.c", Line: 4, Status: PatchEffectLineMapped}},
	}
	for _, tc := range cases {
		got := RemapPatchEffectOldLine(tc.effect, tc.path, tc.line)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: got %+v want %+v", tc.name, got, tc.want)
		}
		if got.Mapped() != (tc.want.Status == PatchEffectLineMapped) {
			t.Fatalf("%s: Mapped()=%v disagrees with status %s", tc.name, got.Mapped(), got.Status)
		}
	}
}

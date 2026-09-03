package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// patch_effect_parser_body_lines_test.go — V5-1 review fold-in
// (colleague_merge_audit §40.35 复核): the unified-diff parser must treat
// "---"/"+++" as file headers only before the first hunk, strip exactly one
// positional prefix, and decode git's C-quoted paths — the hunk tables feed
// the PatchEffect line remap, so a mis-parsed body line silently corrupts
// every later line number.

func parseTestDiff(t *testing.T, diff string) types.PatchEffectRecord {
	t.Helper()
	return PatchEffectRecordFromUnifiedDiff("plan", "slice", "test", "base", "head", diff)
}

func TestPatchEffectParserKeepsDashDashBodyLinesAsRemovals(t *testing.T) {
	diff := "diff --git a/q.sql b/q.sql\n--- a/q.sql\n+++ b/q.sql\n@@ -1,5 +1,4 @@\n select 1;\n--- old comment\n select 2;\n select 3;\n select 4;\n"
	rec := parseTestDiff(t, diff)
	if len(rec.Files) != 1 || rec.Files[0].Path != "q.sql" || rec.Files[0].OldPath != "q.sql" {
		t.Fatalf("a removed `-- comment` line must not clobber the file header: %+v", rec.Files)
	}
	hunk := rec.Files[0].Hunks[0]
	if hunk.RemovedLines != 1 || len(hunk.RemovedLineTexts) != 1 || hunk.RemovedLineTexts[0].Line != 2 || hunk.RemovedLineTexts[0].Text != "-- old comment" {
		t.Fatalf("removed line table = %+v", hunk)
	}
	if got := types.RemapPatchEffectOldLine(&rec, "q.sql", 3); got.Status != types.PatchEffectLineMapped || got.Line != 2 {
		t.Fatalf("old line 3 must map to new line 2 after the removal: %+v", got)
	}
}

func TestPatchEffectParserCountsYamlAndTomlDelimiterLines(t *testing.T) {
	// old: ---/name: a/version: 1/+++/last: x   new: name: a/version: 2/---/last: x/+++
	diff := "diff --git a/cfg.yaml b/cfg.yaml\n--- a/cfg.yaml\n+++ b/cfg.yaml\n@@ -1,5 +1,5 @@\n----\n name: a\n-version: 1\n+version: 2\n+---\n-+++\n last: x\n++++\n"
	rec := parseTestDiff(t, diff)
	hunk := rec.Files[0].Hunks[0]
	if hunk.RemovedLines != 3 || hunk.AddedLines != 3 {
		t.Fatalf("delimiter body lines must be counted: %+v", hunk)
	}
	if got := types.RemapPatchEffectOldLine(&rec, "cfg.yaml", 2); got.Status != types.PatchEffectLineMapped || got.Line != 1 {
		t.Fatalf("surviving `name: a` (old 2) must map to new 1: %+v", got)
	}
	if got := types.RemapPatchEffectOldLine(&rec, "cfg.yaml", 5); got.Status != types.PatchEffectLineMapped || got.Line != 4 {
		t.Fatalf("surviving `last: x` (old 5) must map to new 4: %+v", got)
	}
	if got := types.RemapPatchEffectOldLine(&rec, "cfg.yaml", 1); got.Status != types.PatchEffectLineRemovedByPatch {
		t.Fatalf("the removed `---` line must be removed_by_patch: %+v", got)
	}
}

func TestPatchEffectParserStripsExactlyOnePositionalPrefix(t *testing.T) {
	diff := "diff --git a/b/main.c b/b/main.c\n--- a/b/main.c\n+++ b/b/main.c\n@@ -1,0 +1,2 @@\n+// one\n+// two\n"
	rec := parseTestDiff(t, diff)
	if len(rec.Files) != 1 || rec.Files[0].Path != "b/main.c" || rec.Files[0].OldPath != "b/main.c" {
		t.Fatalf("a top-level directory named b must survive: %+v", rec.Files)
	}
	if got := types.RemapPatchEffectOldLine(&rec, "main.c", 3); got.Status != types.PatchEffectLineFileNotInPatch {
		t.Fatalf("the untouched top-level main.c must not bind through b/main.c: %+v", got)
	}
}

func TestPatchEffectParserDecodesGitQuotedPaths(t *testing.T) {
	diff := "diff --git \"a/\\346\\226\\207\\346\\241\\243.txt\" \"b/\\346\\226\\207\\346\\241\\243.txt\"\n--- \"a/\\346\\226\\207\\346\\241\\243.txt\"\n+++ \"b/\\346\\226\\207\\346\\241\\243.txt\"\n@@ -0,0 +1,3 @@\n+x\n+y\n+z\n"
	rec := parseTestDiff(t, diff)
	if len(rec.Files) != 1 || rec.Files[0].Path != "文档.txt" || rec.Files[0].OldPath != "文档.txt" {
		t.Fatalf("quoted non-ASCII path must decode: %+v", rec.Files)
	}
	if got := types.RemapPatchEffectOldLine(&rec, "文档.txt", 3); got.Status != types.PatchEffectLineMapped || got.Line != 6 {
		t.Fatalf("old line 3 must map to 6 after three inserted lines: %+v", got)
	}
	if got := unquoteGitPath(`"tab\there \"q\" back\\slash"`); got != "tab\there \"q\" back\\slash" {
		t.Fatalf("escape decoding = %q", got)
	}
}

func TestPatchEffectParserMarksBinaryRewritesAndCountsBlankContextLines(t *testing.T) {
	binary := "diff --git a/blob.c b/blob.c\nindex 1111111..2222222 100644\nBinary files a/blob.c and b/blob.c differ\n"
	rec := parseTestDiff(t, binary)
	if len(rec.Files) != 1 || !rec.Files[0].Binary || rec.Files[0].Status != "modified" {
		t.Fatalf("binary rewrite must be marked: %+v", rec.Files)
	}
	if got := types.RemapPatchEffectOldLine(&rec, "blob.c", 1); got.Status != types.PatchEffectLineHunkUnmapped {
		t.Fatalf("a binary rewrite has no line table: %+v", got)
	}
	modeOnly := "diff --git a/run.sh b/run.sh\nold mode 100644\nnew mode 100755\n"
	rec = parseTestDiff(t, modeOnly)
	if got := types.RemapPatchEffectOldLine(&rec, "run.sh", 3); got.Status != types.PatchEffectLineMapped || got.Line != 3 {
		t.Fatalf("a mode-only change keeps every line: %+v", got)
	}
	// diff.suppressBlankEmpty=true prints blank context lines as "".
	blank := "diff --git a/f.c b/f.c\n--- a/f.c\n+++ b/f.c\n@@ -1,4 +1,5 @@\n l1\n\n-l3\n+l3x\n+l3y\n l4\n"
	rec = parseTestDiff(t, blank)
	hunk := rec.Files[0].Hunks[0]
	if hunk.RemovedLineTexts[0].Line != 3 || hunk.AddedLineNumbers[0] != 3 || hunk.AddedLineNumbers[1] != 4 {
		t.Fatalf("blank context line must occupy old/new line 2: %+v", hunk)
	}
	if got := types.RemapPatchEffectOldLine(&rec, "f.c", 4); got.Status != types.PatchEffectLineMapped || got.Line != 5 {
		t.Fatalf("`l4` (old 4) must map to new 5: %+v", got)
	}
}

func TestPatchEffectParserSplitsUnquotedSpacePaths(t *testing.T) {
	diff := "diff --git a/sp ace.c b/sp ace.c\nold mode 100644\nnew mode 100755\n"
	rec := parseTestDiff(t, diff)
	if len(rec.Files) != 1 || rec.Files[0].Path != "sp ace.c" || rec.Files[0].OldPath != "sp ace.c" {
		t.Fatalf("git's symmetric split must recover a space path on a hunk-less entry: %+v", rec.Files)
	}
	if got := splitPatchEffectDiffGitOperands("a/x b/y"); len(got) != 2 || got[0] != "a/x" || got[1] != "b/y" {
		t.Fatalf("two-token operands unchanged: %v", got)
	}
}

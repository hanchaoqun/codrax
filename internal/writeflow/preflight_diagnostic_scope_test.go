package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestFilterPreflightDiagnosticsToChangedLinesKeepsOnlyPatchTouchedLines(t *testing.T) {
	plan := &types.ChangePlan{Changes: []types.FileChange{{
		Path:  "pkg/mod.py",
		Kind:  "patch",
		Patch: "--- a/pkg/mod.py\n+++ b/pkg/mod.py\n@@ -1,4 +1,4 @@\n def old():\n-    return 1\n+    return missing\n def untouched():\n     return preexisting\n",
	}}}
	got := FilterPreflightDiagnosticsToChangedLines(plan, "pkg/mod.py", "", []PreflightDiagnostic{
		{Path: "pkg/mod.py", Line: 2, Code: "undefined_name", Message: "missing"},
		{Path: "pkg/mod.py", Line: 4, Code: "undefined_name", Message: "preexisting"},
	})
	if len(got) != 1 || got[0].Line != 2 {
		t.Fatalf("filtered diagnostics = %+v, want only changed line 2", got)
	}
}

// Bug #1+#4 regression: when a patch's @@ headers are offset from where
// git actually applied the hunk (git apply --recount / patch fuzz relocate
// by content), the changed-line set MUST come from the captured applied
// geometry (plan.PatchEffect), not the LLM header. A real diagnostic on
// the relocated line must stay in scope so the hard gates still catch it.
func TestChangedLinesPrefersAppliedGeometryOverPatchHeader(t *testing.T) {
	plan := &types.ChangePlan{
		Changes: []types.FileChange{{
			Path: "pkg/mod.py",
			Kind: "patch",
			// LLM header claims the added line is at line 2 ...
			Patch: "--- a/pkg/mod.py\n+++ b/pkg/mod.py\n@@ -1,1 +1,2 @@\n def old():\n+    return missing\n",
		}},
		// ... but git landed it at line 12 after --recount/fuzz relocation.
		PatchEffect: &types.PatchEffectRecord{Files: []types.PatchEffectFile{{
			Path:   "pkg/mod.py",
			Status: "modified",
			Hunks:  []types.PatchEffectHunk{{NewStart: 12, AddedLineNumbers: []int{12}}},
		}}},
	}
	changed, precise := ChangedLinesForPlanPath(plan, "pkg/mod.py", "")
	if !precise {
		t.Fatalf("applied geometry should yield a precise set")
	}
	if _, ok := changed[12]; !ok {
		t.Errorf("real applied line 12 missing from changed set %v", changed)
	}
	if _, ok := changed[2]; ok {
		t.Errorf("header line 2 must not be trusted when applied geometry exists: %v", changed)
	}
	// A diagnostic at the relocated line is retained (would have been
	// silently dropped when scoped against the stale @@ header).
	got := FilterPreflightDiagnosticsToChangedLines(plan, "pkg/mod.py", "", []PreflightDiagnostic{
		{Path: "pkg/mod.py", Line: 12, Code: "undefined_name", Message: "missing"},
	})
	if len(got) != 1 || got[0].Line != 12 {
		t.Fatalf("relocated diagnostic must be kept, got %+v", got)
	}
}

// Without captured geometry (apply not yet run / apply failed), the header
// fallback is preserved unchanged — the safe, byte-identical legacy path.
func TestChangedLinesFallsBackToHeaderWithoutPatchEffect(t *testing.T) {
	plan := &types.ChangePlan{Changes: []types.FileChange{{
		Path:  "pkg/mod.py",
		Kind:  "patch",
		Patch: "--- a/pkg/mod.py\n+++ b/pkg/mod.py\n@@ -1,1 +1,2 @@\n def old():\n+    return missing\n",
	}}}
	changed, precise := ChangedLinesForPlanPath(plan, "pkg/mod.py", "")
	if !precise || len(changed) == 0 {
		t.Fatalf("header fallback should still produce a precise set, got %v precise=%v", changed, precise)
	}
	if _, ok := changed[2]; !ok {
		t.Errorf("header-derived added line 2 expected in fallback set %v", changed)
	}
}

// kind=modify keeps its conservative whole-file scope even when a (minimal)
// PatchEffect is present: narrowing it to the minimal applied diff would
// move a hard gate's scope in the fail-OPEN direction for errors in
// untouched regions of a rewritten file.
func TestChangedLinesModifyKeepsWholeFileScopeDespiteGeometry(t *testing.T) {
	plan := &types.ChangePlan{
		Changes: []types.FileChange{{
			Path:       "pkg/mod.py",
			Kind:       "modify",
			NewContent: "l1\nl2\nl3\nl4\nl5\n",
		}},
		PatchEffect: &types.PatchEffectRecord{Files: []types.PatchEffectFile{{
			Path:   "pkg/mod.py",
			Status: "modified",
			Hunks:  []types.PatchEffectHunk{{NewStart: 3, AddedLineNumbers: []int{3}}},
		}}},
	}
	changed, precise := ChangedLinesForPlanPath(plan, "pkg/mod.py", "")
	if !precise {
		t.Fatalf("modify should be precise")
	}
	// Line 5 is untouched by the minimal diff but must remain in scope.
	if _, ok := changed[5]; !ok {
		t.Errorf("modify must keep whole-file scope (line 5 expected) but got %v", changed)
	}
}

func TestChangedLinesFromAppliedGeometryDeletedAndRename(t *testing.T) {
	deleted := &types.ChangePlan{PatchEffect: &types.PatchEffectRecord{Files: []types.PatchEffectFile{{
		Path: "pkg/gone.py", Status: "deleted",
	}}}}
	lines, ok := ChangedLinesFromAppliedGeometry(deleted, "pkg/gone.py")
	if !ok || len(lines) != 0 {
		t.Errorf("deleted file should match with an empty line set, got ok=%v lines=%v", ok, lines)
	}
	rename := &types.ChangePlan{PatchEffect: &types.PatchEffectRecord{Files: []types.PatchEffectFile{{
		Path: "pkg/new.py", OldPath: "pkg/old.py", Status: "renamed",
		Hunks: []types.PatchEffectHunk{{AddedLineNumbers: []int{4}}},
	}}}}
	for _, p := range []string{"pkg/new.py", "pkg/old.py"} {
		lines, ok := ChangedLinesFromAppliedGeometry(rename, p)
		if !ok {
			t.Errorf("rename should match via %s", p)
		}
		if _, has := lines[4]; !has {
			t.Errorf("rename added line 4 expected for %s, got %v", p, lines)
		}
	}
	if _, ok := ChangedLinesFromAppliedGeometry(&types.ChangePlan{}, "pkg/x.py"); ok {
		t.Errorf("nil PatchEffect must report no match (fall back to header)")
	}
}

func TestFilterPreflightDiagnosticsToChangedLinesFailsClosedWithoutPreciseSurface(t *testing.T) {
	plan := &types.ChangePlan{TargetPaths: []string{"pkg/mod.py"}}
	got := FilterPreflightDiagnosticsToChangedLines(plan, "pkg/mod.py", "", []PreflightDiagnostic{{
		Path:    "pkg/mod.py",
		Line:    8,
		Code:    "undefined_name",
		Message: "missing",
	}})
	if len(got) != 1 {
		t.Fatalf("diagnostics should be preserved without a precise patch surface, got %+v", got)
	}
}

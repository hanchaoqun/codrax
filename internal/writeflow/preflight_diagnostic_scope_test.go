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

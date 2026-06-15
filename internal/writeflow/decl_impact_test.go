package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

type fakeDeclSpans struct {
	lines map[string][]int
}

func (f *fakeDeclSpans) ExportedDeclLines(relPath string) ([]int, bool) {
	lines, ok := f.lines[relPath]
	return lines, ok
}

func bodyOnlyHeaderPatch() string {
	return `--- a/include/lib/conversions.hpp
+++ b/include/lib/conversions.hpp
@@ -40,7 +40,7 @@ inline void dump_float(char* buf)
     // body context
     int written;
-    written = snprintf(buf, len, "%.*lg", prec, value);
+    written = snprintf(buf, len, "%.*Lg", prec, value);
     // more body
     return;
 }
`
}

func declLinePatch() string {
	return `--- a/include/lib/conversions.hpp
+++ b/include/lib/conversions.hpp
@@ -38,4 +38,4 @@
 // doc
-inline void dump_float(char* buf)
+inline void dump_float(char* buf, int extra)
 {
     int written;
`
}

// The two-header literal bugfix shape: body-only patches inside exported
// functions of a public header. The analyzer flags affects_public_api, but
// the typed declaration-line corroboration finds no decl change — the plan
// must grade medium and auto-execute under auto_safe.
func TestAssessWriteRisk_BodyOnlyPublicHeaderPatchAutoExecutes(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan-header-fix",
		Changes: []types.FileChange{
			{Path: "include/lib/conversions.hpp", Kind: "patch", Patch: bodyOnlyHeaderPatch()},
			{Path: "include/lib/conversions_b.hpp", Kind: "patch", Patch: bodyOnlyHeaderPatch()},
		},
		WriteAnalysisIR: &types.WriteAnalysisIR{
			Request: types.WriteRequestModel{
				Risk: types.WriteRiskProfile{AffectsPublicAPI: true, Overall: types.RiskBandLow},
			},
		},
	}
	decls := &fakeDeclSpans{lines: map[string][]int{
		"include/lib/conversions.hpp":   {38},
		"include/lib/conversions_b.hpp": {38},
	}}
	assessment := AssessWriteRisk(AssessmentInput{Plan: plan, Decls: decls})
	if assessment.Level != RiskMedium {
		t.Fatalf("body-only patch in a public header should grade medium, got %s (%+v)", assessment.Level, assessment.Reasons)
	}
	decision := DecideWriteApproval(ApprovalPolicyAutoSafe, assessment)
	if decision.Action != ApprovalActionAutoExecute {
		t.Fatalf("auto_safe should auto-execute the medium grade, got %+v", decision)
	}
	foundUncorroborated := false
	for _, r := range assessment.Reasons {
		if r.Code == "affects_public_api_uncorroborated" {
			foundUncorroborated = true
		}
		if r.Code == "affects_public_api" && r.Level == RiskHigh {
			t.Fatalf("analyzer boolean must not contribute a high grade: %+v", r)
		}
	}
	if !foundUncorroborated {
		t.Fatalf("the analyzer's claim must stay visible as an uncorroborated medium reason: %+v", assessment.Reasons)
	}
}

// A patch that modifies the exported declaration line itself is precise typed
// evidence of API impact, but not automatically a dangerous operation. In the
// write worktree it should remain visible at medium and auto-execute under
// auto_safe unless another structural high/critical signal is present.
func TestAssessWriteRisk_DeclLinePatchAutoExecutesUnderAutoSafe(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan-signature-change",
		Changes: []types.FileChange{
			{Path: "include/lib/conversions.hpp", Kind: "patch", Patch: declLinePatch()},
		},
		WriteAnalysisIR: &types.WriteAnalysisIR{
			Request: types.WriteRequestModel{
				Risk: types.WriteRiskProfile{AffectsPublicAPI: true, Overall: types.RiskBandLow},
			},
		},
	}
	decls := &fakeDeclSpans{lines: map[string][]int{
		"include/lib/conversions.hpp": {39},
	}}
	assessment := AssessWriteRisk(AssessmentInput{Plan: plan, Decls: decls})
	if assessment.Level != RiskMedium {
		t.Fatalf("declaration-line patch should grade medium, got %s (%+v)", assessment.Level, assessment.Reasons)
	}
	found := false
	for _, r := range assessment.Reasons {
		if r.Code == "public_decl_line_changed" && r.Level == RiskMedium {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected public_decl_line_changed medium reason: %+v", assessment.Reasons)
	}
	if decision := DecideWriteApproval(ApprovalPolicyAutoSafe, assessment); decision.Action != ApprovalActionAutoExecute {
		t.Fatalf("medium declaration-line patch should auto-execute under auto_safe, got %+v", decision)
	}
}

// Declaration-line changes compose with the rest of the structural policy:
// touching an exported declaration in a dependency/build manifest remains high
// because the manifest path is the blast-radius signal.
func TestAssessWriteRisk_DeclLinePatchPlusManifestStillRequiresApproval(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan-signature-and-manifest",
		Changes: []types.FileChange{
			{Path: "include/lib/conversions.hpp", Kind: "patch", Patch: declLinePatch()},
			{Path: "pyproject.toml", Kind: "patch", Patch: "@@ -1,1 +1,1 @@\n-name = \"a\"\n+name = \"b\"\n"},
		},
		WriteAnalysisIR: &types.WriteAnalysisIR{
			Request: types.WriteRequestModel{
				Risk: types.WriteRiskProfile{AffectsPublicAPI: true, ChangesBuildSystem: true, Overall: types.RiskBandLow},
			},
		},
	}
	decls := &fakeDeclSpans{lines: map[string][]int{
		"include/lib/conversions.hpp": {39},
	}}
	assessment := AssessWriteRisk(AssessmentInput{Plan: plan, Decls: decls})
	if assessment.Level != RiskHigh {
		t.Fatalf("manifest path should keep combined plan high, got %s (%+v)", assessment.Level, assessment.Reasons)
	}
	if !hasRiskReason(assessment, "public_decl_line_changed") || !hasRiskReason(assessment, "path_class") {
		t.Fatalf("combined plan should retain both declaration and path-class evidence: %+v", assessment.Reasons)
	}
	if decision := DecideWriteApproval(ApprovalPolicyAutoSafe, assessment); decision.Action != ApprovalActionManual {
		t.Fatalf("high manifest plan must require manual approval, got %+v", decision)
	}
}

// Without a declaration source (no graph loaded), the analyzer axes stand at
// medium — no silent low, no hard high.
func TestAssessWriteRisk_NilDeclSourceDegradesToMedium(t *testing.T) {
	plan := &types.ChangePlan{
		ID:      "plan-no-graph",
		Changes: []types.FileChange{{Path: "src/lib.c", Kind: "patch", Patch: bodyOnlyHeaderPatch()}},
		WriteAnalysisIR: &types.WriteAnalysisIR{
			Request: types.WriteRequestModel{Risk: types.WriteRiskProfile{AffectsPublicAPI: true}},
		},
	}
	assessment := AssessWriteRisk(AssessmentInput{Plan: plan})
	if assessment.Level != RiskMedium {
		t.Fatalf("nil decl source should leave the medium grade, got %s", assessment.Level)
	}
}

// A file the graph cannot vouch for (ok=false) contributes nothing.
func TestAssessExportedDeclImpact_UnindexedFileIsSkipped(t *testing.T) {
	plan := &types.ChangePlan{Changes: []types.FileChange{
		{Path: "vendor/blob.c", Kind: "patch", Patch: declLinePatch()},
	}}
	a := &RiskAssessment{Level: RiskNone}
	if assessExportedDeclImpact(a, plan, &fakeDeclSpans{lines: map[string][]int{}}) {
		t.Fatal("unindexed file must not produce decl impact")
	}
}

// Structured edits carry typed line ranges directly.
func TestPlanChangeTouchedOldLines_StructuredEditsAndDefaults(t *testing.T) {
	touched := planChangeTouchedOldLines(types.FileChange{
		Kind: "patch",
		Edits: []types.StructuredEdit{
			{Kind: "replace", StartLine: 10, EndLine: 12, Content: "x\n"},
			{Kind: "insert_after", StartLine: 30, Content: "y\n"},
		},
	})
	for _, want := range []int{10, 11, 12} {
		if !touched[want] {
			t.Fatalf("replace range must mark line %d: %+v", want, touched)
		}
	}
	if touched[30] || touched[31] {
		t.Fatalf("pure insertion must not mark old lines: %+v", touched)
	}
}

// Existing precise path policy keeps its hard grades.
func TestAssessWriteRisk_PathPolicyHardGradesUnchanged(t *testing.T) {
	manifest := &types.ChangePlan{Changes: []types.FileChange{{Path: "go.mod", Kind: "patch", Patch: "@@ -1,1 +1,1 @@\n-module a\n+module b\n"}}}
	if got := AssessWriteRisk(AssessmentInput{Plan: manifest}); got.Level != RiskHigh {
		t.Fatalf("build manifest patch must stay high, got %s", got.Level)
	}
	gitPath := &types.ChangePlan{Changes: []types.FileChange{{Path: ".git/config", Kind: "modify", NewContent: "x"}}}
	if got := AssessWriteRisk(AssessmentInput{Plan: gitPath}); got.Level != RiskCritical {
		t.Fatalf(".git path must stay critical, got %s", got.Level)
	}
	migration := &types.ChangePlan{Changes: []types.FileChange{{Path: "migrations/0001_init.sql", Kind: "create", NewContent: "CREATE TABLE t();"}}}
	got := AssessWriteRisk(AssessmentInput{Plan: migration})
	if got.Level != RiskHigh {
		t.Fatalf("migration path must grade high, got %s (%+v)", got.Level, got.Reasons)
	}
}

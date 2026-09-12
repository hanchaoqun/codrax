package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1575EmitChangePlanProtectsOnlyEffectiveMultiPathCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		single     bool
		refuse     bool
	}{
		{name: "legacy_python_not_protected", path: "src/options.py"},
		{name: "native_still_protected", path: "src/client.ts", refuse: true},
		{name: "single_applied_path_keeps_original_rule", path: "src/options.py", single: true, refuse: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := passingProbeAppliedReplanContext(t)
			ctx.RepoRoot = t.TempDir()
			if err := os.MkdirAll(filepath.Join(ctx.RepoRoot, "src"), 0o755); err != nil {
				t.Fatal(err)
			}
			for path, content := range map[string]string{"src/client.ts": "export const VALUE = 41;\n", "src/options.py": "VALUE = 41\n"} {
				if err := os.WriteFile(filepath.Join(ctx.RepoRoot, path), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			plan := ctx.Mutable.ChangePlan()
			plan.AppliedCommitSHA = "applied"
			plan.TargetPaths = []string{"src/client.ts", "src/options.py"}
			plan.AppliedPaths = append([]string(nil), plan.TargetPaths...)
			plan.Changes = nil
			if tc.single {
				// Keep a single applied-path inventory independently from the
				// target set. This intentionally exercises the pre-existing arm.
				plan.AppliedPaths = []string{"src/options.py"}
			}
			report := ctx.Mutable.PlanStageProbeReports()[0]
			report.TestResults = append(report.TestResults, types.TestResult{AssertionID: "plain", Suite: "verification_probe/python", Passed: true})
			report.ChangedPathCoverage[0].Caliber = types.ChangedPathVerificationProjectRunner
			report.ChangedPathCoverage[0].Runner = "native"
			report.ChangedPathCoverage = append(report.ChangedPathCoverage, types.ChangedPathVerificationCoverage{
				Path: "src/options.py", Status: types.ChangedPathVerificationCovered,
				Caliber: types.ChangedPathVerificationProbe, Capability: types.VerificationCapabilityTargetBehavior,
				Runner: "verification_probe", Source: "plain", LanguageFamilies: []types.VerificationLanguageFamily{types.VerificationLanguagePython},
			})
			if q := qualifyNoChangeReplanForCurrentState(ctx); !q.Allowed {
				t.Fatalf("native positive must qualify the mixed current report: %+v", q)
			}
			before, err := json.Marshal([]any{plan, report})
			if err != nil {
				t.Fatal(err)
			}
			content := "VALUE = 42\n"
			if strings.HasSuffix(tc.path, ".ts") {
				content = "export const VALUE = 42;\n"
			}
			raw, err := json.Marshal(map[string]any{
				"request": "Correct the remaining value in the selected file.",
				"summary": "Update only the selected file to the expected value. Keep the independently verified file unchanged.",
				"changes": []map[string]any{{"path": tc.path, "kind": "modify", "new_content": content, "rationale": "Correct the still-unverified value."}},
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := (&EmitChangePlan{}).Execute(ctx, raw)
			if err != nil {
				t.Fatal(err)
			}
			if tc.refuse {
				if result.Success || !strings.Contains(result.Summary, "refusing a second mutation") {
					t.Errorf("existing protection lost: %+v", result)
				}
				if ctx.Mutable.ChangePlan() != plan {
					t.Error("refusal replaced prior plan")
				}
			} else if !result.Success {
				t.Errorf("legacy Python label must not freeze an unproved path: %+v", result)
			} else {
				next := ctx.Mutable.ChangePlan()
				if next == nil || len(next.Changes) != 1 || next.Changes[0].Path != tc.path || next.Changes[0].NewContent != content {
					t.Errorf("accepted model change was altered: %+v", next)
				}
			}
			after, err := json.Marshal([]any{plan, report})
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("qualification or emission rewrote the original plan/report")
			}
			for _, path := range []string{"src/client.ts", "src/options.py"} {
				body, err := os.ReadFile(filepath.Join(ctx.RepoRoot, path))
				if err != nil || !bytes.Contains(body, []byte("41")) {
					t.Fatal("proposal emission changed actual source")
				}
			}
		})
	}
}

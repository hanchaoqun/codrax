package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func passingProbeAppliedReplanContext(t *testing.T) *types.BusContext {
	t.Helper()
	ctx := newTestBusCtx()
	ctx.Mode = types.ModeApply
	ctx.PipelineStage = types.StagePlan
	ctx.Mutable.SetChangePlan(&types.ChangePlan{
		ID:           "plan-applied",
		Status:       types.PlanStatusVerifyFailed,
		AppliedPaths: []string{"src/client.ts"},
		Changes: []types.FileChange{{
			Path: "src/client.ts",
			Kind: "patch",
			Apply: &types.FileChangeApplyRecord{
				Status: "applied",
			},
		}},
	})
	ctx.Mutable.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{
		PlanID:      "plan-applied",
		BatchID:     "batch-1",
		Attempt:     1,
		FailureKind: types.FailureKindTestsFailed,
	})
	ctx.Mutable.AppendPlanStageProbeReport(&types.ChangeReport{
		PlanID:             "plan-applied",
		Channel:            types.ChangeReportChannelPlannerProbe,
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
		TestResults: []types.TestResult{{
			AssertionID: "search-client-contract",
			Kind:        types.TestResultKindUnit,
			Passed:      true,
		}},
	})
	return ctx
}

func TestPassingProbeReplanRejectsSecondMutationOfAppliedPath(t *testing.T) {
	ctx := passingProbeAppliedReplanContext(t)
	rej, paths := validatePassingProbeReplanAppliedPathMutation(ctx, []types.FileChange{{
		Path: "./src/client.ts",
		Kind: "patch",
	}})
	if rej == "" {
		t.Fatal("expected second mutation of typed applied path to be rejected")
	}
	for _, want := range []string{"latest bounded planner probe passed", "src/client.ts", "changes: []", types.PlanStatusNoChangeRequired} {
		if !strings.Contains(rej, want) {
			t.Fatalf("rejection missing %q: %s", want, rej)
		}
	}
	if len(paths) != 1 || paths[0] != "src/client.ts" {
		t.Fatalf("repair paths = %#v, want applied path", paths)
	}
}

func TestPlanFullContentWiresPassingProbeAppliedPathGate(t *testing.T) {
	ctx := passingProbeAppliedReplanContext(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "client.ts"), []byte("export class Client {}\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	ctx.RepoRoot = root
	rej, pack := validatePlanFullContentWithRepair(ctx, "emit_change_plan", "second mutation", []types.FileChange{{
		Path:       "src/client.ts",
		Kind:       "modify",
		NewContent: "export class Client { value = 1; }\n",
		Rationale:  "try a second edit after the scoped probe already passed",
	}}, nil)
	if rej == "" || pack == nil {
		t.Fatalf("shared full-content seam must reject and emit typed repair: rej=%q pack=%+v", rej, pack)
	}
	if pack.ReasonCode != "replan_passing_probe_applied_path_mutation" {
		t.Fatalf("repair reason = %q, want typed gate reason; pack=%+v", pack.ReasonCode, pack)
	}
}

func TestPassingProbeReplanAllowsDifferentPath(t *testing.T) {
	ctx := passingProbeAppliedReplanContext(t)
	if rej, paths := validatePassingProbeReplanAppliedPathMutation(ctx, []types.FileChange{{
		Path: "tests/client.test.ts",
		Kind: "create",
	}}); rej != "" || len(paths) != 0 {
		t.Fatalf("a distinct path must remain available for a distinct fix: rej=%q paths=%v", rej, paths)
	}
}

func TestPassingProbeReplanDoesNotFreezeUncoveredPathsInMultiFilePlan(t *testing.T) {
	ctx := passingProbeAppliedReplanContext(t)
	plan := ctx.Mutable.ChangePlan()
	plan.AppliedPaths = append(plan.AppliedPaths, "src/options.ts")
	plan.Changes = append(plan.Changes, types.FileChange{
		Path:  "src/options.ts",
		Kind:  "patch",
		Apply: &types.FileChangeApplyRecord{Status: "applied"},
	})
	if rej, _ := validatePassingProbeReplanAppliedPathMutation(ctx, []types.FileChange{{Path: "src/options.ts", Kind: "patch"}}); rej != "" {
		t.Fatalf("an unbound probe must not freeze every path in a multi-file plan: %s", rej)
	}
	latest := ctx.Mutable.PlanStageProbeReports()
	latest[len(latest)-1].ChangedPathCoverage = []types.ChangedPathVerificationCoverage{{
		Path:   "src/options.ts",
		Status: types.ChangedPathVerificationCovered,
	}}
	if rej, _ := validatePassingProbeReplanAppliedPathMutation(ctx, []types.FileChange{{Path: "src/options.ts", Kind: "patch"}}); rej == "" {
		t.Fatal("exact typed covered path in a multi-file plan should be protected")
	}
}

func TestPassingProbeReplanLatestFailingProbeReopensAppliedPath(t *testing.T) {
	ctx := passingProbeAppliedReplanContext(t)
	ctx.Mutable.AppendPlanStageProbeReport(&types.ChangeReport{
		PlanID:             "plan-applied",
		Channel:            types.ChangeReportChannelPlannerProbe,
		Passed:             false,
		VerificationStatus: types.VerificationStatusFailed,
		FailureKind:        types.FailureKindTestsFailed,
		TestResults: []types.TestResult{{
			AssertionID: "distinct-remaining-contract",
			Kind:        types.TestResultKindUnit,
			Passed:      false,
		}},
	})
	if rej, _ := validatePassingProbeReplanAppliedPathMutation(ctx, []types.FileChange{{Path: "src/client.ts", Kind: "patch"}}); rej != "" {
		t.Fatalf("latest failing typed probe must re-open the applied path: %s", rej)
	}
}

func TestPassingProbeReplanRequiresTypedAppliedWork(t *testing.T) {
	ctx := passingProbeAppliedReplanContext(t)
	ctx.Mutable.SetChangePlan(&types.ChangePlan{ID: "plan-applied", Status: types.PlanStatusVerifyFailed})
	if rej, _ := validatePassingProbeReplanAppliedPathMutation(ctx, []types.FileChange{{Path: "src/client.ts", Kind: "patch"}}); rej != "" {
		t.Fatalf("probe without typed applied work must not suppress an edit: %s", rej)
	}
}

func TestNewSourceDelimiterImbalanceRejectsCorruptedTypeScriptReplanShape(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "src", "client.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	original := "export class Client {\n  async textSearch() {\n    return fetch('/v1/search');\n  }\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	planned := "export class Client {\n  async textSearch() {\n    return fetch('/v1/search');\n  }\n  return res.json();\n}\n}\n"
	ctx := newTestBusCtx()
	ctx.RepoRoot = root
	rej, paths := validateNewSourceDelimiterImbalance(ctx, []types.FileChange{{
		Path:       "src/client.ts",
		Kind:       "modify",
		NewContent: planned,
	}})
	if rej == "" || !strings.Contains(rej, "unexpected \"}\"") {
		t.Fatalf("expected definite extra brace rejection, got %q", rej)
	}
	if len(paths) != 1 || paths[0] != "src/client.ts" {
		t.Fatalf("repair paths = %#v", paths)
	}
}

func TestSourceDelimiterFallbackCoversBraceLanguageMatrix(t *testing.T) {
	exts := []string{
		".c", ".cc", ".cpp", ".cxx", ".h", ".hpp", ".m", ".mm", ".cu",
		".go", ".java", ".kt", ".kts", ".rs", ".swift", ".js", ".jsx", ".mjs", ".cjs",
		".ts", ".tsx", ".ets", ".cj", ".cs", ".php", ".scala", ".dart",
	}
	for _, ext := range exts {
		t.Run(strings.TrimPrefix(ext, "."), func(t *testing.T) {
			root := t.TempDir()
			name := "sample" + ext
			if err := os.WriteFile(filepath.Join(root, name), []byte("unit { call(); }\n"), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			ctx := newTestBusCtx()
			ctx.RepoRoot = root
			rej, _ := validateNewSourceDelimiterImbalance(ctx, []types.FileChange{{
				Path:       name,
				Kind:       "modify",
				NewContent: "unit { call(); } }\n",
			}})
			if rej == "" {
				t.Fatalf("extension %s did not receive delimiter fallback", ext)
			}
		})
	}
}

func TestSourceDelimiterFallbackIgnoresLiteralsCommentsAndRegex(t *testing.T) {
	original := `export function sample() {
  const quoted = "}";
  const template = ` + "`" + `{ ignored: true }` + "`" + `;
  const regex = /[{]literal[}]/;
  // }
  /* { nested /* } */ still ignored } */
  return quoted;
}
`
	scan := scanSourceDelimiters(original, ".ts")
	if !scan.Certain || !scan.Balanced {
		t.Fatalf("valid literals/comments/regex should remain balanced: %+v", scan)
	}
}

func TestSourceDelimiterFallbackFailsOpenForLegacyOrLexicallyUncertainSource(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "legacy.ts"), []byte("export function legacy() {\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	ctx := newTestBusCtx()
	ctx.RepoRoot = root
	rej, _ := validateNewSourceDelimiterImbalance(ctx, []types.FileChange{{
		Path:       "legacy.ts",
		Kind:       "modify",
		NewContent: "export function legacy() {\n}\n}\n",
	}})
	if rej != "" {
		t.Fatalf("already-unbalanced legacy source must fail open: %s", rej)
	}
	uncertain := scanSourceDelimiters("export const value = \"unterminated\n}\n", ".ts")
	if uncertain.Certain {
		t.Fatalf("unterminated literal must be lexically uncertain: %+v", uncertain)
	}
}
